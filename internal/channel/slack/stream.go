package slack

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	slackapi "github.com/slack-go/slack"
)

const (
	streamPlaceholder = "…"
	streamMinEditGap  = 400 * time.Millisecond
)

// editStream posts a placeholder then updates it via chat.update.
type editStream struct {
	api       poster
	channelID string
	threadTS  string
	chunkMax  int

	ts       string
	lastEdit time.Time
	pending  string
	started  bool
}

func newEditStream(api poster, channelID, threadTS string, chunkMax int) *editStream {
	if chunkMax < 1 {
		chunkMax = slackMaxMessageRunes
	}
	return &editStream{api: api, channelID: channelID, threadTS: threadTS, chunkMax: chunkMax}
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
	if s.ts == "" {
		return s.sendInitial(ctx, display)
	}
	if time.Since(s.lastEdit) < streamMinEditGap {
		return nil
	}
	return s.edit(ctx, display)
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
	if s.ts == "" {
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
		opts := []slackapi.MsgOption{slackapi.MsgOptionText(parts[i], false)}
		if s.threadTS != "" {
			opts = append(opts, slackapi.MsgOptionTS(s.threadTS))
		}
		if _, _, err := s.api.PostMessageContext(ctx, s.channelID, opts...); err != nil {
			return err
		}
	}
	return nil
}

func (s *editStream) sendInitial(ctx context.Context, text string) error {
	opts := []slackapi.MsgOption{slackapi.MsgOptionText(text, false)}
	if s.threadTS != "" {
		opts = append(opts, slackapi.MsgOptionTS(s.threadTS))
	}
	_, ts, err := s.api.PostMessageContext(ctx, s.channelID, opts...)
	if err != nil {
		return fmt.Errorf("slack: stream send: %w", err)
	}
	s.ts = ts
	s.lastEdit = time.Now()
	s.pending = text
	return nil
}

func (s *editStream) edit(ctx context.Context, text string) error {
	_, _, _, err := s.api.UpdateMessageContext(ctx, s.channelID, s.ts, slackapi.MsgOptionText(text, false))
	if err != nil {
		return fmt.Errorf("slack: stream update: %w", err)
	}
	s.lastEdit = time.Now()
	s.pending = text
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
