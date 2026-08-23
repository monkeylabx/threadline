package outboxrelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxbreaker"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

func TestCoordinatorSecurityRejectsNilAndZeroValueInputsBeforeWork(t *testing.T) {
	t.Parallel()

	store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
	encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")}
	publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}
	relay := mustRelay(t, store, encoder, publisher)
	breaker := coordinatorSecurityBreaker(t)

	for _, test := range []struct {
		name    string
		relay   *Relay
		breaker *outboxbreaker.Breaker
	}{
		{name: "nil Relay", breaker: breaker},
		{name: "nil Breaker", relay: relay},
		{name: "zero-value Breaker", relay: relay, breaker: &outboxbreaker.Breaker{}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			coordinator, err := NewCoordinator(test.relay, test.breaker)
			if coordinator != nil || relayErrorCode(err) != RelayErrorInvalidInput {
				t.Fatalf("NewCoordinator = %#v/%v, want nil/invalid-input", coordinator, err)
			}
		})
	}

	var nilCoordinator *Coordinator
	if result, err := nilCoordinator.RunOne(context.Background()); result != (Result{}) || relayErrorCode(err) != RelayErrorInvalidInput {
		t.Fatalf("nil Coordinator RunOne = %#v/%v, want zero/invalid-input", result, err)
	}
	if result, err := (&Coordinator{}).RunOne(context.Background()); result != (Result{}) || relayErrorCode(err) != RelayErrorInvalidInput {
		t.Fatalf("zero Coordinator RunOne = %#v/%v, want zero/invalid-input", result, err)
	}
	coordinator := coordinatorSecurityMustCoordinator(t, relay, breaker)
	if result, err := coordinator.RunOne(nil); result != (Result{}) || relayErrorCode(err) != RelayErrorInvalidInput {
		t.Fatalf("nil-context RunOne = %#v/%v, want zero/invalid-input", result, err)
	}

	if calls := store.snapshot(); calls != (relaySecurityStoreSnapshot{}) ||
		encoder.callCount() != 0 || publisher.callCount() != 0 {
		t.Fatalf("invalid inputs reached dependencies: Store=%#v Encode=%d Publish=%d",
			calls, encoder.callCount(), publisher.callCount())
	}
}

func TestCoordinatorSecurityPreCanceledContextWinsBeforeRelayWork(t *testing.T) {
	t.Parallel()

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		want := want
		t.Run(want.Error(), func(t *testing.T) {
			t.Parallel()
			ctx := newRelaySecurityContext()
			ctx.fail(want)
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
			encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")}
			publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}
			breaker := coordinatorSecurityBreaker(t)
			coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(t, store, encoder, publisher), breaker)

			result, err := coordinator.RunOne(ctx)
			if result != (Result{}) || !errors.Is(err, want) || err != want {
				t.Fatalf("RunOne = %#v/%v, want zero and exact %v", result, err, want)
			}
			if calls := store.snapshot(); calls != (relaySecurityStoreSnapshot{}) ||
				encoder.callCount() != 0 || publisher.callCount() != 0 {
				t.Fatalf("pre-canceled call reached dependencies: Store=%#v Encode=%d Publish=%d",
					calls, encoder.callCount(), publisher.callCount())
			}
			coordinatorSecurityAcquireAndRelease(t, breaker, outboxbreaker.DecisionAllowed)
		})
	}
}

func TestCoordinatorSecurityPublisherInvariantPathsReleaseWithoutEvidence(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		ack  outboxpublish.Acknowledgement
		err  func(*testing.T) error
	}{
		{name: "raw error", err: func(*testing.T) error { return errors.New("raw-broker-credential-secret") }},
		{name: "invalid classified error", err: func(t *testing.T) error {
			return relaySecurityPublishError(t, outboxpublish.FailureInvalidInput)
		}},
		{name: "contradictory Ack and transport error", ack: relaySecurityAcknowledgement(), err: func(t *testing.T) error {
			return relaySecurityPublishError(t, outboxpublish.FailureTransportUnavailable)
		}},
		{name: "contradictory Ack and outcome-unknown error", ack: relaySecurityAcknowledgement(), err: func(t *testing.T) error {
			return relaySecurityPublishError(t, outboxpublish.FailurePublishOutcomeUnknown)
		}},
		{name: "malformed Ack", ack: outboxpublish.Acknowledgement{
			Stream: relayTestBinding.Stream, MessageID: validEncoderTestFacts(nil).BrokerMessageID,
		}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
			publisher := &relaySecurityPublisher{acknowledgement: test.ack}
			if test.err != nil {
				publisher.err = test.err(t)
			}
			clock := coordinatorSecurityNewClock()
			breaker := coordinatorSecurityBreakerWithClock(t, clock)
			coordinatorSecurityOpenBreaker(t, breaker)
			clock.Advance(5 * time.Second)
			coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(
				t,
				store,
				&relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")},
				publisher,
			), breaker)

			result, err := coordinator.RunOne(context.Background())
			if result != (Result{}) || relayErrorCode(err) != RelayErrorInvariantViolation {
				t.Fatalf("RunOne = %#v/%v, want zero/invariant-violation", result, err)
			}
			if calls := store.snapshot(); calls.claims != 1 || calls.acknowledgements != 0 || calls.failures != 0 ||
				publisher.callCount() != 1 {
				t.Fatalf("Claim/Publish/Ack/Failure = %d/%d/%d/%d, want 1/1/0/0",
					calls.claims, publisher.callCount(), calls.acknowledgements, calls.failures)
			}
			// The invariant path held the half-open probe. A release makes another
			// probe immediately eligible; leaked or manufactured evidence would deny.
			coordinatorSecurityAcquireAndRelease(t, breaker, outboxbreaker.DecisionProbe)
		})
	}
}

