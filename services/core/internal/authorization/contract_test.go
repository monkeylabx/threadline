package authorization_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const fixturePath = "../../../../test/fixtures/proto/authorization/scenarios.json"

type contractFixture struct {
	Actions    []string         `json:"actions"`
	RoleMatrix []roleMatrixRow  `json:"roleMatrix"`
	Cases      []map[string]any `json:"cases"`
}

type roleMatrixRow struct {
	OrganizationRole string   `json:"organizationRole"`
	ChannelRole      string   `json:"channelRole"`
	AllowedActions   []string `json:"allowedActions"`
}

type fixtureActor struct {
	TenantID  string `json:"tenantId"`
	ActorType string `json:"actorType"`
	ActorID   string `json:"actorId"`
}

type fixtureResource struct {
	Kind       string `json:"kind"`
	TenantID   string `json:"tenantId"`
	ResourceID string `json:"resourceId"`
	Exists     bool   `json:"exists"`
	Visibility string `json:"visibility"`
	State      string `json:"state"`
}

type fixtureOrganization struct {
	TenantID      string `json:"tenantId"`
	State         string `json:"state"`
	PolicyVersion string `json:"policyVersion"`
}

type fixtureMember struct {
	fixtureActor
	State string `json:"state"`
	Role  string `json:"role"`
}

type fixtureMembership struct {
	fixtureActor
	ChannelID         string   `json:"channelId"`
	Active            bool     `json:"active"`
	Role              string   `json:"role"`
	CurrentIntervalID string   `json:"currentIntervalId"`
	DepartedIntervals []string `json:"departedIntervalIds"`
}

type fixtureACLEntry struct {
	ActorType string `json:"actorType"`
	ActorID   string `json:"actorId"`
	Action    string `json:"action"`
	Effect    string `json:"effect"`
}

type fixtureACL struct {
	Resource      fixtureResource   `json:"resource"`
	Version       string            `json:"version"`
	DefaultEffect string            `json:"defaultEffect"`
	Entries       []fixtureACLEntry `json:"entries"`
}

type fixtureRuntime struct {
	Required          bool `json:"required"`
	CapabilityPresent bool `json:"capabilityPresent"`
	CapabilityAllows  bool `json:"capabilityAllows"`
	DelegationAllows  bool `json:"delegationAllows"`
}

type fixtureApproval struct {
	Required bool `json:"required"`
	Present  bool `json:"present"`
}

type fixtureExpected struct {
	Effect        string `json:"effect"`
	Reason        string `json:"reason"`
	PolicyVersion string `json:"policyVersion"`
	ACLVersion    string `json:"aclVersion"`
}

type fixtureCase struct {
	ID                string              `json:"id"`
	Action            string              `json:"action"`
	Principal         *fixtureActor       `json:"principal"`
	FactsAvailable    bool                `json:"factsAvailable"`
	Resource          fixtureResource     `json:"resource"`
	Organization      fixtureOrganization `json:"organization"`
	Member            fixtureMember       `json:"member"`
	ChannelMembership fixtureMembership   `json:"channelMembership"`
	ACL               fixtureACL          `json:"acl"`
	Runtime           fixtureRuntime      `json:"runtime"`
	Approval          fixtureApproval     `json:"approval"`
	Expected          fixtureExpected     `json:"expected"`
}

type fixtureVerifier struct{ principal fixtureActor }

func (v fixtureVerifier) VerifySession(context.Context, string) (rpcmiddleware.VerifiedSession, error) {
	return rpcmiddleware.VerifiedSession{
		TenantID: v.principal.TenantID, ActorType: actorType(v.principal.ActorType), ActorID: v.principal.ActorID,
		DeviceID: "fixture-device", SessionID: "fixture-session",
	}, nil
}

