package ceiling

import standing "github.com/monkeylabx/threadline/services/core/internal/authorization"

// WithinRoleCeiling is necessary only. A true result grants no operation,
// content, receipt, execution authority or current approval eligibility.
func WithinRoleCeiling(action Action, state RoleState) bool {
	class := classFor(action)
	if !validRoles(state) || !tierAllows(class, state) {
		return false
	}
	switch state.Route {
	case Human:
		return humanRole(class, state)
	case AssignedRuntime:
		return state.TaskRole == Assignee && (class == metadata || class == runtime)
	default:
		return false
	}
}

func tierAllows(class actionClass, state RoleState) bool {
	if class == read || class == metadata || class == control {
		return true
	}
	return state.OrganizationRole != standing.OrganizationRoleGuest && state.ChannelRole != standing.ChannelRoleGuest
}

func humanRole(class actionClass, state RoleState) bool {
	switch class {
	case read, metadata, create:
		return state.TaskRole != Assignee
	case start:
		return state.TaskRole == Owner
	case control:
		return state.TaskRole == Owner || state.TaskRole == Observer || state.TaskRole == Approver
	case approval:
		return approvalRole(state)
	default:
		return false
	}
}

func approvalRole(state RoleState) bool {
	if state.TaskRole == Approver {
		return true
	}
	p := state.SelfApproval
	return state.TaskRole == Owner && p.DeploymentClass == "dedicated-dev" &&
		p.AllowOwner && p.ConfiguredOwner && p.OwnerRoleEnabled
}

func validRoles(state RoleState) bool {
	return knownOrganizationRole(state.OrganizationRole) && knownChannelRole(state.ChannelRole) && knownTaskRole(state.TaskRole)
}

func knownOrganizationRole(role standing.OrganizationRole) bool {
	switch role {
	case standing.OrganizationRoleOwner, standing.OrganizationRoleAdmin, standing.OrganizationRoleSecurityAdmin,
		standing.OrganizationRoleMember, standing.OrganizationRoleGuest:
		return true
	default:
		return false
	}
}

func knownChannelRole(role standing.ChannelRole) bool {
	switch role {
	case standing.ChannelRoleOwner, standing.ChannelRoleModerator, standing.ChannelRoleMember, standing.ChannelRoleGuest:
		return true
	default:
		return false
	}
}

func knownTaskRole(role TaskRole) bool {
	switch role {
	case None, Owner, Assignee, Approver, Observer:
		return true
	default:
		return false
	}
}
