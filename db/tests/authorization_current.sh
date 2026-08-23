#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DB_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

. "$SCRIPT_DIR/postgres_harness.sh"

postgres_test_start authorization_current

lock_holder_pid=
lock_holder_backend_pid=
authorization_current_cleanup() {
  case "$lock_holder_backend_pid" in
    '' | *[!0-9]*) ;;
    *)
      "$PSQL" --no-psqlrc --quiet --tuples-only --no-align --dbname="$PGDATABASE" \
        --command="SELECT pg_terminate_backend($lock_holder_backend_pid)" >/dev/null 2>&1 || true
      ;;
  esac
  if test -n "$lock_holder_pid"; then
    kill "$lock_holder_pid" >/dev/null 2>&1 || true
    wait "$lock_holder_pid" >/dev/null 2>&1 || true
  fi
  postgres_test_cleanup
}
trap authorization_current_cleanup EXIT HUP INT TERM

for migration in \
  000001_core_foundation.up.sql \
  000002_organization.up.sql \
  000003_member.up.sql \
  000004_space.up.sql \
  000005_channel_dm.up.sql \
  000006_channel_membership.up.sql \
  000007_resource_acl.up.sql
do
  psql_test --file="$DB_DIR/migrations/$migration"
done

psql_test <<'SQL'
INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
VALUES
  ('tenant-authz-alpha-synthetic', 'Authorization Alpha Synthetic', 1, 'policy-authz-alpha-v1'),
  ('tenant-authz-beta-synthetic', 'Authorization Beta Synthetic', 1, 'policy-authz-beta-v1');

INSERT INTO domain.members (tenant_id, actor_type, actor_id, display_name, role, state)
VALUES
  ('tenant-authz-alpha-synthetic', 1, 'actor-shared-synthetic', 'Alpha Actor Synthetic', 4, 2),
  ('tenant-authz-alpha-synthetic', 1, 'actor-departed-synthetic', 'Departed Actor Synthetic', 4, 2),
  ('tenant-authz-alpha-synthetic', 1, 'actor-new-synthetic', 'New Actor Synthetic', 4, 2),
  ('tenant-authz-beta-synthetic', 1, 'actor-shared-synthetic', 'Beta Actor Synthetic', 5, 2);

INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
VALUES
  ('tenant-authz-alpha-synthetic', 'space-shared-synthetic', 'Alpha Space Synthetic', TRUE),
  ('tenant-authz-alpha-synthetic', 'space-other-synthetic', 'Alpha Other Space Synthetic', FALSE),
  ('tenant-authz-beta-synthetic', 'space-shared-synthetic', 'Beta Space Synthetic', FALSE);

INSERT INTO domain.channels (
  tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id
) VALUES
  ('tenant-authz-alpha-synthetic', 'channel-shared-synthetic', 'space-shared-synthetic',
    'Alpha Channel Synthetic', 1, 1, 'group-authz-alpha-synthetic'),
  ('tenant-authz-alpha-synthetic', 'channel-other-synthetic', 'space-other-synthetic',
    'Alpha Other Channel Synthetic', 2, 1, 'group-authz-alpha-other-synthetic'),
  ('tenant-authz-beta-synthetic', 'channel-shared-synthetic', 'space-shared-synthetic',
    'Beta Channel Synthetic', 2, 2, 'group-authz-beta-synthetic');

INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
VALUES
  ('tenant-authz-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-shared-synthetic', 3),
  ('tenant-authz-alpha-synthetic', 'channel-shared-synthetic', 1, 'actor-departed-synthetic', 4),
  ('tenant-authz-beta-synthetic', 'channel-shared-synthetic', 1, 'actor-shared-synthetic', 4);

UPDATE domain.channel_memberships
SET left_at = CURRENT_TIMESTAMP
WHERE tenant_id = 'tenant-authz-alpha-synthetic'
  AND channel_id = 'channel-shared-synthetic'
  AND actor_type = 1
  AND actor_id = 'actor-departed-synthetic'
  AND left_at IS NULL;

