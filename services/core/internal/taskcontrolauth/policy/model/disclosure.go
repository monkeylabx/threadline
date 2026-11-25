package model

import standing "github.com/monkeylabx/threadline/services/core/internal/authorization"

const (
	PolicyProfile          = "task-control-auth/1"
	DeploymentDedicatedDev = "dedicated-dev"
)

type ReadAudience string

const (
	ReadChannelReaders          ReadAudience = "channel_readers"
	ReadAssignedRuntimeMetadata ReadAudience = "assigned_runtime_metadata"
)

type ParentFacts struct {
	Bound
	State      standing.ResourceState
	Membership standing.ChannelMembershipFacts
	ACL        standing.ResourceACLFacts
	Decisions  []standing.Decision
}

type PolicyRule struct {
	Audiences []ReadAudience
	Roles     []TaskRole
}

type PolicyFacts struct {
	Bound
	Profile, Revision, Deployment                                string
	Rules                                                        map[Action]PolicyRule // key present = enabled
	ActorDenied, AllowOwnerSelfApproval, ConfiguredOwnerApprover bool
}

type Participant struct {
	Actor Actor
	Role  TaskRole
}

type TaskFacts struct {
	Bound
	Creator                        Actor
	Participants                   []Participant
	ManifestRevision, TriggerEvent string
}

type ReceiptNamespace struct {
	Actor                        Actor
	Device, RPC, Enrollment, Run string
	RuntimeGeneration            uint64
}

type ReceiptFacts struct {
	Bound
	Committed        bool
	Namespace        ReceiptNamespace
	Action           Action
	Resource, Result ResourceRef
	RequestDigest    [32]byte
}
