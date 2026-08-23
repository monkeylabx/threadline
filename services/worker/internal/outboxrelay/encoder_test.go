package outboxrelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
)

const encoderTestMessageID = "e57ad815a402753dd7698b0e941f70108383c92afecfc5d0c2b699ac36c82e97"

func TestEncoderFailureCategoriesAreStableAndSecretSafe(t *testing.T) {
	t.Parallel()

	for _, code := range []EncoderFailureCode{
		EncoderFailureInvalidInput,
		EncoderFailureEventRetryable,
		EncoderFailureEventPermanent,
	} {
		failure := encodeError(code)
		got, ok := EncoderFailureCodeOf(failure)
		if !ok || got != code || failure.Error() != "transactional outbox encode: "+string(code) {
			t.Fatalf("failure = %v, category = %q/%t, want %q", failure, got, ok, code)
		}
		encoded, err := json.Marshal(failure)
		if err != nil || string(encoded) != `"transactional outbox encode: `+string(code)+`"` {
			t.Fatalf("failure JSON = %q, error = %v", encoded, err)
		}
	}

	if code, ok := EncoderFailureCodeOf(context.Canceled); ok || code != "" {
		t.Fatalf("context cancellation category = %q/%t, want no encoder category", code, ok)
	}
}

func TestScriptedEncoderRejectsInvalidSteps(t *testing.T) {
	t.Parallel()

	for _, step := range []ScriptedEncodingStep{
		{},
		{Failure: "raw-encoder-secret"},
		{Envelope: []byte("encoded-secret"), Failure: EncoderFailureEventRetryable},
	} {
		if _, err := NewScriptedEncoder(step); encoderFailureCode(err) != EncoderFailureInvalidInput {
			t.Fatalf("invalid step error = %v, want invalid-input", err)
		}
	}
}

func TestScriptedEncoderReturnsEveryStableFailure(t *testing.T) {
	t.Parallel()

	facts := validEncoderTestFacts(nil)
	for _, code := range []EncoderFailureCode{
		EncoderFailureInvalidInput,
		EncoderFailureEventRetryable,
		EncoderFailureEventPermanent,
	} {
		code := code
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			encoder, err := NewScriptedEncoder(ScriptedEncodingStep{Failure: code})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := encoder.Encode(context.Background(), facts)
			if encoded != nil || encoderFailureCode(err) != code {
				t.Fatalf("encoded = %q, error = %v, want nil/%q", encoded, err, code)
			}
		})
	}
}

func TestScriptedEncoderPreservesContextCancellationWithoutConsumingStep(t *testing.T) {
	t.Parallel()

	encoder, err := NewScriptedEncoder(ScriptedEncodingStep{Envelope: []byte("envelope")})
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := encoder.Encode(canceled, validEncoderTestFacts(nil)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Encode error = %v, want context.Canceled", err)
	}
	if len(encoder.Calls()) != 0 {
		t.Fatal("canceled Encode recorded a call")
	}
	expired, expire := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer expire()
	if _, err := encoder.Encode(expired, validEncoderTestFacts(nil)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired Encode error = %v, want context.DeadlineExceeded", err)
	}
	if len(encoder.Calls()) != 0 {
		t.Fatal("expired Encode recorded a call")
	}
	encoded, err := encoder.Encode(context.Background(), validEncoderTestFacts(nil))
	if err != nil || string(encoded) != "envelope" {
		t.Fatalf("post-cancel Encode = %q, %v, want preserved step", encoded, err)
	}
	if _, err := encoder.Encode(nil, validEncoderTestFacts(nil)); encoderFailureCode(err) != EncoderFailureInvalidInput {
		t.Fatalf("nil-context Encode error = %v, want invalid-input", err)
	}
}

func TestScriptedEncoderRejectsInvalidFactsBeforeRecording(t *testing.T) {
	t.Parallel()

	encoder, err := NewScriptedEncoder(ScriptedEncodingStep{Envelope: []byte("envelope")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Encode(context.Background(), outboxdb.PublishFacts{}); encoderFailureCode(err) != EncoderFailureInvalidInput {
		t.Fatalf("zero facts Encode error = %v, want invalid-input", err)
	}
	if len(encoder.Calls()) != 0 {
		t.Fatal("invalid facts were recorded")
	}

	// Empty opaque payload is valid input; the Encoder produces the non-empty
	// complete envelope and must not treat the stored payload as the wire body.
	encoded, err := encoder.Encode(context.Background(), validEncoderTestFacts(nil))
	if err != nil || string(encoded) != "envelope" {
		t.Fatalf("empty-payload Encode = %q, %v, want non-empty envelope", encoded, err)
	}
}

func TestTypedNilScriptedEncoderFailsClosed(t *testing.T) {
	t.Parallel()

	var typedNil *ScriptedEncoder
	var encoder Encoder = typedNil
	if _, err := encoder.Encode(context.Background(), validEncoderTestFacts(nil)); encoderFailureCode(err) != EncoderFailureInvalidInput {
		t.Fatalf("typed-nil Encode error = %v, want invalid-input", err)
	}
	if calls := typedNil.Calls(); calls != nil {
		t.Fatalf("typed-nil Calls = %#v, want nil", calls)
	}
}

func TestScriptedEncodingValuesAreSecretSafe(t *testing.T) {
	t.Parallel()

	secretEnvelope := "encoded-envelope-secret"
	secretPayload := "opaque-payload-secret"
	encoder, err := NewScriptedEncoder(ScriptedEncodingStep{Envelope: []byte(secretEnvelope)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Encode(context.Background(), validEncoderTestFacts([]byte(secretPayload))); err != nil {
		t.Fatal(err)
	}
	values := []any{
		ScriptedEncodingStep{Envelope: []byte(secretEnvelope)},
		encoder,
		encoder.Calls()[0],
		encodeError(EncoderFailureEventRetryable),
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, rendered := range []string{
			fmt.Sprint(value),
			fmt.Sprintf("%+v", value),
			fmt.Sprintf("%#v", value),
			string(encoded),
		} {
			for _, secret := range encoderTestSecrets(secretEnvelope, secretPayload) {
				if strings.Contains(rendered, secret) {
					t.Fatalf("rendering exposed secret %q: %q", secret, rendered)
				}
			}
		}
	}
}

func validEncoderTestFacts(payload []byte) outboxdb.PublishFacts {
	return outboxdb.PublishFacts{
		TenantID:           "tenant-secret",
		EventID:            "event-secret",
		OutboxEntryID:      17,
		LogicalDestination: "domain-events",
		BrokerMessageID:    encoderTestMessageID,
		EventType:          "message.committed",
		SchemaVersion:      1,
		AggregateKind:      "channel-secret",
		AggregateID:        "aggregate-secret",
		Payload:            payload,
		OccurredAt:         time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC),
		EnqueuedAt:         time.Date(2026, 8, 24, 1, 2, 4, 0, time.UTC),
	}
}

func encoderFailureCode(err error) EncoderFailureCode {
	code, _ := EncoderFailureCodeOf(err)
	return code
}

func encoderTestSecrets(extra ...string) []string {
	return append([]string{
		"tenant-secret",
		"event-secret",
		"aggregate-secret",
		"channel-secret",
		"domain-events",
		encoderTestMessageID,
	}, extra...)
}
