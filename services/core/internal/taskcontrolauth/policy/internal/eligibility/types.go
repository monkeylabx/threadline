// Package eligibility computes and verifies current Task Approval Human
// eligibility intersections bound to an immutable Approval action and snapshot.
// It never authenticates callers, reads persistence, or issues Grants.
package eligibility

import (
	"time"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

const (
	ApprovalScopeOnce ApprovalScope  = "ONCE"
	ApprovalPending   ApprovalStatus = "PENDING"
	ApprovalApproved  ApprovalStatus = "APPROVED"
	ApprovalDenied    ApprovalStatus = "DENIED"
	ApprovalRevoked   ApprovalStatus = "REVOKED"
	ApprovalConsumed  ApprovalStatus = "CONSUMED"

	ReasonActionDisabled         model.Reason = "action-disabled"
	ReasonPolicyActorDenied      model.Reason = "policy-actor-denied"
	ReasonParentStateDenied      model.Reason = "parent-state-denied"
	ReasonOrgTaskCeiling         model.Reason = "organization-task-ceiling"
	ReasonChannelTaskCeiling     model.Reason = "channel-task-ceiling"
	ReasonApproverRoleDenied     model.Reason = "approver-role-denied"
	ReasonSelfApprovalDenied     model.Reason = "self-approval-denied"
	ReasonApproverSnapshotDenied model.Reason = "approver-snapshot-denied"
	ReasonApproverCurrentDenied  model.Reason = "approver-current-denied"
	ReasonScopeExpanded          model.Reason = "scope-expanded"
	ReasonUnsupportedMode        model.Reason = "unsupported-mode"
	ReasonSourceHistoryDenied    model.Reason = "source-history-denied"
	ReasonGrantExpired           model.Reason = "grant-expired"
	ReasonGrantRevoked           model.Reason = "grant-revoked"
	ReasonInvalidStateTransition model.Reason = "invalid-state-transition"
)

type (
	ApprovalScope  string
	ApprovalStatus string
	ActionBinding  struct {
		Bound                                         model.Bound
		ActionID, ActionVersion, AdapterID, Operation string
		Commitment                                    [32]byte
		Capabilities                                  []grantpolicy.Capability
		Scope                                         grantpolicy.ResourceScope
		ApprovalScope                                 ApprovalScope
	}
	PolicyCeiling struct {
		Available    bool
		Capabilities []grantpolicy.Capability
		Scope        grantpolicy.ResourceScope
	}
	CandidateFacts struct {
		Member                                   model.MemberFacts
		Membership                               standing.ChannelMembershipFacts
		ActorDenied, SourceEntitled              bool
		AuthorityAvailable, RequireDeviceCurrent bool
		Capabilities                             []grantpolicy.Capability
		Scope                                    grantpolicy.ResourceScope
		Device                                   model.DeviceFacts
	}
	EligibilityFacts struct {
		Scope         model.Scope
		ObservedAt    time.Time
		Organization  model.OrganizationFacts
		Parent        model.ParentFacts
		Policy        model.PolicyFacts
		Task          model.TaskFacts
		PolicyCeiling PolicyCeiling
		Candidates    map[model.Actor]CandidateFacts
	}
	EligibleApprover struct {
		Actor            model.Actor
		TaskRole         model.TaskRole
		OrganizationRole standing.OrganizationRole
		ChannelRole      standing.ChannelRole
	}
	Snapshot struct {
		Bound                                   model.Bound
		ActionID, ActionVersion, PolicyRevision string
		Commitment                              [32]byte
		Eligible                                []EligibleApprover
	}
	StoredApproval struct {
		Bound                                   model.Bound
		Action                                  ActionBinding
		Snapshot                                Snapshot
		Status                                  ApprovalStatus
		ExpiresAt                               time.Time
		Decider                                 model.Actor
		DecidedDevice                           string
		RequireDeciderDevice                    bool
		RunBlocked, CancelIntent, SafetyBarrier bool
	}
	DecisionRequest struct {
		Caller        model.Actor
		Action        model.Action
		Approved      bool
		ApprovalScope ApprovalScope
	}
	Result struct {
		Refusal               model.Refusal
		Scope                 model.Scope
		PolicyRevision        string
		Eligible              []EligibleApprover
		Snapshot              Snapshot
		Decider               *EligibleApprover
		UsedOwnerSelfApproval bool
	}
)

func refuse(r model.Reason, s model.Status, c model.Code) model.Refusal {
	return model.Refusal{Reason: r, Status: s, Code: c}
}
func firstRef(rs ...model.Refusal) model.Refusal {
	for _, r := range rs {
		if r.Refused() {
			return r
		}
	}
	return model.Refusal{}
}
func cond(fail bool, r model.Refusal) model.Refusal {
	if fail {
		return r
	}
	return model.Refusal{}
}
