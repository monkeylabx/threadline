package eligibility_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/capability/grantpolicy"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/eligibility"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

var (
	baseObs, baseSnap                                    = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), model.Snapshot{TransactionID: "tx-7", Revision: 7}
	humanOwner, humanAppA, humanAppB, humanObs, humanNon = hActor("synthetic-human-owner"), hActor("synthetic-human-a"), hActor("synthetic-human-b"), hActor("synthetic-human-obs"), hActor("synthetic-human-none")
	agentA                                               = model.Actor{Type: rpcmiddleware.ActorTypeAgent, ID: "synthetic-agent-a"}
	fullAnc                                              = model.Ancestry{Channel: "synthetic-channel", Task: "synthetic-task", Run: "synthetic-run", Approval: "synthetic-approval"}
	reqScope, reqCaps                                    = grantpolicy.ResourceScope{ToolIDs: []string{"noop/1"}}, []grantpolicy.Capability{grantpolicy.CapabilityToolInvoke}
	chRef                                                = standing.ResourceRef{TenantID: "synthetic-tenant", Kind: standing.ResourceKindChannel, ID: "synthetic-channel"}
)

type scenarioEnv struct {
	act eligibility.ActionBinding
	f   eligibility.EligibilityFacts
	st  eligibility.StoredApproval
	req eligibility.DecisionRequest
}
type envNegCase struct {
	name string
	mut  func(*scenarioEnv)
	want model.Refusal
}

func hActor(id string) model.Actor { return model.Actor{Type: rpcmiddleware.ActorTypeHuman, ID: id} }

func baseEnv(kind string) scenarioEnv {
	lv := map[bool]model.Level{true: model.LevelRun, false: model.LevelApproval}[kind == "req"]
	stStatus := map[bool]eligibility.ApprovalStatus{true: eligibility.ApprovalApproved, false: eligibility.ApprovalPending}[kind == "use"]
	sc, tb, cb := model.Scope{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(lv)}, model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant"}, model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelChannel)}
	act := eligibility.ActionBinding{Bound: sc, ActionID: "act-1", ActionVersion: "1", AdapterID: "synthetic", Operation: "noop/1", Commitment: [32]byte{7}, Capabilities: reqCaps, Scope: reqScope, ApprovalScope: eligibility.ApprovalScopeOnce}
	var decs []standing.Decision
	cands := map[model.Actor]eligibility.CandidateFacts{}
	for _, a := range []model.Actor{humanOwner, humanAppA, humanAppB, humanObs, humanNon} {
		decs = append(decs, standing.Decision{Effect: standing.EffectAllow, Reason: standing.ReasonAllowed, Action: standing.ActionChannelRead, Resource: chRef, Actor: a, PolicyVersion: "7", ACLVersion: "7"})
		cands[a] = eligibility.CandidateFacts{Member: model.MemberFacts{Bound: tb, Actor: a, Active: true, Role: standing.OrganizationRoleMember}, SourceEntitled: true, AuthorityAvailable: true, Membership: standing.ChannelMembershipFacts{TenantID: "synthetic-tenant", ChannelID: "synthetic-channel", Actor: a, Current: true, IntervalID: "7", Role: standing.ChannelRoleMember}, Capabilities: reqCaps, Scope: reqScope, Device: model.DeviceFacts{Bound: tb, Owner: a, Device: "dev-" + a.ID, Current: true}}
	}
	f := eligibility.EligibilityFacts{Scope: sc, ObservedAt: baseObs, Organization: model.OrganizationFacts{Bound: tb, Active: true, PolicyVersion: "7"}, PolicyCeiling: eligibility.PolicyCeiling{Available: true, Capabilities: reqCaps, Scope: reqScope}, Candidates: cands, Parent: model.ParentFacts{Bound: cb, State: standing.ResourceStateActive, ACL: standing.ResourceACLFacts{Resource: chRef, Version: "7", DefaultEffect: standing.ACLEffectAllow}, Decisions: decs}, Policy: model.PolicyFacts{Bound: cb, Profile: model.PolicyProfile, Revision: "7", Deployment: model.DeploymentDedicatedDev, Rules: map[model.Action]model.PolicyRule{ceiling.ApprovalRequest: {Roles: []model.TaskRole{ceiling.Assignee}}, ceiling.ApprovalDecide: {Roles: []model.TaskRole{ceiling.Approver}}, ceiling.ApprovalRevoke: {Roles: []model.TaskRole{ceiling.Approver}}}}, Task: model.TaskFacts{Bound: model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelTask)}, Creator: humanOwner, Participants: []model.Participant{{Actor: humanOwner, Role: ceiling.Owner}, {Actor: agentA, Role: ceiling.Assignee}, {Actor: humanAppB, Role: ceiling.Approver}, {Actor: humanAppA, Role: ceiling.Approver}, {Actor: humanObs, Role: ceiling.Observer}}}}
	st := eligibility.StoredApproval{Bound: sc, Action: act, Status: stStatus, ExpiresAt: baseObs.Add(10 * time.Minute), Decider: humanAppA, Snapshot: eligibility.Snapshot{Bound: model.Bound{Snapshot: model.Snapshot{TransactionID: "tx-5", Revision: 5}, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelRun)}, ActionID: "act-1", ActionVersion: "1", Commitment: [32]byte{7}, PolicyRevision: "7", Eligible: []eligibility.EligibleApprover{{Actor: humanAppA, TaskRole: ceiling.Approver}, {Actor: humanAppB, TaskRole: ceiling.Approver}}}}
	return scenarioEnv{act: act, f: f, st: st, req: eligibility.DecisionRequest{Caller: humanAppA, Action: ceiling.ApprovalDecide, Approved: true, ApprovalScope: eligibility.ApprovalScopeOnce}}
}

