package process

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxrelay"
)

type testRunner struct {
	call func() (outboxrelay.Result, error)
}

func (r testRunner) RunOne(context.Context) (outboxrelay.Result, error) { return r.call() }

func TestLoopCannotClaimBeforeReadinessAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	done := make(chan struct{})
	startup := make(chan int, 2)
	count := 0
	var ready atomic.Bool
	runner := testRunner{call: func() (outboxrelay.Result, error) {
		if !ready.Load() {
			t.Error("claim before readiness")
		}
		cancel()
		return outboxrelay.Result{Outcome: outboxrelay.OutcomeDelivered}, nil
	}}
	go func() {
		defer close(done)
		runLoop(ctx, ticks, runner, &ready, func(context.Context) error {
			count++
			startup <- count
			if count == 1 {
				return errors.New("broker down")
			}
			return nil
		})
	}()
	ticks <- time.Time{}
	if <-startup != 1 {
		t.Fatal("startup not attempted")
	}
	ticks <- time.Time{}
	if <-startup != 2 {
		t.Fatal("startup not retried")
	}
	<-done
	if ready.Load() {
		t.Fatal("stopped process ready")
	}
}

func TestHealthSeparatesLivenessFromReadiness(t *testing.T) {
	ready := false
	handler := healthHandler(func() bool { return ready })
	for _, test := range []struct {
		path   string
		status int
	}{{"/healthz", 200}, {"/readyz", 503}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
		if response.Code != test.status {
			t.Fatal("incorrect health status")
		}
	}
	ready = true
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
	if response.Code != 200 {
		t.Fatal("ready process unavailable")
	}
}

func TestSubjectPermissionSetRejectsBroaderOrDuplicateAuthority(t *testing.T) {
	for _, test := range []struct {
		actual, expected []string
		want             bool
	}{
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{[]string{"a", "a"}, []string{"a", "b"}, false},
		{[]string{">"}, []string{"a"}, false},
		{nil, []string{"a"}, false},
	} {
		if sameSubjects(test.actual, test.expected) != test.want {
			t.Fatal("permission set mismatch")
		}
	}
}
