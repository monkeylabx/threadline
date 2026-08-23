package outboxrelay

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

const relayTestOwner = "worker-relay-test"

var (
	relayTestBinding = outboxdb.Binding{
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		Stream:             "DOMAIN_EVENTS",
	}
	relayTestMapping = outboxpublish.Mapping{
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		Stream:             relayTestBinding.Stream,
		Subject:            "threadline.domain.events.v1",
	}
)

func TestRelayIdleAndOverclaimAreTerminalBeforeEncoding(t *testing.T) {
	t.Parallel()

	t.Run("idle", func(t *testing.T) {
		t.Parallel()
		store := mustRelayStore(t, outboxdb.ScriptedStorePlan{
			Claims: []outboxdb.ScriptedClaimStep{{}},
		})
		encoder := mustRelayEncoder(t)
		publisher := mustRelayPublisher(t)
		relay := mustRelay(t, store, encoder, publisher)

		result, err := relay.RunOne(context.Background())
		if err != nil || result.Outcome != OutcomeIdle || !result.NextAttemptAt.IsZero() {
			t.Fatalf("RunOne = %#v/%v, want Idle", result, err)
		}
		calls := store.Calls()
		if len(calls.Claims) != 1 || calls.Claims[0] != (outboxdb.ClaimRequest{
			ClaimOwnerID: relayTestOwner,
			BatchSize:    1,
		}) {
			t.Fatalf("Claim calls = %#v, want exact bound owner and batch one", calls.Claims)
		}
		assertNoRelayAttemptCalls(t, store, encoder, publisher)
	})

	t.Run("overclaim", func(t *testing.T) {
		t.Parallel()
		claimSource := mustRelayStore(t, outboxdb.ScriptedStorePlan{
			Claims: []outboxdb.ScriptedClaimStep{{Claims: []outboxdb.ScriptedClaim{
				relayScriptedClaim([]byte("one")),
				relayScriptedClaim([]byte("two")),
			}}},
		})
		claims, err := claimSource.Claim(context.Background(), outboxdb.ClaimRequest{
			ClaimOwnerID: relayTestOwner,
			BatchSize:    2,
		})
		if err != nil || len(claims) != 2 {
			t.Fatalf("fixture claims = %d/%v, want two", len(claims), err)
		}
		store := &claimResultStore{claims: claims}
		encoder := mustRelayEncoder(t)
		publisher := mustRelayPublisher(t)
		relay := mustRelay(t, store, encoder, publisher)

		result, err := relay.RunOne(context.Background())
		if result != (Result{}) || relayErrorCode(err) != RelayErrorInvariantViolation {
			t.Fatalf("RunOne = %#v/%v, want zero/invariant-violation", result, err)
		}
		if store.claimRequest != (outboxdb.ClaimRequest{ClaimOwnerID: relayTestOwner, BatchSize: 1}) {
			t.Fatalf("Claim request = %#v, want bound owner/batch one", store.claimRequest)
		}
		if len(encoder.Calls()) != 0 || len(publisher.Calls()) != 0 || store.mutationCalls != 0 {
			t.Fatal("overclaim reached Encode, Publish, or a Store mutation")
		}
	})
}

