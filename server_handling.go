package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"

	"github.com/webstradev/peerstra/p2p"
)

type Message struct {
	From    string
	Payload any
}

type MessageStoreFile struct {
	ID   string
	Key  string
	Size int64
}

type MessageGetFile struct {
	ID  string
	Key string
}

type MessageDeleteFile struct {
	ID  string
	Key string
}

func (fs *FileServer) handleMessage(from string, msg *Message) error {
	switch v := msg.Payload.(type) {
	case MessageStoreFile:
		return fs.handleMessageStoreFile(from, &v)
	case MessageGetFile:
		return fs.handleMessageGetFile(from, &v)
	case MessageDeleteFile:
		return fs.handleMessageDeleteFile(from, &v)
	default:
		return fmt.Errorf("unknown message type: %T", v)
	}
}

func (fs *FileServer) handleMessageGetFile(from string, msg *MessageGetFile) error {
	fmt.Printf("[%s] received request for file (%s) from peer: %s\n", fs.Transport.Addr(), msg.Key, from)
	if !fs.store.Has(msg.ID, msg.Key) {
		return fmt.Errorf("[]%s file (%s) not found locally", fs.Transport.Addr(), msg.Key)
	}

	fmt.Printf("[%s] serving file (%s) over the network\n", fs.Transport.Addr(), msg.Key)

	size, r, err := fs.store.Read(msg.ID, msg.Key)
	if err != nil {
		return err
	}

	// If the reader is a ReadCloser, close it when we're done
	if rc, ok := r.(io.ReadCloser); ok {
		defer rc.Close()
	}

	peer, ok := fs.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	// First send Incoming stream byte then send the filesize as int64
	peer.Send([]byte{p2p.IncomingStream})
	binary.Write(peer, binary.LittleEndian, size)

	n, err := io.Copy(peer, r)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] written %d bytes over the network to peer: %s\n", fs.Transport.Addr(), n, from)

	return nil
}

func (fs *FileServer) handleMessageStoreFile(from string, msg *MessageStoreFile) error {
	peer, ok := fs.peers[from]
	if !ok {
		return fmt.Errorf("peer %s not found", from)
	}

	n, err := fs.store.Write(msg.ID, msg.Key, io.LimitReader(peer, msg.Size))
	if err != nil {
		return err
	}

	fmt.Printf("[%s] received and written byte to disk: %d\n", fs.Transport.Addr(), n)

	peer.CloseStream()

	return nil
}

func (fs *FileServer) handleMessageDeleteFile(from string, msg *MessageDeleteFile) error {
	fmt.Printf("[%s] received delete message from peer: %s\n", fs.Transport.Addr(), from)
	if err := fs.store.Delete(msg.ID, msg.Key); err != nil {
		return err
	}

	fmt.Printf("[%s] deleted file (%s) from disk\n", fs.Transport.Addr(), msg.Key)
	return nil
}

func (fs *FileServer) bootstrapNetwork() error {
	for _, addr := range fs.BootstrapNodes {
		go func(addr string) {
			if err := fs.Transport.Dial(addr); err != nil {
				log.Printf("failed to dial bootstrap node: %s", err)
			}
		}(addr)
	}
	return nil
}

func (fs *FileServer) broadcast(msg *Message) error {
	buf := bytes.NewBuffer(nil)
	if err := gob.NewEncoder(buf).Encode(msg); err != nil {
		return err
	}

	for _, peer := range fs.peers {
		peer.Send([]byte{p2p.IncomingMessage})
		if err := peer.Send([]byte(buf.Bytes())); err != nil {
			return err
		}
	}

	return nil
}

func (fs *FileServer) stream(msg *Message) error {
	// Map peers to one multiwriter
	peers := []io.Writer{}
	for _, peer := range fs.peers {
		peers = append(peers, peer)
	}
	mw := io.MultiWriter(peers...)

	// Encode payload to multiwriter(all peers)
	return gob.NewEncoder(mw).Encode(msg)
}

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
	gob.Register(MessageDeleteFile{})
}
