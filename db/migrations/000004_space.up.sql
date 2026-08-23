BEGIN;

CREATE TABLE domain.spaces (
  tenant_id text NOT NULL,
  space_id text NOT NULL,
  display_name text NOT NULL,
  discoverable boolean NOT NULL,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, space_id),
  CONSTRAINT spaces_organization_fk
    FOREIGN KEY (tenant_id) REFERENCES domain.organizations (tenant_id),
  CONSTRAINT spaces_tenant_id_not_blank
    CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
  CONSTRAINT spaces_space_id_not_blank
    CHECK (space_id <> '' AND space_id = btrim(space_id))
);

COMMIT;
