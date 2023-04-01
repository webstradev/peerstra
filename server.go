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
	ID                string
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

	if opts.ID == "" {
		opts.ID = generateID()
	}

	return &FileServer{
		FileServerOpts: opts,
		store:          NewStore(storeOpts),
		quitChan:       make(chan struct{}),
		peerLock:       sync.Mutex{},
		peers:          make(map[string]p2p.Peer),
	}
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

func (fs *FileServer) Stop() {
	close(fs.quitChan)
}

func (fs *FileServer) Get(key string) (io.Reader, error) {
	if fs.store.Has(fs.ID, key) {
		fmt.Printf("[%s] file found locally, returning...\n", fs.Transport.Addr())
		_, r, err := fs.store.Read(fs.ID, key)
		return r, err
	}

	fmt.Printf("[%s] file not found locally, requesting from peers...\n", fs.Transport.Addr())

	msg := &Message{
		From: fs.ListenAddr,
		Payload: MessageGetFile{
			ID:  fs.ID,
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

		n, err := fs.store.WriteDecrypt(fs.EncKey, fs.ID, key, io.LimitReader(peer, fileSize))
		if err != nil {
			return nil, err
		}

		fmt.Printf("[%s] received %d bytes over the network from %s\n", fs.Transport.Addr(), n, peer.RemoteAddr())

		peer.CloseStream()
	}

	_, r, err := fs.store.Read(fs.ID, key)
	return r, err
}

func (fs *FileServer) Delete(key string) error {
	fmt.Printf("[%s] deleting file (%s) locally...\n", fs.Transport.Addr(), key)
	if err := fs.store.Delete(fs.ID, key); err != nil {
		return err
	}

	// Broadcast to peers
	msg := &Message{
		From: fs.ListenAddr,
		Payload: MessageDeleteFile{
			ID:  fs.ID,
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

	size, err := fs.store.Write(fs.ID, key, tee)
	if err != nil {
		return err
	}

	msg := &Message{
		From: fs.ListenAddr,
		Payload: MessageStoreFile{
			ID:   fs.ID,
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
