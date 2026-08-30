package auditstore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
)

func TestAuditStorePostgresAppendRetryConflictAndRollback(t *testing.T) {
	ctx, database := openAuditStoreDatabase(t)
	createAuditStoreTenant(t, ctx, database, "tenant-1")
	createAuditStoreTenant(t, ctx, database, "tenant-2")

	input := testCandidate()
	created := appendAndCommit(t, ctx, database, input)
	if created.disposition != dispositionCreated || created.tenantSequence != 1 ||
		created.recordedAt.IsZero() || !bytes.Equal(created.previousEventHash, make([]byte, hashBytes)) ||
		len(created.eventHash) != hashBytes {
		t.Fatalf("created Event = %#v, want genesis database facts", created)
	}

	retried := appendAndCommit(t, ctx, database, input)
	if retried.disposition != dispositionAlreadyPresent || retried.tenantSequence != created.tenantSequence ||
		!retried.recordedAt.Equal(created.recordedAt) || !bytes.Equal(retried.eventHash, created.eventHash) {
		t.Fatalf("exact retry = %#v, want existing Event", retried)
	}
	assertAuditStoreCounts(t, ctx, database, "tenant-1", 1, 1)

	conflicting := input
	conflicting.reason = reasonStateConflict
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = appendEvent(ctx, tx, conflicting)
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if !hasErrorCode(err, errorIdempotencyConflict) {
		t.Fatalf("conflicting retry error = %v, want %q", err, errorIdempotencyConflict)
	}

	rolledBack := input
	rolledBack.auditEventID = "audit-event-rollback"
	rolledBack.requestID = "request-rollback"
	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appendEvent(ctx, tx, rolledBack); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertAuditStoreCounts(t, ctx, database, "tenant-1", 1, 1)

	crossTenant := input
	crossTenant.tenantID = "tenant-2"
	crossTenantResult := appendAndCommit(t, ctx, database, crossTenant)
	if crossTenantResult.tenantSequence != 1 {
		t.Fatal("same Event ID in another Tenant did not receive an independent genesis slot")
	}

	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second := input
	second.auditEventID = "audit-event-2"
	second.requestID = "request-2"
	secondResult, err := appendEvent(ctx, tx, second)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	third := input
	third.auditEventID = "audit-event-3"
	third.requestID = "request-3"
	thirdResult, err := appendEvent(ctx, tx, third)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if secondResult.tenantSequence != 2 || thirdResult.tenantSequence != 3 ||
		!bytes.Equal(thirdResult.previousEventHash, secondResult.eventHash) {
		t.Fatal("multi-Event transaction did not preserve the Tenant chain")
	}
	assertAuditStoreCounts(t, ctx, database, "tenant-1", 3, 3)
}

func TestAuditStorePostgresSerializesConcurrentExactRetry(t *testing.T) {
	ctx, database := openAuditStoreDatabase(t)
	createAuditStoreTenant(t, ctx, database, "tenant-1")
	if err := dbgen.New(database).EnsureAuditTenantHead(ctx, "tenant-1"); err != nil {
		t.Fatal(err)
	}

	secondConnection, err := pgx.ConnectConfig(ctx, database.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondConnection.Close(context.Background()) })
	observerConnection, err := pgx.ConnectConfig(ctx, database.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = observerConnection.Close(context.Background()) })
	tx1, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := appendEvent(ctx, tx1, testCandidate())
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatal(err)
	}
	tx2, err := secondConnection.Begin(ctx)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatal(err)
	}
	result := make(chan concurrentAppendResult, 1)
	appendCtx, cancelAppend := context.WithCancel(ctx)
	defer cancelAppend()
	blockedPID := secondConnection.PgConn().PID()
	go func() {
		got, appendErr := appendEvent(appendCtx, tx2, testCandidate())
		result <- concurrentAppendResult{event: got, err: appendErr}
	}()
	completed, waitErr := waitForAuditStoreLock(ctx, observerConnection, blockedPID, result)
	if waitErr != nil {
		_ = tx1.Rollback(ctx)
		cancelAppend()
		if !completed {
			<-result
		}
		_ = tx2.Rollback(ctx)
		t.Fatal(waitErr)
	}
	if err := tx1.Commit(ctx); err != nil {
		cancelAppend()
		<-result
		_ = tx2.Rollback(ctx)
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got.err != nil || got.event.disposition != dispositionAlreadyPresent ||
			got.event.tenantSequence != first.tenantSequence || !bytes.Equal(got.event.eventHash, first.eventHash) {
			_ = tx2.Rollback(ctx)
			t.Fatalf("concurrent retry = (%#v, %v), want existing Event", got.event, got.err)
		}
	case <-time.After(3 * time.Second):
		cancelAppend()
		<-result
		_ = tx2.Rollback(ctx)
		t.Fatal("concurrent retry did not resume after head commit")
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertAuditStoreCounts(t, ctx, database, "tenant-1", 1, 1)
}

