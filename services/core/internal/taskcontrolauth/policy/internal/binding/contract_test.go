package binding_test

import (
	"context"
	"encoding/json"
	fixture "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/testfixture"
	"maps"
	"os"
	"testing"
	"time"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func setOwner(p *model.Principal, f *model.Facts) {
	f.Task.Creator = p.Actor
	f.Task.Participants = []model.Participant{{Actor: p.Actor, Role: ceiling.Owner}, {Actor: values.Agent, Role: ceiling.Assignee}}
}

func recMut(fn mutFn) mutFn {
	return func(p *model.Principal, r *model.Request, f *model.Facts) {
		setOwner(p, f)
		fixture.SetTarget(r, f, ceiling.RunStart, model.ResourceTask)
		withReceipt(p, r, f)
		fn(p, r, f)
	}
}

func ownMut(a model.Action, k model.ResourceKind) mutFn {
	return func(p *model.Principal, r *model.Request, f *model.Facts) {
		setOwner(p, f)
		fixture.SetTarget(r, f, a, k)
	}
}

func appMut(a model.Action) mutFn {
	return func(p *model.Principal, r *model.Request, f *model.Facts) {
		f.Task.Participants = append(f.Task.Participants, model.Participant{Actor: p.Actor, Role: ceiling.Approver})
		fixture.SetTarget(r, f, a, model.ResourceApproval)
	}
}

func rtMut(a model.Action, fn mutFn) mutFn {
	return func(p *model.Principal, r *model.Request, f *model.Facts) {
		*p, *r, *f = fixture.BaseRuntime()
		fixture.SetTarget(r, f, a, model.ResourceRun)
		if fn != nil {
			fn(p, r, f)
		}
	}
}

func mappedMutators() map[string]mutFn {
	return map[string]mutFn{
		"allow-task-read": func(*model.Principal, *model.Request, *model.Facts) {},
		"allow-run-read": func(_ *model.Principal, r *model.Request, f *model.Facts) {
			fixture.SetTarget(r, f, "run.read", model.ResourceRun)
		},
		"auth-missing": func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Actor.ID = "" },
		"auth-before-foreign-shape": func(p *model.Principal, r *model.Request, f *model.Facts) {
			p.Actor.ID, r.Resource.ID, f.Lookup.Found = "", "", false
		},
		"session-expired": func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.ExpiresAt = values.Now.Add(-time.Second) },
		"runtime-wrong-audience": func(p *model.Principal, r *model.Request, f *model.Facts) {
			*p, *r, *f = fixture.BaseRuntime()
			r.Action, p.Audience = "run.report", model.AudienceHumanSession
		},
		"unknown-purpose":        func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Purpose = "synthetic-unknown" },
		"rpc-name-is-not-action": func(*model.Principal, *model.Request, *model.Facts) {},
		"task-edit-unsupported":  func(*model.Principal, *model.Request, *model.Facts) {},
		"dm-unsupported": func(_ *model.Principal, r *model.Request, f *model.Facts) {
			fixture.SetTarget(r, f, "task.create", model.ResourceChannel)
			r.ParentKind = "dm"
		},
		"device-revoked":            func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Current = false },
		"organization-inactive":     func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Organization.Active = false },
		"member-inactive":           func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Member.Active = false },
		"lookup-missing":            func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Found = false },
		"lookup-foreign":            func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Tenant = "foreign-tenant" },
		"child-parent-substitution": func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Ancestry.Channel = "other-channel" },
		"fact-other-tenant":         func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Organization.Tenant = "other-tenant" },
		"fact-other-actor":          func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Member.Actor.ID = "other-actor" },
		"fact-other-resource":       func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Resource.ID = "other-resource" },
		"fact-old-transaction":      func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Snapshot.Revision = 6 },
	}
}

