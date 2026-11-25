package eligibility

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

var (
	refUnav, refBind = refuse(binding.ReasonFactsUnavailable, model.StatusUnavailable, ""), refuse(binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	validOrgRoles    = []standing.OrganizationRole{standing.OrganizationRoleOwner, standing.OrganizationRoleAdmin, standing.OrganizationRoleSecurityAdmin, standing.OrganizationRoleMember, standing.OrganizationRoleGuest}
	validChRoles     = []standing.ChannelRole{standing.ChannelRoleNone, standing.ChannelRoleOwner, standing.ChannelRoleModerator, standing.ChannelRoleMember, standing.ChannelRoleGuest}
)

func validateCommonFacts(f EligibilityFacts, act ActionBinding) model.Refusal {
	ch := standing.ResourceRef{TenantID: f.Scope.Tenant, Kind: standing.ResourceKindChannel, ID: f.Scope.Ancestry.Channel}
	tScope := model.Scope{Snapshot: f.Scope.Snapshot, Tenant: f.Scope.Tenant}
	okEff := func(e standing.ACLEffect) bool { return e == standing.ACLEffectAllow || e == standing.ACLEffectDeny }
	aclOK := f.Parent.ACL.Resource == ch && okEff(f.Parent.ACL.DefaultEffect) && !slices.ContainsFunc(f.Parent.ACL.Entries, func(e standing.ACLEntry) bool { return !okEff(e.Effect) })
	return firstRef(
		cond(f.ObservedAt.IsZero() || slices.Contains([]string{f.Scope.Snapshot.TransactionID, f.Scope.Tenant, f.Scope.Ancestry.Channel, f.Scope.Ancestry.Task, f.Scope.Ancestry.Run}, ""), refUnav),
		binding.Section(tScope, f.Organization.Bound, model.LevelTenant),
		cond(f.Organization.PolicyVersion == "" || !f.PolicyCeiling.Available, refUnav),
		cond(!f.Organization.Active, refuse(binding.ReasonOrganizationInactive, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(!validAuth(f.PolicyCeiling.Capabilities, f.PolicyCeiling.Scope, true), refBind),
		binding.Section(f.Scope, f.Parent.Bound, model.LevelChannel),
		cond(f.Parent.ACL.Version == "", refUnav),
		cond(!aclOK || !validDecisions(f.Parent.Decisions, ch, f.Parent.ACL.Version, f.Organization.PolicyVersion), refBind),
		cond(f.Policy.Snapshot.TransactionID == "", refuse(binding.ReasonPolicyUnavailable, model.StatusUnavailable, "")),
		binding.Section(f.Scope, f.Policy.Bound, model.LevelChannel),
		cond(f.Policy.Profile != model.PolicyProfile || f.Policy.Revision == "", refuse(binding.ReasonPolicyInvalid, model.StatusUnavailable, "")),
		binding.Section(f.Scope, f.Task.Bound, model.LevelTask),
		cond(!validParticipants(f.Task), refBind),
		validateActionBinding(f.Scope, act),
	)
}

func validateApprovalMode(m ApprovalScope) model.Refusal {
	return firstRef(
		cond(m == "", refuse(binding.ReasonShapeInvalid, model.StatusInvalidArgument, "")),
		cond(m != ApprovalScopeOnce, refuse(ReasonUnsupportedMode, model.StatusUnimplemented, "")),
	)
}

func validateActionBinding(scope model.Scope, act ActionBinding) model.Refusal {
	lv := model.LevelRun
	if scope.Ancestry.Approval != "" {
		lv = model.LevelApproval
	}
	return firstRef(
		binding.Section(scope, act.Bound, lv),
		cond(slices.Contains([]string{act.ActionID, act.ActionVersion, act.AdapterID, act.Operation}, ""), refBind),
		validateApprovalMode(act.ApprovalScope),
		cond(!validAuth(act.Capabilities, act.Scope, false), refuse(binding.ReasonShapeInvalid, model.StatusInvalidArgument, "")),
		cond(!slices.ContainsFunc(scopeLists(act.Scope), func(l []string) bool { return len(l) > 0 }), refuse(ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope)),
	)
}

func validateCandidateBinding(f EligibilityFacts, actor model.Actor, c CandidateFacts) model.Refusal {
	tScope := model.Scope{Snapshot: f.Scope.Snapshot, Tenant: f.Scope.Tenant}
	m := c.Membership
	chOK := m.TenantID == f.Scope.Tenant && m.ChannelID == f.Scope.Ancestry.Channel && m.Actor == actor && slices.Contains(validChRoles, m.Role) && (!m.Current || m.Role != standing.ChannelRoleNone)
	hasRead := slices.ContainsFunc(f.Parent.Decisions, func(d standing.Decision) bool { return d.Actor == actor && d.Action == standing.ActionChannelRead })
	return firstRef(
		binding.Section(tScope, c.Member.Bound, model.LevelTenant),
		cond(c.Member.Actor != actor || !slices.Contains(validOrgRoles, c.Member.Role) || !chOK, refBind),
		cond(!hasRead || !c.AuthorityAvailable, refUnav),
		cond(!validAuth(c.Capabilities, c.Scope, true), refBind),
		validateCandidateDevice(tScope, actor, c),
	)
}

func validateCandidateDevice(tScope model.Scope, actor model.Actor, c CandidateFacts) model.Refusal {
	if !c.RequireDeviceCurrent && c.Device.Bound.Snapshot.TransactionID == "" {
		return model.Refusal{}
	}
	return firstRef(
		binding.Section(tScope, c.Device.Bound, model.LevelTenant),
		cond(c.Device.Owner != actor || c.Device.Device == "", refBind),
	)
}

func validParticipants(t model.TaskFacts) bool {
	if len(t.Participants) > 16 || t.Creator.Type != rpcmiddleware.ActorTypeHuman || t.Creator.ID == "" {
		return false
	}
	owners := 0
	return validUnique(t.Participants, func(p model.Participant) model.Actor { return p.Actor }, func(p model.Participant) bool {
		if p.Role == ceiling.Owner {
			owners++
		}
		return p.Actor.ID != "" && ((p.Role == ceiling.Assignee && p.Actor.Type == rpcmiddleware.ActorTypeAgent) ||
			((p.Role == ceiling.Owner || p.Role == ceiling.Approver || p.Role == ceiling.Observer) && p.Actor.Type == rpcmiddleware.ActorTypeHuman && (p.Role != ceiling.Owner || p.Actor == t.Creator)))
	}) && owners == 1
}

func validDecisions(decisions []standing.Decision, ch standing.ResourceRef, aclVer, polVer string) bool {
	return validUnique(decisions, func(d standing.Decision) [2]any { return [2]any{d.Actor, d.Action} }, func(d standing.Decision) bool {
		return d.Resource == ch && d.ACLVersion == aclVer && d.PolicyVersion == polVer && (d.Action == standing.ActionChannelRead || d.Action == standing.ActionChannelPublish)
	})
}

func validUnique[T any, K comparable](items []T, keyFn func(T) K, okFn func(T) bool) bool {
	seen := make(map[K]bool, len(items))
	for _, it := range items {
		k := keyFn(it)
		if !okFn(it) || seen[k] {
			return false
		}
		seen[k] = true
	}
	return true
}

func validAuth(caps []grantpolicy.Capability, scope grantpolicy.ResourceScope, allowEmpty bool) bool {
	capsOK := (allowEmpty || len(caps) > 0) && validUnique(caps, func(c grantpolicy.Capability) grantpolicy.Capability { return c }, func(c grantpolicy.Capability) bool {
		return c >= grantpolicy.CapabilityMessageRead && c <= grantpolicy.CapabilityAuditRead
	})
	return capsOK && !slices.ContainsFunc(scopeLists(scope), func(list []string) bool {
		return !validUnique(list, func(v string) string { return v }, func(v string) bool {
			return utf8.ValidString(v) && v != "" && strings.TrimSpace(v) == v && !strings.ContainsAny(v, "*?") && !strings.ContainsFunc(v, unicode.IsControl)
		})
	})
}

func scopeLists(s grantpolicy.ResourceScope) [][]string {
	return [][]string{s.ChannelIDs, s.DMIDs, s.EventIDs, s.ThreadIDs, s.FileIDs, s.WorkspaceBindingIDs, s.WorkspacePathPrefixes, s.ToolIDs}
}

func subset[T comparable](req, ceil []T) bool {
	return !slices.ContainsFunc(req, func(v T) bool { return !slices.Contains(ceil, v) })
}

func authorityCovers(cCaps []grantpolicy.Capability, cScope grantpolicy.ResourceScope, rCaps []grantpolicy.Capability, rScope grantpolicy.ResourceScope) bool {
	return subset(rCaps, cCaps) && !slices.ContainsFunc([]int{0, 1, 2, 3, 4, 5, 6, 7}, func(i int) bool {
		return !subset(scopeLists(rScope)[i], scopeLists(cScope)[i])
	})
}
