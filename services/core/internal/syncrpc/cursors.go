package syncrpc

import (
	"context"
	"errors"
	"math"
	"strings"
	"unicode"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/current"
	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/sync/v1/syncv1connect"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	conversationKindChannel = int16(1)
	maxIdentifierBytes      = 128
	maxCursorsPerRequest    = 256
)

// TransactionStarter opens the read-committed transaction a request runs in.
// *pgxpool.Pool satisfies it.
type TransactionStarter interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// Handler serves SyncService. Procedures not implemented here answer
// CodeUnimplemented from the embedded generated type.
type Handler struct {
	syncv1connect.UnimplementedSyncServiceHandler
	db TransactionStarter
}

// New builds the handler over db.
func New(db TransactionStarter) (*Handler, error) {
	if db == nil {
		return nil, errors.New("syncrpc: transaction starter required")
	}
	return &Handler{db: db}, nil
}

// cursorInput is one validated requested cursor.
type cursorInput struct {
	channelID string
	position  int64
	original  *syncv1.SyncCursor
}

// GetCursors lists the authenticated Device's cursors, optionally restricted
// to the given Channels. Cursors are the Device's own state, so no further
// authorization applies.
func (handler *Handler) GetCursors(
	ctx context.Context,
	request *connect.Request[syncv1.GetCursorsRequest],
) (*connect.Response[syncv1.GetCursorsResponse], error) {
	principal, authenticated := rpcmiddleware.PrincipalFromContext(ctx)
	if !authenticated {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if len(request.Msg.GetDmIds()) > 0 {
		return nil, rpcError(connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "dm_sync_not_enabled")
	}
	tx, err := handler.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, unavailable(ctx, "cursors_unavailable")
	}
	defer rollback(ctx, tx)
	rows, err := dbgen.New(tx).ListDeviceSyncCursors(ctx, dbgen.ListDeviceSyncCursorsParams{
		TenantID: principal.TenantID(), DeviceID: principal.DeviceID(),
	})
	if err != nil {
		return nil, unavailable(ctx, "cursors_unavailable")
	}
	return connect.NewResponse(&syncv1.GetCursorsResponse{Cursors: selectCursors(rows, request.Msg.GetChannelIds())}), nil
}

// selectCursors maps Channel cursors to the wire shape, keeping only the
// requested Channels when a filter is given.
func selectCursors(rows []dbgen.DomainSyncCursor, channelIDs []string) []*syncv1.SyncCursor {
	wanted := make(map[string]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		wanted[channelID] = struct{}{}
	}
	cursors := make([]*syncv1.SyncCursor, 0, len(rows))
	for _, row := range rows {
		if row.ConversationKind != conversationKindChannel {
			continue
		}
		if _, keep := wanted[row.ConversationID]; len(wanted) > 0 && !keep {
			continue
		}
		cursors = append(cursors, toProto(row))
	}
	return cursors
}

func toProto(row dbgen.DomainSyncCursor) *syncv1.SyncCursor {
	cursor := &syncv1.SyncCursor{
		Conversation:   &syncv1.SyncCursor_ChannelId{ChannelId: row.ConversationID},
		LastAppliedSeq: uint64(row.LastAppliedSeq),
	}
	if row.UpdatedAt.Valid {
		cursor.UpdatedAt = timestamppb.New(row.UpdatedAt.Time)
	}
	return cursor
}

