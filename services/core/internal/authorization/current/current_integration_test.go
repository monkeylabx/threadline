package current_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/aclstore"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/current"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

type principalVerifier struct {
	tenantID string
	actorID  string
}

func (v principalVerifier) VerifySession(context.Context, string) (rpcmiddleware.VerifiedSession, error) {
	return rpcmiddleware.VerifiedSession{
		TenantID:  v.tenantID,
		ActorType: rpcmiddleware.ActorTypeHuman,
		ActorID:   v.actorID,
		DeviceID:  "device-current-synthetic",
		SessionID: "session-current-synthetic",
	}, nil
}

type principalRequest struct{}

func TestEvaluateCurrentRequiresAuthenticatedPrincipalBeforeSQL(t *testing.T) {
	t.Parallel()

	decision, err := current.EvaluateCurrent(
		context.Background(),
		nil,
		authorization.ActionChannelRead,
		authorization.ResourceRef{
			TenantID: "tenant-current-synthetic",
			Kind:     authorization.ResourceKindChannel,
			ID:       "channel-current-synthetic",
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCurrent() error = %v, want nil policy error", err)
	}
	if decision.Effect != authorization.EffectDeny ||
		decision.Reason != authorization.ReasonAuthenticationRequired {
		t.Fatalf("EvaluateCurrent() = %#v, want authentication-required deny", decision)
	}
}

func TestEvaluateCurrentRejectsTenantMismatchBeforeSQL(t *testing.T) {
	t.Parallel()

	decision, err := evaluateWithPrincipal(
		t,
		"tenant-current-synthetic",
		"actor-current-synthetic",
		nil,
		authorization.ActionChannelRead,
		authorization.ResourceRef{
			TenantID: "tenant-other-synthetic",
			Kind:     authorization.ResourceKindChannel,
			ID:       "channel-current-synthetic",
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCurrent() error = %v, want nil policy error", err)
	}
	if decision.Effect != authorization.EffectDeny || decision.Reason != authorization.ReasonTenantMismatch {
		t.Fatalf("EvaluateCurrent() = %#v, want tenant-mismatch deny", decision)
	}
}

func TestEvaluateCurrentPreservesContextCancellationBeforeSQL(t *testing.T) {
	t.Parallel()

	interceptor := rpcmiddleware.NewAuthenticationInterceptor(principalVerifier{
		tenantID: "tenant-current-synthetic",
		actorID:  "actor-current-synthetic",
	})
	request := connect.NewRequest(&principalRequest{})
	request.Header().Set("Authorization", "Bearer current-fixture-credential")
	var evaluationErr error
	handler := interceptor.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()
		_, evaluationErr = current.EvaluateCurrent(
			canceledCtx,
			nil,
			authorization.ActionChannelRead,
			authorization.ResourceRef{
				TenantID: "tenant-current-synthetic", Kind: authorization.ResourceKindChannel, ID: "channel-current-synthetic",
			},
		)
		return connect.NewResponse(&principalRequest{}), nil
	})
	if _, err := handler(context.Background(), request); err != nil {
		t.Fatalf("authenticate test Principal: %v", err)
	}
	if !errors.Is(evaluationErr, context.Canceled) {
		t.Fatalf("EvaluateCurrent() error = %v, want context.Canceled", evaluationErr)
	}
}

func TestEvaluateCurrentRejectsInvalidTransactionWithoutHidingAuthenticationDecisions(t *testing.T) {
	t.Parallel()

	_, err := evaluateWithPrincipal(
		t,
		"tenant-current-synthetic",
		"actor-current-synthetic",
		nil,
		authorization.ActionChannelRead,
		authorization.ResourceRef{
			TenantID: "tenant-current-synthetic",
			Kind:     authorization.ResourceKindChannel,
			ID:       "channel-current-synthetic",
		},
	)
	var currentError *current.Error
	if !errors.As(err, &currentError) || currentError.Code() != current.ErrorInvalidInput {
		t.Fatalf("EvaluateCurrent() error = %v, want typed invalid-input", err)
	}
	if err.Error() != "current authorization: invalid-input" {
		t.Fatalf("EvaluateCurrent() error text = %q, want stable secret-safe text", err.Error())
	}
}

func TestEvaluateCurrentAllowsSpaceFromResolvedFactsWithoutOwningTransaction(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	fixtures := createCurrentAuthorizationFixtures(t, ctx, database)
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin caller-owned transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref := authorization.ResourceRef{
		TenantID: fixtures.tenantID,
		Kind:     authorization.ResourceKindSpace,
		ID:       fixtures.spaceID,
	}
	decision, err := evaluateWithPrincipal(
		t,
		fixtures.tenantID,
		fixtures.actorID,
		tx,
		authorization.ActionSpaceDiscover,
		ref,
	)
	if err != nil {
		t.Fatalf("EvaluateCurrent() error = %v", err)
	}
	if decision.Effect != authorization.EffectAllow ||
		decision.Reason != authorization.ReasonAllowed ||
		decision.Resource != ref ||
		decision.Actor != (authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: fixtures.actorID}) ||
		decision.PolicyVersion != "policy-current-v1" ||
		decision.ACLVersion != fixtures.spaceACLVersion {
		t.Fatalf("EvaluateCurrent() = %#v, want exact resolved Space allow", decision)
	}
	var one int
	if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("EvaluateCurrent() ended caller transaction: value=%d error=%v", one, err)
	}
}

