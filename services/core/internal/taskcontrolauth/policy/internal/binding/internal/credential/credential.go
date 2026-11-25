// Package credential checks typed identity observations supplied by trusted
// adapters. These checks do not authenticate a credential or grant permission.
package credential

import (
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const ReasonAuthenticationRequired model.Reason = "authentication-required"

func CheckAuthentication(p model.Principal, _ model.Request, f model.Facts) model.Refusal {
	if p.Tenant == "" || p.Actor.ID == "" || p.Device == "" {
		return model.Refusal{Reason: ReasonAuthenticationRequired, Status: model.StatusUnauthenticated}
	}
	if f.ObservedAt.IsZero() || !f.ObservedAt.Before(p.ExpiresAt) || !validRouteCredential(p) {
		return model.Refusal{Reason: ReasonAuthenticationRequired, Status: model.StatusUnauthenticated}
	}
	return model.Refusal{}
}

func validRouteCredential(p model.Principal) bool {
	switch p.Route {
	case ceiling.Human:
		return humanCredential(p)
	case ceiling.AssignedRuntime:
		return runtimeCredential(p)
	default:
		return p.Audience == model.AudienceHumanSession || p.Audience == model.AudienceRuntimeControl
	}
}

func humanCredential(p model.Principal) bool {
	return p.Actor.Type == rpcmiddleware.ActorTypeHuman && p.Audience == model.AudienceHumanSession &&
		p.Operator == (model.Actor{}) && p.Enrollment == ""
}

func runtimeCredential(p model.Principal) bool {
	return p.Actor.Type == rpcmiddleware.ActorTypeAgent && p.Operator.Type == rpcmiddleware.ActorTypeHuman &&
		p.Operator.ID != "" && p.Enrollment != "" &&
		p.RuntimeGeneration > 0 && p.Audience == model.AudienceRuntimeControl
}

func DeviceOwner(p model.Principal) model.Actor {
	if p.Route == ceiling.AssignedRuntime {
		return p.Operator
	}
	return p.Actor
}
