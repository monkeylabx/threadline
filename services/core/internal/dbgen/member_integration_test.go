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

const memberTestDSNEnvironment = "THREADLINE_TEST_POSTGRES_DSN"

func TestMemberPersistenceIntegration(t *testing.T) {
	dsn := os.Getenv(memberTestDSNEnvironment)
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
		t.Fatal("PostgreSQL 16.4 is required for the Member integration test")
	}

	databaseName := "threadline_member_go_test_" + strconv.Itoa(os.Getpid()) + "_" +
		strconv.FormatInt(time.Now().UnixNano(), 10)
	quotedDatabaseName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedDatabaseName); err != nil {
		t.Fatal("create disposable Member test database failed")
	}

	testConfig := adminConfig.Copy()
	testConfig.Database = databaseName
	var testDatabase *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if testDatabase != nil {
			if err := testDatabase.Close(cleanupCtx); err != nil {
				t.Error("close disposable Member test database failed")
			}
		}
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedDatabaseName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable Member test database failed")
		}
	})
	testDatabase, err = pgx.ConnectConfig(ctx, testConfig)
	if err != nil {
		t.Fatal("connect to disposable Member test database failed")
	}

	applyMemberTestMigrations(t, ctx, testDatabase)
	queries := New(testDatabase)

	if _, err := queries.CreateOrganization(ctx, CreateOrganizationParams{
		TenantID:      "tenant-member-go-alpha-synthetic",
		DisplayName:   "Member Go Alpha Synthetic",
		State:         1,
		PolicyVersion: "policy-member-go-alpha-v1",
	}); err != nil {
		t.Fatal("create synthetic Organization for Member integration test failed")
	}

	orgUnitPath := "engineering/synthetic"
	created, err := queries.CreateMember(ctx, CreateMemberParams{
		TenantID:    "tenant-member-go-alpha-synthetic",
		ActorType:   1,
		ActorID:     "actor-member-go-human-synthetic",
		DisplayName: "Member Go Human Synthetic",
		Role:        4,
		State:       1,
		OrgUnitPath: &orgUnitPath,
	})
	if err != nil {
		t.Fatal("typed Member create failed")
	}
	if created.TenantID != "tenant-member-go-alpha-synthetic" ||
		created.ActorType != 1 ||
		created.ActorID != "actor-member-go-human-synthetic" ||
		created.DisplayName != "Member Go Human Synthetic" ||
		created.Role != 4 ||
		created.State != 1 ||
		created.OrgUnitPath == nil || *created.OrgUnitPath != orgUnitPath ||
		!created.JoinedAt.Valid {
		t.Fatal("typed Member create returned unexpected fields")
	}

	memberKey := GetMemberParams{
		TenantID:  created.TenantID,
		ActorType: created.ActorType,
		ActorID:   created.ActorID,
	}
	got, err := queries.GetMember(ctx, memberKey)
	if err != nil {
		t.Fatal("typed Member get failed")
	}
	assertSameMemberImmutableFields(t, created, got)
	if got.Role != created.Role || got.State != created.State {
		t.Fatal("typed Member get did not preserve Role and MemberState")
	}

	missingKey := GetMemberParams{
		TenantID:  created.TenantID,
		ActorType: 3,
		ActorID:   created.ActorID,
	}
	if _, err := queries.GetMember(ctx, missingKey); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Member exact-key miss did not return pgx.ErrNoRows")
	}
	if _, err := queries.UpdateMemberRoleState(ctx, UpdateMemberRoleStateParams{
		TenantID:  missingKey.TenantID,
		ActorType: missingKey.ActorType,
		ActorID:   missingKey.ActorID,
		Role:      2,
		State:     2,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("typed Member exact-key update miss did not return pgx.ErrNoRows")
	}

	updated, err := queries.UpdateMemberRoleState(ctx, UpdateMemberRoleStateParams{
		TenantID:  created.TenantID,
		ActorType: created.ActorType,
		ActorID:   created.ActorID,
		Role:      2,
		State:     2,
	})
	if err != nil {
		t.Fatal("typed Member role-state update failed")
	}
	assertSameMemberImmutableFields(t, created, updated)
	if updated.Role != 2 || updated.State != 2 {
		t.Fatal("typed Member role-state update returned unexpected mutable fields")
	}
	if _, err := queries.UpdateMemberRoleState(ctx, UpdateMemberRoleStateParams{
		TenantID:  created.TenantID,
		ActorType: created.ActorType,
		ActorID:   created.ActorID,
		Role:      0,
		State:     2,
	}); err == nil {
		t.Fatal("typed Member update accepted invalid Role")
	}
	if _, err := queries.UpdateMemberRoleState(ctx, UpdateMemberRoleStateParams{
		TenantID:  created.TenantID,
		ActorType: created.ActorType,
		ActorID:   created.ActorID,
		Role:      2,
		State:     0,
	}); err == nil {
		t.Fatal("typed Member update accepted invalid MemberState")
	}
	afterRejectedUpdates, err := queries.GetMember(ctx, memberKey)
	if err != nil {
		t.Fatal("typed Member get after rejected updates failed")
	}
	assertSameMemberImmutableFields(t, created, afterRejectedUpdates)
	if afterRejectedUpdates.Role != 2 || afterRejectedUpdates.State != 2 {
		t.Fatal("rejected typed Member updates changed stored Role or MemberState")
	}

	if _, err := queries.CreateMember(ctx, CreateMemberParams{
		TenantID:    created.TenantID,
		ActorType:   created.ActorType,
		ActorID:     created.ActorID,
		DisplayName: "Member Go Duplicate Synthetic",
		Role:        4,
		State:       1,
	}); err == nil {
		t.Fatal("typed duplicate Member create unexpectedly succeeded")
	}
	if _, err := queries.CreateMember(ctx, CreateMemberParams{
		TenantID:    created.TenantID,
		ActorType:   0,
		ActorID:     "actor-member-go-invalid-type-synthetic",
		DisplayName: "Member Go Invalid Actor Synthetic",
		Role:        4,
		State:       1,
	}); err == nil {
		t.Fatal("typed invalid-ActorType Member create unexpectedly succeeded")
	}
	if _, err := queries.CreateMember(ctx, CreateMemberParams{
		TenantID:    created.TenantID,
		ActorType:   2,
		ActorID:     "actor-member-go-invalid-role-synthetic",
		DisplayName: "Member Go Invalid Role Synthetic",
		Role:        0,
		State:       1,
	}); err == nil {
		t.Fatal("typed invalid-Role Member create unexpectedly succeeded")
	}
	if _, err := queries.CreateMember(ctx, CreateMemberParams{
		TenantID:    created.TenantID,
		ActorType:   2,
		ActorID:     "actor-member-go-invalid-state-synthetic",
		DisplayName: "Member Go Invalid State Synthetic",
		Role:        4,
		State:       0,
	}); err == nil {
		t.Fatal("typed invalid-MemberState create unexpectedly succeeded")
	}
	if _, err := queries.CreateMember(ctx, CreateMemberParams{
		TenantID:    "tenant-member-go-missing-synthetic",
		ActorType:   1,
		ActorID:     "actor-member-go-missing-tenant-synthetic",
		DisplayName: "Member Go Missing Tenant Synthetic",
		Role:        4,
		State:       1,
	}); err == nil {
		t.Fatal("typed Member create without Organization unexpectedly succeeded")
	}

	withoutOrgUnit, err := queries.CreateMember(ctx, CreateMemberParams{
		TenantID:    created.TenantID,
		ActorType:   2,
		ActorID:     "actor-member-go-agent-synthetic",
		DisplayName: "Member Go Agent Synthetic",
		Role:        4,
		State:       3,
		OrgUnitPath: nil,
	})
	if err != nil {
		t.Fatal("typed Member create without optional org-unit path failed")
	}
	if withoutOrgUnit.OrgUnitPath != nil {
		t.Fatal("typed Member optional org-unit path did not remain nil")
	}
	gotWithoutOrgUnit, err := queries.GetMember(ctx, GetMemberParams{
		TenantID:  withoutOrgUnit.TenantID,
		ActorType: withoutOrgUnit.ActorType,
		ActorID:   withoutOrgUnit.ActorID,
	})
	if err != nil || gotWithoutOrgUnit.OrgUnitPath != nil {
		t.Fatal("typed Member get did not preserve absent org-unit path")
	}
}

func applyMemberTestMigrations(t *testing.T, ctx context.Context, conn *pgx.Conn) {
	t.Helper()
	for _, name := range []string{
		"000001_core_foundation.up.sql",
		"000002_organization.up.sql",
		"000003_member.up.sql",
	} {
		path := filepath.Join("..", "..", "..", "..", "db", "migrations", name)
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixed Member test migration %s failed", name)
		}
		if _, err := conn.PgConn().Exec(ctx, string(migration)).ReadAll(); err != nil {
			t.Fatalf("apply fixed Member test migration %s failed", name)
		}
	}
}

func assertSameMemberImmutableFields(t *testing.T, want, got DomainMember) {
	t.Helper()
	if got.TenantID != want.TenantID || got.ActorType != want.ActorType ||
		got.ActorID != want.ActorID || got.DisplayName != want.DisplayName ||
		!sameOptionalString(got.OrgUnitPath, want.OrgUnitPath) ||
		got.JoinedAt.Valid != want.JoinedAt.Valid ||
		!got.JoinedAt.Time.Equal(want.JoinedAt.Time) {
		t.Fatal("Member identity, directory fields, or join time changed")
	}
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
