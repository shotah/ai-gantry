package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

// A lone bubble must never pay the settle window — that would delay every
// single reply. Coalescing only kicks in once there is a turn to interrupt.
func TestCoalesce_LoneMessageRunsImmediately(t *testing.T) {
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		return &provider.Result{Content: "ok"}, nil
	}}
	const settle = 3 * time.Second
	a, err := agent.New(agent.Options{
		Completer:      fc,
		Sessions:       newMemHistory(),
		Model:          "m",
		CoalesceSettle: settle,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	reply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "hi"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if reply != "ok" {
		t.Fatalf("reply = %q", reply)
	}
	if elapsed >= settle {
		t.Fatalf("lone message waited %v — settle must not apply with nothing in flight", elapsed)
	}
}

// Chatty-Cathy burst: the first bubble runs, later bubbles land mid-turn and
// interrupt it, then settle into a single joined turn.
func TestCoalesce_JoinsBurstDuringInFlightTurn(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	var saw atomic.Value
	block := &gateCompleter{
		started: started,
		onComplete: func(ctx context.Context, req provider.Request) (*provider.Result, error) {
			if calls.Add(1) == 1 {
				<-ctx.Done() // hold the first turn open until it is interrupted
				return nil, ctx.Err()
			}
			for i := len(req.Messages) - 1; i >= 0; i-- {
				if req.Messages[i].Role == provider.RoleUser {
					saw.Store(req.Messages[i].Content)
					break
				}
			}
			return &provider.Result{Content: "ok"}, nil
		},
	}
	a, err := agent.New(agent.Options{
		Completer:      block,
		Sessions:       newMemHistory(),
		Model:          "m",
		CoalesceSettle: 60 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var replies [3]string
	send := func(i int, text string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reply, err := a.Handle(context.Background(), channel.Message{
				SessionID: "s",
				Text:      text,
			})
			if err != nil {
				t.Errorf("Handle %d: %v", i, err)
				return
			}
			replies[i] = reply
		}()
	}

	send(0, "pull yesterday's ride")
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first turn did not start")
	}
	for i, text := range []string{"from Garmin", "MTB"} {
		send(i+1, text)
		time.Sleep(10 * time.Millisecond) // stay inside the settle window
	}
	wg.Wait()

	var nonEmpty int
	for _, r := range replies {
		if r != "" {
			nonEmpty++
		}
	}
	if nonEmpty != 1 {
		t.Fatalf("want exactly one reply, got %d (%q)", nonEmpty, replies)
	}
	got, _ := saw.Load().(string)
	for _, want := range []string{"pull yesterday's ride", "from Garmin", "MTB"} {
		if !strings.Contains(got, want) {
			t.Fatalf("joined user text %q missing %q", got, want)
		}
	}
}

func TestCoalesce_InterruptsInFlight(t *testing.T) {
	started := make(chan struct{})
	var completeCount atomic.Int32
	block := &gateCompleter{
		started: started,
		onComplete: func(ctx context.Context, req provider.Request) (*provider.Result, error) {
			n := completeCount.Add(1)
			if n == 1 {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			var user string
			for i := len(req.Messages) - 1; i >= 0; i-- {
				if req.Messages[i].Role == provider.RoleUser {
					user = req.Messages[i].Content
					break
				}
			}
			return &provider.Result{Content: "joined:" + user}, nil
		},
	}
	a, err := agent.New(agent.Options{
		Completer:      block,
		Sessions:       newMemHistory(),
		Model:          "m",
		CoalesceSettle: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan struct{})
	var firstReply string
	go func() {
		defer close(firstDone)
		firstReply, _ = a.Handle(context.Background(), channel.Message{
			SessionID: "s",
			Text:      "first ask",
		})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first turn did not start")
	}

	secondReply, err := a.Handle(context.Background(), channel.Message{
		SessionID: "s",
		Text:      "and also this",
	})
	if err != nil {
		t.Fatal(err)
	}
	<-firstDone

	if firstReply != "" {
		t.Fatalf("interrupted first reply should be empty, got %q", firstReply)
	}
	if !strings.Contains(secondReply, "first ask") || !strings.Contains(secondReply, "and also this") {
		t.Fatalf("second reply should include both: %q", secondReply)
	}
}

// /cancel during the settle window must drop the buffered batch, so the joined
// turn never reaches the model. Pending state only exists after an interrupt.
func TestCoalesce_CancelClearsPending(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	block := &gateCompleter{
		started: started,
		onComplete: func(ctx context.Context, _ provider.Request) (*provider.Result, error) {
			if calls.Add(1) == 1 {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			t.Error("joined turn ran after /cancel cleared pending")
			return &provider.Result{Content: "should not happen"}, nil
		},
	}
	const settle = 200 * time.Millisecond
	a, err := agent.New(agent.Options{
		Completer:      block,
		Sessions:       newMemHistory(),
		Model:          "m",
		CoalesceSettle: settle,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "first ask"})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first turn did not start")
	}

	// Second bubble interrupts the in-flight turn and starts settling.
	secondDone := make(chan struct{})
	var secondReply string
	go func() {
		defer close(secondDone)
		secondReply, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "and also this"})
	}()
	time.Sleep(20 * time.Millisecond)

	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/cancel"})
	if err != nil {
		t.Fatal(err)
	}
	// The first turn was already interrupted by the second bubble, so only the
	// settle buffer is left to clear.
	if !strings.Contains(got, "nothing in progress") {
		t.Fatalf("expected idle cancel while only settling, got %q", got)
	}
	<-firstDone
	<-secondDone
	if secondReply != "" {
		t.Fatalf("cancelled batch should not reply, got %q", secondReply)
	}
	time.Sleep(2 * settle) // the settle timer would have fired by now
	if n := calls.Load(); n != 1 {
		t.Fatalf("model calls = %d, want 1 (only the interrupted turn)", n)
	}
}

type slowTools struct {
	defs    []provider.ToolDef
	started chan struct{}
	calls   *atomic.Int32
}

func (s *slowTools) Tools() []provider.ToolDef { return s.defs }

func (s *slowTools) ToolCount() int { return len(s.defs) }

func (s *slowTools) Call(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	s.calls.Add(1)
	select {
	case <-s.started:
	default:
		close(s.started)
	}
	time.Sleep(150 * time.Millisecond)
	return `{"ok":true}`, nil
}

func TestCoalesce_DoesNotInterruptAfterTools(t *testing.T) {
	toolStarted := make(chan struct{})
	var toolCalls atomic.Int32
	tools := &slowTools{
		defs:    []provider.ToolDef{{Name: "search__q", Parameters: map[string]any{"type": "object"}}},
		started: toolStarted,
		calls:   &toolCalls,
	}
	var n atomic.Int32
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		i := n.Add(1)
		if i == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "search__q", Arguments: `{}`},
			}}, nil
		}
		if i == 2 {
			return &provider.Result{Content: "first done"}, nil
		}
		var user string
		for j := len(req.Messages) - 1; j >= 0; j-- {
			if req.Messages[j].Role == provider.RoleUser {
				user = req.Messages[j].Content
				break
			}
		}
		return &provider.Result{Content: "second:" + user}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:      fc,
		Sessions:       newMemHistory(),
		Tools:          tools,
		Model:          "m",
		CoalesceSettle: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan struct{})
	var firstReply string
	go func() {
		defer close(firstDone)
		firstReply, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "search flights"})
	}()
	select {
	case <-toolStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("tool did not start")
	}
	secondReply, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "also check returns"})
	if err != nil {
		t.Fatal(err)
	}
	<-firstDone
	if firstReply != "first done" {
		t.Fatalf("first turn should complete without coalesce cancel, got %q", firstReply)
	}
	if toolCalls.Load() != 1 {
		t.Fatalf("toolCalls=%d want 1 (no reburn)", toolCalls.Load())
	}
	if !strings.Contains(secondReply, "also check returns") {
		t.Fatalf("second reply=%q", secondReply)
	}
}

func TestCoalesce_SkipsCron(t *testing.T) {
	started := make(chan struct{}, 1)
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		started <- struct{}{}
		return &provider.Result{Content: "cron-ok"}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:      fc,
		Sessions:       newMemHistory(),
		Model:          "m",
		CoalesceSettle: time.Hour, // would hang if cron waited on settle
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reply, err := a.Handle(ctx, channel.Message{
		SessionID: "s",
		Text:      "[cron] Scheduled job — do the thing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "cron-ok" {
		t.Fatalf("reply = %q", reply)
	}
	select {
	case <-started:
	default:
		t.Fatal("cron should bypass coalesce")
	}
}

// gateCompleter signals started once, then delegates to onComplete.
type gateCompleter struct {
	started    chan struct{}
	once       sync.Once
	onComplete func(context.Context, provider.Request) (*provider.Result, error)
}

func (g *gateCompleter) Complete(ctx context.Context, req provider.Request) (*provider.Result, error) {
	g.once.Do(func() { close(g.started) })
	return g.onComplete(ctx, req)
}
