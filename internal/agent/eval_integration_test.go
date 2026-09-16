//go:build integration

package agent_test

// Live behavioral eval — docs/evaluation.md gap 1. Never in `go test ./...`:
//
//	make integration-test              # sources .env, 3 runs per fixture
//	make integration-test EVAL_ARGS='-eval.n=10 -eval.only=scoop_at_2,spark_gym_no_workout'
//
// Each fixture under testdata/eval runs N times against the configured model
// with the shipped persona seed and canned tools; every run must pass. A rule
// that holds two times in three is a rule the persona is not carrying.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

var (
	evalN           = flag.Int("eval.n", 3, "runs per fixture; every run must pass")
	evalOnly        = flag.String("eval.only", "", "comma-separated fixture names to run (default all)")
	evalPersonaFlag = flag.String("eval.persona", "", "persona file to test instead of the shipped seed (bake-off)")
	evalVerbose     = flag.Bool("eval.v", false, "dump calls with args and the reply for passing runs too")
)

// evalSelected parses -eval.only; nil means every fixture.
func evalSelected() []string {
	if *evalOnly == "" {
		return nil
	}
	var names []string
	for _, n := range strings.Split(*evalOnly, ",") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// evalTurnTimeout bounds one turn (two or three completer rounds).
const evalTurnTimeout = 3 * time.Minute

func TestEval_Live(t *testing.T) {
	baseURL, apiKey, model := os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	if apiKey == "" || baseURL == "" || model == "" {
		t.Skip("LLM_BASE_URL, LLM_API_KEY, LLM_MODEL required; put them in .env and run make integration-test")
	}
	completer := provider.New(baseURL, apiKey, model)
	if *evalPersonaFlag != "" {
		evalPersonaPath = *evalPersonaFlag
	}
	evalLiveTools = evalLiveCatalog
	t.Logf("model %s at %s; %d run(s) per fixture; persona %s", model, baseURL, *evalN, evalPersonaPath)

	selected := evalSelected()
	var total evalTotals
	for _, fx := range loadEvalFixtures(t, evalFixtureDir) {
		if selected != nil && !slices.Contains(selected, fx.Name) {
			continue
		}
		t.Run(fx.Name, func(t *testing.T) {
			t.Log(fx.Why)
			var sub evalTotals
			for i := 1; i <= *evalN; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), evalTurnTimeout)
				out := runEvalFixture(ctx, t, completer, fx)
				fails := checkEval(ctx, out, fx.Expect)
				cancel()
				over := overBudget(out, fx.Expect)
				sub.add(out, over != "")
				if len(fails) > 0 {
					t.Errorf("run %d/%d FAIL: %s\n%s", i, *evalN, strings.Join(fails, "; "), describeEval(out))
					continue
				}
				if over != "" {
					over = " (" + over + ")"
				}
				t.Logf("run %d/%d ok%s: %s — %s", i, *evalN, over, describeCost(out), describeBatches(out))
				if *evalVerbose {
					t.Log(describeEval(out))
				}
			}
			t.Logf("%s: %s", fx.Name, sub.String())
			total.merge(sub)
		})
	}
	t.Logf("all fixtures: %s", total.String())
}

// evalLiveCatalog fetches the latest release of every server in
// testdata/eval/mcp.toml, boots them, and returns their published tool
// defs. Once per process; the first tools_from fixture pays for it. The
// binaries are closed as soon as tools/list is in hand — the defs are
// data, and no fixture ever calls the real thing.
var (
	liveCatalogOnce sync.Once
	liveCatalogDefs []provider.ToolDef
	liveCatalogErr  error
)

func evalLiveCatalog(t *testing.T) []provider.ToolDef {
	t.Helper()
	liveCatalogOnce.Do(func() { liveCatalogDefs, liveCatalogErr = fetchLiveCatalog(t) })
	if liveCatalogErr != nil {
		t.Fatalf("live MCP catalog: %v", liveCatalogErr)
	}
	return liveCatalogDefs
}

func fetchLiveCatalog(t *testing.T) ([]provider.ToolDef, error) {
	manifestPath := filepath.Join(evalFixtureDir, "mcp.toml")
	m, err := mcp.LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	binDir := os.Getenv("EVAL_MCP_BIN")
	if binDir == "" {
		binDir = filepath.Join(os.TempDir(), "gantry-eval-mcp")
	}
	res, err := mcp.FetchDownloads(m, mcp.FetchOptions{
		OutDir: binDir,
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
		Logf:   func(format string, args ...any) { t.Logf("tools-fetch: "+format, args...) },
	})
	if err != nil {
		return nil, fmt.Errorf("tools-fetch: %w", err)
	}
	t.Logf("live catalog: binaries in %s (installed %d, cached %d)", binDir, len(res.Installed), len(res.Skipped))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	host, err := mcp.Start(ctx, mcp.Options{
		ManifestPath: manifestPath,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = host.Close() }()
	for _, s := range host.ServerHealth() {
		if s.State == mcp.ServerSkipped {
			t.Logf("live catalog: %s skipped (%s): %s", s.Name, s.Reason, s.Note)
		}
	}
	defs := host.Tools()
	t.Logf("live catalog: %d tools from %s", len(defs), manifestPath)
	return defs, nil
}

// evalTotals is the cost roll-up the bake-off compares: mean rounds, mean
// prompt tokens per turn, and how many runs went over their round_budget.
type evalTotals struct {
	runs, rounds, prompt, completion, over int
}

func (e *evalTotals) add(out evalOutcome, overBudget bool) {
	e.runs++
	e.rounds += out.Rounds
	e.prompt += out.PromptTokens
	e.completion += out.CompletionTokens
	if overBudget {
		e.over++
	}
}

func (e *evalTotals) merge(o evalTotals) {
	e.runs += o.runs
	e.rounds += o.rounds
	e.prompt += o.prompt
	e.completion += o.completion
	e.over += o.over
}

func (e evalTotals) String() string {
	if e.runs == 0 {
		return "no runs"
	}
	n := float64(e.runs)
	s := fmt.Sprintf("%d runs, mean %.2f rounds, mean %.1fk prompt / %.0f completion tokens per turn",
		e.runs, float64(e.rounds)/n, float64(e.prompt)/n/1000, float64(e.completion)/n)
	if e.over > 0 {
		s += fmt.Sprintf(", %d over round budget", e.over)
	}
	return s
}
