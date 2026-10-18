package channelrpc

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	channelv1 "github.com/monkeylabx/threadline/services/gen/threadline/channel/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

type stubStarter struct {
	calls atomic.Int32
	err   error
}

func (starter *stubStarter) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	starter.calls.Add(1)
	return nil, starter.err
}

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

func memberSession(tenantID, actorID string) rpcmiddleware.VerifiedSession {
	return rpcmiddleware.VerifiedSession{
		TenantID: tenantID, ActorType: rpcmiddleware.ActorTypeHuman, ActorID: actorID,
		DeviceID: "device-" + actorID, SessionID: "session-" + actorID,
	}
}

// authenticatedGetChannel runs GetChannel behind the real interceptor so the
// principal reaches the handler the same way it does in the process.
func authenticatedGetChannel(
	t *testing.T,
	handler *Handler,
	verifier rpcmiddleware.SessionVerifier,
	credential string,
	channelID string,
) (*connect.Response[channelv1.GetChannelResponse], error) {
	t.Helper()
	unary := rpcmiddleware.NewAuthenticationInterceptor(verifier).WrapUnary(
		func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			typed, ok := request.(*connect.Request[channelv1.GetChannelRequest])
			if !ok {
				t.Fatal("interceptor passed an unexpected request type")
			}
			return handler.GetChannel(ctx, typed)
		},
	)
	request := connect.NewRequest(&channelv1.GetChannelRequest{ChannelId: channelID})
	request.Header().Set("Authorization", "Bearer "+credential)
	response, err := unary(context.Background(), request)
	if err != nil {
		return nil, err
	}
	typed, ok := response.(*connect.Response[channelv1.GetChannelResponse])
	if !ok {
		t.Fatal("interceptor returned an unexpected response type")
	}
	return typed, nil
}

func TestNewRequiresTransactionStarter(t *testing.T) {
	t.Parallel()
	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) error = nil, want rejection")
	}
}

func TestGetChannelRequiresPrincipal(t *testing.T) {
	t.Parallel()
	starter := &stubStarter{err: errors.New("must not be reached")}
	handler, err := New(starter)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = handler.GetChannel(context.Background(), connect.NewRequest(&channelv1.GetChannelRequest{ChannelId: "channel-synthetic"}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated || starter.calls.Load() != 0 {
		t.Fatalf("GetChannel() without principal = %v after %d BeginTx calls, want CodeUnauthenticated and 0", err, starter.calls.Load())
	}
}

func TestGetChannelValidatesChannelIDBeforeTouchingTheDatabase(t *testing.T) {
	t.Parallel()
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"member-token": memberSession("tenant-synthetic", "actor-synthetic"),
	}}
	for name, channelID := range map[string]string{"empty": "", "oversized": strings.Repeat("c", maxChannelIDBytes+1)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			starter := &stubStarter{err: errors.New("must not be reached")}
			handler, err := New(starter)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			_, err = authenticatedGetChannel(t, handler, verifier, "member-token", channelID)
			if connect.CodeOf(err) != connect.CodeInvalidArgument || starter.calls.Load() != 0 {
				t.Fatalf("GetChannel(%q) = %v after %d BeginTx calls, want CodeInvalidArgument and 0", channelID, err, starter.calls.Load())
			}
		})
	}
}

func TestGetChannelReportsTransactionStartFailureAsUnavailable(t *testing.T) {
	t.Parallel()
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"member-token": memberSession("tenant-synthetic", "actor-synthetic"),
	}}
	starter := &stubStarter{err: errors.New("pool exhausted")}
	handler, err := New(starter)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = authenticatedGetChannel(t, handler, verifier, "member-token", "channel-synthetic")
	if connect.CodeOf(err) != connect.CodeUnavailable || starter.calls.Load() != 1 {
		t.Fatalf("GetChannel() with failing starter = %v after %d calls, want CodeUnavailable and 1", err, starter.calls.Load())
	}
	if strings.Contains(err.Error(), "pool exhausted") {
		t.Fatal("GetChannel() leaked the infrastructure error text")
	}
}

func TestToProtoMapsStoredCodes(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, time.October, 18, 9, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		row        dbgen.DomainChannel
		visibility channelv1.ChannelVisibility
		state      channelv1.ChannelState
		created    bool
	}{
		"public active with timestamp": {
			row:        dbgen.DomainChannel{Visibility: 1, State: 1, CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true}},
			visibility: channelv1.ChannelVisibility_CHANNEL_VISIBILITY_PUBLIC, state: channelv1.ChannelState_CHANNEL_STATE_ACTIVE, created: true,
		},
		"private archived": {
			row:        dbgen.DomainChannel{Visibility: 2, State: 2},
			visibility: channelv1.ChannelVisibility_CHANNEL_VISIBILITY_PRIVATE, state: channelv1.ChannelState_CHANNEL_STATE_ARCHIVED,
		},
		"pending deletion": {
			row:        dbgen.DomainChannel{Visibility: 2, State: 3},
			visibility: channelv1.ChannelVisibility_CHANNEL_VISIBILITY_PRIVATE, state: channelv1.ChannelState_CHANNEL_STATE_PENDING_DELETION,
		},
		"unknown codes stay unspecified": {
			row:        dbgen.DomainChannel{Visibility: 9, State: 9},
			visibility: channelv1.ChannelVisibility_CHANNEL_VISIBILITY_UNSPECIFIED, state: channelv1.ChannelState_CHANNEL_STATE_UNSPECIFIED,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tc.row.ChannelID, tc.row.TenantID, tc.row.SpaceID, tc.row.Name, tc.row.E2eeGroupID =
				"channel-synthetic", "tenant-synthetic", "space-synthetic", "Synthetic", "group-synthetic"
			message := toProto(tc.row)
			if message.GetVisibility() != tc.visibility || message.GetState() != tc.state {
				t.Fatalf("toProto() = %v/%v, want %v/%v", message.GetVisibility(), message.GetState(), tc.visibility, tc.state)
			}
			if (message.GetCreatedAt() != nil) != tc.created || (tc.created && !message.GetCreatedAt().AsTime().Equal(createdAt)) {
				t.Fatalf("toProto() created_at = %v, want present=%v", message.GetCreatedAt(), tc.created)
			}
			if message.GetChannelId() != "channel-synthetic" || message.GetTenantId() != "tenant-synthetic" ||
				message.GetSpaceId() != "space-synthetic" || message.GetName() != "Synthetic" || message.GetE2EeGroupId() != "group-synthetic" {
				t.Fatalf("toProto() identity fields = %v, want exact copy", message)
			}
		})
	}
}
