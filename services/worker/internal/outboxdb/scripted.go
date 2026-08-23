package outboxdb

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

// ScriptedClaim describes publish facts and database-authored lease timing
// from which a ScriptedStore mints one opaque, Store-bound Claim.
type ScriptedClaim struct {
	PublishFacts PublishFacts
	Lease        Lease
}

// ScriptedClaimStep is one deterministic Claim result. Failure steps cannot
// also return Claims. A zero step is a successful empty Claim result.
type ScriptedClaimStep struct {
	Claims  []ScriptedClaim
	Failure StoreErrorCode
}

// ScriptedRenewStep is one deterministic Renew result.
type ScriptedRenewStep struct {
	Renewal Renewal
	Failure StoreErrorCode
}

// ScriptedAcknowledgementStep is one deterministic Acknowledge result.
type ScriptedAcknowledgementStep struct {
	Acknowledgement Acknowledgement
	Failure         StoreErrorCode
}

// ScriptedFailureStep is one deterministic RecordFailure result.
type ScriptedFailureStep struct {
	Result  FailureResult
	Failure StoreErrorCode
}

// ScriptedStorePlan contains independent FIFO plans for each Store method.
// Inputs are cloned by NewScriptedStore.
type ScriptedStorePlan struct {
	Claims           []ScriptedClaimStep
	Renewals         []ScriptedRenewStep
	Acknowledgements []ScriptedAcknowledgementStep
	Failures         []ScriptedFailureStep
}

// ScriptedAcknowledgementCall is an authority-free invocation snapshot.
type ScriptedAcknowledgementCall struct {
	PublishFacts    PublishFacts
	Acknowledgement outboxpublish.Acknowledgement
}

// ScriptedFailureCall is an authority-free invocation snapshot.
type ScriptedFailureCall struct {
	PublishFacts PublishFacts
	Failure      FailureCode
}

// ScriptedStoreCalls is a deep-cloned, authority-free invocation snapshot.
// It contains no Claim fence or token material.
type ScriptedStoreCalls struct {
	Claims           []ClaimRequest
	Renewals         []PublishFacts
	Acknowledgements []ScriptedAcknowledgementCall
	Failures         []ScriptedFailureCall
}

// ScriptedStore is a race-safe reusable fake at the Store seam. Every minted
// Claim is bound to this exact fake and cannot be used with another Store.
type ScriptedStore struct {
	mutex    sync.Mutex
	identity *storeIdentity
	binding  Binding
	plan     ScriptedStorePlan
	calls    ScriptedStoreCalls
}

var _ Store = (*ScriptedStore)(nil)

// NewScriptedStore validates and clones a deterministic Store plan.
func NewScriptedStore(binding Binding, plan ScriptedStorePlan) (*ScriptedStore, error) {
	if !binding.Valid() || !validScriptedStorePlan(plan) {
		return nil, operationError(errorInvalidInput)
	}
	return &ScriptedStore{
		identity: &storeIdentity{kind: scriptedStoreIdentity},
		binding:  binding,
		plan:     cloneScriptedStorePlan(plan),
	}, nil
}

