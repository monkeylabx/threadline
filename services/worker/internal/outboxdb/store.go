package outboxdb

import (
	"context"
	"reflect"

	"github.com/monkeylabx/threadline/services/worker/internal/dbgen"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

type productionStore struct {
	adapter  *adapter
	binding  Binding
	identity *storeIdentity
}

var _ Store = (*productionStore)(nil)

func (*productionStore) String() string   { return "<redacted-outbox-production-store>" }
func (*productionStore) GoString() string { return "<redacted-outbox-production-store>" }
func (*productionStore) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-production-store]"`), nil
}

// NewStore binds the reviewed C1 database adapter to one trusted destination
// and expected broker Stream. The database dependency is fixed for the Store's
// lifetime and is never accepted per operation.
func NewStore(database dbgen.DBTX, binding Binding) (Store, error) {
	if nilDBTX(database) || !binding.Valid() {
		return nil, operationError(errorInvalidInput)
	}
	bound, err := newAdapter(database)
	if err != nil {
		return nil, err
	}
	return newProductionStore(bound, binding)
}

func newProductionStore(bound *adapter, binding Binding) (Store, error) {
	if bound == nil || bound.queries == nil || !binding.Valid() {
		return nil, operationError(errorInvalidInput)
	}
	return &productionStore{
		adapter:  bound,
		binding:  binding,
		identity: &storeIdentity{kind: productionStoreIdentity},
	}, nil
}

func (store *productionStore) Claim(ctx context.Context, request ClaimRequest) ([]Claim, error) {
	if ctx == nil || !store.valid() {
		return nil, operationError(errorInvalidInput)
	}
	internalRequest := claimRequest{
		claimOwnerID: request.ClaimOwnerID,
		batchSize:    request.BatchSize,
	}
	if !validClaimRequest(internalRequest) {
		return nil, operationError(errorInvalidInput)
	}

	events, err := store.adapter.claim(ctx, internalRequest)
	if err != nil {
		return nil, err
	}
	claims := make([]Claim, 0, len(events))
	for index := range events {
		event := &events[index]
		facts := publishFactsFromClaimedEvent(*event)
		lease := Lease{
			ClaimedAt:              event.claimedAt,
			ExpiresAt:              event.leaseExpiresAt,
			AbsoluteLeaseExpiresAt: event.absoluteLeaseExpiresAt,
		}
		if !facts.Valid() || !lease.Valid() ||
			facts.LogicalDestination != store.binding.LogicalDestination ||
			!factsMatchFence(facts, event.fence) {
			clearClaimedEvents(events)
			return nil, operationError(errorPersistence)
		}
		claims = append(claims, newOpaqueClaim(store.identity, event.fence, facts, lease))
	}
	clearClaimedEvents(events)
	return claims, nil
}

func (store *productionStore) Renew(ctx context.Context, claim Claim) (Renewal, error) {
	if ctx == nil || !store.valid() {
		return Renewal{}, operationError(errorInvalidInput)
	}
	authority, release, err := store.acquireProductionAuthority(ctx, claim)
	if err != nil {
		return Renewal{}, err
	}
	defer release()

	internal, err := store.adapter.renew(ctx, authority.fence)
	if err != nil {
		return Renewal{}, err
	}
	if !updateClaimLease(claim, store.identity, internal.leaseExpiresAt) {
		return Renewal{}, operationError(errorPersistence)
	}
	return Renewal{LeaseExpiresAt: internal.leaseExpiresAt}, nil
}

func (store *productionStore) Acknowledge(
	ctx context.Context,
	claim Claim,
	ack outboxpublish.Acknowledgement,
) (Acknowledgement, error) {
	if ctx == nil || !store.valid() {
		return 0, operationError(errorInvalidInput)
	}
	authority, release, err := store.acquireProductionAuthority(ctx, claim)
	if err != nil {
		return 0, err
	}
	defer release()
	if ack.Stream != store.binding.Stream || ack.Sequence == 0 ||
		ack.MessageID != authority.facts.BrokerMessageID || !validMessageID(ack.MessageID) {
		return 0, operationError(errorInvalidInput)
	}

	result, err := store.adapter.acknowledge(ctx, acknowledgementRequest{
		fence: authority.fence,
		pubAck: pubAck{
			stream:    ack.Stream,
			sequence:  ack.Sequence,
			duplicate: ack.Duplicate,
			messageID: ack.MessageID,
		},
	})
	if err != nil {
		return 0, err
	}
	switch result {
	case acknowledgementDelivered:
		return AcknowledgementDelivered, nil
	case acknowledgementAlreadyDelivered:
		return AcknowledgementAlreadyDelivered, nil
	default:
		return 0, operationError(errorPersistence)
	}
}

