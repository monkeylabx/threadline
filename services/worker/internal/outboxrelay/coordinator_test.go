package outboxrelay

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxbreaker"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

func TestCoordinatorRelayFamilyBreakerEffectMatrix(t *testing.T) {
	t.Parallel()

	type dependencies struct {
		store     *relaySecurityStore
		encoder   *relaySecurityEncoder
		publisher *relaySecurityPublisher
	}
	claim := func(t *testing.T) []outboxdb.Claim { return []outboxdb.Claim{relaySecurityClaim(t)} }
	base := func(t *testing.T) dependencies {
		return dependencies{
			store:     &relaySecurityStore{claims: claim(t), ackResult: outboxdb.AcknowledgementDelivered},
			encoder:   &relaySecurityEncoder{envelope: []byte("encoded-envelope")},
			publisher: &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()},
		}
	}
	tests := []struct {
		name         string
		configure    func(*testing.T, *dependencies)
		wantOutcome  Outcome
		wantError    RelayErrorCode
		wantDecision outboxbreaker.Decision
	}{
		{name: "idle", configure: func(_ *testing.T, d *dependencies) { d.store.claims = nil }, wantOutcome: OutcomeIdle, wantDecision: outboxbreaker.DecisionProbe},
		{name: "claim unavailable", configure: func(t *testing.T, d *dependencies) {
			d.store.claimError = relaySecurityStoreError(t, outboxdb.StoreErrorPersistence)
		}, wantError: RelayErrorClaimUnavailable, wantDecision: outboxbreaker.DecisionProbe},
		{name: "encoder retryable", configure: func(_ *testing.T, d *dependencies) {
			d.encoder.err = encodeError(EncoderFailureEventRetryable)
			d.encoder.envelope = nil
			d.store.failureResult = relaySecurityRetryResult()
		}, wantOutcome: OutcomeRetryScheduled, wantDecision: outboxbreaker.DecisionProbe},
		{name: "encoder permanent", configure: func(_ *testing.T, d *dependencies) {
			d.encoder.err = encodeError(EncoderFailureEventPermanent)
			d.encoder.envelope = nil
			d.store.failureResult = outboxdb.FailureResult{Disposition: outboxdb.FailureParked}
		}, wantOutcome: OutcomeParked, wantDecision: outboxbreaker.DecisionProbe},
		{name: "publisher permanent", configure: func(t *testing.T, d *dependencies) {
			d.publisher.acknowledgement = outboxpublish.Acknowledgement{}
			d.publisher.err = relaySecurityPublishError(t, outboxpublish.FailureEventPermanent)
			d.store.failureResult = outboxdb.FailureResult{Disposition: outboxdb.FailureParked}
		}, wantOutcome: OutcomeParked, wantDecision: outboxbreaker.DecisionProbe},
		{name: "publisher raw", configure: func(_ *testing.T, d *dependencies) {
			d.publisher.acknowledgement = outboxpublish.Acknowledgement{}
			d.publisher.err = errors.New("raw publisher error")
		}, wantError: RelayErrorInvariantViolation, wantDecision: outboxbreaker.DecisionProbe},
		{name: "publisher malformed acknowledgement", configure: func(_ *testing.T, d *dependencies) { d.publisher.acknowledgement.Sequence = 0 }, wantError: RelayErrorInvariantViolation, wantDecision: outboxbreaker.DecisionProbe},
		{name: "transport unavailable", configure: func(t *testing.T, d *dependencies) {
			d.publisher.acknowledgement = outboxpublish.Acknowledgement{}
			d.publisher.err = relaySecurityPublishError(t, outboxpublish.FailureTransportUnavailable)
			d.store.failureResult = relaySecurityRetryResult()
		}, wantOutcome: OutcomeRetryScheduled, wantDecision: outboxbreaker.DecisionDenied},
		{name: "publish outcome unknown", configure: func(t *testing.T, d *dependencies) {
			d.publisher.acknowledgement = outboxpublish.Acknowledgement{}
			d.publisher.err = relaySecurityPublishError(t, outboxpublish.FailurePublishOutcomeUnknown)
			d.store.failureResult = relaySecurityRetryResult()
		}, wantOutcome: OutcomeRetryScheduled, wantDecision: outboxbreaker.DecisionDenied},
		{name: "delivered", configure: func(_ *testing.T, _ *dependencies) {}, wantOutcome: OutcomeDelivered, wantDecision: outboxbreaker.DecisionAllowed},
		{name: "already delivered", configure: func(_ *testing.T, d *dependencies) { d.store.ackResult = outboxdb.AcknowledgementAlreadyDelivered }, wantOutcome: OutcomeAlreadyDelivered, wantDecision: outboxbreaker.DecisionAllowed},
		{name: "ack claim lost", configure: func(t *testing.T, d *dependencies) {
			d.store.ackError = relaySecurityStoreError(t, outboxdb.StoreErrorClaimDenied)
		}, wantOutcome: OutcomeClaimLost, wantDecision: outboxbreaker.DecisionAllowed},
		{name: "ack persistence", configure: func(t *testing.T, d *dependencies) {
			d.store.ackError = relaySecurityStoreError(t, outboxdb.StoreErrorPersistence)
		}, wantOutcome: OutcomeAcknowledgementUnconfirmed, wantError: RelayErrorAcknowledgementUnconfirmed, wantDecision: outboxbreaker.DecisionAllowed},
		{name: "ack invariant result", configure: func(_ *testing.T, d *dependencies) { d.store.ackResult = 0 }, wantOutcome: OutcomeAcknowledgementUnconfirmed, wantError: RelayErrorInvariantViolation, wantDecision: outboxbreaker.DecisionAllowed},
		{name: "failure claim lost", configure: func(t *testing.T, d *dependencies) {
			d.publisher.acknowledgement = outboxpublish.Acknowledgement{}
			d.publisher.err = relaySecurityPublishError(t, outboxpublish.FailureTransportUnavailable)
			d.store.failureError = relaySecurityStoreError(t, outboxdb.StoreErrorClaimDenied)
		}, wantOutcome: OutcomeClaimLost, wantDecision: outboxbreaker.DecisionDenied},
		{name: "failure persistence", configure: func(t *testing.T, d *dependencies) {
			d.publisher.acknowledgement = outboxpublish.Acknowledgement{}
			d.publisher.err = relaySecurityPublishError(t, outboxpublish.FailureTransportUnavailable)
			d.store.failureError = relaySecurityStoreError(t, outboxdb.StoreErrorPersistence)
		}, wantOutcome: OutcomeFailureUnconfirmed, wantError: RelayErrorFailureUnconfirmed, wantDecision: outboxbreaker.DecisionDenied},
		{name: "failure invariant result", configure: func(t *testing.T, d *dependencies) {
			d.publisher.acknowledgement = outboxpublish.Acknowledgement{}
			d.publisher.err = relaySecurityPublishError(t, outboxpublish.FailureTransportUnavailable)
		}, wantOutcome: OutcomeFailureUnconfirmed, wantError: RelayErrorInvariantViolation, wantDecision: outboxbreaker.DecisionDenied},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			clock := coordinatorSecurityNewClock()
			breaker := coordinatorSecurityBreakerWithClock(t, clock)
			coordinatorSecurityOpenBreaker(t, breaker)
			clock.Advance(5 * time.Second)
			deps := base(t)
			test.configure(t, &deps)
			coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(t, deps.store, deps.encoder, deps.publisher), breaker)
			result, err := coordinator.RunOne(context.Background())
			if result.Outcome != test.wantOutcome || relayErrorCode(err) != test.wantError {
				t.Fatalf("RunOne = %#v/%v, want outcome %v/error %q", result, err, test.wantOutcome, test.wantError)
			}
			permit, decision, acquireErr := breaker.Acquire()
			if acquireErr != nil || decision != test.wantDecision {
				t.Fatalf("post-attempt Acquire = %v/%v, want %v/nil", decision, acquireErr, test.wantDecision)
			}
			if decision != outboxbreaker.DecisionDenied {
				if releaseErr := breaker.Release(permit); releaseErr != nil {
					t.Fatalf("Release = %v", releaseErr)
				}
			}
		})
	}
}

