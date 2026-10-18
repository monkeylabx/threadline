package messagerpc

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"

	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	testTenantID  = "tenant-message-synthetic"
	testChannelID = "channel-message-synthetic"
	testActorID   = "actor-message-sender-synthetic"
	testDeviceID  = "device-message-sender-synthetic"
)

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

// withPrincipal runs body with the Principal the real interceptor derives from
// session, because Principal cannot be constructed outside rpcmiddleware.
func withPrincipal(t *testing.T, session rpcmiddleware.VerifiedSession, body func(rpcmiddleware.Principal)) {
	t.Helper()
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{"principal-token": session}}
	unary := rpcmiddleware.NewAuthenticationInterceptor(verifier).WrapUnary(
		func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
			principal, ok := rpcmiddleware.PrincipalFromContext(ctx)
			if !ok {
				t.Fatal("interceptor did not attach a principal")
			}
			body(principal)
			return nil, nil
		},
	)
	request := connect.NewRequest(&messagev1.SendEventRequest{})
	request.Header().Set("Authorization", "Bearer principal-token")
	if _, err := unary(context.Background(), request); err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
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

func TestValidateSendRejectsEveryUnsupportedOrMismatchedEnvelope(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		mutate func(*messagev1.ChannelEventEnvelope) *messagev1.ChannelEventEnvelope
		code   connect.Code
		detail typev1.ErrorCode
		reason string
	}{
		"nil envelope": {func(*messagev1.ChannelEventEnvelope) *messagev1.ChannelEventEnvelope { return nil },
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_decode_failed"},
		"server commit supplied": {keep(func(e *messagev1.ChannelEventEnvelope) { e.ServerCommit = &messagev1.ServerCommit{ChannelSeq: 1} }),
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "server_commit_supplied"},
		"direct message": {keep(func(e *messagev1.ChannelEventEnvelope) {
			e.Conversation = &messagev1.ChannelEventEnvelope_DmId{DmId: "dm-synthetic"}
		}),
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "dm_send_not_enabled"},
		"redaction category": {keep(func(e *messagev1.ChannelEventEnvelope) { e.Category = messagev1.EventCategory_EVENT_CATEGORY_REDACTION }),
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "category_not_enabled"},
		"mls body": {keep(func(e *messagev1.ChannelEventEnvelope) {
			e.Body = &messagev1.ChannelEventEnvelope_MlsMessage{MlsMessage: nil}
		}), connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_body_invalid"},
		"agent attribution": {keep(func(e *messagev1.ChannelEventEnvelope) {
			e.AgentAttribution = &messagev1.AgentAttribution{AgentActorId: "agent-synthetic"}
		}),
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "agent_attribution_not_enabled"},
		"attachments": {keep(func(e *messagev1.ChannelEventEnvelope) { e.AttachmentBlobIds = []string{"blob-synthetic"} }),
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "attachments_not_enabled"},
		"redaction target": {keep(func(e *messagev1.ChannelEventEnvelope) { e.RedactionTargetEventId = "event-0" }),
			connect.CodeFailedPrecondition, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "redaction_not_enabled"},
		"blank event id": {keep(func(e *messagev1.ChannelEventEnvelope) { e.EventId = "" }),
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_identifier_invalid"},
		"padded channel id": {keep(func(e *messagev1.ChannelEventEnvelope) {
			e.Conversation = &messagev1.ChannelEventEnvelope_ChannelId{ChannelId: " " + testChannelID}
		}), connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_identifier_invalid"},
		"oversized key": {keep(func(e *messagev1.ChannelEventEnvelope) { e.IdempotencyKey = strings.Repeat("k", maxIdentifierBytes+1) }),
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_identifier_invalid"},
		"control character in key": {keep(func(e *messagev1.ChannelEventEnvelope) { e.IdempotencyKey = "key\n1" }),
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_identifier_invalid"},
		"short content hash": {keep(func(e *messagev1.ChannelEventEnvelope) { e.ContentHash = []byte{0x01} }),
			connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_body_invalid"},
		"empty body": {keep(func(e *messagev1.ChannelEventEnvelope) {
			e.Body = &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: nil}
		}), connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_CIPHERTEXT_CORRUPT, "envelope_body_invalid"},
		"other tenant": {keep(func(e *messagev1.ChannelEventEnvelope) { e.TenantId = "tenant-other-synthetic" }),
			connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_TENANT_MISMATCH, "envelope_tenant_mismatch"},
		"other device": {keep(func(e *messagev1.ChannelEventEnvelope) { e.SenderDeviceId = "device-other-synthetic" }),
			connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "sender_mismatch"},
		"other actor": {keep(func(e *messagev1.ChannelEventEnvelope) { e.SenderActorId = "actor-other-synthetic" }),
			connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "sender_mismatch"},
		"oversized envelope": {keep(func(e *messagev1.ChannelEventEnvelope) {
			e.Body = &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: make([]byte, maxEnvelopeBytes)}
		}), connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_PAYLOAD_TOO_LARGE, "envelope_too_large"},
	}
	withPrincipal(t, senderSession(testTenantID, testActorID, testDeviceID), func(principal rpcmiddleware.Principal) {
		for name, tc := range cases {
			_, err := validateSend(principal, tc.mutate(validEnvelope("event-1", "key-1")))
			code, reason := detailReason(t, err)
			if connect.CodeOf(err) != tc.code || code != tc.detail || reason != tc.reason {
				t.Fatalf("validateSend(%s) = %v (%v/%s), want %v %v/%s", name, err, code, reason, tc.code, tc.detail, tc.reason)
			}
		}
	})
}

func keep(mutate func(*messagev1.ChannelEventEnvelope)) func(*messagev1.ChannelEventEnvelope) *messagev1.ChannelEventEnvelope {
	return func(envelope *messagev1.ChannelEventEnvelope) *messagev1.ChannelEventEnvelope {
		mutate(envelope)
		return envelope
	}
}

func TestValidateSendRejectsNonHumanSenders(t *testing.T) {
	t.Parallel()
	agent := senderSession(testTenantID, testActorID, testDeviceID)
	agent.ActorType = rpcmiddleware.ActorTypeAgent
	withPrincipal(t, agent, func(principal rpcmiddleware.Principal) {
		_, err := validateSend(principal, validEnvelope("event-1", "key-1"))
		if _, reason := detailReason(t, err); connect.CodeOf(err) != connect.CodePermissionDenied || reason != "sender_mismatch" {
			t.Fatalf("validateSend(agent principal) = %v, want PermissionDenied sender_mismatch", err)
		}
	})
}

func TestValidateSendBindsTheDeterministicEncoding(t *testing.T) {
	t.Parallel()
	withPrincipal(t, senderSession(testTenantID, testActorID, testDeviceID), func(principal rpcmiddleware.Principal) {
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
		if first.tenantID != testTenantID || first.channelID != testChannelID || first.eventID != "event-1" ||
			first.idempotencyKey != "key-1" || first.deviceID != testDeviceID || first.actorID != testActorID ||
			!bytes.Equal(first.contentHash, bytes.Repeat([]byte{0x11}, contentHashBytes)) {
			t.Fatalf("bound command = %#v, want principal-derived identity and envelope facts", first)
		}
	})
}
