package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
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
	tenant := seedLiveEvent(t, ctx, pool)

	observed := observeLiveEvent(t, ctx, broker, tenant)

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
	if observed.Load() != 1 {
		t.Fatal("expected exactly one publication of this test Event across restart")
	}
	assertOneAttempt(t, ctx, pool, tenant)

}

func assertOneAttempt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenant string) {
	t.Helper()
	var attempts int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM domain.outbox_delivery_attempts WHERE tenant_id=$1", tenant).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("acknowledged Event was reclaimed")
	}

}

func observeLiveEvent(t *testing.T, ctx context.Context, broker, tenant string) *atomic.Int32 {
	t.Helper()
	connection, err := nats.Connect(broker, nats.UserInfo("threadline_realtime_dev", "threadline-worker-dev-only"))
	if err != nil {
		t.Fatal("fixture observer unavailable")
	}
	t.Cleanup(connection.Close)
	var observed atomic.Int32
	subscription, err := connection.Subscribe(mapping.Subject, func(message *nats.Msg) {
		if bytes.Contains(message.Data, []byte(tenant)) {
			observed.Add(1)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := subscription.Unsubscribe(); err != nil {
			t.Error("observer cleanup failed")
		}
	})
	if err := connection.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}

	return &observed
}

func seedLiveEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
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
	return tenant
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
