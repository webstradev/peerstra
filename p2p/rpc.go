package p2p

import "net"

// RPC holds any arbitrary data that is being sent over
// each transport between two nodes in the networ
type RPC struct {
	From    net.Addr
	Payload []byte
}
