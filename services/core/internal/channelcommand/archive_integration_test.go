package channelcommand

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

	"github.com/jackc/pgx/v5"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/aclstore"
	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	archiveTenantID      = "tenant-channel-archive-synthetic"
	archiveBetaTenantID  = "tenant-channel-archive-beta-synthetic"
	archiveChannelID     = "channel-archive-shared-synthetic"
	archiveBetaOnlyID    = "channel-archive-beta-only-synthetic"
	archiveOwnerActorID  = "actor-channel-archive-owner-synthetic"
	archiveDeniedActorID = "actor-channel-archive-denied-synthetic"
)

type archiveFixtures struct {
	aclVersion string
}

func TestArchiveAuthorizedMutationRemainsCallerOwnedAndTenantExact(t *testing.T) {
	ctx, database := openArchiveDatabase(t)
	fixtures := createArchiveFixtures(t, ctx, database)
	before := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID)

	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin rollback archive transaction failed")
	}
	result, commandErr, authenticationErr := archiveAuthenticated(
		ctx, archiveTenantID, archiveOwnerActorID, tx, archiveChannelID,
	)
	if authenticationErr != nil || commandErr != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Archive() errors = auth:%v command:%v", authenticationErr, commandErr)
	}
	if result.ChannelID != archiveChannelID ||
		result.PolicyVersion != "policy-channel-archive-v1" ||
		result.ACLVersion != fixtures.aclVersion {
		_ = tx.Rollback(ctx)
		t.Fatalf("Archive() = %#v, want exact mutation and authorization evidence", result)
	}
	inside := getArchiveChannel(t, ctx, tx, archiveTenantID, archiveChannelID)
	assertOnlyChannelStateChanged(t, before, inside, 2)
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal("rollback caller-owned archive transaction failed")
	}
	afterRollback := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID)
	assertOnlyChannelStateChanged(t, before, afterRollback, 1)

	commitTx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin commit archive transaction failed")
	}
	result, commandErr, authenticationErr = archiveAuthenticated(
		ctx, archiveTenantID, archiveOwnerActorID, commitTx, archiveChannelID,
	)
	if authenticationErr != nil || commandErr != nil {
		_ = commitTx.Rollback(ctx)
		t.Fatalf("committed Archive() errors = auth:%v command:%v", authenticationErr, commandErr)
	}
	if result.ACLVersion != fixtures.aclVersion {
		_ = commitTx.Rollback(ctx)
		t.Fatal("committed Archive() lost ACL version evidence")
	}
	if err := commitTx.Commit(ctx); err != nil {
		t.Fatal("commit caller-owned archive transaction failed")
	}
	afterCommit := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID)
	assertOnlyChannelStateChanged(t, before, afterCommit, 2)
	beta := getArchiveChannel(t, ctx, database, archiveBetaTenantID, archiveChannelID)
	if beta.State != 1 || beta.Name != "Beta Same-ID Channel Synthetic" {
		t.Fatal("Archive() changed the same Channel ID in another Tenant")
	}

	repeatTx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin repeat archive transaction failed")
	}
	defer func() { _ = repeatTx.Rollback(context.Background()) }()
	_, commandErr, authenticationErr = archiveAuthenticated(
		ctx, archiveTenantID, archiveOwnerActorID, repeatTx, archiveChannelID,
	)
	assertArchiveDenied(t, commandErr, authenticationErr, authorization.ReasonResourceStateDenied)
}