func cMut(fn func(*eligibility.CandidateFacts)) func(*scenarioEnv) {
	return func(e *scenarioEnv) { c := e.f.Candidates[humanAppA]; fn(&c); e.f.Candidates[humanAppA] = c }
}
func callKind(kind string, e scenarioEnv) (eligibility.Result, error) {
	if kind == "req" {
		return eligibility.ValidateRequest(context.Background(), e.act, e.f)
	}
	if kind == "use" {
		return eligibility.ValidateDeciderForUse(context.Background(), e.st, e.f)
	}
	return eligibility.ValidateDecision(context.Background(), e.req, e.st, e.f)
}
func runNegs(t *testing.T, kind string, cases []envNegCase) {
	t.Helper()
	for _, tc := range cases {
		e := baseEnv(kind)
		tc.mut(&e)
		out, err := callKind(kind, e)
		if err != nil || !out.Refusal.Refused() || out.Refusal != tc.want {
			t.Fatalf("%s/%s: err=%v got %+v want %+v", kind, tc.name, err, out.Refusal, tc.want)
		}
	}
}

func TestCurrentAndOwnerSelfMatrix(t *testing.T) {
	e := baseEnv("req")
	if res, err := eligibility.ValidateRequest(context.Background(), e.act, e.f); err != nil || res.Refusal.Refused() || len(res.Eligible) != 2 || res.Eligible[0].Actor != humanAppA {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	b := e.f.Candidates[humanAppB]
	b.SourceEntitled = false
	e.f.Candidates[humanAppB] = b
	e.f.Parent.ACL.DefaultEffect, e.f.Parent.ACL.Entries = standing.ACLEffectDeny, []standing.ACLEntry{{Actor: humanAppA, Action: standing.ActionChannelRead, Effect: standing.ACLEffectAllow}}
	if res, _ := eligibility.Current(context.Background(), e.act, e.f); len(res.Eligible) != 1 || res.Eligible[0].Actor != humanAppA {
		t.Fatalf("expected A only via ACL, got %+v", res)
	}
	for mask := 0; mask < 8; mask++ {
		for _, dep := range []string{"", "production", model.DeploymentDedicatedDev} {
			checkOwnerSelfCombo(t, mask, dep)
		}
	}
}
func checkOwnerSelfCombo(t *testing.T, mask int, dep string) {
	t.Helper()
	e := baseEnv("req")
	e.f.Policy.Deployment, e.f.Policy.AllowOwnerSelfApproval, e.f.Policy.ConfiguredOwnerApprover = dep, mask&1 != 0, mask&2 != 0
	e.f.Policy.Rules[ceiling.ApprovalDecide] = model.PolicyRule{Roles: map[bool][]model.TaskRole{false: {ceiling.Approver}, true: {ceiling.Approver, ceiling.Owner}}[mask&4 != 0]}
	want := mask == 7 && dep == model.DeploymentDedicatedDev
	if !want {
		delete(e.f.Candidates, humanOwner)
	}
	r, _ := eligibility.Current(context.Background(), e.act, e.f)
	if r.Refusal.Refused() || slices.ContainsFunc(r.Eligible, func(a eligibility.EligibleApprover) bool { return a.Actor == humanOwner && a.TaskRole == ceiling.Owner }) != want || r.UsedOwnerSelfApproval != want {
		t.Fatalf("mask=%d dep=%s res=%+v", mask, dep, r)
	}
}

func TestRequestDecisionAndUseNegatives(t *testing.T) {
	e := baseEnv("dec")
	b := e.f.Candidates[humanAppB]
	b.SourceEntitled = false
	e.f.Candidates[humanAppB] = b
	if res, err := eligibility.ValidateDecision(context.Background(), e.req, e.st, e.f); err != nil || res.Refusal.Refused() || len(res.Eligible) != 1 {
		t.Fatalf("dec err=%v res=%+v", err, res)
	}
	u, pd, ia, gs, fp, cp, iv := model.StatusUnavailable, model.StatusPermissionDenied, model.StatusInvalidArgument, model.CodeGrantOutOfScope, model.StatusFailedPrecondition, model.CodePermissionDenied, model.CodeInvalidStateTransition
	unav, bnd, ro := model.Refusal{Reason: binding.ReasonFactsUnavailable, Status: u}, model.Refusal{Reason: binding.ReasonFactsBindingInvalid, Status: u}, model.Refusal{Reason: eligibility.ReasonScopeExpanded, Status: pd, Code: gs}
	aclDeny, snap2x, p17 := []standing.ACLEntry{{Actor: humanAppA, Action: standing.ActionChannelRead, Effect: standing.ACLEffectDeny}}, append(append([]eligibility.EligibleApprover(nil), e.st.Snapshot.Eligible...), e.st.Snapshot.Eligible[0]), append(append([]model.Participant(nil), e.f.Task.Participants...), make([]model.Participant, 12)...)
	for i := 5; i < 17; i++ {
		p17[i] = model.Participant{Actor: hActor("p" + string(rune('a'+i))), Role: ceiling.Approver}
	}
	runNegs(t, "req", []envNegCase{
		{"zero-obs", func(e *scenarioEnv) { e.f.ObservedAt = time.Time{} }, unav}, {"no-run", func(e *scenarioEnv) { e.f.Scope.Ancestry.Run = "" }, unav}, {"org-bnd", func(e *scenarioEnv) { e.f.Organization.Tenant = "x" }, bnd}, {"org-pv", func(e *scenarioEnv) { e.f.Organization.PolicyVersion = "" }, unav}, {"org-inact", func(e *scenarioEnv) { e.f.Organization.Active = false }, model.Refusal{Reason: binding.ReasonOrganizationInactive, Status: pd, Code: cp}}, {"pc-unav", func(e *scenarioEnv) { e.f.PolicyCeiling.Available = false }, unav}, {"pc-cap", func(e *scenarioEnv) { e.f.PolicyCeiling.Capabilities = []grantpolicy.Capability{99} }, bnd}, {"par-bnd", func(e *scenarioEnv) { e.f.Parent.Ancestry.Channel = "x" }, bnd}, {"acl-0", func(e *scenarioEnv) { e.f.Parent.ACL.Version = "" }, unav}, {"acl-eff", func(e *scenarioEnv) { e.f.Parent.ACL.Entries = []standing.ACLEntry{{Effect: "bad"}} }, bnd},
		{"dec-dup", func(e *scenarioEnv) { e.f.Parent.Decisions = append(e.f.Parent.Decisions, e.f.Parent.Decisions[0]) }, bnd}, {"pol-unav", func(e *scenarioEnv) { e.f.Policy.Snapshot.TransactionID = "" }, model.Refusal{Reason: binding.ReasonPolicyUnavailable, Status: u}}, {"pol-bnd", func(e *scenarioEnv) { e.f.Policy.Ancestry.Channel = "x" }, bnd}, {"pol-prof", func(e *scenarioEnv) { e.f.Policy.Profile = "bad" }, model.Refusal{Reason: binding.ReasonPolicyInvalid, Status: u}}, {"pol-rev", func(e *scenarioEnv) { e.f.Policy.Revision = "" }, model.Refusal{Reason: binding.ReasonPolicyInvalid, Status: u}}, {"task-bnd", func(e *scenarioEnv) { e.f.Task.Ancestry.Task = "x" }, bnd}, {"task-own", func(e *scenarioEnv) { e.f.Task.Participants = e.f.Task.Participants[1:] }, bnd}, {"task-17", func(e *scenarioEnv) { e.f.Task.Participants = p17 }, bnd}, {"task-agt", func(e *scenarioEnv) { e.f.Task.Participants[1].Role = ceiling.Approver }, bnd}, {"task-asn", func(e *scenarioEnv) { e.f.Task.Participants[1].Actor = humanNon }, bnd},
		{"act-bnd", func(e *scenarioEnv) { e.act.Bound.Ancestry.Run = "x" }, bnd}, {"act-id", func(e *scenarioEnv) { e.act.ActionID = "" }, bnd}, {"act-mode0", func(e *scenarioEnv) { e.act.ApprovalScope = "" }, model.Refusal{Reason: binding.ReasonShapeInvalid, Status: ia}}, {"act-mode", func(e *scenarioEnv) { e.act.ApprovalScope = "TIME_BOUNDED" }, model.Refusal{Reason: eligibility.ReasonUnsupportedMode, Status: model.StatusUnimplemented}}, {"act-cap0", func(e *scenarioEnv) { e.act.Capabilities = nil }, model.Refusal{Reason: binding.ReasonShapeInvalid, Status: ia}}, {"act-wild", func(e *scenarioEnv) { e.act.Scope.ToolIDs = []string{"noop/*"} }, model.Refusal{Reason: binding.ReasonShapeInvalid, Status: ia}}, {"pc-scope", func(e *scenarioEnv) { e.f.PolicyCeiling.Scope = grantpolicy.ResourceScope{} }, ro}, {"pc-cap-out", func(e *scenarioEnv) { e.f.PolicyCeiling.Capabilities = reqCaps[:0] }, ro},
		{"c-miss", func(e *scenarioEnv) { delete(e.f.Candidates, humanAppA) }, unav}, {"c-bnd", cMut(func(c *eligibility.CandidateFacts) { c.Member.Tenant = "x" }), bnd}, {"c-org", cMut(func(c *eligibility.CandidateFacts) { c.Member.Role = "bad" }), bnd}, {"c-nodec", func(e *scenarioEnv) { e.f.Parent.Decisions = e.f.Parent.Decisions[:1] }, unav}, {"c-auth", cMut(func(c *eligibility.CandidateFacts) { c.AuthorityAvailable = false }), unav}, {"c-cap", cMut(func(c *eligibility.CandidateFacts) { c.Capabilities = []grantpolicy.Capability{99} }), bnd}, {"c-devsn", cMut(func(c *eligibility.CandidateFacts) { c.Device.Bound.Snapshot.Revision = 6 }), bnd}, {"c-devow", cMut(func(c *eligibility.CandidateFacts) { c.Device.Owner = humanAppB }), bnd}, {"req-0", func(e *scenarioEnv) { e.f.Parent.ACL.DefaultEffect = standing.ACLEffectDeny }, model.Refusal{Reason: eligibility.ReasonApproverCurrentDenied, Status: pd, Code: cp}},
	})
	runNegs(t, "dec", []envNegCase{
		{"bad-act", func(e *scenarioEnv) { e.req.Action = ceiling.RunStart }, model.Refusal{Reason: binding.ReasonUnknownAction, Status: model.StatusUnimplemented}}, {"mode0", func(e *scenarioEnv) { e.req.ApprovalScope = "" }, model.Refusal{Reason: binding.ReasonShapeInvalid, Status: ia}}, {"agent", func(e *scenarioEnv) { e.req.Caller = agentA }, model.Refusal{Reason: eligibility.ReasonApproverRoleDenied, Status: pd, Code: cp}}, {"nonpart", func(e *scenarioEnv) { e.req.Caller = humanNon }, model.Refusal{Reason: eligibility.ReasonApproverRoleDenied, Status: pd, Code: cp}}, {"no-appr", func(e *scenarioEnv) { e.f.Scope.Ancestry.Approval = "" }, unav}, {"st-bnd", func(e *scenarioEnv) { e.st.Bound.Ancestry.Approval = "other" }, bnd}, {"snap0", func(e *scenarioEnv) { e.st.Snapshot.Bound.Snapshot.TransactionID = "" }, unav}, {"snap-id", func(e *scenarioEnv) { e.st.Snapshot.ActionID = "other" }, bnd},
		{"snap-2x", func(e *scenarioEnv) { e.st.Snapshot.Eligible = snap2x }, bnd}, {"p-del", func(e *scenarioEnv) { e.f.Parent.State = standing.ResourceStatePendingDeletion }, model.Refusal{Reason: eligibility.ReasonParentStateDenied, Status: pd, Code: cp}}, {"no-cal", func(e *scenarioEnv) { delete(e.f.Candidates, humanAppA) }, unav}, {"mem-off", cMut(func(c *eligibility.CandidateFacts) { c.Member.Active = false }), model.Refusal{Reason: binding.ReasonMemberInactive, Status: pd, Code: cp}}, {"no-mem", cMut(func(c *eligibility.CandidateFacts) { c.Membership.Current = false }), model.Refusal{Reason: binding.ReasonNotAMember, Status: pd, Code: model.CodeNotAMember}}, {"rd-deny", func(e *scenarioEnv) { e.f.Parent.Decisions[1].Effect = standing.EffectDeny }, model.Refusal{Reason: binding.ReasonParentReadDenied, Status: pd, Code: cp}}, {"acl-d", func(e *scenarioEnv) { e.f.Parent.ACL.Entries = aclDeny }, model.Refusal{Reason: binding.ReasonParentACLDenied, Status: pd, Code: cp}}, {"pol-d", cMut(func(c *eligibility.CandidateFacts) { c.ActorDenied = true }), model.Refusal{Reason: eligibility.ReasonPolicyActorDenied, Status: pd, Code: cp}},
		{"ch-gst", cMut(func(c *eligibility.CandidateFacts) { c.Membership.Role = standing.ChannelRoleGuest }), model.Refusal{Reason: eligibility.ReasonChannelTaskCeiling, Status: pd, Code: cp}}, {"src-no", cMut(func(c *eligibility.CandidateFacts) { c.SourceEntitled = false }), model.Refusal{Reason: eligibility.ReasonSourceHistoryDenied, Status: pd, Code: model.CodeHistorySharingDenied}}, {"pc-narrow", func(e *scenarioEnv) { e.f.PolicyCeiling.Scope = grantpolicy.ResourceScope{} }, ro}, {"exp", func(e *scenarioEnv) { e.st.ExpiresAt = e.f.ObservedAt }, model.Refusal{Reason: eligibility.ReasonGrantExpired, Status: pd, Code: model.CodeGrantExpired}}, {"denied", func(e *scenarioEnv) { e.st.Status = eligibility.ApprovalDenied }, model.Refusal{Reason: eligibility.ReasonInvalidStateTransition, Status: fp, Code: iv}}, {"rev-rev", func(e *scenarioEnv) { e.req.Action, e.st.Status = ceiling.ApprovalRevoke, eligibility.ApprovalRevoked }, model.Refusal{Reason: eligibility.ReasonGrantRevoked, Status: pd, Code: model.CodeGrantRevoked}}, {"rev-con", func(e *scenarioEnv) { e.req.Action, e.st.Status = ceiling.ApprovalRevoke, eligibility.ApprovalConsumed }, model.Refusal{Reason: eligibility.ReasonInvalidStateTransition, Status: fp, Code: iv}},
	})
	runNegs(t, "use", []envNegCase{{"bad-stored", func(e *scenarioEnv) { e.st.Bound.Snapshot.Revision = 6 }, bnd}, {"bad-decider", func(e *scenarioEnv) { e.st.Decider = model.Actor{} }, bnd}, {"use-miss-dev", func(e *scenarioEnv) { e.st.RequireDeciderDevice = true; delete(e.f.Candidates, humanAppA) }, unav}, {"dev-mismatch", func(e *scenarioEnv) { e.st.RequireDeciderDevice = true }, bnd}, {"revoked", func(e *scenarioEnv) { e.st.Status = eligibility.ApprovalRevoked }, model.Refusal{Reason: eligibility.ReasonGrantRevoked, Status: pd, Code: model.CodeGrantRevoked}}, {"zero-exp", func(e *scenarioEnv) { e.st.ExpiresAt = time.Time{} }, unav}, {"disabled", func(e *scenarioEnv) { delete(e.f.Policy.Rules, ceiling.ApprovalDecide) }, model.Refusal{Reason: eligibility.ReasonActionDisabled, Status: pd, Code: cp}}})
}

func TestUseDeviceAndCancel(t *testing.T) {
	e := baseEnv("use")
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, fn := range []func() error{func() error { _, err := eligibility.Current(cctx, e.act, e.f); return err }, func() error { _, err := eligibility.ValidateDecision(cctx, e.req, e.st, e.f); return err }, func() error { _, err := eligibility.ValidateDeciderForUse(cctx, e.st, e.f); return err }} {
		if err := fn(); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected canceled, got %v", err)
		}
	}
	e.st.RequireDeciderDevice, e.st.DecidedDevice = true, "dev-"+humanAppA.ID
	cMut(func(c *eligibility.CandidateFacts) { c.RequireDeviceCurrent = true })(&e)
	if r2, err := eligibility.ValidateDeciderForUse(context.Background(), e.st, e.f); err != nil || r2.Refusal.Refused() || r2.Decider.Actor != humanAppA {
		t.Fatalf("dev err=%v res=%+v", err, r2)
	}
	cMut(func(c *eligibility.CandidateFacts) { c.Device.Current = false })(&e)
	if rRev, err := eligibility.ValidateDeciderForUse(context.Background(), e.st, e.f); err != nil || rRev.Refusal != (model.Refusal{Reason: binding.ReasonDeviceRevoked, Status: model.StatusPermissionDenied, Code: model.CodeDeviceRevoked}) {
		t.Fatalf("revoked err=%v ref=%+v", err, rRev.Refusal)
	}
}
