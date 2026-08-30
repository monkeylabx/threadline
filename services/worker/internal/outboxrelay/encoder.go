// Package outboxrelay defines the Worker's private relay orchestration seams.
package outboxrelay

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
)

// Encoder converts immutable publish facts into one complete, already encoded
// Event envelope. It does not choose a broker destination or message ID.
type Encoder interface {
	Encode(context.Context, outboxdb.PublishFacts) ([]byte, error)
}

// EncoderFailureCode is a stable, secret-safe encoding outcome category.
type EncoderFailureCode string

const (
	EncoderFailureInvalidInput   EncoderFailureCode = "invalid-input"
	EncoderFailureEventRetryable EncoderFailureCode = "event-retryable"
	EncoderFailureEventPermanent EncoderFailureCode = "event-permanent"
)

type encoderFailure struct{ code EncoderFailureCode }

func (failure *encoderFailure) Error() string {
	return "transactional outbox encode: " + string(failure.category())
}

func (failure *encoderFailure) String() string   { return failure.Error() }
func (failure *encoderFailure) GoString() string { return failure.Error() }
func (failure *encoderFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal(failure.Error())
}

func (failure *encoderFailure) category() EncoderFailureCode {
	if failure == nil {
		return ""
	}
	return failure.code
}

func encodeError(code EncoderFailureCode) *encoderFailure {
	return &encoderFailure{code: code}
}

// EncoderFailureCodeOf returns the stable category for errors produced by an
// Encoder adapter. Context cancellation and deadline errors are not encoding
// failures and therefore return ok=false.
func EncoderFailureCodeOf(err error) (code EncoderFailureCode, ok bool) {
	var failure *encoderFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	return failure.category(), true
}

func validEncoderFailureCode(code EncoderFailureCode) bool {
	switch code {
	case EncoderFailureInvalidInput,
		EncoderFailureEventRetryable,
		EncoderFailureEventPermanent:
		return true
	default:
		return false
	}
}
