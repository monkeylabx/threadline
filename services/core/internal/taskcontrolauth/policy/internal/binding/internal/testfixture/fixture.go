// Package testfixture supplies independent value copies for binding tests.
package testfixture

import (
	"context"
	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
	"testing"
	"time"
)

var (
	baseObs    = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	baseSnap   = model.Snapshot{TransactionID: "tx-7", Revision: 7}
	humanA     = model.Actor{Type: rpcmiddleware.ActorTypeHuman, ID: "synthetic-human-a"}
	humanOwner = model.Actor{Type: rpcmiddleware.ActorTypeHuman, ID: "synthetic-human-owner"}
	agentA     = model.Actor{Type: rpcmiddleware.ActorTypeAgent, ID: "synthetic-agent-a"}
	fullAnc    = model.Ancestry{Channel: "synthetic-channel", Task: "synthetic-task", Run: "synthetic-run", Approval: "synthetic-approval"}
)

type Values struct {
	Now                 time.Time
	Snapshot            model.Snapshot
	Human, Owner, Agent model.Actor
	Ancestry            model.Ancestry
}

func Golden() Values {
	return Values{Now: baseObs, Snapshot: baseSnap, Human: humanA, Owner: humanOwner, Agent: agentA, Ancestry: fullAnc}
}

func BaseHuman() (model.Principal, model.Request, model.Facts) {
	b := model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant"}
	chBound := model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelChannel)}
	chRef := standing.ResourceRef{TenantID: "synthetic-tenant", Kind: standing.ResourceKindChannel, ID: "synthetic-channel"}
	p := model.Principal{Route: ceiling.Human, Tenant: "synthetic-tenant", Actor: humanA, Device: "synthetic-device-a", SessionGeneration: 7, Audience: model.AudienceHumanSession, ExpiresAt: baseObs.Add(time.Hour)}
	req := model.Request{Action: ceiling.TaskRead, Purpose: model.PurposeOperation, ParentKind: model.ParentChannel, Resource: model.ResourceRef{Kind: model.ResourceTask, ID: "synthetic-task"}}
	f := model.Facts{
		Snapshot: baseSnap, ObservedAt: baseObs,
		Device:       model.DeviceFacts{Bound: b, Owner: humanA, Device: "synthetic-device-a", Current: true},
		Organization: model.OrganizationFacts{Bound: b, Active: true, PolicyVersion: "7"},
		Member:       model.MemberFacts{Bound: b, Actor: humanA, Active: true, Role: standing.OrganizationRoleMember},
		Lookup:       model.LookupFacts{Bound: model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelTask)}, Found: true, Resource: req.Resource},
		Parent: model.ParentFacts{
			Bound: chBound, State: standing.ResourceStateActive,
			Membership: standing.ChannelMembershipFacts{TenantID: "synthetic-tenant", ChannelID: "synthetic-channel", Actor: humanA, Current: true, IntervalID: "7", Role: standing.ChannelRoleMember},
			ACL:        standing.ResourceACLFacts{Resource: chRef, Version: "7", DefaultEffect: standing.ACLEffectAllow},
			Decisions:  []standing.Decision{{Effect: standing.EffectAllow, Reason: standing.ReasonAllowed, Action: standing.ActionChannelRead, Resource: chRef, Actor: humanA, PolicyVersion: "7", ACLVersion: "7"}},
		},
		Policy: model.PolicyFacts{Bound: chBound, Profile: model.PolicyProfile, Revision: "7", Deployment: model.DeploymentDedicatedDev, Rules: defaultRules()},
		Task: model.TaskFacts{
			Bound: model.Bound{Snapshot: baseSnap, Tenant: "synthetic-tenant", Ancestry: fullAnc.Truncate(model.LevelTask)}, Creator: humanOwner,
			Participants: []model.Participant{{Actor: humanOwner, Role: ceiling.Owner}, {Actor: agentA, Role: ceiling.Assignee}},
		},
	}
	return p, req, f
}

func defaultRules() map[model.Action]model.PolicyRule {
	m := map[model.Action]model.PolicyRule{}
	for _, a := range []model.Action{ceiling.TaskCreate, ceiling.TaskRead, ceiling.RunStart, ceiling.RunRead, ceiling.RunControl, ceiling.ApprovalRead, ceiling.ApprovalRequest, ceiling.ApprovalDecide, ceiling.ApprovalRevoke, ceiling.RunReport} {
		m[a] = model.PolicyRule{Audiences: []model.ReadAudience{model.ReadChannelReaders}}
	}
	return m
}

func BaseRuntime() (model.Principal, model.Request, model.Facts) {
	p, req, f := BaseHuman()
	p.Route, p.Actor, p.Operator = ceiling.AssignedRuntime, agentA, humanA
	p.Audience, p.Enrollment, p.RuntimeGeneration = model.AudienceRuntimeControl, "enroll-1", 1
	f.Member.Actor, f.Parent.Membership.Actor = agentA, agentA
	chRef := f.Parent.ACL.Resource
	f.Parent.Decisions = []standing.Decision{
		{Effect: standing.EffectAllow, Reason: standing.ReasonAllowed, Action: standing.ActionChannelRead, Resource: chRef, Actor: agentA, PolicyVersion: "7", ACLVersion: "7"},
		{Effect: standing.EffectAllow, Reason: standing.ReasonAllowed, Action: standing.ActionChannelRead, Resource: chRef, Actor: humanA, PolicyVersion: "7", ACLVersion: "7"},
	}
	SetTarget(&req, &f, ceiling.RunRead, model.ResourceRun)
	return p, req, f
}

func SetTarget(req *model.Request, f *model.Facts, action model.Action, kind model.ResourceKind) model.Ancestry {
	levels := map[model.ResourceKind]model.Level{model.ResourceChannel: model.LevelChannel, model.ResourceTask: model.LevelTask, model.ResourceRun: model.LevelRun, model.ResourceApproval: model.LevelApproval}
	req.Action, req.Resource = action, model.ResourceRef{Kind: kind, ID: "synthetic-" + string(kind)}
	f.Lookup.Resource, f.Lookup.Bound.Ancestry = req.Resource, fullAnc.Truncate(levels[kind])
	return f.Lookup.Bound.Ancestry
}

func AssertRefusal(t *testing.T, got model.Refusal, reason model.Reason, status model.Status, code model.Code) {
	t.Helper()
	if !got.Refused() || got.Reason != reason || got.Status != status || got.Code != code {
		t.Fatalf("got refusal %+v, want (%s, %s, %s)", got, reason, status, code)
	}
}

func AssertBound(t *testing.T, p model.Principal, req model.Request, f model.Facts, wantAnc model.Ancestry) {
	t.Helper()
	res, err := binding.Validate(context.Background(), p, req, f)
	if err != nil || res.Refusal.Refused() || res.Scope != (model.Scope{Snapshot: baseSnap, Tenant: p.Tenant, Ancestry: wantAnc}) {
		t.Fatalf("%s/%s: err=%v res=%+v", req.Action, req.Purpose, err, res)
	}
}
