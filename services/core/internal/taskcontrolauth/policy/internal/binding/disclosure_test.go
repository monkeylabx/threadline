package binding_test

import (
	"context"
	"fmt"
	fixture "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/testfixture"
	"testing"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func fMut(fn func(*model.Facts)) mutFn {
	return func(_ *model.Principal, _ *model.Request, f *model.Facts) { fn(f) }
}

func withReceipt(p *model.Principal, r *model.Request, f *model.Facts) {
	r.Purpose, r.RPC = model.PurposeOwnReceiptRead, "TaskService.Op"
	run := ""
	if p.Route == ceiling.AssignedRuntime {
		run = f.Lookup.Ancestry.Run
	}
	f.Receipt = model.ReceiptFacts{
		Bound: model.Bound{Snapshot: values.Snapshot, Tenant: p.Tenant}, Committed: true, Action: r.Action, Resource: r.Resource,
		Namespace: model.ReceiptNamespace{Actor: p.Actor, Device: p.Device, RPC: r.RPC, Enrollment: p.Enrollment, RuntimeGeneration: p.RuntimeGeneration, Run: run},
	}
}

func recCreateMut(resKind model.ResourceKind, resID, taskAnc string) mutFn {
	return func(p *model.Principal, r *model.Request, f *model.Facts) {
		fixture.SetTarget(r, f, ceiling.TaskCreate, model.ResourceChannel)
		withReceipt(p, r, f)
		f.Task.Ancestry.Task = taskAnc
		f.Receipt.Result = model.ResourceRef{Kind: resKind, ID: resID}
	}
}

func recAct(a model.Action, k model.ResourceKind) mutFn {
	return func(p *model.Principal, r *model.Request, f *model.Facts) {
		fixture.SetTarget(r, f, a, k)
		withReceipt(p, r, f)
	}
}

func tooManyParticipants() []model.Participant {
	parts := []model.Participant{{Actor: values.Owner, Role: ceiling.Owner}, {Actor: values.Agent, Role: ceiling.Assignee}}
	for i := range 15 {
		parts = append(parts, model.Participant{Actor: model.Actor{Type: values.Owner.Type, ID: fmt.Sprintf("h-%d", i)}, Role: ceiling.Observer})
	}
	return parts
}

func TestDisclosureNegatives(t *testing.T) {
	runNegatives(t, []negCase{
		{"parent-absent", false, fMut(func(f *model.Facts) { f.Parent.Snapshot.TransactionID = "" }), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"acl-empty-ver", false, fMut(func(f *model.Facts) { f.Parent.ACL.Version = "" }), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"receipt-absent", false, recMut(fMut(func(f *model.Facts) { f.Receipt.Snapshot.TransactionID = "" })), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"task-absent", false, fMut(func(f *model.Facts) { f.Task.Snapshot.TransactionID = "" }), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"human-no-dec", false, fMut(func(f *model.Facts) { f.Parent.Decisions = nil }), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"rt-no-op-dec", true, fMut(func(f *model.Facts) { f.Parent.Decisions = f.Parent.Decisions[:1] }), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"parent-old-snap", false, fMut(func(f *model.Facts) { f.Parent.Snapshot.Revision = 6 }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"parent-channel", false, fMut(func(f *model.Facts) { f.Parent.Ancestry.Channel = "other" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"policy-channel", false, fMut(func(f *model.Facts) { f.Policy.Ancestry.Channel = "other" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"task-ancestry", false, fMut(func(f *model.Facts) { f.Task.Ancestry.Task = "other" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"create-rec-kind", false, recCreateMut(model.ResourceRun, "r", "r"), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"create-rec-empty-id", false, recCreateMut(model.ResourceTask, "", ""), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"receipt-tenant", false, recMut(fMut(func(f *model.Facts) { f.Receipt.Tenant = "other" })), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"mem-tenant-mismatch", false, fMut(func(f *model.Facts) { f.Parent.Membership.TenantID = "other" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"mem-chan-mismatch", false, fMut(func(f *model.Facts) { f.Parent.Membership.ChannelID = "other" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"mem-actor-mismatch", false, fMut(func(f *model.Facts) { f.Parent.Membership.Actor.ID = "other" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"acl-wrong-res", false, fMut(func(f *model.Facts) { f.Parent.ACL.Resource.ID = "other" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"acl-bad-default", false, fMut(func(f *model.Facts) { f.Parent.ACL.DefaultEffect = "bad" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"acl-bad-entry", false, fMut(func(f *model.Facts) {
			f.Parent.ACL.Entries = []standing.ACLEntry{{Actor: values.Human, Action: standing.ActionChannelRead, Effect: "bad"}}
		}), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dec-wrong-res", false, fMut(func(f *model.Facts) { f.Parent.Decisions[0].Resource.ID = "other" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dec-policy-ver", false, fMut(func(f *model.Facts) { f.Parent.Decisions[0].PolicyVersion = "6" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dec-bad-action", false, fMut(func(f *model.Facts) {
			f.Parent.Decisions = append(f.Parent.Decisions, standing.Decision{Effect: standing.EffectAllow, Action: standing.ActionChannelUpdate, Resource: f.Parent.ACL.Resource, Actor: values.Human, PolicyVersion: "7", ACLVersion: "7"})
		}), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dec-duplicate", false, fMut(func(f *model.Facts) { f.Parent.Decisions = append(f.Parent.Decisions, f.Parent.Decisions[0]) }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	})
}

func TestRolesAndPolicyNegatives(t *testing.T) {
	runNegatives(t, []negCase{
		{"chan-role-none-cur", false, fMut(func(f *model.Facts) { f.Parent.Membership.Role = standing.ChannelRoleNone }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"parts-over-16", false, fMut(func(f *model.Facts) { f.Task.Participants = tooManyParticipants() }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"parts-dup-actor", false, fMut(func(f *model.Facts) {
			f.Task.Participants = append(f.Task.Participants, model.Participant{Actor: values.Agent, Role: ceiling.Assignee})
		}), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"parts-zero-owner", false, fMut(func(f *model.Facts) {
			f.Task.Participants = []model.Participant{{Actor: values.Agent, Role: ceiling.Assignee}}
		}), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"parts-two-owners", false, fMut(func(f *model.Facts) {
			f.Task.Participants = append(f.Task.Participants, model.Participant{Actor: values.Human, Role: ceiling.Owner})
		}), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"parts-owner-creator", false, fMut(func(f *model.Facts) { f.Task.Participants[0].Actor = values.Human }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"parts-agent-creator", false, fMut(func(f *model.Facts) { f.Task.Creator = values.Agent; f.Task.Participants[0].Actor = values.Agent }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"parts-agent-approver", false, fMut(func(f *model.Facts) { f.Task.Participants[1].Role = ceiling.Approver }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"policy-absent", false, fMut(func(f *model.Facts) { f.Policy.Snapshot.TransactionID = "" }), binding.ReasonPolicyUnavailable, model.StatusUnavailable, ""},
		{"policy-rev-empty", false, fMut(func(f *model.Facts) { f.Policy.Revision = "" }), binding.ReasonPolicyInvalid, model.StatusUnavailable, ""},
		{"mem-not-current", false, fMut(func(f *model.Facts) {
			f.Parent.Membership.Current = false
			f.Parent.Membership.Role = standing.ChannelRoleNone
		}), binding.ReasonNotAMember, model.StatusPermissionDenied, model.CodeNotAMember},
		{"mem-no-interval", false, fMut(func(f *model.Facts) { f.Parent.Membership.IntervalID = "" }), binding.ReasonNotAMember, model.StatusPermissionDenied, model.CodeNotAMember},
	})
}

func TestDisclosureOrderingNegatives(t *testing.T) {
	runNegatives(t, []negCase{
		{"order-unavail-bind", false, fMut(func(f *model.Facts) { f.Parent.ACL.Version = ""; f.Parent.Snapshot.Revision = 6 }), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"order-rec-unavail-bind", false, recMut(fMut(func(f *model.Facts) { f.Receipt.Snapshot.TransactionID = ""; f.Parent.Snapshot.Revision = 6 })), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"order-task-unavail-bind", false, fMut(func(f *model.Facts) { f.Task.Snapshot.TransactionID = ""; f.Parent.Snapshot.Revision = 6 }), binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"order-bind-policy", false, fMut(func(f *model.Facts) { f.Parent.Snapshot.Revision = 6; f.Policy.Snapshot.TransactionID = "" }), binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"order-policy-mem", false, fMut(func(f *model.Facts) { f.Policy.Snapshot.TransactionID = ""; f.Parent.Membership.Current = false }), binding.ReasonPolicyUnavailable, model.StatusUnavailable, ""},
		{"order-mem-read", false, fMut(func(f *model.Facts) {
			f.Parent.Membership.Current = false
			f.Parent.Decisions[0].Effect = standing.EffectDeny
		}), binding.ReasonNotAMember, model.StatusPermissionDenied, model.CodeNotAMember},
		{"order-read-acl", false, fMut(func(f *model.Facts) {
			f.Parent.Decisions[0].Effect = standing.EffectDeny
			f.Parent.ACL.DefaultEffect = standing.ACLEffectDeny
		}), binding.ReasonParentReadDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
	})
}

type discPosCase struct {
	name string
	rt   bool
	set  mutFn
	role model.TaskRole
	eff  model.Action
}

func runDisclosurePositives(t *testing.T, cases []discPosCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, req, f := fixture.BaseHuman()
			if tc.rt {
				p, req, f = fixture.BaseRuntime()
			}
			tc.set(&p, &req, &f)
			res, err := binding.Validate(context.Background(), p, req, f)
			if err != nil || res.Refusal.Refused() || res.TaskRole != tc.role || res.EffectiveAction != tc.eff {
				t.Fatalf("err=%v res=%+v want role=%s eff=%s", err, res, tc.role, tc.eff)
			}
			if ref := binding.ValidateReceipt(p, req, f, res); ref.Refused() {
				t.Fatalf("unexpected receipt refusal: %+v", ref)
			}
		})
	}
}

func TestDisclosurePositives(t *testing.T) {
	runDisclosurePositives(t, []discPosCase{
		{"none-role", false, fMut(func(*model.Facts) {}), ceiling.None, ceiling.TaskRead},
		{"owner-role", false, fMut(func(f *model.Facts) { f.Task.Creator = values.Human; f.Task.Participants[0].Actor = values.Human }), ceiling.Owner, ceiling.TaskRead},
		{"approver-role", false, fMut(func(f *model.Facts) {
			f.Task.Participants = append(f.Task.Participants, model.Participant{Actor: values.Human, Role: ceiling.Approver})
		}), ceiling.Approver, ceiling.TaskRead},
		{"rt-assignee", true, fMut(func(*model.Facts) {}), ceiling.Assignee, ceiling.RunRead},
		{"acl-default-deny-allow", false, fMut(func(f *model.Facts) {
			f.Parent.ACL.DefaultEffect = standing.ACLEffectDeny
			f.Parent.ACL.Entries = []standing.ACLEntry{{Actor: values.Human, Action: standing.ActionChannelRead, Effect: standing.ACLEffectAllow}}
		}), ceiling.None, ceiling.TaskRead},
		{"create-no-task", false, func(_ *model.Principal, r *model.Request, f *model.Facts) {
			fixture.SetTarget(r, f, ceiling.TaskCreate, model.ResourceChannel)
			f.Task = model.TaskFacts{}
		}, ceiling.None, ceiling.TaskCreate},
		{"receipt-no-record", false, recMut(fMut(func(f *model.Facts) { f.Receipt.Committed = false })), ceiling.Owner, ceiling.RunStart},
	})
}

func TestCommittedReceipts(t *testing.T) {
	runDisclosurePositives(t, []discPosCase{
		{"rec-create", false, recCreateMut(model.ResourceTask, "synthetic-task", "synthetic-task"), ceiling.None, ceiling.TaskRead},
		{"rec-run-start", false, recMut(fMut(func(*model.Facts) {})), ceiling.Owner, ceiling.RunRead},
		{"rec-run-ctrl", false, recAct(ceiling.RunControl, model.ResourceRun), ceiling.None, ceiling.RunRead},
		{"rec-run-rep", true, recAct(ceiling.RunReport, model.ResourceRun), ceiling.Assignee, ceiling.RunRead},
		{"rec-app-req", true, recAct(ceiling.ApprovalRequest, model.ResourceRun), ceiling.Assignee, ceiling.ApprovalRead},
		{"rec-app-dec", false, recAct(ceiling.ApprovalDecide, model.ResourceApproval), ceiling.None, ceiling.ApprovalRead},
		{"rec-app-rev", false, recAct(ceiling.ApprovalRevoke, model.ResourceApproval), ceiling.None, ceiling.ApprovalRead},
	})
	p, req, f := fixture.BaseRuntime()
	req.Action = ceiling.RunReport
	withReceipt(&p, &req, &f)
	res, err := binding.Validate(context.Background(), p, req, f)
	if err != nil || res.Refusal.Refused() {
		t.Fatalf("receipt binding: err=%v res=%+v", err, res)
	}
	for _, tc := range []negCase{
		{"ns-actor", true, fMut(func(f *model.Facts) { f.Receipt.Namespace.Actor.ID = "x" }), binding.ReasonReceiptNamespaceDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"ns-device", true, fMut(func(f *model.Facts) { f.Receipt.Namespace.Device = "x" }), binding.ReasonReceiptNamespaceDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"ns-rpc", true, fMut(func(f *model.Facts) { f.Receipt.Namespace.RPC = "x" }), binding.ReasonReceiptNamespaceDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"ns-enroll", true, fMut(func(f *model.Facts) { f.Receipt.Namespace.Enrollment = "x" }), binding.ReasonReceiptNamespaceDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"ns-rtgen", true, fMut(func(f *model.Facts) { f.Receipt.Namespace.RuntimeGeneration = 9 }), binding.ReasonReceiptNamespaceDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"ns-run", true, fMut(func(f *model.Facts) { f.Receipt.Namespace.Run = "x" }), binding.ReasonReceiptNamespaceDenied, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"action-diff", true, fMut(func(f *model.Facts) { f.Receipt.Action = ceiling.ApprovalRequest }), binding.ReasonIdempotencyConflict, model.StatusAlreadyExists, model.CodeIdempotencyConflict},
		{"res-diff", true, fMut(func(f *model.Facts) { f.Receipt.Resource.ID = "x" }), binding.ReasonIdempotencyConflict, model.StatusAlreadyExists, model.CodeIdempotencyConflict},
		{"digest-diff", true, fMut(func(f *model.Facts) { f.Receipt.RequestDigest = [32]byte{1} }), binding.ReasonIdempotencyConflict, model.StatusAlreadyExists, model.CodeIdempotencyConflict},
	} {
		f2 := f
		tc.mutate(&p, &req, &f2)
		fixture.AssertRefusal(t, binding.ValidateReceipt(p, req, f2, res), tc.reason, tc.status, tc.code)
	}
}
