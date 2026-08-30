package aclstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/aclstore"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

type authorizationVerifier struct{}

func (authorizationVerifier) VerifySession(context.Context, string) (rpcmiddleware.VerifiedSession, error) {
	return rpcmiddleware.VerifiedSession{
		TenantID: "tenant-acl-store-synthetic", ActorType: rpcmiddleware.ActorTypeHuman,
		ActorID: "actor-acl-store-synthetic", DeviceID: "device-acl-store-synthetic",
		SessionID: "session-acl-store-synthetic",
	}, nil
}

type authorizationRequest struct{}

func TestLoadCurrentDoesNotSynthesizeACL(t *testing.T) {
	ctx, database := openACLStoreDatabase(t)
	createACLStoreFixtures(t, ctx, database)
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	got, err := aclstore.LoadCurrent(ctx, tx, authorization.ResourceRef{
		TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindSpace, ID: "space-acl-store-synthetic",
	})
	if !hasStoreErrorCode(err, aclstore.ErrorCurrentNotFound) || !reflect.DeepEqual(got, authorization.ResourceACLFacts{}) {
		t.Fatalf("LoadCurrent() = (%#v, %v), want zero facts and current-not-found", got, err)
	}
}

func TestReplaceCurrentMakesCompleteACLLoadable(t *testing.T) {
	ctx, database := openACLStoreDatabase(t)
	createACLStoreFixtures(t, ctx, database)

	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	replacement := aclstore.Replacement{
		Resource:      authorization.ResourceRef{TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindSpace, ID: "space-acl-store-synthetic"},
		DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{
			{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}, Action: authorization.ActionSpaceDiscover, Effect: authorization.ACLEffectAllow},
		},
	}
	created, err := aclstore.ReplaceCurrent(ctx, tx, replacement)
	if err != nil {
		t.Fatalf("ReplaceCurrent() error = %v", err)
	}
	if created.Version == "" {
		t.Fatal("ReplaceCurrent() returned an empty server-generated version")
	}
	loaded, err := aclstore.LoadCurrent(ctx, tx, replacement.Resource)
	if err != nil {
		t.Fatalf("LoadCurrent() error = %v", err)
	}
	if !reflect.DeepEqual(created, loaded) {
		t.Fatalf("LoadCurrent() = %#v, want created %#v", loaded, created)
	}
	if loaded.Resource != replacement.Resource || loaded.DefaultEffect != replacement.DefaultEffect ||
		!reflect.DeepEqual(loaded.Entries, replacement.Entries) {
		t.Fatalf("loaded ACL = %#v, want exact replacement %#v", loaded, replacement)
	}
}

func TestChannelACLCanonicalizesAllActionsWithoutMutatingInput(t *testing.T) {
	ctx, database := openACLStoreDatabase(t)
	createACLStoreFixtures(t, ctx, database)
	actions := []authorization.Action{
		authorization.ActionSpaceDiscover,
		authorization.ActionSpaceChannelCreate,
		authorization.ActionChannelDiscover,
		authorization.ActionChannelRead,
		authorization.ActionChannelPublish,
		authorization.ActionChannelUpdate,
		authorization.ActionChannelArchive,
		authorization.ActionChannelMembershipList,
		authorization.ActionChannelMembershipAdd,
		authorization.ActionChannelMembershipRemove,
		authorization.ActionChannelACLUpdate,
	}
	human := authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}
	input := make([]authorization.ACLEntry, 0, len(actions)+2)
	for index := len(actions) - 1; index >= 0; index-- {
		input = append(input, authorization.ACLEntry{Actor: human, Action: actions[index], Effect: authorization.ACLEffectDeny})
	}
	input = append(input,
		authorization.ACLEntry{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeService, ID: "service-acl-store-synthetic"}, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow},
		authorization.ACLEntry{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeAgent, ID: "agent-acl-store-synthetic"}, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow},
	)
	before := append([]authorization.ACLEntry(nil), input...)
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	created, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource:      authorization.ResourceRef{TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindChannel, ID: "channel-acl-store-synthetic"},
		DefaultEffect: authorization.ACLEffectAllow,
		Entries:       input,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("ReplaceCurrent mutated caller entries")
	}
	want := make([]authorization.ACLEntry, 0, len(actions)+2)
	for _, action := range actions {
		want = append(want, authorization.ACLEntry{Actor: human, Action: action, Effect: authorization.ACLEffectDeny})
	}
	want = append(want,
		authorization.ACLEntry{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeAgent, ID: "agent-acl-store-synthetic"}, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow},
		authorization.ACLEntry{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeService, ID: "service-acl-store-synthetic"}, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow},
	)
	if !reflect.DeepEqual(created.Entries, want) {
		t.Fatalf("canonical entries = %#v, want %#v", created.Entries, want)
	}
}

