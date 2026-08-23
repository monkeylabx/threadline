package dbgen

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

const (
	conversationAlphaTenant = "tenant-conversation-go-alpha-synthetic"
	conversationBetaTenant  = "tenant-conversation-go-beta-synthetic"
)

func TestChannelDirectMessagePersistenceIntegration(t *testing.T) {
	ctx, testDatabase := openPostgresIntegrationTest(t, "channel_dm")
	applyPostgresTestMigrations(t, ctx, testDatabase,
		"000001_core_foundation.up.sql",
		"000002_organization.up.sql",
		"000003_member.up.sql",
		"000004_space.up.sql",
		"000005_channel_dm.up.sql",
	)
	queries := New(testDatabase)

	createConversationFixtures(t, ctx, queries)
	testChannelGeneratedQueries(t, ctx, queries)
	testDirectMessageGeneratedQueries(t, ctx, testDatabase, queries)
}

func createConversationFixtures(t *testing.T, ctx context.Context, queries *Queries) {
	t.Helper()
	for _, organization := range []CreateOrganizationParams{
		{
			TenantID:      conversationAlphaTenant,
			DisplayName:   "Conversation Go Alpha Synthetic",
			State:         1,
			PolicyVersion: "policy-conversation-go-alpha-v1",
		},
		{
			TenantID:      conversationBetaTenant,
			DisplayName:   "Conversation Go Beta Synthetic",
			State:         1,
			PolicyVersion: "policy-conversation-go-beta-v1",
		},
	} {
		if _, err := queries.CreateOrganization(ctx, organization); err != nil {
			t.Fatal("create synthetic Organization for Channel/DM integration test failed")
		}
	}

	for _, member := range []CreateMemberParams{
		{
			TenantID:    conversationAlphaTenant,
			ActorType:   1,
			ActorID:     "actor-conversation-go-alice-synthetic",
			DisplayName: "Conversation Go Alice Synthetic",
			Role:        4,
			State:       2,
		},
		{
			TenantID:    conversationAlphaTenant,
			ActorType:   1,
			ActorID:     "actor-conversation-go-bob-synthetic",
			DisplayName: "Conversation Go Bob Synthetic",
			Role:        4,
			State:       2,
		},
		{
			TenantID:    conversationBetaTenant,
			ActorType:   1,
			ActorID:     "actor-conversation-go-beta-only-synthetic",
			DisplayName: "Conversation Go Beta Only Synthetic",
			Role:        4,
			State:       2,
		},
	} {
		if _, err := queries.CreateMember(ctx, member); err != nil {
			t.Fatal("create synthetic Member for Channel/DM integration test failed")
		}
	}

	for _, space := range []CreateSpaceParams{
		{
			TenantID:     conversationAlphaTenant,
			SpaceID:      "space-conversation-go-shared-synthetic",
			DisplayName:  "Conversation Go Alpha Space Synthetic",
			Discoverable: true,
		},
		{
			TenantID:     conversationBetaTenant,
			SpaceID:      "space-conversation-go-shared-synthetic",
			DisplayName:  "Conversation Go Beta Space Synthetic",
			Discoverable: false,
		},
		{
			TenantID:     conversationBetaTenant,
			SpaceID:      "space-conversation-go-beta-only-synthetic",
			DisplayName:  "Conversation Go Beta Only Space Synthetic",
			Discoverable: false,
		},
	} {
		if _, err := queries.CreateSpace(ctx, space); err != nil {
			t.Fatal("create synthetic Space for Channel/DM integration test failed")
		}
	}
}