func TestCoordinatorSecurityVerifiedPubAckResetsBreakerDespiteAckFailure(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		configure   func(*testing.T, *relaySecurityContext, *relaySecurityStore)
		wantOutcome Outcome
		wantError   func(error) bool
	}{
		{
			name: "caller cancellation",
			configure: func(_ *testing.T, ctx *relaySecurityContext, store *relaySecurityStore) {
				store.ackHook = func(context.Context) { ctx.fail(context.Canceled) }
			},
			wantOutcome: OutcomeAcknowledgementUnconfirmed,
			wantError:   func(err error) bool { return errors.Is(err, context.Canceled) },
		},
		{
			name: "dependency context error",
			configure: func(_ *testing.T, _ *relaySecurityContext, store *relaySecurityStore) {
				store.ackError = context.DeadlineExceeded
			},
			wantOutcome: OutcomeAcknowledgementUnconfirmed,
			wantError:   func(err error) bool { return errors.Is(err, context.DeadlineExceeded) },
		},
		{
			name: "persistence error",
			configure: func(t *testing.T, _ *relaySecurityContext, store *relaySecurityStore) {
				store.ackError = relaySecurityStoreError(t, outboxdb.StoreErrorPersistence)
			},
			wantOutcome: OutcomeAcknowledgementUnconfirmed,
			wantError: func(err error) bool {
				return relayErrorCode(err) == RelayErrorAcknowledgementUnconfirmed
			},
		},
		{
			name: "raw error",
			configure: func(_ *testing.T, _ *relaySecurityContext, store *relaySecurityStore) {
				store.ackError = errors.New("raw-ack-credential-secret")
			},
			wantOutcome: OutcomeAcknowledgementUnconfirmed,
			wantError: func(err error) bool {
				return relayErrorCode(err) == RelayErrorInvariantViolation
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			clock := coordinatorSecurityNewClock()
			breaker := coordinatorSecurityBreakerWithClock(t, clock)
			coordinatorSecurityOpenBreaker(t, breaker)
			clock.Advance(5 * time.Second)

			ctx := newRelaySecurityContext()
			store := &relaySecurityStore{
				claims:    []outboxdb.Claim{relaySecurityClaim(t)},
				ackResult: outboxdb.AcknowledgementDelivered,
			}
			test.configure(t, ctx, store)
			coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(
				t,
				store,
				&relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")},
				&relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()},
			), breaker)

			result, err := coordinator.RunOne(ctx)
			if result.Outcome != test.wantOutcome || !test.wantError(err) {
				t.Fatalf("PubAck RunOne = %#v/%v, want %v and expected error", result, err, test.wantOutcome)
			}
			if calls := store.snapshot(); calls.claims != 1 || calls.acknowledgements != 1 || calls.failures != 0 {
				t.Fatalf("PubAck Claim/Ack/Failure = %d/%d/%d, want 1/1/0",
					calls.claims, calls.acknowledgements, calls.failures)
			}

			// A release of the half-open probe would let the next infrastructure
			// failure immediately reopen the breaker. A verified PubAck reset makes
			// the following two failures ordinary and therefore both runnable.
			failureStore, failureCoordinator := coordinatorSecurityInfrastructureCoordinator(
				t, breaker, outboxpublish.FailureTransportUnavailable,
			)
			for attempt := 0; attempt < 2; attempt++ {
				failureResult, failureErr := failureCoordinator.RunOne(context.Background())
				if failureErr != nil || failureResult.Outcome != OutcomeRetryScheduled {
					t.Fatalf("post-reset failure %d = %#v/%v, want RetryScheduled/nil",
						attempt+1, failureResult, failureErr)
				}
			}
			if calls := failureStore.snapshot(); calls.claims != 2 || calls.failures != 2 {
				t.Fatalf("post-reset Claim/Failure = %d/%d, want 2/2", calls.claims, calls.failures)
			}
		})
	}
}