func TestFailedOrRolledBackReplacementPreservesPriorCurrentACL(t *testing.T) {
	ctx, database := openACLStoreDatabase(t)
	createACLStoreFixtures(t, ctx, database)
	resource := authorization.ResourceRef{TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindSpace, ID: "space-acl-store-synthetic"}
	initial := replaceAndCommit(t, ctx, database, aclstore.Replacement{
		Resource: resource, DefaultEffect: authorization.ACLEffectAllow,
		Entries: []authorization.ACLEntry{{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}, Action: authorization.ActionSpaceDiscover, Effect: authorization.ACLEffectAllow}},
	})

	failedTx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = aclstore.ReplaceCurrent(ctx, failedTx, aclstore.Replacement{
		Resource: resource, DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{
			{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}, Action: authorization.ActionSpaceDiscover, Effect: authorization.ACLEffectDeny},
			{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "missing-actor-acl-store-synthetic"}, Action: authorization.ActionSpaceDiscover, Effect: authorization.ACLEffectAllow},
		},
	})
	if !hasStoreErrorCode(err, aclstore.ErrorPersistence) {
		t.Fatalf("mid-replacement failure = %v, want persistence-failure", err)
	}
	if err := failedTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertCurrentACL(t, ctx, database, resource, initial)

	rolledBackTx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rolledBack, err := aclstore.ReplaceCurrent(ctx, rolledBackTx, aclstore.Replacement{
		Resource: resource, DefaultEffect: authorization.ACLEffectDeny,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Version == initial.Version {
		t.Fatal("replacement reused an ACL version")
	}
	if err := rolledBackTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertCurrentACL(t, ctx, database, resource, initial)
}

func hasStoreErrorCode(err error, code aclstore.ErrorCode) bool {
	var storeError *aclstore.Error
	return errors.As(err, &storeError) && storeError.Code() == code
}

func TestConcurrentReplacementsProduceOneCompleteCurrentACL(t *testing.T) {
	ctx, database := openACLStoreDatabase(t)
	createACLStoreFixtures(t, ctx, database)
	resource := authorization.ResourceRef{TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindChannel, ID: "channel-acl-store-synthetic"}
	replacements := []aclstore.Replacement{
		{Resource: resource, DefaultEffect: authorization.ACLEffectAllow, Entries: []authorization.ACLEntry{{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow}}},
		{Resource: resource, DefaultEffect: authorization.ACLEffectDeny, Entries: []authorization.ACLEntry{{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeAgent, ID: "agent-acl-store-synthetic"}, Action: authorization.ActionChannelPublish, Effect: authorization.ACLEffectDeny}}},
	}
	type result struct {
		facts authorization.ResourceACLFacts
		err   error
	}
	results := make(chan result, len(replacements))
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(len(replacements))
	for _, replacement := range replacements {
		replacement := replacement
		go func() {
			ready.Done()
			<-start
			connection, err := pgx.ConnectConfig(ctx, database.Config().Copy())
			if err != nil {
				results <- result{err: err}
				return
			}
			defer func() { _ = connection.Close(context.Background()) }()
			tx, err := connection.Begin(ctx)
			if err != nil {
				results <- result{err: err}
				return
			}
			facts, err := aclstore.ReplaceCurrent(ctx, tx, replacement)
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			results <- result{facts: facts, err: err}
		}()
	}
	ready.Wait()
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent replacements failed: %v, %v", first.err, second.err)
	}
	if first.facts.Version == second.facts.Version {
		t.Fatal("concurrent replacements reused an ACL version")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := aclstore.LoadCurrent(ctx, tx, resource)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, first.facts) && !reflect.DeepEqual(current, second.facts) {
		t.Fatalf("current ACL mixed concurrent snapshots: %#v; candidates %#v / %#v", current, first.facts, second.facts)
	}
}

