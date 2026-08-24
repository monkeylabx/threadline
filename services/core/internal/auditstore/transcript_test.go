package auditstore

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestAuditFixtureHashesMatchExactly(t *testing.T) {
	t.Parallel()

	fixture := loadAuditFixture(t)
	for index, fixtureEvent := range fixture.Valid.Events {
		input, slot := fixtureCandidateAndSlot(t, fixture.Valid.Events, index)
		transcript, digest, err := hashEvent(input, slot)
		if err != nil {
			t.Fatalf("fixture Event %d hash failed: %v", index+1, err)
		}
		if got := hex.EncodeToString(digest); got != fixtureEvent.EventHashHex {
			t.Fatalf("fixture Event %d hash = %s, want %s", index+1, got, fixtureEvent.EventHashHex)
		}
		if !bytes.HasPrefix(transcript, []byte(transcriptPrefix+"{")) {
			t.Fatalf("fixture Event %d transcript has wrong domain", index+1)
		}
	}

	firstInput, firstSlot := fixtureCandidateAndSlot(t, fixture.Valid.Events, 0)
	firstTranscript, _, err := hashEvent(firstInput, firstSlot)
	if err != nil {
		t.Fatal(err)
	}
	want := transcriptPrefix + `{"action":"channel.archive","approvalId":null,"auditEventId":"audit-event-fixture-01","contractVersion":"1","evidenceDigestHex":null,"outcome":"succeeded","policyVersion":"policy-audit-fixture-01","previousEventHashHex":"0000000000000000000000000000000000000000000000000000000000000000","principal":{"actorId":"human-audit-fixture-01","actorType":"1"},"reason":"authorized","recordedAt":"2026-08-24T08:00:00.000000000Z","recoveryCaseId":null,"requestId":"request-audit-fixture-01","target":{"targetId":"channel-audit-fixture-01","targetType":"channel","targetVersion":null},"tenantId":"tenant-audit-fixture-01","tenantSequence":"1"}`
	if string(firstTranscript) != want {
		t.Fatalf("fixture transcript mismatch\n got: %s\nwant: %s", firstTranscript, want)
	}
}

func TestAuditTranscriptBindsEveryPersistedFact(t *testing.T) {
	t.Parallel()

	input := testCandidate()
	slot := testSlot()
	slot.tenantSequence = 2
	previousID := "audit-event-0"
	slot.previousAuditEventID = &previousID
	slot.previousEventHash = bytes.Repeat([]byte{1}, hashBytes)
	_, baseline, err := hashEvent(input, slot)
	if err != nil {
		t.Fatal(err)
	}
	version := int64(2)
	mutations := map[string]func(*candidate, *appendSlot){
		"tenant":         func(c *candidate, s *appendSlot) { c.tenantID = "tenant-2"; s.tenantID = "tenant-2" },
		"event":          func(c *candidate, _ *appendSlot) { c.auditEventID = "audit-event-2" },
		"principal ID":   func(c *candidate, _ *appendSlot) { c.principal.id = "actor-2" },
		"principal type": func(c *candidate, _ *appendSlot) { c.principal.typeID = actorAgent },
		"action": func(c *candidate, _ *appendSlot) {
			c.action = actionCapabilityGrantIssue
			c.target.typeID = targetCapabilityGrant
		},
		"outcome": func(c *candidate, _ *appendSlot) { c.outcome = outcomeDenied },
		"reason":  func(c *candidate, _ *appendSlot) { c.reason = reasonPolicyDenied },
		"target type": func(c *candidate, _ *appendSlot) {
			c.action = actionCapabilityGrantIssue
			c.target.typeID = targetCapabilityGrant
		},
		"target ID":      func(c *candidate, _ *appendSlot) { c.target.id = "channel-2" },
		"target version": func(c *candidate, _ *appendSlot) { c.target.version = &version },
		"policy":         func(c *candidate, _ *appendSlot) { c.policyVersion = "policy-2" },
		"request":        func(c *candidate, _ *appendSlot) { c.requestID = "request-2" },
		"approval": func(c *candidate, _ *appendSlot) {
			value := "approval-1"
			c.approvalID = &value
		},
		"evidence":      func(c *candidate, _ *appendSlot) { c.evidenceDigest = bytes.Repeat([]byte{9}, hashBytes) },
		"sequence":      func(_ *candidate, s *appendSlot) { s.tenantSequence = 3 },
		"time":          func(_ *candidate, s *appendSlot) { s.recordedAt = s.recordedAt.Add(time.Nanosecond) },
		"previous hash": func(_ *candidate, s *appendSlot) { s.previousEventHash = bytes.Repeat([]byte{7}, hashBytes) },
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			inputCopy := cloneCandidate(input)
			slotCopy := cloneSlot(slot)
			mutate(&inputCopy, &slotCopy)
			_, got, err := hashEvent(inputCopy, slotCopy)
			if err != nil {
				t.Fatalf("valid mutation failed: %v", err)
			}
			if bytes.Equal(got, baseline) {
				t.Fatal("persisted fact mutation did not change the Event hash")
			}
		})
	}
}

