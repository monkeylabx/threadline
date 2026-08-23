package dbgen

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestMemberPersistenceIntegration(t *testing.T) {
	ctx, testDatabase := openPostgresIntegrationTest(t, "member")
	applyPostgresTestMigrations(t, ctx, testDatabase,
		"000001_core_foundation.up.sql",
		"000002_organization.up.sql",
		"000003_member.up.sql",
	)
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