func TestStoredACLFeedsEvaluatorWithoutCallerReshaping(t *testing.T) {
	ctx, database := openACLStoreDatabase(t)
	createACLStoreFixtures(t, ctx, database)
	resource := authorization.ResourceRef{TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindChannel, ID: "channel-acl-store-synthetic"}
	stored := replaceAndCommit(t, ctx, database, aclstore.Replacement{
		Resource: resource, DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow}},
	})

	interceptor := rpcmiddleware.NewAuthenticationInterceptor(authorizationVerifier{})
	request := connect.NewRequest(&authorizationRequest{})
	request.Header().Set("Authorization", "Bearer acl-store-fixture-credential")
	var decision authorization.Decision
	handler := interceptor.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		var err error
		decision, err = authorization.Evaluate(ctx, authorization.ActionChannelRead, authorization.CurrentFacts{
			Available:    true,
			Resource:     authorization.ResourceFacts{Ref: resource, Exists: true, Visibility: authorization.VisibilityPublic, State: authorization.ResourceStateActive},
			Organization: authorization.OrganizationFacts{TenantID: resource.TenantID, State: authorization.OrganizationStateActive, PolicyVersion: "policy-acl-store-v1"},
			Member:       authorization.MemberFacts{TenantID: resource.TenantID, Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}, State: authorization.MemberStateActive, Role: authorization.OrganizationRoleMember},
			Membership:   authorization.ChannelMembershipFacts{TenantID: resource.TenantID, ChannelID: resource.ID, Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}, Current: true, IntervalID: "membership-acl-store-synthetic", Role: authorization.ChannelRoleMember},
			ACL:          stored,
		})
		return connect.NewResponse(&authorizationRequest{}), err
	})
	if _, err := handler(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if decision.Effect != authorization.EffectAllow || decision.ACLVersion != stored.Version {
		t.Fatalf("Evaluate() = %#v, want stored ACL allow/version", decision)
	}
}

func TestInvalidReplacementFactsFailBeforePersistence(t *testing.T) {
	ctx, database := openACLStoreDatabase(t)
	createACLStoreFixtures(t, ctx, database)
	validResource := authorization.ResourceRef{TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindSpace, ID: "space-acl-store-synthetic"}
	validEntry := authorization.ACLEntry{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}, Action: authorization.ActionSpaceDiscover, Effect: authorization.ACLEffectAllow}
	tests := []aclstore.Replacement{
		{Resource: authorization.ResourceRef{TenantID: validResource.TenantID, Kind: "future-resource", ID: validResource.ID}, DefaultEffect: authorization.ACLEffectAllow},
		{Resource: authorization.ResourceRef{TenantID: validResource.TenantID, Kind: validResource.Kind, ID: " space-acl-store-synthetic"}, DefaultEffect: authorization.ACLEffectAllow},
		{Resource: validResource, DefaultEffect: "future-effect"},
		{Resource: validResource, DefaultEffect: authorization.ACLEffectAllow, Entries: []authorization.ACLEntry{{Actor: authorization.ActorRef{Type: 0, ID: validEntry.Actor.ID}, Action: validEntry.Action, Effect: validEntry.Effect}}},
		{Resource: validResource, DefaultEffect: authorization.ACLEffectAllow, Entries: []authorization.ACLEntry{{Actor: authorization.ActorRef{Type: validEntry.Actor.Type, ID: ""}, Action: validEntry.Action, Effect: validEntry.Effect}}},
		{Resource: validResource, DefaultEffect: authorization.ACLEffectAllow, Entries: []authorization.ACLEntry{{Actor: validEntry.Actor, Action: "future.action", Effect: validEntry.Effect}}},
		{Resource: validResource, DefaultEffect: authorization.ACLEffectAllow, Entries: []authorization.ACLEntry{{Actor: validEntry.Actor, Action: validEntry.Action, Effect: "future-effect"}}},
		{Resource: validResource, DefaultEffect: authorization.ACLEffectAllow, Entries: []authorization.ACLEntry{validEntry, validEntry}},
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for index, replacement := range tests {
		if _, err := aclstore.ReplaceCurrent(ctx, tx, replacement); !hasStoreErrorCode(err, aclstore.ErrorInvalidInput) {
			t.Fatalf("invalid replacement %d error = %v, want invalid-input", index, err)
		}
	}
	if _, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource:      authorization.ResourceRef{TenantID: validResource.TenantID, Kind: authorization.ResourceKindSpace, ID: "missing-space-acl-store-synthetic"},
		DefaultEffect: authorization.ACLEffectAllow,
	}); !hasStoreErrorCode(err, aclstore.ErrorResourceNotFound) {
		t.Fatalf("missing resource error = %v, want resource-not-found", err)
	}
	if _, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource: validResource, DefaultEffect: authorization.ACLEffectAllow, Entries: []authorization.ACLEntry{validEntry},
	}); err != nil {
		t.Fatalf("validation failures changed transaction usability: %v", err)
	}
}

