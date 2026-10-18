package channelrpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/aclstore"
	channelv1 "github.com/monkeylabx/threadline/services/gen/threadline/channel/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/channel/v1/channelv1connect"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	rpcTenantID      = "tenant-channelrpc-synthetic"
	rpcBetaTenantID  = "tenant-channelrpc-beta-synthetic"
	rpcSpaceID       = "space-channelrpc-synthetic"
	rpcChannelID     = "channel-channelrpc-synthetic"
	rpcGroupID       = "group-channelrpc-synthetic"
	rpcMemberActorID = "actor-channelrpc-member-synthetic"
	rpcOutsiderID    = "actor-channelrpc-outsider-synthetic"
	rpcBetaActorID   = "actor-channelrpc-beta-synthetic"
	migrationCount   = 7
)

func TestGetChannelAuthorizesAndReadsInsideOneTransaction(t *testing.T) {
	ctx, pool, databaseName := openRPCDatabase(t)
	createRPCFixtures(t, ctx, pool)
	handler, err := New(pool)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"member-token":   memberSession(rpcTenantID, rpcMemberActorID),
		"outsider-token": memberSession(rpcTenantID, rpcOutsiderID),
		"beta-token":     memberSession(rpcBetaTenantID, rpcBetaActorID),
	}}
	mux := http.NewServeMux()
	mux.Handle(channelv1connect.NewChannelServiceHandler(
		handler, connect.WithInterceptors(rpcmiddleware.NewAuthenticationInterceptor(verifier)),
	))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	client := channelv1connect.NewChannelServiceClient(httpServer.Client(), httpServer.URL)

	response, err := client.GetChannel(ctx, authorizedRequest("member-token", rpcChannelID))
	if err != nil {
		t.Fatalf("GetChannel() as member error = %v", err)
	}
	assertSyntheticChannel(t, response.Msg.GetChannel())

	denied := map[string]*connect.Request[channelv1.GetChannelRequest]{
		"tenant member without channel membership": authorizedRequest("outsider-token", rpcChannelID),
		"member asking for a missing channel":      authorizedRequest("member-token", "channel-channelrpc-missing-synthetic"),
		"other tenant using the same channel id":   authorizedRequest("beta-token", rpcChannelID),
	}
	for name, request := range denied {
		if _, err := client.GetChannel(ctx, request); connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("GetChannel() %s = %v, want CodePermissionDenied", name, err)
		}
	}
	_, err = client.GetChannel(ctx, connect.NewRequest(&channelv1.GetChannelRequest{ChannelId: rpcChannelID}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("GetChannel() without credential = %v, want CodeUnauthenticated", err)
	}
	_, err = client.ListChannels(ctx, authorizedList("member-token"))
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("ListChannels() = %v, want CodeUnimplemented", err)
	}
	assertNoOpenTransactions(t, ctx, pool, databaseName)
}

func authorizedRequest(credential, channelID string) *connect.Request[channelv1.GetChannelRequest] {
	request := connect.NewRequest(&channelv1.GetChannelRequest{ChannelId: channelID})
	request.Header().Set("Authorization", "Bearer "+credential)
	return request
}

func authorizedList(credential string) *connect.Request[channelv1.ListChannelsRequest] {
	request := connect.NewRequest(&channelv1.ListChannelsRequest{})
	request.Header().Set("Authorization", "Bearer "+credential)
	return request
}

func assertSyntheticChannel(t *testing.T, channel *channelv1.Channel) {
	t.Helper()
	if channel.GetChannelId() != rpcChannelID || channel.GetTenantId() != rpcTenantID ||
		channel.GetSpaceId() != rpcSpaceID || channel.GetName() != "Channel RPC Synthetic" ||
		channel.GetE2EeGroupId() != rpcGroupID ||
		channel.GetVisibility() != channelv1.ChannelVisibility_CHANNEL_VISIBILITY_PRIVATE ||
		channel.GetState() != channelv1.ChannelState_CHANNEL_STATE_ACTIVE || channel.GetCreatedAt() == nil {
		t.Fatalf("GetChannel() = %v, want the stored synthetic channel", channel)
	}
}

