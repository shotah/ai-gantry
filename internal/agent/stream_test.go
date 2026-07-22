package agent_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

type streamCompleter struct {
	mu    sync.Mutex
	parts []string
}

func (s *streamCompleter) Complete(ctx context.Context, req provider.Request) (*provider.Result, error) {
	return s.CompleteStream(ctx, req, nil)
}

func (s *streamCompleter) CompleteStream(_ context.Context, _ provider.Request, onText func(full string) error) (*provider.Result, error) {
	s.mu.Lock()
	parts := append([]string(nil), s.parts...)
	s.mu.Unlock()
	var full strings.Builder
	for _, p := range parts {
		full.WriteString(p)
		if onText != nil {
			if err := onText(full.String()); err != nil {
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
