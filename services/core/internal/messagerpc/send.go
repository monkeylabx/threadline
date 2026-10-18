package messagerpc

import (
	"bytes"
	"context"
	"errors"
	"math"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/current"
	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	"github.com/monkeylabx/threadline/services/core/internal/outboxstore"
	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/message/v1/messagev1connect"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	conversationKindChannel = int16(1)
	categoryApplication     = int16(1)
	uniqueViolation         = "23505"
	sendAttempts            = 2
)

// TransactionStarter opens the read-committed transaction one send runs in.
// *pgxpool.Pool satisfies it.
type TransactionStarter interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// Handler serves MessageService. Procedures not implemented here answer
// CodeUnimplemented from the embedded generated type.
type Handler struct {
	messagev1connect.UnimplementedMessageServiceHandler
	db       TransactionStarter
	producer *outboxstore.Producer
}

// New builds the handler. A nil producer means the message producer is not
// ready (no valid trusted policy); SendEvent then fails closed before any
// transaction while the rest of the process keeps serving.
func New(db TransactionStarter, producer *outboxstore.Producer) (*Handler, error) {
	if db == nil {
		return nil, errors.New("messagerpc: transaction starter required")
	}
	return &Handler{db: db, producer: producer}, nil
}

// SendEvent commits one human APPLICATION event or returns the original
// receipt for an exact retry. A unique-constraint race with a concurrent
// identical send re-runs the transaction once so the loser observes the winner.
func (handler *Handler) SendEvent(
	ctx context.Context,
	request *connect.Request[messagev1.SendEventRequest],
) (*connect.Response[messagev1.SendEventResponse], error) {
	principal, authenticated := rpcmiddleware.PrincipalFromContext(ctx)
	if !authenticated {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	command, err := validateSend(principal, request.Msg.GetEnvelope())
	if err != nil {
		return nil, err
	}
	if handler.producer == nil {
		return nil, rpcError(connect.CodeUnavailable, typev1.ErrorCode_ERROR_CODE_NOT_DURABLE, "message_producer_not_ready")
	}
	for range sendAttempts {
		response, err := handler.sendOnce(ctx, command)
		if errors.Is(err, errRace) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(response), nil
	}
	return nil, unavailable(ctx, "commit_not_durable")
}

// sendOnce runs one complete transaction: authorize, observe the binding,
// allocate the sequence, append event + binding + Domain Event, commit.
func (handler *Handler) sendOnce(ctx context.Context, command sendCommand) (*messagev1.SendEventResponse, error) {
	tx, err := handler.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, unavailable(ctx, "commit_not_durable")
	}
	defer rollback(ctx, tx)

	if err := authorizePublish(ctx, tx, command); err != nil {
		return nil, err
	}
	queries := dbgen.New(tx)
	if response, found, err := observeBinding(ctx, queries, command); found || err != nil {
		return response, err
	}
	sequence, err := allocateSequence(ctx, queries, command)
	if err != nil {
		return nil, err
	}
	committedAt, err := appendEvent(ctx, tx, handler.producer, command, sequence)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, unavailable(ctx, "commit_not_durable")
	}
	return receipt(command, sequence, committedAt, false), nil
}

// authorizePublish evaluates channel.publish on current facts under the
// resolver's locks. Denied and absent Channels share one answer (D12).
func authorizePublish(ctx context.Context, tx pgx.Tx, command sendCommand) error {
	ref := authorization.ResourceRef{
		TenantID: command.tenantID, Kind: authorization.ResourceKindChannel, ID: command.channelID,
	}
	decision, err := current.EvaluateCurrent(ctx, tx, authorization.ActionChannelPublish, ref)
	if err != nil {
		var evaluation *current.Error
		if errors.As(err, &evaluation) && evaluation.Code() == current.ErrorInvalidInput {
			return invalidEnvelope("envelope_identifier_invalid")
		}
		return unavailable(ctx, "authorization_facts_unavailable")
	}
	if decision.Effect != authorization.EffectAllow {
		return rpcError(connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "publish_unavailable")
	}
	return nil
}

// observeBinding returns the original receipt for an exact retry, a conflict
// for a reused key or event id, or found=false when the command is new.
func observeBinding(ctx context.Context, queries *dbgen.Queries, command sendCommand) (*messagev1.SendEventResponse, bool, error) {
	binding, err := queries.GetSendCommandBinding(ctx, dbgen.GetSendCommandBindingParams{
		TenantID: command.tenantID, SenderDeviceID: command.deviceID, IdempotencyKey: command.idempotencyKey,
	})
	if err == nil {
		if !exactBinding(binding, command) {
			return nil, true, conflict("idempotency_key_conflict")
		}
		return receipt(command, binding.ChannelSeq, binding.CommittedAt.Time, true), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, true, unavailable(ctx, "binding_unavailable")
	}
	_, err = queries.GetChannelEvent(ctx, dbgen.GetChannelEventParams{TenantID: command.tenantID, EventID: command.eventID})
	if err == nil {
		return nil, true, conflict("event_id_conflict")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, true, unavailable(ctx, "binding_unavailable")
	}
	return nil, false, nil
}

