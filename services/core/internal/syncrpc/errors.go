package syncrpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
)

// rpcError builds a Connect error carrying the stable ErrorDetail code and
// reason token from the sync contract.
func rpcError(code connect.Code, errorCode typev1.ErrorCode, reason string) error {
	err := connect.NewError(code, errors.New(reason))
	if detail, detailErr := connect.NewErrorDetail(&typev1.ErrorDetail{Code: errorCode, Reason: reason}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
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

// cursorInvalid is the per-cursor rejection detail; it names a reason only.
func cursorInvalid(reason string) *typev1.ErrorDetail {
	return &typev1.ErrorDetail{Code: typev1.ErrorCode_ERROR_CODE_CURSOR_INVALID, Reason: reason}
}
