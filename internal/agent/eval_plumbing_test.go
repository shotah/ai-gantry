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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

// evalManifest lists the servers a tools_from fixture may name; the
// integration run fetches and boots them for their real catalogs.
var evalManifest = filepath.Join(evalFixtureDir, "mcp.toml")

// evalLiveServers reads the eval manifest's server names (no network).
func evalLiveServers(t *testing.T) map[string]mcp.ServerSpec {
	t.Helper()
	m, err := mcp.LoadManifest(evalManifest)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]mcp.ServerSpec{}
	for _, s := range m.Servers {
		if s.DownloadURL == "" {
			t.Errorf("%s: server %q needs download_url so the eval can fetch its latest release", evalManifest, s.Name)
		}
		out[s.Name] = s
	}
	return out
}

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
	liveServers := evalLiveServers(t)
	seen := map[string]bool{}
	for _, fx := range fixtures {
		if seen[fx.Name] {
			t.Errorf("duplicate fixture name %q", fx.Name)
		}
		seen[fx.Name] = true
		if fx.Why == "" {
			t.Errorf("%s: why is empty", fx.Name)
		}
		if fx.Inbound == "" && fx.Spark == "" && fx.Cron == "" {
			t.Errorf("%s: needs inbound, spark, or cron", fx.Name)
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
		for _, server := range fx.ToolsFrom {
			if _, ok := liveServers[server]; !ok {
				t.Errorf("%s: tools_from %q is not a server in %s", fx.Name, server, evalManifest)
			}
			for _, tl := range fx.Tools {
				if strings.HasPrefix(tl.Name, server+"__") && (tl.Description != "" || tl.Params != nil) {
					t.Errorf("%s: %s comes from the live catalog; drop its description/params", fx.Name, tl.Name)
				}
			}
		}
		compileEvalRegexes(t, fx.Name, fx.Expect)
	}
}

// The live-catalog merge: real defs win, canned results attach by name, a
// name the live server does not publish is drift and fails before a model
// call, other servers pass through hand-written.
func TestMergeLiveTools(t *testing.T) {
	live := []provider.ToolDef{
		{Name: "rentals__listings_search", Description: "THRIFTY: one call", Parameters: map[string]any{"type": "object", "properties": map[string]any{"neighborhood": map[string]any{"type": "string"}}}},
		{Name: "rentals__account_get", Description: "FREE counter"},
		{Name: "flights__offers_search", Description: "not asked for"},
	}
	fixture := []evalTool{
		{Name: "rentals__listings_search", Result: `{"count":1}`},
		{Name: "google__calendar_list_events", Description: "hand-written", Result: "[]"},
	}
	got, err := mergeLiveTools(live, []string{"rentals"}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(got))
	for _, tl := range got {
		names = append(names, tl.Name)
	}
	if want := []string{"google__calendar_list_events", "rentals__account_get", "rentals__listings_search"}; !slices.Equal(names, want) {
		t.Fatalf("names=%v want %v", names, want)
	}
	for _, tl := range got {
		switch tl.Name {
		case "rentals__listings_search":
			if tl.Description != "THRIFTY: one call" || tl.Params == nil || tl.Result != `{"count":1}` {
				t.Fatalf("live def + canned result not merged: %+v", tl)
			}
		case "rentals__account_get":
			if tl.Result != "{}" {
				t.Fatalf("uncanned live tool should default to {}: %+v", tl)
			}
		case "google__calendar_list_events":
			if tl.Description != "hand-written" {
				t.Fatalf("other server should pass through: %+v", tl)
			}
		}
	}

	_, err = mergeLiveTools(live, []string{"rentals"}, []evalTool{{Name: "rentals__search", Result: "[]"}})
	if err == nil || !strings.Contains(err.Error(), `"rentals__search" is not in the live rentals catalog`) || !strings.Contains(err.Error(), "rentals__listings_search") {
		t.Fatalf("renamed tool should fail with the live names: %v", err)
	}
	if _, err := mergeLiveTools(live, []string{"garmin"}, nil); err == nil || !strings.Contains(err.Error(), "no live tools") {
		t.Fatalf("unconnected server should fail: %v", err)
	}
}

