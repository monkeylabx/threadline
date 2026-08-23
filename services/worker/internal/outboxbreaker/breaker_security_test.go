package outboxbreaker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

const (
	breakerSecurityStream  = "SECURITY_STREAM_DO_NOT_LEAK"
	breakerSecuritySubject = "security.subject.do-not-leak"
)

var breakerSecurityStart = time.Date(2042, time.July, 8, 9, 10, 11, 123456789, time.UTC)

func TestBreakerConstructorRejectsNilAndTypedNilClocks(t *testing.T) {
	t.Parallel()

	var typedNilClock *breakerFakeClock
	for _, test := range []struct {
		name  string
		clock Clock
	}{
		{name: "nil"},
		{name: "typed nil", clock: typedNilClock},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			breaker, err := New(breakerSecurityMapping(), test.clock)
			if breaker != nil || breakerSecurityErrorCode(err) != ErrorInvalidInput {
				t.Fatalf("New = %#v/%v, want nil/invalid-input", breaker, err)
			}
			breakerSecurityAssertRedacted(t, err)
		})
	}
}

func TestBreakerConstructorRejectsMalformedMappingsWithoutLeakingThem(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		mapping outboxpublish.Mapping
	}{
		{name: "zero"},
		{name: "wrong logical destination", mapping: outboxpublish.Mapping{LogicalDestination: "private-destination-do-not-leak", Stream: breakerSecurityStream, Subject: breakerSecuritySubject}},
		{name: "empty stream", mapping: outboxpublish.Mapping{LogicalDestination: outboxpublish.LogicalDestinationDomainEvents, Subject: breakerSecuritySubject}},
		{name: "wildcard stream", mapping: outboxpublish.Mapping{LogicalDestination: outboxpublish.LogicalDestinationDomainEvents, Stream: "PRIVATE.*", Subject: breakerSecuritySubject}},
		{name: "empty subject", mapping: outboxpublish.Mapping{LogicalDestination: outboxpublish.LogicalDestinationDomainEvents, Stream: breakerSecurityStream}},
		{name: "wildcard subject", mapping: outboxpublish.Mapping{LogicalDestination: outboxpublish.LogicalDestinationDomainEvents, Stream: breakerSecurityStream, Subject: "private.>"}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			breaker, err := New(test.mapping, &breakerFakeClock{now: breakerSecurityStart})
			if breaker != nil || breakerSecurityErrorCode(err) != ErrorInvalidInput {
				t.Fatalf("New = %#v/%v, want nil/invalid-input", breaker, err)
			}
			breakerSecurityAssertRedacted(t, err)
		})
	}
}

func TestBreakerRejectsNilReceiverAndMalformedCalls(t *testing.T) {
	t.Parallel()

	var nilBreaker *Breaker
	if err := nilBreaker.Ready(); breakerSecurityErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("nil Ready error = %v, want invalid-input", err)
	}
	permit, decision, err := nilBreaker.Acquire()
	if decision != DecisionDenied || breakerSecurityErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("nil Acquire = %#v/%v/%v, want zero/denied/invalid-input", permit, decision, err)
	}
	if err := nilBreaker.Observe(Permit{}, ObservationVerifiedPubAck); breakerSecurityErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("nil Observe error = %v, want invalid-input", err)
	}
	if err := nilBreaker.Release(Permit{}); breakerSecurityErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("nil Release error = %v, want invalid-input", err)
	}

	breaker := breakerSecurityNew(t)
	if err := breaker.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if err := breaker.Ready(); breakerSecurityErrorCode(err) != ErrorInvalidState {
		t.Fatalf("duplicate Ready error = %v, want invalid-state", err)
	} else {
		breakerSecurityAssertRedacted(t, err)
	}
	validPermit := breakerSecurityAcquire(t, breaker, DecisionAllowed)
	if err := breaker.Observe(validPermit, Observation("invalid-observation")); breakerSecurityErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("invalid observation error = %v, want invalid-input", err)
	}
	if err := breaker.Observe(validPermit, ObservationVerifiedPubAck); err != nil {
		t.Fatalf("valid observation after rejected input: %v", err)
	}
	if err := breaker.Observe(Permit{}, ObservationVerifiedPubAck); breakerSecurityErrorCode(err) != ErrorInvalidPermit {
		t.Fatalf("zero Permit Observe error = %v, want invalid-permit", err)
	}
	if err := breaker.Release(Permit{}); breakerSecurityErrorCode(err) != ErrorInvalidPermit {
		t.Fatalf("zero Permit Release error = %v, want invalid-permit", err)
	}
	for _, malformed := range []Permit{
		{breaker: breaker, record: &permitRecord{issued: false, decision: DecisionAllowed}},
		{breaker: breaker, record: &permitRecord{issued: true, decision: Decision("invalid-decision")}},
	} {
		if err := breaker.Observe(malformed, ObservationVerifiedPubAck); breakerSecurityErrorCode(err) != ErrorInvalidPermit {
			t.Fatalf("malformed Permit Observe error = %v, want invalid-permit", err)
		}
		if err := breaker.Release(malformed); breakerSecurityErrorCode(err) != ErrorInvalidPermit {
			t.Fatalf("malformed Permit Release error = %v, want invalid-permit", err)
		}
	}
}

