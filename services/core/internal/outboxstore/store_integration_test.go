package outboxstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestInsertCreatesOnceAndObservesExactRetry(t *testing.T) {
	ctx, database := openOutboxStoreDatabase(t)
	createOutboxStoreTenant(t, ctx, database)
	input := testCandidate()

	created := insertAndCommit(t, ctx, database, input)
	if created.disposition != dispositionCreated || created.eventID != input.eventID ||
		created.outboxEntryID <= 0 || created.enqueuedAt.IsZero() {
		t.Fatalf("created result = %#v, want generated Event/Entry facts", created)
	}

	reloadedPolicy := input
	reloadedPolicy.policy.snapshotDigest = append([]byte(nil), input.policy.snapshotDigest...)
	reloadedPolicy.policy.snapshotDigest[0] = 1
	reloadedPolicy.policy.leaseMS = 45_000
	observed := insertAndCommit(t, ctx, database, reloadedPolicy)
	if observed.disposition != dispositionAlreadyPresent ||
		observed.eventID != created.eventID ||
		observed.outboxEntryID != created.outboxEntryID ||
		!observed.enqueuedAt.Equal(created.enqueuedAt) {
		t.Fatalf("exact retry result = %#v, want existing %#v", observed, created)
	}
	assertOutboxRowCounts(t, ctx, database, 1, 1)
}

func TestInsertConflictingImmutableFactsFailsClosed(t *testing.T) {
	ctx, database := openOutboxStoreDatabase(t)
	createOutboxStoreTenant(t, ctx, database)
	input := testCandidate()
	insertAndCommit(t, ctx, database, input)

	conflicting := input
	conflicting.payload = []byte(`{"message_id":"different"}`)
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = insert(ctx, tx, conflicting)
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if !hasStoreErrorCode(err, errorIdempotencyConflict) {
		t.Fatalf("conflicting insert error = %v, want %q", err, errorIdempotencyConflict)
	}
	assertOutboxRowCounts(t, ctx, database, 1, 1)
}

func TestInsertLeavesTransactionLifetimeToCaller(t *testing.T) {
	ctx, database := openOutboxStoreDatabase(t)
	createOutboxStoreTenant(t, ctx, database)

	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insert(ctx, tx, testCandidate()); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertOutboxRowCounts(t, ctx, database, 0, 0)
}

func TestCallerCommitsDomainStateEventAndEntryTogether(t *testing.T) {
	ctx, database := openOutboxStoreDatabase(t)
	input := testCandidate()
	input.tenantID = "tenant-outbox-atomic-commit-synthetic"

	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ($1, 'Outbox Atomic Commit Synthetic', 1, 'policy-outbox-atomic-v1')
	`, input.tenantID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := insert(ctx, tx, input); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertTenantOutboxRowCounts(t, ctx, database, input.tenantID, 1, 1, 1)
}

func TestCallerRollbackRemovesDomainStateEventAndEntryAfterSiblingFailure(t *testing.T) {
	ctx, database := openOutboxStoreDatabase(t)
	input := testCandidate()
	input.tenantID = "tenant-outbox-atomic-synthetic"

	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ($1, 'Outbox Atomic Synthetic', 1, 'policy-outbox-atomic-v1')
	`, input.tenantID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := insert(ctx, tx, input); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ($1, 'Forced Sibling Failure Synthetic', 1, 'policy-outbox-atomic-v1')
	`, input.tenantID); err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("forced sibling constraint violation unexpectedly succeeded")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	assertTenantOutboxRowCounts(t, ctx, database, input.tenantID, 0, 0, 0)
}

func assertTenantOutboxRowCounts(
	t *testing.T,
	ctx context.Context,
	database *pgx.Conn,
	tenantID string,
	organizationsWant int,
	eventsWant int,
	entriesWant int,
) {
	t.Helper()
	var organizations, events, entries int
	if err := database.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM domain.organizations WHERE tenant_id = $1),
			(SELECT count(*) FROM domain.domain_events WHERE tenant_id = $1),
			(SELECT count(*) FROM domain.transactional_outbox WHERE tenant_id = $1)
	`, tenantID).Scan(&organizations, &events, &entries); err != nil {
		t.Fatal(err)
	}
	if organizations != organizationsWant || events != eventsWant || entries != entriesWant {
		t.Fatalf(
			"tenant rows = (%d organizations, %d Events, %d Entries), want (%d, %d, %d)",
			organizations,
			events,
			entries,
			organizationsWant,
			eventsWant,
			entriesWant,
		)
	}
}

