package channelrpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/aclstore"
	channelv1 "github.com/monkeylabx/threadline/services/gen/threadline/channel/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/channel/v1/channelv1connect"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	rpcSecondChannelID = "channel-channelrpc-second-synthetic"
	rpcDeniedChannelID = "channel-channelrpc-denied-synthetic"
	rpcOtherSpaceID    = "space-channelrpc-other-synthetic"
)

func TestListChannelsReturnsReadableMembershipsInOrder(t *testing.T) {
	ctx, pool, databaseName := openRPCDatabase(t)
	createRPCFixtures(t, ctx, pool)
	addListFixtures(t, ctx, pool)
	handler, err := New(pool)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"member-token":   memberSession(rpcTenantID, rpcMemberActorID),
		"outsider-token": memberSession(rpcTenantID, rpcOutsiderID),
		"beta-token":     memberSession(rpcBetaTenantID, rpcBetaActorID),
	}}
	mux := http.NewServeMux()
	mux.Handle(channelv1connect.NewChannelServiceHandler(handler, connect.WithInterceptors(rpcmiddleware.NewAuthenticationInterceptor(verifier))))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	client := channelv1connect.NewChannelServiceClient(httpServer.Client(), httpServer.URL)

	all, err := client.ListChannels(ctx, listRequest("member-token", &channelv1.ListChannelsRequest{}))
	if err != nil {
		t.Fatalf("ListChannels(member) error = %v", err)
	}
	assertChannelIDs(t, all.Msg, []string{rpcSecondChannelID, rpcChannelID}, "")
	if all.Msg.GetChannels()[1].GetName() != "Channel RPC Synthetic" || all.Msg.GetChannels()[1].GetE2EeGroupId() != rpcGroupID {
		t.Fatalf("listed channel = %v, want the same mapping as GetChannel", all.Msg.GetChannels()[1])
	}

	paged, err := client.ListChannels(ctx, listRequest("member-token", &channelv1.ListChannelsRequest{Page: &typev1.PageRequest{PageSize: 1}}))
	if err != nil {
		t.Fatalf("ListChannels(page 1) error = %v", err)
	}
	assertChannelIDs(t, paged.Msg, []string{rpcSecondChannelID}, rpcSecondChannelID)
	next, err := client.ListChannels(ctx, listRequest("member-token", &channelv1.ListChannelsRequest{Page: &typev1.PageRequest{PageSize: 1, PageToken: paged.Msg.GetPage().GetNextPageToken()}}))
	if err != nil {
		t.Fatalf("ListChannels(page 2) error = %v", err)
	}
	assertChannelIDs(t, next.Msg, []string{rpcChannelID}, rpcChannelID)
	last, err := client.ListChannels(ctx, listRequest("member-token", &channelv1.ListChannelsRequest{Page: &typev1.PageRequest{PageSize: 1, PageToken: next.Msg.GetPage().GetNextPageToken()}}))
	if err != nil {
		t.Fatalf("ListChannels(page 3) error = %v", err)
	}
	assertChannelIDs(t, last.Msg, nil, "")

	scoped, err := client.ListChannels(ctx, listRequest("member-token", &channelv1.ListChannelsRequest{SpaceId: rpcOtherSpaceID}))
	if err != nil {
		t.Fatalf("ListChannels(space) error = %v", err)
	}
	assertChannelIDs(t, scoped.Msg, []string{rpcSecondChannelID}, "")

	for name, credential := range map[string]string{"tenant member without memberships": "outsider-token", "other tenant": "beta-token"} {
		empty, err := client.ListChannels(ctx, listRequest(credential, &channelv1.ListChannelsRequest{}))
		if err != nil || len(empty.Msg.GetChannels()) != 0 {
			t.Fatalf("ListChannels(%s) = %v %v, want an empty list", name, empty, err)
		}
	}
	if _, err := client.ListChannels(ctx, connect.NewRequest(&channelv1.ListChannelsRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("ListChannels() without credential = %v, want CodeUnauthenticated", err)
	}
	assertNoOpenTransactions(t, ctx, pool, databaseName)
}

func listRequest(credential string, body *channelv1.ListChannelsRequest) *connect.Request[channelv1.ListChannelsRequest] {
	request := connect.NewRequest(body)
	request.Header().Set("Authorization", "Bearer "+credential)
	return request
}

func assertChannelIDs(t *testing.T, response *channelv1.ListChannelsResponse, want []string, token string) {
	t.Helper()
	if len(response.GetChannels()) != len(want) || response.GetPage().GetNextPageToken() != token {
		t.Fatalf("page = %d channels token %q, want %v token %q", len(response.GetChannels()), response.GetPage().GetNextPageToken(), want, token)
	}
	for index, channel := range response.GetChannels() {
		if channel.GetChannelId() != want[index] {
			t.Fatalf("page[%d] = %s, want %s", index, channel.GetChannelId(), want[index])
		}
	}
}

// addListFixtures gives the member a second readable Channel in another Space
// and a third membership whose channel.read the ACL denies, so the list must
// return two Channels in channel_id order and omit the denied one.
func addListFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("begin list fixture transaction failed")
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable) VALUES ($1, $2, 'Other Space Synthetic', TRUE)`,
			[]any{rpcTenantID, rpcOtherSpaceID}},
		{`INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id) VALUES
		  ($1, $3, $2, 'Second Channel Synthetic', 2, 1, 'group-channelrpc-second-synthetic'),
		  ($1, $4, $2, 'Denied Channel Synthetic', 2, 1, 'group-channelrpc-denied-synthetic')`,
			[]any{rpcTenantID, rpcOtherSpaceID, rpcSecondChannelID, rpcDeniedChannelID}},
		{`INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role) VALUES ($1, $2, 1, $4, 3), ($1, $3, 1, $4, 3)`,
			[]any{rpcTenantID, rpcSecondChannelID, rpcDeniedChannelID, rpcMemberActorID}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("create list fixtures failed: %v", err)
		}
	}
	for channelID, effect := range map[string]authorization.ACLEffect{rpcSecondChannelID: authorization.ACLEffectAllow, rpcDeniedChannelID: authorization.ACLEffectDeny} {
		_, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
			Resource:      authorization.ResourceRef{TenantID: rpcTenantID, Kind: authorization.ResourceKindChannel, ID: channelID},
			DefaultEffect: authorization.ACLEffectDeny,
			Entries: []authorization.ACLEntry{{
				Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: rpcMemberActorID}, Action: authorization.ActionChannelRead, Effect: effect,
			}},
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("seed ACL for %s failed: %v", channelID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("commit list fixtures failed")
	}
}
