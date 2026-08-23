package outboxrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

func TestRelayConstructorRejectsTypedNilDependenciesAndUnsafeOwner(t *testing.T) {
	t.Parallel()

	store := mustRelayStore(t, outboxdb.ScriptedStorePlan{})
	encoder := mustRelayEncoder(t)
	publisher := mustRelayPublisher(t)
	var nilStore *relaySecurityStore
	var nilEncoder *relaySecurityEncoder
	var nilPublisher *relaySecurityPublisher

	for _, test := range []struct {
		name      string
		store     outboxdb.Store
		encoder   Encoder
		publisher outboxpublish.Publisher
		owner     string
	}{
		{name: "nil Store", encoder: encoder, publisher: publisher, owner: relayTestOwner},
		{name: "typed-nil Store", store: nilStore, encoder: encoder, publisher: publisher, owner: relayTestOwner},
		{name: "nil Encoder", store: store, publisher: publisher, owner: relayTestOwner},
		{name: "typed-nil Encoder", store: store, encoder: nilEncoder, publisher: publisher, owner: relayTestOwner},
		{name: "nil Publisher", store: store, encoder: encoder, owner: relayTestOwner},
		{name: "typed-nil Publisher", store: store, encoder: encoder, publisher: nilPublisher, owner: relayTestOwner},
		{name: "empty owner", store: store, encoder: encoder, publisher: publisher},
		{name: "untrimmed owner", store: store, encoder: encoder, publisher: publisher, owner: " " + relayTestOwner},
		{name: "control owner", store: store, encoder: encoder, publisher: publisher, owner: "worker\nowner"},
		{name: "oversized owner", store: store, encoder: encoder, publisher: publisher, owner: strings.Repeat("a", 129)},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewRelay(test.store, test.encoder, test.publisher, test.owner)
			if got != nil || relayErrorCode(err) != RelayErrorInvalidInput {
				t.Fatalf("NewRelay = %#v/%v, want nil/invalid-input", got, err)
			}
		})
	}

	if len(store.Calls().Claims) != 0 || len(encoder.Calls()) != 0 || len(publisher.Calls()) != 0 {
		t.Fatal("constructor validation called a dependency")
	}
}

func TestRelayRejectsNilContextAndNilReceiverWithoutCallingDependencies(t *testing.T) {
	t.Parallel()

	var nilRelay *Relay
	if result, err := nilRelay.RunOne(context.Background()); result != (Result{}) || relayErrorCode(err) != RelayErrorInvalidInput {
		t.Fatalf("nil Relay RunOne = %#v/%v, want zero/invalid-input", result, err)
	}

	store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
	encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope")}
	publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}
	result, err := mustRelay(t, store, encoder, publisher).RunOne(nil)
	if result != (Result{}) || relayErrorCode(err) != RelayErrorInvalidInput {
		t.Fatalf("nil-context RunOne = %#v/%v, want zero/invalid-input", result, err)
	}
	if calls := store.snapshot(); calls != (relaySecurityStoreSnapshot{}) ||
		encoder.callCount() != 0 || publisher.callCount() != 0 {
		t.Fatal("nil-context RunOne called a dependency")
	}
}

func TestRelayClaimFailuresAreNormalizedBeforeAttemptWork(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		err      error
		wantCode RelayErrorCode
	}{
		{name: "persistence", err: relaySecurityStoreError(t, outboxdb.StoreErrorPersistence), wantCode: RelayErrorClaimUnavailable},
		{name: "invalid input", err: relaySecurityStoreError(t, outboxdb.StoreErrorInvalidInput), wantCode: RelayErrorInvariantViolation},
		{name: "claim denied", err: relaySecurityStoreError(t, outboxdb.StoreErrorClaimDenied), wantCode: RelayErrorInvariantViolation},
		{name: "raw", err: errors.New("raw claim credential secret"), wantCode: RelayErrorInvariantViolation},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := &relaySecurityStore{claimError: test.err}
			encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope")}
			publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}
			result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
			if result != (Result{}) || relayErrorCode(err) != test.wantCode {
				t.Fatalf("RunOne = %#v/%v, want zero/%v", result, err, test.wantCode)
			}
			calls := store.snapshot()
			if calls.claims != 1 || calls.acknowledgements != 0 || calls.failures != 0 ||
				encoder.callCount() != 0 || publisher.callCount() != 0 {
				t.Fatalf("Claim/Encode/Publish/Ack/Failure = %d/%d/%d/%d/%d, want 1/0/0/0/0",
					calls.claims, encoder.callCount(), publisher.callCount(), calls.acknowledgements, calls.failures)
			}
			if err != nil && strings.Contains(err.Error(), "credential") {
				t.Fatalf("Relay leaked raw Claim error: %v", err)
			}
		})
	}
}

