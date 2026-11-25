package binding

import (
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

var receiptViewActions = map[model.Action]model.Action{
	ceiling.TaskCreate:      ceiling.TaskRead,
	ceiling.RunStart:        ceiling.RunRead,
	ceiling.RunControl:      ceiling.RunRead,
	ceiling.RunReport:       ceiling.RunRead,
	ceiling.ApprovalRequest: ceiling.ApprovalRead,
	ceiling.ApprovalDecide:  ceiling.ApprovalRead,
	ceiling.ApprovalRevoke:  ceiling.ApprovalRead,
}

func effectiveAction(action model.Action, receiptSelected bool) model.Action {
	if !receiptSelected {
		return action
	}
	if view, ok := receiptViewActions[action]; ok {
		return view
	}
	return action
}

// ValidateReceipt verifies committed own_receipt_read namespace and exact-input
// equality. Composition (#435) calls this last, after disclosure checks pass.
func ValidateReceipt(p model.Principal, req model.Request, f model.Facts, res Result) model.Refusal {
	if !res.ReceiptSelected {
		return model.Refusal{}
	}
	wantRun := ""
	if p.Route == ceiling.AssignedRuntime {
		wantRun = res.Scope.Ancestry.Run
	}
	want := model.ReceiptNamespace{Actor: p.Actor, Device: p.Device, RPC: req.RPC, Enrollment: p.Enrollment, RuntimeGeneration: p.RuntimeGeneration, Run: wantRun}
	if f.Receipt.Namespace != want {
		return refuse(ReasonReceiptNamespaceDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	if f.Receipt.Action != req.Action || f.Receipt.Resource != req.Resource || f.Receipt.RequestDigest != req.RequestDigest {
		return refuse(ReasonIdempotencyConflict, model.StatusAlreadyExists, model.CodeIdempotencyConflict)
	}
	return model.Refusal{}
}
