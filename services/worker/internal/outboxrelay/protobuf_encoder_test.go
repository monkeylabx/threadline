package outboxrelay

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"google.golang.org/protobuf/encoding/protowire"
)

func wireFacts() outboxdb.PublishFacts {
	stamp := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	return outboxdb.PublishFacts{TenantID: "t", EventID: "e", OutboxEntryID: 1,
		LogicalDestination: "domain-events", BrokerMessageID: strings.Repeat("a", 64),
		EventType: "channel.message.committed", SchemaVersion: 1, AggregateKind: "channel", AggregateID: "c",
		Payload: []byte{0, 255, 128}, OccurredAt: stamp, EnqueuedAt: stamp}
}

func TestProtobufEncoderMatchesGoldenAndRetainsOpaquePayload(t *testing.T) {
	facts := wireFacts()
	encoded, err := (ProtobufEncoder{}).Encode(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../../../proto/golden/domain-events-v1/envelope.hex")
	if err != nil {
		t.Fatal(err)
	}
	golden, err := hex.DecodeString(strings.TrimSpace(string(fixture)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(golden, encoded) {
		t.Fatal("known fields differ from contract Golden")
	}
	canary := golden[len(encoded):]
	number, kind, count := protowire.ConsumeTag(canary)
	if count < 0 || number != 50000 || kind != protowire.BytesType {
		t.Fatal("Golden canary changed")
	}
	before := bytes.Clone(encoded)
	facts.Payload[0] = 1
	if !bytes.Equal(encoded, before) {
		t.Fatal("encoded payload aliases caller")
	}

}

func TestProtobufEncoderAllowsEmptyAndRejectsInvalidFacts(t *testing.T) {
	facts := wireFacts()
	facts.Payload = nil
	if _, err := (ProtobufEncoder{}).Encode(context.Background(), facts); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*outboxdb.PublishFacts){
		func(f *outboxdb.PublishFacts) { f.SchemaVersion = 0 },
		func(f *outboxdb.PublishFacts) { f.OutboxEntryID = 0 },
		func(f *outboxdb.PublishFacts) { f.Payload = make([]byte, 262145) },
		func(f *outboxdb.PublishFacts) { f.OccurredAt = time.Time{} },
		func(f *outboxdb.PublishFacts) { f.TenantID = "" },
	} {
		invalid := wireFacts()
		mutate(&invalid)
		_, err := (ProtobufEncoder{}).Encode(context.Background(), invalid)
		if code, ok := EncoderFailureCodeOf(err); !ok || code != EncoderFailureInvalidInput {
			t.Fatal("invalid facts accepted")
		}
	}
}

func TestProtobufEncoderRejectsCancellationAndUnrepresentableValues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (ProtobufEncoder{}).Encode(ctx, wireFacts()); err != context.Canceled {
		t.Fatal("cancellation lost")
	}
	if _, err := (ProtobufEncoder{}).Encode(nil, wireFacts()); err == nil {
		t.Fatal("nil context accepted")
	}
	for _, mutate := range []func(*outboxdb.PublishFacts){
		func(f *outboxdb.PublishFacts) { f.OccurredAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		func(f *outboxdb.PublishFacts) { f.TenantID = string([]byte{255}) },
	} {
		facts := wireFacts()
		mutate(&facts)
		_, err := (ProtobufEncoder{}).Encode(context.Background(), facts)
		if code, ok := EncoderFailureCodeOf(err); !ok || code != EncoderFailureEventPermanent {
			t.Fatal("permanent encoding failure unclassified")
		}
	}
}
