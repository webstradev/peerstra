package p2p

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTCPTransport(t *testing.T) {
	tOpts := TCPTransportOpts{
		ListenAddr:    ":3000",
		HandShakeFunc: NOPHandshakeFunc,
		Decoder:       NewGOBDecoder(),
	}
	tr := NewTCPTransport(tOpts)

	assert.Equal(t, tr.ListenAddr, ":3000")

	err := tr.ListenAndAccept()
	assert.Nil(t, err)
}
