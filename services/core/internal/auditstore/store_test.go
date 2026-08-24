package auditstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
)

func TestAppendRejectsInvalidInputWithoutTransaction(t *testing.T) {
	t.Parallel()

	_, err := appendEvent(context.Background(), nil, testCandidate())
	if !hasErrorCode(err, errorInvalidInput) {
		t.Fatalf("nil transaction error = %v, want %q", err, errorInvalidInput)
	}

	invalid := testCandidate()
	invalid.requestID = "bad?request"
	_, err = appendEvent(context.Background(), nil, invalid)
	if !hasErrorCode(err, errorInvalidInput) {
		t.Fatalf("invalid candidate error = %v, want %q", err, errorInvalidInput)
	}
}

func TestAppendReturnsCanceledContextFirst(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := appendEvent(ctx, nil, candidate{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("append error = %v, want context.Canceled", err)
	}
}

func TestAuditStoreErrorsDoNotExposeFacts(t *testing.T) {
	t.Parallel()

	secret := "synthetic-secret-canary"
	input := testCandidate()
	input.requestID = secret + "?"
	_, err := appendEvent(context.Background(), nil, input)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposed candidate facts: %v", err)
	}
}

func TestAppendParametersCopyMutableFacts(t *testing.T) {
	t.Parallel()

	input := testCandidate()
	input.evidenceDigest = bytes.Repeat([]byte{3}, hashBytes)
	slot := testSlot()
	digest := bytes.Repeat([]byte{4}, hashBytes)
	row := dbSlot(slot)
	params := appendParams(input, row, digest)

	input.evidenceDigest[0] = 9
	slot.previousEventHash[0] = 9
	digest[0] = 9
	if params.EvidenceDigest[0] != 3 || params.PreviousEventHash[0] != 0 || params.EventHash[0] != 4 {
		t.Fatal("append parameters retained caller-owned mutable slices")
	}
}

func FuzzAuditTranscriptDeterministic(f *testing.F) {
	f.Add("tenant-fuzz", "event-fuzz", "request-fuzz")
	f.Fuzz(func(t *testing.T, tenantID, eventID, requestID string) {
		input := testCandidate()
		input.tenantID = tenantID
		input.auditEventID = eventID
		input.requestID = requestID
		slot := testSlot()
		slot.tenantID = tenantID
		firstTranscript, firstHash, firstErr := hashEvent(input, slot)
		secondTranscript, secondHash, secondErr := hashEvent(input, slot)
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatal("identical input produced inconsistent validation")
		}
		if firstErr != nil {
			if validCandidate(input) {
				t.Fatal("valid candidate was rejected")
			}
			return
		}
		if !validCandidate(input) || !bytes.Equal(firstTranscript, secondTranscript) || !bytes.Equal(firstHash, secondHash) {
			t.Fatal("valid input produced a nondeterministic transcript or hash")
		}
		oracle := sha256.Sum256(firstTranscript)
		if !bytes.Equal(firstHash, oracle[:]) || !bytes.HasPrefix(firstTranscript, []byte(transcriptPrefix)) {
			t.Fatal("hash or transcript domain does not match the independent oracle")
		}
		mutated := cloneCandidate(input)
		if len(mutated.auditEventID)+2 <= identifierMaxBytes {
			mutated.auditEventID += "-x"
			if validCandidate(mutated) {
				_, mutatedHash, mutationErr := hashEvent(mutated, slot)
				if mutationErr != nil || bytes.Equal(mutatedHash, firstHash) {
					t.Fatal("valid single-field mutation did not change the Event hash")
				}
			}
		}
	})
}