func TestArchiveDenialsNeverMutateAnyChannel(t *testing.T) {
	ctx, database := openArchiveDatabase(t)
	createArchiveFixtures(t, ctx, database)
	tests := []struct {
		name      string
		actorID   string
		channelID string
		reason    authorization.Reason
	}{
		{name: "ACL deny", actorID: archiveDeniedActorID, channelID: archiveChannelID, reason: authorization.ReasonACLDefaultDeny},
		{name: "departed Membership", actorID: "actor-channel-archive-departed-synthetic", channelID: archiveChannelID, reason: authorization.ReasonNotAMember},
		{name: "disabled Member", actorID: "actor-channel-archive-disabled-synthetic", channelID: archiveChannelID, reason: authorization.ReasonMemberInactive},
		{name: "missing Member", actorID: "actor-channel-archive-missing-synthetic", channelID: archiveChannelID, reason: authorization.ReasonFactsUnavailable},
		{name: "missing Channel", actorID: archiveOwnerActorID, channelID: "channel-archive-missing-synthetic", reason: authorization.ReasonFactsUnavailable},
		{name: "cross-Tenant only ID", actorID: archiveOwnerActorID, channelID: archiveBetaOnlyID, reason: authorization.ReasonFactsUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx, err := database.Begin(ctx)
			if err != nil {
				t.Fatal("begin denied archive transaction failed")
			}
			_, commandErr, authenticationErr := archiveAuthenticated(
				ctx, archiveTenantID, test.actorID, tx, test.channelID,
			)
			assertArchiveDenied(t, commandErr, authenticationErr, test.reason)
			if got := getArchiveChannel(t, ctx, tx, archiveTenantID, archiveChannelID); got.State != 1 {
				t.Fatal("denied Archive() wrote the authorized fixture Channel before rollback")
			}
			if got := getArchiveChannel(t, ctx, tx, archiveBetaTenantID, archiveBetaOnlyID); got.State != 1 {
				t.Fatal("denied Archive() wrote a cross-Tenant Channel before rollback")
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal("rollback denied archive transaction failed")
			}
			if got := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID); got.State != 1 {
				t.Fatal("denied Archive() mutated the authorized fixture Channel")
			}
			if got := getArchiveChannel(t, ctx, database, archiveBetaTenantID, archiveBetaOnlyID); got.State != 1 {
				t.Fatal("denied Archive() mutated a cross-Tenant Channel")
			}
		})
	}
}

func TestArchiveRejectsSnapshotIsolationWithoutMutation(t *testing.T) {
	ctx, database := openArchiveDatabase(t)
	createArchiveFixtures(t, ctx, database)
	tx, err := database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal("begin repeatable-read archive transaction failed")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, commandErr, authenticationErr := archiveAuthenticated(
		ctx, archiveTenantID, archiveOwnerActorID, tx, archiveChannelID,
	)
	if authenticationErr != nil {
		t.Fatalf("authenticate test Principal: %v", authenticationErr)
	}
	var typed *Error
	if !errors.As(commandErr, &typed) || typed.Code() != ErrorInvalidInput {
		t.Fatalf("Archive() error = %v, want typed invalid-input", commandErr)
	}
	if got := getArchiveChannel(t, ctx, tx, archiveTenantID, archiveChannelID); got.State != 1 {
		t.Fatal("non-READ-COMMITTED Archive() wrote Channel before rollback")
	}
	if got := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID); got.State != 1 {
		t.Fatal("non-READ-COMMITTED Archive() mutated Channel")
	}
}

func TestArchiveDeadlineDuringAuthorizationLockWaitNeverMutates(t *testing.T) {
	ctx, database := openArchiveDatabase(t)
	createArchiveFixtures(t, ctx, database)
	writer, err := pgx.ConnectConfig(ctx, database.Config().Copy())
	if err != nil {
		t.Fatal("connect deadline lock writer failed")
	}
	defer func() { _ = writer.Close(context.Background()) }()
	writerTx, err := writer.Begin(ctx)
	if err != nil {
		t.Fatal("begin deadline lock writer failed")
	}
	defer func() { _ = writerTx.Rollback(context.Background()) }()
	if _, err := writerTx.Exec(ctx, `
		UPDATE domain.channels SET name = name WHERE tenant_id = $1 AND channel_id = $2
	`, archiveTenantID, archiveChannelID); err != nil {
		t.Fatal("lock Channel for deadline test failed")
	}

	resolver, err := pgx.ConnectConfig(ctx, database.Config().Copy())
	if err != nil {
		t.Fatal("connect deadline archive resolver failed")
	}
	defer func() { _ = resolver.Close(context.Background()) }()
	resolverTx, err := resolver.Begin(ctx)
	if err != nil {
		t.Fatal("begin deadline archive resolver failed")
	}
	defer func() { _ = resolverTx.Rollback(context.Background()) }()
	deadlineCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, commandErr, authenticationErr := archiveAuthenticated(
		deadlineCtx, archiveTenantID, archiveOwnerActorID, resolverTx, archiveChannelID,
	)
	if authenticationErr != nil {
		t.Fatalf("authenticate test Principal: %v", authenticationErr)
	}
	if !errors.Is(commandErr, context.DeadlineExceeded) {
		t.Fatalf("Archive() error = %v, want context.DeadlineExceeded", commandErr)
	}
	if got := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID); got.State != 1 {
		t.Fatal("deadline during authorization lock wait mutated Channel")
	}
}

