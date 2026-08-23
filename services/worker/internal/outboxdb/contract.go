package outboxdb

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

// Store is the Worker's narrow, fenced authority seam for one claimed Outbox
// destination. Claims remain opaque; callers can only inspect cloned publish
// facts and lease timing required by the relay.
type Store interface {
	Claim(context.Context, ClaimRequest) ([]Claim, error)
	Renew(context.Context, Claim) (Renewal, error)
	Acknowledge(context.Context, Claim, outboxpublish.Acknowledgement) (Acknowledgement, error)
	RecordFailure(context.Context, Claim, FailureCode) (FailureResult, error)
}

// Binding binds a Store to the same trusted logical destination and
// broker Stream as its Publisher. Per-call input cannot change either value.
type Binding struct {
	LogicalDestination string
	Stream             string
}

func (Binding) String() string   { return "<redacted-outbox-store-binding>" }
func (Binding) GoString() string { return "<redacted-outbox-store-binding>" }
func (Binding) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-store-binding]"`), nil
}

// Valid reports whether the binding names the sole frozen v1 destination and
// one exact, concrete broker Stream.
func (binding Binding) Valid() bool {
	return binding.LogicalDestination == outboxpublish.LogicalDestinationDomainEvents &&
		validBoundedIdentifier(binding.Stream, maximumStreamBytes) &&
		!strings.ContainsFunc(binding.Stream, unicode.IsSpace) &&
		!strings.ContainsAny(binding.Stream, ".*/\\>")
}

// ClaimRequest contains trusted Worker process policy, never request or Event
// data.
type ClaimRequest struct {
	ClaimOwnerID string
	BatchSize    int32
}

func (ClaimRequest) String() string   { return "<redacted-outbox-claim-request>" }
func (ClaimRequest) GoString() string { return "<redacted-outbox-claim-request>" }
func (ClaimRequest) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-claim-request]"`), nil
}

// Claim is opaque authority minted by exactly one Store. Copying a Claim does
// not expose or make its fencing tuple or one-time token caller-settable.
type Claim struct{ authority *claimAuthority }

func (Claim) String() string   { return "<redacted-outbox-claim>" }
func (Claim) GoString() string { return "<redacted-outbox-claim>" }
func (Claim) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-claim]"`), nil
}

// PublishFacts returns an ownership-independent clone of the facts an Event
// encoder is permitted to inspect. It is not a serialized wire envelope.
func (claim Claim) PublishFacts() PublishFacts {
	if claim.authority == nil {
		return PublishFacts{}
	}
	return claim.authority.facts.Clone()
}

// Lease returns the database-authored timing facts needed to schedule renewal.
func (claim Claim) Lease() Lease {
	if claim.authority == nil {
		return Lease{}
	}
	claim.authority.stateMutex.RLock()
	defer claim.authority.stateMutex.RUnlock()
	return claim.authority.lease
}

// PublishFacts is the exact redacted encoder input frozen by the Outbox
// contract. Payload is opaque, and Clone must be used when retaining it.
type PublishFacts struct {
	TenantID           string
	EventID            string
	OutboxEntryID      int64
	LogicalDestination string
	BrokerMessageID    string
	EventType          string
	SchemaVersion      int32
	AggregateKind      string
	AggregateID        string
	Payload            []byte
	OccurredAt         time.Time
	EnqueuedAt         time.Time
}

func (PublishFacts) String() string   { return "<redacted-outbox-publish-facts>" }
func (PublishFacts) GoString() string { return "<redacted-outbox-publish-facts>" }
func (PublishFacts) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-publish-facts]"`), nil
}

// Clone returns a deep copy suitable for another owner.
func (facts PublishFacts) Clone() PublishFacts {
	facts.Payload = append([]byte(nil), facts.Payload...)
	return facts
}

// Valid reports whether the facts have the frozen shape required before an
// encoder may inspect them. Empty opaque payloads are valid.
func (facts PublishFacts) Valid() bool {
	return validBoundedIdentifier(facts.TenantID, 0) &&
		validBoundedIdentifier(facts.EventID, 0) &&
		facts.OutboxEntryID > 0 &&
		facts.LogicalDestination == outboxpublish.LogicalDestinationDomainEvents &&
		validMessageID(facts.BrokerMessageID) &&
		validEventType(facts.EventType) &&
		facts.SchemaVersion > 0 &&
		validBoundedIdentifier(facts.AggregateKind, 0) &&
		validBoundedIdentifier(facts.AggregateID, 0) &&
		len(facts.Payload) <= payloadHardBytes &&
		!facts.OccurredAt.IsZero() && !facts.EnqueuedAt.IsZero()
}

// Lease is the immutable claim time plus its current and absolute database
// deadlines.
type Lease struct {
	ClaimedAt              time.Time
	ExpiresAt              time.Time
	AbsoluteLeaseExpiresAt time.Time
}

func (Lease) String() string   { return "<redacted-outbox-lease>" }
func (Lease) GoString() string { return "<redacted-outbox-lease>" }
func (Lease) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-lease]"`), nil
}

// Valid reports whether the database-authored lease deadlines are ordered.
func (lease Lease) Valid() bool {
	return !lease.ClaimedAt.IsZero() && lease.ClaimedAt.Before(lease.ExpiresAt) &&
		!lease.AbsoluteLeaseExpiresAt.Before(lease.ExpiresAt)
}

// Renewal is a successful database-authored claim extension.
type Renewal struct{ LeaseExpiresAt time.Time }

func (Renewal) String() string   { return "<redacted-outbox-renewal>" }
func (Renewal) GoString() string { return "<redacted-outbox-renewal>" }
func (Renewal) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-renewal]"`), nil
}

// Acknowledgement is the exact durable database outcome.
type Acknowledgement uint8

