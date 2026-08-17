package agent

import (
	"context"
	"strings"
	"sync"

	"github.com/shotah/ai-gantry/internal/channel"
)

// turnSlot tracks one in-flight model turn so /cancel can abort it and a
// follow-up bubble can steer it (cancel Completer only; tools keep running).
type turnSlot struct {
	id          uint64
	cancel      context.CancelFunc
	cancelComp  context.CancelFunc // current Completer call only
	interrupted bool               // /cancel
	userText    string
	storeText   string // original + steers; one history user turn
	images      []channel.Image
	steers      []coalescePart
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

// interruptTurn cancels an in-flight turn (/cancel). Tools and Completer stop.
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
	comp := t.cancelComp
	t.cancelComp = nil
	a.turnMu.Unlock()
	if comp != nil {
		comp()
	}
	cancel()
	return text, images, true
}

func (a *Agent) turnInFlight(sessionID string) bool {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	_, ok := a.turns[sessionID]
	return ok
}

func (a *Agent) armCompleter(parent context.Context, sessionID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	a.turnMu.Lock()
	if t, ok := a.turns[sessionID]; ok {
		t.cancelComp = cancel
	}
	a.turnMu.Unlock()
	return ctx, func() {
		cancel()
		a.turnMu.Lock()
		if t, ok := a.turns[sessionID]; ok {
			t.cancelComp = nil
		}
		a.turnMu.Unlock()
	}
}

// bumpCompleter cancels the current Completer call without aborting the turn.
// In-flight MCP calls use the turn context and keep running.
func (a *Agent) bumpCompleter(sessionID string) {
	a.turnMu.Lock()
	var cancel context.CancelFunc
	if t, ok := a.turns[sessionID]; ok {
		cancel = t.cancelComp
		t.cancelComp = nil
	}
	a.turnMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *Agent) queueSteer(sessionID string, parts ...coalescePart) {
	if len(parts) == 0 {
		return
	}
	a.turnMu.Lock()
	t, ok := a.turns[sessionID]
	if !ok {
		a.turnMu.Unlock()
		return
	}
	t.steers = append(t.steers, parts...)
	for _, p := range parts {
		if txt := strings.TrimSpace(p.text); txt != "" {
			if t.storeText != "" {
				t.storeText += "\n\n"
			}
			t.storeText += txt
		}
		t.images = append(t.images, p.images...)
	}
	a.turnMu.Unlock()
}

func (a *Agent) takeSteers(sessionID string) []coalescePart {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	t, ok := a.turns[sessionID]
	if !ok || len(t.steers) == 0 {
		return nil
	}
	out := t.steers
	t.steers = nil
	return out
}

func (a *Agent) hasSteers(sessionID string) bool {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	t, ok := a.turns[sessionID]
	return ok && len(t.steers) > 0
}

func (a *Agent) turnStoreText(sessionID, fallback string) string {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	if t, ok := a.turns[sessionID]; ok && t.storeText != "" {
		return t.storeText
	}
	return fallback
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
// Cancel marked it interrupted.
func (a *Agent) beginTurn(parent context.Context, sessionID, userText string, images []channel.Image) (ctx context.Context, finish func() (interrupted bool)) {
	ctx, cancel := context.WithCancel(parent)
	a.turnMu.Lock()
	a.turnSeq++
	id := a.turnSeq
	a.turns[sessionID] = &turnSlot{
		id:        id,
		cancel:    cancel,
		userText:  userText,
		storeText: userText,
		images:    append([]channel.Image(nil), images...),
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
