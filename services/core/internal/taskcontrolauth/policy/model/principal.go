package model

import (
	"time"

	standing "github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/ceiling"
)

type Action = ceiling.Action
type Route = ceiling.Route
type TaskRole = ceiling.TaskRole
type Actor = standing.ActorRef
type Audience string

const (
	AudienceHumanSession   Audience = "threadline.human-session"
	AudienceRuntimeControl Audience = "threadline.runtime-control"
)

// Principal holds verified caller identity attributes for one evaluation.
// For AssignedRuntime, Actor is the assigned Agent and Operator is the bound Human.
type Principal struct {
	Route             Route
	Tenant            string
	Actor             Actor
	Device            string
	SessionGeneration uint64
	Audience          Audience
	ExpiresAt         time.Time
	Operator          Actor
	Enrollment        string
	RuntimeGeneration uint64
}