func TestRelayRejectsInvariantEncoderResultsBeforePublishOrMutation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		envelope []byte
		err      error
	}{
		{name: "invalid-input", err: encodeError(EncoderFailureInvalidInput)},
		{name: "empty success"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
			encoder := &relaySecurityEncoder{envelope: test.envelope, err: test.err}
			publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}
			result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
			if result != (Result{}) || relayErrorCode(err) != RelayErrorInvariantViolation {
				t.Fatalf("RunOne = %#v/%v, want zero/invariant-violation", result, err)
			}
			calls := store.snapshot()
			if calls.claims != 1 || encoder.callCount() != 1 || publisher.callCount() != 0 ||
				calls.acknowledgements != 0 || calls.failures != 0 {
				t.Fatalf("Claim/Encode/Publish/Ack/Failure = %d/%d/%d/%d/%d, want 1/1/0/0/0",
					calls.claims, encoder.callCount(), publisher.callCount(), calls.acknowledgements, calls.failures)
			}
		})
	}
}

func TestRelayRejectsPublisherInvariantResultsWithoutTerminalMutation(t *testing.T) {
	t.Parallel()

	t.Run("invalid-input", func(t *testing.T) {
		t.Parallel()
		store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
		encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope")}
		publisher := mustRelayPublisher(t, outboxpublish.ScriptedStep{Failure: outboxpublish.FailureInvalidInput})
		result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
		if result != (Result{}) || relayErrorCode(err) != RelayErrorInvariantViolation {
			t.Fatalf("RunOne = %#v/%v, want zero/invariant-violation", result, err)
		}
		calls := store.snapshot()
		if calls.claims != 1 || encoder.callCount() != 1 || len(publisher.Calls()) != 1 ||
			calls.acknowledgements != 0 || calls.failures != 0 {
			t.Fatalf("Claim/Encode/Publish/Ack/Failure = %d/%d/%d/%d/%d, want 1/1/1/0/0",
				calls.claims, encoder.callCount(), len(publisher.Calls()), calls.acknowledgements, calls.failures)
		}
	})

	for _, test := range []struct {
		name string
		ack  outboxpublish.Acknowledgement
	}{
		{name: "zero sequence", ack: outboxpublish.Acknowledgement{Stream: relayTestBinding.Stream, MessageID: validEncoderTestFacts(nil).BrokerMessageID}},
		{name: "wrong message ID", ack: outboxpublish.Acknowledgement{Stream: relayTestBinding.Stream, Sequence: 1, MessageID: strings.Repeat("b", 64)}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
			encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope")}
			publisher := &relaySecurityPublisher{acknowledgement: test.ack}
			result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
			if result != (Result{}) || relayErrorCode(err) != RelayErrorInvariantViolation {
				t.Fatalf("RunOne = %#v/%v, want zero/invariant-violation", result, err)
			}
			calls := store.snapshot()
			if calls.claims != 1 || encoder.callCount() != 1 || publisher.callCount() != 1 ||
				calls.acknowledgements != 0 || calls.failures != 0 {
				t.Fatalf("Claim/Encode/Publish/Ack/Failure = %d/%d/%d/%d/%d, want 1/1/1/0/0",
					calls.claims, encoder.callCount(), publisher.callCount(), calls.acknowledgements, calls.failures)
			}
		})
	}
}

