package binding

import (
	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func membershipBound(m standing.ChannelMembershipFacts, scope model.Scope, actor model.Actor) bool {
	return m.TenantID == scope.Tenant && m.ChannelID == scope.Ancestry.Channel && m.Actor == actor
}

func validACL(acl standing.ResourceACLFacts, ch standing.ResourceRef) bool {
	if acl.Resource != ch || !validACLEffect(acl.DefaultEffect) {
		return false
	}
	for _, e := range acl.Entries {
		if !validACLEffect(e.Effect) {
			return false
		}
	}
	return true
}

func validACLEffect(eff standing.ACLEffect) bool {
	return eff == standing.ACLEffectAllow || eff == standing.ACLEffectDeny
}

type decisionKey struct {
	actor  model.Actor
	action standing.Action
}

func validDecisions(decisions []standing.Decision, ch standing.ResourceRef, aclVer, policyVer string) bool {
	seen := make(map[decisionKey]bool, len(decisions))
	for _, d := range decisions {
		if d.Resource != ch || d.ACLVersion != aclVer || d.PolicyVersion != policyVer {
			return false
		}
		if d.Action != standing.ActionChannelRead && d.Action != standing.ActionChannelPublish {
			return false
		}
		k := decisionKey{actor: d.Actor, action: d.Action}
		if seen[k] {
			return false
		}
		seen[k] = true
	}
	return true
}

func findReadDecision(decisions []standing.Decision, actor model.Actor) (standing.Decision, bool) {
	for _, d := range decisions {
		if d.Actor == actor && d.Action == standing.ActionChannelRead {
			return d, true
		}
	}
	return standing.Decision{}, false
}

func checkPolicyPresence(s *disclosureState) model.Refusal {
	if s.f.Policy.Snapshot.TransactionID == "" {
		return refuse(ReasonPolicyUnavailable, model.StatusUnavailable, "")
	}
	return model.Refusal{}
}

func checkPolicyFormat(s *disclosureState) model.Refusal {
	if s.f.Policy.Profile != model.PolicyProfile || s.f.Policy.Revision == "" {
		return refuse(ReasonPolicyInvalid, model.StatusUnavailable, "")
	}
	return model.Refusal{}
}

func checkParentMembership(s *disclosureState) model.Refusal {
	m := s.f.Parent.Membership
	if !m.Current || m.IntervalID == "" {
		return refuse(ReasonNotAMember, model.StatusPermissionDenied, model.CodeNotAMember)
	}
	return model.Refusal{}
}

func checkParentRead(s *disclosureState) model.Refusal {
	for _, r := range s.readers {
		d, ok := findReadDecision(s.f.Parent.Decisions, r)
		if !ok || d.Effect != standing.EffectAllow {
			return refuse(ReasonParentReadDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
		}
	}
	return model.Refusal{}
}

func checkParentACL(s *disclosureState) model.Refusal {
	acl := s.f.Parent.ACL
	for _, r := range s.readers {
		if !aclAllowsRead(acl, r) {
			return refuse(ReasonParentACLDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
		}
	}
	return model.Refusal{}
}

func aclAllowsRead(acl standing.ResourceACLFacts, actor model.Actor) bool {
	matchedAllow := false
	for _, e := range acl.Entries {
		if e.Actor != actor || e.Action != standing.ActionChannelRead {
			continue
		}
		if e.Effect == standing.ACLEffectDeny {
			return false
		}
		matchedAllow = true
	}
	return matchedAllow || acl.DefaultEffect == standing.ACLEffectAllow
}