func TestCoordinatorPreservesExactCallerContextForEveryDependency(t *testing.T) {
	t.Parallel()

	for _, failure := range []outboxpublish.FailureCode{"", outboxpublish.FailureTransportUnavailable} {
		failure := failure
		t.Run(string(failure), func(t *testing.T) {
			t.Parallel()
			ctx := context.WithValue(context.Background(), struct{ name string }{"coordinator"}, "exact")
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}, ackResult: outboxdb.AcknowledgementDelivered, failureResult: relaySecurityRetryResult()}
			encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope")}
			publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}
			if failure != "" {
				publisher.acknowledgement = outboxpublish.Acknowledgement{}
				publisher.err = relaySecurityPublishError(t, failure)
			}
			coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(t, store, encoder, publisher), coordinatorSecurityBreaker(t))
			if _, err := coordinator.RunOne(ctx); err != nil {
				t.Fatalf("RunOne = %v", err)
			}
			assertRelaySecurityContexts(t, ctx, store.contextsSnapshot(), encoder.contextsSnapshot(), publisher.contextsSnapshot())
		})
	}
}

func TestCoordinatorOpensAfterThreeInfrastructureObservationsAndThenSkipsClaim(t *testing.T) {
	t.Parallel()

	breaker := coordinatorSecurityBreaker(t)
	store, coordinator := coordinatorSecurityInfrastructureCoordinator(
		t, breaker, outboxpublish.FailureTransportUnavailable,
	)
	for attempt := 0; attempt < 3; attempt++ {
		result, err := coordinator.RunOne(context.Background())
		if err != nil || result.Outcome != OutcomeRetryScheduled {
			t.Fatalf("RunOne attempt %d = %#v/%v, want RetryScheduled/nil", attempt+1, result, err)
		}
	}
	before := store.snapshot()
	result, err := coordinator.RunOne(context.Background())
	if err != nil || result.Outcome != OutcomeCircuitOpen {
		t.Fatalf("fourth RunOne = %#v/%v, want CircuitOpen/nil", result, err)
	}
	if after := store.snapshot(); after != before || after.claims != 3 || after.failures != 3 {
		t.Fatalf("denied call changed Store calls: before=%#v after=%#v", before, after)
	}
}