func TestAuthorizationContractEdgeCases(t *testing.T) {
	t.Parallel()
	fixture := loadContractFixture(t)
	resolved := resolveFixtureCases(t, fixture.Cases)
	if len(resolved) != 54 {
		t.Fatalf("fixture case count = %d, want 54", len(resolved))
	}
	for id, testCase := range resolved {
		t.Run(id, func(t *testing.T) {
			decision, err := evaluateFixtureCase(testCase)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if decision.Effect != authorization.Effect(testCase.Expected.Effect) ||
				decision.Reason != authorization.Reason(testCase.Expected.Reason) ||
				decision.PolicyVersion != testCase.Expected.PolicyVersion ||
				decision.ACLVersion != testCase.Expected.ACLVersion {
				t.Fatalf("Decision = %#v, want effect=%q reason=%q policy=%q acl=%q", decision,
					testCase.Expected.Effect, testCase.Expected.Reason, testCase.Expected.PolicyVersion, testCase.Expected.ACLVersion)
			}
			if testCase.Principal != nil {
				wantResource := authorization.ResourceRef{TenantID: testCase.Resource.TenantID, Kind: authorization.ResourceKind(testCase.Resource.Kind), ID: testCase.Resource.ResourceID}
				wantActor := authorization.ActorRef{Type: actorType(testCase.Principal.ActorType), ID: testCase.Principal.ActorID}
				if decision.Action != authorization.Action(testCase.Action) || decision.Resource != wantResource || decision.Actor != wantActor {
					t.Fatalf("Decision binding = (%q, %#v, %#v), want (%q, %#v, %#v)", decision.Action, decision.Resource, decision.Actor, testCase.Action, wantResource, wantActor)
				}
			}
		})
	}
}

func TestAuthorizationRoleMatrix(t *testing.T) {
	t.Parallel()
	fixture := loadContractFixture(t)
	base := resolveFixtureCases(t, fixture.Cases)["member-read-allowed"]
	cells := 0
	for _, row := range fixture.RoleMatrix {
		allowed := make(map[string]bool, len(row.AllowedActions))
		for _, action := range row.AllowedActions {
			allowed[action] = true
		}
		for _, action := range fixture.Actions {
			cells++
			testCase := base
			testCase.ID = row.OrganizationRole + "/" + row.ChannelRole + "/" + action
			testCase.Action = action
			testCase.Member.Role = row.OrganizationRole
			testCase.ChannelMembership.Role = row.ChannelRole
			testCase.ChannelMembership.Active = row.ChannelRole != string(authorization.ChannelRoleNone)
			if !testCase.ChannelMembership.Active {
				testCase.ChannelMembership.CurrentIntervalID = ""
			}
			if action == string(authorization.ActionSpaceDiscover) || action == string(authorization.ActionSpaceChannelCreate) {
				testCase.Resource.Kind, testCase.Resource.ResourceID = "space", "space-a"
				testCase.ACL.Resource.Kind, testCase.ACL.Resource.ResourceID = "space", "space-a"
			}
			decision, err := evaluateFixtureCase(testCase)
			if err != nil {
				t.Fatalf("%s: Evaluate() error = %v", testCase.ID, err)
			}
			if got := decision.Effect == authorization.EffectAllow; got != allowed[action] {
				t.Fatalf("%s: allow = %v, want %v (%#v)", testCase.ID, got, allowed[action], decision)
			}
		}
	}
	if cells != 275 {
		t.Fatalf("matrix cells = %d, want 275", cells)
	}
}

func TestEvaluateReturnsOnlyContextErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ctx  func() context.Context
		want error
	}{
		{name: "canceled", ctx: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }, want: context.Canceled},
		{name: "deadline", ctx: func() context.Context {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			t.Cleanup(cancel)
			return ctx
		}, want: context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := authorization.Evaluate(test.ctx(), authorization.ActionChannelRead, authorization.CurrentFacts{})
			if !errors.Is(err, test.want) || decision != (authorization.Decision{}) {
				t.Fatalf("Evaluate() = (%#v, %v), want zero Decision and %v", decision, err, test.want)
			}
		})
	}
}

