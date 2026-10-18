package messagerpc

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
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

func channelList(mutate func(*messagev1.ListEventsRequest)) *messagev1.ListEventsRequest {
	request := &messagev1.ListEventsRequest{
		Conversation: &messagev1.ListEventsRequest_ChannelId{ChannelId: testChannelID},
		Order:        messagev1.EventOrder_EVENT_ORDER_OLDEST_FIRST,
	}
	if mutate != nil {
		mutate(request)
	}
	return request
}

func TestValidateListAppliesGatesBoundsAndTokens(t *testing.T) {
	t.Parallel()
	rejections := map[string]struct {
		mutate func(*messagev1.ListEventsRequest)
		code   connect.Code
		detail typev1.ErrorCode
		reason string
	}{
		"direct message": {func(r *messagev1.ListEventsRequest) {
			r.Conversation = &messagev1.ListEventsRequest_DmId{DmId: "dm-synthetic"}
		},
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "dm_read_not_enabled"},
		"thread filter": {func(r *messagev1.ListEventsRequest) { r.ThreadId = "thread-synthetic" },
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "thread_filter_not_enabled"},
		"missing channel": {func(r *messagev1.ListEventsRequest) { r.Conversation = nil },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_UNSPECIFIED, "channel_id_invalid"},
		"negative page size": {func(r *messagev1.ListEventsRequest) { r.Page = &typev1.PageRequest{PageSize: -1} },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_UNSPECIFIED, "page_size_invalid"},
		"non-numeric token": {func(r *messagev1.ListEventsRequest) { r.Page = &typev1.PageRequest{PageToken: "abc"} },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CURSOR_INVALID, "page_token_invalid"},
		"zero token": {func(r *messagev1.ListEventsRequest) { r.Page = &typev1.PageRequest{PageToken: "0"} },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CURSOR_INVALID, "page_token_invalid"},
		"padded token": {func(r *messagev1.ListEventsRequest) { r.Page = &typev1.PageRequest{PageToken: "007"} },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CURSOR_INVALID, "page_token_invalid"},
	}
	for name, tc := range rejections {
		_, err := validateList(channelList(tc.mutate))
		code, reason := detailReason(t, err)
		if connect.CodeOf(err) != tc.code || code != tc.detail || reason != tc.reason {
			t.Fatalf("validateList(%s) = %v (%v/%s), want %v %v/%s", name, err, code, reason, tc.code, tc.detail, tc.reason)
		}
	}

	defaults, err := validateList(channelList(nil))
	if err != nil || defaults.limit != defaultPageSize || defaults.newestFirst || defaults.afterSeq != 0 {
		t.Fatalf("validateList(defaults) = %+v %v, want oldest-first, after 0, limit 50", defaults, err)
	}
	clamped, err := validateList(channelList(func(r *messagev1.ListEventsRequest) { r.Page = &typev1.PageRequest{PageSize: 500} }))
	if err != nil || clamped.limit != maxPageSize {
		t.Fatalf("validateList(page_size 500) limit = %d %v, want 200", clamped.limit, err)
	}
	newest, err := validateList(channelList(func(r *messagev1.ListEventsRequest) { r.Order = messagev1.EventOrder_EVENT_ORDER_NEWEST_FIRST }))
	if err != nil || !newest.newestFirst || newest.beforeSeq != 1<<63-1 {
		t.Fatalf("validateList(newest, before 0) = %+v %v, want unbounded newest-first", newest, err)
	}
	continued, err := validateList(channelList(func(r *messagev1.ListEventsRequest) {
		r.Order = messagev1.EventOrder_EVENT_ORDER_NEWEST_FIRST
		r.BeforeSeq = 9
		r.Page = &typev1.PageRequest{PageToken: "7"}
	}))
	if err != nil || continued.beforeSeq != 7 {
		t.Fatalf("validateList(token 7) beforeSeq = %d %v, want the token to win over before_seq", continued.beforeSeq, err)
	}
	huge, err := validateList(channelList(func(r *messagev1.ListEventsRequest) { r.AfterSeq = ^uint64(0) }))
	if err != nil || huge.afterSeq != 1<<63-1 {
		t.Fatalf("validateList(after max uint64) afterSeq = %d %v, want clamped to max int64", huge.afterSeq, err)
	}
}

