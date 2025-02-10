package p2p

import (
	// "encoding/binary"
	"encoding/binary"
	"encoding/gob"
	"io"
	"net"
	"sync"
)

type Decoder interface {
	Decode(size int, r io.Reader, v *netOp, wgg *sync.WaitGroup) error
}

type GOBDecoder struct{}

func (d *GOBDecoder) Decode(r io.Reader, v *RPC) error {
	return gob.NewDecoder(r).Decode(v)
}

type DefaultDecoder struct {
	conn *net.Conn
}

func (d *DefaultDecoder) Decode(size int, r io.Reader, v *netOp, mu *sync.WaitGroup) error {
	(*mu).Add(1)

	buf := make([]byte, size)
	err := binary.Read(r, binary.LittleEndian, buf) // read here has no deadline so we wait in the loop for data

	if err != nil {
		return err
	}
	*v = netOp(binary.LittleEndian.Uint64(buf[:size]))
	return nil
}
