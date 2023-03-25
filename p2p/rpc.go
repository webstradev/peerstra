package p2p

const (
	IncomingMessage = 0x1
	IncomingStream  = 0x2
)

// RPC holds any arbitrary data that is being sent over
// each transport between two nodes in the networ
type RPC struct {
	From    string
	Payload []byte
	Stream  bool
}
