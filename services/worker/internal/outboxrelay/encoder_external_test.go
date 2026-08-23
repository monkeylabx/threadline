package outboxrelay_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxrelay"
)

const externalEncoderMessageID = "e57ad815a402753dd7698b0e941f70108383c92afecfc5d0c2b699ac36c82e97"

func TestSiblingPackageCanUseEncoderAndScriptedAdapter(t *testing.T) {
	t.Parallel()

	encoder, err := outboxrelay.NewScriptedEncoder(
		outboxrelay.ScriptedEncodingStep{Envelope: []byte("complete-envelope")},
		outboxrelay.ScriptedEncodingStep{Failure: outboxrelay.EncoderFailureEventRetryable},
	)
	if err != nil {
		t.Fatal(err)
	}
	var relayEncoder outboxrelay.Encoder = encoder
	facts := outboxdb.PublishFacts{
		TenantID:           "tenant",
		EventID:            "event",
		OutboxEntryID:      1,
		LogicalDestination: "domain-events",
		BrokerMessageID:    externalEncoderMessageID,
		EventType:          "message.committed",
		SchemaVersion:      1,
		AggregateKind:      "channel",
		AggregateID:        "channel-1",
		OccurredAt:         time.Unix(1, 0).UTC(),
		EnqueuedAt:         time.Unix(2, 0).UTC(),
	}
	encoded, err := relayEncoder.Encode(context.Background(), facts)
	if err != nil || string(encoded) != "complete-envelope" {
		t.Fatalf("Encode = %q, %v", encoded, err)
	}
	if _, err := relayEncoder.Encode(context.Background(), facts); err == nil {
		t.Fatal("scripted failure unexpectedly succeeded")
	} else if code, ok := outboxrelay.EncoderFailureCodeOf(err); !ok || code != outboxrelay.EncoderFailureEventRetryable {
		t.Fatalf("failure category = %q/%t, want event-retryable", code, ok)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := relayEncoder.Encode(canceled, facts); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v, want context.Canceled", err)
	}
	if got := len(encoder.Calls()); got != 2 {
		t.Fatalf("recorded calls = %d, want two non-canceled calls", got)
	}
}
