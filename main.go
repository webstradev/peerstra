package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/webstradev/peerstra/p2p"
)

func makeServer(listenAddr string, nodes ...string) *FileServer {
	tcpTransportOpts := p2p.TCPTransportOpts{
		ListenAddr: listenAddr,
	}

	t := p2p.NewTCPTransport(tcpTransportOpts)

	fileServerOpts := FileServerOpts{
		EncKey:            newEncryptionKey(),
		ListenAddr:        listenAddr,
		StorageRoot:       fmt.Sprintf("%s_network", listenAddr),
		PathTransformFunc: CASPathTransformFunc,
		Transport:         t,
		BootstrapNodes:    nodes,
	}

	s := NewFileServer(fileServerOpts)

	t.OnPeer = s.OnPeer

	return s
}

func main() {
	s1 := makeServer(":3000")
	s2 := makeServer(":4000", ":3000")
	s3 := makeServer(":5000", ":3000", ":4000")

	go func() {
		log.Fatal(s1.Start())
	}()

	go func() {
		time.Sleep(500 * time.Millisecond)
		log.Fatal(s2.Start())
	}()

	go func() {
		time.Sleep(1000 * time.Millisecond)
		log.Fatal(s3.Start())
	}()

	time.Sleep(1 * time.Second)
	for i := 0; i < 1; i++ {
		key := fmt.Sprintf("picture_%d.png", i)

		data := bytes.NewReader([]byte("my big data file here!"))

		err := s2.Store(key, data)
		if err != nil {
			log.Fatal(err)
		}
		time.Sleep(1 * time.Millisecond)

		if err := s2.store.Delete(s2.ID, key); err != nil {
			log.Fatal(err)
		}

		r, err := s2.Get(key)
		if err != nil {
			log.Fatal(err)
		}

		b, err := io.ReadAll(r)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(b))
	}

	select {}
}
