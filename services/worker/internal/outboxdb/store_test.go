package outboxdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/monkeylabx/threadline/services/worker/internal/dbgen"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

var testStoreBinding = Binding{
	LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
	Stream:             "DOMAIN_EVENTS",
}

func TestNewStoreRejectsNilTypedNilAndInvalidBinding(t *testing.T) {
	t.Parallel()

	if _, err := NewStore(nil, testStoreBinding); storeErrorCode(err) != StoreErrorInvalidInput {
		t.Fatalf("nil database error = %v, want invalid-input", err)
	}
	var typedNil *typedNilDB
	if _, err := NewStore(typedNil, testStoreBinding); storeErrorCode(err) != StoreErrorInvalidInput {
		t.Fatalf("typed-nil database error = %v, want invalid-input", err)
	}
	for _, stream := range []string{"DOMAIN.*", "DOMAIN EVENTS"} {
		invalid := testStoreBinding
		invalid.Stream = stream
		if _, err := NewStore(&typedNilDB{}, invalid); storeErrorCode(err) != StoreErrorInvalidInput {
			t.Fatalf("invalid binding %q error = %v, want invalid-input", stream, err)
		}
	}
}

func TestProductionStoreClaimMapsClonedRedactedFactsAndLease(t *testing.T) {
	t.Parallel()

	raw := testRawClaimToken()
	queries := &scriptedQueries{claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(raw)}}
	store := mustProductionStore(t, queries, testStoreBinding)
	claims, err := store.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker-1", BatchSize: 64})
	if err != nil || len(claims) != 1 {
		t.Fatalf("claims = %d, error = %v, want one", len(claims), err)
	}

	facts := claims[0].PublishFacts()
	row := queries.claimRows[0]
	if facts.TenantID != row.TenantID || facts.EventID != row.EventID ||
		facts.OutboxEntryID != row.OutboxEntryID || facts.LogicalDestination != row.Destination ||
		facts.BrokerMessageID != row.BrokerMessageID || facts.EventType != row.EventType ||
		facts.SchemaVersion != row.SchemaVersion || facts.AggregateKind != row.AggregateKind ||
		facts.AggregateID != row.AggregateID || string(facts.Payload) != string(row.Payload) ||
		!facts.OccurredAt.Equal(row.OccurredAt.Time.UTC()) || !facts.EnqueuedAt.Equal(row.EnqueuedAt.Time.UTC()) {
		t.Fatalf("publish facts did not preserve exact row mapping: %#v", facts)
	}
	lease := claims[0].Lease()
	if !lease.ClaimedAt.Equal(row.ClaimedAt.Time.UTC()) ||
		!lease.ExpiresAt.Equal(row.LeaseExpiresAt.Time.UTC()) ||
		!lease.AbsoluteLeaseExpiresAt.Equal(row.AbsoluteLeaseExpiresAt.Time.UTC()) {
		t.Fatalf("lease = %#v, want exact database times", lease)
	}

	queries.claimRows[0].Payload[0] = 'X'
	facts.Payload[0] = 'Y'
	if got := string(claims[0].PublishFacts().Payload); got != "secret-payload" {
		t.Fatalf("retained payload = %q, want independent clone", got)
	}
	for _, value := range raw {
		if value != 0 {
			t.Fatal("database raw claim token was not cleared")
		}
	}
	assertStoreValuesRedacted(t, store, claims[0], claims[0].PublishFacts(), claims[0].Lease())
}

func TestProductionStoreRejectsZeroForeignSyntheticAndSubstitutedClaimsBeforeDatabase(t *testing.T) {
	t.Parallel()

	firstQueries := &scriptedQueries{claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())}}
	first := mustProductionStore(t, firstQueries, testStoreBinding)
	claim := mustProductionClaim(t, first)

	secondQueries := &scriptedQueries{}
	second := mustProductionStore(t, secondQueries, testStoreBinding)
	facts := claim.PublishFacts()
	lease := claim.Lease()
	synthetic := newOpaqueClaim(
		&storeIdentity{kind: scriptedStoreIdentity},
		testFence(),
		facts,
		lease,
	)
	substitutedFacts := facts.Clone()
	substitutedFacts.EventID = "event-substituted"
	substituted := newOpaqueClaim(second.identity, testFence(), substitutedFacts, lease)

	for name, candidate := range map[string]Claim{
		"zero":        {},
		"foreign":     claim,
		"synthetic":   synthetic,
		"substituted": substituted,
	} {
		if _, err := second.Renew(context.Background(), candidate); storeErrorCode(err) != StoreErrorClaimDenied {
			t.Errorf("%s claim error = %v, want claim-denied", name, err)
		}
	}
	if secondQueries.calls != 0 {
		t.Fatalf("rejected claims made %d database calls, want zero", secondQueries.calls)
	}
}

