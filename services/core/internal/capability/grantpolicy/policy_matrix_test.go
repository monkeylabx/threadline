package grantpolicy

import (
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var allCapabilities = []Capability{
	CapabilityMessageRead,
	CapabilityMessageReadHistory,
	CapabilityMessagePublish,
	CapabilityMessagePublishDraft,
	CapabilityFileRead,
	CapabilityFileWrite,
	CapabilityWorkspaceRead,
	CapabilityWorkspaceWrite,
	CapabilityToolInvoke,
	CapabilityActionExecute,
	CapabilityAgentDelegate,
	CapabilityMemoryRetain,
	CapabilityTaskApprove,
	CapabilityAuditRead,
}

type scopeField struct {
	name string
	get  func(ResourceScope) []string
	set  func(*ResourceScope, []string)
}

var scopeFields = []scopeField{
	{"channel_ids", func(s ResourceScope) []string { return s.ChannelIDs }, func(s *ResourceScope, v []string) { s.ChannelIDs = v }},
	{"dm_ids", func(s ResourceScope) []string { return s.DMIDs }, func(s *ResourceScope, v []string) { s.DMIDs = v }},
	{"event_ids", func(s ResourceScope) []string { return s.EventIDs }, func(s *ResourceScope, v []string) { s.EventIDs = v }},
	{"thread_ids", func(s ResourceScope) []string { return s.ThreadIDs }, func(s *ResourceScope, v []string) { s.ThreadIDs = v }},
	{"file_ids", func(s ResourceScope) []string { return s.FileIDs }, func(s *ResourceScope, v []string) { s.FileIDs = v }},
	{"workspace_binding_ids", func(s ResourceScope) []string { return s.WorkspaceBindingIDs }, func(s *ResourceScope, v []string) { s.WorkspaceBindingIDs = v }},
	{"workspace_path_prefixes", func(s ResourceScope) []string { return s.WorkspacePathPrefixes }, func(s *ResourceScope, v []string) { s.WorkspacePathPrefixes = v }},
	{"tool_ids", func(s ResourceScope) []string { return s.ToolIDs }, func(s *ResourceScope, v []string) { s.ToolIDs = v }},
}

func TestValidateAndNarrowCapabilities(t *testing.T) {
	t.Parallel()

	at, _, authority := validFixture()
	for _, capability := range allCapabilities {
		capability := capability
		t.Run(strconv.Itoa(int(capability)), func(t *testing.T) {
			request := Request{Capabilities: []Capability{capability}, ExpiresAt: at.Add(time.Minute)}
			got, err := ValidateAndNarrow(at, request, authority)
			if err != nil {
				t.Fatalf("published capability %d rejected: %v", capability, err)
			}
			if want := []Capability{capability}; !reflect.DeepEqual(got.Capabilities(), want) {
				t.Fatalf("Capabilities() = %v, want %v", got.Capabilities(), want)
			}
		})
	}

	requestCases := []struct {
		name         string
		capabilities []Capability
		code         ErrorCode
	}{
		{"empty", nil, ErrorInvalidInput},
		{"unspecified", []Capability{0}, ErrorInvalidInput},
		{"unknown", []Capability{15}, ErrorInvalidInput},
		{"duplicate", []Capability{CapabilityMessageRead, CapabilityMessageRead}, ErrorInvalidInput},
	}
	for _, test := range requestCases {
		t.Run("request_"+test.name, func(t *testing.T) {
			request := Request{Capabilities: test.capabilities, ExpiresAt: at.Add(time.Minute)}
			assertErrorCode(t, test.code, func() (ValidatedRequest, error) {
				return ValidateAndNarrow(at, request, authority)
			})
		})
	}

	t.Run("outside_ceiling", func(t *testing.T) {
		authority.Capabilities = []Capability{CapabilityMessageRead}
		request := Request{Capabilities: []Capability{CapabilityFileRead}, ExpiresAt: at.Add(time.Minute)}
		assertErrorCode(t, ErrorCapabilityDenied, func() (ValidatedRequest, error) {
			return ValidateAndNarrow(at, request, authority)
		})
	})

	for _, test := range requestCases {
		t.Run("authority_"+test.name, func(t *testing.T) {
			invalid := authority
			invalid.Capabilities = test.capabilities
			request := Request{Capabilities: []Capability{CapabilityMessageRead}, ExpiresAt: at.Add(time.Minute)}
			assertErrorCode(t, ErrorInvalidAuthority, func() (ValidatedRequest, error) {
				return ValidateAndNarrow(at, request, invalid)
			})
		})
	}
}

func TestValidateAndNarrowScopeFields(t *testing.T) {
	t.Parallel()

	at, _, baseAuthority := validFixture()
	for index, field := range scopeFields {
		index, field := index, field
		t.Run(field.name, func(t *testing.T) {
			authority := baseAuthority
			authority.Scope = ResourceScope{}
			field.set(&authority.Scope, []string{"value-c", "value-a", "value-b"})

			t.Run("empty_grants_nothing", func(t *testing.T) {
				got, err := ValidateAndNarrow(at, validRequest(at, ResourceScope{}), authority)
				if err != nil {
					t.Fatalf("empty scope rejected: %v", err)
				}
				if values := field.get(got.Scope()); len(values) != 0 {
					t.Fatalf("scope = %v, want empty", values)
				}
			})

			t.Run("exact_subset_is_canonical", func(t *testing.T) {
				requestScope := ResourceScope{}
				field.set(&requestScope, []string{"value-b", "value-a"})
				got, err := ValidateAndNarrow(at, validRequest(at, requestScope), authority)
				if err != nil {
					t.Fatalf("exact subset rejected: %v", err)
				}
				if want := []string{"value-a", "value-b"}; !reflect.DeepEqual(field.get(got.Scope()), want) {
					t.Fatalf("scope = %v, want %v", field.get(got.Scope()), want)
				}
			})

			t.Run("outside_ceiling", func(t *testing.T) {
				requestScope := ResourceScope{}
				field.set(&requestScope, []string{"value-outside"})
				assertErrorCode(t, ErrorScopeDenied, func() (ValidatedRequest, error) {
					return ValidateAndNarrow(at, validRequest(at, requestScope), authority)
				})
			})

			t.Run("cross_kind_substitution", func(t *testing.T) {
				requestScope := ResourceScope{}
				next := scopeFields[(index+1)%len(scopeFields)]
				next.set(&requestScope, []string{"value-a"})
				assertErrorCode(t, ErrorScopeDenied, func() (ValidatedRequest, error) {
					return ValidateAndNarrow(at, validRequest(at, requestScope), authority)
				})
			})

			for name, values := range map[string][]string{
				"duplicate": {"value-a", "value-a"},
				"blank":     {""},
				"untrimmed": {" value-a"},
				"control":   {"value\nsecret"},
				"wildcard":  {"value-*"},
			} {
				t.Run("request_"+name, func(t *testing.T) {
					requestScope := ResourceScope{}
					field.set(&requestScope, values)
					assertErrorCode(t, ErrorInvalidInput, func() (ValidatedRequest, error) {
						return ValidateAndNarrow(at, validRequest(at, requestScope), authority)
					})
				})
				t.Run("authority_"+name, func(t *testing.T) {
					invalid := authority
					field.set(&invalid.Scope, values)
					assertErrorCode(t, ErrorInvalidAuthority, func() (ValidatedRequest, error) {
						return ValidateAndNarrow(at, validRequest(at, ResourceScope{}), invalid)
					})
				})
			}
		})
	}

	t.Run("workspace_paths_are_exact_not_prefix_inferred", func(t *testing.T) {
		authority := baseAuthority
		authority.Scope = ResourceScope{WorkspacePathPrefixes: []string{"repo/src"}}
		request := validRequest(at, ResourceScope{WorkspacePathPrefixes: []string{"repo/src/pkg"}})
		assertErrorCode(t, ErrorScopeDenied, func() (ValidatedRequest, error) {
			return ValidateAndNarrow(at, request, authority)
		})
	})
}

func TestValidateAndNarrowAuthority(t *testing.T) {
	t.Parallel()

	at, request, authority := validFixture()
	identifierFields := []struct {
		name string
		set  func(*IssuanceAuthority, string)
	}{
		{"tenant", func(a *IssuanceAuthority, value string) { a.TenantID = value }},
		{"task", func(a *IssuanceAuthority, value string) { a.TaskID = value }},
		{"run", func(a *IssuanceAuthority, value string) { a.RunID = value }},
		{"device", func(a *IssuanceAuthority, value string) { a.DeviceID = value }},
		{"policy_version", func(a *IssuanceAuthority, value string) { a.PolicyVersion = value }},
	}
	for _, field := range identifierFields {
		for name, value := range map[string]string{
			"blank": "", "untrimmed": " value", "control": "value\x00secret", "wildcard": "value?",
		} {
			t.Run(field.name+"_"+name, func(t *testing.T) {
				invalid := authority
				field.set(&invalid, value)
				assertErrorCode(t, ErrorInvalidAuthority, func() (ValidatedRequest, error) {
					return ValidateAndNarrow(at, request, invalid)
				})
			})
		}
	}

	for _, test := range []struct {
		name   string
		mutate func(*IssuanceAuthority)
	}{
		{"grantee_type", func(a *IssuanceAuthority) { a.Grantee.Type = 0 }},
		{"grantee_id", func(a *IssuanceAuthority) { a.Grantee.ID = "" }},
		{"initiator_type", func(a *IssuanceAuthority) { a.Initiator.Type = 4 }},
		{"initiator_id", func(a *IssuanceAuthority) { a.Initiator.ID = " actor" }},
		{"not_after_zero", func(a *IssuanceAuthority) { a.NotAfter = time.Time{} }},
		{"not_after_equal_at", func(a *IssuanceAuthority) { a.NotAfter = at }},
		{"not_after_before_at", func(a *IssuanceAuthority) { a.NotAfter = at.Add(-time.Second) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := authority
			test.mutate(&invalid)
			assertErrorCode(t, ErrorInvalidAuthority, func() (ValidatedRequest, error) {
				return ValidateAndNarrow(at, request, invalid)
			})
		})
	}

	t.Run("zero_evaluation_time", func(t *testing.T) {
		assertErrorCode(t, ErrorInvalidAuthority, func() (ValidatedRequest, error) {
			return ValidateAndNarrow(time.Time{}, request, authority)
		})
	})
}

func TestValidateAndNarrowExpiry(t *testing.T) {
	t.Parallel()

	at, request, authority := validFixture()
	for _, test := range []struct {
		name    string
		expires time.Time
		code    ErrorCode
	}{
		{"zero", time.Time{}, ErrorInvalidInput},
		{"before_at", at.Add(-time.Second), ErrorExpiryDenied},
		{"equal_at", at, ErrorExpiryDenied},
		{"beyond_ceiling", authority.NotAfter.Add(time.Nanosecond), ErrorExpiryDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := request
			invalid.ExpiresAt = test.expires
			assertErrorCode(t, test.code, func() (ValidatedRequest, error) {
				return ValidateAndNarrow(at, invalid, authority)
			})
		})
	}

	t.Run("exact_ceiling", func(t *testing.T) {
		request.ExpiresAt = authority.NotAfter
		got, err := ValidateAndNarrow(at, request, authority)
		if err != nil {
			t.Fatalf("exact expiry ceiling rejected: %v", err)
		}
		if !got.ExpiresAt().Equal(authority.NotAfter) {
			t.Fatalf("ExpiresAt() = %v, want %v", got.ExpiresAt(), authority.NotAfter)
		}
	})
}

