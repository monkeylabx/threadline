package messagerpc

import (
	"context"
	"errors"
	"math"
	"strconv"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/current"
	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	defaultPageSize = int32(50)
	maxPageSize     = int32(200)
)

// listQuery is the validated, bounded form of one ListEvents request.
type listQuery struct {
	channelID   string
	newestFirst bool
	afterSeq    int64
	beforeSeq   int64
	limit       int32
}

// ListEvents returns one contiguous page of a Channel's stored events with
// their ServerCommit re-attached. The tenant comes from the principal; read
// authorization and the page read share one read-committed transaction.
func (handler *Handler) ListEvents(
	ctx context.Context,
	request *connect.Request[messagev1.ListEventsRequest],
) (*connect.Response[messagev1.ListEventsResponse], error) {
	principal, authenticated := rpcmiddleware.PrincipalFromContext(ctx)
	if !authenticated {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	query, err := validateList(request.Msg)
	if err != nil {
		return nil, err
	}
	tx, err := handler.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, unavailable(ctx, "read_unavailable")
	}
	defer rollback(ctx, tx)

	if err := authorizeRead(ctx, tx, principal.TenantID(), query.channelID); err != nil {
		return nil, err
	}
	rows, err := readPage(ctx, dbgen.New(tx), principal.TenantID(), query)
	if err != nil {
		return nil, unavailable(ctx, "read_unavailable")
	}
	events, err := decodeContiguous(rows, query.newestFirst)
	if err != nil {
		return nil, unavailable(ctx, "stored_envelope_undecodable")
	}
	return connect.NewResponse(&messagev1.ListEventsResponse{
		Events: events,
		Page:   &typev1.PageResponse{NextPageToken: nextPageToken(events, query.limit)},
	}), nil
}

// validateList applies the Slice A gates (Channels only, no Thread filter),
// resolves the order, the page token and the half-open bound, and clamps the
// page size to 1..200 with 50 as the default.
func validateList(request *messagev1.ListEventsRequest) (listQuery, error) {
	if request.GetDmId() != "" {
		return listQuery{}, sliceGate("dm_read_not_enabled")
	}
	if request.GetThreadId() != "" {
		return listQuery{}, sliceGate("thread_filter_not_enabled")
	}
	if !validIdentifier(request.GetChannelId()) {
		return listQuery{}, rpcError(connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_UNSPECIFIED, "channel_id_invalid")
	}
	pageSize := request.GetPage().GetPageSize()
	if pageSize < 0 {
		return listQuery{}, rpcError(connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_UNSPECIFIED, "page_size_invalid")
	}
	query := listQuery{
		channelID:   request.GetChannelId(),
		newestFirst: request.GetOrder() == messagev1.EventOrder_EVENT_ORDER_NEWEST_FIRST,
		afterSeq:    clampSequence(request.GetAfterSeq()),
		beforeSeq:   clampSequence(request.GetBeforeSeq()),
		limit:       min(max(pageSize, 0), maxPageSize),
	}
	if query.limit == 0 {
		query.limit = defaultPageSize
	}
	if query.newestFirst && request.GetBeforeSeq() == 0 {
		query.beforeSeq = math.MaxInt64
	}
	return applyPageToken(query, request.GetPage().GetPageToken())
}

// applyPageToken continues from the sequence the previous page ended on. The
// token is the decimal channel_seq that page returned; anything else is a
// cursor the server never issued.
func applyPageToken(query listQuery, token string) (listQuery, error) {
	if token == "" {
		return query, nil
	}
	sequence, err := strconv.ParseInt(token, 10, 64)
	if err != nil || sequence < 1 || strconv.FormatInt(sequence, 10) != token {
		return listQuery{}, rpcError(connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CURSOR_INVALID, "page_token_invalid")
	}
	if query.newestFirst {
		query.beforeSeq = sequence
	} else {
		query.afterSeq = sequence
	}
	return query, nil
}

func clampSequence(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

// authorizeRead evaluates channel.read on current facts. Denied and absent
// Channels share one answer so a non-member learns nothing about existence.
func authorizeRead(ctx context.Context, tx pgx.Tx, tenantID, channelID string) error {
	ref := authorization.ResourceRef{TenantID: tenantID, Kind: authorization.ResourceKindChannel, ID: channelID}
	decision, err := current.EvaluateCurrent(ctx, tx, authorization.ActionChannelRead, ref)
	if err != nil {
		var evaluation *current.Error
		if errors.As(err, &evaluation) && evaluation.Code() == current.ErrorInvalidInput {
			return rpcError(connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_UNSPECIFIED, "channel_id_invalid")
		}
		return unavailable(ctx, "authorization_facts_unavailable")
	}
	if decision.Effect != authorization.EffectAllow {
		return rpcError(connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "read_unavailable")
	}
	return nil
}

func readPage(ctx context.Context, queries *dbgen.Queries, tenantID string, query listQuery) ([]dbgen.DomainChannelEvent, error) {
	if query.newestFirst {
		return queries.ListChannelEventsNewestFirst(ctx, dbgen.ListChannelEventsNewestFirstParams{
			TenantID: tenantID, ChannelID: &query.channelID, BeforeSeq: query.beforeSeq, PageLimit: query.limit,
		})
	}
	return queries.ListChannelEventsOldestFirst(ctx, dbgen.ListChannelEventsOldestFirstParams{
		TenantID: tenantID, ChannelID: &query.channelID, AfterSeq: query.afterSeq, PageLimit: query.limit,
	})
}

// decodeContiguous re-attaches each row's ServerCommit and truncates the page
// at the first hole in the sequence rather than skipping it, so applying the
// page in order never leaves a gap behind. Holes cannot occur under sequence
// head allocation; the check guards the invariant instead of assuming it.
func decodeContiguous(rows []dbgen.DomainChannelEvent, newestFirst bool) ([]*messagev1.ChannelEventEnvelope, error) {
	step := int64(1)
	if newestFirst {
		step = -1
	}
	events := make([]*messagev1.ChannelEventEnvelope, 0, len(rows))
	for index, row := range rows {
		if index > 0 && row.ChannelSeq != rows[index-1].ChannelSeq+step {
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
	}
	return events, nil
}

// nextPageToken names the sequence a full page ended on; a short page ends the
// result set.
func nextPageToken(events []*messagev1.ChannelEventEnvelope, limit int32) string {
	if len(events) == 0 || int32(len(events)) < limit {
		return ""
	}
	return strconv.FormatUint(events[len(events)-1].GetServerCommit().GetChannelSeq(), 10)
}
