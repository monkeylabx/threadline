package operation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/operation"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

var (
	baseObs    = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	baseSnap   = model.Snapshot{TransactionID: "tx-7", Revision: 7}
	humanA     = model.Actor{Type: rpcmiddleware.ActorTypeHuman, ID: "synthetic-human-a"}
	humanOwner = model.Actor{Type: rpcmiddleware.ActorTypeHuman, ID: "synthetic-human-owner"}
	agentA     = model.Actor{Type: rpcmiddleware.ActorTypeAgent, ID: "synthetic-agent-a"}
	fullAnc    = model.Ancestry{Channel: "synthetic-channel", Task: "synthetic-task", Run: "synthetic-run", Approval: "synthetic-approval"}
)

type fixture struct {
	p   model.Principal
	req model.Request
	f   model.Facts
	op  operation.Facts
}
type negCase struct {
	name, kind string
	fn         func(*fixture)
	reason     model.Reason
	status     model.Status
	code       model.Code
}

func newHuman() fixture {
	b, chB, runB := model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant"}, model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelChannel)}, model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelRun)}
	chRef := standing.ResourceRef{TenantID: "synthetic-tenant", Kind: standing.ResourceKindChannel, ID: "synthetic-channel"}
	ceil := operation.AuthorityCeiling{Capabilities: []grantpolicy.Capability{grantpolicy.CapabilityMessageRead, grantpolicy.CapabilityActionExecute}, Scope: grantpolicy.ResourceScope{ChannelIDs: []string{"synthetic-channel"}, EventIDs: []string{"evt-1"}, ThreadIDs: []string{"thr-1"}, ToolIDs: []string{"noop-tool"}}, NotAfter: baseObs.Add(time.Hour)}
	rules := map[model.Action]model.PolicyRule{}
	for _, a := range []model.Action{ceiling.TaskCreate, ceiling.TaskRead, ceiling.RunStart, ceiling.RunRead, ceiling.RunControl, ceiling.ApprovalRead, ceiling.ApprovalRequest, ceiling.ApprovalDecide, ceiling.ApprovalRevoke, ceiling.RunReport} {
		rules[a] = model.PolicyRule{Audiences: []model.ReadAudience{model.ReadChannelReaders}, Roles: []model.TaskRole{ceiling.Owner, ceiling.Approver, ceiling.Observer}}
	}
	return fixture{
		p:   model.Principal{Route: ceiling.Human, Tenant: "synthetic-tenant", Actor: humanA, Device: "synthetic-device-a", SessionGeneration: 7, Audience: model.AudienceHumanSession, ExpiresAt: baseObs.Add(time.Hour)},
		req: model.Request{Action: ceiling.TaskRead, Purpose: model.PurposeOperation, ParentKind: model.ParentChannel, Resource: model.ResourceRef{Kind: model.ResourceTask, ID: "synthetic-task"}},
		f: model.Facts{
			Snapshot: baseSnap, ObservedAt: baseObs, Device: model.DeviceFacts{Bound: b, Owner: humanA, Device: "synthetic-device-a", Current: true}, Organization: model.OrganizationFacts{Bound: b, Active: true, PolicyVersion: "7"}, Member: model.MemberFacts{Bound: b, Actor: humanA, Active: true, Role: standing.OrganizationRoleMember},
			Lookup: model.LookupFacts{Bound: model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelTask)}, Found: true, Resource: model.ResourceRef{Kind: model.ResourceTask, ID: "synthetic-task"}},
			Parent: model.ParentFacts{Bound: chB, State: standing.ResourceStateActive, ACL: standing.ResourceACLFacts{Resource: chRef, Version: "7", DefaultEffect: standing.ACLEffectAllow}, Membership: standing.ChannelMembershipFacts{TenantID: "synthetic-tenant", ChannelID: "synthetic-channel", Actor: humanA, Current: true, IntervalID: "7", Role: standing.ChannelRoleMember}, Decisions: []standing.Decision{{Effect: standing.EffectAllow, Reason: standing.ReasonAllowed, Action: standing.ActionChannelRead, Resource: chRef, Actor: humanA, PolicyVersion: "7", ACLVersion: "7"}, {Effect: standing.EffectAllow, Reason: standing.ReasonAllowed, Action: standing.ActionChannelPublish, Resource: chRef, Actor: humanA, PolicyVersion: "7", ACLVersion: "7"}}},
			Policy: model.PolicyFacts{Bound: chB, Profile: model.PolicyProfile, Revision: "7", Deployment: model.DeploymentDedicatedDev, Rules: rules},
			Task:   model.TaskFacts{Bound: model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelTask)}, Creator: humanOwner, TriggerEvent: "evt-1", Participants: []model.Participant{{Actor: humanOwner, Role: ceiling.Owner}, {Actor: agentA, Role: ceiling.Assignee}}},
		},
		op: operation.Facts{
			Create:       operation.CreateFacts{DerivedParticipants: []model.Participant{{Actor: humanA, Role: ceiling.Owner}, {Actor: agentA, Role: ceiling.Assignee}}},
			Source:       operation.SourceFacts{Bound: chB, EventID: "evt-1", Retained: true, InCurrentInterval: true},
			Target:       operation.TargetFacts{Bound: chB, Enrollment: "enroll-1", Device: "dev-rt", RuntimeGeneration: 1, ExpectedRuntimeGeneration: 1, DeviceGeneration: 1, ExpectedDeviceGeneration: 1, Agent: agentA, AgentEnabled: true, Operator: humanA, OperatorMemberActive: true, OperatorOrgRole: standing.OrganizationRoleMember, DeviceCurrent: true, PermittedByPolicy: true, Online: true},
			Delegation:   operation.DelegationFacts{Bound: chB, Initiator: humanA, Agent: agentA, Current: true, InitiatorAuthorityCurrent: true, ExpiresAt: baseObs.Add(time.Hour), ConfiguredSyntheticToolID: "noop-tool", PolicyCeiling: ceil, InitiatorCeiling: ceil, AgentCeiling: ceil, ResourceCeiling: ceil},
			ScopeRequest: operation.ScopeRequest{Capabilities: []grantpolicy.Capability{grantpolicy.CapabilityActionExecute}, Scope: grantpolicy.ResourceScope{ChannelIDs: []string{"synthetic-channel"}, ToolIDs: []string{"noop-tool"}}, ExpiresAt: baseObs.Add(10 * time.Minute), ActionAdapter: "noop/1", ArgumentsEmpty: true},
			Runtime:      operation.RuntimeFacts{Bound: runB, Verified: true, AssignedAgent: agentA, AssignedRun: "synthetic-run", ExecutionOwner: "enroll-1", RuntimeGeneration: 1, AgentMemberCurrent: true, OperatorMemberCurrent: true, OperatorOrgRole: standing.OrganizationRoleMember, LeaseCurrent: true, FenceCurrent: true},
			Grant:        operation.GrantFacts{Bound: runB, Present: true, State: operation.GrantStateActive, SignatureValid: true, ExpiresAt: baseObs.Add(10 * time.Minute)},
		},
	}
}

