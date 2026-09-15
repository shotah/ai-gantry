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
	Spark   string `json:"spark,omitempty"`
	Surface string `json:"surface,omitempty"`
	Input   string `json:"input,omitempty"`
	// History is appended to the session before the turn (role user|assistant).
	History []evalHistory `json:"history,omitempty"`
	Memory  []evalMemory  `json:"memory,omitempty"`
	// Self is SELF.md body bullets ("- ..." lines). Empty file when absent.
	Self string `json:"self,omitempty"`
	// Tools are canned MCP tools. Result is returned verbatim on every call.
	Tools []evalTool `json:"tools,omitempty"`
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
	Order        []string           `json:"order,omitempty"`
	ReplyRegex   string             `json:"reply_regex,omitempty"`
	ReplyNot     string             `json:"reply_not_regex,omitempty"`
	MaxQuestions *int               `json:"max_questions,omitempty"`
	Wait         *bool              `json:"wait,omitempty"`
	Silent       *bool              `json:"silent,omitempty"`
	Memory       []evalMemoryExpect `json:"memory,omitempty"`
	// CronWithin: at least one job for this session lands in [after, before]
	// minutes from the turn.
	CronWithin *evalWindow  `json:"cron_within,omitempty"`
	AnyOf      []evalExpect `json:"any_of,omitempty"`
}

type evalCallExpect struct {
	Name      string `json:"name"`
	ArgsRegex string `json:"args_regex,omitempty"`
}

type evalMemoryExpect struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
}

type evalWindow struct {
	AfterMin  int `json:"after_min"`
	BeforeMin int `json:"before_min"`
}

// evalCall is one recorded tool call, builtin or MCP.
type evalCall struct {
	Name string
	Args string
}

// evalOutcome is what one turn produced, gathered from the recorder, the wait
// hook, and the stores.
type evalOutcome struct {
	Calls    []evalCall
	Reply    string // what Handle returned (wait tokens stripped)
	RawReply string // what the wait hook saw
	Waiting  bool
	Silent   bool
	Jobs     []cron.Job
	Started  time.Time
	mem      memory.Memory
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

func newCannedTools(tools []evalTool) *cannedTools {
	c := &cannedTools{results: map[string]string{}}
	for _, tl := range tools {
		params := tl.Params
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		c.defs = append(c.defs, provider.ToolDef{Name: tl.Name, Description: tl.Description, Parameters: params})
		c.results[tl.Name] = tl.Result
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

// recordingTools wraps the outermost composite so builtins and MCP calls are
// both seen, then delegates.
type recordingTools struct {
	inner agent.Tools
	mu    sync.Mutex
	calls []evalCall
}

func (r *recordingTools) Tools() []provider.ToolDef { return r.inner.Tools() }

func (r *recordingTools) ToolCount() int { return r.inner.ToolCount() }

func (r *recordingTools) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, evalCall{Name: name, Args: string(args)})
	r.mu.Unlock()
	return r.inner.Call(ctx, name, args)
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
	seed, err := os.ReadFile(evalPersonaSeed)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	text := strings.Replace(string(seed), "- **Name:** (pick one)", "- **Name:** Kit", 1)
	text = strings.Replace(text, "- **Name:** Your Name", "- **Name:** Sam", 1)
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
	// recorder on top.
	var tools agent.Tools = newCannedTools(fx.Tools)
	tools = memory.Composite{Memory: memory.Tools{Backend: mem}, Other: tools}
	tools = cron.Composite{Cron: cron.Tools{Store: jobs, TZ: evalTZ, Memory: mem}, Other: tools}
	tools = selfnote.Composite{Self: selfnote.Tools{Store: self}, Other: tools}
	base := tools
	tools = mcpenable.Composite{
		Enable: mcpenable.Tools{Store: enable, Index: func() []string { return mcpenable.Index(base.Tools()) }},
		Other:  base,
	}
	rec := &recordingTools{inner: tools}
	wait := &recordingWait{inner: &cron.WaitService{State: sessions, Jobs: jobs, TZ: evalTZ}}

	a, err := agent.New(agent.Options{
		Persona:     personaText,
		Completer:   completer,
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

	started := time.Now().In(loc)
	text := expandEvalClock(fx.Inbound, started)
	if fx.Spark != "" {
		line, ok := sparkLine(fx.Spark)
		if !ok {
			t.Fatalf("%s: no spark line contains %q", fx.Name, fx.Spark)
		}
		text = cron.SparkPingPrefix + line
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
		Calls:    rec.snapshot(),
		Reply:    reply,
		RawReply: raw,
		Waiting:  state.WaitingForReply,
		Silent:   strings.TrimSpace(reply) == "" || cron.IsSilentReply(raw),
		Jobs:     list,
		Started:  started,
		mem:      mem,
	}
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
	var re *regexp.Regexp
	if c.ArgsRegex != "" {
		re = regexp.MustCompile(c.ArgsRegex)
	}
	for _, call := range calls {
		if call.Name != c.Name {
			continue
		}
		if re == nil || re.MatchString(call.Args) {
			return true
		}
	}
	return false
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

// describeEval is the failure dump: calls, then the reply.
func describeEval(out evalOutcome) string {
	var b strings.Builder
	b.WriteString("--- tool calls ---\n")
	if len(out.Calls) == 0 {
		b.WriteString("(none)\n")
	}
	for _, c := range out.Calls {
		fmt.Fprintf(&b, "%s %s\n", c.Name, c.Args)
	}
	fmt.Fprintf(&b, "--- waiting=%v silent=%v jobs=%s ---\n", out.Waiting, out.Silent, describeJobs(out))
	b.WriteString("--- reply ---\n")
	b.WriteString(out.RawReply)
	if out.RawReply == "" {
		b.WriteString(out.Reply)
	}
	b.WriteString("\n")
	return b.String()
}
