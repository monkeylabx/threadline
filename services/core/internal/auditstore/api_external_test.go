package auditstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/monkeylabx/threadline/services/core/internal/auditstore"
)

func TestExportedAuditStoreSurfaceConstructsOnlyValidatedFacts(t *testing.T) {
	t.Parallel()

	principal, err := auditstore.NewPrincipal(auditstore.ActorTypeHuman, "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	version := int64(7)
	target, err := auditstore.NewTarget(auditstore.TargetTypeChannel, "channel-1", &version)
	if err != nil {
		t.Fatal(err)
	}
	version = 9
	returnedVersion := target.Version()
	if returnedVersion == nil || *returnedVersion != 7 {
		t.Fatal("Target retained caller-owned version state")
	}
	*returnedVersion = 11
	if *target.Version() != 7 {
		t.Fatal("Target version accessor exposed mutable internal state")
	}

	candidate, err := auditstore.NewCandidate(auditstore.CandidateInput{
		TenantID: "tenant-1", AuditEventID: "audit-event-1", Principal: principal,
		Action: auditstore.ActionChannelArchive, Outcome: auditstore.OutcomeSucceeded,
		Reason: auditstore.ReasonAuthorized, Target: target,
		PolicyVersion: "policy-1", RequestID: "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = auditstore.Append(context.Background(), nil, candidate)
	var storeErr *auditstore.Error
	if !errors.As(err, &storeErr) || storeErr.Code() != auditstore.ErrorCodeInvalidInput {
		t.Fatalf("Append error = %v, want exported invalid-input category", err)
	}
}

func TestExportedConstructorsRejectUnknownFacts(t *testing.T) {
	t.Parallel()

	if _, err := auditstore.NewPrincipal(99, "actor-1"); err == nil {
		t.Fatal("unknown Actor type was accepted")
	}
	if _, err := auditstore.NewTarget("message", "message-1", nil); err == nil {
		t.Fatal("unknown Target type was accepted")
	}
}
