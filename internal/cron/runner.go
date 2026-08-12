package cron

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// DefaultTick is how often the runner polls for due jobs.
const DefaultTick = 15 * time.Second

// DefaultSparkSkipRecent is how long a recent user message suppresses a spark ping.
const DefaultSparkSkipRecent = 15 * time.Minute

// RecentUserActivity reports whether the human messaged recently (spark barge-in guard).
type RecentUserActivity interface {
	UserActiveSince(ctx context.Context, sessionID string, since time.Time) (bool, error)
}

// ExamplePromptBuilder builds the live inventory-aware prompt for examples_ping.
type ExamplePromptBuilder interface {
	BuildPingPrompt(ctx context.Context) string
}

// Runner wakes due jobs, runs the agent, and pushes replies.
type Runner struct {
	Store    *Store
	Handle   channel.Handler
	Pusher   channel.Pusher
	Interval time.Duration
	Logger   *slog.Logger
	// Recent is optional; when set, spark_ping / examples_ping defer if the user chatted recently.
	Recent RecentUserActivity
	// SparkSkipRecent defaults to DefaultSparkSkipRecent when <= 0.
	SparkSkipRecent time.Duration
	// ExamplesSkipRecent defaults to SparkSkipRecent / DefaultSparkSkipRecent when <= 0.
	ExamplesSkipRecent time.Duration
	// Examples is optional; when set, examples_ping prompts are built at fire time.
	Examples ExamplePromptBuilder
}

// Start polls until ctx is cancelled. Jobs run serially (overlap skipped via Claim).
func (r *Runner) Start(ctx context.Context) {
	if r == nil || r.Store == nil || r.Handle == nil || r.Pusher == nil {
		return
	}
	log := r.Logger
	if log == nil {
		log = slog.Default()
	}
	interval := r.Interval
	if interval <= 0 {
		interval = DefaultTick
	}
	if n, err := r.Store.ClearStaleRunning(ctx); err != nil {
		log.Warn("cron clear stale running failed", "err", err)
	} else if n > 0 {
		log.Info("cron cleared stale running flags", "count", n)
	}
	log.Info("cron runner started", "interval", interval.String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.poll(ctx, log)
	for {
		select {
		case <-ctx.Done():
			log.Info("cron runner stopped")
			return
		case <-ticker.C:
			r.poll(ctx, log)
		}
	}
}

func (r *Runner) poll(ctx context.Context, log *slog.Logger) {
	now := time.Now().UTC()
	jobs, err := r.Store.Due(ctx, now, 5)
	if err != nil {
		log.Warn("cron due query failed", "err", err)
		return
	}
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return
		}
		ok, err := r.Store.Claim(ctx, job.ID, now)
		if err != nil {
			log.Warn("cron claim failed", "id", job.ID, "err", err)
			continue
		}
		if !ok {
			continue
		}
		r.runOne(ctx, log, job)
	}
}

