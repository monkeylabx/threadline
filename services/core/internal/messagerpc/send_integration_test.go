package messagerpc

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
	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/message/v1/messagev1connect"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	betaTenantID   = "tenant-message-beta-synthetic"
	outsiderID     = "actor-message-outsider-synthetic"
	betaActorID    = "actor-message-beta-synthetic"
	migrationCount = 11
)

type rowCounts struct{ events, bindings, domainEvents, entries int }

func TestSendEventCommitsOnceAndReturnsTheOriginalReceiptOnRetry(t *testing.T) {
	ctx, pool, databaseName := openMessageDatabase(t)
	createMessageFixtures(t, ctx, pool)
	handler, err := New(pool, testProducer(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"sender-token":   senderSession(testTenantID, testActorID, testDeviceID),
		"outsider-token": senderSession(testTenantID, outsiderID, "device-message-outsider-synthetic"),
		"beta-token":     senderSession(betaTenantID, betaActorID, "device-message-beta-synthetic"),
	}}
	mux := http.NewServeMux()
	mux.Handle(messagev1connect.NewMessageServiceHandler(handler, connect.WithInterceptors(rpcmiddleware.NewAuthenticationInterceptor(verifier))))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	client := messagev1connect.NewMessageServiceClient(httpServer.Client(), httpServer.URL)

	first, err := client.SendEvent(ctx, authorizedSend("sender-token", validEnvelope("event-1", "key-1")))
	if err != nil {
		t.Fatalf("first SendEvent() error = %v", err)
	}
	if first.Msg.GetServerCommit().GetChannelSeq() != 1 || first.Msg.GetDeduplicated() ||
		first.Msg.GetEventId() != "event-1" || first.Msg.GetIdempotencyKey() != "key-1" || first.Msg.GetServerCommit().GetCommittedAt() == nil {
		t.Fatalf("first receipt = %v, want seq 1, not deduplicated, event-1/key-1 with commit time", first.Msg)
	}
	assertCounts(t, ctx, pool, rowCounts{1, 1, 1, 1})

	retry, err := client.SendEvent(ctx, authorizedSend("sender-token", validEnvelope("event-1", "key-1")))
	if err != nil {
		t.Fatalf("retry SendEvent() error = %v", err)
	}
	if !retry.Msg.GetDeduplicated() || retry.Msg.GetServerCommit().GetChannelSeq() != 1 ||
		!retry.Msg.GetServerCommit().GetCommittedAt().AsTime().Equal(first.Msg.GetServerCommit().GetCommittedAt().AsTime()) {
		t.Fatalf("retry receipt = %v, want the original receipt with deduplicated=true", retry.Msg)
	}
	assertCounts(t, ctx, pool, rowCounts{1, 1, 1, 1})

	changed := validEnvelope("event-1", "key-1")
	changed.Body = &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: []byte("changed-body-synthetic")}
	assertSendFails(t, ctx, client, "same key changed body", authorizedSend("sender-token", changed), connect.CodeAlreadyExists, "idempotency_key_conflict")
	assertSendFails(t, ctx, client, "same event id other key", authorizedSend("sender-token", validEnvelope("event-1", "key-2")), connect.CodeAlreadyExists, "event_id_conflict")
	assertCounts(t, ctx, pool, rowCounts{1, 1, 1, 1})

	second, err := client.SendEvent(ctx, authorizedSend("sender-token", validEnvelope("event-2", "key-2")))
	if err != nil || second.Msg.GetServerCommit().GetChannelSeq() != 2 || second.Msg.GetDeduplicated() {
		t.Fatalf("second SendEvent() = %v %v, want seq 2", second, err)
	}
	assertCounts(t, ctx, pool, rowCounts{2, 2, 2, 2})

	outsider := validEnvelope("event-3", "key-3")
	outsider.SenderActorId, outsider.SenderDeviceId = outsiderID, "device-message-outsider-synthetic"
	assertSendFails(t, ctx, client, "tenant member without channel membership", authorizedSend("outsider-token", outsider), connect.CodePermissionDenied, "publish_unavailable")
	beta := validEnvelope("event-4", "key-4")
	beta.TenantId, beta.SenderActorId, beta.SenderDeviceId = betaTenantID, betaActorID, "device-message-beta-synthetic"
	assertSendFails(t, ctx, client, "other tenant using the same channel id", authorizedSend("beta-token", beta), connect.CodePermissionDenied, "publish_unavailable")
	if _, err := client.SendEvent(ctx, connect.NewRequest(&messagev1.SendEventRequest{Envelope: validEnvelope("event-5", "key-5")})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("SendEvent() without credential = %v, want CodeUnauthenticated", err)
	}
	assertCounts(t, ctx, pool, rowCounts{2, 2, 2, 2})
	assertNoOpenTransactions(t, ctx, pool, databaseName)
}

