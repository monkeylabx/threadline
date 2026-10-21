package process

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// This test requires an explicitly disposable migrated database and the #329
// pinned NATS fixture. It asserts real DB/broker state, never process logs.
func TestLiveProcessPublishesOnceAcrossRestart(t *testing.T) {
	dsn, broker := os.Getenv("THREADLINE_TEST_WORKER_DSN"), os.Getenv("THREADLINE_TEST_WORKER_NATS_URL")
	if dsn == "" || broker == "" {
		t.Skip("real Worker process fixtures not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("invalid fixture database")
	}
	defer pool.Close()
	if !strings.HasPrefix(pool.Config().ConnConfig.Database, "threadline_worker_process_test_") {
		t.Fatal("refusing non-disposable database")
	}
	tenant := fmt.Sprintf("worker-live-%d", time.Now().UnixNano())
	seed, err := os.ReadFile("../../testdata/event.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(seed), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := pool.Exec(ctx, statement, tenant); err != nil {
			t.Fatal("fixture insert failed:", err)
		}
	}
	connection, err := nats.Connect(broker, nats.UserInfo("threadline_platform_dev", "threadline-worker-dev-only"), nats.CustomInboxPrefix("_INBOX.threadline.platform"))
	if err != nil {
		t.Fatal("fixture broker unavailable")
	}
	defer connection.Close()
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(ctx, mapping.Stream)
	if err != nil {
		t.Fatal(err)
	}
	before, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{address: "127.0.0.1:0", dsn: dsn, broker: broker, user: "threadline_worker_dev", password: "threadline-worker-dev-only", owner: tenant, policyFile: "../../../../deploy/compose/outbox-policy.json"}
	for pass := 0; pass < 2; pass++ {
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- Run(runCtx, config) }()
		waitDelivered(t, ctx, pool, tenant)
		// A restarted loop must get an idle turn, not a second publish.
		if pass == 1 {
			time.Sleep(600 * time.Millisecond)
		}
		stop()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	after, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.State.Msgs != before.State.Msgs+1 {
		t.Fatal("expected exactly one real broker message across restart")
	}
}

func waitDelivered(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenant string) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var state string
		if err := pool.QueryRow(ctx, "SELECT delivery_state FROM domain.transactional_outbox WHERE tenant_id=$1", tenant).Scan(&state); err != nil {
			t.Fatal("delivery state query failed")
		}
		if state == "delivered" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("Worker never delivered")
		case <-ticker.C:
		}
	}
}
