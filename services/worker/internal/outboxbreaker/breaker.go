// Package outboxbreaker provides the process-local broker-health circuit
// breaker for one trusted transactional-outbox destination mapping.
package outboxbreaker

import (
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

const failureThreshold = 3

var openIntervals = [...]time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	40 * time.Second,
	60 * time.Second,
}

// Clock is the sole time source used by Breaker. Production lifecycle wiring
// supplies a monotonic-capable implementation; deterministic tests supply a
// controllable fake.
type Clock interface {
	Now() time.Time
}

// Decision is a stable, secret-safe result of an Acquire call.
type Decision string

const (
	DecisionDenied  Decision = "denied"
	DecisionAllowed Decision = "allowed"
	DecisionProbe   Decision = "probe"
)

func (decision Decision) String() string   { return decision.surface() }
func (decision Decision) GoString() string { return decision.surface() }
func (decision Decision) MarshalJSON() ([]byte, error) {
	return []byte(`"` + decision.surface() + `"`), nil
}

func (decision Decision) surface() string {
	if validDecision(decision) {
		return string(decision)
	}
	return "invalid"
}

// Observation is the complete set of broker-health evidence accepted by a
// Breaker. Event, encoder, and database outcomes intentionally have no value in
// this type and cannot affect broker health.
type Observation string

const (
	ObservationVerifiedPubAck        Observation = "verified-pub-ack"
	ObservationTransportUnavailable  Observation = "transport-unavailable"
	ObservationPublishOutcomeUnknown Observation = "publish-outcome-unknown"
)

func (observation Observation) String() string   { return observation.surface() }
func (observation Observation) GoString() string { return observation.surface() }
func (observation Observation) MarshalJSON() ([]byte, error) {
	return []byte(`"` + observation.surface() + `"`), nil
}

func (observation Observation) surface() string {
	if validObservation(observation) {
		return string(observation)
	}
	return "invalid"
}

// ErrorCode is a stable, secret-safe breaker failure category.
type ErrorCode string

const (
	ErrorInvalidInput  ErrorCode = "invalid-input"
	ErrorInvalidPermit ErrorCode = "invalid-permit"
	ErrorStalePermit   ErrorCode = "stale-permit"
	ErrorInvalidState  ErrorCode = "invalid-state"
)

func (code ErrorCode) String() string   { return code.surface() }
func (code ErrorCode) GoString() string { return code.surface() }
func (code ErrorCode) MarshalJSON() ([]byte, error) {
	return []byte(`"` + code.surface() + `"`), nil
}

func (code ErrorCode) surface() string {
	if validErrorCode(code) {
		return string(code)
	}
	return "invalid"
}

type breakerError struct{ code ErrorCode }

func (failure *breakerError) Error() string {
	return "transactional outbox breaker: " + failure.category().surface()
}

func (failure *breakerError) String() string   { return failure.Error() }
func (failure *breakerError) GoString() string { return failure.Error() }
func (failure *breakerError) MarshalJSON() ([]byte, error) {
	return []byte(`"` + failure.Error() + `"`), nil
}

func (failure *breakerError) category() ErrorCode {
	if failure == nil {
		return ""
	}
	return failure.code
}

func newError(code ErrorCode) *breakerError { return &breakerError{code: code} }

// ErrorCodeOf returns the stable category for errors produced by this module.
// It does not expose or unwrap implementation or destination details.
func ErrorCodeOf(err error) (ErrorCode, bool) {
	var failure *breakerError
	if !errors.As(err, &failure) || failure == nil {
		return "", false
	}
	code := failure.category()
	return code, validErrorCode(code)
}

// Permit is opaque one-use authority to report or release one acquisition.
// Copies share completion state and therefore cannot be consumed twice.
type Permit struct {
	breaker *Breaker
	record  *permitRecord
}

func (Permit) String() string   { return "<redacted-outbox-breaker-permit>" }
func (Permit) GoString() string { return "<redacted-outbox-breaker-permit>" }
func (Permit) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-breaker-permit]"`), nil
}

type permitRecord struct {
	decision  Decision
	completed bool
	issued    bool
}

// Breaker is a race-safe process-local circuit breaker bound to one trusted
// resolved destination. It is not durable correctness authority.
type Breaker struct {
	mutex sync.Mutex

	mapping outboxpublish.Mapping
	clock   Clock
	ready   bool

	consecutiveFailures uint8
	openIntervalIndex   uint8
	openUntil           time.Time
	probeOutstanding    bool
	probeRecord         *permitRecord
}

func (*Breaker) String() string   { return "<redacted-outbox-breaker>" }
func (*Breaker) GoString() string { return "<redacted-outbox-breaker>" }
func (*Breaker) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-breaker]"`), nil
}

// New constructs a startup-denied breaker for exactly one trusted mapping.
// Ready must observe successful startup readiness before Acquire can allow
// work.
func New(mapping outboxpublish.Mapping, clock Clock) (*Breaker, error) {
	if !mapping.Valid() || nilLike(clock) {
		return nil, newError(ErrorInvalidInput)
	}
	return &Breaker{
		mapping: mapping,
		clock:   clock,
	}, nil
}