// max_calls: the metered-API gate counts calls matching name (+ args).
func TestCheckEval_MaxCalls(t *testing.T) {
	two := 2
	one := 1
	out := evalOutcome{Calls: []evalCall{
		{Name: "rentals__listings_search", Args: `{"neighborhood":"Ballard"}`},
		{Name: "rentals__listings_search", Args: `{"neighborhood":"Fremont"}`},
		{Name: "rentals__account_get", Args: `{}`},
	}}
	if fails := checkEval(context.Background(), out, evalExpect{ToolsCalled: []evalCallExpect{{Name: "rentals__listings_search", MaxCalls: &two}}}); len(fails) != 0 {
		t.Fatalf("two calls within max 2: %v", fails)
	}
	fails := checkEval(context.Background(), out, evalExpect{ToolsCalled: []evalCallExpect{{Name: "rentals__listings_search", MaxCalls: &one}}})
	if len(fails) != 1 || fails[0] != "tool rentals__listings_search called 2 times, max 1" {
		t.Fatalf("fails=%v", fails)
	}
	// ArgsRegex narrows the count.
	if fails := checkEval(context.Background(), out, evalExpect{ToolsCalled: []evalCallExpect{{Name: "rentals__listings_search", ArgsRegex: "Ballard", MaxCalls: &one}}}); len(fails) != 0 {
		t.Fatalf("one Ballard call within max 1: %v", fails)
	}
}

// prices_from_tools: a "$N" the tools never returned is an invented fact.
func TestCheckEval_PricesFromTools(t *testing.T) {
	yes := true
	calls := []evalCall{{Name: "flights__offers_search", Result: `{"offers":[{"price":218},{"price":189}],"usage":{"searches_left":88}}`}}
	ok := evalOutcome{Reply: "Alaska at $189 or United for $218.00 — both nonstop.", Calls: calls}
	if fails := checkEval(context.Background(), ok, evalExpect{PricesFromTools: &yes}); len(fails) != 0 {
		t.Fatalf("real prices flagged: %v", fails)
	}
	bad := evalOutcome{Reply: "Sep 25 is cheaper: $139 on Alaska and $ 149 on United; Sep 18 is $189.", Calls: calls}
	fails := checkEval(context.Background(), bad, evalExpect{PricesFromTools: &yes})
	if len(fails) != 2 || fails[0] != "invented price $139 (in no tool result or input)" || fails[1] != "invented price $149 (in no tool result or input)" {
		t.Fatalf("fails=%v", fails)
	}
	// Commas on either side do not matter, and the human's own number —
	// the budget in the cron prompt or an aim row — is given, not invented.
	rent := evalOutcome{
		Reply: "Two new 2BR under $2,400: 5417 NW 57th St at $2,295/mo",
		Calls: []evalCall{{Result: `{"price":2295}`}},
		Given: evalGiven(evalFixture{Cron: "new 2BR under $2400", Memory: []evalMemory{{Content: "under $2,400/mo by Nov 1"}}}, "under $2400"),
	}
	if fails := checkEval(context.Background(), rent, evalExpect{PricesFromTools: &yes}); len(fails) != 0 {
		t.Fatalf("given/comma price flagged: %v", fails)
	}
}

// cron fixtures wake with the same prefix cron.Runner puts on a scheduled job.
func TestEvalInboundText(t *testing.T) {
	now := time.Now()
	text, err := evalInboundText(evalFixture{Cron: "Daily rental check"}, now)
	if err != nil || !strings.HasPrefix(text, cron.JobUserPrefix) || !strings.HasSuffix(text, "Daily rental check") {
		t.Fatalf("cron text=%q err=%v", text, err)
	}
	text, err = evalInboundText(evalFixture{Inbound: "hi"}, now)
	if err != nil || text != "hi" {
		t.Fatalf("inbound text=%q err=%v", text, err)
	}
	if _, err := evalInboundText(evalFixture{Spark: "no such line"}, now); err == nil {
		t.Fatal("unknown spark line should error")
	}
}

