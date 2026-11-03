package ceiling

import standing "github.com/monkeylabx/threadline/services/core/internal/authorization"

type actionClass uint8

const (
	unsupported actionClass = iota
	read
	metadata
	create
	start
	control
	approval
	runtime
)

func classFor(action Action) actionClass {
	switch action {
	case TaskRead, ApprovalRead:
		return read
	case RunRead:
		return metadata
	case TaskCreate:
		return create
	case RunStart:
		return start
	case RunControl:
		return control
	case ApprovalDecide, ApprovalRevoke:
		return approval
	case ApprovalRequest, RunReport:
		return runtime
	default:
		return unsupported
	}
}

// RequiredParentActions returns fresh descriptors, not evaluated permissions.
// Runtime callers must separately bind Agent and operator parent evidence.
func RequiredParentActions(action Action) ([]standing.Action, bool) {
	switch classFor(action) {
	case unsupported:
		return nil, false
	case create, start:
		return []standing.Action{standing.ActionChannelRead, standing.ActionChannelPublish}, true
	default:
		return []standing.Action{standing.ActionChannelRead}, true
	}
}