// Valid reports whether Breaker was constructed with its immutable trusted
// mapping and clock dependencies. Startup readiness is intentionally not part
// of structural validity.
func (breaker *Breaker) Valid() bool {
	return breaker != nil && breaker.mapping.Valid() && !nilLike(breaker.clock)
}

// Ready records the one startup readiness success for this breaker instance.
// It cannot be reused as a reload-time state reset.
func (breaker *Breaker) Ready() error {
	if breaker == nil {
		return newError(ErrorInvalidInput)
	}
	breaker.mutex.Lock()
	defer breaker.mutex.Unlock()
	if breaker.ready {
		return newError(ErrorInvalidState)
	}
	breaker.ready = true
	return nil
}

// Acquire returns a one-use permit when broker work may begin. Startup and
// open-circuit denial are normal decisions, not errors.
func (breaker *Breaker) Acquire() (Permit, Decision, error) {
	if breaker == nil {
		return Permit{}, DecisionDenied, newError(ErrorInvalidInput)
	}
	breaker.mutex.Lock()
	defer breaker.mutex.Unlock()
	if !breaker.ready {
		return Permit{}, DecisionDenied, nil
	}

	decision := DecisionAllowed
	if !breaker.openUntil.IsZero() {
		if breaker.clock.Now().Before(breaker.openUntil) || breaker.probeOutstanding {
			return Permit{}, DecisionDenied, nil
		}
		decision = DecisionProbe
		breaker.probeOutstanding = true
	}

	record := &permitRecord{decision: decision, issued: true}
	if decision == DecisionProbe {
		breaker.probeRecord = record
	}
	return Permit{breaker: breaker, record: record}, decision, nil
}

// Observe consumes permit and applies one classified broker-health outcome.
func (breaker *Breaker) Observe(permit Permit, observation Observation) error {
	if breaker == nil {
		return newError(ErrorInvalidInput)
	}
	if !validObservation(observation) {
		return newError(ErrorInvalidInput)
	}

	breaker.mutex.Lock()
	defer breaker.mutex.Unlock()
	record, err := breaker.liveRecord(permit)
	if err != nil {
		return err
	}
	record.completed = true

	if observation == ObservationVerifiedPubAck {
		breaker.closeAndReset(record)
		return nil
	}

	if record.decision == DecisionProbe {
		breaker.probeOutstanding = false
		breaker.probeRecord = nil
		if breaker.openIntervalIndex < uint8(len(openIntervals)-1) {
			breaker.openIntervalIndex++
		}
		breaker.openUntil = breaker.clock.Now().Add(openIntervals[breaker.openIntervalIndex])
		return nil
	}

	// Outcomes from ordinary permits that complete after another permit opened
	// the breaker are consumed but cannot advance its half-open backoff.
	if !breaker.openUntil.IsZero() {
		return nil
	}
	breaker.consecutiveFailures++
	if breaker.consecutiveFailures == failureThreshold {
		breaker.consecutiveFailures = 0
		breaker.openIntervalIndex = 0
		breaker.openUntil = breaker.clock.Now().Add(openIntervals[0])
	}
	return nil
}

// Release consumes an unused permit without manufacturing broker-health
// evidence. Releasing a probe makes the already-expired interval immediately
// eligible for a replacement probe.
func (breaker *Breaker) Release(permit Permit) error {
	if breaker == nil {
		return newError(ErrorInvalidInput)
	}
	breaker.mutex.Lock()
	defer breaker.mutex.Unlock()
	record, err := breaker.liveRecord(permit)
	if err != nil {
		return err
	}
	record.completed = true
	if record.decision == DecisionProbe {
		breaker.probeOutstanding = false
		breaker.probeRecord = nil
	}
	return nil
}

func (breaker *Breaker) liveRecord(permit Permit) (*permitRecord, error) {
	if permit.breaker == nil || permit.record == nil || permit.breaker != breaker {
		return nil, newError(ErrorInvalidPermit)
	}
	record := permit.record
	if !record.issued || (record.decision != DecisionAllowed && record.decision != DecisionProbe) {
		return nil, newError(ErrorInvalidPermit)
	}
	if record.completed {
		return nil, newError(ErrorStalePermit)
	}
	return record, nil
}

func (breaker *Breaker) closeAndReset(observed *permitRecord) {
	if breaker.probeRecord != nil && breaker.probeRecord != observed {
		breaker.probeRecord.completed = true
	}
	breaker.consecutiveFailures = 0
	breaker.openIntervalIndex = 0
	breaker.openUntil = time.Time{}
	breaker.probeOutstanding = false
	breaker.probeRecord = nil
}

func validDecision(decision Decision) bool {
	return decision == DecisionDenied || decision == DecisionAllowed || decision == DecisionProbe
}

func validObservation(observation Observation) bool {
	switch observation {
	case ObservationVerifiedPubAck, ObservationTransportUnavailable, ObservationPublishOutcomeUnknown:
		return true
	default:
		return false
	}
}

func validErrorCode(code ErrorCode) bool {
	switch code {
	case ErrorInvalidInput, ErrorInvalidPermit, ErrorStalePermit, ErrorInvalidState:
		return true
	default:
		return false
	}
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	kind := reflect.ValueOf(value).Kind()
	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(value).IsNil()
	default:
		return false
	}
}
