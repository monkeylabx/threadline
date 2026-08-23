package outboxpublish

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var testMapping = Mapping{
	LogicalDestination: LogicalDestinationDomainEvents,
	Stream:             "DOMAIN_EVENTS",
	Subject:            "threadline.domain.events.v1",
}

func TestJetStreamPublisherBindsTrustedMappingAndRejectsNilBroker(t *testing.T) {
	t.Parallel()

	if _, err := NewJetStreamPublisher(nil, testMapping); !hasFailureCode(err, FailureInvalidInput) {
		t.Fatalf("nil broker error = %v, want invalid-input", err)
	}
	var typedNil *scriptedJetStream
	if _, err := NewJetStreamPublisher(typedNil, testMapping); !hasFailureCode(err, FailureInvalidInput) {
		t.Fatalf("typed-nil broker error = %v, want invalid-input", err)
	}
	invalid := testMapping
	invalid.LogicalDestination = "DOMAIN-EVENTS"
	if _, err := NewJetStreamPublisher(&scriptedJetStream{}, invalid); !hasFailureCode(err, FailureInvalidInput) {
		t.Fatalf("invalid mapping error = %v, want invalid-input", err)
	}
	if _, err := NewJetStreamPublisher(&scriptedJetStream{}, testMapping); err != nil {
		t.Fatalf("valid constructor failed: %v", err)
	}
}

func TestJetStreamPublisherMeasuresCompleteMessageAndPublishesOnce(t *testing.T) {
	t.Parallel()

	for _, totalBytes := range []int{327_679, wireHardBytes} {
		totalBytes := totalBytes
		t.Run(strconv.Itoa(totalBytes), func(t *testing.T) {
			t.Parallel()
			message := testMessageWithTotalSize(t, totalBytes)
			originalBody := bytes.Clone(message.Body)
			broker := &scriptedJetStream{ack: &jetstream.PubAck{
				Stream: testMapping.Stream, Sequence: 1,
			}}
			bound := mustJetStreamPublisher(t, broker, testMapping)

			acknowledgement, err := bound.Publish(context.Background(), message)
			if err != nil {
				t.Fatal(err)
			}
			if !validAcknowledgement(acknowledgement, testMapping.Stream, message.MessageID) {
				t.Fatalf("acknowledgement = %#v, want exact normalized evidence", acknowledgement)
			}
			calls := broker.snapshot()
			if len(calls) != 1 || calls[0].optionCount != 1 {
				t.Fatalf("broker calls/options = %d/%d, want 1/1", len(calls), optionCount(calls))
			}
			assertExactNATSMessage(t, calls[0].message, totalBytes, message)
			if !bytes.Equal(message.Body, originalBody) {
				t.Fatal("publish mutated caller-owned body")
			}
		})
	}
}

func TestJetStreamPublisherRejectsOversizeAndMalformedMessagesBeforePublish(t *testing.T) {
	t.Parallel()

	broker := &scriptedJetStream{ack: &jetstream.PubAck{Stream: testMapping.Stream, Sequence: 1}}
	bound := mustJetStreamPublisher(t, broker, testMapping)
	if _, err := bound.Publish(context.Background(), testMessageWithTotalSize(t, wireHardBytes+1)); !hasFailureCode(err, FailureEventPermanent) {
		t.Fatalf("oversize error = %v, want event-permanent", err)
	}
	if _, err := bound.Publish(context.Background(), Message{Body: []byte{0x01}, MessageID: strings.Repeat("A", 64)}); !hasFailureCode(err, FailureInvalidInput) {
		t.Fatalf("malformed message error = %v, want invalid-input", err)
	}
	if got := len(broker.snapshot()); got != 0 {
		t.Fatalf("rejected message broker calls = %d, want zero", got)
	}
}

func TestJetStreamPublisherNormalizesExactPubAcks(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		sequence  uint64
		duplicate bool
	}{
		{name: "first", sequence: 1},
		{name: "max duplicate", sequence: math.MaxUint64, duplicate: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			broker := &scriptedJetStream{ack: &jetstream.PubAck{
				Stream: testMapping.Stream, Sequence: test.sequence, Duplicate: test.duplicate,
			}}
			bound := mustJetStreamPublisher(t, broker, testMapping)
			ack, err := bound.Publish(context.Background(), Message{Body: []byte{0x01}, MessageID: testMessageID})
			if err != nil {
				t.Fatal(err)
			}
			if ack.Stream != testMapping.Stream || ack.Sequence != test.sequence || ack.Duplicate != test.duplicate || ack.MessageID != testMessageID {
				t.Fatalf("acknowledgement = %#v, want exact PubAck normalization", ack)
			}
		})
	}
}

