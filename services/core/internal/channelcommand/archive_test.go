package channelcommand_test

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/channelcommand"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

type archiveVerifier struct {
	tenantID string
	actorID  string
}

func (v archiveVerifier) VerifySession(context.Context, string) (rpcmiddleware.VerifiedSession, error) {
	return rpcmiddleware.VerifiedSession{
		TenantID:  v.tenantID,
		ActorType: rpcmiddleware.ActorTypeHuman,
		ActorID:   v.actorID,
		DeviceID:  "device-channel-archive-synthetic",
		SessionID: "session-channel-archive-synthetic",
	}, nil
}

type archiveRequest struct{}

func TestArchiveRequiresAuthenticatedPrincipalBeforeSQL(t *testing.T) {
	t.Parallel()

	_, err := channelcommand.Archive(context.Background(), nil, "channel-archive-synthetic")
	var commandError *channelcommand.Error
	if !errors.As(err, &commandError) ||
		commandError.Code() != channelcommand.ErrorDenied ||
		commandError.Reason() != authorization.ReasonAuthenticationRequired {
		t.Fatalf("Archive() error = %v, want typed authentication-required denial", err)
	}
	if err.Error() != "channel archive: denied" {
		t.Fatalf("Archive() error text = %q, want stable secret-safe text", err.Error())
	}
}

func TestArchivePreservesCancellationBeforeSQL(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := channelcommand.Archive(ctx, nil, "channel-archive-synthetic")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Archive() error = %v, want context.Canceled", err)
	}
}

func TestArchiveRejectsInvalidTransactionWithStableError(t *testing.T) {
	t.Parallel()

	_, err, authenticationErr := archiveAuthenticated(
		context.Background(), "tenant-channel-archive-synthetic", "actor-channel-archive-synthetic", nil,
		"channel-archive-synthetic",
	)
	if authenticationErr != nil {
		t.Fatalf("authenticate test Principal: %v", authenticationErr)
	}
	var commandError *channelcommand.Error
	if !errors.As(err, &commandError) || commandError.Code() != channelcommand.ErrorInvalidInput {
		t.Fatalf("Archive() error = %v, want typed invalid-input", err)
	}
	if err.Error() != "channel archive: invalid-input" {
		t.Fatalf("Archive() error text = %q, want stable secret-safe text", err.Error())
	}
}

func archiveAuthenticated(
	ctx context.Context,
	tenantID string,
	actorID string,
	tx pgx.Tx,
	channelID string,
) (channelcommand.Result, error, error) {
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(archiveVerifier{
		tenantID: tenantID,
		actorID:  actorID,
	})
	request := connect.NewRequest(&archiveRequest{})
	request.Header().Set("Authorization", "Bearer channel-archive-fixture-credential")
	var result channelcommand.Result
	var commandErr error
	handler := interceptor.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		result, commandErr = channelcommand.Archive(ctx, tx, channelID)
		return connect.NewResponse(&archiveRequest{}), nil
	})
	if _, err := handler(ctx, request); err != nil {
		return channelcommand.Result{}, nil, err
	}
	return result, commandErr, nil
}
