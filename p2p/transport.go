package p2p

import "net"

// Peer is an interface that represents a remote node in the network.
type Peer interface {
	net.Conn
	Send([]byte) error
}

// Transport is an an interface that represents a transport layer.
type Transport interface {
	Dial(string) error
	ListenAndAccept() error
	Consume() <-chan RPC
	Close() error
}
