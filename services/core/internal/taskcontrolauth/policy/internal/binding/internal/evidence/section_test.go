package evidence_test

import (
	"testing"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	fixture "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/testfixture"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func TestPublicSectionRejectsIncompleteEvidence(t *testing.T) {
	golden := fixture.Golden()
	scope := model.Scope{Snapshot: golden.Snapshot, Tenant: "tenant", Ancestry: golden.Ancestry}
	noTenant, noAncestors := scope, scope
	noTenant.Tenant, noAncestors.Ancestry = "", model.Ancestry{}
	cases := []struct {
		scope model.Scope
		level model.Level
	}{
		{noTenant, model.LevelTenant}, {noAncestors, model.LevelApproval},
		{scope, 99}, {scope, 255},
	}
	for _, tc := range cases {
		bound := model.Bound{Snapshot: tc.scope.Snapshot, Tenant: tc.scope.Tenant, Ancestry: tc.scope.Ancestry.Truncate(tc.level)}
		fixture.AssertRefusal(t, binding.Section(tc.scope, bound, tc.level), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	for field := 0; field < 4; field++ {
		broken := scope
		switch field {
		case 0:
			broken.Ancestry.Channel = "bad\nchannel"
		case 1:
			broken.Ancestry.Task = "bad\ntask"
		case 2:
			broken.Ancestry.Run = "bad\nrun"
		case 3:
			broken.Ancestry.Approval = "bad\napproval"
		}
		bound := model.Bound{Snapshot: broken.Snapshot, Tenant: broken.Tenant, Ancestry: broken.Ancestry}
		fixture.AssertRefusal(t, binding.Section(broken, bound, model.LevelApproval), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
}
