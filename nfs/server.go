package nfs

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	p2p2 "go-fs/nfs/p2p"
	"go-fs/store"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"reflect"
	"unicode"

	// "strings"
	"sync"
	"time"
)

type fileOp int64
type ServerType string

var streamEndBytesFlag = []byte{0xc, 0xc, 0xa}

const (
	IncomingStorageStream fileOp = 1 << (iota + 1)
	OutgoingStorageStream
	ResetStorageStream = 0
)

const (
	MasterNode  ServerType = "master"
	ReplicaNode ServerType = "replica"
)

var defaultBasePath = "storage-path"

type FileServerOpts struct {
	StorageRoot    string
	Transport      *p2p2.TCPTransport
	BootstrapNodes []string
	ServerType     ServerType
	Logger         *zerolog.Logger
}

type FileServer struct {
	FileServerOpts
	store   *store.Store
	quictch chan struct{}
}

type Payload struct {
	Key [255]byte
}

type Message struct {
	From    string
	Payload any
}

func NewFileServer(opts FileServerOpts) *FileServer {

	bp := opts.StorageRoot + "/" + opts.Transport.ListenAddr
	if len(bp) == 0 {
		bp = defaultBasePath + "/" + opts.Transport.ListenAddr
	}
	return &FileServer{
		FileServerOpts: opts,
		quictch:        make(chan struct{}),
		store: store.NewStore(store.StoreOpts{
			PathTransformFunc: store.DefaultPathTransformFunc,
			BasePath:          bp,
		}, opts.Logger),
	}
}
func (f *FileServer) StartMaster() error {

	err := f.bootstrapNetwork()
	if err != nil {
		return err
	}
	go f.OnData() //listen for events on master server
	return nil
}
func (f *FileServer) Start() error {
	if err := f.Transport.ListenAndAccept(); err != nil {
		f.Logger.Panic().Err(err).Msg("")
	}

	return nil
}
func (f *FileServer) Stop() {
	defer func() { //graceful stop nfs-server
		f.Transport.Mu.Lock()

		for _, p := range f.Transport.Peers {
			if p != nil {
				if err := p.PClose(); err != nil {
					panic(err)
				}
			}
		}
		f.Transport.Mu.Unlock()
		f.Logger.Info().
			Msg("Stopping network nfs system " + string(f.ServerType) + " instance ")
		close(f.quictch)

	}()
}

// broadcast() sends the network operation IncomingStorageStream | OutgoingStorageStream
// and sends the payload too all connected peers
func (f *FileServer) broadcast(key string, r *os.File) error {

	defer func() {
		_ = r.Close()
	}()

	peers := []io.Writer{}
	f.Transport.Mu.RLock()

	for _, p := range f.Transport.Peers {
		peers = append(peers, p) //each peer fulfils the io.Writer interface
		_ = p.SetWriteDeadline(time.Now().Add(time.Second * 5))

	}
	f.Transport.Mu.RUnlock()

	mw := io.MultiWriter(peers...) //batch peers so we can write to all efficiently

	//send stream operation constant
	fileOp := IncomingStorageStream
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(fileOp))
	_, err := mw.Write(buf)

	if err != nil {
		return err
	}

	//send the Payload struct which in this case is only the file-name
	var msgFile Payload
	copy(msgFile.Key[:], key)
	bufs := new(bytes.Buffer)
	err = binary.Write(bufs, binary.LittleEndian, msgFile)
	if err != nil {
		fmt.Println("binary.Write failed:", err)
	}
	_, err = mw.Write(bufs.Bytes())

	//copy the actual file data over the wire
	_, err = io.Copy(mw, r)
	if err != nil {
		return err
	}

	return nil
}

