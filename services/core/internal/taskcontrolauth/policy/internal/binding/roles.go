package binding

import (
	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

func checkKnownRoles(s *disclosureState) model.Refusal {
	if !knownOrgRole(s.f.Member.Role) {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	m := s.f.Parent.Membership
	if !knownChannelRole(m.Role) || (m.Current && m.Role == standing.ChannelRoleNone) {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	return model.Refusal{}
}

func knownOrgRole(role standing.OrganizationRole) bool {
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
	case standing.ChannelRoleNone, standing.ChannelRoleOwner, standing.ChannelRoleModerator,
		standing.ChannelRoleMember, standing.ChannelRoleGuest:
		return true
	default:
		return false
	}
}

func checkTaskParticipants(s *disclosureState) model.Refusal {
	if !s.taskRequired {
		return model.Refusal{}
	}
	role, ok := validateParticipants(s.f.Task, s.p.Actor)
	if !ok {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	s.taskRole = role
	return model.Refusal{}
}

func validateParticipants(t model.TaskFacts, caller model.Actor) (model.TaskRole, bool) {
	if len(t.Participants) > 16 {
		return "", false
	}
	seen := make(map[model.Actor]bool, len(t.Participants))
	owners, callerRole := 0, ceiling.None
	for _, part := range t.Participants {
		if seen[part.Actor] || !validParticipantRole(part) {
			return "", false
		}
		seen[part.Actor] = true
		if part.Role == ceiling.Owner {
			if part.Actor != t.Creator {
				return "", false
			}
			owners++
		}
		if part.Actor == caller {
			callerRole = part.Role
		}
	}
	return callerRole, owners == 1
}

func validParticipantRole(p model.Participant) bool {
	switch p.Role {
	case ceiling.Assignee:
		return p.Actor.Type == rpcmiddleware.ActorTypeAgent
	case ceiling.Owner, ceiling.Approver, ceiling.Observer:
		return p.Actor.Type == rpcmiddleware.ActorTypeHuman
	default:
		return false
	}
}