func TestProductionStoreRenewsExactAuthorityAndUpdatesSharedLease(t *testing.T) {
	t.Parallel()

	queries := &scriptedQueries{claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())}}
	store := mustProductionStore(t, queries, testStoreBinding)
	claim := mustProductionClaim(t, store)
	original := claim.Lease()
	next := original.ExpiresAt.Add(30 * time.Second)
	queries.renewRow.ResultCode = "renewed"
	queries.renewRow.LeaseExpiresAt = timestamp(next)

	renewal, err := store.Renew(context.Background(), claim)
	if err != nil || !renewal.LeaseExpiresAt.Equal(next) {
		t.Fatalf("renewal = %#v, error = %v", renewal, err)
	}
	if got := claim.Lease(); !got.ExpiresAt.Equal(next) || !got.AbsoluteLeaseExpiresAt.Equal(original.AbsoluteLeaseExpiresAt) {
		t.Fatalf("updated lease = %#v, want new current and unchanged absolute deadline", got)
	}
	if queries.renewParams.TenantID != testFence().tenantID ||
		queries.renewParams.EventID != testFence().eventID ||
		queries.renewParams.OutboxEntryID != testFence().outboxEntryID ||
		queries.renewParams.DeliveryAttemptID != testFence().deliveryAttemptID ||
		queries.renewParams.ReplayGeneration != testFence().replayGeneration ||
		queries.renewParams.ClaimOwnerID != testFence().claimOwnerID {
		t.Fatalf("renew did not preserve exact fence: %#v", queries.renewParams)
	}
}

func TestProductionStoreOperationGateLetsWaitingCallCancelWithoutDatabaseAccess(t *testing.T) {
	t.Parallel()

	base := &scriptedQueries{claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())}}
	queries := &blockingRenewQueries{
		scriptedQueries: base,
		entered:         make(chan struct{}),
		release:         make(chan struct{}),
	}
	store := mustProductionStore(t, queries, testStoreBinding)
	claim := mustProductionClaim(t, store)
	next := claim.Lease().ExpiresAt.Add(30 * time.Second)
	base.renewRow.ResultCode = "renewed"
	base.renewRow.LeaseExpiresAt = timestamp(next)

	firstDone := make(chan error, 1)
	go func() {
		_, err := store.Renew(context.Background(), claim)
		firstDone <- err
	}()
	<-queries.entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Renew(ctx, claim); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting Renew error = %v, want context.Canceled", err)
	}
	if got := queries.renewCalls.Load(); got != 1 {
		t.Fatalf("database Renew calls = %d, want only the blocked first call", got)
	}
	close(queries.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Renew failed: %v", err)
	}
}

func TestProductionStorePreservesC2AcknowledgementAndRejectsForeignEvidence(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		dbResult   string
		wantResult Acknowledgement
		sequence   uint64
		duplicate  bool
	}{
		{name: "delivered sequence one", dbResult: "delivered", wantResult: AcknowledgementDelivered, sequence: 1},
		{name: "already delivered max duplicate", dbResult: "already-delivered", wantResult: AcknowledgementAlreadyDelivered, sequence: math.MaxUint64, duplicate: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			queries := &scriptedQueries{
				claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())},
				ackResult: test.dbResult,
			}
			store := mustProductionStore(t, queries, testStoreBinding)
			claim := mustProductionClaim(t, store)
			facts := claim.PublishFacts()
			ack := outboxpublish.Acknowledgement{
				Stream: testStoreBinding.Stream, Sequence: test.sequence,
				Duplicate: test.duplicate, MessageID: facts.BrokerMessageID,
			}
			got, err := store.Acknowledge(context.Background(), claim, ack)
			if err != nil || got != test.wantResult {
				t.Fatalf("acknowledgement = %v, error = %v", got, err)
			}
			if queries.ackParams.BrokerStream != ack.Stream ||
				queries.ackParams.BrokerDuplicate != ack.Duplicate ||
				queries.ackParams.BrokerMessageID != ack.MessageID ||
				queries.ackParams.BrokerSequence.Int.Cmp(new(big.Int).SetUint64(ack.Sequence)) != 0 {
				t.Fatalf("C2 Ack changed before C1: %#v", queries.ackParams)
			}
		})
	}

	queries := &scriptedQueries{claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())}}
	store := mustProductionStore(t, queries, testStoreBinding)
	claim := mustProductionClaim(t, store)
	facts := claim.PublishFacts()
	baseCalls := queries.calls
	for name, ack := range map[string]outboxpublish.Acknowledgement{
		"foreign Stream": {Stream: "OTHER", Sequence: 1, MessageID: facts.BrokerMessageID},
		"wrong message":  {Stream: testStoreBinding.Stream, Sequence: 1, MessageID: strings.Repeat("b", 64)},
		"zero sequence":  {Stream: testStoreBinding.Stream, MessageID: facts.BrokerMessageID},
	} {
		if _, err := store.Acknowledge(context.Background(), claim, ack); storeErrorCode(err) != StoreErrorInvalidInput {
			t.Errorf("%s error = %v, want invalid-input", name, err)
		}
	}
	if queries.calls != baseCalls {
		t.Fatalf("foreign/malformed Acks made %d database calls, want zero", queries.calls-baseCalls)
	}
}