func TestRelayPreservesEncodedMessageAndExactAcknowledgement(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		sequence    uint64
		duplicate   bool
		stored      outboxdb.Acknowledgement
		wantOutcome Outcome
	}{
		{
			name:        "delivered sequence one",
			sequence:    1,
			stored:      outboxdb.AcknowledgementDelivered,
			wantOutcome: OutcomeDelivered,
		},
		{
			name:        "already delivered duplicate max sequence",
			sequence:    math.MaxUint64,
			duplicate:   true,
			stored:      outboxdb.AcknowledgementAlreadyDelivered,
			wantOutcome: OutcomeAlreadyDelivered,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			payload := []byte("stored-opaque-payload")
			envelope := []byte("complete-encoded-envelope")
			facts := validEncoderTestFacts(payload)
			expectedFacts := facts.Clone()
			ack := outboxpublish.Acknowledgement{
				Stream:    relayTestBinding.Stream,
				Sequence:  test.sequence,
				Duplicate: test.duplicate,
				MessageID: facts.BrokerMessageID,
			}
			store := mustRelayStore(t, outboxdb.ScriptedStorePlan{
				Claims: []outboxdb.ScriptedClaimStep{{Claims: []outboxdb.ScriptedClaim{{
					PublishFacts: facts,
					Lease:        relayTestLease(),
				}}}},
				Acknowledgements: []outboxdb.ScriptedAcknowledgementStep{{
					Acknowledgement: test.stored,
				}},
			})
			encoder := mustRelayEncoder(t, ScriptedEncodingStep{Envelope: envelope})
			publisher := mustRelayPublisher(t, outboxpublish.ScriptedStep{Acknowledgement: ack})
			payload[0] = 'X'
			envelope[0] = 'X'

			result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
			if err != nil || result.Outcome != test.wantOutcome || !result.NextAttemptAt.IsZero() {
				t.Fatalf("RunOne = %#v/%v, want %v", result, err, test.wantOutcome)
			}
			encoderCalls := encoder.Calls()
			publishCalls := publisher.Calls()
			storeCalls := store.Calls()
			if len(encoderCalls) != 1 || len(publishCalls) != 1 ||
				len(storeCalls.Acknowledgements) != 1 || len(storeCalls.Failures) != 0 {
				t.Fatalf("call counts Encode/Publish/Ack/Failure = %d/%d/%d/%d, want 1/1/1/0",
					len(encoderCalls), len(publishCalls), len(storeCalls.Acknowledgements), len(storeCalls.Failures))
			}
			if !reflect.DeepEqual(encoderCalls[0], expectedFacts) ||
				string(publishCalls[0].Body) != "complete-encoded-envelope" ||
				publishCalls[0].MessageID != facts.BrokerMessageID ||
				storeCalls.Acknowledgements[0].Acknowledgement != ack {
				t.Fatalf("relay changed facts, encoded message, ID, or Ack")
			}

			encoderCalls[0].Payload[0] = 'Y'
			publishCalls[0].Body[0] = 'Y'
			storeCalls.Acknowledgements[0].PublishFacts.Payload[0] = 'Y'
			if string(encoder.Calls()[0].Payload) != "stored-opaque-payload" ||
				string(publisher.Calls()[0].Body) != "complete-encoded-envelope" ||
				string(store.Calls().Acknowledgements[0].PublishFacts.Payload) != "stored-opaque-payload" {
				t.Fatal("relay or fake snapshots shared byte ownership")
			}
		})
	}
}

