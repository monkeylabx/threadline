// Package operation evaluates pure non-approval Task operation and Runtime
// policy guards over bound principal, disclosure, and typed fact sections.
package operation

import (
	"time"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

const (
	ReasonParentStateDenied      model.Reason = "parent-state-denied"
	ReasonActionDisabled         model.Reason = "action-disabled"
	ReasonPolicyAudienceDenied   model.Reason = "policy-audience-denied"
	ReasonPolicyActorDenied      model.Reason = "policy-actor-denied"
	ReasonParentPublishDenied    model.Reason = "parent-publish-denied"
	ReasonPrincipalRouteDenied   model.Reason = "principal-route-denied"
	ReasonOrganizationCeiling    model.Reason = "organization-task-ceiling"
	ReasonChannelCeiling         model.Reason = "channel-task-ceiling"
	ReasonTaskRoleDenied         model.Reason = "task-role-denied"
	ReasonSourceErased           model.Reason = "source-erased"
	ReasonSourceHistoryDenied    model.Reason = "source-history-denied"
	ReasonTargetBindingDenied    model.Reason = "target-binding-denied"
	ReasonUnsupportedContext     model.Reason = "unsupported-context"
	ReasonUnsupportedTool        model.Reason = "unsupported-tool"
	ReasonPayloadTooLarge        model.Reason = "payload-too-large"
	ReasonParticipantSetInvalid  model.Reason = "participant-set-invalid"
	ReasonDelegationDenied       model.Reason = "delegation-denied"
	ReasonScopeExpanded          model.Reason = "scope-expanded"
	ReasonNotExecutionOwner      model.Reason = "not-execution-owner"
	ReasonLeaseLost              model.Reason = "lease-lost"
	ReasonFenceStale             model.Reason = "fence-stale"
	ReasonExecutionGrantRequired model.Reason = "execution-grant-required"
	ReasonGrantExpired           model.Reason = "grant-expired"
	ReasonGrantRevoked           model.Reason = "grant-revoked"
	ReasonGrantSignatureInvalid  model.Reason = "grant-signature-invalid"
)

type GrantState string

const (
	GrantStateActive  GrantState = "ACTIVE"
	GrantStateExpired GrantState = "EXPIRED"
	GrantStateRevoked GrantState = "REVOKED"
)

// Facts bundles typed non-approval operation and runtime evidence sections.
type Facts struct {
	Approved     bool
	Create       CreateFacts
	Source       SourceFacts
	Target       TargetFacts
	Delegation   DelegationFacts
	ScopeRequest ScopeRequest
	Runtime      RuntimeFacts
	Grant        GrantFacts
}
type CreateFacts struct {
	AdditionalReferences, HasParticipantTimestamps bool
	Participants, DerivedParticipants              []model.Participant
}
type SourceFacts struct {
	model.Bound
	EventID                                            string
	Retained, InCurrentInterval, HistorySharingCurrent bool
}
type TargetFacts struct {
	model.Bound
	Enrollment, Device                                                                       string
	RuntimeGeneration, ExpectedRuntimeGeneration, DeviceGeneration, ExpectedDeviceGeneration uint64
	Agent, Operator                                                                          model.Actor
	OperatorOrgRole                                                                          standing.OrganizationRole
	AgentEnabled, OperatorMemberActive, DeviceCurrent, PermittedByPolicy, Online             bool
}
type AuthorityCeiling struct {
	Capabilities []grantpolicy.Capability
	Scope        grantpolicy.ResourceScope
	NotAfter     time.Time
}
type DelegationFacts struct {
	model.Bound
	Initiator, Agent                                               model.Actor
	Current, InitiatorAuthorityCurrent                             bool
	SessionBound, SessionCurrent, DeviceBound, DeviceCurrent       bool
	ExpiresAt                                                      time.Time
	ConfiguredSyntheticToolID                                      string
	PolicyCeiling, InitiatorCeiling, AgentCeiling, ResourceCeiling AuthorityCeiling
}
type ScopeRequest struct {
	Capabilities                  []grantpolicy.Capability
	Scope                         grantpolicy.ResourceScope
	ExpiresAt                     time.Time
	ActionAdapter                 string
	ArgumentsEmpty, UnknownFields bool
}
type RuntimeFacts struct {
	model.Bound
	Verified, AgentMemberCurrent, OperatorMemberCurrent, LeaseCurrent, FenceCurrent bool
	AssignedAgent                                                                   model.Actor
	AssignedRun, ExecutionOwner                                                     string
	RuntimeGeneration                                                               uint64
	OperatorOrgRole                                                                 standing.OrganizationRole
}
type GrantFacts struct {
	model.Bound
	Present, SignatureValid bool
	State                   GrantState
	ExpiresAt               time.Time
}
