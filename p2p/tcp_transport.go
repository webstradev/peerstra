package p2p

import (
	"fmt"
	"io"
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

type TCPTransportOpts struct {
	ListenAddr    string
	HandShakeFunc HandShakeFunc
	Decoder       Decoder
}

type TCPTransport struct {
	TCPTransportOpts
	listener net.Listener

	mu    sync.RWMutex
	peers map[net.Addr]Peer
}

func NewTCPTransport(opts TCPTransportOpts) *TCPTransport {
	return &TCPTransport{
		TCPTransportOpts: opts,
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
	peer := NewTCPPeer(conn, true)

	if err := t.HandShakeFunc(peer); err != nil {
		fmt.Printf("TCP handshake error: %v\n", err)
		conn.Close()
		return
	}

	msg := &Message{}
	for {
		if err := t.Decoder.Decode(conn, msg); err != nil {
			if err == io.EOF {
				fmt.Printf("TCP connection closed: %v\n", conn.RemoteAddr())
				conn.Close()
				return
			}
			fmt.Printf("TCP decode error: %v\n", err)
			continue
		}

		msg.From = conn.RemoteAddr()

		fmt.Printf("from %+v: %+v", msg.From.String(), string(msg.Payload))
	}
}
