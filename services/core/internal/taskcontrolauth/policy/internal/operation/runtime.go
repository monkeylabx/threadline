package operation

import (
	"errors"
	"slices"
	"time"

	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func checkDelegation(s *evalState) model.Refusal {
	if s.op != ceiling.TaskCreate && s.op != ceiling.RunStart && s.p.Route != ceiling.AssignedRuntime {
		return model.Refusal{}
	}
	level := model.LevelRun
	if s.op == ceiling.TaskCreate {
		level = model.LevelChannel
	} else if s.op == ceiling.RunStart {
		level = model.LevelTask
	}
	d := s.opFacts.Delegation
	if r := binding.Section(s.res.Scope, d.Bound, level); r.Refused() {
		return r
	}
	wantInit, wantAgent := s.p.Actor, s.opFacts.Target.Agent
	if s.p.Route == ceiling.AssignedRuntime {
		wantInit, wantAgent = s.f.Task.Creator, s.p.Actor
	}
	if !delegationValid(s.f.ObservedAt, d, wantInit, wantAgent) {
		return refuse(ReasonDelegationDenied, model.StatusPermissionDenied, model.CodeGrantOutOfScope)
	}
	return model.Refusal{}
}

func delegationValid(observedAt time.Time, d DelegationFacts, wantInit, wantAgent model.Actor) bool {
	if !d.Current || !d.InitiatorAuthorityCurrent || d.Initiator != wantInit || d.Agent != wantAgent {
		return false
	}
	if (d.SessionBound && !d.SessionCurrent) || (d.DeviceBound && !d.DeviceCurrent) {
		return false
	}
	return !d.ExpiresAt.IsZero() && observedAt.Before(d.ExpiresAt)
}

func checkScopeRequest(s *evalState) model.Refusal {
	if !scopeOps[s.op] {
		return model.Refusal{}
	}
	sr := s.opFacts.ScopeRequest
	if hasUnsupportedToolOrScope(s.op, sr, s.opFacts.Delegation.ConfiguredSyntheticToolID) {
		return refuse(ReasonUnsupportedTool, model.StatusUnimplemented, "")
	}
	if sr.UnknownFields || len(sr.Capabilities) == 0 {
		return refuse(binding.ReasonShapeInvalid, model.StatusInvalidArgument, "")
	}
	if len(sr.Scope.ChannelIDs)+len(sr.Scope.EventIDs)+len(sr.Scope.ThreadIDs)+len(sr.Scope.ToolIDs) == 0 {
		return refuse(ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope)
	}
	return checkCeilingsFit(s, sr)
}

func checkCeilingsFit(s *evalState, sr ScopeRequest) model.Refusal {
	d := s.opFacts.Delegation
	req := grantpolicy.Request{Capabilities: sr.Capabilities, Scope: sr.Scope, ExpiresAt: sr.ExpiresAt}
	taskID, runID := nonEmptyOr(s.res.Scope.Ancestry.Task, "pending-task"), nonEmptyOr(s.res.Scope.Ancestry.Run, "pending-run")
	auth := grantpolicy.IssuanceAuthority{
		TenantID: s.res.Scope.Tenant, TaskID: taskID, RunID: runID, DeviceID: s.p.Device, PolicyVersion: s.f.Policy.Revision,
		Grantee:   grantpolicy.ActorRef{Type: grantpolicy.ActorType(d.Agent.Type), ID: d.Agent.ID},
		Initiator: grantpolicy.ActorRef{Type: grantpolicy.ActorType(d.Initiator.Type), ID: d.Initiator.ID},
	}
	for _, c := range [4]AuthorityCeiling{d.PolicyCeiling, d.InitiatorCeiling, d.AgentCeiling, d.ResourceCeiling} {
		auth.Capabilities, auth.Scope, auth.NotAfter = c.Capabilities, c.Scope, c.NotAfter
		if _, err := grantpolicy.ValidateAndNarrow(s.f.ObservedAt, req, auth); err != nil {
			return mapGrantPolicyError(err)
		}
	}
	return model.Refusal{}
}

func mapGrantPolicyError(err error) model.Refusal {
	var ge *grantpolicy.Error
	if errors.As(err, &ge) {
		switch ge.Code() {
		case grantpolicy.ErrorInvalidAuthority:
			return refuse(binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
		case grantpolicy.ErrorInvalidInput:
			return refuse(binding.ReasonShapeInvalid, model.StatusInvalidArgument, "")
		}
	}
	return refuse(ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope)
}

func nonEmptyOr(val, fallback string) string {
	if val != "" {
		return val
	}
	return fallback
}

var unsupportedCaps = map[grantpolicy.Capability]bool{
	grantpolicy.CapabilityFileRead: true, grantpolicy.CapabilityFileWrite: true,
	grantpolicy.CapabilityWorkspaceRead: true, grantpolicy.CapabilityWorkspaceWrite: true,
}

func hasUnsupportedToolOrScope(op model.Action, sr ScopeRequest, syntheticToolID string) bool {
	if slices.ContainsFunc(sr.Capabilities, func(c grantpolicy.Capability) bool { return unsupportedCaps[c] }) {
		return true
	}
	sc := sr.Scope
	if len(sc.DMIDs)+len(sc.FileIDs)+len(sc.WorkspaceBindingIDs)+len(sc.WorkspacePathPrefixes) > 0 || !validAdapter(op, sr) {
		return true
	}
	return slices.ContainsFunc(sc.ToolIDs, func(id string) bool { return syntheticToolID == "" || id != syntheticToolID })
}

func validAdapter(op model.Action, sr ScopeRequest) bool {
	if sr.ActionAdapter != "" && sr.ActionAdapter != "noop/1" {
		return false
	}
	return op != ceiling.ApprovalRequest || (sr.ActionAdapter == "noop/1" && sr.ArgumentsEmpty)
}

// ValidateRuntime enforces Runtime membership, assignment, owner/lease/fence,
// and execution Grant guards for AssignedRuntime operations.
func ValidateRuntime(p model.Principal, f model.Facts, res binding.Result, opFacts Facts) model.Refusal {
	if res.Refusal.Refused() {
		return res.Refusal
	}
	if p.Route != ceiling.AssignedRuntime {
		return refuse(ReasonPrincipalRouteDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	if r := checkRuntimeBinding(res.Scope, opFacts.Runtime); r.Refused() {
		return r
	}
	if r := checkRuntimeState(p, res.Scope, f.Task, opFacts.Runtime); r.Refused() {
		return r
	}
	return checkGrantFacts(res.Scope, f.ObservedAt, opFacts.Grant)
}

func checkRuntimeBinding(scope model.Scope, rt RuntimeFacts) model.Refusal {
	if r := binding.Section(scope, rt.Bound, model.LevelRun); r.Refused() {
		return r
	}
	if !orgRoles[rt.OperatorOrgRole] {
		return refuse(binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	return model.Refusal{}
}

func checkRuntimeState(p model.Principal, scope model.Scope, task model.TaskFacts, rt RuntimeFacts) model.Refusal {
	if !rt.AgentMemberCurrent || !rt.OperatorMemberCurrent {
		return refuse(binding.ReasonMemberInactive, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	assignee, ok := findAssignee(task.Participants)
	if !ok || rt.AssignedAgent != p.Actor || rt.AssignedAgent != assignee {
		return refuse(ReasonTargetBindingDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	if !validRuntimeOwner(p, scope, rt) {
		return refuse(ReasonNotExecutionOwner, model.StatusFailedPrecondition, model.CodeNotExecutionOwner)
	}
	if !rt.LeaseCurrent {
		return refuse(ReasonLeaseLost, model.StatusFailedPrecondition, model.CodeLeaseLost)
	}
	if !rt.FenceCurrent {
		return refuse(ReasonFenceStale, model.StatusFailedPrecondition, model.CodeFencingTokenStale)
	}
	return model.Refusal{}
}

func validRuntimeOwner(p model.Principal, scope model.Scope, rt RuntimeFacts) bool {
	return rt.AssignedRun == scope.Ancestry.Run && rt.ExecutionOwner == p.Enrollment && rt.RuntimeGeneration == p.RuntimeGeneration
}

func checkGrantFacts(scope model.Scope, observedAt time.Time, g GrantFacts) model.Refusal {
	if r := binding.Section(scope, g.Bound, model.LevelRun); r.Refused() {
		return r
	}
	if !g.Present {
		return refuse(ReasonExecutionGrantRequired, model.StatusPermissionDenied, model.CodeApprovalRequired)
	}
	if g.State == GrantStateExpired {
		return refuse(ReasonGrantExpired, model.StatusPermissionDenied, model.CodeGrantExpired)
	}
	if g.State != GrantStateActive {
		return refuse(ReasonGrantRevoked, model.StatusPermissionDenied, model.CodeGrantRevoked)
	}
	if !g.SignatureValid {
		return refuse(ReasonGrantSignatureInvalid, model.StatusPermissionDenied, model.CodeGrantOutOfScope)
	}
	if g.ExpiresAt.IsZero() {
		return refuse(binding.ReasonFactsUnavailable, model.StatusUnavailable, "")
	}
	if !observedAt.Before(g.ExpiresAt) {
		return refuse(ReasonGrantExpired, model.StatusPermissionDenied, model.CodeGrantExpired)
	}
	return model.Refusal{}
}
