package drain_test

import (
	"context"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/drain"
)

func TestGate_WaitDrains(t *testing.T) {
	g := &drain.Gate{}
	started := make(chan struct{})
	release := make(chan struct{})

	h := g.Handler(func(_ context.Context, _ channel.Message) (string, error) {
		close(started)
		<-release
		return "ok", nil
	})

	go func() {
		_, _ = h(context.Background(), channel.Message{Text: "hi"})
	}()
	<-started

	done := make(chan bool, 1)
	go func() { done <- g.Wait(time.Second) }()

	select {
	case <-done:
		t.Fatal("Wait returned before handler finished")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if ok := <-done; !ok {
		t.Fatal("expected clean drain")
	}
}

func TestGate_WaitTimeout(t *testing.T) {
	g := &drain.Gate{}
	block := make(chan struct{})
	h := g.Handler(func(_ context.Context, _ channel.Message) (string, error) {
		<-block
		return "", nil
	})
	go func() { _, _ = h(context.Background(), channel.Message{}) }()
	time.Sleep(20 * time.Millisecond)
	if g.Wait(30 * time.Millisecond) {
		t.Fatal("expected timeout")
	}
	close(block)
}