// Claim records trusted process policy and returns newly minted opaque Claims.
func (store *ScriptedStore) Claim(ctx context.Context, request ClaimRequest) ([]Claim, error) {
	if store == nil || store.identity == nil || store.identity.kind != scriptedStoreIdentity ||
		!store.binding.Valid() || !validClaimRequest(claimRequest{
		claimOwnerID: request.ClaimOwnerID,
		batchSize:    request.BatchSize,
	}) {
		return nil, operationError(errorInvalidInput)
	}
	if ctx == nil {
		return nil, operationError(errorInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.calls.Claims = append(store.calls.Claims, request)
	if len(store.plan.Claims) == 0 {
		return nil, operationError(errorPersistence)
	}
	step := store.plan.Claims[0]
	store.plan.Claims = store.plan.Claims[1:]
	if step.Failure != "" {
		return nil, scriptedStoreError(step.Failure)
	}
	if len(step.Claims) > int(request.BatchSize) {
		return nil, operationError(errorPersistence)
	}
	claims := make([]Claim, len(step.Claims))
	for index, scripted := range step.Claims {
		claims[index] = newOpaqueClaim(
			store.identity,
			claimFence{},
			scripted.PublishFacts,
			scripted.Lease,
		)
	}
	return claims, nil
}

// Renew consumes only an opaque Claim minted by this ScriptedStore.
func (store *ScriptedStore) Renew(ctx context.Context, claim Claim) (Renewal, error) {
	if store == nil || store.identity == nil || store.identity.kind != scriptedStoreIdentity ||
		!store.binding.Valid() || ctx == nil {
		return Renewal{}, operationError(errorInvalidInput)
	}
	authority, release, err := acquireClaimOperation(ctx, claim, store.identity)
	if err != nil {
		return Renewal{}, err
	}
	defer release()

	store.mutex.Lock()
	if err := ctx.Err(); err != nil {
		store.mutex.Unlock()
		return Renewal{}, err
	}
	store.calls.Renewals = append(store.calls.Renewals, authority.facts.Clone())
	if len(store.plan.Renewals) == 0 {
		store.mutex.Unlock()
		return Renewal{}, operationError(errorPersistence)
	}
	step := store.plan.Renewals[0]
	store.plan.Renewals = store.plan.Renewals[1:]
	if step.Failure != "" {
		store.mutex.Unlock()
		return Renewal{}, scriptedStoreError(step.Failure)
	}
	store.mutex.Unlock()
	// The Claim operation gate serializes FIFO consumption and lease mutation
	// for this authority. The Store mutex is released before state is locked.
	if !updateClaimLease(claim, store.identity, step.Renewal.LeaseExpiresAt) {
		return Renewal{}, operationError(errorPersistence)
	}
	return step.Renewal, nil
}

// Acknowledge consumes only this fake's Claim and exact mapping-bound C2 Ack.
func (store *ScriptedStore) Acknowledge(
	ctx context.Context,
	claim Claim,
	acknowledgement outboxpublish.Acknowledgement,
) (Acknowledgement, error) {
	if store == nil || store.identity == nil || store.identity.kind != scriptedStoreIdentity ||
		!store.binding.Valid() || ctx == nil {
		return 0, operationError(errorInvalidInput)
	}
	authority, release, err := acquireClaimOperation(ctx, claim, store.identity)
	if err != nil {
		return 0, err
	}
	defer release()
	facts := authority.facts.Clone()
	if !validScriptedAcknowledgement(store.binding, facts, acknowledgement) {
		return 0, operationError(errorInvalidInput)
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	store.calls.Acknowledgements = append(store.calls.Acknowledgements, ScriptedAcknowledgementCall{
		PublishFacts:    facts.Clone(),
		Acknowledgement: acknowledgement,
	})
	if len(store.plan.Acknowledgements) == 0 {
		return 0, operationError(errorPersistence)
	}
	step := store.plan.Acknowledgements[0]
	store.plan.Acknowledgements = store.plan.Acknowledgements[1:]
	if step.Failure != "" {
		return 0, scriptedStoreError(step.Failure)
	}
	return step.Acknowledgement, nil
}

// RecordFailure consumes only this fake's Claim and the frozen failure enum.
func (store *ScriptedStore) RecordFailure(
	ctx context.Context,
	claim Claim,
	failure FailureCode,
) (FailureResult, error) {
	if store == nil || store.identity == nil || store.identity.kind != scriptedStoreIdentity ||
		!store.binding.Valid() || ctx == nil {
		return FailureResult{}, operationError(errorInvalidInput)
	}
	authority, release, err := acquireClaimOperation(ctx, claim, store.identity)
	if err != nil {
		return FailureResult{}, err
	}
	defer release()
	if !validFailureCodeContract(failure) {
		return FailureResult{}, operationError(errorInvalidInput)
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return FailureResult{}, err
	}
	store.calls.Failures = append(store.calls.Failures, ScriptedFailureCall{
		PublishFacts: authority.facts.Clone(),
		Failure:      failure,
	})
	if len(store.plan.Failures) == 0 {
		return FailureResult{}, operationError(errorPersistence)
	}
	step := store.plan.Failures[0]
	store.plan.Failures = store.plan.Failures[1:]
	if step.Failure != "" {
		return FailureResult{}, scriptedStoreError(step.Failure)
	}
	return step.Result, nil
}

// Calls returns deep-cloned, authority-free snapshots in invocation order.
func (store *ScriptedStore) Calls() ScriptedStoreCalls {
	if store == nil {
		return ScriptedStoreCalls{}
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return cloneScriptedStoreCalls(store.calls)
}

func validScriptedStorePlan(plan ScriptedStorePlan) bool {
	for _, step := range plan.Claims {
		if step.Failure != "" {
			if !validStoreErrorCode(step.Failure) || len(step.Claims) != 0 {
				return false
			}
			continue
		}
		for _, claim := range step.Claims {
			if !claim.PublishFacts.Valid() || !claim.Lease.Valid() {
				return false
			}
		}
	}
	for _, step := range plan.Renewals {
		if step.Failure != "" {
			if !validStoreErrorCode(step.Failure) || !step.Renewal.LeaseExpiresAt.IsZero() {
				return false
			}
			continue
		}
		if step.Renewal.LeaseExpiresAt.IsZero() {
			return false
		}
	}
	for _, step := range plan.Acknowledgements {
		if step.Failure != "" {
			if !validStoreErrorCode(step.Failure) || step.Acknowledgement != 0 {
				return false
			}
			continue
		}
		if step.Acknowledgement != AcknowledgementDelivered &&
			step.Acknowledgement != AcknowledgementAlreadyDelivered {
			return false
		}
	}
	for _, step := range plan.Failures {
		if step.Failure != "" {
			if !validStoreErrorCode(step.Failure) || step.Result != (FailureResult{}) {
				return false
			}
			continue
		}
		switch step.Result.Disposition {
		case FailureRetryScheduled:
			if step.Result.NextAttemptAt.IsZero() {
				return false
			}
		case FailureParked:
			if !step.Result.NextAttemptAt.IsZero() {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validStoreErrorCode(code StoreErrorCode) bool {
	switch code {
	case StoreErrorInvalidInput, StoreErrorClaimDenied, StoreErrorPersistence:
		return true
	default:
		return false
	}
}

func scriptedStoreError(code StoreErrorCode) error {
	switch code {
	case StoreErrorInvalidInput:
		return operationError(errorInvalidInput)
	case StoreErrorClaimDenied:
		return operationError(errorClaimDenied)
	default:
		return operationError(errorPersistence)
	}
}

func validScriptedAcknowledgement(
	binding Binding,
	facts PublishFacts,
	acknowledgement outboxpublish.Acknowledgement,
) bool {
	return binding.Valid() && acknowledgement.Stream == binding.Stream &&
		acknowledgement.Sequence > 0 && acknowledgement.MessageID == facts.BrokerMessageID &&
		validMessageID(acknowledgement.MessageID)
}

func cloneScriptedStorePlan(plan ScriptedStorePlan) ScriptedStorePlan {
	cloned := ScriptedStorePlan{
		Claims:           make([]ScriptedClaimStep, len(plan.Claims)),
		Renewals:         append([]ScriptedRenewStep(nil), plan.Renewals...),
		Acknowledgements: append([]ScriptedAcknowledgementStep(nil), plan.Acknowledgements...),
		Failures:         append([]ScriptedFailureStep(nil), plan.Failures...),
	}
	for stepIndex, step := range plan.Claims {
		cloned.Claims[stepIndex].Failure = step.Failure
		cloned.Claims[stepIndex].Claims = make([]ScriptedClaim, len(step.Claims))
		for claimIndex, claim := range step.Claims {
			cloned.Claims[stepIndex].Claims[claimIndex] = ScriptedClaim{
				PublishFacts: claim.PublishFacts.Clone(),
				Lease:        claim.Lease,
			}
		}
	}
	return cloned
}

func cloneScriptedStoreCalls(calls ScriptedStoreCalls) ScriptedStoreCalls {
	cloned := ScriptedStoreCalls{
		Claims:           append([]ClaimRequest(nil), calls.Claims...),
		Renewals:         make([]PublishFacts, len(calls.Renewals)),
		Acknowledgements: make([]ScriptedAcknowledgementCall, len(calls.Acknowledgements)),
		Failures:         make([]ScriptedFailureCall, len(calls.Failures)),
	}
	for index := range calls.Renewals {
		cloned.Renewals[index] = calls.Renewals[index].Clone()
	}
	for index, call := range calls.Acknowledgements {
		cloned.Acknowledgements[index] = ScriptedAcknowledgementCall{
			PublishFacts:    call.PublishFacts.Clone(),
			Acknowledgement: call.Acknowledgement,
		}
	}
	for index, call := range calls.Failures {
		cloned.Failures[index] = ScriptedFailureCall{
			PublishFacts: call.PublishFacts.Clone(),
			Failure:      call.Failure,
		}
	}
	return cloned
}

func redactedScriptedJSON(value string) ([]byte, error) { return json.Marshal(value) }

func (ScriptedClaim) String() string   { return "<redacted-outbox-scripted-claim>" }
func (ScriptedClaim) GoString() string { return "<redacted-outbox-scripted-claim>" }
func (ScriptedClaim) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-claim]")
}

func (ScriptedClaimStep) String() string   { return "<redacted-outbox-scripted-claim-step>" }
func (ScriptedClaimStep) GoString() string { return "<redacted-outbox-scripted-claim-step>" }
func (ScriptedClaimStep) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-claim-step]")
}

func (ScriptedRenewStep) String() string   { return "<redacted-outbox-scripted-renew-step>" }
func (ScriptedRenewStep) GoString() string { return "<redacted-outbox-scripted-renew-step>" }
func (ScriptedRenewStep) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-renew-step]")
}

func (ScriptedAcknowledgementStep) String() string {
	return "<redacted-outbox-scripted-acknowledgement-step>"
}
func (ScriptedAcknowledgementStep) GoString() string {
	return "<redacted-outbox-scripted-acknowledgement-step>"
}
func (ScriptedAcknowledgementStep) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-acknowledgement-step]")
}

