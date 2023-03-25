package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/webstradev/peerstra/p2p"
)

type FileServerOpts struct {
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
		return fs.store.Read(key)
	}

	fmt.Printf("file not found locally, broadcasting to peers...\n")

	msg := &Message{
		From: fs.ListenAddr,
		Payload: MessageGetFile{
			Key: key,
		},
	}

	if err := fs.broadcast(msg); err != nil {
		return nil, err
	}

	time.Sleep(1 * time.Second)

	for _, peer := range fs.peers {
		fileBuffer := bytes.Buffer{}
		n, err := io.Copy(&fileBuffer, peer)
		if err != nil {
			return nil, err
		}
		fmt.Println("received", n, "bytes")
		fmt.Println(fileBuffer.String())
	}

	select {}

	return nil, fmt.Errorf("file not found")
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
			Key:  key,
			Size: size,
		},
	}

	if err := fs.broadcast(msg); err != nil {
		return err
	}

	// TODO (@webstradev): use a multi writer
	time.Sleep(1 * time.Second)

	for _, peer := range fs.peers {
		n, err := io.Copy(peer, fileBuffer)
		if err != nil {
			return err
		}

		fmt.Println("received and written byte to disk: ", n)
	}

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
	default:
		return fmt.Errorf("unknown message type: %T", v)
	}
}

func (fs *FileServer) handleMessageGetFile(from string, msg *MessageGetFile) error {
	if !fs.store.Has(msg.Key) {
		return fmt.Errorf("file (%s) not found locally", msg.Key)
	}

	r, err := fs.store.Read(msg.Key)
	if err != nil {
		return err
	}

	peer, ok := fs.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	n, err := io.Copy(peer, r)
	if err != nil {
		return err
	}

	fmt.Printf("wrote %d bytes to peer: %s\n", n, from)

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

	fmt.Printf("received and written byte to disk: %d\n", n)

	peer.(*p2p.TCPPeer).Wg.Done()

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
}
