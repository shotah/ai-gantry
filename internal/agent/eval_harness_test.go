package agent_test

// Behavioral eval harness (docs/evaluation.md, gap 1).
//
// A fixture is one turn against the shipped persona seed with a real session,
// memory, cron, mcp_enable, and self-note store — the same composition as
// cmd/gantry/run.go — and canned MCP tools. Only the Completer varies: the
// live eval (eval_integration_test.go, build tag `integration`) plugs in the
// real provider; the plumbing test below plugs in a scripted fake so the
// harness itself is covered by `go test ./...`.
//
// Expectations are about shape, not prose: which tools were called and with
// what, whether [wait] armed, whether the turn was [silent], which memory rows
// and cron jobs landed. The sentence is the part that changes run to run.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/mcpenable"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/persona"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/selfnote"
	"github.com/shotah/ai-gantry/internal/session"
)

// evalFixtureDir holds one JSON per scenario row in docs/persona_doc_goals.md.
const evalFixtureDir = "testdata/eval"

// evalPersonaSeed is the file under test — the shipped seed, not a stub.
const evalPersonaSeed = "../../examples/persona/PERSONA.example.md"

// evalPersonaPath is the persona actually loaded; the live eval overrides it
// with -eval.persona to bake off a candidate seed against the fixtures.
var evalPersonaPath = evalPersonaSeed

const evalTZ = "America/Los_Angeles"

// evalFixture is one scenario. Time placeholders in Inbound are expanded at
// run time against the real clock: `{{+120m}}` → local "3:04PM" 120 minutes
// from now. cron.Tools parses schedules against time.Now(), so the harness
// clock is not frozen here.
type evalFixture struct {
	Name string `json:"name"`
	Why  string `json:"why"`
	// Inbound is the human's text. Ignored when Spark is set.
	Inbound string `json:"inbound"`
	// Spark selects a line of cron.DefaultSparkPrompt by substring and sends it
	// as a wake turn (cron.SparkPingPrefix + line). Empty means a human turn.
	Spark string `json:"spark,omitempty"`
	// Cron sends the prompt as a scheduled-job wake (cron.JobUserPrefix +
	// prompt) — the shape of a daily "check X" job the model scheduled.
	Cron    string `json:"cron,omitempty"`
	Surface string `json:"surface,omitempty"`
	Input   string `json:"input,omitempty"`
	// History is appended to the session before the turn (role user|assistant).
	History []evalHistory `json:"history,omitempty"`
	Memory  []evalMemory  `json:"memory,omitempty"`
	// Self is SELF.md body bullets ("- ..." lines). Empty file when absent.
	Self string `json:"self,omitempty"`
	// Tools are canned MCP tools. Result is returned verbatim on every call.
	Tools []evalTool `json:"tools,omitempty"`
	// ToolsFrom names servers in testdata/eval/mcp.toml whose real catalog —
	// names, descriptions, schemas from the latest release binary's
	// tools/list — replaces hand-written defs. Every tool the server
	// publishes is published here; fixture Tools for those servers carry
	// only name + canned result, and a name the live catalog lacks fails
	// before any model call. Needs the integration tag (network).
	ToolsFrom []string `json:"tools_from,omitempty"`
	// Force lists MCP prefixes published without mcp_enable (like MCP_ENABLE_FORCE).
	Force  []string   `json:"force,omitempty"`
	Expect evalExpect `json:"expect"`
}

type evalHistory struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type evalMemory struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Content string `json:"content"`
}

type evalTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Params      map[string]any `json:"params,omitempty"`
	Result      string         `json:"result"`
}

