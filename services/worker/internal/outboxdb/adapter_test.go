package outboxdb

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monkeylabx/threadline/services/worker/internal/dbgen"
)

func TestClaimMapsAuthorityAndClearsOneTimeDatabaseToken(t *testing.T) {
	t.Parallel()

	raw := make([]byte, claimTokenRawBytes)
	for index := range raw {
		raw[index] = byte(index)
	}
	queries := &scriptedQueries{claimRows: []dbgen.ClaimTransactionalOutboxBatchRow{testClaimRow(raw)}}
	claimed, err := (&adapter{queries: queries}).claim(context.Background(), claimRequest{
		claimOwnerID: "worker-1",
		batchSize:    64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].fence.token.wire != goldenClaimTokenWire {
		t.Fatalf("claimed = %#v, want one canonical Claim", claimed)
	}
	if claimed[0].fence.claimOwnerID != "worker-1" || claimed[0].fence.outboxEntryID != 17 {
		t.Fatalf("claim mapping = %#v, want exact authority tuple", claimed[0])
	}
	for _, value := range raw {
		if value != 0 {
			t.Fatal("database raw token buffer was not cleared")
		}
	}
	if strings.Contains(fmtAll(claimed[0]), goldenClaimTokenWire) || strings.Contains(fmtAll(claimed[0]), "secret-payload") {
		t.Fatal("claimed event rendering exposed token or payload")
	}
}

func TestRenewUsesExactGoldenCandidateDigest(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, 8, 24, 4, 5, 6, 0, time.UTC)
	queries := &scriptedQueries{renewRow: dbgen.RenewTransactionalOutboxClaimRow{
		ResultCode:     "renewed",
		LeaseExpiresAt: timestamp(expiresAt),
	}}
	got, err := (&adapter{queries: queries}).renew(context.Background(), testFence())
	if err != nil {
		t.Fatal(err)
	}
	if !got.leaseExpiresAt.Equal(expiresAt) {
		t.Fatalf("lease expiry = %v, want %v", got.leaseExpiresAt, expiresAt)
	}
	want, _ := hex.DecodeString("0fced3787dc44e7855171187da0812df307108fb766c97f6082824715b310994")
	if string(queries.renewParams.CandidateDigest) != string(want) {
		t.Fatalf("candidate digest = %x, want Golden", queries.renewParams.CandidateDigest)
	}
}

func TestMalformedTokenIsIndistinguishableAndNeverQueries(t *testing.T) {
	t.Parallel()

	fence := testFence()
	fence.token = claimToken{wire: goldenClaimTokenWire + "="}
	queries := &scriptedQueries{}
	_, err := (&adapter{queries: queries}).renew(context.Background(), fence)
	if !hasOperationCode(err, errorClaimDenied) {
		t.Fatalf("renew error = %v, want claim-denied", err)
	}
	if queries.calls != 0 || strings.Contains(err.Error(), fence.token.wire) {
		t.Fatalf("malformed token queried or leaked: calls=%d error=%q", queries.calls, err)
	}
}

func TestMalformedFenceFactsAreClaimDeniedWithoutQuery(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*claimFence){
		"empty tenant":       func(fence *claimFence) { fence.tenantID = "" },
		"empty event":        func(fence *claimFence) { fence.eventID = "" },
		"invalid entry":      func(fence *claimFence) { fence.outboxEntryID = 0 },
		"invalid attempt":    func(fence *claimFence) { fence.deliveryAttemptID = 0 },
		"invalid generation": func(fence *claimFence) { fence.replayGeneration = -1 },
		"invalid owner":      func(fence *claimFence) { fence.claimOwnerID = " worker-1" },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fence := testFence()
			mutate(&fence)
			queries := &scriptedQueries{}
			_, err := (&adapter{queries: queries}).renew(context.Background(), fence)
			if !hasOperationCode(err, errorClaimDenied) {
				t.Fatalf("renew error = %v, want claim-denied", err)
			}
			if queries.calls != 0 {
				t.Fatalf("database calls = %d, want none", queries.calls)
			}
		})
	}
}

