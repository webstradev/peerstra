package p2p

import (
	"encoding/gob"
	"io"
)

type Decoder interface {
	Decode(io.Reader, *RPC) error
}

type GOBDecoder struct{}

func NewGOBDecoder() *GOBDecoder {
	return &GOBDecoder{}
}

func (dec *GOBDecoder) Decode(r io.Reader, msg *RPC) error {
	return gob.NewDecoder(r).Decode(msg)
}

type DefaultDecoder struct{}

func NewDefaultDecoder() *DefaultDecoder {
	return &DefaultDecoder{}
}

func (dec *DefaultDecoder) Decode(r io.Reader, msg *RPC) error {
	peek := make([]byte, 1)
	_, err := r.Read(peek)
	if err != nil {
		return err
	}

	// If we have a stream, we don't need to decode the payload
	if peek[0] == IncomingStream {
		msg.Stream = true
		return nil
	}

	buf := make([]byte, 1024)
	n, err := r.Read(buf)
	if err != nil {
		return err
	}

	msg.Payload = buf[:n]

	return nil
}
