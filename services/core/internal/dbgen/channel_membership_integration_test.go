package dbgen

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

const (
	membershipAlphaTenant = "tenant-membership-go-alpha-synthetic"
	membershipBetaTenant  = "tenant-membership-go-beta-synthetic"
	membershipChannelID   = "channel-membership-go-shared-synthetic"
	membershipActorType   = int16(1)
)

func TestChannelMembershipPersistenceIntegration(t *testing.T) {
	ctx, testDatabase := openPostgresIntegrationTest(t, "channel_membership")
	applyPostgresTestMigrations(t, ctx, testDatabase,
		"000001_core_foundation.up.sql",
		"000002_organization.up.sql",
		"000003_member.up.sql",
		"000004_space.up.sql",
		"000005_channel_dm.up.sql",
		"000006_channel_membership.up.sql",
	)
	queries := New(testDatabase)

	createChannelMembershipFixtures(t, ctx, queries)
	testChannelMembershipGeneratedQueries(t, ctx, testDatabase, queries)
}

func createChannelMembershipFixtures(t *testing.T, ctx context.Context, queries *Queries) {
	t.Helper()
	for _, organization := range []CreateOrganizationParams{
		{
			TenantID: membershipAlphaTenant, DisplayName: "Membership Go Alpha Synthetic",
			State: 1, PolicyVersion: "policy-membership-go-alpha-v1",
		},
		{
			TenantID: membershipBetaTenant, DisplayName: "Membership Go Beta Synthetic",
			State: 1, PolicyVersion: "policy-membership-go-beta-v1",
		},
	} {
		if _, err := queries.CreateOrganization(ctx, organization); err != nil {
			t.Fatal("create synthetic Organization for Channel Membership test failed")
		}
	}

	for _, member := range []CreateMemberParams{
		{TenantID: membershipAlphaTenant, ActorType: membershipActorType, ActorID: "actor-membership-go-alice-synthetic", DisplayName: "Membership Go Alice Synthetic", Role: 4, State: 2},
		{TenantID: membershipAlphaTenant, ActorType: membershipActorType, ActorID: "actor-membership-go-bob-synthetic", DisplayName: "Membership Go Bob Synthetic", Role: 4, State: 2},
		{TenantID: membershipAlphaTenant, ActorType: membershipActorType, ActorID: "actor-membership-go-dave-synthetic", DisplayName: "Membership Go Dave Synthetic", Role: 4, State: 2},
		{TenantID: membershipAlphaTenant, ActorType: membershipActorType, ActorID: "actor-membership-go-concurrent-synthetic", DisplayName: "Membership Go Concurrent Synthetic", Role: 4, State: 2},
		{TenantID: membershipAlphaTenant, ActorType: membershipActorType, ActorID: "actor-membership-go-invited-synthetic", DisplayName: "Membership Go Invited Synthetic", Role: 4, State: 1},
		{TenantID: membershipAlphaTenant, ActorType: membershipActorType, ActorID: "actor-membership-go-deactivated-synthetic", DisplayName: "Membership Go Deactivated Synthetic", Role: 4, State: 3},
		{TenantID: membershipBetaTenant, ActorType: membershipActorType, ActorID: "actor-membership-go-alice-synthetic", DisplayName: "Membership Go Beta Alice Synthetic", Role: 4, State: 2},
		{TenantID: membershipBetaTenant, ActorType: membershipActorType, ActorID: "actor-membership-go-beta-only-synthetic", DisplayName: "Membership Go Beta Only Synthetic", Role: 4, State: 2},
	} {
		if _, err := queries.CreateMember(ctx, member); err != nil {
			t.Fatal("create synthetic Member for Channel Membership test failed")
		}
	}

	for _, space := range []CreateSpaceParams{
		{TenantID: membershipAlphaTenant, SpaceID: "space-membership-go-shared-synthetic", DisplayName: "Membership Go Alpha Space Synthetic", Discoverable: true},
		{TenantID: membershipBetaTenant, SpaceID: "space-membership-go-shared-synthetic", DisplayName: "Membership Go Beta Space Synthetic", Discoverable: false},
	} {
		if _, err := queries.CreateSpace(ctx, space); err != nil {
			t.Fatal("create synthetic Space for Channel Membership test failed")
		}
	}

	for _, channel := range []CreateChannelParams{
		{TenantID: membershipAlphaTenant, ChannelID: membershipChannelID, SpaceID: "space-membership-go-shared-synthetic", Name: "Membership Go Alpha Channel Synthetic", Visibility: 1, State: 1, E2eeGroupID: "group-membership-go-alpha-synthetic"},
		{TenantID: membershipBetaTenant, ChannelID: membershipChannelID, SpaceID: "space-membership-go-shared-synthetic", Name: "Membership Go Beta Channel Synthetic", Visibility: 2, State: 1, E2eeGroupID: "group-membership-go-beta-synthetic"},
		{TenantID: membershipBetaTenant, ChannelID: "channel-membership-go-beta-only-synthetic", SpaceID: "space-membership-go-shared-synthetic", Name: "Membership Go Beta Only Channel Synthetic", Visibility: 2, State: 1, E2eeGroupID: "group-membership-go-beta-only-synthetic"},
	} {
		if _, err := queries.CreateChannel(ctx, channel); err != nil {
			t.Fatal("create synthetic Channel for Channel Membership test failed")
		}
	}
}

