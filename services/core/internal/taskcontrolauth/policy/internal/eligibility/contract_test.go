package eligibility_test

import (
	"encoding/json"
	"os"
	"testing"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/eligibility"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

type scenarioMut func(*scenarioEnv)
type fixtureCase struct {
	ID, When, Action string
	Then             struct{ Decision, Reason, Status, Code string }
}

func ownMut(caller model.Actor, flags uint8, dep string) scenarioMut {
	return func(e *scenarioEnv) {
		e.req.Caller, e.f.Policy.Deployment, e.f.Policy.AllowOwnerSelfApproval, e.f.Policy.ConfiguredOwnerApprover = caller, dep, flags&1 != 0, flags&2 != 0
		roles := map[bool][]model.TaskRole{false: {ceiling.Approver}, true: {ceiling.Approver, ceiling.Owner}}[flags&4 != 0]
		e.f.Policy.Rules[ceiling.ApprovalDecide], e.f.Policy.Rules[ceiling.ApprovalRevoke] = model.PolicyRule{Roles: roles}, model.PolicyRule{Roles: roles}
		e.st.Snapshot.Eligible = append(e.st.Snapshot.Eligible, eligibility.EligibleApprover{Actor: humanOwner, TaskRole: ceiling.Owner})
		e.st.Decider = humanOwner
	}
}
func actMut(act model.Action, fn scenarioMut) scenarioMut {
	return func(e *scenarioEnv) { e.req.Action = act; fn(e) }
}

func scenarioMutators() map[string]scenarioMut {
	noop := func(*scenarioEnv) {}
	m := map[string]scenarioMut{
		"allow-approval-request": noop, "disabled-approval-request": func(e *scenarioEnv) { delete(e.f.Policy.Rules, ceiling.ApprovalRequest) }, "empty-eligible-no-pending-action": func(e *scenarioEnv) { e.f.Policy.Rules[ceiling.ApprovalDecide] = model.PolicyRule{} }, "allow-approval-decide": noop, "disabled-approval-decide": func(e *scenarioEnv) { delete(e.f.Policy.Rules, ceiling.ApprovalDecide) }, "allow-approval-revoke": actMut(ceiling.ApprovalRevoke, noop),
		"disabled-approval-revoke": actMut(ceiling.ApprovalRevoke, func(e *scenarioEnv) { delete(e.f.Policy.Rules, ceiling.ApprovalRevoke) }), "owner-self-explicit-dev": ownMut(humanOwner, 7, model.DeploymentDedicatedDev), "owner-self-production": ownMut(humanOwner, 7, "production"), "owner-self-not-configured": ownMut(humanOwner, 5, model.DeploymentDedicatedDev), "owner-self-role-not-enabled": ownMut(humanOwner, 3, model.DeploymentDedicatedDev), "owner-self-renderer-flag-only": ownMut(humanOwner, 6, model.DeploymentDedicatedDev),
		"eligible-rejection-no-grant": func(e *scenarioEnv) { e.req.Approved = false }, "archived-eligible-rejection": func(e *scenarioEnv) { e.f.Parent.State, e.req.Approved = standing.ResourceStateArchived, false }, "archived-eligible-revoke": actMut(ceiling.ApprovalRevoke, func(e *scenarioEnv) { e.f.Parent.State = standing.ResourceStateArchived }), "observer-cannot-reject": func(e *scenarioEnv) { e.req.Caller, e.req.Approved = humanObs, false }, "archived-new-approval-denied": func(e *scenarioEnv) { e.f.Parent.State = standing.ResourceStateArchived }, "time-bounded-decision-unsupported": func(e *scenarioEnv) { e.req.ApprovalScope = "TIME_BOUNDED" },
		"use-approver-facts-unavailable": func(e *scenarioEnv) { delete(e.f.Candidates, humanAppA) }, "use-approver-removed": func(e *scenarioEnv) { e.f.Policy.Rules[ceiling.ApprovalDecide] = model.PolicyRule{} }, "use-owner-self-disabled": ownMut(humanAppA, 6, model.DeploymentDedicatedDev), "use-empty-resource-scope": func(e *scenarioEnv) { e.st.Action.Scope = grantpolicy.ResourceScope{} }, "use-grant-expiry-boundary": func(e *scenarioEnv) { e.st.ExpiresAt = e.f.ObservedAt }, "use-policy-restored-but-run-blocked": func(e *scenarioEnv) { e.f.Policy.Revision, e.st.RunBlocked = "9", true },
		"use-cancel-before-consume": func(e *scenarioEnv) { e.st.CancelIntent = true }, "use-barrier-present": func(e *scenarioEnv) { e.st.SafetyBarrier = true }, "use-once-already-consumed": func(e *scenarioEnv) { e.st.Status = eligibility.ApprovalConsumed },
	}
	for _, a := range []model.Action{ceiling.ApprovalDecide, ceiling.ApprovalRevoke} {
		pfx := map[model.Action]string{ceiling.ApprovalDecide: "approval-decide-", ceiling.ApprovalRevoke: "approval-revoke-"}[a]
		m[pfx+"observer"], m[pfx+"owner-self-default-off"] = actMut(a, func(e *scenarioEnv) { e.req.Caller = humanObs }), actMut(a, func(e *scenarioEnv) { e.req.Caller = humanOwner })
		m[pfx+"not-in-snapshot"], m[pfx+"current-policy-removed"] = actMut(a, func(e *scenarioEnv) { e.st.Snapshot.Eligible = e.st.Snapshot.Eligible[1:] }), actMut(a, func(e *scenarioEnv) { e.f.Policy.Rules[a] = model.PolicyRule{} })
		m[pfx+"new-approver-outside-snapshot"], m[pfx+"resource-authority-lost"] = actMut(a, func(e *scenarioEnv) { e.f.Policy.Revision, e.st.Snapshot.Eligible = "8", e.st.Snapshot.Eligible[1:] }), actMut(a, cMut(func(c *eligibility.CandidateFacts) { c.Scope = grantpolicy.ResourceScope{} }))
		m[pfx+"guest-approver"] = actMut(a, cMut(func(c *eligibility.CandidateFacts) { c.Member.Role = standing.OrganizationRoleGuest }))
	}
	return m
}

func assertScenarioResult(t *testing.T, c fixtureCase, res eligibility.Result, isUse bool) {
	t.Helper()
	switch {
	case c.Then.Decision == "allow" && !res.Refusal.Refused():
	case isUse && res.Refusal.Refused() && res.Refusal.Status == model.Status(c.Then.Status) && res.Refusal.Code == model.Code(c.Then.Code):
	case !isUse && c.Then.Decision != "allow" && res.Refusal == (model.Refusal{Reason: model.Reason(c.Then.Reason), Status: model.Status(c.Then.Status), Code: model.Code(c.Then.Code)}):
	default:
		t.Fatalf("case %s got %+v want %+v", c.ID, res.Refusal, c.Then)
	}
}

func TestMappedEligibilityContractScenarios(t *testing.T) {
	data, err := os.ReadFile("../../../../../../../test/fixtures/proto/task-control-authorization/scenarios.json")
	var sf struct{ Cases []fixtureCase }
	if err != nil || json.Unmarshal(data, &sf) != nil {
		t.Fatal(err)
	}
	byID := make(map[string]fixtureCase, len(sf.Cases))
	for _, c := range sf.Cases {
		byID[c.ID] = c
	}
	for id, mut := range scenarioMutators() {
		c, ok := byID[id]
		if !ok {
			t.Fatalf("missing fixture case %s", id)
		}
		kind := "dec"
		if c.Action == string(ceiling.ApprovalRequest) {
			kind = "req"
		} else if c.When == "AdmitAction.current-authority" {
			kind = "use"
		}
		e := baseEnv(kind)
		mut(&e)
		res, err := callKind(kind, e)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		assertScenarioResult(t, c, res, kind == "use")
	}
}
