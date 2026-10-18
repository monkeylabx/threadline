package outboxstore

import (
	"testing"
	"time"
)

func TestAppendChannelEventCommittedWritesOneEventAndOneEntry(t *testing.T) {
	ctx, database := openOutboxStoreDatabase(t)
	createOutboxStoreTenant(t, ctx, database)
	producer := defaultProducer(t)
	fact := validFact()

	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appended, err := producer.AppendChannelEventCommitted(ctx, tx, fact)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Append() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if appended.DomainEventID == "" || appended.OutboxEntryID <= 0 || appended.EnqueuedAt.IsZero() {
		t.Fatalf("Append() = %#v, want generated identifiers and enqueue time", appended)
	}

	var eventType, aggregateKind, aggregateID, destination, deliveryState string
	var schemaVersion int32
	var payload []byte
	var occurredAt time.Time
	err = database.QueryRow(ctx, `
		SELECT events.event_type, events.schema_version, events.aggregate_kind, events.aggregate_id,
		       events.payload, events.occurred_at, entries.destination, entries.delivery_state
		FROM domain.domain_events AS events
		JOIN domain.transactional_outbox AS entries
		  ON entries.tenant_id = events.tenant_id AND entries.event_id = events.event_id
		WHERE events.tenant_id = $1 AND events.event_id = $2`, fact.TenantID, appended.DomainEventID,
	).Scan(&eventType, &schemaVersion, &aggregateKind, &aggregateID, &payload, &occurredAt, &destination, &deliveryState)
	if err != nil {
		t.Fatalf("read appended rows failed: %v", err)
	}
	if eventType != messageCommittedEventType || schemaVersion != messageCommittedSchemaVersion ||
		aggregateKind != channelAggregateKind || aggregateID != fact.ChannelID ||
		string(payload) != string(encodeChannelEventPayload(fact.ChannelID, fact.ChannelSeq)) ||
		!occurredAt.Equal(fact.CommittedAt) || destination != "domain-events" || deliveryState != "pending" {
		t.Fatalf("stored descriptor = %s/%d/%s/%s payload %x at %v → %s/%s, want the registered message.committed v1 values",
			eventType, schemaVersion, aggregateKind, aggregateID, payload, occurredAt, destination, deliveryState)
	}
	assertOutboxRowCounts(t, ctx, database, 1, 1)

	tx, err = database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := producer.AppendChannelEventCommitted(ctx, tx, ChannelEventCommitted{
		TenantID: fact.TenantID, ChannelID: fact.ChannelID, ChannelSeq: 2, CommittedAt: fact.CommittedAt.Add(time.Second),
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("second Append() error = %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if second.DomainEventID == appended.DomainEventID {
		t.Fatal("second Append() reused the first Domain Event ID")
	}
	assertOutboxRowCounts(t, ctx, database, 1, 1)
}
