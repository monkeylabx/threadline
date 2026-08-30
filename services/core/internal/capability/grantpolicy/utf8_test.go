package grantpolicy

import "testing"

func TestValidateAndNarrowRejectsInvalidUTF8AcrossIdentifierSurfaces(t *testing.T) {
	t.Parallel()

	invalid := string([]byte{0xff})
	authorityMutations := []struct {
		name   string
		mutate func(*IssuanceAuthority)
	}{
		{"tenant", func(a *IssuanceAuthority) { a.TenantID = invalid }},
		{"task", func(a *IssuanceAuthority) { a.TaskID = invalid }},
		{"run", func(a *IssuanceAuthority) { a.RunID = invalid }},
		{"device", func(a *IssuanceAuthority) { a.DeviceID = invalid }},
		{"policy", func(a *IssuanceAuthority) { a.PolicyVersion = invalid }},
		{"grantee", func(a *IssuanceAuthority) { a.Grantee.ID = invalid }},
		{"initiator", func(a *IssuanceAuthority) { a.Initiator.ID = invalid }},
	}
	for _, field := range scopeFields {
		field := field
		authorityMutations = append(authorityMutations, struct {
			name   string
			mutate func(*IssuanceAuthority)
		}{"authority-" + field.name, func(a *IssuanceAuthority) {
			values := field.get(a.Scope)
			values[0] = invalid
			field.set(&a.Scope, values)
		}})
	}

	for _, testCase := range authorityMutations {
		t.Run(testCase.name, func(t *testing.T) {
			at, request, authority := validFixture()
			testCase.mutate(&authority)
			assertErrorCode(t, ErrorInvalidAuthority, func() (ValidatedRequest, error) {
				return ValidateAndNarrow(at, request, authority)
			})
		})
	}

	for _, field := range scopeFields {
		field := field
		t.Run("request-"+field.name, func(t *testing.T) {
			at, request, authority := validFixture()
			values := field.get(request.Scope)
			values[0] = invalid
			field.set(&request.Scope, values)
			assertErrorCode(t, ErrorInvalidInput, func() (ValidatedRequest, error) {
				return ValidateAndNarrow(at, request, authority)
			})
		})
	}
}
