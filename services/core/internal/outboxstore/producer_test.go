package outboxstore

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/internal/outboxpolicy"
)

const defaultPolicyDocument = `{
  "policy_id": "threadline.outbox.policy/v1",
  "payload_hard_bytes": 262144,
  "wire_hard_bytes": 327680,
  "batch_size": 64,
  "lease_ms": 30000,
  "absolute_lifetime_ms": 300000,
  "event_retry_ceiling": 8,
  "transport_base_ms": 1000,
  "transport_cap_ms": 60000,
  "unknown_base_ms": 5000,
  "unknown_cap_ms": 300000,
  "event_base_ms": 5000,
  "event_cap_ms": 300000,
  "retention_days": 90,
  "duplicate_window_ms": 120000
}`

func defaultProducer(t *testing.T) *Producer {
	t.Helper()
	policy, err := outboxpolicy.Parse([]byte(defaultPolicyDocument))
	if err != nil {
		t.Fatalf("Parse(default policy) error = %v", err)
	}
	producer, err := NewProducer(policy)
	if err != nil {
		t.Fatalf("NewProducer() error = %v", err)
	}
	return producer
}

func validFact() ChannelEventCommitted {
	return ChannelEventCommitted{
		TenantID:    "tenant-1",
		ChannelID:   "channel-synthetic",
		ChannelSeq:  1,
		CommittedAt: time.Date(2026, time.October, 18, 10, 0, 0, 0, time.UTC),
	}
}

func TestNewProducerRejectsAnUnparsedSnapshot(t *testing.T) {
	t.Parallel()
	if _, err := NewProducer(outboxpolicy.Snapshot{}); !hasStoreErrorCode(err, errorInvalidInput) {
		t.Fatalf("NewProducer(zero) error = %v, want %q", err, errorInvalidInput)
	}
}

func TestNewProducerCopiesTheTrustedPolicyExactly(t *testing.T) {
	t.Parallel()
	policy, err := outboxpolicy.Parse([]byte(defaultPolicyDocument))
	if err != nil {
		t.Fatalf("Parse(default policy) error = %v", err)
	}
	producer, err := NewProducer(policy)
	if err != nil {
		t.Fatalf("NewProducer() error = %v", err)
	}
	digest := policy.Digest()
	want := policySnapshot{
		id: policyV1ID, snapshotDigest: digest[:],
		leaseMS: 30_000, absoluteLifetimeMS: 300_000, eventRetryCeiling: 8,
		transportBaseMS: 1_000, transportCapMS: 60_000,
		unknownBaseMS: 5_000, unknownCapMS: 300_000,
		eventBaseMS: 5_000, eventCapMS: 300_000, retentionDays: 90,
	}
	got := producer.policy
	if got.id != want.id || string(got.snapshotDigest) != string(want.snapshotDigest) ||
		got.leaseMS != want.leaseMS || got.absoluteLifetimeMS != want.absoluteLifetimeMS ||
		got.eventRetryCeiling != want.eventRetryCeiling ||
		got.transportBaseMS != want.transportBaseMS || got.transportCapMS != want.transportCapMS ||
		got.unknownBaseMS != want.unknownBaseMS || got.unknownCapMS != want.unknownCapMS ||
		got.eventBaseMS != want.eventBaseMS || got.eventCapMS != want.eventCapMS ||
		got.retentionDays != want.retentionDays {
		t.Fatalf("producer policy = %#v, want %#v", got, want)
	}
}

func TestAppendRejectsInvalidFactsBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	producer := defaultProducer(t)
	cases := map[string]func(*ChannelEventCommitted){
		"blank tenant":       func(fact *ChannelEventCommitted) { fact.TenantID = "" },
		"padded tenant":      func(fact *ChannelEventCommitted) { fact.TenantID = " tenant-1" },
		"blank channel":      func(fact *ChannelEventCommitted) { fact.ChannelID = "" },
		"control in channel": func(fact *ChannelEventCommitted) { fact.ChannelID = "channel\n" },
		"oversized channel":  func(fact *ChannelEventCommitted) { fact.ChannelID = strings.Repeat("c", maxIdentifierBytes+1) },
		"sequence zero":      func(fact *ChannelEventCommitted) { fact.ChannelSeq = 0 },
		"zero commit time":   func(fact *ChannelEventCommitted) { fact.CommittedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fact := validFact()
			mutate(&fact)
			tx := &scriptedTx{}
			_, err := producer.AppendChannelEventCommitted(context.Background(), tx, fact)
			if !hasStoreErrorCode(err, errorInvalidInput) || tx.calls != 0 {
				t.Fatalf("Append(%s) = %v after %d database calls, want %q and 0", name, err, tx.calls, errorInvalidInput)
			}
		})
	}
	if _, err := producer.AppendChannelEventCommitted(context.Background(), nil, validFact()); !hasStoreErrorCode(err, errorInvalidInput) {
		t.Fatalf("Append(nil tx) error = %v, want %q", err, errorInvalidInput)
	}
	var missing *Producer
	if _, err := missing.AppendChannelEventCommitted(context.Background(), &scriptedTx{}, validFact()); !hasStoreErrorCode(err, errorInvalidInput) {
		t.Fatalf("Append on nil producer error = %v, want %q", err, errorInvalidInput)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := producer.AppendChannelEventCommitted(ctx, &scriptedTx{}, validFact()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Append(canceled) error = %v, want context.Canceled", err)
	}
}

func TestEncodeChannelEventPayloadMatchesTheGoldenVector(t *testing.T) {
	t.Parallel()
	golden, err := hex.DecodeString("0100000001630000000000000001")
	if err != nil {
		t.Fatal("decode golden vector failed")
	}
	if got := encodeChannelEventPayload("c", 1); string(got) != string(golden) {
		t.Fatalf("payload(c, 1) = %x, want %x", got, golden)
	}
	long := encodeChannelEventPayload("channel-synthetic", 0x0102030405060708)
	if len(long) != 1+4+len("channel-synthetic")+8 || long[0] != payloadKindChannel ||
		long[4] != byte(len("channel-synthetic")) || long[len(long)-1] != 0x08 || long[len(long)-8] != 0x01 {
		t.Fatalf("payload layout = %x, want kind, big-endian length, id, big-endian sequence", long)
	}
}

func TestNewDomainEventIDIsCanonicalAndUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, 256)
	for range 256 {
		id, err := newDomainEventID()
		if err != nil {
			t.Fatalf("newDomainEventID() error = %v", err)
		}
		if len(id) != 2*domainEventIDBytes || id != strings.ToLower(id) || !validIdentifier(id) {
			t.Fatalf("newDomainEventID() = %q, want 32 lowercase hex characters", id)
		}
		if _, err := hex.DecodeString(id); err != nil {
			t.Fatalf("newDomainEventID() = %q is not hexadecimal", id)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("newDomainEventID() repeated %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestErrorCodeOfExposesOnlyStoreCategories(t *testing.T) {
	t.Parallel()
	for _, code := range []errorCode{errorInvalidInput, errorIdempotencyConflict, errorPersistence} {
		got, ok := ErrorCodeOf(storeError(code))
		if !ok || got != string(code) {
			t.Fatalf("ErrorCodeOf(%q) = %q %v, want the same category", code, got, ok)
		}
	}
	if _, ok := ErrorCodeOf(errors.New("other")); ok {
		t.Fatal("ErrorCodeOf(foreign error) reported a store category")
	}
	if ErrorCodeInvalidInput != "invalid-input" || ErrorCodeIdempotencyConflict != "idempotency-conflict" || ErrorCodePersistence != "persistence-failure" {
		t.Fatal("exported error codes drifted from the store categories")
	}
}
