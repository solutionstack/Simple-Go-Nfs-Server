package p2p

import (
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"io"
	"net"
	"os"
	"sync"
	"time"
	// "time"
)

type netOp int

var Log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Logger().
	Level(zerolog.TraceLevel)

const (
	IncomingStorageStream netOp = 1 << (iota + 1)
	OutgoingStorageStream
	ResetStorageStream = 0
)

// TCPPeer tcp remote node
type TCPPeer struct {
	net.Conn //embed iface methods of net.conn
	//are we an Outbound (Dial) or inbound connection
	Outbound         bool
	peerLocalAddress string //if an outbound conn, this is the local port on the dialer side
	peerClosed       bool
	mu               *sync.RWMutex
}

func (p *TCPPeer) PClose() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.peerClosed {
		return nil
	}
	p.peerClosed = true

	if p.Outbound {
		Log.Trace().Str("addr", p.RemoteAddr().String()).Msg("dropping outbound peer connection ")
	} else {
		Log.Trace().Str("remote", p.RemoteAddr().String()).Msg("dropping connection ")

	}
	return p.Close()
}
func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{conn, outbound, "", false, &sync.RWMutex{}} //conn
}

type TCPTransportOpts struct {
	ListenAddr    string
	HandshakeFunc HandshakeFunc
	OnPeer        func(peer Peer) error
	Decoder       Decoder
	Logger        *zerolog.Logger
}
type TCPPeerMessageHeader struct {
	NetOp         netOp
	Peer          *TCPPeer
	MessageWaiter *sync.WaitGroup
}
type TCPTransport struct {
	TCPTransportOpts
	listener *net.Listener
	Mu       *sync.RWMutex
	Peers    map[net.Addr]*TCPPeer

	rpcmsg chan TCPPeerMessageHeader
}

func NewTCPTransport(opts TCPTransportOpts) *TCPTransport {

	return &TCPTransport{
		TCPTransportOpts: opts,
		rpcmsg:           make(chan TCPPeerMessageHeader),

		Mu:    &sync.RWMutex{}, //multiple goroutines rw peers
		Peers: map[net.Addr]*TCPPeer{},
	}
}

// Consume return ro chan for reading recieved messages from a peer
func (t *TCPTransport) Consume() <-chan TCPPeerMessageHeader {
	return t.rpcmsg
}

func (t *TCPTransport) ListenAndAccept() error {

	l, err := net.Listen("tcp", t.TCPTransportOpts.ListenAddr)
	(*t).listener = &l
	if err != nil {
		return err
	}
	t.TCPTransportOpts.Logger.Trace().
		Str("addr", (*(*t).listener).Addr().String()).
		Int("pid", os.Getpid()).
		Msgf("Listening in replica mode")
	go t.connAcceptLoop()

	return nil
}

func (t *TCPTransport) connAcceptLoop() {
	for {
		con, err := (*(*t).listener).Accept()
		if err != nil {
			t.TCPTransportOpts.Logger.
				Error().
				Msgf("TCP Aceept Error: %s\n", err)
		}
		go t.handleCon(con, false)
	}
}
func (t *TCPTransport) handleCon(con net.Conn, outbound bool) {
	netOpBufferSize := 8 //net ops are int bytes
	peer := NewTCPPeer(con, outbound)
	var op netOp

	defer func() {
		if err := peer.PClose(); err != nil {
			t.TCPTransportOpts.Logger.Panic().
				Str("remote", peer.RemoteAddr().String()).
				Str("local", t.ListenAddr).
				Err(err).Msg("peer close failed")
		}
	}()
	//if there's an onpeer hook and it doesn't return an error
	if t.OnPeer != nil && t.OnPeer(peer) != nil {
		return
	}
	if err := t.TCPTransportOpts.HandshakeFunc(peer); err != nil {
		fmt.Printf("TCP handhskae error %s\n", err)
	}

	t.Mu.Lock()
	if outbound {
		t.Peers[peer.RemoteAddr()] = peer
		peer.peerLocalAddress = peer.LocalAddr().String()
		t.TCPTransportOpts.Logger.Trace().Msgf("master instance established new connection to peer: [%s - >%s]", peer.LocalAddr().String(), peer.RemoteAddr())

	} else {
		t.Peers[peer.RemoteAddr()] = peer
		fmt.Println()
		t.TCPTransportOpts.Logger.Trace().Msgf("replica instance %s accepted new connection from: %s\n", t.ListenAddr, peer.RemoteAddr())
	}
	t.Mu.Unlock()

	if peer.Outbound {
		select {}
	} //we only wait for network opeation reads for inbound connections

	for {
		netOpWaiter := sync.WaitGroup{}
		err := t.Decoder.Decode(netOpBufferSize, peer, &op, &netOpWaiter)
		if err != nil {
			if peer.peerClosed {
				return
			}
			if errors.Is(err, io.EOF) {
				t.TCPTransportOpts.Logger.Trace().
					Str("remote", peer.RemoteAddr().String()).
					Str("local", t.ListenAddr).
					Msgf("remote connection closed peer")
			} else {

				t.TCPTransportOpts.Logger.Trace().Msgf("read error from peer: %s: %s\n", peer.RemoteAddr(), err.Error())
				continue
			}

			return
		}
		//once we read the first set of bytes we send on the message channel so any available
		//server can process the stream, then we wait for a waitGroup.Done() signal which tells
		//us to resume reading for a new network operation

		msg := TCPPeerMessageHeader{NetOp: op, Peer: peer, MessageWaiter: &netOpWaiter}
		t.rpcmsg <- msg
		netOpWaiter.Wait()
	}

}

func (t *TCPTransport) Dial(addr string) error {
	if len(addr) == 0 {
		return nil
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	go t.handleCon(conn, true)
	return nil
}
