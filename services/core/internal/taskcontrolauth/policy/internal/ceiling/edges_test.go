package ceiling_test

import (
	"testing"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
)

func TestUnknownInputsDeny(t *testing.T) {
	assertDenied(t, ceiling.RoleState{})
	for field := 0; field < 4; field++ {
		for _, value := range []string{"", "unknown", "*"} {
			assertDenied(t, replaceRoleField(memberState(), field, value))
		}
	}
	state := memberState()
	state.TaskRole = ceiling.Assignee
	assertDenied(t, state)
	state = memberState()
	state.ChannelRole = standing.ChannelRoleNone
	assertDenied(t, state)
}

func replaceRoleField(state ceiling.RoleState, field int, value string) ceiling.RoleState {
	switch field {
	case 0:
		state.Route = ceiling.Route(value)
	case 1:
		state.TaskRole = ceiling.TaskRole(value)
	case 2:
		state.OrganizationRole = standing.OrganizationRole(value)
	case 3:
		state.ChannelRole = standing.ChannelRole(value)
	}
	return state
}

func assertDenied(t *testing.T, state ceiling.RoleState) {
	t.Helper()
	for _, entry := range loadContract(t).Actions {
		if ceiling.WithinRoleCeiling(entry.Action, state) {
			t.Errorf("%s admitted invalid roles %+v", entry.Action, state)
		}
	}
}

func TestEveryCurrentRoleTier(t *testing.T) {
	organizations := []standing.OrganizationRole{
		standing.OrganizationRoleOwner, standing.OrganizationRoleAdmin,
		standing.OrganizationRoleSecurityAdmin, standing.OrganizationRoleMember, standing.OrganizationRoleGuest,
	}
	channels := []standing.ChannelRole{
		standing.ChannelRoleOwner, standing.ChannelRoleModerator, standing.ChannelRoleMember, standing.ChannelRoleGuest,
	}
	for _, org := range organizations {
		for _, channel := range channels {
			state := memberState()
			state.OrganizationRole, state.ChannelRole = org, channel
			if !ceiling.WithinRoleCeiling(ceiling.TaskRead, state) || !ceiling.WithinRoleCeiling(ceiling.RunControl, state) {
				t.Errorf("read/control ceiling lost: %+v", state)
			}
			want := org != standing.OrganizationRoleGuest && channel != standing.ChannelRoleGuest
			if ceiling.WithinRoleCeiling(ceiling.TaskCreate, state) != want || ceiling.WithinRoleCeiling(ceiling.RunStart, state) != want {
				t.Errorf("mixed Guest ceiling widened: %+v", state)
			}
			state.TaskRole = ceiling.None
			if ceiling.WithinRoleCeiling(ceiling.RunStart, state) {
				t.Errorf("Organization/Channel role bypassed OWNER: %+v", state)
			}
		}
	}
}

func TestOwnerSelfApprovalSwitches(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		for _, deployment := range []string{"", "production", "dedicated-dev", "dedicated-dev-unknown"} {
			state := memberState()
			state.SelfApproval = ceiling.SelfApprovalPolicy{
				DeploymentClass: deployment, AllowOwner: mask&1 != 0,
				ConfiguredOwner: mask&2 != 0, OwnerRoleEnabled: mask&4 != 0,
			}
			assertApproval(t, state, mask == 7 && deployment == "dedicated-dev")
		}
	}
	state := memberState()
	state.SelfApproval = ceiling.SelfApprovalPolicy{
		DeploymentClass: "dedicated-dev", AllowOwner: true, ConfiguredOwner: true, OwnerRoleEnabled: true,
	}
	for _, role := range []ceiling.TaskRole{ceiling.None, ceiling.Observer, ceiling.Assignee} {
		state.TaskRole = role
		assertApproval(t, state, false)
	}
	state.TaskRole, state.SelfApproval = ceiling.Approver, ceiling.SelfApprovalPolicy{}
	assertApproval(t, state, true)
}

func assertApproval(t *testing.T, state ceiling.RoleState, want bool) {
	t.Helper()
	for _, action := range []ceiling.Action{ceiling.ApprovalDecide, ceiling.ApprovalRevoke} {
		if got := ceiling.WithinRoleCeiling(action, state); got != want {
			t.Errorf("%s/%+v: got %v want %v", action, state, got, want)
		}
	}
}
