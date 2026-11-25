package binding

import (
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/evidence"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func validateLookup(p model.Principal, req model.Request, f model.Facts) Result {
	l := f.Lookup
	if !l.Found || l.Bound.Tenant != p.Tenant || !evidence.MatchesAssertions(req.Ancestry, l.Bound.Ancestry) {
		return Result{Refusal: refuse(ReasonObjectAbsent, model.StatusNotFound, "")}
	}
	if l.Bound.Snapshot != f.Snapshot || l.Resource != req.Resource || !evidence.CompleteAncestry(req.Resource, l.Bound.Ancestry) {
		return Result{Refusal: refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")}
	}
	return Result{Scope: model.Scope{Snapshot: f.Snapshot, Tenant: p.Tenant, Ancestry: l.Bound.Ancestry}}
}