func TestArchiveMutationFailureIsTypedAndNeverPersists(t *testing.T) {
	ctx, database := openArchiveDatabase(t)
	createArchiveFixtures(t, ctx, database)
	if _, err := database.Exec(ctx, `
		CREATE FUNCTION domain.fail_channel_archive_synthetic()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.state = 2 THEN
		    RAISE EXCEPTION 'synthetic archive failure';
		  END IF;
		  RETURN NEW;
		END;
		$$
	`); err != nil {
		t.Fatal("create synthetic archive failure function failed")
	}
	if _, err := database.Exec(ctx, `
		CREATE TRIGGER channel_archive_failure_synthetic
		BEFORE UPDATE ON domain.channels
		FOR EACH ROW EXECUTE FUNCTION domain.fail_channel_archive_synthetic()
	`); err != nil {
		t.Fatal("create synthetic archive failure trigger failed")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin mutation-failure archive transaction failed")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, commandErr, authenticationErr := archiveAuthenticated(
		ctx, archiveTenantID, archiveOwnerActorID, tx, archiveChannelID,
	)
	if authenticationErr != nil {
		t.Fatalf("authenticate test Principal: %v", authenticationErr)
	}
	var typed *Error
	if !errors.As(commandErr, &typed) || typed.Code() != ErrorPersistence {
		t.Fatalf("Archive() error = %v, want typed persistence-failure", commandErr)
	}
	if commandErr.Error() != "channel archive: persistence-failure" {
		t.Fatalf("Archive() error text = %q, want stable secret-safe failure", commandErr.Error())
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal("rollback failed archive mutation transaction failed")
	}
	if got := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID); got.State != 1 {
		t.Fatal("failed archive mutation became visible")
	}
}

func TestArchiveReadsWriterCommittedDenialBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, pgx.Tx) error
		reason authorization.Reason
	}{
		{
			name: "ACL replacement",
			mutate: func(ctx context.Context, tx pgx.Tx) error {
				_, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
					Resource: authorization.ResourceRef{
						TenantID: archiveTenantID, Kind: authorization.ResourceKindChannel, ID: archiveChannelID,
					},
					DefaultEffect: authorization.ACLEffectDeny,
				})
				return err
			},
			reason: authorization.ReasonACLDefaultDeny,
		},
		{
			name: "Membership departure",
			mutate: func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `
					UPDATE domain.channel_memberships SET left_at = CURRENT_TIMESTAMP
					WHERE tenant_id = $1 AND channel_id = $2 AND actor_type = 1 AND actor_id = $3 AND left_at IS NULL
				`, archiveTenantID, archiveChannelID, archiveOwnerActorID)
				return err
			},
			reason: authorization.ReasonNotAMember,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runWriterFirstArchiveDenial(t, test.mutate, test.reason)
		})
	}
}

func TestArchiveRetainsAuthorizationLocksUntilCallerRollback(t *testing.T) {
	tests := []struct {
		name            string
		applicationName string
		mutate          func(context.Context, pgx.Tx) error
		wantChannelName string
	}{
		{
			name:            "ACL writer",
			applicationName: "threadline-channel-archive-acl-writer",
			mutate: func(ctx context.Context, tx pgx.Tx) error {
				_, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
					Resource: authorization.ResourceRef{
						TenantID: archiveTenantID, Kind: authorization.ResourceKindChannel, ID: archiveChannelID,
					},
					DefaultEffect: authorization.ACLEffectDeny,
				})
				return err
			},
			wantChannelName: "Alpha Same-ID Channel Synthetic",
		},
		{
			name:            "direct Channel writer",
			applicationName: "threadline-channel-archive-resource-writer",
			mutate: func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `
					UPDATE domain.channels SET name = 'Channel Writer Synthetic'
					WHERE tenant_id = $1 AND channel_id = $2
				`, archiveTenantID, archiveChannelID)
				return err
			},
			wantChannelName: "Channel Writer Synthetic",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runArchiveLockRetention(t, test.applicationName, test.mutate, test.wantChannelName)
		})
	}
}

