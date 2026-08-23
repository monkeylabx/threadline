package dbgen

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const postgresTestDSNEnvironment = "THREADLINE_TEST_POSTGRES_DSN"

func openPostgresIntegrationTest(t *testing.T, suite string) (context.Context, *pgx.Conn) {
	t.Helper()
	if !knownPostgresTestSuite(suite) {
		t.Fatal("refusing unexpected PostgreSQL integration-test suite")
	}

	dsn := os.Getenv(postgresTestDSNEnvironment)
	if dsn == "" {
		t.Skip("set THREADLINE_TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

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
	if !supportedPostgresTestVersion(serverVersion) {
		t.Fatal("PostgreSQL 16.4 is required for integration tests")
	}

	databaseName := "threadline_" + suite + "_go_test_" + strconv.Itoa(os.Getpid()) + "_" +
		strconv.FormatInt(time.Now().UnixNano(), 10)
	if !safePostgresTestDatabaseName(databaseName, suite) {
		t.Fatal("refusing unsafe disposable PostgreSQL database name")
	}
	quotedDatabaseName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedDatabaseName); err != nil {
		t.Fatal("create disposable PostgreSQL test database failed")
	}

	testConfig := adminConfig.Copy()
	testConfig.Database = databaseName
	var testDatabase *pgx.Conn
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if testDatabase != nil {
			if err := testDatabase.Close(cleanupCtx); err != nil {
				t.Error("close disposable PostgreSQL test database failed")
			}
		}
		if !safePostgresTestDatabaseName(databaseName, suite) {
			t.Error("refusing to drop unexpected PostgreSQL test database")
			return
		}
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedDatabaseName+" WITH (FORCE)"); err != nil {
			t.Error("drop disposable PostgreSQL test database failed")
		}
	})

	testDatabase, err = pgx.ConnectConfig(ctx, testConfig)
	if err != nil {
		t.Fatal("connect to disposable PostgreSQL test database failed")
	}
	return ctx, testDatabase
}

func applyPostgresTestMigrations(
	t *testing.T,
	ctx context.Context,
	conn *pgx.Conn,
	names ...string,
) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join("..", "..", "..", "..", "db", "migrations", name)
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixed PostgreSQL test migration %s failed", name)
		}
		if _, err := conn.PgConn().Exec(ctx, string(migration)).ReadAll(); err != nil {
			t.Fatalf("apply fixed PostgreSQL test migration %s failed", name)
		}
	}
}

func knownPostgresTestSuite(suite string) bool {
	switch suite {
	case "organization", "member", "space", "channel_dm", "channel_membership":
		return true
	default:
		return false
	}
}

func supportedPostgresTestVersion(version string) bool {
	return version == "16.4" || strings.HasPrefix(version, "16.4.")
}

func safePostgresTestDatabaseName(databaseName, suite string) bool {
	if !knownPostgresTestSuite(suite) {
		return false
	}
	prefix := "threadline_" + suite + "_go_test_"
	if !strings.HasPrefix(databaseName, prefix) {
		return false
	}
	pid, timestamp, found := strings.Cut(strings.TrimPrefix(databaseName, prefix), "_")
	return found && decimalDigitsOnly(pid) && decimalDigitsOnly(timestamp)
}

func decimalDigitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func TestPostgresIntegrationHarnessGuards(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"16.4", "16.4.1", "16.4.12"} {
		if !supportedPostgresTestVersion(version) {
			t.Fatalf("expected PostgreSQL version %s to be supported", version)
		}
	}
	for _, version := range []string{"", "16.3", "16.40", "17.4"} {
		if supportedPostgresTestVersion(version) {
			t.Fatalf("expected PostgreSQL version %s to be rejected", version)
		}
	}

	if !safePostgresTestDatabaseName("threadline_space_go_test_123_456", "space") {
		t.Fatal("expected generated Space test database name to pass the deletion guard")
	}
	if !safePostgresTestDatabaseName("threadline_channel_dm_go_test_123_456", "channel_dm") {
		t.Fatal("expected generated Channel/DM test database name to pass the deletion guard")
	}
	if !safePostgresTestDatabaseName("threadline_channel_membership_go_test_123_456", "channel_membership") {
		t.Fatal("expected generated Channel Membership test database name to pass the deletion guard")
	}
	for _, testCase := range []struct {
		name  string
		suite string
	}{
		{name: "threadline_space_go_test_123_456", suite: "unknown"},
		{name: "threadline_member_go_test_123_456", suite: "space"},
		{name: "threadline_space_go_test_", suite: "space"},
		{name: "threadline_space_go_test_123", suite: "space"},
		{name: "threadline_space_go_test_123_456_extra", suite: "space"},
		{name: "threadline_space_go_test_123_secret", suite: "space"},
	} {
		if safePostgresTestDatabaseName(testCase.name, testCase.suite) {
			t.Fatalf("expected PostgreSQL test database deletion guard to reject %s", testCase.name)
		}
	}
}
