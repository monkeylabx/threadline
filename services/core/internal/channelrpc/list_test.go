package channelrpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	channelv1 "github.com/monkeylabx/threadline/services/gen/threadline/channel/v1"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

func TestValidateListClampsPagesAndGatesDiscovery(t *testing.T) {
	t.Parallel()
	defaults, err := validateList(&channelv1.ListChannelsRequest{})
	if err != nil || defaults.limit != defaultPageSize || defaults.spaceID != nil || defaults.afterChannelID != "" {
		t.Fatalf("validateList(defaults) = %+v %v, want limit 50, no space, no token", defaults, err)
	}
	scoped, err := validateList(&channelv1.ListChannelsRequest{SpaceId: "space-synthetic", Page: &typev1.PageRequest{PageSize: 500, PageToken: "channel-b"}})
	if err != nil || scoped.limit != maxPageSize || scoped.spaceID == nil || *scoped.spaceID != "space-synthetic" || scoped.afterChannelID != "channel-b" {
		t.Fatalf("validateList(scoped) = %+v %v, want limit 200, space filter and token", scoped, err)
	}
	rejections := map[string]struct {
		request *channelv1.ListChannelsRequest
		code    connect.Code
	}{
		"discoverable":       {&channelv1.ListChannelsRequest{IncludeDiscoverable: true}, connect.CodeFailedPrecondition},
		"negative page size": {&channelv1.ListChannelsRequest{Page: &typev1.PageRequest{PageSize: -1}}, connect.CodeInvalidArgument},
		"oversized token":    {&channelv1.ListChannelsRequest{Page: &typev1.PageRequest{PageToken: strings.Repeat("c", maxChannelIDBytes+1)}}, connect.CodeInvalidArgument},
		"oversized space":    {&channelv1.ListChannelsRequest{SpaceId: strings.Repeat("s", maxChannelIDBytes+1)}, connect.CodeInvalidArgument},
	}
	for name, tc := range rejections {
		if _, err := validateList(tc.request); connect.CodeOf(err) != tc.code {
			t.Fatalf("validateList(%s) = %v, want %v", name, err, tc.code)
		}
	}
}

func TestNextPageTokenNamesTheLastFetchedChannel(t *testing.T) {
	t.Parallel()
	rows := []dbgen.DomainChannel{{ChannelID: "channel-a"}, {ChannelID: "channel-b"}}
	if nextPageToken(rows, 2) != "channel-b" || nextPageToken(rows, 3) != "" || nextPageToken(nil, 1) != "" {
		t.Fatal("nextPageToken must name the last channel of a full page only")
	}
}

func TestListChannelsFailsClosedBeforeTheDatabase(t *testing.T) {
	t.Parallel()
	starter := &stubStarter{err: errors.New("pool exhausted")}
	handler, err := New(starter)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = handler.ListChannels(context.Background(), connect.NewRequest(&channelv1.ListChannelsRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated || starter.calls.Load() != 0 {
		t.Fatalf("ListChannels() without principal = %v after %d BeginTx calls, want CodeUnauthenticated and 0", err, starter.calls.Load())
	}
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{"member-token": memberSession("tenant-synthetic", "actor-synthetic")}}
	unary := rpcmiddleware.NewAuthenticationInterceptor(verifier).WrapUnary(
		func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			typed, ok := request.(*connect.Request[channelv1.ListChannelsRequest])
			if !ok {
				t.Fatal("interceptor passed an unexpected request type")
			}
			return handler.ListChannels(ctx, typed)
		},
	)
	gated := connect.NewRequest(&channelv1.ListChannelsRequest{IncludeDiscoverable: true})
	gated.Header().Set("Authorization", "Bearer member-token")
	if _, err := unary(context.Background(), gated); connect.CodeOf(err) != connect.CodeFailedPrecondition || starter.calls.Load() != 0 {
		t.Fatalf("ListChannels(discoverable) = %v after %d BeginTx calls, want FailedPrecondition and 0", err, starter.calls.Load())
	}
	valid := connect.NewRequest(&channelv1.ListChannelsRequest{})
	valid.Header().Set("Authorization", "Bearer member-token")
	if _, err := unary(context.Background(), valid); connect.CodeOf(err) != connect.CodeUnavailable || starter.calls.Load() != 1 || strings.Contains(err.Error(), "pool exhausted") {
		t.Fatalf("ListChannels() with failing starter = %v after %d calls, want Unavailable, 1 call, no leaked text", err, starter.calls.Load())
	}
}