func TestEvaluateCurrentRejectsSnapshotIsolationThatCouldReadStaleFacts(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	tx, err := database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal("begin repeatable-read transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = evaluateWithPrincipal(
		t,
		"tenant-current-synthetic",
		"actor-current-synthetic",
		tx,
		authorization.ActionChannelRead,
		authorization.ResourceRef{
			TenantID: "tenant-current-synthetic",
			Kind:     authorization.ResourceKindChannel,
			ID:       "channel-current-synthetic",
		},
	)
	var currentError *current.Error
	if !errors.As(err, &currentError) || currentError.Code() != current.ErrorInvalidInput {
		t.Fatalf("EvaluateCurrent() error = %v, want typed invalid-input for repeatable-read transaction", err)
	}
	if err := tx.QueryRow(ctx, "SELECT 1").Scan(new(int)); err != nil {
		t.Fatalf("EvaluateCurrent() ended caller transaction: %v", err)
	}
}

func TestEvaluateCurrentAllowsChannelFromCurrentMembershipAndACL(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	fixtures := createCurrentAuthorizationFixtures(t, ctx, database)
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin caller-owned transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref := authorization.ResourceRef{
		TenantID: fixtures.tenantID,
		Kind:     authorization.ResourceKindChannel,
		ID:       fixtures.channelID,
	}
	decision, err := evaluateWithPrincipal(
		t,
		fixtures.tenantID,
		fixtures.actorID,
		tx,
		authorization.ActionChannelRead,
		ref,
	)
	if err != nil {
		t.Fatalf("EvaluateCurrent() error = %v", err)
	}
	if decision.Effect != authorization.EffectAllow ||
		decision.Reason != authorization.ReasonAllowed ||
		decision.Resource != ref ||
		decision.PolicyVersion != "policy-current-v1" ||
		decision.ACLVersion != fixtures.channelACLVersion {
		t.Fatalf("EvaluateCurrent() = %#v, want exact resolved Channel allow", decision)
	}
}

func TestEvaluateCurrentNeverReadsDepartedMembershipHistory(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	fixtures := createCurrentAuthorizationFixtures(t, ctx, database)
	if _, err := database.Exec(ctx, `
		UPDATE domain.channel_memberships
		SET left_at = CURRENT_TIMESTAMP
		WHERE tenant_id = $1 AND channel_id = $2 AND actor_type = 1 AND actor_id = $3 AND left_at IS NULL
	`, fixtures.tenantID, fixtures.channelID, fixtures.actorID); err != nil {
		t.Fatal("depart current Channel Membership fixture failed")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin caller-owned transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ref := authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindChannel, ID: fixtures.channelID}

	read, err := evaluateWithPrincipal(t, fixtures.tenantID, fixtures.actorID, tx, authorization.ActionChannelRead, ref)
	if err != nil {
		t.Fatalf("EvaluateCurrent(ChannelRead) error = %v", err)
	}
	if read.Effect != authorization.EffectDeny || read.Reason != authorization.ReasonNotAMember {
		t.Fatalf("EvaluateCurrent(ChannelRead) = %#v, want exact current non-membership deny", read)
	}
	discover, err := evaluateWithPrincipal(t, fixtures.tenantID, fixtures.actorID, tx, authorization.ActionChannelDiscover, ref)
	if err != nil {
		t.Fatalf("EvaluateCurrent(ChannelDiscover) error = %v", err)
	}
	if discover.Effect != authorization.EffectDeny || discover.Reason != authorization.ReasonACLDefaultDeny {
		t.Fatalf("EvaluateCurrent(ChannelDiscover) = %#v, want non-member facts evaluated through current ACL", discover)
	}
}

