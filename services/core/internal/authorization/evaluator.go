package authorization

import (
	"context"

	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

// Evaluate makes one fail-closed authorization decision from authenticated
// request identity and explicitly supplied current facts.
func Evaluate(ctx context.Context, action Action, facts CurrentFacts) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	principal, ok := rpcmiddleware.PrincipalFromContext(ctx)
	if !ok {
		return Decision{
			Effect: EffectDeny,
			Reason: ReasonAuthenticationRequired,
		}, nil
	}

	decision := Decision{
		Effect:   EffectDeny,
		Action:   action,
		Resource: facts.Resource.Ref,
		Actor: ActorRef{
			Type: principal.Actor().Type(),
			ID:   principal.Actor().ID(),
		},
	}
	deny := func(reason Reason) (Decision, error) {
		decision.Reason = reason
		return decision, nil
	}

	if facts.Resource.Ref.TenantID != principal.TenantID() {
		return deny(ReasonTenantMismatch)
	}
	if !facts.Available {
		return deny(ReasonFactsUnavailable)
	}
	if facts.Organization.TenantID != principal.TenantID() {
		return deny(ReasonFactsUnavailable)
	}
	decision.PolicyVersion = facts.Organization.PolicyVersion
	if facts.Organization.State != OrganizationStateActive {
		return deny(ReasonOrganizationUnavailable)
	}
	if facts.Organization.PolicyVersion == "" {
		decision.PolicyVersion = ""
		return deny(ReasonPolicyVersionInvalid)
	}
	if !sameActorAndTenant(facts.Member.TenantID, facts.Member.Actor, principal) {
		return deny(ReasonFactsUnavailable)
	}
	if facts.Member.State != MemberStateActive {
		return deny(ReasonMemberInactive)
	}
	if !knownOrganizationRole(facts.Member.Role) {
		return deny(ReasonOrganizationRoleDenied)
	}
	if !knownAction(action) {
		return deny(ReasonUnknownAction)
	}
	if !knownResource(action, facts.Resource) {
		return deny(ReasonUnknownResource)
	}

	channelRole := ChannelRoleNone
	if facts.Resource.Ref.Kind == ResourceKindChannel {
		if !sameMembership(facts.Membership, facts.Resource.Ref, principal) {
			return deny(ReasonFactsUnavailable)
		}
		if facts.Resource.Visibility != VisibilityPublic && facts.Resource.Visibility != VisibilityPrivate {
			return deny(ReasonUnknownResource)
		}
		if !knownResourceState(facts.Resource.State) {
			return deny(ReasonResourceStateDenied)
		}
		discoverable := action == ActionChannelDiscover && facts.Resource.Visibility == VisibilityPublic
		if facts.Membership.Current && facts.Membership.IntervalID == "" {
			return deny(ReasonNotAMember)
		}
		if !facts.Membership.Current && !discoverable {
			return deny(ReasonNotAMember)
		}
		if facts.Membership.Current {
			channelRole = facts.Membership.Role
			if !knownCurrentChannelRole(channelRole) {
				return deny(ReasonChannelRoleDenied)
			}
		}
	}

	if !roleAllows(facts.Member.Role, channelRole, action) {
		if organizationCanPerform(facts.Member.Role, action) {
			return deny(ReasonChannelRoleDenied)
		}
		return deny(ReasonOrganizationRoleDenied)
	}
	if facts.Resource.Ref.Kind == ResourceKindChannel {
		if facts.Resource.State == ResourceStatePendingDeletion ||
			(facts.Resource.State == ResourceStateArchived && stateChanging(action)) {
			return deny(ReasonResourceStateDenied)
		}
	}

	if facts.ACL.Version == "" {
		return deny(ReasonACLVersionInvalid)
	}
	decision.ACLVersion = facts.ACL.Version
	if facts.ACL.Resource != facts.Resource.Ref ||
		(facts.ACL.DefaultEffect != ACLEffectAllow && facts.ACL.DefaultEffect != ACLEffectDeny) ||
		!validACLEntries(facts.ACL.Entries) {
		return deny(ReasonACLInvalid)
	}
	matchedAllow := false
	for _, entry := range facts.ACL.Entries {
		if entry.Actor == decision.Actor && entry.Action == action {
			if entry.Effect == ACLEffectDeny {
				return deny(ReasonACLMatchedDeny)
			}
			matchedAllow = true
		}
	}
	if !matchedAllow && facts.ACL.DefaultEffect == ACLEffectDeny {
		return deny(ReasonACLDefaultDeny)
	}

	if facts.Runtime.Required {
		switch facts.Runtime.Capability {
		case CapabilityUnspecified:
			return deny(ReasonCapabilityRequired)
		case CapabilityAllow:
			if !facts.Runtime.DelegationAllows {
				return deny(ReasonDelegationDenied)
			}
		default:
			return deny(ReasonCapabilityDenied)
		}
	}
	if facts.Approval.Required && !facts.Approval.Present {
		return deny(ReasonApprovalRequired)
	}
	decision.Effect = EffectAllow
	decision.Reason = ReasonAllowed
	return decision, nil
}

