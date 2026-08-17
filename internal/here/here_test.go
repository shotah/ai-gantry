package here_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/here"
)

func TestSetGetFormat(t *testing.T) {
	const sid = "here-test-1"
	at := time.Date(2026, 8, 17, 16, 22, 0, 0, time.UTC)
	here.Set(sid, here.Pin{Lat: 37.386051, Lon: -122.083855, Label: "Cafe", At: at})
	p, ok := here.Get(sid)
	if !ok || p.Label != "Cafe" {
		t.Fatalf("get = %+v ok=%v", p, ok)
	}
	got := here.Format(p, at.Add(3*time.Minute), "America/Los_Angeles")
	if !strings.Contains(got, "[last pin]") || !strings.Contains(got, "37.386051") || !strings.Contains(got, "3m ago") {
		t.Fatalf("format = %q", got)
	}
	if !strings.Contains(got, "Cafe") {
		t.Fatalf("missing label: %q", got)
	}
}
