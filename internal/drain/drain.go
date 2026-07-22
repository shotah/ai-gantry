// Package drain tracks in-flight channel handlers for graceful shutdown.
package drain

import (
	"context"
	"sync"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// DefaultWait is how long shutdown waits for the current turn to finish.
const DefaultWait = 2 * time.Minute

// Gate counts active Handle calls so shutdown can wait for them.
type Gate struct {
	wg sync.WaitGroup
}

// Handler wraps h so each invocation is tracked until it returns.
func (g *Gate) Handler(h channel.Handler) channel.Handler {
	return func(ctx context.Context, msg channel.Message) (string, error) {
		g.wg.Add(1)
		defer g.wg.Done()
		return h(ctx, msg)
	}
}

// Wait blocks until all in-flight handlers finish or timeout elapses.
// Returns true if drained cleanly.
func (g *Gate) Wait(timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = DefaultWait
	}
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