func TestRelayMapsEncoderAndPublisherFailuresToOneDatabaseMutation(t *testing.T) {
	t.Parallel()

	nextAttemptAt := time.Date(2026, 8, 24, 2, 3, 4, 0, time.UTC)
	tests := []struct {
		name             string
		encoderFailure   EncoderFailureCode
		publisherFailure outboxpublish.FailureCode
		wantFailure      outboxdb.FailureCode
		storeResult      outboxdb.FailureResult
		wantOutcome      Outcome
		wantNext         time.Time
		wantPublishCalls int
	}{
		{
			name:           "encoder retryable schedules retry",
			encoderFailure: EncoderFailureEventRetryable,
			wantFailure:    outboxdb.FailureEventRetryable,
			storeResult: outboxdb.FailureResult{
				Disposition:   outboxdb.FailureRetryScheduled,
				NextAttemptAt: nextAttemptAt,
			},
			wantOutcome: OutcomeRetryScheduled,
			wantNext:    nextAttemptAt,
		},
		{
			name:           "encoder permanent parks",
			encoderFailure: EncoderFailureEventPermanent,
			wantFailure:    outboxdb.FailureEventPermanent,
			storeResult:    outboxdb.FailureResult{Disposition: outboxdb.FailureParked},
			wantOutcome:    OutcomeParked,
		},
		{
			name:             "publisher transport unavailable schedules retry",
			publisherFailure: outboxpublish.FailureTransportUnavailable,
			wantFailure:      outboxdb.FailureTransportUnavailable,
			storeResult: outboxdb.FailureResult{
				Disposition:   outboxdb.FailureRetryScheduled,
				NextAttemptAt: nextAttemptAt,
			},
			wantOutcome:      OutcomeRetryScheduled,
			wantNext:         nextAttemptAt,
			wantPublishCalls: 1,
		},
		{
			name:             "publisher outcome unknown schedules retry",
			publisherFailure: outboxpublish.FailurePublishOutcomeUnknown,
			wantFailure:      outboxdb.FailurePublishOutcomeUnknown,
			storeResult: outboxdb.FailureResult{
				Disposition:   outboxdb.FailureRetryScheduled,
				NextAttemptAt: nextAttemptAt,
			},
			wantOutcome:      OutcomeRetryScheduled,
			wantNext:         nextAttemptAt,
			wantPublishCalls: 1,
		},
		{
			name:             "publisher permanent parks",
			publisherFailure: outboxpublish.FailureEventPermanent,
			wantFailure:      outboxdb.FailureEventPermanent,
			storeResult:      outboxdb.FailureResult{Disposition: outboxdb.FailureParked},
			wantOutcome:      OutcomeParked,
			wantPublishCalls: 1,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			facts := validEncoderTestFacts([]byte("stored-payload"))
			store := mustRelayStore(t, outboxdb.ScriptedStorePlan{
				Claims: []outboxdb.ScriptedClaimStep{{Claims: []outboxdb.ScriptedClaim{{
					PublishFacts: facts,
					Lease:        relayTestLease(),
				}}}},
				Failures: []outboxdb.ScriptedFailureStep{{Result: test.storeResult}},
			})
			encoderStep := ScriptedEncodingStep{Envelope: []byte("encoded-envelope")}
			if test.encoderFailure != "" {
				encoderStep = ScriptedEncodingStep{Failure: test.encoderFailure}
			}
			encoder := mustRelayEncoder(t, encoderStep)
			var publisherSteps []outboxpublish.ScriptedStep
			if test.publisherFailure != "" {
				publisherSteps = []outboxpublish.ScriptedStep{{Failure: test.publisherFailure}}
			}
			publisher := mustRelayPublisher(t, publisherSteps...)

			result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
			if err != nil || result.Outcome != test.wantOutcome || !result.NextAttemptAt.Equal(test.wantNext) {
				t.Fatalf("RunOne = %#v/%v, want %v/%v", result, err, test.wantOutcome, test.wantNext)
			}
			calls := store.Calls()
			if len(encoder.Calls()) != 1 || len(publisher.Calls()) != test.wantPublishCalls ||
				len(calls.Failures) != 1 || len(calls.Acknowledgements) != 0 ||
				calls.Failures[0].Failure != test.wantFailure {
				t.Fatalf("calls Encode/Publish/Failure/Ack = %d/%d/%#v/%d, want exact single %q failure",
					len(encoder.Calls()), len(publisher.Calls()), calls.Failures, len(calls.Acknowledgements), test.wantFailure)
			}
		})
	}
}

func TestRelayMapsClaimLossAndUnconfirmedTerminalMutationsWithoutRetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		acknowledge      bool
		storeFailure     outboxdb.StoreErrorCode
		wantOutcome      Outcome
		wantRelayError   RelayErrorCode
		wantPublishCalls int
	}{
		{
			name:             "ack claim lost",
			acknowledge:      true,
			storeFailure:     outboxdb.StoreErrorClaimDenied,
			wantOutcome:      OutcomeClaimLost,
			wantPublishCalls: 1,
		},
		{
			name:             "ack persistence unconfirmed",
			acknowledge:      true,
			storeFailure:     outboxdb.StoreErrorPersistence,
			wantOutcome:      OutcomeAcknowledgementUnconfirmed,
			wantRelayError:   RelayErrorAcknowledgementUnconfirmed,
			wantPublishCalls: 1,
		},
		{
			name:         "failure claim lost",
			storeFailure: outboxdb.StoreErrorClaimDenied,
			wantOutcome:  OutcomeClaimLost,
		},
		{
			name:           "failure persistence unconfirmed",
			storeFailure:   outboxdb.StoreErrorPersistence,
			wantOutcome:    OutcomeFailureUnconfirmed,
			wantRelayError: RelayErrorFailureUnconfirmed,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			facts := validEncoderTestFacts([]byte("stored-payload"))
			plan := outboxdb.ScriptedStorePlan{
				Claims: []outboxdb.ScriptedClaimStep{{Claims: []outboxdb.ScriptedClaim{{
					PublishFacts: facts,
					Lease:        relayTestLease(),
				}}}},
			}
			publisherSteps := []outboxpublish.ScriptedStep(nil)
			if test.acknowledge {
				plan.Acknowledgements = []outboxdb.ScriptedAcknowledgementStep{{Failure: test.storeFailure}}
				publisherSteps = []outboxpublish.ScriptedStep{{Acknowledgement: outboxpublish.Acknowledgement{
					Stream:    relayTestBinding.Stream,
					Sequence:  1,
					MessageID: facts.BrokerMessageID,
				}}}
			} else {
				plan.Failures = []outboxdb.ScriptedFailureStep{{Failure: test.storeFailure}}
			}
			store := mustRelayStore(t, plan)
			encoderStep := ScriptedEncodingStep{Failure: EncoderFailureEventRetryable}
			if test.acknowledge {
				encoderStep = ScriptedEncodingStep{Envelope: []byte("encoded-envelope")}
			}
			encoder := mustRelayEncoder(t, encoderStep)
			publisher := mustRelayPublisher(t, publisherSteps...)

			result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
			if result.Outcome != test.wantOutcome || !result.NextAttemptAt.IsZero() ||
				relayErrorCode(err) != test.wantRelayError {
				t.Fatalf("RunOne = %#v/%v, want %v/%q", result, err, test.wantOutcome, test.wantRelayError)
			}
			calls := store.Calls()
			if len(publisher.Calls()) != test.wantPublishCalls ||
				len(calls.Acknowledgements) != boolCount(test.acknowledge) ||
				len(calls.Failures) != boolCount(!test.acknowledge) {
				t.Fatalf("calls Publish/Ack/Failure = %d/%d/%d, want %d/%d/%d",
					len(publisher.Calls()), len(calls.Acknowledgements), len(calls.Failures),
					test.wantPublishCalls, boolCount(test.acknowledge), boolCount(!test.acknowledge))
			}
		})
	}
}

