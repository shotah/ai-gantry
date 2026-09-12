package pendant

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/shotah/ai-gantry/internal/cron"
)

// Pendant TEXT_MAX is 8_000 bytes; stay under that in runes for ASCII-heavy traces.
const draftTextMax = 8000

const makingCallsPrefix = "Making Calls:"

// How often Update (token stream) may hit the wire. Status and tool traces
// flush immediately — they are few per turn. Tests may shorten this.
// 250ms with a trailing flush: at most one draft per gap, and one more
// after the last delta so a fast model does not leave the bubble on "bo".
var streamMinGap = 250 * time.Millisecond

type frameWriter func(outboundFrame) error

// editStream is Telegram layer 2 for the mailbox: a replaceable Kit bubble.
// kind=draft is ephemeral (not queued). Finish writes the persisted reply.
type editStream struct {
	write  frameWriter
	userID string

	mu          sync.Mutex
	status      string
	body        string
	answer      string
	started     bool
	finished    bool
	latest      string
	lastFlushed string
	lastFlushAt time.Time
	flushTimer  *time.Timer
	flushWG     sync.WaitGroup
	photos      []string
}

func newEditStream(write frameWriter, userID string) *editStream {
	return &editStream{write: write, userID: userID}
}

func (s *editStream) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

func (s *editStream) Update(_ context.Context, fullText string) error {
	s.mu.Lock()
	s.status = ""
	s.setAnswerLocked(fullText)
	return s.pushLocked(false)
}

func (s *editStream) UpdateProgress(_ context.Context, note string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	s.mu.Lock()
	s.commitAnswerLocked()
	switch {
	case note == makingCallsPrefix && trailingMakingCallsLine(s.body):
	case note == "✓" || note == "✗":
		s.body = appendMakingCallsMark(s.body, note)
	default:
		if s.body != "" {
			s.body += "\n"
		}
		s.body += note
	}
	return s.pushLocked(true)
}

func (s *editStream) UpdateStatus(_ context.Context, note string) error {
	note = strings.TrimSpace(note)
	s.mu.Lock()
	if note == "" && s.status == "" {
		s.mu.Unlock()
		return nil
	}
	s.status = note
	return s.pushLocked(true)
}

func (s *editStream) Discard(_ context.Context) error {
	s.mu.Lock()
	s.stopFlushLocked()
	had := s.started
	s.status = ""
	s.body = ""
	s.answer = ""
	s.latest = ""
	s.lastFlushed = ""
	s.started = false
	userID := s.userID
	s.mu.Unlock()
	s.flushWG.Wait()
	if !had {
		return nil
	}
	return s.write(outboundFrame{Kind: "draft", UserID: userID, Text: ""})
}

func (s *editStream) setPhotos(urls []string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.photos = append([]string(nil), urls...)
	s.mu.Unlock()
}