func TestInsertRequiresReadCommittedBeforeMutation(t *testing.T) {
	ctx, database := openOutboxStoreDatabase(t)
	createOutboxStoreTenant(t, ctx, database)

	tx, err := database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	_, err = insert(ctx, tx, testCandidate())
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if !hasStoreErrorCode(err, errorPersistence) {
		t.Fatalf("repeatable-read insert error = %v, want %q", err, errorPersistence)
	}
	assertOutboxRowCounts(t, ctx, database, 0, 0)
}

func insertAndCommit(t *testing.T, ctx context.Context, database *pgx.Conn, input candidate) result {
	t.Helper()
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := insert(ctx, tx, input)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return got
}

func assertOutboxRowCounts(t *testing.T, ctx context.Context, database *pgx.Conn, events, entries int) {
	t.Helper()
	var eventCount, entryCount int
	if err := database.QueryRow(ctx, `SELECT count(*) FROM domain.domain_events`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(ctx, `SELECT count(*) FROM domain.transactional_outbox`).Scan(&entryCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != events || entryCount != entries {
		t.Fatalf("row counts = (%d Events, %d Entries), want (%d, %d)", eventCount, entryCount, events, entries)
	}
}

func hasStoreErrorCode(err error, code errorCode) bool {
	var storeErr *storeFailure
	return errors.As(err, &storeErr) && storeErr.category() == code
}

func openOutboxStoreDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run Outbox store integration tests")
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
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := admin.Close(cleanupCtx); err != nil {
			t.Error("close PostgreSQL maintenance connection failed")
		}
	})
	var version string
	if err := admin.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		t.Fatal("read PostgreSQL server version failed")
	}
	if version != "16.4" && !strings.HasPrefix(version, "16.4.") {
		t.Fatalf("PostgreSQL 16.4 required, found %s", version)
	}

	databaseName := "threadline_outbox_store_go_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if !safeOutboxStoreTestDatabaseName(databaseName) {
		t.Fatal("refusing unsafe disposable PostgreSQL database name")
	}
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		t.Fatal("create disposable PostgreSQL test database failed")
	}
	config := adminConfig.Copy()
	config.Database = databaseName
	var database *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if database != nil {
			if err := database.Close(cleanupCtx); err != nil {
				t.Error("close disposable PostgreSQL test database failed")
			}
		}
		if !safeOutboxStoreTestDatabaseName(databaseName) {
			t.Error("refusing to drop unexpected PostgreSQL test database")
			return
		}
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable PostgreSQL test database failed")
		}
	})
	database, err = pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("connect to disposable PostgreSQL test database failed")
	}
	for index := 1; index <= 8; index++ {
		prefix := fmt.Sprintf("%06d_", index)
		matches, globErr := filepath.Glob(filepath.Join("..", "..", "..", "..", "db", "migrations", prefix+"*.up.sql"))
		if globErr != nil || len(matches) != 1 {
			t.Fatalf("resolve migration %s failed: %v (%v)", prefix, globErr, matches)
		}
		migration, readErr := os.ReadFile(matches[0])
		if readErr != nil {
			t.Fatalf("read migration %s failed", prefix)
		}
		if _, applyErr := database.PgConn().Exec(ctx, string(migration)).ReadAll(); applyErr != nil {
			t.Fatalf("apply migration %s failed: %v", prefix, applyErr)
		}
	}
	return ctx, database
}

func safeOutboxStoreTestDatabaseName(databaseName string) bool {
	const prefix = "threadline_outbox_store_go_test_"
	if !strings.HasPrefix(databaseName, prefix) {
		return false
	}
	pid, timestamp, found := strings.Cut(strings.TrimPrefix(databaseName, prefix), "_")
	return found && decimalDigitsOnly(pid) && decimalDigitsOnly(timestamp)
}

func decimalDigitsOnly(value string) bool {
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

func createOutboxStoreTenant(t *testing.T, ctx context.Context, database *pgx.Conn) {
	t.Helper()
	if _, err := database.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ('tenant-1', 'Outbox Store Synthetic', 1, 'policy-outbox-store-v1')
	`); err != nil {
		t.Fatal("create Outbox store Tenant fixture failed")
	}
}
