BEGIN;

CREATE TABLE domain.organizations (
  tenant_id text PRIMARY KEY,
  display_name text NOT NULL,
  state smallint NOT NULL,
  policy_version text NOT NULL,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT organizations_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT organizations_state_known
    CHECK (state IN (1, 2, 3))
);

COMMIT;
