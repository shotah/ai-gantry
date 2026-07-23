package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	streamPlaceholder = "…"
	streamMinEditGap  = 400 * time.Millisecond
)

// editStream sends a placeholder message then edits it as tokens arrive.
type editStream struct {
	bot      *bot.Bot
	chatID   int64
	threadID int
	chunkMax int
	onSent   func(msgID int, text string) // optional; remember outbound for reactions

	msgID    int
	lastEdit time.Time
	pending  string
	started  bool
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

func (s *editStream) Started() bool { return s.started }

func (s *editStream) Update(ctx context.Context, fullText string) error {
	s.started = true
	display := fullText
	if display == "" {
		display = streamPlaceholder
	}
	display = clipRunes(display, s.chunkMax)
	s.pending = display
	if s.msgID == 0 {
		if err := s.sendInitial(ctx, display); err != nil {
			// Never abort the LLM mid-stream for channel send/edit failures;
			// Finish (or a later Update) retries the final text.
			slog.Warn("telegram stream update skipped", "err", err)
			return nil
		}
		return nil
	}
	if time.Since(s.lastEdit) < streamMinEditGap {
		return nil
	}
	if err := s.edit(ctx, display); err != nil {
		slog.Warn("telegram stream update skipped", "err", err)
		return nil
	}
	return nil
}

func (s *editStream) Finish(ctx context.Context, final string) error {
	if !s.started {
		return nil
	}
	if final == "" {
		final = s.pending
	}
	if final == "" {
		final = streamPlaceholder
	}
	parts := splitMessage(final, s.chunkMax)
	if len(parts) == 0 {
		return nil
	}
	if s.msgID == 0 {
		return s.sendInitial(ctx, parts[0])
	}
	if err := s.edit(ctx, parts[0]); err != nil {
		return err
	}
	for i := 1; i < len(parts); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(chunkPause):
		}
		var overflow *models.Message
		if err := doWith429Retry(ctx, func() error {
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
		}); err != nil {
			return err
		}
		if overflow != nil {
			s.remember(overflow.ID, parts[i])
		}
	}
	return nil
}

func (s *editStream) sendInitial(ctx context.Context, text string) error {
	var msg *models.Message
	err := doWith429Retry(ctx, func() error {
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
	})
	if err != nil {
		return fmt.Errorf("telegram: stream send: %w", err)
	}
	s.msgID = msg.ID
	s.lastEdit = time.Now()
	s.pending = text
	s.remember(msg.ID, text)
	return nil
}

func (s *editStream) edit(ctx context.Context, text string) error {
	if text == s.pending && s.msgID != 0 && time.Since(s.lastEdit) < streamMinEditGap {
		return nil
	}
	err := doWith429Retry(ctx, func() error {
		_, err := s.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    s.chatID,
			MessageID: s.msgID,
			Text:      text,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("telegram: stream edit: %w", err)
	}
	s.lastEdit = time.Now()
	s.pending = text
	s.remember(s.msgID, text)
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
