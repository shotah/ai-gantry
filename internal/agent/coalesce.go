package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

// DefaultCoalesceSettle is how long to wait after the last inbound bubble
// before injecting one steer (or starting a joined turn if the first finished).
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
	quiet   chan struct{} // closed when a burst flushes or is cleared
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
	return strings.HasPrefix(t, "[cron]") || strings.HasPrefix(t, "[watch]") || strings.HasPrefix(t, "[reaction]")
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
	a.signalCoalesceQuietLocked(s)
	for gen, ch := range s.waiters {
		ch <- coalesceAcceptResult{run: false}
		delete(s.waiters, gen)
	}
}

// coalesceAccept decides what to do with an inbound bubble:
//   - Nothing in flight and nothing buffered → (msg, true) immediately. A lone
//     message must not pay the settle window; that would delay every reply.
//   - In-flight turn → buffer + settle, then steer the live loop (Completer
//     cancelled, MCP calls keep running). This Handle does not start a turn.
//   - Turn ended during settle → (joined follow-ups, true) as a new turn.
func (a *Agent) coalesceAccept(ctx context.Context, msg channel.Message) (channel.Message, bool, error) {
	s := a.coalesceSession(msg.SessionID)

	s.mu.Lock()
	inFlight := a.turnInFlight(msg.SessionID)
	if !inFlight && len(s.parts) == 0 {
		s.mu.Unlock()
		return msg, true, nil
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
	if s.quiet == nil {
		s.quiet = make(chan struct{})
	}
	ch := make(chan coalesceAcceptResult, 1)
	s.waiters[myGen] = ch
	settle := a.coalesceSettle
	s.timer = time.AfterFunc(settle, func() {
		a.coalesceFlush(msg.SessionID, myGen)
	})
	if inFlight {
		a.bumpCompleter(msg.SessionID)
		a.log.Info("steer buffer", "session_id", msg.SessionID)
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		a.coalesceCancelWaiter(msg.SessionID, myGen)
		return channel.Message{}, false, ctx.Err()
	case res := <-ch:
		return res.msg, res.run, nil
	}
}

func (a *Agent) coalesceFlush(sessionID string, gen uint64) {
	s := a.coalesceSession(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	joined := joinCoalesceParts(s.parts, s.meta)
	parts := s.parts
	n := len(s.waiters)
	inFlight := a.turnInFlight(sessionID)
	s.parts = nil
	s.timer = nil
	if inFlight {
		// Queue before closing quiet so runLoop's wait sees steers.
		a.queueSteer(sessionID, parts...)
	}
	a.signalCoalesceQuietLocked(s)
	if inFlight {
		for g, ch := range s.waiters {
			ch <- coalesceAcceptResult{run: false}
			delete(s.waiters, g)
		}
		a.log.Info("steer flush",
			"session_id", sessionID,
			"waiters", n,
			"chars", len(joined.Text),
		)
		return
	}
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

func (a *Agent) signalCoalesceQuietLocked(s *coalesceSession) {
	if s.quiet != nil {
		close(s.quiet)
		s.quiet = nil
	}
}

func (a *Agent) waitCoalesceQuiet(ctx context.Context, sessionID string) {
	if a.coalesceSettle <= 0 {
		return
	}
	s := a.coalesceSession(sessionID)
	for {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		pending := len(s.parts) > 0 || s.timer != nil
		ch := s.quiet
		s.mu.Unlock()
		if !pending {
			return
		}
		if ch == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ch:
		}
	}
}

func (a *Agent) drainSteers(ctx context.Context, sessionID string, messages []provider.Message, hasProgress bool, progress channel.ProgressWriter) []provider.Message {
	a.waitCoalesceQuiet(ctx, sessionID)
	return a.applySteers(ctx, messages, a.takeSteers(sessionID), hasProgress, progress)
}

func (a *Agent) applySteers(ctx context.Context, messages []provider.Message, parts []coalescePart, hasProgress bool, progress channel.ProgressWriter) []provider.Message {
	if len(parts) == 0 {
		return messages
	}
	text, images := joinSteer(parts)
	if text == "" && len(images) == 0 {
		return messages
	}
	if hasProgress && progress != nil {
		if line := firstLine(text); line != "" {
			_ = progress.UpdateProgress(ctx, "redirect: "+line)
		} else {
			_ = progress.UpdateProgress(ctx, "redirect")
		}
	}
	msg := provider.Message{
		Role:    provider.RoleUser,
		Content: "[steer] " + text,
	}
	for _, img := range images {
		if u := strings.TrimSpace(img.URL); u != "" {
			msg.ImageURLs = append(msg.ImageURLs, u)
		}
	}
	return append(messages, msg)
}

func joinSteer(parts []coalescePart) (text string, images []channel.Image) {
	texts := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p.text); t != "" {
			texts = append(texts, t)
		}
		images = append(images, p.images...)
	}
	return strings.Join(texts, "\n\n"), images
}

func (a *Agent) finishText(ctx context.Context, sessionID string, messages []provider.Message, text string) ([]provider.Message, string, bool, error) {
	a.waitCoalesceQuiet(ctx, sessionID)
	if err := ctx.Err(); err != nil {
		return messages, "", false, err
	}
	if !a.hasSteers(sessionID) {
		return messages, text, false, nil
	}
	if strings.TrimSpace(text) != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleAssistant,
			Content: text,
		})
	}
	return messages, "", true, nil
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
