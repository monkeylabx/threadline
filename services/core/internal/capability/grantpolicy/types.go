// Package grantpolicy validates untrusted Capability Grant requests against a
// trusted issuance ceiling. It does not create, sign, or persist grants.
package grantpolicy

import "time"

// Capability mirrors the published Protocol capability values.
type Capability int32

const (
	CapabilityMessageRead         Capability = 1
	CapabilityMessageReadHistory  Capability = 2
	CapabilityMessagePublish      Capability = 3
	CapabilityMessagePublishDraft Capability = 4
	CapabilityFileRead            Capability = 5
	CapabilityFileWrite           Capability = 6
	CapabilityWorkspaceRead       Capability = 7
	CapabilityWorkspaceWrite      Capability = 8
	CapabilityToolInvoke          Capability = 9
	CapabilityActionExecute       Capability = 10
	CapabilityAgentDelegate       Capability = 11
	CapabilityMemoryRetain        Capability = 12
	CapabilityTaskApprove         Capability = 13
	CapabilityAuditRead           Capability = 14
)

// ActorType mirrors the published Protocol actor types.
type ActorType int32

const (
	ActorTypeHuman   ActorType = 1
	ActorTypeAgent   ActorType = 2
	ActorTypeService ActorType = 3
)

// ActorRef identifies one trusted actor binding.
type ActorRef struct {
	Type ActorType
	ID   string
}

// ResourceScope names exact resources. Empty fields grant nothing, and an
// entry never implies descendants or resources of another kind.
type ResourceScope struct {
	ChannelIDs            []string
	DMIDs                 []string
	EventIDs              []string
	ThreadIDs             []string
	FileIDs               []string
	WorkspaceBindingIDs   []string
	WorkspacePathPrefixes []string
	ToolIDs               []string
}

// Request contains the only facts an untrusted requester may control.
type Request struct {
	Capabilities []Capability
	Scope        ResourceScope
	ExpiresAt    time.Time
}

// IssuanceAuthority contains trusted bindings and maximum authority.
type IssuanceAuthority struct {
	TenantID      string
	TaskID        string
	RunID         string
	DeviceID      string
	Grantee       ActorRef
	Initiator     ActorRef
	PolicyVersion string
	Capabilities  []Capability
	Scope         ResourceScope
	NotAfter      time.Time
}

// ErrorCode is a stable, secret-safe validation failure category.
type ErrorCode string

const (
	ErrorInvalidInput     ErrorCode = "invalid-input"
	ErrorInvalidAuthority ErrorCode = "invalid-authority"
	ErrorCapabilityDenied ErrorCode = "capability-denied"
	ErrorScopeDenied      ErrorCode = "scope-denied"
	ErrorExpiryDenied     ErrorCode = "expiry-denied"
)

// Error reports a policy failure without including input values.
type Error struct{ code ErrorCode }

func (e *Error) Error() string { return "capability grant policy: " + string(e.Code()) }

// Code returns the stable failure category.
func (e *Error) Code() ErrorCode {
	if e == nil {
		return ""
	}
	return e.code
}

// ValidatedRequest is an immutable, canonically ordered validation result. It
// is not a Capability Grant or credential.
type ValidatedRequest struct {
	tenantID      string
	taskID        string
	runID         string
	deviceID      string
	grantee       ActorRef
	initiator     ActorRef
	policyVersion string
	capabilities  []Capability
	scope         ResourceScope
	expiresAt     time.Time
}

func (r ValidatedRequest) TenantID() string      { return r.tenantID }
func (r ValidatedRequest) TaskID() string        { return r.taskID }
func (r ValidatedRequest) RunID() string         { return r.runID }
func (r ValidatedRequest) DeviceID() string      { return r.deviceID }
func (r ValidatedRequest) Grantee() ActorRef     { return r.grantee }
func (r ValidatedRequest) Initiator() ActorRef   { return r.initiator }
func (r ValidatedRequest) PolicyVersion() string { return r.policyVersion }
func (r ValidatedRequest) ExpiresAt() time.Time  { return r.expiresAt }
func (r ValidatedRequest) Capabilities() []Capability {
	return append([]Capability(nil), r.capabilities...)
}
func (r ValidatedRequest) Scope() ResourceScope { return cloneScope(r.scope) }

func cloneScope(scope ResourceScope) ResourceScope {
	return ResourceScope{
		ChannelIDs:            append([]string(nil), scope.ChannelIDs...),
		DMIDs:                 append([]string(nil), scope.DMIDs...),
		EventIDs:              append([]string(nil), scope.EventIDs...),
		ThreadIDs:             append([]string(nil), scope.ThreadIDs...),
		FileIDs:               append([]string(nil), scope.FileIDs...),
		WorkspaceBindingIDs:   append([]string(nil), scope.WorkspaceBindingIDs...),
		WorkspacePathPrefixes: append([]string(nil), scope.WorkspacePathPrefixes...),
		ToolIDs:               append([]string(nil), scope.ToolIDs...),
	}
}
