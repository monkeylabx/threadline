package fanout

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
)

type coreFunc func(context.Context, *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error)

func (f coreFunc) SyncEvents(ctx context.Context, r *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
	return f(ctx, r)
}

type wake struct {
	signals chan struct{}
	closed  atomic.Int32
	err     error
}

func (w *wake) Subscribe(context.Context) (<-chan struct{}, func() error, error) {
	return w.signals, func() error { w.closed.Add(1); return w.err }, nil
}

func channelCursor(seq uint64) *syncv1.SyncCursor {
	return &syncv1.SyncCursor{Conversation: &syncv1.SyncCursor_ChannelId{ChannelId: "c"}, LastAppliedSeq: seq}
}
func batch(seq uint64) *syncv1.EventBatch {
	return &syncv1.EventBatch{Events: []*messagev1.ChannelEventEnvelope{{Conversation: &messagev1.ChannelEventEnvelope_ChannelId{ChannelId: "c"}, ServerCommit: &messagev1.ServerCommit{ChannelSeq: seq}}}, NextSeq: seq}
}
func streamRequest() *connect.Request[syncv1.StreamEventsRequest] {
	r := connect.NewRequest(&syncv1.StreamEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(0)}})
	r.Header().Set("Authorization", "Bearer synthetic")
	return r
}

func TestCatchUpForwardsOnlyCoreAuthorizedContiguousEvents(t *testing.T) {
	calls := 0
	source := &wake{}
	core := coreFunc(func(_ context.Context, r *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
		calls++
		if r.Header().Get("Authorization") != "Bearer synthetic" || r.Msg.GetCursors()[0].GetLastAppliedSeq() != uint64(calls-1) {
			t.Error("credential or cursor changed")
		}
		result := batch(uint64(calls))
		result.HasMore = calls == 1
		return connect.NewResponse(&syncv1.SyncEventsResponse{Batches: []*syncv1.EventBatch{result}}), nil
	})
	handler, err := New(core, source)
	if err != nil {
		t.Fatal(err)
	}
	cursor := channelCursor(0)
	var delivered []uint64
	err = handler.catchUp(context.Background(), "Bearer synthetic", cursor, func(r *syncv1.StreamEventsResponse) error { delivered = append(delivered, r.Batch.NextSeq); return nil })
	if err != nil || calls != 2 || len(delivered) != 2 || cursor.LastAppliedSeq != 2 {
		t.Fatal("catch-up did not advance exactly once per delivered event")
	}
}

func TestCatchUpRejectsDeniedUnavailableAndMismatchedBatches(t *testing.T) {
	bad := batch(1)
	bad.Events[0].Conversation = &messagev1.ChannelEventEnvelope_ChannelId{ChannelId: "foreign"}
	cases := []*syncv1.SyncEventsResponse{{CursorRejections: []*syncv1.CursorRejection{{Cursor: channelCursor(0)}}}, {Batches: []*syncv1.EventBatch{bad}}, {Batches: []*syncv1.EventBatch{batch(2)}}, {Batches: []*syncv1.EventBatch{nil}}}
	for _, value := range cases {
		handler, _ := New(coreFunc(func(context.Context, *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
			return connect.NewResponse(value), nil
		}), &wake{})
		sent := false
		if err := handler.catchUp(context.Background(), "token", channelCursor(0), func(*syncv1.StreamEventsResponse) error { sent = true; return nil }); err == nil || sent {
			t.Fatal("unreadable data delivered")
		}
	}
	for _, fail := range []coreFunc{
		func(context.Context, *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("secret"))
		},
		func(context.Context, *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
			return nil, nil
		},
	} {
		handler, _ := New(fail, &wake{})
		if err := handler.catchUp(context.Background(), "token", channelCursor(0), func(*syncv1.StreamEventsResponse) error { t.Error("unexpected delivery"); return nil }); err == nil {
			t.Fatal("dependency failure accepted")
		}
	}
}

func TestStreamCancellationReleasesSubscriptionAndKeepsCallerCursor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &wake{signals: make(chan struct{}, 1)}
	handler, _ := New(coreFunc(func(context.Context, *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
		return connect.NewResponse(&syncv1.SyncEventsResponse{Batches: []*syncv1.EventBatch{batch(1)}}), nil
	}), source)
	request := streamRequest()
	err := handler.serve(ctx, request, func(*syncv1.StreamEventsResponse) error { cancel(); return nil })
	if err == nil || source.closed.Load() != 1 || request.Msg.Cursors[0].LastAppliedSeq != 0 || len(handler.slots) != 0 {
		t.Fatal("stream state or subscription leaked")
	}
}

func TestRevocationOnNextHintCannotDeliverAnotherEvent(t *testing.T) {
	calls := 0
	source := &wake{signals: make(chan struct{}, 1)}
	source.signals <- struct{}{}
	handler, _ := New(coreFunc(func(context.Context, *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
		calls++
		result := &syncv1.SyncEventsResponse{}
		if calls == 2 {
			result.CursorRejections = []*syncv1.CursorRejection{{Cursor: channelCursor(0)}}
		}
		return connect.NewResponse(result), nil
	}), source)
	err := handler.serve(context.Background(), streamRequest(), func(*syncv1.StreamEventsResponse) error { t.Error("revoked subscriber received data"); return nil })
	if connect.CodeOf(err) != connect.CodePermissionDenied || calls != 2 || source.closed.Load() != 1 {
		t.Fatal("revocation did not terminate stream")
	}
}

func TestInvalidRequestsAndCapacityFailBeforeSubscription(t *testing.T) {
	handler, _ := New(coreFunc(func(context.Context, *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
		t.Error("Core must not be called")
		return nil, nil
	}), &wake{})
	for _, request := range []*connect.Request[syncv1.StreamEventsRequest]{connect.NewRequest(&syncv1.StreamEventsRequest{}), connect.NewRequest(&syncv1.StreamEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(0)}})} {
		if err := handler.serve(context.Background(), request, nil); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	for index := 0; index < cap(handler.slots); index++ {
		handler.slots <- struct{}{}
	}
	if err := handler.serve(context.Background(), streamRequest(), nil); connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatal("capacity not bounded")
	}
	if _, err := New(nil, &wake{}); err == nil {
		t.Fatal("nil Core accepted")
	}
	if _, err := New(handler.core, nil); err == nil {
		t.Fatal("nil source accepted")
	}
}