func sameActorAndTenant(tenantID string, actor ActorRef, principal rpcmiddleware.Principal) bool {
	return tenantID == principal.TenantID() && actor.Type == principal.Actor().Type() && actor.ID == principal.Actor().ID()
}

func sameMembership(membership ChannelMembershipFacts, resource ResourceRef, principal rpcmiddleware.Principal) bool {
	return sameActorAndTenant(membership.TenantID, membership.Actor, principal) && membership.ChannelID == resource.ID
}

func knownAction(action Action) bool {
	switch action {
	case ActionSpaceDiscover, ActionSpaceChannelCreate, ActionChannelDiscover, ActionChannelRead,
		ActionChannelPublish, ActionChannelUpdate, ActionChannelArchive, ActionChannelMembershipList,
		ActionChannelMembershipAdd, ActionChannelMembershipRemove, ActionChannelACLUpdate:
		return true
	default:
		return false
	}
}

func knownOrganizationRole(role OrganizationRole) bool {
	switch role {
	case OrganizationRoleOwner, OrganizationRoleAdmin, OrganizationRoleSecurityAdmin,
		OrganizationRoleMember, OrganizationRoleGuest:
		return true
	default:
		return false
	}
}

func knownCurrentChannelRole(role ChannelRole) bool {
	switch role {
	case ChannelRoleOwner, ChannelRoleModerator, ChannelRoleMember, ChannelRoleGuest:
		return true
	default:
		return false
	}
}

func knownResource(action Action, resource ResourceFacts) bool {
	if !resource.Exists || resource.Ref.ID == "" {
		return false
	}
	switch resource.Ref.Kind {
	case ResourceKindSpace:
		return action == ActionSpaceDiscover || action == ActionSpaceChannelCreate
	case ResourceKindChannel:
		return action != ActionSpaceDiscover && action != ActionSpaceChannelCreate
	default:
		return false
	}
}

func knownResourceState(state ResourceState) bool {
	return state == ResourceStateActive || state == ResourceStateArchived || state == ResourceStatePendingDeletion
}

func stateChanging(action Action) bool {
	switch action {
	case ActionChannelPublish, ActionChannelUpdate, ActionChannelArchive, ActionChannelMembershipAdd,
		ActionChannelMembershipRemove, ActionChannelACLUpdate:
		return true
	default:
		return false
	}
}