func testChannelMembershipGeneratedQueries(
	t *testing.T,
	ctx context.Context,
	testDatabase *pgx.Conn,
	queries *Queries,
) {
	t.Helper()
	aliceKey := GetActiveChannelMembershipParams{
		TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
		ActorType: membershipActorType, ActorID: "actor-membership-go-alice-synthetic",
	}
	created, err := queries.CreateActiveChannelMembership(ctx, CreateActiveChannelMembershipParams{
		TenantID: aliceKey.TenantID, ChannelID: aliceKey.ChannelID,
		ActorType: aliceKey.ActorType, ActorID: aliceKey.ActorID, Role: 1,
	})
	if err != nil {
		t.Fatal("typed active Channel Membership create failed")
	}
	if created.IntervalID <= 0 || created.TenantID != aliceKey.TenantID ||
		created.ChannelID != aliceKey.ChannelID || created.ActorType != aliceKey.ActorType ||
		created.ActorID != aliceKey.ActorID || created.Role != 1 ||
		!created.JoinedAt.Valid || created.LeftAt.Valid {
		t.Fatal("typed active Channel Membership create returned unexpected facts")
	}

	got, err := queries.GetActiveChannelMembership(ctx, aliceKey)
	if err != nil {
		t.Fatal("typed active Channel Membership get failed")
	}
	assertSameChannelMembershipInterval(t, created, got)

	active, err := queries.ListActiveChannelMemberships(ctx, ListActiveChannelMembershipsParams{
		TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
	})
	if err != nil || len(active) != 1 {
		t.Fatal("typed active Channel Membership list was not exact")
	}
	assertSameChannelMembershipInterval(t, created, active[0])

	beta, err := queries.CreateActiveChannelMembership(ctx, CreateActiveChannelMembershipParams{
		TenantID: membershipBetaTenant, ChannelID: membershipChannelID,
		ActorType: membershipActorType, ActorID: aliceKey.ActorID, Role: 4,
	})
	if err != nil || beta.TenantID != membershipBetaTenant {
		t.Fatal("typed same-key cross-Tenant Channel Membership create failed")
	}
	gotAgain, err := queries.GetActiveChannelMembership(ctx, aliceKey)
	if err != nil {
		t.Fatal("typed exact-Tenant Channel Membership get failed")
	}
	assertSameChannelMembershipInterval(t, created, gotAgain)

	for _, invalid := range []CreateActiveChannelMembershipParams{
		{TenantID: membershipAlphaTenant, ChannelID: membershipChannelID, ActorType: membershipActorType, ActorID: "actor-membership-go-bob-synthetic", Role: 0},
		{TenantID: membershipAlphaTenant, ChannelID: membershipChannelID, ActorType: membershipActorType, ActorID: "actor-membership-go-bob-synthetic", Role: 5},
		{TenantID: membershipAlphaTenant, ChannelID: "channel-membership-go-missing-synthetic", ActorType: membershipActorType, ActorID: "actor-membership-go-bob-synthetic", Role: 2},
		{TenantID: membershipAlphaTenant, ChannelID: "channel-membership-go-beta-only-synthetic", ActorType: membershipActorType, ActorID: "actor-membership-go-bob-synthetic", Role: 2},
		{TenantID: membershipAlphaTenant, ChannelID: membershipChannelID, ActorType: membershipActorType, ActorID: "actor-membership-go-missing-synthetic", Role: 2},
		{TenantID: membershipAlphaTenant, ChannelID: membershipChannelID, ActorType: membershipActorType, ActorID: "actor-membership-go-beta-only-synthetic", Role: 2},
		{TenantID: membershipAlphaTenant, ChannelID: membershipChannelID, ActorType: membershipActorType, ActorID: "actor-membership-go-invited-synthetic", Role: 2},
		{TenantID: membershipAlphaTenant, ChannelID: membershipChannelID, ActorType: membershipActorType, ActorID: "actor-membership-go-deactivated-synthetic", Role: 2},
		{TenantID: membershipAlphaTenant, ChannelID: membershipChannelID, ActorType: membershipActorType, ActorID: aliceKey.ActorID, Role: 1},
	} {
		if _, err := queries.CreateActiveChannelMembership(ctx, invalid); err == nil {
			t.Fatal("typed invalid active Channel Membership create unexpectedly succeeded")
		}
	}

	missingKey := GetActiveChannelMembershipParams{
		TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
		ActorType: membershipActorType, ActorID: "actor-membership-go-missing-synthetic",
	}
	if _, err := queries.GetActiveChannelMembership(ctx, missingKey); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed active Channel Membership exact-key miss did not return pgx.ErrNoRows")
	}
	missingHistory, err := queries.ListChannelMembershipHistory(ctx, ListChannelMembershipHistoryParams{
		TenantID: membershipAlphaTenant, ChannelID: "channel-membership-go-missing-synthetic",
	})
	if err != nil || len(missingHistory) != 0 {
		t.Fatal("typed Channel Membership history exact-key miss was not empty")
	}

	ended, err := queries.EndActiveChannelMembership(ctx, EndActiveChannelMembershipParams(aliceKey))
	if err != nil {
		t.Fatal("typed active Channel Membership end failed")
	}
	if !ended.LeftAt.Valid {
		t.Fatal("typed active Channel Membership end did not assign departure time")
	}
	assertSameChannelMembershipIdentity(t, created, ended)
	if _, err := queries.EndActiveChannelMembership(ctx, EndActiveChannelMembershipParams(aliceKey)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Channel Membership double leave did not return pgx.ErrNoRows")
	}
	if _, err := queries.GetActiveChannelMembership(ctx, aliceKey); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed departed Channel Membership remained active")
	}

	rejoined, err := queries.CreateActiveChannelMembership(ctx, CreateActiveChannelMembershipParams{
		TenantID: aliceKey.TenantID, ChannelID: aliceKey.ChannelID,
		ActorType: aliceKey.ActorType, ActorID: aliceKey.ActorID, Role: 3,
	})
	if err != nil {
		t.Fatal("typed Channel Membership rejoin failed")
	}
	if rejoined.IntervalID == created.IntervalID || rejoined.Role != 3 || rejoined.LeftAt.Valid {
		t.Fatal("typed Channel Membership rejoin did not create a new active interval")
	}
	history, err := queries.ListChannelMembershipHistory(ctx, ListChannelMembershipHistoryParams{
		TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
	})
	if err != nil || len(history) != 2 {
		t.Fatal("typed Channel Membership history did not preserve both intervals")
	}
	assertSameChannelMembershipInterval(t, ended, history[0])
	assertSameChannelMembershipInterval(t, rejoined, history[1])

	testChannelMembershipMutationGuards(t, ctx, testDatabase, ended)
	testChannelMembershipFailedTransaction(t, ctx, testDatabase, queries)
	testConcurrentDuplicateChannelMembership(t, ctx, testDatabase)
}

