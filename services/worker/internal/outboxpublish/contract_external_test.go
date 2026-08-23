package outboxpublish_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

const externalMessageID = "e57ad815a402753dd7698b0e941f70108383c92afecfc5d0c2b699ac36c82e97"

func TestRelayPackageCanConsumeProductionAndScriptedPublishers(t *testing.T) {
	t.Parallel()

	mapping := outboxpublish.Mapping{
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		Stream:             "DOMAIN_EVENTS",
		Subject:            "threadline.domain.events.v1",
	}
	if !mapping.Valid() {
		t.Fatal("valid mapping rejected by exported validation seam")
	}
	production, err := outboxpublish.NewJetStreamPublisher(externalJetStream{}, mapping)
	if err != nil {
		t.Fatal(err)
	}
	message := outboxpublish.Message{Body: []byte("opaque"), MessageID: externalMessageID}
	if _, err := production.Publish(context.Background(), message); err != nil {
		t.Fatal(err)
	}

	scripted, err := outboxpublish.NewScriptedPublisher(mapping, outboxpublish.ScriptedStep{
		Acknowledgement: outboxpublish.Acknowledgement{
			Stream: mapping.Stream, Sequence: 1, MessageID: externalMessageID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var relayBoundary outboxpublish.Publisher = scripted
	if _, err := relayBoundary.Publish(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if got := len(scripted.Calls()); got != 1 {
		t.Fatalf("scripted calls = %d, want one", got)
	}
	if _, err := relayBoundary.Publish(context.Background(), message); err == nil {
		t.Fatal("exhausted scripted publisher unexpectedly succeeded")
	} else if code, ok := outboxpublish.FailureCodeOf(err); !ok || code != outboxpublish.FailurePublishOutcomeUnknown {
		t.Fatalf("failure category = %q/%t, want publish-outcome-unknown", code, ok)
	}
}

type externalJetStream struct{}

func (externalJetStream) PublishMsg(
	_ context.Context,
	message *nats.Msg,
	_ ...jetstream.PublishOpt,
) (*jetstream.PubAck, error) {
	return &jetstream.PubAck{
		Stream:   message.Header.Get(jetstream.ExpectedStreamHeader),
		Sequence: 1,
	}, nil
}
