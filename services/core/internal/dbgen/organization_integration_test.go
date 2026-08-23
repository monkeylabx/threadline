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

const organizationTestDSNEnvironment = "THREADLINE_TEST_POSTGRES_DSN"

func TestOrganizationPersistenceIntegration(t *testing.T) {
	dsn := os.Getenv(organizationTestDSNEnvironment)
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
		t.Fatal("PostgreSQL 16.4 is required for the Organization integration test")
	}

	databaseName := "threadline_organization_go_test_" + strconv.Itoa(os.Getpid()) + "_" +
		strconv.FormatInt(time.Now().UnixNano(), 10)
	quotedDatabaseName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedDatabaseName); err != nil {
		t.Fatal("create disposable Organization test database failed")
	}

	testConfig := adminConfig.Copy()
	testConfig.Database = databaseName
	var testDatabase *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if testDatabase != nil {
			if err := testDatabase.Close(cleanupCtx); err != nil {
				t.Error("close disposable Organization test database failed")
			}
		}
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedDatabaseName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable Organization test database failed")
		}
	})
	testDatabase, err = pgx.ConnectConfig(ctx, testConfig)
	if err != nil {
		t.Fatal("connect to disposable Organization test database failed")
	}

	applyOrganizationTestMigrations(t, ctx, testDatabase)
	queries := New(testDatabase)

	created, err := queries.CreateOrganization(ctx, CreateOrganizationParams{
		TenantID:      "tenant-go-alpha-synthetic",
		DisplayName:   "Go Alpha Synthetic",
		State:         1,
		PolicyVersion: "policy-go-alpha-v1",
	})
	if err != nil {
		t.Fatal("typed Organization create failed")
	}
	if created.TenantID != "tenant-go-alpha-synthetic" ||
		created.DisplayName != "Go Alpha Synthetic" ||
		created.State != 1 ||
		created.PolicyVersion != "policy-go-alpha-v1" ||
		!created.CreatedAt.Valid {
		t.Fatal("typed Organization create returned unexpected fields")
	}

	got, err := queries.GetOrganization(ctx, "tenant-go-alpha-synthetic")
	if err != nil {
		t.Fatal("typed Organization get failed")
	}
	assertSameOrganizationIdentity(t, created, got)
	if got.State != created.State || got.PolicyVersion != created.PolicyVersion {
		t.Fatal("typed Organization get did not preserve lifecycle state and policy version")
	}

	if _, err := queries.GetOrganization(ctx, "tenant-go-beta-synthetic"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Organization exact-key miss did not return pgx.ErrNoRows")
	}
	if _, err := queries.UpdateOrganizationStatePolicy(ctx, UpdateOrganizationStatePolicyParams{
		TenantID:      "tenant-go-beta-synthetic",
		State:         2,
		PolicyVersion: "policy-go-beta-v2",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Organization exact-key update miss did not return pgx.ErrNoRows")
	}

	updated, err := queries.UpdateOrganizationStatePolicy(ctx, UpdateOrganizationStatePolicyParams{
		TenantID:      "tenant-go-alpha-synthetic",
		State:         2,
		PolicyVersion: "policy-go-alpha-v2",
	})
	if err != nil {
		t.Fatal("typed Organization state-policy update failed")
	}
	assertSameOrganizationIdentity(t, created, updated)
	if updated.State != 2 || updated.PolicyVersion != "policy-go-alpha-v2" {
		t.Fatal("typed Organization state-policy update returned unexpected mutable fields")
	}

	if _, err := queries.CreateOrganization(ctx, CreateOrganizationParams{
		TenantID:      "tenant-go-alpha-synthetic",
		DisplayName:   "Go Duplicate Synthetic",
		State:         1,
		PolicyVersion: "policy-go-duplicate-v1",
	}); err == nil {
		t.Fatal("typed duplicate Organization create unexpectedly succeeded")
	}
	if _, err := queries.CreateOrganization(ctx, CreateOrganizationParams{
		TenantID:      "tenant-go-invalid-synthetic",
		DisplayName:   "Go Invalid Synthetic",
		State:         0,
		PolicyVersion: "policy-go-invalid-v1",
	}); err == nil {
		t.Fatal("typed invalid-state Organization create unexpectedly succeeded")
	}
}

func applyOrganizationTestMigrations(t *testing.T, ctx context.Context, conn *pgx.Conn) {
	t.Helper()
	for _, name := range []string{
		"000001_core_foundation.up.sql",
		"000002_organization.up.sql",
	} {
		path := filepath.Join("..", "..", "..", "..", "db", "migrations", name)
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixed Organization test migration %s failed", name)
		}
		if _, err := conn.PgConn().Exec(ctx, string(migration)).ReadAll(); err != nil {
			t.Fatalf("apply fixed Organization test migration %s failed", name)
		}
	}
}

func assertSameOrganizationIdentity(t *testing.T, want, got DomainOrganization) {
	t.Helper()
	if got.TenantID != want.TenantID || got.DisplayName != want.DisplayName ||
		got.CreatedAt.Valid != want.CreatedAt.Valid ||
		!got.CreatedAt.Time.Equal(want.CreatedAt.Time) {
		t.Fatal("Organization identity or creation time changed")
	}
}
