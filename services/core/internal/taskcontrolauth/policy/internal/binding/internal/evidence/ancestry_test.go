package evidence_test

import (
	"testing"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/evidence"
	fixture "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/testfixture"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func TestEvidenceShapeBoundaries(t *testing.T) {
	scope := model.Scope{Snapshot: fixture.Golden().Snapshot, Tenant: "tenant", Ancestry: fixture.Golden().Ancestry}
	for level := model.LevelTenant; level <= model.LevelApproval; level++ {
		if !evidence.ValidScope(scope, level) || !evidence.CompleteAtLevel(scope.Ancestry.Truncate(level), level) {
			t.Fatalf("complete scope refused at level %d", level)
		}
	}
	for _, level := range []model.Level{5, 99, 255} {
		if evidence.ValidScope(scope, level) || evidence.CompleteAtLevel(scope.Ancestry, level) {
			t.Fatalf("unsupported level %d accepted", level)
		}
	}
	scope.Snapshot.TransactionID = ""
	if evidence.ValidScope(scope, model.LevelTenant) {
		t.Fatal("missing transaction accepted")
	}
	if evidence.CompleteAncestry(model.ResourceRef{Kind: "unknown", ID: "run"}, fixture.Golden().Ancestry) {
		t.Fatal("unknown resource accepted")
	}
	if evidence.CompleteAncestry(model.ResourceRef{Kind: model.ResourceRun, ID: "other"}, fixture.Golden().Ancestry.Truncate(model.LevelRun)) {
		t.Fatal("wrong addressed identity accepted")
	}
}
