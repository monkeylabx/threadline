#!/bin/sh
set -eu

DB_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TESTS_DIR="$DB_DIR/tests"

. "$TESTS_DIR/postgres_harness.sh"
postgres_test_start resource_acl

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

sql_scalar() {
  psql_test --tuples-only --no-align --command="$1"
}

expect_entry_failure() {
  label=$1
  entry_values=$2
  pattern=$3
  expect_sql_failure_matching "$label" "
    BEGIN;
    INSERT INTO domain.resource_acl_snapshots (
      tenant_id, resource_kind, resource_id, space_id, default_effect
    ) VALUES (
      'tenant-acl-alpha-synthetic', 1, 'space-acl-beta-synthetic',
      'space-acl-beta-synthetic', 2
    );
    INSERT INTO domain.resource_acl_entries (
      tenant_id, acl_version, entry_ordinal, actor_type, actor_id, action, effect
    )
    SELECT tenant_id, acl_version, $entry_values
    FROM domain.resource_acl_snapshots
    WHERE tenant_id = 'tenant-acl-alpha-synthetic'
      AND resource_kind = 1
      AND resource_id = 'space-acl-beta-synthetic'
      AND NOT entries_sealed;
    COMMIT;
  " "$pattern"
}

psql_test --command="
  INSERT INTO domain.organizations (tenant_id, display_name, state, policy_version)
  VALUES
    ('tenant-acl-alpha-synthetic', 'ACL Alpha Synthetic', 1, 'policy-acl-alpha-v1'),
    ('tenant-acl-beta-synthetic', 'ACL Beta Synthetic', 1, 'policy-acl-beta-v1');

  INSERT INTO domain.members (
    tenant_id, actor_type, actor_id, display_name, role, state
  ) VALUES
    ('tenant-acl-alpha-synthetic', 1, 'human-acl-alpha-synthetic', 'Human ACL Alpha Synthetic', 4, 2),
    ('tenant-acl-alpha-synthetic', 2, 'agent-acl-alpha-synthetic', 'Agent ACL Alpha Synthetic', 4, 2),
    ('tenant-acl-alpha-synthetic', 3, 'service-acl-alpha-synthetic', 'Service ACL Alpha Synthetic', 4, 2),
    ('tenant-acl-beta-synthetic', 1, 'human-acl-beta-synthetic', 'Human ACL Beta Synthetic', 4, 2);

  INSERT INTO domain.spaces (tenant_id, space_id, display_name, discoverable)
  VALUES
    ('tenant-acl-alpha-synthetic', 'space-acl-alpha-synthetic', 'ACL Space Alpha Synthetic', TRUE),
    ('tenant-acl-alpha-synthetic', 'space-acl-beta-synthetic', 'ACL Space Beta Synthetic', FALSE),
    ('tenant-acl-beta-synthetic', 'space-acl-shared-synthetic', 'ACL Space Shared Synthetic', TRUE);

  INSERT INTO domain.channels (
    tenant_id, channel_id, space_id, name, visibility, state, e2ee_group_id
  ) VALUES
    ('tenant-acl-alpha-synthetic', 'channel-acl-alpha-synthetic', 'space-acl-alpha-synthetic',
      'ACL Channel Alpha Synthetic', 1, 1, 'e2ee-acl-alpha-synthetic'),
    ('tenant-acl-alpha-synthetic', 'channel-acl-beta-synthetic', 'space-acl-beta-synthetic',
      'ACL Channel Beta Synthetic', 2, 1, 'e2ee-acl-beta-synthetic'),
    ('tenant-acl-beta-synthetic', 'channel-acl-shared-synthetic', 'space-acl-shared-synthetic',
      'ACL Channel Shared Synthetic', 1, 1, 'e2ee-acl-shared-synthetic');
"

current_count=$(sql_scalar "SELECT count(*) FROM domain.resource_acl_heads")
test "$current_count" = "0" || postgres_test_fail "resource without ACL synthesized a current snapshot"

