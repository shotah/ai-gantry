package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const streamPlaceholder = "…"

// How often the cached stream text is pushed to Telegram.
// Overridable in tests. ~1/s stays under typical per-chat edit flood control.
var streamFlushEvery = time.Second

// editStream caches model text and flushes it to Telegram on a timer.
// Update never blocks the LLM on Telegram 429s; Finish waits out retries.
type editStream struct {
	bot      *bot.Bot
	chatID   int64
	threadID int
	chunkMax int
	onSent   func(msgID int, text string)

	mu               sync.Mutex
	msgID            int
	latest           string
	lastFlushed      string
	pending          string
	thinking         string // raw CoT of the current model call (not HTML)
	thinkingLog      string // archived CoT from earlier calls this turn (tool loops)
	status           string // transient "hang on" line, cleared by real output
	body             string // committed prose + tool-trace lines (survives tool loops)
	answer           string // in-progress model stream for the current iteration
	useHTML          bool
	started          bool
	rateLimitedUntil time.Time
	flushStop        chan struct{}
	flushDone        chan struct{}
	flushOnce        sync.Once
	flushCtx         context.Context
}

func newEditStream(b *bot.Bot, chatID int64, threadID, chunkMax int) *editStream {
	if chunkMax < 1 {
		chunkMax = telegramMaxMessageRunes
	}
	return &editStream{bot: b, chatID: chatID, threadID: threadID, chunkMax: chunkMax}
}

func (s *editStream) remember(msgID int, text string) {
	if s.onSent != nil && msgID != 0 {
		s.onSent(msgID, text)
	}
}

func (s *editStream) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

// Update caches the latest full text and ensures the flush loop is running.
// It does not wait on Telegram rate limits — that would stall the LLM.
//
// Empty content is ignored so a fresh post-tool stream does not wipe prose
// already shown. A non-prefix content restart commits the prior answer into
// body (tool-loop iteration) instead of replacing the bubble.
func (s *editStream) Update(ctx context.Context, fullText string) error {
	s.mu.Lock()
	s.thinking = ""
	s.status = ""
	s.setAnswerLocked(fullText)
	display, useHTML := buildStreamDisplay(s.statusThinkingLocked(), s.visibleAnswerLocked(), s.chunkMax, false)
	s.useHTML = useHTML
	s.started = true
	s.latest = display
	s.pending = display
	s.ensureFlusherLocked(ctx)
	s.mu.Unlock()
	return nil
}

// UpdateThinking caches thinking + answer and flushes live italics (not
// expandable — edits would keep collapsing a blockquote).
//
// thinking accumulates within one model call; a tool loop starts a fresh call
// whose stream restarts from empty. Archive the previous call's CoT instead of
// overwriting it, so earlier reasoning never vanishes from the bubble.
// Answer text is preserved the same way across tool-loop iterations.
func (s *editStream) UpdateThinking(ctx context.Context, thinking, content string) error {
	s.mu.Lock()
	if s.thinking != "" && !strings.HasPrefix(thinking, s.thinking) {
		if s.thinkingLog != "" {
			s.thinkingLog += "\n\n"
		}
		s.thinkingLog += s.thinking
	}
	s.thinking = thinking
	s.status = ""
	s.setAnswerLocked(content)
	display, useHTML := buildStreamDisplay(s.statusThinkingLocked(), s.visibleAnswerLocked(), s.chunkMax, false)
	s.useHTML = useHTML
	s.started = true
	s.latest = display
	s.pending = display
	s.ensureFlusherLocked(ctx)
	s.mu.Unlock()
	return nil
}

// makingCallsPrefix is the TOOL_TRACE=compact header line.
const makingCallsPrefix = "Making Calls:"