func (s *editStream) Finish(_ context.Context, final string) error {
	s.mu.Lock()
	s.stopFlushLocked()
	s.mu.Unlock()
	s.flushWG.Wait()

	s.mu.Lock()
	s.status = ""
	final = strings.TrimSpace(final)
	switch {
	case final == "":
		s.commitAnswerLocked()
		final = s.body
	case s.answer != "" && (final == s.answer || strings.HasPrefix(final, s.answer)):
		s.answer = final
		final = s.visibleAnswerLocked()
	default:
		visible := s.visibleAnswerLocked()
		switch {
		case visible == "":
		case final == visible || strings.HasSuffix(visible, final) || strings.Contains(visible, final):
			final = visible
		default:
			s.commitAnswerLocked()
			if s.body != "" {
				final = s.body + "\n\n" + final
			}
		}
	}
	s.body = cron.StripWaitTokens(final)
	s.answer = ""
	s.started = true
	s.latest = s.body
	userID := s.userID
	body := clipRunes(s.body)
	extra := append([]string(nil), s.photos...)
	s.photos = nil
	s.mu.Unlock()
	frames := replyFrames("reply", userID, "", body, extra...)
	if len(frames) == 0 {
		return nil
	}
	var first error
	for _, f := range frames {
		if err := s.write(f); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *editStream) pushLocked(force bool) error {
	defer s.mu.Unlock()
	if s.finished {
		return nil
	}
	display := s.displayLocked()
	s.started = true
	s.latest = display
	if display == "" || display == s.lastFlushed {
		return nil
	}
	now := time.Now()
	throttled := !force && streamMinGap > 0 && s.lastFlushed != "" && now.Sub(s.lastFlushAt) < streamMinGap
	if throttled {
		s.armTrailingLocked()
		return nil
	}
	s.cancelTimerLocked()
	s.lastFlushed = display
	s.lastFlushAt = now
	return s.write(outboundFrame{Kind: "draft", UserID: s.userID, Text: display})
}

func (s *editStream) armTrailingLocked() {
	if s.finished || s.flushTimer != nil || streamMinGap <= 0 {
		return
	}
	wait := streamMinGap - time.Since(s.lastFlushAt)
	if wait < 0 {
		wait = 0
	}
	s.flushWG.Add(1)
	s.flushTimer = time.AfterFunc(wait, s.trailingFlush)
}

func (s *editStream) trailingFlush() {
	defer s.flushWG.Done()
	s.mu.Lock()
	s.flushTimer = nil
	if s.finished {
		s.mu.Unlock()
		return
	}
	display := s.latest
	if display == "" || display == s.lastFlushed {
		s.mu.Unlock()
		return
	}
	s.lastFlushed = display
	s.lastFlushAt = time.Now()
	userID := s.userID
	s.mu.Unlock()
	_ = s.write(outboundFrame{Kind: "draft", UserID: userID, Text: display})
}

func (s *editStream) stopFlushLocked() {
	s.finished = true
	s.cancelTimerLocked()
}

func (s *editStream) cancelTimerLocked() {
	if s.flushTimer == nil {
		return
	}
	if s.flushTimer.Stop() {
		s.flushWG.Done()
	}
	s.flushTimer = nil
}

func (s *editStream) displayLocked() string {
	block := s.status
	answer := s.visibleAnswerLocked()
	switch {
	case block == "":
		return clipRunes(answer)
	case answer == "":
		return clipRunes(block)
	default:
		return clipRunes(block + "\n" + answer)
	}
}

func (s *editStream) setAnswerLocked(content string) {
	content = cron.StripWaitTokensLive(content)
	if content == "" {
		return
	}
	if s.answer != "" && !strings.HasPrefix(content, s.answer) {
		s.commitAnswerLocked()
	}
	s.answer = content
}

func (s *editStream) commitAnswerLocked() {
	ans := strings.TrimSpace(s.answer)
	s.answer = ""
	if ans == "" {
		return
	}
	if s.body != "" {
		s.body += "\n\n"
	}
	s.body += ans
}

func (s *editStream) visibleAnswerLocked() string {
	switch {
	case s.body == "":
		return s.answer
	case s.answer == "":
		return s.body
	default:
		return s.body + "\n\n" + s.answer
	}
}

func trailingMakingCallsLine(body string) bool {
	return strings.HasPrefix(trailingLine(body), makingCallsPrefix)
}

func trailingLine(body string) string {
	i := strings.LastIndexByte(body, '\n')
	if i < 0 {
		return body
	}
	return body[i+1:]
}

func appendMakingCallsMark(body, mark string) string {
	i := strings.LastIndexByte(body, '\n')
	prefix, line := "", body
	if i >= 0 {
		prefix = body[:i+1]
		line = body[i+1:]
	}
	if !strings.HasPrefix(line, makingCallsPrefix) {
		if body != "" {
			return body + "\n" + makingCallsPrefix + " " + mark
		}
		return makingCallsPrefix + " " + mark
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, makingCallsPrefix))
	if rest == "" {
		return prefix + makingCallsPrefix + " " + mark
	}
	return prefix + makingCallsPrefix + " " + rest + ", " + mark
}

func clipRunes(s string) string {
	if utf8.RuneCountInString(s) <= draftTextMax {
		return s
	}
	return string([]rune(s)[:draftTextMax-1]) + "…"
}
