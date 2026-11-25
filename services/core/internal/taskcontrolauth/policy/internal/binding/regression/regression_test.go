package regression_test

import (
	"context"
	"strings"
	"testing"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding"
	fixture "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/testfixture"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func TestTaskScopedReadsAndMatchingAssertions(t *testing.T) {
	cases := []struct {
		action model.Action
		kind   model.ResourceKind
		bound  bool
	}{
		{"task.read", model.ResourceTask, true}, {"run.read", model.ResourceTask, true},
		{"run.read", model.ResourceRun, true}, {"approval.read", model.ResourceTask, true},
		{"approval.read", model.ResourceApproval, true},
		{"run.read", model.ResourceChannel, false}, {"run.read", model.ResourceApproval, false},
		{"approval.read", model.ResourceChannel, false}, {"approval.read", model.ResourceRun, false},
	}
	for _, tc := range cases {
		p, req, facts := fixture.BaseHuman()
		ancestor := fixture.SetTarget(&req, &facts, tc.action, tc.kind)
		req.Ancestry = ancestor
		if tc.bound {
			fixture.AssertBound(t, p, req, facts, ancestor)
			continue
		}
		result, err := binding.Validate(context.Background(), p, req, facts)
		if err != nil {
			t.Fatal(err)
		}
		fixture.AssertRefusal(t, result.Refusal, binding.ReasonShapeInvalid, model.StatusInvalidArgument, "")
	}
}

func TestAncestryAssertionMismatches(t *testing.T) {
	for field := 0; field < 4; field++ {
		p, req, facts := fixture.BaseHuman()
		req.Ancestry = fixture.SetTarget(&req, &facts, "approval.read", model.ResourceApproval)
		switch field {
		case 0:
			req.Ancestry.Channel = "other-channel"
		case 1:
			req.Ancestry.Task = "other-task"
		case 2:
			req.Ancestry.Run = "other-run"
		case 3:
			req.Ancestry.Approval = "other-approval"
		}
		result, err := binding.Validate(context.Background(), p, req, facts)
		if err != nil {
			t.Fatal(err)
		}
		fixture.AssertRefusal(t, result.Refusal, binding.ReasonObjectAbsent, model.StatusNotFound, "")
	}
}

func TestMalformedResolvedAncestors(t *testing.T) {
	for _, id := range []string{"bad\nchannel", "\xff", strings.Repeat("c", 129)} {
		p, req, facts := fixture.BaseHuman()
		facts.Lookup.Ancestry.Channel = id
		result, err := binding.Validate(context.Background(), p, req, facts)
		if err != nil {
			t.Fatal(err)
		}
		fixture.AssertRefusal(t, result.Refusal, binding.ReasonFactsBindingInvalid, model.StatusUnavailable, "")
	}
	p, req, facts := fixture.BaseHuman()
	id := strings.Repeat("c", 128)
	facts.Lookup.Ancestry.Channel = id
	facts.Parent.Ancestry.Channel, facts.Policy.Ancestry.Channel, facts.Task.Ancestry.Channel = id, id, id
	facts.Parent.Membership.ChannelID, facts.Parent.ACL.Resource.ID = id, id
	for i := range facts.Parent.Decisions {
		facts.Parent.Decisions[i].Resource.ID = id
	}
	fixture.AssertBound(t, p, req, facts, facts.Lookup.Ancestry)
}
