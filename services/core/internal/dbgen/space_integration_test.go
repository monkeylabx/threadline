package dbgen

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSpacePersistenceIntegration(t *testing.T) {
	ctx, testDatabase := openPostgresIntegrationTest(t, "space")
	applyPostgresTestMigrations(t, ctx, testDatabase,
		"000001_core_foundation.up.sql",
		"000002_organization.up.sql",
		"000003_member.up.sql",
		"000004_space.up.sql",
	)
	queries := New(testDatabase)

	for _, organization := range []CreateOrganizationParams{
		{
			TenantID:      "tenant-space-go-alpha-synthetic",
			DisplayName:   "Space Go Alpha Synthetic",
			State:         1,
			PolicyVersion: "policy-space-go-alpha-v1",
		},
		{
			TenantID:      "tenant-space-go-beta-synthetic",
			DisplayName:   "Space Go Beta Synthetic",
			State:         1,
			PolicyVersion: "policy-space-go-beta-v1",
		},
	} {
		if _, err := queries.CreateOrganization(ctx, organization); err != nil {
			t.Fatal("create synthetic Organization for Space integration test failed")
		}
	}

	created, err := queries.CreateSpace(ctx, CreateSpaceParams{
		TenantID:     "tenant-space-go-alpha-synthetic",
		SpaceID:      "space-go-shared-synthetic",
		DisplayName:  "Space Go Shared Alpha Synthetic",
		Discoverable: true,
	})
	if err != nil {
		t.Fatal("typed Space create failed")
	}
	if created.TenantID != "tenant-space-go-alpha-synthetic" ||
		created.SpaceID != "space-go-shared-synthetic" ||
		created.DisplayName != "Space Go Shared Alpha Synthetic" ||
		!created.Discoverable ||
		!created.CreatedAt.Valid {
		t.Fatal("typed Space create returned unexpected fields")
	}

	spaceKey := GetSpaceParams{TenantID: created.TenantID, SpaceID: created.SpaceID}
	got, err := queries.GetSpace(ctx, spaceKey)
	if err != nil {
		t.Fatal("typed Space get failed")
	}
	assertSameSpaceImmutableFields(t, created, got)
	if got.DisplayName != created.DisplayName || got.Discoverable != created.Discoverable {
		t.Fatal("typed Space get did not preserve directory metadata")
	}

	missingKey := GetSpaceParams{
		TenantID: created.TenantID,
		SpaceID:  "space-go-missing-synthetic",
	}
	if _, err := queries.GetSpace(ctx, missingKey); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Space exact-key miss did not return pgx.ErrNoRows")
	}
	if _, err := queries.UpdateSpaceDirectoryMetadata(ctx, UpdateSpaceDirectoryMetadataParams{
		TenantID:     missingKey.TenantID,
		SpaceID:      missingKey.SpaceID,
		DisplayName:  "Space Go Missing Updated Synthetic",
		Discoverable: true,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Space exact-key update miss did not return pgx.ErrNoRows")
	}

	updated, err := queries.UpdateSpaceDirectoryMetadata(ctx, UpdateSpaceDirectoryMetadataParams{
		TenantID:     created.TenantID,
		SpaceID:      created.SpaceID,
		DisplayName:  "Space Go Shared Renamed Synthetic",
		Discoverable: false,
	})
	if err != nil {
		t.Fatal("typed Space directory metadata update failed")
	}
	assertSameSpaceImmutableFields(t, created, updated)
	if updated.DisplayName != "Space Go Shared Renamed Synthetic" || updated.Discoverable {
		t.Fatal("typed Space directory metadata update returned unexpected mutable fields")
	}

	beta, err := queries.CreateSpace(ctx, CreateSpaceParams{
		TenantID:     "tenant-space-go-beta-synthetic",
		SpaceID:      created.SpaceID,
		DisplayName:  "Space Go Shared Beta Synthetic",
		Discoverable: true,
	})
	if err != nil {
		t.Fatal("typed same-ID cross-Tenant Space create failed")
	}
	isolatedUpdate, err := queries.UpdateSpaceDirectoryMetadata(ctx, UpdateSpaceDirectoryMetadataParams{
		TenantID:     created.TenantID,
		SpaceID:      created.SpaceID,
		DisplayName:  "Space Go Shared Isolated Synthetic",
		Discoverable: true,
	})
	if err != nil {
		t.Fatal("typed exact-Tenant Space update failed")
	}
	assertSameSpaceImmutableFields(t, created, isolatedUpdate)
	updated = isolatedUpdate

	gotBeta, err := queries.GetSpace(ctx, GetSpaceParams{
		TenantID: beta.TenantID,
		SpaceID:  beta.SpaceID,
	})
	if err != nil {
		t.Fatal("typed cross-Tenant Space get failed")
	}
	if gotBeta.DisplayName != beta.DisplayName || gotBeta.Discoverable != beta.Discoverable {
		t.Fatal("typed exact-Tenant update changed another Space row")
	}

	if _, err := queries.CreateSpace(ctx, CreateSpaceParams{
		TenantID:     created.TenantID,
		SpaceID:      created.SpaceID,
		DisplayName:  "Space Go Duplicate Synthetic",
		Discoverable: false,
	}); err == nil {
		t.Fatal("typed duplicate Space create unexpectedly succeeded")
	}
	if _, err := queries.CreateSpace(ctx, CreateSpaceParams{
		TenantID:     created.TenantID,
		SpaceID:      "",
		DisplayName:  "Space Go Blank Synthetic",
		Discoverable: false,
	}); err == nil {
		t.Fatal("typed blank-ID Space create unexpectedly succeeded")
	}
	if _, err := queries.CreateSpace(ctx, CreateSpaceParams{
		TenantID:     created.TenantID,
		SpaceID:      " space-go-untrimmed-synthetic ",
		DisplayName:  "Space Go Untrimmed Synthetic",
		Discoverable: false,
	}); err == nil {
		t.Fatal("typed untrimmed-ID Space create unexpectedly succeeded")
	}
	if _, err := queries.CreateSpace(ctx, CreateSpaceParams{
		TenantID:     "tenant-space-go-missing-synthetic",
		SpaceID:      "space-go-missing-tenant-synthetic",
		DisplayName:  "Space Go Missing Tenant Synthetic",
		Discoverable: false,
	}); err == nil {
		t.Fatal("typed Space create without Organization unexpectedly succeeded")
	}

	afterNegativeCases, err := queries.GetSpace(ctx, spaceKey)
	if err != nil {
		t.Fatal("typed Space get after negative cases failed")
	}
	assertSameSpaceImmutableFields(t, created, afterNegativeCases)
	if afterNegativeCases.DisplayName != updated.DisplayName ||
		afterNegativeCases.Discoverable != updated.Discoverable {
		t.Fatal("rejected Space operations changed stored directory metadata")
	}
}

func assertSameSpaceImmutableFields(t *testing.T, want, got DomainSpace) {
	t.Helper()
	if got.TenantID != want.TenantID || got.SpaceID != want.SpaceID ||
		got.CreatedAt.Valid != want.CreatedAt.Valid ||
		!got.CreatedAt.Time.Equal(want.CreatedAt.Time) {
		t.Fatal("Space identity or creation time changed")
	}
}