func testChannelGeneratedQueries(t *testing.T, ctx context.Context, queries *Queries) {
	t.Helper()
	created, err := queries.CreateChannel(ctx, CreateChannelParams{
		TenantID:    conversationAlphaTenant,
		ChannelID:   "channel-conversation-go-shared-synthetic",
		SpaceID:     "space-conversation-go-shared-synthetic",
		Name:        "Conversation Go Alpha Channel Synthetic",
		Visibility:  1,
		State:       1,
		E2eeGroupID: "group-conversation-go-channel-alpha-synthetic",
	})
	if err != nil {
		t.Fatal("typed Channel create failed")
	}
	if created.TenantID != conversationAlphaTenant ||
		created.ChannelID != "channel-conversation-go-shared-synthetic" ||
		created.SpaceID != "space-conversation-go-shared-synthetic" ||
		created.Name != "Conversation Go Alpha Channel Synthetic" ||
		created.Visibility != 1 || created.State != 1 ||
		created.E2eeGroupID != "group-conversation-go-channel-alpha-synthetic" ||
		!created.CreatedAt.Valid {
		t.Fatal("typed Channel create returned unexpected fields")
	}

	channelKey := GetChannelParams{TenantID: created.TenantID, ChannelID: created.ChannelID}
	got, err := queries.GetChannel(ctx, channelKey)
	if err != nil {
		t.Fatal("typed Channel get failed")
	}
	assertSameChannelImmutableFields(t, created, got)
	if got.Name != created.Name || got.State != created.State {
		t.Fatal("typed Channel get did not preserve mutable lifecycle fields")
	}

	missingKey := GetChannelParams{
		TenantID:  created.TenantID,
		ChannelID: "channel-conversation-go-missing-synthetic",
	}
	if _, err := queries.GetChannel(ctx, missingKey); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Channel exact-key miss did not return pgx.ErrNoRows")
	}
	if _, err := queries.UpdateChannelNameState(ctx, UpdateChannelNameStateParams{
		TenantID: missingKey.TenantID, ChannelID: missingKey.ChannelID,
		Name: "Conversation Go Missing Updated Synthetic", State: 2,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Channel exact-key update miss did not return pgx.ErrNoRows")
	}

	updated, err := queries.UpdateChannelNameState(ctx, UpdateChannelNameStateParams{
		TenantID: created.TenantID, ChannelID: created.ChannelID,
		Name: "Conversation Go Alpha Renamed Synthetic", State: 2,
	})
	if err != nil {
		t.Fatal("typed Channel name-state update failed")
	}
	assertSameChannelImmutableFields(t, created, updated)
	if updated.Name != "Conversation Go Alpha Renamed Synthetic" || updated.State != 2 {
		t.Fatal("typed Channel update returned unexpected mutable fields")
	}

	beta, err := queries.CreateChannel(ctx, CreateChannelParams{
		TenantID:    conversationBetaTenant,
		ChannelID:   created.ChannelID,
		SpaceID:     created.SpaceID,
		Name:        "Conversation Go Beta Channel Synthetic",
		Visibility:  2,
		State:       1,
		E2eeGroupID: "group-conversation-go-channel-beta-synthetic",
	})
	if err != nil {
		t.Fatal("typed same-ID cross-Tenant Channel create failed")
	}
	gotBeta, err := queries.GetChannel(ctx, GetChannelParams{
		TenantID: beta.TenantID, ChannelID: beta.ChannelID,
	})
	if err != nil || gotBeta.Name != beta.Name || gotBeta.State != beta.State {
		t.Fatal("typed exact-Tenant Channel update changed another Tenant")
	}

	for _, invalid := range []CreateChannelParams{
		{
			TenantID: created.TenantID, ChannelID: created.ChannelID,
			SpaceID: created.SpaceID, Name: "Conversation Go Duplicate Channel Synthetic",
			Visibility: 1, State: 1, E2eeGroupID: "group-conversation-go-channel-duplicate-synthetic",
		},
		{
			TenantID: created.TenantID, ChannelID: "",
			SpaceID: created.SpaceID, Name: "Conversation Go Blank Channel Synthetic",
			Visibility: 1, State: 1, E2eeGroupID: "group-conversation-go-channel-blank-synthetic",
		},
		{
			TenantID: created.TenantID, ChannelID: " channel-conversation-go-untrimmed-synthetic ",
			SpaceID: created.SpaceID, Name: "Conversation Go Untrimmed Channel Synthetic",
			Visibility: 1, State: 1, E2eeGroupID: "group-conversation-go-channel-untrimmed-synthetic",
		},
		{
			TenantID: created.TenantID, ChannelID: "channel-conversation-go-blank-name-synthetic",
			SpaceID: created.SpaceID, Name: "",
			Visibility: 1, State: 1, E2eeGroupID: "group-conversation-go-channel-blank-name-synthetic",
		},
		{
			TenantID: created.TenantID, ChannelID: "channel-conversation-go-visibility-synthetic",
			SpaceID: created.SpaceID, Name: "Conversation Go Invalid Visibility Synthetic",
			Visibility: 0, State: 1, E2eeGroupID: "group-conversation-go-channel-visibility-synthetic",
		},
		{
			TenantID: created.TenantID, ChannelID: "channel-conversation-go-state-synthetic",
			SpaceID: created.SpaceID, Name: "Conversation Go Invalid State Synthetic",
			Visibility: 1, State: 0, E2eeGroupID: "group-conversation-go-channel-state-synthetic",
		},
		{
			TenantID: created.TenantID, ChannelID: "channel-conversation-go-missing-space-synthetic",
			SpaceID: "space-conversation-go-missing-synthetic", Name: "Conversation Go Missing Space Synthetic",
			Visibility: 1, State: 1, E2eeGroupID: "group-conversation-go-channel-missing-space-synthetic",
		},
		{
			TenantID: created.TenantID, ChannelID: "channel-conversation-go-cross-tenant-space-synthetic",
			SpaceID: "space-conversation-go-beta-only-synthetic", Name: "Conversation Go Cross Tenant Space Synthetic",
			Visibility: 1, State: 1, E2eeGroupID: "group-conversation-go-channel-cross-tenant-space-synthetic",
		},
		{
			TenantID: created.TenantID, ChannelID: "channel-conversation-go-duplicate-group-synthetic",
			SpaceID: created.SpaceID, Name: "Conversation Go Duplicate Group Synthetic",
			Visibility: 1, State: 1, E2eeGroupID: created.E2eeGroupID,
		},
	} {
		if _, err := queries.CreateChannel(ctx, invalid); err == nil {
			t.Fatal("typed invalid Channel create unexpectedly succeeded")
		}
	}

	for _, invalid := range []UpdateChannelNameStateParams{
		{TenantID: created.TenantID, ChannelID: created.ChannelID, Name: "", State: 2},
		{TenantID: created.TenantID, ChannelID: created.ChannelID, Name: updated.Name, State: 0},
		{TenantID: created.TenantID, ChannelID: created.ChannelID, Name: updated.Name, State: 4},
	} {
		if _, err := queries.UpdateChannelNameState(ctx, invalid); err == nil {
			t.Fatal("typed invalid Channel update unexpectedly succeeded")
		}
	}
	afterRejected, err := queries.GetChannel(ctx, channelKey)
	if err != nil {
		t.Fatal("typed Channel get after rejected operations failed")
	}
	assertSameChannelImmutableFields(t, created, afterRejected)
	if afterRejected.Name != updated.Name || afterRejected.State != updated.State {
		t.Fatal("rejected Channel operations changed stored mutable fields")
	}
}

