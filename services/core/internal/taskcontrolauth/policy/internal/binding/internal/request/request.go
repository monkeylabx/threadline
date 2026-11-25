// Package request checks the closed request vocabulary and identifier shapes.
package request

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

const (
	ReasonShapeInvalid      model.Reason = "shape-invalid"
	ReasonUnknownAction     model.Reason = "unknown-action"
	ReasonPurposeInvalid    model.Reason = "purpose-invalid"
	ReasonUnsupportedParent model.Reason = "unsupported-parent"
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
	ceiling.TaskCreate: true, ceiling.RunStart: true, ceiling.RunControl: true,
	ceiling.RunReport: true, ceiling.ApprovalRequest: true,
	ceiling.ApprovalDecide: true, ceiling.ApprovalRevoke: true,
}

func CheckShape(_ model.Principal, req model.Request, _ model.Facts) model.Refusal {
	if !ValidID(req.Resource.ID) || !validOptionalAncestry(req.Ancestry) {
		return model.Refusal{Reason: ReasonShapeInvalid, Status: model.StatusInvalidArgument}
	}
	if want, known := actionResourceKinds[req.Action]; known && req.Resource.Kind != want && !taskScopedRead(req) {
		return model.Refusal{Reason: ReasonShapeInvalid, Status: model.StatusInvalidArgument}
	}
	if req.Purpose == model.PurposeOwnReceiptRead && !ValidID(req.RPC) {
		return model.Refusal{Reason: ReasonShapeInvalid, Status: model.StatusInvalidArgument}
	}
	return model.Refusal{}
}

func taskScopedRead(req model.Request) bool {
	return req.Resource.Kind == model.ResourceTask && (req.Action == ceiling.RunRead || req.Action == ceiling.ApprovalRead)
}

func validOptionalAncestry(a model.Ancestry) bool {
	for _, id := range [4]string{a.Channel, a.Task, a.Run, a.Approval} {
		if id != "" && !ValidID(id) {
			return false
		}
	}
	return true
}

func ValidID(s string) bool {
	if len(s) == 0 || len(s) > 128 || !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}

func CheckAction(_ model.Principal, req model.Request, _ model.Facts) model.Refusal {
	if _, ok := ceiling.RequiredParentActions(req.Action); !ok {
		return model.Refusal{Reason: ReasonUnknownAction, Status: model.StatusUnimplemented}
	}
	return model.Refusal{}
}

func CheckPurpose(_ model.Principal, req model.Request, _ model.Facts) model.Refusal {
	if req.Purpose == model.PurposeOperation {
		return model.Refusal{}
	}
	if req.Purpose == model.PurposeOwnReceiptRead && receiptCapableActions[req.Action] {
		return model.Refusal{}
	}
	return model.Refusal{Reason: ReasonPurposeInvalid, Status: model.StatusInvalidArgument}
}

func CheckParentKind(_ model.Principal, req model.Request, _ model.Facts) model.Refusal {
	if req.ParentKind != model.ParentChannel {
		return model.Refusal{Reason: ReasonUnsupportedParent, Status: model.StatusUnimplemented}
	}
	return model.Refusal{}
}