// UpdateCursors persists every acceptable cursor in one transaction and
// reports each rejected one with the safe server position. Accepted cursors
// commit together; a rejection never hides progress on another Channel.
func (handler *Handler) UpdateCursors(
	ctx context.Context,
	request *connect.Request[syncv1.UpdateCursorsRequest],
) (*connect.Response[syncv1.UpdateCursorsResponse], error) {
	principal, authenticated := rpcmiddleware.PrincipalFromContext(ctx)
	if !authenticated {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if len(request.Msg.GetCursors()) == 0 || len(request.Msg.GetCursors()) > maxCursorsPerRequest {
		return nil, rpcError(connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CURSOR_INVALID, "cursor_count_invalid")
	}
	inputs, rejections := validateCursors(request.Msg.GetCursors())
	tx, err := handler.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, unavailable(ctx, "cursors_unavailable")
	}
	defer rollback(ctx, tx)
	for _, input := range inputs {
		rejection, err := applyCursor(ctx, tx, principal, input)
		if err != nil {
			return nil, err
		}
		if rejection != nil {
			rejections = append(rejections, rejection)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, unavailable(ctx, "cursors_not_durable")
	}
	return connect.NewResponse(response(rejections)), nil
}

// validateCursors separates well-formed Channel cursors from per-cursor
// rejections: DM cursors, invalid identifiers and duplicate Channels.
func validateCursors(cursors []*syncv1.SyncCursor) ([]cursorInput, []*syncv1.CursorRejection) {
	inputs := make([]cursorInput, 0, len(cursors))
	rejections := make([]*syncv1.CursorRejection, 0)
	seen := make(map[string]struct{}, len(cursors))
	for _, cursor := range cursors {
		reason := cursorShapeReason(cursor, seen)
		if reason != "" {
			rejections = append(rejections, &syncv1.CursorRejection{Cursor: cursor, Error: cursorInvalid(reason)})
			continue
		}
		seen[cursor.GetChannelId()] = struct{}{}
		inputs = append(inputs, cursorInput{
			channelID: cursor.GetChannelId(), position: clampPosition(cursor.GetLastAppliedSeq()), original: cursor,
		})
	}
	return inputs, rejections
}

func cursorShapeReason(cursor *syncv1.SyncCursor, seen map[string]struct{}) string {
	switch {
	case cursor.GetDmId() != "":
		return "dm_sync_not_enabled"
	case !validIdentifier(cursor.GetChannelId()):
		return "cursor_conversation_invalid"
	case cursor.GetLastAppliedSeq() > math.MaxInt64:
		return "cursor_ahead_of_history"
	default:
		if _, duplicate := seen[cursor.GetChannelId()]; duplicate {
			return "cursor_duplicate"
		}
		return ""
	}
}

// applyCursor authorizes the Channel read, bounds the position by committed
// history and inserts or advances the Device cursor. It returns a rejection
// for a policy outcome and an error only for infrastructure failure.
func applyCursor(ctx context.Context, tx pgx.Tx, principal rpcmiddleware.Principal, input cursorInput) (*syncv1.CursorRejection, error) {
	if err := authorizeRead(ctx, tx, principal.TenantID(), input.channelID); err != nil {
		var denied *readDenied
		if errors.As(err, &denied) {
			return &syncv1.CursorRejection{Cursor: input.original, Error: cursorInvalid("read_unavailable")}, nil
		}
		return nil, err
	}
	queries := dbgen.New(tx)
	head, err := committedHead(ctx, queries, principal.TenantID(), input.channelID)
	if err != nil {
		return nil, unavailable(ctx, "history_unavailable")
	}
	if input.position > head {
		return &syncv1.CursorRejection{Cursor: input.original, Error: cursorInvalid("cursor_ahead_of_history")}, nil
	}
	return storeCursor(ctx, queries, principal, input)
}

// storeCursor locks the Device's existing cursor, refuses regression with the
// server position, and otherwise inserts or advances.
func storeCursor(ctx context.Context, queries *dbgen.Queries, principal rpcmiddleware.Principal, input cursorInput) (*syncv1.CursorRejection, error) {
	key := dbgen.GetSyncCursorForUpdateParams{
		TenantID: principal.TenantID(), DeviceID: principal.DeviceID(),
		ConversationKind: conversationKindChannel, ConversationID: input.channelID,
	}
	existing, err := queries.GetSyncCursorForUpdate(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		_, insertErr := queries.InsertSyncCursor(ctx, dbgen.InsertSyncCursorParams{
			TenantID: key.TenantID, DeviceID: key.DeviceID, ConversationKind: key.ConversationKind, ConversationID: key.ConversationID,
			ActorType: int16(principal.Actor().Type()), ActorID: principal.Actor().ID(), LastAppliedSeq: input.position,
		})
		if insertErr != nil {
			return nil, unavailable(ctx, "cursors_not_durable")
		}
		return nil, nil
	}
	if err != nil {
		return nil, unavailable(ctx, "cursors_unavailable")
	}
	if input.position < existing.LastAppliedSeq {
		return &syncv1.CursorRejection{
			Cursor: input.original, Error: cursorInvalid("cursor_regression"), ServerCursor: toProto(existing),
		}, nil
	}
	if _, err := queries.AdvanceSyncCursor(ctx, dbgen.AdvanceSyncCursorParams{
		LastAppliedSeq: input.position, TenantID: key.TenantID, DeviceID: key.DeviceID,
		ConversationKind: key.ConversationKind, ConversationID: key.ConversationID,
	}); err != nil {
		return nil, unavailable(ctx, "cursors_not_durable")
	}
	return nil, nil
}

// committedHead reads the Channel's last committed sequence without locking
// it; a Channel without events has head 0.
func committedHead(ctx context.Context, queries *dbgen.Queries, tenantID, channelID string) (int64, error) {
	head, err := queries.GetConversationSequenceHead(ctx, dbgen.GetConversationSequenceHeadParams{
		TenantID: tenantID, ConversationKind: conversationKindChannel, ConversationID: channelID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return head.LastSeq, nil
}

// readDenied marks a channel.read denial so the caller can turn it into a
// per-cursor rejection instead of failing the request.
type readDenied struct{}

func (*readDenied) Error() string { return "channel read denied" }

func authorizeRead(ctx context.Context, tx pgx.Tx, tenantID, channelID string) error {
	ref := authorization.ResourceRef{TenantID: tenantID, Kind: authorization.ResourceKindChannel, ID: channelID}
	decision, err := current.EvaluateCurrent(ctx, tx, authorization.ActionChannelRead, ref)
	if err != nil {
		var evaluation *current.Error
		if errors.As(err, &evaluation) && evaluation.Code() == current.ErrorInvalidInput {
			return &readDenied{}
		}
		return unavailable(ctx, "authorization_facts_unavailable")
	}
	if decision.Effect != authorization.EffectAllow {
		return &readDenied{}
	}
	return nil
}

func response(rejections []*syncv1.CursorRejection) *syncv1.UpdateCursorsResponse {
	rejected := make([]*syncv1.SyncCursor, 0, len(rejections))
	for _, rejection := range rejections {
		rejected = append(rejected, rejection.GetCursor())
	}
	return &syncv1.UpdateCursorsResponse{RejectedCursors: rejected, CursorRejections: rejections}
}

func clampPosition(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

func validIdentifier(value string) bool {
	return value != "" && len(value) <= maxIdentifierBytes && value == strings.TrimSpace(value) &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

// rollback ends the transaction; after Commit it reports ErrTxClosed and after
// a decided failure there is nothing durable to reconcile, so the result is
// deliberately dropped.
func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(context.WithoutCancel(ctx))
}