func TestRelayCancellationPrecedesFailureMutationButNotVerifiedAck(t *testing.T) {
	t.Parallel()

	t.Run("encoder failure after cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		store := relayClaimStore(t)
		baseEncoder := mustRelayEncoder(t, ScriptedEncodingStep{Failure: EncoderFailureEventRetryable})
		encoder := cancelingEncoder{Encoder: baseEncoder, cancel: cancel}
		publisher := mustRelayPublisher(t)

		result, err := mustRelay(t, store, encoder, publisher).RunOne(ctx)
		if result != (Result{}) || !errors.Is(err, context.Canceled) {
			t.Fatalf("RunOne = %#v/%v, want exact cancellation", result, err)
		}
		if len(store.Calls().Failures) != 0 || len(publisher.Calls()) != 0 {
			t.Fatal("canceled encoder result reached Publish or RecordFailure")
		}
	})

	t.Run("publisher failure after cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		store := relayClaimStore(t)
		encoder := mustRelayEncoder(t, ScriptedEncodingStep{Envelope: []byte("encoded-envelope")})
		basePublisher := mustRelayPublisher(t, outboxpublish.ScriptedStep{Failure: outboxpublish.FailureTransportUnavailable})
		publisher := cancelingPublisher{Publisher: basePublisher, cancel: cancel}

		result, err := mustRelay(t, store, encoder, publisher).RunOne(ctx)
		if result != (Result{}) || !errors.Is(err, context.Canceled) {
			t.Fatalf("RunOne = %#v/%v, want exact cancellation", result, err)
		}
		if len(store.Calls().Failures) != 0 || len(basePublisher.Calls()) != 1 {
			t.Fatal("canceled publisher result did not stop before RecordFailure")
		}
	})

	t.Run("verified ack is attempted once with canceled context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		baseStore := relayClaimStore(t)
		store := &terminalStore{Store: baseStore}
		store.acknowledge = func(got context.Context, _ outboxdb.Claim, _ outboxpublish.Acknowledgement) (outboxdb.Acknowledgement, error) {
			if got != ctx {
				t.Fatal("Acknowledge received a different context")
			}
			return 0, got.Err()
		}
		encoder := mustRelayEncoder(t, ScriptedEncodingStep{Envelope: []byte("encoded-envelope")})
		basePublisher := mustRelayPublisher(t, outboxpublish.ScriptedStep{Acknowledgement: relayTestAck(1, false)})
		publisher := cancelingPublisher{Publisher: basePublisher, cancel: cancel}

		result, err := mustRelay(t, store, encoder, publisher).RunOne(ctx)
		if result.Outcome != OutcomeAcknowledgementUnconfirmed || !errors.Is(err, context.Canceled) {
			t.Fatalf("RunOne = %#v/%v, want AckUnconfirmed/context.Canceled", result, err)
		}
		if store.acknowledgementCalls != 1 || store.failureCalls != 0 || len(basePublisher.Calls()) != 1 {
			t.Fatalf("calls Publish/Ack/Failure = %d/%d/%d, want 1/1/0",
				len(basePublisher.Calls()), store.acknowledgementCalls, store.failureCalls)
		}
	})
}

