package telegram

import (
	"context"
	"fmt"
	"log/slog"
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
func (s *editStream) Update(ctx context.Context, fullText string) error {
	display := fullText
	if display == "" {
		display = streamPlaceholder
	}
	display = clipRunes(display, s.chunkMax)

	s.mu.Lock()
	s.started = true
	s.latest = display
	s.pending = display
	s.ensureFlusherLocked(ctx)
	s.mu.Unlock()
	return nil
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
	if final == "" {
		s.mu.Lock()
		final = s.pending
		if final == "" {
			final = s.latest
		}
		s.mu.Unlock()
	}
	if final == "" {
		final = streamPlaceholder
	}

	s.mu.Lock()
	s.started = true
	// Keep latest as the first chunk preview during settle; Finish sends full split text.
	s.latest = clipRunes(final, s.chunkMax)
	s.pending = s.latest
	s.mu.Unlock()

	s.stopFlusher()

	parts := splitMessage(final, s.chunkMax)
	if len(parts) == 0 {
		return nil
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
		if err := doWith429RetryMax(ctx, func() error {
			m, err := s.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          s.chatID,
				MessageThreadID: s.threadID,
				Text:            parts[i],
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
			s.remember(overflow.ID, parts[i])
		}
	}
	return nil
}

func (s *editStream) pushFinal(ctx context.Context, text string) error {
	s.mu.Lock()
	msgID := s.msgID
	s.mu.Unlock()
	if msgID == 0 {
		return s.sendInitialRetry(ctx, text)
	}
	return s.editRetry(ctx, text)
}

func (s *editStream) sendInitial(ctx context.Context, text string) error {
	msg, err := s.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          s.chatID,
		MessageThreadID: s.threadID,
		Text:            text,
	})
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
		m, err := s.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          s.chatID,
			MessageThreadID: s.threadID,
			Text:            text,
		})
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
	_, err := s.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    s.chatID,
		MessageID: msgID,
		Text:      text,
	})
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

func (s *editStream) editRetry(ctx context.Context, text string) error {
	s.mu.Lock()
	msgID := s.msgID
	s.mu.Unlock()
	if msgID == 0 {
		return fmt.Errorf("telegram: stream edit: missing message id")
	}
	err := doWith429RetryMax(ctx, func() error {
		_, err := s.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    s.chatID,
			MessageID: msgID,
			Text:      text,
		})
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
