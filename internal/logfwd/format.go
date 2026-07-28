package logfwd

import (
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

const defaultMaxRunes = 3500

// FormatHTML builds a Telegram HTML alert: emoji header + expandable details.
func FormatHTML(rec slog.Record, extra []slog.Attr, suppressed int, maxRunes int) string {
	if maxRunes < 1 {
		maxRunes = defaultMaxRunes
	}
	level := strings.ToUpper(rec.Level.String())
	emoji := "🔴"
	if rec.Level < slog.LevelError {
		emoji = "🟠"
	}
	msg := strings.TrimSpace(rec.Message)
	if msg == "" {
		msg = "(no message)"
	}
	header := fmt.Sprintf("%s <b>gantry %s</b> · %s", emoji, html.EscapeString(level), html.EscapeString(clipRunes(msg, 120)))

	var body strings.Builder
	body.WriteString("time: ")
	body.WriteString(rec.Time.UTC().Format(time.RFC3339))
	body.WriteString("\nmsg: ")
	body.WriteString(msg)
	rec.Attrs(func(a slog.Attr) bool {
		writeAttr(&body, a)
		return true
	})
	for _, a := range extra {
		writeAttr(&body, a)
	}
	if suppressed > 0 {
		_, _ = fmt.Fprintf(&body, "\n(suppressed %d similar since last report)", suppressed)
	}

	escaped := html.EscapeString(clipRunes(body.String(), maxRunes-len(header)-64))
	out := header + "\n<blockquote expandable>" + escaped + "</blockquote>"
	if utf8.RuneCountInString(out) > maxRunes {
		return clipRunes(out, maxRunes)
	}
	return out
}

func writeAttr(b *strings.Builder, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	key := strings.TrimSpace(a.Key)
	if key == "" {
		return
	}
	val := a.Value.String()
	if sensitiveKey(key) {
		val = "[redacted]"
	}
	b.WriteByte('\n')
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(val)
}

func sensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, needle := range []string{"token", "secret", "password", "authorization", "api_key", "apikey"} {
		if strings.Contains(k, needle) {
			return true
		}
	}
	return false
}

func clipRunes(s string, limit int) string {
	if limit < 1 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	if limit < 2 {
		return string(r[:1])
	}
	return string(r[:limit-1]) + "…"
}
