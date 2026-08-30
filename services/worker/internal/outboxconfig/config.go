// Package outboxconfig parses the frozen Transactional Outbox deployment
// policy into a redacted, immutable Worker snapshot.
package outboxconfig

import (
	"encoding/json"
	"errors"
	"time"
)

const PolicyV1ID = "threadline.outbox.policy/v1"

// Backoff is one immutable database-authored retry profile.
type Backoff struct {
	base time.Duration
	cap  time.Duration
}

func (Backoff) String() string   { return "<redacted-outbox-config-backoff>" }
func (Backoff) GoString() string { return "<redacted-outbox-config-backoff>" }
func (Backoff) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-config-backoff]")
}
func (backoff Backoff) Base() time.Duration { return backoff.base }
func (backoff Backoff) Cap() time.Duration  { return backoff.cap }

// Snapshot is a validated v1 policy value. It contains no caller-owned memory.
type Snapshot struct {
	policyID        string
	payloadBytes    uint32
	wireBytes       uint32
	batchSize       uint32
	lease           time.Duration
	absolute        time.Duration
	eventCeiling    uint32
	transport       Backoff
	unknown         Backoff
	event           Backoff
	retentionDays   uint32
	duplicateWindow time.Duration
	digest          [32]byte
}

func (Snapshot) String() string   { return "<redacted-outbox-config-snapshot>" }
func (Snapshot) GoString() string { return "<redacted-outbox-config-snapshot>" }
func (Snapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-config-snapshot]")
}

func (snapshot Snapshot) PolicyID() string                { return snapshot.policyID }
func (snapshot Snapshot) PayloadHardBytes() uint32        { return snapshot.payloadBytes }
func (snapshot Snapshot) WireHardBytes() uint32           { return snapshot.wireBytes }
func (snapshot Snapshot) BatchSize() uint32               { return snapshot.batchSize }
func (snapshot Snapshot) Lease() time.Duration            { return snapshot.lease }
func (snapshot Snapshot) AbsoluteLifetime() time.Duration { return snapshot.absolute }
func (snapshot Snapshot) EventRetryCeiling() uint32       { return snapshot.eventCeiling }
func (snapshot Snapshot) TransportBackoff() Backoff       { return snapshot.transport }
func (snapshot Snapshot) UnknownBackoff() Backoff         { return snapshot.unknown }
func (snapshot Snapshot) EventBackoff() Backoff           { return snapshot.event }
func (snapshot Snapshot) RetentionDays() uint32           { return snapshot.retentionDays }
func (snapshot Snapshot) DuplicateWindow() time.Duration  { return snapshot.duplicateWindow }
func (snapshot Snapshot) Digest() [32]byte                { return snapshot.digest }

// Valid reports whether Snapshot was produced intact by Parse. It lets later
// assembly reject zero values without reimplementing the frozen policy.
func (snapshot Snapshot) Valid() bool {
	if snapshot.policyID != PolicyV1ID {
		return false
	}
	values := snapshot.values()
	return values.valid() && newSnapshot(snapshot.policyID, values) == snapshot
}

func (snapshot Snapshot) values() policyValues {
	return policyValues{
		payloadHardBytes:   snapshot.payloadBytes,
		wireHardBytes:      snapshot.wireBytes,
		batchSize:          snapshot.batchSize,
		leaseMS:            uint32(snapshot.lease / time.Millisecond),
		absoluteLifetimeMS: uint32(snapshot.absolute / time.Millisecond),
		eventRetryCeiling:  snapshot.eventCeiling,
		transportBaseMS:    uint32(snapshot.transport.base / time.Millisecond),
		transportCapMS:     uint32(snapshot.transport.cap / time.Millisecond),
		unknownBaseMS:      uint32(snapshot.unknown.base / time.Millisecond),
		unknownCapMS:       uint32(snapshot.unknown.cap / time.Millisecond),
		eventBaseMS:        uint32(snapshot.event.base / time.Millisecond),
		eventCapMS:         uint32(snapshot.event.cap / time.Millisecond),
		retentionDays:      snapshot.retentionDays,
		duplicateWindowMS:  uint32(snapshot.duplicateWindow / time.Millisecond),
	}
}

type ErrorCode string

const ErrorInvalidInput ErrorCode = "invalid-input"

func (ErrorCode) String() string   { return "<redacted-outbox-config-error-code>" }
func (ErrorCode) GoString() string { return "<redacted-outbox-config-error-code>" }
func (ErrorCode) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-config-error-code]")
}

// ErrorCodeOf exposes only the stable category produced by this module.
func ErrorCodeOf(err error) (ErrorCode, bool) {
	var failure *parseFailure
	if !errors.As(err, &failure) || failure == nil {
		return "", false
	}
	return ErrorInvalidInput, true
}

type parseFailure struct{}

func (*parseFailure) Error() string    { return "outbox-config-invalid-input" }
func (*parseFailure) String() string   { return "<redacted-outbox-config-error>" }
func (*parseFailure) GoString() string { return "<redacted-outbox-config-error>" }
func (*parseFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-config-error]")
}

func invalidInput() error { return &parseFailure{} }
