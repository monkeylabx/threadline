package messagerpc

import (
	"bytes"
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

const (
	testTenantID  = "tenant-message-synthetic"
	testChannelID = "channel-message-synthetic"
	testActorID   = "actor-message-sender-synthetic"
	testDeviceID  = "device-message-sender-synthetic"
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

func senderSession(tenantID, actorID, deviceID string) rpcmiddleware.VerifiedSession {
	return rpcmiddleware.VerifiedSession{
		TenantID: tenantID, ActorType: rpcmiddleware.ActorTypeHuman, ActorID: actorID,
		DeviceID: deviceID, SessionID: "session-" + deviceID,
	}
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

func validEnvelope(eventID, key string) *messagev1.ChannelEventEnvelope {
	return &messagev1.ChannelEventEnvelope{
		EventId:        eventID,
		TenantId:       testTenantID,
		Conversation:   &messagev1.ChannelEventEnvelope_ChannelId{ChannelId: testChannelID},
		E2EeGroupId:    "group-message-synthetic",
		Epoch:          1,
		Category:       messagev1.EventCategory_EVENT_CATEGORY_APPLICATION,
		SenderDeviceId: testDeviceID,
		SenderActorId:  testActorID,
		IdempotencyKey: key,
		CryptoProfile:  "tl-plain-slice-a",
		Body:           &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: []byte("opaque-body-synthetic")},
		ContentHash:    bytes.Repeat([]byte{0x11}, contentHashBytes),
	}
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
		detail typev1.ErrorCode
		reason string
	}{
		"server commit supplied": {func(e *messagev1.ChannelEventEnvelope) { e.ServerCommit = &messagev1.ServerCommit{ChannelSeq: 1} },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "server_commit_supplied"},
		"direct message": {func(e *messagev1.ChannelEventEnvelope) {
			e.Conversation = &messagev1.ChannelEventEnvelope_DmId{DmId: "dm-synthetic"}
		},
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "dm_send_not_enabled"},
		"redaction category": {func(e *messagev1.ChannelEventEnvelope) { e.Category = messagev1.EventCategory_EVENT_CATEGORY_REDACTION },
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "category_not_enabled"},
		"agent attribution": {func(e *messagev1.ChannelEventEnvelope) {
			e.AgentAttribution = &messagev1.AgentAttribution{AgentActorId: "agent-synthetic"}
		},
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "agent_attribution_not_enabled"},
		"attachments": {func(e *messagev1.ChannelEventEnvelope) { e.AttachmentBlobIds = []string{"blob-synthetic"} },
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "attachments_not_enabled"},
		"redaction target": {func(e *messagev1.ChannelEventEnvelope) { e.RedactionTargetEventId = "event-0" },
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "redaction_not_enabled"},
		"blank event id": {func(e *messagev1.ChannelEventEnvelope) { e.EventId = "" },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_identifier_invalid"},
		"oversized key": {func(e *messagev1.ChannelEventEnvelope) { e.IdempotencyKey = strings.Repeat("k", maxIdentifierBytes+1) },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_identifier_invalid"},
		"short content hash": {func(e *messagev1.ChannelEventEnvelope) { e.ContentHash = []byte{0x01} },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_body_invalid"},
		"empty body": {func(e *messagev1.ChannelEventEnvelope) {
			e.Body = &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: nil}
		}, connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_body_invalid"},
		"other tenant": {func(e *messagev1.ChannelEventEnvelope) { e.TenantId = "tenant-other-synthetic" },
			connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_TENANT_MISMATCH, "envelope_tenant_mismatch"},
		"other device": {func(e *messagev1.ChannelEventEnvelope) { e.SenderDeviceId = "device-other-synthetic" },
			connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "sender_mismatch"},
		"other actor": {func(e *messagev1.ChannelEventEnvelope) { e.SenderActorId = "actor-other-synthetic" },
			connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "sender_mismatch"},
		"oversized envelope": {func(e *messagev1.ChannelEventEnvelope) {
			e.Body = &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: make([]byte, maxEnvelopeBytes)}
		}, connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_PAYLOAD_TOO_LARGE, "envelope_too_large"},
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
			code, reason := detailReason(t, err)
			if connect.CodeOf(err) != tc.code || code != tc.detail || reason != tc.reason || starter.calls.Load() != 0 {
				t.Fatalf("SendEvent(%s) = %v (%v/%s) after %d BeginTx calls, want %v %v/%s and 0",
					name, err, code, reason, starter.calls.Load(), tc.code, tc.detail, tc.reason)
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

func TestValidateSendBindsTheDeterministicEncoding(t *testing.T) {
	t.Parallel()
	session := senderSession(testTenantID, testActorID, testDeviceID)
	unary := rpcmiddleware.NewAuthenticationInterceptor(fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{"binding-token": session}}).WrapUnary(
		func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
			principal, _ := rpcmiddleware.PrincipalFromContext(ctx)
			first, err := validateSend(principal, validEnvelope("event-1", "key-1"))
			if err != nil {
				t.Fatalf("validateSend() error = %v", err)
			}
			again, err := validateSend(principal, validEnvelope("event-1", "key-1"))
			if err != nil || again.digest != first.digest || !bytes.Equal(again.envelope, first.envelope) {
				t.Fatal("identical envelopes must bind to identical bytes and digest")
			}
			changed := validEnvelope("event-1", "key-1")
			changed.Body = &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: []byte("different-body-synthetic")}
			other, err := validateSend(principal, changed)
			if err != nil || other.digest == first.digest {
				t.Fatal("a changed body must change the digest")
			}
			if first.tenantID != testTenantID || first.channelID != testChannelID || first.deviceID != testDeviceID || first.actorID != testActorID {
				t.Fatalf("bound command = %#v, want principal-derived identity", first)
			}
			return nil, nil
		},
	)
	request := connect.NewRequest(&messagev1.SendEventRequest{})
	request.Header().Set("Authorization", "Bearer binding-token")
	if _, err := unary(context.Background(), request); err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
}
