package dbgen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const spaceTestDSNEnvironment = "THREADLINE_TEST_POSTGRES_DSN"

func TestSpacePersistenceIntegration(t *testing.T) {
	dsn := os.Getenv(spaceTestDSNEnvironment)
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run the PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("parse configured PostgreSQL maintenance DSN failed")
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("connect to configured PostgreSQL maintenance database failed")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := admin.Close(cleanupCtx); err != nil {
			t.Error("close PostgreSQL maintenance connection failed")
		}
	})

	var serverVersion string
	if err := admin.QueryRow(ctx, "SHOW server_version").Scan(&serverVersion); err != nil {
		t.Fatal("read PostgreSQL server version failed")
	}
	if serverVersion != "16.4" && !strings.HasPrefix(serverVersion, "16.4.") {
		t.Fatal("PostgreSQL 16.4 is required for the Space integration test")
	}

	databaseName := "threadline_space_go_test_" + strconv.Itoa(os.Getpid()) + "_" +
		strconv.FormatInt(time.Now().UnixNano(), 10)
	quotedDatabaseName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedDatabaseName); err != nil {
		t.Fatal("create disposable Space test database failed")
	}

	testConfig := adminConfig.Copy()
	testConfig.Database = databaseName
	var testDatabase *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if testDatabase != nil {
			if err := testDatabase.Close(cleanupCtx); err != nil {
				t.Error("close disposable Space test database failed")
			}
		}
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedDatabaseName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable Space test database failed")
		}
	})
	testDatabase, err = pgx.ConnectConfig(ctx, testConfig)
	if err != nil {
		t.Fatal("connect to disposable Space test database failed")
	}

	applySpaceTestMigrations(t, ctx, testDatabase)
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

func applySpaceTestMigrations(t *testing.T, ctx context.Context, conn *pgx.Conn) {
	t.Helper()
	for _, name := range []string{
		"000001_core_foundation.up.sql",
		"000002_organization.up.sql",
		"000003_member.up.sql",
		"000004_space.up.sql",
	} {
		path := filepath.Join("..", "..", "..", "..", "db", "migrations", name)
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixed Space test migration %s failed", name)
		}
		if _, err := conn.PgConn().Exec(ctx, string(migration)).ReadAll(); err != nil {
			t.Fatalf("apply fixed Space test migration %s failed", name)
		}
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