func TestValidateAndNarrowDoesNotAliasInputsOrOutputs(t *testing.T) {
	t.Parallel()

	at, request, authority := validFixture()
	requestBefore := cloneRequest(request)
	authorityBefore := cloneAuthority(authority)
	got, err := ValidateAndNarrow(at, request, authority)
	if err != nil {
		t.Fatalf("ValidateAndNarrow() error = %v", err)
	}
	if !reflect.DeepEqual(request, requestBefore) || !reflect.DeepEqual(authority, authorityBefore) {
		t.Fatal("ValidateAndNarrow mutated an input")
	}
	if !slices.IsSorted(got.Capabilities()) {
		t.Fatalf("Capabilities() is not canonical: %v", got.Capabilities())
	}
	for _, field := range scopeFields {
		if values := field.get(got.Scope()); !slices.IsSorted(values) {
			t.Fatalf("%s is not canonical: %v", field.name, values)
		}
	}

	request.Capabilities[0] = CapabilityAuditRead
	authority.Capabilities[0] = CapabilityAuditRead
	for _, field := range scopeFields {
		field.get(request.Scope)[0] = "mutated-request"
		field.get(authority.Scope)[0] = "mutated-authority"
	}
	if reflect.DeepEqual(got.Capabilities(), request.Capabilities) {
		t.Fatal("validated capabilities alias request input")
	}
	for _, field := range scopeFields {
		if slices.Contains(field.get(got.Scope()), "mutated-request") || slices.Contains(field.get(got.Scope()), "mutated-authority") {
			t.Fatalf("validated %s aliases an input", field.name)
		}
	}

	capabilities := got.Capabilities()
	capabilities[0] = CapabilityAuditRead
	if got.Capabilities()[0] == CapabilityAuditRead {
		t.Fatal("Capabilities accessor aliases stored result")
	}
	for _, field := range scopeFields {
		scope := got.Scope()
		field.get(scope)[0] = "mutated-output"
		if slices.Contains(field.get(got.Scope()), "mutated-output") {
			t.Fatalf("Scope accessor aliases stored %s", field.name)
		}
	}
}

