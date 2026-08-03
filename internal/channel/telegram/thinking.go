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
// Answer stays raw/escaped (no Markdown conversion) because partial fences break parsers.
//
// collapsible=true (final): <blockquote expandable><i>…</i></blockquote> plus
// Markdown→Telegram HTML for the answer.
func buildStreamDisplay(thinking, content string, limit int, collapsible bool) (text string, useHTML bool) {
	thinking = strings.TrimSpace(thinking)
	content = strings.TrimSpace(content)
	if limit < 1 {
		limit = telegramMaxMessageRunes
	}

	// Agent may promote CoT to the reply when the model left Content empty
	// (Qwen think). Don't render the same text as both italic thinking and answer.
	if thinking != "" && content != "" && content == thinking {
		thinking = ""
	}

	if thinking == "" {
		if content == "" {
			return streamPlaceholder, false
		}
		if !collapsible {
			return clipRunes(content, limit), false
		}
		return formatFinalAnswer(content, limit)
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
		if collapsible {
			answer := markdownToTelegramHTML(content)
			if utf8.RuneCountInString(answer) > contentBudget {
				answer = clipRunes(answer, contentBudget)
			}
			b.WriteString(answer)
		} else {
			b.WriteString(html.EscapeString(clipRunes(content, contentBudget)))
		}
	}
	out := b.String()
	if utf8.RuneCountInString(out) > limit {
		out = clipRunes(out, limit)
	}
	return out, true
}

// formatFinalAnswer converts Markdown for a thinking-free final bubble.
// Falls back to plain (so Finish can split) when the HTML would exceed the limit.
func formatFinalAnswer(content string, limit int) (text string, useHTML bool) {
	htmlBody := markdownToTelegramHTML(content)
	if htmlBody == "" {
		return streamPlaceholder, false
	}
	if utf8.RuneCountInString(htmlBody) <= limit {
		return htmlBody, true
	}
	// Too long for one HTML message — keep plain so Finish can splitMessage.
	return clipRunes(content, limit), false
}
