package fanout

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
)

// WakeSource opens a coalesced, live-only notification subscription.
// Closing a subscription never changes broker history or Core cursors.
type WakeSource interface {
	Subscribe(context.Context) (<-chan struct{}, func() error, error)
}

type NATS struct{ Connection *nats.Conn }

func (source NATS) Subscribe(ctx context.Context) (<-chan struct{}, func() error, error) {
	if source.Connection == nil || !source.Connection.IsConnected() {
		return nil, nil, errors.New("broker unavailable")
	}
	signals := make(chan struct{}, 1)
	subscription, err := source.Connection.Subscribe("threadline.domain.events.v1", func(message *nats.Msg) {
		if !messageHint(message.Data) {
			return
		}
		select {
		case signals <- struct{}{}:
		default:
		} // One wake suffices: Core owns catch-up.
	})
	if err != nil {
		return nil, nil, errors.New("broker subscription failed")
	}
	operation, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err = subscription.SetPendingLimits(32, 1048576); err == nil {
		err = source.Connection.FlushWithContext(operation)
	}
	if err != nil {
		if closeErr := subscription.Unsubscribe(); closeErr != nil {
			return nil, nil, errors.New("broker subscription cleanup failed")
		}
		return nil, nil, errors.New("broker subscription failed")
	}
	closeSubscription := func() error {
		if err := subscription.Unsubscribe(); err != nil {
			return errors.New("broker subscription cleanup failed")
		}
		return nil
	}
	return signals, closeSubscription, nil
}
