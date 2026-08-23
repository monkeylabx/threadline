package outboxdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

const scriptedMessageID = "e57ad815a402753dd7698b0e941f70108383c92afecfc5d0c2b699ac36c82e97"

var scriptedBinding = Binding{
	LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
	Stream:             "DOMAIN_EVENTS",
}

func TestScriptedStoreClonesClaimsAndPreservesExactCalls(t *testing.T) {
	t.Parallel()

	facts := scriptedPublishFacts()
	originalPayload := bytes.Clone(facts.Payload)
	lease := scriptedLease()
	nextAttemptAt := lease.ExpiresAt.Add(time.Second)
	store := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Claims: []ScriptedClaimStep{{Claims: []ScriptedClaim{{PublishFacts: facts, Lease: lease}}}},
		Renewals: []ScriptedRenewStep{{Renewal: Renewal{
			LeaseExpiresAt: lease.ExpiresAt.Add(5 * time.Second),
		}}},
		Acknowledgements: []ScriptedAcknowledgementStep{{
			Acknowledgement: AcknowledgementAlreadyDelivered,
		}},
		Failures: []ScriptedFailureStep{{Result: FailureResult{
			Disposition:   FailureRetryScheduled,
			NextAttemptAt: nextAttemptAt,
		}}},
	})

	facts.Payload[0] = 'X'
	claims, err := store.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker-scripted-1", BatchSize: 1})
	if err != nil || len(claims) != 1 {
		t.Fatalf("claims = %d, error = %v, want one", len(claims), err)
	}
	observed := claims[0].PublishFacts()
	if !bytes.Equal(observed.Payload, originalPayload) || observed.BrokerMessageID != scriptedMessageID {
		t.Fatal("scripted Claim did not preserve cloned publish facts")
	}
	observed.Payload[0] = 'Y'
	if !bytes.Equal(claims[0].PublishFacts().Payload, originalPayload) {
		t.Fatal("PublishFacts exposed fake-owned payload memory")
	}
	if claims[0].Lease() != lease {
		t.Fatalf("lease = %#v, want exact database-authored timing", claims[0].Lease())
	}

	renewed, err := store.Renew(context.Background(), claims[0])
	if err != nil || renewed.LeaseExpiresAt.IsZero() {
		t.Fatalf("renewal = %#v, error = %v", renewed, err)
	}
	ack := outboxpublish.Acknowledgement{
		Stream: scriptedBinding.Stream, Sequence: math.MaxUint64, Duplicate: true, MessageID: scriptedMessageID,
	}
	acknowledged, err := store.Acknowledge(context.Background(), claims[0], ack)
	if err != nil || acknowledged != AcknowledgementAlreadyDelivered {
		t.Fatalf("acknowledgement = %v, error = %v", acknowledged, err)
	}
	failed, err := store.RecordFailure(context.Background(), claims[0], FailurePublishOutcomeUnknown)
	if err != nil || failed.Disposition != FailureRetryScheduled || !failed.NextAttemptAt.Equal(nextAttemptAt) {
		t.Fatalf("failure = %#v, error = %v", failed, err)
	}

	calls := store.Calls()
	if len(calls.Claims) != 1 || len(calls.Renewals) != 1 || len(calls.Acknowledgements) != 1 || len(calls.Failures) != 1 {
		t.Fatalf("call counts = %d/%d/%d/%d, want 1/1/1/1", len(calls.Claims), len(calls.Renewals), len(calls.Acknowledgements), len(calls.Failures))
	}
	if calls.Acknowledgements[0].Acknowledgement != ack || calls.Failures[0].Failure != FailurePublishOutcomeUnknown {
		t.Fatal("scripted Store call snapshots changed Ack or failure evidence")
	}
	calls.Renewals[0].Payload[0] = 'Z'
	if !bytes.Equal(store.Calls().Renewals[0].Payload, originalPayload) {
		t.Fatal("Calls exposed fake-owned payload memory")
	}
}