func newRuntime() fixture {
	fx := newHuman()
	fx.p.Route, fx.p.Actor, fx.p.Operator, fx.p.Audience, fx.p.Enrollment, fx.p.RuntimeGeneration = ceiling.AssignedRuntime, agentA, humanA, model.AudienceRuntimeControl, "enroll-1", 1
	fx.f.Member.Actor, fx.f.Parent.Membership.Actor = agentA, agentA
	chRef := fx.f.Parent.ACL.Resource
	fx.f.Parent.Decisions = []standing.Decision{{Effect: standing.EffectAllow, Reason: standing.ReasonAllowed, Action: standing.ActionChannelRead, Resource: chRef, Actor: agentA, PolicyVersion: "7", ACLVersion: "7"}, {Effect: standing.EffectAllow, Reason: standing.ReasonAllowed, Action: standing.ActionChannelRead, Resource: chRef, Actor: humanA, PolicyVersion: "7", ACLVersion: "7"}}
	fx.op.Delegation.Bound.Ancestry, fx.op.Delegation.Initiator = fullAnc.Truncate(model.LevelRun), humanOwner
	fx.setTarget(ceiling.RunRead, model.ResourceRun)
	return fx
}

func (fx *fixture) setTarget(action model.Action, kind model.ResourceKind) {
	levels := map[model.ResourceKind]model.Level{model.ResourceChannel: model.LevelChannel, model.ResourceTask: model.LevelTask, model.ResourceRun: model.LevelRun, model.ResourceApproval: model.LevelApproval}
	fx.req.Action, fx.req.Resource = action, model.ResourceRef{Kind: kind, ID: "synthetic-" + string(kind)}
	fx.f.Lookup.Resource, fx.f.Lookup.Bound.Ancestry = fx.req.Resource, fullAnc.Truncate(levels[kind])
}

