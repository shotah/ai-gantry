package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// perfRingSize is the fixed in-process history for /perf (not an env knob).
const perfRingSize = 12

// perfRecord is one completed turn's latency split (same numbers as "turn perf").
type perfRecord struct {
	seq          uint64
	when         time.Time
	totalMS      int64
	modelMS      int64
	toolMS       int64
	iters        int
	firstTokenMS int64  // iteration 1 only; 0 = non-streaming
	volatileEst  int    // iteration 1 volatile token estimate
	source       string // user | cron | reaction
	cold         bool   // first turn after boot
}

type perfRing struct {
	mu      sync.Mutex
	buf     [perfRingSize]perfRecord
	len     int
	next    int // next write index
	total   uint64
	started time.Time
}

func newPerfRing(started time.Time) *perfRing {
	return &perfRing{started: started}
}

func (r *perfRing) append(rec perfRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.total++
	rec.seq = r.total
	r.buf[r.next] = rec
	r.next = (r.next + 1) % perfRingSize
	if r.len < perfRingSize {
		r.len++
	}
}

func (r *perfRing) snapshot() (recs []perfRecord, total uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	total = r.total
	if r.len == 0 {
		return nil, total
	}
	recs = make([]perfRecord, r.len)
	// Newest first.
	for i := 0; i < r.len; i++ {
		idx := (r.next - 1 - i + perfRingSize) % perfRingSize
		recs[i] = r.buf[idx]
	}
	return recs, total
}

func (r *perfRing) turnCount() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.total
}

func turnSource(text string) string {
	t := strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(t, "[cron]"):
		return "cron"
	case strings.HasPrefix(t, "[reaction]"):
		return "reaction"
	default:
		return "user"
	}
}

func (a *Agent) formatPerf() string {
	recs, _ := a.perf.snapshot()
	if len(recs) == 0 {
		return "no turns yet"
	}
	uptime := formatUptime(time.Since(a.startedAt))
	var b strings.Builder
	fmt.Fprintf(&b, "perf — last %d turns (uptime %s)\n", len(recs), uptime)
	for _, rec := range recs {
		when := rec.when
		if a.loc != nil {
			when = when.In(a.loc)
		}
		line := fmt.Sprintf("#%d %s  total=%s model=%s tool=%s iters=%d first_token=%s volatile≈%s",
			rec.seq,
			when.Format("15:04:05"),
			formatSec(rec.totalMS),
			formatSec(rec.modelMS),
			formatSec(rec.toolMS),
			rec.iters,
			formatSec(rec.firstTokenMS),
			formatVolatileK(rec.volatileEst),
		)
		if rec.cold {
			line += "  ← cold"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatSec(ms int64) string {
	if ms <= 0 {
		return "0s"
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func formatVolatileK(n int) string {
	if n < 0 {
		n = 0
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// formatUptime renders a compact uptime like 2h13m (no trailing 0s).
func formatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
