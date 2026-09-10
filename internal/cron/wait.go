package cron

import (
	"fmt"
	"strings"
	"time"
)

// KindFollowUp is a one-shot poke while waiting_for_reply is set.
const KindFollowUp = "followup"

// WaitToken is an own-line (or trailing) marker: the agent asked something
// the human should answer. Stripped from the pushed text; stored in history.
const WaitToken = "[wait]"

// NoWaitToken drops an open wait from the agent side (give up / no longer asking).
const NoWaitToken = "[nowait]"

// FollowUpTurnMarker is the start of FollowUpPrefix. Agent uses it so a poke
// cannot re-arm the campaign.
const FollowUpTurnMarker = "[cron] Follow-up"

// MaxWaitNudges is two pokes after the original question (three messages total).
const MaxWaitNudges = 2

// FollowUpDelays is the pause before nudge 1 and nudge 2 (from the previous send).
var FollowUpDelays = []time.Duration{2 * time.Minute, 15 * time.Minute}

// FollowUpPrompt is the job body; the wrapper carries the poke instructions.
const FollowUpPrompt = "follow up on the unanswered question in this chat"

// FollowUpPrefix wraps a wait poke. nudge is 0-based (0 → "nudge 1 of 2").
func FollowUpPrefix(nudge, maxNudges int) string {
	if maxNudges < 1 {
		maxNudges = MaxWaitNudges
	}
	return fmt.Sprintf(
		"%s — you asked something and they have not replied (nudge %d of %d). "+
			"waiting_for_reply is still set. One short poke about that open question, or [silent] if a poke would nag. "+
			"Do not ask a new different question. Do not include [wait] (already waiting). "+
			"[nowait] on its own line drops the wait if you are done asking.\n\n",
		FollowUpTurnMarker, nudge+1, maxNudges)
}

// IsFollowUpTurn reports whether this user text is a wait-nudge wake.
func IsFollowUpTurn(userText string) bool {
	return strings.HasPrefix(strings.TrimSpace(userText), FollowUpTurnMarker)
}

// FollowUpParsed builds a one-shot follow-up at t.
func FollowUpParsed(t time.Time, tz string) Parsed {
	return Parsed{
		Kind:     KindFollowUp,
		Expr:     t.UTC().Format(time.RFC3339Nano),
		NextRun:  t.UTC(),
		Timezone: tz,
	}
}

// NextFollowUpDelay is the pause before the next poke given current wait_nudges
// (0 → first poke, 1 → second). ok is false when the campaign is exhausted.
func NextFollowUpDelay(nudges int) (time.Duration, bool) {
	if nudges < 0 || nudges >= len(FollowUpDelays) {
		return 0, false
	}
	return FollowUpDelays[nudges], true
}

// HasWaitToken reports whether reply opted into waiting_for_reply.
func HasWaitToken(s string) bool {
	_, ok := stripBareToken(s, WaitToken)
	return ok
}

// HasNoWaitToken reports whether reply dropped the wait from the agent side.
func HasNoWaitToken(s string) bool {
	_, ok := stripBareToken(s, NoWaitToken)
	return ok
}

// StripWaitTokens removes [wait] / [nowait] so the human never sees them.
func StripWaitTokens(s string) string {
	s, _ = stripBareToken(s, WaitToken)
	s, _ = stripBareToken(s, NoWaitToken)
	return s
}

func stripBareToken(s, token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" || s == "" {
		return s, false
	}
	found := false
	lowerTok := strings.ToLower(token)
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.EqualFold(t, token) {
			found = true
			continue
		}
		if n := len(token); len(t) > n && strings.HasSuffix(strings.ToLower(t), " "+lowerTok) {
			found = true
			line = strings.TrimSpace(t[:len(t)-n])
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n")), found
}