func (store *productionStore) RecordFailure(
	ctx context.Context,
	claim Claim,
	code FailureCode,
) (FailureResult, error) {
	if ctx == nil || !store.valid() {
		return FailureResult{}, operationError(errorInvalidInput)
	}
	authority, release, err := store.acquireProductionAuthority(ctx, claim)
	if err != nil {
		return FailureResult{}, err
	}
	defer release()
	if !validFailureCodeContract(code) {
		return FailureResult{}, operationError(errorInvalidInput)
	}

	result, err := store.adapter.recordFailure(ctx, publishFailureRequest{
		fence: authority.fence,
		code:  failureCode(code),
	})
	if err != nil {
		return FailureResult{}, err
	}
	switch result.disposition {
	case failureDispositionRetryScheduled:
		if result.nextAttemptAt.IsZero() {
			return FailureResult{}, operationError(errorPersistence)
		}
		return FailureResult{
			Disposition:   FailureRetryScheduled,
			NextAttemptAt: result.nextAttemptAt,
		}, nil
	case failureDispositionParked:
		if !result.nextAttemptAt.IsZero() {
			return FailureResult{}, operationError(errorPersistence)
		}
		return FailureResult{Disposition: FailureParked}, nil
	default:
		return FailureResult{}, operationError(errorPersistence)
	}
}

func (store *productionStore) valid() bool {
	return store != nil && store.adapter != nil && store.adapter.queries != nil &&
		store.binding.Valid() && store.identity != nil &&
		store.identity.kind == productionStoreIdentity
}

func (store *productionStore) acquireProductionAuthority(
	ctx context.Context,
	claim Claim,
) (*claimAuthority, func(), error) {
	if !store.valid() {
		return nil, nil, operationError(errorInvalidInput)
	}
	authority, release, err := acquireClaimOperation(ctx, claim, store.identity)
	if err != nil {
		return nil, nil, err
	}
	if authority.owner.kind != productionStoreIdentity ||
		!validFenceFacts(claim.authority.fence) ||
		!factsMatchFence(claim.authority.facts, claim.authority.fence) ||
		claim.authority.facts.LogicalDestination != store.binding.LogicalDestination {
		release()
		return nil, nil, operationError(errorClaimDenied)
	}
	digest, ok := authority.fence.token.candidateDigest()
	clear(digest[:])
	if !ok {
		release()
		return nil, nil, operationError(errorClaimDenied)
	}
	return authority, release, nil
}

func publishFactsFromClaimedEvent(event claimedEvent) PublishFacts {
	return PublishFacts{
		TenantID:           event.fence.tenantID,
		EventID:            event.fence.eventID,
		OutboxEntryID:      event.fence.outboxEntryID,
		LogicalDestination: event.destination,
		BrokerMessageID:    event.brokerMessageID,
		EventType:          event.eventType,
		SchemaVersion:      event.schemaVersion,
		AggregateKind:      event.aggregateKind,
		AggregateID:        event.aggregateID,
		Payload:            append([]byte(nil), event.payload...),
		OccurredAt:         event.occurredAt,
		EnqueuedAt:         event.enqueuedAt,
	}
}

func factsMatchFence(facts PublishFacts, fence claimFence) bool {
	return facts.TenantID == fence.tenantID && facts.EventID == fence.eventID &&
		facts.OutboxEntryID == fence.outboxEntryID
}

func clearClaimedEvents(events []claimedEvent) {
	for index := range events {
		clear(events[index].payload)
	}
}

func nilDBTX(database dbgen.DBTX) bool {
	if database == nil {
		return true
	}
	value := reflect.ValueOf(database)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
