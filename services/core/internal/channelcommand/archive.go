// Package channelcommand owns protected Channel mutations. Each command binds
// a fixed authorization action to one narrow database write.
package channelcommand

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/current"
	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

// ErrorCode classifies a stable Channel archive failure.
type ErrorCode string

const (
	// ErrorInvalidInput means the trusted command received an invalid Channel
	// identifier or caller-owned transaction.
	ErrorInvalidInput ErrorCode = "invalid-input"
	// ErrorDenied means current authorization facts denied the fixed archive
	// action. Reason returns the policy reason.
	ErrorDenied ErrorCode = "denied"
	// ErrorPersistence means the protected mutation could not be completed
	// safely from PostgreSQL facts.
	ErrorPersistence ErrorCode = "persistence-failure"
)

// Error is a stable, secret-safe Channel archive error.
type Error struct {
	code   ErrorCode
	reason authorization.Reason
}

func (e *Error) Error() string { return "channel archive: " + string(e.Code()) }

// Code returns the stable failure category.
func (e *Error) Code() ErrorCode {
	if e == nil {
		return ""
	}
	return e.code
}

// Reason returns the authorization denial reason, or an empty value for
// non-policy failures.
func (e *Error) Reason() authorization.Reason {
	if e == nil || e.code != ErrorDenied {
		return ""
	}
	return e.reason
}

// Result is the minimal committed-or-rollbackable archive mutation result.
// The version fields identify the facts used for authorization; callers must
// not reuse them as a permission cache.
type Result struct {
	ChannelID     string
	PolicyVersion string
	ACLVersion    string
}

// Archive authorizes and archives one Channel in a caller-owned transaction.
// Tenant, action, and target state are intentionally not caller-controlled.
// The caller must commit or roll back tx after all same-operation work ends.
func Archive(ctx context.Context, tx pgx.Tx, channelID string) (Result, error) {
	tenantID := ""
	principal, authenticated := rpcmiddleware.PrincipalFromContext(ctx)
	if authenticated {
		tenantID = principal.TenantID()
	}
	ref := authorization.ResourceRef{
		TenantID: tenantID,
		Kind:     authorization.ResourceKindChannel,
		ID:       channelID,
	}
	decision, err := current.EvaluateCurrent(ctx, tx, authorization.ActionChannelArchive, ref)
	if err != nil {
		return Result{}, mapCurrentError(err)
	}
	if decision.Effect != authorization.EffectAllow {
		if decision.Effect != authorization.EffectDeny || decision.Reason == authorization.ReasonAllowed {
			return Result{}, &Error{code: ErrorPersistence}
		}
		return Result{}, &Error{code: ErrorDenied, reason: decision.Reason}
	}
	if !authenticated ||
		decision.Reason != authorization.ReasonAllowed ||
		decision.Action != authorization.ActionChannelArchive ||
		decision.Resource != ref ||
		decision.Actor != (authorization.ActorRef{Type: principal.Actor().Type(), ID: principal.Actor().ID()}) ||
		decision.PolicyVersion == "" || decision.ACLVersion == "" {
		return Result{}, &Error{code: ErrorPersistence}
	}

	archived, err := dbgen.New(tx).ArchiveActiveChannel(ctx, dbgen.ArchiveActiveChannelParams{
		TenantID:  tenantID,
		ChannelID: channelID,
	})
	if err != nil {
		return Result{}, persistenceError(ctx)
	}
	if archived.TenantID != tenantID || archived.ChannelID != channelID || archived.State != 2 {
		return Result{}, &Error{code: ErrorPersistence}
	}
	return Result{
		ChannelID:     archived.ChannelID,
		PolicyVersion: decision.PolicyVersion,
		ACLVersion:    decision.ACLVersion,
	}, nil
}

func mapCurrentError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var currentError *current.Error
	if errors.As(err, &currentError) {
		switch currentError.Code() {
		case current.ErrorInvalidInput:
			return &Error{code: ErrorInvalidInput}
		case current.ErrorPersistence:
			return &Error{code: ErrorPersistence}
		}
	}
	return &Error{code: ErrorPersistence}
}

func persistenceError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return &Error{code: ErrorPersistence}
}
