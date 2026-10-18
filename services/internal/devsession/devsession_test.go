package devsession

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const fixtureDocument = `{
  "sessions": [
    {"token": "dev-alice-laptop-token-synthetic", "tenant_id": "tenant-dev-synthetic", "actor_type": "human",
     "actor_id": "actor-alice-synthetic", "device_id": "device-alice-laptop-synthetic", "session_id": "session-alice-laptop-synthetic"},
    {"token": "dev-alice-phone-token-synthetic", "tenant_id": "tenant-dev-synthetic", "actor_type": "human",
     "actor_id": "actor-alice-synthetic", "device_id": "device-alice-phone-synthetic", "session_id": "session-alice-phone-synthetic",
     "expires_at": "2026-10-19T00:00:00Z"},
    {"token": "dev-bob-laptop-token-synthetic", "tenant_id": "tenant-dev-synthetic", "actor_type": "human",
     "actor_id": "actor-bob-synthetic", "device_id": "device-bob-laptop-synthetic", "session_id": "session-bob-laptop-synthetic"},
    {"token": "dev-bob-revoked-token-synthetic", "tenant_id": "tenant-dev-synthetic", "actor_type": "human",
     "actor_id": "actor-bob-synthetic", "device_id": "device-bob-old-synthetic", "session_id": "session-bob-old-synthetic", "revoked": true}
  ]
}`

func parsedFixtures(t *testing.T) []Fixture {
	t.Helper()
	fixtures, err := Parse([]byte(fixtureDocument))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return fixtures
}

func TestParseAcceptsTheDevDocumentAndRejectsMalformedOnes(t *testing.T) {
	t.Parallel()
	if fixtures := parsedFixtures(t); len(fixtures) != 4 || fixtures[1].ExpiresAt.IsZero() || !fixtures[3].Revoked {
		t.Fatalf("Parse() = %+v, want four fixtures with expiry and revocation preserved", fixtures)
	}
	cases := map[string]string{
		"empty":            "",
		"unknown field":    strings.Replace(fixtureDocument, `"revoked": true`, `"revoked": true, "password": "x"`, 1),
		"no sessions":      `{"sessions": []}`,
		"duplicate token":  strings.Replace(fixtureDocument, "dev-bob-laptop-token-synthetic", "dev-alice-laptop-token-synthetic", 1),
		"short token":      strings.Replace(fixtureDocument, "dev-bob-laptop-token-synthetic", "short", 1),
		"unknown actor":    strings.Replace(fixtureDocument, `"actor_type": "human"`, `"actor_type": "root"`, 1),
		"blank device":     strings.Replace(fixtureDocument, "device-bob-laptop-synthetic", "", 1),
		"padded tenant":    strings.Replace(fixtureDocument, `"tenant_id": "tenant-dev-synthetic"`, `"tenant_id": " tenant-dev-synthetic"`, 1),
		"trailing content": fixtureDocument + " {}",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(input))
			if err == nil {
				t.Fatal("Parse() error = nil, want rejection")
			}
			if strings.Contains(err.Error(), "token-synthetic") {
				t.Fatal("Parse() error echoed a token")
			}
		})
	}
}

func TestNewRefusesUnlessExplicitlyEnabledWithFixtures(t *testing.T) {
	t.Parallel()
	fixtures := parsedFixtures(t)
	if _, err := New(Options{Fixtures: fixtures}); err == nil {
		t.Fatal("New(disabled) error = nil, want refusal")
	}
	if _, err := New(Options{Enabled: true}); err == nil {
		t.Fatal("New(no fixtures) error = nil, want refusal")
	}
	broken := append([]Fixture(nil), fixtures...)
	broken[0].SessionID = ""
	if _, err := New(Options{Enabled: true, Fixtures: broken}); err == nil {
		t.Fatal("New(invalid fixture) error = nil, want refusal")
	}
	if _, err := New(Options{Enabled: true, Fixtures: fixtures}); err != nil {
		t.Fatalf("New(enabled) error = %v", err)
	}
}

func TestVerifySessionMapsFixturesToOutcomes(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC)
	verifier, err := New(Options{Enabled: true, Fixtures: parsedFixtures(t), Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	session, err := verifier.VerifySession(context.Background(), "dev-alice-laptop-token-synthetic")
	if err != nil || session != (rpcmiddleware.VerifiedSession{
		TenantID: "tenant-dev-synthetic", ActorType: rpcmiddleware.ActorTypeHuman, ActorID: "actor-alice-synthetic",
		DeviceID: "device-alice-laptop-synthetic", SessionID: "session-alice-laptop-synthetic",
	}) {
		t.Fatalf("VerifySession(alice laptop) = %+v %v, want the fixture claims", session, err)
	}
	live, err := verifier.VerifySession(context.Background(), "dev-alice-phone-token-synthetic")
	if err != nil || live.DeviceID != "device-alice-phone-synthetic" {
		t.Fatalf("VerifySession(unexpired) = %+v %v, want success before expiry", live, err)
	}
	failures := map[string]struct {
		credential string
		clock      time.Time
		want       rpcmiddleware.VerificationFailure
	}{
		"unknown token":   {"dev-nobody-token-synthetic", clock, rpcmiddleware.VerificationRejected},
		"prefix of token": {"dev-alice-laptop-token", clock, rpcmiddleware.VerificationRejected},
		"revoked":         {"dev-bob-revoked-token-synthetic", clock, rpcmiddleware.VerificationRevoked},
		"expired":         {"dev-alice-phone-token-synthetic", time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC), rpcmiddleware.VerificationExpired},
	}
	for name, tc := range failures {
		clock = tc.clock
		_, err := verifier.VerifySession(context.Background(), tc.credential)
		var failure *rpcmiddleware.VerificationError
		if !errors.As(err, &failure) || failure.Failure() != tc.want {
			t.Fatalf("VerifySession(%s) error = %v, want failure %d", name, err, tc.want)
		}
		if strings.Contains(err.Error(), "token") {
			t.Fatalf("VerifySession(%s) error echoed credential material", name)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := verifier.VerifySession(canceled, "dev-alice-laptop-token-synthetic"); !errors.Is(err, context.Canceled) {
		t.Fatalf("VerifySession(canceled) error = %v, want context.Canceled", err)
	}
}

func TestVerifierProducesAPrincipalThroughTheInterceptor(t *testing.T) {
	t.Parallel()
	verifier, err := New(Options{Enabled: true, Fixtures: parsedFixtures(t)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	unary := rpcmiddleware.NewAuthenticationInterceptor(verifier).WrapUnary(
		func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
			principal, ok := rpcmiddleware.PrincipalFromContext(ctx)
			if !ok || principal.TenantID() != "tenant-dev-synthetic" || principal.Actor().ID() != "actor-bob-synthetic" ||
				principal.DeviceID() != "device-bob-laptop-synthetic" {
				t.Fatalf("principal = %+v %v, want bob's laptop", principal, ok)
			}
			return nil, nil
		},
	)
	request := connect.NewRequest(&struct{}{})
	request.Header().Set("Authorization", "Bearer dev-bob-laptop-token-synthetic")
	if _, err := unary(context.Background(), request); err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
	revoked := connect.NewRequest(&struct{}{})
	revoked.Header().Set("Authorization", "Bearer dev-bob-revoked-token-synthetic")
	if _, err := unary(context.Background(), revoked); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("interceptor with revoked fixture = %v, want CodeUnauthenticated", err)
	}
}
