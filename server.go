package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/webstradev/peerstra/p2p"
)

type FileServerOpts struct {
	EncKey            []byte
	ListenAddr        string
	StorageRoot       string
	PathTransformFunc PathTransformFunc
	Transport         p2p.Transport
	BootstrapNodes    []string
}

type FileServer struct {
	FileServerOpts

	peerLock sync.Mutex
	peers    map[string]p2p.Peer

	store    *Store
	quitChan chan struct{}
}

func NewFileServer(opts FileServerOpts) *FileServer {
	storeOpts := StoreOpts{
		Root:              opts.StorageRoot,
		PathTransformFunc: opts.PathTransformFunc,
	}

	return &FileServer{
		FileServerOpts: opts,
		store:          NewStore(storeOpts),
		quitChan:       make(chan struct{}),
		peerLock:       sync.Mutex{},
		peers:          make(map[string]p2p.Peer),
	}
}

func (fs *FileServer) Stop() {
	close(fs.quitChan)
}

type Message struct {
	From    string
	Payload any
}

type MessageStoreFile struct {
	Key  string
	Size int64
}

func (fs *FileServer) broadcast(msg *Message) error {
	buf := bytes.NewBuffer(nil)
	if err := gob.NewEncoder(buf).Encode(msg); err != nil {
		return err
	}

	for _, peer := range fs.peers {
		peer.Send([]byte{p2p.IncomingMessage})
		if err := peer.Send([]byte(buf.Bytes())); err != nil {
			return err
		}
	}

	return nil
}

func (fs *FileServer) stream(msg *Message) error {
	// Map peers to one multiwriter
	peers := []io.Writer{}
	for _, peer := range fs.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)

	// Encode payload to multiwriter(all peers)
	return gob.NewEncoder(mw).Encode(msg)
}

type MessageGetFile struct {
	Key string
}

func (fs *FileServer) Get(key string) (io.Reader, error) {
	if fs.store.Has(key) {
		fmt.Printf("[%s] file found locally, returning...\n", fs.Transport.Addr())
		_, r, err := fs.store.Read(key)
		return r, err
	}

	fmt.Printf("[%s] file not found locally, requesting from peers...\n", fs.Transport.Addr())

	msg := &Message{
		From: fs.ListenAddr,
		Payload: MessageGetFile{
			Key: hashKey(key),
		},
	}

	if err := fs.broadcast(msg); err != nil {
		return nil, err
	}

	time.Sleep(5 * time.Millisecond)

	for _, peer := range fs.peers {
		// Read file size to limit # bytes that we read from the connection
		var fileSize int64

		if err := binary.Read(peer, binary.LittleEndian, &fileSize); err != nil {
			return nil, err
		}

		n, err := fs.store.WriteDecrypt(fs.EncKey, key, io.LimitReader(peer, fileSize))
		if err != nil {
			return nil, err
		}

		fmt.Printf("[%s] received %d bytes over the network from %s\n", fs.Transport.Addr(), n, peer.RemoteAddr())

		peer.CloseStream()
	}

	_, r, err := fs.store.Read(key)
	return r, err
}

type MessageDeleteFile struct {
	Key string
}

func (fs *FileServer) Delete(key string) error {
	fmt.Printf("[%s] deleting file (%s) locally...\n", fs.Transport.Addr(), key)
	if err := fs.store.Delete(key); err != nil {
		return err
	}

	// Broadcast to peers
	msg := &Message{
		From: fs.ListenAddr,
		Payload: MessageDeleteFile{
			Key: key,
		},
	}

	fmt.Printf("[%s] broadcasting delete message to peers...\n", fs.Transport.Addr())
	if err := fs.broadcast(msg); err != nil {
		return err
	}

	return nil
}

func (fs *FileServer) Store(key string, r io.Reader) error {
	fileBuffer := bytes.NewBuffer(nil)
	tee := io.TeeReader(r, fileBuffer)

	size, err := fs.store.Write(key, tee)
	if err != nil {
		return err
	}

	msg := &Message{
		From: fs.ListenAddr,
		Payload: MessageStoreFile{
			Key:  hashKey(key),
			Size: size + 16,
		},
	}

	if err := fs.broadcast(msg); err != nil {
		return err
	}

	time.Sleep(5 * time.Millisecond)

	peers := []io.Writer{}

	for _, peer := range fs.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)
	mw.Write([]byte{p2p.IncomingStream})

	n, err := copyEncrypt(fs.EncKey, fileBuffer, mw)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] received and written %d to disk\n", fs.Transport.Addr(), n)

	return nil
}

