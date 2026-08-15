package watch

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultInterval is used when watch_add omits interval.
	DefaultInterval = 15 * time.Minute
	// MinInterval keeps a watch from spinning the MCP child.
	MinInterval = time.Minute
	// MaxInterval is a week — longer is a forgotten job.
	MaxInterval = 7 * 24 * time.Hour
	// MaxSeenIDs caps the cursor so the row stays small.
	MaxSeenIDs = 200
)

// ParseInterval accepts Go durations ("15m", "1h") or integer seconds.
func ParseInterval(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultInterval, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 1 {
			return 0, fmt.Errorf("watch: interval must be positive")
		}
		return clampInterval(time.Duration(n) * time.Second)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("watch: interval %q: use 15m, 1h, or seconds", s)
	}
	return clampInterval(d)
}

func clampInterval(d time.Duration) (time.Duration, error) {
	if d < MinInterval {
		return 0, fmt.Errorf("watch: interval must be >= %s", MinInterval)
	}
	if d > MaxInterval {
		return 0, fmt.Errorf("watch: interval must be <= %s", MaxInterval)
	}
	return d, nil
}
