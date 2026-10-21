package fanout

import (
	"context"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/sync/v1/syncv1connect"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestLiveBrokerFanoutAuthorizationAndSubscriptionCleanup(t *testing.T) {
	address := os.Getenv("THREADLINE_TEST_REALTIME_NATS_URL")
	if address == "" {
		t.Skip("real NATS fixture not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reader, err := nats.Connect(address, nats.UserInfo("threadline_realtime_dev", "threadline-worker-dev-only"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	publisher, err := nats.Connect(address, nats.UserInfo("threadline_worker_dev", "threadline-worker-dev-only"), nats.CustomInboxPrefix("_INBOX.threadline.worker"))
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	sequence, started, core := liveCore()

	handler, err := New(core, NATS{Connection: reader})
	if err != nil {
		t.Fatal(err)
	}
	_, httpHandler := syncv1connect.NewSyncServiceHandler(handler)
	server := httptest.NewServer(httpHandler)
	defer server.Close()
	client := syncv1connect.NewSyncServiceClient(server.Client(), server.URL)
	received := make(chan uint64, 2)
	for index := 0; index < 2; index++ {
		go receiveOnce(ctx, client, received)
	}
	for index := 0; index < 2; index++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("subscribers did not start")
		}
	}
	sequence.Store(1)
	js, err := jetstream.New(publisher)
	if err != nil {
		t.Fatal(err)
	}
	ack, err := js.Publish(ctx, "threadline.domain.events.v1", hintBytes(hintFields()), jetstream.WithMsgID("realtime-synthetic-"+time.Now().Format("150405.000000000")))
	if err != nil || ack.Stream != "THREADLINE_DOMAIN_EVENTS_V1" {
		t.Fatal("real event publish failed")
	}
	for index := 0; index < 2; index++ {
		select {
		case seq := <-received:
			if seq != 1 {
				t.Fatal("subscriber missed event")
			}
		case <-ctx.Done():
			t.Fatal("fan-out timed out")
		}
	}
	assertOutsiderDenied(t, ctx, client)

	waitSubscriptionsClosed(t, ctx, reader)
}

func assertOutsiderDenied(t *testing.T, ctx context.Context, client syncv1connect.SyncServiceClient) {
	t.Helper()
	outsider := streamRequest()
	outsider.Header().Set("Authorization", "Bearer outsider")
	denied, err := client.StreamEvents(ctx, outsider)
	if err == nil {
		defer denied.Close()
		if denied.Receive() {
			t.Fatal("outsider received event")
		}
		err = denied.Err()
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal("outsider was not denied")
	}
}

func liveCore() (*atomic.Uint64, <-chan struct{}, coreFunc) {
	var sequence atomic.Uint64
	started := make(chan struct{}, 4)
	core := coreFunc(func(_ context.Context, r *connect.Request[syncv1.SyncEventsRequest]) (*connect.Response[syncv1.SyncEventsResponse], error) {
		select {
		case started <- struct{}{}:
		default:
		}
		if r.Header().Get("Authorization") == "Bearer outsider" {
			return connect.NewResponse(&syncv1.SyncEventsResponse{CursorRejections: []*syncv1.CursorRejection{{Cursor: channelCursor(0)}}}), nil
		}
		result := &syncv1.SyncEventsResponse{}
		if sequence.Load() > r.Msg.Cursors[0].LastAppliedSeq {
			result.Batches = []*syncv1.EventBatch{batch(sequence.Load())}
		}
		return connect.NewResponse(result), nil
	})
	return &sequence, started, core
}

func receiveOnce(ctx context.Context, client syncv1connect.SyncServiceClient, received chan<- uint64) {
	stream, err := client.StreamEvents(ctx, streamRequest())
	if err != nil {
		received <- 0
		return
	}
	defer stream.Close()
	if !stream.Receive() {
		received <- 0
		return
	}
	received <- stream.Msg().GetBatch().GetNextSeq()
}

func waitSubscriptionsClosed(t *testing.T, ctx context.Context, connection *nats.Conn) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for connection.NumSubscriptions() != 0 {
		select {
		case <-ctx.Done():
			t.Fatal("subscription leak")
		case <-ticker.C:
		}
	}
}
