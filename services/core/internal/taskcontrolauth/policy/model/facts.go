package model

import (
	"time"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
)

type Facts struct {
	Snapshot     Snapshot
	ObservedAt   time.Time
	Organization OrganizationFacts
	Member       MemberFacts
	Device       DeviceFacts
	Lookup       LookupFacts
}

type OrganizationFacts struct {
	Bound
	Active        bool
	PolicyVersion string
}

type MemberFacts struct {
	Bound
	Actor  Actor
	Active bool
	Role   standing.OrganizationRole
}

type DeviceFacts struct {
	Bound
	Owner   Actor
	Device  string
	Current bool
}

type LookupFacts struct {
	Bound
	Found    bool
	Resource ResourceRef
}
