package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/monkeylabx/threadline/services/internal/outboxpolicy"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxbreaker"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxinspect"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxrelay"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

var mapping = outboxpublish.Mapping{LogicalDestination: "domain-events", Stream: "THREADLINE_DOMAIN_EVENTS_V1", Subject: "threadline.domain.events.v1"}

func Run(ctx context.Context, config Config) error {
	document, err := os.ReadFile(config.policyFile)
	if err != nil {
		return errors.New("worker policy unreadable")
	}
	policy, err := outboxpolicy.Parse(document)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, config.dsn)
	if err != nil {
		return errors.New("worker database configuration rejected")
	}
	defer pool.Close()
	connection, err := nats.Connect(config.broker, nats.UserInfo(config.user, config.password),
		nats.CustomInboxPrefix("_INBOX.threadline.worker"), nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1), nats.ReconnectWait(time.Second), nats.Timeout(2*time.Second),
		nats.ErrorHandler(func(*nats.Conn, *nats.Subscription, error) {
			fmt.Fprintln(os.Stderr, "worker broker operation rejected")
		}))
	if err != nil {
		return errors.New("worker broker configuration rejected")
	}
	defer connection.Close()
	broker, err := jetstream.New(connection)
	if err != nil {
		return errors.New("worker broker setup failed")
	}
	return serveWorker(ctx, config, resources{pool, connection, broker, policy})
}

type resources struct {
	pool       *pgxpool.Pool
	connection *nats.Conn
	broker     jetstream.JetStream
	policy     outboxpolicy.Snapshot
}

func serveWorker(ctx context.Context, c Config, deps resources) error {
	pool, connection, broker, policy := deps.pool, deps.connection, deps.broker, deps.policy
	runner, breaker, err := assemble(pool, broker, c.owner)
	if err != nil {
		return err
	}
	inspector, err := outboxinspect.New(connection, broker, "", mapping, policy)
	if err != nil {
		return err
	}
	startup := func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return errors.New("worker database unavailable")
		}
		if err := inspector.Check(ctx); err != nil {
			return err
		}
		if err := checkPublishPermission(ctx, connection, c.user); err != nil {
			return err
		}
		return breaker.Ready()
	}
	var ready atomic.Bool
	workerCtx, stop := context.WithCancel(ctx)
	defer stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	done := make(chan struct{})
	go func() { defer close(done); runLoop(workerCtx, ticker.C, runner, &ready, startup) }()
	err = serveHealth(ctx, c.address, func() bool { return ready.Load() && connection.IsConnected() })
	stop()
	<-done
	return err
}

func assemble(pool *pgxpool.Pool, broker jetstream.JetStream, owner string) (*outboxrelay.Coordinator, *outboxbreaker.Breaker, error) {
	store, err := outboxdb.NewStore(pool, outboxdb.Binding{LogicalDestination: mapping.LogicalDestination, Stream: mapping.Stream})
	if err != nil {
		return nil, nil, err
	}
	publisher, err := outboxpublish.NewJetStreamPublisher(broker, mapping)
	if err != nil {
		return nil, nil, err
	}
	relay, err := outboxrelay.NewRelay(store, outboxrelay.ProtobufEncoder{}, publisher, owner)
	if err != nil {
		return nil, nil, err
	}
	breaker, err := outboxbreaker.New(mapping, wallClock{})
	if err != nil {
		return nil, nil, err
	}
	runner, err := outboxrelay.NewCoordinator(relay, breaker)
	return runner, breaker, err
}
