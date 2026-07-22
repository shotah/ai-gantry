package channel_test

import (
	"context"
	"testing"

	"github.com/shotah/ai-gantry/internal/channel"
)

type stubWriter struct{ n int }

func (s *stubWriter) Update(context.Context, string) error { s.n++; return nil }

func (s *stubWriter) Started() bool { return s.n > 0 }

func (s *stubWriter) Finish(context.Context, string) error { return nil }

func TestReplyWriterContext(t *testing.T) {
	w := &stubWriter{}
	ctx := channel.WithReplyWriter(context.Background(), w)
	got, ok := channel.ReplyWriterFrom(ctx)
	if !ok || got != w {
		t.Fatalf("ok=%v got=%v", ok, got)
	}
	_, ok = channel.ReplyWriterFrom(context.Background())
	if ok {
		t.Fatal("expected missing writer")
	}
}
