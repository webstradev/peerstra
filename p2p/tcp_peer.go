package p2p

import (
	"net"
	"sync"
)

// TCPPeer represent the remote node over a TCP established connection
type TCPPeer struct {
	// The underlying TCP connection to the remote node
	net.Conn

	// if we dial a connection => outbound = true
	// if we accept an incoming connection => outbound = false
	outbound bool

	wg *sync.WaitGroup
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		Conn:     conn,
		outbound: outbound,
		wg:       &sync.WaitGroup{},
	}
}

// CloseStream implements the peer interface and closes a file stream
func (p *TCPPeer) CloseStream() {
	p.wg.Done()
}

// Send implements the peer implements the Peer interface
// and sends the given bytes to the remote node
func (p *TCPPeer) Send(b []byte) error {
	_, err := p.Conn.Write(b)
	return err
}