func TestRelayRejectsContradictoryDependencyTuplesWithoutMutation(t *testing.T) {
	t.Parallel()

	t.Run("encoder envelope and error", func(t *testing.T) {
		t.Parallel()
		store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
		encoder := &relaySecurityEncoder{
			envelope: []byte("contradictory-envelope-secret"),
			err:      encodeError(EncoderFailureEventRetryable),
		}
		publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}

		result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
		if result != (Result{}) || relayErrorCode(err) != RelayErrorInvariantViolation {
			t.Fatalf("RunOne = %#v/%v, want zero/invariant-violation", result, err)
		}
		calls := store.snapshot()
		if encoder.callCount() != 1 || publisher.callCount() != 0 ||
			calls.acknowledgements != 0 || calls.failures != 0 {
			t.Fatalf("Encode/Publish/Ack/Failure = %d/%d/%d/%d, want 1/0/0/0",
				encoder.callCount(), publisher.callCount(), calls.acknowledgements, calls.failures)
		}
	})

	t.Run("publisher acknowledgement and error", func(t *testing.T) {
		t.Parallel()
		store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
		encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope")}
		publisher := &relaySecurityPublisher{
			acknowledgement: relaySecurityAcknowledgement(),
			err:             relaySecurityPublishError(t, outboxpublish.FailurePublishOutcomeUnknown),
		}

		result, err := mustRelay(t, store, encoder, publisher).RunOne(context.Background())
		if result != (Result{}) || relayErrorCode(err) != RelayErrorInvariantViolation {
			t.Fatalf("RunOne = %#v/%v, want zero/invariant-violation", result, err)
		}
		calls := store.snapshot()
		if encoder.callCount() != 1 || publisher.callCount() != 1 ||
			calls.acknowledgements != 0 || calls.failures != 0 {
			t.Fatalf("Encode/Publish/Ack/Failure = %d/%d/%d/%d, want 1/1/0/0",
				encoder.callCount(), publisher.callCount(), calls.acknowledgements, calls.failures)
		}
	})
}

func TestRelayClonesAnAliasedEncoderEnvelopeBeforeClearingClaimFacts(t *testing.T) {
	t.Parallel()

	wantBody := []byte("payload-secret")
	store := &relaySecurityStore{
		claims:    []outboxdb.Claim{relaySecurityClaim(t)},
		ackResult: outboxdb.AcknowledgementDelivered,
	}
	publisher := &relaySecurityPublisher{
		hook: func(_ context.Context, message outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
			if !bytes.Equal(message.Body, wantBody) {
				t.Fatalf("published body = %v, want the original aliased payload", message.Body)
			}
			return relaySecurityAcknowledgement(), nil
		},
	}

	result, err := mustRelay(t, store, relayAliasingEncoder{}, publisher).RunOne(context.Background())
	if err != nil || result.Outcome != OutcomeDelivered {
		t.Fatalf("RunOne = %#v/%v, want Delivered/nil", result, err)
	}
	if calls := store.snapshot(); calls.acknowledgements != 1 || calls.failures != 0 {
		t.Fatalf("Ack/Failure calls = %d/%d, want 1/0", calls.acknowledgements, calls.failures)
	}
}

func TestRelayCallerContextErrorWinsOverDependencyContextError(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{
		"claim",
		"encode",
		"contradictory encode",
		"publish",
		"contradictory publish",
		"acknowledge",
		"record failure",
	} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			ctx := newRelaySecurityContext()
			store := &relaySecurityStore{
				claims:        []outboxdb.Claim{relaySecurityClaim(t)},
				ackResult:     outboxdb.AcknowledgementDelivered,
				failureResult: relaySecurityRetryResult(),
			}
			encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope")}
			publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}
			wantOutcome := Outcome(0)

			switch stage {
			case "claim":
				store.claimHook = func(context.Context) { ctx.fail(context.DeadlineExceeded) }
				store.claimError = context.Canceled
			case "encode":
				encoder.hook = func(context.Context, outboxdb.PublishFacts) ([]byte, error) {
					ctx.fail(context.DeadlineExceeded)
					return nil, context.Canceled
				}
			case "contradictory encode":
				encoder.hook = func(context.Context, outboxdb.PublishFacts) ([]byte, error) {
					ctx.fail(context.DeadlineExceeded)
					return []byte("contradictory-envelope"), context.Canceled
				}
			case "publish":
				publisher.hook = func(context.Context, outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
					ctx.fail(context.DeadlineExceeded)
					return outboxpublish.Acknowledgement{}, context.Canceled
				}
			case "contradictory publish":
				publisher.hook = func(context.Context, outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
					ctx.fail(context.DeadlineExceeded)
					return relaySecurityAcknowledgement(), context.Canceled
				}
			case "acknowledge":
				store.ackHook = func(context.Context) { ctx.fail(context.DeadlineExceeded) }
				store.ackError = context.Canceled
				wantOutcome = OutcomeAcknowledgementUnconfirmed
			case "record failure":
				encoder.err = encodeError(EncoderFailureEventRetryable)
				encoder.envelope = nil
				store.failureHook = func(context.Context) { ctx.fail(context.DeadlineExceeded) }
				store.failureError = context.Canceled
				wantOutcome = OutcomeFailureUnconfirmed
			}

			result, err := mustRelay(t, store, encoder, publisher).RunOne(ctx)
			if result.Outcome != wantOutcome || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("RunOne = %#v/%v, want outcome %v and caller deadline", result, err, wantOutcome)
			}
		})
	}
}