func TestEvaluateCurrentFailsClosedForAbsentAndInactiveDomainFacts(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	fixtures := createCurrentAuthorizationFixtures(t, ctx, database)

	assertDecision := func(action authorization.Action, ref authorization.ResourceRef, reason authorization.Reason) {
		t.Helper()
		tx, err := database.Begin(ctx)
		if err != nil {
			t.Fatal("begin caller-owned transaction failed")
		}
		decision, evaluationErr := evaluateWithPrincipal(t, fixtures.tenantID, fixtures.actorID, tx, action, ref)
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			t.Fatal("rollback caller-owned transaction failed")
		}
		if evaluationErr != nil {
			t.Fatalf("EvaluateCurrent() error = %v", evaluationErr)
		}
		if decision.Effect != authorization.EffectDeny || decision.Reason != reason {
			t.Fatalf("EvaluateCurrent() = %#v, want %s deny", decision, reason)
		}
	}

	assertDecision(
		authorization.ActionChannelRead,
		authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindChannel, ID: "missing-channel-current-synthetic"},
		authorization.ReasonFactsUnavailable,
	)
	if _, err := database.Exec(ctx, `
		INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
		VALUES ($1, 'space-without-acl-current-synthetic', 'Space Without ACL Synthetic', TRUE)
	`, fixtures.tenantID); err != nil {
		t.Fatal("create Space without ACL failed")
	}
	assertDecision(
		authorization.ActionSpaceDiscover,
		authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindSpace, ID: "space-without-acl-current-synthetic"},
		authorization.ReasonACLVersionInvalid,
	)
	if _, err := database.Exec(ctx, `
		UPDATE domain.organizations SET policy_version = '' WHERE tenant_id = $1
	`, fixtures.tenantID); err != nil {
		t.Fatal("invalidate Organization policy version fixture failed")
	}
	assertDecision(
		authorization.ActionSpaceDiscover,
		authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindSpace, ID: fixtures.spaceID},
		authorization.ReasonPolicyVersionInvalid,
	)
	if _, err := database.Exec(ctx, `
		UPDATE domain.organizations SET policy_version = 'policy-current-v1' WHERE tenant_id = $1
	`, fixtures.tenantID); err != nil {
		t.Fatal("restore Organization policy version fixture failed")
	}
	if _, err := database.Exec(ctx, `
		UPDATE domain.members SET state = 3
		WHERE tenant_id = $1 AND actor_type = 1 AND actor_id = $2
	`, fixtures.tenantID, fixtures.actorID); err != nil {
		t.Fatal("deactivate Member fixture failed")
	}
	assertDecision(
		authorization.ActionSpaceDiscover,
		authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindSpace, ID: fixtures.spaceID},
		authorization.ReasonMemberInactive,
	)
	if _, err := database.Exec(ctx, `
		UPDATE domain.members SET state = 2
		WHERE tenant_id = $1 AND actor_type = 1 AND actor_id = $2
	`, fixtures.tenantID, fixtures.actorID); err != nil {
		t.Fatal("reactivate Member fixture failed")
	}
	if _, err := database.Exec(ctx, `
		UPDATE domain.organizations SET state = 2 WHERE tenant_id = $1
	`, fixtures.tenantID); err != nil {
		t.Fatal("suspend Organization fixture failed")
	}
	assertDecision(
		authorization.ActionSpaceDiscover,
		authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindSpace, ID: fixtures.spaceID},
		authorization.ReasonOrganizationUnavailable,
	)
}

