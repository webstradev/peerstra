package p2p

import (
	"fmt"
	"net"
	"sync"
)

// TCPPeer represent the remote node over a TCP established connection
type TCPPeer struct {
	// The connection to the remote node
	conn net.Conn

	// if we dial a connection => outbound = true
	// if we accept an incoming connection => outbound = false
	outbound bool
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		conn:     conn,
		outbound: outbound,
	}
}

// Close implements the peer implements the Peer interface
func (p *TCPPeer) Close() error {
	return p.conn.Close()
}

type TCPTransportOpts struct {
	ListenAddr    string
	HandShakeFunc HandShakeFunc
	Decoder       Decoder
	OnPeer        func(Peer) error
}

type TCPTransport struct {
	TCPTransportOpts
	listener net.Listener
	rpcChan  chan RPC

	mu sync.RWMutex
}

func NewTCPTransport(opts TCPTransportOpts) *TCPTransport {
	return &TCPTransport{
		TCPTransportOpts: opts,
		rpcChan:          make(chan RPC),
	}
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error

	t.listener, err = net.Listen("tcp", t.ListenAddr)
	if err != nil {
		return err
	}

	go t.startAcceptLoop()

	return nil
}

// Consume implements the Transport interface which will return a
// read-only channel for reading the incoming RPCs from another peer
func (t *TCPTransport) Consume() <-chan RPC {
	return t.rpcChan
}

func (t *TCPTransport) startAcceptLoop() error {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			fmt.Printf("TCP accept error: \n%v", err)
		}

		fmt.Printf("new incoming tcp connection %+v\n", conn.RemoteAddr())

		go t.handleConn(conn)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn) {
	var err error
	defer func() {
		fmt.Printf("dropping peer connection: %v\n", err)
		conn.Close()
	}()

	peer := NewTCPPeer(conn, true)

	if err = t.HandShakeFunc(peer); err != nil {
		fmt.Printf("TCP handshake error: %v\n", err)
		conn.Close()
		return
	}

	if t.OnPeer != nil {
		if err = t.OnPeer(peer); err != nil {
			conn.Close()
			return
		}
	}

	// Read loop
	rpc := RPC{}
	for {
		if err = t.Decoder.Decode(conn, &rpc); err != nil {
			return
		}

		rpc.From = conn.RemoteAddr()
		t.rpcChan <- rpc
	}
}
