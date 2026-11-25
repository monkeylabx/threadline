package binding

import "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"

// Section validates that bound evidence matches scope at the requested level.
func Section(scope model.Scope, b model.Bound, level model.Level) model.Refusal {
	if b.Snapshot.TransactionID == "" {
		return refuse(ReasonFactsUnavailable, model.StatusUnavailable, "")
	}
	if b.Snapshot != scope.Snapshot || b.Tenant != scope.Tenant || b.Ancestry != scope.Ancestry.Truncate(level) {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	return model.Refusal{}
}
