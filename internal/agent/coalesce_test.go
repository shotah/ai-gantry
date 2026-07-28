package agent_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestCoalesce_JoinsBurst(t *testing.T) {
	var saw atomic.Value
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == provider.RoleUser {
				saw.Store(req.Messages[i].Content)
				break
			}
		}
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:      fc,
		Sessions:       newMemHistory(),
		Model:          "m",
		CoalesceSettle: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var replies [3]string
	for i, text := range []string{"pull yesterday's ride", "from Garmin", "MTB"} {
		wg.Add(1)
		go func(i int, text string) {
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
		}(i, text)
		time.Sleep(5 * time.Millisecond) // keep burst inside settle window
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

func TestCoalesce_CancelClearsPending(t *testing.T) {
	fc := &fakeCompleter{fn: func(provider.Request) (*provider.Result, error) {
		t.Fatal("model should not run after /cancel cleared pending")
		return nil, nil
	}}
	a, err := agent.New(agent.Options{
		Completer:      fc,
		Sessions:       newMemHistory(),
		Model:          "m",
		CoalesceSettle: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "pending ask"})
	}()
	time.Sleep(20 * time.Millisecond)
	got, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/cancel"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "nothing in progress") {
		t.Fatalf("expected idle cancel while only settling, got %q", got)
	}
	<-done
	time.Sleep(250 * time.Millisecond) // settle would have fired if not cleared
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
