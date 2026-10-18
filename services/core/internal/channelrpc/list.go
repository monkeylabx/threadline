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
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	defaultPageSize = int32(50)
	maxPageSize     = int32(200)
)

// listQuery is the validated, bounded form of one ListChannels request. The
// page token is the channel_id the previous page ended on.
type listQuery struct {
	spaceID        *string
	afterChannelID string
	limit          int32
}

// ListChannels returns the Channels the authenticated Member currently
// belongs to, in channel_id order, omitting any Channel whose channel.read is
// denied by current facts. Discoverable-but-unjoined Channels are not part of
// Slice A.
func (handler *Handler) ListChannels(
	ctx context.Context,
	request *connect.Request[channelv1.ListChannelsRequest],
) (*connect.Response[channelv1.ListChannelsResponse], error) {
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
		return nil, unavailable(ctx)
	}
	defer rollback(ctx, tx)

	rows, err := dbgen.New(tx).ListActorActiveChannels(ctx, dbgen.ListActorActiveChannelsParams{
		TenantID: principal.TenantID(), ActorType: int16(principal.Actor().Type()), ActorID: principal.Actor().ID(),
		SpaceID: query.spaceID, AfterChannelID: query.afterChannelID, PageLimit: query.limit,
	})
	if err != nil {
		return nil, unavailable(ctx)
	}
	channels, err := readableChannels(ctx, tx, principal.TenantID(), rows)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&channelv1.ListChannelsResponse{
		Channels: channels,
		Page:     &typev1.PageResponse{NextPageToken: nextPageToken(rows, query.limit)},
	}), nil
}

// validateList applies the Slice A gate, the page size clamp (default 50,
// max 200) and the page token, which must be a valid channel_id.
func validateList(request *channelv1.ListChannelsRequest) (listQuery, error) {
	if request.GetIncludeDiscoverable() {
		return listQuery{}, connect.NewError(connect.CodeFailedPrecondition, errors.New("discoverable_not_enabled"))
	}
	pageSize := request.GetPage().GetPageSize()
	if pageSize < 0 {
		return listQuery{}, connect.NewError(connect.CodeInvalidArgument, errors.New("page_size_invalid"))
	}
	query := listQuery{limit: min(max(pageSize, 0), maxPageSize), afterChannelID: request.GetPage().GetPageToken()}
	if query.limit == 0 {
		query.limit = defaultPageSize
	}
	if query.afterChannelID != "" && (len(query.afterChannelID) > maxChannelIDBytes) {
		return listQuery{}, connect.NewError(connect.CodeInvalidArgument, errors.New("page_token_invalid"))
	}
	if spaceID := request.GetSpaceId(); spaceID != "" {
		if len(spaceID) > maxChannelIDBytes {
			return listQuery{}, connect.NewError(connect.CodeInvalidArgument, errors.New("space_id_invalid"))
		}
		query.spaceID = &spaceID
	}
	return query, nil
}

// readableChannels keeps the rows whose channel.read is allowed on current
// facts inside the same transaction. A denied Channel is omitted rather than
// failing the page, so one ACL change never hides the rest of the list.
func readableChannels(ctx context.Context, tx pgx.Tx, tenantID string, rows []dbgen.DomainChannel) ([]*channelv1.Channel, error) {
	channels := make([]*channelv1.Channel, 0, len(rows))
	for _, row := range rows {
		ref := authorization.ResourceRef{TenantID: tenantID, Kind: authorization.ResourceKindChannel, ID: row.ChannelID}
		decision, err := current.EvaluateCurrent(ctx, tx, authorization.ActionChannelRead, ref)
		if err != nil {
			return nil, unavailable(ctx)
		}
		if decision.Effect == authorization.EffectAllow {
			channels = append(channels, toProto(row))
		}
	}
	return channels, nil
}

// nextPageToken names the last channel_id of a full page; a short page ends
// the listing. The token is computed from the fetched rows, not the readable
// subset, so a page of denied Channels still advances.
func nextPageToken(rows []dbgen.DomainChannel, limit int32) string {
	if len(rows) == 0 || int32(len(rows)) < limit {
		return ""
	}
	return rows[len(rows)-1].ChannelID
}
