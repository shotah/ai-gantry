package discord

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"
)

const (
	streamPlaceholder = "…"
	streamMinEditGap  = 400 * time.Millisecond
)

// editStream sends a placeholder message then edits it as tokens arrive.
type editStream struct {
	sess      session
	channelID string
	chunkMax  int

	msgID    string
	lastEdit time.Time
	pending  string
	started  bool
}

func newEditStream(s session, channelID string, chunkMax int) *editStream {
	if chunkMax < 1 {
		chunkMax = discordMaxMessageRunes
	}
	return &editStream{sess: s, channelID: channelID, chunkMax: chunkMax}
}

func (s *editStream) Started() bool { return s.started }

func (s *editStream) Update(_ context.Context, fullText string) error {
	s.started = true
	display := fullText
	if display == "" {
		display = streamPlaceholder
	}
	display = clipRunes(display, s.chunkMax)
	s.pending = display
	if s.msgID == "" {
		return s.sendInitial(display)
	}
	if time.Since(s.lastEdit) < streamMinEditGap {
		return nil
	}
	return s.edit(display)
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
	if s.msgID == "" {
		return s.sendInitial(parts[0])
	}
	if err := s.edit(parts[0]); err != nil {
		return err
	}
	for i := 1; i < len(parts); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(chunkPause):
		}
		if _, err := s.sess.ChannelMessageSend(s.channelID, parts[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *editStream) sendInitial(text string) error {
	msg, err := s.sess.ChannelMessageSend(s.channelID, text)
	if err != nil {
		return fmt.Errorf("discord: stream send: %w", err)
	}
	s.msgID = msg.ID
	s.lastEdit = time.Now()
	return nil
}

func (s *editStream) edit(text string) error {
	_, err := s.sess.ChannelMessageEdit(s.channelID, s.msgID, text)
	if err != nil {
		return fmt.Errorf("discord: stream edit: %w", err)
	}
	s.lastEdit = time.Now()
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
