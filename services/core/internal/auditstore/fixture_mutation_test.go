package auditstore

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestCommittedAuditMutationsAreRejectedOrRebound(t *testing.T) {
	t.Parallel()

	fixture := loadAuditFixture(t)
	gotIDs := make([]string, 0, len(fixture.RejectedMutations))
	for _, mutation := range fixture.RejectedMutations {
		gotIDs = append(gotIDs, mutation.ID)
	}
	if !reflect.DeepEqual(gotIDs, requiredAuditMutationIDs) {
		t.Fatalf("fixture mutation IDs changed\n got: %v\nwant: %v", gotIDs, requiredAuditMutationIDs)
	}

	notCandidateInputs := map[string]bool{
		"contract-version": true, "target-version": true, "event-hash": true,
		"delete-middle": true, "reorder": true, "cross-tenant": true, "head-hash": true,
		"unknown-field": true, "disallowed-prompt-field": true,
	}
	for _, mutation := range fixture.RejectedMutations {
		mutation := mutation
		t.Run(mutation.ID, func(t *testing.T) {
			t.Parallel()
			if notCandidateInputs[mutation.ID] {
				// These facts are fixed/derived or chain-envelope operations and
				// therefore cannot enter the typed Candidate API.
				return
			}
			index := fixtureMutationEventIndex(mutation.ID)
			input, slot := fixtureCandidateAndSlot(t, fixture.Valid.Events, index)
			_, baseline, err := hashEvent(input, slot)
			if err != nil {
				t.Fatal(err)
			}
			applyFixtureMutation(t, mutation, &input, &slot)
			_, mutated, mutationErr := hashEvent(input, slot)
			if mutationErr == nil && bytes.Equal(mutated, baseline) {
				t.Fatal("committed mutation was neither rejected nor bound by the Event hash")
			}
		})
	}
}

func fixtureMutationEventIndex(id string) int {
	switch id {
	case "duplicate-event-id", "sequence-gap", "sequence-overflow", "target-version",
		"approval", "evidence-digest", "previous-hash":
		return 1
	case "recovery-case":
		return 2
	default:
		return 0
	}
}

func applyFixtureMutation(t *testing.T, mutation fixtureMutation, input *candidate, slot *appendSlot) {
	t.Helper()
	stringValue := func() string {
		var value string
		if err := json.Unmarshal(mutation.Value, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	switch mutation.ID {
	case "event-id", "duplicate-event-id":
		input.auditEventID = stringValue()
	case "tenant":
		input.tenantID = stringValue()
	case "invalid-unicode":
		input.auditEventID = string([]byte{0xed, 0xa0, 0x80})
	case "sequence-gap":
		slot.tenantSequence = 3
	case "sequence-overflow":
		slot.tenantSequence = math.MinInt64
	case "recorded-at":
		value, err := time.Parse("2006-01-02T15:04:05.000000000Z", stringValue())
		if err != nil {
			t.Fatal(err)
		}
		slot.recordedAt = value
	case "principal-id":
		input.principal.id = stringValue()
	case "principal-type":
		input.principal.typeID = actorAgent
	case "action", "unknown-action":
		input.action = action(stringValue())
	case "unknown-outcome", "outcome":
		input.outcome = outcome(stringValue())
	case "unknown-reason", "reason":
		input.reason = reason(stringValue())
	case "unknown-target-type", "target-type":
		input.target.typeID = targetType(stringValue())
	case "target-id":
		input.target.id = stringValue()
	case "policy":
		input.policyVersion = stringValue()
	case "request":
		input.requestID = stringValue()
	case "approval":
		value := stringValue()
		input.approvalID = &value
	case "recovery-case":
		value := stringValue()
		input.recoveryCaseID = &value
	case "evidence-digest":
		value, err := hex.DecodeString(stringValue())
		if err != nil {
			t.Fatal(err)
		}
		input.evidenceDigest = value
	case "previous-hash", "wrong-genesis":
		value, err := hex.DecodeString(stringValue())
		if err != nil {
			t.Fatal(err)
		}
		slot.previousEventHash = value
	default:
		t.Fatalf("fixture mutation %q has no producer coverage", mutation.ID)
	}
}

var requiredAuditMutationIDs = []string{
	"contract-version", "event-id", "duplicate-event-id", "tenant", "invalid-unicode",
	"sequence-gap", "sequence-overflow", "recorded-at", "principal-id", "principal-type",
	"action", "unknown-action", "unknown-outcome", "unknown-reason", "unknown-target-type",
	"outcome", "reason", "target-type", "target-id", "target-version", "policy", "request",
	"approval", "recovery-case", "evidence-digest", "previous-hash", "event-hash",
	"delete-middle", "reorder", "cross-tenant", "wrong-genesis", "head-hash", "unknown-field",
	"disallowed-prompt-field",
}
