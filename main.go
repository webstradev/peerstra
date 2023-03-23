package main

import (
	"bytes"
	"fmt"
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

	go func() {
		log.Fatal(s1.Start())
	}()

	go func() {
		time.Sleep(500 * time.Millisecond)
		log.Fatal(s2.Start())
	}()

	time.Sleep(1 * time.Second)

	data := bytes.NewReader([]byte("my big data file here!"))

	err := s2.StoreFile("myprivatedata", data)
	if err != nil {
		log.Fatal(err)
	}

	select {}
}
