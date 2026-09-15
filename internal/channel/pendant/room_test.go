package pendant

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// Notices are the exact shapes frontends.md pins: face keeps the legacy
// text rev, backdrop and theme have no text.
func TestNoteRoom_MailboxNotices(t *testing.T) {
	c := &Channel{}
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	feed := func(raw string, at time.Time) bool {
		var frame inboundFrame
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		return c.noteRoom(frame, at)
	}

	if !feed(`{"kind":"theme","theme":"ember"}`, now) {
		t.Fatal("theme notice should land")
	}
	if !feed(`{"kind":"backdrop","rev":1757800000000}`, now.Add(time.Minute)) {
		t.Fatal("backdrop notice should land")
	}
	if !feed(`{"kind":"face","text":"1757800001000"}`, now.Add(2*time.Minute)) {
		t.Fatal("face notice should land")
	}
	r := c.Room()
	if r.Theme != "ember" || !r.ThemeAt.Equal(now) {
		t.Fatalf("theme %+v", r)
	}
	if !r.Backdrop || !r.BackdropAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("backdrop %+v", r)
	}
	if !r.FaceAt.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("face %+v", r)
	}

	// Clears: theme null, backdrop rev 0.
	if !feed(`{"kind":"theme","theme":null}`, now.Add(3*time.Minute)) {
		t.Fatal("theme null should clear")
	}
	if !feed(`{"kind":"backdrop","rev":0}`, now.Add(3*time.Minute)) {
		t.Fatal("backdrop 0 should clear")
	}
	r = c.Room()
	if r.Theme != "" || r.ThemeAt.IsZero() || r.Backdrop || r.BackdropAt.IsZero() {
		t.Fatalf("cleared %+v", r)
	}
}

// Junk is swallowed like a phone swallows it: no state change, no panic.
func TestNoteRoom_JunkIgnored(t *testing.T) {
	c := &Channel{}
	now := time.Now()
	for _, raw := range []string{
		`{"kind":"theme"}`,
		`{"kind":"theme","theme":"<script>"}`,
		`{"kind":"theme","theme":"a-very-long-theme-id-that-is-not-a-card"}`,
		`{"kind":"backdrop"}`,
		`{"kind":"backdrop","rev":-1}`,
		`{"kind":"backdrop","rev":"7"}`,
		`{"kind":"face","text":"0"}`,
		`{"kind":"face","text":"abc"}`,
		`{"kind":"reply","text":"hi"}`,
	} {
		var frame inboundFrame
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if c.noteRoom(frame, now) {
			t.Fatalf("junk landed: %s", raw)
		}
	}
	if r := c.Room(); r != (channel.Room{}) {
		t.Fatalf("state changed on junk: %+v", r)
	}
}

// Room notices must still be ignored as turns: dispatch caches and returns
// without touching the allowlist or the handler.
func TestDispatch_RoomNoticeCachesWithoutTurn(t *testing.T) {
	c, err := New(Config{MailboxURL: "wss://mb.example/ws/kit", Bearer: "b", AllowedUsers: []string{"1182"}})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	handle := func(context.Context, channel.Message) (string, error) { called = true; return "", nil }
	if err := c.dispatch(context.Background(), nil, []byte(`{"kind":"theme","theme":"noir"}`), handle); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("room notice must not start a turn")
	}
	if r := c.Room(); r.Theme != "noir" || r.ThemeAt.IsZero() {
		t.Fatalf("room %+v", r)
	}
}
