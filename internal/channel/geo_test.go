package channel_test

import (
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestGeoFooter(t *testing.T) {
	if (*channel.Geo)(nil).Footer() != "" {
		t.Fatal("nil geo")
	}
	got := (&channel.Geo{Lat: 47.6, Lon: -122.3, AccuracyM: 8, Label: "Cafe"}).Footer()
	if !strings.Contains(got, "[location ±8m]") || !strings.Contains(got, "47.600000") || !strings.Contains(got, "-122.300000") || !strings.Contains(got, "Cafe") {
		t.Fatalf("%q", got)
	}
	if strings.Contains(got, "ago") || strings.Contains(got, "[last pin]") {
		t.Fatalf("must not age or last-pin: %q", got)
	}
}
