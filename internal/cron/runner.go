package cron

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/memory"
)

// JobUserPrefix wraps user-scheduled job prompts (not spark/examples pings).
// It must stay a single paragraph plus a trailing blank line so the agent can
// split wrapper from job body when deciding whether live tools were skipped.
const JobUserPrefix = "[cron] Scheduled job — you scheduled this continuation. If [job memory] is present, that is why. Recall/tools as needed. Decide what is useful now: message, act, postpone, cancel, or [silent]. Do not nag. The original chat may be gone. If this job needs live data, call those tools first — do not write the report, tables, or numbers until tool results are in context. Never guess metrics; if a tool fails, say so. If the human does not need a message (all-clear or work-only), reply with exactly [silent] and nothing else.\n\n"

// SparkTurnMarker is the start of SparkPingPrefix. Agent and tests use it to
// recognize a spark horizon turn (tools required, not a chat ping).
const SparkTurnMarker = "[cron] Spark of life"

// SparkPingPrefix wraps spark-of-life horizon wakes.
const SparkPingPrefix = SparkTurnMarker + " — the user is the aim. In one response: recall aim/ + pref/hours + pref/calendar + cron_list, mcp_enable then call live tools that match (Garmin, calendar, search). Shape the human-facing move by [current time]: a gym aim with no workout yet in the morning is a short grounded joke or nudge; evening with still nothing can be disappointed-uncle about the miss. A joke or ritual is allowed when it is about this turn's tool results — never a joke with zero tools. Hours unknown → ask sleep/work/quiet once (work is not DND) and memory_store preference subject pref/hours. Else at most one user-model question (food, activity, team). A real empty calendar (tools returned none) is a hole, not an all-clear: ask ONE what they want on it today (lunch/dinner or a training plan); try to get something scheduled (ask first before writing events). Never agree-and-stop. A clock time you commit (scoop, leave, refuel) is cron_schedule with memory_id or one offer to ping — a calendar event is not the Telegram reminder. [silent] if nothing useful. Empty aim board: ask ONE months-scale question (do not invent). Ask-first: no email, spend, or posts:\n\n"

// IsSparkTurn reports whether this user text is a spark-of-life horizon wake.
func IsSparkTurn(userText string) bool {
	return strings.HasPrefix(strings.TrimSpace(userText), SparkTurnMarker)
}

// ExamplesPingPrefix wraps capability-example pings.
const ExamplesPingPrefix = "[cron] Capability example — inspire the human with one concrete idea (propose only). If a ping would be noise, reply with exactly [silent] and nothing else:\n\n"

// DefaultTick is how often the runner polls for due jobs.
const DefaultTick = 15 * time.Second

// DefaultSparkSkipRecent is how long a recent user message suppresses a spark ping.
const DefaultSparkSkipRecent = 30 * time.Minute

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
	// Memory is optional; used to inject [job memory] and skip spark during sleep.
	Memory memory.Memory
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
	log.Info("cron job firing",
		"id", job.ID,
		"kind", job.Kind,
		"session_id", job.SessionID,
		"memory_id", job.MemoryID,
		"prompt", truncate(job.Prompt, 80),
	)

	if job.Kind == KindSpark {
		r.runSparkPlanner(ctx, log, job)
		return
	}
	if job.Kind == KindExamples {
		r.runExamplesPlanner(ctx, log, job)
		return
	}

	if (job.Kind == KindSparkPing || job.Kind == KindExamplesPing) && r.asleep(ctx, job) {
		until := time.Now().UTC().Add(time.Hour)
		log.Info("cron ping deferred (sleep hours)",
			"id", job.ID, "kind", job.Kind, "session_id", job.SessionID, "until", until.Format(time.RFC3339))
		_ = r.Store.Defer(ctx, job.ID, until, "skipped: sleep hours")
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

	prefix := JobUserPrefix
	prompt := job.Prompt
	handleCtx := ctx
	switch job.Kind {
	case KindSparkPing:
		prefix = SparkPingPrefix
		prompt = PickSparkPrompt(job.Prompt)
	case KindExamplesPing:
		prefix = ExamplesPingPrefix
		if r.Examples != nil {
			prompt = r.Examples.BuildPingPrompt(ctx)
		}
		handleCtx = channel.WithNoTools(ctx)
	}
	text := prefix + r.jobMemoryBlock(ctx, log, job) + prompt
	msg := channel.Message{
		SessionID: job.SessionID,
		UserID:    job.UserID,
		ChatID:    job.ChatID,
		ThreadID:  job.ThreadID,
		Text:      text,
	}
	reply, err := r.Handle(handleCtx, msg)
	if err != nil {
		log.Warn("cron job handle failed", "id", job.ID, "err", err, "outcome", "error")
		_ = r.Store.Finish(ctx, job, err)
		return
	}
	outcome := "push"
	if IsSilentReply(reply) {
		outcome = "silent"
		log.Info("cron silent skip", "id", job.ID, "kind", job.Kind, "session_id", job.SessionID, "outcome", outcome)
	} else if reply != "" {
		if err := r.Pusher.Push(ctx, channel.Outbound{
			SessionID: job.SessionID,
			UserID:    job.UserID,
			ChatID:    job.ChatID,
			ThreadID:  job.ThreadID,
			Text:      reply,
		}); err != nil {
			log.Warn("cron push failed", "id", job.ID, "err", err, "outcome", "error")
			_ = r.Store.Finish(ctx, job, fmt.Errorf("push: %w", err))
			return
		}
		log.Info("cron job pushed", "id", job.ID, "kind", job.Kind, "session_id", job.SessionID, "outcome", outcome)
	} else {
		outcome = "silent"
		log.Info("cron empty reply", "id", job.ID, "kind", job.Kind, "session_id", job.SessionID, "outcome", outcome)
	}
	if err := r.Store.Finish(ctx, job, nil); err != nil {
		log.Warn("cron finish failed", "id", job.ID, "err", err)
	}
}

func (r *Runner) jobMemoryBlock(ctx context.Context, log *slog.Logger, job Job) string {
	if r == nil || r.Memory == nil {
		return ""
	}
	if job.MemoryID == 0 && strings.TrimSpace(job.MemorySubject) == "" {
		return ""
	}
	e, ok := memory.ResolvePin(ctx, r.Memory, job.MemoryID, job.MemorySubject)
	if !ok {
		if log != nil {
			log.Warn("cron job memory missing; running on prompt",
				"id", job.ID, "memory_id", job.MemoryID, "subject", job.MemorySubject)
		}
		return ""
	}
	return FormatJobMemory(e.Kind, e.Subject, e.Content)
}

// FormatJobMemory renders the pinned row for a scheduled turn.
func FormatJobMemory(kind, subject, content string) string {
	kind = strings.TrimSpace(kind)
	subject = strings.TrimSpace(subject)
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return fmt.Sprintf("[job memory]\n- (%s) %s: %s\n\n", kind, subject, content)
}

func (r *Runner) asleep(ctx context.Context, job Job) bool {
	if r == nil || r.Memory == nil {
		return false
	}
	e, ok, err := r.Memory.ActiveByKindSubject(ctx, memory.KindPreference, memory.SubjectHours)
	if err != nil || !ok {
		return false
	}
	loc, err := loadTZ(job.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	return memory.ParseHours(e.Content).AsleepAt(time.Now().In(loc))
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
