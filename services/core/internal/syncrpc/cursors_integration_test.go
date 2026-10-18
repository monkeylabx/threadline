package syncrpc

import (
	"bytes"
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
	"google.golang.org/protobuf/proto"

	"github.com/monkeylabx/threadline/services/core/internal/authorization"
	"github.com/monkeylabx/threadline/services/core/internal/authorization/aclstore"
	messagev1 "github.com/monkeylabx/threadline/services/gen/threadline/message/v1"
	syncv1 "github.com/monkeylabx/threadline/services/gen/threadline/sync/v1"
	"github.com/monkeylabx/threadline/services/gen/threadline/sync/v1/syncv1connect"
	"github.com/monkeylabx/threadline/services/internal/rpcmiddleware"
)

const (
	outsiderID     = "actor-sync-outsider-synthetic"
	otherDeviceID  = "device-sync-phone-synthetic"
	migrationCount = 12
)

func TestUpdateAndGetCursorsPersistMonotonicDevicePositions(t *testing.T) {
	ctx, pool, databaseName := openSyncDatabase(t)
	createSyncFixtures(t, ctx, pool)
	handler, err := New(pool)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"laptop-token":   readerSession(testTenantID, testActorID, testDeviceID),
		"phone-token":    readerSession(testTenantID, testActorID, otherDeviceID),
		"outsider-token": readerSession(testTenantID, outsiderID, "device-sync-outsider-synthetic"),
	}}
	mux := http.NewServeMux()
	mux.Handle(syncv1connect.NewSyncServiceHandler(handler, connect.WithInterceptors(rpcmiddleware.NewAuthenticationInterceptor(verifier))))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	client := syncv1connect.NewSyncServiceClient(httpServer.Client(), httpServer.URL)

	// Committed history is two events; the head row says so.
	accepted, err := client.UpdateCursors(ctx, authorized("laptop-token", &syncv1.UpdateCursorsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 2)}}))
	if err != nil || len(accepted.Msg.GetCursorRejections()) != 0 || len(accepted.Msg.GetRejectedCursors()) != 0 {
		t.Fatalf("UpdateCursors(2) = %v %v, want accepted without rejections", accepted, err)
	}
	ahead, err := client.UpdateCursors(ctx, authorized("laptop-token", &syncv1.UpdateCursorsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 3)}}))
	assertRejection(t, ahead, err, "cursor_ahead_of_history", 0)
	regress, err := client.UpdateCursors(ctx, authorized("laptop-token", &syncv1.UpdateCursorsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 1)}}))
	assertRejection(t, regress, err, "cursor_regression", 2)
	same, err := client.UpdateCursors(ctx, authorized("laptop-token", &syncv1.UpdateCursorsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 2)}}))
	if err != nil || len(same.Msg.GetCursorRejections()) != 0 {
		t.Fatalf("UpdateCursors(equal) = %v %v, want idempotent acceptance", same, err)
	}
	denied, err := client.UpdateCursors(ctx, authorized("outsider-token", &syncv1.UpdateCursorsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 1)}}))
	assertRejection(t, denied, err, "read_unavailable", 0)
	mixed, err := client.UpdateCursors(ctx, authorized("phone-token", &syncv1.UpdateCursorsRequest{Cursors: []*syncv1.SyncCursor{
		channelCursor(testChannelID, 1), channelCursor("channel-sync-missing-synthetic", 1),
	}}))
	if err != nil || len(mixed.Msg.GetCursorRejections()) != 1 || mixed.Msg.GetCursorRejections()[0].GetError().GetReason() != "read_unavailable" {
		t.Fatalf("UpdateCursors(mixed) = %v %v, want one accepted and one read_unavailable rejection", mixed, err)
	}

	laptop, err := client.GetCursors(ctx, authorized("laptop-token", &syncv1.GetCursorsRequest{}))
	if err != nil || len(laptop.Msg.GetCursors()) != 1 || laptop.Msg.GetCursors()[0].GetLastAppliedSeq() != 2 || laptop.Msg.GetCursors()[0].GetUpdatedAt() == nil {
		t.Fatalf("GetCursors(laptop) = %v %v, want one cursor at 2 with server time", laptop, err)
	}
	phone, err := client.GetCursors(ctx, authorized("phone-token", &syncv1.GetCursorsRequest{ChannelIds: []string{testChannelID}}))
	if err != nil || len(phone.Msg.GetCursors()) != 1 || phone.Msg.GetCursors()[0].GetLastAppliedSeq() != 1 {
		t.Fatalf("GetCursors(phone) = %v %v, want the phone's own cursor at 1, not the laptop's", phone, err)
	}
	if _, err := client.GetCursors(ctx, authorized("laptop-token", &syncv1.GetCursorsRequest{DmIds: []string{"dm-synthetic"}})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("GetCursors(dm) = %v, want FailedPrecondition", err)
	}
	if _, err := client.GetCursors(ctx, connect.NewRequest(&syncv1.GetCursorsRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("GetCursors() without credential = %v, want CodeUnauthenticated", err)
	}
	assertNoOpenTransactions(t, ctx, pool, databaseName)
}

