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
	// Map peers to one multiwriter
	peers := []io.Writer{}
	for _, peer := range fs.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)

	// Encode payload to multiwriter(all peers)
	return gob.NewEncoder(mw).Encode(msg)
}

func (fs *FileServer) StoreFile(key string, r io.Reader) error {
	buf := bytes.NewBuffer(nil)
	tee := io.TeeReader(r, buf)

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

	msgBuf := bytes.NewBuffer(nil)
	if err := gob.NewEncoder(msgBuf).Encode(msg); err != nil {
		return err
	}

	for _, peer := range fs.peers {
		if err := peer.Send([]byte(msgBuf.Bytes())); err != nil {
			return err
		}
	}

	time.Sleep(1 * time.Second)

	for _, peer := range fs.peers {
		n, err := io.Copy(peer, buf)
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

			peer, ok := fs.peers[rpc.From]
			if !ok {
				panic("peer not found")
			}

			b := make([]byte, 1024)
			_, err := peer.Read(b)
			if err != nil {
				panic(err)
			}

			fmt.Printf("received data: %s\n", string(b))

			peer.(*p2p.TCPPeer).Wg.Done()

		case <-fs.quitChan:
			return
		}
	}
}

func (fs *FileServer) handleMessage(from string, msg *Message) error {
	switch v := msg.Payload.(type) {
	case MessageStoreFile:
		return fs.handleMessageStoreFile(from, &v)
	default:
		return fmt.Errorf("unknown message type: %T", v)
	}
}

func (fs *FileServer) handleMessageStoreFile(from string, msg *MessageStoreFile) error {
	peer, ok := fs.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	if _, err := fs.store.Write(msg.Key, io.LimitReader(peer, msg.Size)); err != nil {
		return err
	}

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
}