func TestCoordinatorSecurityInfrastructureEvidenceSurvivesFailureMutationErrors(t *testing.T) {
	t.Parallel()

	for _, publishFailure := range []outboxpublish.FailureCode{
		outboxpublish.FailureTransportUnavailable,
		outboxpublish.FailurePublishOutcomeUnknown,
	} {
		publishFailure := publishFailure
		for _, mutationFailure := range []string{"cancellation", "persistence", "raw"} {
			mutationFailure := mutationFailure
			t.Run(string(publishFailure)+"/"+mutationFailure, func(t *testing.T) {
				t.Parallel()
				store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
				switch mutationFailure {
				case "cancellation":
					store.failureHook = func(ctx context.Context) {
						ctx.(*relaySecurityContext).fail(context.Canceled)
					}
				case "persistence":
					store.failureError = relaySecurityStoreError(t, outboxdb.StoreErrorPersistence)
				case "raw":
					store.failureError = errors.New("raw-failure-store-credential-secret")
				}
				breaker := coordinatorSecurityBreaker(t)
				publisher := &relaySecurityPublisher{err: relaySecurityPublishError(t, publishFailure)}
				coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(
					t,
					store,
					&relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")},
					publisher,
				), breaker)

				for attempt := 0; attempt < 3; attempt++ {
					ctx := context.Context(context.Background())
					if mutationFailure == "cancellation" {
						ctx = newRelaySecurityContext()
					}
					result, err := coordinator.RunOne(ctx)
					if result.Outcome != OutcomeFailureUnconfirmed {
						t.Fatalf("RunOne attempt %d outcome = %v, want FailureUnconfirmed", attempt+1, result.Outcome)
					}
					switch mutationFailure {
					case "cancellation":
						if !errors.Is(err, context.Canceled) {
							t.Fatalf("RunOne attempt %d error = %v, want canceled", attempt+1, err)
						}
					case "persistence":
						if relayErrorCode(err) != RelayErrorFailureUnconfirmed {
							t.Fatalf("RunOne attempt %d error = %v, want failure-unconfirmed", attempt+1, err)
						}
					case "raw":
						if relayErrorCode(err) != RelayErrorInvariantViolation {
							t.Fatalf("RunOne attempt %d error = %v, want invariant-violation", attempt+1, err)
						}
					}
				}

				beforeDenied := store.snapshot()
				result, err := coordinator.RunOne(context.Background())
				if err != nil || result.Outcome != OutcomeCircuitOpen {
					t.Fatalf("fourth RunOne = %#v/%v, want CircuitOpen/nil", result, err)
				}
				afterDenied := store.snapshot()
				if beforeDenied != afterDenied || afterDenied.claims != 3 || afterDenied.failures != 3 ||
					publisher.callCount() != 3 {
					t.Fatalf("denied call changed work counts: before=%#v after=%#v Publish=%d",
						beforeDenied, afterDenied, publisher.callCount())
				}
			})
		}
	}
}

func TestCoordinatorSecurityTypedBrokerEvidenceSurvivesPublisherDeadline(t *testing.T) {
	t.Parallel()

	for _, failure := range []outboxpublish.FailureCode{
		outboxpublish.FailureTransportUnavailable,
		outboxpublish.FailurePublishOutcomeUnknown,
	} {
		failure := failure
		t.Run(string(failure), func(t *testing.T) {
			t.Parallel()
			clock := coordinatorSecurityNewClock()
			breaker := coordinatorSecurityBreakerWithClock(t, clock)
			coordinatorSecurityOpenBreaker(t, breaker)
			clock.Advance(5 * time.Second)

			ctx := newRelaySecurityContext()
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
			publisherFailure := relaySecurityPublishError(t, failure)
			publisher := &relaySecurityPublisher{hook: func(context.Context, outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
				ctx.fail(context.DeadlineExceeded)
				return outboxpublish.Acknowledgement{}, publisherFailure
			}}
			coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(
				t,
				store,
				&relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")},
				publisher,
			), breaker)

			result, err := coordinator.RunOne(ctx)
			if result != (Result{}) || err != context.DeadlineExceeded {
				t.Fatalf("RunOne = %#v/%v, want zero/exact deadline", result, err)
			}
			if calls := store.snapshot(); calls.claims != 1 || calls.failures != 0 || publisher.callCount() != 1 {
				t.Fatalf("Claim/Publish/Failure = %d/%d/%d, want 1/1/0", calls.claims, publisher.callCount(), calls.failures)
			}
			if _, decision, acquireErr := breaker.Acquire(); acquireErr != nil || decision != outboxbreaker.DecisionDenied {
				t.Fatalf("post-deadline Acquire = %v/%v, want denied/nil", decision, acquireErr)
			}
		})
	}
}

