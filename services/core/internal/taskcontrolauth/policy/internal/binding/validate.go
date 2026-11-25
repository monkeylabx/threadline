package binding

import (
	"context"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/credential"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/request"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

type checkFn func(model.Principal, model.Request, model.Facts) model.Refusal

var orderedChecks = []checkFn{
	credential.CheckAuthentication, request.CheckShape, request.CheckAction, request.CheckPurpose,
	request.CheckParentKind, checkDevice, checkOrganization, checkMember,
}

// Validate checks principal, request, identity, and lookup bindings in contract
// refusal order. A zero Refusal means inputs are bound, never allow.
func Validate(ctx context.Context, p model.Principal, req model.Request, f model.Facts) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	for _, check := range orderedChecks {
		if r := check(p, req, f); r.Refused() {
			return Result{Refusal: r}, nil
		}
	}
	res := validateLookup(p, req, f)
	if res.Refusal.Refused() {
		return res, nil
	}
	return validateDisclosure(p, req, f, res.Scope), nil
}
