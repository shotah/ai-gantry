package examples

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/provider"
)

// PlannerPrompt is stored on the examples planner / ping rows; fire-time content
// is built by BuildPingPrompt from the live tool catalog.
const PlannerPrompt = "capability example ping"

// Service wires seed picking to cron ensure/cancel and session prefs.
type Service struct {
	Store     *cron.Store
	Qty       string // EXAMPLES_QTY; empty or "0" disables proactive ensure
	StartHour int
	EndHour   int
	TZ        string
	Tools     func() []provider.ToolDef // live catalog; nil-safe
}

// ProactiveEnabled reports whether operator config allows examples pings.
func (s *Service) ProactiveEnabled() bool {
	if s == nil {
		return false
	}
	q := strings.TrimSpace(s.Qty)
	return q != "" && q != "0"
}

// SuggestPrompt returns a polish prompt for one eligible seed.
func (s *Service) SuggestPrompt() (prompt string, seed Seed, ok bool) {
	defs := s.toolDefs()
	seed, ok = Pick(defs)
	if !ok {
		return "", Seed{}, false
	}
	return PolishPrompt(seed), seed, true
}

// BuildPingPrompt is used by the cron runner at examples_ping fire time.
func (s *Service) BuildPingPrompt(_ context.Context) string {
	if s == nil {
		return NoMatchMessage
	}
	prompt, seed, ok := s.SuggestPrompt()
	if !ok {
		return "No multi-step recipes match the connected tools. Skip this ping quietly (one short sentence). " + OffHint
	}
	_ = seed
	return prompt
}

// SetEnabled persists /examples on|off and cancels jobs when turning off.
func (s *Service) SetEnabled(ctx context.Context, sessionID string, on bool) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("examples: not configured")
	}
	if err := s.Store.SetExamplesEnabled(ctx, sessionID, on); err != nil {
		return err
	}
	if !on {
		_, _ = s.Store.CancelExamplesPlannerAndPings(ctx, sessionID)
	}
	return nil
}

// EnsureFor seeds the examples planner for a delivery when proactive + pref allow.
func (s *Service) EnsureFor(ctx context.Context, delivery cron.Delivery) (cron.Job, bool, error) {
	if s == nil || s.Store == nil {
		return cron.Job{}, false, fmt.Errorf("examples: not configured")
	}
	if !s.ProactiveEnabled() {
		return cron.Job{}, false, nil
	}
	ok, err := s.Store.ExamplesEnabled(ctx, delivery.SessionID)
	if err != nil {
		return cron.Job{}, false, err
	}
	if !ok {
		return cron.Job{}, false, nil
	}
	loc, err := time.LoadLocation(s.TZ)
	if err != nil {
		loc = time.UTC
	}
	when := fmt.Sprintf("%s@%02d-%02d", strings.TrimSpace(s.Qty), s.StartHour, s.EndHour)
	parsed, err := cron.ParseExamplesSchedule(when, s.StartHour, s.EndHour, loc, time.Now())
	if err != nil {
		return cron.Job{}, false, err
	}
	return s.Store.EnsureExamples(ctx, PlannerPrompt, parsed, delivery)
}

func (s *Service) toolDefs() []provider.ToolDef {
	if s == nil || s.Tools == nil {
		return nil
	}
	return s.Tools()
}
