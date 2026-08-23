package outboxdb_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxrelay"
)

const externalMessageID = "e57ad815a402753dd7698b0e941f70108383c92afecfc5d0c2b699ac36c82e97"

func TestSiblingRelayPackageCanConsumeStoreAndEncoderContracts(t *testing.T) {
	t.Parallel()

	binding := outboxdb.Binding{
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		Stream:             "DOMAIN_EVENTS",
	}
	production, err := outboxdb.NewStore(externalDBTX{}, binding)
	if err != nil {
		t.Fatalf("construct production Store through exported API: %v", err)
	}
	var _ outboxdb.Store = production

	claimedAt := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	facts := outboxdb.PublishFacts{
		TenantID:           "tenant-external-synthetic",
		EventID:            "event-external-synthetic",
		OutboxEntryID:      17,
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		BrokerMessageID:    externalMessageID,
		EventType:          "message.created",
		SchemaVersion:      1,
		AggregateKind:      "channel",
		AggregateID:        "channel-external-synthetic",
		Payload:            []byte("opaque-synthetic-payload"),
		OccurredAt:         claimedAt.Add(-2 * time.Second),
		EnqueuedAt:         claimedAt.Add(-time.Second),
	}
	lease := outboxdb.Lease{
		ClaimedAt: claimedAt, ExpiresAt: claimedAt.Add(30 * time.Second),
		AbsoluteLeaseExpiresAt: claimedAt.Add(5 * time.Minute),
	}
	scripted, err := outboxdb.NewScriptedStore(binding, outboxdb.ScriptedStorePlan{
		Claims: []outboxdb.ScriptedClaimStep{{Claims: []outboxdb.ScriptedClaim{{
			PublishFacts: facts, Lease: lease,
		}}}},
		Renewals: []outboxdb.ScriptedRenewStep{{Renewal: outboxdb.Renewal{
			LeaseExpiresAt: lease.ExpiresAt.Add(time.Second),
		}}},
		Acknowledgements: []outboxdb.ScriptedAcknowledgementStep{{
			Acknowledgement: outboxdb.AcknowledgementDelivered,
		}},
		Failures: []outboxdb.ScriptedFailureStep{{Result: outboxdb.FailureResult{
			Disposition: outboxdb.FailureParked,
		}}},
	})
	if err != nil {
		t.Fatalf("construct scripted Store through exported API: %v", err)
	}
	var relayStore outboxdb.Store = scripted
	claims, err := relayStore.Claim(context.Background(), outboxdb.ClaimRequest{
		ClaimOwnerID: "worker-external-synthetic", BatchSize: 1,
	})
	if err != nil || len(claims) != 1 {
		t.Fatalf("external Claim = %d/%v, want one", len(claims), err)
	}
	if _, err := relayStore.Renew(context.Background(), claims[0]); err != nil {
		t.Fatalf("external Renew failed: %v", err)
	}

	encoder, err := outboxrelay.NewScriptedEncoder(outboxrelay.ScriptedEncodingStep{
		Envelope: []byte("encoded-synthetic-envelope"),
	})
	if err != nil {
		t.Fatalf("construct scripted Encoder through exported API: %v", err)
	}
	var relayEncoder outboxrelay.Encoder = encoder
	envelope, err := relayEncoder.Encode(context.Background(), claims[0].PublishFacts())
	if err != nil || string(envelope) != "encoded-synthetic-envelope" {
		t.Fatalf("external Encode = %q/%v", envelope, err)
	}

	ack := outboxpublish.Acknowledgement{
		Stream: binding.Stream, Sequence: 1, Duplicate: true, MessageID: externalMessageID,
	}
	if result, err := relayStore.Acknowledge(context.Background(), claims[0], ack); err != nil || result != outboxdb.AcknowledgementDelivered {
		t.Fatalf("external Ack = %v/%v", result, err)
	}
	if result, err := relayStore.RecordFailure(context.Background(), claims[0], outboxdb.FailureEventPermanent); err != nil || result.Disposition != outboxdb.FailureParked {
		t.Fatalf("external failure = %#v/%v", result, err)
	}
	if calls := scripted.Calls(); len(calls.Claims) != 1 || len(calls.Renewals) != 1 ||
		len(calls.Acknowledgements) != 1 || len(calls.Failures) != 1 {
		t.Fatalf("external snapshots were incomplete: %#v", calls)
	}
}

type externalDBTX struct{}

func (externalDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	panic("external contract test must not execute SQL")
}

func (externalDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	panic("external contract test must not execute SQL")
}

func (externalDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	panic("external contract test must not execute SQL")
}
