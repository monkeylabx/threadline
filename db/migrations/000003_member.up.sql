BEGIN;

CREATE TABLE domain.members (
  tenant_id text NOT NULL,
  actor_type smallint NOT NULL,
  actor_id text NOT NULL,
  display_name text NOT NULL,
  role smallint NOT NULL,
  state smallint NOT NULL,
  org_unit_path text,
  joined_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, actor_type, actor_id),
  CONSTRAINT members_organization_fk
    FOREIGN KEY (tenant_id) REFERENCES domain.organizations (tenant_id),
  CONSTRAINT members_actor_id_not_blank
    CHECK (actor_id <> '' AND actor_id = btrim(actor_id)),
  CONSTRAINT members_actor_type_known
    CHECK (actor_type IN (1, 2, 3)),
  CONSTRAINT members_role_known
    CHECK (role IN (1, 2, 3, 4, 5)),
  CONSTRAINT members_state_known
    CHECK (state IN (1, 2, 3))
);

COMMIT;
