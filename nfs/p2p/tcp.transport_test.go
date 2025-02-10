package p2p

import (
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

var TLog = zerolog.New(os.Stderr).With().Timestamp().Logger()

func TestNewTCPTransport(t *testing.T) {
	addr := ":10000"
	tr := NewTCPTransport(TCPTransportOpts{ListenAddr: addr, Logger: &TLog})

	assert.Equal(t, tr.ListenAddr, addr)

	assert.Nil(t, tr.ListenAndAccept())
}
