package channelrpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/current"
	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	channelv1 "github.com/monkeylabx/threadline/services/gen/threadline/channel/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/channel/v1/channelv1connect"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

// TransactionStarter opens the transaction that authorization facts and the
// read share. *pgxpool.Pool satisfies it.
type TransactionStarter interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

const maxChannelIDBytes = 128

// Handler serves ChannelService. Unimplemented procedures come from the
// embedded generated type and answer CodeUnimplemented.
type Handler struct {
	channelv1connect.UnimplementedChannelServiceHandler
	db TransactionStarter
}

// New builds a Handler over db.
func New(db TransactionStarter) (*Handler, error) {
	if db == nil {
		return nil, errors.New("channelrpc: transaction starter required")
	}
	return &Handler{db: db}, nil
}

// GetChannel returns one Channel the authenticated principal may read. The
// tenant comes from the principal, never from the request. Every denial maps
// to CodePermissionDenied so a non-member learns nothing about existence.
func (handler *Handler) GetChannel(
	ctx context.Context,
	request *connect.Request[channelv1.GetChannelRequest],
) (*connect.Response[channelv1.GetChannelResponse], error) {
	principal, authenticated := rpcmiddleware.PrincipalFromContext(ctx)
	if !authenticated {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	channelID := request.Msg.GetChannelId()
	if channelID == "" || len(channelID) > maxChannelIDBytes {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("channel_id invalid"))
	}
	tx, err := handler.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, unavailable(ctx)
	}
	defer rollback(ctx, tx)

	channel, err := readAuthorizedChannel(ctx, tx, principal.TenantID(), channelID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&channelv1.GetChannelResponse{Channel: toProto(channel)}), nil
}

// readAuthorizedChannel evaluates channel.read on current facts and, only on
// allow, reads the row inside the same transaction.
func readAuthorizedChannel(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	channelID string,
) (dbgen.DomainChannel, error) {
	ref := authorization.ResourceRef{
		TenantID: tenantID, Kind: authorization.ResourceKindChannel, ID: channelID,
	}
	decision, err := current.EvaluateCurrent(ctx, tx, authorization.ActionChannelRead, ref)
	if err != nil {
		return dbgen.DomainChannel{}, mapEvaluationError(ctx, err)
	}
	if decision.Effect != authorization.EffectAllow {
		return dbgen.DomainChannel{}, connect.NewError(connect.CodePermissionDenied, errors.New("channel read denied"))
	}
	channel, err := dbgen.New(tx).GetChannel(ctx, dbgen.GetChannelParams{TenantID: tenantID, ChannelID: channelID})
	if err != nil {
		return dbgen.DomainChannel{}, unavailable(ctx)
	}
	return channel, nil
}

// mapEvaluationError turns resolver failures into Connect codes without
// leaking SQL or fact detail. An invalid reference is the caller's fault.
func mapEvaluationError(ctx context.Context, err error) error {
	var evaluation *current.Error
	if errors.As(err, &evaluation) && evaluation.Code() == current.ErrorInvalidInput {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("channel_id invalid"))
	}
	return unavailable(ctx)
}

// unavailable reports a context end as such and every other infrastructure
// failure as CodeUnavailable.
func unavailable(ctx context.Context) error {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return connect.NewError(connect.CodeCanceled, context.Canceled)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded)
	default:
		return connect.NewError(connect.CodeUnavailable, errors.New("channel facts unavailable"))
	}
}

// rollback ends the read transaction. The handler never commits, so a rollback
// failure after the response is decided has no durable effect to reconcile;
// pgx.ErrTxClosed is the normal result when the server already ended it. The
// result is therefore deliberately dropped.
func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(context.WithoutCancel(ctx))
}
