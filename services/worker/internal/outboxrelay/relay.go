package outboxrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

const (
	maximumClaimOwnerBytes = 128
	maximumAckStreamBytes  = 255
)

// Outcome is the stable, secret-safe result of one relay attempt.
type Outcome uint8

const (
	OutcomeIdle Outcome = iota + 1
	OutcomeDelivered
	OutcomeAlreadyDelivered
	OutcomeRetryScheduled
	OutcomeParked
	OutcomeClaimLost
	OutcomeAcknowledgementUnconfirmed
	OutcomeFailureUnconfirmed
)

func (Outcome) String() string   { return "<redacted-outbox-relay-outcome>" }
func (Outcome) GoString() string { return "<redacted-outbox-relay-outcome>" }
func (Outcome) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-relay-outcome]")
}

// Result contains only an attempt outcome and, when retrying, the exact
// database-authored next-attempt time. It never contains Event or broker facts.
type Result struct {
	Outcome       Outcome
	NextAttemptAt time.Time
}

func (Result) String() string   { return "<redacted-outbox-relay-result>" }
func (Result) GoString() string { return "<redacted-outbox-relay-result>" }
func (Result) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-relay-result]")
}

// RelayErrorCode is a stable, secret-safe orchestration failure category.
type RelayErrorCode string

const (
	RelayErrorInvalidInput               RelayErrorCode = "invalid-input"
	RelayErrorInvariantViolation         RelayErrorCode = "invariant-violation"
	RelayErrorClaimUnavailable           RelayErrorCode = "claim-unavailable"
	RelayErrorAcknowledgementUnconfirmed RelayErrorCode = "acknowledgement-unconfirmed"
	RelayErrorFailureUnconfirmed         RelayErrorCode = "failure-unconfirmed"
)

func (RelayErrorCode) String() string   { return "<redacted-outbox-relay-error-code>" }
func (RelayErrorCode) GoString() string { return "<redacted-outbox-relay-error-code>" }
func (RelayErrorCode) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-relay-error-code]")
}

type relayFailure struct{ code RelayErrorCode }

func (failure *relayFailure) Error() string {
	return "transactional outbox relay: " + string(failure.category())
}

func (failure *relayFailure) String() string   { return failure.Error() }
func (failure *relayFailure) GoString() string { return failure.Error() }
func (failure *relayFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal(failure.Error())
}

func (failure *relayFailure) category() RelayErrorCode {
	if failure == nil {
		return ""
	}
	return failure.code
}

func relayError(code RelayErrorCode) error { return &relayFailure{code: code} }

// RelayErrorCodeOf extracts only errors produced by Relay. Context errors are
// intentionally left as standard context errors and return ok=false.
func RelayErrorCodeOf(err error) (RelayErrorCode, bool) {
	var failure *relayFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	return failure.category(), true
}

// Relay owns the complete policy for one transactional-outbox publish attempt.
// Its dependencies and claim owner cannot vary per call.
type Relay struct {
	store        outboxdb.Store
	encoder      Encoder
	publisher    outboxpublish.Publisher
	claimOwnerID string
}

func (*Relay) String() string   { return "<redacted-outbox-relay>" }
func (*Relay) GoString() string { return "<redacted-outbox-relay>" }
func (*Relay) MarshalJSON() ([]byte, error) {
	return json.Marshal("[redacted-outbox-relay]")
}

// NewRelay binds one Store, Encoder, Publisher, and trusted claim owner for
// the Relay's lifetime.
func NewRelay(
	store outboxdb.Store,
	encoder Encoder,
	publisher outboxpublish.Publisher,
	claimOwnerID string,
) (*Relay, error) {
	if nilInterface(store) || nilInterface(encoder) || nilInterface(publisher) ||
		!validClaimOwnerID(claimOwnerID) {
		return nil, relayError(RelayErrorInvalidInput)
	}
	return &Relay{
		store:        store,
		encoder:      encoder,
		publisher:    publisher,
		claimOwnerID: claimOwnerID,
	}, nil
}