func TestRelayRunOneIsRaceSafe(t *testing.T) {
	t.Parallel()

	const count = 24
	claimSteps := make([]outboxdb.ScriptedClaimStep, count)
	encoderSteps := make([]ScriptedEncodingStep, count)
	publisherSteps := make([]outboxpublish.ScriptedStep, count)
	ackSteps := make([]outboxdb.ScriptedAcknowledgementStep, count)
	for index := 0; index < count; index++ {
		claimSteps[index] = outboxdb.ScriptedClaimStep{Claims: []outboxdb.ScriptedClaim{
			relayScriptedClaim([]byte("payload")),
		}}
		encoderSteps[index] = ScriptedEncodingStep{Envelope: []byte("encoded-envelope")}
		publisherSteps[index] = outboxpublish.ScriptedStep{Acknowledgement: relayTestAck(1, false)}
		ackSteps[index] = outboxdb.ScriptedAcknowledgementStep{Acknowledgement: outboxdb.AcknowledgementDelivered}
	}
	store := mustRelayStore(t, outboxdb.ScriptedStorePlan{
		Claims:           claimSteps,
		Acknowledgements: ackSteps,
	})
	encoder := mustRelayEncoder(t, encoderSteps...)
	publisher := mustRelayPublisher(t, publisherSteps...)
	relay := mustRelay(t, store, encoder, publisher)

	var wait sync.WaitGroup
	errorsFound := make(chan error, count)
	for index := 0; index < count; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := relay.RunOne(context.Background())
			if err != nil {
				errorsFound <- err
				return
			}
			if result.Outcome != OutcomeDelivered {
				errorsFound <- errors.New("unexpected concurrent outcome")
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
	if calls := store.Calls(); len(calls.Claims) != count || len(calls.Acknowledgements) != count || len(calls.Failures) != 0 {
		t.Fatalf("Store calls Claim/Ack/Failure = %d/%d/%d, want %d/%d/0",
			len(calls.Claims), len(calls.Acknowledgements), len(calls.Failures), count, count)
	}
	if len(encoder.Calls()) != count || len(publisher.Calls()) != count {
		t.Fatalf("Encode/Publish calls = %d/%d, want %d/%d", len(encoder.Calls()), len(publisher.Calls()), count, count)
	}
}

func mustRelay(
	t *testing.T,
	store outboxdb.Store,
	encoder Encoder,
	publisher outboxpublish.Publisher,
) *Relay {
	t.Helper()
	relay, err := NewRelay(store, encoder, publisher, relayTestOwner)
	if err != nil {
		t.Fatal(err)
	}
	return relay
}

func mustRelayStore(t *testing.T, plan outboxdb.ScriptedStorePlan) *outboxdb.ScriptedStore {
	t.Helper()
	store, err := outboxdb.NewScriptedStore(relayTestBinding, plan)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func mustRelayEncoder(t *testing.T, steps ...ScriptedEncodingStep) *ScriptedEncoder {
	t.Helper()
	encoder, err := NewScriptedEncoder(steps...)
	if err != nil {
		t.Fatal(err)
	}
	return encoder
}

func mustRelayPublisher(t *testing.T, steps ...outboxpublish.ScriptedStep) *outboxpublish.ScriptedPublisher {
	t.Helper()
	publisher, err := outboxpublish.NewScriptedPublisher(relayTestMapping, steps...)
	if err != nil {
		t.Fatal(err)
	}
	return publisher
}

func relayScriptedClaim(payload []byte) outboxdb.ScriptedClaim {
	return outboxdb.ScriptedClaim{
		PublishFacts: validEncoderTestFacts(payload),
		Lease:        relayTestLease(),
	}
}

func relayClaimStore(t *testing.T) *outboxdb.ScriptedStore {
	t.Helper()
	return mustRelayStore(t, outboxdb.ScriptedStorePlan{
		Claims: []outboxdb.ScriptedClaimStep{{Claims: []outboxdb.ScriptedClaim{
			relayScriptedClaim([]byte("stored-payload")),
		}}},
	})
}

func relayTestAck(sequence uint64, duplicate bool) outboxpublish.Acknowledgement {
	return outboxpublish.Acknowledgement{
		Stream:    relayTestBinding.Stream,
		Sequence:  sequence,
		Duplicate: duplicate,
		MessageID: encoderTestMessageID,
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func relayTestLease() outboxdb.Lease {
	claimedAt := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	return outboxdb.Lease{
		ClaimedAt:              claimedAt,
		ExpiresAt:              claimedAt.Add(30 * time.Second),
		AbsoluteLeaseExpiresAt: claimedAt.Add(5 * time.Minute),
	}
}

func relayErrorCode(err error) RelayErrorCode {
	code, _ := RelayErrorCodeOf(err)
	return code
}

func assertNoRelayAttemptCalls(
	t *testing.T,
	store *outboxdb.ScriptedStore,
	encoder *ScriptedEncoder,
	publisher *outboxpublish.ScriptedPublisher,
) {
	t.Helper()
	calls := store.Calls()
	if len(encoder.Calls()) != 0 || len(publisher.Calls()) != 0 ||
		len(calls.Renewals) != 0 || len(calls.Acknowledgements) != 0 || len(calls.Failures) != 0 {
		t.Fatal("idle relay performed attempt work")
	}
}

type claimResultStore struct {
	claims        []outboxdb.Claim
	claimRequest  outboxdb.ClaimRequest
	mutationCalls int
}

func (store *claimResultStore) Claim(_ context.Context, request outboxdb.ClaimRequest) ([]outboxdb.Claim, error) {
	store.claimRequest = request
	return append([]outboxdb.Claim(nil), store.claims...), nil
}

func (store *claimResultStore) Renew(context.Context, outboxdb.Claim) (outboxdb.Renewal, error) {
	store.mutationCalls++
	return outboxdb.Renewal{}, errors.New("unexpected Renew")
}

func (store *claimResultStore) Acknowledge(context.Context, outboxdb.Claim, outboxpublish.Acknowledgement) (outboxdb.Acknowledgement, error) {
	store.mutationCalls++
	return 0, errors.New("unexpected Acknowledge")
}

func (store *claimResultStore) RecordFailure(context.Context, outboxdb.Claim, outboxdb.FailureCode) (outboxdb.FailureResult, error) {
	store.mutationCalls++
	return outboxdb.FailureResult{}, errors.New("unexpected RecordFailure")
}

type cancelingEncoder struct {
	Encoder
	cancel context.CancelFunc
}

func (encoder cancelingEncoder) Encode(ctx context.Context, facts outboxdb.PublishFacts) ([]byte, error) {
	envelope, err := encoder.Encoder.Encode(ctx, facts)
	encoder.cancel()
	return envelope, err
}

type cancelingPublisher struct {
	outboxpublish.Publisher
	cancel context.CancelFunc
}

func (publisher cancelingPublisher) Publish(ctx context.Context, message outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
	acknowledgement, err := publisher.Publisher.Publish(ctx, message)
	publisher.cancel()
	return acknowledgement, err
}

type terminalStore struct {
	outboxdb.Store
	acknowledge          func(context.Context, outboxdb.Claim, outboxpublish.Acknowledgement) (outboxdb.Acknowledgement, error)
	recordFailure        func(context.Context, outboxdb.Claim, outboxdb.FailureCode) (outboxdb.FailureResult, error)
	acknowledgementCalls int
	failureCalls         int
}

func (store *terminalStore) Acknowledge(
	ctx context.Context,
	claim outboxdb.Claim,
	acknowledgement outboxpublish.Acknowledgement,
) (outboxdb.Acknowledgement, error) {
	store.acknowledgementCalls++
	if store.acknowledge != nil {
		return store.acknowledge(ctx, claim, acknowledgement)
	}
	return store.Store.Acknowledge(ctx, claim, acknowledgement)
}

func (store *terminalStore) RecordFailure(
	ctx context.Context,
	claim outboxdb.Claim,
	code outboxdb.FailureCode,
) (outboxdb.FailureResult, error) {
	store.failureCalls++
	if store.recordFailure != nil {
		return store.recordFailure(ctx, claim, code)
	}
	return store.Store.RecordFailure(ctx, claim, code)
}
