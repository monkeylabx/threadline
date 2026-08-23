package dbgen

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestOrganizationPersistenceIntegration(t *testing.T) {
	ctx, testDatabase := openPostgresIntegrationTest(t, "organization")
	applyPostgresTestMigrations(t, ctx, testDatabase,
		"000001_core_foundation.up.sql",
		"000002_organization.up.sql",
	)
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

func assertSameOrganizationIdentity(t *testing.T, want, got DomainOrganization) {
	t.Helper()
	if got.TenantID != want.TenantID || got.DisplayName != want.DisplayName ||
		got.CreatedAt.Valid != want.CreatedAt.Valid ||
		!got.CreatedAt.Time.Equal(want.CreatedAt.Time) {
		t.Fatal("Organization identity or creation time changed")
	}
}