func (f *FileServer) StoreData(key string, r *io.ReadCloser) error {
	defer func(closer io.ReadCloser) {
		err := closer.Close()
		if err != nil {
			f.Logger.Error().Err(err).Msg(" Err on [DATA CREATION REQUEST]")
		}
	}((*r))

	fmt.Println()
	f.Logger.Trace().Msg("[DATA CREATION REQUEST] on master instance ")
	if f.ServerType != MasterNode {
		return fmt.Errorf("invalid call to StoreData on non master node")
	}
	f.Logger.Info().Msgf("writing data on master instance... %s\n", f.Transport.ListenAddr)

	if err := f.store.Write(key, *r); err != nil {
		return err
	}

	fd, _, err := f.store.OpenFd(key)
	if err != nil {
		f.Logger.Error().Err(err).Msg(" Err on [DATA CREATION REQUEST]")

	}

	f.Logger.Info().Msg("replicating data to peer nodes...")

	return f.broadcast(key, fd)
}

// bootstrapNetwork setups connections the replica server peers
func (f *FileServer) bootstrapNetwork() error {

	for _, addr := range f.BootstrapNodes {
		if err := f.Transport.Dial(addr); err != nil {
			var opErr *net.OpError
			if errors.As(err, &opErr) {
				f.Logger.Error().Err(err).
					Str("addr", addr).
					Str("netOpErr", opErr.Op).
					Msg("cannot reach peer: is server running?")
			}
			f.Logger.Error().Err(err).
				Str("addr", addr).
				Str("netOpErr", opErr.Op).
				Msg("unknown error connecting to peer")

			return err

		}
	}
	return nil
}

func (f *FileServer) OnData() {

	for {
	waiterNewNetStream:
		for {
			select {
			case <-f.quictch: //SIGINT
				return
			case data := <-f.Transport.Consume():
				var op = fileOp(data.NetOp)
				switch op {

				case IncomingStorageStream:
					peer := data.Peer
					f.Logger.Trace().Msgf("[DATA REPLICATION REQUEST] recieved on peer %s", peer.LocalAddr().String())
					err := f.handleIncomingStorageReplicationStream(peer) //read remaining data and store
					if err != nil {
						f.Logger.Trace().Msg(err.Error())
						data.MessageWaiter.Done()
						break waiterNewNetStream
					}
					data.MessageWaiter.Done()
					break waiterNewNetStream

				case OutgoingStorageStream:

					peer := data.Peer //the peer that sent the stream request so we send the data back over same connection

					f.Logger.Trace().Msgf("[DATA READ REQUEST] recieved on peer %s", peer.LocalAddr().String())

					_ = peer.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))

					//read the Payload struct data containing the file name
					var pl Payload
					if err := binary.Read(peer, binary.LittleEndian, &pl); err != nil {
						f.Logger.Error().Err(err).Msg("binary.Read failed:")
						data.MessageWaiter.Done()
						break waiterNewNetStream
					}

					fileName := f.extractPrintable(pl.Key[:])

					_ = peer.SetReadDeadline(time.Time{})

					err := f.handleOutgoingStorageStream(peer, fileName) //stream requested nfs bytes
					if err != nil {
						f.Logger.Error().Err(err).Msg("binary.Read failed:")
						data.MessageWaiter.Done()
						break waiterNewNetStream
					}
					data.MessageWaiter.Done()
					break waiterNewNetStream

				default:
					break waiterNewNetStream

				}
				break waiterNewNetStream //break out of the inner for{} and wait for new data

			}
		}
	}
}

type StreamResponseReader struct {
	stream io.Reader
	mu     sync.Mutex //?
}

func (s *StreamResponseReader) Read(buf []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rd := bufio.NewReaderSize(s.stream, 512)

	return rd.Read(buf)

}

func NewStreamResponseReader(con *p2p2.TCPPeer) *StreamResponseReader {
	return &StreamResponseReader{
		stream: con,
		mu:     sync.Mutex{},
	}
}

// GetFile return a reader for reading a a nfs stream from a random replica node
func (f *FileServer) GetFile(key string) (*StreamResponseReader, error) {
	if err := f.store.Has(key); err != nil {
		f.Logger.Trace().Msg(err.Error())
		return nil, err
	}

	peer, err := f.selectRandomReadPeer()
	if err != nil {
		return nil, err
	}

	//tell the peer to open a stream for reading the requested nfs
	err = f.sendReadFileRequestToPeer(peer, key)
	if err != nil {
		return nil, err
	}

	return NewStreamResponseReader(peer), nil
}

