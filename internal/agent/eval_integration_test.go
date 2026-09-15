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
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/provider"
)

var (
	evalN    = flag.Int("eval.n", 3, "runs per fixture; every run must pass")
	evalOnly = flag.String("eval.only", "", "comma-separated fixture names to run (default all)")
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
	t.Logf("model %s at %s; %d run(s) per fixture", model, baseURL, *evalN)

	selected := evalSelected()
	for _, fx := range loadEvalFixtures(t, evalFixtureDir) {
		if selected != nil && !slices.Contains(selected, fx.Name) {
			continue
		}
		t.Run(fx.Name, func(t *testing.T) {
			t.Log(fx.Why)
			for i := 1; i <= *evalN; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), evalTurnTimeout)
				out := runEvalFixture(ctx, t, completer, fx)
				fails := checkEval(ctx, out, fx.Expect)
				cancel()
				if len(fails) > 0 {
					t.Errorf("run %d/%d FAIL: %s\n%s", i, *evalN, strings.Join(fails, "; "), describeEval(out))
					continue
				}
				t.Logf("run %d/%d ok: %s", i, *evalN, callNames(out))
			}
		})
	}
}

func callNames(out evalOutcome) string {
	if len(out.Calls) == 0 {
		return "no tools"
	}
	names := make([]string, 0, len(out.Calls))
	for _, c := range out.Calls {
		names = append(names, c.Name)
	}
	return strings.Join(names, " → ")
}
