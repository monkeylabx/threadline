BEGIN;

DROP TRIGGER audit_events_covered_by_head_at_commit ON domain.audit_events;
DROP FUNCTION domain.require_audit_event_covered_by_head();
DROP TRIGGER audit_tenant_heads_truncate_guard ON domain.audit_tenant_heads;
DROP TRIGGER audit_tenant_heads_advance_guard ON domain.audit_tenant_heads;
DROP FUNCTION domain.enforce_audit_tenant_head_advance();
DROP TRIGGER audit_events_truncate_guard ON domain.audit_events;
DROP TRIGGER audit_events_immutable_guard ON domain.audit_events;
DROP FUNCTION domain.reject_audit_event_mutation();
DROP TRIGGER audit_events_append_guard ON domain.audit_events;
DROP FUNCTION domain.prepare_audit_event_append();
DROP TABLE domain.audit_tenant_heads;
DROP TABLE domain.audit_events;
DROP FUNCTION domain.audit_identifier_is_canonical(text);

COMMIT;
