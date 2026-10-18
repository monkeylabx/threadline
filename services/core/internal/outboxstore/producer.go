package outboxstore

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/internal/outboxpolicy"
)

// Stable failure categories exposed to RPC layers through ErrorCodeOf.
const (
	ErrorCodeInvalidInput        = string(errorInvalidInput)
	ErrorCodeIdempotencyConflict = string(errorIdempotencyConflict)
	ErrorCodePersistence         = string(errorPersistence)
)

const (
	messageCommittedEventType     = "message.committed"
	messageCommittedSchemaVersion = 1
	channelAggregateKind          = "channel"
	maxIdentifierBytes            = 128
)

// ChannelEventCommitted is the typed fact set the SendEvent transaction hands
// to the registered message.committed v1 producer: the Principal's tenant, the
// locked Channel, the allocated sequence and the database commit timestamp.
// Event type, schema version, aggregate kind, destination, payload layout,
// Domain Event ID and policy are never caller inputs.
type ChannelEventCommitted struct {
	TenantID    string
	ChannelID   string
	ChannelSeq  uint64
	CommittedAt time.Time
}

// Appended reports the Domain Event and initial domain-events Entry written
// for one committed channel event.
type Appended struct {
	DomainEventID string
	OutboxEntryID int64
	EnqueuedAt    time.Time
}

// Producer appends message.committed v1 Domain Events under one trusted policy
// snapshot taken at startup.
type Producer struct {
	policy policySnapshot
}

// NewProducer binds the producer to a parsed, valid policy snapshot. An invalid
// snapshot means the producer is not ready, so no Producer is built.
func NewProducer(policy outboxpolicy.Snapshot) (*Producer, error) {
	if !policy.Valid() {
		return nil, storeError(errorInvalidInput)
	}
	snapshot := policySnapshotFrom(policy)
	if !validPolicy(snapshot) {
		return nil, storeError(errorInvalidInput)
	}
	return &Producer{policy: snapshot}, nil
}

// AppendChannelEventCommitted appends the Domain Event and its initial Entry
// inside tx. The caller still owns commit and rollback. A Domain Event ID
// collision is a persistence failure, never a deduplicated retry: message
// deduplication lives in the send-command binding, not in this producer.
func (producer *Producer) AppendChannelEventCommitted(
	ctx context.Context,
	tx pgx.Tx,
	fact ChannelEventCommitted,
) (Appended, error) {
	if err := ctx.Err(); err != nil {
		return Appended{}, err
	}
	if producer == nil || tx == nil || !validChannelEventFact(fact) {
		return Appended{}, storeError(errorInvalidInput)
	}
	domainEventID, err := newDomainEventID()
	if err != nil {
		return Appended{}, storeError(errorPersistence)
	}
	appended, err := insert(ctx, tx, candidate{
		tenantID: fact.TenantID,
		eventID:  domainEventID,
		descriptor: eventDescriptor{
			eventType:     messageCommittedEventType,
			schemaVersion: messageCommittedSchemaVersion,
			aggregateKind: channelAggregateKind,
		},
		aggregateID: fact.ChannelID,
		payload:     encodeChannelEventPayload(fact.ChannelID, fact.ChannelSeq),
		occurredAt:  fact.CommittedAt.UTC(),
		policy:      producer.policy,
	})
	if err != nil {
		return Appended{}, err
	}
	if appended.disposition != dispositionCreated {
		return Appended{}, storeError(errorPersistence)
	}
	return Appended{
		DomainEventID: appended.eventID,
		OutboxEntryID: appended.outboxEntryID,
		EnqueuedAt:    appended.enqueuedAt,
	}, nil
}

// ErrorCodeOf exposes the stable failure category of a store error so RPC
// layers can map it without parsing text.
func ErrorCodeOf(err error) (string, bool) {
	var failure *storeFailure
	if !errors.As(err, &failure) || failure == nil {
		return "", false
	}
	return string(failure.category()), true
}

func validChannelEventFact(fact ChannelEventCommitted) bool {
	return validIdentifier(fact.TenantID) && len(fact.TenantID) <= maxIdentifierBytes &&
		validIdentifier(fact.ChannelID) && len(fact.ChannelID) <= maxIdentifierBytes &&
		fact.ChannelSeq >= 1 && !fact.CommittedAt.IsZero()
}

// policySnapshotFrom copies the trusted snapshot into the store's row shape.
// Out-of-range values become -1 so validPolicy rejects them.
func policySnapshotFrom(policy outboxpolicy.Snapshot) policySnapshot {
	digest := policy.Digest()
	return policySnapshot{
		id:                 policy.PolicyID(),
		snapshotDigest:     digest[:],
		leaseMS:            millis(policy.Lease()),
		absoluteLifetimeMS: millis(policy.AbsoluteLifetime()),
		eventRetryCeiling:  int32Of(policy.EventRetryCeiling()),
		transportBaseMS:    millis(policy.TransportBackoff().Base()),
		transportCapMS:     millis(policy.TransportBackoff().Cap()),
		unknownBaseMS:      millis(policy.UnknownBackoff().Base()),
		unknownCapMS:       millis(policy.UnknownBackoff().Cap()),
		eventBaseMS:        millis(policy.EventBackoff().Base()),
		eventCapMS:         millis(policy.EventBackoff().Cap()),
		retentionDays:      int32Of(policy.RetentionDays()),
	}
}

func millis(duration time.Duration) int32 {
	value := duration / time.Millisecond
	if value < 0 || value > math.MaxInt32 {
		return -1
	}
	return int32(value)
}

func int32Of(value uint32) int32 {
	if value > math.MaxInt32 {
		return -1
	}
	return int32(value)
}
