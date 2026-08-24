package outboxinspect

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

const (
	liveURLVariable     = "THREADLINE_TEST_NATS_URL"
	liveStreamVariable  = "THREADLINE_TEST_NATS_STREAM"
	liveSubjectVariable = "THREADLINE_TEST_NATS_SUBJECT"
	liveDomainVariable  = "THREADLINE_TEST_NATS_DOMAIN"
)

func TestLiveCompatibilityInspectionIsReadOnly(t *testing.T) {
	serverURL := os.Getenv(liveURLVariable)
	streamName := os.Getenv(liveStreamVariable)
	subject := os.Getenv(liveSubjectVariable)
	if serverURL == "" || streamName == "" || subject == "" {
		t.Skip("live NATS inspection variables are not set")
	}

	connection, err := nats.Connect(
		serverURL,
		nats.Name("threadline-outbox-read-only-inspection"),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		t.Fatal("live NATS connection failed")
	}
	defer connection.Close()

	domain := os.Getenv(liveDomainVariable)
	var broker jetstream.JetStream
	if domain == "" {
		broker, err = jetstream.New(connection)
	} else {
		broker, err = jetstream.NewWithDomain(connection, domain)
	}
	if err != nil {
		t.Fatal("live JetStream management client setup failed")
	}

	inspector, err := New(connection, broker, domain, outboxpublish.Mapping{
		LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
		Stream:             streamName,
		Subject:            subject,
	}, inspectPolicy(t))
	if err != nil {
		t.Fatal("live Inspector setup failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := inspector.Check(ctx); err != nil {
		t.Fatalf("live read-only compatibility inspection failed with category %v", inspectErrorCode(err))
	}
}
