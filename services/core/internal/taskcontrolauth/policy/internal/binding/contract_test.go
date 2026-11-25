package binding_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func mappedMutators() map[string]mutFn {
	return map[string]mutFn{
		"allow-task-read": func(*model.Principal, *model.Request, *model.Facts) {},
		"allow-run-read": func(_ *model.Principal, r *model.Request, f *model.Facts) {
			setTarget(r, f, "run.read", model.ResourceRun)
		},
		"auth-missing": func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Actor.ID = "" },
		"auth-before-foreign-shape": func(p *model.Principal, r *model.Request, f *model.Facts) {
			p.Actor.ID, r.Resource.ID, f.Lookup.Found = "", "", false
		},
		"session-expired": func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.ExpiresAt = baseObs.Add(-time.Second) },
		"runtime-wrong-audience": func(p *model.Principal, r *model.Request, f *model.Facts) {
			*p, *r, *f = baseRuntime()
			r.Action, p.Audience = "run.report", model.AudienceHumanSession
		},
		"unknown-purpose":        func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Purpose = "synthetic-unknown" },
		"rpc-name-is-not-action": func(*model.Principal, *model.Request, *model.Facts) {},
		"task-edit-unsupported":  func(*model.Principal, *model.Request, *model.Facts) {},
		"dm-unsupported": func(_ *model.Principal, r *model.Request, f *model.Facts) {
			setTarget(r, f, "task.create", model.ResourceChannel)
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
	for id, mutate := range mappedMutators() {
		idx, ok := byID[id]
		if !ok {
			t.Fatalf("missing fixture case %s", id)
		}
		c := sf.Cases[idx]
		t.Run(id, func(t *testing.T) {
			p, req, f := baseHuman()
			req.Action = model.Action(c.Action)
			mutate(&p, &req, &f)
			res, err := binding.Validate(context.Background(), p, req, f)
			if err != nil {
				t.Fatal(err)
			}
			if c.Then.Decision == "allow" {
				if res.Refusal.Refused() {
					t.Fatalf("expected bound, got %+v", res.Refusal)
				}
				return
			}
			assertRefusal(t, res.Refusal, model.Reason(c.Then.Reason), model.Status(c.Then.Status), model.Code(c.Then.Code))
		})
	}
}
