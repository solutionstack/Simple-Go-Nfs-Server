package nfs

import (
	"github.com/rs/zerolog"
	p2p2 "go-fs/nfs/p2p"
)

func MakeFileServer(listenAddr string, storageRoot string, nodes []string, logger *zerolog.Logger) *FileServer {
	tcpOpts := p2p2.TCPTransportOpts{
		ListenAddr:    listenAddr,
		HandshakeFunc: p2p2.NOOPHandshaker,
		Decoder:       &p2p2.DefaultDecoder{},
		Logger:        logger,
	}
	tr := p2p2.NewTCPTransport(tcpOpts)

	fileServOpts := FileServerOpts{
		Transport:      tr,
		BootstrapNodes: nodes,
		StorageRoot:    storageRoot,
		Logger:         logger,
	}

	fileServOpts.ServerType = ReplicaNode
	if listenAddr == "" {
		fileServOpts.ServerType = MasterNode
	}

	srv := NewFileServer(fileServOpts)
	srv.Transport.OnPeer = srv.OnPeer

	return srv
}
