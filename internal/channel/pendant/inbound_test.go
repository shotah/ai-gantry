package pendant

import (
	"encoding/json"
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

func TestParseEntry_Grammar(t *testing.T) {
	tests := []struct {
		in      string
		sub     string
		email   string
		errPart string
	}{
		{in: "118212345678901234567", sub: "118212345678901234567"},
		{in: "118212345678901234567:ada@example.com", sub: "118212345678901234567", email: "ada@example.com"},
		{in: "118212345678901234567:Ada@Example.COM", sub: "118212345678901234567", email: "ada@example.com"},
		{in: "ada@example.com", email: "ada@example.com"},
		{in: "ADA@example.com", email: "ada@example.com"},
		{in: " 1182 ", sub: "1182"},
		{in: "not-a-user", errPart: "not-a-user"},
		{in: "1182:not-an-email", errPart: "1182:not-an-email"},
		{in: ":ada@example.com", errPart: ":ada@example.com"},
		{in: "", errPart: "empty"},
	}
	for _, tc := range tests {
		got, err := ParseEntry(tc.in)
		if tc.errPart != "" {
			if err == nil || !strings.Contains(err.Error(), tc.errPart) {
				t.Fatalf("%q: err=%v want %q", tc.in, err, tc.errPart)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got.Sub != tc.sub || got.Email != tc.email {
			t.Fatalf("%q: got %+v want sub=%q email=%q", tc.in, got, tc.sub, tc.email)
		}
	}
}

func TestParseAllowlist_EmptyJunkDedupe(t *testing.T) {
	if _, err := ParseAllowlist(nil); err == nil || !strings.Contains(err.Error(), "allowlist is empty") {
		t.Fatalf("empty: %v", err)
	}
	if _, err := ParseAllowlist([]string{"", "  "}); err == nil || !strings.Contains(err.Error(), "allowlist is empty") {
		t.Fatalf("whitespace: %v", err)
	}
	if _, err := ParseAllowlist([]string{"nope"}); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("junk: %v", err)
	}
	got, err := ParseAllowlist([]string{" 1182:ada@example.com ", "bob@example.com", "1182"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Sub != "1182" || got[0].Email != "ada@example.com" || got[1].Email != "bob@example.com" || got[1].Sub != "" {
		t.Fatalf("%+v", got)
	}
	got, err = ParseAllowlist([]string{"ada@example.com 1183", "ADA@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Email != "ada@example.com" || got[1].Sub != "1183" {
		t.Fatalf("whitespace split %+v", got)
	}
	got, err = ParseAllowlist([]string{"bob@example.com", "1184:bob@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Sub != "1184" || got[0].Email != "bob@example.com" {
		t.Fatalf("email then sub:email %+v", got)
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

func TestAllowFrame_JSON(t *testing.T) {
	raw, err := json.Marshal(allowFrame([]Entry{
		{Sub: "1182", Email: "ada@example.com"},
		{Email: "bob@example.com"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"kind":"allow"`) || !strings.Contains(s, `"sub":"1182"`) || !strings.Contains(s, `"email":"bob@example.com"`) {
		t.Fatal(s)
	}
	if strings.Contains(s, `"commands"`) {
		t.Fatal(s)
	}
}
