package grantpolicy

import (
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ValidateAndNarrow validates an untrusted request against trusted authority
// and returns an immutable canonical projection of the requested subset.
func ValidateAndNarrow(at time.Time, request Request, authority IssuanceAuthority) (ValidatedRequest, error) {
	if !validAuthority(at, authority) {
		return ValidatedRequest{}, &Error{code: ErrorInvalidAuthority}
	}
	if request.ExpiresAt.IsZero() || !validCapabilities(request.Capabilities) || !validScope(request.Scope) {
		return ValidatedRequest{}, &Error{code: ErrorInvalidInput}
	}
	if !capabilitySubset(request.Capabilities, authority.Capabilities) {
		return ValidatedRequest{}, &Error{code: ErrorCapabilityDenied}
	}
	if !scopeSubset(request.Scope, authority.Scope) {
		return ValidatedRequest{}, &Error{code: ErrorScopeDenied}
	}
	if !at.Before(request.ExpiresAt) || request.ExpiresAt.After(authority.NotAfter) {
		return ValidatedRequest{}, &Error{code: ErrorExpiryDenied}
	}

	capabilities := append([]Capability(nil), request.Capabilities...)
	slices.Sort(capabilities)
	scope := cloneScope(request.Scope)
	sortScope(&scope)

	return ValidatedRequest{
		tenantID:      authority.TenantID,
		taskID:        authority.TaskID,
		runID:         authority.RunID,
		deviceID:      authority.DeviceID,
		grantee:       authority.Grantee,
		initiator:     authority.Initiator,
		policyVersion: authority.PolicyVersion,
		capabilities:  capabilities,
		scope:         scope,
		expiresAt:     request.ExpiresAt.UTC(),
	}, nil
}

func validAuthority(at time.Time, authority IssuanceAuthority) bool {
	if at.IsZero() || authority.NotAfter.IsZero() || !at.Before(authority.NotAfter) {
		return false
	}
	for _, value := range []string{
		authority.TenantID,
		authority.TaskID,
		authority.RunID,
		authority.DeviceID,
		authority.PolicyVersion,
	} {
		if !validIdentifier(value) {
			return false
		}
	}
	if !validActor(authority.Grantee) || !validActor(authority.Initiator) {
		return false
	}
	return validCapabilities(authority.Capabilities) && validScope(authority.Scope)
}

func validActor(actor ActorRef) bool {
	return actor.Type >= ActorTypeHuman && actor.Type <= ActorTypeService && validIdentifier(actor.ID)
}

func validCapabilities(capabilities []Capability) bool {
	if len(capabilities) == 0 {
		return false
	}
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if capability < CapabilityMessageRead || capability > CapabilityAuditRead {
			return false
		}
		if _, duplicate := seen[capability]; duplicate {
			return false
		}
		seen[capability] = struct{}{}
	}
	return true
}

func validScope(scope ResourceScope) bool {
	return validIdentifiers(scope.ChannelIDs) &&
		validIdentifiers(scope.DMIDs) &&
		validIdentifiers(scope.EventIDs) &&
		validIdentifiers(scope.ThreadIDs) &&
		validIdentifiers(scope.FileIDs) &&
		validIdentifiers(scope.WorkspaceBindingIDs) &&
		validIdentifiers(scope.WorkspacePathPrefixes) &&
		validIdentifiers(scope.ToolIDs)
}

func validIdentifiers(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validIdentifier(value) {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validIdentifier(value string) bool {
	if !utf8.ValidString(value) || value == "" || strings.TrimSpace(value) != value ||
		strings.ContainsAny(value, "*?") {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func capabilitySubset(requested, ceiling []Capability) bool {
	allowed := make(map[Capability]struct{}, len(ceiling))
	for _, capability := range ceiling {
		allowed[capability] = struct{}{}
	}
	for _, capability := range requested {
		if _, ok := allowed[capability]; !ok {
			return false
		}
	}
	return true
}

func scopeSubset(requested, ceiling ResourceScope) bool {
	return stringSubset(requested.ChannelIDs, ceiling.ChannelIDs) &&
		stringSubset(requested.DMIDs, ceiling.DMIDs) &&
		stringSubset(requested.EventIDs, ceiling.EventIDs) &&
		stringSubset(requested.ThreadIDs, ceiling.ThreadIDs) &&
		stringSubset(requested.FileIDs, ceiling.FileIDs) &&
		stringSubset(requested.WorkspaceBindingIDs, ceiling.WorkspaceBindingIDs) &&
		stringSubset(requested.WorkspacePathPrefixes, ceiling.WorkspacePathPrefixes) &&
		stringSubset(requested.ToolIDs, ceiling.ToolIDs)
}

func stringSubset(requested, ceiling []string) bool {
	allowed := make(map[string]struct{}, len(ceiling))
	for _, value := range ceiling {
		allowed[value] = struct{}{}
	}
	for _, value := range requested {
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}

func sortScope(scope *ResourceScope) {
	slices.Sort(scope.ChannelIDs)
	slices.Sort(scope.DMIDs)
	slices.Sort(scope.EventIDs)
	slices.Sort(scope.ThreadIDs)
	slices.Sort(scope.FileIDs)
	slices.Sort(scope.WorkspaceBindingIDs)
	slices.Sort(scope.WorkspacePathPrefixes)
	slices.Sort(scope.ToolIDs)
}
