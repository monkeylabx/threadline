package binding_test

import (
	"context"
	"errors"
	fixture "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/testfixture"
	"strings"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

var values = fixture.Golden()

type mutFn func(*model.Principal, *model.Request, *model.Facts)
type negCase struct {
	name   string
	rt     bool
	mutate mutFn
	reason model.Reason
	status model.Status
	code   model.Code
}

func runNegatives(t *testing.T, cases []negCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, req, f := fixture.BaseHuman()
			if tc.rt {
				p, req, f = fixture.BaseRuntime()
			}
			tc.mutate(&p, &req, &f)
			res, err := binding.Validate(context.Background(), p, req, f)
			if err != nil {
				t.Fatal(err)
			}
			fixture.AssertRefusal(t, res.Refusal, tc.reason, tc.status, tc.code)
		})
	}
}

func TestPositiveBindingsAndReceiptModes(t *testing.T) {
	actions := []struct {
		a model.Action
		k model.ResourceKind
		r bool
	}{
		{ceiling.TaskCreate, model.ResourceChannel, true}, {ceiling.TaskRead, model.ResourceTask, false},
		{ceiling.RunStart, model.ResourceTask, true}, {ceiling.RunRead, model.ResourceRun, false},
		{ceiling.RunControl, model.ResourceRun, true}, {ceiling.ApprovalRead, model.ResourceApproval, false},
		{ceiling.ApprovalRequest, model.ResourceRun, true}, {ceiling.ApprovalDecide, model.ResourceApproval, true},
		{ceiling.ApprovalRevoke, model.ResourceApproval, true}, {ceiling.RunReport, model.ResourceRun, true},
	}
	for _, tc := range actions {
		p, req, f := fixture.BaseHuman()
		wantAnc := fixture.SetTarget(&req, &f, tc.a, tc.k)
		fixture.AssertBound(t, p, req, f, wantAnc)
		req.Purpose, req.RPC = model.PurposeOwnReceiptRead, "TaskService.Op"
		f.Receipt = model.ReceiptFacts{Bound: model.Bound{Snapshot: values.Snapshot, Tenant: p.Tenant}}
		if tc.r {
			res, err := binding.Validate(context.Background(), p, req, f)
			if err != nil || res.Refusal.Refused() || res.EffectiveAction != tc.a || res.ReceiptSelected {
				t.Fatalf("%s uncommitted receipt: err=%v res=%+v", tc.a, err, res)
			}
		} else {
			res, err := binding.Validate(context.Background(), p, req, f)
			if err != nil {
				t.Fatal(err)
			}
			fixture.AssertRefusal(t, res.Refusal, binding.ReasonPurposeInvalid, model.StatusInvalidArgument, "")
		}
	}
	p, req, f := fixture.BaseRuntime()
	fixture.AssertBound(t, p, req, f, values.Ancestry.Truncate(model.LevelRun))
}

func TestPrincipalNegatives(t *testing.T) {
	runNegatives(t, []negCase{
		{"empty-tenant", false, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Tenant = "" }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"empty-actor", false, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Actor.ID = "" }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"empty-device", false, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Device = "" }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"zero-obs", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.ObservedAt = time.Time{} }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"at-expiry", false, func(p *model.Principal, _ *model.Request, f *model.Facts) { f.ObservedAt = p.ExpiresAt }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"human-actor-type", false, func(p *model.Principal, _ *model.Request, _ *model.Facts) {
			p.Actor.Type = rpcmiddleware.ActorTypeAgent
		}, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"human-audience", false, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Audience = model.AudienceRuntimeControl }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"human-operator", false, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Operator = values.Human }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"human-enroll", false, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Enrollment = "e" }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"rt-actor-type", true, func(p *model.Principal, _ *model.Request, _ *model.Facts) {
			p.Actor.Type = rpcmiddleware.ActorTypeHuman
		}, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"rt-op-type", true, func(p *model.Principal, _ *model.Request, _ *model.Facts) {
			p.Operator.Type = rpcmiddleware.ActorTypeAgent
		}, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"rt-op-id", true, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Operator.ID = "" }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"rt-enroll", true, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Enrollment = "" }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"rt-gen", true, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.RuntimeGeneration = 0 }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"rt-audience", true, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Audience = model.AudienceHumanSession }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"other-route-aud", false, func(p *model.Principal, _ *model.Request, _ *model.Facts) { p.Route, p.Audience = "bare-agent", "bad" }, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"id-empty", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Resource.ID = "" }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
		{"id-long", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Resource.ID = strings.Repeat("a", 129) }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
		{"id-ctrl", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Resource.ID = "a\nb" }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
		{"id-utf8", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Resource.ID = "\xff" }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
		{"anc-ctrl", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Ancestry.Channel = "c\x00" }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
		{"kind-mismatch", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Action = ceiling.TaskCreate }, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
		{"receipt-no-rpc", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) {
			r.Action, r.Resource.Kind, r.Purpose = ceiling.RunStart, model.ResourceTask, model.PurposeOwnReceiptRead
		}, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
		{"unknown-action", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Action = "task.update" }, binding.ReasonUnknownAction, model.StatusUnimplemented, ""},
		{"unknown-purpose", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Purpose = "bad" }, binding.ReasonPurposeInvalid, model.StatusInvalidArgument, ""},
		{"bad-parent", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.ParentKind = "dm" }, binding.ReasonUnsupportedParent, model.StatusUnimplemented, ""},
	})
}

