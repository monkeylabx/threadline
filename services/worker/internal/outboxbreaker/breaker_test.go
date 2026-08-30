package outboxbreaker

import (
	"sync"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

var breakerEpoch = time.Date(2026, time.August, 24, 0, 0, 0, 0, time.UTC)

type breakerFakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newBreakerFakeClock() *breakerFakeClock {
	return &breakerFakeClock{now: breakerEpoch}
}

func (clock *breakerFakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *breakerFakeClock) Advance(elapsed time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(elapsed)
}

func TestBreakerStartsOpenAndReadinessIsOneShot(t *testing.T) {
	breaker, _ := newFunctionalBreaker(t, false)
	assertAcquireDecision(t, breaker, DecisionDenied)

	if err := breaker.Ready(); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	permit := assertAcquireDecision(t, breaker, DecisionAllowed)
	if err := breaker.Release(permit); err != nil {
		t.Fatalf("Release(ordinary permit) error = %v", err)
	}

	tripBreaker(t, breaker)
	assertAcquireDecision(t, breaker, DecisionDenied)
	assertBreakerError(t, breaker.Ready(), ErrorInvalidState)
	assertAcquireDecision(t, breaker, DecisionDenied)
}

func TestBreakerValidDistinguishesConstructedAndZeroValues(t *testing.T) {
	t.Parallel()

	var nilBreaker *Breaker
	if nilBreaker.Valid() || (&Breaker{}).Valid() {
		t.Fatal("nil or zero-value Breaker reported structurally valid")
	}
	breaker, _ := newFunctionalBreaker(t, true)
	if !breaker.Valid() {
		t.Fatal("constructed Breaker reported invalid")
	}
	tripBreaker(t, breaker)
	if !breaker.Valid() {
		t.Fatal("open Breaker reported structurally invalid")
	}
}

func TestBreakerCountsOnlyConsecutiveInfrastructureOutcomes(t *testing.T) {
	breaker, _ := newFunctionalBreaker(t, true)

	first := assertAcquireDecision(t, breaker, DecisionAllowed)
	if err := breaker.Observe(first, ObservationTransportUnavailable); err != nil {
		t.Fatalf("Observe(first transport failure) error = %v", err)
	}
	second := assertAcquireDecision(t, breaker, DecisionAllowed)
	if err := breaker.Observe(second, ObservationPublishOutcomeUnknown); err != nil {
		t.Fatalf("Observe(second unknown outcome) error = %v", err)
	}
	assertAllowedAndRelease(t, breaker)

	acknowledged := assertAcquireDecision(t, breaker, DecisionAllowed)
	if err := breaker.Observe(acknowledged, ObservationVerifiedPubAck); err != nil {
		t.Fatalf("Observe(verified acknowledgement) error = %v", err)
	}

	for index, observation := range []Observation{
		ObservationPublishOutcomeUnknown,
		ObservationTransportUnavailable,
		ObservationTransportUnavailable,
	} {
		permit := assertAcquireDecision(t, breaker, DecisionAllowed)
		if err := breaker.Observe(permit, observation); err != nil {
			t.Fatalf("Observe(post-ack failure %d) error = %v", index+1, err)
		}
		if index < 2 {
			assertAllowedAndRelease(t, breaker)
		}
	}
	assertAcquireDecision(t, breaker, DecisionDenied)
}

func TestBreakerUsesExactCappedHalfOpenSchedule(t *testing.T) {
	breaker, clock := newFunctionalBreaker(t, true)
	tripBreaker(t, breaker)

	for attempt, interval := range []time.Duration{
		5 * time.Second,
		10 * time.Second,
		20 * time.Second,
		40 * time.Second,
		60 * time.Second,
		60 * time.Second,
	} {
		assertAcquireDecision(t, breaker, DecisionDenied)
		clock.Advance(interval - time.Nanosecond)
		assertAcquireDecision(t, breaker, DecisionDenied)
		clock.Advance(time.Nanosecond)

		probe := assertAcquireDecision(t, breaker, DecisionProbe)
		assertAcquireDecision(t, breaker, DecisionDenied)
		if err := breaker.Observe(probe, ObservationTransportUnavailable); err != nil {
			t.Fatalf("Observe(probe %d) error = %v", attempt+1, err)
		}
	}
}

func TestBreakerAllowsOnlyOneProbeAndReleaseMakesItImmediatelyAvailable(t *testing.T) {
	breaker, clock := newFunctionalBreaker(t, true)
	tripBreaker(t, breaker)
	clock.Advance(5 * time.Second)

	probe := assertAcquireDecision(t, breaker, DecisionProbe)
	assertAcquireDecision(t, breaker, DecisionDenied)
	if err := breaker.Release(probe); err != nil {
		t.Fatalf("Release(probe) error = %v", err)
	}

	replacement := assertAcquireDecision(t, breaker, DecisionProbe)
	if replacement == probe {
		t.Fatal("released probe permit was reused")
	}
	if err := breaker.Observe(replacement, ObservationVerifiedPubAck); err != nil {
		t.Fatalf("Observe(replacement acknowledgement) error = %v", err)
	}
	assertAcquireDecision(t, breaker, DecisionAllowed)
}

func TestBreakerHandlesOutstandingOrdinaryPermitsWhileOpen(t *testing.T) {
	t.Run("infrastructure result does not move probe deadline", func(t *testing.T) {
		breaker, clock := newFunctionalBreaker(t, true)
		permits := acquireOrdinaryPermits(t, breaker, 4)
		for index := 0; index < 3; index++ {
			if err := breaker.Observe(permits[index], ObservationTransportUnavailable); err != nil {
				t.Fatalf("Observe(opening failure %d) error = %v", index+1, err)
			}
		}

		clock.Advance(4 * time.Second)
		if err := breaker.Observe(permits[3], ObservationPublishOutcomeUnknown); err != nil {
			t.Fatalf("Observe(outstanding ordinary failure) error = %v", err)
		}
		clock.Advance(time.Second)
		assertAcquireDecision(t, breaker, DecisionProbe)
	})

	t.Run("verified acknowledgement closes and resets", func(t *testing.T) {
		breaker, _ := newFunctionalBreaker(t, true)
		permits := acquireOrdinaryPermits(t, breaker, 4)
		for index := 0; index < 3; index++ {
			if err := breaker.Observe(permits[index], ObservationTransportUnavailable); err != nil {
				t.Fatalf("Observe(opening failure %d) error = %v", index+1, err)
			}
		}
		if err := breaker.Observe(permits[3], ObservationVerifiedPubAck); err != nil {
			t.Fatalf("Observe(outstanding acknowledgement) error = %v", err)
		}
		for index := 0; index < 2; index++ {
			permit := assertAcquireDecision(t, breaker, DecisionAllowed)
			if err := breaker.Observe(permit, ObservationTransportUnavailable); err != nil {
				t.Fatalf("Observe(reset failure %d) error = %v", index+1, err)
			}
		}
		assertAcquireDecision(t, breaker, DecisionAllowed)
	})
}

func TestBreakerCompletionOrderPreservesOrdinaryEvidenceAcrossAckReset(t *testing.T) {
	breaker, _ := newFunctionalBreaker(t, true)
	permits := acquireOrdinaryPermits(t, breaker, 5)

	if err := breaker.Observe(permits[0], ObservationTransportUnavailable); err != nil {
		t.Fatalf("Observe(initial failure) error = %v", err)
	}
	if err := breaker.Observe(permits[1], ObservationVerifiedPubAck); err != nil {
		t.Fatalf("Observe(reset acknowledgement) error = %v", err)
	}
	for index, permit := range permits[2:] {
		if err := breaker.Observe(permit, ObservationPublishOutcomeUnknown); err != nil {
			t.Fatalf("Observe(post-ack completion %d) error = %v", index+1, err)
		}
	}
	assertAcquireDecision(t, breaker, DecisionDenied)
}

func TestBreakerOrdinaryAckMakesOutstandingProbeStale(t *testing.T) {
	breaker, clock := newFunctionalBreaker(t, true)
	permits := acquireOrdinaryPermits(t, breaker, 4)
	for index := 0; index < 3; index++ {
		if err := breaker.Observe(permits[index], ObservationTransportUnavailable); err != nil {
			t.Fatalf("Observe(opening failure %d) error = %v", index+1, err)
		}
	}
	clock.Advance(5 * time.Second)
	probe := assertAcquireDecision(t, breaker, DecisionProbe)

	if err := breaker.Observe(permits[3], ObservationVerifiedPubAck); err != nil {
		t.Fatalf("Observe(outstanding ordinary acknowledgement) error = %v", err)
	}
	assertBreakerError(t, breaker.Observe(probe, ObservationTransportUnavailable), ErrorStalePermit)
	assertBreakerError(t, breaker.Release(probe), ErrorStalePermit)
	assertAcquireDecision(t, breaker, DecisionAllowed)
}

func TestBreakerRejectsMalformedForeignAndCompletedPermits(t *testing.T) {
	first, _ := newFunctionalBreaker(t, true)
	second, _ := newFunctionalBreaker(t, true)

	var zero Permit
	assertBreakerError(t, first.Observe(zero, ObservationVerifiedPubAck), ErrorInvalidPermit)
	assertBreakerError(t, first.Release(zero), ErrorInvalidPermit)

	foreign := assertAcquireDecision(t, first, DecisionAllowed)
	assertBreakerError(t, second.Observe(foreign, ObservationTransportUnavailable), ErrorInvalidPermit)
	assertBreakerError(t, second.Release(foreign), ErrorInvalidPermit)

	observed := assertAcquireDecision(t, first, DecisionAllowed)
	if err := first.Observe(observed, ObservationVerifiedPubAck); err != nil {
		t.Fatalf("Observe(valid permit) error = %v", err)
	}
	assertBreakerError(t, first.Observe(observed, ObservationVerifiedPubAck), ErrorStalePermit)
	assertBreakerError(t, first.Release(observed), ErrorStalePermit)

	released := assertAcquireDecision(t, first, DecisionAllowed)
	if err := first.Release(released); err != nil {
		t.Fatalf("Release(valid permit) error = %v", err)
	}
	assertBreakerError(t, first.Release(released), ErrorStalePermit)
	assertBreakerError(t, first.Observe(released, ObservationTransportUnavailable), ErrorStalePermit)
}

func TestBreakerRejectsInvalidObservationWithoutConsumingPermit(t *testing.T) {
	breaker, _ := newFunctionalBreaker(t, true)
	permit := assertAcquireDecision(t, breaker, DecisionAllowed)
	assertBreakerError(t, breaker.Observe(permit, Observation("not-an-observation")), ErrorInvalidInput)
	if err := breaker.Observe(permit, ObservationVerifiedPubAck); err != nil {
		t.Fatalf("Observe(valid retry after invalid observation) error = %v", err)
	}
}

func TestBreakerConcurrentAcquireIssuesAtMostOneProbe(t *testing.T) {
	breaker, clock := newFunctionalBreaker(t, true)
	tripBreaker(t, breaker)
	clock.Advance(5 * time.Second)

	const callers = 64
	type result struct {
		permit   Permit
		decision Decision
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for index := 0; index < callers; index++ {
		go func() {
			defer wait.Done()
			<-start
			permit, decision, err := breaker.Acquire()
			results <- result{permit: permit, decision: decision, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	var probe Permit
	probeCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("Acquire() error = %v", result.err)
		}
		switch result.decision {
		case DecisionProbe:
			probeCount++
			probe = result.permit
		case DecisionDenied:
			if result.permit != (Permit{}) {
				t.Fatal("denied acquire returned a non-zero permit")
			}
		default:
			t.Fatalf("Acquire() decision = %q, want probe or denied", result.decision)
		}
	}
	if probeCount != 1 {
		t.Fatalf("probe count = %d, want 1", probeCount)
	}
	if err := breaker.Release(probe); err != nil {
		t.Fatalf("Release(winning probe) error = %v", err)
	}
	assertAcquireDecision(t, breaker, DecisionProbe)
}

func newFunctionalBreaker(t *testing.T, ready bool) (*Breaker, *breakerFakeClock) {
	t.Helper()
	clock := newBreakerFakeClock()
	breaker, err := New(functionalMapping(), clock)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if ready {
		if err := breaker.Ready(); err != nil {
			t.Fatalf("Ready() error = %v", err)
		}
	}
	return breaker, clock
}

func functionalMapping() outboxpublish.Mapping {
	return outboxpublish.Mapping{
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		Stream:             "DOMAIN_EVENTS",
		Subject:            "threadline.domain.events.v1",
	}
}

func acquireOrdinaryPermits(t *testing.T, breaker *Breaker, count int) []Permit {
	t.Helper()
	permits := make([]Permit, count)
	for index := range permits {
		permits[index] = assertAcquireDecision(t, breaker, DecisionAllowed)
	}
	return permits
}

func tripBreaker(t *testing.T, breaker *Breaker) {
	t.Helper()
	for index := 0; index < 3; index++ {
		permit := assertAcquireDecision(t, breaker, DecisionAllowed)
		if err := breaker.Observe(permit, ObservationTransportUnavailable); err != nil {
			t.Fatalf("Observe(opening failure %d) error = %v", index+1, err)
		}
	}
}

func assertAllowedAndRelease(t *testing.T, breaker *Breaker) {
	t.Helper()
	permit := assertAcquireDecision(t, breaker, DecisionAllowed)
	if err := breaker.Release(permit); err != nil {
		t.Fatalf("Release(allowed permit) error = %v", err)
	}
}

func assertAcquireDecision(t *testing.T, breaker *Breaker, want Decision) Permit {
	t.Helper()
	permit, decision, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if decision != want {
		t.Fatalf("Acquire() decision = %q, want %q", decision, want)
	}
	if want == DecisionDenied && permit != (Permit{}) {
		t.Fatal("denied Acquire() returned a non-zero permit")
	}
	if want != DecisionDenied && permit == (Permit{}) {
		t.Fatalf("Acquire() returned zero permit for %q decision", want)
	}
	return permit
}

func assertBreakerError(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %q", want)
	}
	code, ok := ErrorCodeOf(err)
	if !ok || code != want {
		t.Fatalf("ErrorCodeOf(%v) = (%q, %t), want (%q, true)", err, code, ok, want)
	}
}