func (ScriptedFailureStep) String() string {
	return "<redacted-outbox-scripted-failure-step>"
}
func (ScriptedFailureStep) GoString() string {
	return "<redacted-outbox-scripted-failure-step>"
}
func (ScriptedFailureStep) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-failure-step]")
}

func (ScriptedStorePlan) String() string   { return "<redacted-outbox-scripted-store-plan>" }
func (ScriptedStorePlan) GoString() string { return "<redacted-outbox-scripted-store-plan>" }
func (ScriptedStorePlan) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-store-plan]")
}

func (ScriptedAcknowledgementCall) String() string {
	return "<redacted-outbox-scripted-acknowledgement-call>"
}
func (ScriptedAcknowledgementCall) GoString() string {
	return "<redacted-outbox-scripted-acknowledgement-call>"
}
func (ScriptedAcknowledgementCall) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-acknowledgement-call]")
}

func (ScriptedFailureCall) String() string {
	return "<redacted-outbox-scripted-failure-call>"
}
func (ScriptedFailureCall) GoString() string {
	return "<redacted-outbox-scripted-failure-call>"
}
func (ScriptedFailureCall) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-failure-call]")
}

func (ScriptedStoreCalls) String() string   { return "<redacted-outbox-scripted-store-calls>" }
func (ScriptedStoreCalls) GoString() string { return "<redacted-outbox-scripted-store-calls>" }
func (ScriptedStoreCalls) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-store-calls]")
}

func (*ScriptedStore) String() string   { return "<redacted-outbox-scripted-store>" }
func (*ScriptedStore) GoString() string { return "<redacted-outbox-scripted-store>" }
func (*ScriptedStore) MarshalJSON() ([]byte, error) {
	return redactedScriptedJSON("[redacted-outbox-scripted-store]")
}