func TestLoadCurrentReturnsSnapshotIdentityWithInvalidStoredFacts(t *testing.T) {
	tests := []struct {
		name        string
		prepareDDL  string
		corruptSQL  string
		wantActor   authorization.ActorRef
		wantDefault authorization.ACLEffect
		wantAction  authorization.Action
		wantEffect  authorization.ACLEffect
	}{
		{
			name: "default effect",
			prepareDDL: `
				ALTER TABLE domain.resource_acl_snapshots
				  DROP CONSTRAINT resource_acl_snapshots_default_effect_known;
				ALTER TABLE domain.resource_acl_snapshots
				  DISABLE TRIGGER resource_acl_snapshots_lifecycle_guard;
			`,
			corruptSQL: `
				UPDATE domain.resource_acl_snapshots SET default_effect = 99
				WHERE tenant_id = $1 AND acl_version = $2
			`,
			wantActor:  authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"},
			wantAction: authorization.ActionSpaceDiscover,
			wantEffect: authorization.ACLEffectAllow,
		},
		{
			name: "entry action",
			prepareDDL: `
				ALTER TABLE domain.resource_acl_entries
				  DROP CONSTRAINT resource_acl_entries_action_known;
				ALTER TABLE domain.resource_acl_entries
				  DISABLE TRIGGER resource_acl_entries_lifecycle_guard;
			`,
			corruptSQL: `
				UPDATE domain.resource_acl_entries SET action = 99
				WHERE tenant_id = $1 AND acl_version = $2
			`,
			wantActor:   authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"},
			wantDefault: authorization.ACLEffectAllow,
			wantEffect:  authorization.ACLEffectAllow,
		},
		{
			name: "entry effect",
			prepareDDL: `
				ALTER TABLE domain.resource_acl_entries
				  DROP CONSTRAINT resource_acl_entries_effect_known;
				ALTER TABLE domain.resource_acl_entries
				  DISABLE TRIGGER resource_acl_entries_lifecycle_guard;
			`,
			corruptSQL: `
				UPDATE domain.resource_acl_entries SET effect = 99
				WHERE tenant_id = $1 AND acl_version = $2
			`,
			wantActor:   authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"},
			wantDefault: authorization.ACLEffectAllow,
			wantAction:  authorization.ActionSpaceDiscover,
		},
		{
			name: "entry actor ID",
			prepareDDL: `
				ALTER TABLE domain.resource_acl_entries
				  DROP CONSTRAINT resource_acl_entries_actor_id_not_blank;
				ALTER TABLE domain.resource_acl_entries
				  DROP CONSTRAINT resource_acl_entries_member_fk;
				ALTER TABLE domain.resource_acl_entries
				  DISABLE TRIGGER resource_acl_entries_lifecycle_guard;
			`,
			corruptSQL: `
				UPDATE domain.resource_acl_entries SET actor_id = ' actor-acl-store-synthetic '
				WHERE tenant_id = $1 AND acl_version = $2
			`,
			wantActor:   authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman},
			wantDefault: authorization.ACLEffectAllow,
			wantAction:  authorization.ActionSpaceDiscover,
			wantEffect:  authorization.ACLEffectAllow,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, database := openACLStoreDatabase(t)
			createACLStoreFixtures(t, ctx, database)
			resource := authorization.ResourceRef{
				TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindSpace, ID: "space-acl-store-synthetic",
			}
			actor := authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: "actor-acl-store-synthetic"}
			created := replaceAndCommit(t, ctx, database, aclstore.Replacement{
				Resource: resource, DefaultEffect: authorization.ACLEffectAllow,
				Entries: []authorization.ACLEntry{{
					Actor: actor, Action: authorization.ActionSpaceDiscover, Effect: authorization.ACLEffectAllow,
				}},
			})
			version, err := strconv.ParseInt(created.Version, 10, 64)
			if err != nil {
				t.Fatal("parse generated ACL version fixture failed")
			}
			if _, err := database.Exec(ctx, test.prepareDDL); err != nil {
				t.Fatal("prepare invalid stored ACL fixture failed")
			}
			if _, err := database.Exec(ctx, test.corruptSQL, resource.TenantID, version); err != nil {
				t.Fatal("corrupt stored ACL fixture failed")
			}

			tx, err := database.Begin(ctx)
			if err != nil {
				t.Fatal("begin invalid stored ACL load transaction failed")
			}
			defer func() { _ = tx.Rollback(ctx) }()
			facts, err := aclstore.LoadCurrent(ctx, tx, resource)
			if !hasStoreErrorCode(err, aclstore.ErrorInvalidStoredFacts) {
				t.Fatalf("LoadCurrent() error = %v, want invalid-stored-facts", err)
			}
			if facts.Resource != resource || facts.Version != created.Version || len(facts.Entries) != 1 {
				t.Fatalf("LoadCurrent() facts = %#v, want actual resource/version and one entry", facts)
			}
			if facts.DefaultEffect != test.wantDefault ||
				facts.Entries[0].Actor != test.wantActor ||
				facts.Entries[0].Action != test.wantAction ||
				facts.Entries[0].Effect != test.wantEffect {
				t.Fatalf("LoadCurrent() invalid facts = %#v, want partial typed facts", facts)
			}
		})
	}
}