func runArchiveLockRetention(
	t *testing.T,
	applicationName string,
	mutate func(context.Context, pgx.Tx) error,
	wantChannelName string,
) {
	t.Helper()
	ctx, database := openArchiveDatabase(t)
	createArchiveFixtures(t, ctx, database)
	archiveTx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin lock-retention archive transaction failed")
	}
	_, commandErr, authenticationErr := archiveAuthenticated(
		ctx, archiveTenantID, archiveOwnerActorID, archiveTx, archiveChannelID,
	)
	if authenticationErr != nil || commandErr != nil {
		_ = archiveTx.Rollback(ctx)
		t.Fatalf("Archive() errors = auth:%v command:%v", authenticationErr, commandErr)
	}

	writerConfig := database.Config().Copy()
	writerConfig.RuntimeParams["application_name"] = applicationName
	writer, err := pgx.ConnectConfig(ctx, writerConfig)
	if err != nil {
		_ = archiveTx.Rollback(ctx)
		t.Fatal("connect lock-retention writer failed")
	}
	defer func() { _ = writer.Close(context.Background()) }()
	type writerResult struct{ err error }
	resultChannel := make(chan writerResult, 1)
	go func() {
		writerTx, beginErr := writer.Begin(ctx)
		if beginErr != nil {
			resultChannel <- writerResult{err: beginErr}
			return
		}
		if mutationErr := mutate(ctx, writerTx); mutationErr != nil {
			_ = writerTx.Rollback(context.Background())
			resultChannel <- writerResult{err: mutationErr}
			return
		}
		resultChannel <- writerResult{err: writerTx.Commit(ctx)}
	}()
	waitForArchiveApplicationLock(t, ctx, database, applicationName)
	select {
	case got := <-resultChannel:
		_ = archiveTx.Rollback(ctx)
		t.Fatalf("writer completed before caller ended archive transaction: %v", got.err)
	default:
	}
	if err := archiveTx.Rollback(ctx); err != nil {
		t.Fatal("rollback lock-retention archive transaction failed")
	}
	select {
	case got := <-resultChannel:
		if got.err != nil {
			t.Fatalf("writer failed after archive rollback: %v", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not resume after archive rollback")
	}
	if got := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID); got.State != 1 || got.Name != wantChannelName {
		t.Fatal("caller rollback or resumed writer produced unexpected Channel state")
	}
}

