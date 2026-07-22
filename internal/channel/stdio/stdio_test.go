package stdio_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/channel/stdio"
)

func TestChannel_Run(t *testing.T) {
	in := strings.NewReader("hello\n/quit\n")
	var out bytes.Buffer
	var errOut bytes.Buffer

	ch := &stdio.Channel{In: in, Out: &out, Err: &errOut}
	err := ch.Run(context.Background(), func(_ context.Context, msg channel.Message) (string, error) {
		if msg.Text != "hello" {
			t.Errorf("Text = %q", msg.Text)
		}
		if msg.SessionID != "stdio" {
			t.Errorf("SessionID = %q", msg.SessionID)
		}
		return "pong", nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "pong") {
		t.Fatalf("out = %q, want pong", out.String())
	}
}