func testChannelMembershipMutationGuards(
	t *testing.T,
	ctx context.Context,
	conn *pgx.Conn,
	departed DomainChannelMembership,
) {
	t.Helper()
	keyArgs := []any{departed.TenantID, departed.IntervalID}
	for description, statement := range map[string]string{
		"interval identity": "UPDATE domain.channel_memberships SET interval_id = interval_id + 1000 WHERE tenant_id = $1 AND interval_id = $2",
		"Tenant identity":   "UPDATE domain.channel_memberships SET tenant_id = 'tenant-membership-go-beta-synthetic' WHERE tenant_id = $1 AND interval_id = $2",
		"Channel identity":  "UPDATE domain.channel_memberships SET channel_id = 'channel-membership-go-other-synthetic' WHERE tenant_id = $1 AND interval_id = $2",
		"actor type":        "UPDATE domain.channel_memberships SET actor_type = 2 WHERE tenant_id = $1 AND interval_id = $2",
		"actor identity":    "UPDATE domain.channel_memberships SET actor_id = 'actor-membership-go-bob-synthetic' WHERE tenant_id = $1 AND interval_id = $2",
		"role":              "UPDATE domain.channel_memberships SET role = 2 WHERE tenant_id = $1 AND interval_id = $2",
		"join time":         "UPDATE domain.channel_memberships SET joined_at = joined_at - INTERVAL '1 second' WHERE tenant_id = $1 AND interval_id = $2",
		"reopen":            "UPDATE domain.channel_memberships SET left_at = NULL WHERE tenant_id = $1 AND interval_id = $2",
		"departure time":    "UPDATE domain.channel_memberships SET left_at = left_at + INTERVAL '1 second' WHERE tenant_id = $1 AND interval_id = $2",
	} {
		if _, err := conn.Exec(ctx, statement, keyArgs...); err == nil {
			t.Fatalf("raw Channel Membership %s mutation unexpectedly succeeded", description)
		}
	}
	if _, err := conn.Exec(ctx,
		"DELETE FROM domain.channel_memberships WHERE tenant_id = $1 AND interval_id = $2",
		keyArgs...,
	); err == nil {
		t.Fatal("raw departed Channel Membership delete unexpectedly succeeded")
	}
}

