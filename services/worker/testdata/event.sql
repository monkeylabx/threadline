INSERT INTO domain.organizations(tenant_id,display_name,state,policy_version) VALUES ($1,'Synthetic Worker',1,'v1');
INSERT INTO domain.domain_events(tenant_id,event_id,event_type,schema_version,aggregate_kind,aggregate_id,payload,occurred_at,enqueued_at)
VALUES ($1,$1,'worker.synthetic',1,'channel','c',decode('00ff80','hex'),clock_timestamp(),clock_timestamp());
INSERT INTO domain.transactional_outbox(tenant_id,event_id,destination,next_attempt_at,policy_id,policy_snapshot_digest,effective_lease_ms,effective_absolute_lifetime_ms,effective_event_retry_ceiling,effective_transport_base_ms,effective_transport_cap_ms,effective_unknown_base_ms,effective_unknown_cap_ms,effective_event_base_ms,effective_event_cap_ms,effective_retention_days)
VALUES ($1,$1,'domain-events',clock_timestamp(),'threadline.outbox.policy/v1',decode(repeat('ab',32),'hex'),30000,300000,8,1000,60000,5000,300000,5000,300000,90);