func TestBreakerRejectsForeignDuplicateAndCompletedPermitsWithoutStateChange(t *testing.T) {
	t.Parallel()

	first := breakerSecurityNew(t)
	second := breakerSecurityNew(t)
	if err := first.Ready(); err != nil {
		t.Fatalf("first Ready: %v", err)
	}
	if err := second.Ready(); err != nil {
		t.Fatalf("second Ready: %v", err)
	}

	foreign := breakerSecurityAcquire(t, first, DecisionAllowed)
	if err := second.Observe(foreign, ObservationTransportUnavailable); breakerSecurityErrorCode(err) != ErrorInvalidPermit {
		t.Fatalf("foreign Observe error = %v, want invalid-permit", err)
	}
	if err := second.Release(foreign); breakerSecurityErrorCode(err) != ErrorInvalidPermit {
		t.Fatalf("foreign Release error = %v, want invalid-permit", err)
	}
	if err := first.Release(foreign); err != nil {
		t.Fatalf("owner Release: %v", err)
	}
	if err := first.Release(foreign); breakerSecurityErrorCode(err) != ErrorStalePermit {
		t.Fatalf("duplicate Release error = %v, want stale-permit", err)
	}
	if err := first.Observe(foreign, ObservationVerifiedPubAck); breakerSecurityErrorCode(err) != ErrorStalePermit {
		t.Fatalf("released Observe error = %v, want stale-permit", err)
	}

	completed := breakerSecurityAcquire(t, first, DecisionAllowed)
	if err := first.Observe(completed, ObservationVerifiedPubAck); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if err := first.Observe(completed, ObservationTransportUnavailable); breakerSecurityErrorCode(err) != ErrorStalePermit {
		t.Fatalf("duplicate Observe error = %v, want stale-permit", err)
	}
	if err := first.Release(completed); breakerSecurityErrorCode(err) != ErrorStalePermit {
		t.Fatalf("completed Release error = %v, want stale-permit", err)
	}

	breakerSecurityAcquire(t, first, DecisionAllowed)
}

func TestBreakerFormattingJSONAndErrorsStayStableAndRedacted(t *testing.T) {
	t.Parallel()

	first := breakerSecurityNew(t)
	secondClock := &breakerFakeClock{now: breakerSecurityStart.Add(99 * time.Hour)}
	second, err := New(outboxpublish.Mapping{
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		Stream:             "OTHER_PRIVATE_STREAM",
		Subject:            "other.private.subject",
	}, secondClock)
	if err != nil {
		t.Fatalf("New second: %v", err)
	}
	if err := first.Ready(); err != nil {
		t.Fatalf("first Ready: %v", err)
	}
	if err := second.Ready(); err != nil {
		t.Fatalf("second Ready: %v", err)
	}
	firstPermit := breakerSecurityAcquire(t, first, DecisionAllowed)
	secondPermit := breakerSecurityAcquire(t, second, DecisionAllowed)
	if err := first.Release(firstPermit); err != nil {
		t.Fatalf("first Release: %v", err)
	}
	if err := second.Release(secondPermit); err != nil {
		t.Fatalf("second Release: %v", err)
	}
	firstErr := first.Release(firstPermit)
	secondErr := second.Release(secondPermit)

	for _, value := range []any{first, second, firstPermit, secondPermit, firstErr, secondErr} {
		breakerSecurityAssertRedacted(t, value)
	}
	breakerSecurityAssertSameSurface(t, "Breaker String", fmt.Sprint(first), fmt.Sprint(second))
	breakerSecurityAssertSameSurface(t, "Breaker GoString", fmt.Sprintf("%#v", first), fmt.Sprintf("%#v", second))
	breakerSecurityAssertSameJSON(t, "Breaker JSON", first, second)
	breakerSecurityAssertSameSurface(t, "Permit String", fmt.Sprint(firstPermit), fmt.Sprint(secondPermit))
	breakerSecurityAssertSameSurface(t, "Permit GoString", fmt.Sprintf("%#v", firstPermit), fmt.Sprintf("%#v", secondPermit))
	breakerSecurityAssertSameJSON(t, "Permit JSON", firstPermit, secondPermit)
	breakerSecurityAssertSameSurface(t, "error", firstErr.Error(), secondErr.Error())
	breakerSecurityAssertSameJSON(t, "error JSON", firstErr, secondErr)

	for _, decision := range []Decision{DecisionDenied, DecisionAllowed, DecisionProbe} {
		breakerSecurityAssertRedacted(t, decision)
		breakerSecurityAssertRedactedJSON(t, decision)
	}
	for _, observation := range []Observation{
		ObservationVerifiedPubAck,
		ObservationTransportUnavailable,
		ObservationPublishOutcomeUnknown,
	} {
		breakerSecurityAssertRedacted(t, observation)
		breakerSecurityAssertRedactedJSON(t, observation)
	}

	if _, ok := ErrorCodeOf(nil); ok {
		t.Fatal("ErrorCodeOf(nil) unexpectedly matched")
	}
	if _, ok := ErrorCodeOf(errors.New("raw broker secret")); ok {
		t.Fatal("ErrorCodeOf(raw error) unexpectedly matched")
	}
}