func testChannelMembershipFailedTransaction(
	t *testing.T,
	ctx context.Context,
	conn *pgx.Conn,
	queries *Queries,
) {
	t.Helper()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal("begin Channel Membership rollback test transaction failed")
	}
	txQueries := queries.WithTx(tx)
	if _, err := txQueries.CreateActiveChannelMembership(ctx, CreateActiveChannelMembershipParams{
		TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
		ActorType: membershipActorType, ActorID: "actor-membership-go-dave-synthetic", Role: 2,
	}); err != nil {
		t.Fatal("create Channel Membership inside rollback test failed")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE domain.channel_memberships SET role = 3
		WHERE tenant_id = $1 AND channel_id = $2 AND actor_type = $3 AND actor_id = $4
	`, membershipAlphaTenant, membershipChannelID, membershipActorType, "actor-membership-go-dave-synthetic"); err == nil {
		t.Fatal("forbidden Channel Membership mutation inside transaction unexpectedly succeeded")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal("rollback failed Channel Membership transaction failed")
	}
	if _, err := queries.GetActiveChannelMembership(ctx, GetActiveChannelMembershipParams{
		TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
		ActorType: membershipActorType, ActorID: "actor-membership-go-dave-synthetic",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("failed Channel Membership transaction left a partial interval")
	}
}

func testConcurrentDuplicateChannelMembership(t *testing.T, ctx context.Context, conn *pgx.Conn) {
	t.Helper()
	connections := make([]*pgx.Conn, 2)
	for index := range connections {
		var err error
		connections[index], err = pgx.ConnectConfig(ctx, conn.Config().Copy())
		if err != nil {
			t.Fatal("open concurrent Channel Membership test connection failed")
		}
		defer func(connection *pgx.Conn) {
			if err := connection.Close(ctx); err != nil {
				t.Error("close concurrent Channel Membership test connection failed")
			}
		}(connections[index])
	}

	start := make(chan struct{})
	results := make(chan error, len(connections))
	var workers sync.WaitGroup
	for _, connection := range connections {
		workers.Add(1)
		go func(connection *pgx.Conn) {
			defer workers.Done()
			<-start
			_, err := New(connection).CreateActiveChannelMembership(ctx, CreateActiveChannelMembershipParams{
				TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
				ActorType: membershipActorType, ActorID: "actor-membership-go-concurrent-synthetic", Role: 2,
			})
			results <- err
		}(connection)
	}
	close(start)
	workers.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent duplicate Channel Membership create got %d successes and %d failures", successes, failures)
	}
	if _, err := New(conn).GetActiveChannelMembership(ctx, GetActiveChannelMembershipParams{
		TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
		ActorType: membershipActorType, ActorID: "actor-membership-go-concurrent-synthetic",
	}); err != nil {
		t.Fatal("concurrent duplicate Channel Membership create did not preserve one active interval")
	}
	active, err := New(conn).ListActiveChannelMemberships(ctx, ListActiveChannelMembershipsParams{
		TenantID: membershipAlphaTenant, ChannelID: membershipChannelID,
	})
	if err != nil || len(active) != 2 ||
		active[0].ActorID != "actor-membership-go-alice-synthetic" ||
		active[1].ActorID != "actor-membership-go-concurrent-synthetic" {
		t.Fatal("typed active Channel Membership list was not complete and deterministic")
	}
}

func assertSameChannelMembershipIdentity(t *testing.T, want, got DomainChannelMembership) {
	t.Helper()
	if got.IntervalID != want.IntervalID || got.TenantID != want.TenantID ||
		got.ChannelID != want.ChannelID || got.ActorType != want.ActorType ||
		got.ActorID != want.ActorID || got.Role != want.Role ||
		!got.JoinedAt.Valid || !want.JoinedAt.Valid || !got.JoinedAt.Time.Equal(want.JoinedAt.Time) {
		t.Fatal("Channel Membership immutable interval facts changed")
	}
}

func assertSameChannelMembershipInterval(t *testing.T, want, got DomainChannelMembership) {
	t.Helper()
	assertSameChannelMembershipIdentity(t, want, got)
	if got.LeftAt.Valid != want.LeftAt.Valid ||
		(got.LeftAt.Valid && !got.LeftAt.Time.Equal(want.LeftAt.Time)) {
		t.Fatal("Channel Membership departure fact changed")
	}
}
