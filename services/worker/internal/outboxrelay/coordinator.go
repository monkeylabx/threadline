package outboxrelay

import (
	"context"
	"encoding/json"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxbreaker"
)

// Coordinator binds one Relay attempt stream to its destination circuit
// breaker. Broker permits and observations remain private to this module.
type Coordinator struct {
	relay   *Relay
	breaker *outboxbreaker.Breaker
}

func (*Coordinator) String() string   { return "<redacted-outbox-relay-coordinator>" }
func (*Coordinator) GoString() string { return "<redacted-outbox-relay-coordinator>" }
func (*Coordinator) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-relay-coordinator]")
}

// NewCoordinator binds exactly one valid Relay and one valid Breaker for the
// Coordinator's lifetime. Breaker readiness is deliberately not required:
// startup-denied is a normal circuit-open result.
func NewCoordinator(relay *Relay, breaker *outboxbreaker.Breaker) (*Coordinator, error) {
	if relay == nil || !relay.valid() || breaker == nil || !breaker.Valid() {
		return nil, relayError(RelayErrorInvalidInput)
	}
	return &Coordinator{relay: relay, breaker: breaker}, nil
}

// RunOne acquires circuit authority before delegating exactly one attempt to
// Relay. It performs no detached work, retry, scheduling, or time decision.
func (coordinator *Coordinator) RunOne(ctx context.Context) (Result, error) {
	if ctx == nil || !coordinator.valid() {
		return Result{}, relayError(RelayErrorInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	permit, decision, err := coordinator.breaker.Acquire()
	if err != nil {
		return Result{}, relayError(RelayErrorInvariantViolation)
	}
	switch decision {
	case outboxbreaker.DecisionDenied:
		return Result{Outcome: OutcomeCircuitOpen}, nil
	case outboxbreaker.DecisionAllowed, outboxbreaker.DecisionProbe:
		// Continue below. Relay performs its own cancellation check with the
		// exact caller context before touching its dependencies.
	default:
		// An impossible decision might still carry a permit. Make exactly one
		// best-effort consumption attempt, but never retry or run Relay.
		_ = coordinator.breaker.Release(permit)
		return Result{}, relayError(RelayErrorInvariantViolation)
	}

	result, observation, relayErr := coordinator.relay.runOne(ctx)
	if err := coordinator.consume(permit, observation); err != nil {
		return Result{}, relayError(RelayErrorInvariantViolation)
	}
	return result, relayErr
}

func (coordinator *Coordinator) valid() bool {
	return coordinator != nil && coordinator.relay != nil && coordinator.relay.valid() &&
		coordinator.breaker != nil && coordinator.breaker.Valid()
}

func (coordinator *Coordinator) consume(
	permit outboxbreaker.Permit,
	observation brokerObservation,
) error {
	switch observation {
	case brokerObservationNone:
		return coordinator.breaker.Release(permit)
	case brokerObservationVerifiedAcknowledgement:
		return coordinator.breaker.Observe(permit, outboxbreaker.ObservationVerifiedPubAck)
	case brokerObservationTransportUnavailable:
		return coordinator.breaker.Observe(permit, outboxbreaker.ObservationTransportUnavailable)
	case brokerObservationPublishOutcomeUnknown:
		return coordinator.breaker.Observe(permit, outboxbreaker.ObservationPublishOutcomeUnknown)
	default:
		// Preserve the one-consumption rule even if internal state is corrupt.
		_ = coordinator.breaker.Release(permit)
		return relayError(RelayErrorInvariantViolation)
	}
}
