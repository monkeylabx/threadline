package binding

import (
	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

type disclosureState struct {
	p                             model.Principal
	req                           model.Request
	f                             model.Facts
	scope                         model.Scope
	receiptSelected, taskRequired bool
	readers                       []model.Actor
	taskRole                      model.TaskRole
}

type disclosureStep func(*disclosureState) model.Refusal

var disclosureSteps = []disclosureStep{
	checkDisclosureAvailability, checkDisclosureBinding, checkKnownRoles,
	checkTaskParticipants, checkPolicyPresence, checkPolicyFormat,
	checkParentMembership, checkParentRead, checkParentACL,
}

func validateDisclosure(p model.Principal, req model.Request, f model.Facts, scope model.Scope) Result {
	s := &disclosureState{p: p, req: req, f: f, scope: scope, taskRole: ceiling.None}
	for _, step := range disclosureSteps {
		if r := step(s); r.Refused() {
			return Result{Refusal: r}
		}
	}
	return Result{Scope: s.scope, TaskRole: s.taskRole, EffectiveAction: effectiveAction(req.Action, s.receiptSelected), ReceiptSelected: s.receiptSelected}
}

func checkDisclosureAvailability(s *disclosureState) model.Refusal {
	if s.f.Parent.ACL.Version == "" {
		return refuse(ReasonFactsUnavailable, model.StatusUnavailable, "")
	}
	if s.req.Purpose == model.PurposeOwnReceiptRead && s.f.Receipt.Snapshot.TransactionID == "" {
		return refuse(ReasonFactsUnavailable, model.StatusUnavailable, "")
	}
	s.receiptSelected = s.req.Purpose == model.PurposeOwnReceiptRead && s.f.Receipt.Committed
	s.taskRequired = s.req.Action != ceiling.TaskCreate || s.receiptSelected
	if s.taskRequired && s.f.Task.Snapshot.TransactionID == "" {
		return refuse(ReasonFactsUnavailable, model.StatusUnavailable, "")
	}
	s.readers = []model.Actor{s.p.Actor}
	if s.p.Route == ceiling.AssignedRuntime {
		s.readers = []model.Actor{s.p.Actor, s.p.Operator}
	}
	for _, r := range s.readers {
		if _, ok := findReadDecision(s.f.Parent.Decisions, r); !ok {
			return refuse(ReasonFactsUnavailable, model.StatusUnavailable, "")
		}
	}
	return model.Refusal{}
}

func checkDisclosureBinding(s *disclosureState) model.Refusal {
	if r := checkHeaderBindings(s); r.Refused() {
		return r
	}
	chRef := standing.ResourceRef{TenantID: s.scope.Tenant, Kind: standing.ResourceKindChannel, ID: s.scope.Ancestry.Channel}
	if !membershipBound(s.f.Parent.Membership, s.scope, s.p.Actor) || !validACL(s.f.Parent.ACL, chRef) {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	if !validDecisions(s.f.Parent.Decisions, chRef, s.f.Parent.ACL.Version, s.f.Organization.PolicyVersion) {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	return model.Refusal{}
}

func checkHeaderBindings(s *disclosureState) model.Refusal {
	if r := Section(s.scope, s.f.Parent.Bound, model.LevelChannel); r.Refused() {
		return r
	}
	if s.f.Policy.Snapshot.TransactionID != "" {
		if r := Section(s.scope, s.f.Policy.Bound, model.LevelChannel); r.Refused() {
			return r
		}
	}
	if s.req.Purpose == model.PurposeOwnReceiptRead {
		if r := Section(s.scope, s.f.Receipt.Bound, model.LevelTenant); r.Refused() {
			return r
		}
	}
	if !s.taskRequired {
		return model.Refusal{}
	}
	taskScope, ok := taskHeaderScope(s)
	if !ok {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	return Section(taskScope, s.f.Task.Bound, model.LevelTask)
}

func taskHeaderScope(s *disclosureState) (model.Scope, bool) {
	taskScope := s.scope
	if s.req.Action == ceiling.TaskCreate && s.receiptSelected {
		if s.f.Receipt.Result.Kind != model.ResourceTask || s.f.Receipt.Result.ID == "" {
			return model.Scope{}, false
		}
		taskScope.Ancestry.Task = s.f.Receipt.Result.ID
	}
	return taskScope, true
}
