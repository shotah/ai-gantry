package agent_test

// Plumbing for the behavioral eval, run in `go test ./...` with a scripted
// Completer. Pins that the fixtures are well-formed and that the harness
// records what it should: tool calls through the recorder, cron jobs in the
// store, [wait] via the wait service, mcp_enable filtering, spark wake text.
// The live model is eval_integration_test.go.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/provider"
)

// scriptCompleter answers round i with res[i]; the last result repeats.
type scriptCompleter struct {
	mu   sync.Mutex
	reqs []provider.Request
	res  []*provider.Result
}

func (s *scriptCompleter) Complete(_ context.Context, req provider.Request) (*provider.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	i := len(s.reqs) - 1
	if i >= len(s.res) {
		i = len(s.res) - 1
	}
	return s.res[i], nil
}

func toolCall(id, name string, args map[string]any) provider.ToolCall {
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return provider.ToolCall{ID: id, Name: name, Arguments: string(raw)}
}

func hasToolDef(defs []provider.ToolDef, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}

func lastUserText(req provider.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == provider.RoleUser {
			return req.Messages[i].Content
		}
	}
	return ""
}

func compileEvalRegexes(t *testing.T, name string, e evalExpect) {
	t.Helper()
	for _, re := range []string{e.ReplyRegex, e.ReplyNot} {
		if re == "" {
			continue
		}
		if _, err := regexp.Compile(re); err != nil {
			t.Errorf("%s: bad regex %q: %v", name, re, err)
		}
	}
	for _, c := range e.ToolsCalled {
		if c.ArgsRegex == "" {
			continue
		}
		if _, err := regexp.Compile(c.ArgsRegex); err != nil {
			t.Errorf("%s: bad args_regex %q: %v", name, c.ArgsRegex, err)
		}
	}
	for _, alt := range e.AnyOf {
		compileEvalRegexes(t, name, alt)
	}
}

func TestEvalFixtures_WellFormed(t *testing.T) {
	fixtures := loadEvalFixtures(t, evalFixtureDir)
	if len(fixtures) < 7 {
		t.Fatalf("scenario table has 7 rows; %d fixtures", len(fixtures))
	}
	seen := map[string]bool{}
	for _, fx := range fixtures {
		if seen[fx.Name] {
			t.Errorf("duplicate fixture name %q", fx.Name)
		}
		seen[fx.Name] = true
		if fx.Why == "" {
			t.Errorf("%s: why is empty", fx.Name)
		}
		if fx.Inbound == "" && fx.Spark == "" {
			t.Errorf("%s: needs inbound or spark", fx.Name)
		}
		if fx.Spark != "" {
			if _, ok := sparkLine(fx.Spark); !ok {
				t.Errorf("%s: no DefaultSparkPrompt line contains %q", fx.Name, fx.Spark)
			}
		}
		for _, tl := range fx.Tools {
			if !strings.Contains(tl.Name, "__") {
				t.Errorf("%s: canned tool %q needs an MCP prefix (server__name)", fx.Name, tl.Name)
			}
		}
		compileEvalRegexes(t, fx.Name, fx.Expect)
	}
}

func TestExpandEvalClock(t *testing.T) {
	loc, _ := payloadClock()
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, loc)
	got := expandEvalClock("Sprint is at {{+150m}}; take the scoop at {{+120m}}. Slept {{-30m}}.", now)
	want := "Sprint is at 2:30PM; take the scoop at 2:00PM. Slept 11:30AM."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Canned tool results take the same clock, so a calendar stub stays in
	// the future whenever the eval runs.
	canned := newCannedTools([]evalTool{{Name: "google__calendar_list_events", Result: `[{"summary":"Dinner","start":"{{+90m}}"}]`}}, now)
	if out, _ := canned.Call(context.Background(), "google__calendar_list_events", nil); out != `[{"summary":"Dinner","start":"1:30PM"}]` {
		t.Fatalf("canned result not expanded: %q", out)
	}
}