func TestFactsNegatives(t *testing.T) {
	runNegatives(t, []negCase{
		{"dev-absent", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Snapshot.TransactionID = "" }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"dev-old-snap", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Snapshot.Revision = 6 }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dev-tenant", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Tenant = "other" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dev-ancestry", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Ancestry.Channel = "c" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dev-owner", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Owner.ID = "other" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"rt-dev-owner", true, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Owner = values.Agent }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dev-id", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Device = "other" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"dev-revoked", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Device.Current = false }, binding.ReasonDeviceRevoked, model.StatusPermissionDenied, model.CodeDeviceRevoked},
		{"org-absent", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Organization.Snapshot.TransactionID = "" }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"org-tenant", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Organization.Tenant = "other" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"org-inactive", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Organization.Active = false }, binding.ReasonOrganizationInactive, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"mem-absent", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Member.Snapshot.TransactionID = "" }, binding.ReasonFactsUnavailable, model.StatusUnavailable, ""},
		{"mem-actor", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Member.Actor.ID = "other" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"mem-inactive", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Member.Active = false }, binding.ReasonMemberInactive, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"lookup-missing", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Found = false }, binding.ReasonObjectAbsent, model.StatusNotFound, ""},
		{"lookup-tenant", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Tenant = "other" }, binding.ReasonObjectAbsent, model.StatusNotFound, ""},
		{"lookup-req-anc", false, func(_ *model.Principal, r *model.Request, _ *model.Facts) { r.Ancestry.Channel = "other" }, binding.ReasonObjectAbsent, model.StatusNotFound, ""},
		{"lookup-snap", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Snapshot.Revision = 6 }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"lookup-res", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Resource.ID = "other" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"lookup-no-chan", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Ancestry.Channel = "" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"lookup-deep", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) { f.Lookup.Ancestry.Run = "extra" }, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"chan-extra-task", false, func(_ *model.Principal, r *model.Request, f *model.Facts) {
			fixture.SetTarget(r, f, ceiling.TaskCreate, model.ResourceChannel)
			f.Lookup.Ancestry.Task = "extra"
		}, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"run-no-task", false, func(_ *model.Principal, r *model.Request, f *model.Facts) {
			fixture.SetTarget(r, f, ceiling.RunRead, model.ResourceRun)
			f.Lookup.Ancestry.Task = ""
		}, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
		{"appr-no-run", false, func(_ *model.Principal, r *model.Request, f *model.Facts) {
			fixture.SetTarget(r, f, ceiling.ApprovalRead, model.ResourceApproval)
			f.Lookup.Ancestry.Run = ""
		}, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, ""},
	})
}

func TestOrderingContextAndSectionLevels(t *testing.T) {
	runNegatives(t, []negCase{
		{"auth-first", false, func(p *model.Principal, q *model.Request, f *model.Facts) {
			p.Tenant, q.Resource.ID, f.Lookup.Found = "", "", false
		}, binding.ReasonAuthenticationRequired, model.StatusUnauthenticated, ""},
		{"shape-before-action", false, func(_ *model.Principal, q *model.Request, _ *model.Facts) {
			q.Resource.ID, q.Action = "", "task.update"
		}, binding.ReasonShapeInvalid, model.StatusInvalidArgument, ""},
		{"org-before-lookup", false, func(_ *model.Principal, _ *model.Request, f *model.Facts) {
			f.Organization.Active, f.Lookup.Found = false, false
		}, binding.ReasonOrganizationInactive, model.StatusPermissionDenied, model.CodePermissionDenied},
		{"not-found-before-snap", false, func(_ *model.Principal, q *model.Request, f *model.Facts) {
			q.Ancestry.Channel, f.Lookup.Snapshot.Revision = "x", 99
		}, binding.ReasonObjectAbsent, model.StatusNotFound, ""},
	})
	p, req, f := fixture.BaseHuman()
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res, err := binding.Validate(cctx, p, req, f); !errors.Is(err, context.Canceled) || res != (binding.Result{}) {
		t.Fatalf("canceled: err=%v res=%+v", err, res)
	}
	dctx, dcancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer dcancel()
	if res, err := binding.Validate(dctx, p, req, f); !errors.Is(err, context.DeadlineExceeded) || res != (binding.Result{}) {
		t.Fatalf("deadline: err=%v res=%+v", err, res)
	}
	scope := model.Scope{Snapshot: values.Snapshot, Tenant: "t", Ancestry: values.Ancestry}
	for _, lv := range []model.Level{model.LevelTenant, model.LevelChannel, model.LevelTask, model.LevelRun, model.LevelApproval} {
		if ref := binding.Section(scope, model.Bound{Snapshot: values.Snapshot, Tenant: "t", Ancestry: values.Ancestry.Truncate(lv)}, lv); ref.Refused() {
			t.Fatalf("level %d unexpected refusal: %+v", lv, ref)
		}
	}
}
