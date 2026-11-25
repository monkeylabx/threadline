package credential_test

import (
	"testing"

	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/credential"
	fixture "github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/internal/binding/internal/testfixture"
	"github.com/monkeylabx/threadline/services/core/internal/taskcontrolauth/policy/model"
)

func TestOtherVerifiedActorObservations(t *testing.T) {
	p, req, facts := fixture.BaseHuman()
	p.Route, p.Actor = "bare-agent", fixture.Golden().Agent
	for _, audience := range []model.Audience{model.AudienceHumanSession, model.AudienceRuntimeControl} {
		p.Audience = audience
		if refusal := credential.CheckAuthentication(p, req, facts); refusal.Refused() {
			t.Fatalf("verified observation refused: %+v", refusal)
		}
	}
	p.Audience = "unknown"
	if refusal := credential.CheckAuthentication(p, req, facts); refusal.Status != model.StatusUnauthenticated {
		t.Fatalf("unknown audience accepted: %+v", refusal)
	}
}
