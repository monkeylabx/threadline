package outboxrelay

import (
	"bytes"
	"context"
	"sync"
	"testing"
)

func TestScriptedEncoderClonesInputsCallsAndResults(t *testing.T) {
	t.Parallel()

	originalEnvelope := []byte("encoded-envelope")
	encoder, err := NewScriptedEncoder(ScriptedEncodingStep{Envelope: originalEnvelope})
	if err != nil {
		t.Fatal(err)
	}
	originalEnvelope[0] = 'X'

	payload := []byte("opaque-payload")
	facts := validEncoderTestFacts(payload)
	encoded, err := encoder.Encode(context.Background(), facts)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "encoded-envelope" {
		t.Fatalf("encoded result = %q, want constructor-time clone", encoded)
	}
	payload[0] = 'X'
	encoded[0] = 'X'

	calls := encoder.Calls()
	if len(calls) != 1 || string(calls[0].Payload) != "opaque-payload" {
		t.Fatalf("calls = %#v, want one cloned PublishFacts value", calls)
	}
	calls[0].Payload[0] = 'Y'
	if got := encoder.Calls(); len(got) != 1 || string(got[0].Payload) != "opaque-payload" {
		t.Fatal("mutating a Calls snapshot changed the fake's retained input")
	}

	if _, err := encoder.Encode(context.Background(), facts); encoderFailureCode(err) != EncoderFailureInvalidInput {
		t.Fatalf("exhausted fake error = %v, want invalid-input", err)
	}
}

func TestScriptedEncoderIsRaceSafeAndReturnsIndependentBytes(t *testing.T) {
	t.Parallel()

	const count = 64
	steps := make([]ScriptedEncodingStep, count)
	for index := range steps {
		steps[index].Envelope = []byte("encoded-envelope")
	}
	encoder, err := NewScriptedEncoder(steps...)
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan []byte, count)
	var wait sync.WaitGroup
	for index := 0; index < count; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			encoded, err := encoder.Encode(context.Background(), validEncoderTestFacts([]byte("opaque")))
			if err != nil {
				t.Errorf("parallel Encode failed: %v", err)
				return
			}
			results <- encoded
		}()
	}
	wait.Wait()
	close(results)

	var first []byte
	for encoded := range results {
		if !bytes.Equal(encoded, []byte("encoded-envelope")) {
			t.Errorf("parallel encoded result = %q", encoded)
		}
		if first == nil {
			first = encoded
		} else if len(first) > 0 && len(encoded) > 0 && &first[0] == &encoded[0] {
			t.Error("parallel Encode results shared backing storage")
		}
	}
	if got := len(encoder.Calls()); got != count {
		t.Fatalf("parallel calls = %d, want %d", got, count)
	}
}