// The scoop fixture: a scripted cron_schedule + [wait] passes; the recorder
// saw the builtin, the job landed in the window, the wait armed, the forced
// google schemas were published, and the persona under test is the seed.
func TestEvalHarness_ScoopSchedulesCron(t *testing.T) {
	ctx := context.Background()
	fx := loadEvalFixture(t, filepath.Join(evalFixtureDir, "01_scoop_at_2.json"))
	when := time.Now().Add(120 * time.Minute).Format(time.RFC3339)
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{toolCall("c1", "cron_schedule", map[string]any{
			"prompt": "Take the scoop — sprint in 30.", "when": when,
		})}},
		{Content: "Scoop reminder set. Want the sprint on the calendar too?\n[wait]"},
	}}

	out := runEvalFixture(ctx, t, sc, fx)

	if fails := checkEval(ctx, out, fx.Expect); len(fails) > 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
	if !out.Waiting {
		t.Fatalf("wait did not arm\n%s", describeEval(out))
	}
	if firstCall(out.Calls, "cron_schedule") != 0 {
		t.Fatalf("recorder missed cron_schedule\n%s", describeEval(out))
	}
	if out.Rounds != 2 || out.Calls[0].Round != 1 {
		t.Fatalf("rounds=%d call round=%d, want 2 and 1", out.Rounds, out.Calls[0].Round)
	}
	if got := describeBatches(out); got != "[cron_schedule] → reply" {
		t.Fatalf("batches %q", got)
	}
	one := 1
	if fails := checkEval(ctx, out, evalExpect{RoundBudget: &one}); len(fails) != 0 {
		t.Fatalf("round_budget must never fail a run: %v", fails)
	}
	if got := overBudget(out, evalExpect{RoundBudget: &one}); got != "over budget: 2 rounds, budget 1" {
		t.Fatalf("over-budget note %q", got)
	}
	two := 2
	if got := overBudget(out, evalExpect{RoundBudget: &two}); got != "" {
		t.Fatalf("inside budget should be quiet, got %q", got)
	}
	req := sc.reqs[0]
	for _, name := range []string{"cron_schedule", "memory_store", "self_note", "mcp_enable", "google__calendar_list_events"} {
		if !hasToolDef(req.Tools, name) {
			t.Errorf("round 1 did not publish %s", name)
		}
	}
	if u := lastUserText(req); strings.Contains(u, "{{") || !strings.Contains(u, "take the scoop at") {
		t.Errorf("clock placeholders not expanded: %q", u)
	}
	var persona string
	for _, m := range req.Messages {
		if m.Role == provider.RoleSystem && strings.Contains(m.Content, "## About you") {
			persona = m.Content
		}
	}
	if !strings.Contains(persona, "- **Name:** Kit") || !strings.Contains(persona, "- **Name:** Sam") {
		t.Errorf("persona under test is not the named seed")
	}
}

func TestEvalHarness_BareAckFails(t *testing.T) {
	ctx := context.Background()
	fx := loadEvalFixture(t, filepath.Join(evalFixtureDir, "01_scoop_at_2.json"))
	sc := &scriptCompleter{res: []*provider.Result{{Content: "Got it."}}}

	out := runEvalFixture(ctx, t, sc, fx)

	fails := checkEval(ctx, out, fx.Expect)
	if len(fails) != 2 {
		t.Fatalf("want reply_not + any_of failures, got %v", fails)
	}
	if !strings.Contains(fails[0], "should not match") || !strings.Contains(fails[1], "any_of") {
		t.Fatalf("unexpected failures %v", fails)
	}
}