func TestValidateAndNarrowEquivalentOrdersHaveIdenticalOutput(t *testing.T) {
	t.Parallel()

	at, request, authority := validFixture()
	first, err := ValidateAndNarrow(at, request, authority)
	if err != nil {
		t.Fatalf("first ValidateAndNarrow() error = %v", err)
	}
	slices.Reverse(request.Capabilities)
	slices.Reverse(authority.Capabilities)
	for _, field := range scopeFields {
		slices.Reverse(field.get(request.Scope))
		slices.Reverse(field.get(authority.Scope))
	}
	second, err := ValidateAndNarrow(at, request, authority)
	if err != nil {
		t.Fatalf("second ValidateAndNarrow() error = %v", err)
	}
	if !equalValidated(first, second) {
		t.Fatalf("equivalent logical inputs differ:\nfirst: %#v\nsecond: %#v", first, second)
	}
}

func TestValidateAndNarrowErrorsAreStableAndSecretSafe(t *testing.T) {
	t.Parallel()

	at, request, authority := validFixture()
	secret := "never-leak-this-marker"
	tests := []struct {
		name string
		code ErrorCode
		run  func() (ValidatedRequest, error)
	}{
		{"invalid_input", ErrorInvalidInput, func() (ValidatedRequest, error) {
			invalid := request
			invalid.Scope.ChannelIDs = []string{secret + "*"}
			return ValidateAndNarrow(at, invalid, authority)
		}},
		{"invalid_authority", ErrorInvalidAuthority, func() (ValidatedRequest, error) {
			invalid := authority
			invalid.TenantID = secret + "*"
			return ValidateAndNarrow(at, request, invalid)
		}},
		{"capability_denied", ErrorCapabilityDenied, func() (ValidatedRequest, error) {
			invalid := authority
			invalid.Capabilities = []Capability{CapabilityFileRead}
			return ValidateAndNarrow(at, request, invalid)
		}},
		{"scope_denied", ErrorScopeDenied, func() (ValidatedRequest, error) {
			invalid := request
			invalid.Scope.ChannelIDs = []string{secret}
			return ValidateAndNarrow(at, invalid, authority)
		}},
		{"expiry_denied", ErrorExpiryDenied, func() (ValidatedRequest, error) {
			invalid := request
			invalid.ExpiresAt = authority.NotAfter.Add(time.Second)
			return ValidateAndNarrow(at, invalid, authority)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.run()
			var policyError *Error
			if !errors.As(err, &policyError) || policyError.Code() != test.code {
				t.Fatalf("error = %v, want code %q", err, test.code)
			}
			if want := "capability grant policy: " + string(test.code); err.Error() != want {
				t.Fatalf("Error() = %q, want %q", err.Error(), want)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("Error() leaked input: %q", err)
			}
		})
	}
}

