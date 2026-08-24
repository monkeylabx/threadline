package auditstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestAppendUsesLockedSlotAndAtomicFinalize(t *testing.T) {
	t.Parallel()

	input := testCandidate()
	slot := testSlot()
	_, digest, err := hashEvent(input, slot)
	if err != nil {
		t.Fatal(err)
	}
	tx := &scriptedAuditTx{rows: []pgx.Row{
		auditRow("read committed"),
		auditRowError(pgx.ErrNoRows),
		auditRow(slotRowValues(slot)...),
		auditRowError(pgx.ErrNoRows),
		auditRow(eventRowValues(input, slot, digest)...),
	}}
	got, err := appendEvent(context.Background(), tx, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.disposition != dispositionCreated || got.tenantSequence != slot.tenantSequence ||
		!bytes.Equal(got.eventHash, digest) || tx.queryCalls != 5 || tx.execCalls != 1 {
		t.Fatalf("append = %#v, query/exec calls = %d/%d", got, tx.queryCalls, tx.execCalls)
	}
}

func TestAppendRechecksIdempotencyAfterTenantHeadLock(t *testing.T) {
	t.Parallel()

	input := testCandidate()
	existingSlot := testSlot()
	_, existingHash, err := hashEvent(input, existingSlot)
	if err != nil {
		t.Fatal(err)
	}
	nextSlot := existingSlot
	nextSlot.tenantSequence = 2
	previousID := input.auditEventID
	nextSlot.previousAuditEventID = &previousID
	nextSlot.previousEventHash = bytes.Clone(existingHash)
	tx := &scriptedAuditTx{rows: []pgx.Row{
		auditRow("read committed"),
		auditRowError(pgx.ErrNoRows),
		auditRow(slotRowValues(nextSlot)...),
		auditRow(append(eventRowValues(input, existingSlot, existingHash), true)...),
	}}
	got, err := appendEvent(context.Background(), tx, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.disposition != dispositionAlreadyPresent || got.tenantSequence != 1 ||
		tx.queryCalls != 4 || tx.execCalls != 1 {
		t.Fatalf("locked retry = %#v, query/exec calls = %d/%d", got, tx.queryCalls, tx.execCalls)
	}
}

func TestAppendExactObservationChecksStoredHash(t *testing.T) {
	t.Parallel()

	input := testCandidate()
	slot := testSlot()
	_, digest, err := hashEvent(input, slot)
	if err != nil {
		t.Fatal(err)
	}
	for name, storedHash := range map[string][]byte{
		"valid":   digest,
		"corrupt": bytes.Repeat([]byte{9}, hashBytes),
	} {
		name, storedHash := name, storedHash
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tx := &scriptedAuditTx{rows: []pgx.Row{
				auditRow("read committed"),
				auditRow(append(eventRowValues(input, slot, storedHash), true)...),
			}}
			got, err := appendEvent(context.Background(), tx, input)
			if name == "valid" {
				if err != nil || got.disposition != dispositionAlreadyPresent || tx.execCalls != 0 {
					t.Fatalf("exact observation = (%#v, %v)", got, err)
				}
				return
			}
			if !hasErrorCode(err, errorPersistence) {
				t.Fatalf("corrupt observation error = %v, want %q", err, errorPersistence)
			}
		})
	}
}

func TestAppendFailsClosedForConflictAndIsolation(t *testing.T) {
	t.Parallel()

	input := testCandidate()
	slot := testSlot()
	_, digest, err := hashEvent(input, slot)
	if err != nil {
		t.Fatal(err)
	}
	conflict := &scriptedAuditTx{rows: []pgx.Row{
		auditRow("read committed"),
		auditRow(append(eventRowValues(input, slot, digest), false)...),
	}}
	if _, err := appendEvent(context.Background(), conflict, input); !hasErrorCode(err, errorIdempotencyConflict) {
		t.Fatalf("conflict error = %v, want %q", err, errorIdempotencyConflict)
	}
	unsupported := &scriptedAuditTx{rows: []pgx.Row{auditRow("repeatable read")}}
	if _, err := appendEvent(context.Background(), unsupported, input); !hasErrorCode(err, errorPersistence) {
		t.Fatalf("isolation error = %v, want %q", err, errorPersistence)
	}
	if unsupported.queryCalls != 1 || unsupported.execCalls != 0 {
		t.Fatal("unsupported isolation reached the Audit storage seam")
	}
}

