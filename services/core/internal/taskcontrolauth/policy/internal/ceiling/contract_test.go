package ceiling_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
)

type contract struct {
	Actions []struct {
		Action  ceiling.Action    `json:"action"`
		Parents []standing.Action `json:"parentActions"`
	} `json:"actions"`
	Rows []struct {
		ID       string           `json:"id"`
		Route    ceiling.Route    `json:"route"`
		TaskRole ceiling.TaskRole `json:"taskRole"`
		Tier     string           `json:"currentOrganizationAndChannelTier"`
		Self     bool             `json:"devSelfException"`
		Allowed  []ceiling.Action `json:"allowedActions"`
	} `json:"roleCeilings"`
}

func loadContract(t *testing.T) contract {
	t.Helper()
	data, err := os.ReadFile("../../../../../../../test/fixtures/proto/task-control-authorization/decisions.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture contract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Actions) != 10 || len(fixture.Rows) != 10 {
		t.Fatal("incomplete reference matrix")
	}
	return fixture
}

func TestReferenceRoleCeilings(t *testing.T) {
	fixture := loadContract(t)
	for _, row := range fixture.Rows {
		state := ceiling.RoleState{
			Route: row.Route, TaskRole: row.TaskRole,
			OrganizationRole: standing.OrganizationRole("ROLE_" + row.Tier),
			ChannelRole:      standing.ChannelRole("CHANNEL_MEMBER_ROLE_" + row.Tier),
			SelfApproval: ceiling.SelfApprovalPolicy{
				DeploymentClass: "dedicated-dev", AllowOwner: row.Self,
				ConfiguredOwner: row.Self, OwnerRoleEnabled: row.Self,
			},
		}
		for _, entry := range fixture.Actions {
			want := slices.Contains(row.Allowed, entry.Action)
			if got := ceiling.WithinRoleCeiling(entry.Action, state); got != want {
				t.Errorf("%s/%s: got %v want %v", row.ID, entry.Action, got, want)
			}
		}
	}
}

func TestRequiredParentActions(t *testing.T) {
	for _, entry := range loadContract(t).Actions {
		got, ok := ceiling.RequiredParentActions(entry.Action)
		if !ok || !slices.Equal(got, entry.Parents) {
			t.Fatalf("%s: got %v, %v", entry.Action, got, ok)
		}
		got[0] = standing.ActionChannelDiscover
		fresh, _ := ceiling.RequiredParentActions(entry.Action)
		if !slices.Equal(fresh, entry.Parents) {
			t.Fatal("parent descriptor aliases shared state")
		}
	}
	for _, action := range []ceiling.Action{"", "*", "task.update", "TaskService.GetTask"} {
		parents, ok := ceiling.RequiredParentActions(action)
		if ok || len(parents) != 0 || ceiling.WithinRoleCeiling(action, memberState()) {
			t.Errorf("unknown action %q admitted", action)
		}
	}
}

func memberState() ceiling.RoleState {
	return ceiling.RoleState{Route: ceiling.Human, TaskRole: ceiling.Owner,
		OrganizationRole: standing.OrganizationRoleMember, ChannelRole: standing.ChannelRoleMember}
}