// A tools_from fixture outside the integration tag is a hard error, never a
// silently hand-written schema.
func TestEvalHarness_ToolsFromNeedsLiveCatalog(t *testing.T) {
	if evalLiveTools != nil {
		t.Skip("live catalog wired")
	}
	fx := evalFixture{Name: "x", Inbound: "hi", ToolsFrom: []string{"rentals"}}
	_, err := mergeLiveTools(nil, fx.ToolsFrom, nil)
	if err == nil {
		t.Fatal("empty live catalog must fail")
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

// Reactions through the harness: a [react 👍] alone is a reaction, not
// silence, and satisfies 05's any_of and 12's react_only; text next to it
// fails react_only.
func TestEvalHarness_ReactToken(t *testing.T) {
	ctx := context.Background()
	fx := loadEvalFixture(t, filepath.Join(evalFixtureDir, "12_react_to_thanks.json"))
	sc := &scriptCompleter{res: []*provider.Result{{Content: "[react 👍]"}}}

	out := runEvalFixture(ctx, t, sc, fx)
	if out.Reaction != "👍" || out.Reply != "" || out.Silent {
		t.Fatalf("reaction=%q reply=%q silent=%v", out.Reaction, out.Reply, out.Silent)
	}
	if fails := checkEval(ctx, out, fx.Expect); len(fails) > 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
	five := loadEvalFixture(t, filepath.Join(evalFixtureDir, "05_thanks_sounds_good.json"))
	if fails := checkEval(ctx, out, five.Expect); len(fails) > 0 {
		t.Fatalf("05 must accept a reaction alone: %v", fails)
	}

	chatty := evalOutcome{Reaction: "👍", Reply: "You're welcome!", RawReply: "You're welcome!\n[react 👍]", mem: out.mem}
	fails := checkEval(ctx, chatty, fx.Expect)
	if len(fails) != 1 || !strings.Contains(fails[0], "no text") {
		t.Fatalf("want one react_only failure, got %v", fails)
	}
	none := evalOutcome{Reply: "Anytime.", RawReply: "Anytime.", mem: out.mem}
	fails = checkEval(ctx, none, fx.Expect)
	if len(fails) != 2 || !strings.Contains(fails[0], "expected a reaction") {
		t.Fatalf("want reaction + react_only failures, got %v", fails)
	}
}

// Their 👍 with nothing pending never reaches the model; waiting, it does.
func TestEvalHarness_ReactionTriage(t *testing.T) {
	ctx := context.Background()

	idle := loadEvalFixture(t, filepath.Join(evalFixtureDir, "13_reaction_idle_thumbs_up.json"))
	sc := &scriptCompleter{res: []*provider.Result{{Content: "should not be asked"}}}
	out := runEvalFixture(ctx, t, sc, idle)
	if fails := checkEval(ctx, out, idle.Expect); len(fails) > 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
	if len(sc.reqs) != 0 {
		t.Fatalf("model called %d times on an idle 👍", len(sc.reqs))
	}
	billed := evalOutcome{Rounds: 1, RawReply: "[silent]", Silent: true, mem: out.mem}
	if fails := checkEval(ctx, billed, idle.Expect); len(fails) != 1 || !strings.Contains(fails[0], "model rounds") {
		t.Fatalf("want a no_model_call failure, got %v", fails)
	}

	waiting := loadEvalFixture(t, filepath.Join(evalFixtureDir, "14_reaction_waiting_thumbs_up.json"))
	sc = &scriptCompleter{res: []*provider.Result{
		{ToolCalls: []provider.ToolCall{toolCall("c1", "google__calendar_create_event", map[string]any{
			"summary": "Gym", "start": "2026-09-17T07:00:00-07:00", "end": "2026-09-17T08:00:00-07:00",
		})}},
		{Content: "Thursday 7–8 blocked for the gym."},
	}}
	out = runEvalFixture(ctx, t, sc, waiting)
	if fails := checkEval(ctx, out, waiting.Expect); len(fails) > 0 {
		t.Fatalf("%v\n%s", fails, describeEval(out))
	}
	if len(sc.reqs) != 2 {
		t.Fatalf("waiting 👍 must reach the model, got %d rounds", len(sc.reqs))
	}
}