func TestJetStreamPublisherRejectsUncorrelatedOrMalformedPubAcks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ack    *jetstream.PubAck
		mutate func(*nats.Msg)
	}{
		{name: "nil"},
		{name: "blank stream", ack: &jetstream.PubAck{Sequence: 1}},
		{name: "foreign stream", ack: &jetstream.PubAck{Stream: "OTHER", Sequence: 1}},
		{name: "zero sequence", ack: &jetstream.PubAck{Stream: testMapping.Stream}},
		{
			name: "message ID mismatch",
			ack:  &jetstream.PubAck{Stream: testMapping.Stream, Sequence: 1},
			mutate: func(message *nats.Msg) {
				message.Header.Set(jetstream.MsgIDHeader, strings.Repeat("a", 64))
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			broker := &scriptedJetStream{ack: test.ack, mutate: test.mutate}
			bound := mustJetStreamPublisher(t, broker, testMapping)
			if _, err := bound.Publish(context.Background(), Message{Body: []byte{0x01}, MessageID: testMessageID}); !hasFailureCode(err, FailurePublishOutcomeUnknown) {
				t.Fatalf("PubAck error = %v, want publish-outcome-unknown", err)
			}
			if got := len(broker.snapshot()); got != 1 {
				t.Fatalf("broker calls = %d, want one", got)
			}
		})
	}
}

func TestJetStreamPublisherClassifiesPreHandoffAndPostHandoffFailures(t *testing.T) {
	t.Parallel()

	message := Message{Body: []byte{0x01}, MessageID: testMessageID}
	preCanceledBroker := &scriptedJetStream{}
	bound := mustJetStreamPublisher(t, preCanceledBroker, testMapping)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := bound.Publish(canceled, message); !hasFailureCode(err, FailureTransportUnavailable) {
		t.Fatalf("pre-canceled error = %v, want transport-unavailable", err)
	}
	if got := len(preCanceledBroker.snapshot()); got != 0 {
		t.Fatalf("pre-canceled broker calls = %d, want zero", got)
	}

	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "no responders", err: nats.ErrNoResponders},
		{name: "no Stream response", err: jetstream.ErrNoStreamResponse},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			broker := &scriptedJetStream{err: test.err}
			bound := mustJetStreamPublisher(t, broker, testMapping)
			_, err := bound.Publish(context.Background(), message)
			if !hasFailureCode(err, FailureTransportUnavailable) {
				t.Fatalf("transport error = %v, want transport-unavailable", err)
			}
			assertSecretSafePublishError(t, err)
			if got := len(broker.snapshot()); got != 1 {
				t.Fatalf("transport broker calls = %d, want one", got)
			}
		})
	}

	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "connection closed", err: nats.ErrConnectionClosed},
		{name: "disconnected", err: nats.ErrDisconnected},
		{name: "JetStream connection closed", err: jetstream.ErrConnectionClosed},
		{name: "wrapped disconnect", err: errors.Join(errors.New("credential secret"), nats.ErrDisconnected)},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "canceled after invocation", err: context.Canceled},
		{name: "raw broker", err: errors.New("raw broker credential secret")},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			broker := &scriptedJetStream{err: test.err}
			bound := mustJetStreamPublisher(t, broker, testMapping)
			_, err := bound.Publish(context.Background(), message)
			if !hasFailureCode(err, FailurePublishOutcomeUnknown) {
				t.Fatalf("post-handoff error = %v, want publish-outcome-unknown", err)
			}
			assertSecretSafePublishError(t, err)
			if got := len(broker.snapshot()); got != 1 {
				t.Fatalf("post-handoff broker calls = %d, want one", got)
			}
		})
	}
}

func assertSecretSafePublishError(t *testing.T, err error) {
	t.Helper()
	for _, secret := range []string{"raw broker", "credential", testMapping.Stream, testMapping.Subject, testMessageID} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("publish error exposed secret %q: %q", secret, err)
		}
	}
}

