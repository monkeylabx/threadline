package auditstore

import (
	"bytes"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type ActorType = actorType

const (
	ActorTypeHuman   ActorType = actorHuman
	ActorTypeAgent   ActorType = actorAgent
	ActorTypeService ActorType = actorService
)

type Action = action

const (
	ActionChannelArchive            Action = actionChannelArchive
	ActionCapabilityGrantIssue      Action = actionCapabilityGrantIssue
	ActionCapabilityGrantRevoke     Action = actionCapabilityGrantRevoke
	ActionRetentionExpire           Action = actionRetentionExpire
	ActionRetentionLegalHoldApply   Action = actionRetentionLegalHoldApply
	ActionRetentionLegalHoldRelease Action = actionRetentionLegalHoldRelease
	ActionRecoveryRequest           Action = actionRecoveryRequest
	ActionRecoveryDecision          Action = actionRecoveryDecision
	ActionRecoveryCommit            Action = actionRecoveryCommit
	ActionOutboxReplayRequest       Action = actionOutboxReplayRequest
)

type Outcome = outcome

const (
	OutcomeSucceeded Outcome = outcomeSucceeded
	OutcomeDenied    Outcome = outcomeDenied
	OutcomeFailed    Outcome = outcomeFailed
)

type Reason = reason

const (
	ReasonAuthorized          Reason = reasonAuthorized
	ReasonAuthorizationDenied Reason = reasonAuthorizationDenied
	ReasonEvidenceInvalid     Reason = reasonEvidenceInvalid
	ReasonPolicyDenied        Reason = reasonPolicyDenied
	ReasonRetentionExpired    Reason = reasonRetentionExpired
	ReasonStateConflict       Reason = reasonStateConflict
	ReasonInvalidInput        Reason = reasonInvalidInput
	ReasonInternalFailure     Reason = reasonInternalFailure
)

type TargetType = targetType

const (
	TargetTypeChannel          TargetType = targetChannel
	TargetTypeCapabilityGrant  TargetType = targetCapabilityGrant
	TargetTypeRetentionSubject TargetType = targetRetentionSubject
	TargetTypeRecoveryCase     TargetType = targetRecoveryCase
	TargetTypeOutboxEntry      TargetType = targetOutboxEntry
)

type Principal = actor
type Target = target
type Candidate = candidate
type Event = event
type Disposition = disposition
type ErrorCode = errorCode
type Error = storeFailure

const (
	DispositionCreated           Disposition = dispositionCreated
	DispositionAlreadyPresent    Disposition = dispositionAlreadyPresent
	ErrorCodeInvalidInput        ErrorCode   = errorInvalidInput
	ErrorCodeIdempotencyConflict ErrorCode   = errorIdempotencyConflict
	ErrorCodePersistence         ErrorCode   = errorPersistence
)

type CandidateInput struct {
	TenantID       string
	AuditEventID   string
	Principal      Principal
	Action         Action
	Outcome        Outcome
	Reason         Reason
	Target         Target
	PolicyVersion  string
	RequestID      string
	ApprovalID     *string
	RecoveryCaseID *string
	EvidenceDigest []byte
}

func NewPrincipal(actorType ActorType, actorID string) (Principal, error) {
	value := actor{typeID: actorType, id: actorID}
	if !validActor(value) {
		return Principal{}, storeError(errorInvalidInput)
	}
	return value, nil
}

func (value actor) Type() ActorType { return value.typeID }
func (value actor) ID() string      { return value.id }

func NewTarget(kind TargetType, targetID string, version *int64) (Target, error) {
	value := target{typeID: kind, id: targetID, version: cloneInt64(version)}
	if !validTarget(value) {
		return Target{}, storeError(errorInvalidInput)
	}
	return value, nil
}

func (value target) Type() TargetType { return value.typeID }
func (value target) ID() string       { return value.id }
func (value target) Version() *int64  { return cloneInt64(value.version) }

func NewCandidate(input CandidateInput) (Candidate, error) {
	value := candidate{
		tenantID: input.TenantID, auditEventID: input.AuditEventID,
		principal: input.Principal, action: input.Action, outcome: input.Outcome, reason: input.Reason,
		target:        target{typeID: input.Target.typeID, id: input.Target.id, version: cloneInt64(input.Target.version)},
		policyVersion: input.PolicyVersion, requestID: input.RequestID,
		approvalID: cloneString(input.ApprovalID), recoveryCaseID: cloneString(input.RecoveryCaseID),
		evidenceDigest: bytes.Clone(input.EvidenceDigest),
	}
	if !validCandidate(value) {
		return Candidate{}, storeError(errorInvalidInput)
	}
	return value, nil
}

func Append(ctx context.Context, tx pgx.Tx, input Candidate) (Event, error) {
	return appendEvent(ctx, tx, input)
}

func (e *storeFailure) Code() ErrorCode { return e.category() }

func (value event) ContractVersion() int16 { return 1 }
func (value event) TenantID() string       { return value.tenantID }
func (value event) AuditEventID() string   { return value.auditEventID }
func (value event) TenantSequence() int64  { return value.tenantSequence }
func (value event) RecordedAt() time.Time  { return value.recordedAt }
func (value event) Principal() Principal   { return value.principal }
func (value event) Action() Action         { return value.action }
func (value event) Outcome() Outcome       { return value.outcome }
func (value event) Reason() Reason         { return value.reason }
func (value event) Target() Target {
	return target{typeID: value.target.typeID, id: value.target.id, version: cloneInt64(value.target.version)}
}
func (value event) PolicyVersion() string     { return value.policyVersion }
func (value event) RequestID() string         { return value.requestID }
func (value event) ApprovalID() *string       { return cloneString(value.approvalID) }
func (value event) RecoveryCaseID() *string   { return cloneString(value.recoveryCaseID) }
func (value event) EvidenceDigest() []byte    { return bytes.Clone(value.evidenceDigest) }
func (value event) PreviousEventHash() []byte { return bytes.Clone(value.previousEventHash) }
func (value event) EventHash() []byte         { return bytes.Clone(value.eventHash) }
func (value event) Disposition() Disposition  { return value.disposition }
