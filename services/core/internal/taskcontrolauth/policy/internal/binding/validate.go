package binding

import (
	"context"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

type checkFn func(model.Principal, model.Request, model.Facts) model.Refusal

var orderedChecks = []checkFn{
	checkAuthentication, checkShape, checkAction, checkPurpose,
	checkParentKind, checkDevice, checkOrganization, checkMember,
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
	return validateLookup(p, req, f), nil
}
