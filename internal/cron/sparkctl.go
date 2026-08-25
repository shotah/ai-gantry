package cron

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SparkService wires /spark|/engagement and boot ensure to session prefs + the planner.
type SparkService struct {
	Store  *Store
	TZ     string
	Prompt string
	// StartHour/EndHour override the seed window when EndHour > StartHour (tests).
	StartHour int
	EndHour   int
}

// ProactiveEnabled reports whether spark can run (cron store present).
func (s *SparkService) ProactiveEnabled() bool {
	return s != nil && s.Store != nil
}

// DefaultQty is the built-in daily count (not an env var).
func (s *SparkService) DefaultQty() string {
	return DefaultSparkQty
}

// Window is the local hour range used when seeding pings.
func (s *SparkService) Window() (startHour, endHour int) {
	if s != nil && s.EndHour > s.StartHour {
		return s.StartHour, s.EndHour
	}
	return DefaultSparkStartHour, DefaultSparkEndHour
}

// SessionQty is the raw override (empty = inherit default).
func (s *SparkService) SessionQty(ctx context.Context, sessionID string) (string, error) {
	if s == nil || s.Store == nil {
		return "", fmt.Errorf("spark: not configured")
	}
	return s.Store.SparkQty(ctx, sessionID)
}

// ResolvedQty is session override or DefaultSparkQty. "0" means off.
func (s *SparkService) ResolvedQty(ctx context.Context, sessionID string) (string, error) {
	pref, err := s.SessionQty(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if pref == "0" {
		return "0", nil
	}
	if pref != "" {
		return pref, nil
	}
	return DefaultSparkQty, nil
}

// SetQty persists a session override. Empty inherits default; "0" is off.
func (s *SparkService) SetQty(ctx context.Context, sessionID, qty string) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("spark: not configured")
	}
	return s.Store.SetSparkQty(ctx, sessionID, qty)
}

// EnsureFor seeds the spark planner when the session has not opted out.
func (s *SparkService) EnsureFor(ctx context.Context, delivery Delivery) (Job, bool, error) {
	if s == nil || s.Store == nil {
		return Job{}, false, fmt.Errorf("spark: not configured")
	}
	qty, err := s.ResolvedQty(ctx, delivery.SessionID)
	if err != nil {
		return Job{}, false, err
	}
	if strings.TrimSpace(qty) == "" || qty == "0" {
		_, _ = s.Store.CancelSparkPlannerAndPings(ctx, delivery.SessionID)
		return Job{}, false, nil
	}
	loc, err := time.LoadLocation(s.TZ)
	if err != nil {
		loc = time.UTC
	}
	start, end := s.Window()
	when := fmt.Sprintf("%s@%02d-%02d", qty, start, end)
	parsed, err := ParseSparkSchedule(when, start, end, loc, time.Now())
	if err != nil {
		return Job{}, false, err
	}
	prompt := strings.TrimSpace(s.Prompt)
	if prompt == "" {
		prompt = DefaultSparkPrompt
	}
	return s.Store.EnsureSpark(ctx, prompt, parsed, delivery)
}
