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
	"github.com/shotah/ai-gantry/internal/session"
)

type streamCompleter struct {
	mu    sync.Mutex
	parts []string
}

func (s *streamCompleter) Complete(ctx context.Context, req provider.Request) (*provider.Result, error) {
	return s.CompleteStream(ctx, req, nil)
}

func (s *streamCompleter) CompleteStream(_ context.Context, _ provider.Request, onProgress func(content, thinking string) error) (*provider.Result, error) {
	s.mu.Lock()
	parts := append([]string(nil), s.parts...)
	s.mu.Unlock()
	var full strings.Builder
	for _, p := range parts {
		full.WriteString(p)
		if onProgress != nil {
			if err := onProgress(full.String(), ""); err != nil {
				return nil, err
			}
		}
	}
	return &provider.Result{Content: full.String()}, nil
}

type memWriter struct {
	mu    sync.Mutex
	texts []string
	start bool
}

func (m *memWriter) Update(_ context.Context, fullText string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.start = true
	m.texts = append(m.texts, fullText)
	return nil
}

func (m *memWriter) Started() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.start
}

func (m *memWriter) Finish(_ context.Context, final string) error {
	return m.Update(context.Background(), final)
}

func TestAgent_StreamsFinalText(t *testing.T) {
	sc := &streamCompleter{parts: []string{"Hel", "lo ", "world"}}
	w := &memWriter{}
	a, err := agent.New(agent.Options{
		Completer:     sc,
		Sessions:      newMemHistory(),
		StreamReplies: true,
		Model:         "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := channel.WithReplyWriter(context.Background(), w)
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "Hello world" {
		t.Fatalf("reply=%q", reply)
	}
	if !w.Started() || len(w.texts) < 3 {
		t.Fatalf("writer texts=%v", w.texts)
	}
	if w.texts[len(w.texts)-1] != "Hello world" {
		t.Fatalf("last=%q", w.texts[len(w.texts)-1])
	}
}

func TestAgent_StreamStripsWaitToken(t *testing.T) {
	sc := &streamCompleter{parts: []string{"Thai or pizza?\n", "[", "wait]"}}
	w := &memWriter{}
	a, err := agent.New(agent.Options{
		Completer:     sc,
		Sessions:      newMemHistory(),
		StreamReplies: true,
		Model:         "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := channel.WithReplyWriter(context.Background(), w)
	reply, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "dinner?"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reply, "[wait]") {
		t.Fatalf("reply leaked [wait]: %q", reply)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, txt := range w.texts {
		if strings.Contains(strings.ToLower(txt), "[wait]") {
			t.Fatalf("stream leaked [wait]: %q in %v", txt, w.texts)
		}
	}
}

func TestAgent_Handle_FinishesStreamBeforeAppend(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	hist := &gateHistory{memHistory: newMemHistory(), entered: entered, release: release}
	w := &finishWriter{finished: finished}
	a, err := agent.New(agent.Options{
		Completer:     &streamCompleter{parts: []string{"done"}},
		Sessions:      hist,
		StreamReplies: true,
		Model:         "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := channel.WithReplyWriter(context.Background(), w)
	errCh := make(chan error, 1)
	go func() {
		_, err := a.Handle(ctx, channel.Message{SessionID: "s", Text: "hi"})
		errCh <- err
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("Finish did not run before Append unblocked")
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("Append never started")
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

type finishWriter struct {
	memWriter
	once     sync.Once
	finished chan struct{}
}

func (f *finishWriter) Finish(ctx context.Context, final string) error {
	err := f.memWriter.Finish(ctx, final)
	f.once.Do(func() { close(f.finished) })
	return err
}

type gateHistory struct {
	*memHistory
	entered chan struct{}
	release chan struct{}
}

func (g *gateHistory) Append(ctx context.Context, id string, msgs ...session.Message) error {
	close(g.entered)
	select {
	case <-g.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return g.memHistory.Append(ctx, id, msgs...)
}