// selectRandomReadPeer retruns a random read source for fulfilling a nfs read request
func (f *FileServer) selectRandomReadPeer() (*p2p2.TCPPeer, error) {
	f.Transport.Mu.RLock()
	defer f.Transport.Mu.RUnlock()

	if len(f.Transport.Peers) == 0 {
		return nil, errors.New("no peers available for read")
	}
	var mapKeys = reflect.ValueOf(f.Transport.Peers).MapKeys()
	randKey := mapKeys[rand.IntN(len(mapKeys))].Interface()
	return f.Transport.Peers[randKey.(net.Addr)], nil

}

func (f *FileServer) sendReadFileRequestToPeer(peer *p2p2.TCPPeer, key string) error {
	//send nfs operation type so they know to return a data stream
	//send nfs key to retrieve

	fileOp := OutgoingStorageStream
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(fileOp))
	_, err := peer.Write(buf)

	if err != nil {
		return err
	}

	//send the Payload struct which in this case is only the file-name
	var msgFile Payload
	copy(msgFile.Key[:], key)
	bufs := new(bytes.Buffer)
	err = binary.Write(bufs, binary.LittleEndian, msgFile)
	if err != nil {
		fmt.Println("binary.Write failed:", err)
	}
	_, err = peer.Write(bufs.Bytes())

	return nil
}

func (f *FileServer) handleOutgoingStorageStream(peer *p2p2.TCPPeer, filekey string) error {

	r, fi, err := f.store.OpenFd(filekey)
	if err != nil {
		return fmt.Errorf("handleoutgoingStoragestream.store: error reading data from storage: %s", err)
	}

	f.Logger.Trace().Msgf("streaming requested file to master:  %s", filekey)

	writtenBytes := 0
	for {
		buf := make([]byte, 65536) //we'd send back the stream in 64kb chunks
		n, err := r.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("handleoutgoingStoragestream: read error: %s", err)
		}

		_, err = peer.Write(
			buf[:n])
		if err != nil {
			return fmt.Errorf("handleoutgoingStoragestream: write error: %s", err)

		}

		writtenBytes += n
		if writtenBytes == int((*fi).Size()) { //have we written length == fileSize
			if err := f.store.CloseFd((*fi).Name()); err != nil { //close the file-descriptor after streaming
				return fmt.Errorf("handleoutgoingStoragestream: CloseFd error: %s", err)
			}

			_, err := peer.Write(streamEndBytesFlag) //write end-of-stream sequence
			if err != nil {
				return fmt.Errorf("handleoutgoingStoragestream: write error: %s", err)
			}
			break

		}
	}

	return nil
}

func (f *FileServer) handleIncomingStorageReplicationStream(peer *p2p2.TCPPeer) error {
	_ = peer.SetReadDeadline(time.Now().Add(4 * time.Second))

	//read the Payload struct data containing the file name
	var data Payload
	if err := binary.Read(peer, binary.LittleEndian, &data); err != nil {
		return fmt.Errorf("binary.Read failed: %s", err)
	}

	f.Logger.Info().Msg("writing data on replica node... ")

	fileName := f.extractPrintable(data.Key[:])

	err := f.store.Write(fileName, peer)
	if err != nil {
		return fmt.Errorf("handleIncomingStorageReplicationStream.store: error writing data to storage: %s", err)
	}

	////cancel the read deadline so it doesn't effect the main tcp read loop
	_ = peer.SetReadDeadline(time.Time{})

	return nil
}

func (f *FileServer) OnPeer(peer p2p2.Peer) error {
	//some peer validation stuff here
	return nil
}

// extractPrintable byte slices could contain strings folowed by other chars esp when extracted from fixed length sources
// we extract just the printable string
func (f FileServer) extractPrintable(data []byte) string {

	for k, v := range data {

		if !unicode.IsPrint(rune(v)) {
			return string(data[0:k])
		}
	}

	return string(data[:])
}