// evalExpect is the shape contract. Every set field must hold. AnyOf holds
// when at least one alternative holds in full.
type evalExpect struct {
	ToolsCalled    []evalCallExpect `json:"tools_called,omitempty"`
	ToolsNotCalled []string         `json:"tools_not_called,omitempty"`
	// Order lists tool names whose first calls must appear in this order.
	Order        []string `json:"order,omitempty"`
	ReplyRegex   string   `json:"reply_regex,omitempty"`
	ReplyNot     string   `json:"reply_not_regex,omitempty"`
	MaxQuestions *int     `json:"max_questions,omitempty"`
	// RoundBudget is the completer rounds the rule needs (tool rounds + the
	// reply). Going over is reported, never failed: the gate is on missing
	// work, and a model that does something extra and useful is not wrong.
	RoundBudget *int  `json:"round_budget,omitempty"`
	Wait        *bool `json:"wait,omitempty"`
	Silent      *bool `json:"silent,omitempty"`
	// PricesFromTools: every "$N" in the reply must appear in some tool
	// result this turn. The "never invent live facts" gate — a model that
	// gets the same fares back for two dates and makes up a cheaper pair
	// to tell them apart fails here, not in the human's inbox.
	PricesFromTools *bool              `json:"prices_from_tools,omitempty"`
	Memory          []evalMemoryExpect `json:"memory,omitempty"`
	// CronWithin: at least one job for this session lands in [after, before]
	// minutes from the turn.
	CronWithin *evalWindow  `json:"cron_within,omitempty"`
	AnyOf      []evalExpect `json:"any_of,omitempty"`
}

type evalCallExpect struct {
	Name      string `json:"name"`
	ArgsRegex string `json:"args_regex,omitempty"`
	// MaxCalls caps how often this tool may be called in the turn (matching
	// ArgsRegex when set). The metered-API gate: one search, not five.
	MaxCalls *int `json:"max_calls,omitempty"`
}

type evalMemoryExpect struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
}

type evalWindow struct {
	AfterMin  int `json:"after_min"`
	BeforeMin int `json:"before_min"`
}

// evalCall is one recorded tool call, builtin or MCP. Round is the completer
// round that requested it (1-based), so a batch is the calls sharing a round.
type evalCall struct {
	Name   string
	Args   string
	Round  int
	Result string // what the tool returned (empty on error)
}

// evalOutcome is what one turn produced, gathered from the recorder, the wait
// hook, the completer counter, and the stores.
type evalOutcome struct {
	Calls    []evalCall
	Reply    string // what Handle returned (wait tokens stripped)
	RawReply string // what the wait hook saw
	Waiting  bool
	Silent   bool
	Jobs     []cron.Job
	Started  time.Time
	// Rounds is completer calls for the turn; the last one is the reply.
	Rounds           int
	PromptTokens     int // native usage summed over rounds; 0 when the provider omits it
	CompletionTokens int
	// Given is everything the model was handed besides tool results: the
	// turn text, seeded memory, history, SELF.md. A "$2,400" that restates
	// the human's own budget is not invented.
	Given string
	mem   memory.Memory
}

func loadEvalFixtures(t *testing.T, dir string) []evalFixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var out []evalFixture
	for _, p := range paths {
		out = append(out, loadEvalFixture(t, p))
	}
	if len(out) == 0 {
		t.Fatalf("no fixtures under %s", dir)
	}
	return out
}

