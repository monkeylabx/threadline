package outboxpublish

import (
	"bytes"
	"context"
	"errors"
	"reflect"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetStream is deliberately narrower than jetstream.JetStream. Connection
// lifecycle, broker inspection, and readiness belong to the startup layer.
type JetStream interface {
	PublishMsg(context.Context, *nats.Msg, ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

type jetStreamPublisher struct {
	broker  JetStream
	mapping Mapping
}

func (*jetStreamPublisher) String() string   { return "<redacted-outbox-jetstream-publisher>" }
func (*jetStreamPublisher) GoString() string { return "<redacted-outbox-jetstream-publisher>" }
func (*jetStreamPublisher) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-jetstream-publisher]"`), nil
}

// NewJetStreamPublisher binds one trusted logical destination, Stream, and
// concrete subject to a single-attempt Publisher.
func NewJetStreamPublisher(broker JetStream, mapping Mapping) (Publisher, error) {
	if nilJetStream(broker) || !validMapping(mapping) {
		return nil, publishError(FailureInvalidInput)
	}
	return &jetStreamPublisher{broker: broker, mapping: mapping}, nil
}

func (bound *jetStreamPublisher) Publish(ctx context.Context, message Message) (Acknowledgement, error) {
	if bound == nil || nilJetStream(bound.broker) || !validMapping(bound.mapping) || !validMessage(message) {
		return Acknowledgement{}, publishError(FailureInvalidInput)
	}
	if ctx == nil || ctx.Err() != nil {
		return Acknowledgement{}, publishError(FailureTransportUnavailable)
	}

	outbound := buildJetStreamMessage(bound.mapping, message)
	if outbound.Size() > wireHardBytes {
		return Acknowledgement{}, publishError(FailureEventPermanent)
	}

	expectedBody := bytes.Clone(outbound.Data)
	ack, err := bound.broker.PublishMsg(
		ctx,
		outbound,
		jetstream.WithRetryAttempts(0),
	)
	if err != nil {
		return Acknowledgement{}, classifyJetStreamPublishError(err)
	}
	if ack == nil || ack.Stream != bound.mapping.Stream || ack.Sequence == 0 ||
		outbound.Subject != bound.mapping.Subject || outbound.Reply != "" ||
		len(outbound.Header) != 2 ||
		outbound.Header.Get(jetstream.MsgIDHeader) != message.MessageID ||
		outbound.Header.Get(jetstream.ExpectedStreamHeader) != bound.mapping.Stream ||
		!bytes.Equal(outbound.Data, expectedBody) {
		return Acknowledgement{}, publishError(FailurePublishOutcomeUnknown)
	}

	normalized := Acknowledgement{
		Stream:    ack.Stream,
		Sequence:  ack.Sequence,
		Duplicate: ack.Duplicate,
		MessageID: message.MessageID,
	}
	if !validAcknowledgement(normalized, bound.mapping.Stream, message.MessageID) {
		return Acknowledgement{}, publishError(FailurePublishOutcomeUnknown)
	}
	return normalized, nil
}

func classifyJetStreamPublishError(err error) *publishFailure {
	// No-responder errors prove that no JetStream responder accepted the
	// request. Connection errors are deliberately excluded: the connection can
	// close after the server persisted the message but before the Ack arrives.
	if errors.Is(err, nats.ErrNoResponders) ||
		errors.Is(err, jetstream.ErrNoStreamResponse) {
		return publishError(FailureTransportUnavailable)
	}
	return publishError(FailurePublishOutcomeUnknown)
}

func nilJetStream(broker JetStream) bool {
	if broker == nil {
		return true
	}
	value := reflect.ValueOf(broker)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func buildJetStreamMessage(mapping Mapping, message Message) *nats.Msg {
	header := nats.Header{}
	header.Set(jetstream.MsgIDHeader, message.MessageID)
	header.Set(jetstream.ExpectedStreamHeader, mapping.Stream)
	return &nats.Msg{
		Subject: mapping.Subject,
		Header:  header,
		Data:    bytes.Clone(message.Body),
	}
}
