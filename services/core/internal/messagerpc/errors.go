package messagerpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
)

// errRace marks a unique-constraint race with a concurrent send of the same
// key or first sequence head; the caller re-runs the transaction to observe
// the winner instead of inventing an outcome.
var errRace = errors.New("concurrent send race")

// rpcError builds a Connect error carrying the stable ErrorDetail code and
// reason from the message contract. The reason is a token, never user data.
func rpcError(code connect.Code, errorCode typev1.ErrorCode, reason string) error {
	err := connect.NewError(code, errors.New(reason))
	if detail, detailErr := connect.NewErrorDetail(&typev1.ErrorDetail{Code: errorCode, Reason: reason}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func invalidEnvelope(reason string) error {
	return rpcError(connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, reason)
}

func sliceGate(reason string) error {
	return rpcError(connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, reason)
}

func conflict(reason string) error {
	return rpcError(connect.CodeAlreadyExists, typev1.ErrorCode_ERROR_CODE_IDEMPOTENCY_CONFLICT, reason)
}

// unavailable reports a context end as such and every other infrastructure
// failure as a retryable NOT_DURABLE condition.
func unavailable(ctx context.Context, reason string) error {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return connect.NewError(connect.CodeCanceled, context.Canceled)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded)
	default:
		return rpcError(connect.CodeUnavailable, typev1.ErrorCode_ERROR_CODE_NOT_DURABLE, reason)
	}
}
