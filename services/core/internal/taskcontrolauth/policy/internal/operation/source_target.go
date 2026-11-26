package operation

import (
	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

var participantActorType = map[model.TaskRole]rpcmiddleware.ActorType{
	ceiling.Assignee: rpcmiddleware.ActorTypeAgent,
	ceiling.Owner:    rpcmiddleware.ActorTypeHuman,
	ceiling.Approver: rpcmiddleware.ActorTypeHuman,
	ceiling.Observer: rpcmiddleware.ActorTypeHuman,
}

func checkSource(s *evalState) model.Refusal {
	if !sourceOps[s.op] {
		return model.Refusal{}
	}
	src := s.opFacts.Source
	if r := binding.Section(s.res.Scope, src.Bound, model.LevelChannel); r.Refused() {
		return r
	}
	if src.EventID == "" || (s.op != ceiling.TaskCreate && src.EventID != s.f.Task.TriggerEvent) {
		return refuse(binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	if !src.Retained {
		return refuse(ReasonSourceErased, model.StatusFailedPrecondition, model.CodeRetentionExpired)
	}
	if !src.InCurrentInterval && !src.HistorySharingCurrent {
		return refuse(ReasonSourceHistoryDenied, model.StatusPermissionDenied, model.CodeHistorySharingDenied)
	}
	return model.Refusal{}
}

func checkTarget(s *evalState) model.Refusal {
	if s.op != ceiling.TaskCreate && s.op != ceiling.RunStart {
		return model.Refusal{}
	}
	t := s.opFacts.Target
	if r := binding.Section(s.res.Scope, t.Bound, model.LevelChannel); r.Refused() {
		return r
	}
	if !orgRoles[t.OperatorOrgRole] {
		return refuse(binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	if t.OperatorOrgRole == standing.OrganizationRoleGuest {
		return refuse(ReasonOrganizationCeiling, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	parts := s.f.Task.Participants
	if s.op == ceiling.TaskCreate {
		parts = s.opFacts.Create.DerivedParticipants
	}
	wantAgent, ok := findAssignee(parts)
	if !ok || !validTargetBinding(t, wantAgent) {
		return refuse(ReasonTargetBindingDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	if !t.DeviceCurrent {
		return refuse(binding.ReasonDeviceRevoked, model.StatusPermissionDenied, model.CodeDeviceRevoked)
	}
	return model.Refusal{}
}

func findAssignee(participants []model.Participant) (model.Actor, bool) {
	for _, part := range participants {
		if part.Role == ceiling.Assignee && part.Actor.ID != "" {
			return part.Actor, true
		}
	}
	return model.Actor{}, false
}

func validTargetBinding(t TargetFacts, wantAgent model.Actor) bool {
	return t.PermittedByPolicy && t.Enrollment != "" && t.Device != "" &&
		t.RuntimeGeneration > 0 && t.RuntimeGeneration == t.ExpectedRuntimeGeneration &&
		t.DeviceGeneration > 0 && t.DeviceGeneration == t.ExpectedDeviceGeneration &&
		t.AgentEnabled && t.Agent == wantAgent &&
		t.OperatorMemberActive && t.Operator.Type == rpcmiddleware.ActorTypeHuman && t.Operator.ID != ""
}

func checkCreateRequest(s *evalState) model.Refusal {
	if s.op != ceiling.TaskCreate {
		return model.Refusal{}
	}
	c := s.opFacts.Create
	if c.AdditionalReferences {
		return refuse(ReasonUnsupportedContext, model.StatusUnimplemented, "")
	}
	if len(c.Participants) > maxTaskParticipants {
		return refuse(ReasonPayloadTooLarge, model.StatusResourceExhausted, model.CodePayloadTooLarge)
	}
	if c.HasParticipantTimestamps {
		return refuse(ReasonParticipantSetInvalid, model.StatusInvalidArgument, "")
	}
	want, ok := derivedParticipantMap(c.DerivedParticipants, s.p.Actor, s.opFacts.Target.Agent)
	if !ok {
		return refuse(binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	if len(c.Participants) > 0 && !participantsMatchDerived(c.Participants, want) {
		return refuse(ReasonParticipantSetInvalid, model.StatusInvalidArgument, "")
	}
	return model.Refusal{}
}

func derivedParticipantMap(parts []model.Participant, creator, agent model.Actor) (map[model.Actor]model.TaskRole, bool) {
	if len(parts) == 0 || len(parts) > maxTaskParticipants {
		return nil, false
	}
	m, owners, assignees := make(map[model.Actor]model.TaskRole, len(parts)), 0, 0
	for _, part := range parts {
		if !validParticipantActorRole(part) || m[part.Actor] != "" {
			return nil, false
		}
		m[part.Actor] = part.Role
		if part.Role == ceiling.Owner && part.Actor == creator {
			owners++
		}
		if part.Role == ceiling.Assignee && part.Actor == agent {
			assignees++
		}
	}
	return m, owners == 1 && assignees == 1
}

func validParticipantActorRole(p model.Participant) bool {
	want, ok := participantActorType[p.Role]
	return ok && p.Actor.Type == want && p.Actor.ID != ""
}

func participantsMatchDerived(parts []model.Participant, want map[model.Actor]model.TaskRole) bool {
	seen := make(map[model.Actor]bool, len(parts))
	for _, part := range parts {
		if seen[part.Actor] || want[part.Actor] != part.Role {
			return false
		}
		seen[part.Actor] = true
	}
	return len(seen) == len(want)
}
