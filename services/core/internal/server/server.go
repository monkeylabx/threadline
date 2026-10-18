package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/monkeylabx/threadline/services/gen/threadline/channel/v1/channelv1connect"
	"github.com/monkeylabx/threadline/services/gen/threadline/message/v1/messagev1connect"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// Dependencies are the collaborators the core handler composes. Every field is
// required so a half-wired process cannot start.
type Dependencies struct {
	Pinger   Pinger
	Channels channelv1connect.ChannelServiceHandler
	Messages messagev1connect.MessageServiceHandler
	Verifier rpcmiddleware.SessionVerifier
}

// NewHandler composes GET /healthz and every Connect service behind the
// authentication interceptor. Unary procedures work over HTTP/1.1; the first
// streaming procedure registered here must add h2c.
func NewHandler(deps Dependencies) (http.Handler, error) {
	if deps.Pinger == nil || deps.Channels == nil || deps.Messages == nil || deps.Verifier == nil {
		return nil, errors.New("core server: pinger, channel and message handlers and session verifier are required")
	}
	authenticated := connect.WithInterceptors(rpcmiddleware.NewAuthenticationInterceptor(deps.Verifier))
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthHandler(deps.Pinger))
	mux.Handle(channelv1connect.NewChannelServiceHandler(deps.Channels, authenticated))
	mux.Handle(messagev1connect.NewMessageServiceHandler(deps.Messages, authenticated))
	return mux, nil
}

// Serve listens on address and serves handler until ctx ends, then shuts down
// gracefully within shutdownTimeout.
func Serve(ctx context.Context, address string, handler http.Handler) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New("core server: listen failed")
	}
	return ServeListener(ctx, listener, handler)
}

// ServeListener is Serve for a caller-owned listener. The HTTP server takes
// ownership of listener and closes it.
func ServeListener(ctx context.Context, listener net.Listener, handler http.Handler) error {
	httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: readHeaderTimeout}
	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