func FuzzValidateAndNarrowNeverWidens(f *testing.F) {
	f.Add([]byte{0, 0, 0})
	f.Add([]byte{0xff, 0xff, 0})
	f.Add([]byte{0x55, 0xaa, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		at, _, authority := validFixture()
		request := Request{ExpiresAt: at.Add(time.Minute)}
		for index, capability := range authority.Capabilities {
			if len(data) == 0 || data[index%len(data)]&(1<<uint(index%8)) != 0 {
				request.Capabilities = append(request.Capabilities, capability)
			}
		}
		if len(request.Capabilities) == 0 {
			request.Capabilities = []Capability{CapabilityMessageRead}
		}
		for index, field := range scopeFields {
			ceiling := field.get(authority.Scope)
			selected := make([]string, 0, len(ceiling))
			for valueIndex, value := range ceiling {
				if len(data) == 0 || data[(index+valueIndex)%len(data)]&1 != 0 {
					selected = append(selected, value)
				}
			}
			field.set(&request.Scope, selected)
		}
		if len(data) > 0 && data[len(data)-1]&2 != 0 {
			request.Scope.ToolIDs = append(request.Scope.ToolIDs, "outside-ceiling")
		}

		got, err := ValidateAndNarrow(at, request, authority)
		if err != nil {
			return
		}
		for _, capability := range got.Capabilities() {
			if !slices.Contains(authority.Capabilities, capability) {
				t.Fatalf("successful output widened capability %d", capability)
			}
		}
		if !slices.IsSorted(got.Capabilities()) {
			t.Fatalf("successful output capabilities are not canonical: %v", got.Capabilities())
		}
		for _, field := range scopeFields {
			values := field.get(got.Scope())
			if !slices.IsSorted(values) {
				t.Fatalf("successful output %s is not canonical: %v", field.name, values)
			}
			for _, value := range values {
				if !slices.Contains(field.get(authority.Scope), value) {
					t.Fatalf("successful output widened %s with %q", field.name, value)
				}
			}
		}
		if !at.Before(got.ExpiresAt()) || got.ExpiresAt().After(authority.NotAfter) {
			t.Fatalf("successful output widened expiry to %v", got.ExpiresAt())
		}
	})
}

