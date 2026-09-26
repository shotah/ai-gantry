package cron

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PlannerService wires /planner and boot ensure to the session clock + the one daily job.
type PlannerService struct {
	Store *Store
	TZ    string
	// At is the operator default (DAILY_PLANNER_AT). Empty uses DefaultPlannerAt.
	At string
	// Prompt overrides the kernel body (tests). Empty uses DefaultDailyPlannerPrompt.
	Prompt string
}

// ProactiveEnabled reports whether the daily planner can run (cron store present).
func (s *PlannerService) ProactiveEnabled() bool {
	return s != nil && s.Store != nil
}

// DefaultAt is the operator clock (not the session override).
func (s *PlannerService) DefaultAt() string {
	if s == nil {
		return DefaultPlannerAt
	}
	at := strings.TrimSpace(s.At)
	if at == "" {
		return DefaultPlannerAt
	}
	hour, minute, err := ParsePlannerAt(at)
	if err != nil {
		return DefaultPlannerAt
	}
	return FormatPlannerAt(hour, minute)
}

// SessionAt is the raw override. Empty inherits the operator default.
func (s *PlannerService) SessionAt(ctx context.Context, sessionID string) (string, error) {
	if s == nil || s.Store == nil {
		return "", fmt.Errorf("planner: not configured")
	}
	return s.Store.PlannerAt(ctx, sessionID)
}

// ResolvedAt is the session clock, or DefaultAt. PlannerOff means off.
// Anything that is not a clock time inherits the operator default.
func (s *PlannerService) ResolvedAt(ctx context.Context, sessionID string) (string, error) {
	pref, err := s.SessionAt(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if pref == PlannerOff {
		return PlannerOff, nil
	}
	if pref != "" {
		if hour, minute, err := ParsePlannerAt(pref); err == nil {
			return FormatPlannerAt(hour, minute), nil
		}
	}
	return s.DefaultAt(), nil
}

// SetAt persists a session clock. Empty inherits the operator default; "0" is off.
func (s *PlannerService) SetAt(ctx context.Context, sessionID, at string) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("planner: not configured")
	}
	at = strings.TrimSpace(at)
	if at != "" && at != PlannerOff {
		hour, minute, err := ParsePlannerAt(at)
		if err != nil {
			return err
		}
		at = FormatPlannerAt(hour, minute)
	}
	return s.Store.SetPlannerAt(ctx, sessionID, at)
}

// EnsureFor installs the daily planner when the session has not opted out.
func (s *PlannerService) EnsureFor(ctx context.Context, delivery Delivery) (Job, bool, error) {
	if s == nil || s.Store == nil {
		return Job{}, false, fmt.Errorf("planner: not configured")
	}
	at, err := s.ResolvedAt(ctx, delivery.SessionID)
	if err != nil {
		return Job{}, false, err
	}
	if strings.TrimSpace(at) == "" || at == PlannerOff {
		_, _ = s.Store.CancelDailyPlanner(ctx, delivery.SessionID)
		return Job{}, false, nil
	}
	loc, err := time.LoadLocation(s.TZ)
	if err != nil {
		loc = time.UTC
	}
	parsed, err := ParsePlannerSchedule(at, loc, time.Now())
	if err != nil {
		return Job{}, false, err
	}
	prompt := strings.TrimSpace(s.Prompt)
	if prompt == "" {
		prompt = DefaultDailyPlannerPrompt
	}
	return s.Store.EnsureDailyPlanner(ctx, prompt, parsed, delivery)
}
