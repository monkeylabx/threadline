package outboxrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
)

// ScriptedEncodingStep is one deterministic result returned by a
// ScriptedEncoder. Exactly one of Envelope or Failure describes the result.
type ScriptedEncodingStep struct {
	Envelope []byte
	Failure  EncoderFailureCode
}

func (ScriptedEncodingStep) String() string {
	return "<redacted-outbox-scripted-encoding-step>"
}
func (ScriptedEncodingStep) GoString() string {
	return "<redacted-outbox-scripted-encoding-step>"
}
func (ScriptedEncodingStep) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-scripted-encoding-step]")
}

// ScriptedEncoder is the race-safe reusable fake at the Encoder seam.
type ScriptedEncoder struct {
	mutex sync.Mutex
	steps []ScriptedEncodingStep
	calls []outboxdb.PublishFacts
}

var _ Encoder = (*ScriptedEncoder)(nil)

func (*ScriptedEncoder) String() string   { return "<redacted-outbox-scripted-encoder>" }
func (*ScriptedEncoder) GoString() string { return "<redacted-outbox-scripted-encoder>" }
func (*ScriptedEncoder) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-scripted-encoder]")
}

// NewScriptedEncoder constructs a fake with cloned result bytes. Failure
// steps contain no envelope; successful steps contain a non-empty envelope.
func NewScriptedEncoder(steps ...ScriptedEncodingStep) (*ScriptedEncoder, error) {
	cloned := make([]ScriptedEncodingStep, len(steps))
	for index, step := range steps {
		switch {
		case step.Failure != "":
			if !validEncoderFailureCode(step.Failure) || len(step.Envelope) != 0 {
				return nil, encodeError(EncoderFailureInvalidInput)
			}
		case len(step.Envelope) == 0:
			return nil, encodeError(EncoderFailureInvalidInput)
		default:
			cloned[index].Envelope = bytes.Clone(step.Envelope)
		}
		cloned[index].Failure = step.Failure
	}
	return &ScriptedEncoder{steps: cloned}, nil
}

// Encode records a cloned PublishFacts value and returns the next cloned
// scripted result. A canceled context consumes neither a call nor a step.
func (encoder *ScriptedEncoder) Encode(
	ctx context.Context,
	facts outboxdb.PublishFacts,
) ([]byte, error) {
	if encoder == nil || !facts.Valid() {
		return nil, encodeError(EncoderFailureInvalidInput)
	}
	if ctx == nil {
		return nil, encodeError(EncoderFailureInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	encoder.mutex.Lock()
	defer encoder.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoder.calls = append(encoder.calls, facts.Clone())
	if len(encoder.steps) == 0 {
		return nil, encodeError(EncoderFailureInvalidInput)
	}
	step := encoder.steps[0]
	encoder.steps = encoder.steps[1:]
	if step.Failure != "" {
		return nil, encodeError(step.Failure)
	}
	return bytes.Clone(step.Envelope), nil
}

// Calls returns cloned PublishFacts snapshots in invocation order.
func (encoder *ScriptedEncoder) Calls() []outboxdb.PublishFacts {
	if encoder == nil {
		return nil
	}
	encoder.mutex.Lock()
	defer encoder.mutex.Unlock()
	calls := make([]outboxdb.PublishFacts, len(encoder.calls))
	for index := range encoder.calls {
		calls[index] = encoder.calls[index].Clone()
	}
	return calls
}
