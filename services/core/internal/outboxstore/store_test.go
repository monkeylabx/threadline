package outboxstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestInsertRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	valid := testCandidate()
	cases := map[string]candidate{
		"non-canonical tenant ID":         withCandidate(valid, func(value *candidate) { value.tenantID = " tenant-1" }),
		"non-canonical event ID":          withCandidate(valid, func(value *candidate) { value.eventID = "event-1\n" }),
		"unknown descriptor event type":   withCandidate(valid, func(value *candidate) { value.descriptor.eventType = "" }),
		"non-canonical descriptor type":   withCandidate(valid, func(value *candidate) { value.descriptor.eventType = "Message.Created" }),
		"non-dotted descriptor type":      withCandidate(valid, func(value *candidate) { value.descriptor.eventType = "message" }),
		"unknown descriptor schema":       withCandidate(valid, func(value *candidate) { value.descriptor.schemaVersion = 0 }),
		"unknown descriptor aggregate":    withCandidate(valid, func(value *candidate) { value.descriptor.aggregateKind = "" }),
		"non-canonical aggregate ID":      withCandidate(valid, func(value *candidate) { value.aggregateID = " aggregate-1" }),
		"missing occurrence time":         withCandidate(valid, func(value *candidate) { value.occurredAt = time.Time{} }),
		"payload above schema hard limit": withCandidate(valid, func(value *candidate) { value.payload = make([]byte, payloadHardBytes+1) }),
		"unknown policy ID":               withCandidate(valid, func(value *candidate) { value.policy.id = "future-policy" }),
		"invalid policy digest":           withCandidate(valid, func(value *candidate) { value.policy.snapshotDigest = make([]byte, policyDigestBytes-1) }),
		"invalid lease":                   withCandidate(valid, func(value *candidate) { value.policy.leaseMS = 0 }),
		"invalid absolute lifetime":       withCandidate(valid, func(value *candidate) { value.policy.absoluteLifetimeMS = 0 }),
		"absolute lifetime below twice lease": withCandidate(valid, func(value *candidate) {
			value.policy.absoluteLifetimeMS = value.policy.leaseMS
		}),
		"invalid retry ceiling":  withCandidate(valid, func(value *candidate) { value.policy.eventRetryCeiling = 0 }),
		"invalid transport base": withCandidate(valid, func(value *candidate) { value.policy.transportBaseMS = 0 }),
		"transport cap below base": withCandidate(valid, func(value *candidate) {
			value.policy.transportBaseMS = 10_000
			value.policy.transportCapMS = 1_000
		}),
		"invalid unknown base": withCandidate(valid, func(value *candidate) { value.policy.unknownBaseMS = 0 }),
		"unknown cap below base": withCandidate(valid, func(value *candidate) {
			value.policy.unknownBaseMS = 30_000
			value.policy.unknownCapMS = 5_000
		}),
		"invalid event base": withCandidate(valid, func(value *candidate) { value.policy.eventBaseMS = 0 }),
		"event cap below base": withCandidate(valid, func(value *candidate) {
			value.policy.eventBaseMS = 30_000
			value.policy.eventCapMS = 5_000
		}),
		"invalid retention": withCandidate(valid, func(value *candidate) { value.policy.retentionDays = 29 }),
	}

	for name, input := range cases {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tx := &scriptedTx{}
			_, err := insert(context.Background(), tx, input)
			var storeErr *storeFailure
			if !errors.As(err, &storeErr) || storeErr.category() != errorInvalidInput {
				t.Fatalf("insert error = %v, want %q", err, errorInvalidInput)
			}
			if tx.calls != 0 {
				t.Fatalf("database calls = %d, want none for invalid input", tx.calls)
			}
		})
	}

	_, err := insert(context.Background(), nil, valid)
	if !hasStoreErrorCode(err, errorInvalidInput) {
		t.Fatalf("nil transaction error = %v, want %q", err, errorInvalidInput)
	}
}

func TestInsertPropagatesCanceledContextBeforeValidation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := insert(ctx, nil, candidate{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("insert error = %v, want context.Canceled", err)
	}
}

func TestStoreErrorDoesNotExposeCandidateFacts(t *testing.T) {
	t.Parallel()

	secret := "secret-payload-and-identity"
	input := testCandidate()
	input.tenantID = secret + "\n"
	input.payload = []byte(secret)
	_, err := insert(context.Background(), nil, input)
	if err == nil {
		t.Fatal("insert succeeded, want invalid-input")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposed candidate facts: %q", err)
	}
}

func TestInsertParamsPreserveAllowedEmptyPayloadAsNonNullBytes(t *testing.T) {
	t.Parallel()

	input := testCandidate()
	input.payload = nil
	params := insertParams(input)
	if params.Payload == nil || len(params.Payload) != 0 {
		t.Fatalf("database payload = %#v, want non-null zero bytes", params.Payload)
	}
	observed := observationParams(input)
	if observed.Payload == nil || len(observed.Payload) != 0 {
		t.Fatalf("observation payload = %#v, want non-null zero bytes", observed.Payload)
	}
}

func testCandidate() candidate {
	return candidate{
		tenantID:    "tenant-1",
		eventID:     "event-1",
		descriptor:  eventDescriptor{eventType: "message.created", schemaVersion: 1, aggregateKind: "message"},
		aggregateID: "message-1",
		payload:     []byte(`{"message_id":"message-1"}`),
		occurredAt:  time.Date(2026, 8, 24, 6, 7, 8, 123456000, time.UTC),
		policy: policySnapshot{
			id:                 policyV1ID,
			snapshotDigest:     make([]byte, policyDigestBytes),
			leaseMS:            30_000,
			absoluteLifetimeMS: 300_000,
			eventRetryCeiling:  8,
			transportBaseMS:    1_000,
			transportCapMS:     60_000,
			unknownBaseMS:      5_000,
			unknownCapMS:       300_000,
			eventBaseMS:        5_000,
			eventCapMS:         300_000,
			retentionDays:      90,
		},
	}
}

func withCandidate(input candidate, mutate func(*candidate)) candidate {
	mutate(&input)
	return input
}
