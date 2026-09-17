package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
)

// harnessNotePrefix labels the per-turn block so Completer RoleUser stays
// speech. Trailing unlabeled RoleSystem was easy to skip; leading with
// [current time] primed calendar/tool fixation. After their words, tagged.
const harnessNotePrefix = "[harness] Not user text — "

// harnessTags is the stamp order inside the block and the word the header
// uses for each. The header names only what is actually present.
var harnessTags = []struct{ tag, word string }{
	{"[location", "location"},
	{"[current time]", "clock"},
	{"[hours]", "hours"},
	{"[aims]", "horizon"},
	{"[loops]", "horizon"},
	{"[wakes]", "wakes"},
	{"[surface]", "surface"},
	{"[input]", "input"},
	{"[room]", "room"},
	{"[last contact]", "last contact"},
}

func formatHarnessClock(clock string) string {
	clock = strings.TrimSpace(clock)
	if clock == "" {
		return ""
	}
	return harnessNote(clock) + "\n" + clock
}

// harnessNote is the header built from the tags in this turn's block, so a
// memory-off turn is not told about hours it does not have.
func harnessNote(clock string) string {
	var words []string
	for _, t := range harnessTags {
		if !strings.Contains(clock, t.tag) {
			continue
		}
		if n := len(words); n > 0 && words[n-1] == t.word {
			continue
		}
		words = append(words, t.word)
	}
	return harnessNotePrefix + joinAnd(words) + " for this turn."
}

func joinAnd(words []string) string {
	switch len(words) {
	case 0:
		return "context"
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
	}
}

// stampLine prefixes a non-empty stamp with its newline.
func stampLine(s string) string {
	if s == "" {
		return ""
	}
	return "\n" + s
}

// hoursStamp is the [hours] line. Skipped (not "unknown") when the backend
// cannot answer a live-row lookup, so MCP memory does not nag every turn.
func (a *Agent) hoursStamp(ctx context.Context) string {
	e, ok, err := a.memory.ActiveByKindSubject(ctx, memory.KindPreference, memory.SubjectHours)
	if err != nil {
		if !errors.Is(err, memory.ErrNotSupported) {
			a.log.Warn("hours lookup failed", "err", err)
		}
		return ""
	}
	raw := ""
	if ok {
		raw = e.Content
	}
	return stampLine(memory.ParseHours(raw).Footer())
}

// horizon is this turn's aim/ and waiting/ follow/ rows: stamped on
// [harness] and dropped from [memory] hydration so they are not paid twice.
type horizon struct {
	aims, waiting, follow []memory.Entry
	// asked is the aim/bootstrap marker when the board is empty and the
	// months-scale question has already gone out; nil otherwise.
	asked *memory.Entry
}

func (a *Agent) loadHorizon(ctx context.Context) horizon {
	var h horizon
	list := func(kind, prefix, what string) []memory.Entry {
		rows, err := a.memory.ListBySubjectPrefix(ctx, kind, prefix, 0)
		if err != nil {
			a.log.Warn(what+" lookup failed", "err", err)
			return nil
		}
		return rows
	}
	h.aims = list(memory.KindInsight, memory.SubjectAimPrefix, "aims")
	h.waiting = list(memory.KindFact, memory.SubjectWaitingPrefix, "waiting")
	h.follow = list(memory.KindFact, memory.SubjectFollowPrefix, "follow")
	if len(h.aims) == 0 {
		if e, ok, err := a.memory.ActiveByKindSubject(ctx, memory.KindFact, memory.SubjectAimBootstrap); err == nil && ok {
			h.asked = &e
		}
	}
	return h
}

func (h horizon) stamp(now time.Time) string {
	aims := memory.FormatAims(h.aims, now)
	if aims == "" {
		aims = memory.FormatAimsEmpty(h.asked, now)
	}
	return stampLine(aims) + stampLine(memory.FormatLoops(h.waiting, h.follow, now))
}