func TestLoadCurrentKeepsUnknownSQLFailureClassifiedAsPersistence(t *testing.T) {
	ctx, database := openACLStoreDatabase(t)
	createACLStoreFixtures(t, ctx, database)
	resource := authorization.ResourceRef{
		TenantID: "tenant-acl-store-synthetic", Kind: authorization.ResourceKindSpace, ID: "space-acl-store-synthetic",
	}
	replaceAndCommit(t, ctx, database, aclstore.Replacement{
		Resource: resource, DefaultEffect: authorization.ACLEffectAllow,
	})
	if _, err := database.Exec(ctx, "DROP TABLE domain.resource_acl_entries CASCADE"); err != nil {
		t.Fatal("remove disposable ACL entry table fixture failed")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin ACL persistence failure transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	facts, err := aclstore.LoadCurrent(ctx, tx, resource)
	if !hasStoreErrorCode(err, aclstore.ErrorPersistence) {
		t.Fatalf("LoadCurrent() error = %v, want persistence-failure", err)
	}
	if !reflect.DeepEqual(facts, authorization.ResourceACLFacts{}) {
		t.Fatalf("LoadCurrent() facts = %#v, want zero facts for unknown SQL failure", facts)
	}
}

func replaceAndCommit(
	t *testing.T,
	ctx context.Context,
	database *pgx.Conn,
	replacement aclstore.Replacement,
) authorization.ResourceACLFacts {
	t.Helper()
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	created, err := aclstore.ReplaceCurrent(ctx, tx, replacement)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return created
}

func assertCurrentACL(
	t *testing.T,
	ctx context.Context,
	database *pgx.Conn,
	resource authorization.ResourceRef,
	want authorization.ResourceACLFacts,
) {
	t.Helper()
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	got, err := aclstore.LoadCurrent(ctx, tx, resource)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("current ACL = %#v, want preserved %#v", got, want)
	}
}

