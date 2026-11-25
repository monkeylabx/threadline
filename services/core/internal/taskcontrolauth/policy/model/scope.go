package model

type Snapshot struct {
	TransactionID string
	Revision      uint64
}

type Level uint8

const (
	LevelTenant Level = iota
	LevelChannel
	LevelTask
	LevelRun
	LevelApproval
)

// Truncate returns the ancestry prefix bounded at level.
func (a Ancestry) Truncate(l Level) Ancestry {
	switch l {
	case LevelChannel:
		return Ancestry{Channel: a.Channel}
	case LevelTask:
		return Ancestry{Channel: a.Channel, Task: a.Task}
	case LevelRun:
		return Ancestry{Channel: a.Channel, Task: a.Task, Run: a.Run}
	case LevelApproval:
		return a
	default:
		return Ancestry{}
	}
}

type Bound struct {
	Snapshot Snapshot
	Tenant   string
	Ancestry Ancestry
}

type Scope = Bound
