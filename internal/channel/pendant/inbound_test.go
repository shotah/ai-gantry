package pendant

import (
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/here"
)

func TestApplyGeo_DoesNotStuffLocationIntoText(t *testing.T) {
	sid := "pendant:kit:geo-test"
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	applyGeo(sid, &frameContext{
		At:  at.Format(time.RFC3339),
		Geo: &geo{Lat: 47.6, Lon: -122.3},
	}, time.Now())
	p, ok := here.Get(sid)
	if !ok || p.Lat != 47.6 || p.Lon != -122.3 {
		t.Fatalf("pin = %+v ok=%v", p, ok)
	}
}

func TestSilentPin(t *testing.T) {
	ctx := &frameContext{Geo: &geo{Lat: 1, Lon: 2}}
	if !silentPin("", nil, ctx) {
		t.Fatal("bare geo should be silent")
	}
	if silentPin("hi", nil, ctx) {
		t.Fatal("text starts a turn")
	}
	if silentPin("", []channel.Image{{URL: "data:image/jpeg;base64,aa"}}, ctx) {
		t.Fatal("photo starts a turn")
	}
	if silentPin("", nil, nil) {
		t.Fatal("no geo")
	}
}

func TestNormalizeSub(t *testing.T) {
	if got := normalizeSub(" 1182:ada@example.com "); got != "1182" {
		t.Fatal(got)
	}
	if got := normalizeSub("1182"); got != "1182" {
		t.Fatal(got)
	}
}

func TestSessionID(t *testing.T) {
	if got := sessionID("kit", "1182"); got != "pendant:kit:1182" {
		t.Fatal(got)
	}
	if !strings.HasPrefix(sessionID(" ", "x"), "pendant:crane:") {
		t.Fatal(sessionID(" ", "x"))
	}
}