// assertNoOpenTransactions proves every request rolled back: a pool connection
// left "idle in transaction" would hold the FOR UPDATE locks the evaluator took.
func assertNoOpenTransactions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, databaseName string) {
	t.Helper()
	var open int
	err := pool.QueryRow(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND state LIKE 'idle in transaction%'",
		databaseName,
	).Scan(&open)
	if err != nil || open != 0 {
		t.Fatalf("open transactions after requests = %d (%v), want 0", open, err)
	}
}

func createRPCFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("begin fixture transaction failed")
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version) VALUES
		  ($1, 'Channel RPC Synthetic', 1, 'policy-channelrpc-v1'),
		  ($2, 'Channel RPC Beta Synthetic', 1, 'policy-channelrpc-beta-v1')`, []any{rpcTenantID, rpcBetaTenantID}},
		{`INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state) VALUES
		  ($1, 1, $3, 'Member Synthetic', 4, 2),
		  ($1, 1, $4, 'Outsider Synthetic', 4, 2),
		  ($2, 1, $5, 'Beta Member Synthetic', 4, 2)`, []any{rpcTenantID, rpcBetaTenantID, rpcMemberActorID, rpcOutsiderID, rpcBetaActorID}},
		{`INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable) VALUES ($1, $2, 'Space Synthetic', TRUE)`,
			[]any{rpcTenantID, rpcSpaceID}},
		{`INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id) VALUES
		  ($1, $2, $3, 'Channel RPC Synthetic', 2, 1, $4)`, []any{rpcTenantID, rpcChannelID, rpcSpaceID, rpcGroupID}},
		{`INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role) VALUES ($1, $2, 1, $3, 3)`,
			[]any{rpcTenantID, rpcChannelID, rpcMemberActorID}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("create channelrpc fixtures failed: %v", err)
		}
	}
	_, err = aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource:      authorization.ResourceRef{TenantID: rpcTenantID, Kind: authorization.ResourceKindChannel, ID: rpcChannelID},
		DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{{
			Actor:  authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: rpcMemberActorID},
			Action: authorization.ActionChannelRead,
			Effect: authorization.ACLEffectAllow,
		}},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("seed channel.read ACL failed: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("commit fixtures failed")
	}
}

// openRPCDatabase creates a disposable PostgreSQL 16.4 database with the first
// seven core migrations and returns a pool bound to it.
func openRPCDatabase(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run channelrpc integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("parse PostgreSQL maintenance DSN failed")
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("connect PostgreSQL maintenance database failed")
	}
	databaseName := "threadline_channelrpc_go_test_" + strconv.Itoa(os.Getpid()) + "_" +
		strconv.FormatInt(time.Now().UnixNano(), 10)
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		t.Fatal("create disposable PostgreSQL database failed")
	}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("parse pool configuration failed")
	}
	poolConfig.ConnConfig.Database = databaseName
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("open disposable PostgreSQL pool failed")
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if !strings.HasPrefix(databaseName, "threadline_channelrpc_go_test_") {
			t.Error("refusing to drop unexpected PostgreSQL database")
		} else if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable PostgreSQL database failed")
		}
		if err := admin.Close(cleanupCtx); err != nil {
			t.Error("close PostgreSQL maintenance connection failed")
		}
	})
	applyMigrations(t, ctx, pool)
	return ctx, pool, databaseName
}

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for index := 1; index <= migrationCount; index++ {
		pattern := filepath.Join("..", "..", "..", "..", "db", "migrations", fmt.Sprintf("%06d_*.up.sql", index))
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) != 1 {
			t.Fatalf("resolve migration %06d failed: %v (%v)", index, err, matches)
		}
		migration, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal("read migration failed")
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %s failed: %v", filepath.Base(matches[0]), err)
		}
	}
}
