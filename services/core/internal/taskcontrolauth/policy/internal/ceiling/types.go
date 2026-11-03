// Package ceiling describes necessary Task role ceilings, never Authorization
// Decisions. Authentication, current policy and bound facts remain required.
package ceiling

import standing "github.com/monkeylabx/threadline/services/core/internal/authorization"

type Action string

const (
	TaskCreate      Action = "task.create"
	TaskRead        Action = "task.read"
	RunStart        Action = "run.start"
	RunRead         Action = "run.read"
	RunControl      Action = "run.control"
	ApprovalRead    Action = "approval.read"
	ApprovalRequest Action = "approval.request"
	ApprovalDecide  Action = "approval.decide"
	ApprovalRevoke  Action = "approval.revoke"
	RunReport       Action = "run.report"
)

type Route string

const (
	Human           Route = "human"
	AssignedRuntime Route = "assigned-runtime"
)

type TaskRole string

const (
	None     TaskRole = "NONE"
	Owner    TaskRole = "OWNER"
	Assignee TaskRole = "ASSIGNEE"
	Approver TaskRole = "APPROVER"
	Observer TaskRole = "OBSERVER"
)

// SelfApprovalPolicy contains independently resolved, current configuration.
// These switches do not establish snapshot eligibility or resource authority.
type SelfApprovalPolicy struct {
	DeploymentClass                               string
	AllowOwner, ConfiguredOwner, OwnerRoleEnabled bool
}

type RoleState struct {
	Route            Route
	TaskRole         TaskRole
	OrganizationRole standing.OrganizationRole
	ChannelRole      standing.ChannelRole
	SelfApproval     SelfApprovalPolicy
}
