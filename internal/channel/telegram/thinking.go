package telegram

import (
	"html"
	"strings"
	"unicode/utf8"
)

// buildStreamDisplay formats thinking + answer for Telegram.
//
// collapsible=false (live stream): plain <i>…</i> — every editMessageText would
// reset Telegram's expandable UI, so we don't use blockquote until finish.
// collapsible=true (final): <blockquote expandable><i>…</i></blockquote>.
func buildStreamDisplay(thinking, content string, limit int, collapsible bool) (text string, useHTML bool) {
	thinking = strings.TrimSpace(thinking)
	content = strings.TrimSpace(content)
	if thinking == "" {
		if content == "" {
			return streamPlaceholder, false
		}
		return clipRunes(content, limit), false
	}
	// Agent may promote CoT to the reply when the model left Content empty
	// (Qwen think). Don't render the same text as both italic thinking and answer.
	if content != "" && content == thinking {
		return clipRunes(content, limit), false
	}
	if limit < 1 {
		limit = telegramMaxMessageRunes
	}
	overhead := 16 // <i>…</i>\n\n
	if collapsible {
		overhead = 64 // <blockquote expandable><i>…</i></blockquote>\n\n
	}
	thinkBudget := limit / 2
	if thinkBudget > 1800 {
		thinkBudget = 1800
	}
	contentBudget := limit - thinkBudget - overhead
	if contentBudget < 200 {
		contentBudget = 200
	}

	escapedThink := html.EscapeString(clipRunes(thinking, thinkBudget))
	var b strings.Builder
	if collapsible {
		b.WriteString("<blockquote expandable><i>")
		b.WriteString(escapedThink)
		b.WriteString("</i></blockquote>")
	} else {
		b.WriteString("<i>")
		b.WriteString(escapedThink)
		b.WriteString("</i>")
	}
	if content != "" {
		b.WriteString("\n\n")
		b.WriteString(html.EscapeString(clipRunes(content, contentBudget)))
	}
	out := b.String()
	if utf8.RuneCountInString(out) > limit {
		out = clipRunes(out, limit)
	}
	return out, true
}
