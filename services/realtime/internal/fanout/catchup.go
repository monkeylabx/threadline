package fanout

import (
	"context"

	"connectrpc.com/connect"
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
	"google.golang.org/protobuf/proto"
)

func (h *Handler) catchUp(ctx context.Context, credential string, cursor *syncv1.SyncCursor, send func(*syncv1.StreamEventsResponse) error) error {
	for page := 0; page < 128; page++ {
		request := connect.NewRequest(&syncv1.SyncEventsRequest{Cursors: []*syncv1.SyncCursor{proto.Clone(cursor).(*syncv1.SyncCursor)}, MaxEvents: 1})
		request.Header().Set("Authorization", credential)
		response, err := h.core.SyncEvents(ctx, request)
		if err != nil {
			return failure(connect.CodeOf(err), "Core sync unavailable")
		}
		batch, err := coreBatch(response, cursor)
		if err != nil {
			return err
		}
		if batch == nil {
			return nil
		}

		if err := send(&syncv1.StreamEventsResponse{Batch: batch}); err != nil {
			return failure(connect.CodeCanceled, "stream delivery ended")
		}
		cursor.LastAppliedSeq = batch.GetNextSeq()
		if !batch.GetHasMore() {
			return nil
		}
	}
	return failure(connect.CodeResourceExhausted, "catch-up limit; reconnect with applied cursor")
}

func coreBatch(response *connect.Response[syncv1.SyncEventsResponse], cursor *syncv1.SyncCursor) (*syncv1.EventBatch, error) {
	if response == nil || response.Msg == nil {
		return nil, failure(connect.CodeUnavailable, "Core sync unavailable")
	}
	if len(response.Msg.GetCursorRejections()) > 0 {
		return nil, failure(connect.CodePermissionDenied, "cursor unavailable")
	}
	batches := response.Msg.GetBatches()
	if len(batches) == 0 {
		return nil, nil
	}
	if len(batches) != 1 || !validBatch(batches[0], cursor) {
		return nil, failure(connect.CodeDataLoss, "invalid Core batch")
	}
	return batches[0], nil
}

func validBatch(batch *syncv1.EventBatch, cursor *syncv1.SyncCursor) bool {
	if batch == nil || len(batch.GetEvents()) != 1 {
		return false
	}
	event := batch.GetEvents()[0]
	return event.GetChannelId() == cursor.GetChannelId() && event.GetServerCommit().GetChannelSeq() == cursor.GetLastAppliedSeq()+1 &&
		batch.GetNextSeq() == event.GetServerCommit().GetChannelSeq()
}