func TestCoordinatorSecurityFormattingAndJSONAreRedacted(t *testing.T) {
	t.Parallel()

	const rawSecret = "coordinator-raw-broker-credential-secret"
	store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
	publisher := &relaySecurityPublisher{err: errors.New(rawSecret)}
	coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(
		t,
		store,
		&relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")},
		publisher,
	), coordinatorSecurityBreaker(t))
	result, err := coordinator.RunOne(context.Background())
	if relayErrorCode(err) != RelayErrorInvariantViolation {
		t.Fatalf("RunOne error = %v, want invariant-violation", err)
	}

	values := []any{coordinator, result, result.Outcome, OutcomeCircuitOpen, err, RelayErrorInvariantViolation}
	securityAssertRelayRedacted(t, values, rawSecret)
	for _, value := range values {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(encoded)} {
			if strings.Contains(rendered, rawSecret) {
				t.Fatalf("Coordinator rendering leaked secret: %q", rendered)
			}
		}
	}
}

type coordinatorSecurityClock struct {
	mutex sync.Mutex
	now   time.Time
}

func coordinatorSecurityNewClock() *coordinatorSecurityClock {
	return &coordinatorSecurityClock{now: time.Date(2026, time.August, 24, 0, 0, 0, 0, time.UTC)}
}

func (clock *coordinatorSecurityClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.now
}

func (clock *coordinatorSecurityClock) Advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.now = clock.now.Add(duration)
}

func coordinatorSecurityBreaker(t *testing.T) *outboxbreaker.Breaker {
	t.Helper()
	return coordinatorSecurityBreakerWithClock(t, coordinatorSecurityNewClock())
}

func coordinatorSecurityBreakerWithClock(t *testing.T, clock outboxbreaker.Clock) *outboxbreaker.Breaker {
	t.Helper()
	breaker, err := outboxbreaker.New(relayTestMapping, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := breaker.Ready(); err != nil {
		t.Fatal(err)
	}
	return breaker
}

func coordinatorSecurityMustCoordinator(t *testing.T, relay *Relay, breaker *outboxbreaker.Breaker) *Coordinator {
	t.Helper()
	coordinator, err := NewCoordinator(relay, breaker)
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

func coordinatorSecurityAcquireAndRelease(
	t *testing.T,
	breaker *outboxbreaker.Breaker,
	want outboxbreaker.Decision,
) {
	t.Helper()
	permit, decision, err := breaker.Acquire()
	if err != nil || decision != want {
		t.Fatalf("Acquire = %v/%v, want %v/nil", decision, err, want)
	}
	if err := breaker.Release(permit); err != nil {
		t.Fatalf("Release = %v", err)
	}
}

func coordinatorSecurityInfrastructureCoordinator(
	t *testing.T,
	breaker *outboxbreaker.Breaker,
	failure outboxpublish.FailureCode,
) (*relaySecurityStore, *Coordinator) {
	t.Helper()
	store := &relaySecurityStore{
		claims:        []outboxdb.Claim{relaySecurityClaim(t)},
		failureResult: relaySecurityRetryResult(),
	}
	relay := mustRelay(
		t,
		store,
		&relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")},
		&relaySecurityPublisher{err: relaySecurityPublishError(t, failure)},
	)
	return store, coordinatorSecurityMustCoordinator(t, relay, breaker)
}

func coordinatorSecurityOpenBreaker(t *testing.T, breaker *outboxbreaker.Breaker) {
	t.Helper()
	store, coordinator := coordinatorSecurityInfrastructureCoordinator(
		t, breaker, outboxpublish.FailureTransportUnavailable,
	)
	for attempt := 0; attempt < 3; attempt++ {
		result, err := coordinator.RunOne(context.Background())
		if err != nil || result.Outcome != OutcomeRetryScheduled {
			t.Fatalf("opening attempt %d = %#v/%v, want RetryScheduled/nil", attempt+1, result, err)
		}
	}
	if calls := store.snapshot(); calls.claims != 3 || calls.failures != 3 {
		t.Fatalf("opening Claim/Failure = %d/%d, want 3/3", calls.claims, calls.failures)
	}
}
