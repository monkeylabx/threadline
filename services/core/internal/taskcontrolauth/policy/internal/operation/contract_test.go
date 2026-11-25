package operation_test

import (
	"encoding/json"
	"os"
	"testing"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/operation"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func setStartOwner(fx *fixture) {
	fx.setTarget(ceiling.RunStart, model.ResourceTask)
	fx.p.Actor, fx.op.Delegation.Initiator, fx.op.Delegation.Bound.Ancestry = humanOwner, humanOwner, fullAnc.Truncate(model.LevelTask)
	fx.f.Member.Actor, fx.f.Parent.Membership.Actor, fx.f.Device.Owner = humanOwner, humanOwner, humanOwner
	chRef := fx.f.Parent.ACL.Resource
	fx.f.Parent.Decisions = []standing.Decision{{Effect: standing.EffectAllow, Action: standing.ActionChannelRead, Resource: chRef, Actor: humanOwner, PolicyVersion: "7", ACLVersion: "7"}, {Effect: standing.EffectAllow, Action: standing.ActionChannelPublish, Resource: chRef, Actor: humanOwner, PolicyVersion: "7", ACLVersion: "7"}}
}

func setReceipt(fx *fixture) {
	setStartOwner(fx)
	fx.req.Purpose, fx.req.RPC = model.PurposeOwnReceiptRead, "TaskService.StartRun"
	fx.f.Receipt = model.ReceiptFacts{Bound: model.Bound{Snapshot: baseSnap, Tenant: fx.p.Tenant}, Committed: true, Action: ceiling.RunStart, Resource: fx.req.Resource, Namespace: model.ReceiptNamespace{Actor: humanOwner, Device: fx.p.Device, RPC: fx.req.RPC}}
}

type scEntry struct {
	id, base string
	mod      func(*fixture)
}

func allScenarios() []scEntry {
	out := make([]scEntry, 0, 110)
	for _, tc := range []struct {
		sfx string
		fn  func(*fixture)
	}{
		{"publish-denied", func(fx *fixture) { fx.f.Parent.Decisions[1].Effect = standing.EffectDeny }},
		{"guest-org", func(fx *fixture) { fx.f.Member.Role = standing.OrganizationRoleGuest }},
		{"guest-channel", func(fx *fixture) { fx.f.Parent.Membership.Role = standing.ChannelRoleGuest }},
		{"archived", func(fx *fixture) { fx.f.Parent.State = standing.ResourceStateArchived }},
		{"history-lost", func(fx *fixture) { fx.op.Source.InCurrentInterval = false }},
		{"source-erased", func(fx *fixture) { fx.op.Source.Retained = false }},
		{"target-mismatch", func(fx *fixture) { fx.op.Target.PermittedByPolicy = false }},
		{"target-device-revoked", func(fx *fixture) { fx.op.Target.DeviceCurrent = false }},
		{"target-agent-disabled", func(fx *fixture) { fx.op.Target.AgentEnabled = false }},
		{"delegation-denied", func(fx *fixture) { fx.op.Delegation.Current = false }},
		{"scope-expanded", func(fx *fixture) { fx.op.ScopeRequest.ExpiresAt = fx.op.Delegation.PolicyCeiling.NotAfter.Add(1) }},
	} {
		out = append(out, scEntry{"task-create-" + tc.sfx, "create", tc.fn}, scEntry{"run-start-" + tc.sfx, "start", tc.fn})
	}
	for _, sp := range []struct {
		slug, base string
		act        model.Action
	}{
		{"task-create", "create", ceiling.TaskCreate}, {"run-start", "start", ceiling.RunStart},
		{"task-read", "read", ceiling.TaskRead}, {"run-read", "run-read", ceiling.RunRead}, {"run-control", "ctrl", ceiling.RunControl},
		{"approval-read", "app-read", ceiling.ApprovalRead}, {"approval-request", "rt-req", ceiling.ApprovalRequest},
		{"approval-decide", "decide", ceiling.ApprovalDecide}, {"approval-revoke", "revoke", ceiling.ApprovalRevoke}, {"run-report", "rt-rep", ceiling.RunReport},
	} {
		act := sp.act
		out = append(out, scEntry{"allow-" + sp.slug, sp.base, nil}, scEntry{"disabled-" + sp.slug, sp.base, func(fx *fixture) { delete(fx.f.Policy.Rules, act) }})
	}
	return append(out, scenarioCases...)
}

