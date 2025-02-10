package cmd

import (
	"context"
	"encoding/gob"
	"errors"
	"github.com/alecthomas/kingpin/v2"
	"go-fs/nfs"
	"go-fs/server"
	"go-fs/server/handlers"
	"go-fs/server/router"
	"go-fs/service"
	"golang.org/x/sync/errgroup"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type MasterNode struct {
	StoragePath string
	HTTPPort    uint16
}

var replicaNodes []string

func (m *MasterNode) Run(_ *kingpin.ParseContext) error {
	var httpSrv *server.Server //the http server to expose callable api's
	//the http server service package, its outside the go routine its created as it needs to
	//be updated once the nfs filer-server is created
	var httpSvc *service.HttpService

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)

	g, gctx := errgroup.WithContext(context.Background())
	gctx = context.WithValue(gctx, "masterPort", m.HTTPPort)

	gob.Register(nfs.Payload{})
	g.Go(func() error {
		//start http server

		//handlers
		httpSvc = service.NewHttpService(Log, m.StoragePath, replicaNodes)
		httpHandler := handlers.NewHttpHandler(httpSvc, &Log)
		mux := router.RouterMux(httpHandler)

		Log.Info().
			Msg("Starting up network nfs system http-server")

		httpSrv = server.New(gctx, mux)
		if err := server.Start(gctx, httpSrv); err != nil && !errors.Is(err, context.Canceled) {
			if strings.Contains(err.Error(), "bind: address already in use") {
				Log.Fatal().Err(err).Msgf("http port in use: %d", m.HTTPPort)
			}
			return err
		}
		return nil
	})

	g.Go(func() error {
		for {
			select {
			case <-c:
				httpSrv.Shutdown(gctx)
				//tcpFileServer.Stop()
				return nil
			}
		}
	})
	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}

		Log.Panic().Err(err).Msg("Failed to start master server")

	}
	return nil
}

// ParseReplicaPeerList The cli library is clunky when intercepting cli arguments being parsed
// we need a custom func to handle the list of peers into an array
func ParseReplicaPeerList(c *kingpin.CmdClause) error {
	var peersFlagArgs string
	for _, v := range os.Args {
		if strings.Contains(v, "peers") {
			peersFlagArgs = v
		}
	}

	if peersFlagArgs == "" {
		return errors.New("must specify at least one peer to connect to")
	}

	peersParts := strings.Split(strings.Split(peersFlagArgs, "=")[1], ",")

	if len(peersParts) == 0 {
		return errors.New("must specify at least one peer to connect to")
	}
	for _, v := range peersParts {
		if _, err := netip.ParseAddrPort(v); err != nil {
			return errors.New("invalid peer address: " + v)
		}
		replicaNodes = append(replicaNodes, v)
	}
	return nil
}
