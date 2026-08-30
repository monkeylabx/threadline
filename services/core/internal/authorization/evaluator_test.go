package authorization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

type staticVerifier struct{}

func (staticVerifier) VerifySession(context.Context, string) (rpcmiddleware.VerifiedSession, error) {
	return rpcmiddleware.VerifiedSession{
		TenantID:  "tenant-a",
		ActorType: rpcmiddleware.ActorTypeHuman,
		ActorID:   "actor-alice",
		DeviceID:  "device-a",
		SessionID: "session-a",
	}, nil
}

type testRequest struct{}

func TestEvaluateRequiresAuthenticatedPrincipal(t *testing.T) {
	t.Parallel()

	decision, err := authorization.Evaluate(
		context.Background(),
		authorization.ActionChannelRead,
		authorization.CurrentFacts{},
	)
	if err != nil {
		t.Fatalf("Evaluate() error = %v, want nil policy error", err)
	}
	if decision.Effect != authorization.EffectDeny {
		t.Fatalf("Decision.Effect = %q, want %q", decision.Effect, authorization.EffectDeny)
	}
	if decision.Reason != authorization.ReasonAuthenticationRequired {
		t.Fatalf("Decision.Reason = %q, want %q", decision.Reason, authorization.ReasonAuthenticationRequired)
	}
}

func TestEvaluateAllowsMemberReadFromAuthenticatedContext(t *testing.T) {
	t.Parallel()

	interceptor := rpcmiddleware.NewAuthenticationInterceptor(staticVerifier{})
	request := connect.NewRequest(&testRequest{})
	request.Header().Set("Authorization", "Bearer fixture-credential")
	var decision authorization.Decision
	handler := interceptor.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		var err error
		decision, err = authorization.Evaluate(ctx, authorization.ActionChannelRead, authorization.CurrentFacts{
			Available: true,
			Resource: authorization.ResourceFacts{
				Ref:        authorization.ResourceRef{TenantID: "tenant-a", Kind: authorization.ResourceKindChannel, ID: "channel-a"},
				Exists:     true,
				Visibility: authorization.VisibilityPublic,
				State:      authorization.ResourceStateActive,
			},
			Organization: authorization.OrganizationFacts{TenantID: "tenant-a", State: authorization.OrganizationStateActive, PolicyVersion: "policy-7"},
			Member:       authorization.MemberFacts{TenantID: "tenant-a", Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-alice"}, State: authorization.MemberStateActive, Role: authorization.OrganizationRoleMember},
			Membership:   authorization.ChannelMembershipFacts{TenantID: "tenant-a", ChannelID: "channel-a", Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-alice"}, Current: true, IntervalID: "membership-2", Role: authorization.ChannelRoleMember},
			ACL:          authorization.ResourceACLFacts{Resource: authorization.ResourceRef{TenantID: "tenant-a", Kind: authorization.ResourceKindChannel, ID: "channel-a"}, Version: "acl-3", DefaultEffect: authorization.ACLEffectAllow},
		})
		return connect.NewResponse(&testRequest{}), err
	})
	if _, err := handler(context.Background(), request); err != nil {
		t.Fatalf("authenticated Evaluate() error = %v", err)
	}
	if decision.Effect != authorization.EffectAllow || decision.Reason != authorization.ReasonAllowed {
		t.Fatalf("Decision = %#v, want allowed", decision)
	}
}
