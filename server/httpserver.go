package server

import (
	"context"
	"errors"
	"fmt"
	zerolog "github.com/rs/zerolog"
	"go-fs/nfs"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

var Log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Logger().
	Level(zerolog.TraceLevel)

// Server is a light wrapper around http.Server.
type Server struct {
	server     *http.Server
	Address    string
	Listener   net.Listener
	FileServer *nfs.FileServer
}

// New creates a Server tsruct instance and returns it.
func New(ctx context.Context, mux *http.ServeMux) *Server {
	srv := &http.Server{BaseContext: func(listener net.Listener) context.Context {
		return ctx
	}}
	srv.Handler = mux
	return &Server{server: srv}
}

// Start creates a new Server, built off of a base http.Server and starts it
func Start(ctx context.Context, srv *Server) error {
	// Default server timout, in seconds
	const defaultSrvTimeout = 15 * time.Second
	var port = strconv.FormatUint(uint64(ctx.Value("masterPort").(uint16)), 10)

	// ensure timeouts are set
	if srv.server.ReadTimeout == 0 {
		srv.server.ReadTimeout = defaultSrvTimeout
	}

	if srv.server.WriteTimeout == 0 {
		srv.server.WriteTimeout = defaultSrvTimeout
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", port))
	if err != nil {
		Log.Error().Err(err).Msgf("%s unavailable", port)
		return err
	}
	srv.Address = listener.Addr().String()
	srv.Listener = listener

	return srv.Start()
}

// Start begins serving
func (srv *Server) Start() error {
	var err error

	Log.Trace().
		Str("address", srv.Address).
		Int("pid", os.Getpid()).
		Msg("Server listening")
	err = srv.server.Serve(srv.Listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.New("server failed to start")
	}
	return nil
}

// Shutdown http server
func (srv *Server) Shutdown(ctx context.Context) {
	// Allow up to thirty seconds for server operations to finish before
	// canceling them.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	Log.Info().Msg("Stopping network nfs system http-server\n")

	if err := srv.server.Shutdown(ctx); err != nil {
		Log.Error().
			Err(err).
			Msg("http-server shutdown error")
	}
	fmt.Println("post shutdown")

}
