package agent

import (
	"testing"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestCronJobImpliesLiveTools(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "morning audit",
			text: cron.JobUserPrefix + "Fetch Garmin sleep/HRV and present the Unified Morning Audit.",
			want: true,
		},
		{
			name: "calendar digest",
			text: cron.JobUserPrefix + "Summarize calendar + work email for the past 8 hours.",
			want: true,
		},
		{
			name: "timecard reminder",
			text: cron.JobUserPrefix + "Remind me to submit my timecard.",
			want: false,
		},
		{
			name: "wrapper alone is not live data",
			text: cron.JobUserPrefix,
			want: false,
		},
		{
			name: "legacy prefix still splits",
			text: "[cron] Scheduled job — do the following and reply with the result for the user:\n\nFetch Garmin sleep.",
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cronJobImpliesLiveTools(tc.text); got != tc.want {
				t.Fatalf("cronJobImpliesLiveTools(%q) = %v, want %v (body=%q)",
					tc.text, got, tc.want, cronJobBody(tc.text))
			}
		})
	}
}

func TestDropCronHistory(t *testing.T) {
	t.Parallel()
	in := []session.Message{
		{Role: session.RoleUser, Content: "hey"},
		{Role: session.RoleAssistant, Content: "hi"},
		{Role: session.RoleUser, Content: cron.JobUserPrefix + "Fetch Garmin sleep"},
		{Role: session.RoleAssistant, Content: "Sleep 81"},
		{Role: session.RoleUser, Content: "what's up"},
		{Role: session.RoleAssistant, Content: "nm"},
	}
	out := dropCronHistory(in)
	if len(out) != 4 {
		t.Fatalf("len=%d want 4: %+v", len(out), out)
	}
	if out[0].Content != "hey" || out[1].Content != "hi" || out[2].Content != "what's up" || out[3].Content != "nm" {
		t.Fatalf("out=%+v", out)
	}
}

func TestLastUserContent(t *testing.T) {
	t.Parallel()
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "persona"},
		{Role: provider.RoleUser, Content: "older"},
		{Role: provider.RoleAssistant, Content: "ok"},
		{Role: provider.RoleUser, Content: "latest ask"},
		{Role: provider.RoleSystem, Content: "[current time]"},
	}
	if got := lastUserContent(msgs); got != "latest ask" {
		t.Fatalf("lastUserContent = %q", got)
	}
	if got := lastUserContent(nil); got != "" {
		t.Fatalf("empty = %q", got)
	}
}
