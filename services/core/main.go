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

	"github.com/monkeylabx/threadline/services/core/internal/channelrpc"
	"github.com/monkeylabx/threadline/services/core/internal/messagerpc"
	"github.com/monkeylabx/threadline/services/core/internal/outboxstore"
	"github.com/monkeylabx/threadline/services/core/internal/server"
	"github.com/monkeylabx/threadline/services/internal/outboxpolicy"
)

const envOutboxPolicyFile = "THREADLINE_OUTBOX_POLICY_FILE"

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

	channels, err := channelrpc.New(pool)
	if err != nil {
		return err
	}
	messages, err := messagerpc.New(pool, loadProducer())
	if err != nil {
		return err
	}
	handler, err := server.NewHandler(server.Dependencies{
		Pinger: pool, Channels: channels, Messages: messages, Verifier: server.RejectAllVerifier{},
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "threadline-core: no session verifier configured; every RPC fails closed (NOT PRODUCTION)")
	return server.Serve(ctx, config.ListenAddress(), handler)
}

// loadProducer reads the trusted Outbox policy named by
// THREADLINE_OUTBOX_POLICY_FILE. Without a valid file the message producer is
// not ready: SendEvent fails closed with NOT_DURABLE while the rest of the
// process keeps serving. The file path and contents are never echoed.
func loadProducer() *outboxstore.Producer {
	path, found := os.LookupEnv(envOutboxPolicyFile)
	if !found || path == "" {
		fmt.Fprintln(os.Stderr, "threadline-core: "+envOutboxPolicyFile+" unset; message producer not ready")
		return nil
	}
	document, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "threadline-core: outbox policy file unreadable; message producer not ready")
		return nil
	}
	policy, err := outboxpolicy.Parse(document)
	if err != nil {
		fmt.Fprintln(os.Stderr, "threadline-core: outbox policy file invalid; message producer not ready")
		return nil
	}
	producer, err := outboxstore.NewProducer(policy)
	if err != nil {
		fmt.Fprintln(os.Stderr, "threadline-core: outbox policy rejected; message producer not ready")
		return nil
	}
	return producer
}