func (fx *fixture) eval(t *testing.T) model.Refusal {
	t.Helper()
	res, err := binding.Validate(context.Background(), fx.p, fx.req, fx.f)
	if err != nil || res.Refusal.Refused() {
		t.Fatalf("binding failed: err=%v refusal=%+v", err, res.Refusal)
	}
	return operation.Validate(fx.p, fx.f, res, fx.op)
}

func assertRefusal(t *testing.T, got model.Refusal, reason model.Reason, status model.Status, code model.Code) {
	t.Helper()
	if !got.Refused() || got.Reason != reason || got.Status != status || got.Code != code {
		t.Fatalf("got %+v, want (%s, %s, %s)", got, reason, status, code)
	}
}

func overDerived() []model.Participant {
	out := []model.Participant{{Actor: humanA, Role: ceiling.Owner}, {Actor: agentA, Role: ceiling.Assignee}}
	for i := range 15 {
		out = append(out, model.Participant{Actor: model.Actor{Type: humanA.Type, ID: fmt.Sprintf("h-%d", i)}, Role: ceiling.Observer})
	}
	return out
}

var edgeNegCases = []negCase{
	{"publish-missing", "create", func(fx *fixture) { fx.f.Parent.Decisions = fx.f.Parent.Decisions[:1] }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
	{"publish-acl-deny", "create", func(fx *fixture) {
		fx.f.Parent.ACL.Entries = []standing.ACLEntry{{Actor: humanA, Action: standing.ActionChannelPublish, Effect: standing.ACLEffectDeny}}
	}, binding.ReasonParentACLDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
	{"publish-acl-default", "create", func(fx *fixture) {
		fx.f.Parent.ACL.DefaultEffect = standing.ACLEffectDeny
		fx.f.Parent.ACL.Entries = []standing.ACLEntry{{Actor: humanA, Action: standing.ActionChannelRead, Effect: standing.ACLEffectAllow}}
	}, binding.ReasonParentACLDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
	{"source-absent", "read", func(fx *fixture) { fx.op.Source.Snapshot.TransactionID = "" }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
	{"source-bad-event", "read", func(fx *fixture) { fx.op.Source.EventID = "other" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	{"target-bad-op-role", "create", func(fx *fixture) { fx.op.Target.OperatorOrgRole = "bad" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	{"target-guest-op", "create", func(fx *fixture) { fx.op.Target.OperatorOrgRole = standing.OrganizationRoleGuest }, operation.ReasonOrganizationCeiling, model.StatusPermissionDenied, model.CodePermissionDenied},
	{"target-rt-gen", "create", func(fx *fixture) { fx.op.Target.RuntimeGeneration = 2 }, operation.ReasonTargetBindingDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
	{"target-dev-gen", "create", func(fx *fixture) { fx.op.Target.DeviceGeneration = 2 }, operation.ReasonTargetBindingDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
	{"target-op-inactive", "create", func(fx *fixture) { fx.op.Target.OperatorMemberActive = false }, operation.ReasonTargetBindingDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
	{"start-no-assignee", "start", func(fx *fixture) { fx.f.Task.Participants = fx.f.Task.Participants[:1] }, operation.ReasonTargetBindingDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
	{"create-bad-derived", "create", func(fx *fixture) {
		fx.op.Create.DerivedParticipants = []model.Participant{{Actor: agentA, Role: ceiling.Assignee}}
	}, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	{"create-dup-derived", "create", func(fx *fixture) {
		fx.op.Create.DerivedParticipants = append(fx.op.Create.DerivedParticipants, model.Participant{Actor: agentA, Role: ceiling.Assignee})
	}, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	{"create-bad-role-derived", "create", func(fx *fixture) { fx.op.Create.DerivedParticipants[0].Role = "bad" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	{"create-over-derived", "create", func(fx *fixture) { fx.op.Create.DerivedParticipants = overDerived() }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	{"deleg-absent", "create", func(fx *fixture) { fx.op.Delegation.Snapshot.TransactionID = "" }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
	{"deleg-sess-lost", "create", func(fx *fixture) { fx.op.Delegation.SessionBound = true }, operation.ReasonDelegationDenied, model.StatusPermissionDenied, model.CodeGrantOutOfScope},
	{"deleg-dev-lost", "create", func(fx *fixture) { fx.op.Delegation.DeviceBound = true }, operation.ReasonDelegationDenied, model.StatusPermissionDenied, model.CodeGrantOutOfScope},
	{"deleg-expired", "create", func(fx *fixture) { fx.op.Delegation.ExpiresAt = baseObs }, operation.ReasonDelegationDenied, model.StatusPermissionDenied, model.CodeGrantOutOfScope},
	{"scope-file-cap", "create", func(fx *fixture) {
		fx.op.ScopeRequest.Capabilities = []grantpolicy.Capability{grantpolicy.CapabilityFileRead}
	}, operation.ReasonUnsupportedTool, model.StatusUnimplemented, ""},
	{"scope-file-id", "create", func(fx *fixture) { fx.op.ScopeRequest.Scope.FileIDs = []string{"f1"} }, operation.ReasonUnsupportedTool, model.StatusUnimplemented, ""},
	{"scope-bad-adapter", "create", func(fx *fixture) { fx.op.ScopeRequest.ActionAdapter = "bash/1" }, operation.ReasonUnsupportedTool, model.StatusUnimplemented, ""},
	{"scope-bad-tool", "create", func(fx *fixture) { fx.op.ScopeRequest.Scope.ToolIDs = []string{"real-tool"} }, operation.ReasonUnsupportedTool, model.StatusUnimplemented, ""},
	{"scope-unknown-field", "create", func(fx *fixture) { fx.op.ScopeRequest.UnknownFields = true }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
	{"scope-empty-cap", "create", func(fx *fixture) { fx.op.ScopeRequest.Capabilities = nil }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
	{"scope-unknown-cap", "create", func(fx *fixture) { fx.op.ScopeRequest.Capabilities = []grantpolicy.Capability{99} }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
	{"scope-wildcard-id", "create", func(fx *fixture) { fx.op.ScopeRequest.Scope.ChannelIDs = []string{"*"} }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
	{"scope-empty", "create", func(fx *fixture) { fx.op.ScopeRequest.Scope = grantpolicy.ResourceScope{} }, operation.ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope},
	{"scope-expired", "create", func(fx *fixture) { fx.op.ScopeRequest.ExpiresAt = baseObs }, operation.ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope},
	{"scope-cap-over", "create", func(fx *fixture) {
		fx.op.ScopeRequest.Capabilities = []grantpolicy.Capability{grantpolicy.CapabilityAuditRead}
	}, operation.ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope},
	{"scope-res-over", "create", func(fx *fixture) { fx.op.ScopeRequest.Scope.ChannelIDs = []string{"other-ch"} }, operation.ReasonScopeExpanded, model.StatusPermissionDenied, model.CodeGrantOutOfScope},
	{"scope-bad-ceiling", "create", func(fx *fixture) { fx.op.Delegation.ResourceCeiling.NotAfter = time.Time{} }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	{"rt-absent", "rt-read", func(fx *fixture) { fx.op.Runtime.Snapshot.TransactionID = ""; fx.op.Runtime.Verified = false }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
	{"rt-unverified", "rt-read", func(fx *fixture) { fx.op.Runtime.Verified = false }, operation.ReasonPrincipalRouteDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
	{"rt-bad-op-org", "rt-read", func(fx *fixture) { fx.op.Runtime.OperatorOrgRole = "bad" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	{"rt-req-empty-adpt", "rt-req", func(fx *fixture) { fx.op.ScopeRequest.ActionAdapter = "" }, operation.ReasonUnsupportedTool, model.StatusUnimplemented, ""},
	{"rt-req-nonempty-arg", "rt-req", func(fx *fixture) { fx.op.ScopeRequest.ArgumentsEmpty = false }, operation.ReasonUnsupportedTool, model.StatusUnimplemented, ""},
	{"rt-gen-mismatch", "rt-read", func(fx *fixture) { fx.op.Runtime.RuntimeGeneration = 9 }, operation.ReasonNotExecutionOwner, model.StatusFailedPrecondition, model.CodeNotExecutionOwner},
	{"rt-grant-absent", "rt-read", func(fx *fixture) { fx.op.Grant.Snapshot.TransactionID = "" }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
	{"rt-grant-sig", "rt-read", func(fx *fixture) { fx.op.Grant.SignatureValid = false }, operation.ReasonGrantSignatureInvalid, model.StatusPermissionDenied, model.CodeGrantOutOfScope},
	{"rt-grant-zero-exp", "rt-read", func(fx *fixture) { fx.op.Grant.ExpiresAt = time.Time{} }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
	{"rt-grant-at-exp", "rt-read", func(fx *fixture) { fx.op.Grant.ExpiresAt = baseObs }, operation.ReasonGrantExpired, model.StatusPermissionDenied, model.CodeGrantExpired},
}

func TestEdgeAndBoundaryCases(t *testing.T) {
	for _, tc := range edgeNegCases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newHuman()
			applyBase(&fx, tc.kind)
			tc.fn(&fx)
			assertRefusal(t, fx.eval(t), tc.reason, tc.status, tc.code)
		})
	}
	fx := newHuman()
	assertRefusal(t, operation.Validate(fx.p, fx.f, binding.Result{Refusal: model.Refusal{Reason: "x", Status: model.StatusUnauthenticated}}, fx.op), "x", model.StatusUnauthenticated, "")
	assertRefusal(t, operation.ValidatePreApproval(fx.p, fx.f, binding.Result{}, fx.op), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	assertRefusal(t, operation.ValidateRuntime(fx.p, fx.f, binding.Result{Refusal: model.Refusal{Reason: "x", Status: model.StatusUnauthenticated}}, fx.op), "x", model.StatusUnauthenticated, "")
	assertRefusal(t, operation.ValidateRuntime(fx.p, fx.f, binding.Result{Scope: model.Scope{Snapshot: baseSnap, Tenant: fx.p.Tenant, Ancestry: fullAnc.Truncate(model.LevelRun)}}, fx.op), operation.ReasonPrincipalRouteDenied, model.StatusPermissionDenied, model.CodePermissionDenied)
	rt := newRuntime()
	rt.op.Runtime.Snapshot.TransactionID = ""
	assertRefusal(t, operation.ValidateRuntime(rt.p, rt.f, binding.Result{Scope: model.Scope{Snapshot: baseSnap, Tenant: rt.p.Tenant, Ancestry: fullAnc.Truncate(model.LevelRun)}}, rt.op), binding.ReasonFactsUnavailable, model.StatusUnavailable, "")
	fx.setTarget(ceiling.TaskCreate, model.ResourceChannel)
	fx.f.Parent.ACL.DefaultEffect = standing.ACLEffectDeny
	fx.f.Parent.ACL.Entries = []standing.ACLEntry{{Actor: humanA, Action: standing.ActionChannelRead, Effect: standing.ACLEffectAllow}, {Actor: humanA, Action: standing.ActionChannelPublish, Effect: standing.ACLEffectAllow}}
	if ref := fx.eval(t); ref.Refused() {
		t.Fatalf("explicit publish ACL allow refused: %+v", ref)
	}
}
