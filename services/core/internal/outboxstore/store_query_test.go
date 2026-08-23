package outboxstore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestInsertUsesAtomicCreateResult(t *testing.T) {
	t.Parallel()

	enqueuedAt := time.Date(2026, 8, 24, 7, 8, 9, 0, time.UTC)
	tx := &scriptedTx{rows: []pgx.Row{
		scriptedRow{values: []any{"read committed"}},
		scriptedRow{values: []any{"event-1", int64(17), pgtype.Timestamptz{Time: enqueuedAt, Valid: true}}},
	}}
	got, err := insert(context.Background(), tx, testCandidate())
	if err != nil {
		t.Fatal(err)
	}
	if got.eventID != "event-1" || got.outboxEntryID != 17 || !got.enqueuedAt.Equal(enqueuedAt) || got.disposition != dispositionCreated {
		t.Fatalf("insert result = %#v, want created Event/Entry facts", got)
	}
	if tx.calls != 2 {
		t.Fatalf("query calls = %d, want isolation + atomic insert", tx.calls)
	}
}

func TestInsertObservesExactRetryWithoutCurrentPolicyDependency(t *testing.T) {
	t.Parallel()

	enqueuedAt := time.Date(2026, 8, 24, 7, 8, 9, 0, time.UTC)
	tx := &scriptedTx{rows: []pgx.Row{
		scriptedRow{values: []any{"read committed"}},
		scriptedRow{err: pgx.ErrNoRows},
		scriptedRow{values: []any{"event-1", int64(17), pgtype.Timestamptz{Time: enqueuedAt, Valid: true}, true}},
	}}
	got, err := insert(context.Background(), tx, testCandidate())
	if err != nil {
		t.Fatal(err)
	}
	if got.disposition != dispositionAlreadyPresent || got.outboxEntryID != 17 {
		t.Fatalf("exact retry result = %#v, want existing Entry", got)
	}
	if gotArgs := len(tx.arguments[2]); gotArgs != 8 {
		t.Fatalf("observation arguments = %d, want only immutable Event facts and identity", gotArgs)
	}
}

func TestInsertFailsClosedForConflictingObservation(t *testing.T) {
	t.Parallel()

	tx := &scriptedTx{rows: []pgx.Row{
		scriptedRow{values: []any{"read committed"}},
		scriptedRow{err: pgx.ErrNoRows},
		scriptedRow{values: []any{"event-1", int64(17), pgtype.Timestamptz{Time: time.Now(), Valid: true}, false}},
	}}
	_, err := insert(context.Background(), tx, testCandidate())
	if !hasStoreErrorCode(err, errorIdempotencyConflict) {
		t.Fatalf("insert error = %v, want %q", err, errorIdempotencyConflict)
	}
}

func TestInsertRejectsUnsupportedIsolationBeforeWrite(t *testing.T) {
	t.Parallel()

	tx := &scriptedTx{rows: []pgx.Row{scriptedRow{values: []any{"repeatable read"}}}}
	_, err := insert(context.Background(), tx, testCandidate())
	if !hasStoreErrorCode(err, errorPersistence) {
		t.Fatalf("insert error = %v, want %q", err, errorPersistence)
	}
	if tx.calls != 1 {
		t.Fatalf("query calls = %d, want isolation check only", tx.calls)
	}
}

type scriptedTx struct {
	pgx.Tx
	rows      []pgx.Row
	arguments [][]any
	calls     int
}

func (tx *scriptedTx) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	tx.arguments = append(tx.arguments, append([]any(nil), args...))
	if tx.calls >= len(tx.rows) {
		tx.calls++
		return scriptedRow{err: errors.New("unexpected query")}
	}
	row := tx.rows[tx.calls]
	tx.calls++
	return row
}

type scriptedRow struct {
	values []any
	err    error
}

func (row scriptedRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(destinations) != len(row.values) {
		return fmt.Errorf("scan destinations = %d, values = %d", len(destinations), len(row.values))
	}
	for index, value := range row.values {
		switch destination := destinations[index].(type) {
		case *string:
			*destination = value.(string)
		case *int64:
			*destination = value.(int64)
		case *bool:
			*destination = value.(bool)
		case *pgtype.Timestamptz:
			*destination = value.(pgtype.Timestamptz)
		default:
			return fmt.Errorf("unsupported scan destination %T", destination)
		}
	}
	return nil
}
