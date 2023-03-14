package main

import (
	"fmt"
	"log"

	"github.com/webstradev/peerstra/p2p"
)

func OnPeer(peer p2p.Peer) error {
	fmt.Println("OnPeer called")
	// peer.Close()
	return nil
}

func main() {
	tcpOpts := p2p.TCPTransportOpts{
		ListenAddr:    ":3000",
		HandShakeFunc: p2p.NOPHandshakeFunc,
		Decoder:       p2p.NewDefaultDecoder(),
		OnPeer:        OnPeer,
	}
	tr := p2p.NewTCPTransport(tcpOpts)

	go func() {
		for rpc := range tr.Consume() {
			log.Printf("Received RPC from %s: %s", rpc.From, rpc.Payload)
		}
	}()

	if err := tr.ListenAndAccept(); err != nil {
		log.Fatal(err)
	}

	select {}
}