BEGIN;
INSERT INTO domain.resource_acl_snapshots (
  tenant_id, resource_kind, resource_id, channel_id, default_effect
) VALUES (
  'tenant-authz-alpha-synthetic', 2, 'channel-shared-synthetic',
  'channel-shared-synthetic', 1
)
RETURNING acl_version AS alpha_current_acl_version \gset
UPDATE domain.resource_acl_snapshots
SET entries_sealed = TRUE
WHERE acl_version = :alpha_current_acl_version;
INSERT INTO domain.resource_acl_heads (
  tenant_id, resource_kind, resource_id, channel_id, current_acl_version
) VALUES (
  'tenant-authz-alpha-synthetic', 2, 'channel-shared-synthetic',
  'channel-shared-synthetic', :alpha_current_acl_version
);

INSERT INTO domain.resource_acl_snapshots (
  tenant_id, resource_kind, resource_id, channel_id, default_effect
) VALUES (
  'tenant-authz-alpha-synthetic', 2, 'channel-shared-synthetic',
  'channel-shared-synthetic', 2
)
RETURNING acl_version AS alpha_replacement_acl_version \gset
UPDATE domain.resource_acl_snapshots
SET entries_sealed = TRUE
WHERE acl_version = :alpha_replacement_acl_version;

INSERT INTO domain.resource_acl_snapshots (
  tenant_id, resource_kind, resource_id, channel_id, default_effect
) VALUES (
  'tenant-authz-beta-synthetic', 2, 'channel-shared-synthetic',
  'channel-shared-synthetic', 2
)
RETURNING acl_version AS beta_current_acl_version \gset
UPDATE domain.resource_acl_snapshots
SET entries_sealed = TRUE
WHERE acl_version = :beta_current_acl_version;
INSERT INTO domain.resource_acl_heads (
  tenant_id, resource_kind, resource_id, channel_id, current_acl_version
) VALUES (
  'tenant-authz-beta-synthetic', 2, 'channel-shared-synthetic',
  'channel-shared-synthetic', :beta_current_acl_version
);
COMMIT;
SQL

space_facts=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT tenant_id, space_id
  FROM domain.spaces
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND space_id = 'space-shared-synthetic'
  FOR UPDATE
")
test "$space_facts" = "tenant-authz-alpha-synthetic|space-shared-synthetic" || \
  postgres_test_fail "Space authorization facts were not exact"

channel_facts=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT tenant_id, channel_id, visibility, state
  FROM domain.channels
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
  FOR UPDATE
")
test "$channel_facts" = "tenant-authz-alpha-synthetic|channel-shared-synthetic|1|1" || \
  postgres_test_fail "Channel authorization facts were not exact"

organization_facts=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT tenant_id, state, policy_version
  FROM domain.organizations
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
  FOR SHARE
")
test "$organization_facts" = "tenant-authz-alpha-synthetic|1|policy-authz-alpha-v1" || \
  postgres_test_fail "Organization authorization facts were not exact"

member_facts=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT tenant_id, actor_type, actor_id, role, state
  FROM domain.members
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-shared-synthetic'
  FOR SHARE
")
test "$member_facts" = "tenant-authz-alpha-synthetic|1|actor-shared-synthetic|4|2" || \
  postgres_test_fail "Member authorization facts were not exact"

membership_facts=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT interval_id, tenant_id, channel_id, actor_type, actor_id, role
  FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-shared-synthetic'
    AND left_at IS NULL
  FOR SHARE
")
case "$membership_facts" in
  *'|tenant-authz-alpha-synthetic|channel-shared-synthetic|1|actor-shared-synthetic|3') ;;
  *) postgres_test_fail "active Channel Membership authorization facts were not exact" ;;
esac

