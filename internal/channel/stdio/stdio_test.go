package stdio_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/channel/stdio"
)

func TestChannel_Push(t *testing.T) {
	var out bytes.Buffer
	ch := &stdio.Channel{Out: &out}
	if err := ch.Push(context.Background(), channel.Outbound{Text: "wake up"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[cron] wake up") {
		t.Fatalf("out=%q", out.String())
	}
}

func TestNew(t *testing.T) {
	ch := stdio.New()
	if ch == nil || ch.In == nil || ch.Out == nil || ch.Err == nil {
		t.Fatal("New() incomplete")
	}
}

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

func TestChannel_SkipsBlankAndHandlesError(t *testing.T) {
	in := strings.NewReader("\n\nboom\n/exit\n")
	var out, errOut bytes.Buffer
	ch := &stdio.Channel{In: in, Out: &out, Err: &errOut}
	calls := 0
	err := ch.Run(context.Background(), func(_ context.Context, msg channel.Message) (string, error) {
		calls++
		if msg.Text == "boom" {
			return "", errors.New("nope")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	if !strings.Contains(errOut.String(), "nope") {
		t.Fatalf("errOut=%q", errOut.String())
	}
}

func TestChannel_ContextCancel(t *testing.T) {
	// Block on a pipe read; cancel should make the next loop iteration exit
	// once a line arrives after cancel — use already-cancelled ctx before prompt.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := &stdio.Channel{
		In:  strings.NewReader("hello\n"),
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
	}
	start := time.Now()
	err := ch.Run(ctx, func(context.Context, channel.Message) (string, error) {
		t.Fatal("should not handle when ctx already cancelled")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("took too long")
	}
}

func TestChannel_EmptyReply(t *testing.T) {
	in := strings.NewReader("hi\n/q\n")
	var out bytes.Buffer
	ch := &stdio.Channel{In: in, Out: &out, Err: &bytes.Buffer{}}
	if err := ch.Run(context.Background(), func(context.Context, channel.Message) (string, error) {
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	// prompts only — empty handler reply must not add a body line
	if strings.Contains(out.String(), "hi") {
		t.Fatalf("unexpected body in %q", out.String())
	}
}