func testDirectMessageGeneratedQueries(
	t *testing.T,
	ctx context.Context,
	testDatabase *pgx.Conn,
	queries *Queries,
) {
	t.Helper()
	created, err := queries.CreateDirectMessage(ctx, CreateDirectMessageParams{
		TenantID: conversationAlphaTenant, DmID: "dm-conversation-go-shared-synthetic",
		E2eeGroupID: "group-conversation-go-dm-alpha-synthetic",
	})
	if err != nil {
		t.Fatal("typed Direct Message create failed")
	}
	if created.TenantID != conversationAlphaTenant ||
		created.DmID != "dm-conversation-go-shared-synthetic" ||
		created.E2eeGroupID != "group-conversation-go-dm-alpha-synthetic" ||
		!created.CreatedAt.Valid {
		t.Fatal("typed Direct Message create returned unexpected fields")
	}

	dmKey := GetDirectMessageParams{TenantID: created.TenantID, DmID: created.DmID}
	got, err := queries.GetDirectMessage(ctx, dmKey)
	if err != nil {
		t.Fatal("typed Direct Message get failed")
	}
	assertSameDirectMessageImmutableFields(t, created, got)

	participants := []AddDirectMessageParticipantParams{
		{TenantID: created.TenantID, DmID: created.DmID, ActorType: 1, ActorID: "actor-conversation-go-bob-synthetic"},
		{TenantID: created.TenantID, DmID: created.DmID, ActorType: 1, ActorID: "actor-conversation-go-alice-synthetic"},
	}
	for _, participant := range participants {
		added, err := queries.AddDirectMessageParticipant(ctx, participant)
		if err != nil {
			t.Fatal("typed Direct Message participant create failed")
		}
		if added.TenantID != participant.TenantID || added.DmID != participant.DmID ||
			added.ActorType != participant.ActorType || added.ActorID != participant.ActorID {
			t.Fatal("typed Direct Message participant create returned unexpected fields")
		}
	}
	listed, err := queries.ListDirectMessageParticipants(ctx, ListDirectMessageParticipantsParams{
		TenantID: created.TenantID, DmID: created.DmID,
	})
	if err != nil {
		t.Fatal("typed Direct Message participant list failed")
	}
	if len(listed) != 2 || listed[0].ActorID != "actor-conversation-go-alice-synthetic" ||
		listed[1].ActorID != "actor-conversation-go-bob-synthetic" {
		t.Fatal("typed Direct Message participant set did not round trip deterministically")
	}

	missingKey := GetDirectMessageParams{
		TenantID: created.TenantID, DmID: "dm-conversation-go-missing-synthetic",
	}
	if _, err := queries.GetDirectMessage(ctx, missingKey); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Direct Message exact-key miss did not return pgx.ErrNoRows")
	}
	missingParticipants, err := queries.ListDirectMessageParticipants(ctx, ListDirectMessageParticipantsParams{
		TenantID: missingKey.TenantID, DmID: missingKey.DmID,
	})
	if err != nil || len(missingParticipants) != 0 {
		t.Fatal("typed Direct Message exact-key participant miss did not return an empty set")
	}

	beta, err := queries.CreateDirectMessage(ctx, CreateDirectMessageParams{
		TenantID: conversationBetaTenant, DmID: created.DmID,
		E2eeGroupID: "group-conversation-go-dm-beta-synthetic",
	})
	if err != nil {
		t.Fatal("typed same-ID cross-Tenant Direct Message create failed")
	}
	if _, err := queries.AddDirectMessageParticipant(ctx, AddDirectMessageParticipantParams{
		TenantID: beta.TenantID, DmID: beta.DmID, ActorType: 1,
		ActorID: "actor-conversation-go-beta-only-synthetic",
	}); err != nil {
		t.Fatal("typed cross-Tenant Direct Message participant create failed")
	}
	betaParticipants, err := queries.ListDirectMessageParticipants(ctx, ListDirectMessageParticipantsParams{
		TenantID: beta.TenantID, DmID: beta.DmID,
	})
	if err != nil || len(betaParticipants) != 1 ||
		betaParticipants[0].ActorID != "actor-conversation-go-beta-only-synthetic" {
		t.Fatal("typed Direct Message participant list crossed Tenant scope")
	}

	for _, invalid := range []CreateDirectMessageParams{
		{TenantID: created.TenantID, DmID: created.DmID, E2eeGroupID: "group-conversation-go-dm-duplicate-synthetic"},
		{TenantID: created.TenantID, DmID: "", E2eeGroupID: "group-conversation-go-dm-blank-synthetic"},
		{TenantID: created.TenantID, DmID: " dm-conversation-go-untrimmed-synthetic ", E2eeGroupID: "group-conversation-go-dm-untrimmed-synthetic"},
		{TenantID: "tenant-conversation-go-missing-synthetic", DmID: "dm-conversation-go-missing-tenant-synthetic", E2eeGroupID: "group-conversation-go-dm-missing-tenant-synthetic"},
		{TenantID: created.TenantID, DmID: "dm-conversation-go-duplicate-group-synthetic", E2eeGroupID: created.E2eeGroupID},
	} {
		if _, err := queries.CreateDirectMessage(ctx, invalid); err == nil {
			t.Fatal("typed invalid Direct Message create unexpectedly succeeded")
		}
	}

	for _, invalid := range []AddDirectMessageParticipantParams{
		participants[0],
		{TenantID: created.TenantID, DmID: "dm-conversation-go-missing-synthetic", ActorType: 1, ActorID: "actor-conversation-go-alice-synthetic"},
		{TenantID: created.TenantID, DmID: created.DmID, ActorType: 1, ActorID: "actor-conversation-go-missing-synthetic"},
		{TenantID: created.TenantID, DmID: created.DmID, ActorType: 1, ActorID: "actor-conversation-go-beta-only-synthetic"},
	} {
		if _, err := queries.AddDirectMessageParticipant(ctx, invalid); err == nil {
			t.Fatal("typed invalid Direct Message participant create unexpectedly succeeded")
		}
	}

	tx, err := testDatabase.Begin(ctx)
	if err != nil {
		t.Fatal("begin failed Direct Message transaction fixture failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txQueries := queries.WithTx(tx)
	rollbackDM, err := txQueries.CreateDirectMessage(ctx, CreateDirectMessageParams{
		TenantID: created.TenantID, DmID: "dm-conversation-go-rollback-synthetic",
		E2eeGroupID: "group-conversation-go-dm-rollback-synthetic",
	})
	if err != nil {
		t.Fatal("create Direct Message rollback fixture failed")
	}
	if _, err := txQueries.AddDirectMessageParticipant(ctx, AddDirectMessageParticipantParams{
		TenantID: rollbackDM.TenantID, DmID: rollbackDM.DmID, ActorType: 1,
		ActorID: "actor-conversation-go-alice-synthetic",
	}); err != nil {
		t.Fatal("create valid Direct Message rollback participant failed")
	}
	if _, err := txQueries.AddDirectMessageParticipant(ctx, AddDirectMessageParticipantParams{
		TenantID: rollbackDM.TenantID, DmID: rollbackDM.DmID, ActorType: 1,
		ActorID: "actor-conversation-go-missing-synthetic",
	}); err == nil {
		t.Fatal("failed Direct Message transaction accepted a missing Member")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal("rollback failed Direct Message transaction failed")
	}
	if _, err := queries.GetDirectMessage(ctx, GetDirectMessageParams{
		TenantID: rollbackDM.TenantID, DmID: rollbackDM.DmID,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("failed Direct Message transaction left a partial parent row")
	}
	rollbackParticipants, err := queries.ListDirectMessageParticipants(ctx, ListDirectMessageParticipantsParams{
		TenantID: rollbackDM.TenantID, DmID: rollbackDM.DmID,
	})
	if err != nil || len(rollbackParticipants) != 0 {
		t.Fatal("failed Direct Message transaction left partial participant rows")
	}

	afterNegativeCases, err := queries.GetDirectMessage(ctx, dmKey)
	if err != nil {
		t.Fatal("typed Direct Message get after negative cases failed")
	}
	assertSameDirectMessageImmutableFields(t, created, afterNegativeCases)
	afterParticipants, err := queries.ListDirectMessageParticipants(ctx, ListDirectMessageParticipantsParams{
		TenantID: created.TenantID, DmID: created.DmID,
	})
	if err != nil || !sameDirectMessageParticipants(listed, afterParticipants) {
		t.Fatal("rejected Direct Message operations changed the participant set")
	}
}

func assertSameChannelImmutableFields(t *testing.T, want, got DomainChannel) {
	t.Helper()
	if got.TenantID != want.TenantID || got.ChannelID != want.ChannelID ||
		got.SpaceID != want.SpaceID || got.Visibility != want.Visibility ||
		got.E2eeGroupID != want.E2eeGroupID ||
		got.CreatedAt.Valid != want.CreatedAt.Valid ||
		!got.CreatedAt.Time.Equal(want.CreatedAt.Time) {
		t.Fatal("Channel identity, Space, visibility, group binding, or creation time changed")
	}
}

func assertSameDirectMessageImmutableFields(t *testing.T, want, got DomainDirectMessage) {
	t.Helper()
	if got.TenantID != want.TenantID || got.DmID != want.DmID ||
		got.E2eeGroupID != want.E2eeGroupID ||
		got.CreatedAt.Valid != want.CreatedAt.Valid ||
		!got.CreatedAt.Time.Equal(want.CreatedAt.Time) {
		t.Fatal("Direct Message identity, group binding, or creation time changed")
	}
}

func sameDirectMessageParticipants(left, right []DomainDirectMessageParticipant) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
