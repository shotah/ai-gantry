package pendant

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// Room notices ride the same socket as chat: the Durable Object sends one
// frame to every socket (crane included) when the face, wallpaper, or theme
// changes. The crane used to drop them; now they feed the [room] stamp so
// the model knows what the phone looks like without a tool call.
const (
	kindFace     = "face"
	kindBackdrop = "backdrop"
	kindTheme    = "theme"
)

// Room is the look last announced by the mailbox. Zero times = no notice
// since boot (the mailbox flushes theme to phones on connect, not to the crane).
func (c *Channel) Room() channel.Room {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.room
}

// noteRoom folds a face / backdrop / theme notice into the cache. Junk is
// ignored the same way a phone ignores it: no bubble, no state change.
func (c *Channel) noteRoom(frame inboundFrame, now time.Time) bool {
	switch frame.Kind {
	case kindFace:
		if faceRev(frame.Text) <= 0 {
			return false
		}
		c.mu.Lock()
		c.room.FaceAt = now
		c.mu.Unlock()
		return true
	case kindBackdrop:
		rev, ok := backdropRev(frame.Rev)
		if !ok {
			return false
		}
		c.mu.Lock()
		c.room.Backdrop = rev > 0
		c.room.BackdropAt = now
		c.mu.Unlock()
		return true
	case kindTheme:
		id, ok := themeID(frame.Theme)
		if !ok {
			return false
		}
		c.mu.Lock()
		c.room.Theme = id
		c.room.ThemeAt = now
		c.mu.Unlock()
		return true
	default:
		return false
	}
}

// faceRev reads the legacy face notice: text is the rev as a string, > 0.
func faceRev(text string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// backdropRev reads rev from a backdrop notice: a safe integer >= 0, 0 = cleared.
func backdropRev(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// themeID reads theme from a theme notice: explicit null clears; a short id
// token is kept as-is (the catalog lives in the Worker and theme_list, not
// here); a missing key or anything else is junk.
func themeID(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	if string(raw) == "null" {
		return "", true
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return "", false
	}
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 24 {
		return "", false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return "", false
		}
	}
	return id, true
}
