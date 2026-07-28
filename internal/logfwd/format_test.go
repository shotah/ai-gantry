package logfwd

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestFormatHTML_ErrorExpandable(t *testing.T) {
	rec := slog.NewRecord(time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC), slog.LevelError, "tool call failed", 0)
	rec.AddAttrs(slog.String("err", "boom <tag>"), slog.String("name", "google-workspace__get_events"))
	got := FormatHTML(rec, nil, 2, 0)
	for _, want := range []string{
		"🔴 <b>gantry ERROR</b>",
		"tool call failed",
		"<blockquote expandable>",
		"err: boom &lt;tag&gt;",
		"name: google-workspace__get_events",
		"suppressed 2 similar",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatHTML_WarnEmoji(t *testing.T) {
	rec := slog.NewRecord(time.Now(), slog.LevelWarn, "slow", 0)
	got := FormatHTML(rec, nil, 0, 0)
	if !strings.Contains(got, "🟠 <b>gantry WARN</b>") {
		t.Fatalf("got %s", got)
	}
}

func TestFormatHTML_RedactsSecrets(t *testing.T) {
	rec := slog.NewRecord(time.Now(), slog.LevelError, "auth", 0)
	rec.AddAttrs(slog.String("bot_token", "secret-value"), slog.String("ok", "visible"))
	got := FormatHTML(rec, nil, 0, 0)
	if strings.Contains(got, "secret-value") {
		t.Fatalf("token leaked: %s", got)
	}
	if !strings.Contains(got, "bot_token: [redacted]") || !strings.Contains(got, "ok: visible") {
		t.Fatalf("got %s", got)
	}
}
