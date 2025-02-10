package cmd

import (
	"encoding/gob"
	"github.com/alecthomas/kingpin/v2"
	"github.com/rs/zerolog"
	"go-fs/nfs"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var srv *nfs.FileServer
var Log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Logger().
	Level(zerolog.TraceLevel)

// mu := &sync.RWMutex{}
type ReplicaNode struct {
	Port        uint16
	StoragePath string
}

func (r *ReplicaNode) Run(_ *kingpin.ParseContext) error {
	srv = nfs.MakeFileServer(":"+strconv.FormatInt(int64(r.Port), 10), r.StoragePath, []string{}, &Log)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)
	gob.Register(nfs.Payload{})

	go func() {
		for {
			select {
			case <-c:
				srv.Stop()
			}
		}
	}()

	Log.Info().
		Msg("Starting up network nfs system replica instance")
	if err := srv.Start(); err != nil {
		Log.Fatal().Err(err).Msg("start replica")
	}
	srv.OnData()
	return nil
}