func TestScriptedStoreRejectsForeignClaimsAndAcknowledgementsWithoutConsumingPlans(t *testing.T) {
	t.Parallel()

	claimOwner := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Claims: []ScriptedClaimStep{{Claims: []ScriptedClaim{{
			PublishFacts: scriptedPublishFacts(), Lease: scriptedLease(),
		}}}},
	})
	foreign := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Renewals: []ScriptedRenewStep{{Renewal: Renewal{
			LeaseExpiresAt: scriptedLease().ExpiresAt.Add(time.Second),
		}}},
	})
	claims, err := claimOwner.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker-scripted-1", BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.Renew(context.Background(), claims[0]); !hasStoreErrorCode(err, StoreErrorClaimDenied) {
		t.Fatalf("cross-fake Renew error = %v, want claim-denied", err)
	}
	if len(foreign.Calls().Renewals) != 0 {
		t.Fatal("cross-fake Claim reached the fake plan")
	}

	store := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Claims: []ScriptedClaimStep{{Claims: []ScriptedClaim{{
			PublishFacts: scriptedPublishFacts(), Lease: scriptedLease(),
		}}}},
		Acknowledgements: []ScriptedAcknowledgementStep{{Acknowledgement: AcknowledgementDelivered}},
	})
	owned, err := store.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker-scripted-2", BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	for name, ack := range map[string]outboxpublish.Acknowledgement{
		"foreign stream": {Stream: "OTHER", Sequence: 1, MessageID: scriptedMessageID},
		"wrong message":  {Stream: scriptedBinding.Stream, Sequence: 1, MessageID: strings.Repeat("a", 64)},
		"zero sequence":  {Stream: scriptedBinding.Stream, MessageID: scriptedMessageID},
	} {
		if _, err := store.Acknowledge(context.Background(), owned[0], ack); !hasStoreErrorCode(err, StoreErrorInvalidInput) {
			t.Errorf("%s Ack error = %v, want invalid-input", name, err)
		}
	}
	if len(store.Calls().Acknowledgements) != 0 {
		t.Fatal("foreign/malformed Ack reached the fake plan")
	}
	if _, err := store.Acknowledge(context.Background(), owned[0], outboxpublish.Acknowledgement{
		Stream: scriptedBinding.Stream, Sequence: 1, MessageID: scriptedMessageID,
	}); err != nil {
		t.Fatalf("exact Ack did not consume retained plan: %v", err)
	}
}

func TestScriptedStoreRejectsInvalidPlansAndInputs(t *testing.T) {
	t.Parallel()

	invalidPlans := []ScriptedStorePlan{
		{Claims: []ScriptedClaimStep{{Claims: []ScriptedClaim{{}}}}},
		{Claims: []ScriptedClaimStep{{Claims: []ScriptedClaim{{PublishFacts: scriptedPublishFacts(), Lease: scriptedLease()}}, Failure: StoreErrorPersistence}}},
		{Renewals: []ScriptedRenewStep{{}}},
		{Acknowledgements: []ScriptedAcknowledgementStep{{}}},
		{Failures: []ScriptedFailureStep{{Result: FailureResult{Disposition: FailureRetryScheduled}}}},
		{Failures: []ScriptedFailureStep{{Result: FailureResult{Disposition: FailureParked, NextAttemptAt: time.Now()}}}},
	}
	for index, plan := range invalidPlans {
		if _, err := NewScriptedStore(scriptedBinding, plan); !hasStoreErrorCode(err, StoreErrorInvalidInput) {
			t.Errorf("invalid plan %d error = %v, want invalid-input", index, err)
		}
	}
	if _, err := NewScriptedStore(Binding{LogicalDestination: "domain-events", Stream: "OTHER.*"}, ScriptedStorePlan{}); !hasStoreErrorCode(err, StoreErrorInvalidInput) {
		t.Fatalf("invalid binding error = %v, want invalid-input", err)
	}

	store := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{})
	if _, err := store.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker", BatchSize: 0}); !hasStoreErrorCode(err, StoreErrorInvalidInput) {
		t.Fatalf("invalid Claim request error = %v, want invalid-input", err)
	}
	if _, err := store.Renew(context.Background(), Claim{}); !hasStoreErrorCode(err, StoreErrorClaimDenied) {
		t.Fatalf("zero Claim Renew error = %v, want claim-denied", err)
	}
	if _, err := store.RecordFailure(context.Background(), Claim{}, FailureTransportUnavailable); !hasStoreErrorCode(err, StoreErrorClaimDenied) {
		t.Fatalf("zero Claim failure error = %v, want claim-denied", err)
	}
}