func TestProductionStoreAllowsOnlyFrozenFailureCodes(t *testing.T) {
	t.Parallel()

	for _, code := range []FailureCode{
		FailureTransportUnavailable,
		FailurePublishOutcomeUnknown,
		FailureEventRetryable,
		FailureEventPermanent,
	} {
		queries := &scriptedQueries{
			claimRows:  []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())},
			failureRow: dbgenFailureRow("retry-scheduled", time.Now().UTC().Add(time.Minute)),
		}
		store := mustProductionStore(t, queries, testStoreBinding)
		claim := mustProductionClaim(t, store)
		if _, err := store.RecordFailure(context.Background(), claim, code); err != nil {
			t.Fatalf("allowed code %q failed: %v", string(code), err)
		}
		if queries.failParams.FailureCode != string(code) {
			t.Fatalf("failure code = %q, want %q", queries.failParams.FailureCode, code)
		}
	}

	queries := &scriptedQueries{claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())}}
	store := mustProductionStore(t, queries, testStoreBinding)
	claim := mustProductionClaim(t, store)
	baseCalls := queries.calls
	for _, code := range []FailureCode{"future-code", FailureCode(outboxpublish.FailureInvalidInput)} {
		if _, err := store.RecordFailure(context.Background(), claim, code); storeErrorCode(err) != StoreErrorInvalidInput {
			t.Fatalf("invalid failure %q error = %v, want invalid-input", string(code), err)
		}
	}
	if queries.calls != baseCalls {
		t.Fatalf("invalid failure codes made %d database calls, want zero", queries.calls-baseCalls)
	}
}

func TestProductionStoreMapsFailureResultsErrorsAndCancellation(t *testing.T) {
	t.Parallel()

	next := time.Now().UTC().Add(time.Minute)
	queries := &scriptedQueries{
		claimRows:  []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())},
		failureRow: dbgenFailureRow("retry-scheduled", next),
	}
	store := mustProductionStore(t, queries, testStoreBinding)
	claim := mustProductionClaim(t, store)
	result, err := store.RecordFailure(context.Background(), claim, FailureTransportUnavailable)
	if err != nil || result.Disposition != FailureRetryScheduled || !result.NextAttemptAt.Equal(next) {
		t.Fatalf("retry result = %#v, error = %v", result, err)
	}
	queries.failureRow = dbgenFailureRow("parked", time.Time{})
	result, err = store.RecordFailure(context.Background(), claim, FailureEventPermanent)
	if err != nil || result.Disposition != FailureParked || !result.NextAttemptAt.IsZero() {
		t.Fatalf("park result = %#v, error = %v", result, err)
	}

	queries.renewErr = errors.New("raw database credential secret")
	if _, err := store.Renew(context.Background(), claim); storeErrorCode(err) != StoreErrorPersistence ||
		strings.Contains(err.Error(), "credential") {
		t.Fatalf("persistence error leaked detail: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	baseCalls := queries.calls
	if _, err := store.Renew(ctx, claim); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Renew error = %v, want context.Canceled", err)
	}
	if queries.calls != baseCalls {
		t.Fatalf("pre-canceled Renew made %d database calls, want zero", queries.calls-baseCalls)
	}
}

