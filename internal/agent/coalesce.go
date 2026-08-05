package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// DefaultCoalesceSettle is how long to wait after the last inbound bubble
// before running one joined turn (interrupt + coalesce + settle).
const DefaultCoalesceSettle = 2 * time.Second

type coalescePart struct {
	text   string
	images []channel.Image
}

type coalesceAcceptResult struct {
	run bool
	msg channel.Message
}

type coalesceSession struct {
	mu      sync.Mutex
	parts   []coalescePart
	gen     uint64
	timer   *time.Timer
	waiters map[uint64]chan coalesceAcceptResult
	meta    channel.Message
}

func (a *Agent) initCoalesce() {
	a.coalesce = make(map[string]*coalesceSession)
}

func (a *Agent) coalesceSession(sessionID string) *coalesceSession {
	a.coalesceMu.Lock()
	defer a.coalesceMu.Unlock()
	s, ok := a.coalesce[sessionID]
	if !ok {
		s = &coalesceSession{waiters: make(map[uint64]chan coalesceAcceptResult)}
		a.coalesce[sessionID] = s
	}
	return s
}

func skipCoalesce(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "[cron]") || strings.HasPrefix(t, "[reaction]")
}

func messageStoreText(msg channel.Message) string {
	t := strings.TrimSpace(msg.Text)
	if t == "" {
		return "[photo]"
	}
	return t
}

func joinCoalesceParts(parts []coalescePart, meta channel.Message) channel.Message {
	texts := make([]string, 0, len(parts))
	var images []channel.Image
	for _, p := range parts {
		if strings.TrimSpace(p.text) != "" {
			texts = append(texts, p.text)
		}
		images = append(images, p.images...)
	}
	out := meta
	out.Text = strings.Join(texts, "\n\n")
	out.Images = images
	return out
}

// coalesceClear drops a pending settle buffer (e.g. /cancel, /new).
func (a *Agent) coalesceClear(sessionID string) {
	if a.coalesceSettle <= 0 {
		return
	}
	s := a.coalesceSession(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.parts = nil
	s.gen++ // invalidate in-flight timers
	for gen, ch := range s.waiters {
		ch <- coalesceAcceptResult{run: false}
		delete(s.waiters, gen)
	}
}

// coalesceAccept decides what to do with an inbound bubble:
//   - Nothing in flight and nothing buffered → (msg, true) immediately. A lone
//     message must not pay the settle window; that would delay every reply.
//   - Otherwise buffer it, interrupt the in-flight turn (re-merging its user
//     text), and wait out the quiet window, returning either:
//     (joined, true) — this caller runs the joined turn
//     (zero, false)  — superseded; a later Accept runs the joined batch
func (a *Agent) coalesceAccept(ctx context.Context, msg channel.Message) (channel.Message, bool, error) {
	s := a.coalesceSession(msg.SessionID)

	s.mu.Lock()
	text, images, interrupted := a.interruptTurnForCoalesce(msg.SessionID)
	if !interrupted && len(s.parts) == 0 {
		// Fast path when idle. Mid-turn after tools have started also lands here
		// (interrupt refused): buffer+settle so the new bubble runs after the
		// current turn without cancelling paid MCP search calls.
		if _, _, inFlight := a.turnPeek(msg.SessionID); !inFlight {
			s.mu.Unlock()
			return msg, true, nil
		}
	}
	if interrupted {
		a.log.Info("coalesce interrupt", "session_id", msg.SessionID)
		if !coalescePartsContain(s.parts, text) {
			s.parts = append([]coalescePart{{text: text, images: images}}, s.parts...)
		}
	}
	s.parts = append(s.parts, coalescePart{
		text:   messageStoreText(msg),
		images: append([]channel.Image(nil), msg.Images...),
	})
	s.meta = msg
	s.gen++
	myGen := s.gen
	if s.timer != nil {
		s.timer.Stop()
	}
	ch := make(chan coalesceAcceptResult, 1)
	s.waiters[myGen] = ch
	settle := a.coalesceSettle
	s.timer = time.AfterFunc(settle, func() {
		a.coalesceFlush(msg.SessionID, myGen)
	})
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		a.coalesceCancelWaiter(msg.SessionID, myGen)
		return channel.Message{}, false, ctx.Err()
	case res := <-ch:
		return res.msg, res.run, nil
	}
}

func coalescePartsContain(parts []coalescePart, text string) bool {
	for _, p := range parts {
		if p.text == text {
			return true
		}
	}
	return false
}

func (a *Agent) coalesceFlush(sessionID string, gen uint64) {
	s := a.coalesceSession(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	joined := joinCoalesceParts(s.parts, s.meta)
	s.parts = nil
	s.timer = nil
	n := len(s.waiters)
	for g, ch := range s.waiters {
		if g == gen {
			ch <- coalesceAcceptResult{run: true, msg: joined}
		} else {
			ch <- coalesceAcceptResult{run: false}
		}
		delete(s.waiters, g)
	}
	if n > 1 || strings.Count(joined.Text, "\n\n") > 0 {
		a.log.Info("coalesce flush",
			"session_id", sessionID,
			"waiters", n,
			"chars", len(joined.Text),
		)
	}
}

func (a *Agent) coalesceCancelWaiter(sessionID string, gen uint64) {
	s := a.coalesceSession(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.waiters[gen]; ok {
		delete(s.waiters, gen)
		ch <- coalesceAcceptResult{run: false}
	}
}