expect_sql_failure_matching "explicit ACL version" "
  INSERT INTO domain.resource_acl_snapshots (
    acl_version, tenant_id, resource_kind, resource_id, space_id, default_effect, entries_sealed
  ) VALUES (
    7007, 'tenant-acl-alpha-synthetic', 1, 'space-acl-alpha-synthetic',
    'space-acl-alpha-synthetic', 2, TRUE
  )
" 'generated always'

expect_sql_failure_matching "blank ACL version" "
  INSERT INTO domain.resource_acl_snapshots (
    acl_version, tenant_id, resource_kind, resource_id, space_id, default_effect, entries_sealed
  ) VALUES (
    '', 'tenant-acl-alpha-synthetic', 1, 'space-acl-alpha-synthetic',
    'space-acl-alpha-synthetic', 2, TRUE
  )
" 'bigint|generated always'

expect_sql_failure_matching "cross-Tenant Space binding" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-shared-synthetic',
    'space-acl-shared-synthetic', 2, TRUE
  )
" 'resource_acl_snapshots_space_fk'

expect_sql_failure_matching "missing Space binding" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-missing-synthetic',
    'space-acl-missing-synthetic', 2, TRUE
  )
" 'resource_acl_snapshots_space_fk'

expect_sql_failure_matching "missing Channel binding" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, channel_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 2, 'channel-acl-missing-synthetic',
    'channel-acl-missing-synthetic', 2, TRUE
  )
" 'resource_acl_snapshots_channel_fk'

expect_sql_failure_matching "cross-Tenant Channel binding" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, channel_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 2, 'channel-acl-shared-synthetic',
    'channel-acl-shared-synthetic', 2, TRUE
  )
" 'resource_acl_snapshots_channel_fk'

expect_sql_failure_matching "resource with neither typed identifier" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-alpha-synthetic', 2, TRUE
  )
" 'resource_acl_snapshots_exactly_one_typed_resource'

expect_sql_failure_matching "resource with both typed identifiers" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, channel_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-alpha-synthetic',
    'space-acl-alpha-synthetic', 'channel-acl-alpha-synthetic', 2, TRUE
  )
" 'resource_acl_snapshots_exactly_one_typed_resource'

expect_sql_failure_matching "resource kind and identifier mismatch" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, channel_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'channel-acl-alpha-synthetic',
    'channel-acl-alpha-synthetic', 2, TRUE
  )
" 'resource_acl_snapshots_exactly_one_typed_resource'

expect_sql_failure_matching "unknown resource kind" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 3, 'space-acl-alpha-synthetic',
    'space-acl-alpha-synthetic', 2, TRUE
  )
" 'resource_acl_snapshots_(resource_kind_known|exactly_one_typed_resource)'

expect_sql_failure_matching "unknown ACL default effect" "
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, default_effect, entries_sealed
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-alpha-synthetic',
    'space-acl-alpha-synthetic', 3, TRUE
  )
" 'resource_acl_snapshots_default_effect_known'

expect_entry_failure "missing ACL Actor" \
  "1, 1, 'human-acl-missing-synthetic', 1, 1" \
  'resource_acl_entries_member_fk'
expect_entry_failure "cross-Tenant ACL Actor" \
  "1, 1, 'human-acl-beta-synthetic', 1, 1" \
  'resource_acl_entries_member_fk'
expect_entry_failure "blank ACL Actor ID" \
  "1, 1, '', 1, 1" \
  'resource_acl_entries_actor_id_not_blank'
expect_entry_failure "unknown ACL Actor type" \
  "1, 4, 'actor-acl-unknown-synthetic', 1, 1" \
  'resource_acl_entries_actor_type_known|resource_acl_entries_member_fk'
expect_entry_failure "unknown ACL action" \
  "1, 1, 'human-acl-alpha-synthetic', 12, 1" \
  'resource_acl_entries_action_known'
expect_entry_failure "unspecified ACL action" \
  "1, 1, 'human-acl-alpha-synthetic', 0, 1" \
  'resource_acl_entries_action_known'
expect_entry_failure "unknown ACL entry effect" \
  "1, 1, 'human-acl-alpha-synthetic', 1, 3" \
  'resource_acl_entries_effect_known'
