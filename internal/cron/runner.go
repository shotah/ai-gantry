package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// DefaultTick is how often the runner polls for due jobs.
const DefaultTick = 15 * time.Second

// Runner wakes due jobs, runs the agent, and pushes replies.
type Runner struct {
	Store    *Store
	Handle   channel.Handler
	Pusher   channel.Pusher
	Interval time.Duration
	Logger   *slog.Logger
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
	text := "[cron] Scheduled job — do the following and reply with the result for the user:\n\n" + job.Prompt
	msg := channel.Message{
		SessionID: job.SessionID,
		UserID:    job.UserID,
		ChatID:    job.ChatID,
		ThreadID:  job.ThreadID,
		Text:      text,
	}
	reply, err := r.Handle(ctx, msg)
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

// FireDueForTest runs one poll cycle (tests).
func (r *Runner) FireDueForTest(ctx context.Context) {
	r.poll(ctx, slog.Default())
}