// dropStamped filters hydration rows already on [aims] / [loops].
func (h horizon) dropStamped(entries []memory.Entry) []memory.Entry {
	stamped := make(map[int64]struct{}, len(h.aims)+len(h.waiting)+len(h.follow)+1)
	if h.asked != nil {
		stamped[h.asked.ID] = struct{}{}
	}
	for _, set := range [][]memory.Entry{h.aims, h.waiting, h.follow} {
		for _, e := range set {
			if e.ID > 0 {
				stamped[e.ID] = struct{}{}
			}
		}
	}
	if len(stamped) == 0 {
		return entries
	}
	kept := make([]memory.Entry, 0, len(entries))
	for _, e := range entries {
		if _, dup := stamped[e.ID]; dup {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

// WakeLister is *cron.Store: this session's jobs for the [wakes] stamp.
type WakeLister interface {
	ListSession(ctx context.Context, sessionID string, includeDisabled bool) ([]cron.Job, error)
}

func (a *Agent) wakesStamp(ctx context.Context, sessionID string, now time.Time) string {
	if a.wakes == nil {
		return ""
	}
	jobs, err := a.wakes.ListSession(ctx, sessionID, false)
	if err != nil {
		a.log.Warn("wakes lookup failed", "err", err)
		return ""
	}
	return cron.FormatWakes(jobs, now)
}

// spokenHint is the read-aloud shape. The reply goes to a speaker, not a
// screen: Auto / CarPlay read the card with the host engine, a hold-to-talk
// turn is read back by the pocket. One string so the two mouths cannot drift.
const spokenHint = "read aloud like a person: conversational prose, a few short sentences, no markdown, lists, code, links, or emoji"

// drivingSurface is a car head unit: the host speaks and the human cannot read.
func drivingSurface(surface string) bool {
	return surface == "android_auto" || surface == "carplay"
}

// surfaceStamp is the [surface] line. Driving surfaces get the spoken hint;
// the phone tells the harness, the harness tells the model, nobody types it.
func surfaceStamp(surface string) string {
	switch {
	case surface == "":
		return ""
	case drivingSurface(surface):
		return "[surface] " + surface + " — driving; " + spokenHint
	default:
		return "[surface] " + surface
	}
}

// inputSpoken is the one [input] value: hold-to-talk, the mouth reads the
// reply aloud.
const inputSpoken = "spoken"

// inputStamp is the [input] line: how the human produced the turn. The
// closed set is "spoken" (hold-to-talk; the mouth reads the reply). A
// driving surface already carries the hint on [surface], so the line stays
// bare there rather than saying it twice.
func inputStamp(input, surface string) string {
	switch {
	case input == "":
		return ""
	case drivingSurface(surface):
		return "[input] " + input
	default:
		return "[input] " + input + " — " + spokenHint
	}
}

// RoomSource is the pendant channel: the phone's look (theme, wallpaper,
// face) as last announced by the mailbox, for the [room] stamp.
type RoomSource interface {
	Room() channel.Room
}

// roomToolPrefix is the pendant MCP's server prefix — the room's tools.
const roomToolPrefix = "pendant__"

// roomStamp is the [room] line: what the phone looks like now, with ages,
// and one clause of nudge. The tool descriptions carry the how; the model
// only needs to know the room is its own and when it last changed. Needs
// the pendant mouth (RoomSource) and the pendant MCP in the catalog —
// absent either, no line. Prefix off for this chat → say so, no nudge.
func (a *Agent) roomStamp(now time.Time, published []provider.ToolDef, toolsOff bool) string {
	src := a.roomSource()
	if src == nil || a.tools == nil || toolsOff {
		return ""
	}
	catalog := a.tools.Tools()
	if !hasToolPrefix(catalog, roomToolPrefix) {
		return ""
	}
	r := src.Room()
	parts := make([]string, 0, 3)
	switch {
	case r.ThemeAt.IsZero():
		parts = append(parts, "theme not seen since boot (theme_list shows it)")
	case r.Theme == "":
		parts = append(parts, "theme cleared "+channel.Age(now.Sub(r.ThemeAt)))
	default:
		parts = append(parts, fmt.Sprintf("theme %s (set %s)", r.Theme, channel.Age(now.Sub(r.ThemeAt))))
	}
	if !r.BackdropAt.IsZero() {
		if r.Backdrop {
			parts = append(parts, "wallpaper set "+channel.Age(now.Sub(r.BackdropAt)))
		} else {
			parts = append(parts, "no wallpaper")
		}
	}
	if !r.FaceAt.IsZero() {
		parts = append(parts, "face changed "+channel.Age(now.Sub(r.FaceAt)))
	}
	line := "[room] " + strings.Join(parts, " · ")
	if !hasToolPrefix(published, roomToolPrefix) {
		return line + " — pendant is off this chat"
	}
	return line + " — yours; redress when the hour or your mood moves on"
}

func hasToolPrefix(defs []provider.ToolDef, prefix string) bool {
	for _, d := range defs {
		if strings.HasPrefix(d.Name, prefix) {
			return true
		}
	}
	return false
}

// lastContactSource is the optional History capability behind [last contact]
// (session.Store has it; test fakes need not).
type lastContactSource interface {
	LastUserAt(ctx context.Context, sessionID string) (time.Time, bool, error)
}

func (a *Agent) lastContactStamp(ctx context.Context, sessionID string, now time.Time) string {
	src, ok := a.sessions.(lastContactSource)
	if !ok {
		return ""
	}
	at, ok, err := src.LastUserAt(ctx, sessionID)
	if err != nil {
		a.log.Warn("last contact lookup failed", "err", err)
		return ""
	}
	if !ok {
		return "[last contact] none in this session — first message"
	}
	return fmt.Sprintf("[last contact] last human message %s (%s)", channel.Age(now.Sub(at)), channel.WhenShort(at, now))
}

// stripHarnessContext drops pasted / old-client clock and hydration blocks
// from inbound text so they are not stored or used as the hydrate query.
func stripHarnessContext(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "\n\n")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if harnessBlock(p) {
			continue
		}
		trimmed := strings.TrimRight(stripTrailingHarnessLines(p), "\n")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func harnessBlock(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return harnessTagLine(line)
	}
	return false
}

func stripTrailingHarnessLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if !harnessTagLine(strings.TrimSpace(line)) {
			continue
		}
		if i == 0 {
			return s
		}
		return strings.Join(lines[:i], "\n")
	}
	return s
}

func harnessTagLine(line string) bool {
	if strings.HasPrefix(line, "[harness]") || strings.HasPrefix(line, "[memory]") {
		return true
	}
	for _, t := range harnessTags {
		if strings.HasPrefix(line, t.tag) {
			return true
		}
	}
	return false
}