func disclosureMutators() map[string]mutFn {
	return map[string]mutFn{
		"allow-task-create": func(_ *model.Principal, r *model.Request, f *model.Facts) {
			fixture.SetTarget(r, f, ceiling.TaskCreate, model.ResourceChannel)
		},
		"allow-run-start":   ownMut(ceiling.RunStart, model.ResourceTask),
		"allow-run-control": ownMut(ceiling.RunControl, model.ResourceRun),
		"allow-approval-read": func(_ *model.Principal, r *model.Request, f *model.Facts) {
			fixture.SetTarget(r, f, ceiling.ApprovalRead, model.ResourceApproval)
		},
		"allow-approval-request":      rtMut(ceiling.ApprovalRequest, nil),
		"allow-approval-decide":       appMut(ceiling.ApprovalDecide),
		"allow-approval-revoke":       appMut(ceiling.ApprovalRevoke),
		"allow-run-report":            rtMut(ceiling.RunReport, nil),
		"fact-unavailable":            fMut(func(f *model.Facts) { f.Parent.Snapshot.TransactionID = "" }),
		"acl-version-missing":         fMut(func(f *model.Facts) { f.Parent.ACL.Version = "" }),
		"fact-old-parent-decision":    fMut(func(f *model.Facts) { f.Parent.Decisions[0].ACLVersion = "6" }),
		"policy-other-channel":        fMut(func(f *model.Facts) { f.Policy.Ancestry.Channel = "other" }),
		"unknown-organization-role":   fMut(func(f *model.Facts) { f.Member.Role = "synthetic-unknown" }),
		"unknown-channel-role":        fMut(func(f *model.Facts) { f.Parent.Membership.Role = "synthetic-unknown" }),
		"unknown-persisted-task-role": fMut(func(f *model.Facts) { f.Task.Participants[0].Role = "synthetic-unknown" }),
		"human-assignee-unsupported-stored-fact": fMut(func(f *model.Facts) {
			f.Task.Participants = append(f.Task.Participants, model.Participant{Actor: values.Human, Role: ceiling.Assignee})
		}),
		"policy-unavailable":           fMut(func(f *model.Facts) { f.Policy.Snapshot.TransactionID = "" }),
		"policy-empty-revision":        fMut(func(f *model.Facts) { f.Policy.Revision = "" }),
		"policy-unknown-format":        fMut(func(f *model.Facts) { f.Policy.Profile = "synthetic-unknown/1" }),
		"membership-departed":          fMut(func(f *model.Facts) { f.Parent.Membership.Current = false }),
		"membership-interval-missing":  fMut(func(f *model.Facts) { f.Parent.Membership.IntervalID = "" }),
		"public-discovery-is-not-read": fMut(func(f *model.Facts) { f.Parent.Membership.Current = false }),
		"owner-cannot-bypass-membership": func(p *model.Principal, _ *model.Request, f *model.Facts) {
			setOwner(p, f)
			f.Member.Role, f.Parent.Membership.Current = standing.OrganizationRoleOwner, false
		},
		"security-admin-cannot-bypass-membership": fMut(func(f *model.Facts) {
			f.Member.Role, f.Parent.Membership.Current = standing.OrganizationRoleSecurityAdmin, false
		}),
		"parent-acl-deny": fMut(func(f *model.Facts) {
			f.Parent.ACL.Entries = []standing.ACLEntry{{Actor: values.Human, Action: standing.ActionChannelRead, Effect: standing.ACLEffectDeny}}
		}),
		"parent-acl-deny-wins-allow": fMut(func(f *model.Facts) {
			f.Parent.ACL.Entries = []standing.ACLEntry{{Actor: values.Human, Action: standing.ActionChannelRead, Effect: standing.ACLEffectAllow}, {Actor: values.Human, Action: standing.ActionChannelRead, Effect: standing.ACLEffectDeny}}
		}),
		"parent-acl-default-deny":      fMut(func(f *model.Facts) { f.Parent.ACL.DefaultEffect = standing.ACLEffectDeny }),
		"parent-read-denied":           fMut(func(f *model.Facts) { f.Parent.Decisions[0].Effect = standing.EffectDeny }),
		"runtime-operator-read-denied": rtMut(ceiling.RunReport, fMut(func(f *model.Facts) { f.Parent.Decisions[1].Effect = standing.EffectDeny })),
		"runtime-agent-read-denied":    rtMut(ceiling.RunReport, fMut(func(f *model.Facts) { f.Parent.Decisions[0].Effect = standing.EffectDeny })),
		"receipt-relogin-same-device":  recMut(func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.SessionGeneration = 8 }),
		"receipt-membership-revoked":   recMut(fMut(func(f *model.Facts) { f.Parent.Membership.Current = false })),
		"receipt-parent-acl-revoked": recMut(fMut(func(f *model.Facts) {
			f.Parent.ACL.Entries = []standing.ACLEntry{{Actor: values.Human, Action: standing.ActionChannelRead, Effect: standing.ACLEffectDeny}}
		})),
		"receipt-foreign-device": recMut(fMut(func(f *model.Facts) { f.Receipt.Namespace.Device = "other-device" })),
		"receipt-changed-bytes":  recMut(fMut(func(f *model.Facts) { f.Receipt.RequestDigest = [32]byte{1} })),
	}
}

func TestMappedContractScenarios(t *testing.T) {
	data, err := os.ReadFile("../../../../../../../test/fixtures/proto/task-control-authorization/scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	var sf struct {
		Cases []struct {
			ID, Action string
			Then       struct{ Decision, Reason, Status, Code string }
		}
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]int, len(sf.Cases))
	for i, c := range sf.Cases {
		byID[c.ID] = i
	}
	all := mappedMutators()
	maps.Copy(all, disclosureMutators())
	for id, mutate := range all {
		idx, ok := byID[id]
		if !ok {
			t.Fatalf("missing fixture case %s", id)
		}
		c := sf.Cases[idx]
		t.Run(id, func(t *testing.T) {
			p, req, f := fixture.BaseHuman()
			req.Action = model.Action(c.Action)
			mutate(&p, &req, &f)
			res, err := binding.Validate(context.Background(), p, req, f)
			if err != nil {
				t.Fatal(err)
			}
			ref := res.Refusal
			if !ref.Refused() {
				ref = binding.ValidateReceipt(p, req, f, res)
			}
			if c.Then.Decision == "allow" {
				if ref.Refused() {
					t.Fatalf("expected bound, got %+v", ref)
				}
				return
			}
			fixture.AssertRefusal(t, ref, model.Reason(c.Then.Reason), model.Status(c.Then.Status), model.Code(c.Then.Code))
		})
	}
}