var scenarioCases = []scEntry{
	{"channel-pending-deletion", "", func(fx *fixture) { fx.f.Parent.State = standing.ResourceStatePendingDeletion }},
	{"policy-actor-constraint-denies", "", func(fx *fixture) { fx.f.Policy.ActorDenied = true }},
	{"read-audience-missing", "", func(fx *fixture) { fx.f.Policy.Rules[ceiling.TaskRead] = model.PolicyRule{} }},
	{"start-role-rule-missing", "start", func(fx *fixture) { fx.f.Policy.Rules[ceiling.RunStart] = model.PolicyRule{} }},
	{"target-facts-unavailable", "start", func(fx *fixture) { fx.op.Target.Snapshot.TransactionID = "" }},
	{"extra-context-refs-unsupported", "create", func(fx *fixture) { fx.op.Create.AdditionalReferences = true }},
	{"new-policy-revision-rechecks-and-passes", "", func(fx *fixture) { fx.f.Policy.Revision = "8" }},
	{"new-policy-revision-removes-read", "", func(fx *fixture) { fx.f.Policy.Revision = "8"; delete(fx.f.Policy.Rules, ceiling.TaskRead) }},
	{"nonparticipant-channel-reader", "", nil},
	{"archived-task-readable", "", func(fx *fixture) { fx.f.Parent.State = standing.ResourceStateArchived }},
	{"source-history-lost-run-metadata-visible", "run-read", func(fx *fixture) { fx.op.Source.InCurrentInterval = false }},
	{"task-authored-history-lost", "", func(fx *fixture) { fx.op.Source.InCurrentInterval = false }},
	{"approval-authored-history-lost", "app-read", func(fx *fixture) { fx.op.Source.InCurrentInterval = false }},
	{"task-authored-source-erased", "", func(fx *fixture) { fx.op.Source.Retained = false }},
	{"guest-metadata-readable", "", func(fx *fixture) {
		fx.f.Member.Role, fx.f.Parent.Membership.Role = standing.OrganizationRoleGuest, standing.ChannelRoleGuest
	}},
	{"create-explicit-history-sharing", "create", func(fx *fixture) { fx.op.Source.InCurrentInterval, fx.op.Source.HistorySharingCurrent = false, true }},
	{"start-offline-target-queues", "start", func(fx *fixture) { fx.op.Target.Online = false }},
	{"create-empty-participants-derive", "create", func(fx *fixture) { fx.op.Create.Participants = nil }},
	{"create-exact-participants", "create", func(fx *fixture) {
		fx.op.Create.Participants = []model.Participant{{Actor: agentA, Role: ceiling.Assignee}, {Actor: humanA, Role: ceiling.Owner}}
	}},
	{"create-untrusted-approver", "create", func(fx *fixture) {
		fx.op.Create.Participants = []model.Participant{{Actor: humanA, Role: ceiling.Owner}, {Actor: humanOwner, Role: ceiling.Approver}, {Actor: agentA, Role: ceiling.Assignee}}
	}},
	{"create-duplicate-role", "create", func(fx *fixture) {
		fx.op.Create.Participants = []model.Participant{{Actor: humanA, Role: ceiling.Owner}, {Actor: humanA, Role: ceiling.Observer}, {Actor: agentA, Role: ceiling.Assignee}}
	}},
	{"create-owner-plus-approver-duplicate", "create", func(fx *fixture) {
		fx.op.Create.Participants = []model.Participant{{Actor: humanA, Role: ceiling.Owner}, {Actor: humanA, Role: ceiling.Approver}, {Actor: agentA, Role: ceiling.Assignee}}
	}},
	{"create-extra-owner", "create", func(fx *fixture) {
		fx.op.Create.Participants = []model.Participant{{Actor: humanA, Role: ceiling.Owner}, {Actor: humanOwner, Role: ceiling.Owner}, {Actor: agentA, Role: ceiling.Assignee}}
	}},
	{"create-caller-timestamp", "create", func(fx *fixture) { fx.op.Create.HasParticipantTimestamps = true }},
	{"participant-over-limit", "create", func(fx *fixture) { fx.op.Create.Participants = make([]model.Participant, 17) }},
	{"observer-cannot-start", "", func(fx *fixture) {
		fx.setTarget(ceiling.RunStart, model.ResourceTask)
		fx.f.Task.Participants = append(fx.f.Task.Participants, model.Participant{Actor: humanA, Role: ceiling.Observer})
	}},
	{"admin-nonowner-cannot-start", "", func(fx *fixture) {
		fx.setTarget(ceiling.RunStart, model.ResourceTask)
		fx.f.Member.Role, fx.f.Parent.Membership.Role = standing.OrganizationRoleAdmin, standing.ChannelRoleOwner
	}},
	{"observer-can-interrupt", "ctrl", nil},
	{"approver-can-interrupt-if-enabled", "", func(fx *fixture) {
		fx.setTarget(ceiling.RunControl, model.ResourceRun)
		fx.f.Task.Participants = append(fx.f.Task.Participants, model.Participant{Actor: humanA, Role: ceiling.Approver})
	}},
	{"guest-observer-can-interrupt", "ctrl", func(fx *fixture) {
		fx.f.Member.Role, fx.f.Parent.Membership.Role = standing.OrganizationRoleGuest, standing.ChannelRoleGuest
	}},
	{"archived-cancel-still-allowed", "ctrl", func(fx *fixture) { fx.f.Parent.State = standing.ResourceStateArchived }},
	{"cancel-needs-no-execution-grant", "ctrl", func(fx *fixture) { fx.op.Grant.State = operation.GrantStateRevoked }},
	{"observer-control-disabled", "ctrl", func(fx *fixture) {
		fx.f.Policy.Rules[ceiling.RunControl] = model.PolicyRule{Roles: []model.TaskRole{ceiling.Owner}}
	}},
	{"nonparticipant-cannot-control", "run-read", func(fx *fixture) { fx.req.Action = ceiling.RunControl }},
	{"approval-decide-guest-approver", "decide", func(fx *fixture) { fx.f.Member.Role = standing.OrganizationRoleGuest }},
	{"approval-revoke-guest-approver", "revoke", func(fx *fixture) { fx.f.Member.Role = standing.OrganizationRoleGuest }},
	{"archived-eligible-rejection", "decide", func(fx *fixture) { fx.f.Parent.State = standing.ResourceStateArchived }},
	{"archived-eligible-revoke", "revoke", func(fx *fixture) { fx.f.Parent.State = standing.ResourceStateArchived }},
	{"archived-new-approval-denied", "decide", func(fx *fixture) { fx.f.Parent.State, fx.op.Approved = standing.ResourceStateArchived, true }},
	{"request-before-action-grant-issued", "rt-req", nil},
	{"human-owner-not-execution-owner", "run-read", func(fx *fixture) { fx.req.Action = ceiling.RunReport }},
	{"bare-agent-no-runtime", "run-read", func(fx *fixture) { fx.p.Route, fx.req.Action = "bare-agent", ceiling.RunReport }},
	{"gateway-service-not-runtime", "run-read", func(fx *fixture) { fx.p.Route, fx.req.Action = "service", ceiling.RunReport }},
	{"runtime-agent-substitution", "rt-rep", func(fx *fixture) { fx.op.Runtime.AssignedAgent.ID = "other" }},
	{"runtime-initiator-authority-lost", "rt-rep", func(fx *fixture) { fx.op.Delegation.InitiatorAuthorityCurrent = false }},
	{"runtime-agent-member-revoked", "rt-rep", func(fx *fixture) { fx.op.Runtime.AgentMemberCurrent = false }},
	{"runtime-operator-member-revoked", "rt-rep", func(fx *fixture) { fx.op.Runtime.OperatorMemberCurrent = false }},
	{"runtime-guest-operator", "rt-rep", func(fx *fixture) { fx.op.Runtime.OperatorOrgRole = standing.OrganizationRoleGuest }},
	{"runtime-wrong-owner", "rt-rep", func(fx *fixture) { fx.op.Runtime.ExecutionOwner = "other" }},
	{"runtime-lease-expired", "rt-rep", func(fx *fixture) { fx.op.Runtime.LeaseCurrent = false }},
	{"runtime-stale-fence", "rt-rep", func(fx *fixture) { fx.op.Runtime.FenceCurrent = false }},
	{"runtime-grant-expired", "rt-rep", func(fx *fixture) { fx.op.Grant.State = operation.GrantStateExpired }},
	{"runtime-grant-revoked", "rt-rep", func(fx *fixture) { fx.op.Grant.State = operation.GrantStateRevoked }},
	{"runtime-grant-missing", "rt-rep", func(fx *fixture) { fx.op.Grant.Present = false }},
	{"assigned-runtime-metadata-read", "rt-read", func(fx *fixture) {
		fx.f.Policy.Rules[ceiling.RunRead] = model.PolicyRule{Audiences: []model.ReadAudience{model.ReadAssignedRuntimeMetadata}}
	}},
	{"runtime-cannot-read-task-authored", "rt-read", func(fx *fixture) { fx.setTarget(ceiling.TaskRead, model.ResourceTask) }},
	{"runtime-cannot-read-approval-authored", "rt-read", func(fx *fixture) { fx.setTarget(ceiling.ApprovalRead, model.ResourceApproval) }},
	{"runtime-foreign-run-read", "rt-read", func(fx *fixture) { fx.op.Runtime.AssignedRun = "other" }},
	{"receipt-terminal", "rec", nil},
	{"receipt-grant-revoked", "rec", func(fx *fixture) { fx.op.Grant.State = operation.GrantStateRevoked }},
	{"receipt-approval-expired", "rec", nil},
	{"receipt-writer-eligibility-lost-retains-read", "rec", func(fx *fixture) { delete(fx.f.Policy.Rules, ceiling.RunStart) }},
	{"receipt-read-policy-revoked", "rec", func(fx *fixture) { delete(fx.f.Policy.Rules, ceiling.RunRead) }},
	{"receipt-mode-no-record-no-bypass", "rec", func(fx *fixture) {
		fx.f.Receipt.Committed = false
		fx.f.Task.Participants = append(fx.f.Task.Participants, model.Participant{Actor: humanA, Role: ceiling.Observer})
		fx.p.Actor, fx.f.Member.Actor, fx.f.Parent.Membership.Actor, fx.f.Device.Owner = humanA, humanA, humanA, humanA
		fx.f.Parent.Decisions[0].Actor, fx.f.Parent.Decisions[1].Actor = humanA, humanA
	}},
	{"receipt-run-metadata-history-lost", "rec", func(fx *fixture) { fx.op.Source.InCurrentInterval = false }},
	{"receipt-full-task-history-lost", "rec", func(fx *fixture) {
		fx.setTarget(ceiling.TaskCreate, model.ResourceChannel)
		fx.f.Receipt.Action, fx.f.Receipt.Resource, fx.f.Receipt.Result = ceiling.TaskCreate, fx.req.Resource, model.ResourceRef{Kind: model.ResourceTask, ID: "synthetic-task"}
		fx.op.Source.InCurrentInterval = false
	}},
}