func TestRelayVerifiedPubAckNeverFallsBackToFailureOrRetries(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		ackResult   outboxdb.Acknowledgement
		ackError    error
		armContext  func(*relaySecurityContext, *relaySecurityStore)
		wantOutcome Outcome
		wantCode    RelayErrorCode
		wantContext error
	}{
		{
			name: "persistence failure", ackError: relaySecurityStoreError(t, outboxdb.StoreErrorPersistence),
			wantOutcome: OutcomeAcknowledgementUnconfirmed, wantCode: RelayErrorAcknowledgementUnconfirmed,
		},
		{
			name: "claim denied", ackError: relaySecurityStoreError(t, outboxdb.StoreErrorClaimDenied),
			wantOutcome: OutcomeClaimLost,
		},
		{
			name: "canceled in acknowledgement",
			armContext: func(ctx *relaySecurityContext, store *relaySecurityStore) {
				store.ackHook = func(context.Context) { ctx.fail(context.Canceled) }
			},
			wantOutcome: OutcomeAcknowledgementUnconfirmed, wantContext: context.Canceled,
		},
		{
			name: "deadline in acknowledgement",
			armContext: func(ctx *relaySecurityContext, store *relaySecurityStore) {
				store.ackHook = func(context.Context) { ctx.fail(context.DeadlineExceeded) }
			},
			wantOutcome: OutcomeAcknowledgementUnconfirmed, wantContext: context.DeadlineExceeded,
		},
		{
			name: "invalid acknowledgement result", ackResult: 99,
			wantOutcome: OutcomeAcknowledgementUnconfirmed, wantCode: RelayErrorInvariantViolation,
		},
		{
			name: "raw acknowledgement error", ackError: errors.New("raw acknowledgement credential secret"),
			wantOutcome: OutcomeAcknowledgementUnconfirmed, wantCode: RelayErrorInvariantViolation,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := newRelaySecurityContext()
			claim := relaySecurityClaim(t)
			store := &relaySecurityStore{claims: []outboxdb.Claim{claim}, ackResult: test.ackResult, ackError: test.ackError}
			if test.armContext != nil {
				test.armContext(ctx, store)
			}
			encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")}
			ack := relaySecurityAcknowledgement()
			publisher := &relaySecurityPublisher{acknowledgement: ack}
			relay := mustRelay(t, store, encoder, publisher)

			result, err := relay.RunOne(ctx)
			if result.Outcome != test.wantOutcome {
				t.Fatalf("RunOne outcome = %v, want %v (error %v)", result.Outcome, test.wantOutcome, err)
			}
			switch {
			case test.wantContext != nil && !errors.Is(err, test.wantContext):
				t.Fatalf("RunOne error = %v, want %v", err, test.wantContext)
			case test.wantContext == nil && test.wantCode != "" && relayErrorCode(err) != test.wantCode:
				t.Fatalf("RunOne error = %v, want %v", err, test.wantCode)
			case test.wantContext == nil && test.wantCode == "" && err != nil:
				t.Fatalf("RunOne error = %v, want nil", err)
			}

			calls := store.snapshot()
			if calls.claims != 1 || calls.acknowledgements != 1 || calls.failures != 0 ||
				encoder.callCount() != 1 || publisher.callCount() != 1 {
				t.Fatalf("Claim/Encode/Publish/Ack/Failure calls = %d/%d/%d/%d/%d, want 1/1/1/1/0",
					calls.claims, encoder.callCount(), publisher.callCount(), calls.acknowledgements, calls.failures)
			}
			if store.lastAcknowledgement() != ack {
				t.Fatal("Relay changed the verified acknowledgement before the sole database ACK")
			}
			assertRelaySecurityContexts(t, ctx, store.contextsSnapshot(), encoder.contextsSnapshot(), publisher.contextsSnapshot())
			if err != nil && strings.Contains(err.Error(), "credential") {
				t.Fatalf("Relay leaked raw acknowledgement error: %v", err)
			}
		})
	}
}

