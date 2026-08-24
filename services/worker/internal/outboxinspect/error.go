package outboxinspect

import (
	"context"
	"encoding/json"
	"errors"
)

type ErrorCode string

const (
	ErrorInvalidInput ErrorCode = "invalid-input"
	ErrorUnavailable  ErrorCode = "unavailable"
	ErrorIncompatible ErrorCode = "incompatible"
)

func (ErrorCode) String() string   { return "<redacted-outbox-inspection-error-code>" }
func (ErrorCode) GoString() string { return "<redacted-outbox-inspection-error-code>" }
func (ErrorCode) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-inspection-error-code]")
}

func ErrorCodeOf(err error) (ErrorCode, bool) {
	var failure *inspectionFailure
	if !errors.As(err, &failure) || failure == nil {
		return "", false
	}
	return failure.code, validErrorCode(failure.code)
}

type inspectionFailure struct{ code ErrorCode }

func (failure *inspectionFailure) Error() string {
	if failure == nil || !validErrorCode(failure.code) {
		return "outbox-inspection-invalid"
	}
	return "outbox-inspection-" + string(failure.code)
}
func (failure *inspectionFailure) String() string   { return failure.Error() }
func (failure *inspectionFailure) GoString() string { return failure.Error() }
func (failure *inspectionFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal(failure.Error())
}

func inspectError(code ErrorCode) error { return &inspectionFailure{code: code} }

func postDependencyError(ctx context.Context, err error) error {
	if callerErr := ctx.Err(); callerErr != nil {
		return callerErr
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return inspectError(ErrorUnavailable)
}

func validErrorCode(code ErrorCode) bool {
	switch code {
	case ErrorInvalidInput, ErrorUnavailable, ErrorIncompatible:
		return true
	default:
		return false
	}
}