func TestAppendClassifiesEveryStorageStageFailure(t *testing.T) {
	t.Parallel()

	input := testCandidate()
	slot := testSlot()
	stages := map[string]*scriptedAuditTx{
		"initial observation": {rows: []pgx.Row{
			auditRow("read committed"), auditRowError(errors.New("observe failed")),
		}},
		"ensure head": {rows: []pgx.Row{
			auditRow("read committed"), auditRowError(pgx.ErrNoRows),
		}, execErrors: []error{errors.New("ensure failed")}},
		"lock head": {rows: []pgx.Row{
			auditRow("read committed"), auditRowError(pgx.ErrNoRows), auditRowError(errors.New("lock failed")),
		}},
		"locked observation": {rows: []pgx.Row{
			auditRow("read committed"), auditRowError(pgx.ErrNoRows), auditRow(slotRowValues(slot)...),
			auditRowError(errors.New("locked observe failed")),
		}},
		"finalize": {rows: []pgx.Row{
			auditRow("read committed"), auditRowError(pgx.ErrNoRows), auditRow(slotRowValues(slot)...),
			auditRowError(pgx.ErrNoRows), auditRowError(errors.New("finalize failed")),
		}},
	}
	for name, tx := range stages {
		name, tx := name, tx
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := appendEvent(context.Background(), tx, input); !hasErrorCode(err, errorPersistence) {
				t.Fatalf("stage error = %v, want %q", err, errorPersistence)
			}
		})
	}
}

type scriptedAuditTx struct {
	pgx.Tx
	rows       []pgx.Row
	execErrors []error
	queryCalls int
	execCalls  int
}

func (tx *scriptedAuditTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if tx.queryCalls >= len(tx.rows) {
		tx.queryCalls++
		return auditRowError(errors.New("unexpected Audit query"))
	}
	row := tx.rows[tx.queryCalls]
	tx.queryCalls++
	return row
}

func (tx *scriptedAuditTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	index := tx.execCalls
	tx.execCalls++
	if index < len(tx.execErrors) && tx.execErrors[index] != nil {
		return pgconn.CommandTag{}, tx.execErrors[index]
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

type scriptedAuditRow struct {
	values []any
	err    error
}

func auditRow(values ...any) pgx.Row { return scriptedAuditRow{values: values} }

func auditRowError(err error) pgx.Row { return scriptedAuditRow{err: err} }

func (row scriptedAuditRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(destinations) != len(row.values) {
		return fmt.Errorf("scan destinations = %d, values = %d", len(destinations), len(row.values))
	}
	for index, destination := range destinations {
		target := reflect.ValueOf(destination)
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return fmt.Errorf("scan destination %d is not a pointer", index)
		}
		value := row.values[index]
		if value == nil {
			target.Elem().SetZero()
			continue
		}
		source := reflect.ValueOf(value)
		if !source.Type().AssignableTo(target.Elem().Type()) {
			return fmt.Errorf("scan value %T is not assignable to %T", value, destination)
		}
		target.Elem().Set(source)
	}
	return nil
}

func slotRowValues(value appendSlot) []any {
	return []any{
		value.tenantID,
		value.tenantSequence,
		pgtype.Timestamptz{Time: value.recordedAt, Valid: true},
		value.transactionID,
		cloneString(value.previousAuditEventID),
		bytes.Clone(value.previousEventHash),
	}
}

func eventRowValues(input candidate, slot appendSlot, digest []byte) []any {
	return []any{
		input.tenantID,
		input.auditEventID,
		int16(1),
		slot.tenantSequence,
		pgtype.Timestamptz{Time: slot.recordedAt, Valid: true},
		int16(input.principal.typeID),
		input.principal.id,
		string(input.action),
		string(input.outcome),
		string(input.reason),
		string(input.target.typeID),
		input.target.id,
		cloneInt64(input.target.version),
		input.policyVersion,
		input.requestID,
		cloneString(input.approvalID),
		cloneString(input.recoveryCaseID),
		bytes.Clone(input.evidenceDigest),
		bytes.Clone(slot.previousEventHash),
		bytes.Clone(digest),
	}
}
