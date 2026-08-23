package outboxpublish

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

const testMessageID = "e57ad815a402753dd7698b0e941f70108383c92afecfc5d0c2b699ac36c82e97"

func hasFailureCode(err error, expected FailureCode) bool {
	actual, ok := FailureCodeOf(err)
	return ok && actual == expected
}

func TestTrustedMappingRejectsAliasesAndBrokerWildcards(t *testing.T) {
	t.Parallel()

	valid := Mapping{
		LogicalDestination: LogicalDestinationDomainEvents,
		Stream:             "DOMAIN_EVENTS",
		Subject:            "threadline.domain.events.v1",
	}
	if !valid.Valid() || !validMapping(valid) {
		t.Fatal("valid trusted mapping rejected")
	}

	invalid := map[string]Mapping{
		"blank destination":   {Stream: valid.Stream, Subject: valid.Subject},
		"destination alias":   {LogicalDestination: "DOMAIN-EVENTS", Stream: valid.Stream, Subject: valid.Subject},
		"blank stream":        {LogicalDestination: LogicalDestinationDomainEvents, Subject: valid.Subject},
		"untrimmed stream":    {LogicalDestination: LogicalDestinationDomainEvents, Stream: " DOMAIN_EVENTS", Subject: valid.Subject},
		"control stream":      {LogicalDestination: LogicalDestinationDomainEvents, Stream: "DOMAIN\nEVENTS", Subject: valid.Subject},
		"wildcard stream":     {LogicalDestination: LogicalDestinationDomainEvents, Stream: "DOMAIN_*", Subject: valid.Subject},
		"blank subject":       {LogicalDestination: LogicalDestinationDomainEvents, Stream: valid.Stream},
		"untrimmed subject":   {LogicalDestination: LogicalDestinationDomainEvents, Stream: valid.Stream, Subject: valid.Subject + " "},
		"control subject":     {LogicalDestination: LogicalDestinationDomainEvents, Stream: valid.Stream, Subject: "threadline.\nevents"},
		"star subject":        {LogicalDestination: LogicalDestinationDomainEvents, Stream: valid.Stream, Subject: "threadline.*"},
		"tail subject":        {LogicalDestination: LogicalDestinationDomainEvents, Stream: valid.Stream, Subject: "threadline.>"},
		"empty subject token": {LogicalDestination: LogicalDestinationDomainEvents, Stream: valid.Stream, Subject: "threadline..events"},
	}
	for name, mapping := range invalid {
		if mapping.Valid() || validMapping(mapping) {
			t.Errorf("%s accepted: %#v", name, mapping)
		}
	}
}

func TestMessageAndAcknowledgementValidation(t *testing.T) {
	t.Parallel()

	if !validMessage(Message{Body: []byte{0}, MessageID: testMessageID}) {
		t.Fatal("canonical message rejected")
	}
	if validMessage(Message{MessageID: testMessageID}) {
		t.Fatal("empty encoded Event envelope accepted")
	}
	for name, messageID := range map[string]string{
		"blank":     "",
		"short":     testMessageID[:63],
		"uppercase": strings.ToUpper(testMessageID),
		"non-hex":   strings.Repeat("z", 64),
	} {
		if validMessage(Message{MessageID: messageID}) {
			t.Errorf("%s message ID accepted", name)
		}
	}

	valid := Acknowledgement{
		Stream:    "DOMAIN_EVENTS",
		Sequence:  math.MaxUint64,
		Duplicate: true,
		MessageID: testMessageID,
	}
	if !validAcknowledgement(valid, valid.Stream, testMessageID) {
		t.Fatal("exact duplicate acknowledgement rejected")
	}
	for name, mutate := range map[string]func(*Acknowledgement){
		"foreign stream": func(value *Acknowledgement) { value.Stream = "OTHER" },
		"zero sequence":  func(value *Acknowledgement) { value.Sequence = 0 },
		"wrong message":  func(value *Acknowledgement) { value.MessageID = strings.Repeat("a", 64) },
	} {
		candidate := valid
		mutate(&candidate)
		if validAcknowledgement(candidate, valid.Stream, testMessageID) {
			t.Errorf("%s acknowledgement accepted", name)
		}
	}
}

