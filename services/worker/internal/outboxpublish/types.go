package outboxpublish

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"unicode"
)

const (
	// LogicalDestinationDomainEvents is the sole trusted v1 outbox target.
	LogicalDestinationDomainEvents = "domain-events"
	wireHardBytes                  = 327_680
	maximumStreamBytes             = 255
	maximumSubjectBytes            = 1_024
)

// Publisher is the module's one-method seam. The production JetStream adapter
// and scripted test adapter both satisfy it.
type Publisher interface {
	Publish(context.Context, Message) (Acknowledgement, error)
}

// Mapping is the trusted constructor-time binding for the sole v1 logical
// destination. Message inputs cannot override any of these values.
type Mapping struct {
	LogicalDestination string
	Stream             string
	Subject            string
}

// Valid reports whether Mapping names the sole trusted v1 logical
// destination and one exact concrete Stream and subject. It lets later Worker
// modules bind the already-reviewed mapping without duplicating its rules.
func (mapping Mapping) Valid() bool { return validMapping(mapping) }

func (Mapping) String() string   { return "<redacted-outbox-publish-mapping>" }
func (Mapping) GoString() string { return "<redacted-outbox-publish-mapping>" }
func (Mapping) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-publish-mapping]"`), nil
}

// Message carries a complete, already encoded Event envelope and the exact
// deterministic message ID obtained from the database Claim.
type Message struct {
	Body      []byte
	MessageID string
}

func (Message) String() string   { return "<redacted-outbox-publish-message>" }
func (Message) GoString() string { return "<redacted-outbox-publish-message>" }
func (Message) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-publish-message]"`), nil
}

// Acknowledgement is normalized evidence for the exact synchronous publish
// future. MessageID is copied from the current Message because JetStream's
// PubAck does not echo it.
type Acknowledgement struct {
	Stream    string
	Sequence  uint64
	Duplicate bool
	MessageID string
}

func (Acknowledgement) String() string   { return "<redacted-outbox-publish-acknowledgement>" }
func (Acknowledgement) GoString() string { return "<redacted-outbox-publish-acknowledgement>" }
func (Acknowledgement) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-publish-acknowledgement]"`), nil
}

// FailureCode is a stable, secret-safe publish outcome category.
type FailureCode string

const (
	FailureInvalidInput          FailureCode = "invalid-input"
	FailureEventPermanent        FailureCode = "event-permanent"
	FailureTransportUnavailable  FailureCode = "transport-unavailable"
	FailurePublishOutcomeUnknown FailureCode = "publish-outcome-unknown"
)

type publishFailure struct{ code FailureCode }

func (failure *publishFailure) Error() string {
	return "transactional outbox publish: " + string(failure.category())
}

func (failure *publishFailure) String() string   { return failure.Error() }
func (failure *publishFailure) GoString() string { return failure.Error() }
func (failure *publishFailure) MarshalJSON() ([]byte, error) {
	return []byte(`"` + failure.Error() + `"`), nil
}

func (failure *publishFailure) category() FailureCode {
	if failure == nil {
		return ""
	}
	return failure.code
}

func publishError(code FailureCode) *publishFailure {
	return &publishFailure{code: code}
}

// FailureCodeOf returns the stable category for errors produced by this
// module. It never unwraps or exposes raw broker details.
func FailureCodeOf(err error) (FailureCode, bool) {
	var failure *publishFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	return failure.category(), true
}

func validFailureCode(code FailureCode) bool {
	switch code {
	case FailureInvalidInput,
		FailureEventPermanent,
		FailureTransportUnavailable,
		FailurePublishOutcomeUnknown:
		return true
	default:
		return false
	}
}

func validMapping(mapping Mapping) bool {
	return mapping.LogicalDestination == LogicalDestinationDomainEvents &&
		validStream(mapping.Stream) && validSubject(mapping.Subject)
}

func validMessage(message Message) bool {
	return len(message.Body) > 0 && validMessageID(message.MessageID)
}

func validMessageID(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validAcknowledgement(acknowledgement Acknowledgement, expectedStream, expectedMessageID string) bool {
	return acknowledgement.Stream == expectedStream &&
		acknowledgement.Sequence > 0 &&
		acknowledgement.MessageID == expectedMessageID &&
		validMessageID(acknowledgement.MessageID)
}

func validStream(value string) bool {
	return validTargetValue(value, maximumStreamBytes) &&
		!strings.ContainsAny(value, ".*/\\>")
}

func validSubject(value string) bool {
	if !validTargetValue(value, maximumSubjectBytes) || strings.ContainsAny(value, "*>") {
		return false
	}
	for _, token := range strings.Split(value, ".") {
		if token == "" {
			return false
		}
	}
	return true
}

func validTargetValue(value string, maximumBytes int) bool {
	return value != "" &&
		len(value) <= maximumBytes &&
		value == strings.TrimSpace(value) &&
		!strings.ContainsFunc(value, func(character rune) bool {
			return unicode.IsControl(character) || unicode.IsSpace(character)
		})
}