expect_entry_failure "unspecified ACL entry effect" \
  "1, 1, 'human-acl-alpha-synthetic', 1, 0" \
  'resource_acl_entries_effect_known'
expect_entry_failure "nonpositive ACL entry ordinal" \
  "0, 1, 'human-acl-alpha-synthetic', 1, 1" \
  'resource_acl_entries_ordinal_positive'

expect_sql_failure_matching "duplicate exact ACL entry" "
  BEGIN;
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, default_effect
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-beta-synthetic',
    'space-acl-beta-synthetic', 2
  );
  INSERT INTO domain.resource_acl_entries (
    tenant_id, acl_version, entry_ordinal, actor_type, actor_id, action, effect
  )
  SELECT tenant_id, acl_version, 1, 1, 'human-acl-alpha-synthetic', 4, 1
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_id = 'space-acl-beta-synthetic'
    AND NOT entries_sealed;
  INSERT INTO domain.resource_acl_entries (
    tenant_id, acl_version, entry_ordinal, actor_type, actor_id, action, effect
  )
  SELECT tenant_id, acl_version, 2, 1, 'human-acl-alpha-synthetic', 4, 1
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_id = 'space-acl-beta-synthetic'
    AND NOT entries_sealed;
  COMMIT;
" 'resource_acl_entries_exact_unique'

snapshot_count=$(sql_scalar "SELECT count(*) FROM domain.resource_acl_snapshots")
test "$snapshot_count" = "0" || postgres_test_fail "failed validation left a partial ACL snapshot"

psql_test <<'SQL'
BEGIN;
INSERT INTO domain.resource_acl_snapshots (
  tenant_id, resource_kind, resource_id, space_id, default_effect
) VALUES (
  'tenant-acl-alpha-synthetic', 1, 'space-acl-alpha-synthetic',
  'space-acl-alpha-synthetic', 2
);
INSERT INTO domain.resource_acl_entries (
  tenant_id, acl_version, entry_ordinal, actor_type, actor_id, action, effect
)
SELECT snapshot.tenant_id, snapshot.acl_version, value.entry_ordinal,
  value.actor_type, value.actor_id, value.action, value.effect
FROM domain.resource_acl_snapshots AS snapshot
CROSS JOIN (VALUES
  (1, 1, 'human-acl-alpha-synthetic', 1, 1),
  (2, 1, 'human-acl-alpha-synthetic', 2, 1),
  (3, 1, 'human-acl-alpha-synthetic', 3, 1),
  (4, 1, 'human-acl-alpha-synthetic', 4, 1),
  (5, 1, 'human-acl-alpha-synthetic', 5, 1),
  (6, 1, 'human-acl-alpha-synthetic', 6, 1),
  (7, 1, 'human-acl-alpha-synthetic', 7, 1),
  (8, 1, 'human-acl-alpha-synthetic', 8, 1),
  (9, 1, 'human-acl-alpha-synthetic', 9, 1),
  (10, 1, 'human-acl-alpha-synthetic', 10, 1),
  (11, 1, 'human-acl-alpha-synthetic', 11, 1),
  (12, 1, 'human-acl-alpha-synthetic', 4, 2),
  (13, 2, 'agent-acl-alpha-synthetic', 4, 1),
  (14, 3, 'service-acl-alpha-synthetic', 4, 1)
) AS value(entry_ordinal, actor_type, actor_id, action, effect)
WHERE snapshot.tenant_id = 'tenant-acl-alpha-synthetic'
  AND snapshot.resource_kind = 1
  AND snapshot.resource_id = 'space-acl-alpha-synthetic'
  AND NOT snapshot.entries_sealed;
UPDATE domain.resource_acl_snapshots
SET entries_sealed = TRUE
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 1
  AND resource_id = 'space-acl-alpha-synthetic'
  AND NOT entries_sealed;
