//go:build !linux

package agent

// selfRSSMB is Linux-only; omit on other GOOS rather than print n/a noise.
func selfRSSMB() (mb int, ok bool) {
	return 0, false
}