// UpdateProgress commits any in-flight answer, then appends a tool-trace line
// inline in the body so → / ✓ / ✗ sit between prose chunks instead of replacing
// them. CoT stays in the thinking block; traces ride with the conversation.
//
// Compact mode (TOOL_TRACE=compact) builds one line: "Making Calls: ✓, ✗, ✓".
// A duplicate header is ignored when that line is already open; lone ✓ / ✗
// marks append to it (or open it if missing).
func (s *editStream) UpdateProgress(ctx context.Context, note string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	s.mu.Lock()
	// Park live CoT so the next model call can start a fresh thinking stream.
	if s.thinking != "" {
		if s.thinkingLog != "" {
			s.thinkingLog += "\n\n"
		}
		s.thinkingLog += s.thinking
		s.thinking = ""
	}
	s.commitAnswerLocked()
	switch {
	case note == makingCallsPrefix && trailingMakingCallsLine(s.body):
		// Already open from an earlier tool batch this turn.
	case note == "✓" || note == "✗":
		s.body = appendMakingCallsMark(s.body, note)
	default:
		if s.body != "" {
			s.body += "\n"
		}
		s.body += note
	}
	display, useHTML := buildStreamDisplay(s.statusThinkingLocked(), s.visibleAnswerLocked(), s.chunkMax, false)
	s.useHTML = useHTML
	s.started = true
	s.latest = display
	s.pending = display
	s.ensureFlusherLocked(ctx)
	s.mu.Unlock()
	return nil
}

// trailingMakingCallsLine reports whether body ends with a Making Calls line.
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

// appendMakingCallsMark adds ✓ / ✗ to a trailing Making Calls line, or opens one.
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

// UpdateStatus sets the transient "hang on" line shown while the model is still
// silent (prefill emits nothing, so the bubble would otherwise not exist yet).
// An empty note clears it: the reply replaces the notice rather than leaving it
// behind in the finished bubble.
func (s *editStream) UpdateStatus(ctx context.Context, note string) error {
	note = strings.TrimSpace(note)
	s.mu.Lock()
	if note == "" && s.status == "" {
		s.mu.Unlock()
		return nil
	}
	s.status = note
	display, useHTML := buildStreamDisplay(s.statusThinkingLocked(), s.visibleAnswerLocked(), s.chunkMax, false)
	s.useHTML = useHTML
	s.started = true
	s.latest = display
	s.pending = display
	s.ensureFlusherLocked(ctx)
	s.mu.Unlock()
	return nil
}

// setAnswerLocked grows or restarts the in-progress answer. Empty content is a
// no-op (keeps prior prose). A non-prefix restart commits the old answer into
// body first. Callers hold s.mu.
func (s *editStream) setAnswerLocked(content string) {
	if content == "" {
		return
	}
	if s.answer != "" && !strings.HasPrefix(content, s.answer) {
		s.commitAnswerLocked()
	}
	s.answer = content
}

// commitAnswerLocked moves the in-progress answer into body. Callers hold s.mu.
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

// visibleAnswerLocked is body + in-flight answer for display. Callers hold s.mu.
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

// statusThinkingLocked appends the status line below the reasoning/trace block,
// so it reads as "what is happening right now". Callers hold s.mu.
func (s *editStream) statusThinkingLocked() string {
	block := s.combinedThinkingLocked()
	switch {
	case s.status == "":
		return block
	case block == "":
		return s.status
	default:
		return block + "\n" + s.status
	}
}

// combinedThinkingLocked joins archived + current CoT. Callers hold s.mu.
func (s *editStream) combinedThinkingLocked() string {
	if s.thinkingLog == "" {
		return s.thinking
	}
	if s.thinking == "" {
		return s.thinkingLog
	}
	return s.thinkingLog + "\n\n" + s.thinking
}

func (s *editStream) ensureFlusherLocked(ctx context.Context) {
	if s.flushStop != nil {
		return
	}
	s.flushStop = make(chan struct{})
	s.flushDone = make(chan struct{})
	s.flushCtx = ctx
	go s.flushLoop()
}

func (s *editStream) flushLoop() {
	defer close(s.flushDone)
	// Push ASAP so the bubble appears, then on a steady cadence.
	s.flushOnceAPI(false)
	t := time.NewTicker(streamFlushEvery)
	defer t.Stop()
	for {
		s.mu.Lock()
		stop := s.flushStop
		ctx := s.flushCtx
		s.mu.Unlock()
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			s.flushOnceAPI(false)
		}
	}
}