func roleAllows(organizationRole OrganizationRole, channelRole ChannelRole, action Action) bool {
	switch organizationRole {
	case OrganizationRoleOwner, OrganizationRoleAdmin:
		switch channelRole {
		case ChannelRoleNone:
			return oneOf(action, ActionSpaceDiscover, ActionSpaceChannelCreate, ActionChannelDiscover)
		case ChannelRoleOwner:
			return true
		case ChannelRoleModerator:
			return oneOf(action, ActionSpaceDiscover, ActionSpaceChannelCreate, ActionChannelDiscover,
				ActionChannelRead, ActionChannelPublish, ActionChannelUpdate, ActionChannelMembershipList,
				ActionChannelMembershipAdd, ActionChannelMembershipRemove)
		case ChannelRoleMember, ChannelRoleGuest:
			return oneOf(action, ActionSpaceDiscover, ActionSpaceChannelCreate, ActionChannelDiscover,
				ActionChannelRead, ActionChannelPublish, ActionChannelMembershipList)
		}
	case OrganizationRoleSecurityAdmin:
		switch channelRole {
		case ChannelRoleNone:
			return oneOf(action, ActionSpaceDiscover, ActionChannelDiscover)
		case ChannelRoleOwner:
			return oneOf(action, ActionSpaceDiscover, ActionChannelDiscover, ActionChannelRead,
				ActionChannelPublish, ActionChannelMembershipList, ActionChannelMembershipRemove,
				ActionChannelACLUpdate)
		case ChannelRoleModerator:
			return oneOf(action, ActionSpaceDiscover, ActionChannelDiscover, ActionChannelRead,
				ActionChannelPublish, ActionChannelMembershipList, ActionChannelMembershipRemove)
		case ChannelRoleMember, ChannelRoleGuest:
			return collaborationAction(action)
		}
	case OrganizationRoleMember:
		switch channelRole {
		case ChannelRoleNone:
			return oneOf(action, ActionSpaceDiscover, ActionChannelDiscover)
		case ChannelRoleOwner:
			return oneOf(action, ActionSpaceDiscover, ActionChannelDiscover, ActionChannelRead,
				ActionChannelPublish, ActionChannelUpdate, ActionChannelArchive, ActionChannelMembershipList,
				ActionChannelMembershipAdd, ActionChannelMembershipRemove, ActionChannelACLUpdate)
		case ChannelRoleModerator:
			return oneOf(action, ActionSpaceDiscover, ActionChannelDiscover, ActionChannelRead,
				ActionChannelPublish, ActionChannelUpdate, ActionChannelMembershipList,
				ActionChannelMembershipAdd, ActionChannelMembershipRemove)
		case ChannelRoleMember, ChannelRoleGuest:
			return collaborationAction(action)
		}
	case OrganizationRoleGuest:
		if channelRole == ChannelRoleNone {
			return action == ActionSpaceDiscover
		}
		return collaborationAction(action)
	}
	return false
}

func organizationCanPerform(organizationRole OrganizationRole, action Action) bool {
	for _, channelRole := range [...]ChannelRole{ChannelRoleNone, ChannelRoleOwner, ChannelRoleModerator, ChannelRoleMember, ChannelRoleGuest} {
		if roleAllows(organizationRole, channelRole, action) {
			return true
		}
	}
	return false
}

func collaborationAction(action Action) bool {
	return oneOf(action, ActionSpaceDiscover, ActionChannelDiscover, ActionChannelRead,
		ActionChannelPublish, ActionChannelMembershipList)
}

func oneOf(action Action, allowed ...Action) bool {
	for _, candidate := range allowed {
		if action == candidate {
			return true
		}
	}
	return false
}

func validACLEntries(entries []ACLEntry) bool {
	for _, entry := range entries {
		if entry.Actor.ID == "" || !knownActorType(entry.Actor.Type) || !knownAction(entry.Action) ||
			(entry.Effect != ACLEffectAllow && entry.Effect != ACLEffectDeny) {
			return false
		}
	}
	return true
}

func knownActorType(actorType rpcmiddleware.ActorType) bool {
	return actorType == rpcmiddleware.ActorTypeHuman || actorType == rpcmiddleware.ActorTypeAgent || actorType == rpcmiddleware.ActorTypeService
}
