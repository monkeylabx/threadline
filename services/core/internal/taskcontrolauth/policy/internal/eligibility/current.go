package eligibility

import (
	"context"
	"slices"
	"strings"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

// Current computes the pure current eligible Human intersection bound to the
// immutable Approval action and snapshot. It authorizes no caller or write.
func Current(ctx context.Context, act ActionBinding, f EligibilityFacts) (Result, error) {
	return evalCurrent(ctx, act, f, false)
}

// ValidateRequest computes the snapshot-bound eligible Human intersection at
// RequestApproval and rejects empty or policy-disabled sets.
func ValidateRequest(ctx context.Context, act ActionBinding, f EligibilityFacts) (Result, error) {
	return evalCurrent(ctx, act, f, true)
}

func evalCurrent(ctx context.Context, act ActionBinding, f EligibilityFacts, requireReq bool) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if r := firstRef(validateCommonFacts(f, act), validateProvidedCandidates(f)); r.Refused() {
		return Result{Refusal: r}, nil
	}
	if requireReq {
		_, hasReq := f.Policy.Rules[ceiling.ApprovalRequest]
		if r := firstRef(
			cond(!hasReq, refuse(ReasonActionDisabled, model.StatusPermissionDenied, model.CodePermissionDenied)),
			cond(!authorityCovers(f.PolicyCeiling.Capabilities, f.PolicyCeiling.Scope, act.Capabilities, act.Scope), refuse(ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope)),
		); r.Refused() {
			return Result{Refusal: r}, nil
		}
	}
	eligible, usedOwner, r := computeEligible(f, act)
	if r.Refused() {
		return Result{Refusal: r}, nil
	}
	if requireReq && len(eligible) == 0 {
		return Result{Refusal: refuse(ReasonApproverCurrentDenied, model.StatusPermissionDenied, model.CodePermissionDenied)}, nil
	}
	snap := Snapshot{Bound: model.Bound{Snapshot: f.Scope.Snapshot, Tenant: f.Scope.Tenant, Ancestry: f.Scope.Ancestry}, ActionID: act.ActionID, ActionVersion: act.ActionVersion, Commitment: act.Commitment, PolicyRevision: f.Policy.Revision, Eligible: append([]EligibleApprover(nil), eligible...)}
	return Result{Scope: f.Scope, PolicyRevision: f.Policy.Revision, Eligible: eligible, Snapshot: snap, UsedOwnerSelfApproval: usedOwner}, nil
}

func validateProvidedCandidates(f EligibilityFacts) model.Refusal {
	for actor, cand := range f.Candidates {
		if r := validateCandidateBinding(f, actor, cand); r.Refused() {
			return r
		}
	}
	return model.Refusal{}
}

func computeEligible(f EligibilityFacts, act ActionBinding) ([]EligibleApprover, bool, model.Refusal) {
	rule := f.Policy.Rules[ceiling.ApprovalDecide]
	if !authorityCovers(f.PolicyCeiling.Capabilities, f.PolicyCeiling.Scope, act.Capabilities, act.Scope) {
		return nil, false, model.Refusal{}
	}
	var out []EligibleApprover
	usedOwner := false
	ownerSelf := f.Policy.Deployment == model.DeploymentDedicatedDev && f.Policy.AllowOwnerSelfApproval && f.Policy.ConfiguredOwnerApprover && slices.Contains(rule.Roles, ceiling.Owner)
	for _, part := range f.Task.Participants {
		if !(part.Role == ceiling.Approver && slices.Contains(rule.Roles, ceiling.Approver)) && !(part.Role == ceiling.Owner && ownerSelf) {
			continue
		}
		if _, ok := f.Candidates[part.Actor]; !ok {
			return nil, false, refUnav
		}
		if app, r := evaluateActor(ceiling.ApprovalDecide, part.Actor, act, nil, f); !r.Refused() {
			usedOwner = usedOwner || part.Role == ceiling.Owner
			out = append(out, app)
		}
	}
	sortEligible(out)
	return out, usedOwner, model.Refusal{}
}

func sortEligible(out []EligibleApprover) {
	slices.SortFunc(out, func(a, b EligibleApprover) int { return strings.Compare(a.Actor.ID, b.Actor.ID) })
}