func authorizedSend(credential string, envelope *messagev1.ChannelEventEnvelope) *connect.Request[messagev1.SendEventRequest] {
	request := connect.NewRequest(&messagev1.SendEventRequest{Envelope: envelope})
	request.Header().Set("Authorization", "Bearer "+credential)
	return request
}

func assertSendFails(t *testing.T, ctx context.Context, client messagev1connect.MessageServiceClient, label string,
	request *connect.Request[messagev1.SendEventRequest], code connect.Code, reason string) {
	t.Helper()
	_, err := client.SendEvent(ctx, request)
	_, gotReason := detailReason(t, err)
	if connect.CodeOf(err) != code || gotReason != reason {
		t.Fatalf("SendEvent() %s = %v (%s), want %v/%s", label, err, gotReason, code, reason)
	}
}

func assertCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want rowCounts) {
	t.Helper()
	var got rowCounts
	err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM domain.channel_events), (SELECT count(*) FROM domain.send_commands),
		       (SELECT count(*) FROM domain.domain_events), (SELECT count(*) FROM domain.transactional_outbox)`,
	).Scan(&got.events, &got.bindings, &got.domainEvents, &got.entries)
	if err != nil || got != want {
		t.Fatalf("row counts = %+v (%v), want %+v", got, err, want)
	}
}

func assertNoOpenTransactions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, databaseName string) {
	t.Helper()
	var open int
	err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND state LIKE 'idle in transaction%'", databaseName).Scan(&open)
	if err != nil || open != 0 {
		t.Fatalf("open transactions after requests = %d (%v), want 0", open, err)
	}
}

func createMessageFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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
		  ($1, 'Message Synthetic', 1, 'policy-message-v1'), ($2, 'Message Beta Synthetic', 1, 'policy-message-beta-v1')`, []any{testTenantID, betaTenantID}},
		{`INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state) VALUES
		  ($1, 1, $3, 'Sender Synthetic', 4, 2), ($1, 1, $4, 'Outsider Synthetic', 4, 2), ($2, 1, $5, 'Beta Synthetic', 4, 2)`,
			[]any{testTenantID, betaTenantID, testActorID, outsiderID, betaActorID}},
		{`INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable) VALUES ($1, 'space-message-synthetic', 'Space Synthetic', TRUE)`, []any{testTenantID}},
		{`INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id) VALUES
		  ($1, $2, 'space-message-synthetic', 'Message Channel Synthetic', 2, 1, 'group-message-synthetic')`, []any{testTenantID, testChannelID}},
		{`INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role) VALUES ($1, $2, 1, $3, 3)`, []any{testTenantID, testChannelID, testActorID}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("create message fixtures failed: %v", err)
		}
	}
	_, err = aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource:      authorization.ResourceRef{TenantID: testTenantID, Kind: authorization.ResourceKindChannel, ID: testChannelID},
		DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{
			{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: testActorID}, Action: authorization.ActionChannelPublish, Effect: authorization.ACLEffectAllow},
			{Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: testActorID}, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow},
		},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("seed channel.publish ACL failed: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("commit fixtures failed")
	}
}

// openMessageDatabase creates a disposable PostgreSQL 16.4 database with the
// first eleven core migrations and returns a pool bound to it.
func openMessageDatabase(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run messagerpc integration tests")
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
	databaseName := "threadline_messagerpc_go_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
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
	if _, err := pool.Exec(ctx, "CREATE EXTENSION pgcrypto"); err != nil {
		pool.Close()
		t.Fatalf("provision pgcrypto for migration 000009 failed: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if !strings.HasPrefix(databaseName, "threadline_messagerpc_go_test_") {
			t.Error("refusing to drop unexpected PostgreSQL database")
		} else if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable PostgreSQL database failed")
		}
		if err := admin.Close(cleanupCtx); err != nil {
			t.Error("close PostgreSQL maintenance connection failed")
		}
	})
	for index := 1; index <= migrationCount; index++ {
		matches, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "db", "migrations", fmt.Sprintf("%06d_*.up.sql", index)))
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
	return ctx, pool, databaseName
}
