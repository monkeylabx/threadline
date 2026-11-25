package eligibility

import (
	"context"
	"time"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

// ValidateDecision verifies that an authenticated Human caller may approve,
// reject, or revoke an existing Approval within the snapshot ∩ current intersection.
func ValidateDecision(ctx context.Context, req DecisionRequest, stored StoredApproval, f EligibilityFacts) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	_, hasAct := f.Policy.Rules[req.Action]
	if r := firstRef(
		validateDecisionShape(req),
		validateStoredAndCommon(stored, f),
		cond(!hasAct, refuse(ReasonActionDisabled, model.StatusPermissionDenied, model.CodePermissionDenied)),
		cond(f.Parent.State == standing.ResourceStatePendingDeletion || (f.Parent.State == standing.ResourceStateArchived && req.Action == ceiling.ApprovalDecide && req.Approved), refuse(ReasonParentStateDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
	); r.Refused() {
		return Result{Refusal: r}, nil
	}
	decider, r := evaluateActor(req.Action, req.Caller, stored.Action, &stored.Snapshot, f)
	if r.Refused() {
		return Result{Refusal: r}, nil
	}
	if r := checkDecisionLifecycle(req, stored, f.ObservedAt); r.Refused() {
		return Result{Refusal: r}, nil
	}
	return intersectedResult(f, stored, decider), nil
}

// ValidateDeciderForUse verifies at protected use that the recorded deciding
// Human remains in the snapshot ∩ current intersection without requiring a Human Session.
func ValidateDeciderForUse(ctx context.Context, stored StoredApproval, f EligibilityFacts) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	cand, ok := f.Candidates[stored.Decider]
	_, hasDecide := f.Policy.Rules[ceiling.ApprovalDecide]
	if r := firstRef(
		validateStoredAndCommon(stored, f),
		cond(stored.Decider.Type != rpcmiddleware.ActorTypeHuman || stored.Decider.ID == "", refBind),
		cond(!ok, refUnav),
		cond(stored.RequireDeciderDevice && (!cand.RequireDeviceCurrent || cand.Device.Device != stored.DecidedDevice), refBind),
		checkUseLifecycle(stored, f.ObservedAt),
		cond(!hasDecide, refuse(ReasonActionDisabled, model.StatusPermissionDenied, model.CodePermissionDenied)),
	); r.Refused() {
		return Result{Refusal: r}, nil
	}
	decider, r := evaluateActor(ceiling.ApprovalDecide, stored.Decider, stored.Action, &stored.Snapshot, f)
	if r.Refused() {
		return Result{Refusal: r}, nil
	}
	return intersectedResult(f, stored, decider), nil
}

func validateDecisionShape(req DecisionRequest) model.Refusal {
	return firstRef(
		cond(req.Action != ceiling.ApprovalDecide && req.Action != ceiling.ApprovalRevoke, refuse(binding.ReasonUnknownAction, model.StatusUnimplemented, "")),
		validateApprovalMode(req.ApprovalScope),
		cond(req.Caller.Type != rpcmiddleware.ActorTypeHuman || req.Caller.ID == "", refuse(ReasonApproverRoleDenied, model.StatusPermissionDenied, model.CodePermissionDenied)),
	)
}

func validateStoredAndCommon(stored StoredApproval, f EligibilityFacts) model.Refusal {
	sb := stored.Snapshot.Bound
	entriesOK := validUnique(stored.Snapshot.Eligible, func(e EligibleApprover) model.Actor { return e.Actor }, func(e EligibleApprover) bool {
		return e.Actor.Type == rpcmiddleware.ActorTypeHuman && e.Actor.ID != "" && (e.TaskRole == ceiling.Approver || e.TaskRole == ceiling.Owner)
	})
	return firstRef(
		cond(f.Scope.Ancestry.Approval == "", refUnav),
		validateCommonFacts(f, stored.Action),
		validateProvidedCandidates(f),
		binding.Section(f.Scope, stored.Bound, model.LevelApproval),
		cond(sb.Snapshot.TransactionID == "" || stored.Snapshot.PolicyRevision == "", refUnav),
		cond(sb.Tenant != f.Scope.Tenant || sb.Ancestry.Truncate(model.LevelRun) != f.Scope.Ancestry.Truncate(model.LevelRun) ||
			stored.Snapshot.ActionID != stored.Action.ActionID || stored.Snapshot.ActionVersion != stored.Action.ActionVersion ||
			stored.Snapshot.Commitment != stored.Action.Commitment || !entriesOK, refBind),
	)
}

func checkDecisionLifecycle(req DecisionRequest, stored StoredApproval, now time.Time) model.Refusal {
	if !stored.ExpiresAt.IsZero() && !now.Before(stored.ExpiresAt) {
		return refuse(ReasonGrantExpired, model.StatusPermissionDenied, model.CodeGrantExpired)
	}
	if req.Action == ceiling.ApprovalRevoke {
		return firstRef(
			cond(stored.Status == ApprovalRevoked, refuse(ReasonGrantRevoked, model.StatusPermissionDenied, model.CodeGrantRevoked)),
			cond(stored.Status != ApprovalPending && stored.Status != ApprovalApproved, refuse(ReasonInvalidStateTransition, model.StatusFailedPrecondition, model.CodeInvalidStateTransition)),
		)
	}
	return cond(stored.Status != ApprovalPending || (req.Approved && (stored.RunBlocked || stored.CancelIntent || stored.SafetyBarrier)), refuse(ReasonInvalidStateTransition, model.StatusFailedPrecondition, model.CodeInvalidStateTransition))
}

func checkUseLifecycle(stored StoredApproval, now time.Time) model.Refusal {
	return firstRef(
		cond(stored.Status == ApprovalRevoked, refuse(ReasonGrantRevoked, model.StatusPermissionDenied, model.CodeGrantRevoked)),
		cond(stored.Status != ApprovalApproved || stored.RunBlocked || stored.CancelIntent || stored.SafetyBarrier, refuse(ReasonInvalidStateTransition, model.StatusFailedPrecondition, model.CodeInvalidStateTransition)),
		cond(stored.ExpiresAt.IsZero(), refUnav),
		cond(!now.Before(stored.ExpiresAt), refuse(ReasonGrantExpired, model.StatusPermissionDenied, model.CodeGrantExpired)),
	)
}

func intersectedResult(f EligibilityFacts, stored StoredApproval, decider EligibleApprover) Result {
	out := []EligibleApprover{decider}
	for _, e := range stored.Snapshot.Eligible {
		if _, ok := f.Candidates[e.Actor]; !ok || e.Actor == decider.Actor {
			continue
		}
		if app, r := evaluateActor(ceiling.ApprovalDecide, e.Actor, stored.Action, &stored.Snapshot, f); !r.Refused() {
			out = append(out, app)
		}
	}
	sortEligible(out)
	return Result{Scope: f.Scope, PolicyRevision: f.Policy.Revision, Eligible: out, Snapshot: stored.Snapshot, Decider: &decider, UsedOwnerSelfApproval: decider.TaskRole == ceiling.Owner}
}
