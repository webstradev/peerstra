package p2p

import (
	"errors"
	"fmt"
	"io"
	"log"
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

// RemoteAddr implements the peer implements the Peer interface
// It returns the remote address of the peer's underlying connection
func (p *TCPPeer) RemoteAddr() net.Addr {
	return p.conn.RemoteAddr()
}

// Send implements the peer implements the Peer interface
// and sends the given bytes to the remote node
func (p *TCPPeer) Send(b []byte) error {
	_, err := p.conn.Write(b)
	return err
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
	t := &TCPTransport{
		TCPTransportOpts: opts,
		rpcChan:          make(chan RPC),
	}

	if t.Decoder == nil {
		t.Decoder = NewDefaultDecoder()
	}

	if t.HandShakeFunc == nil {
		t.HandShakeFunc = NOPHandshakeFunc
	}

	return t
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error

	t.listener, err = net.Listen("tcp", t.ListenAddr)
	if err != nil {
		return err
	}

	go t.startAcceptLoop()

	log.Printf("TCP transport listening on port: %s", t.ListenAddr)

	return nil
}

// Close implements the Transport interface.
func (t *TCPTransport) Close() error {
	return t.listener.Close()
}

// Dial implments the Transport interface which will
// dial a remote node and handle the connection
func (t *TCPTransport) Dial(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	peer := NewTCPPeer(conn, true)

	if err = t.HandShakeFunc(peer); err != nil {
		return err
	}

	if t.OnPeer != nil {
		if err = t.OnPeer(peer); err != nil {
			return err
		}
	}

	go t.handleConn(conn, true)

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
		if errors.Is(err, net.ErrClosed) {
			return nil
		}

		if err != nil {
			fmt.Printf("TCP accept error: \n%v", err)
		}

		go t.handleConn(conn, false)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn, outbound bool) {
	var err error
	defer func() {
		if errors.Is(err, io.EOF) {
			fmt.Printf("connection closed by peer: %v\n", conn.RemoteAddr())
			conn.Close()
			return
		}

		fmt.Printf("dropping peer connection: %v\n", err)
		conn.Close()
	}()

	peer := NewTCPPeer(conn, outbound)

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
