package messagerpc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/message/v1/messagev1connect"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

func TestListEventsReturnsContiguousAuthorizedPages(t *testing.T) {
	ctx, pool, databaseName := openMessageDatabase(t)
	createMessageFixtures(t, ctx, pool)
	handler, err := New(pool, testProducer(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"sender-token":   senderSession(testTenantID, testActorID, testDeviceID),
		"outsider-token": senderSession(testTenantID, outsiderID, "device-message-outsider-synthetic"),
		"beta-token":     senderSession(betaTenantID, betaActorID, "device-message-beta-synthetic"),
	}}
	mux := http.NewServeMux()
	mux.Handle(messagev1connect.NewMessageServiceHandler(handler, connect.WithInterceptors(rpcmiddleware.NewAuthenticationInterceptor(verifier))))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	client := messagev1connect.NewMessageServiceClient(httpServer.Client(), httpServer.URL)

	sent := make([]*messagev1.ChannelEventEnvelope, 0, 3)
	for _, id := range []string{"1", "2", "3"} {
		envelope := validEnvelope("event-"+id, "key-"+id)
		envelope.Body = &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: []byte("body-" + id)}
		if _, err := client.SendEvent(ctx, authorizedSend("sender-token", envelope)); err != nil {
			t.Fatalf("SendEvent(%s) error = %v", id, err)
		}
		sent = append(sent, envelope)
	}

	oldest, err := client.ListEvents(ctx, authorizedList("sender-token", channelList(nil)))
	if err != nil {
		t.Fatalf("ListEvents(oldest) error = %v", err)
	}
	assertPage(t, oldest.Msg, []uint64{1, 2, 3}, "")
	for index, event := range oldest.Msg.GetEvents() {
		if event.GetEventId() != sent[index].GetEventId() || string(event.GetApplicationCiphertext()) != string(sent[index].GetApplicationCiphertext()) ||
			event.GetIdempotencyKey() != sent[index].GetIdempotencyKey() || event.GetServerCommit().GetCommittedAt() == nil {
			t.Fatalf("listed event %d = %v, want the stored envelope %v with its commit", index, event, sent[index])
		}
	}

	newest, err := client.ListEvents(ctx, authorizedList("sender-token", channelList(func(r *messagev1.ListEventsRequest) {
		r.Order = messagev1.EventOrder_EVENT_ORDER_NEWEST_FIRST
		r.Page = &typev1.PageRequest{PageSize: 2}
	})))
	if err != nil {
		t.Fatalf("ListEvents(newest, size 2) error = %v", err)
	}
	assertPage(t, newest.Msg, []uint64{3, 2}, "2")
	continued, err := client.ListEvents(ctx, authorizedList("sender-token", channelList(func(r *messagev1.ListEventsRequest) {
		r.Order = messagev1.EventOrder_EVENT_ORDER_NEWEST_FIRST
		r.Page = &typev1.PageRequest{PageSize: 2, PageToken: newest.Msg.GetPage().GetNextPageToken()}
	})))
	if err != nil {
		t.Fatalf("ListEvents(continued) error = %v", err)
	}
	assertPage(t, continued.Msg, []uint64{1}, "")

	after, err := client.ListEvents(ctx, authorizedList("sender-token", channelList(func(r *messagev1.ListEventsRequest) { r.AfterSeq = 2 })))
	if err != nil {
		t.Fatalf("ListEvents(after 2) error = %v", err)
	}
	assertPage(t, after.Msg, []uint64{3}, "")

	for name, request := range map[string]*connect.Request[messagev1.ListEventsRequest]{
		"tenant member without channel membership": authorizedList("outsider-token", channelList(nil)),
		"other tenant using the same channel id":   authorizedList("beta-token", channelList(nil)),
	} {
		_, err := client.ListEvents(ctx, request)
		if _, reason := detailReason(t, err); connect.CodeOf(err) != connect.CodePermissionDenied || reason != "read_unavailable" {
			t.Fatalf("ListEvents() %s = %v, want PermissionDenied read_unavailable", name, err)
		}
	}
	if _, err := client.ListEvents(ctx, connect.NewRequest(channelList(nil))); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("ListEvents() without credential = %v, want CodeUnauthenticated", err)
	}
	assertNoOpenTransactions(t, ctx, pool, databaseName)
}

func authorizedList(credential string, body *messagev1.ListEventsRequest) *connect.Request[messagev1.ListEventsRequest] {
	request := connect.NewRequest(body)
	request.Header().Set("Authorization", "Bearer "+credential)
	return request
}

func assertPage(t *testing.T, page *messagev1.ListEventsResponse, sequences []uint64, token string) {
	t.Helper()
	if len(page.GetEvents()) != len(sequences) || page.GetPage().GetNextPageToken() != token {
		t.Fatalf("page = %d events token %q, want %v token %q", len(page.GetEvents()), page.GetPage().GetNextPageToken(), sequences, token)
	}
	for index, event := range page.GetEvents() {
		if event.GetServerCommit().GetChannelSeq() != sequences[index] {
			t.Fatalf("page[%d] seq = %d, want %d", index, event.GetServerCommit().GetChannelSeq(), sequences[index])
		}
	}
}