func TestPublishValuesAndFailuresAreSecretSafe(t *testing.T) {
	t.Parallel()

	secretBody := "secret-payload-marker"
	secretStream := "SECRET_STREAM"
	secretSubject := "secret.subject"
	secretMapping := Mapping{
		LogicalDestination: LogicalDestinationDomainEvents,
		Stream:             secretStream,
		Subject:            secretSubject,
	}
	secretAck := Acknowledgement{Stream: secretStream, Sequence: 1, MessageID: testMessageID}
	scripted, err := NewScriptedPublisher(secretMapping, ScriptedStep{Acknowledgement: secretAck})
	if err != nil {
		t.Fatal(err)
	}
	production, err := NewJetStreamPublisher(&scriptedJetStream{}, secretMapping)
	if err != nil {
		t.Fatal(err)
	}
	values := []any{
		Mapping{LogicalDestination: LogicalDestinationDomainEvents, Stream: secretStream, Subject: secretSubject},
		Message{Body: []byte(secretBody), MessageID: testMessageID},
		secretAck,
		ScriptedStep{Acknowledgement: secretAck},
		scripted,
		production,
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, rendered := range []string{
			fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(encoded),
		} {
			for _, secret := range []string{secretBody, secretStream, secretSubject, testMessageID} {
				if strings.Contains(rendered, secret) {
					t.Fatalf("rendering exposed publish fact %q: %q", secret, rendered)
				}
			}
		}
	}

	for _, code := range []FailureCode{
		FailureInvalidInput,
		FailureEventPermanent,
		FailureTransportUnavailable,
		FailurePublishOutcomeUnknown,
	} {
		failure := publishError(code)
		if !hasFailureCode(failure, code) || failure.Error() != "transactional outbox publish: "+string(code) {
			t.Fatalf("failure = %#v, want stable %q", failure, code)
		}
		encoded, err := json.Marshal(failure)
		if err != nil || !strings.Contains(string(encoded), string(code)) {
			t.Fatalf("failure JSON = %q, error = %v", encoded, err)
		}
	}
}

func TestScriptedFakeImplementsPublisherContract(t *testing.T) {
	t.Parallel()

	wantAck := Acknowledgement{Stream: "DOMAIN_EVENTS", Sequence: 1, MessageID: testMessageID}
	fake, err := NewScriptedPublisher(
		testMapping,
		ScriptedStep{Acknowledgement: wantAck},
		ScriptedStep{Failure: FailureTransportUnavailable},
	)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("opaque")
	message := Message{Body: body, MessageID: testMessageID}
	gotAck, err := fake.Publish(context.Background(), message)
	if err != nil || gotAck != wantAck {
		t.Fatalf("acknowledgement = %#v, error = %v", gotAck, err)
	}
	body[0] = 'X'
	if _, err := fake.Publish(context.Background(), message); !hasFailureCode(err, FailureTransportUnavailable) {
		t.Fatalf("second publish error = %v, want transport-unavailable", err)
	}
	calls := fake.Calls()
	if len(calls) != 2 || string(calls[0].Body) != "opaque" {
		t.Fatalf("captured calls = %#v, want cloned messages", calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fake.Publish(ctx, message); !hasFailureCode(err, FailureTransportUnavailable) {
		t.Fatalf("pre-canceled publish error = %v, want transport-unavailable", err)
	}
	if got := len(fake.Calls()); got != 2 {
		t.Fatalf("pre-canceled publish calls = %d, want 2", got)
	}
	if _, err := fake.Publish(nil, message); !hasFailureCode(err, FailureTransportUnavailable) {
		t.Fatalf("nil-context publish error = %v, want transport-unavailable", err)
	}
	if got := len(fake.Calls()); got != 2 {
		t.Fatalf("nil-context publish calls = %d, want 2", got)
	}
}

func TestScriptedFakeRejectsInvalidOrMixedFailureSteps(t *testing.T) {
	t.Parallel()

	for _, step := range []ScriptedStep{
		{},
		{Failure: "future-category"},
		{Acknowledgement: Acknowledgement{Stream: "OTHER", Sequence: 1, MessageID: testMessageID}},
		{
			Acknowledgement: Acknowledgement{Stream: "DOMAIN_EVENTS", Sequence: 1, MessageID: testMessageID},
			Failure:         FailureTransportUnavailable,
		},
	} {
		if _, err := NewScriptedPublisher(testMapping, step); !hasFailureCode(err, FailureInvalidInput) {
			t.Fatalf("invalid scripted step error = %v, want invalid-input", err)
		}
	}
}

func TestScriptedFakeIsRaceSafeForParallelRelayTests(t *testing.T) {
	t.Parallel()

	const count = 32
	steps := make([]ScriptedStep, count)
	for index := range steps {
		steps[index].Acknowledgement = Acknowledgement{
			Stream: "DOMAIN_EVENTS", Sequence: uint64(index + 1), MessageID: testMessageID,
		}
	}
	fake, err := NewScriptedPublisher(testMapping, steps...)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for index := 0; index < count; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = fake.Publish(context.Background(), Message{Body: []byte("opaque"), MessageID: testMessageID})
		}()
	}
	wait.Wait()
	if got := len(fake.Calls()); got != count {
		t.Fatalf("parallel publish calls = %d, want %d", got, count)
	}
}
