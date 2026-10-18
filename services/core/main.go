// Command core runs the threadline-core Connect process: health, PostgreSQL
// pool and ChannelService behind the authentication interceptor.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monkeylabx/threadline/services/core/internal/server"
	"github.com/monkeylabx/threadline/services/gen/threadline/channel/v1/channelv1connect"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "threadline-core:", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := server.LoadConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, config.PostgresDSN())
	if err != nil {
		return errors.New("postgres pool configuration failed")
	}
	defer pool.Close()

	// ChannelService procedures answer CodeUnimplemented until the first read
	// handler (#262 follow-up A1b) replaces this placeholder.
	handler, err := server.NewHandler(server.Dependencies{
		Pinger:   pool,
		Channels: channelv1connect.UnimplementedChannelServiceHandler{},
		Verifier: server.RejectAllVerifier{},
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "threadline-core: no session verifier configured; every RPC fails closed (NOT PRODUCTION)")
	return server.Serve(ctx, config.ListenAddress(), handler)
}