func (r *Runner) runOne(ctx context.Context, log *slog.Logger, job Job) {
	log.Info("cron job firing", "id", job.ID, "kind", job.Kind)

	if job.Kind == KindSpark {
		r.runSparkPlanner(ctx, log, job)
		return
	}
	if job.Kind == KindExamples {
		r.runExamplesPlanner(ctx, log, job)
		return
	}

	if (job.Kind == KindSparkPing || job.Kind == KindExamplesPing) && r.Recent != nil {
		skipFor := r.skipRecentFor(job.Kind)
		since := time.Now().UTC().Add(-skipFor)
		active, err := r.Recent.UserActiveSince(ctx, job.SessionID, since)
		if err != nil {
			log.Warn("cron recent-chat check failed", "id", job.ID, "kind", job.Kind, "err", err)
		} else if active {
			// One defer; if still chatting on retry, drop the ping (day already planned).
			if strings.HasPrefix(job.LastError, "skipped:") {
				log.Info("cron ping skipped after defer (recent chat); dropping",
					"id", job.ID, "kind", job.Kind, "session_id", job.SessionID)
				_ = r.Store.Finish(ctx, job, nil)
				return
			}
			until := time.Now().UTC().Add(skipFor)
			log.Info("cron ping deferred (recent chat)",
				"id", job.ID, "kind", job.Kind, "session_id", job.SessionID, "until", until.Format(time.RFC3339))
			_ = r.Store.Defer(ctx, job.ID, until, "skipped: user active in last "+skipFor.String())
			return
		}
	}

	prefix := "[cron] Scheduled job — do the following and reply with the result for the user:\n\n"
	prompt := job.Prompt
	handleCtx := ctx
	switch job.Kind {
	case KindSparkPing:
		prefix = "[cron] Spark of life — check in with the human now:\n\n"
		prompt = PickSparkPrompt(job.Prompt)
		handleCtx = channel.WithNoTools(ctx)
	case KindExamplesPing:
		prefix = "[cron] Capability example — inspire the human with one concrete idea (propose only):\n\n"
		if r.Examples != nil {
			prompt = r.Examples.BuildPingPrompt(ctx)
		}
		handleCtx = channel.WithNoTools(ctx)
	}
	text := prefix + prompt
	msg := channel.Message{
		SessionID: job.SessionID,
		UserID:    job.UserID,
		ChatID:    job.ChatID,
		ThreadID:  job.ThreadID,
		Text:      text,
	}
	reply, err := r.Handle(handleCtx, msg)
	if err != nil {
		log.Warn("cron job handle failed", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	if reply != "" {
		if err := r.Pusher.Push(ctx, channel.Outbound{
			SessionID: job.SessionID,
			UserID:    job.UserID,
			ChatID:    job.ChatID,
			ThreadID:  job.ThreadID,
			Text:      reply,
		}); err != nil {
			log.Warn("cron push failed", "id", job.ID, "err", err)
			_ = r.Store.Finish(ctx, job, fmt.Errorf("push: %w", err))
			return
		}
	}
	if err := r.Store.Finish(ctx, job, nil); err != nil {
		log.Warn("cron finish failed", "id", job.ID, "err", err)
	}
}

func (r *Runner) skipRecentFor(kind string) time.Duration {
	switch kind {
	case KindExamplesPing:
		if r.ExamplesSkipRecent > 0 {
			return r.ExamplesSkipRecent
		}
	}
	if r.SparkSkipRecent > 0 {
		return r.SparkSkipRecent
	}
	return DefaultSparkSkipRecent
}

// runSparkPlanner rolls today's qty, inserts spaced spark_ping jobs, advances planner.
func (r *Runner) runSparkPlanner(ctx context.Context, log *slog.Logger, job Job) {
	spec, err := ParseSparkExpr(job.Expr)
	if err != nil {
		log.Warn("spark planner bad expr", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	loc, err := loadTZ(job.Timezone)
	if err != nil {
		log.Warn("spark planner tz", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	delivery := Delivery{
		SessionID: job.SessionID,
		UserID:    job.UserID,
		ChatID:    job.ChatID,
		ThreadID:  job.ThreadID,
	}
	_, _ = r.Store.CancelSparkPings(ctx, job.SessionID)
	n, times, err := PlanSparkDayTimes(spec, loc, time.Now())
	if err != nil {
		log.Warn("spark planner plan failed", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	created, err := r.Store.ScheduleSparkPings(ctx, job.Prompt, delivery, loc.String(), times)
	if err != nil {
		log.Warn("spark planner schedule pings failed", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	log.Info("spark planner seeded day",
		"id", job.ID,
		"qty", n,
		"pings", created,
		"session_id", job.SessionID,
	)
	if err := r.Store.Finish(ctx, job, nil); err != nil {
		log.Warn("spark planner finish failed", "id", job.ID, "err", err)
	}
}

// runExamplesPlanner rolls today's qty, inserts spaced examples_ping jobs, advances planner.
func (r *Runner) runExamplesPlanner(ctx context.Context, log *slog.Logger, job Job) {
	spec, err := ParseSparkExpr(job.Expr)
	if err != nil {
		log.Warn("examples planner bad expr", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	loc, err := loadTZ(job.Timezone)
	if err != nil {
		log.Warn("examples planner tz", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	delivery := Delivery{
		SessionID: job.SessionID,
		UserID:    job.UserID,
		ChatID:    job.ChatID,
		ThreadID:  job.ThreadID,
	}
	_, _ = r.Store.CancelExamplesPings(ctx, job.SessionID)
	n, times, err := PlanSparkDayTimes(spec, loc, time.Now())
	if err != nil {
		log.Warn("examples planner plan failed", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	created, err := r.Store.ScheduleExamplesPings(ctx, job.Prompt, delivery, loc.String(), times)
	if err != nil {
		log.Warn("examples planner schedule pings failed", "id", job.ID, "err", err)
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	log.Info("examples planner seeded day",
		"id", job.ID,
		"qty", n,
		"pings", created,
		"session_id", job.SessionID,
	)
	if err := r.Store.Finish(ctx, job, nil); err != nil {
		log.Warn("examples planner finish failed", "id", job.ID, "err", err)
	}
}

// FireDueForTest runs one poll cycle (tests).
func (r *Runner) FireDueForTest(ctx context.Context) {
	r.poll(ctx, slog.Default())
}