func TestEvaluateCurrentFailsClosedBeforeResourceLookupWhenMemberIsAbsent(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	if _, err := database.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ('tenant-current-synthetic', 'Current Authorization Synthetic', 1, 'policy-current-v1')
	`); err != nil {
		t.Fatal("create Organization-only fixture failed")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin caller-owned transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	decision, err := evaluateWithPrincipal(
		t,
		"tenant-current-synthetic",
		"actor-current-synthetic",
		tx,
		authorization.ActionChannelRead,
		authorization.ResourceRef{
			TenantID: "tenant-current-synthetic", Kind: authorization.ResourceKindChannel, ID: "same-id-in-another-tenant",
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCurrent() error = %v", err)
	}
	if decision.Effect != authorization.EffectDeny || decision.Reason != authorization.ReasonFactsUnavailable {
		t.Fatalf("EvaluateCurrent() = %#v, want absent-Member fail-closed decision", decision)
	}
}

func TestEvaluateCurrentFailsClosedForInvalidPersistedEnums(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	fixtures := createCurrentAuthorizationFixtures(t, ctx, database)
	assertDecision := func(action authorization.Action, ref authorization.ResourceRef, reason authorization.Reason) {
		t.Helper()
		tx, err := database.Begin(ctx)
		if err != nil {
			t.Fatal("begin caller-owned transaction failed")
		}
		decision, evaluationErr := evaluateWithPrincipal(t, fixtures.tenantID, fixtures.actorID, tx, action, ref)
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			t.Fatal("rollback caller-owned transaction failed")
		}
		if evaluationErr != nil {
			t.Fatalf("EvaluateCurrent() error = %v", evaluationErr)
		}
		if decision.Effect != authorization.EffectDeny || decision.Reason != reason {
			t.Fatalf("EvaluateCurrent() = %#v, want %s deny", decision, reason)
		}
	}

	if _, err := database.Exec(ctx, `
		ALTER TABLE domain.members DROP CONSTRAINT members_role_known;
		UPDATE domain.members SET role = 99
		WHERE tenant_id = 'tenant-current-synthetic' AND actor_type = 1 AND actor_id = 'actor-current-synthetic';
	`); err != nil {
		t.Fatal("install invalid persisted Organization Role fixture failed")
	}
	assertDecision(
		authorization.ActionSpaceDiscover,
		authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindSpace, ID: fixtures.spaceID},
		authorization.ReasonOrganizationRoleDenied,
	)
	if _, err := database.Exec(ctx, `
		UPDATE domain.members SET role = 4
		WHERE tenant_id = 'tenant-current-synthetic' AND actor_type = 1 AND actor_id = 'actor-current-synthetic';
		ALTER TABLE domain.channels DROP CONSTRAINT channels_visibility_known;
		UPDATE domain.channels SET visibility = 99
		WHERE tenant_id = 'tenant-current-synthetic' AND channel_id = 'channel-current-synthetic';
	`); err != nil {
		t.Fatal("install invalid persisted Channel Visibility fixture failed")
	}
	assertDecision(
		authorization.ActionChannelRead,
		authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindChannel, ID: fixtures.channelID},
		authorization.ReasonUnknownResource,
	)
}

func TestEvaluateCurrentMapsInvalidStoredACLToACLInvalidDecision(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	fixtures := createCurrentAuthorizationFixtures(t, ctx, database)
	version, err := strconv.ParseInt(fixtures.channelACLVersion, 10, 64)
	if err != nil {
		t.Fatal("parse Channel ACL version fixture failed")
	}
	if _, err := database.Exec(ctx, `
		ALTER TABLE domain.resource_acl_snapshots
		  DROP CONSTRAINT resource_acl_snapshots_default_effect_known;
		ALTER TABLE domain.resource_acl_snapshots
		  DISABLE TRIGGER resource_acl_snapshots_lifecycle_guard;
	`); err != nil {
		t.Fatal("prepare invalid stored ACL fixture failed")
	}
	if _, err := database.Exec(ctx, `
		UPDATE domain.resource_acl_snapshots SET default_effect = 99
		WHERE tenant_id = $1 AND acl_version = $2
	`, fixtures.tenantID, version); err != nil {
		t.Fatal("corrupt stored ACL default fixture failed")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin caller-owned transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	decision, err := evaluateWithPrincipal(
		t,
		fixtures.tenantID,
		fixtures.actorID,
		tx,
		authorization.ActionChannelRead,
		authorization.ResourceRef{
			TenantID: fixtures.tenantID, Kind: authorization.ResourceKindChannel, ID: fixtures.channelID,
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCurrent() error = %v, want fail-closed Decision", err)
	}
	if decision.Effect != authorization.EffectDeny ||
		decision.Reason != authorization.ReasonACLInvalid ||
		decision.ACLVersion != fixtures.channelACLVersion {
		t.Fatalf("EvaluateCurrent() = %#v, want ACL-invalid deny with actual version", decision)
	}
}

func TestEvaluateCurrentNeverAllowsInvalidStoredACLActorID(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	fixtures := createCurrentAuthorizationFixtures(t, ctx, database)
	resource := authorization.ResourceRef{
		TenantID: fixtures.tenantID, Kind: authorization.ResourceKindChannel, ID: fixtures.channelID,
	}
	replacementTx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin default-allow ACL fixture transaction failed")
	}
	stored, err := aclstore.ReplaceCurrent(ctx, replacementTx, aclstore.Replacement{
		Resource: resource, DefaultEffect: authorization.ACLEffectAllow,
		Entries: []authorization.ACLEntry{{
			Actor:  authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: fixtures.actorID},
			Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectDeny,
		}},
	})
	if err != nil {
		_ = replacementTx.Rollback(ctx)
		t.Fatal("create default-allow ACL fixture failed")
	}
	if err := replacementTx.Commit(ctx); err != nil {
		t.Fatal("commit default-allow ACL fixture failed")
	}
	version, err := strconv.ParseInt(stored.Version, 10, 64)
	if err != nil {
		t.Fatal("parse default-allow ACL version fixture failed")
	}
	if _, err := database.Exec(ctx, `
		ALTER TABLE domain.resource_acl_entries
		  DROP CONSTRAINT resource_acl_entries_actor_id_not_blank;
		ALTER TABLE domain.resource_acl_entries
		  DROP CONSTRAINT resource_acl_entries_member_fk;
		ALTER TABLE domain.resource_acl_entries
		  DISABLE TRIGGER resource_acl_entries_lifecycle_guard;
	`); err != nil {
		t.Fatal("prepare invalid stored ACL Actor ID fixture failed")
	}
	if _, err := database.Exec(ctx, `
		UPDATE domain.resource_acl_entries SET actor_id = ' actor-current-synthetic '
		WHERE tenant_id = $1 AND acl_version = $2
	`, fixtures.tenantID, version); err != nil {
		t.Fatal("corrupt stored ACL Actor ID fixture failed")
	}

	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin caller-owned transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	decision, err := evaluateWithPrincipal(
		t,
		fixtures.tenantID,
		fixtures.actorID,
		tx,
		authorization.ActionChannelRead,
		resource,
	)
	if err != nil {
		t.Fatalf("EvaluateCurrent() error = %v, want fail-closed Decision", err)
	}
	if decision.Effect != authorization.EffectDeny ||
		decision.Reason != authorization.ReasonACLInvalid ||
		decision.ACLVersion != stored.Version {
		t.Fatalf("EvaluateCurrent() = %#v, want ACL-invalid deny with actual version", decision)
	}
}

func TestEvaluateCurrentReadsWriterCommittedFactsAfterLockWait(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(context.Context, *pgx.Conn, currentAuthorizationFixtures) error
		mutate   func(context.Context, pgx.Tx, currentAuthorizationFixtures) error
		action   authorization.Action
		resource authorization.ResourceKind
		effect   authorization.Effect
		reason   authorization.Reason
	}{
		{
			name: "organization freeze",
			mutate: func(ctx context.Context, tx pgx.Tx, fixtures currentAuthorizationFixtures) error {
				_, err := tx.Exec(ctx, "UPDATE domain.organizations SET state = 2 WHERE tenant_id = $1", fixtures.tenantID)
				return err
			},
			action: authorization.ActionSpaceDiscover, resource: authorization.ResourceKindSpace,
			effect: authorization.EffectDeny, reason: authorization.ReasonOrganizationUnavailable,
		},
		{
			name: "member disable",
			mutate: func(ctx context.Context, tx pgx.Tx, fixtures currentAuthorizationFixtures) error {
				_, err := tx.Exec(ctx, "UPDATE domain.members SET state = 3 WHERE tenant_id = $1 AND actor_type = 1 AND actor_id = $2", fixtures.tenantID, fixtures.actorID)
				return err
			},
			action: authorization.ActionSpaceDiscover, resource: authorization.ResourceKindSpace,
			effect: authorization.EffectDeny, reason: authorization.ReasonMemberInactive,
		},
		{
			name: "channel archive",
			mutate: func(ctx context.Context, tx pgx.Tx, fixtures currentAuthorizationFixtures) error {
				_, err := tx.Exec(ctx, "UPDATE domain.channels SET state = 2 WHERE tenant_id = $1 AND channel_id = $2", fixtures.tenantID, fixtures.channelID)
				return err
			},
			action: authorization.ActionChannelPublish, resource: authorization.ResourceKindChannel,
			effect: authorization.EffectDeny, reason: authorization.ReasonResourceStateDenied,
		},
		{
			name: "membership departure",
			mutate: func(ctx context.Context, tx pgx.Tx, fixtures currentAuthorizationFixtures) error {
				_, err := tx.Exec(ctx, `
					UPDATE domain.channel_memberships SET left_at = CURRENT_TIMESTAMP
					WHERE tenant_id = $1 AND channel_id = $2 AND actor_type = 1 AND actor_id = $3 AND left_at IS NULL
				`, fixtures.tenantID, fixtures.channelID, fixtures.actorID)
				return err
			},
			action: authorization.ActionChannelRead, resource: authorization.ResourceKindChannel,
			effect: authorization.EffectDeny, reason: authorization.ReasonNotAMember,
		},
		{
			name: "new membership",
			prepare: func(ctx context.Context, database *pgx.Conn, fixtures currentAuthorizationFixtures) error {
				_, err := database.Exec(ctx, `
					UPDATE domain.channel_memberships SET left_at = CURRENT_TIMESTAMP
					WHERE tenant_id = $1 AND channel_id = $2 AND actor_type = 1 AND actor_id = $3 AND left_at IS NULL
				`, fixtures.tenantID, fixtures.channelID, fixtures.actorID)
				return err
			},
			mutate: func(ctx context.Context, tx pgx.Tx, fixtures currentAuthorizationFixtures) error {
				_, err := tx.Exec(ctx, `
					INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
					VALUES ($1, $2, 1, $3, 3)
				`, fixtures.tenantID, fixtures.channelID, fixtures.actorID)
				return err
			},
			action: authorization.ActionChannelRead, resource: authorization.ResourceKindChannel,
			effect: authorization.EffectAllow, reason: authorization.ReasonAllowed,
		},
		{
			name: "ACL replacement",
			mutate: func(ctx context.Context, tx pgx.Tx, fixtures currentAuthorizationFixtures) error {
				_, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
					Resource: authorization.ResourceRef{
						TenantID: fixtures.tenantID, Kind: authorization.ResourceKindChannel, ID: fixtures.channelID,
					},
					DefaultEffect: authorization.ACLEffectDeny,
				})
				return err
			},
			action: authorization.ActionChannelRead, resource: authorization.ResourceKindChannel,
			effect: authorization.EffectDeny, reason: authorization.ReasonACLDefaultDeny,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runWriterFirstCurrentAuthorization(t, test.prepare, test.mutate, test.action, test.resource, test.effect, test.reason)
		})
	}
}

func TestEvaluateCurrentKeepsDifferentTenantKeysIndependent(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	alpha := createCurrentAuthorizationFixtures(t, ctx, database)
	if _, err := database.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ('tenant-current-beta-synthetic', 'Current Beta Synthetic', 1, 'policy-current-beta-v1');
		INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
		VALUES ('tenant-current-beta-synthetic', 1, 'actor-current-synthetic', 'Current Beta Human Synthetic', 4, 2);
		INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
		VALUES ('tenant-current-beta-synthetic', 'space-current-synthetic', 'Current Beta Space Synthetic', TRUE);
	`); err != nil {
		t.Fatal("create same-ID second-Tenant fixtures failed")
	}
	betaRef := authorization.ResourceRef{
		TenantID: "tenant-current-beta-synthetic", Kind: authorization.ResourceKindSpace, ID: alpha.spaceID,
	}
	betaACLTx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin second-Tenant ACL fixture transaction failed")
	}
	betaACL, err := aclstore.ReplaceCurrent(ctx, betaACLTx, aclstore.Replacement{
		Resource: betaRef, DefaultEffect: authorization.ACLEffectDeny,
	})
	if err != nil {
		_ = betaACLTx.Rollback(ctx)
		t.Fatal("create second-Tenant ACL fixture failed")
	}
	if err := betaACLTx.Commit(ctx); err != nil {
		t.Fatal("commit second-Tenant ACL fixture failed")
	}

	alphaTx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin first-Tenant resolver transaction failed")
	}
	defer func() { _ = alphaTx.Rollback(context.Background()) }()
	alphaRef := authorization.ResourceRef{TenantID: alpha.tenantID, Kind: authorization.ResourceKindSpace, ID: alpha.spaceID}
	alphaDecision, err := evaluateWithPrincipal(
		t, alpha.tenantID, alpha.actorID, alphaTx, authorization.ActionSpaceDiscover, alphaRef,
	)
	if err != nil || alphaDecision.Effect != authorization.EffectAllow {
		t.Fatalf("first-Tenant EvaluateCurrent() = (%#v, %v), want allow", alphaDecision, err)
	}

	betaConnection, err := pgx.ConnectConfig(ctx, database.Config().Copy())
	if err != nil {
		t.Fatal("connect second-Tenant resolver failed")
	}
	defer func() { _ = betaConnection.Close(context.Background()) }()
	betaTx, err := betaConnection.Begin(ctx)
	if err != nil {
		t.Fatal("begin second-Tenant resolver transaction failed")
	}
	defer func() { _ = betaTx.Rollback(context.Background()) }()
	shortCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	betaDecision, evaluationErr, authenticationErr := evaluateAuthenticated(
		shortCtx,
		betaRef.TenantID,
		alpha.actorID,
		betaTx,
		authorization.ActionSpaceDiscover,
		betaRef,
	)
	if authenticationErr != nil || evaluationErr != nil {
		t.Fatalf("second-Tenant EvaluateCurrent() errors = auth:%v evaluation:%v", authenticationErr, evaluationErr)
	}
	if betaDecision.Effect != authorization.EffectDeny ||
		betaDecision.Reason != authorization.ReasonACLDefaultDeny ||
		betaDecision.PolicyVersion != "policy-current-beta-v1" ||
		betaDecision.ACLVersion != betaACL.Version {
		t.Fatalf("second-Tenant EvaluateCurrent() = %#v, want independent beta facts", betaDecision)
	}
}