func (s *editStream) stopFlusher() {
	s.mu.Lock()
	s.flushOnce.Do(func() {
		if s.flushStop != nil {
			close(s.flushStop)
		}
	})
	done := s.flushDone
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}

// flushOnceAPI pushes s.latest to Telegram unless rate-limited or unchanged.
func (s *editStream) flushOnceAPI(force bool) {
	s.mu.Lock()
	ctx := s.flushCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if !force {
		if time.Now().Before(s.rateLimitedUntil) {
			s.mu.Unlock()
			return
		}
		if s.msgID != 0 && s.latest == s.lastFlushed {
			s.mu.Unlock()
			return
		}
	}
	text := s.latest
	if text == "" {
		text = streamPlaceholder
	}
	msgID := s.msgID
	s.mu.Unlock()

	if msgID == 0 {
		if err := s.sendInitial(ctx, text); err != nil {
			s.noteRateLimit(err)
			if !isTooManyRequests(err) {
				slog.Warn("telegram stream flush send skipped", "err", err)
			}
		}
		return
	}
	if err := s.editOnce(ctx, text); err != nil {
		s.noteRateLimit(err)
		if !isTooManyRequests(err) {
			slog.Warn("telegram stream flush edit skipped", "err", err)
		}
	}
}

func (s *editStream) noteRateLimit(err error) {
	wait, ok := retryAfterDuration(err)
	if !ok {
		return
	}
	until := time.Now().Add(wait)
	s.mu.Lock()
	if until.After(s.rateLimitedUntil) {
		s.rateLimitedUntil = until
	}
	s.mu.Unlock()
	slog.Warn("telegram stream cooling down", "retry_after", wait.String())
}

func (s *editStream) Finish(ctx context.Context, final string) error {
	s.mu.Lock()
	// The status line is a waiting indicator — never part of the final bubble,
	// even if the turn ended by error or cancel before anything cleared it.
	s.status = ""
	thinking := s.combinedThinkingLocked()
	final = strings.TrimSpace(final)
	// Raw answer only for the content half. pending/latest hold formatted
	// display (may contain HTML); never re-compose from those.
	switch {
	case final == "":
		s.commitAnswerLocked()
		final = s.body
	case s.answer != "" && (final == s.answer || strings.HasPrefix(final, s.answer)):
		// Same iteration: final is the completed stream (or identical).
		s.answer = final
		final = s.visibleAnswerLocked()
	default:
		visible := s.visibleAnswerLocked()
		switch {
		case visible == "":
			// only the agent-returned final
		case final == visible || strings.HasSuffix(visible, final) || strings.Contains(visible, final):
			final = visible
		default:
			// New segment after tools — keep math/traces, append last prose.
			s.commitAnswerLocked()
			if s.body != "" {
				final = s.body + "\n\n" + final
			}
		}
	}
	s.body = final
	s.answer = ""
	// Final edit may use expandable — stream flushes are done, so it won't keep collapsing.
	display, useHTML := buildStreamDisplay(thinking, final, s.chunkMax, true)
	s.useHTML = useHTML
	s.started = true
	s.latest = display
	s.pending = display
	s.mu.Unlock()

	s.stopFlusher()

	// HTML thinking must stay one message (splitting would break tags).
	if useHTML {
		return s.pushFinal(ctx, display)
	}

	parts := splitMessage(final, s.chunkMax)
	if len(parts) == 0 {
		if final == "" {
			parts = []string{streamPlaceholder}
		} else {
			return nil
		}
	}
	if err := s.pushFinal(ctx, parts[0]); err != nil {
		return err
	}
	for i := 1; i < len(parts); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(chunkPause):
		}
		var overflow *models.Message
		chunk := parts[i]
		if err := doWith429RetryMax(ctx, func() error {
			m, err := s.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          s.chatID,
				MessageThreadID: s.threadID,
				Text:            chunk,
			})
			if err != nil {
				return err
			}
			overflow = m
			return nil
		}, finishRetryMaxWait); err != nil {
			return err
		}
		if overflow != nil {
			s.remember(overflow.ID, chunk)
		}
	}
	return nil
}