func TestJetStreamPublisherIsRaceSafeForParallelPublishes(t *testing.T) {
	t.Parallel()

	const count = 32
	broker := &scriptedJetStream{ack: &jetstream.PubAck{Stream: testMapping.Stream, Sequence: 1}}
	bound := mustJetStreamPublisher(t, broker, testMapping)
	var wait sync.WaitGroup
	for index := 0; index < count; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := bound.Publish(context.Background(), Message{Body: []byte{0x00, 0xff}, MessageID: testMessageID}); err != nil {
				t.Errorf("parallel publish failed: %v", err)
			}
		}()
	}
	wait.Wait()
	if got := len(broker.snapshot()); got != count {
		t.Fatalf("parallel broker calls = %d, want %d", got, count)
	}
}

func mustJetStreamPublisher(t *testing.T, broker JetStream, mapping Mapping) Publisher {
	t.Helper()
	bound, err := NewJetStreamPublisher(broker, mapping)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func testMessageWithTotalSize(t *testing.T, totalBytes int) Message {
	t.Helper()
	message := Message{MessageID: testMessageID}
	overhead := buildJetStreamMessage(testMapping, message).Size()
	if totalBytes < overhead {
		t.Fatalf("test total %d is below NATS message overhead %d", totalBytes, overhead)
	}
	message.Body = bytes.Repeat([]byte{0x80}, totalBytes-overhead)
	if got := buildJetStreamMessage(testMapping, message).Size(); got != totalBytes {
		t.Fatalf("constructed message size = %d, want %d", got, totalBytes)
	}
	return message
}

func assertExactNATSMessage(t *testing.T, actual *nats.Msg, totalBytes int, expected Message) {
	t.Helper()
	if actual == nil || actual.Size() != totalBytes || actual.Subject != testMapping.Subject || actual.Reply != "" {
		t.Fatalf("NATS message shape = %#v, want exact bound target and %d bytes", actual, totalBytes)
	}
	if len(actual.Header) != 2 || len(actual.Header.Values(jetstream.MsgIDHeader)) != 1 || len(actual.Header.Values(jetstream.ExpectedStreamHeader)) != 1 {
		t.Fatalf("NATS headers = %#v, want exactly two singleton headers", actual.Header)
	}
	if actual.Header.Get(jetstream.MsgIDHeader) != expected.MessageID || actual.Header.Get(jetstream.ExpectedStreamHeader) != testMapping.Stream {
		t.Fatalf("NATS publish headers did not preserve exact authority")
	}
	if !bytes.Equal(actual.Data, expected.Body) {
		t.Fatal("NATS message body changed")
	}
}

type jetStreamCall struct {
	message     *nats.Msg
	optionCount int
}

type scriptedJetStream struct {
	mutex  sync.Mutex
	ack    *jetstream.PubAck
	err    error
	mutate func(*nats.Msg)
	calls  []jetStreamCall
}

func (broker *scriptedJetStream) PublishMsg(_ context.Context, message *nats.Msg, options ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.calls = append(broker.calls, jetStreamCall{
		message:     cloneNATSMessage(message),
		optionCount: len(options),
	})
	if broker.mutate != nil {
		broker.mutate(message)
	}
	if broker.ack == nil {
		return nil, broker.err
	}
	ack := *broker.ack
	return &ack, broker.err
}

func (broker *scriptedJetStream) snapshot() []jetStreamCall {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	calls := make([]jetStreamCall, len(broker.calls))
	for index := range broker.calls {
		calls[index] = jetStreamCall{
			message:     cloneNATSMessage(broker.calls[index].message),
			optionCount: broker.calls[index].optionCount,
		}
	}
	return calls
}

func cloneNATSMessage(message *nats.Msg) *nats.Msg {
	if message == nil {
		return nil
	}
	clone := &nats.Msg{
		Subject: message.Subject,
		Reply:   message.Reply,
		Header:  nats.Header{},
		Data:    bytes.Clone(message.Data),
	}
	for key, values := range message.Header {
		clone.Header[key] = append([]string(nil), values...)
	}
	return clone
}

func optionCount(calls []jetStreamCall) int {
	if len(calls) == 0 {
		return 0
	}
	return calls[0].optionCount
}
