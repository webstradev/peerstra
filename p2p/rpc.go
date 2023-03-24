package p2p

// RPC holds any arbitrary data that is being sent over
// each transport between two nodes in the networ
type RPC struct {
	From    string
	Payload []byte
}