func authorized[T any](credential string, body *T) *connect.Request[T] {
	request := connect.NewRequest(body)
	request.Header().Set("Authorization", "Bearer "+credential)
	return request
}

func assertRejection(t *testing.T, response *connect.Response[syncv1.UpdateCursorsResponse], err error, reason string, serverPosition uint64) {
	t.Helper()
	if err != nil || len(response.Msg.GetCursorRejections()) != 1 || len(response.Msg.GetRejectedCursors()) != 1 {
		t.Fatalf("UpdateCursors() = %v %v, want exactly one rejection reported twice", response, err)
	}
	rejection := response.Msg.GetCursorRejections()[0]
	if rejection.GetError().GetReason() != reason || rejection.GetServerCursor().GetLastAppliedSeq() != serverPosition {
		t.Fatalf("rejection = %v, want reason %s with server position %d", rejection, reason, serverPosition)
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

// createSyncFixtures seeds a Channel with two committed events (head 2), a
// member reader with channel.read, and a tenant member without membership.
func createSyncFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("begin fixture transaction failed")
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version) VALUES ($1, 'Sync Synthetic', 1, 'policy-sync-v1')`, []any{testTenantID}},
		{`INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state) VALUES ($1, 1, $2, 'Reader Synthetic', 4, 2), ($1, 1, $3, 'Outsider Synthetic', 4, 2)`, []any{testTenantID, testActorID, outsiderID}},
		{`INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable) VALUES ($1, 'space-sync-synthetic', 'Space Synthetic', TRUE)`, []any{testTenantID}},
		{`INSERT INTO domain.channels (tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id) VALUES ($1, $2, 'space-sync-synthetic', 'Sync Channel Synthetic', 2, 1, 'group-sync-synthetic')`, []any{testTenantID, testChannelID}},
		{`INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role) VALUES ($1, $2, 1, $3, 3)`, []any{testTenantID, testChannelID, testActorID}},
		{`INSERT INTO domain.conversation_sequence_heads (tenant_id, conversation_kind, conversation_id, last_seq) VALUES ($1, 1, $2, 2)`, []any{testTenantID, testChannelID}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("create sync fixtures failed: %v", err)
		}
	}
	_, err = aclstore.ReplaceCurrent(ctx, tx, aclstore.Replacement{
		Resource:      authorization.ResourceRef{TenantID: testTenantID, Kind: authorization.ResourceKindChannel, ID: testChannelID},
		DefaultEffect: authorization.ACLEffectDeny,
		Entries: []authorization.ACLEntry{{
			Actor: authorization.ActorRef{Type: rpcmiddleware.ActorTypeHuman, ID: testActorID}, Action: authorization.ActionChannelRead, Effect: authorization.ACLEffectAllow,
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

// openSyncDatabase creates a disposable PostgreSQL 16.4 database with the
// first twelve core migrations and returns a pool bound to it.
func openSyncDatabase(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("THREADLINE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run syncrpc integration tests")
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
	databaseName := "threadline_syncrpc_go_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
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
		if !strings.HasPrefix(databaseName, "threadline_syncrpc_go_test_") {
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

func TestSyncEventsReturnsContiguousBatchesPerCursor(t *testing.T) {
	ctx, pool, databaseName := openSyncDatabase(t)
	createSyncFixtures(t, ctx, pool)
	insertSyncEvents(t, ctx, pool, 1, 2)
	handler, err := New(pool)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	verifier := fakeVerifier{sessions: map[string]rpcmiddleware.VerifiedSession{
		"laptop-token":   readerSession(testTenantID, testActorID, testDeviceID),
		"outsider-token": readerSession(testTenantID, outsiderID, "device-sync-outsider-synthetic"),
	}}
	mux := http.NewServeMux()
	mux.Handle(syncv1connect.NewSyncServiceHandler(handler, connect.WithInterceptors(rpcmiddleware.NewAuthenticationInterceptor(verifier))))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	client := syncv1connect.NewSyncServiceClient(httpServer.Client(), httpServer.URL)

	all, err := client.SyncEvents(ctx, authorized("laptop-token", &syncv1.SyncEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 0)}}))
	if err != nil || len(all.Msg.GetBatches()) != 1 || len(all.Msg.GetCursorRejections()) != 0 {
		t.Fatalf("SyncEvents(from 0) = %v %v, want one batch", all, err)
	}
	batch := all.Msg.GetBatches()[0]
	if len(batch.GetEvents()) != 2 || batch.GetNextSeq() != 2 || batch.GetHasMore() ||
		batch.GetEvents()[0].GetServerCommit().GetChannelSeq() != 1 || batch.GetEvents()[0].GetEventId() != "event-sync-1-synthetic" {
		t.Fatalf("batch = %v, want events 1,2 with next_seq 2 and no more", batch)
	}
	limited, err := client.SyncEvents(ctx, authorized("laptop-token", &syncv1.SyncEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 0)}, MaxEvents: 1}))
	if err != nil || len(limited.Msg.GetBatches()) != 1 || len(limited.Msg.GetBatches()[0].GetEvents()) != 1 ||
		limited.Msg.GetBatches()[0].GetNextSeq() != 1 || !limited.Msg.GetBatches()[0].GetHasMore() {
		t.Fatalf("SyncEvents(max 1) = %v %v, want one event with has_more", limited, err)
	}
	caughtUp, err := client.SyncEvents(ctx, authorized("laptop-token", &syncv1.SyncEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 2)}}))
	if err != nil || len(caughtUp.Msg.GetBatches()) != 0 {
		t.Fatalf("SyncEvents(from 2) = %v %v, want no batch", caughtUp, err)
	}
	mixed, err := client.SyncEvents(ctx, authorized("outsider-token", &syncv1.SyncEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 0)}}))
	if err != nil || len(mixed.Msg.GetBatches()) != 0 || len(mixed.Msg.GetCursorRejections()) != 1 || mixed.Msg.GetCursorRejections()[0].GetError().GetReason() != "read_unavailable" {
		t.Fatalf("SyncEvents(outsider) = %v %v, want one read_unavailable rejection", mixed, err)
	}
	if _, err := client.SyncEvents(ctx, connect.NewRequest(&syncv1.SyncEventsRequest{Cursors: []*syncv1.SyncCursor{channelCursor(testChannelID, 0)}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("SyncEvents() without credential = %v, want CodeUnauthenticated", err)
	}
	assertNoOpenTransactions(t, ctx, pool, databaseName)
}

// insertSyncEvents stores opaque envelopes at the given sequences for the
// fixture Channel; the head row already says last_seq 2.
func insertSyncEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sequences ...int64) {
	t.Helper()
	for _, sequence := range sequences {
		id := fmt.Sprintf("event-sync-%d-synthetic", sequence)
		encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(&messagev1.ChannelEventEnvelope{
			EventId: id, TenantId: testTenantID,
			Conversation: &messagev1.ChannelEventEnvelope_ChannelId{ChannelId: testChannelID},
			Body:         &messagev1.ChannelEventEnvelope_ApplicationCiphertext{ApplicationCiphertext: []byte("body-synthetic")},
		})
		if err != nil {
			t.Fatal("marshal fixture envelope failed")
		}
		if _, err := pool.Exec(ctx, `INSERT INTO domain.channel_events (tenant_id, event_id, channel_id, channel_seq, sender_actor_type, sender_actor_id,
			sender_device_id, idempotency_key, category, envelope, content_hash)
			VALUES ($1, $2, $3, $4, 1, $5, $6, $7, 1, $8, $9)`,
			testTenantID, id, testChannelID, sequence, testActorID, testDeviceID, "key-"+id, encoded, bytes.Repeat([]byte{0x11}, 32)); err != nil {
			t.Fatalf("insert fixture event %d failed: %v", sequence, err)
		}
	}
}
