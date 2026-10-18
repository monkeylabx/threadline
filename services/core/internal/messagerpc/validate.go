package messagerpc

import (
	"crypto/sha256"
	"strings"
	"unicode"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	typev1 "github.com/monkeylabx/threadline/services/gen/threadline/type/v1"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	maxEnvelopeBytes   = 262_144
	maxIdentifierBytes = 128
	contentHashBytes   = 32
)

// sendCommand is the validated, principal-bound form of one SendEvent request.
// envelope holds the deterministic encoding with server_commit absent; digest
// is its SHA-256 and is the exact-retry comparison value.
type sendCommand struct {
	tenantID       string
	channelID      string
	eventID        string
	idempotencyKey string
	actorID        string
	deviceID       string
	contentHash    []byte
	envelope       []byte
	digest         [32]byte
}

// validateSend checks shape and Slice A gates before any lock is taken, then
// binds the envelope to the authenticated principal: identifiers in the
// envelope are compared with trusted context, never trusted on their own.
func validateSend(principal rpcmiddleware.Principal, envelope *messagev1.ChannelEventEnvelope) (sendCommand, error) {
	if envelope == nil {
		return sendCommand{}, invalidEnvelope("envelope_decode_failed")
	}
	if envelope.GetServerCommit() != nil {
		return sendCommand{}, invalidEnvelope("server_commit_supplied")
	}
	if err := checkSliceGates(envelope); err != nil {
		return sendCommand{}, err
	}
	if !validIdentifier(envelope.GetEventId()) || !validIdentifier(envelope.GetIdempotencyKey()) ||
		!validIdentifier(envelope.GetChannelId()) {
		return sendCommand{}, invalidEnvelope("envelope_identifier_invalid")
	}
	if len(envelope.GetContentHash()) != contentHashBytes || len(envelope.GetApplicationCiphertext()) == 0 {
		return sendCommand{}, invalidEnvelope("envelope_body_invalid")
	}
	if envelope.GetTenantId() != principal.TenantID() {
		return sendCommand{}, rpcError(connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_TENANT_MISMATCH, "envelope_tenant_mismatch")
	}
	if !senderMatches(principal, envelope) {
		return sendCommand{}, rpcError(connect.CodePermissionDenied, typev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "sender_mismatch")
	}
	return bindCommand(principal, envelope)
}

// checkSliceGates rejects every envelope shape Slice A does not implement.
func checkSliceGates(envelope *messagev1.ChannelEventEnvelope) error {
	switch {
	case envelope.GetDmId() != "":
		return sliceGate("dm_send_not_enabled")
	case envelope.GetCategory() != messagev1.EventCategory_EVENT_CATEGORY_APPLICATION || envelope.GetMlsMessage() != nil:
		return sliceGate("category_not_enabled")
	case envelope.GetAgentAttribution() != nil:
		return sliceGate("agent_attribution_not_enabled")
	case len(envelope.GetAttachmentBlobIds()) > 0:
		return sliceGate("attachments_not_enabled")
	case envelope.GetRedactionTargetEventId() != "":
		return sliceGate("redaction_not_enabled")
	default:
		return nil
	}
}

func senderMatches(principal rpcmiddleware.Principal, envelope *messagev1.ChannelEventEnvelope) bool {
	return principal.Actor().Type() == rpcmiddleware.ActorTypeHuman &&
		envelope.GetSenderActorId() == principal.Actor().ID() &&
		envelope.GetSenderDeviceId() == principal.DeviceID()
}

func bindCommand(principal rpcmiddleware.Principal, envelope *messagev1.ChannelEventEnvelope) (sendCommand, error) {
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(envelope)
	if err != nil {
		return sendCommand{}, invalidEnvelope("envelope_decode_failed")
	}
	if len(encoded) > maxEnvelopeBytes {
		return sendCommand{}, rpcError(connect.CodeInvalidArgument, typev1.ErrorCode_ERROR_CODE_PAYLOAD_TOO_LARGE, "envelope_too_large")
	}
	return sendCommand{
		tenantID:       principal.TenantID(),
		channelID:      envelope.GetChannelId(),
		eventID:        envelope.GetEventId(),
		idempotencyKey: envelope.GetIdempotencyKey(),
		actorID:        principal.Actor().ID(),
		deviceID:       principal.DeviceID(),
		contentHash:    envelope.GetContentHash(),
		envelope:       encoded,
		digest:         sha256.Sum256(encoded),
	}, nil
}

func validIdentifier(value string) bool {
	return value != "" && len(value) <= maxIdentifierBytes && value == strings.TrimSpace(value) &&
		!strings.ContainsFunc(value, unicode.IsControl)
}
