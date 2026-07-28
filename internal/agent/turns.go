package agent

import (
	"context"
	"sync"

	"github.com/shotah/ai-gantry/internal/channel"
)

// turnSlot tracks one in-flight model turn so /cancel (or coalesce) can interrupt it.
type turnSlot struct {
	id          uint64
	cancel      context.CancelFunc
	interrupted bool
	userText    string
	images      []channel.Image
}

type sessionGate struct {
	mu sync.Mutex
}

func (a *Agent) initTurns() {
	a.turns = make(map[string]*turnSlot)
	a.sessionLocks = make(map[string]*sessionGate)
	a.initCoalesce()
}

// Cancel interrupts an in-flight turn for sessionID. Returns false when idle.
// Safe to call from another Handle (/cancel) while the turn is running.
func (a *Agent) Cancel(sessionID string) bool {
	_, _, ok := a.interruptTurn(sessionID)
	return ok
}

// interruptTurn cancels an in-flight turn and returns its user payload so
// coalesce can re-merge it into the next batch.
func (a *Agent) interruptTurn(sessionID string) (text string, images []channel.Image, ok bool) {
	a.turnMu.Lock()
	t, found := a.turns[sessionID]
	if !found {
		a.turnMu.Unlock()
		return "", nil, false
	}
	t.interrupted = true
	text = t.userText
	images = append([]channel.Image(nil), t.images...)
	cancel := t.cancel
	a.turnMu.Unlock()
	cancel()
	return text, images, true
}

// lockSession serializes normal turns (and slash commands other than /cancel)
// per session so Workers>1 cannot double-append history for the same chat.
func (a *Agent) lockSession(sessionID string) func() {
	a.sessionMu.Lock()
	g, ok := a.sessionLocks[sessionID]
	if !ok {
		g = &sessionGate{}
		a.sessionLocks[sessionID] = g
	}
	a.sessionMu.Unlock()
	g.mu.Lock()
	return g.mu.Unlock
}

// beginTurn registers a cancellable child context for sessionID.
// finish must be called exactly once; it cancels the turn and reports whether
// Cancel/interrupt marked it interrupted.
func (a *Agent) beginTurn(parent context.Context, sessionID, userText string, images []channel.Image) (ctx context.Context, finish func() (interrupted bool)) {
	ctx, cancel := context.WithCancel(parent)
	a.turnMu.Lock()
	a.turnSeq++
	id := a.turnSeq
	a.turns[sessionID] = &turnSlot{
		id:       id,
		cancel:   cancel,
		userText: userText,
		images:   append([]channel.Image(nil), images...),
	}
	a.turnMu.Unlock()
	finish = func() bool {
		cancel()
		a.turnMu.Lock()
		defer a.turnMu.Unlock()
		t, ok := a.turns[sessionID]
		if !ok || t.id != id {
			return false
		}
		interrupted := t.interrupted
		delete(a.turns, sessionID)
		return interrupted
	}
	return ctx, finish
}
