// Command realtime serves the development live-only StreamEvents endpoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/monkeylabx/threadline/services/gen/threadline/sync/v1/syncv1connect"
	"github.com/monkeylabx/threadline/services/realtime/internal/fanout"
	"github.com/nats-io/nats.go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "threadline-realtime:", err)
		os.Exit(1)
	}
}

func run() error {
	c, err := load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWithContext(ctx, c)
}

func runWithContext(ctx context.Context, c config) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	connection, err := nats.Connect(c.broker, nats.UserInfo(c.user, c.password), nats.MaxReconnects(-1), nats.Timeout(2*time.Second),
		nats.ErrorHandler(func(*nats.Conn, *nats.Subscription, error) {
			fmt.Fprintln(os.Stderr, "realtime broker operation rejected")
			stop()
		}))
	if err != nil {
		return errors.New("realtime broker unavailable")
	}
	defer connection.Close()
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	core := syncv1connect.NewSyncServiceClient(client, c.core)
	handler, err := fanout.New(core, fanout.NATS{Connection: connection})
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	_, service := syncv1connect.NewSyncServiceHandler(handler, connect.WithReadMaxBytes(65536), connect.WithSendMaxBytes(1048576))
	mux.Handle(syncv1connect.SyncServiceStreamEventsProcedure, service)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if !connection.IsConnected() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	})
	return serve(ctx, c.address, mux)
}

func serve(ctx context.Context, address string, handler http.Handler) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New("realtime listen failed")
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
	case <-result:
		return errors.New("realtime server failed")
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		if closeErr := server.Close(); closeErr != nil {
			return errors.New("realtime close failed")
		}
		return errors.New("realtime shutdown failed")
	}
	if err := <-result; !errors.Is(err, http.ErrServerClosed) {
		return errors.New("realtime server failed")
	}
	return nil
}