const (
	AcknowledgementDelivered Acknowledgement = iota + 1
	AcknowledgementAlreadyDelivered
)

func (Acknowledgement) String() string   { return "<redacted-outbox-acknowledgement-result>" }
func (Acknowledgement) GoString() string { return "<redacted-outbox-acknowledgement-result>" }
func (Acknowledgement) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-acknowledgement-result]"`), nil
}

// FailureCode is the frozen database failure allowlist. Publisher invalid-input
// is deliberately absent because it is an invariant/readiness failure.
type FailureCode string

const (
	FailureTransportUnavailable  FailureCode = "transport-unavailable"
	FailurePublishOutcomeUnknown FailureCode = "publish-outcome-unknown"
	FailureEventRetryable        FailureCode = "event-retryable"
	FailureEventPermanent        FailureCode = "event-permanent"
)

func (FailureCode) String() string   { return "<redacted-outbox-failure-code>" }
func (FailureCode) GoString() string { return "<redacted-outbox-failure-code>" }
func (FailureCode) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-failure-code]"`), nil
}

// FailureDisposition is the durable result of recording an Event failure.
type FailureDisposition uint8

const (
	FailureRetryScheduled FailureDisposition = iota + 1
	FailureParked
)

func (FailureDisposition) String() string   { return "<redacted-outbox-failure-disposition>" }
func (FailureDisposition) GoString() string { return "<redacted-outbox-failure-disposition>" }
func (FailureDisposition) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-failure-disposition]"`), nil
}

// FailureResult contains only the database-authored schedule/disposition.
type FailureResult struct {
	Disposition   FailureDisposition
	NextAttemptAt time.Time
}

func (FailureResult) String() string   { return "<redacted-outbox-failure-result>" }
func (FailureResult) GoString() string { return "<redacted-outbox-failure-result>" }
func (FailureResult) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-failure-result]"`), nil
}

// StoreErrorCode is a stable, secret-safe Store failure category.
type StoreErrorCode string

const (
	StoreErrorInvalidInput StoreErrorCode = "invalid-input"
	StoreErrorClaimDenied  StoreErrorCode = "claim-denied"
	StoreErrorPersistence  StoreErrorCode = "persistence-failure"
)

func (StoreErrorCode) String() string   { return "<redacted-outbox-store-error-code>" }
func (StoreErrorCode) GoString() string { return "<redacted-outbox-store-error-code>" }
func (StoreErrorCode) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-store-error-code]"`), nil
}

// StoreErrorCodeOf extracts only errors produced by this Store module.
// Context cancellation and deadlines remain standard context errors.
func StoreErrorCodeOf(err error) (StoreErrorCode, bool) {
	var failure *operationFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	switch failure.category() {
	case errorInvalidInput:
		return StoreErrorInvalidInput, true
	case errorClaimDenied:
		return StoreErrorClaimDenied, true
	case errorPersistence:
		return StoreErrorPersistence, true
	default:
		return "", false
	}
}

type storeIdentity struct{ kind storeIdentityKind }

type storeIdentityKind uint8

const (
	productionStoreIdentity storeIdentityKind = iota + 1
	scriptedStoreIdentity
)

type claimAuthority struct {
	stateMutex    sync.RWMutex
	operationGate chan struct{}
	owner         *storeIdentity
	fence         claimFence
	facts         PublishFacts
	lease         Lease
}

func newOpaqueClaim(owner *storeIdentity, fence claimFence, facts PublishFacts, lease Lease) Claim {
	authority := &claimAuthority{
		operationGate: make(chan struct{}, 1),
		owner:         owner,
		fence:         fence,
		facts:         facts.Clone(),
		lease:         lease,
	}
	authority.operationGate <- struct{}{}
	return Claim{authority: authority}
}

func claimOwnedBy(claim Claim, owner *storeIdentity) bool {
	if owner == nil || claim.authority == nil || claim.authority.owner != owner ||
		claim.authority.operationGate == nil ||
		!claim.authority.facts.Valid() {
		return false
	}
	claim.authority.stateMutex.RLock()
	defer claim.authority.stateMutex.RUnlock()
	return claim.authority.lease.Valid()
}

func acquireClaimOperation(
	ctx context.Context,
	claim Claim,
	owner *storeIdentity,
) (*claimAuthority, func(), error) {
	if !claimOwnedBy(claim, owner) {
		return nil, nil, operationError(errorClaimDenied)
	}
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-claim.authority.operationGate:
		return claim.authority, func() {
			claim.authority.operationGate <- struct{}{}
		}, nil
	}
}

// updateClaimLease performs the shared production/scripted monotonic renewal
// transition without exposing mutable authority state to callers.
func updateClaimLease(claim Claim, owner *storeIdentity, expiresAt time.Time) bool {
	if owner == nil || claim.authority == nil || claim.authority.owner != owner ||
		!claim.authority.facts.Valid() || expiresAt.IsZero() {
		return false
	}
	claim.authority.stateMutex.Lock()
	defer claim.authority.stateMutex.Unlock()
	return updateClaimLeaseLocked(claim.authority, expiresAt)
}

func updateClaimLeaseLocked(authority *claimAuthority, expiresAt time.Time) bool {
	if authority == nil || !authority.lease.Valid() || expiresAt.IsZero() ||
		!expiresAt.After(authority.lease.ExpiresAt) ||
		expiresAt.After(authority.lease.AbsoluteLeaseExpiresAt) {
		return false
	}
	authority.lease.ExpiresAt = expiresAt
	return true
}

func validFailureCodeContract(code FailureCode) bool {
	switch code {
	case FailureTransportUnavailable,
		FailurePublishOutcomeUnknown,
		FailureEventRetryable,
		FailureEventPermanent:
		return true
	default:
		return false
	}
}
