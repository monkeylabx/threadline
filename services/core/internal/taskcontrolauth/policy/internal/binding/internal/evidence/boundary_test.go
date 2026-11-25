package evidence_test

import (
	"testing"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/evidence"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func TestClosedAncestryEvidence(t *testing.T) {
	a := model.Ancestry{Channel: "channel", Task: "task"}
	if !evidence.CompleteAncestry(model.ResourceRef{Kind: model.ResourceTask, ID: "task"}, a) {
		t.Fatal("complete addressed Task ancestry rejected")
	}
	if evidence.CompleteAncestry(model.ResourceRef{Kind: "unknown", ID: "task"}, a) {
		t.Fatal("unknown resource kind bound")
	}
	if evidence.CompleteAtLevel(model.Ancestry{}, model.Level(99)) {
		t.Fatal("undefined ancestry level bound")
	}
}
