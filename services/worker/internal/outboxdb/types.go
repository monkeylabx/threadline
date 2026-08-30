// Package outboxdb is the Worker's private, fenced authority boundary for
// transactional Outbox claim, renewal, acknowledgement, and failure updates.
package outboxdb

import (
	"encoding/hex"
	"strings"
	"time"
	"unicode"
)

const (
	maximumClaimBatchSize = 256
	maximumOwnerBytes     = 128
	maximumStreamBytes    = 255
	policyDigestBytes     = 32
)

type errorCode string

const (
	errorInvalidInput errorCode = "invalid-input"
	errorClaimDenied  errorCode = "claim-denied"
	errorPersistence  errorCode = "persistence-failure"
)

type operationFailure struct{ code errorCode }

func (failure *operationFailure) Error() string {
	return "transactional outbox: " + string(failure.category())
}

func (failure *operationFailure) category() errorCode {
	if failure == nil {
		return ""
	}
	return failure.code
}

func operationError(code errorCode) *operationFailure {
	return &operationFailure{code: code}
}

type claimToken struct{ wire string }

func newClaimToken(raw []byte) (claimToken, bool) {
	if len(raw) != claimTokenRawBytes {
		return claimToken{}, false
	}
	wire := strictRawURLBase64.EncodeToString(raw)
	if len(wire) != claimTokenWireBytes {
		return claimToken{}, false
	}
	return claimToken{wire: wire}, true
}

func (claimToken) String() string   { return "<redacted-claim-token>" }
func (claimToken) GoString() string { return "<redacted-claim-token>" }

func (token claimToken) candidateDigest() ([32]byte, bool) {
	return claimTokenCandidateDigest(token.wire)
}

type claimRequest struct {
	claimOwnerID string
	batchSize    int32
}

type claimFence struct {
	tenantID          string
	eventID           string
	outboxEntryID     int64
	deliveryAttemptID int64
	replayGeneration  int64
	claimOwnerID      string
	token             claimToken
}

func (claimFence) String() string   { return "<redacted-claim-fence>" }
func (claimFence) GoString() string { return "<redacted-claim-fence>" }

type claimedEvent struct {
	fence                   claimFence
	totalAttemptNumber      int64
	generationAttemptNumber int64
	claimedAt               time.Time
	leaseExpiresAt          time.Time
	absoluteLeaseExpiresAt  time.Time
	brokerMessageID         string
	destination             string
	eventType               string
	schemaVersion           int32
	aggregateKind           string
	aggregateID             string
	payload                 []byte
	occurredAt              time.Time
	enqueuedAt              time.Time
	policyID                string
	policySnapshotDigest    [policyDigestBytes]byte
}

func (claimedEvent) String() string   { return "<redacted-claimed-event>" }
func (claimedEvent) GoString() string { return "<redacted-claimed-event>" }

type renewal struct{ leaseExpiresAt time.Time }

type pubAck struct {
	stream    string
	sequence  uint64
	duplicate bool
	messageID string
}

type acknowledgement uint8

const (
	acknowledgementDelivered acknowledgement = iota + 1
	acknowledgementAlreadyDelivered
)

type acknowledgementRequest struct {
	fence  claimFence
	pubAck pubAck
}

func (acknowledgementRequest) String() string   { return "<redacted-acknowledgement-request>" }
func (acknowledgementRequest) GoString() string { return "<redacted-acknowledgement-request>" }

type failureCode string

const (
	failureTransportUnavailable  failureCode = "transport-unavailable"
	failurePublishOutcomeUnknown failureCode = "publish-outcome-unknown"
	failureEventRetryable        failureCode = "event-retryable"
	failureEventPermanent        failureCode = "event-permanent"
)

type failureDisposition uint8

const (
	failureDispositionRetryScheduled failureDisposition = iota + 1
	failureDispositionParked
)

type failureResult struct {
	disposition   failureDisposition
	nextAttemptAt time.Time
}

type publishFailureRequest struct {
	fence claimFence
	code  failureCode
}

func (publishFailureRequest) String() string   { return "<redacted-publish-failure-request>" }
func (publishFailureRequest) GoString() string { return "<redacted-publish-failure-request>" }

func validClaimRequest(request claimRequest) bool {
	return validBoundedIdentifier(request.claimOwnerID, maximumOwnerBytes) &&
		request.batchSize >= 1 && request.batchSize <= maximumClaimBatchSize
}

func validFenceFacts(fence claimFence) bool {
	return validBoundedIdentifier(fence.tenantID, 0) &&
		validBoundedIdentifier(fence.eventID, 0) &&
		fence.outboxEntryID > 0 &&
		fence.deliveryAttemptID > 0 &&
		fence.replayGeneration >= 0 &&
		validBoundedIdentifier(fence.claimOwnerID, maximumOwnerBytes)
}

func validPubAck(ack pubAck) bool {
	return validBoundedIdentifier(ack.stream, maximumStreamBytes) && ack.sequence > 0 && validMessageID(ack.messageID)
}

func validMessageID(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validFailureCode(code failureCode) bool {
	switch code {
	case failureTransportUnavailable,
		failurePublishOutcomeUnknown,
		failureEventRetryable,
		failureEventPermanent:
		return true
	default:
		return false
	}
}

func validBoundedIdentifier(value string, maximumBytes int) bool {
	return value != "" &&
		value == strings.TrimSpace(value) &&
		(maximumBytes == 0 || len(value) <= maximumBytes) &&
		!strings.ContainsFunc(value, unicode.IsControl)
}
