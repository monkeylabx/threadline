package outboxpublish

import (
	"context"
	"sync"
)

// ScriptedStep is one deterministic result returned by ScriptedPublisher.
// Exactly one of Acknowledgement or Failure must describe the result.
type ScriptedStep struct {
	Acknowledgement Acknowledgement
	Failure         FailureCode
}

func (ScriptedStep) String() string   { return "<redacted-outbox-publish-scripted-step>" }
func (ScriptedStep) GoString() string { return "<redacted-outbox-publish-scripted-step>" }
func (ScriptedStep) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-publish-scripted-step]"`), nil
}

// ScriptedPublisher is the deterministic fake at the Publisher seam for
// later relay orchestration and contract tests.
type ScriptedPublisher struct {
	mutex   sync.Mutex
	mapping Mapping
	steps   []ScriptedStep
	calls   []Message
}

var _ Publisher = (*ScriptedPublisher)(nil)

func (*ScriptedPublisher) String() string   { return "<redacted-outbox-scripted-publisher>" }
func (*ScriptedPublisher) GoString() string { return "<redacted-outbox-scripted-publisher>" }
func (*ScriptedPublisher) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-scripted-publisher]"`), nil
}

// NewScriptedPublisher creates a race-safe Publisher fake bound to the same
// trusted Mapping as production. Failure steps must use one of this package's
// stable FailureCode values; success acknowledgements are correlated and
// validated against the Mapping and Message.
func NewScriptedPublisher(mapping Mapping, steps ...ScriptedStep) (*ScriptedPublisher, error) {
	if !validMapping(mapping) {
		return nil, publishError(FailureInvalidInput)
	}
	cloned := append([]ScriptedStep(nil), steps...)
	for _, step := range cloned {
		if step.Failure != "" {
			if !validFailureCode(step.Failure) || step.Acknowledgement != (Acknowledgement{}) {
				return nil, publishError(FailureInvalidInput)
			}
			continue
		}
		if !validAcknowledgement(
			step.Acknowledgement,
			mapping.Stream,
			step.Acknowledgement.MessageID,
		) {
			return nil, publishError(FailureInvalidInput)
		}
	}
	return &ScriptedPublisher{mapping: mapping, steps: cloned}, nil
}

// Publish records a cloned Message and returns the next scripted result.
func (fake *ScriptedPublisher) Publish(ctx context.Context, message Message) (Acknowledgement, error) {
	if fake == nil || !validMapping(fake.mapping) || !validMessage(message) {
		return Acknowledgement{}, publishError(FailureInvalidInput)
	}
	if ctx == nil || ctx.Err() != nil {
		return Acknowledgement{}, publishError(FailureTransportUnavailable)
	}

	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.calls = append(fake.calls, cloneMessage(message))
	if len(fake.steps) == 0 {
		return Acknowledgement{}, publishError(FailurePublishOutcomeUnknown)
	}
	step := fake.steps[0]
	fake.steps = fake.steps[1:]
	if step.Failure != "" {
		return Acknowledgement{}, publishError(step.Failure)
	}
	if !validAcknowledgement(step.Acknowledgement, fake.mapping.Stream, message.MessageID) {
		return Acknowledgement{}, publishError(FailurePublishOutcomeUnknown)
	}
	return step.Acknowledgement, nil
}

// Calls returns cloned Messages in invocation order.
func (fake *ScriptedPublisher) Calls() []Message {
	if fake == nil {
		return nil
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	calls := make([]Message, len(fake.calls))
	for index := range fake.calls {
		calls[index] = cloneMessage(fake.calls[index])
	}
	return calls
}

func cloneMessage(message Message) Message {
	message.Body = append([]byte(nil), message.Body...)
	return message
}
