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
			TenantID:    conversationAlphaTenant,
			ActorType:   1,
			ActorID:     "actor-conversation-go-carol-synthetic",
			DisplayName: "Conversation Go Carol Synthetic",
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
	participants := []AddDirectMessageParticipantParams{
		{TenantID: conversationAlphaTenant, DmID: "dm-conversation-go-shared-synthetic", ActorType: 1, ActorID: "actor-conversation-go-bob-synthetic"},
		{TenantID: conversationAlphaTenant, DmID: "dm-conversation-go-shared-synthetic", ActorType: 1, ActorID: "actor-conversation-go-alice-synthetic"},
	}
	created := createFinalizedDirectMessage(t, ctx, testDatabase, queries, CreateDirectMessageParams{
		TenantID: conversationAlphaTenant, DmID: "dm-conversation-go-shared-synthetic",
		E2eeGroupID: "group-conversation-go-dm-alpha-synthetic",
	}, participants)
	if created.TenantID != conversationAlphaTenant ||
		created.DmID != "dm-conversation-go-shared-synthetic" ||
		created.E2eeGroupID != "group-conversation-go-dm-alpha-synthetic" ||
		!created.ParticipantsSealed ||
		!created.CreatedAt.Valid {
		t.Fatal("typed Direct Message create returned unexpected fields")
	}

	dmKey := GetDirectMessageParams{TenantID: created.TenantID, DmID: created.DmID}
	got, err := queries.GetDirectMessage(ctx, dmKey)
	if err != nil {
		t.Fatal("typed Direct Message get failed")
	}
	assertSameDirectMessageImmutableFields(t, created, got)

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

	beta := createFinalizedDirectMessage(t, ctx, testDatabase, queries, CreateDirectMessageParams{
		TenantID: conversationBetaTenant, DmID: created.DmID,
		E2eeGroupID: "group-conversation-go-dm-beta-synthetic",
	}, []AddDirectMessageParticipantParams{{
		TenantID: conversationBetaTenant, DmID: created.DmID, ActorType: 1,
		ActorID: "actor-conversation-go-beta-only-synthetic",
	}})
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
		{TenantID: created.TenantID, DmID: "dm-conversation-go-blank-group-synthetic", E2eeGroupID: ""},
	} {
		if _, err := queries.CreateDirectMessage(ctx, invalid); err == nil {
			t.Fatal("typed invalid Direct Message create unexpectedly succeeded")
		}
	}

	if _, err := queries.FinalizeDirectMessageParticipants(ctx, FinalizeDirectMessageParticipantsParams{
		TenantID: missingKey.TenantID, DmID: missingKey.DmID,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Direct Message participant finalize exact-key miss did not return pgx.ErrNoRows")
	}
	if _, err := queries.FinalizeDirectMessageParticipants(ctx, FinalizeDirectMessageParticipantsParams{
		TenantID: created.TenantID, DmID: created.DmID,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Direct Message participant finalize allowed a sealed set to be reopened")
	}

	for _, invalid := range []AddDirectMessageParticipantParams{
		participants[0],
		{TenantID: created.TenantID, DmID: "dm-conversation-go-missing-synthetic", ActorType: 1, ActorID: "actor-conversation-go-alice-synthetic"},
		{TenantID: created.TenantID, DmID: created.DmID, ActorType: 1, ActorID: "actor-conversation-go-missing-synthetic"},
		{TenantID: created.TenantID, DmID: created.DmID, ActorType: 1, ActorID: "actor-conversation-go-beta-only-synthetic"},
		{TenantID: created.TenantID, DmID: created.DmID, ActorType: 1, ActorID: "actor-conversation-go-carol-synthetic"},
	} {
		if _, err := queries.AddDirectMessageParticipant(ctx, invalid); err == nil {
			t.Fatal("typed invalid Direct Message participant create unexpectedly succeeded")
		}
	}

	assertDirectMessageParticipantMutationFails(t, ctx, testDatabase, created)
	assertUnfinalizedDirectMessageRollsBack(t, ctx, testDatabase, queries, created.TenantID)
	assertDuplicateParticipantRollsBack(t, ctx, testDatabase, queries, created.TenantID)

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

func createFinalizedDirectMessage(
	t *testing.T,
	ctx context.Context,
	testDatabase *pgx.Conn,
	queries *Queries,
	params CreateDirectMessageParams,
	participants []AddDirectMessageParticipantParams,
) DomainDirectMessage {
	t.Helper()
	tx, err := testDatabase.Begin(ctx)
	if err != nil {
		t.Fatal("begin Direct Message creation transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txQueries := queries.WithTx(tx)
	created, err := txQueries.CreateDirectMessage(ctx, params)
	if err != nil {
		t.Fatal("typed Direct Message create failed")
	}
	if created.ParticipantsSealed {
		t.Fatal("typed Direct Message create returned a prematurely sealed participant set")
	}
	for _, participant := range participants {
		added, err := txQueries.AddDirectMessageParticipant(ctx, participant)
		if err != nil {
			t.Fatal("typed Direct Message participant create failed")
		}
		if added.TenantID != participant.TenantID || added.DmID != participant.DmID ||
			added.ActorType != participant.ActorType || added.ActorID != participant.ActorID {
			t.Fatal("typed Direct Message participant create returned unexpected fields")
		}
	}
	finalized, err := txQueries.FinalizeDirectMessageParticipants(ctx, FinalizeDirectMessageParticipantsParams{
		TenantID: created.TenantID, DmID: created.DmID,
	})
	if err != nil {
		t.Fatal("typed Direct Message participant finalize failed")
	}
	if !finalized.ParticipantsSealed {
		t.Fatal("typed Direct Message participant finalize did not seal the set")
	}
	assertSameDirectMessageImmutableFields(t, created, finalized)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("commit finalized Direct Message transaction failed")
	}
	return finalized
}

func assertDirectMessageParticipantMutationFails(
	t *testing.T,
	ctx context.Context,
	testDatabase *pgx.Conn,
	directMessage DomainDirectMessage,
) {
	t.Helper()
	for _, statement := range []string{
		`UPDATE domain.direct_message_participants SET actor_id = 'actor-conversation-go-carol-synthetic' WHERE tenant_id = $1 AND dm_id = $2 AND actor_type = 1 AND actor_id = 'actor-conversation-go-bob-synthetic'`,
		`DELETE FROM domain.direct_message_participants WHERE tenant_id = $1 AND dm_id = $2 AND actor_type = 1 AND actor_id = 'actor-conversation-go-bob-synthetic'`,
		`UPDATE domain.direct_messages SET participants_sealed = FALSE WHERE tenant_id = $1 AND dm_id = $2`,
	} {
		if _, err := testDatabase.Exec(ctx, statement, directMessage.TenantID, directMessage.DmID); err == nil {
			t.Fatal("database accepted a forbidden Direct Message participant lifecycle mutation")
		}
	}
}

func assertUnfinalizedDirectMessageRollsBack(
	t *testing.T,
	ctx context.Context,
	testDatabase *pgx.Conn,
	queries *Queries,
	tenantID string,
) {
	t.Helper()
	const dmID = "dm-conversation-go-unfinalized-synthetic"
	tx, err := testDatabase.Begin(ctx)
	if err != nil {
		t.Fatal("begin unfinalized Direct Message transaction failed")
	}
	txQueries := queries.WithTx(tx)
	if _, err := txQueries.CreateDirectMessage(ctx, CreateDirectMessageParams{
		TenantID: tenantID, DmID: dmID,
		E2eeGroupID: "group-conversation-go-dm-unfinalized-synthetic",
	}); err != nil {
		t.Fatal("create unfinalized Direct Message fixture failed")
	}
	if _, err := txQueries.AddDirectMessageParticipant(ctx, AddDirectMessageParticipantParams{
		TenantID: tenantID, DmID: dmID, ActorType: 1,
		ActorID: "actor-conversation-go-alice-synthetic",
	}); err != nil {
		t.Fatal("add unfinalized Direct Message participant fixture failed")
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("unfinalized Direct Message transaction unexpectedly committed")
	}
	if _, err := queries.GetDirectMessage(ctx, GetDirectMessageParams{TenantID: tenantID, DmID: dmID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("unfinalized Direct Message transaction left a partial parent row")
	}
	rows, err := queries.ListDirectMessageParticipants(ctx, ListDirectMessageParticipantsParams{
		TenantID: tenantID, DmID: dmID,
	})
	if err != nil || len(rows) != 0 {
		t.Fatal("unfinalized Direct Message transaction left partial participant rows")
	}
}

func assertDuplicateParticipantRollsBack(
	t *testing.T,
	ctx context.Context,
	testDatabase *pgx.Conn,
	queries *Queries,
	tenantID string,
) {
	t.Helper()
	const dmID = "dm-conversation-go-duplicate-participant-synthetic"
	tx, err := testDatabase.Begin(ctx)
	if err != nil {
		t.Fatal("begin duplicate Direct Message participant transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txQueries := queries.WithTx(tx)
	if _, err := txQueries.CreateDirectMessage(ctx, CreateDirectMessageParams{
		TenantID: tenantID, DmID: dmID,
		E2eeGroupID: "group-conversation-go-dm-duplicate-participant-synthetic",
	}); err != nil {
		t.Fatal("create duplicate Direct Message participant fixture failed")
	}
	participant := AddDirectMessageParticipantParams{
		TenantID: tenantID, DmID: dmID, ActorType: 1,
		ActorID: "actor-conversation-go-alice-synthetic",
	}
	if _, err := txQueries.AddDirectMessageParticipant(ctx, participant); err != nil {
		t.Fatal("add duplicate Direct Message participant fixture failed")
	}
	if _, err := txQueries.AddDirectMessageParticipant(ctx, participant); err == nil {
		t.Fatal("typed duplicate Direct Message participant create unexpectedly succeeded")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal("rollback duplicate Direct Message participant transaction failed")
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