func runWriterFirstArchiveDenial(
	t *testing.T,
	mutate func(context.Context, pgx.Tx) error,
	wantReason authorization.Reason,
) {
	t.Helper()
	ctx, database := openArchiveDatabase(t)
	createArchiveFixtures(t, ctx, database)
	writer, err := pgx.ConnectConfig(ctx, database.Config().Copy())
	if err != nil {
		t.Fatal("connect writer-first archive mutation failed")
	}
	defer func() { _ = writer.Close(context.Background()) }()
	writerTx, err := writer.Begin(ctx)
	if err != nil {
		t.Fatal("begin writer-first archive mutation failed")
	}
	defer func() { _ = writerTx.Rollback(context.Background()) }()
	if err := mutate(ctx, writerTx); err != nil {
		t.Fatal("stage writer-first archive denial failed")
	}

	resolverConfig := database.Config().Copy()
	resolverConfig.RuntimeParams["application_name"] = "threadline-channel-archive-resolver"
	resolver, err := pgx.ConnectConfig(ctx, resolverConfig)
	if err != nil {
		t.Fatal("connect writer-first archive resolver failed")
	}
	defer func() { _ = resolver.Close(context.Background()) }()
	resolverTx, err := resolver.Begin(ctx)
	if err != nil {
		t.Fatal("begin writer-first archive resolver failed")
	}
	defer func() { _ = resolverTx.Rollback(context.Background()) }()
	type archiveResult struct {
		commandErr        error
		authenticationErr error
	}
	resultChannel := make(chan archiveResult, 1)
	go func() {
		_, commandErr, authenticationErr := archiveAuthenticated(
			ctx, archiveTenantID, archiveOwnerActorID, resolverTx, archiveChannelID,
		)
		resultChannel <- archiveResult{commandErr: commandErr, authenticationErr: authenticationErr}
	}()
	waitForArchiveApplicationLock(t, ctx, database, "threadline-channel-archive-resolver")
	if err := writerTx.Commit(ctx); err != nil {
		t.Fatal("commit writer-first archive denial failed")
	}
	select {
	case got := <-resultChannel:
		assertArchiveDenied(t, got.commandErr, got.authenticationErr, wantReason)
		if channel := getArchiveChannel(t, ctx, resolverTx, archiveTenantID, archiveChannelID); channel.State != 1 {
			t.Fatal("writer-first authorization denial wrote Channel before rollback")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Archive() did not resume after writer commit")
	}
	if got := getArchiveChannel(t, ctx, database, archiveTenantID, archiveChannelID); got.State != 1 {
		t.Fatal("writer-first authorization denial allowed Channel mutation")
	}
}

func assertArchiveDenied(
	t *testing.T,
	commandErr error,
	authenticationErr error,
	wantReason authorization.Reason,
) {
	t.Helper()
	if authenticationErr != nil {
		t.Fatalf("authenticate test Principal: %v", authenticationErr)
	}
	var typed *Error
	if !errors.As(commandErr, &typed) ||
		typed.Code() != ErrorDenied ||
		typed.Reason() != wantReason {
		t.Fatalf("Archive() error = %v, want typed denial %s", commandErr, wantReason)
	}
	if commandErr.Error() != "channel archive: denied" {
		t.Fatalf("Archive() error text = %q, want stable secret-safe denial", commandErr.Error())
	}
}

func createArchiveFixtures(t *testing.T, ctx context.Context, database *pgx.Conn) archiveFixtures {
	t.Helper()
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal("begin Channel archive fixture transaction failed")
	}
	fixtures := []struct {
		query string
		args  []any
	}{
		{
			query: `INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version) VALUES
			  ($1, 'Channel Archive Synthetic', 1, 'policy-channel-archive-v1'),
			  ($2, 'Channel Archive Beta Synthetic', 1, 'policy-channel-archive-beta-v1')`,
			args: []any{archiveTenantID, archiveBetaTenantID},
		},
		{
			query: `INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state) VALUES
			  ($1, 1, $3, 'Archive Owner Synthetic', 4, 2),
			  ($1, 1, $4, 'Archive Denied Synthetic', 4, 2),
			  ($1, 1, 'actor-channel-archive-departed-synthetic', 'Archive Departed Synthetic', 4, 2),
			  ($1, 1, 'actor-channel-archive-disabled-synthetic', 'Archive Disabled Synthetic', 4, 2),
			  ($2, 1, $3, 'Archive Beta Owner Synthetic', 4, 2)`,
			args: []any{archiveTenantID, archiveBetaTenantID, archiveOwnerActorID, archiveDeniedActorID},
		},
		{
			query: `INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable) VALUES
			  ($1, 'space-channel-archive-synthetic', 'Archive Space Synthetic', TRUE),
			  ($2, 'space-channel-archive-synthetic', 'Archive Beta Space Synthetic', TRUE)`,
			args: []any{archiveTenantID, archiveBetaTenantID},
		},
		{
			query: `INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id) VALUES
			  ($1, $3, 'space-channel-archive-synthetic', 'Alpha Same-ID Channel Synthetic', 2, 1, 'group-channel-archive-alpha-synthetic'),
			  ($2, $3, 'space-channel-archive-synthetic', 'Beta Same-ID Channel Synthetic', 2, 1, 'group-channel-archive-beta-synthetic'),
			  ($2, $4, 'space-channel-archive-synthetic', 'Beta Only Channel Synthetic', 2, 1, 'group-channel-archive-beta-only-synthetic')`,
			args: []any{archiveTenantID, archiveBetaTenantID, archiveChannelID, archiveBetaOnlyID},
		},
		{
			query: `INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role) VALUES
			  ($1, $3, 1, $4, 1),
			  ($1, $3, 1, $5, 1),
			  ($1, $3, 1, 'actor-channel-archive-departed-synthetic', 1),
			  ($1, $3, 1, 'actor-channel-archive-disabled-synthetic', 1),
			  ($2, $3, 1, $4, 1)`,
			args: []any{archiveTenantID, archiveBetaTenantID, archiveChannelID, archiveOwnerActorID, archiveDeniedActorID},
		},
		{
			query: `UPDATE domain.channel_memberships SET left_at = CURRENT_TIMESTAMP
			WHERE tenant_id = $1 AND channel_id = $2 AND actor_id = 'actor-channel-archive-departed-synthetic'`,
			args: []any{archiveTenantID, archiveChannelID},
		},
		{
			query: `UPDATE domain.members SET state = 3
			WHERE tenant_id = $1 AND actor_type = 1 AND actor_id = 'actor-channel-archive-disabled-synthetic'`,
			args: []any{archiveTenantID},
		},
	}
	for _, fixture := range fixtures {
		if _, err := tx.Exec(ctx, fixture.query, fixture.args...); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal("create Channel archive fixtures failed")
		}
	}
	stored, err := aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource: authorization.ResourceRef{
			TenantID: archiveTenantID, Kind: authorization.ResourceKindChannel, ID: archiveChannelID,
		},
		DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{{
			Actor:  authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: archiveOwnerActorID},
			Action: authorization.ActionChannelArchive,
			Effect: authorization.ACLEffectAllow,
		}},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal("create Channel archive ACL fixture failed")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("commit Channel archive ACL fixture failed")
	}
	return archiveFixtures{aclVersion: stored.Version}
}

type channelReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getArchiveChannel(
	t *testing.T,
	ctx context.Context,
	database channelReader,
	tenantID string,
	channelID string,
) dbgen.DomainChannel {
	t.Helper()
	row := database.QueryRow(ctx, `
		SELECT tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id, created_at
		FROM domain.channels WHERE tenant_id = $1 AND channel_id = $2
	`, tenantID, channelID)
	var channel dbgen.DomainChannel
	if err := row.Scan(
		&channel.TenantID, &channel.ChannelID, &channel.SpaceID, &channel.Name,
		&channel.Visibility, &channel.State, &channel.E2eeGroupID, &channel.CreatedAt,
	); err != nil {
		t.Fatal("read Channel archive fixture failed")
	}
	return channel
}

func assertOnlyChannelStateChanged(t *testing.T, before, after dbgen.DomainChannel, wantState int16) {
	t.Helper()
	if after.TenantID != before.TenantID || after.ChannelID != before.ChannelID ||
		after.SpaceID != before.SpaceID || after.Name != before.Name ||
		after.Visibility != before.Visibility || after.E2eeGroupID != before.E2eeGroupID ||
		after.CreatedAt != before.CreatedAt || after.State != wantState {
		t.Fatalf("Channel mutation changed fields outside state: before=%#v after=%#v", before, after)
	}
}

func waitForArchiveApplicationLock(t *testing.T, ctx context.Context, observer *pgx.Conn, applicationName string) {
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
			t.Fatal("observe Channel archive lock wait failed")
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected Channel archive transaction to wait on an authorization lock")
}

func openArchiveDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run Channel archive integration tests")
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
	databaseName := "threadline_channel_archive_go_test_" + strconv.Itoa(os.Getpid()) + "_" +
		strconv.FormatInt(time.Now().UnixNano(), 10)
	if !safeArchiveDatabaseName(databaseName) {
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
		if !safeArchiveDatabaseName(databaseName) {
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
		matches, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "db", "migrations", prefix+"*.up.sql"))
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

func safeArchiveDatabaseName(name string) bool {
	const prefix = "threadline_channel_archive_go_test_"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	pid, timestamp, found := strings.Cut(strings.TrimPrefix(name, prefix), "_")
	return found && archiveDigitsOnly(pid) && archiveDigitsOnly(timestamp)
}

func archiveDigitsOnly(value string) bool {
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
