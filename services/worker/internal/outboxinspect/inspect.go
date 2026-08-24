// Package outboxinspect verifies read-only NATS and JetStream compatibility
// before a Worker may proceed to a separate credential publish probe.
package outboxinspect

import (
	"context"
	"reflect"
	"strings"
	"unicode"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxconfig"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

type serverFacts interface {
	MaxPayload() int64
	HeadersSupported() bool
}

type stream interface {
	Info(context.Context, ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error)
}

type manager interface {
	Domain() string
	AccountInfo(context.Context) (*jetstream.AccountInfo, error)
	StreamNameBySubject(context.Context, string) (string, error)
	Stream(context.Context, string) (stream, error)
}

// Inspector binds one immutable policy, destination, connection, and
// JetStream management capability for its lifetime.
type Inspector struct {
	server  serverFacts
	manager manager
	domain  string
	mapping outboxpublish.Mapping
	policy  outboxconfig.Snapshot
}

func (*Inspector) String() string   { return "<redacted-outbox-inspector>" }
func (*Inspector) GoString() string { return "<redacted-outbox-inspector>" }
func (*Inspector) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted-outbox-inspector]"`), nil
}

// New binds production NATS and JetStream clients without opening, closing, or
// changing either dependency.
func New(
	connection *nats.Conn,
	broker jetstream.JetStream,
	domain string,
	mapping outboxpublish.Mapping,
	policy outboxconfig.Snapshot,
) (*Inspector, error) {
	if connection == nil || nilLike(broker) || !validDomain(domain) || !mapping.Valid() || !policy.Valid() {
		return nil, inspectError(ErrorInvalidInput)
	}
	// Server facts and JetStream management must describe the same authenticated
	// connection; otherwise readiness could combine authority from two brokers.
	if broker.Conn() != connection {
		return nil, inspectError(ErrorInvalidInput)
	}
	return newInspector(connection, &jetStreamManager{broker: broker}, domain, mapping, policy)
}

func newInspector(
	server serverFacts,
	manager manager,
	domain string,
	mapping outboxpublish.Mapping,
	policy outboxconfig.Snapshot,
) (*Inspector, error) {
	if nilLike(server) || nilLike(manager) || !validDomain(domain) || !mapping.Valid() || !policy.Valid() {
		return nil, inspectError(ErrorInvalidInput)
	}
	return &Inspector{server: server, manager: manager, domain: domain, mapping: mapping, policy: policy}, nil
}

// Check performs one ordered, read-only compatibility inspection. Every
// management call receives the exact caller context.
func (inspector *Inspector) Check(ctx context.Context) error {
	if ctx == nil || !inspector.valid() {
		return inspectError(ErrorInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if inspector.manager.Domain() != inspector.domain ||
		inspector.server.MaxPayload() < int64(inspector.policy.WireHardBytes()) ||
		!inspector.server.HeadersSupported() {
		return inspectError(ErrorIncompatible)
	}

	account, dependencyErr := inspector.manager.AccountInfo(ctx)
	if err := postDependencyError(ctx, dependencyErr); err != nil {
		return err
	}
	if account == nil || account.Domain != inspector.domain {
		return inspectError(ErrorIncompatible)
	}

	streamName, dependencyErr := inspector.manager.StreamNameBySubject(ctx, inspector.mapping.Subject)
	if err := postDependencyError(ctx, dependencyErr); err != nil {
		return err
	}
	if streamName != inspector.mapping.Stream {
		return inspectError(ErrorIncompatible)
	}

	bound, dependencyErr := inspector.manager.Stream(ctx, inspector.mapping.Stream)
	if err := postDependencyError(ctx, dependencyErr); err != nil {
		return err
	}
	if nilLike(bound) {
		return inspectError(ErrorIncompatible)
	}

	info, dependencyErr := bound.Info(ctx)
	if err := postDependencyError(ctx, dependencyErr); err != nil {
		return err
	}
	if !inspector.compatible(info) {
		return inspectError(ErrorIncompatible)
	}
	return nil
}

func (inspector *Inspector) valid() bool {
	return inspector != nil && !nilLike(inspector.server) && !nilLike(inspector.manager) &&
		validDomain(inspector.domain) && inspector.mapping.Valid() && inspector.policy.Valid()
}

func (inspector *Inspector) compatible(info *jetstream.StreamInfo) bool {
	if info == nil {
		return false
	}
	config := info.Config
	return config.Name == inspector.mapping.Stream &&
		(config.MaxMsgSize == -1 || config.MaxMsgSize >= int32(inspector.policy.WireHardBytes())) &&
		config.Duplicates == inspector.policy.DuplicateWindow() &&
		!config.NoAck && !config.Sealed
}

func validDomain(domain string) bool {
	return len(domain) <= 255 && domain == strings.TrimSpace(domain) &&
		!strings.ContainsAny(domain, ".*>/\\") &&
		!strings.ContainsFunc(domain, func(character rune) bool {
			return unicode.IsControl(character) || unicode.IsSpace(character)
		})
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type jetStreamManager struct{ broker jetstream.JetStream }

func (manager *jetStreamManager) Domain() string { return manager.broker.Options().Domain }
func (manager *jetStreamManager) AccountInfo(ctx context.Context) (*jetstream.AccountInfo, error) {
	return manager.broker.AccountInfo(ctx)
}
func (manager *jetStreamManager) StreamNameBySubject(ctx context.Context, subject string) (string, error) {
	return manager.broker.StreamNameBySubject(ctx, subject)
}
func (manager *jetStreamManager) Stream(ctx context.Context, name string) (stream, error) {
	return manager.broker.Stream(ctx, name)
}
