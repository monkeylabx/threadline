package syncrpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	defaultMaxEvents = int32(200)
	maxMaxEvents     = int32(1_000)
)

// SyncEvents returns, for each requested Channel cursor, the contiguous run of
// events committed after the cursor, bounded by max_events across the whole
// response. Each batch starts where the cursor ended and stops at the first
// hole; invalid or unreadable cursors are rejected individually.
func (handler *Handler) SyncEvents(
	ctx context.Context,
	request *connect.Request[syncv1.SyncEventsRequest],
) (*connect.Response[syncv1.SyncEventsResponse], error) {
	principal, authenticated := rpcmiddleware.PrincipalFromContext(ctx)
	if !authenticated {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if len(request.Msg.GetCursors()) == 0 || len(request.Msg.GetCursors()) > maxCursorsPerRequest {
		return nil, rpcError(connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CURSOR_INVALID, "cursors_required")
	}
	inputs, rejections := validateCursors(request.Msg.GetCursors())
	budget := clampMaxEvents(request.Msg.GetMaxEvents())
	tx, err := handler.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, unavailable(ctx, "events_unavailable")
	}
	defer rollback(ctx, tx)

	batches := make([]*syncv1.EventBatch, 0, len(inputs))
	for _, input := range inputs {
		if budget <= 0 {
			break
		}
		batch, rejection, err := syncOne(ctx, tx, principal.TenantID(), input, budget)
		if err != nil {
			return nil, err
		}
		if rejection != nil {
			rejections = append(rejections, rejection)
			continue
		}
		if batch != nil {
			batches = append(batches, batch)
			budget -= int32(len(batch.GetEvents()))
		}
	}
	return connect.NewResponse(&syncv1.SyncEventsResponse{Batches: batches, CursorRejections: rejections}), nil
}

// clampMaxEvents applies the server default and ceiling; zero never means
// unbounded.
func clampMaxEvents(requested uint32) int32 {
	switch {
	case requested == 0:
		return defaultMaxEvents
	case requested > uint32(maxMaxEvents):
		return maxMaxEvents
	default:
		return int32(requested)
	}
}

// syncOne authorizes the read and fetches one bounded contiguous batch. A nil
// batch with nil rejection means the Channel has nothing after the cursor.
func syncOne(ctx context.Context, tx pgx.Tx, tenantID string, input cursorInput, budget int32) (*syncv1.EventBatch, *syncv1.CursorRejection, error) {
	if err := authorizeRead(ctx, tx, tenantID, input.channelID); err != nil {
		var denied *readDenied
		if errors.As(err, &denied) {
			return nil, &syncv1.CursorRejection{Cursor: input.original, Error: cursorInvalid("read_unavailable")}, nil
		}
		return nil, nil, err
	}
	rows, err := dbgen.New(tx).ListChannelEventsOldestFirst(ctx, dbgen.ListChannelEventsOldestFirstParams{
		TenantID: tenantID, ChannelID: &input.channelID, AfterSeq: input.position, PageLimit: budget + 1,
	})
	if err != nil {
		return nil, nil, unavailable(ctx, "events_unavailable")
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	events, err := decodeContiguous(rows, input.position, int(budget))
	if err != nil {
		return nil, nil, unavailable(ctx, "stored_envelope_undecodable")
	}
	if len(events) == 0 {
		return nil, nil, nil
	}
	last := events[len(events)-1].GetServerCommit().GetChannelSeq()
	return &syncv1.EventBatch{
		Events:  events,
		NextSeq: last,
		HasMore: len(rows) > len(events) && rows[len(events)].ChannelSeq == int64(last)+1,
	}, nil, nil
}

// decodeContiguous re-attaches each row's ServerCommit and stops at the first
// hole after the cursor or at the budget, so applying the batch in order never
// leaves a gap behind. The first row must be cursor+1 or the batch is empty.
func decodeContiguous(rows []dbgen.DomainChannelEvent, after int64, budget int) ([]*messagev1.ChannelEventEnvelope, error) {
	events := make([]*messagev1.ChannelEventEnvelope, 0, min(len(rows), budget))
	expected := after + 1
	for _, row := range rows {
		if len(events) >= budget || row.ChannelSeq != expected {
			break
		}
		envelope := &messagev1.ChannelEventEnvelope{}
		if err := proto.Unmarshal(row.Envelope, envelope); err != nil || !row.CommittedAt.Valid {
			return nil, errors.New("stored envelope undecodable")
		}
		envelope.ServerCommit = &messagev1.ServerCommit{
			ChannelSeq: uint64(row.ChannelSeq), CommittedAt: timestamppb.New(row.CommittedAt.Time),
		}
		events = append(events, envelope)
		expected++
	}
	return events, nil
}
