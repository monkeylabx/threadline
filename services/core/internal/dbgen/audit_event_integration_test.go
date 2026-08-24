package dbgen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestAuditEventAppendChainAndIdempotencyIntegration(t *testing.T) {
	ctx, database := openAuditEventDatabase(t)
	queries := New(database)
	createAuditTenant(t, ctx, queries, "tenant-audit-alpha-synthetic")
	createAuditTenant(t, ctx, queries, "tenant-audit-beta-synthetic")

	first := appendAuditEvent(t, ctx, database, auditCandidate{
		tenantID: "tenant-audit-alpha-synthetic", eventID: "audit-event-1", hashByte: 1,
	})
	if first.TenantSequence != 1 || !first.RecordedAt.Valid ||
		!bytes.Equal(first.PreviousEventHash, make([]byte, 32)) {
		t.Fatalf("genesis Event returned unexpected database facts: %#v", first)
	}

	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second := appendAuditEventInTx(t, ctx, tx, auditCandidate{
		tenantID: "tenant-audit-alpha-synthetic", eventID: "audit-event-2", hashByte: 2,
	})
	third := appendAuditEventInTx(t, ctx, tx, auditCandidate{
		tenantID: "tenant-audit-alpha-synthetic", eventID: "audit-event-3", hashByte: 3,
	})
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if second.TenantSequence != 2 || third.TenantSequence != 3 ||
		!bytes.Equal(second.PreviousEventHash, first.EventHash) ||
		!bytes.Equal(third.PreviousEventHash, second.EventHash) ||
		third.RecordedAt.Time.Before(second.RecordedAt.Time) {
		t.Fatal("multi-Event transaction did not preserve the contiguous chain")
	}
	head, err := queries.GetAuditTenantHead(ctx, "tenant-audit-alpha-synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if head.LastSequence != 3 || head.LastAuditEventID == nil || *head.LastAuditEventID != third.AuditEventID ||
		!bytes.Equal(head.LastEventHash, third.EventHash) {
		t.Fatal("Tenant head does not bind the final Event")
	}

	exact, err := queries.ObserveAuditEventIdempotency(ctx, observeParams(third, "authorized"))
	if err != nil || !exact.ExactMatch || exact.TenantSequence != third.TenantSequence {
		t.Fatalf("exact retry observation = (%#v, %v)", exact, err)
	}
	conflict, err := queries.ObserveAuditEventIdempotency(ctx, observeParams(third, "state_conflict"))
	if err != nil || conflict.ExactMatch {
		t.Fatalf("conflicting retry observation = (%#v, %v)", conflict, err)
	}
	_, err = queries.ObserveAuditEventIdempotency(ctx, ObserveAuditEventIdempotencyParams{
		TenantID: "tenant-audit-beta-synthetic", AuditEventID: third.AuditEventID,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-Tenant observation error = %v, want pgx.ErrNoRows", err)
	}

	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appendAuditEventInTx(t, ctx, tx, auditCandidate{
		tenantID: "tenant-audit-alpha-synthetic", eventID: "audit-event-rollback", hashByte: 4,
	})
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetAuditEvent(ctx, GetAuditEventParams{
		TenantID: "tenant-audit-alpha-synthetic", AuditEventID: "audit-event-rollback",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("rolled-back Event lookup error = %v, want pgx.ErrNoRows", err)
	}

	if _, err := database.Exec(ctx, `UPDATE domain.audit_events SET reason = 'failed' WHERE tenant_id = $1`, first.TenantID); err == nil {
		t.Fatal("Audit Event update unexpectedly succeeded")
	}
	if _, err := database.Exec(ctx, `DELETE FROM domain.audit_events WHERE tenant_id = $1`, first.TenantID); err == nil {
		t.Fatal("Audit Event delete unexpectedly succeeded")
	}
	if _, err := database.Exec(ctx, `DELETE FROM domain.audit_tenant_heads WHERE tenant_id = $1`, first.TenantID); err == nil {
		t.Fatal("Audit Tenant head delete unexpectedly succeeded")
	}
	if _, err := database.Exec(ctx, `TRUNCATE domain.audit_tenant_heads, domain.audit_events`); err == nil {
		t.Fatal("Audit tables truncate unexpectedly succeeded")
	}
}

func TestAuditEventConstraintsAndStaleSlotIntegration(t *testing.T) {
	ctx, database := openAuditEventDatabase(t)
	queries := New(database)
	createAuditTenant(t, ctx, queries, "tenant-audit-constraints-synthetic")
	first := appendAuditEvent(t, ctx, database, auditCandidate{
		tenantID: "tenant-audit-constraints-synthetic", eventID: "audit-constraint-1", hashByte: 10,
	})

	invalid := []struct {
		name   string
		mutate func(*AppendAuditEventAndAdvanceHeadParams)
	}{
		{name: "unknown action", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.Action = "message.read" }},
		{name: "unknown outcome", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.Outcome = "maybe" }},
		{name: "unknown reason", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.Reason = "unspecified" }},
		{name: "unknown actor type", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.PrincipalActorType = 9 }},
		{name: "unknown target type", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.TargetType = "message" }},
		{name: "target mismatch", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.TargetType = "outbox_entry" }},
		{name: "nonpositive target version", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) {
			zero := int64(0)
			p.TargetVersion = &zero
		}},
		{name: "short hash", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.EventHash = []byte{1} }},
		{name: "short previous hash", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.PreviousEventHash = []byte{1} }},
		{name: "short evidence digest", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.EvidenceDigest = []byte{1} }},
		{name: "noncanonical identifier", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.RequestID = "bad?request" }},
		{name: "unicode whitespace identifier", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) { p.RequestID = "\u00a0bad-request" }},
		{name: "unbound recovery reference", mutate: func(p *AppendAuditEventAndAdvanceHeadParams) {
			recoveryID := "recovery-case-1"
			p.RecoveryCaseID = &recoveryID
		}},
	}
	for _, testCase := range invalid {
		t.Run(testCase.name, func(t *testing.T) {
			tx, err := database.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			qtx := New(tx)
			slot, err := qtx.LockAuditAppendSlot(ctx, first.TenantID)
			if err != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(err)
			}
			params := paramsForSlot(slot, auditCandidate{
				tenantID: first.TenantID, eventID: "invalid-" + testCase.name, hashByte: 11,
			})
			testCase.mutate(&params)
			if _, err := qtx.AppendAuditEventAndAdvanceHead(ctx, params); err == nil {
				_ = tx.Rollback(ctx)
				t.Fatal("invalid Audit Event unexpectedly succeeded")
			}
			_ = tx.Rollback(ctx)
		})
	}

	version := int64(7)
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qtx := New(tx)
	slot, err := qtx.LockAuditAppendSlot(ctx, first.TenantID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	versioned := paramsForSlot(slot, auditCandidate{
		tenantID: first.TenantID, eventID: "audit-versioned-channel", hashByte: 12,
	})
	versioned.TargetVersion = &version
	if _, err := qtx.AppendAuditEventAndAdvanceHead(ctx, versioned); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal("positive target version allowed by the frozen contract was rejected")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qtx = New(tx)
	slot, err = qtx.LockAuditAppendSlot(ctx, first.TenantID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	wrongPreviousID := "not-the-current-event"
	params := paramsForSlot(slot, auditCandidate{
		tenantID: first.TenantID, eventID: "audit-stale-cas", hashByte: 12,
	})
	params.PreviousAuditEventID = &wrongPreviousID
	if _, err := qtx.AppendAuditEventAndAdvanceHead(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		t.Fatalf("stale head CAS error = %v, want pgx.ErrNoRows", err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("deferred head coverage unexpectedly allowed an unadvanced Event")
	}

	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oldSlot, err := New(tx).LockAuditAppendSlot(ctx, first.TenantID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	crossTransaction := paramsForSlot(oldSlot, auditCandidate{
		tenantID: first.TenantID, eventID: "audit-cross-transaction-slot", hashByte: 13,
	})
	if _, err := New(tx).AppendAuditEventAndAdvanceHead(ctx, crossTransaction); !errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		t.Fatalf("cross-transaction slot error = %v, want pgx.ErrNoRows", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	staleSlot, err := New(tx).LockAuditAppendSlot(ctx, first.TenantID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	stale := paramsForSlot(staleSlot, auditCandidate{
		tenantID: first.TenantID, eventID: "audit-stale-slot", hashByte: 14,
	})
	stale.TenantSequence = 1
	stale.PreviousAuditEventID = nil
	stale.PreviousEventHash = make([]byte, 32)
	if _, err := New(tx).AppendAuditEventAndAdvanceHead(ctx, stale); err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("stale append slot unexpectedly succeeded")
	}
	_ = tx.Rollback(ctx)
}

func TestAuditEventHeadLockSerializesConcurrentAppendIntegration(t *testing.T) {
	ctx, database := openAuditEventDatabase(t)
	queries := New(database)
	createAuditTenant(t, ctx, queries, "tenant-audit-concurrent-synthetic")
	if err := queries.EnsureAuditTenantHead(ctx, "tenant-audit-concurrent-synthetic"); err != nil {
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
	slot1, err := New(tx1).LockAuditAppendSlot(ctx, "tenant-audit-concurrent-synthetic")
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatal(err)
	}
	tx2, err := secondConnection.Begin(ctx)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatal(err)
	}
	type slotResult struct {
		slot LockAuditAppendSlotRow
		err  error
	}
	result := make(chan slotResult, 1)
	lockCtx, cancelLock := context.WithCancel(ctx)
	defer cancelLock()
	blockedPID := secondConnection.PgConn().PID()
	go func() {
		slot, lockErr := New(tx2).LockAuditAppendSlot(lockCtx, "tenant-audit-concurrent-synthetic")
		result <- slotResult{slot: slot, err: lockErr}
	}()
	deadline := time.Now().Add(3 * time.Second)
	var waitErr error
	secondCompleted := false
	for {
		select {
		case got := <-result:
			secondCompleted = true
			waitErr = fmt.Errorf("second head lock returned before first transaction completed: %v", got.err)
		default:
		}
		if waitErr != nil {
			break
		}
		var waiting bool
		err = observerConnection.QueryRow(ctx, `
			SELECT COALESCE(state = 'active' AND wait_event_type = 'Lock', false)
			FROM pg_stat_activity
			WHERE pid = $1
		`, blockedPID).Scan(&waiting)
		if err != nil {
			waitErr = fmt.Errorf("observe second backend lock wait: %w", err)
			break
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			waitErr = fmt.Errorf("second backend never entered a PostgreSQL lock wait")
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if waitErr != nil {
		_ = tx1.Rollback(ctx)
		cancelLock()
		if !secondCompleted {
			<-result
		}
		_ = tx2.Rollback(ctx)
		t.Fatal(waitErr)
	}
	params := paramsForSlot(slot1, auditCandidate{
		tenantID: "tenant-audit-concurrent-synthetic", eventID: "audit-concurrent-1", hashByte: 20,
	})
	if _, err := New(tx1).AppendAuditEventAndAdvanceHead(ctx, params); err != nil {
		_ = tx1.Rollback(ctx)
		cancelLock()
		<-result
		_ = tx2.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx1.Commit(ctx); err != nil {
		cancelLock()
		<-result
		_ = tx2.Rollback(ctx)
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got.err != nil || got.slot.TenantSequence != 2 || !bytes.Equal(got.slot.PreviousEventHash, params.EventHash) {
			_ = tx2.Rollback(ctx)
			t.Fatalf("serialized slot = (%#v, %v), want sequence 2", got.slot, got.err)
		}
	case <-time.After(3 * time.Second):
		cancelLock()
		<-result
		_ = tx2.Rollback(ctx)
		t.Fatal("second head lock did not resume after commit")
	}
	if err := tx2.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAuditEventMigrationDownUpIntegration(t *testing.T) {
	ctx, database := openPostgresIntegrationTest(t, "audit_event")
	applyPostgresTestMigrations(t, ctx, database,
		"000001_core_foundation.up.sql", "000002_organization.up.sql", "000010_audit_event.up.sql",
	)
	queries := New(database)
	createAuditTenant(t, ctx, queries, "tenant-audit-roundtrip-synthetic")
	applyPostgresTestMigrations(t, ctx, database, "000010_audit_event.down.sql")
	var count int
	if err := database.QueryRow(ctx, `SELECT count(*) FROM domain.organizations`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("Audit down migration changed prior Organization state: count=%d err=%v", count, err)
	}
	applyPostgresTestMigrations(t, ctx, database, "000010_audit_event.up.sql")
	if err := New(database).EnsureAuditTenantHead(ctx, "tenant-audit-roundtrip-synthetic"); err != nil {
		t.Fatal("Audit up migration was not repeatable")
	}
}

type auditCandidate struct {
	tenantID string
	eventID  string
	hashByte byte
}

func openAuditEventDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	ctx, database := openPostgresIntegrationTest(t, "audit_event")
	applyPostgresTestMigrations(t, ctx, database,
		"000001_core_foundation.up.sql", "000002_organization.up.sql", "000010_audit_event.up.sql",
	)
	return ctx, database
}

func createAuditTenant(t *testing.T, ctx context.Context, queries *Queries, tenantID string) {
	t.Helper()
	if _, err := queries.CreateOrganization(ctx, CreateOrganizationParams{
		TenantID: tenantID, DisplayName: "Audit Synthetic", State: 1, PolicyVersion: "policy-audit-v1",
	}); err != nil {
		t.Fatal(err)
	}
}

func appendAuditEvent(t *testing.T, ctx context.Context, database *pgx.Conn, candidate auditCandidate) AppendAuditEventAndAdvanceHeadRow {
	t.Helper()
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row := appendAuditEventInTx(t, ctx, tx, candidate)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return row
}

func appendAuditEventInTx(t *testing.T, ctx context.Context, tx pgx.Tx, candidate auditCandidate) AppendAuditEventAndAdvanceHeadRow {
	t.Helper()
	queries := New(tx)
	if err := queries.EnsureAuditTenantHead(ctx, candidate.tenantID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	slot, err := queries.LockAuditAppendSlot(ctx, candidate.tenantID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	row, err := queries.AppendAuditEventAndAdvanceHead(ctx, paramsForSlot(slot, candidate))
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	return row
}

func paramsForSlot(slot LockAuditAppendSlotRow, candidate auditCandidate) AppendAuditEventAndAdvanceHeadParams {
	return AppendAuditEventAndAdvanceHeadParams{
		SlotTransactionID: slot.SlotTransactionID, RecordedAt: slot.RecordedAt,
		TenantID: candidate.tenantID, AuditEventID: candidate.eventID,
		TenantSequence: slot.TenantSequence, PreviousAuditEventID: slot.PreviousAuditEventID,
		PrincipalActorType: 1, PrincipalActorID: "actor-audit-synthetic",
		Action: "channel.archive", Outcome: "succeeded", Reason: "authorized",
		TargetType: "channel", TargetID: "channel-audit-synthetic",
		PolicyVersion: "policy-audit-v1", RequestID: "request-" + candidate.eventID,
		PreviousEventHash: slot.PreviousEventHash,
		EventHash:         bytes.Repeat([]byte{candidate.hashByte}, 32),
	}
}

func observeParams(event AppendAuditEventAndAdvanceHeadRow, reason string) ObserveAuditEventIdempotencyParams {
	return ObserveAuditEventIdempotencyParams{
		TenantID: event.TenantID, AuditEventID: event.AuditEventID,
		PrincipalActorType: event.PrincipalActorType, PrincipalActorID: event.PrincipalActorID,
		Action: event.Action, Outcome: event.Outcome, Reason: reason,
		TargetType: event.TargetType, TargetID: event.TargetID, TargetVersion: event.TargetVersion,
		PolicyVersion: event.PolicyVersion, RequestID: event.RequestID,
		ApprovalID: event.ApprovalID, RecoveryCaseID: event.RecoveryCaseID, EvidenceDigest: event.EvidenceDigest,
	}
}
