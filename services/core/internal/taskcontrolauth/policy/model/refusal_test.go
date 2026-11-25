package model_test

import (
	"testing"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func TestRefusalOnlyAcceptsExactZero(t *testing.T) {
	cases := []struct {
		name  string
		value model.Refusal
		want  bool
	}{
		{"zero", model.Refusal{}, false},
		{"reason-only", model.Refusal{Reason: "invalid-binding"}, true},
		{"code-only", model.Refusal{Code: model.CodePermissionDenied}, true},
		{"status-only", model.Refusal{Status: model.StatusUnavailable}, true},
		{"unknown-status", model.Refusal{Status: "unknown"}, true},
		{"complete", model.Refusal{Reason: "invalid-binding", Status: model.StatusUnavailable}, true},
	}
	for _, tc := range cases {
		if got := tc.value.Refused(); got != tc.want {
			t.Errorf("%s: refused=%v, want %v", tc.name, got, tc.want)
		}
	}
}