func loadEvalFixture(t *testing.T, path string) evalFixture {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fx evalFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if fx.Name == "" {
		fx.Name = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	return fx
}

var evalClockRe = regexp.MustCompile(`\{\{([+-]\d+)m\}\}`)

// expandEvalClock replaces {{+Nm}} with the local clock N minutes from now.
func expandEvalClock(text string, now time.Time) string {
	return evalClockRe.ReplaceAllStringFunc(text, func(m string) string {
		var n int
		if _, err := fmt.Sscanf(evalClockRe.FindStringSubmatch(m)[1], "%d", &n); err != nil {
			return m
		}
		return now.Add(time.Duration(n) * time.Minute).Format("3:04PM")
	})
}

// evalInboundText is the turn's text: a spark wake, a scheduled-job wake,
// or the human's line — the same prefixes cron.Runner puts on the wire.
func evalInboundText(fx evalFixture, now time.Time) (string, error) {
	switch {
	case fx.Spark != "":
		line, ok := sparkLine(fx.Spark)
		if !ok {
			return "", fmt.Errorf("%s: no spark line contains %q", fx.Name, fx.Spark)
		}
		return cron.SparkPingPrefix + line, nil
	case fx.Cron != "":
		return cron.JobUserPrefix + expandEvalClock(fx.Cron, now), nil
	default:
		return expandEvalClock(fx.Inbound, now), nil
	}
}

// evalLiveTools returns the real catalogs of testdata/eval/mcp.toml servers.
// Set by the integration test (network: tools-fetch + boot); nil in plain
// `go test`, where a tools_from fixture is a hard error rather than a
// silently faked schema.
var evalLiveTools func(t *testing.T) []provider.ToolDef

// mergeLiveTools builds the canned set for a tools_from fixture: every tool
// the live servers publish, with the fixture's canned result where it gave
// one and "{}" where it did not. A fixture result for a tool the live
// catalog does not have is the drift signal — a sibling release renamed
// or dropped it — and fails here, before a single model call. Fixture
// tools on other servers pass through hand-written.
func mergeLiveTools(live []provider.ToolDef, servers []string, fixture []evalTool) ([]evalTool, error) {
	byName := map[string]evalTool{}
	for _, tl := range fixture {
		byName[tl.Name] = tl
	}
	var out []evalTool
	covered := map[string]bool{}
	for _, server := range servers {
		prefix := server + "__"
		n := 0
		for _, def := range live {
			if !strings.HasPrefix(def.Name, prefix) {
				continue
			}
			n++
			result := "{}"
			if tl, ok := byName[def.Name]; ok {
				result = tl.Result
			}
			covered[def.Name] = true
			out = append(out, evalTool{Name: def.Name, Description: def.Description, Params: def.Parameters, Result: result})
		}
		if n == 0 {
			return nil, fmt.Errorf("tools_from %q: no live tools (server not connected or not in testdata/eval/mcp.toml)", server)
		}
		for _, tl := range fixture {
			if strings.HasPrefix(tl.Name, prefix) && !covered[tl.Name] {
				return nil, fmt.Errorf("fixture tool %q is not in the live %s catalog (have %s)", tl.Name, server, strings.Join(liveNames(live, prefix), ", "))
			}
		}
	}
	for _, tl := range fixture {
		if !covered[tl.Name] {
			out = append(out, tl)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func liveNames(live []provider.ToolDef, prefix string) []string {
	var names []string
	for _, def := range live {
		if strings.HasPrefix(def.Name, prefix) {
			names = append(names, def.Name)
		}
	}
	sort.Strings(names)
	return names
}

// sparkLine picks the cron.DefaultSparkPrompt line containing needle.
func sparkLine(needle string) (string, bool) {
	for _, line := range cron.ParseSparkPrompts(cron.DefaultSparkPrompt) {
		if strings.Contains(line, needle) {
			return line, true
		}
	}
	return "", false
}

// cannedTools is the innermost Tools: MCP stand-ins that answer verbatim.
type cannedTools struct {
	defs    []provider.ToolDef
	results map[string]string
}

func newCannedTools(tools []evalTool, now time.Time) *cannedTools {
	c := &cannedTools{results: map[string]string{}}
	for _, tl := range tools {
		params := tl.Params
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		c.defs = append(c.defs, provider.ToolDef{Name: tl.Name, Description: tl.Description, Parameters: params})
		c.results[tl.Name] = expandEvalClock(tl.Result, now)
	}
	return c
}

func (c *cannedTools) Tools() []provider.ToolDef { return c.defs }

func (c *cannedTools) ToolCount() int { return len(c.defs) }

func (c *cannedTools) Call(_ context.Context, name string, _ json.RawMessage) (string, error) {
	if out, ok := c.results[name]; ok {
		return out, nil
	}
	return "", fmt.Errorf("eval: no canned tool %q", name)
}

// countingCompleter counts rounds and sums native usage; the recorder reads
// the round so each tool call is tagged with the batch it came from.
type countingCompleter struct {
	inner  provider.Completer
	mu     sync.Mutex
	rounds int
	usage  provider.Usage
}

func (c *countingCompleter) Complete(ctx context.Context, req provider.Request) (*provider.Result, error) {
	c.mu.Lock()
	c.rounds++
	c.mu.Unlock()
	res, err := c.inner.Complete(ctx, req)
	if res != nil {
		c.mu.Lock()
		c.usage.PromptTokens += res.Usage.PromptTokens
		c.usage.CompletionTokens += res.Usage.CompletionTokens
		c.mu.Unlock()
	}
	return res, err
}

func (c *countingCompleter) round() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rounds
}

// recordingTools wraps the outermost composite so builtins and MCP calls are
// both seen, then delegates.
type recordingTools struct {
	inner agent.Tools
	round func() int
	mu    sync.Mutex
	calls []evalCall
}

func (r *recordingTools) Tools() []provider.ToolDef { return r.inner.Tools() }

func (r *recordingTools) ToolCount() int { return r.inner.ToolCount() }

func (r *recordingTools) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, evalCall{Name: name, Args: string(args), Round: r.round()})
	i := len(r.calls) - 1
	r.mu.Unlock()
	out, err := r.inner.Call(ctx, name, args)
	if err == nil {
		r.mu.Lock()
		r.calls[i].Result = out
		r.mu.Unlock()
	}
	return out, err
}

func (r *recordingTools) snapshot() []evalCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]evalCall(nil), r.calls...)
}