func TestRelayCancellationStopsAtEveryStageWithoutDetachedWork(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		stage                            string
		claims, encodes, publishes, acks int
	}{
		{stage: "before claim"},
		{stage: "after claim before encode", claims: 1},
		{stage: "in encoder", claims: 1, encodes: 1},
		{stage: "after encode before publish", claims: 1, encodes: 1},
		{stage: "empty encoder success after cancellation", claims: 1, encodes: 1},
		{stage: "in publisher", claims: 1, encodes: 1, publishes: 1},
		{stage: "malformed publisher success after cancellation", claims: 1, encodes: 1, publishes: 1},
		{stage: "after PubAck in acknowledgement", claims: 1, encodes: 1, publishes: 1, acks: 1},
	} {
		test := test
		t.Run(test.stage, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
			encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope")}
			publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}

			switch test.stage {
			case "before claim":
				cancel()
			case "after claim before encode":
				store.claimHook = func(context.Context) { cancel() }
			case "in encoder":
				encoder.hook = func(ctx context.Context, _ outboxdb.PublishFacts) ([]byte, error) {
					cancel()
					return nil, ctx.Err()
				}
			case "after encode before publish":
				encoder.hook = func(context.Context, outboxdb.PublishFacts) ([]byte, error) {
					cancel()
					return []byte("encoded-envelope"), nil
				}
			case "empty encoder success after cancellation":
				encoder.hook = func(context.Context, outboxdb.PublishFacts) ([]byte, error) {
					cancel()
					return nil, nil
				}
			case "in publisher":
				publisher.hook = func(ctx context.Context, _ outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
					cancel()
					return outboxpublish.Acknowledgement{}, ctx.Err()
				}
			case "malformed publisher success after cancellation":
				publisher.hook = func(context.Context, outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
					cancel()
					return outboxpublish.Acknowledgement{}, nil
				}
			case "after PubAck in acknowledgement":
				publisher.hook = func(context.Context, outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
					cancel()
					return relaySecurityAcknowledgement(), nil
				}
				store.ackHook = func(context.Context) {}
			}

			result, err := mustRelay(t, store, encoder, publisher).RunOne(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("RunOne = %#v/%v, want context.Canceled", result, err)
			}
			calls := store.snapshot()
			if calls.failures != 0 {
				t.Fatalf("canceled relay recorded %d Event failures", calls.failures)
			}
			if calls.claims != test.claims || encoder.callCount() != test.encodes ||
				publisher.callCount() != test.publishes || calls.acknowledgements != test.acks {
				t.Fatalf("canceled Claim/Encode/Publish/Ack calls = %d/%d/%d/%d, want %d/%d/%d/%d",
					calls.claims, encoder.callCount(), publisher.callCount(), calls.acknowledgements,
					test.claims, test.encodes, test.publishes, test.acks)
			}
			if test.stage == "after PubAck in acknowledgement" {
				if result.Outcome != OutcomeAcknowledgementUnconfirmed || calls.acknowledgements != 1 || publisher.callCount() != 1 {
					t.Fatalf("post-PubAck cancellation = %#v calls %#v/%d, want AckUnconfirmed and one Publish/Ack",
						result, calls, publisher.callCount())
				}
			}
			assertRelaySecurityContexts(t, ctx, store.contextsSnapshot(), encoder.contextsSnapshot(), publisher.contextsSnapshot())
		})
	}
}