INSERT INTO domain.resource_acl_heads (
  tenant_id, resource_kind, resource_id, space_id, current_acl_version
)
SELECT tenant_id, resource_kind, resource_id, space_id, acl_version
FROM domain.resource_acl_snapshots
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 1
  AND resource_id = 'space-acl-alpha-synthetic';
COMMIT;
SQL

psql_test <<'SQL'
BEGIN;
INSERT INTO domain.resource_acl_snapshots (
  tenant_id, resource_kind, resource_id, channel_id, default_effect
) VALUES (
  'tenant-acl-alpha-synthetic', 2, 'channel-acl-alpha-synthetic',
  'channel-acl-alpha-synthetic', 1
);
INSERT INTO domain.resource_acl_entries (
  tenant_id, acl_version, entry_ordinal, actor_type, actor_id, action, effect
)
SELECT tenant_id, acl_version, 1, 1, 'human-acl-alpha-synthetic', 4, 2
FROM domain.resource_acl_snapshots
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 2
  AND resource_id = 'channel-acl-alpha-synthetic'
  AND NOT entries_sealed;
UPDATE domain.resource_acl_snapshots
SET entries_sealed = TRUE
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 2
  AND resource_id = 'channel-acl-alpha-synthetic'
  AND NOT entries_sealed;
INSERT INTO domain.resource_acl_heads (
  tenant_id, resource_kind, resource_id, channel_id, current_acl_version
)
SELECT tenant_id, resource_kind, resource_id, channel_id, acl_version
FROM domain.resource_acl_snapshots
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 2
  AND resource_id = 'channel-acl-alpha-synthetic';
COMMIT;
SQL

