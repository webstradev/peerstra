package p2p

import "net"

// Message holds any arbitrary data that is being sent over
// each transport between two nodes in the networ
type Message struct {
	From    net.Addr
	Payload []byte
}
