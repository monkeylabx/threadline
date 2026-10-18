package syncrpc

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
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	testTenantID  = "tenant-sync-synthetic"
	testChannelID = "channel-sync-synthetic"
	testActorID   = "actor-sync-reader-synthetic"
	testDeviceID  = "device-sync-laptop-synthetic"
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

func readerSession(tenantID, actorID, deviceID string) rpcmiddleware.VerifiedSession {
	return rpcmiddleware.VerifiedSession{
		TenantID: tenantID, ActorType: rpcmiddleware.ActorTypeHuman, ActorID: actorID,
		DeviceID: deviceID, SessionID: "session-" + deviceID,
	}
}

func channelCursor(channelID string, position uint64) *syncv1.SyncCursor {
	return &syncv1.SyncCursor{Conversation: &syncv1.SyncCursor_ChannelId{ChannelId: channelID}, LastAppliedSeq: position}
}

// authenticatedUpdate runs UpdateCursors behind the real interceptor.
func authenticatedUpdate(t *testing.T, handler *Handler, verifier rpcmiddleware.SessionVerifier, credential string, cursors ...*syncv1.SyncCursor) (*connect.Response[syncv1.UpdateCursorsResponse], error) {
	t.Helper()
	unary := rpcmiddleware.NewAuthenticationInterceptor(verifier).WrapUnary(
		func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			typed, ok := request.(*connect.Request[syncv1.UpdateCursorsRequest])
			if !ok {
				t.Fatal("interceptor passed an unexpected request type")
			}
			return handler.UpdateCursors(ctx, typed)
		},
	)
	request := connect.NewRequest(&syncv1.UpdateCursorsRequest{Cursors: cursors})
	request.Header().Set("Authorization", "Bearer "+credential)
	response, err := unary(context.Background(), request)
	if err != nil {
		return nil, err
	}
	typed, ok := response.(*connect.Response[syncv1.UpdateCursorsResponse])
	if !ok {
		t.Fatal("interceptor returned an unexpected response type")
	}
	return typed, nil
}

func detailReason(t *testing.T, err error) (typev1.ErrorCode, string) {
	t.Helper()
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("error %v is not a Connect error", err)
	}
	for _, detail := range connectErr.Details() {
		message, valueErr := detail.Value()
		if valueErr != nil {
			continue
		}
		if typed, ok := message.(*typev1.ErrorDetail); ok {
			return typed.GetCode(), typed.GetReason()
		}
	}
	return typev1.ErrorCode_ERROR_CODE_UNSPECIFIED, ""
}

func TestNewRequiresTransactionStarter(t *testing.T) {
	t.Parallel()
	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) error = nil, want rejection")
	}
}

func TestValidateCursorsSeparatesShapeRejections(t *testing.T) {
	t.Parallel()
	inputs, rejections := validateCursors([]*syncv1.SyncCursor{
		channelCursor(testChannelID, 3),
		{Conversation: &syncv1.SyncCursor_DmId{DmId: "dm-synthetic"}},
		channelCursor("", 1),
		channelCursor(" padded", 1),
		channelCursor(strings.Repeat("c", maxIdentifierBytes+1), 1),
		channelCursor("channel-huge-synthetic", 1<<63),
		channelCursor(testChannelID, 4),
	})
	if len(inputs) != 1 || inputs[0].channelID != testChannelID || inputs[0].position != 3 {
		t.Fatalf("inputs = %+v, want exactly the first valid cursor", inputs)
	}
	reasons := make([]string, 0, len(rejections))
	for _, rejection := range rejections {
		reasons = append(reasons, rejection.GetError().GetReason())
		if rejection.GetError().GetCode() != typev1.ErrorCode_ERROR_CODE_CURSOR_INVALID || rejection.GetCursor() == nil {
			t.Fatalf("rejection %v lacks CURSOR_INVALID or the original cursor", rejection)
		}
	}
	want := []string{"dm_sync_not_enabled", "cursor_conversation_invalid", "cursor_conversation_invalid", "cursor_conversation_invalid", "cursor_ahead_of_history", "cursor_duplicate"}
	if strings.Join(reasons, ",") != strings.Join(want, ",") {
		t.Fatalf("rejection reasons = %v, want %v", reasons, want)
	}
}

func TestSelectCursorsFiltersChannelsAndMapsPositions(t *testing.T) {
	t.Parallel()
	updated := time.Date(2026, time.October, 18, 13, 0, 0, 0, time.UTC)
	rows := []dbgen.DomainSyncCursor{
		{ConversationKind: conversationKindChannel, ConversationID: "channel-a-synthetic", LastAppliedSeq: 5, UpdatedAt: pgtype.Timestamptz{Time: updated, Valid: true}},
		{ConversationKind: 2, ConversationID: "dm-synthetic", LastAppliedSeq: 9, UpdatedAt: pgtype.Timestamptz{Time: updated, Valid: true}},
		{ConversationKind: conversationKindChannel, ConversationID: "channel-b-synthetic", LastAppliedSeq: 1},
	}
	all := selectCursors(rows, nil)
	if len(all) != 2 || all[0].GetChannelId() != "channel-a-synthetic" || all[0].GetLastAppliedSeq() != 5 ||
		!all[0].GetUpdatedAt().AsTime().Equal(updated) || all[1].GetUpdatedAt() != nil {
		t.Fatalf("selectCursors(all) = %v, want both channel cursors with their positions", all)
	}
	filtered := selectCursors(rows, []string{"channel-b-synthetic"})
	if len(filtered) != 1 || filtered[0].GetChannelId() != "channel-b-synthetic" {
		t.Fatalf("selectCursors(filtered) = %v, want only channel-b", filtered)
	}
}

func TestHandlersFailClosedBeforeTheDatabase(t *testing.T) {
	t.Parallel()
	starter := &stubStarter{err: errors.New("pool exhausted")}
	handler, err := New(starter)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = handler.UpdateCursors(context.Background(), connect.NewRequest(&syncv1.UpdateCursorsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 1)}}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated || starter.calls.Load() != 0 {
		t.Fatalf("UpdateCursors() without principal = %v after %d BeginTx calls, want CodeUnauthenticated and 0", err, starter.calls.Load())
	}
	_, err = handler.GetCursors(context.Background(), connect.NewRequest(&syncv1.GetCursorsRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated || starter.calls.Load() != 0 {
		t.Fatalf("GetCursors() without principal = %v, want CodeUnauthenticated with no database call", err)
	}

	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{"reader-token": readerSession(testTenantID, testActorID, testDeviceID)}}
	_, err = authenticatedUpdate(t, handler, verifier, "reader-token")
	_, reason := detailReason(t, err)
	if connect.CodeOf(err) != connect.CodeInvalidArgument || reason != "cursor_count_invalid" || starter.calls.Load() != 0 {
		t.Fatalf("UpdateCursors(no cursors) = %v (%s), want InvalidArgument cursor_count_invalid with no database call", err, reason)
	}
	_, err = authenticatedUpdate(t, handler, verifier, "reader-token", channelCursor(testChannelID, 1))
	_, reason = detailReason(t, err)
	if connect.CodeOf(err) != connect.CodeUnavailable || reason != "cursors_unavailable" || starter.calls.Load() != 1 || strings.Contains(err.Error(), "pool exhausted") {
		t.Fatalf("UpdateCursors() with failing starter = %v (%s) after %d calls, want Unavailable cursors_unavailable, 1 call, no leaked text", err, reason, starter.calls.Load())
	}
}
