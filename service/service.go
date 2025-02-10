package service

import (
	"fmt"
	"github.com/rs/zerolog"
	"go-fs/nfs"
	"io"
	"time"
)

type HttpService struct {
	Log                    *zerolog.Logger
	FileServerStoragePath  string
	FileServerReplicaNodes []string
}

func NewHttpService(logger zerolog.Logger, sp string, rn []string) *HttpService {
	return &HttpService{Log: &logger, FileServerStoragePath: sp, FileServerReplicaNodes: rn}
}

func (h *HttpService) HandleFilePost(r *io.ReadCloser, filename string, contentType string) error {
	fmt.Println()
	h.Log.Trace().Msg("creating nfs and peer connections for data replication")
	nfsInstance, err := h.createNFSFileServerInstance()
	if err != nil {
		return err
	}

	replicationStartTm := time.Now()
	if err := nfsInstance.StoreData(filename, r); err != nil {
		return err
	}
	replicationEndTm := time.Since(replicationStartTm)
	h.Log.Info().Msgf("data replication to peers completed: [%s]", replicationEndTm.String())
	nfsInstance.Stop()

	return nil
}
func (h *HttpService) HandleFileFetch(filename string) (*nfs.StreamResponseReader, *nfs.FileServer, error) {
	fmt.Println()
	h.Log.Trace().Msg("creating nfs and peer connections for data read")
	nfsInstance, err := h.createNFSFileServerInstance()
	if err != nil {
		return nil, nil, err
	}

	var stream *nfs.StreamResponseReader
	if stream, err = nfsInstance.GetFile(filename); err != nil {
		return nil, nil, err
	}
	//nfsInstance.Stop()
	return stream, nfsInstance, nil
}

func (h *HttpService) createNFSFileServerInstance() (*nfs.FileServer, error) {
	tcpFileServer := nfs.MakeFileServer("", h.FileServerStoragePath, h.FileServerReplicaNodes, h.Log)
	if err := tcpFileServer.StartMaster(); err != nil {
		return nil, err
	}
	time.Sleep(1 * time.Second)
	return tcpFileServer, nil
}