// recordingWait sees the reply before wait tokens are stripped, then
// delegates to the real WaitService so waiting_for_reply is written.
type recordingWait struct {
	inner *cron.WaitService
	mu    sync.Mutex
	raw   string
}

func (w *recordingWait) OnUserTurn(ctx context.Context, sessionID string) error {
	return w.inner.OnUserTurn(ctx, sessionID)
}

func (w *recordingWait) AfterReply(ctx context.Context, delivery cron.Delivery, userText, reply string) error {
	w.mu.Lock()
	w.raw = reply
	w.mu.Unlock()
	return w.inner.AfterReply(ctx, delivery, userText, reply)
}

func (w *recordingWait) last() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.raw
}

// evalPersona copies the shipped seed into a fresh persona dir, names the
// agent, and loads it the way boot does (kernel sections stamped).
func evalPersona(t *testing.T, dir string) string {
	t.Helper()
	seed, err := os.ReadFile(evalPersonaPath)
	if err != nil {
		t.Fatalf("read seed %s: %v", evalPersonaPath, err)
	}
	text := strings.Replace(string(seed), "- **Name:** (pick one)", "- **Name:** Kit", 1)
	text = strings.Replace(text, "- **Name:** Your Name", "- **Name:** Sam", 1)
	// A real city in evalTZ: a flight search needs an origin, and "City,
	// Region" would earn a fair "from where?" instead of the search.
	text = strings.Replace(text, "- **Location:** City, Region", "- **Location:** Seattle, Washington", 1)
	if err := os.WriteFile(filepath.Join(dir, persona.FilePersona), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := persona.SyncKernel(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := persona.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// runEvalFixture builds a fresh world, runs the one turn, and gathers the
// outcome. The stores are closed when the test ends.
func runEvalFixture(ctx context.Context, t *testing.T, completer provider.Completer, fx evalFixture) evalOutcome {
	t.Helper()
	loc, err := time.LoadLocation(evalTZ)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	personaDir := filepath.Join(root, "persona")
	dataDir := filepath.Join(root, "data")
	for _, d := range []string{personaDir, dataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if fx.Self != "" {
		if err := os.WriteFile(filepath.Join(personaDir, selfnote.FileName), []byte(fx.Self+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	personaText := evalPersona(t, personaDir)

	// One gantry.db handle for every store, as run.go does. A second handle
	// on the same file (memory.Open) makes a parallel tool batch fight over
	// the write lock — SQLITE_BUSY that production never sees.
	sessions, err := session.Open(dataDir, 50, 100000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessions.Close() })
	mem, err := memory.OpenDB(sessions.DB())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range fx.Memory {
		if _, err := mem.Store(ctx, row.Kind, row.Subject, row.Content); err != nil {
			t.Fatalf("seed memory %s/%s: %v", row.Kind, row.Subject, err)
		}
	}
	jobs, err := cron.OpenDB(sessions.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	enable, err := mcpenable.OpenDB(sessions.DB())
	if err != nil {
		t.Fatal(err)
	}
	self, err := selfnote.Open(personaDir)
	if err != nil {
		t.Fatal(err)
	}

	sessionID := "eval-" + fx.Name
	for _, h := range fx.History {
		role := session.RoleUser
		if h.Role == "assistant" {
			role = session.RoleAssistant
		}
		if err := sessions.Append(ctx, sessionID, session.Message{Role: role, Content: h.Text}); err != nil {
			t.Fatal(err)
		}
	}

	// Same stack as cmd/gantry/run.go, canned MCP host at the bottom,
	// recorder on top. Canned results take the same {{+Nm}} clock as the
	// inbound text so a "dinner at 7" fixture is still ahead at 9pm.
	started := time.Now().In(loc)
	canned := fx.Tools
	if len(fx.ToolsFrom) > 0 {
		if evalLiveTools == nil {
			t.Fatalf("%s: tools_from needs the live MCP catalog; run under the integration tag", fx.Name)
		}
		canned, err = mergeLiveTools(evalLiveTools(t), fx.ToolsFrom, fx.Tools)
		if err != nil {
			t.Fatalf("%s: %v", fx.Name, err)
		}
	}
	var tools agent.Tools = newCannedTools(canned, started)
	tools = memory.Composite{Memory: memory.Tools{Backend: mem}, Other: tools}
	tools = cron.Composite{Cron: cron.Tools{Store: jobs, TZ: evalTZ, Memory: mem}, Other: tools}
	tools = selfnote.Composite{Self: selfnote.Tools{Store: self}, Other: tools}
	base := tools
	tools = mcpenable.Composite{
		Enable: mcpenable.Tools{Store: enable, Index: func() []string { return mcpenable.Index(base.Tools()) }},
		Other:  base,
	}
	counter := &countingCompleter{inner: completer}
	rec := &recordingTools{inner: tools, round: counter.round}
	wait := &recordingWait{inner: &cron.WaitService{State: sessions, Jobs: jobs, TZ: evalTZ}}

	a, err := agent.New(agent.Options{
		Persona:     personaText,
		Completer:   counter,
		Sessions:    sessions,
		Memory:      mem,
		Tools:       rec,
		Wakes:       jobs,
		Wait:        wait,
		SelfNotes:   self,
		Enable:      enable,
		EnableForce: mcpenable.Force{Prefixes: fx.Force},
		Model:       "eval",
		Location:    loc,
		TZName:      evalTZ,
	})
	if err != nil {
		t.Fatal(err)
	}

	text, err := evalInboundText(fx, started)
	if err != nil {
		t.Fatal(err)
	}
	msg := channel.Message{
		SessionID: sessionID,
		UserID:    "eval",
		Text:      text,
		Surface:   fx.Surface,
		Input:     fx.Input,
	}
	reply, err := a.Handle(ctx, msg)
	if err != nil {
		t.Fatalf("%s: Handle: %v", fx.Name, err)
	}

	state, err := sessions.TalkState(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := jobs.ListSession(ctx, sessionID, false)
	if err != nil {
		t.Fatal(err)
	}
	raw := wait.last()
	return evalOutcome{
		Calls:            rec.snapshot(),
		Reply:            reply,
		RawReply:         raw,
		Waiting:          state.WaitingForReply,
		Silent:           strings.TrimSpace(reply) == "" || cron.IsSilentReply(raw),
		Jobs:             list,
		Started:          started,
		Rounds:           counter.round(),
		PromptTokens:     counter.usage.PromptTokens,
		CompletionTokens: counter.usage.CompletionTokens,
		Given:            evalGiven(fx, text),
		mem:              mem,
	}
}

// evalGiven joins the non-tool inputs of a turn for inventedPrices.
func evalGiven(fx evalFixture, text string) string {
	var b strings.Builder
	b.WriteString(text)
	b.WriteByte('\n')
	b.WriteString(fx.Self)
	b.WriteByte('\n')
	for _, m := range fx.Memory {
		b.WriteString(m.Content)
		b.WriteByte('\n')
	}
	for _, h := range fx.History {
		b.WriteString(h.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// checkEval returns one line per failed expectation. Empty means pass.
func checkEval(ctx context.Context, out evalOutcome, want evalExpect) []string {
	var fails []string
	reply := out.RawReply
	if reply == "" {
		reply = out.Reply
	}

	for _, c := range want.ToolsCalled {
		if !calledMatching(out.Calls, c) {
			fails = append(fails, fmt.Sprintf("expected tool %s%s", c.Name, argsNote(c.ArgsRegex)))
			continue
		}
		if c.MaxCalls != nil {
			if n := countMatching(out.Calls, c); n > *c.MaxCalls {
				fails = append(fails, fmt.Sprintf("tool %s called %d times, max %d", c.Name, n, *c.MaxCalls))
			}
		}
	}
	for _, name := range want.ToolsNotCalled {
		if calledMatching(out.Calls, evalCallExpect{Name: name}) {
			fails = append(fails, "unexpected tool "+name)
		}
	}
	if len(want.Order) > 1 {
		last := -1
		for _, name := range want.Order {
			i := firstCall(out.Calls, name)
			if i < 0 {
				fails = append(fails, "order: "+name+" never called")
				break
			}
			if i <= last {
				fails = append(fails, "order: "+name+" before "+strings.Join(want.Order, " → "))
				break
			}
			last = i
		}
	}
	if want.ReplyRegex != "" && !regexp.MustCompile(want.ReplyRegex).MatchString(reply) {
		fails = append(fails, "reply should match /"+want.ReplyRegex+"/")
	}
	if want.ReplyNot != "" && regexp.MustCompile(want.ReplyNot).MatchString(reply) {
		fails = append(fails, "reply should not match /"+want.ReplyNot+"/")
	}
	if want.MaxQuestions != nil {
		if n := strings.Count(reply, "?") + strings.Count(reply, "？"); n > *want.MaxQuestions {
			fails = append(fails, fmt.Sprintf("%d questions, max %d", n, *want.MaxQuestions))
		}
	}
	if want.Wait != nil && out.Waiting != *want.Wait {
		fails = append(fails, fmt.Sprintf("waiting_for_reply=%v, want %v", out.Waiting, *want.Wait))
	}
	if want.Silent != nil && out.Silent != *want.Silent {
		fails = append(fails, fmt.Sprintf("silent=%v, want %v", out.Silent, *want.Silent))
	}
	if want.PricesFromTools != nil && *want.PricesFromTools {
		for _, p := range inventedPrices(reply, out) {
			fails = append(fails, "invented price $"+p+" (in no tool result or input)")
		}
	}
	for _, m := range want.Memory {
		if !memoryRowExists(ctx, out.mem, m) {
			fails = append(fails, "expected memory row "+m.Kind+" "+m.Subject)
		}
	}
	if w := want.CronWithin; w != nil && !jobWithin(out, *w) {
		fails = append(fails, fmt.Sprintf("no cron job %d–%d min out (have %s)", w.AfterMin, w.BeforeMin, describeJobs(out)))
	}
	if len(want.AnyOf) > 0 {
		var alts []string
		for i, alt := range want.AnyOf {
			sub := checkEval(ctx, out, alt)
			if len(sub) == 0 {
				alts = nil
				break
			}
			alts = append(alts, fmt.Sprintf("alt %d: %s", i+1, strings.Join(sub, "; ")))
		}
		if len(alts) > 0 {
			fails = append(fails, "none of any_of held — "+strings.Join(alts, " | "))
		}
	}
	return fails
}

// memoryRowExists checks a live row by subject; empty Kind accepts any of the
// kinds a model plausibly picks for it.
func memoryRowExists(ctx context.Context, mem memory.Memory, m evalMemoryExpect) bool {
	kinds := []string{m.Kind}
	if m.Kind == "" {
		kinds = []string{memory.KindPreference, memory.KindFact, memory.KindInsight}
	}
	for _, kind := range kinds {
		if _, ok, err := mem.ActiveByKindSubject(ctx, kind, m.Subject); err == nil && ok {
			return true
		}
	}
	return false
}

func calledMatching(calls []evalCall, c evalCallExpect) bool {
	return countMatching(calls, c) > 0
}

var evalPriceRe = regexp.MustCompile(`\$\s?(\d[\d,]*(?:\.\d+)?)`)

// inventedPrices returns the "$N" figures in reply whose digits appear in
// no tool result and nowhere in the turn's inputs (out.Given). Commas are
// ignored on both sides ($2,295 vs 2295).
func inventedPrices(reply string, out evalOutcome) []string {
	var results strings.Builder
	for _, c := range out.Calls {
		results.WriteString(strings.ReplaceAll(c.Result, ",", ""))
		results.WriteByte('\n')
	}
	results.WriteString(strings.ReplaceAll(out.Given, ",", ""))
	haystack := results.String()
	var invented []string
	seen := map[string]bool{}
	for _, m := range evalPriceRe.FindAllStringSubmatch(reply, -1) {
		raw := m[1]
		digits := strings.ReplaceAll(raw, ",", "")
		digits = strings.TrimSuffix(strings.TrimSuffix(digits, ".00"), ".0")
		if seen[digits] || strings.Contains(haystack, digits) {
			continue
		}
		seen[digits] = true
		invented = append(invented, raw)
	}
	return invented
}

func countMatching(calls []evalCall, c evalCallExpect) int {
	var re *regexp.Regexp
	if c.ArgsRegex != "" {
		re = regexp.MustCompile(c.ArgsRegex)
	}
	n := 0
	for _, call := range calls {
		if call.Name != c.Name {
			continue
		}
		if re == nil || re.MatchString(call.Args) {
			n++
		}
	}
	return n
}

func firstCall(calls []evalCall, name string) int {
	for i, c := range calls {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func argsNote(re string) string {
	if re == "" {
		return ""
	}
	return " with args /" + re + "/"
}

func jobWithin(out evalOutcome, w evalWindow) bool {
	for _, j := range out.Jobs {
		if j.Kind == cron.KindFollowUp {
			continue // wait pokes are not the reminder
		}
		d := j.NextRunAt.Sub(out.Started)
		if d >= time.Duration(w.AfterMin)*time.Minute && d <= time.Duration(w.BeforeMin)*time.Minute {
			return true
		}
	}
	return false
}

func describeJobs(out evalOutcome) string {
	if len(out.Jobs) == 0 {
		return "none"
	}
	var parts []string
	for _, j := range out.Jobs {
		parts = append(parts, fmt.Sprintf("%s@+%dm", j.Kind, int(j.NextRunAt.Sub(out.Started).Minutes())))
	}
	return strings.Join(parts, ", ")
}

// overBudget is the cost note for a run that took more rounds than the
// fixture's round_budget — printed beside a passing run, never a failure.
// Empty when there is no budget or the run stayed inside it.
func overBudget(out evalOutcome, want evalExpect) string {
	if want.RoundBudget == nil || out.Rounds <= *want.RoundBudget {
		return ""
	}
	return fmt.Sprintf("over budget: %d rounds, budget %d", out.Rounds, *want.RoundBudget)
}

// describeBatches is the turn's shape: tool calls grouped by completer
// round, then the reply. "[a b] → [c] → reply" is three rounds; the
// arrows are the serial cost.
func describeBatches(out evalOutcome) string {
	var parts []string
	for r := 1; r <= out.Rounds; r++ {
		var names []string
		for _, c := range out.Calls {
			if c.Round == r {
				names = append(names, c.Name)
			}
		}
		if len(names) > 0 {
			parts = append(parts, "["+strings.Join(names, " ")+"]")
		}
	}
	parts = append(parts, "reply")
	return strings.Join(parts, " → ")
}

// describeCost is the per-run cost line: rounds and native tokens when the
// provider reports them.
func describeCost(out evalOutcome) string {
	if out.PromptTokens == 0 {
		return fmt.Sprintf("%d rounds", out.Rounds)
	}
	return fmt.Sprintf("%d rounds, %.1fk prompt / %d completion tokens", out.Rounds, float64(out.PromptTokens)/1000, out.CompletionTokens)
}

// describeEval is the failure dump: calls, then the reply.
func describeEval(out evalOutcome) string {
	var b strings.Builder
	b.WriteString("--- tool calls ---\n")
	if len(out.Calls) == 0 {
		b.WriteString("(none)\n")
	}
	for _, c := range out.Calls {
		fmt.Fprintf(&b, "r%d %s %s\n", c.Round, c.Name, c.Args)
	}
	fmt.Fprintf(&b, "--- %s: %s ---\n", describeCost(out), describeBatches(out))
	fmt.Fprintf(&b, "--- waiting=%v silent=%v jobs=%s ---\n", out.Waiting, out.Silent, describeJobs(out))
	b.WriteString("--- reply ---\n")
	b.WriteString(out.RawReply)
	if out.RawReply == "" {
		b.WriteString(out.Reply)
	}
	b.WriteString("\n")
	return b.String()
}