func TestScriptedStoreFailureAllowlistAndAckBoundaries(t *testing.T) {
	t.Parallel()

	claimSpecs := make([]ScriptedClaim, 4)
	for index := range claimSpecs {
		facts := scriptedPublishFacts()
		facts.OutboxEntryID = int64(index + 1)
		claimSpecs[index] = ScriptedClaim{PublishFacts: facts, Lease: scriptedLease()}
	}
	store := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Claims: []ScriptedClaimStep{{Claims: claimSpecs}},
		Acknowledgements: []ScriptedAcknowledgementStep{
			{Acknowledgement: AcknowledgementDelivered},
			{Acknowledgement: AcknowledgementAlreadyDelivered},
		},
		Failures: []ScriptedFailureStep{
			{Result: FailureResult{Disposition: FailureRetryScheduled, NextAttemptAt: time.Now().Add(time.Second)}},
			{Result: FailureResult{Disposition: FailureRetryScheduled, NextAttemptAt: time.Now().Add(2 * time.Second)}},
			{Result: FailureResult{Disposition: FailureRetryScheduled, NextAttemptAt: time.Now().Add(3 * time.Second)}},
			{Result: FailureResult{Disposition: FailureParked}},
		},
	})
	claims, err := store.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker-scripted-boundaries", BatchSize: 4})
	if err != nil {
		t.Fatal(err)
	}

	for index, ack := range []outboxpublish.Acknowledgement{
		{Stream: scriptedBinding.Stream, Sequence: 1, Duplicate: false, MessageID: scriptedMessageID},
		{Stream: scriptedBinding.Stream, Sequence: math.MaxUint64, Duplicate: true, MessageID: scriptedMessageID},
	} {
		if _, err := store.Acknowledge(context.Background(), claims[index], ack); err != nil {
			t.Errorf("Ack boundary %d failed: %v", index, err)
		}
	}
	for index, code := range []FailureCode{
		FailureTransportUnavailable,
		FailurePublishOutcomeUnknown,
		FailureEventRetryable,
		FailureEventPermanent,
	} {
		if _, err := store.RecordFailure(context.Background(), claims[index], code); err != nil {
			t.Errorf("allowlisted failure %q failed: %v", code, err)
		}
	}
	before := len(store.Calls().Failures)
	for _, code := range []FailureCode{"future-failure", FailureCode(outboxpublish.FailureInvalidInput)} {
		if _, err := store.RecordFailure(context.Background(), claims[0], code); !hasStoreErrorCode(err, StoreErrorInvalidInput) {
			t.Errorf("failure %q error = %v, want invalid-input", code, err)
		}
	}
	if got := len(store.Calls().Failures); got != before {
		t.Fatalf("invalid failure calls = %d, want retained %d", got, before)
	}
}

func TestScriptedStoreCancellationConsumesNoCallsOrSteps(t *testing.T) {
	t.Parallel()

	store := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Claims: []ScriptedClaimStep{{Claims: []ScriptedClaim{{
			PublishFacts: scriptedPublishFacts(), Lease: scriptedLease(),
		}}}},
	})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	request := ClaimRequest{ClaimOwnerID: "worker-scripted-cancel", BatchSize: 1}
	if _, err := store.Claim(canceled, request); err != context.Canceled {
		t.Fatalf("canceled Claim error = %v, want context.Canceled", err)
	}
	if len(store.Calls().Claims) != 0 {
		t.Fatal("canceled Claim recorded a call")
	}
	claims, err := store.Claim(context.Background(), request)
	if err != nil || len(claims) != 1 {
		t.Fatalf("retained Claim step = %d/%v, want one", len(claims), err)
	}
}

func TestScriptedStoreRenewMatchesProductionLeaseRulesAndUpdatesClaimCopies(t *testing.T) {
	t.Parallel()

	lease := scriptedLease()
	validExpiry := lease.ExpiresAt.Add(time.Second)
	store := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Claims: []ScriptedClaimStep{{Claims: []ScriptedClaim{{
			PublishFacts: scriptedPublishFacts(), Lease: lease,
		}}}},
		Renewals: []ScriptedRenewStep{
			{Renewal: Renewal{LeaseExpiresAt: lease.ExpiresAt}},
			{Renewal: Renewal{LeaseExpiresAt: lease.AbsoluteLeaseExpiresAt.Add(time.Nanosecond)}},
			{Renewal: Renewal{LeaseExpiresAt: validExpiry}},
		},
	})
	claims, err := store.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker-scripted-renew", BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	claimCopy := claims[0]
	for _, name := range []string{"not strictly later", "past absolute cap"} {
		if _, err := store.Renew(context.Background(), claims[0]); !hasStoreErrorCode(err, StoreErrorPersistence) {
			t.Errorf("%s Renew error = %v, want persistence-failure", name, err)
		}
		if claims[0].Lease() != lease {
			t.Fatalf("%s Renew changed the shared lease", name)
		}
	}
	if _, err := store.Renew(context.Background(), claims[0]); err != nil {
		t.Fatalf("valid Renew failed: %v", err)
	}
	if got := claimCopy.Lease().ExpiresAt; !got.Equal(validExpiry) {
		t.Fatalf("copied Claim expiry = %v, want shared update %v", got, validExpiry)
	}
}