func (fs *FileServer) OnPeer(p p2p.Peer) error {
	fs.peerLock.Lock()
	defer fs.peerLock.Unlock()

	fs.peers[p.RemoteAddr().String()] = p

	log.Printf("%s connected with remote %s", fs.ListenAddr, p.RemoteAddr())
	log.Printf("peers for %s: %+v", fs.ListenAddr, fs.peers)
	return nil
}

func (fs *FileServer) loop() {
	defer func() {
		log.Println("file server stopped")
		fs.Transport.Close()
	}()

	for {
		select {
		case rpc := <-fs.Transport.Consume():
			var msg Message
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&msg); err != nil {
				log.Println(err)
				continue
			}

			if err := fs.handleMessage(rpc.From, &msg); err != nil {
				log.Println(err)
				continue
			}
		case <-fs.quitChan:
			return
		}
	}
}

func (fs *FileServer) handleMessage(from string, msg *Message) error {
	switch v := msg.Payload.(type) {
	case MessageStoreFile:
		return fs.handleMessageStoreFile(from, &v)
	case MessageGetFile:
		return fs.handleMessageGetFile(from, &v)
	case MessageDeleteFile:
		return fs.handleMessageDeleteFile(from, &v)
	default:
		return fmt.Errorf("unknown message type: %T", v)
	}
}

func (fs *FileServer) handleMessageGetFile(from string, msg *MessageGetFile) error {
	fmt.Printf("[%s] received request for file (%s) from peer: %s\n", fs.Transport.Addr(), msg.Key, from)
	if !fs.store.Has(msg.Key) {
		return fmt.Errorf("[]%s file (%s) not found locally", fs.Transport.Addr(), msg.Key)
	}

	fmt.Printf("[%s] serving file (%s) over the network\n", fs.Transport.Addr(), msg.Key)

	size, r, err := fs.store.Read(msg.Key)
	if err != nil {
		return err
	}

	// If the reader is a ReadCloser, close it when we're done
	if rc, ok := r.(io.ReadCloser); ok {
		defer rc.Close()
	}

	peer, ok := fs.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	// First send Incoming stream byte then send the filesize as int64
	peer.Send([]byte{p2p.IncomingStream})
	binary.Write(peer, binary.LittleEndian, size)

	n, err := io.Copy(peer, r)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] written %d bytes over the network to peer: %s\n", fs.Transport.Addr(), n, from)

	return nil
}

func (fs *FileServer) handleMessageStoreFile(from string, msg *MessageStoreFile) error {
	peer, ok := fs.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	n, err := fs.store.Write(msg.Key, io.LimitReader(peer, msg.Size))
	if err != nil {
		return err
	}

	fmt.Printf("[%s] received and written byte to disk: %d\n", fs.Transport.Addr(), n)

	peer.CloseStream()

	return nil
}

func (fs *FileServer) handleMessageDeleteFile(from string, msg *MessageDeleteFile) error {
	fmt.Printf("[%s] received delete message from peer: %s\n", fs.Transport.Addr(), from)
	if err := fs.store.Delete(msg.Key); err != nil {
		return err
	}

	fmt.Printf("[%s] deleted file (%s) from disk\n", fs.Transport.Addr(), msg.Key)
	return nil
}

func (fs *FileServer) bootstrapNetwork() error {
	for _, addr := range fs.BootstrapNodes {
		go func(addr string) {
			if err := fs.Transport.Dial(addr); err != nil {
				log.Printf("failed to dial bootstrap node: %s", err)
			}
		}(addr)
	}
	return nil
}

func (fs *FileServer) Start() error {
	if err := fs.Transport.ListenAndAccept(); err != nil {
		return err
	}

	if len(fs.BootstrapNodes) > 0 {
		fs.bootstrapNetwork()
	}

	fs.loop()

	return nil
}

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
	gob.Register(MessageDeleteFile{})
}
