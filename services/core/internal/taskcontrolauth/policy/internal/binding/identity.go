package binding

import (
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/credential"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func tenantScope(p model.Principal, f model.Facts) model.Scope {
	return model.Scope{Snapshot: f.Snapshot, Tenant: p.Tenant}
}

func checkDevice(p model.Principal, _ model.Request, f model.Facts) model.Refusal {
	if r := Section(tenantScope(p, f), f.Device.Bound, model.LevelTenant); r.Refused() {
		return r
	}
	if f.Device.Owner != credential.DeviceOwner(p) || f.Device.Device != p.Device {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	if !f.Device.Current {
		return refuse(ReasonDeviceRevoked, model.StatusPermissionDenied, model.CodeDeviceRevoked)
	}
	return model.Refusal{}
}

func checkOrganization(p model.Principal, _ model.Request, f model.Facts) model.Refusal {
	if r := Section(tenantScope(p, f), f.Organization.Bound, model.LevelTenant); r.Refused() {
		return r
	}
	if !f.Organization.Active {
		return refuse(ReasonOrganizationInactive, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	return model.Refusal{}
}

func checkMember(p model.Principal, _ model.Request, f model.Facts) model.Refusal {
	if r := Section(tenantScope(p, f), f.Member.Bound, model.LevelTenant); r.Refused() {
		return r
	}
	if f.Member.Actor != p.Actor {
		return refuse(ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	if !f.Member.Active {
		return refuse(ReasonMemberInactive, model.StatusPermissionDenied, model.CodePermissionDenied)
	}
	return model.Refusal{}
}