func TestAuditCandidateValidationFailsClosed(t *testing.T) {
	t.Parallel()

	valid := testCandidate()
	zero := int64(0)
	recovery := "recovery-1"
	invalid := map[string]candidate{
		"blank tenant":           withCandidate(valid, func(c *candidate) { c.tenantID = "" }),
		"trimmed tenant":         withCandidate(valid, func(c *candidate) { c.tenantID = " tenant-1" }),
		"unicode trim":           withCandidate(valid, func(c *candidate) { c.requestID = "\u00a0request-1" }),
		"forbidden wildcard":     withCandidate(valid, func(c *candidate) { c.auditEventID = "event?" }),
		"invalid UTF-8":          withCandidate(valid, func(c *candidate) { c.auditEventID = string([]byte{0xff}) }),
		"unknown actor":          withCandidate(valid, func(c *candidate) { c.principal.typeID = 9 }),
		"unknown action":         withCandidate(valid, func(c *candidate) { c.action = "message.read" }),
		"unknown outcome":        withCandidate(valid, func(c *candidate) { c.outcome = "unknown" }),
		"unknown reason":         withCandidate(valid, func(c *candidate) { c.reason = "unknown" }),
		"unknown target":         withCandidate(valid, func(c *candidate) { c.target.typeID = "message" }),
		"nonpositive version":    withCandidate(valid, func(c *candidate) { c.target.version = &zero }),
		"action target mismatch": withCandidate(valid, func(c *candidate) { c.target.typeID = targetOutboxEntry }),
		"unbound recovery":       withCandidate(valid, func(c *candidate) { c.recoveryCaseID = &recovery }),
		"short digest":           withCandidate(valid, func(c *candidate) { c.evidenceDigest = []byte{1} }),
		"replay without evidence": withCandidate(valid, func(c *candidate) {
			version := int64(1)
			c.action = actionOutboxReplayRequest
			c.target = target{typeID: targetOutboxEntry, id: "outbox-1", version: &version}
		}),
	}
	for name, input := range invalid {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if validCandidate(input) {
				t.Fatal("invalid candidate was accepted")
			}
			if _, _, err := hashEvent(input, testSlot()); !hasErrorCode(err, errorInvalidInput) {
				t.Fatalf("hash error = %v, want %q", err, errorInvalidInput)
			}
		})
	}
}

func TestJSONStringEncodingUsesJCSRules(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	writeJSONString(&output, "quote=\" slash=\\ separators=\u2028\u2029 <>&")
	if got, want := output.String(), `"quote=\" slash=\\ separators=   <>&"`; got != want {
		t.Fatalf("JCS string = %q, want %q", got, want)
	}
}

type auditFixture struct {
	Valid struct {
		Events []fixtureEvent `json:"events"`
	} `json:"valid"`
	RejectedMutations []fixtureMutation `json:"rejectedMutations"`
}

type fixtureMutation struct {
	ID        string          `json:"id"`
	Operation string          `json:"operation"`
	Path      string          `json:"path"`
	Value     json.RawMessage `json:"value"`
}

