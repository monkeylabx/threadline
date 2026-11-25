package binding

import "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"

func validateLookup(p model.Principal, req model.Request, f model.Facts) Result {
	l := f.Lookup
	if !l.Found || l.Bound.Tenant != p.Tenant || !ancestryMatches(req.Ancestry, l.Bound.Ancestry) {
		return Result{Refusal: refuse(ReasonObjectAbsent, model.StatusNotFound, "")}
	}
	if l.Bound.Snapshot != f.Snapshot || l.Resource != req.Resource || !completeAncestry(req.Resource, l.Bound.Ancestry) {
		return Result{Refusal: refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")}
	}
	return Result{Scope: model.Scope{Snapshot: f.Snapshot, Tenant: p.Tenant, Ancestry: l.Bound.Ancestry}}
}

func ancestryMatches(req, stored model.Ancestry) bool {
	pairs := [4][2]string{
		{req.Channel, stored.Channel},
		{req.Task, stored.Task},
		{req.Run, stored.Run},
		{req.Approval, stored.Approval},
	}
	for _, pair := range pairs {
		if pair[0] != "" && pair[0] != pair[1] {
			return false
		}
	}
	return true
}

func completeAncestry(res model.ResourceRef, a model.Ancestry) bool {
	switch res.Kind {
	case model.ResourceChannel:
		return a.Channel == res.ID && a.Task == "" && a.Run == "" && a.Approval == ""
	case model.ResourceTask:
		return a.Channel != "" && a.Task == res.ID && a.Run == "" && a.Approval == ""
	case model.ResourceRun:
		return a.Channel != "" && a.Task != "" && a.Run == res.ID && a.Approval == ""
	case model.ResourceApproval:
		return a.Channel != "" && a.Task != "" && a.Run != "" && a.Approval == res.ID
	default:
		return false
	}
}
