package agent_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestCancel_NothingInProgress(t *testing.T) {
	a, err := agent.New(agent.Options{
		Completer: &fakeCompleter{},
		Sessions:  newMemHistory(),
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Cancel("s") {
		t.Fatal("expected false when idle")
	}
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/cancel"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "nothing in progress") {
		t.Fatalf("got %q", got)
	}
}

func TestCancel_InterruptsInFlight(t *testing.T) {
	block := &blockingCompleter{started: make(chan struct{})}
	a, err := agent.New(agent.Options{
		Completer: block,
		Sessions:  newMemHistory(),
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var reply string
	var handleErr error
	go func() {
		defer close(done)
		reply, handleErr = a.Handle(context.Background(), channel.Message{
			SessionID: "s1",
			Text:      "long running ask",
		})
	}()

	select {
	case <-block.started:
	case <-time.After(2 * time.Second):
		t.Fatal("turn did not start")
	}

	cancelReply, err := a.Handle(context.Background(), channel.Message{
		SessionID: "s1",
		Text:      "/cancel@TimBot",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cancelReply, "cancelled") {
		t.Fatalf("cancel reply = %q", cancelReply)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight Handle did not return after cancel")
	}
	if handleErr != nil {
		t.Fatalf("Handle err = %v", handleErr)
	}
	if reply != "" {
		t.Fatalf("cancelled turn should return empty reply, got %q", reply)
	}
}

func TestCancel_DoesNotAppendHistory(t *testing.T) {
	block := &blockingCompleter{started: make(chan struct{})}
	hist := newMemHistory()
	a, err := agent.New(agent.Options{
		Completer: block,
		Sessions:  hist,
		Model:     "m",
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "hello"})
	}()
	select {
	case <-block.started:
	case <-time.After(2 * time.Second):
		t.Fatal("turn did not start")
	}
	if !a.Cancel("s") {
		t.Fatal("Cancel returned false")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle did not return")
	}

	msgs, err := hist.Messages(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("history should be empty after cancel, got %+v", msgs)
	}
}

// blockingCompleter hangs in Complete until ctx is cancelled.
type blockingCompleter struct {
	started chan struct{}
	once    sync.Once
}

func (b *blockingCompleter) Complete(ctx context.Context, _ provider.Request) (*provider.Result, error) {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}
