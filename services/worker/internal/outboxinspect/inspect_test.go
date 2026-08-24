package outboxinspect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxconfig"
	"github.com/monkeylabx/threadline/services/worker/internal/outboxpublish"
)

var inspectMapping = outboxpublish.Mapping{
	LogicalDestination: outboxpublish.LogicalDestinationDomainEvents,
	Stream:             "DOMAIN_EVENTS",
	Subject:            "threadline.domain.events.v1",
}

func TestConstructorRejectsInvalidBindingsWithoutDependencyCalls(t *testing.T) {
	t.Parallel()

	server, baseManager := validInspectDependencies()
	policy := inspectPolicy(t)
	var nilServer *inspectServer
	var nilManager *inspectManager
	tests := []struct {
		name    string
		server  serverFacts
		manager manager
		domain  string
		mapping outboxpublish.Mapping
		policy  outboxconfig.Snapshot
	}{
		{name: "nil server", manager: baseManager, mapping: inspectMapping, policy: policy},
		{name: "typed nil server", server: nilServer, manager: baseManager, mapping: inspectMapping, policy: policy},
		{name: "nil manager", server: server, mapping: inspectMapping, policy: policy},
		{name: "typed nil manager", server: server, manager: nilManager, mapping: inspectMapping, policy: policy},
		{name: "untrimmed domain", server: server, manager: baseManager, domain: " domain", mapping: inspectMapping, policy: policy},
		{name: "spaced domain", server: server, manager: baseManager, domain: "do main", mapping: inspectMapping, policy: policy},
		{name: "wildcard domain", server: server, manager: baseManager, domain: "domain.*", mapping: inspectMapping, policy: policy},
		{name: "invalid mapping", server: server, manager: baseManager, mapping: outboxpublish.Mapping{}, policy: policy},
		{name: "zero policy", server: server, manager: baseManager, mapping: inspectMapping},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inspector, err := newInspector(test.server, test.manager, test.domain, test.mapping, test.policy)
			if inspector != nil || inspectErrorCode(err) != ErrorInvalidInput {
				t.Fatalf("newInspector = %#v/%v, want nil/invalid-input", inspector, err)
			}
		})
	}
	if inspector, err := New(nil, nil, "", inspectMapping, policy); inspector != nil || inspectErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("New nil production dependencies = %#v/%v", inspector, err)
	}
	connection := &nats.Conn{}
	broker, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if inspector, err := New(connection, broker, "", inspectMapping, policy); inspector == nil || err != nil {
		t.Fatalf("New matching production dependencies = %#v/%v", inspector, err)
	}
	if inspector, err := New(&nats.Conn{}, broker, "", inspectMapping, policy); inspector != nil || inspectErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("New mismatched production dependencies = %#v/%v", inspector, err)
	}
	if got := baseManager.recorder.snapshot(); len(got) != 0 {
		t.Fatalf("constructor called dependencies: %v", got)
	}
}

func TestCheckAcceptsEveryByteBoundaryAndUnlimitedStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxPayload int64
		maxMessage int32
	}{
		{name: "unlimited at server boundary", maxPayload: 327_680, maxMessage: -1},
		{name: "both exact boundary", maxPayload: 327_680, maxMessage: 327_680},
		{name: "both above boundary", maxPayload: 327_681, maxMessage: 327_681},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server, manager := validInspectDependencies()
			server.maxPayload = test.maxPayload
			manager.bound.info.Config.MaxMsgSize = test.maxMessage
			inspector := mustInspector(t, server, manager)
			ctx := context.WithValue(context.Background(), struct{}{}, test.name)
			if err := inspector.Check(ctx); err != nil {
				t.Fatalf("Check = %v", err)
			}
			want := []string{"domain", "max-payload", "headers", "account", "resolve", "stream", "info"}
			if got := manager.recorder.snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("calls = %v, want %v", got, want)
			}
			manager.recorder.assertContexts(t, ctx)
		})
	}
}

