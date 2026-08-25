package memory

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SubjectHours is the category row for sleep / work / extra quiet windows.
const SubjectHours = "pref/hours"

// Hours is a parse of pref/hours content. Unknown or unparseable fields stay unset.
//
// Expected shape (agent-taught):
//
//	sleep: 23:00-07:00
//	work: 09:00-17:00
//	quiet: (none)
type Hours struct {
	SleepStart, SleepEnd int // minutes from midnight; -1 if unknown
	WorkStart, WorkEnd   int
	Quiet                string
	Raw                  string
}

// ParseHours reads structured sleep/work/quiet lines. Fail-open: garbage is ignored.
func ParseHours(content string) Hours {
	h := Hours{
		SleepStart: -1, SleepEnd: -1,
		WorkStart: -1, WorkEnd: -1,
		Raw: strings.TrimSpace(content),
	}
	if h.Raw == "" {
		return h
	}
	for _, line := range strings.Split(h.Raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(strings.ToLower(line), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "sleep":
			h.SleepStart, h.SleepEnd = parseWindow(val)
		case "work":
			h.WorkStart, h.WorkEnd = parseWindow(val)
		case "quiet":
			if val != "" && val != "(none)" && val != "none" && val != "-" {
				h.Quiet = strings.TrimSpace(val)
			}
		}
	}
	return h
}

func parseWindow(s string) (start, end int) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "–", "-")
	s = strings.ReplaceAll(s, "—", "-")
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return -1, -1
	}
	start, ok1 := parseHM(a)
	end, ok2 := parseHM(b)
	if !ok1 || !ok2 {
		return -1, -1
	}
	return start, end
}

func parseHM(s string) (int, bool) {
	s = strings.TrimSpace(s)
	hh, mm, ok := strings.Cut(s, ":")
	if !ok {
		return 0, false
	}
	h, err1 := strconv.Atoi(hh)
	m, err2 := strconv.Atoi(mm)
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// Known reports whether a sleep window was parsed.
func (h Hours) Known() bool {
	return h.SleepStart >= 0 && h.SleepEnd >= 0
}

// AsleepAt reports whether now falls in the sleep window. Unknown hours → false
// (do not skip spark; the agent should ask). Work is not DND.
func (h Hours) AsleepAt(now time.Time) bool {
	if !h.Known() {
		return false
	}
	t := now.Hour()*60 + now.Minute()
	return inWindow(t, h.SleepStart, h.SleepEnd)
}

func inWindow(t, start, end int) bool {
	if start == end {
		return false
	}
	if start < end {
		return t >= start && t < end
	}
	return t >= start || t < end
}

func fmtHM(mins int) string {
	if mins < 0 {
		return "unknown"
	}
	return fmt.Sprintf("%02d:%02d", mins/60, mins%60)
}

// Footer is the per-turn [hours] line. Empty raw → "hours unknown".
func (h Hours) Footer() string {
	if strings.TrimSpace(h.Raw) == "" {
		return "[hours] unknown — ask sleep / work / extra quiet once; store preference subject pref/hours. Work is not DND."
	}
	sleep := "unknown"
	if h.Known() {
		sleep = fmtHM(h.SleepStart) + "-" + fmtHM(h.SleepEnd)
	}
	work := "unknown"
	if h.WorkStart >= 0 && h.WorkEnd >= 0 {
		work = fmtHM(h.WorkStart) + "-" + fmtHM(h.WorkEnd)
	}
	quiet := "none"
	if h.Quiet != "" {
		quiet = h.Quiet
	}
	return "[hours] sleep " + sleep + " · work " + work + " · quiet " + quiet + " — work is not DND; do not spark in sleep"
}
