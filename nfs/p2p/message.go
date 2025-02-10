package p2p

import "net"

type RPC struct {
	Payload []byte
	From    net.Addr
}
