package outboxdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestAdapterPostgresConcurrentClaimAndTransitions(t *testing.T) {
	ctx, firstDatabase, secondDatabase := openWorkerOutboxDatabase(t)
	insertWorkerOutboxFixtures(t, ctx, firstDatabase, 8)

	firstAdapter, err := newAdapter(firstDatabase)
	if err != nil {
		t.Fatal(err)
	}
	secondAdapter, err := newAdapter(secondDatabase)
	if err != nil {
		t.Fatal(err)
	}

	type claimResult struct {
		events []claimedEvent
		err    error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	claimConcurrently := func(bound *adapter, owner string) {
		<-start
		events, claimErr := bound.claim(ctx, claimRequest{claimOwnerID: owner, batchSize: 4})
		results <- claimResult{events: events, err: claimErr}
	}
	go claimConcurrently(firstAdapter, "worker-go-first-synthetic")
	go claimConcurrently(secondAdapter, "worker-go-second-synthetic")
	close(start)

	firstResult := <-results
	secondResult := <-results
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("concurrent claim errors = (%v, %v)", firstResult.err, secondResult.err)
	}
	if len(firstResult.events) != 4 || len(secondResult.events) != 4 {
		t.Fatalf("concurrent claim sizes = (%d, %d), want (4, 4)", len(firstResult.events), len(secondResult.events))
	}
	seen := make(map[int64]struct{}, 8)
	for _, event := range append(firstResult.events, secondResult.events...) {
		if _, duplicate := seen[event.fence.outboxEntryID]; duplicate {
			t.Fatal("concurrent adapters received the same Outbox Entry")
		}
		seen[event.fence.outboxEntryID] = struct{}{}
	}

	delivered := firstResult.events[0]
	if _, err := firstAdapter.renew(ctx, delivered.fence); err != nil {
		t.Fatal(err)
	}
	acknowledged, err := firstAdapter.acknowledge(ctx, acknowledgementRequest{
		fence: delivered.fence,
		pubAck: pubAck{
			stream:    "DOMAIN_EVENTS",
			sequence:  1,
			duplicate: false,
			messageID: delivered.brokerMessageID,
		},
	})
	if err != nil || acknowledged != acknowledgementDelivered {
		t.Fatalf("acknowledgement = %v, error = %v", acknowledged, err)
	}

	retrying := secondResult.events[0]
	failed, err := secondAdapter.recordFailure(ctx, publishFailureRequest{
		fence: retrying.fence,
		code:  failureTransportUnavailable,
	})
	if err != nil || failed.disposition != failureDispositionRetryScheduled || failed.nextAttemptAt.IsZero() {
		t.Fatalf("failure result was not a database schedule: error = %v", err)
	}

	var deliveredCount, pendingCount, claimedCount int
	if err := firstDatabase.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE delivery_state = 'delivered'),
			count(*) FILTER (WHERE delivery_state = 'pending'),
			count(*) FILTER (WHERE delivery_state = 'claimed')
		FROM domain.transactional_outbox
	`).Scan(&deliveredCount, &pendingCount, &claimedCount); err != nil {
		t.Fatal(err)
	}
	if deliveredCount != 1 || pendingCount != 1 || claimedCount != 6 {
		t.Fatalf("Entry states = (%d delivered, %d pending, %d claimed), want (1, 1, 6)", deliveredCount, pendingCount, claimedCount)
	}
}

func openWorkerOutboxDatabase(t *testing.T) (context.Context, *pgx.Conn, *pgx.Conn) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run Worker Outbox integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("parse configured PostgreSQL maintenance DSN failed")
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("connect to configured PostgreSQL maintenance database failed")
	}
	var version string
	if err := admin.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		t.Fatal("read PostgreSQL server version failed")
	}
	if version != "16.4" && !strings.HasPrefix(version, "16.4.") {
		t.Fatalf("PostgreSQL 16.4 required, found %s", version)
	}

	databaseName := "threadline_worker_outbox_go_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if !safeWorkerOutboxDatabaseName(databaseName) {
		t.Fatal("refusing unsafe disposable PostgreSQL database name")
	}
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		t.Fatal("create disposable PostgreSQL test database failed")
	}

	config := adminConfig.Copy()
	config.Database = databaseName
	var firstDatabase, secondDatabase *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if firstDatabase != nil {
			if closeErr := firstDatabase.Close(cleanupCtx); closeErr != nil {
				t.Error("close first Worker database connection failed")
			}
		}
		if secondDatabase != nil {
			if closeErr := secondDatabase.Close(cleanupCtx); closeErr != nil {
				t.Error("close second Worker database connection failed")
			}
		}
		if safeWorkerOutboxDatabaseName(databaseName) {
			if _, dropErr := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedName+" WITH (FORCE)"); dropErr != nil {
				t.Error("drop disposable Worker database failed")
			}
		} else {
			t.Error("refusing to drop unexpected Worker database")
		}
		if closeErr := admin.Close(cleanupCtx); closeErr != nil {
			t.Error("close PostgreSQL maintenance connection failed")
		}
	})

	firstDatabase, err = pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("connect to first disposable Worker database failed")
	}
	secondDatabase, err = pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("connect to second disposable Worker database failed")
	}
	if _, err := firstDatabase.Exec(ctx, "CREATE EXTENSION pgcrypto"); err != nil {
		t.Fatal("pre-provision pgcrypto in disposable Worker database failed")
	}
	for index := 1; index <= 9; index++ {
		prefix := fmt.Sprintf("%06d_", index)
		matches, globErr := filepath.Glob(filepath.Join("..", "..", "..", "..", "db", "migrations", prefix+"*.up.sql"))
		if globErr != nil || len(matches) != 1 {
			t.Fatalf("resolve migration %s failed: %v (%v)", prefix, globErr, matches)
		}
		migration, readErr := os.ReadFile(matches[0])
		if readErr != nil {
			t.Fatalf("read migration %s failed", prefix)
		}
		if _, applyErr := firstDatabase.PgConn().Exec(ctx, string(migration)).ReadAll(); applyErr != nil {
			t.Fatalf("apply migration %s failed: %v", prefix, applyErr)
		}
	}
	return ctx, firstDatabase, secondDatabase
}

func safeWorkerOutboxDatabaseName(databaseName string) bool {
	const prefix = "threadline_worker_outbox_go_test_"
	if !strings.HasPrefix(databaseName, prefix) {
		return false
	}
	pid, timestamp, found := strings.Cut(strings.TrimPrefix(databaseName, prefix), "_")
	return found && decimalDigits(pid) && decimalDigits(timestamp)
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func insertWorkerOutboxFixtures(t *testing.T, ctx context.Context, database *pgx.Conn, count int) {
	t.Helper()
	if _, err := database.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ('tenant-worker-go-synthetic', 'Worker Go Synthetic', 1, 'policy-worker-go-v1')
	`); err != nil {
		t.Fatalf("create Worker organization fixture failed: %v", err)
	}
	if _, err := database.Exec(ctx, `
		INSERT INTO domain.domain_events (
			tenant_id, event_id, event_type, schema_version,
			aggregate_kind, aggregate_id, payload, occurred_at, enqueued_at
		)
		SELECT
			'tenant-worker-go-synthetic', 'event-worker-go-' || ordinal,
			'worker.synthetic', 1, 'SyntheticAggregate', 'aggregate-worker-go-' || ordinal,
			decode('00ff80', 'hex'), clock_timestamp(),
			clock_timestamp() + ordinal * interval '1 microsecond'
		FROM generate_series(1, $1) AS ordinal
	`, count); err != nil {
		t.Fatalf("create Worker event fixtures failed: %v", err)
	}
	if _, err := database.Exec(ctx, `
		INSERT INTO domain.transactional_outbox (
			tenant_id, event_id, destination, next_attempt_at,
			policy_id, policy_snapshot_digest,
			effective_lease_ms, effective_absolute_lifetime_ms,
			effective_event_retry_ceiling,
			effective_transport_base_ms, effective_transport_cap_ms,
			effective_unknown_base_ms, effective_unknown_cap_ms,
			effective_event_base_ms, effective_event_cap_ms,
			effective_retention_days
		)
		SELECT
			tenant_id, event_id, 'domain-events', clock_timestamp() - interval '1 second',
			'threadline.outbox.policy/v1', decode(repeat('ab', 32), 'hex'),
			30000, 300000, 8, 1000, 60000, 5000, 300000, 5000, 300000, 90
		FROM domain.domain_events
		WHERE tenant_id = 'tenant-worker-go-synthetic'
	`); err != nil {
		t.Fatalf("create Worker Outbox fixtures failed: %v", err)
	}
}
