package stdio_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/channel/stdio"
)

func TestStdio_StreamReplies(t *testing.T) {
	in := strings.NewReader("hello\n/quit\n")
	var out bytes.Buffer
	var errOut bytes.Buffer
	ch := &stdio.Channel{In: in, Out: &out, Err: &errOut, StreamReplies: true}

	err := ch.Run(context.Background(), func(ctx context.Context, _ channel.Message) (string, error) {
		w, ok := channel.ReplyWriterFrom(ctx)
		if !ok {
			t.Fatal("missing ReplyWriter")
		}
		_ = w.Update(ctx, "Hi")
		_ = w.Update(ctx, "Hi there")
		return "Hi there", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Hi there") {
		t.Fatalf("out=%q", got)
	}
}