departed_count=$(psql_test --tuples-only --no-align --command="
  SELECT count(*)
  FROM domain.channel_memberships
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-departed-synthetic'
    AND left_at IS NULL
")
test "$departed_count" = "0" || postgres_test_fail "departed Membership history appeared current"

transaction_isolation=$(psql_test --tuples-only --no-align --command="
  SELECT current_setting('transaction_isolation')::text
")
test "$transaction_isolation" = "read committed" || \
  postgres_test_fail "PostgreSQL test transaction isolation was not read committed"

current_acl_head=$(psql_test --tuples-only --no-align --field-separator='|' --command="
  SELECT tenant_id, resource_kind, resource_id, current_acl_version
  FROM domain.resource_acl_heads
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND resource_kind = 2
    AND resource_id = 'channel-shared-synthetic'
  FOR SHARE
")
case "$current_acl_head" in
  'tenant-authz-alpha-synthetic|2|channel-shared-synthetic|'[0-9]*) ;;
  *) postgres_test_fail "current ACL head authorization fact was not exact" ;;
esac

alpha_acl_version=${current_acl_head##*|}
beta_acl_version=$(psql_test --tuples-only --no-align --command="
  SELECT current_acl_version
  FROM domain.resource_acl_heads
  WHERE tenant_id = 'tenant-authz-beta-synthetic'
    AND resource_kind = 2
    AND resource_id = 'channel-shared-synthetic'
  FOR SHARE
")
test "$alpha_acl_version" != "$beta_acl_version" || \
  postgres_test_fail "current ACL head query crossed exact Tenant scope"

test -f "$DB_DIR/queries/core/authorization_facts.sql" || \
  postgres_test_fail "missing current authorization fact queries"

for query_name in \
  LockAuthorizationSpace \
  LockAuthorizationChannel \
  LockAuthorizationOrganization \
  LockAuthorizationMember \
  LockActiveAuthorizationChannelMembership \
  LockCurrentAuthorizationACL \
  GetAuthorizationTransactionIsolation
do
  grep -Fq -- "-- name: $query_name :one" "$DB_DIR/queries/core/authorization_facts.sql" || \
    postgres_test_fail "missing $query_name query"
done

# One caller-owned transaction holds every fact lock in the documented order:
# Organization, Member, exact Resource parent, current Membership, then exact
# current ACL head.
lock_holder_application="threadline_authz_holder_$$"
PGAPPNAME="$lock_holder_application" \
  "$PSQL" --no-psqlrc --quiet --set=ON_ERROR_STOP=1 --dbname="$test_db" --command="
    BEGIN;
    SELECT tenant_id
    FROM domain.organizations
    WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    FOR SHARE;
    SELECT actor_id
    FROM domain.members
    WHERE tenant_id = 'tenant-authz-alpha-synthetic'
      AND actor_type = 1
      AND actor_id = 'actor-shared-synthetic'
    FOR SHARE;
    SELECT space_id
    FROM domain.spaces
    WHERE tenant_id = 'tenant-authz-alpha-synthetic'
      AND space_id = 'space-shared-synthetic'
    FOR UPDATE;
    SELECT channel_id
    FROM domain.channels
    WHERE tenant_id = 'tenant-authz-alpha-synthetic'
      AND channel_id = 'channel-shared-synthetic'
    FOR UPDATE;
    SELECT interval_id
    FROM domain.channel_memberships
    WHERE tenant_id = 'tenant-authz-alpha-synthetic'
      AND channel_id = 'channel-shared-synthetic'
      AND actor_type = 1
      AND actor_id = 'actor-shared-synthetic'
      AND left_at IS NULL
    FOR SHARE;
    SELECT current_acl_version
    FROM domain.resource_acl_heads
    WHERE tenant_id = 'tenant-authz-alpha-synthetic'
      AND resource_kind = 2
      AND resource_id = 'channel-shared-synthetic'
    FOR SHARE;
    SELECT pg_sleep(15);
    ROLLBACK;
  " >"$temp_dir/lock-holder.out" 2>"$temp_dir/lock-holder.err" &
lock_holder_pid=$!

lock_holder_backend_pid=
lock_attempt=0
while test "$lock_attempt" -lt 100
do
  lock_holder_backend_pid=$(
    "$PSQL" --no-psqlrc --tuples-only --no-align --dbname="$PGDATABASE" --command="
    SELECT pid
    FROM pg_stat_activity
    WHERE datname = '$test_db'
      AND application_name = '$lock_holder_application'
      AND state = 'active'
      AND wait_event = 'PgSleep'
    LIMIT 1
  ")
  case "$lock_holder_backend_pid" in
    *[!0-9] | '') ;;
    *) break ;;
  esac
  lock_attempt=$((lock_attempt + 1))
  sleep 0.05
done
case "$lock_holder_backend_pid" in
  *[!0-9] | '') postgres_test_fail "authorization lock holder did not become ready" ;;
esac

expect_sql_lock_timeout "Space mutation during authorization" "
  UPDATE domain.spaces
  SET discoverable = NOT discoverable
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND space_id = 'space-shared-synthetic';
"
expect_sql_lock_timeout "Channel state mutation during authorization" "
  UPDATE domain.channels
  SET state = 2
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic';
"
expect_sql_lock_timeout "Channel ACL replacement during authorization" "
  SELECT channel_id
  FROM domain.channels
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
  FOR NO KEY UPDATE;
"
expect_sql_lock_timeout "current ACL head update during authorization" "
  UPDATE domain.resource_acl_heads
  SET current_acl_version = (
    SELECT max(acl_version)
    FROM domain.resource_acl_snapshots
    WHERE tenant_id = 'tenant-authz-alpha-synthetic'
      AND resource_kind = 2
      AND resource_id = 'channel-shared-synthetic'
  )
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND resource_kind = 2
    AND resource_id = 'channel-shared-synthetic';
"
expect_sql_lock_timeout "Organization freeze during authorization" "
  UPDATE domain.organizations
  SET state = 2
  WHERE tenant_id = 'tenant-authz-alpha-synthetic';
"
expect_sql_lock_timeout "Member role change during authorization" "
  UPDATE domain.members
  SET role = 2, state = 3
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-shared-synthetic';
"
expect_sql_lock_timeout "Membership departure during authorization" "
  UPDATE domain.channel_memberships
  SET left_at = CURRENT_TIMESTAMP
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND channel_id = 'channel-shared-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-shared-synthetic'
    AND left_at IS NULL;
"
expect_sql_lock_timeout "first Membership insertion during authorization" "
  INSERT INTO domain.channel_memberships (tenant_id, channel_id, actor_type, actor_id, role)
  VALUES (
    'tenant-authz-alpha-synthetic', 'channel-shared-synthetic',
    1, 'actor-new-synthetic', 4
  );
"

unrelated_update=$(psql_test --tuples-only --no-align --command="
  UPDATE domain.channels
  SET state = 3
  WHERE tenant_id = 'tenant-authz-beta-synthetic'
    AND channel_id = 'channel-shared-synthetic'
  RETURNING state
")
test "$unrelated_update" = "3" || postgres_test_fail "different Tenant/resource was globally serialized"

same_tenant_other_resource=$(psql_test --tuples-only --no-align --command="
  UPDATE domain.channels
  SET state = 2
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND channel_id = 'channel-other-synthetic'
  RETURNING state
")
test "$same_tenant_other_resource" = "2" || \
  postgres_test_fail "different Resource in one Tenant was globally serialized"

same_tenant_other_actor=$(psql_test --tuples-only --no-align --command="
  UPDATE domain.members
  SET role = 5
  WHERE tenant_id = 'tenant-authz-alpha-synthetic'
    AND actor_type = 1
    AND actor_id = 'actor-new-synthetic'
  RETURNING role
")
test "$same_tenant_other_actor" = "5" || \
  postgres_test_fail "different Actor in one Tenant was globally serialized"

"$PSQL" --no-psqlrc --quiet --tuples-only --no-align --dbname="$PGDATABASE" \
  --command="SELECT pg_terminate_backend($lock_holder_backend_pid)" >/dev/null
wait "$lock_holder_pid" >/dev/null 2>&1 || true
lock_holder_pid=
lock_holder_backend_pid=

remaining_sessions=1
session_attempt=0
while test "$session_attempt" -lt 100
do
  remaining_sessions=$(
    "$PSQL" --no-psqlrc --tuples-only --no-align --dbname="$PGDATABASE" --command="
      SELECT count(*) FROM pg_stat_activity WHERE datname = '$test_db'
    "
  )
  test "$remaining_sessions" = "0" && break
  session_attempt=$((session_attempt + 1))
  sleep 0.05
done
test "$remaining_sessions" = "0" || postgres_test_fail "authorization lock holder did not disconnect"

postgres_test_finish "current authorization facts passed"