func TestEvaluateCurrentReturnsSecretSafeTypedPersistenceFailure(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	if _, err := database.Exec(ctx, "DROP TABLE domain.organizations CASCADE"); err != nil {
		t.Fatal("remove disposable fact table fixture failed")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin caller-owned transaction failed")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = evaluateWithPrincipal(
		t,
		"tenant-current-synthetic",
		"actor-current-synthetic",
		tx,
		authorization.ActionSpaceDiscover,
		authorization.ResourceRef{
			TenantID: "tenant-current-synthetic", Kind: authorization.ResourceKindSpace, ID: "space-current-synthetic",
		},
	)
	var currentError *current.Error
	if !errors.As(err, &currentError) || currentError.Code() != current.ErrorPersistence {
		t.Fatalf("EvaluateCurrent() error = %v, want typed persistence-failure", err)
	}
	if err.Error() != "current authorization: persistence-failure" {
		t.Fatalf("EvaluateCurrent() error text = %q, want stable secret-safe text", err.Error())
	}
}

func runWriterFirstCurrentAuthorization(
	t *testing.T,
	prepare func(context.Context, *pgx.Conn, currentAuthorizationFixtures) error,
	mutate func(context.Context, pgx.Tx, currentAuthorizationFixtures) error,
	action authorization.Action,
	resourceKind authorization.ResourceKind,
	wantEffect authorization.Effect,
	wantReason authorization.Reason,
) {
	t.Helper()
	ctx, database := openCurrentAuthorizationDatabase(t)
	fixtures := createCurrentAuthorizationFixtures(t, ctx, database)
	if prepare != nil {
		if err := prepare(ctx, database, fixtures); err != nil {
			t.Fatal("prepare writer-first authorization case failed")
		}
	}

	writer, err := pgx.ConnectConfig(ctx, database.Config().Copy())
	if err != nil {
		t.Fatal("connect writer-first mutation connection failed")
	}
	defer func() { _ = writer.Close(context.Background()) }()
	writerTx, err := writer.Begin(ctx)
	if err != nil {
		t.Fatal("begin writer-first mutation transaction failed")
	}
	defer func() { _ = writerTx.Rollback(context.Background()) }()
	if err := mutate(ctx, writerTx, fixtures); err != nil {
		t.Fatal("stage writer-first mutation failed")
	}

	resolverConfig := database.Config().Copy()
	resolverConfig.RuntimeParams["application_name"] = "threadline-current-resolver"
	resolver, err := pgx.ConnectConfig(ctx, resolverConfig)
	if err != nil {
		t.Fatal("connect writer-first resolver connection failed")
	}
	defer func() { _ = resolver.Close(context.Background()) }()
	resolverTx, err := resolver.Begin(ctx)
	if err != nil {
		t.Fatal("begin writer-first resolver transaction failed")
	}
	defer func() { _ = resolverTx.Rollback(context.Background()) }()

	resourceID := fixtures.spaceID
	if resourceKind == authorization.ResourceKindChannel {
		resourceID = fixtures.channelID
	}
	ref := authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: resourceKind, ID: resourceID}
	type result struct {
		decision          authorization.Decision
		evaluationErr     error
		authenticationErr error
	}
	resultChannel := make(chan result, 1)
	go func() {
		decision, evaluationErr, authenticationErr := evaluateAuthenticated(
			ctx, fixtures.tenantID, fixtures.actorID, resolverTx, action, ref,
		)
		resultChannel <- result{decision: decision, evaluationErr: evaluationErr, authenticationErr: authenticationErr}
	}()
	waitForResolverLock(t, ctx, database, "threadline-current-resolver")
	if err := writerTx.Commit(ctx); err != nil {
		t.Fatal("commit writer-first mutation failed")
	}

	select {
	case got := <-resultChannel:
		if got.authenticationErr != nil || got.evaluationErr != nil {
			t.Fatalf("EvaluateCurrent() errors after writer commit = auth:%v evaluation:%v", got.authenticationErr, got.evaluationErr)
		}
		if got.decision.Effect != wantEffect || got.decision.Reason != wantReason {
			t.Fatalf("EvaluateCurrent() after writer commit = %#v, want %s/%s", got.decision, wantEffect, wantReason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("EvaluateCurrent() did not resume after writer commit")
	}
}

func waitForResolverLock(t *testing.T, ctx context.Context, observer *pgx.Conn, applicationName string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		err := observer.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM pg_stat_activity
			  WHERE datname = current_database()
			    AND application_name = $1
			    AND wait_event_type = 'Lock'
			)
		`, applicationName).Scan(&waiting)
		if err != nil {
			t.Fatal("observe writer-first resolver lock wait failed")
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("resolver did not block behind writer-held authorization fact lock")
}

func TestEvaluateCurrentRejectsInvalidActionAndResourceBeforeFactQueries(t *testing.T) {
	ctx, database := openCurrentAuthorizationDatabase(t)
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin caller-owned transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tests := []struct {
		action authorization.Action
		ref    authorization.ResourceRef
	}{
		{
			action: "future.action",
			ref: authorization.ResourceRef{
				TenantID: "tenant-current-synthetic", Kind: authorization.ResourceKindChannel, ID: "channel-current-synthetic",
			},
		},
		{
			action: authorization.ActionChannelRead,
			ref: authorization.ResourceRef{
				TenantID: "tenant-current-synthetic", Kind: "future-resource", ID: "channel-current-synthetic",
			},
		},
		{
			action: authorization.ActionChannelRead,
			ref: authorization.ResourceRef{
				TenantID: "tenant-current-synthetic", Kind: authorization.ResourceKindChannel, ID: " channel-current-synthetic",
			},
		},
	}
	for index, test := range tests {
		_, err := evaluateWithPrincipal(
			t,
			"tenant-current-synthetic",
			"actor-current-synthetic",
			tx,
			test.action,
			test.ref,
		)
		var currentError *current.Error
		if !errors.As(err, &currentError) || currentError.Code() != current.ErrorInvalidInput {
			t.Fatalf("invalid case %d error = %v, want typed invalid-input", index, err)
		}
	}
	if err := tx.QueryRow(ctx, "SELECT 1").Scan(new(int)); err != nil {
		t.Fatalf("validation failures changed caller transaction usability: %v", err)
	}
}

func evaluateWithPrincipal(
	t *testing.T,
	tenantID string,
	actorID string,
	tx pgx.Tx,
	action authorization.Action,
	ref authorization.ResourceRef,
) (authorization.Decision, error) {
	t.Helper()
	decision, evaluationErr, authenticationErr := evaluateAuthenticated(
		context.Background(), tenantID, actorID, tx, action, ref,
	)
	if authenticationErr != nil {
		t.Fatalf("authenticate test Principal: %v", authenticationErr)
	}
	return decision, evaluationErr
}

func evaluateAuthenticated(
	ctx context.Context,
	tenantID string,
	actorID string,
	tx pgx.Tx,
	action authorization.Action,
	ref authorization.ResourceRef,
) (authorization.Decision, error, error) {
	interceptor := rpcmiddleware.NewAuthenticationInterceptor(principalVerifier{
		tenantID: tenantID,
		actorID:  actorID,
	})
	request := connect.NewRequest(&principalRequest{})
	request.Header().Set("Authorization", "Bearer current-fixture-credential")
	var decision authorization.Decision
	var evaluationErr error
	handler := interceptor.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		decision, evaluationErr = current.EvaluateCurrent(ctx, tx, action, ref)
		return connect.NewResponse(&principalRequest{}), nil
	})
	if _, err := handler(ctx, request); err != nil {
		return authorization.Decision{}, nil, err
	}
	return decision, evaluationErr, nil
}

type currentAuthorizationFixtures struct {
	tenantID          string
	actorID           string
	spaceID           string
	channelID         string
	spaceACLVersion   string
	channelACLVersion string
}

func createCurrentAuthorizationFixtures(
	t *testing.T,
	ctx context.Context,
	database *pgx.Conn,
) currentAuthorizationFixtures {
	t.Helper()
	fixtures := currentAuthorizationFixtures{
		tenantID:  "tenant-current-synthetic",
		actorID:   "actor-current-synthetic",
		spaceID:   "space-current-synthetic",
		channelID: "channel-current-synthetic",
	}
	_, err := database.Exec(ctx, `
		INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
		VALUES ('tenant-current-synthetic', 'Current Authorization Synthetic', 1, 'policy-current-v1');
		INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
		VALUES ('tenant-current-synthetic', 1, 'actor-current-synthetic', 'Current Human Synthetic', 4, 2);
		INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
		VALUES ('tenant-current-synthetic', 'space-current-synthetic', 'Current Space Synthetic', TRUE);
		INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id)
		VALUES ('tenant-current-synthetic', 'channel-current-synthetic', 'space-current-synthetic', 'Current Channel Synthetic', 1, 1, 'group-current-synthetic');
		INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
		VALUES ('tenant-current-synthetic', 'channel-current-synthetic', 1, 'actor-current-synthetic', 3);
	`)
	if err != nil {
		t.Fatal("create current-authorization fixtures failed")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin ACL fixture transaction failed")
	}
	actor := authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: fixtures.actorID}
	spaceACL, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource:      authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindSpace, ID: fixtures.spaceID},
		DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{{
			Actor: actor, Action: authorization.ActionSpaceDiscover, Effect: authorization.ACLEffectAllow,
		}},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal("create Space ACL fixture failed")
	}
	channelACL, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource:      authorization.ResourceRef{TenantID: fixtures.tenantID, Kind: authorization.ResourceKindChannel, ID: fixtures.channelID},
		DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{{
			Actor: actor, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow,
		}},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal("create Channel ACL fixture failed")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("commit ACL fixtures failed")
	}
	fixtures.spaceACLVersion = spaceACL.Version
	fixtures.channelACLVersion = channelACL.Version
	return fixtures
}

func openCurrentAuthorizationDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run current-authorization integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("parse PostgreSQL maintenance DSN failed")
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("connect PostgreSQL maintenance database failed")
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
		t.Fatal("read PostgreSQL server version failed")
	}
	if version != "16.4" && !strings.HasPrefix(version, "16.4.") {
		t.Fatalf("PostgreSQL 16.4 required, found %s", version)
	}
	databaseName := "threadline_authorization_current_go_test_" + strconv.Itoa(os.Getpid()) + "_" +
		strconv.FormatInt(time.Now().UnixNano(), 10)
	if !safeCurrentAuthorizationDatabaseName(databaseName) {
		t.Fatal("refusing unsafe disposable PostgreSQL database name")
	}
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		t.Fatal("create disposable PostgreSQL database failed")
	}
	config := adminConfig.Copy()
	config.Database = databaseName
	var database *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if database != nil {
			if err := database.Close(cleanupCtx); err != nil {
				t.Error("close disposable PostgreSQL database failed")
			}
		}
		if !safeCurrentAuthorizationDatabaseName(databaseName) {
			t.Error("refusing to drop unexpected PostgreSQL database")
			return
		}
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable PostgreSQL database failed")
		}
	})
	database, err = pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("connect disposable PostgreSQL database failed")
	}
	for index := 1; index <= 7; index++ {
		prefix := fmt.Sprintf("%06d_", index)
		matches, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "..", "db", "migrations", prefix+"*.up.sql"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("resolve migration %s failed: %v (%v)", prefix, err, matches)
		}
		migration, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal("read migration failed")
		}
		if _, err := database.PgConn().Exec(ctx, string(migration)).ReadAll(); err != nil {
			t.Fatalf("apply migration %s failed", filepath.Base(matches[0]))
		}
	}
	return ctx, database
}

func safeCurrentAuthorizationDatabaseName(name string) bool {
	const prefix = "threadline_authorization_current_go_test_"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	pid, timestamp, found := strings.Cut(strings.TrimPrefix(name, prefix), "_")
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
