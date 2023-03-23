package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

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
	peers    map[net.Addr]p2p.Peer

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
		peers:          make(map[net.Addr]p2p.Peer),
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
	Key string
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
	msg := &Message{
		From: fs.ListenAddr,
		Payload: MessageStoreFile{
			Key: key,
		},
	}

	if err := gob.NewEncoder(buf).Encode(msg); err != nil {
		return err
	}

	for _, peer := range fs.peers {
		if err := peer.Send([]byte(buf.Bytes())); err != nil {
			return err
		}
	}

	// time.Sleep(1 * time.Second)

	// payload := []byte("THIS LARGE FILE")

	// for _, peer := range fs.peers {
	// 	if err := peer.Send(payload); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

func (fs *FileServer) OnPeer(p p2p.Peer) error {
	fs.peerLock.Lock()
	defer fs.peerLock.Unlock()

	fs.peers[p.RemoteAddr()] = p

	log.Printf("connected with remote %s", p.RemoteAddr())
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

			fmt.Printf("%+v\n", msg.Payload)

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

// func (fs *FileServer) handleMessage(msg *Message) error {
// 	switch v := msg.Payload.(type) {
// 	case *DataMessage:
// 		fmt.Printf("received data message from %s: %s\n", msg.From, v.Key)
// 		return nil
// 	}

// 	return nil
// }

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