func TestCheckRejectsEachIncompatibleFactAndStops(t *testing.T) {
	t.Parallel()

	full := []string{"domain", "max-payload", "headers", "account", "resolve", "stream", "info"}
	tests := []struct {
		name   string
		mutate func(*inspectServer, *inspectManager)
		calls  []string
	}{
		{name: "wrong domain", mutate: func(_ *inspectServer, m *inspectManager) { m.domain = "other" }, calls: full[:1]},
		{name: "server max payload low", mutate: func(s *inspectServer, _ *inspectManager) { s.maxPayload = 327_679 }, calls: full[:2]},
		{name: "headers disabled", mutate: func(s *inspectServer, _ *inspectManager) { s.headers = false }, calls: full[:3]},
		{name: "nil account info", mutate: func(_ *inspectServer, m *inspectManager) { m.account = nil }, calls: full[:4]},
		{name: "wrong account domain", mutate: func(_ *inspectServer, m *inspectManager) { m.account.Domain = "other" }, calls: full[:4]},
		{name: "wrong resolved stream", mutate: func(_ *inspectServer, m *inspectManager) { m.resolved = "OTHER" }, calls: full[:5]},
		{name: "nil stream", mutate: func(_ *inspectServer, m *inspectManager) { m.bound = nil }, calls: full[:6]},
		{name: "nil stream info", mutate: func(_ *inspectServer, m *inspectManager) { m.bound.info = nil }, calls: full},
		{name: "wrong info name", mutate: func(_ *inspectServer, m *inspectManager) { m.bound.info.Config.Name = "OTHER" }, calls: full},
		{name: "stream max message low", mutate: func(_ *inspectServer, m *inspectManager) { m.bound.info.Config.MaxMsgSize = 327_679 }, calls: full},
		{name: "stream max message zero", mutate: func(_ *inspectServer, m *inspectManager) { m.bound.info.Config.MaxMsgSize = 0 }, calls: full},
		{name: "unknown negative max message", mutate: func(_ *inspectServer, m *inspectManager) { m.bound.info.Config.MaxMsgSize = -2 }, calls: full},
		{name: "duplicate window low", mutate: func(_ *inspectServer, m *inspectManager) {
			m.bound.info.Config.Duplicates = 2*time.Minute - time.Nanosecond
		}, calls: full},
		{name: "duplicate window high", mutate: func(_ *inspectServer, m *inspectManager) {
			m.bound.info.Config.Duplicates = 2*time.Minute + time.Nanosecond
		}, calls: full},
		{name: "PubAcks disabled", mutate: func(_ *inspectServer, m *inspectManager) { m.bound.info.Config.NoAck = true }, calls: full},
		{name: "sealed stream", mutate: func(_ *inspectServer, m *inspectManager) { m.bound.info.Config.Sealed = true }, calls: full},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server, manager := validInspectDependencies()
			test.mutate(server, manager)
			err := mustInspector(t, server, manager).Check(context.Background())
			if inspectErrorCode(err) != ErrorIncompatible {
				t.Fatalf("Check = %v, want incompatible", err)
			}
			if got := manager.recorder.snapshot(); !reflect.DeepEqual(got, test.calls) {
				t.Fatalf("calls = %v, want %v", got, test.calls)
			}
		})
	}
}

func TestCheckNormalizesDependencyFailuresAndStops(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*inspectManager)
		calls  []string
	}{
		{name: "account", mutate: func(m *inspectManager) { m.accountErr = errors.New("account-secret") }, calls: []string{"domain", "max-payload", "headers", "account"}},
		{name: "resolve", mutate: func(m *inspectManager) { m.resolveErr = errors.New("resolve-secret") }, calls: []string{"domain", "max-payload", "headers", "account", "resolve"}},
		{name: "stream", mutate: func(m *inspectManager) { m.streamErr = errors.New("stream-secret") }, calls: []string{"domain", "max-payload", "headers", "account", "resolve", "stream"}},
		{name: "info", mutate: func(m *inspectManager) { m.bound.err = errors.New("info-secret") }, calls: []string{"domain", "max-payload", "headers", "account", "resolve", "stream", "info"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server, manager := validInspectDependencies()
			test.mutate(manager)
			err := mustInspector(t, server, manager).Check(context.Background())
			if inspectErrorCode(err) != ErrorUnavailable || strings.Contains(err.Error(), "secret") {
				t.Fatalf("Check = %v, want redacted unavailable", err)
			}
			if got := manager.recorder.snapshot(); !reflect.DeepEqual(got, test.calls) {
				t.Fatalf("calls = %v, want %v", got, test.calls)
			}
		})
	}
}

