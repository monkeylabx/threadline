package syncrpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/proto"

	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

func storedEvent(sequence int64) dbgen.DomainChannelEvent {
	encoded, _ := proto.MarshalOptions{Deterministic: true}.Marshal(&messagev1.ChannelEventEnvelope{
		EventId: "event-" + string(rune('0'+sequence)), TenantId: testTenantID,
		Conversation: &messagev1.ChannelEventEnvelope_ChannelId{ChannelId: testChannelID},
		Body:         &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: []byte("body")},
	})
	channel := testChannelID
	return dbgen.DomainChannelEvent{
		TenantID: testTenantID, EventID: "event-" + string(rune('0'+sequence)), ChannelID: &channel, ChannelSeq: sequence,
		Envelope: encoded, CommittedAt: pgtype.Timestamptz{Time: time.Unix(1_800_000_000+sequence, 0).UTC(), Valid: true},
	}
}

func TestClampMaxEventsAppliesDefaultAndCeiling(t *testing.T) {
	t.Parallel()
	if clampMaxEvents(0) != defaultMaxEvents || clampMaxEvents(5) != 5 || clampMaxEvents(10_000) != maxMaxEvents {
		t.Fatal("clampMaxEvents must default zero, pass small values and cap large ones")
	}
}

func TestDecodeContiguousStartsAtTheCursorAndStopsAtHolesOrBudget(t *testing.T) {
	t.Parallel()
	rows := []dbgen.DomainChannelEvent{storedEvent(2), storedEvent(3), storedEvent(5)}
	events, err := decodeContiguous(rows, 1, 10)
	if err != nil || len(events) != 2 || events[1].GetServerCommit().GetChannelSeq() != 3 {
		t.Fatalf("decodeContiguous(after 1) = %d events %v, want 2 (stopped at the hole before 5)", len(events), err)
	}
	if !events[0].GetServerCommit().GetCommittedAt().AsTime().Equal(rows[0].CommittedAt.Time) || events[0].GetEventId() != "event-2" {
		t.Fatalf("decoded event = %v, want the stored envelope with its commit", events[0])
	}
	limited, err := decodeContiguous(rows, 1, 1)
	if err != nil || len(limited) != 1 {
		t.Fatalf("decodeContiguous(budget 1) = %d events %v, want 1", len(limited), err)
	}
	gap, err := decodeContiguous(rows, 0, 10)
	if err != nil || len(gap) != 0 {
		t.Fatalf("decodeContiguous(after 0, first row 2) = %d events %v, want none because 1 is missing", len(gap), err)
	}
	corrupt := storedEvent(2)
	corrupt.Envelope = []byte{0xff}
	if _, err := decodeContiguous([]dbgen.DomainChannelEvent{corrupt}, 1, 10); err == nil {
		t.Fatal("decodeContiguous(corrupt) error = nil, want failure instead of a skipped row")
	}
}

func TestSyncEventsFailsClosedBeforeTheDatabase(t *testing.T) {
	t.Parallel()
	starter := &stubStarter{err: errors.New("pool exhausted")}
	handler, err := New(starter)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = handler.SyncEvents(context.Background(), connect.NewRequest(&syncv1.SyncEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 0)}}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated || starter.calls.Load() != 0 {
		t.Fatalf("SyncEvents() without principal = %v after %d BeginTx calls, want CodeUnauthenticated and 0", err, starter.calls.Load())
	}
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{"reader-token": readerSession(testTenantID, testActorID, testDeviceID)}}
	unary := rpcmiddleware.NewAuthenticationInterceptor(verifier).WrapUnary(
		func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			typed, ok := request.(*connect.Request[syncv1.SyncEventsRequest])
			if !ok {
				t.Fatal("interceptor passed an unexpected request type")
			}
			return handler.SyncEvents(ctx, typed)
		},
	)
	empty := connect.NewRequest(&syncv1.SyncEventsRequest{})
	empty.Header().Set("Authorization", "Bearer reader-token")
	_, err = unary(context.Background(), empty)
	if _, reason := detailReason(t, err); connect.CodeOf(err) != connect.CodeInvalidArgument || reason != "cursors_required" || starter.calls.Load() != 0 {
		t.Fatalf("SyncEvents(no cursors) = %v, want InvalidArgument cursors_required with no database call", err)
	}
	valid := connect.NewRequest(&syncv1.SyncEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 0)}})
	valid.Header().Set("Authorization", "Bearer reader-token")
	_, err = unary(context.Background(), valid)
	if _, reason := detailReason(t, err); connect.CodeOf(err) != connect.CodeUnavailable || reason != "events_unavailable" || starter.calls.Load() != 1 {
		t.Fatalf("SyncEvents() with failing starter = %v after %d calls, want Unavailable events_unavailable and 1", err, starter.calls.Load())
	}
}
