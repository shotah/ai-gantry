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
	"github.com/shotah/ai-gantry/internal/session"
)

// A lone bubble must never pay the settle window — that would delay every
// single reply. Coalescing only kicks in once there is a turn to steer.
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

// Chatty-Cathy burst: the first bubble runs; later bubbles settle into one
// steer on that same turn. The first Handle owns the reply.
func TestCoalesce_JoinsBurstDuringInFlightTurn(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	var saw atomic.Value
	block := &gateCompleter{
		started: started,
		onComplete: func(ctx context.Context, req provider.Request) (*provider.Result, error) {
			if calls.Add(1) == 1 {
				<-ctx.Done() // hold until Completer-only cancel
				return nil, ctx.Err()
			}
			saw.Store(allUserText(req.Messages))
			return &provider.Result{Content: "ok"}, nil
		},
	}
	hist := newMemHistory()
	a, err := agent.New(agent.Options{
		Completer:      block,
		Sessions:       hist,
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

	if replies[0] != "ok" {
		t.Fatalf("first Handle should own the reply, got %q", replies[0])
	}
	if replies[1] != "" || replies[2] != "" {
		t.Fatalf("follow-ups should return empty, got %q", replies)
	}
	got, _ := saw.Load().(string)
	for _, want := range []string{"pull yesterday's ride", "from Garmin", "MTB"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt user text %q missing %q", got, want)
		}
	}
	if !strings.Contains(got, "[steer]") {
		t.Fatalf("prompt missing [steer] marker: %q", got)
	}
	assertOneUserTurn(t, hist, "s", "pull yesterday's ride", "from Garmin", "MTB")
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
			return &provider.Result{Content: "joined:" + lastUser(req.Messages)}, nil
		},
	}
	hist := newMemHistory()
	w := &progressWriter{}
	a, err := agent.New(agent.Options{
		Completer:      block,
		Sessions:       hist,
		Model:          "m",
		CoalesceSettle: 40 * time.Millisecond,
		ToolTrace:      agent.ToolTraceFull,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan struct{})
	var firstReply string
	go func() {
		defer close(firstDone)
		firstReply, _ = a.Handle(channel.WithReplyWriter(context.Background(), w), channel.Message{
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

	if secondReply != "" {
		t.Fatalf("follow-up Handle should return empty, got %q", secondReply)
	}
	if !strings.Contains(firstReply, "and also this") {
		t.Fatalf("first Handle should include the steer, got %q", firstReply)
	}
	var sawRedirect bool
	for _, n := range w.traced() {
		if strings.Contains(n, "redirect:") && strings.Contains(n, "and also this") {
			sawRedirect = true
		}
	}
	if !sawRedirect {
		t.Fatalf("want redirect progress line, got %v", w.traced())
	}
	assertOneUserTurn(t, hist, "s", "first ask", "and also this")
}

// /cancel during the settle window aborts the still-in-flight first turn
// and drops the buffered steer. Pending state is not "idle".
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
			t.Error("Completer ran again after /cancel")
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
	var firstReply string
	go func() {
		defer close(firstDone)
		firstReply, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "first ask"})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first turn did not start")
	}

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
	if !strings.Contains(got, "cancelled") {
		t.Fatalf("expected cancel of in-flight turn, got %q", got)
	}
	<-firstDone
	<-secondDone
	if firstReply != "" {
		t.Fatalf("cancelled first Handle should be empty, got %q", firstReply)
	}
	if secondReply != "" {
		t.Fatalf("cancelled steer should not reply, got %q", secondReply)
	}
	time.Sleep(2 * settle) // the settle timer would have fired by now
	if n := calls.Load(); n != 1 {
		t.Fatalf("model calls = %d, want 1 (only the interrupted Completer)", n)
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

func TestCoalesce_SteersKeepToolResults(t *testing.T) {
	toolStarted := make(chan struct{})
	var toolCalls atomic.Int32
	tools := &slowTools{
		defs:    []provider.ToolDef{{Name: "search__q", Parameters: map[string]any{"type": "object"}}},
		started: toolStarted,
		calls:   &toolCalls,
	}
	raw := json.RawMessage(`{"id":"c1","type":"function","function":{"name":"search__q","arguments":"{}"},"extra_content":{"google":{"thought_signature":"sig-keep"}}}`)
	var n atomic.Int32
	var secondReq atomic.Value
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		i := n.Add(1)
		if i == 1 {
			return &provider.Result{ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "search__q", Arguments: `{}`, Raw: raw},
			}}, nil
		}
		secondReq.Store(cloneMessages(req.Messages))
		return &provider.Result{Content: "steered done"}, nil
	}}
	hist := newMemHistory()
	a, err := agent.New(agent.Options{
		Completer:      fc,
		Sessions:       hist,
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
	if firstReply != "steered done" {
		t.Fatalf("first Handle should finish the steered turn, got %q", firstReply)
	}
	if secondReply != "" {
		t.Fatalf("follow-up should return empty, got %q", secondReply)
	}
	if toolCalls.Load() != 1 {
		t.Fatalf("toolCalls=%d want 1 (no reburn)", toolCalls.Load())
	}
	msgs, _ := secondReq.Load().([]provider.Message)
	if !messagesContainRole(msgs, provider.RoleTool, `{"ok":true}`) {
		t.Fatalf("second Completer missing tool result: %+v", msgs)
	}
	if !messagesContainRole(msgs, provider.RoleUser, "[steer] also check returns") {
		t.Fatalf("second Completer missing steer: %+v", msgs)
	}
	if !messagesContainRaw(msgs, "sig-keep") {
		t.Fatalf("thought_signature dropped on steered retry: %+v", msgs)
	}
	assertOneUserTurn(t, hist, "s", "search flights", "also check returns")
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

func TestCoalesce_SkipsWatch(t *testing.T) {
	started := make(chan struct{}, 1)
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		started <- struct{}{}
		return &provider.Result{Content: "watch-ok"}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:      fc,
		Sessions:       newMemHistory(),
		Model:          "m",
		CoalesceSettle: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reply, err := a.Handle(ctx, channel.Message{
		SessionID: "s",
		Text:      "[watch] New items from a subscription.\n\n- id=nws-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "watch-ok" {
		t.Fatalf("reply = %q", reply)
	}
	select {
	case <-started:
	default:
		t.Fatal("watch should bypass coalesce")
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

func lastUser(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

func allUserText(msgs []provider.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role == provider.RoleUser {
			b.WriteString(m.Content)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func cloneMessages(in []provider.Message) []provider.Message {
	out := make([]provider.Message, len(in))
	copy(out, in)
	return out
}

func messagesContainRole(msgs []provider.Message, role provider.Role, sub string) bool {
	for _, m := range msgs {
		if m.Role == role && strings.Contains(m.Content, sub) {
			return true
		}
	}
	return false
}

func messagesContainRaw(msgs []provider.Message, sub string) bool {
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if strings.Contains(string(tc.Raw), sub) {
				return true
			}
		}
	}
	return false
}

func assertOneUserTurn(t *testing.T, hist *memHistory, sessionID string, parts ...string) {
	t.Helper()
	msgs, err := hist.Messages(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var users []session.Message
	for _, m := range msgs {
		if m.Role == session.RoleUser {
			users = append(users, m)
		}
	}
	if len(users) != 1 {
		t.Fatalf("history user turns = %d, want 1: %+v", len(users), msgs)
	}
	for _, p := range parts {
		if !strings.Contains(users[0].Content, p) {
			t.Fatalf("history user text %q missing %q", users[0].Content, p)
		}
	}
}