// Off prefix: round 1 hides the google schema and stamps it off; after
// mcp_enable round 2 publishes it; the order check passes.
func TestEvalHarness_OffPrefixEnableThenCall(t *testing.T) {
	ctx := context.Background()
	fx := loadEvalFixture(t, filepath.Join(evalFixtureDir, "04_off_prefix_enable_then_call.json"))
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{toolCall("c1", "mcp_enable", map[string]any{"prefixes": []string{"google"}})}},
		{ToolCalls: []provider.ToolCall{toolCall("c2", "google__calendar_list_events", map[string]any{})}},
		{Content: "Sprint planning at 2:30, dentist at 5. Want a heads-up before the dentist?"},
	}}

	out := runEvalFixture(ctx, t, sc, fx)

	if fails := checkEval(ctx, out, fx.Expect); len(fails) > 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
	if hasToolDef(sc.reqs[0].Tools, "google__calendar_list_events") {
		t.Error("round 1 published an off prefix")
	}
	if !hasToolDef(sc.reqs[0].Tools, "mcp_enable") {
		t.Error("round 1 did not publish mcp_enable")
	}
	if !hasToolDef(sc.reqs[1].Tools, "google__calendar_list_events") {
		t.Error("round 2 did not publish the enabled prefix")
	}
	if got := describeBatches(out); got != "[mcp_enable] → [google__calendar_list_events] → reply" {
		t.Errorf("batches %q", got)
	}

	// Reversed order is caught.
	swapped := evalOutcome{Calls: []evalCall{
		{Name: "google__calendar_list_events"}, {Name: "mcp_enable", Args: `{"prefixes":["google"]}`},
	}, RawReply: "Sprint at 2:30.", mem: out.mem}
	fails := checkEval(ctx, swapped, fx.Expect)
	if len(fails) != 1 || !strings.HasPrefix(fails[0], "order:") {
		t.Fatalf("want one order failure, got %v", fails)
	}
}

// Spark wake: the turn is SparkPingPrefix + the gym line, Garmin is published
// (forced) and recorded, SELF.md seeded, not silent.
func TestEvalHarness_SparkWake(t *testing.T) {
	ctx := context.Background()
	fx := loadEvalFixture(t, filepath.Join(evalFixtureDir, "07_spark_gym_no_workout.json"))
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{toolCall("c1", "garmin__activities_list", map[string]any{})}},
		{Content: "Garmin is blank and the shoes are still by the door. Gym before lunch?"},
	}}

	out := runEvalFixture(ctx, t, sc, fx)

	if fails := checkEval(ctx, out, fx.Expect); len(fails) > 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
	u := lastUserText(sc.reqs[0])
	if !strings.Contains(u, cron.SparkTurnMarker) || !strings.Contains(u, "Gym / fitness aim") {
		t.Errorf("spark turn text wrong: %q", u)
	}
	var persona string
	for _, m := range sc.reqs[0].Messages {
		if m.Role == provider.RoleSystem && strings.Contains(m.Content, "## About you") {
			persona = m.Content
		}
	}
	if !strings.Contains(persona, "shoes by the door") {
		t.Error("SELF.md seed not in the persona")
	}

	// A [silent] spark is caught.
	quiet := &scriptCompleter{res: []*provider.Result{{Content: "[silent]"}}}
	out = runEvalFixture(ctx, t, quiet, fx)
	fails := checkEval(ctx, out, fx.Expect)
	if len(fails) == 0 || !out.Silent {
		t.Fatalf("silent spark should fail: %v\n%s", fails, describeEval(out))
	}
}

// pref/calendar with an unspecified kind is accepted under any kind the model
// plausibly picks.
func TestEvalHarness_MemoryAnyKind(t *testing.T) {
	ctx := context.Background()
	fx := loadEvalFixture(t, filepath.Join(evalFixtureDir, "02_get_something_on_it.json"))
	sc := &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{toolCall("c1", "memory_store", map[string]any{
			"kind": "fact", "subject": "pref/calendar", "content": "empty day → get something on it",
		})}},
		{Content: "Noted. Today is empty — lunch out or a training block?\n[wait]"},
	}}

	out := runEvalFixture(ctx, t, sc, fx)

	if fails := checkEval(ctx, out, fx.Expect); len(fails) > 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
}