func TestBreakerConcurrentAcquireObserveAndReleaseIsRaceSafe(t *testing.T) {
	t.Parallel()

	const callers = 512
	breaker := breakerSecurityNew(t)
	if err := breaker.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	startAcquire := make(chan struct{})
	acquired := make([]breakerSecurityAcquireResult, callers)
	var group sync.WaitGroup
	group.Add(callers)
	for index := range callers {
		index := index
		go func() {
			defer group.Done()
			<-startAcquire
			permit, decision, err := breaker.Acquire()
			acquired[index] = breakerSecurityAcquireResult{permit: permit, decision: decision, err: err}
		}()
	}
	close(startAcquire)
	group.Wait()
	for index, result := range acquired {
		if result.err != nil || result.decision != DecisionAllowed {
			t.Fatalf("Acquire %d = %#v/%v/%v, want permit/allowed/nil", index, result.permit, result.decision, result.err)
		}
	}

	startCompletion := make(chan struct{})
	errorsSeen := make(chan error, callers)
	group.Add(callers)
	for index := range callers {
		index := index
		go func() {
			defer group.Done()
			<-startCompletion
			var err error
			if index%2 == 0 {
				err = breaker.Observe(acquired[index].permit, ObservationTransportUnavailable)
			} else {
				err = breaker.Release(acquired[index].permit)
			}
			if err != nil {
				errorsSeen <- err
			}
		}()
	}
	close(startCompletion)
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Errorf("concurrent breaker operation: %v", err)
	}
	if permit, decision, err := breaker.Acquire(); err != nil || decision != DecisionDenied {
		t.Fatalf("Acquire after concurrent infrastructure failures = %#v/%v/%v, want zero/denied/nil", permit, decision, err)
	}
}

type breakerSecurityAcquireResult struct {
	permit   Permit
	decision Decision
	err      error
}

func breakerSecurityMapping() outboxpublish.Mapping {
	return outboxpublish.Mapping{
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		Stream:             breakerSecurityStream,
		Subject:            breakerSecuritySubject,
	}
}

func breakerSecurityNew(t *testing.T) *Breaker {
	t.Helper()
	breaker, err := New(breakerSecurityMapping(), &breakerFakeClock{now: breakerSecurityStart})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return breaker
}

func breakerSecurityAcquire(t *testing.T, breaker *Breaker, want Decision) Permit {
	t.Helper()
	permit, decision, err := breaker.Acquire()
	if err != nil || decision != want {
		t.Fatalf("Acquire = %#v/%v/%v, want permit/%v/nil", permit, decision, err, want)
	}
	return permit
}

func breakerSecurityErrorCode(err error) ErrorCode {
	code, _ := ErrorCodeOf(err)
	return code
}

func breakerSecurityAssertRedacted(t *testing.T, value any) {
	t.Helper()
	for _, formatted := range []string{fmt.Sprint(value), fmt.Sprintf("%#v", value)} {
		breakerSecurityAssertSafeText(t, formatted)
	}
	breakerSecurityAssertRedactedJSON(t, value)
}

func breakerSecurityAssertRedactedJSON(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", value, err)
	}
	breakerSecurityAssertSafeText(t, string(encoded))
}

func breakerSecurityAssertSafeText(t *testing.T, value string) {
	t.Helper()
	for _, forbidden := range []string{
		outboxpublish.LogicalDestinationDomainEvents,
		breakerSecurityStream,
		breakerSecuritySubject,
		"OTHER_PRIVATE_STREAM",
		"other.private.subject",
		"private-destination-do-not-leak",
		"PRIVATE.*",
		"private.>",
		breakerSecurityStart.Format(time.RFC3339),
		breakerSecurityStart.Format(time.RFC3339Nano),
		breakerSecurityStart.Add(99 * time.Hour).Format(time.RFC3339),
		breakerSecurityStart.Add(99 * time.Hour).Format(time.RFC3339Nano),
		"raw broker secret",
	} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("surface %q leaked %q", value, forbidden)
		}
	}
}

func breakerSecurityAssertSameSurface(t *testing.T, name, first, second string) {
	t.Helper()
	if first != second {
		t.Fatalf("%s differs by instance: %q != %q", name, first, second)
	}
}

func breakerSecurityAssertSameJSON(t *testing.T, name string, first, second any) {
	t.Helper()
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("json.Marshal first %s: %v", name, err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("json.Marshal second %s: %v", name, err)
	}
	breakerSecurityAssertSameSurface(t, name, string(firstJSON), string(secondJSON))
}
