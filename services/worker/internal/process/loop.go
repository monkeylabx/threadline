package process

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxrelay"
)

type attemptRunner interface {
	RunOne(context.Context) (outboxrelay.Result, error)
}

// runLoop owns bounded scheduling, not claim/retry/circuit decisions.
func runLoop(ctx context.Context, ticks <-chan time.Time, runner attemptRunner, state *atomic.Bool, startup func(context.Context) error) {
	admitted := false
	for {
		select {
		case <-ctx.Done():
			state.Store(false)
			return
		case <-ticks:
			operation, cancel := context.WithTimeout(ctx, 5*time.Second)
			if !admitted {
				admitted = startup(operation) == nil
				state.Store(admitted)
			}
			if admitted {
				result, err := runner.RunOne(operation)
				state.Store(err == nil && result.Outcome != outboxrelay.OutcomeCircuitOpen)
			}
			cancel()
		}
	}
}