func validFixture() (time.Time, Request, IssuanceAuthority) {
	at := time.Date(2026, time.August, 24, 1, 0, 0, 0, time.UTC)
	scope := ResourceScope{}
	for _, field := range scopeFields {
		field.set(&scope, []string{
			field.name + "-c",
			field.name + "-a",
			field.name + "-b",
		})
	}
	authority := IssuanceAuthority{
		TenantID:      "tenant-a",
		TaskID:        "task-a",
		RunID:         "run-a",
		DeviceID:      "device-a",
		Grantee:       ActorRef{Type: ActorTypeAgent, ID: "agent-a"},
		Initiator:     ActorRef{Type: ActorTypeHuman, ID: "human-a"},
		PolicyVersion: "policy-v1",
		Capabilities:  append([]Capability(nil), allCapabilities...),
		Scope:         scope,
		NotAfter:      at.Add(time.Hour),
	}
	requestScope := ResourceScope{}
	for _, field := range scopeFields {
		values := field.get(scope)
		field.set(&requestScope, []string{values[1], values[2]})
	}
	request := Request{
		Capabilities: []Capability{CapabilityFileRead, CapabilityMessageRead},
		Scope:        requestScope,
		ExpiresAt:    at.Add(15 * time.Minute),
	}
	return at, request, authority
}

func validRequest(at time.Time, scope ResourceScope) Request {
	return Request{
		Capabilities: []Capability{CapabilityMessageRead},
		Scope:        scope,
		ExpiresAt:    at.Add(time.Minute),
	}
}

func assertErrorCode(t *testing.T, want ErrorCode, validate func() (ValidatedRequest, error)) {
	t.Helper()
	_, err := validate()
	var policyError *Error
	if !errors.As(err, &policyError) || policyError.Code() != want {
		t.Fatalf("error = %v, want code %q", err, want)
	}
}

func cloneRequest(request Request) Request {
	request.Capabilities = append([]Capability(nil), request.Capabilities...)
	request.Scope = cloneScope(request.Scope)
	return request
}

func cloneAuthority(authority IssuanceAuthority) IssuanceAuthority {
	authority.Capabilities = append([]Capability(nil), authority.Capabilities...)
	authority.Scope = cloneScope(authority.Scope)
	return authority
}

func equalValidated(left, right ValidatedRequest) bool {
	return left.TenantID() == right.TenantID() &&
		left.TaskID() == right.TaskID() &&
		left.RunID() == right.RunID() &&
		left.DeviceID() == right.DeviceID() &&
		left.Grantee() == right.Grantee() &&
		left.Initiator() == right.Initiator() &&
		left.PolicyVersion() == right.PolicyVersion() &&
		left.ExpiresAt().Equal(right.ExpiresAt()) &&
		reflect.DeepEqual(left.Capabilities(), right.Capabilities()) &&
		reflect.DeepEqual(left.Scope(), right.Scope())
}
