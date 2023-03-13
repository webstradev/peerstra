package p2p

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTCPTransport(t *testing.T) {
	listeAddr := ":4000"
	tr := NewTCPTransport(listeAddr)

	assert.Equal(t, tr.listenAddr, listeAddr)

	err := tr.ListenAndAccept()
	assert.Nil(t, err)
}
