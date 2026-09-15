package cron_test

import (
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
)

func TestFormatWakes_NextThreeHumanJobsSoonestFirst(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 14, 12, 2, 0, 0, loc)
	jobs := []cron.Job{
		{Kind: cron.KindDaily, Enabled: true, NextRunAt: now.Add(44 * time.Hour), Prompt: "Morning digest"},
		{Kind: cron.KindOnce, Enabled: true, NextRunAt: now.Add(5 * time.Hour), Prompt: "  Remind them to   leave for the gym  "},
		{Kind: cron.KindSparkPing, Enabled: true, NextRunAt: now.Add(time.Hour), Prompt: "[cron] Spark of life"},
		{Kind: cron.KindOnce, Enabled: false, NextRunAt: now.Add(2 * time.Hour), Prompt: "cancelled"},
		{Kind: cron.KindEvery, Enabled: true, NextRunAt: now.Add(9 * 24 * time.Hour), Prompt: "Water the plants and check the balcony tomatoes"},
		{Kind: cron.KindOnce, Enabled: true, NextRunAt: now.Add(10 * 24 * time.Hour), Prompt: "fourth"},
	}
	got := cron.FormatWakes(jobs, now)
	want := "[wakes] 5:02 PM Remind them to leave for the gym · Wed 8:02 AM Morning digest · Sep 23 12:02 PM Water the plants and check the balcony…" +
		" (+1 more — cron_list)"
	if got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

func TestFormatWakes_EmptyWithoutHumanJobs(t *testing.T) {
	now := time.Now()
	if got := cron.FormatWakes(nil, now); got != "" {
		t.Fatalf("nil %q", got)
	}
	only := []cron.Job{{Kind: cron.KindExamplesPing, Enabled: true, NextRunAt: now.Add(time.Hour), Prompt: "x"}}
	if got := cron.FormatWakes(only, now); got != "" {
		t.Fatalf("internal pings %q", got)
	}
}
