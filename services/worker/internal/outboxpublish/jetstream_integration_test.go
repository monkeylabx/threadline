package outboxpublish

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	liveJetStreamURLVariable = "THREADLINE_TEST_NATS_URL"
	liveJetStreamNamePrefix  = "THREADLINE_C2_TEST_"
	liveJetStreamOwnerKey    = "threadline.io/synthetic-test-owner"
	liveDuplicateWindow      = 120 * time.Second
)

func TestLiveJetStreamDuplicateAndExpectedStreamFencing(t *testing.T) {
	serverURL := os.Getenv(liveJetStreamURLVariable)
	if serverURL == "" {
		t.Skip(liveJetStreamURLVariable + " is not set")
	}

	testID := liveJetStreamTestID(t)
	streamName := liveJetStreamNamePrefix + strings.ToUpper(testID)
	subject := "threadline.synthetic.outbox." + testID

	connection, err := nats.Connect(
		serverURL,
		nats.Name("threadline-c2-live-jetstream-test"),
		nats.NoReconnect(),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		t.Fatal("live JetStream connection failed")
	}
	t.Cleanup(connection.Close)

	broker, err := jetstream.New(connection)
	if err != nil {
		t.Fatal("live JetStream client setup failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := broker.Stream(ctx, streamName); !errors.Is(err, jetstream.ErrStreamNotFound) {
		if err == nil {
			t.Fatal("refusing to reuse an existing synthetic Stream")
		}
		t.Fatal("synthetic Stream ownership preflight failed")
	}

	stream, err := broker.CreateStream(ctx, jetstream.StreamConfig{
		Name:       streamName,
		Subjects:   []string{subject},
		Storage:    jetstream.MemoryStorage,
		MaxMsgs:    16,
		Duplicates: liveDuplicateWindow,
		Metadata: map[string]string{
			liveJetStreamOwnerKey: testID,
		},
	})
	if err != nil {
		t.Fatal("synthetic Stream creation failed")
	}
	t.Cleanup(func() {
		cleanupLiveJetStream(t, broker, streamName, testID)
	})

	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal("synthetic Stream inspection failed")
	}
	if info.Config.Duplicates != liveDuplicateWindow {
		t.Fatalf("duplicate window = %s, want %s", info.Config.Duplicates, liveDuplicateWindow)
	}

	mapping := Mapping{
		LogicalDestination: LogicalDestinationDomainEvents,
		Stream:             streamName,
		Subject:            subject,
	}
	bound, err := NewJetStreamPublisher(broker, mapping)
	if err != nil {
		t.Fatal("live JetStream publisher setup failed")
	}
	message := Message{
		Body:      []byte("threadline-c2-synthetic-event"),
		MessageID: strings.Repeat("a", 64),
	}
	first, err := bound.Publish(ctx, message)
	if err != nil {
		t.Fatal("first synthetic publish failed")
	}
	if first.Stream != streamName || first.Sequence == 0 || first.Duplicate || first.MessageID != message.MessageID {
		t.Fatalf("first PubAck = %#v, want exact Stream, positive sequence, and non-duplicate", first)
	}

	second, err := bound.Publish(ctx, message)
	if err != nil {
		t.Fatal("duplicate synthetic publish failed")
	}
	if second.Stream != streamName || second.Sequence != first.Sequence || !second.Duplicate || second.MessageID != message.MessageID {
		t.Fatalf("duplicate PubAck = %#v, want the first sequence and Duplicate=true", second)
	}

	wrongMapping := mapping
	wrongMapping.Stream += "_WRONG"
	wrongBound, err := NewJetStreamPublisher(broker, wrongMapping)
	if err != nil {
		t.Fatal("wrong-stream publisher setup failed")
	}
	wrongMessage := Message{
		Body:      []byte("threadline-c2-wrong-stream-synthetic-event"),
		MessageID: strings.Repeat("b", 64),
	}
	if _, err := wrongBound.Publish(ctx, wrongMessage); !hasFailureCode(err, FailurePublishOutcomeUnknown) {
		t.Fatal("publish with the wrong expected Stream unexpectedly succeeded")
	}

	info, err = stream.Info(ctx)
	if err != nil {
		t.Fatal("synthetic Stream postcondition inspection failed")
	}
	if info.State.Msgs != 1 {
		t.Fatalf("stored messages = %d, want exactly one deduplicated publish", info.State.Msgs)
	}
}

func liveJetStreamTestID(t *testing.T) string {
	t.Helper()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal("synthetic Stream identity generation failed")
	}
	return hex.EncodeToString(raw[:])
}

func cleanupLiveJetStream(
	t *testing.T,
	broker jetstream.JetStream,
	streamName string,
	testID string,
) {
	t.Helper()
	if !strings.HasPrefix(streamName, liveJetStreamNamePrefix) || testID == "" {
		t.Error("refusing unguarded synthetic Stream cleanup")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := broker.Stream(ctx, streamName)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		return
	}
	if err != nil {
		t.Error("synthetic Stream cleanup inspection failed")
		return
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Error("synthetic Stream cleanup ownership check failed")
		return
	}
	if info.Config.Metadata[liveJetStreamOwnerKey] != testID {
		t.Error("refusing to delete a synthetic Stream with mismatched ownership")
		return
	}
	if err := broker.DeleteStream(ctx, streamName); err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
		t.Error("synthetic Stream cleanup failed")
	}
}