func exactBinding(binding dbgen.GetSendCommandBindingRow, command sendCommand) bool {
	return binding.EventID == command.eventID &&
		bytes.Equal(binding.EnvelopeDigest, command.digest[:]) &&
		binding.ChannelID != nil && *binding.ChannelID == command.channelID &&
		binding.CommittedAt.Valid
}

// allocateSequence locks (or creates) the Channel's sequence head and advances
// it by exactly one inside the transaction, so a rollback leaves no gap.
func allocateSequence(ctx context.Context, queries *dbgen.Queries, command sendCommand) (int64, error) {
	key := dbgen.LockConversationSequenceHeadParams{
		TenantID: command.tenantID, ConversationKind: conversationKindChannel, ConversationID: command.channelID,
	}
	head, err := queries.LockConversationSequenceHead(ctx, key)
	lastSeq := head.LastSeq
	if errors.Is(err, pgx.ErrNoRows) {
		inserted, insertErr := queries.InsertConversationSequenceHead(ctx, dbgen.InsertConversationSequenceHeadParams(key))
		if insertErr != nil {
			return 0, mapWriteError(ctx, insertErr)
		}
		lastSeq = inserted.LastSeq
	} else if err != nil {
		return 0, unavailable(ctx, "sequence_head_unavailable")
	}
	if lastSeq >= math.MaxInt64 {
		return 0, rpcError(connect.CodeResourceExhausted, typev1.ErrorCode_ERROR_CODE_UNSPECIFIED, "channel_seq_exhausted")
	}
	sequence, err := queries.AdvanceConversationSequenceHead(ctx, dbgen.AdvanceConversationSequenceHeadParams{
		TenantID: key.TenantID, ConversationKind: key.ConversationKind, ConversationID: key.ConversationID, ExpectedLastSeq: lastSeq,
	})
	if err != nil {
		return 0, unavailable(ctx, "sequence_head_unavailable")
	}
	return sequence, nil
}

// appendEvent stores the opaque envelope, binds the command to it and appends
// the message.committed Domain Event, all inside tx.
func appendEvent(ctx context.Context, tx pgx.Tx, producer *outboxstore.Producer, command sendCommand, sequence int64) (time.Time, error) {
	queries := dbgen.New(tx)
	channelID := command.channelID
	inserted, err := queries.InsertChannelEvent(ctx, dbgen.InsertChannelEventParams{
		TenantID: command.tenantID, EventID: command.eventID, ChannelID: &channelID, ChannelSeq: sequence,
		SenderActorType: int16(rpcmiddleware.ActorTypeHuman), SenderActorID: command.actorID,
		SenderDeviceID: command.deviceID, IdempotencyKey: command.idempotencyKey,
		Category: categoryApplication, Envelope: command.envelope, ContentHash: command.contentHash,
	})
	if err != nil {
		return time.Time{}, mapWriteError(ctx, err)
	}
	if _, err := queries.InsertSendCommand(ctx, dbgen.InsertSendCommandParams{
		TenantID: command.tenantID, SenderDeviceID: command.deviceID, IdempotencyKey: command.idempotencyKey,
		EventID: command.eventID, EnvelopeDigest: command.digest[:],
	}); err != nil {
		return time.Time{}, mapWriteError(ctx, err)
	}
	if _, err := producer.AppendChannelEventCommitted(ctx, tx, outboxstore.ChannelEventCommitted{
		TenantID: command.tenantID, ChannelID: command.channelID, ChannelSeq: uint64(sequence), CommittedAt: inserted.CommittedAt.Time,
	}); err != nil {
		return time.Time{}, unavailable(ctx, "domain_event_unavailable")
	}
	return inserted.CommittedAt.Time, nil
}

// mapWriteError turns a unique violation into a race (same key or first head:
// re-run to observe the winner) or a conflict (event id already bound), and
// everything else into a retryable unavailability.
func mapWriteError(ctx context.Context, err error) error {
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != uniqueViolation {
		return unavailable(ctx, "commit_not_durable")
	}
	switch pgError.ConstraintName {
	case "send_commands_pkey", "conversation_sequence_heads_pkey":
		return errRace
	default:
		return conflict("event_id_conflict")
	}
}

func receipt(command sendCommand, sequence int64, committedAt time.Time, deduplicated bool) *messagev1.SendEventResponse {
	return &messagev1.SendEventResponse{
		ServerCommit:   &messagev1.ServerCommit{ChannelSeq: uint64(sequence), CommittedAt: timestamppb.New(committedAt)},
		EventId:        command.eventID,
		Deduplicated:   deduplicated,
		IdempotencyKey: command.idempotencyKey,
	}
}

// rollback ends the transaction. After a commit it reports ErrTxClosed, and
// after a decided failure there is no durable effect to reconcile, so the
// result is deliberately dropped.
func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(context.WithoutCancel(ctx))
}