func TestCoordinatorReleasesAnIdleHalfOpenProbeForImmediateReplacement(t *testing.T) {
	t.Parallel()

	clock := coordinatorSecurityNewClock()
	breaker := coordinatorSecurityBreakerWithClock(t, clock)
	coordinatorSecurityOpenBreaker(t, breaker)
	clock.Advance(5 * time.Second)

	store := &relaySecurityStore{}
	coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(
		t,
		store,
		&relaySecurityEncoder{envelope: []byte("unused")},
		&relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()},
	), breaker)
	for attempt := 0; attempt < 2; attempt++ {
		result, err := coordinator.RunOne(context.Background())
		if err != nil || result.Outcome != OutcomeIdle {
			t.Fatalf("idle probe %d = %#v/%v, want Idle/nil", attempt+1, result, err)
		}
	}
	if calls := store.snapshot(); calls.claims != 2 || calls.acknowledgements != 0 || calls.failures != 0 {
		t.Fatalf("idle probe Store calls = %#v, want two Claims only", calls)
	}
}

func TestCoordinatorAllowsOnlyOneOutstandingHalfOpenProbe(t *testing.T) {
	t.Parallel()

	clock := coordinatorSecurityNewClock()
	breaker := coordinatorSecurityBreakerWithClock(t, clock)
	coordinatorSecurityOpenBreaker(t, breaker)
	clock.Advance(5 * time.Second)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	store := &relaySecurityStore{}
	store.claimHook = func(context.Context) {
		entered <- struct{}{}
		<-release
	}
	coordinator := coordinatorSecurityMustCoordinator(t, mustRelay(
		t,
		store,
		&relaySecurityEncoder{envelope: []byte("unused")},
		&relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()},
	), breaker)

	type callResult struct {
		result Result
		err    error
	}
	first := make(chan callResult, 1)
	go func() {
		result, err := coordinator.RunOne(context.Background())
		first <- callResult{result: result, err: err}
	}()
	<-entered

	const deniedCallers = 64
	results := make(chan callResult, deniedCallers)
	var group sync.WaitGroup
	group.Add(deniedCallers)
	for range deniedCallers {
		go func() {
			defer group.Done()
			result, err := coordinator.RunOne(context.Background())
			results <- callResult{result: result, err: err}
		}()
	}
	group.Wait()
	close(results)
	for got := range results {
		if got.err != nil || got.result.Outcome != OutcomeCircuitOpen {
			t.Fatalf("concurrent RunOne = %#v/%v, want CircuitOpen/nil", got.result, got.err)
		}
	}

	close(release)
	got := <-first
	if got.err != nil || got.result.Outcome != OutcomeIdle {
		t.Fatalf("half-open probe = %#v/%v, want Idle/nil", got.result, got.err)
	}
	if calls := store.snapshot(); calls.claims != 1 {
		t.Fatalf("Claim calls = %d, want exactly one", calls.claims)
	}
}