func TestAcknowledgePreservesUint64SequenceAndTypedResults(t *testing.T) {
	t.Parallel()

	queries := &scriptedQueries{ackResult: "already-delivered"}
	got, err := (&adapter{queries: queries}).acknowledge(context.Background(), acknowledgementRequest{
		fence: testFence(),
		pubAck: pubAck{
			stream:    "DOMAIN_EVENTS",
			sequence:  math.MaxUint64,
			duplicate: true,
			messageID: strings.Repeat("a", 64),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != acknowledgementAlreadyDelivered {
		t.Fatalf("acknowledgement = %v, want already-delivered", got)
	}
	if !queries.ackParams.BrokerSequence.Valid || queries.ackParams.BrokerSequence.Exp != 0 ||
		queries.ackParams.BrokerSequence.Int.Cmp(new(big.Int).SetUint64(math.MaxUint64)) != 0 {
		t.Fatalf("numeric sequence = %#v, want MaxUint64 exactly", queries.ackParams.BrokerSequence)
	}
}

func TestFailureMapsRetryAndParkWithoutCallerScheduling(t *testing.T) {
	t.Parallel()

	next := time.Date(2026, 8, 24, 7, 8, 9, 0, time.UTC)
	queries := &scriptedQueries{failureRow: dbgen.RecordTransactionalOutboxPublishFailureRow{
		ResultCode:    "retry-scheduled",
		NextAttemptAt: timestamp(next),
	}}
	got, err := (&adapter{queries: queries}).recordFailure(context.Background(), publishFailureRequest{
		fence: testFence(),
		code:  failureTransportUnavailable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.disposition != failureDispositionRetryScheduled || !got.nextAttemptAt.Equal(next) {
		t.Fatalf("failure result = %#v, want database schedule", got)
	}

	queries.failureRow = dbgen.RecordTransactionalOutboxPublishFailureRow{ResultCode: "parked"}
	got, err = (&adapter{queries: queries}).recordFailure(context.Background(), publishFailureRequest{
		fence: testFence(),
		code:  failureEventPermanent,
	})
	if err != nil || got.disposition != failureDispositionParked || !got.nextAttemptAt.IsZero() {
		t.Fatalf("park result = %#v, error = %v", got, err)
	}
}

func TestAdapterRejectsInvalidTrustedInputBeforeQuery(t *testing.T) {
	t.Parallel()

	queries := &scriptedQueries{}
	bound := &adapter{queries: queries}
	if _, err := bound.claim(context.Background(), claimRequest{claimOwnerID: "worker-1", batchSize: 257}); !hasOperationCode(err, errorInvalidInput) {
		t.Fatalf("claim error = %v, want invalid-input", err)
	}
	if _, err := bound.acknowledge(context.Background(), acknowledgementRequest{
		fence:  testFence(),
		pubAck: pubAck{stream: "DOMAIN_EVENTS", sequence: 0, messageID: strings.Repeat("a", 64)},
	}); !hasOperationCode(err, errorInvalidInput) {
		t.Fatalf("ack error = %v, want invalid-input", err)
	}
	if _, err := bound.recordFailure(context.Background(), publishFailureRequest{
		fence: testFence(),
		code:  "future-code",
	}); !hasOperationCode(err, errorInvalidInput) {
		t.Fatalf("failure error = %v, want invalid-input", err)
	}
	if queries.calls != 0 {
		t.Fatalf("database calls = %d, want none", queries.calls)
	}
}

func TestAdapterErrorsAreSecretSafeAndCancellationPropagates(t *testing.T) {
	t.Parallel()

	queries := &scriptedQueries{renewErr: errors.New("raw sql secret tenant-1")}
	_, err := (&adapter{queries: queries}).renew(context.Background(), testFence())
	if !hasOperationCode(err, errorPersistence) || strings.Contains(err.Error(), "raw sql") || strings.Contains(err.Error(), "tenant-1") {
		t.Fatalf("renew error leaked database detail: %q", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (&adapter{queries: queries}).claim(ctx, claimRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled claim error = %v, want context.Canceled", err)
	}
}

type scriptedQueries struct {
	claimRows   []dbgen.ClaimTransactionalOutboxBatchRow
	claimErr    error
	renewRow    dbgen.RenewTransactionalOutboxClaimRow
	renewErr    error
	ackResult   string
	ackErr      error
	failureRow  dbgen.RecordTransactionalOutboxPublishFailureRow
	failureErr  error
	renewParams dbgen.RenewTransactionalOutboxClaimParams
	ackParams   dbgen.AcknowledgeTransactionalOutboxPublishedParams
	failParams  dbgen.RecordTransactionalOutboxPublishFailureParams
	calls       int
}

func (queries *scriptedQueries) ClaimTransactionalOutboxBatch(_ context.Context, _ dbgen.ClaimTransactionalOutboxBatchParams) ([]dbgen.ClaimTransactionalOutboxBatchRow, error) {
	queries.calls++
	return queries.claimRows, queries.claimErr
}

func (queries *scriptedQueries) RenewTransactionalOutboxClaim(_ context.Context, params dbgen.RenewTransactionalOutboxClaimParams) (dbgen.RenewTransactionalOutboxClaimRow, error) {
	queries.calls++
	params.CandidateDigest = append([]byte(nil), params.CandidateDigest...)
	queries.renewParams = params
	return queries.renewRow, queries.renewErr
}

func (queries *scriptedQueries) AcknowledgeTransactionalOutboxPublished(_ context.Context, params dbgen.AcknowledgeTransactionalOutboxPublishedParams) (string, error) {
	queries.calls++
	params.CandidateDigest = append([]byte(nil), params.CandidateDigest...)
	queries.ackParams = params
	return queries.ackResult, queries.ackErr
}

func (queries *scriptedQueries) RecordTransactionalOutboxPublishFailure(_ context.Context, params dbgen.RecordTransactionalOutboxPublishFailureParams) (dbgen.RecordTransactionalOutboxPublishFailureRow, error) {
	queries.calls++
	params.CandidateDigest = append([]byte(nil), params.CandidateDigest...)
	queries.failParams = params
	return queries.failureRow, queries.failureErr
}

func testClaimRow(raw []byte) dbgen.ClaimTransactionalOutboxBatchRow {
	claimedAt := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	return dbgen.ClaimTransactionalOutboxBatchRow{
		ResultCode:              "claimed",
		TenantID:                "tenant-1",
		EventID:                 "event-1",
		OutboxEntryID:           17,
		DeliveryAttemptID:       23,
		ReplayGeneration:        0,
		TotalAttemptNumber:      1,
		GenerationAttemptNumber: 1,
		ClaimOwnerID:            "worker-1",
		RawClaimToken:           raw,
		ClaimedAt:               timestamp(claimedAt),
		LeaseExpiresAt:          timestamp(claimedAt.Add(30 * time.Second)),
		AbsoluteLeaseExpiresAt:  timestamp(claimedAt.Add(5 * time.Minute)),
		BrokerMessageID:         strings.Repeat("a", 64),
		Destination:             "domain-events",
		EventType:               "message.committed",
		SchemaVersion:           1,
		AggregateKind:           "message",
		AggregateID:             "message-1",
		Payload:                 []byte("secret-payload"),
		OccurredAt:              timestamp(claimedAt.Add(-time.Second)),
		EnqueuedAt:              timestamp(claimedAt.Add(-500 * time.Millisecond)),
		PolicyID:                "threadline.outbox.policy/v1",
		PolicySnapshotDigest:    make([]byte, policyDigestBytes),
	}
}

func testFence() claimFence {
	return claimFence{
		tenantID:          "tenant-1",
		eventID:           "event-1",
		outboxEntryID:     17,
		deliveryAttemptID: 23,
		replayGeneration:  0,
		claimOwnerID:      "worker-1",
		token:             claimToken{wire: goldenClaimTokenWire},
	}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func hasOperationCode(err error, code errorCode) bool {
	var failure *operationFailure
	return errors.As(err, &failure) && failure.category() == code
}

func fmtAll(value any) string {
	return fmt.Sprintf("%v %+v %#v", value, value, value)
}