func TestProductionStoreMapsDatabaseClaimDeniedForEveryMutation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		configure func(*scriptedQueries)
		mutate    func(Store, Claim) error
	}{
		{
			name: "renew",
			configure: func(queries *scriptedQueries) {
				queries.renewRow.ResultCode = "claim-denied"
			},
			mutate: func(store Store, claim Claim) error {
				_, err := store.Renew(context.Background(), claim)
				return err
			},
		},
		{
			name: "acknowledge",
			configure: func(queries *scriptedQueries) {
				queries.ackResult = "claim-denied"
			},
			mutate: func(store Store, claim Claim) error {
				_, err := store.Acknowledge(context.Background(), claim, outboxpublish.Acknowledgement{
					Stream: testStoreBinding.Stream, Sequence: 1,
					MessageID: claim.PublishFacts().BrokerMessageID,
				})
				return err
			},
		},
		{
			name: "record failure",
			configure: func(queries *scriptedQueries) {
				queries.failureRow.ResultCode = "claim-denied"
			},
			mutate: func(store Store, claim Claim) error {
				_, err := store.RecordFailure(context.Background(), claim, FailureEventRetryable)
				return err
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			queries := &scriptedQueries{
				claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(testRawClaimToken())},
			}
			test.configure(queries)
			store := mustProductionStore(t, queries, testStoreBinding)
			claim := mustProductionClaim(t, store)
			if err := test.mutate(store, claim); storeErrorCode(err) != StoreErrorClaimDenied {
				t.Fatalf("database claim-denied error = %v, want stable claim-denied", err)
			}
			if queries.calls != 2 {
				t.Fatalf("database calls = %d, want Claim plus one mutation", queries.calls)
			}
		})
	}
}

func mustProductionStore(t *testing.T, queries outboxQueries, binding Binding) *productionStore {
	t.Helper()
	created, err := newProductionStore(&adapter{queries: queries}, binding)
	if err != nil {
		t.Fatal(err)
	}
	store, ok := created.(*productionStore)
	if !ok {
		t.Fatalf("production Store type = %T", created)
	}
	return store
}

func mustProductionClaim(t *testing.T, store Store) Claim {
	t.Helper()
	claims, err := store.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker-1", BatchSize: 1})
	if err != nil || len(claims) != 1 {
		t.Fatalf("claims = %d, error = %v, want one", len(claims), err)
	}
	return claims[0]
}

func testRawClaimToken() []byte {
	raw := make([]byte, claimTokenRawBytes)
	for index := range raw {
		raw[index] = byte(index)
	}
	return raw
}

func dbgenFailureRow(result string, next time.Time) dbgen.RecordTransactionalOutboxPublishFailureRow {
	row := dbgen.RecordTransactionalOutboxPublishFailureRow{ResultCode: result}
	if !next.IsZero() {
		row.NextAttemptAt = pgtype.Timestamptz{Time: next, Valid: true}
	}
	return row
}

func storeErrorCode(err error) StoreErrorCode {
	code, _ := StoreErrorCodeOf(err)
	return code
}

func assertStoreValuesRedacted(t *testing.T, values ...any) {
	t.Helper()
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(encoded)} {
			for _, secret := range []string{"tenant-1", "event-1", "secret-payload", goldenClaimTokenWire, testStoreBinding.Stream} {
				if strings.Contains(rendered, secret) {
					t.Fatalf("rendering exposed %q: %q", secret, rendered)
				}
			}
		}
	}
}

type typedNilDB struct{}

func (*typedNilDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (*typedNilDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (*typedNilDB) QueryRow(context.Context, string, ...interface{}) pgx.Row { return nil }

type blockingRenewQueries struct {
	*scriptedQueries
	entered    chan struct{}
	release    chan struct{}
	renewCalls atomic.Int32
}

func (queries *blockingRenewQueries) RenewTransactionalOutboxClaim(
	ctx context.Context,
	params dbgen.RenewTransactionalOutboxClaimParams,
) (dbgen.RenewTransactionalOutboxClaimRow, error) {
	if queries.renewCalls.Add(1) == 1 {
		close(queries.entered)
	}
	select {
	case <-ctx.Done():
		return dbgen.RenewTransactionalOutboxClaimRow{}, ctx.Err()
	case <-queries.release:
		return queries.scriptedQueries.RenewTransactionalOutboxClaim(ctx, params)
	}
}