// RunOne claims and attempts at most one Entry. It performs no detached work,
// retry, scheduling, lease renewal, or wall-clock decision.
func (relay *Relay) RunOne(ctx context.Context) (Result, error) {
	if ctx == nil || !relay.valid() {
		return Result{}, relayError(RelayErrorInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	claims, err := relay.store.Claim(ctx, outboxdb.ClaimRequest{
		ClaimOwnerID: relay.claimOwnerID,
		BatchSize:    1,
	})
	if err != nil {
		return Result{}, relay.claimError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	switch len(claims) {
	case 0:
		return Result{Outcome: OutcomeIdle}, nil
	case 1:
	default:
		return Result{}, relayError(RelayErrorInvariantViolation)
	}
	claim := claims[0]
	facts := claim.PublishFacts()
	if !facts.Valid() {
		clear(facts.Payload)
		return Result{}, relayError(RelayErrorInvariantViolation)
	}
	messageID := facts.BrokerMessageID
	encoderEnvelope, err := relay.encoder.Encode(ctx, facts)
	hasEnvelope := len(encoderEnvelope) != 0
	envelope := bytes.Clone(encoderEnvelope)
	clear(encoderEnvelope)
	clear(facts.Payload)
	if err != nil {
		clear(envelope)
		if hasEnvelope {
			return Result{}, relay.contradictoryDependencyError(ctx, err)
		}
		return relay.handleEncoderFailure(ctx, claim, err)
	}
	if err := ctx.Err(); err != nil {
		clear(envelope)
		return Result{}, err
	}
	if !hasEnvelope {
		return Result{}, relayError(RelayErrorInvariantViolation)
	}

	acknowledgement, err := relay.publisher.Publish(ctx, outboxpublish.Message{
		Body:      envelope,
		MessageID: messageID,
	})
	clear(envelope)
	if err != nil {
		if acknowledgement != (outboxpublish.Acknowledgement{}) {
			return Result{}, relay.contradictoryDependencyError(ctx, err)
		}
		return relay.handlePublisherFailure(ctx, claim, err)
	}
	if !validPublisherAcknowledgement(acknowledgement, messageID) {
		if contextErr := ctx.Err(); contextErr != nil {
			return Result{}, contextErr
		}
		return Result{}, relayError(RelayErrorInvariantViolation)
	}

	return relay.acknowledge(ctx, claim, acknowledgement)
}

func (relay *Relay) contradictoryDependencyError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if contextErr := standardContextError(err); contextErr != nil {
		return contextErr
	}
	return relayError(RelayErrorInvariantViolation)
}

func (relay *Relay) valid() bool {
	return relay != nil && !nilInterface(relay.store) && !nilInterface(relay.encoder) &&
		!nilInterface(relay.publisher) && validClaimOwnerID(relay.claimOwnerID)
}

func (relay *Relay) claimError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if contextErr := standardContextError(err); contextErr != nil {
		return contextErr
	}
	code, ok := outboxdb.StoreErrorCodeOf(err)
	if !ok {
		return relayError(RelayErrorInvariantViolation)
	}
	switch code {
	case outboxdb.StoreErrorPersistence:
		return relayError(RelayErrorClaimUnavailable)
	case outboxdb.StoreErrorInvalidInput, outboxdb.StoreErrorClaimDenied:
		return relayError(RelayErrorInvariantViolation)
	default:
		return relayError(RelayErrorInvariantViolation)
	}
}

func (relay *Relay) handleEncoderFailure(
	ctx context.Context,
	claim outboxdb.Claim,
	err error,
) (Result, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return Result{}, contextErr
	}
	if contextErr := standardContextError(err); contextErr != nil {
		return Result{}, contextErr
	}
	code, ok := EncoderFailureCodeOf(err)
	if !ok {
		return Result{}, relayError(RelayErrorInvariantViolation)
	}
	switch code {
	case EncoderFailureEventRetryable:
		return relay.recordFailure(ctx, claim, outboxdb.FailureEventRetryable)
	case EncoderFailureEventPermanent:
		return relay.recordFailure(ctx, claim, outboxdb.FailureEventPermanent)
	case EncoderFailureInvalidInput:
		return Result{}, relayError(RelayErrorInvariantViolation)
	default:
		return Result{}, relayError(RelayErrorInvariantViolation)
	}
}

func (relay *Relay) handlePublisherFailure(
	ctx context.Context,
	claim outboxdb.Claim,
	err error,
) (Result, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return Result{}, contextErr
	}
	if contextErr := standardContextError(err); contextErr != nil {
		return Result{}, contextErr
	}
	code, ok := outboxpublish.FailureCodeOf(err)
	if !ok {
		return Result{}, relayError(RelayErrorInvariantViolation)
	}
	switch code {
	case outboxpublish.FailureTransportUnavailable:
		return relay.recordFailure(ctx, claim, outboxdb.FailureTransportUnavailable)
	case outboxpublish.FailurePublishOutcomeUnknown:
		return relay.recordFailure(ctx, claim, outboxdb.FailurePublishOutcomeUnknown)
	case outboxpublish.FailureEventPermanent:
		return relay.recordFailure(ctx, claim, outboxdb.FailureEventPermanent)
	case outboxpublish.FailureInvalidInput:
		return Result{}, relayError(RelayErrorInvariantViolation)
	default:
		return Result{}, relayError(RelayErrorInvariantViolation)
	}
}

