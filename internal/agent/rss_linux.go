//go:build linux

package agent

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// selfRSSMB reads this process's VmRSS from /proc (kernel binary only —
// MCP children are separate processes). ok is false when unreadable.
func selfRSSMB() (mb int, ok bool) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, false
		}
		return kb / 1024, true
	}
	return 0, false
}
