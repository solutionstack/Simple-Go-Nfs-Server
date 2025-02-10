package p2p

import "net"

type Peer interface {
	PClose() error
	net.Conn //embed methods of net.conn

}

type Transport interface {
	Consume() <-chan RPC
	Dial(string) error
}