var baseApply = map[string]func(*fixture){
	"create": func(fx *fixture) { fx.setTarget(ceiling.TaskCreate, model.ResourceChannel) }, "start": setStartOwner,
	"run-read": func(fx *fixture) { fx.setTarget(ceiling.RunRead, model.ResourceRun) },
	"ctrl": func(fx *fixture) {
		fx.setTarget(ceiling.RunControl, model.ResourceRun)
		fx.f.Task.Participants = append(fx.f.Task.Participants, model.Participant{Actor: humanA, Role: ceiling.Observer})
	},
	"app-read": func(fx *fixture) { fx.setTarget(ceiling.ApprovalRead, model.ResourceApproval) },
	"decide":   func(fx *fixture) { fx.setTarget(ceiling.ApprovalDecide, model.ResourceApproval) },
	"revoke":   func(fx *fixture) { fx.setTarget(ceiling.ApprovalRevoke, model.ResourceApproval) },
	"rt-read":  func(fx *fixture) { *fx = newRuntime() }, "rt-req": func(fx *fixture) { *fx = newRuntime(); fx.setTarget(ceiling.ApprovalRequest, model.ResourceRun) },
	"rt-rep": func(fx *fixture) { *fx = newRuntime(); fx.setTarget(ceiling.RunReport, model.ResourceRun) }, "rec": setReceipt,
}

func applyBase(fx *fixture, base string) {
	if fn, ok := baseApply[base]; ok {
		fn(fx)
	}
}

func TestMappedContractScenarios(t *testing.T) {
	data, err := os.ReadFile("../../../../../../../test/fixtures/proto/task-control-authorization/scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	var sf struct {
		Cases []struct {
			ID   string
			Then struct{ Decision, Reason, Status, Code string }
		}
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]int, len(sf.Cases))
	for i, c := range sf.Cases {
		byID[c.ID] = i
	}
	for _, sc := range allScenarios() {
		idx, ok := byID[sc.id]
		if !ok {
			t.Fatalf("missing fixture case %s", sc.id)
		}
		c := sf.Cases[idx]
		t.Run(sc.id, func(t *testing.T) {
			fx := newHuman()
			applyBase(&fx, sc.base)
			if sc.mod != nil {
				sc.mod(&fx)
			}
			ref := fx.eval(t)
			if c.Then.Decision == "allow" && ref.Refused() {
				t.Fatalf("expected allow, got %+v", ref)
			}
			if c.Then.Decision != "allow" {
				assertRefusal(t, ref, model.Reason(c.Then.Reason), model.Status(c.Then.Status), model.Code(c.Then.Code))
			}
		})
	}
}
