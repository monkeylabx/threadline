// Package evidence checks resolved ancestry and caller equality assertions.
package evidence

import "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"

var resourceLevels = map[model.ResourceKind]model.Level{
	model.ResourceChannel: model.LevelChannel, model.ResourceTask: model.LevelTask,
	model.ResourceRun: model.LevelRun, model.ResourceApproval: model.LevelApproval,
}

func MatchesAssertions(req, stored model.Ancestry) bool {
	pairs := [4][2]string{
		{req.Channel, stored.Channel}, {req.Task, stored.Task},
		{req.Run, stored.Run}, {req.Approval, stored.Approval},
	}
	for _, pair := range pairs {
		if pair[0] != "" && pair[0] != pair[1] {
			return false
		}
	}
	return true
}

func CompleteAncestry(res model.ResourceRef, a model.Ancestry) bool {
	level, known := resourceLevels[res.Kind]
	if !known {
		return false
	}
	ids := [4]string{a.Channel, a.Task, a.Run, a.Approval}
	return ids[int(level)-1] == res.ID && CompleteAtLevel(a, level)
}

func CompleteAtLevel(a model.Ancestry, level model.Level) bool {
	if level > model.LevelApproval {
		return false
	}
	for i, id := range [4]string{a.Channel, a.Task, a.Run, a.Approval} {
		if i < int(level) {
			if id == "" {
				return false
			}
		} else if id != "" {
			return false
		}
	}
	return true
}
