// Package outboxstore appends an immutable Domain Event and its initial
// transactional Outbox Entry inside a caller-owned Core transaction.
package outboxstore

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
)

const (
	payloadHardBytes  = 262_144
	policyDigestBytes = 32
	policyV1ID        = "threadline.outbox.policy/v1"
)

// errorCode classifies a stable Outbox-store failure without exposing Event
// facts, payload bytes, SQL text, or policy material.
type errorCode string

const (
	errorInvalidInput        errorCode = "invalid-input"
	errorIdempotencyConflict errorCode = "idempotency-conflict"
	errorPersistence         errorCode = "persistence-failure"
)

// storeFailure is a stable, secret-safe Outbox-store error.
type storeFailure struct{ code errorCode }

func (e *storeFailure) Error() string { return "transactional outbox store: " + string(e.category()) }

func (e *storeFailure) category() errorCode {
	if e == nil {
		return ""
	}
	return e.code
}

// eventDescriptor remains package-private until a producer Contract task
// registers exact Event Type/version/aggregate/destination combinations.
type eventDescriptor struct {
	eventType     string
	schemaVersion int32
	aggregateKind string
}

// policySnapshot is the trusted v1 policy copied into a new Entry generation.
// Values that are not persisted are nevertheless bound by snapshotDigest.
type policySnapshot struct {
	id                 string
	snapshotDigest     []byte
	leaseMS            int32
	absoluteLifetimeMS int32
	eventRetryCeiling  int32
	transportBaseMS    int32
	transportCapMS     int32
	unknownBaseMS      int32
	unknownCapMS       int32
	eventBaseMS        int32
	eventCapMS         int32
	retentionDays      int32
}

type candidate struct {
	tenantID    string
	eventID     string
	descriptor  eventDescriptor
	aggregateID string
	payload     []byte
	occurredAt  time.Time
	policy      policySnapshot
}

type disposition uint8

const (
	dispositionCreated disposition = iota + 1
	dispositionAlreadyPresent
)

type result struct {
	eventID       string
	outboxEntryID int64
	enqueuedAt    time.Time
	disposition   disposition
}

