// Package observer defines storx operation observation interfaces and adapters.
package observer

import (
	"context"
	"time"
)

// Event describes a completed storage operation.
type Event struct {
	Engine     string
	Target     string
	TargetType string
	Operation  string
	StartedAt  time.Time
	Duration   time.Duration
	Err        error
}

// Observer consumes storage operation events.
type Observer interface {
	Observe(ctx context.Context, event Event)
}

// ObserverFunc adapts a function into an Observer.
type ObserverFunc func(ctx context.Context, event Event)

func (fn ObserverFunc) Observe(ctx context.Context, event Event) {
	if fn != nil {
		fn(ctx, event)
	}
}

// ObserveAll fans an event out to every observer.
func ObserveAll(ctx context.Context, observers []Observer, event Event) {
	for _, obs := range observers {
		if obs != nil {
			obs.Observe(ctx, event)
		}
	}
}

// Clone returns a detached observer slice copy.
func Clone(observers []Observer) []Observer {
	if len(observers) == 0 {
		return nil
	}
	return append([]Observer(nil), observers...)
}
