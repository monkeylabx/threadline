package messagerpc

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/outboxstore"
	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/outboxpolicy"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const defaultPolicyDocument = `{
  "policy_id": "threadline.outbox.policy/v1",
  "payload_hard_bytes": 262144,
  "wire_hard_bytes": 327680,
  "batch_size": 64,
  "lease_ms": 30000,
  "absolute_lifetime_ms": 300000,
  "event_retry_ceiling": 8,
  "transport_base_ms": 1000,
  "transport_cap_ms": 60000,
  "unknown_base_ms": 5000,
  "unknown_cap_ms": 300000,
  "event_base_ms": 5000,
  "event_cap_ms": 300000,
  "retention_days": 90,
  "duplicate_window_ms": 120000
}`

type stubStarter struct {
	calls atomic.Int32
	err   error
}

func (starter *stubStarter) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	starter.calls.Add(1)
	return nil, starter.err
}

func testProducer(t *testing.T) *outboxstore.Producer {
	t.Helper()
	policy, err := outboxpolicy.Parse([]byte(defaultPolicyDocument))
	if err != nil {
		t.Fatalf("Parse(default policy) error = %v", err)
	}
	producer, err := outboxstore.NewProducer(policy)
	if err != nil {
		t.Fatalf("NewProducer() error = %v", err)
	}
	return producer
}

// authenticatedSend runs SendEvent behind the real interceptor so the
// principal reaches the handler the same way it does in the process.
func authenticatedSend(
	t *testing.T,
	handler *Handler,
	verifier rpcmiddleware.SessionVerifier,
	credential string,
	envelope *messagev1.ChannelEventEnvelope,
) (*connect.Response[messagev1.SendEventResponse], error) {
	t.Helper()
	unary := rpcmiddleware.NewAuthenticationInterceptor(verifier).WrapUnary(
		func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			typed, ok := request.(*connect.Request[messagev1.SendEventRequest])
			if !ok {
				t.Fatal("interceptor passed an unexpected request type")
			}
			return handler.SendEvent(ctx, typed)
		},
	)
	request := connect.NewRequest(&messagev1.SendEventRequest{Envelope: envelope})
	request.Header().Set("Authorization", "Bearer "+credential)
	response, err := unary(context.Background(), request)
	if err != nil {
		return nil, err
	}
	typed, ok := response.(*connect.Response[messagev1.SendEventResponse])
	if !ok {
		t.Fatal("interceptor returned an unexpected response type")
	}
	return typed, nil
}

func TestNewRequiresTransactionStarter(t *testing.T) {
	t.Parallel()
	if _, err := New(nil, nil); err == nil {
		t.Fatal("New(nil) error = nil, want rejection")
	}
}

func TestSendEventRequiresPrincipal(t *testing.T) {
	t.Parallel()
	starter := &stubStarter{err: errors.New("must not be reached")}
	handler, err := New(starter, testProducer(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = handler.SendEvent(context.Background(), connect.NewRequest(&messagev1.SendEventRequest{Envelope: validEnvelope("event-1", "key-1")}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated || starter.calls.Load() != 0 {
		t.Fatalf("SendEvent() without principal = %v after %d BeginTx calls, want CodeUnauthenticated and 0", err, starter.calls.Load())
	}
}

func TestSendEventRejectsInvalidEnvelopesBeforeTheDatabase(t *testing.T) {
	t.Parallel()
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"sender-token": senderSession(testTenantID, testActorID, testDeviceID),
	}}
	cases := map[string]struct {
		mutate func(*messagev1.ChannelEventEnvelope)
		code   connect.Code
		reason string
	}{
		"server commit supplied": {func(e *messagev1.ChannelEventEnvelope) { e.ServerCommit = &messagev1.ServerCommit{ChannelSeq: 1} },
			connect.CodeInvalidArgument, "server_commit_supplied"},
		"other tenant": {func(e *messagev1.ChannelEventEnvelope) { e.TenantId = "tenant-other-synthetic" },
			connect.CodePermissionDenied, "envelope_tenant_mismatch"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			starter := &stubStarter{err: errors.New("must not be reached")}
			handler, err := New(starter, testProducer(t))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			envelope := validEnvelope("event-1", "key-1")
			tc.mutate(envelope)
			_, err = authenticatedSend(t, handler, verifier, "sender-token", envelope)
			_, reason := detailReason(t, err)
			if connect.CodeOf(err) != tc.code || reason != tc.reason || starter.calls.Load() != 0 {
				t.Fatalf("SendEvent(%s) = %v (%s) after %d BeginTx calls, want %v/%s and 0", name, err, reason, starter.calls.Load(), tc.code, tc.reason)
			}
		})
	}
}

func TestSendEventFailsClosedWithoutAProducerOrTransaction(t *testing.T) {
	t.Parallel()
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"sender-token": senderSession(testTenantID, testActorID, testDeviceID),
	}}
	starter := &stubStarter{err: errors.New("pool exhausted")}
	notReady, err := New(starter, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = authenticatedSend(t, notReady, verifier, "sender-token", validEnvelope("event-1", "key-1"))
	code, reason := detailReason(t, err)
	if connect.CodeOf(err) != connect.CodeUnavailable || code != typev1.ErrorCode_ERROR_CODE_NOT_DURABLE ||
		reason != "message_producer_not_ready" || starter.calls.Load() != 0 {
		t.Fatalf("SendEvent() without producer = %v (%v/%s) after %d calls, want Unavailable NOT_DURABLE/message_producer_not_ready and 0", err, code, reason, starter.calls.Load())
	}

	ready, err := New(starter, testProducer(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = authenticatedSend(t, ready, verifier, "sender-token", validEnvelope("event-1", "key-1"))
	code, reason = detailReason(t, err)
	if connect.CodeOf(err) != connect.CodeUnavailable || code != typev1.ErrorCode_ERROR_CODE_NOT_DURABLE ||
		reason != "commit_not_durable" || starter.calls.Load() != 1 || strings.Contains(err.Error(), "pool exhausted") {
		t.Fatalf("SendEvent() with failing starter = %v (%v/%s) after %d calls, want Unavailable NOT_DURABLE/commit_not_durable, 1 call and no leaked text", err, code, reason, starter.calls.Load())
	}
}