func loadContractFixture(t *testing.T) contractFixture {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture contractFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func resolveFixtureCases(t *testing.T, rawCases []map[string]any) map[string]fixtureCase {
	t.Helper()
	raw := make(map[string]map[string]any, len(rawCases))
	for _, item := range rawCases {
		raw[item["id"].(string)] = item
	}
	resolved := make(map[string]fixtureCase, len(raw))
	var resolve func(string, map[string]bool) map[string]any
	resolve = func(id string, seen map[string]bool) map[string]any {
		if seen[id] {
			t.Fatalf("cyclic fixture inheritance at %s", id)
		}
		seen[id] = true
		item := raw[id]
		if item == nil {
			t.Fatalf("missing fixture %s", id)
		}
		base := map[string]any{}
		if parent, ok := item["extends"].(string); ok {
			base = resolve(parent, seen)
		}
		return deepMerge(base, item)
	}
	for id := range raw {
		data, err := json.Marshal(resolve(id, map[string]bool{}))
		if err != nil {
			t.Fatal(err)
		}
		var testCase fixtureCase
		if err := json.Unmarshal(data, &testCase); err != nil {
			t.Fatal(err)
		}
		resolved[id] = testCase
	}
	return resolved
}

func deepMerge(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overlay {
		if key == "extends" {
			continue
		}
		baseMap, baseOK := result[key].(map[string]any)
		overlayMap, overlayOK := value.(map[string]any)
		if baseOK && overlayOK {
			result[key] = deepMerge(baseMap, overlayMap)
		} else {
			result[key] = value
		}
	}
	return result
}

func evaluateFixtureCase(testCase fixtureCase) (authorization.Decision, error) {
	facts := factsFromFixture(testCase)
	action := authorization.Action(testCase.Action)
	return evaluateWithPrincipal(testCase.Principal, action, facts)
}

func evaluateWithPrincipal(principal *fixtureActor, action authorization.Action, facts authorization.CurrentFacts) (authorization.Decision, error) {
	if principal == nil {
		return authorization.Evaluate(context.Background(), action, facts)
	}
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(fixtureVerifier{principal: *principal})
	request := connect.NewRequest(&testRequest{})
	request.Header().Set("Authorization", "Bearer fixture-credential")
	var decision authorization.Decision
	handler := interceptor.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		var err error
		decision, err = authorization.Evaluate(ctx, action, facts)
		return connect.NewResponse(&testRequest{}), err
	})
	_, err := handler(context.Background(), request)
	return decision, err
}

func factsFromFixture(testCase fixtureCase) authorization.CurrentFacts {
	entries := make([]authorization.ACLEntry, len(testCase.ACL.Entries))
	for index, entry := range testCase.ACL.Entries {
		entries[index] = authorization.ACLEntry{Actor: authorization.ActorRef{Type: actorType(entry.ActorType), ID: entry.ActorID}, Action: authorization.Action(entry.Action), Effect: authorization.ACLEffect(entry.Effect)}
	}
	capability := authorization.CapabilityUnspecified
	if testCase.Runtime.CapabilityPresent {
		capability = authorization.CapabilityDeny
		if testCase.Runtime.CapabilityAllows {
			capability = authorization.CapabilityAllow
		}
	}
	resource := authorization.ResourceRef{TenantID: testCase.Resource.TenantID, Kind: authorization.ResourceKind(testCase.Resource.Kind), ID: testCase.Resource.ResourceID}
	return authorization.CurrentFacts{
		Available:    testCase.FactsAvailable,
		Resource:     authorization.ResourceFacts{Ref: resource, Exists: testCase.Resource.Exists, Visibility: authorization.Visibility(testCase.Resource.Visibility), State: authorization.ResourceState(testCase.Resource.State)},
		Organization: authorization.OrganizationFacts{TenantID: testCase.Organization.TenantID, State: authorization.OrganizationState(testCase.Organization.State), PolicyVersion: testCase.Organization.PolicyVersion},
		Member:       authorization.MemberFacts{TenantID: testCase.Member.TenantID, Actor: authorization.ActorRef{Type: actorType(testCase.Member.ActorType), ID: testCase.Member.ActorID}, State: authorization.MemberState(testCase.Member.State), Role: authorization.OrganizationRole(testCase.Member.Role)},
		Membership:   authorization.ChannelMembershipFacts{TenantID: testCase.ChannelMembership.TenantID, ChannelID: testCase.ChannelMembership.ChannelID, Actor: authorization.ActorRef{Type: actorType(testCase.ChannelMembership.ActorType), ID: testCase.ChannelMembership.ActorID}, Current: testCase.ChannelMembership.Active, IntervalID: testCase.ChannelMembership.CurrentIntervalID, Role: authorization.ChannelRole(testCase.ChannelMembership.Role)},
		ACL:          authorization.ResourceACLFacts{Resource: authorization.ResourceRef{TenantID: testCase.ACL.Resource.TenantID, Kind: authorization.ResourceKind(testCase.ACL.Resource.Kind), ID: testCase.ACL.Resource.ResourceID}, Version: testCase.ACL.Version, DefaultEffect: authorization.ACLEffect(testCase.ACL.DefaultEffect), Entries: entries},
		Runtime:      authorization.RuntimeFacts{Required: testCase.Runtime.Required, Capability: capability, DelegationAllows: testCase.Runtime.DelegationAllows},
		Approval:     authorization.ApprovalFacts{Required: testCase.Approval.Required, Present: testCase.Approval.Present},
	}
}