func TestACLStoreDatabaseNameGuard(t *testing.T) {
	t.Parallel()

	if !safeACLStoreTestDatabaseName("threadline_resource_acl_store_go_test_123_456") {
		t.Fatal("expected generated ACL store database name to pass the deletion guard")
	}
	for _, name := range []string{
		"",
		"postgres",
		"threadline_resource_acl_store_go_test_",
		"threadline_resource_acl_store_go_test_123",
		"threadline_resource_acl_store_go_test_123_456_extra",
		"threadline_resource_acl_store_go_test_123_secret",
	} {
		if safeACLStoreTestDatabaseName(name) {
			t.Fatalf("unsafe database name %q passed the deletion guard", name)
		}
	}
}

func openACLStoreDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run ACL store integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := admin.Close(cleanupCtx); err != nil {
			t.Error("close PostgreSQL maintenance connection failed")
		}
	})
	var version string
	if err := admin.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "16.4" && !strings.HasPrefix(version, "16.4.") {
		t.Fatalf("PostgreSQL 16.4 required, found %s", version)
	}
	databaseName := "threadline_resource_acl_store_go_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if !safeACLStoreTestDatabaseName(databaseName) {
		t.Fatal("refusing unsafe disposable PostgreSQL database name")
	}
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		t.Fatal(err)
	}
	config := adminConfig.Copy()
	config.Database = databaseName
	var database *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if database != nil {
			if err := database.Close(cleanupCtx); err != nil {
				t.Error("close disposable PostgreSQL test database failed")
			}
		}
		if !safeACLStoreTestDatabaseName(databaseName) {
			t.Error("refusing to drop unexpected PostgreSQL test database")
			return
		}
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable PostgreSQL test database failed")
		}
	})
	database, err = pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 7; index++ {
		name := fmt.Sprintf("%06d_", index)
		matches, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "..", "db", "migrations", name+"*.up.sql"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("resolve migration %s failed: %v (%v)", name, err, matches)
		}
		migration, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.PgConn().Exec(ctx, string(migration)).ReadAll(); err != nil {
			t.Fatalf("apply migration %s failed: %v", matches[0], err)
		}
	}
	return ctx, database
}

func safeACLStoreTestDatabaseName(databaseName string) bool {
	const prefix = "threadline_resource_acl_store_go_test_"
	if !strings.HasPrefix(databaseName, prefix) {
		return false
	}
	pid, timestamp, found := strings.Cut(strings.TrimPrefix(databaseName, prefix), "_")
	return found && decimalDigitsOnly(pid) && decimalDigitsOnly(timestamp)
}

func decimalDigitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func createACLStoreFixtures(t *testing.T, ctx context.Context, database *pgx.Conn) {
	t.Helper()
	_, err := database.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ('tenant-acl-store-synthetic', 'ACL Store Synthetic', 1, 'policy-acl-store-v1');
		INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
		VALUES
		  ('tenant-acl-store-synthetic', 1, 'actor-acl-store-synthetic', 'ACL Human Synthetic', 4, 2),
		  ('tenant-acl-store-synthetic', 2, 'agent-acl-store-synthetic', 'ACL Agent Synthetic', 4, 2),
		  ('tenant-acl-store-synthetic', 3, 'service-acl-store-synthetic', 'ACL Service Synthetic', 4, 2);
		INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
		VALUES ('tenant-acl-store-synthetic', 'space-acl-store-synthetic', 'ACL Space Synthetic', TRUE);
		INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
		VALUES ('tenant-acl-store-synthetic', 'channel-acl-store-synthetic', 'space-acl-store-synthetic', 'ACL Channel Synthetic', 1, 1, 'group-acl-store-synthetic');
	`)
	if err != nil {
		t.Fatal(err)
	}
}