func TestAuditStorePostgresStorageFailuresRemainAtomic(t *testing.T) {
	ctx, database := openAuditStoreDatabase(t)
	createAuditStoreTenant(t, ctx, database, "tenant-1")
	if err := dbgen.New(database).EnsureAuditTenantHead(ctx, "tenant-1"); err != nil {
		t.Fatal(err)
	}

	if _, err := database.Exec(ctx, `
		CREATE FUNCTION domain.reject_audit_store_test_append()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'forced synthetic Audit append failure';
		END;
		$$;
		CREATE TRIGGER zz_audit_store_test_failure
		BEFORE INSERT ON domain.audit_events
		FOR EACH ROW EXECUTE FUNCTION domain.reject_audit_store_test_append();
	`); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = appendEvent(ctx, tx, testCandidate())
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if !hasErrorCode(err, errorPersistence) {
		t.Fatalf("forced finalize error = %v, want %q", err, errorPersistence)
	}
	assertAuditStoreCounts(t, ctx, database, "tenant-1", 0, 0)
	if _, err := database.Exec(ctx, `
		DROP TRIGGER zz_audit_store_test_failure ON domain.audit_events;
		DROP FUNCTION domain.reject_audit_store_test_append();
	`); err != nil {
		t.Fatal(err)
	}

	missingTenant := testCandidate()
	missingTenant.tenantID = "tenant-missing"
	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = appendEvent(ctx, tx, missingTenant)
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if !hasErrorCode(err, errorPersistence) {
		t.Fatalf("missing Organization error = %v, want %q", err, errorPersistence)
	}
	var rows int
	if err := database.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM domain.audit_events WHERE tenant_id = 'tenant-missing') +
			(SELECT count(*) FROM domain.audit_tenant_heads WHERE tenant_id = 'tenant-missing')
	`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("failed ensure left %d Audit rows: %v", rows, err)
	}
}

func appendAndCommit(t *testing.T, ctx context.Context, database *pgx.Conn, input candidate) event {
	t.Helper()
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := appendEvent(ctx, tx, input)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result
}

func waitForAuditStoreLock(
	ctx context.Context,
	observer *pgx.Conn,
	blockedPID uint32,
	result <-chan concurrentAppendResult,
) (bool, error) {
	deadline := time.Now().Add(3 * time.Second)
	for {
		select {
		case got := <-result:
			return true, fmt.Errorf("second append returned before the first transaction completed: %v", got.err)
		default:
		}
		var waiting bool
		if err := observer.QueryRow(ctx, `
			SELECT COALESCE(state = 'active' AND wait_event_type = 'Lock', false)
			FROM pg_stat_activity WHERE pid = $1
		`, blockedPID).Scan(&waiting); err != nil {
			return false, fmt.Errorf("observe second backend lock wait: %w", err)
		}
		if waiting {
			return false, nil
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("second backend never entered a PostgreSQL lock wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type concurrentAppendResult struct {
	event event
	err   error
}

func openAuditStoreDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run Audit store PostgreSQL tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("parse PostgreSQL maintenance DSN failed")
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("connect to PostgreSQL maintenance database failed")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	var version string
	if err := admin.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "16.4" && !strings.HasPrefix(version, "16.4.") {
		t.Fatalf("PostgreSQL 16.4 required, found %s", version)
	}
	databaseName := "threadline_audit_store_go_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if !safeAuditStoreDatabaseName(databaseName) {
		t.Fatal("refusing unsafe disposable PostgreSQL database name")
	}
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		t.Fatal(err)
	}
	config := adminConfig.Copy()
	config.Database = databaseName
	var database *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if database != nil {
			_ = database.Close(cleanupCtx)
		}
		if safeAuditStoreDatabaseName(databaseName) {
			_, _ = admin.Exec(cleanupCtx, "DROP DATABASE "+quotedName+" WITH (FORCE)")
		}
	})
	database, err = pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"000001_core_foundation.up.sql", "000002_organization.up.sql", "000010_audit_event.up.sql",
	} {
		migration, readErr := os.ReadFile(filepath.Join("..", "..", "..", "..", "db", "migrations", name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, execErr := database.PgConn().Exec(ctx, string(migration)).ReadAll(); execErr != nil {
			t.Fatalf("apply %s failed", name)
		}
	}
	return ctx, database
}

func safeAuditStoreDatabaseName(value string) bool {
	prefix := "threadline_audit_store_go_test_"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	pid, timestamp, found := strings.Cut(strings.TrimPrefix(value, prefix), "_")
	return found && digitsOnly(pid) && digitsOnly(timestamp)
}

func digitsOnly(value string) bool {
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

func createAuditStoreTenant(t *testing.T, ctx context.Context, database *pgx.Conn, tenantID string) {
	t.Helper()
	if _, err := database.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ($1, 'Audit Store Synthetic', 1, 'policy-1')
	`, tenantID); err != nil {
		t.Fatal(err)
	}
}

func assertAuditStoreCounts(t *testing.T, ctx context.Context, database *pgx.Conn, tenantID string, events, sequence int) {
	t.Helper()
	var eventCount int
	var lastSequence int64
	if err := database.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM domain.audit_events WHERE tenant_id = $1),
			(SELECT last_sequence FROM domain.audit_tenant_heads WHERE tenant_id = $1)
	`, tenantID).Scan(&eventCount, &lastSequence); err != nil {
		t.Fatal(err)
	}
	if eventCount != events || lastSequence != int64(sequence) {
		t.Fatalf("tenant state = (%d Events, sequence %d), want (%d, %d)", eventCount, lastSequence, events, sequence)
	}
}

func dbSlot(value appendSlot) dbgen.LockAuditAppendSlotRow {
	return dbgen.LockAuditAppendSlotRow{
		TenantID: value.tenantID, TenantSequence: value.tenantSequence,
		RecordedAt: pgtypeTimestamp(value.recordedAt), SlotTransactionID: value.transactionID,
		PreviousAuditEventID: cloneString(value.previousAuditEventID), PreviousEventHash: bytes.Clone(value.previousEventHash),
	}
}

func pgtypeTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