func storedRow(sequence int64, envelope *messagev1.ChannelEventEnvelope) dbgen.DomainChannelEvent {
	encoded, _ := proto.MarshalOptions{Deterministic: true}.Marshal(envelope)
	channel := testChannelID
	return dbgen.DomainChannelEvent{
		TenantID: testTenantID, EventID: envelope.GetEventId(), ChannelID: &channel, ChannelSeq: sequence,
		Envelope: encoded, CommittedAt: pgtype.Timestamptz{Time: time.Unix(1_800_000_000+sequence, 0).UTC(), Valid: true},
	}
}

func TestDecodeContiguousReattachesCommitsAndTruncatesAtAHole(t *testing.T) {
	t.Parallel()
	rows := []dbgen.DomainChannelEvent{
		storedRow(1, validEnvelope("event-1", "key-1")),
		storedRow(2, validEnvelope("event-2", "key-2")),
		storedRow(4, validEnvelope("event-4", "key-4")),
	}
	events, err := decodeContiguous(rows, false)
	if err != nil || len(events) != 2 {
		t.Fatalf("decodeContiguous(1,2,4) = %d events %v, want 2 (truncated at the hole)", len(events), err)
	}
	if events[1].GetEventId() != "event-2" || events[1].GetServerCommit().GetChannelSeq() != 2 ||
		!events[1].GetServerCommit().GetCommittedAt().AsTime().Equal(rows[1].CommittedAt.Time) ||
		string(events[1].GetApplicationCiphertext()) != "opaque-body-synthetic" {
		t.Fatalf("decoded event = %v, want stored envelope with ServerCommit seq 2", events[1])
	}
	if nextPageToken(events, 2) != "2" || nextPageToken(events, 3) != "" || nextPageToken(nil, 1) != "" {
		t.Fatal("nextPageToken must name the last sequence of a full page only")
	}

	descending := []dbgen.DomainChannelEvent{storedRow(3, validEnvelope("event-3", "key-3")), storedRow(2, validEnvelope("event-2", "key-2"))}
	events, err = decodeContiguous(descending, true)
	if err != nil || len(events) != 2 || events[0].GetServerCommit().GetChannelSeq() != 3 {
		t.Fatalf("decodeContiguous(newest-first 3,2) = %v %v, want both in order", events, err)
	}

	corrupt := storedRow(1, validEnvelope("event-1", "key-1"))
	corrupt.Envelope = []byte{0xff, 0xff, 0xff}
	if _, err := decodeContiguous([]dbgen.DomainChannelEvent{corrupt}, false); err == nil {
		t.Fatal("decodeContiguous(corrupt bytes) error = nil, want failure instead of a skipped row")
	}
}

func TestListEventsFailsClosedBeforeTheDatabase(t *testing.T) {
	t.Parallel()
	starter := &stubStarter{err: errors.New("pool exhausted")}
	handler, err := New(starter, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = handler.ListEvents(context.Background(), connect.NewRequest(channelList(nil)))
	if connect.CodeOf(err) != connect.CodeUnauthenticated || starter.calls.Load() != 0 {
		t.Fatalf("ListEvents() without principal = %v after %d BeginTx calls, want CodeUnauthenticated and 0", err, starter.calls.Load())
	}

	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{"reader-token": senderSession(testTenantID, testActorID, testDeviceID)}}
	unary := rpcmiddleware.NewAuthenticationInterceptor(verifier).WrapUnary(
		func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			typed, ok := request.(*connect.Request[messagev1.ListEventsRequest])
			if !ok {
				t.Fatal("interceptor passed an unexpected request type")
			}
			return handler.ListEvents(ctx, typed)
		},
	)
	gated := connect.NewRequest(channelList(func(r *messagev1.ListEventsRequest) { r.ThreadId = "thread-synthetic" }))
	gated.Header().Set("Authorization", "Bearer reader-token")
	if _, err := unary(context.Background(), gated); connect.CodeOf(err) != connect.CodeFailedPrecondition || starter.calls.Load() != 0 {
		t.Fatalf("ListEvents(thread filter) = %v after %d BeginTx calls, want FailedPrecondition and 0", err, starter.calls.Load())
	}
	valid := connect.NewRequest(channelList(nil))
	valid.Header().Set("Authorization", "Bearer reader-token")
	_, err = unary(context.Background(), valid)
	_, reason := detailReason(t, err)
	if connect.CodeOf(err) != connect.CodeUnavailable || reason != "read_unavailable" || starter.calls.Load() != 1 {
		t.Fatalf("ListEvents() with failing starter = %v (%s) after %d calls, want Unavailable read_unavailable and 1", err, reason, starter.calls.Load())
	}
}
