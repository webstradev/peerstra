package p2p

// Peer is an interface that represents a remote node in the network.
type Peer interface{}

// Transport is an an interface that represents a transport layer.
type Transport interface {
	ListenAndAccept() error
}
