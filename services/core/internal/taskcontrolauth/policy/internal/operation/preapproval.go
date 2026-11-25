package operation

import (
	"slices"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

const maxTaskParticipants = 16

var (
	readOps       = map[model.Action]bool{ceiling.TaskRead: true, ceiling.RunRead: true, ceiling.ApprovalRead: true}
	runtimeOps    = map[model.Action]bool{ceiling.RunRead: true, ceiling.ApprovalRequest: true, ceiling.RunReport: true}
	sourceOps     = map[model.Action]bool{ceiling.TaskCreate: true, ceiling.RunStart: true, ceiling.TaskRead: true, ceiling.ApprovalRead: true}
	scopeOps      = map[model.Action]bool{ceiling.TaskCreate: true, ceiling.RunStart: true, ceiling.ApprovalRequest: true}
	privilegedOps = map[model.Action]bool{ceiling.TaskCreate: true, ceiling.RunStart: true, ceiling.ApprovalDecide: true, ceiling.ApprovalRevoke: true, ceiling.ApprovalRequest: true, ceiling.RunReport: true}
	orgRoles      = map[standing.OrganizationRole]bool{standing.OrganizationRoleOwner: true, standing.OrganizationRoleAdmin: true, standing.OrganizationRoleSecurityAdmin: true, standing.OrganizationRoleMember: true, standing.OrganizationRoleGuest: true}
)

func refuse(reason model.Reason, status model.Status, code model.Code) model.Refusal {
	return model.Refusal{Reason: reason, Status: status, Code: code}
}

type evalState struct {
	p       model.Principal
	f       model.Facts
	res     binding.Result
	opFacts Facts
	op      model.Action
	rule    model.PolicyRule
}

var preApprovalSteps = []func(*evalState) model.Refusal{
	checkParentState, checkPolicyRule, checkParentPublish, checkRoute,
	checkRoleTiers, checkTaskRoleRule, checkSource, checkTarget,
	checkCreateRequest, checkDelegation, checkScopeRequest,
}

// Validate evaluates all non-approval operation guards and Runtime guards.
func Validate(p model.Principal, f model.Facts, res binding.Result, opFacts Facts) model.Refusal {
	if r := ValidatePreApproval(p, f, res, opFacts); r.Refused() {
		return r
	}
	if p.Route == ceiling.AssignedRuntime {
		return ValidateRuntime(p, f, res, opFacts)
	}
	return model.Refusal{}
}

// ValidatePreApproval enforces non-approval policy, source, target, request,
// and delegation guards over bound facts.
func ValidatePreApproval(p model.Principal, f model.Facts, res binding.Result, opFacts Facts) model.Refusal {
	if res.Refusal.Refused() {
		return res.Refusal
	}
	if res.Scope.Tenant == "" || res.EffectiveAction == "" {
		return refuse(binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	s := &evalState{p: p, f: f, res: res, opFacts: opFacts, op: res.EffectiveAction}
	for _, step := range preApprovalSteps {
		if r := step(s); r.Refused() {
			return r
		}
	}
	return model.Refusal{}
}

func checkParentState(s *evalState) model.Refusal {
	if s.f.Parent.State == standing.ResourceStateActive {
		return model.Refusal{}
	}
	activeOp := s.op == ceiling.TaskCreate || s.op == ceiling.RunStart || s.op == ceiling.ApprovalRequest || s.op == ceiling.RunReport || (s.op == ceiling.ApprovalDecide && s.opFacts.Approved)
	if s.f.Parent.State == standing.ResourceStateArchived && !activeOp {
		return model.Refusal{}
	}
	return refuse(ReasonParentStateDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
}

func checkPolicyRule(s *evalState) model.Refusal {
	rule, ok := s.f.Policy.Rules[s.op]
	if !ok {
		return refuse(ReasonActionDisabled, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	audOK := slices.Contains(rule.Audiences, model.ReadChannelReaders) || (s.p.Route == ceiling.AssignedRuntime && s.op == ceiling.RunRead && slices.Contains(rule.Audiences, model.ReadAssignedRuntimeMetadata))
	if readOps[s.op] && !audOK {
		return refuse(ReasonPolicyAudienceDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	if s.f.Policy.ActorDenied {
		return refuse(ReasonPolicyActorDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	s.rule = rule
	return model.Refusal{}
}

func checkParentPublish(s *evalState) model.Refusal {
	parents, ok := ceiling.RequiredParentActions(s.op)
	if !ok || !slices.Contains(parents, standing.ActionChannelPublish) {
		return model.Refusal{}
	}
	for _, d := range s.f.Parent.Decisions {
		if d.Actor != s.p.Actor || d.Action != standing.ActionChannelPublish {
			continue
		}
		if d.Effect != standing.EffectAllow {
			return refuse(ReasonParentPublishDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
		}
		if !aclAllowsPublish(s.f.Parent.ACL, s.p.Actor) {
			return refuse(binding.ReasonParentACLDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
		}
		return model.Refusal{}
	}
	return refuse(binding.ReasonFactsUnavailable, model.StatusUnavailable, "")
}

func aclAllowsPublish(acl standing.ResourceACLFacts, actor model.Actor) bool {
	matchedAllow := false
	for _, e := range acl.Entries {
		if e.Actor == actor && e.Action == standing.ActionChannelPublish {
			if e.Effect == standing.ACLEffectDeny {
				return false
			}
			matchedAllow = true
		}
	}
	return matchedAllow || acl.DefaultEffect == standing.ACLEffectAllow
}

func checkRoute(s *evalState) model.Refusal {
	if s.p.Route == ceiling.AssignedRuntime && runtimeOps[s.op] {
		if r := checkRuntimeBinding(s.res.Scope, s.opFacts.Runtime); r.Refused() {
			return r
		}
		if s.opFacts.Runtime.Verified && s.res.TaskRole == ceiling.Assignee {
			return model.Refusal{}
		}
	}
	if s.p.Route == ceiling.Human && s.op != ceiling.ApprovalRequest && s.op != ceiling.RunReport {
		return model.Refusal{}
	}
	return refuse(ReasonPrincipalRouteDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
}

func checkRoleTiers(s *evalState) model.Refusal {
	if !privilegedOps[s.op] {
		return model.Refusal{}
	}
	if s.f.Member.Role == standing.OrganizationRoleGuest || (s.p.Route == ceiling.AssignedRuntime && s.opFacts.Runtime.OperatorOrgRole == standing.OrganizationRoleGuest) {
		return refuse(ReasonOrganizationCeiling, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	if s.f.Parent.Membership.Role == standing.ChannelRoleGuest {
		return refuse(ReasonChannelCeiling, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	return model.Refusal{}
}

func checkTaskRoleRule(s *evalState) model.Refusal {
	role := s.res.TaskRole
	if s.op == ceiling.RunStart && (role != ceiling.Owner || !slices.Contains(s.rule.Roles, ceiling.Owner)) {
		return refuse(ReasonTaskRoleDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	controlRole := role == ceiling.Owner || role == ceiling.Observer || role == ceiling.Approver
	if s.op == ceiling.RunControl && (!controlRole || !slices.Contains(s.rule.Roles, role)) {
		return refuse(ReasonTaskRoleDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	return model.Refusal{}
}
