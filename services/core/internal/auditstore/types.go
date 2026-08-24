// Package auditstore appends minimized, tenant-chained Audit Events inside a
// caller-owned Core transaction.
package auditstore

import "time"

const hashBytes = 32

type actorType int16

const (
	actorHuman actorType = iota + 1
	actorAgent
	actorService
)

type action string

const (
	actionChannelArchive            action = "channel.archive"
	actionCapabilityGrantIssue      action = "capability_grant.issue"
	actionCapabilityGrantRevoke     action = "capability_grant.revoke"
	actionRetentionExpire           action = "retention.expire"
	actionRetentionLegalHoldApply   action = "retention.legal_hold.apply"
	actionRetentionLegalHoldRelease action = "retention.legal_hold.release"
	actionRecoveryRequest           action = "recovery.request"
	actionRecoveryDecision          action = "recovery.decision"
	actionRecoveryCommit            action = "recovery.commit"
	actionOutboxReplayRequest       action = "outbox.replay.request"
)

type outcome string

const (
	outcomeSucceeded outcome = "succeeded"
	outcomeDenied    outcome = "denied"
	outcomeFailed    outcome = "failed"
)

type reason string

const (
	reasonAuthorized          reason = "authorized"
	reasonAuthorizationDenied reason = "authorization_denied"
	reasonEvidenceInvalid     reason = "evidence_invalid"
	reasonPolicyDenied        reason = "policy_denied"
	reasonRetentionExpired    reason = "retention_expired"
	reasonStateConflict       reason = "state_conflict"
	reasonInvalidInput        reason = "invalid_input"
	reasonInternalFailure     reason = "internal_failure"
)

type targetType string

const (
	targetChannel          targetType = "channel"
	targetCapabilityGrant  targetType = "capability_grant"
	targetRetentionSubject targetType = "retention_subject"
	targetRecoveryCase     targetType = "recovery_case"
	targetOutboxEntry      targetType = "outbox_entry"
)

type actor struct {
	typeID actorType
	id     string
}

type target struct {
	typeID  targetType
	id      string
	version *int64
}

// candidate contains only non-database-owned, minimized v1 facts. It remains
// package-private until a command Contract provides trusted constructors.
type candidate struct {
	tenantID       string
	auditEventID   string
	principal      actor
	action         action
	outcome        outcome
	reason         reason
	target         target
	policyVersion  string
	requestID      string
	approvalID     *string
	recoveryCaseID *string
	evidenceDigest []byte
}

type appendSlot struct {
	tenantID             string
	tenantSequence       int64
	recordedAt           time.Time
	transactionID        int64
	previousAuditEventID *string
	previousEventHash    []byte
}

type disposition uint8

const (
	dispositionCreated disposition = iota + 1
	dispositionAlreadyPresent
)

// event is an immutable-by-copy result. No accessor returns a mutable slice.
type event struct {
	candidate
	tenantSequence    int64
	recordedAt        time.Time
	previousEventHash []byte
	eventHash         []byte
	disposition       disposition
}

type errorCode string

const (
	errorInvalidInput        errorCode = "invalid-input"
	errorIdempotencyConflict errorCode = "idempotency-conflict"
	errorPersistence         errorCode = "persistence-failure"
)

type storeFailure struct{ code errorCode }

func (e *storeFailure) Error() string { return "audit store: " + string(e.category()) }

func (e *storeFailure) category() errorCode {
	if e == nil {
		return ""
	}
	return e.code
}