space_v1=$(sql_scalar "
  SELECT current_acl_version
  FROM domain.resource_acl_heads
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-alpha-synthetic'
")
channel_v1=$(sql_scalar "
  SELECT current_acl_version
  FROM domain.resource_acl_heads
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 2
    AND resource_id = 'channel-acl-alpha-synthetic'
")
case "$space_v1:$channel_v1" in
  *[!0-9:]* | :* | *:) postgres_test_fail "server-generated ACL versions were not nonblank numeric values" ;;
esac
test "$space_v1" != "$channel_v1" || postgres_test_fail "server-generated ACL versions were not unique"

space_entry_count=$(sql_scalar "
  SELECT count(*) FROM domain.resource_acl_entries
  WHERE tenant_id = 'tenant-acl-alpha-synthetic' AND acl_version = $space_v1
")
test "$space_entry_count" = "14" || postgres_test_fail "Space ACL did not preserve its complete entry set"

action_set=$(sql_scalar "
  SELECT string_agg(action::text, ',' ORDER BY action)
  FROM (
    SELECT DISTINCT action FROM domain.resource_acl_entries
    WHERE tenant_id = 'tenant-acl-alpha-synthetic' AND acl_version = $space_v1
  ) AS actions
")
test "$action_set" = "1,2,3,4,5,6,7,8,9,10,11" || postgres_test_fail "ACL did not accept all 11 frozen Actions"

actor_type_set=$(sql_scalar "
  SELECT string_agg(actor_type::text, ',' ORDER BY actor_type)
  FROM (
    SELECT DISTINCT actor_type FROM domain.resource_acl_entries
    WHERE tenant_id = 'tenant-acl-alpha-synthetic' AND acl_version = $space_v1
  ) AS actors
")
test "$actor_type_set" = "1,2,3" || postgres_test_fail "ACL did not preserve all three Actor types"

conflict_count=$(sql_scalar "
  SELECT count(*) FROM domain.resource_acl_entries
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND acl_version = $space_v1
    AND actor_type = 1
    AND actor_id = 'human-acl-alpha-synthetic'
    AND action = 4
")
test "$conflict_count" = "2" || postgres_test_fail "conflicting ALLOW and DENY entries were not both preserved"

space_binding=$(sql_scalar "
  SELECT resource_kind || ':' || resource_id || ':' || default_effect || ':' || entries_sealed
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = 'tenant-acl-alpha-synthetic' AND acl_version = $space_v1
")
test "$space_binding" = "1:space-acl-alpha-synthetic:2:true" || postgres_test_fail "Space ACL snapshot facts did not round-trip exactly"

channel_binding=$(sql_scalar "
  SELECT resource_kind || ':' || resource_id || ':' || default_effect || ':' || entries_sealed
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = 'tenant-acl-alpha-synthetic' AND acl_version = $channel_v1
")
test "$channel_binding" = "2:channel-acl-alpha-synthetic:1:true" || postgres_test_fail "Channel ACL snapshot facts did not round-trip exactly"

expect_sql_failure_matching "unsealed ACL snapshot commit" "
  BEGIN;
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, default_effect
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-beta-synthetic',
    'space-acl-beta-synthetic', 2
  );
  COMMIT;
" 'must be sealed before commit'

expect_sql_failure_matching "entry append after ACL sealing" "
  INSERT INTO domain.resource_acl_entries (
    tenant_id, acl_version, entry_ordinal, actor_type, actor_id, action, effect
  ) VALUES (
    'tenant-acl-alpha-synthetic', $space_v1, 15, 1,
    'human-acl-alpha-synthetic', 1, 2
  )
" 'require an unsealed snapshot'

expect_sql_failure_matching "immutable ACL entry update" "
  UPDATE domain.resource_acl_entries SET effect = 2
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND acl_version = $space_v1 AND entry_ordinal = 1
" 'entries are immutable'

expect_sql_failure_matching "immutable ACL entry deletion" "
  DELETE FROM domain.resource_acl_entries
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND acl_version = $space_v1 AND entry_ordinal = 1
" 'entries are immutable'

expect_sql_failure_matching "immutable ACL snapshot update" "
  UPDATE domain.resource_acl_snapshots SET default_effect = 1
  WHERE tenant_id = 'tenant-acl-alpha-synthetic' AND acl_version = $space_v1
" 'snapshot facts are immutable'

expect_sql_failure_matching "immutable ACL snapshot deletion" "
  DELETE FROM domain.resource_acl_snapshots
  WHERE tenant_id = 'tenant-acl-alpha-synthetic' AND acl_version = $space_v1
" 'snapshots cannot be deleted'

expect_sql_failure_matching "ACL head exact snapshot binding" "
  UPDATE domain.resource_acl_heads SET current_acl_version = $channel_v1
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-alpha-synthetic'
" 'requires a sealed exact snapshot'

expect_sql_failure_matching "ACL head one-of binding" "
  UPDATE domain.resource_acl_heads
  SET channel_id = 'channel-acl-alpha-synthetic'
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-alpha-synthetic'
" 'head identity cannot be changed'

expect_sql_failure_matching "new ACL head with both typed identifiers" "
  BEGIN;
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, default_effect
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-beta-synthetic',
    'space-acl-beta-synthetic', 2
  );
  UPDATE domain.resource_acl_snapshots
  SET entries_sealed = TRUE
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-beta-synthetic'
    AND NOT entries_sealed;
  INSERT INTO domain.resource_acl_heads (
    tenant_id, resource_kind, resource_id, space_id, channel_id, current_acl_version
  )
  SELECT tenant_id, resource_kind, resource_id, space_id,
    'channel-acl-alpha-synthetic', acl_version
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-beta-synthetic';
  COMMIT;
" 'resource_acl_heads_exactly_one_typed_resource'

expect_sql_failure_matching "immutable ACL head deletion" "
  DELETE FROM domain.resource_acl_heads
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-alpha-synthetic'
" 'current heads cannot be deleted'

expect_sql_failure_matching "unsealed ACL current head" "
  BEGIN;
  INSERT INTO domain.resource_acl_snapshots (
    tenant_id, resource_kind, resource_id, space_id, default_effect
  ) VALUES (
    'tenant-acl-alpha-synthetic', 1, 'space-acl-beta-synthetic',
    'space-acl-beta-synthetic', 2
  );
  INSERT INTO domain.resource_acl_heads (
    tenant_id, resource_kind, resource_id, space_id, current_acl_version
  )
  SELECT tenant_id, resource_kind, resource_id, space_id, acl_version
  FROM domain.resource_acl_snapshots
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-beta-synthetic'
    AND NOT entries_sealed;
  COMMIT;
" 'requires a sealed exact snapshot'

snapshot_count_before=$(sql_scalar "SELECT count(*) FROM domain.resource_acl_snapshots")

psql_test <<'SQL'
BEGIN;
INSERT INTO domain.resource_acl_snapshots (
  tenant_id, resource_kind, resource_id, space_id, default_effect
) VALUES (
  'tenant-acl-alpha-synthetic', 1, 'space-acl-alpha-synthetic',
  'space-acl-alpha-synthetic', 1
);
UPDATE domain.resource_acl_snapshots
SET entries_sealed = TRUE
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 1
  AND resource_id = 'space-acl-alpha-synthetic'
  AND NOT entries_sealed;
INSERT INTO domain.resource_acl_heads (
  tenant_id, resource_kind, resource_id, space_id, current_acl_version
)
SELECT tenant_id, resource_kind, resource_id, space_id, acl_version
FROM domain.resource_acl_snapshots
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 1
  AND resource_id = 'space-acl-alpha-synthetic'
ORDER BY acl_version DESC
LIMIT 1
ON CONFLICT (tenant_id, resource_kind, resource_id)
DO UPDATE SET current_acl_version = EXCLUDED.current_acl_version;
COMMIT;
SQL

space_v2=$(sql_scalar "
  SELECT current_acl_version FROM domain.resource_acl_heads
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-alpha-synthetic'
")
test "$space_v2" != "$space_v1" || postgres_test_fail "replacement did not move the Space ACL head"

old_entry_count=$(sql_scalar "
  SELECT count(*) FROM domain.resource_acl_entries
  WHERE tenant_id = 'tenant-acl-alpha-synthetic' AND acl_version = $space_v1
")
test "$old_entry_count" = "14" || postgres_test_fail "replacement changed old immutable ACL evidence"

snapshot_count_after=$(sql_scalar "SELECT count(*) FROM domain.resource_acl_snapshots")
test "$snapshot_count_after" -eq $((snapshot_count_before + 1)) || postgres_test_fail "replacement did not append exactly one snapshot"

psql_test <<'SQL'
BEGIN;
INSERT INTO domain.resource_acl_snapshots (
  tenant_id, resource_kind, resource_id, space_id, default_effect
) VALUES (
  'tenant-acl-alpha-synthetic', 1, 'space-acl-alpha-synthetic',
  'space-acl-alpha-synthetic', 2
);
UPDATE domain.resource_acl_snapshots
SET entries_sealed = TRUE
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 1
  AND resource_id = 'space-acl-alpha-synthetic'
  AND NOT entries_sealed;
INSERT INTO domain.resource_acl_heads (
  tenant_id, resource_kind, resource_id, space_id, current_acl_version
)
SELECT tenant_id, resource_kind, resource_id, space_id, acl_version
FROM domain.resource_acl_snapshots
WHERE tenant_id = 'tenant-acl-alpha-synthetic'
  AND resource_kind = 1
  AND resource_id = 'space-acl-alpha-synthetic'
ORDER BY acl_version DESC
LIMIT 1
ON CONFLICT (tenant_id, resource_kind, resource_id)
DO UPDATE SET current_acl_version = EXCLUDED.current_acl_version;
ROLLBACK;
SQL

space_after_rollback=$(sql_scalar "
  SELECT current_acl_version FROM domain.resource_acl_heads
  WHERE tenant_id = 'tenant-acl-alpha-synthetic'
    AND resource_kind = 1
    AND resource_id = 'space-acl-alpha-synthetic'
")
test "$space_after_rollback" = "$space_v2" || postgres_test_fail "rollback changed the prior current ACL head"

snapshot_count_after_rollback=$(sql_scalar "SELECT count(*) FROM domain.resource_acl_snapshots")
test "$snapshot_count_after_rollback" = "$snapshot_count_after" || postgres_test_fail "rollback left a visible partial ACL snapshot"

postgres_test_finish "Resource ACL persistence passed"