func (s *editStream) pushFinal(ctx context.Context, text string) error {
	s.mu.Lock()
	msgID := s.msgID
	same := msgID != 0 && text == s.lastFlushed
	s.mu.Unlock()
	if msgID == 0 {
		return s.sendInitialRetry(ctx, text)
	}
	if same {
		return nil
	}
	return s.editRetry(ctx, text)
}

func (s *editStream) sendInitial(ctx context.Context, text string) error {
	p := &bot.SendMessageParams{
		ChatID:          s.chatID,
		MessageThreadID: s.threadID,
		Text:            text,
	}
	s.mu.Lock()
	if s.useHTML {
		p.ParseMode = models.ParseModeHTML
	}
	s.mu.Unlock()
	msg, err := s.bot.SendMessage(ctx, p)
	if err != nil {
		return fmt.Errorf("telegram: stream send: %w", err)
	}
	s.mu.Lock()
	s.msgID = msg.ID
	s.lastFlushed = text
	s.pending = text
	s.mu.Unlock()
	s.remember(msg.ID, text)
	return nil
}

func (s *editStream) sendInitialRetry(ctx context.Context, text string) error {
	var msg *models.Message
	err := doWith429RetryMax(ctx, func() error {
		p := &bot.SendMessageParams{
			ChatID:          s.chatID,
			MessageThreadID: s.threadID,
			Text:            text,
		}
		s.mu.Lock()
		if s.useHTML {
			p.ParseMode = models.ParseModeHTML
		}
		s.mu.Unlock()
		m, err := s.bot.SendMessage(ctx, p)
		if err != nil {
			return err
		}
		msg = m
		return nil
	}, finishRetryMaxWait)
	if err != nil {
		return fmt.Errorf("telegram: stream send: %w", err)
	}
	s.mu.Lock()
	s.msgID = msg.ID
	s.lastFlushed = text
	s.pending = text
	s.mu.Unlock()
	s.remember(msg.ID, text)
	return nil
}

func (s *editStream) editOnce(ctx context.Context, text string) error {
	s.mu.Lock()
	msgID := s.msgID
	s.mu.Unlock()
	if msgID == 0 {
		return fmt.Errorf("telegram: stream edit: missing message id")
	}
	p := &bot.EditMessageTextParams{
		ChatID:    s.chatID,
		MessageID: msgID,
		Text:      text,
	}
	s.mu.Lock()
	if s.useHTML {
		p.ParseMode = models.ParseModeHTML
	}
	s.mu.Unlock()
	_, err := s.bot.EditMessageText(ctx, p)
	if err != nil {
		if isMessageNotModified(err) {
			s.mu.Lock()
			s.lastFlushed = text
			s.pending = text
			s.mu.Unlock()
			s.remember(msgID, text)
			return nil
		}
		return fmt.Errorf("telegram: stream edit: %w", err)
	}
	s.mu.Lock()
	s.lastFlushed = text
	s.pending = text
	s.mu.Unlock()
	s.remember(msgID, text)
	return nil
}

func (s *editStream) editRetry(ctx context.Context, text string) error {
	s.mu.Lock()
	msgID := s.msgID
	s.mu.Unlock()
	if msgID == 0 {
		return fmt.Errorf("telegram: stream edit: missing message id")
	}
	err := doWith429RetryMax(ctx, func() error {
		p := &bot.EditMessageTextParams{
			ChatID:    s.chatID,
			MessageID: msgID,
			Text:      text,
		}
		s.mu.Lock()
		if s.useHTML {
			p.ParseMode = models.ParseModeHTML
		}
		s.mu.Unlock()
		_, err := s.bot.EditMessageText(ctx, p)
		if isMessageNotModified(err) {
			return nil
		}
		return err
	}, finishRetryMaxWait)
	if err != nil {
		return fmt.Errorf("telegram: stream edit: %w", err)
	}
	s.mu.Lock()
	s.lastFlushed = text
	s.pending = text
	s.mu.Unlock()
	s.remember(msgID, text)
	return nil
}

func clipRunes(s string, limit int) string {
	if limit < 1 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	if limit < 2 {
		return string(r[:1])
	}
	return string(r[:limit-1]) + "…"
}