type fixtureEvent struct {
	AuditEventID   string `json:"auditEventId"`
	TenantID       string `json:"tenantId"`
	TenantSequence string `json:"tenantSequence"`
	RecordedAt     string `json:"recordedAt"`
	Principal      struct {
		ActorID   string `json:"actorId"`
		ActorType string `json:"actorType"`
	} `json:"principal"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
	Target  struct {
		TargetType    string  `json:"targetType"`
		TargetID      string  `json:"targetId"`
		TargetVersion *string `json:"targetVersion"`
	} `json:"target"`
	PolicyVersion        string  `json:"policyVersion"`
	RequestID            string  `json:"requestId"`
	ApprovalID           *string `json:"approvalId"`
	RecoveryCaseID       *string `json:"recoveryCaseId"`
	EvidenceDigestHex    *string `json:"evidenceDigestHex"`
	PreviousEventHashHex string  `json:"previousEventHashHex"`
	EventHashHex         string  `json:"eventHashHex"`
}

func loadAuditFixture(t *testing.T) auditFixture {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "test", "fixtures", "audit", "scenarios.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture auditFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func fixtureCandidateAndSlot(t *testing.T, events []fixtureEvent, index int) (candidate, appendSlot) {
	t.Helper()
	value := events[index]
	sequence, err := parsePositiveInt64(value.TenantSequence)
	if err != nil {
		t.Fatal(err)
	}
	actorValue, err := parsePositiveInt64(value.Principal.ActorType)
	if err != nil {
		t.Fatal(err)
	}
	var targetVersion *int64
	if value.Target.TargetVersion != nil {
		parsed, parseErr := parsePositiveInt64(*value.Target.TargetVersion)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		targetVersion = &parsed
	}
	var evidence []byte
	if value.EvidenceDigestHex != nil {
		evidence, err = hex.DecodeString(*value.EvidenceDigestHex)
		if err != nil {
			t.Fatal(err)
		}
	}
	previousHash, err := hex.DecodeString(value.PreviousEventHashHex)
	if err != nil {
		t.Fatal(err)
	}
	recordedAt, err := time.Parse("2006-01-02T15:04:05.000000000Z", value.RecordedAt)
	if err != nil {
		t.Fatal(err)
	}
	var previousID *string
	if index > 0 {
		previous := events[index-1].AuditEventID
		previousID = &previous
	}
	return candidate{
			tenantID: value.TenantID, auditEventID: value.AuditEventID,
			principal: actor{typeID: actorType(actorValue), id: value.Principal.ActorID},
			action:    action(value.Action), outcome: outcome(value.Outcome), reason: reason(value.Reason),
			target:        target{typeID: targetType(value.Target.TargetType), id: value.Target.TargetID, version: targetVersion},
			policyVersion: value.PolicyVersion, requestID: value.RequestID,
			approvalID: cloneString(value.ApprovalID), recoveryCaseID: cloneString(value.RecoveryCaseID),
			evidenceDigest: evidence,
		}, appendSlot{
			tenantID: value.TenantID, tenantSequence: sequence, recordedAt: recordedAt,
			transactionID: 1, previousAuditEventID: previousID, previousEventHash: previousHash,
		}
}

func parsePositiveInt64(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

func testCandidate() candidate {
	return candidate{
		tenantID: "tenant-1", auditEventID: "audit-event-1",
		principal: actor{typeID: actorHuman, id: "actor-1"},
		action:    actionChannelArchive, outcome: outcomeSucceeded, reason: reasonAuthorized,
		target:        target{typeID: targetChannel, id: "channel-1"},
		policyVersion: "policy-1", requestID: "request-1",
	}
}

func testSlot() appendSlot {
	return appendSlot{
		tenantID: "tenant-1", tenantSequence: 1,
		recordedAt: time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC), transactionID: 1,
		previousEventHash: make([]byte, hashBytes),
	}
}

func withCandidate(input candidate, mutate func(*candidate)) candidate {
	input = cloneCandidate(input)
	mutate(&input)
	return input
}

func cloneCandidate(input candidate) candidate {
	input.target.version = cloneInt64(input.target.version)
	input.approvalID = cloneString(input.approvalID)
	input.recoveryCaseID = cloneString(input.recoveryCaseID)
	input.evidenceDigest = bytes.Clone(input.evidenceDigest)
	return input
}

func cloneSlot(input appendSlot) appendSlot {
	input.previousAuditEventID = cloneString(input.previousAuditEventID)
	input.previousEventHash = bytes.Clone(input.previousEventHash)
	return input
}

func hasErrorCode(err error, code errorCode) bool {
	storeErr, ok := err.(*storeFailure)
	return ok && storeErr.category() == code
}