func evaluateActor(action model.Action, actor model.Actor, act ActionBinding, snap *Snapshot, f EligibilityFacts) (EligibleApprover, model.Refusal) {
	cand, ok := f.Candidates[actor]
	if !ok {
		return EligibleApprover{}, refUnav
	}
	taskRole := ceiling.None
	if i := slices.IndexFunc(f.Task.Participants, func(p model.Participant) bool { return p.Actor == actor }); i >= 0 {
		taskRole = f.Task.Participants[i].Role
	}
	if r := firstRef(
		evaluateCandidateParent(f, actor, cand),
		cond(cand.ActorDenied, refuse(ReasonPolicyActorDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
		evaluateCandidateRole(action, f.Policy, taskRole, cand.Member.Role, cand.Membership.Role),
		cond(snap != nil && !slices.ContainsFunc(snap.Eligible, func(e EligibleApprover) bool { return e.Actor == actor && e.TaskRole == taskRole }), refuse(ReasonApproverSnapshotDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(!slices.Contains(f.Policy.Rules[action].Roles, taskRole), refuse(ReasonApproverCurrentDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(!cand.SourceEntitled, refuse(ReasonSourceHistoryDenied, model.StatusPermissionDenied, model.CodeHistorySharingDenied)),
		cond(!authorityCovers(f.PolicyCeiling.Capabilities, f.PolicyCeiling.Scope, act.Capabilities, act.Scope) ||
			!authorityCovers(cand.Capabilities, cand.Scope, act.Capabilities, act.Scope), refuse(ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope)),
	); r.Refused() {
		return EligibleApprover{}, r
	}
	return EligibleApprover{Actor: cand.Member.Actor, TaskRole: taskRole, OrganizationRole: cand.Member.Role, ChannelRole: cand.Membership.Role}, model.Refusal{}
}

func evaluateCandidateParent(f EligibilityFacts, actor model.Actor, cand CandidateFacts) model.Refusal {
	idx := slices.IndexFunc(f.Parent.Decisions, func(d standing.Decision) bool { return d.Actor == actor && d.Action == standing.ActionChannelRead })
	return firstRef(
		cond((cand.RequireDeviceCurrent || cand.Device.Bound.Snapshot.TransactionID != "") && !cand.Device.Current, refuse(binding.ReasonDeviceRevoked, model.StatusPermissionDenied, model.CodeDeviceRevoked)),
		cond(!cand.Member.Active, refuse(binding.ReasonMemberInactive, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(!cand.Membership.Current || cand.Membership.IntervalID == "", refuse(binding.ReasonNotAMember, model.StatusPermissionDenied, model.CodeNotAMember)),
		cond(idx < 0 || f.Parent.Decisions[idx].Effect != standing.EffectAllow, refuse(binding.ReasonParentReadDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(!aclAllowsRead(f.Parent.ACL, actor), refuse(binding.ReasonParentACLDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
	)
}

func evaluateCandidateRole(action model.Action, pol model.PolicyFacts, taskRole model.TaskRole, orgRole standing.OrganizationRole, chanRole standing.ChannelRole) model.Refusal {
	state := ceiling.RoleState{Route: ceiling.Human, TaskRole: taskRole, OrganizationRole: orgRole, ChannelRole: chanRole, SelfApproval: ceiling.SelfApprovalPolicy{DeploymentClass: pol.Deployment, AllowOwner: pol.AllowOwnerSelfApproval, ConfiguredOwner: pol.ConfiguredOwnerApprover, OwnerRoleEnabled: slices.Contains(pol.Rules[action].Roles, ceiling.Owner)}}
	return firstRef(
		cond(orgRole == standing.OrganizationRoleGuest, refuse(ReasonOrgTaskCeiling, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(chanRole == standing.ChannelRoleGuest, refuse(ReasonChannelTaskCeiling, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(taskRole == ceiling.Owner && !ceiling.WithinRoleCeiling(action, state), refuse(ReasonSelfApprovalDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(taskRole != ceiling.Approver && taskRole != ceiling.Owner, refuse(ReasonApproverRoleDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
	)
}

func aclAllowsRead(acl standing.ResourceACLFacts, actor model.Actor) bool {
	allowed := acl.DefaultEffect == standing.ACLEffectAllow
	for _, e := range acl.Entries {
		if e.Actor == actor && e.Action == standing.ActionChannelRead {
			if e.Effect == standing.ACLEffectDeny {
				return false
			}
			allowed = true
		}
	}
	return allowed
}