func actorType(value string) rpcmiddleware.ActorType {
	switch value {
	case "ACTOR_TYPE_HUMAN":
		return rpcmiddleware.ActorTypeHuman
	case "ACTOR_TYPE_AGENT":
		return rpcmiddleware.ActorTypeAgent
	case "ACTOR_TYPE_SERVICE":
		return rpcmiddleware.ActorTypeService
	default:
		return 0
	}
}

func TestFactsRemainImmutable(t *testing.T) {
	t.Parallel()
	fixture := loadContractFixture(t)
	testCase := resolveFixtureCases(t, fixture.Cases)["member-read-allowed"]
	facts := factsFromFixture(testCase)
	before := authorization.CurrentFacts{}
	data, _ := json.Marshal(facts)
	_ = json.Unmarshal(data, &before)
	if _, err := evaluateWithPrincipal(testCase.Principal, authorization.Action(testCase.Action), facts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(facts, before) {
		t.Fatal("Evaluate mutated CurrentFacts")
	}
}

func TestEvaluateSupportsConcurrentRequests(t *testing.T) {
	t.Parallel()
	fixture := loadContractFixture(t)
	testCase := resolveFixtureCases(t, fixture.Cases)["member-read-allowed"]
	facts := factsFromFixture(testCase)
	const requestCount = 128
	errCh := make(chan error, requestCount)
	var wait sync.WaitGroup
	wait.Add(requestCount)
	for index := range requestCount {
		go func() {
			defer wait.Done()
			requestCase := testCase
			principal := *testCase.Principal
			principal.TenantID = fmt.Sprintf("tenant-%d", index)
			principal.ActorID = fmt.Sprintf("actor-%d", index)
			requestCase.Principal = &principal
			facts := facts
			facts.Resource.Ref.TenantID = principal.TenantID
			facts.Resource.Ref.ID = fmt.Sprintf("channel-%d", index)
			facts.Organization.TenantID = principal.TenantID
			facts.Member.TenantID = principal.TenantID
			facts.Member.Actor.ID = principal.ActorID
			facts.Membership.TenantID = principal.TenantID
			facts.Membership.ChannelID = facts.Resource.Ref.ID
			facts.Membership.Actor.ID = principal.ActorID
			facts.ACL.Resource = facts.Resource.Ref
			decision, err := evaluateWithPrincipal(requestCase.Principal, authorization.Action(requestCase.Action), facts)
			if err != nil {
				errCh <- err
				return
			}
			if decision.Effect != authorization.EffectAllow || decision.Reason != authorization.ReasonAllowed {
				errCh <- errors.New("concurrent evaluation did not allow canonical member read")
				return
			}
			if decision.Actor.ID != principal.ActorID || decision.Resource != facts.Resource.Ref {
				errCh <- errors.New("concurrent evaluation crossed request identity or resource facts")
			}
		}()
	}
	wait.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}