func TestScriptedStoreIsRaceSafeAndConsumesEachRenewalStepOnce(t *testing.T) {
	t.Parallel()

	const count = 32
	claimSpecs := make([]ScriptedClaim, count)
	renewalSteps := make([]ScriptedRenewStep, count)
	base := scriptedLease().ExpiresAt
	for index := 0; index < count; index++ {
		facts := scriptedPublishFacts()
		facts.OutboxEntryID = int64(index + 1)
		claimSpecs[index] = ScriptedClaim{PublishFacts: facts, Lease: scriptedLease()}
		renewalSteps[index] = ScriptedRenewStep{Renewal: Renewal{
			LeaseExpiresAt: base.Add(time.Duration(index+1) * time.Second),
		}}
	}
	store := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Claims:   []ScriptedClaimStep{{Claims: claimSpecs}},
		Renewals: renewalSteps,
	})
	claims, err := store.Claim(context.Background(), ClaimRequest{ClaimOwnerID: "worker-scripted-race", BatchSize: count})
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan time.Time, count)
	var wait sync.WaitGroup
	for _, claim := range claims {
		claim := claim
		wait.Add(1)
		go func() {
			defer wait.Done()
			renewal, renewErr := store.Renew(context.Background(), claim)
			if renewErr != nil {
				t.Errorf("parallel Renew failed: %v", renewErr)
				return
			}
			results <- renewal.LeaseExpiresAt
		}()
	}
	wait.Wait()
	close(results)

	got := make([]time.Time, 0, count)
	for result := range results {
		got = append(got, result)
	}
	sort.Slice(got, func(left, right int) bool { return got[left].Before(got[right]) })
	if len(got) != count {
		t.Fatalf("renewal results = %d, want %d", len(got), count)
	}
	for index := range got {
		if want := base.Add(time.Duration(index+1) * time.Second); !got[index].Equal(want) {
			t.Fatalf("renewal %d = %v, want %v", index, got[index], want)
		}
	}
	if calls := store.Calls(); len(calls.Renewals) != count {
		t.Fatalf("parallel Renew calls = %d, want %d", len(calls.Renewals), count)
	}
}

func TestScriptedStoreValuesAreSecretSafe(t *testing.T) {
	t.Parallel()

	facts := scriptedPublishFacts()
	store := mustScriptedStore(t, scriptedBinding, ScriptedStorePlan{
		Claims: []ScriptedClaimStep{{Claims: []ScriptedClaim{{PublishFacts: facts, Lease: scriptedLease()}}}},
	})
	values := []any{
		ScriptedClaim{PublishFacts: facts, Lease: scriptedLease()},
		ScriptedClaimStep{Claims: []ScriptedClaim{{PublishFacts: facts, Lease: scriptedLease()}}},
		ScriptedRenewStep{Renewal: Renewal{LeaseExpiresAt: scriptedLease().ExpiresAt}},
		ScriptedAcknowledgementStep{Acknowledgement: AcknowledgementDelivered},
		ScriptedFailureStep{Result: FailureResult{Disposition: FailureParked}},
		ScriptedStorePlan{},
		ScriptedAcknowledgementCall{PublishFacts: facts, Acknowledgement: outboxpublish.Acknowledgement{
			Stream: scriptedBinding.Stream, Sequence: 1, MessageID: scriptedMessageID,
		}},
		ScriptedFailureCall{PublishFacts: facts, Failure: FailureEventPermanent},
		store.Calls(),
		store,
	}
	secrets := []string{
		facts.TenantID, facts.EventID, facts.AggregateID, facts.LogicalDestination,
		facts.BrokerMessageID, scriptedBinding.Stream, string(facts.Payload),
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(encoded)} {
			for _, secret := range secrets {
				if strings.Contains(rendered, secret) {
					t.Fatalf("rendering exposed %q: %q", secret, rendered)
				}
			}
		}
	}
}

func mustScriptedStore(t *testing.T, binding Binding, plan ScriptedStorePlan) *ScriptedStore {
	t.Helper()
	store, err := NewScriptedStore(binding, plan)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func hasStoreErrorCode(err error, expected StoreErrorCode) bool {
	actual, ok := StoreErrorCodeOf(err)
	return ok && actual == expected
}

func scriptedPublishFacts() PublishFacts {
	return PublishFacts{
		TenantID:           "tenant-scripted-secret",
		EventID:            "event-scripted-secret",
		OutboxEntryID:      17,
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		BrokerMessageID:    scriptedMessageID,
		EventType:          "message.created",
		SchemaVersion:      1,
		AggregateKind:      "channel",
		AggregateID:        "aggregate-scripted-secret",
		Payload:            []byte("payload-scripted-secret"),
		OccurredAt:         time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC),
		EnqueuedAt:         time.Date(2026, 8, 24, 1, 2, 4, 0, time.UTC),
	}
}

func scriptedLease() Lease {
	claimedAt := time.Date(2026, 8, 24, 1, 2, 5, 0, time.UTC)
	return Lease{
		ClaimedAt: claimedAt, ExpiresAt: claimedAt.Add(30 * time.Second),
		AbsoluteLeaseExpiresAt: claimedAt.Add(5 * time.Minute),
	}
}
