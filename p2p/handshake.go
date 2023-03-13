package p2p

// HandShakeFunc is a function that is called after a connection is established
type HandShakeFunc func(Peer) error

// NOPHandshakeFunc is a function that does nothing
func NOPHandshakeFunc(Peer) error { return nil }
