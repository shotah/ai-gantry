package here_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/here"
)

func TestRememberGetFormat(t *testing.T) {
	const sid = "here-test-cache-1"
	at := time.Date(2026, 8, 17, 16, 22, 0, 0, time.UTC)
	here.Remember(sid, &channel.Geo{Lat: 37.386051, Lon: -122.083855, Label: "Cafe", AccuracyM: 12}, at)
	p, ok := here.Get(sid)
	if !ok || p.Label != "Cafe" || p.AccuracyM != 12 {
		t.Fatalf("get = %+v ok=%v", p, ok)
	}
	got := here.Format(p, at.Add(3*time.Minute), "America/Los_Angeles")
	if !strings.Contains(got, "[location ±12m]") || !strings.Contains(got, "37.386051") || !strings.Contains(got, "3m ago") {
		t.Fatalf("format = %q", got)
	}
	if strings.Contains(got, "[last pin]") {
		t.Fatalf("must not say last pin: %q", got)
	}
	if !strings.Contains(got, "Cafe") {
		t.Fatalf("missing label: %q", got)
	}
}