func TestRelayRecordFailureCancellationIsUnconfirmedAndNeverRetried(t *testing.T) {
	t.Parallel()

	for _, contextError := range []error{context.Canceled, context.DeadlineExceeded} {
		contextError := contextError
		t.Run(contextError.Error(), func(t *testing.T) {
			t.Parallel()
			ctx := newRelaySecurityContext()
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}}
			store.failureHook = func(context.Context) { ctx.fail(contextError) }
			encoder := &relaySecurityEncoder{err: encodeError(EncoderFailureEventRetryable)}
			publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}

			result, err := mustRelay(t, store, encoder, publisher).RunOne(ctx)
			if result.Outcome != OutcomeFailureUnconfirmed || !errors.Is(err, contextError) {
				t.Fatalf("RunOne = %#v/%v, want FailureUnconfirmed/%v", result, err, contextError)
			}
			calls := store.snapshot()
			if calls.claims != 1 || encoder.callCount() != 1 || publisher.callCount() != 0 ||
				calls.acknowledgements != 0 || calls.failures != 1 {
				t.Fatalf("Claim/Encode/Publish/Ack/Failure = %d/%d/%d/%d/%d, want 1/1/0/0/1",
					calls.claims, encoder.callCount(), publisher.callCount(), calls.acknowledgements, calls.failures)
			}
			assertRelaySecurityContexts(t, ctx, store.contextsSnapshot(), encoder.contextsSnapshot())
		})
	}
}

func TestRelayNormalizesRawDependencyErrorsAndRedactsEverySurface(t *testing.T) {
	t.Parallel()

	const rawSecret = "raw-dependency-credential-secret"
	for _, stage := range []string{"claim", "encode", "publish", "acknowledge", "record failure"} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			store := &relaySecurityStore{claims: []outboxdb.Claim{relaySecurityClaim(t)}, ackResult: outboxdb.AcknowledgementDelivered}
			encoder := &relaySecurityEncoder{envelope: []byte("encoded-envelope-secret")}
			publisher := &relaySecurityPublisher{acknowledgement: relaySecurityAcknowledgement()}
			raw := errors.New(rawSecret)
			switch stage {
			case "claim":
				store.claimError = raw
			case "encode":
				encoder.err = raw
			case "publish":
				publisher.err = raw
			case "acknowledge":
				store.ackError = raw
			case "record failure":
				encoder.err = encodeError(EncoderFailureEventRetryable)
				store.failureError = raw
			}
			relay := mustRelay(t, store, encoder, publisher)
			result, err := relay.RunOne(context.Background())
			if relayErrorCode(err) != RelayErrorInvariantViolation {
				t.Fatalf("raw %s error = %v, want invariant-violation", stage, err)
			}
			securityAssertRelayRedacted(t, []any{
				relay,
				result,
				result.Outcome,
				err,
				RelayErrorInvariantViolation,
			}, rawSecret)
		})
	}
}

type relaySecurityStore struct {
	mutex sync.Mutex

	claims        []outboxdb.Claim
	claimError    error
	claimHook     func(context.Context)
	ackResult     outboxdb.Acknowledgement
	ackError      error
	ackHook       func(context.Context)
	failureResult outboxdb.FailureResult
	failureError  error
	failureHook   func(context.Context)

	claimContexts   []context.Context
	ackContexts     []context.Context
	failureContexts []context.Context
	acknowledgement outboxpublish.Acknowledgement
	claimCalls      int
	ackCalls        int
	failureCalls    int
}

type relaySecurityStoreSnapshot struct {
	claims           int
	acknowledgements int
	failures         int
}

func (store *relaySecurityStore) Claim(ctx context.Context, _ outboxdb.ClaimRequest) ([]outboxdb.Claim, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.claimCalls++
	store.claimContexts = append(store.claimContexts, ctx)
	if store.claimHook != nil {
		store.claimHook(ctx)
	}
	return append([]outboxdb.Claim(nil), store.claims...), store.claimError
}

func (*relaySecurityStore) Renew(context.Context, outboxdb.Claim) (outboxdb.Renewal, error) {
	return outboxdb.Renewal{}, errors.New("unexpected Renew")
}