func TestCheckPreservesStandardDependencyContextErrors(t *testing.T) {
	t.Parallel()

	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		sentinel := sentinel
		t.Run(sentinel.Error(), func(t *testing.T) {
			t.Parallel()
			server, manager := validInspectDependencies()
			manager.accountErr = fmt.Errorf("wrapped dependency context: %w", sentinel)
			err := mustInspector(t, server, manager).Check(context.Background())
			if err != sentinel {
				t.Fatalf("Check = %v, want exact %v", err, sentinel)
			}
		})
	}
}

func TestCheckPreservesCancellationPriorityAtEveryBoundary(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{"account", "resolve", "stream", "info"} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			server, manager := validInspectDependencies()
			ctx := newInspectContext()
			manager.cancelStage = stage
			manager.cancel = func() { ctx.fail(context.DeadlineExceeded) }
			if stage == "info" {
				manager.bound.cancel = manager.cancel
				manager.bound.cancelStage = stage
			}
			err := mustInspector(t, server, manager).Check(ctx)
			if err != context.DeadlineExceeded {
				t.Fatalf("Check = %v, want exact deadline", err)
			}
			manager.recorder.assertContexts(t, ctx)
		})
	}

	server, manager := validInspectDependencies()
	ctx := newInspectContext()
	ctx.fail(context.Canceled)
	if err := mustInspector(t, server, manager).Check(ctx); err != context.Canceled {
		t.Fatalf("pre-canceled Check = %v, want exact canceled", err)
	}
	if got := manager.recorder.snapshot(); len(got) != 0 {
		t.Fatalf("pre-canceled Check called dependencies: %v", got)
	}
}

func TestCheckRejectsNilAndZeroReceiverAndIsRaceSafe(t *testing.T) {
	t.Parallel()

	var nilInspector *Inspector
	if err := nilInspector.Check(context.Background()); inspectErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("nil Check = %v", err)
	}
	if err := (&Inspector{}).Check(context.Background()); inspectErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("zero Check = %v", err)
	}
	server, manager := validInspectDependencies()
	inspector := mustInspector(t, server, manager)
	if err := inspector.Check(nil); inspectErrorCode(err) != ErrorInvalidInput {
		t.Fatalf("nil-context Check = %v", err)
	}
	const callers = 128
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			if err := inspector.Check(context.Background()); err != nil {
				t.Errorf("parallel Check = %v", err)
			}
		}()
	}
	group.Wait()
}

func TestInspectionFormattingAndErrorsAreRedacted(t *testing.T) {
	t.Parallel()

	const canary = "raw-broker-config-canary"
	server, manager := validInspectDependencies()
	manager.domain = canary
	manager.account.Domain = canary
	inspector, err := newInspector(server, manager, canary, inspectMapping, inspectPolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	err = inspector.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	values := []any{inspector, ErrorInvalidInput, inspectError(ErrorUnavailable), inspectError(ErrorIncompatible)}
	for _, value := range values {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(encoded)} {
			if strings.Contains(rendered, canary) || strings.Contains(rendered, inspectMapping.Stream) || strings.Contains(rendered, inspectMapping.Subject) {
				t.Fatalf("rendering leaked bound facts: %q", rendered)
			}
		}
	}
}

