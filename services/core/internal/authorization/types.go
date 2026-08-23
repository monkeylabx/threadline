package authorization

import "github.com/monkeylabx/threadline/services/internal/rpcmiddleware"

type Action string

const (
	ActionSpaceDiscover           Action = "space.discover"
	ActionSpaceChannelCreate      Action = "space.channel.create"
	ActionChannelDiscover         Action = "channel.discover"
	ActionChannelRead             Action = "channel.read"
	ActionChannelPublish          Action = "channel.publish"
	ActionChannelUpdate           Action = "channel.update"
	ActionChannelArchive          Action = "channel.archive"
	ActionChannelMembershipList   Action = "channel.membership.list"
	ActionChannelMembershipAdd    Action = "channel.membership.add"
	ActionChannelMembershipRemove Action = "channel.membership.remove"
	ActionChannelACLUpdate        Action = "channel.acl.update"
)

type Effect string

const (
	EffectDeny  Effect = "deny"
	EffectAllow Effect = "allow"
)

type Reason string

const (
	ReasonAllowed                 Reason = "allowed"
	ReasonAuthenticationRequired  Reason = "authentication-required"
	ReasonTenantMismatch          Reason = "tenant-mismatch"
	ReasonOrganizationUnavailable Reason = "organization-unavailable"
	ReasonMemberInactive          Reason = "member-inactive"
	ReasonNotAMember              Reason = "not-a-member"
	ReasonOrganizationRoleDenied  Reason = "organization-role-denied"
	ReasonChannelRoleDenied       Reason = "channel-role-denied"
	ReasonResourceStateDenied     Reason = "resource-state-denied"
	ReasonACLMatchedDeny          Reason = "acl-matched-deny"
	ReasonACLDefaultDeny          Reason = "acl-default-deny"
	ReasonACLInvalid              Reason = "acl-invalid"
	ReasonPolicyVersionInvalid    Reason = "policy-version-invalid"
	ReasonACLVersionInvalid       Reason = "acl-version-invalid"
	ReasonCapabilityRequired      Reason = "capability-required"
	ReasonCapabilityDenied        Reason = "capability-denied"
	ReasonDelegationDenied        Reason = "delegation-denied"
	ReasonApprovalRequired        Reason = "approval-required"
	ReasonFactsUnavailable        Reason = "facts-unavailable"
	ReasonUnknownAction           Reason = "unknown-action"
	ReasonUnknownResource         Reason = "unknown-resource"
)

type ResourceKind string

const (
	ResourceKindSpace   ResourceKind = "space"
	ResourceKindChannel ResourceKind = "channel"
)

type Visibility string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

type ResourceState string

const (
	ResourceStateActive          ResourceState = "active"
	ResourceStateArchived        ResourceState = "archived"
	ResourceStatePendingDeletion ResourceState = "pending-deletion"
)

type OrganizationState string

const OrganizationStateActive OrganizationState = "active"

type MemberState string

const MemberStateActive MemberState = "active"

type OrganizationRole string

const (
	OrganizationRoleOwner         OrganizationRole = "ROLE_OWNER"
	OrganizationRoleAdmin         OrganizationRole = "ROLE_ADMIN"
	OrganizationRoleSecurityAdmin OrganizationRole = "ROLE_SECURITY_ADMIN"
	OrganizationRoleMember        OrganizationRole = "ROLE_MEMBER"
	OrganizationRoleGuest         OrganizationRole = "ROLE_GUEST"
)

type ChannelRole string

const (
	ChannelRoleNone      ChannelRole = "CHANNEL_MEMBER_ROLE_NONE"
	ChannelRoleOwner     ChannelRole = "CHANNEL_MEMBER_ROLE_OWNER"
	ChannelRoleModerator ChannelRole = "CHANNEL_MEMBER_ROLE_MODERATOR"
	ChannelRoleMember    ChannelRole = "CHANNEL_MEMBER_ROLE_MEMBER"
	ChannelRoleGuest     ChannelRole = "CHANNEL_MEMBER_ROLE_GUEST"
)

type ACLEffect string

const (
	ACLEffectAllow ACLEffect = "allow"
	ACLEffectDeny  ACLEffect = "deny"
)

type CapabilityDecision uint8

const (
	CapabilityUnspecified CapabilityDecision = iota
	CapabilityAllow
	CapabilityDeny
)

type ActorRef struct {
	Type rpcmiddleware.ActorType
	ID   string
}
type ResourceRef struct {
	TenantID string
	Kind     ResourceKind
	ID       string
}
type ResourceFacts struct {
	Ref        ResourceRef
	Exists     bool
	Visibility Visibility
	State      ResourceState
}
type OrganizationFacts struct {
	TenantID      string
	State         OrganizationState
	PolicyVersion string
}
type MemberFacts struct {
	TenantID string
	Actor    ActorRef
	State    MemberState
	Role     OrganizationRole
}
type ChannelMembershipFacts struct {
	TenantID   string
	ChannelID  string
	Actor      ActorRef
	Current    bool
	IntervalID string
	Role       ChannelRole
}
type ACLEntry struct {
	Actor  ActorRef
	Action Action
	Effect ACLEffect
}
type ResourceACLFacts struct {
	Resource      ResourceRef
	Version       string
	DefaultEffect ACLEffect
	Entries       []ACLEntry
}
type RuntimeFacts struct {
	Required         bool
	Capability       CapabilityDecision
	DelegationAllows bool
}
type ApprovalFacts struct {
	Required bool
	Present  bool
}

// CurrentFacts is one complete, request-bound snapshot. It deliberately omits
// departed membership history and cryptographic authority.
type CurrentFacts struct {
	Available    bool
	Resource     ResourceFacts
	Organization OrganizationFacts
	Member       MemberFacts
	Membership   ChannelMembershipFacts
	ACL          ResourceACLFacts
	Runtime      RuntimeFacts
	Approval     ApprovalFacts
}

// Decision is evidence for one evaluation, not a reusable permission cache.
type Decision struct {
	Effect        Effect
	Reason        Reason
	Action        Action
	Resource      ResourceRef
	Actor         ActorRef
	PolicyVersion string
	ACLVersion    string
}