func (relay *Relay) acknowledge(
	ctx context.Context,
	claim outboxdb.Claim,
	acknowledgement outboxpublish.Acknowledgement,
) (Result, error) {
	result, err := relay.store.Acknowledge(ctx, claim, acknowledgement)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return Result{Outcome: OutcomeAcknowledgementUnconfirmed}, contextErr
		}
		if contextErr := standardContextError(err); contextErr != nil {
			return Result{Outcome: OutcomeAcknowledgementUnconfirmed}, contextErr
		}
		code, ok := outboxdb.StoreErrorCodeOf(err)
		if !ok {
			return Result{Outcome: OutcomeAcknowledgementUnconfirmed}, relayError(RelayErrorInvariantViolation)
		}
		switch code {
		case outboxdb.StoreErrorClaimDenied:
			return Result{Outcome: OutcomeClaimLost}, nil
		case outboxdb.StoreErrorPersistence:
			return Result{Outcome: OutcomeAcknowledgementUnconfirmed}, relayError(RelayErrorAcknowledgementUnconfirmed)
		case outboxdb.StoreErrorInvalidInput:
			return Result{Outcome: OutcomeAcknowledgementUnconfirmed}, relayError(RelayErrorInvariantViolation)
		default:
			return Result{Outcome: OutcomeAcknowledgementUnconfirmed}, relayError(RelayErrorInvariantViolation)
		}
	}
	switch result {
	case outboxdb.AcknowledgementDelivered:
		return Result{Outcome: OutcomeDelivered}, nil
	case outboxdb.AcknowledgementAlreadyDelivered:
		return Result{Outcome: OutcomeAlreadyDelivered}, nil
	default:
		return Result{Outcome: OutcomeAcknowledgementUnconfirmed}, relayError(RelayErrorInvariantViolation)
	}
}

func (relay *Relay) recordFailure(
	ctx context.Context,
	claim outboxdb.Claim,
	code outboxdb.FailureCode,
) (Result, error) {
	result, err := relay.store.RecordFailure(ctx, claim, code)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return Result{Outcome: OutcomeFailureUnconfirmed}, contextErr
		}
		if contextErr := standardContextError(err); contextErr != nil {
			return Result{Outcome: OutcomeFailureUnconfirmed}, contextErr
		}
		storeCode, ok := outboxdb.StoreErrorCodeOf(err)
		if !ok {
			return Result{Outcome: OutcomeFailureUnconfirmed}, relayError(RelayErrorInvariantViolation)
		}
		switch storeCode {
		case outboxdb.StoreErrorClaimDenied:
			return Result{Outcome: OutcomeClaimLost}, nil
		case outboxdb.StoreErrorPersistence:
			return Result{Outcome: OutcomeFailureUnconfirmed}, relayError(RelayErrorFailureUnconfirmed)
		case outboxdb.StoreErrorInvalidInput:
			return Result{Outcome: OutcomeFailureUnconfirmed}, relayError(RelayErrorInvariantViolation)
		default:
			return Result{Outcome: OutcomeFailureUnconfirmed}, relayError(RelayErrorInvariantViolation)
		}
	}
	switch result.Disposition {
	case outboxdb.FailureRetryScheduled:
		if result.NextAttemptAt.IsZero() {
			return Result{Outcome: OutcomeFailureUnconfirmed}, relayError(RelayErrorInvariantViolation)
		}
		return Result{
			Outcome:       OutcomeRetryScheduled,
			NextAttemptAt: result.NextAttemptAt,
		}, nil
	case outboxdb.FailureParked:
		if !result.NextAttemptAt.IsZero() {
			return Result{Outcome: OutcomeFailureUnconfirmed}, relayError(RelayErrorInvariantViolation)
		}
		return Result{Outcome: OutcomeParked}, nil
	default:
		return Result{Outcome: OutcomeFailureUnconfirmed}, relayError(RelayErrorInvariantViolation)
	}
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func validClaimOwnerID(value string) bool {
	return value != "" && len(value) <= maximumClaimOwnerBytes &&
		value == strings.TrimSpace(value) &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func validPublisherAcknowledgement(
	acknowledgement outboxpublish.Acknowledgement,
	expectedMessageID string,
) bool {
	return acknowledgement.Sequence > 0 && acknowledgement.MessageID == expectedMessageID &&
		acknowledgement.Stream != "" && len(acknowledgement.Stream) <= maximumAckStreamBytes &&
		acknowledgement.Stream == strings.TrimSpace(acknowledgement.Stream) &&
		!strings.ContainsFunc(acknowledgement.Stream, func(character rune) bool {
			return unicode.IsControl(character) || unicode.IsSpace(character)
		}) &&
		!strings.ContainsAny(acknowledgement.Stream, ".*/\\>")
}

func standardContextError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return nil
	}
}
