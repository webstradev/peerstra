package p2p

import "net"

// Peer is an interface that represents a remote node in the network.
type Peer interface {
	net.Conn
	Send([]byte) error
	CloseStream()
}