func (store *relaySecurityStore) Acknowledge(
	ctx context.Context,
	_ outboxdb.Claim,
	acknowledgement outboxpublish.Acknowledgement,
) (outboxdb.Acknowledgement, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.ackCalls++
	store.ackContexts = append(store.ackContexts, ctx)
	store.acknowledgement = acknowledgement
	if store.ackHook != nil {
		store.ackHook(ctx)
	}
	if store.ackError != nil {
		return store.ackResult, store.ackError
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return 0, contextErr
	}
	return store.ackResult, nil
}

func (store *relaySecurityStore) RecordFailure(
	ctx context.Context,
	_ outboxdb.Claim,
	_ outboxdb.FailureCode,
) (outboxdb.FailureResult, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.failureCalls++
	store.failureContexts = append(store.failureContexts, ctx)
	if store.failureHook != nil {
		store.failureHook(ctx)
	}
	if store.failureError != nil {
		return store.failureResult, store.failureError
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return outboxdb.FailureResult{}, contextErr
	}
	return store.failureResult, nil
}

func (store *relaySecurityStore) snapshot() relaySecurityStoreSnapshot {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return relaySecurityStoreSnapshot{claims: store.claimCalls, acknowledgements: store.ackCalls, failures: store.failureCalls}
}

func (store *relaySecurityStore) contextsSnapshot() []context.Context {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	contexts := append([]context.Context(nil), store.claimContexts...)
	contexts = append(contexts, store.ackContexts...)
	return append(contexts, store.failureContexts...)
}

func (store *relaySecurityStore) lastAcknowledgement() outboxpublish.Acknowledgement {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return store.acknowledgement
}

type relaySecurityEncoder struct {
	mutex    sync.Mutex
	envelope []byte
	err      error
	hook     func(context.Context, outboxdb.PublishFacts) ([]byte, error)
	calls    []outboxdb.PublishFacts
	contexts []context.Context
}

type relayAliasingEncoder struct{}

func (relayAliasingEncoder) Encode(_ context.Context, facts outboxdb.PublishFacts) ([]byte, error) {
	return facts.Payload, nil
}

func (encoder *relaySecurityEncoder) Encode(ctx context.Context, facts outboxdb.PublishFacts) ([]byte, error) {
	encoder.mutex.Lock()
	defer encoder.mutex.Unlock()
	encoder.calls = append(encoder.calls, facts.Clone())
	encoder.contexts = append(encoder.contexts, ctx)
	if encoder.hook != nil {
		return encoder.hook(ctx, facts.Clone())
	}
	return append([]byte(nil), encoder.envelope...), encoder.err
}

func (encoder *relaySecurityEncoder) callCount() int {
	encoder.mutex.Lock()
	defer encoder.mutex.Unlock()
	return len(encoder.calls)
}

func (encoder *relaySecurityEncoder) contextsSnapshot() []context.Context {
	encoder.mutex.Lock()
	defer encoder.mutex.Unlock()
	return append([]context.Context(nil), encoder.contexts...)
}

type relaySecurityPublisher struct {
	mutex           sync.Mutex
	acknowledgement outboxpublish.Acknowledgement
	err             error
	hook            func(context.Context, outboxpublish.Message) (outboxpublish.Acknowledgement, error)
	calls           []outboxpublish.Message
	contexts        []context.Context
}

func (publisher *relaySecurityPublisher) Publish(ctx context.Context, message outboxpublish.Message) (outboxpublish.Acknowledgement, error) {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	message.Body = append([]byte(nil), message.Body...)
	publisher.calls = append(publisher.calls, message)
	publisher.contexts = append(publisher.contexts, ctx)
	if publisher.hook != nil {
		return publisher.hook(ctx, message)
	}
	return publisher.acknowledgement, publisher.err
}

func (publisher *relaySecurityPublisher) callCount() int {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	return len(publisher.calls)
}

func (publisher *relaySecurityPublisher) contextsSnapshot() []context.Context {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	return append([]context.Context(nil), publisher.contexts...)
}

type relaySecurityContext struct {
	context.Context
	mutex sync.Mutex
	err   error
	done  chan struct{}
	once  sync.Once
}

func newRelaySecurityContext() *relaySecurityContext {
	return &relaySecurityContext{Context: context.Background(), done: make(chan struct{})}
}

func (ctx *relaySecurityContext) Done() <-chan struct{} { return ctx.done }