// insert appends a candidate inside tx. The caller alone owns transaction
// lifetime and must roll back after every returned error; insert never begins,
// commits, or rolls back. It remains package-private until a registered
// producer descriptor makes it reachable without accepting request-selected
// routing facts.
func insert(ctx context.Context, tx pgx.Tx, input candidate) (result, error) {
	if err := ctx.Err(); err != nil {
		return result{}, err
	}
	if tx == nil || !validCandidate(input) {
		return result{}, storeError(errorInvalidInput)
	}

	queries := dbgen.New(tx)
	isolation, err := queries.GetOutboxTransactionIsolation(ctx)
	if err != nil || isolation != "read committed" {
		return result{}, persistenceError(ctx)
	}

	inserted, err := queries.TryInsertDomainEventAndInitialEntry(ctx, insertParams(input))
	if err == nil {
		if inserted.EventID != input.eventID || inserted.OutboxEntryID <= 0 || !inserted.EnqueuedAt.Valid {
			return result{}, storeError(errorPersistence)
		}
		return result{
			eventID:       inserted.EventID,
			outboxEntryID: inserted.OutboxEntryID,
			enqueuedAt:    inserted.EnqueuedAt.Time.UTC(),
			disposition:   dispositionCreated,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result{}, persistenceError(ctx)
	}

	observed, err := queries.ObserveExactDomainEventAndInitialDestination(ctx, observationParams(input))
	if err != nil {
		return result{}, persistenceError(ctx)
	}
	if !observed.ExactMatch {
		return result{}, storeError(errorIdempotencyConflict)
	}
	if observed.EventID != input.eventID || observed.OutboxEntryID <= 0 || !observed.EnqueuedAt.Valid {
		return result{}, storeError(errorPersistence)
	}
	return result{
		eventID:       observed.EventID,
		outboxEntryID: observed.OutboxEntryID,
		enqueuedAt:    observed.EnqueuedAt.Time.UTC(),
		disposition:   dispositionAlreadyPresent,
	}, nil
}

func insertParams(input candidate) dbgen.TryInsertDomainEventAndInitialEntryParams {
	return dbgen.TryInsertDomainEventAndInitialEntryParams{
		TenantID:                    input.tenantID,
		EventID:                     input.eventID,
		EventType:                   input.descriptor.eventType,
		SchemaVersion:               input.descriptor.schemaVersion,
		AggregateKind:               input.descriptor.aggregateKind,
		AggregateID:                 input.aggregateID,
		Payload:                     databasePayload(input.payload),
		OccurredAt:                  pgtype.Timestamptz{Time: input.occurredAt, Valid: true},
		PolicyID:                    input.policy.id,
		PolicySnapshotDigest:        input.policy.snapshotDigest,
		EffectiveLeaseMs:            input.policy.leaseMS,
		EffectiveAbsoluteLifetimeMs: input.policy.absoluteLifetimeMS,
		EffectiveEventRetryCeiling:  input.policy.eventRetryCeiling,
		EffectiveTransportBaseMs:    input.policy.transportBaseMS,
		EffectiveTransportCapMs:     input.policy.transportCapMS,
		EffectiveUnknownBaseMs:      input.policy.unknownBaseMS,
		EffectiveUnknownCapMs:       input.policy.unknownCapMS,
		EffectiveEventBaseMs:        input.policy.eventBaseMS,
		EffectiveEventCapMs:         input.policy.eventCapMS,
		EffectiveRetentionDays:      input.policy.retentionDays,
	}
}

func observationParams(input candidate) dbgen.ObserveExactDomainEventAndInitialDestinationParams {
	return dbgen.ObserveExactDomainEventAndInitialDestinationParams{
		TenantID:      input.tenantID,
		EventID:       input.eventID,
		EventType:     input.descriptor.eventType,
		SchemaVersion: input.descriptor.schemaVersion,
		AggregateKind: input.descriptor.aggregateKind,
		AggregateID:   input.aggregateID,
		Payload:       databasePayload(input.payload),
		OccurredAt:    pgtype.Timestamptz{Time: input.occurredAt, Valid: true},
	}
}

func validCandidate(input candidate) bool {
	return validIdentifier(input.tenantID) &&
		validIdentifier(input.eventID) &&
		validEventType(input.descriptor.eventType) &&
		input.descriptor.schemaVersion > 0 &&
		validIdentifier(input.descriptor.aggregateKind) &&
		validIdentifier(input.aggregateID) &&
		len(input.payload) <= payloadHardBytes &&
		!input.occurredAt.IsZero() &&
		validPolicy(input.policy)
}

func validEventType(value string) bool {
	dot := strings.IndexByte(value, '.')
	return validIdentifier(value) && value == strings.ToLower(value) && dot > 0 && dot < len(value)-1
}

func databasePayload(payload []byte) []byte {
	if payload == nil {
		return []byte{}
	}
	return payload
}

func validPolicy(policy policySnapshot) bool {
	return policy.id == policyV1ID &&
		len(policy.snapshotDigest) == policyDigestBytes &&
		inRange(policy.leaseMS, 5_000, 120_000) &&
		inRange(policy.absoluteLifetimeMS, 30_000, 900_000) &&
		int64(policy.absoluteLifetimeMS) >= 2*int64(policy.leaseMS) &&
		inRange(policy.eventRetryCeiling, 1, 20) &&
		validBackoff(policy.transportBaseMS, policy.transportCapMS, 100, 10_000, 1_000, 300_000) &&
		validBackoff(policy.unknownBaseMS, policy.unknownCapMS, 500, 30_000, 5_000, 900_000) &&
		validBackoff(policy.eventBaseMS, policy.eventCapMS, 500, 30_000, 5_000, 900_000) &&
		inRange(policy.retentionDays, 30, 365)
}

func validBackoff(base, cap, minimumBase, maximumBase, minimumCap, maximumCap int32) bool {
	return inRange(base, minimumBase, maximumBase) &&
		inRange(cap, minimumCap, maximumCap) &&
		cap >= base
}

func inRange(value, minimum, maximum int32) bool {
	return value >= minimum && value <= maximum
}

func validIdentifier(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func storeError(code errorCode) error { return &storeFailure{code: code} }

func persistenceError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return storeError(errorPersistence)
}
