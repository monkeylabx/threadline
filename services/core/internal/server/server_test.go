package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	channelv1 "github.com/monkeylabx/threadline/services/gen/threadline/channel/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/channel/v1/channelv1connect"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

type fakeVerifier struct {
	sessions map[string]rpcmiddleware.VerifiedSession
}

func (verifier fakeVerifier) VerifySession(_ context.Context, credential string) (rpcmiddleware.VerifiedSession, error) {
	session, found := verifier.sessions[credential]
	if !found {
		return rpcmiddleware.VerifiedSession{}, rpcmiddleware.NewVerificationError(rpcmiddleware.VerificationRejected)
	}
	return session, nil
}

// stubChannels echoes the requested ID and fails if the interceptor did not
// attach a principal, which is the composition property under test.
type stubChannels struct {
	channelv1connect.UnimplementedChannelServiceHandler
}

func (stubChannels) GetChannel(
	ctx context.Context,
	request *connect.Request[channelv1.GetChannelRequest],
) (*connect.Response[channelv1.GetChannelResponse], error) {
	if _, authenticated := rpcmiddleware.PrincipalFromContext(ctx); !authenticated {
		return nil, connect.NewError(connect.CodeInternal, errors.New("principal missing"))
	}
	return connect.NewResponse(&channelv1.GetChannelResponse{
		Channel: &channelv1.Channel{ChannelId: request.Msg.GetChannelId()},
	}), nil
}

func syntheticSessions() map[string]rpcmiddleware.VerifiedSession {
	return map[string]rpcmiddleware.VerifiedSession{"good-token": {
		TenantID: "tenant-synthetic", ActorType: rpcmiddleware.ActorTypeHuman,
		ActorID: "actor-synthetic", DeviceID: "device-synthetic", SessionID: "session-synthetic",
	}}
}

func TestNewHandlerRequiresEveryDependency(t *testing.T) {
	t.Parallel()
	complete := Dependencies{Pinger: fakePinger{}, Channels: stubChannels{}, Verifier: RejectAllVerifier{}}
	cases := map[string]func(Dependencies) Dependencies{
		"no pinger":   func(d Dependencies) Dependencies { d.Pinger = nil; return d },
		"no channels": func(d Dependencies) Dependencies { d.Channels = nil; return d },
		"no verifier": func(d Dependencies) Dependencies { d.Verifier = nil; return d },
	}
	for name, remove := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewHandler(remove(complete)); err == nil {
				t.Fatal("NewHandler() error = nil, want rejection")
			}
		})
	}
	if _, err := NewHandler(complete); err != nil {
		t.Fatalf("NewHandler(complete) error = %v", err)
	}
}

func TestHandlerComposesHealthAndAuthenticatedChannels(t *testing.T) {
	t.Parallel()
	handler, err := NewHandler(Dependencies{
		Pinger: fakePinger{}, Channels: stubChannels{}, Verifier: fakeVerifier{sessions: syntheticSessions()},
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	ctx := context.Background()

	health, err := httpServer.Client().Get(httpServer.URL + "/healthz")
	if err != nil || health.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz = %v %v, want 200", health, err)
	}
	_ = health.Body.Close()

	client := channelv1connect.NewChannelServiceClient(httpServer.Client(), httpServer.URL)
	_, err = client.GetChannel(ctx, connect.NewRequest(&channelv1.GetChannelRequest{ChannelId: "channel-synthetic"}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("GetChannel() without credential = %v, want CodeUnauthenticated", err)
	}

	authorized := connect.NewRequest(&channelv1.GetChannelRequest{ChannelId: "channel-synthetic"})
	authorized.Header().Set("Authorization", "Bearer good-token")
	response, err := client.GetChannel(ctx, authorized)
	if err != nil || response.Msg.GetChannel().GetChannelId() != "channel-synthetic" {
		t.Fatalf("GetChannel() with credential = %v %v, want echoed channel", response, err)
	}

	unimplemented := connect.NewRequest(&channelv1.ListChannelsRequest{})
	unimplemented.Header().Set("Authorization", "Bearer good-token")
	_, err = client.ListChannels(ctx, unimplemented)
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("ListChannels() = %v, want CodeUnimplemented", err)
	}
}

func TestRejectAllVerifierFailsClosedThroughTheInterceptor(t *testing.T) {
	t.Parallel()
	handler, err := NewHandler(Dependencies{Pinger: fakePinger{}, Channels: stubChannels{}, Verifier: RejectAllVerifier{}})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := channelv1connect.NewChannelServiceClient(httpServer.Client(), httpServer.URL)
	request := connect.NewRequest(&channelv1.GetChannelRequest{ChannelId: "channel-synthetic"})
	request.Header().Set("Authorization", "Bearer any-token")
	_, err = client.GetChannel(context.Background(), request)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("GetChannel() with RejectAllVerifier = %v, want CodeUnauthenticated", err)
	}
}

func TestServeListenerStopsWhenContextEnds(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("listen on ephemeral port failed")
	}
	handler, err := NewHandler(Dependencies{Pinger: fakePinger{}, Channels: stubChannels{}, Verifier: RejectAllVerifier{}})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- ServeListener(ctx, listener, handler) }()

	health, err := http.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil || health.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz while serving = %v %v, want 200", health, err)
	}
	_ = health.Body.Close()
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("ServeListener() after cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeListener() did not return after context cancellation")
	}
}