func (ctx *relaySecurityContext) Err() error {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	return ctx.err
}

func (ctx *relaySecurityContext) fail(err error) {
	ctx.mutex.Lock()
	ctx.err = err
	ctx.mutex.Unlock()
	ctx.once.Do(func() { close(ctx.done) })
}

func relaySecurityClaim(t *testing.T) outboxdb.Claim {
	t.Helper()
	store := mustRelayStore(t, outboxdb.ScriptedStorePlan{
		Claims: []outboxdb.ScriptedClaimStep{{Claims: []outboxdb.ScriptedClaim{relayScriptedClaim([]byte("payload-secret"))}}},
	})
	claims, err := store.Claim(context.Background(), outboxdb.ClaimRequest{ClaimOwnerID: relayTestOwner, BatchSize: 1})
	if err != nil || len(claims) != 1 {
		t.Fatalf("security fixture Claim = %d/%v, want one", len(claims), err)
	}
	return claims[0]
}

func relaySecurityAcknowledgement() outboxpublish.Acknowledgement {
	return outboxpublish.Acknowledgement{
		Stream:    relayTestBinding.Stream,
		Sequence:  1,
		MessageID: validEncoderTestFacts(nil).BrokerMessageID,
	}
}

func relaySecurityRetryResult() outboxdb.FailureResult {
	return outboxdb.FailureResult{
		Disposition:   outboxdb.FailureRetryScheduled,
		NextAttemptAt: time.Date(2026, time.August, 24, 1, 2, 3, 0, time.UTC),
	}
}

func relaySecurityStoreError(t *testing.T, code outboxdb.StoreErrorCode) error {
	t.Helper()
	store := mustRelayStore(t, outboxdb.ScriptedStorePlan{
		Claims: []outboxdb.ScriptedClaimStep{{Failure: code}},
	})
	_, err := store.Claim(context.Background(), outboxdb.ClaimRequest{ClaimOwnerID: relayTestOwner, BatchSize: 1})
	if actual, ok := outboxdb.StoreErrorCodeOf(err); !ok || actual != code {
		t.Fatalf("fixture Store error = %v, want %v", err, code)
	}
	return err
}

func relaySecurityPublishError(t *testing.T, code outboxpublish.FailureCode) error {
	t.Helper()
	publisher := mustRelayPublisher(t, outboxpublish.ScriptedStep{Failure: code})
	_, err := publisher.Publish(context.Background(), outboxpublish.Message{
		Body:      []byte("encoded-envelope"),
		MessageID: validEncoderTestFacts(nil).BrokerMessageID,
	})
	if actual, ok := outboxpublish.FailureCodeOf(err); !ok || actual != code {
		t.Fatalf("fixture Publisher error = %v, want %v", err, code)
	}
	return err
}

func assertRelaySecurityContexts(t *testing.T, want context.Context, groups ...[]context.Context) {
	t.Helper()
	for _, group := range groups {
		for _, got := range group {
			if got != want {
				t.Fatalf("dependency context = %T/%p, want exact %T/%p", got, got, want, want)
			}
		}
	}
}

func securityAssertRelayRedacted(t *testing.T, values []any, extraSecrets ...string) {
	t.Helper()
	secrets := append([]string{
		"payload-secret",
		"encoded-envelope-secret",
		"tenant-secret",
		"event-secret",
		"aggregate-secret",
		"channel-secret",
		outboxpublish.LogicalDestinationDomainEvents,
		validEncoderTestFacts(nil).BrokerMessageID,
		relayTestBinding.Stream,
		relayTestOwner,
	}, extraSecrets...)
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, rendered := range []string{
			fmt.Sprint(value),
			fmt.Sprintf("%+v", value),
			fmt.Sprintf("%#v", value),
			string(encoded),
		} {
			for _, secret := range secrets {
				if strings.Contains(rendered, secret) {
					t.Fatalf("relay rendering exposed %q: %q", secret, rendered)
				}
			}
		}
	}
}

var _ outboxdb.Store = (*relaySecurityStore)(nil)
var _ Encoder = relayAliasingEncoder{}
var _ Encoder = (*relaySecurityEncoder)(nil)
var _ outboxpublish.Publisher = (*relaySecurityPublisher)(nil)
