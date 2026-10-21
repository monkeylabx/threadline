// Package fanout delivers Core-authorized Sync pages after live broker hints.
package fanout

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/sync/v1/syncv1connect"
	"google.golang.org/protobuf/proto"
)

type Core interface {
	SyncEvents(context.Context, *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error)
}
type Handler struct {
	syncv1connect.UnimplementedSyncServiceHandler
	core   Core
	source WakeSource
	slots  chan struct{}
}

func New(core Core, source WakeSource) (*Handler, error) {
	if core == nil || source == nil {
		return nil, errors.New("realtime dependencies required")
	}
	return &Handler{core: core, source: source, slots: make(chan struct{}, 256)}, nil
}

func (h *Handler) StreamEvents(ctx context.Context, request *connect.Request[syncv1.StreamEventsRequest], stream *connect.ServerStream[syncv1.StreamEventsResponse]) error {
	return h.serve(ctx, request, stream.Send)
}

func (h *Handler) serve(ctx context.Context, request *connect.Request[syncv1.StreamEventsRequest], send func(*syncv1.StreamEventsResponse) error) (result error) {
	cursor, err := streamCursor(request)
	if err != nil {
		return err
	}
	if err := h.reserve(); err != nil {
		return err
	}
	defer func() { <-h.slots }()

	streamCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	signals, release, err := h.source.Subscribe(streamCtx)
	if err != nil {
		return failure(connect.CodeUnavailable, "broker unavailable")
	}
	defer func() {
		if err := release(); err != nil && result == nil {
			result = failure(connect.CodeUnavailable, "subscription cleanup failed")
		}
	}()
	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	for {
		if err := h.catchUp(streamCtx, request.Header().Get("Authorization"), cursor, send); err != nil {
			return err
		}
		select {
		case <-streamCtx.Done():
			return failure(connect.CodeCanceled, "stream ended")
		case <-signals:
		case <-poll.C:
		}
	}
}

func streamCursor(request *connect.Request[syncv1.StreamEventsRequest]) (*syncv1.SyncCursor, error) {
	cursors := request.Msg.GetCursors()
	if len(cursors) != 1 || cursors[0].GetChannelId() == "" {
		return nil, failure(connect.CodeInvalidArgument, "one Channel cursor required")
	}
	if request.Header().Get("Authorization") == "" {
		return nil, failure(connect.CodeUnauthenticated, "authentication required")
	}
	return proto.Clone(cursors[0]).(*syncv1.SyncCursor), nil
}

func (h *Handler) reserve() error {
	select {
	case h.slots <- struct{}{}:
		return nil
	default:
		return failure(connect.CodeResourceExhausted, "stream limit")
	}
}

func failure(code connect.Code, message string) error {
	return connect.NewError(code, errors.New(message))
}
