package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

type toolRoundResult struct {
	name string
	id   string
	out  string
	err  error
}

type toolRound struct {
	results    []toolRoundResult
	forceNames []string
	wall       time.Duration
}

// runToolRound executes one model-emitted tool batch. Independent calls run
// concurrently; results are appended in the original ToolCalls order so the
// provider still sees a matching tool_call_id sequence. Same-server stdio
// serializes inside the MCP host.
func (a *Agent) runToolRound(ctx context.Context, calls []provider.ToolCall, iter int, hasProgress bool, progress channel.ProgressWriter) (toolRound, bool) {
	n := len(calls)
	out := toolRound{results: make([]toolRoundResult, n)}
	if n == 0 {
		return out, false
	}
	if hasProgress && a.toolTrace == ToolTraceCompact {
		_ = progress.UpdateProgress(ctx, compactCallsHeader)
	}

	start := time.Now()
	if n == 1 {
		out.results[0] = a.execToolCall(ctx, calls[0], iter, hasProgress, progress)
	} else {
		var wg sync.WaitGroup
		wg.Add(n)
		for i, call := range calls {
			go func() {
				defer wg.Done()
				out.results[i] = a.execToolCall(ctx, call, iter, hasProgress, progress)
			}()
		}
		wg.Wait()
	}
	out.wall = time.Since(start)

	canceled := false
	for _, r := range out.results {
		if r.err == nil {
			continue
		}
		if errors.Is(r.err, context.Canceled) || ctx.Err() != nil {
			canceled = true
			continue
		}
		var unknown *mcp.UnknownToolError
		if errors.As(r.err, &unknown) && len(unknown.Candidates) > 0 && len(out.forceNames) == 0 {
			out.forceNames = unknown.Candidates
			a.log.Info("constraining retry to nearest tool names",
				"requested", unknown.Name,
				"candidates", len(unknown.Candidates),
			)
		}
	}
	return out, canceled
}

func (a *Agent) execToolCall(ctx context.Context, call provider.ToolCall, iter int, hasProgress bool, progress channel.ProgressWriter) toolRoundResult {
	a.log.Info("tool call",
		"name", call.Name,
		"id", call.ID,
		"iteration", iter+1,
	)
	if hasProgress && a.toolTrace == ToolTraceFull {
		_ = progress.UpdateProgress(ctx, toolProgressStart(call.Name))
	}
	args := json.RawMessage(call.Arguments)
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	toolStart := time.Now()
	text, err := a.tools.Call(ctx, call.Name, args)
	dur := time.Since(toolStart)
	if err != nil {
		text = fmt.Sprintf("tool error: %v", err)
		a.log.Warn("tool call failed", "name", call.Name, "dur_ms", dur.Milliseconds(), "err", err)
	} else {
		a.log.Info("tool done",
			"name", call.Name,
			"dur_ms", dur.Milliseconds(),
			"result_chars", len(text),
		)
	}
	if hasProgress {
		switch a.toolTrace {
		case ToolTraceFull:
			_ = progress.UpdateProgress(ctx, toolProgressDone(dur, len(text), err != nil))
		case ToolTraceCompact:
			mark := "✓"
			if err != nil {
				mark = "✗"
			}
			_ = progress.UpdateProgress(ctx, mark)
		}
	}
	return toolRoundResult{name: call.Name, id: call.ID, out: text, err: err}
}