func mustInspector(t *testing.T, server serverFacts, manager manager) *Inspector {
	t.Helper()
	inspector, err := newInspector(server, manager, "", inspectMapping, inspectPolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	return inspector
}

func inspectPolicy(t *testing.T) outboxconfig.Snapshot {
	t.Helper()
	policy, err := outboxconfig.Parse([]byte(`{
"policy_id":"threadline.outbox.policy/v1","payload_hard_bytes":262144,"wire_hard_bytes":327680,
"batch_size":64,"lease_ms":30000,"absolute_lifetime_ms":300000,"event_retry_ceiling":8,
"transport_base_ms":1000,"transport_cap_ms":60000,"unknown_base_ms":5000,"unknown_cap_ms":300000,
"event_base_ms":5000,"event_cap_ms":300000,"retention_days":90,"duplicate_window_ms":120000}`))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func inspectErrorCode(err error) ErrorCode {
	code, _ := ErrorCodeOf(err)
	return code
}

type callRecorder struct {
	mutex    sync.Mutex
	calls    []string
	contexts []context.Context
}

func (recorder *callRecorder) add(name string, ctx context.Context) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	recorder.calls = append(recorder.calls, name)
	if ctx != nil {
		recorder.contexts = append(recorder.contexts, ctx)
	}
}

func (recorder *callRecorder) snapshot() []string {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	return append([]string(nil), recorder.calls...)
}

func (recorder *callRecorder) assertContexts(t *testing.T, want context.Context) {
	t.Helper()
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	for _, got := range recorder.contexts {
		if got != want {
			t.Fatalf("dependency context = %T/%p, want exact %T/%p", got, got, want, want)
		}
	}
}

type inspectServer struct {
	recorder   *callRecorder
	maxPayload int64
	headers    bool
}

func (server *inspectServer) MaxPayload() int64 {
	server.recorder.add("max-payload", nil)
	return server.maxPayload
}
func (server *inspectServer) HeadersSupported() bool {
	server.recorder.add("headers", nil)
	return server.headers
}

type inspectManager struct {
	recorder    *callRecorder
	domain      string
	account     *jetstream.AccountInfo
	accountErr  error
	resolved    string
	resolveErr  error
	bound       *inspectStream
	streamErr   error
	cancelStage string
	cancel      func()
}

func (manager *inspectManager) Domain() string {
	manager.recorder.add("domain", nil)
	return manager.domain
}
func (manager *inspectManager) AccountInfo(ctx context.Context) (*jetstream.AccountInfo, error) {
	manager.recorder.add("account", ctx)
	manager.maybeCancel("account")
	return manager.account, manager.accountErr
}
func (manager *inspectManager) StreamNameBySubject(ctx context.Context, _ string) (string, error) {
	manager.recorder.add("resolve", ctx)
	manager.maybeCancel("resolve")
	return manager.resolved, manager.resolveErr
}
func (manager *inspectManager) Stream(ctx context.Context, _ string) (stream, error) {
	manager.recorder.add("stream", ctx)
	manager.maybeCancel("stream")
	return manager.bound, manager.streamErr
}
func (manager *inspectManager) maybeCancel(stage string) {
	if manager.cancelStage == stage && manager.cancel != nil {
		manager.cancel()
	}
}

type inspectStream struct {
	recorder    *callRecorder
	info        *jetstream.StreamInfo
	err         error
	cancelStage string
	cancel      func()
}

func (bound *inspectStream) Info(ctx context.Context, _ ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	bound.recorder.add("info", ctx)
	if bound.cancelStage == "info" && bound.cancel != nil {
		bound.cancel()
	}
	return bound.info, bound.err
}

func validInspectDependencies() (*inspectServer, *inspectManager) {
	recorder := &callRecorder{}
	return &inspectServer{recorder: recorder, maxPayload: 327_680, headers: true}, &inspectManager{
		recorder: recorder,
		account:  &jetstream.AccountInfo{},
		resolved: inspectMapping.Stream,
		bound: &inspectStream{recorder: recorder, info: &jetstream.StreamInfo{Config: jetstream.StreamConfig{
			Name: inspectMapping.Stream, MaxMsgSize: -1, Duplicates: 2 * time.Minute,
		}}},
	}
}

type inspectContext struct {
	context.Context
	mutex sync.Mutex
	err   error
	done  chan struct{}
	once  sync.Once
}

func newInspectContext() *inspectContext {
	return &inspectContext{Context: context.Background(), done: make(chan struct{})}
}
func (ctx *inspectContext) Done() <-chan struct{} { return ctx.done }
func (ctx *inspectContext) Err() error {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	return ctx.err
}
func (ctx *inspectContext) fail(err error) {
	ctx.mutex.Lock()
	ctx.err = err
	ctx.mutex.Unlock()
	ctx.once.Do(func() { close(ctx.done) })
}
