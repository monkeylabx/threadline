package binding

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

var actionResourceKinds = map[model.Action]model.ResourceKind{
	ceiling.TaskCreate:      model.ResourceChannel,
	ceiling.TaskRead:        model.ResourceTask,
	ceiling.RunStart:        model.ResourceTask,
	ceiling.RunRead:         model.ResourceRun,
	ceiling.RunControl:      model.ResourceRun,
	ceiling.ApprovalRequest: model.ResourceRun,
	ceiling.RunReport:       model.ResourceRun,
	ceiling.ApprovalRead:    model.ResourceApproval,
	ceiling.ApprovalDecide:  model.ResourceApproval,
	ceiling.ApprovalRevoke:  model.ResourceApproval,
}

var receiptCapableActions = map[model.Action]bool{
	ceiling.TaskCreate:      true,
	ceiling.RunStart:        true,
	ceiling.RunControl:      true,
	ceiling.RunReport:       true,
	ceiling.ApprovalRequest: true,
	ceiling.ApprovalDecide:  true,
	ceiling.ApprovalRevoke:  true,
}

func checkAuthentication(p model.Principal, _ model.Request, f model.Facts) model.Refusal {
	if p.Tenant == "" || p.Actor.ID == "" || p.Device == "" {
		return refuse(ReasonAuthenticationRequired, model.StatusUnauthenticated, "")
	}
	if f.ObservedAt.IsZero() || !f.ObservedAt.Before(p.ExpiresAt) || !validRouteCredential(p) {
		return refuse(ReasonAuthenticationRequired, model.StatusUnauthenticated, "")
	}
	return model.Refusal{}
}

func validRouteCredential(p model.Principal) bool {
	switch p.Route {
	case ceiling.Human:
		return p.Actor.Type == rpcmiddleware.ActorTypeHuman && p.Audience == model.AudienceHumanSession &&
			p.Operator == (model.Actor{}) && p.Enrollment == ""
	case ceiling.AssignedRuntime:
		return p.Actor.Type == rpcmiddleware.ActorTypeAgent && p.Operator.Type == rpcmiddleware.ActorTypeHuman &&
			p.Operator.ID != "" && p.Enrollment != "" &&
			p.RuntimeGeneration > 0 && p.Audience == model.AudienceRuntimeControl
	default:
		return p.Audience == model.AudienceHumanSession || p.Audience == model.AudienceRuntimeControl
	}
}

func checkShape(_ model.Principal, req model.Request, _ model.Facts) model.Refusal {
	if !validID(req.Resource.ID) || !validOptionalAncestry(req.Ancestry) {
		return refuse(ReasonShapeInvalid, model.StatusInvalidArgument, "")
	}
	if want, known := actionResourceKinds[req.Action]; known && req.Resource.Kind != want {
		return refuse(ReasonShapeInvalid, model.StatusInvalidArgument, "")
	}
	if req.Purpose == model.PurposeOwnReceiptRead && !validID(req.RPC) {
		return refuse(ReasonShapeInvalid, model.StatusInvalidArgument, "")
	}
	return model.Refusal{}
}

func validOptionalAncestry(a model.Ancestry) bool {
	for _, id := range [4]string{a.Channel, a.Task, a.Run, a.Approval} {
		if id != "" && !validID(id) {
			return false
		}
	}
	return true
}

func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 || !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}

func checkAction(_ model.Principal, req model.Request, _ model.Facts) model.Refusal {
	if _, ok := ceiling.RequiredParentActions(req.Action); !ok {
		return refuse(ReasonUnknownAction, model.StatusUnimplemented, "")
	}
	return model.Refusal{}
}

func checkPurpose(_ model.Principal, req model.Request, _ model.Facts) model.Refusal {
	if req.Purpose == model.PurposeOperation {
		return model.Refusal{}
	}
	if req.Purpose == model.PurposeOwnReceiptRead && receiptCapableActions[req.Action] {
		return model.Refusal{}
	}
	return refuse(ReasonPurposeInvalid, model.StatusInvalidArgument, "")
}

func checkParentKind(_ model.Principal, req model.Request, _ model.Facts) model.Refusal {
	if req.ParentKind != model.ParentChannel {
		return refuse(ReasonUnsupportedParent, model.StatusUnimplemented, "")
	}
	return model.Refusal{}
}
