package pendant

import (
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/slash"
)

type geo struct {
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	AccuracyM *float64 `json:"accuracy_m,omitempty"`
	AltM      *float64 `json:"alt_m,omitempty"`
	Heading   *float64 `json:"heading,omitempty"`
	SpeedMPS  *float64 `json:"speed_mps,omitempty"`
}

type frameContext struct {
	At  string `json:"at,omitempty"`
	TZ  string `json:"tz,omitempty"`
	Geo *geo   `json:"geo,omitempty"`
}

type inboundFrame struct {
	Text    string          `json:"text,omitempty"`
	Images  []channel.Image `json:"images,omitempty"`
	Context *frameContext   `json:"context,omitempty"`
	Kind    string          `json:"kind,omitempty"`
	UserID  string          `json:"user_id,omitempty"`
	Email   string          `json:"email,omitempty"`
}

type outboundFrame struct {
	Text     string          `json:"text,omitempty"`
	Kind     string          `json:"kind,omitempty"`
	UserID   string          `json:"user_id,omitempty"`
	Commands []slash.Command `json:"commands,omitempty"`
	Users    []Entry         `json:"users,omitempty"`
}

// Entry is one PENDANT_ALLOWED_USERS row: Google sub, verified email, or both.
type Entry struct {
	Sub   string `json:"sub,omitempty"`
	Email string `json:"email,omitempty"`
}

func cmdsFrame() outboundFrame {
	return outboundFrame{Kind: "cmds", Commands: slash.Catalog()}
}

func allowFrame(users []Entry) outboundFrame {
	return outboundFrame{Kind: "allow", Users: users}
}

func sessionID(slug, sub string) string {
	slug = strings.TrimSpace(slug)
	sub = strings.TrimSpace(sub)
	if slug == "" {
		slug = "crane"
	}
	return "pendant:" + slug + ":" + sub
}

// ParseEntry reads one allowlist token. Email is lowercased. Neither digits
// nor an email is an error that names the token.
func ParseEntry(raw string) (Entry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Entry{}, fmt.Errorf("pendant: empty allowlist entry")
	}
	if i := strings.IndexByte(raw, ':'); i >= 0 {
		sub := strings.TrimSpace(raw[:i])
		email := strings.ToLower(strings.TrimSpace(raw[i+1:]))
		if !isDigits(sub) {
			return Entry{}, fmt.Errorf("pendant: invalid allowlist entry %q", raw)
		}
		if email != "" && !isEmail(email) {
			return Entry{}, fmt.Errorf("pendant: invalid allowlist entry %q", raw)
		}
		return Entry{Sub: sub, Email: email}, nil
	}
	if isDigits(raw) {
		return Entry{Sub: raw}, nil
	}
	email := strings.ToLower(raw)
	if isEmail(email) {
		return Entry{Email: email}, nil
	}
	return Entry{}, fmt.Errorf("pendant: invalid allowlist entry %q", raw)
}

// ParseAllowlist splits comma/whitespace, parses each token, and dedupes by
// sub then by email. Empty after that is today's empty-allowlist error.
func ParseAllowlist(raw []string) ([]Entry, error) {
	var entries []Entry
	bySub := map[string]int{}
	byEmail := map[string]int{}
	for _, item := range raw {
		item = strings.ReplaceAll(item, ",", " ")
		for _, tok := range strings.Fields(item) {
			e, err := ParseEntry(tok)
			if err != nil {
				return nil, err
			}
			entries = mergeEntry(entries, bySub, byEmail, e)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("pendant: allowlist is empty (set PENDANT_ALLOWED_USERS)")
	}
	return entries, nil
}

func mergeEntry(entries []Entry, bySub, byEmail map[string]int, e Entry) []Entry {
	if e.Sub != "" {
		if i, ok := bySub[e.Sub]; ok {
			if e.Email != "" && entries[i].Email == "" {
				entries[i].Email = e.Email
				byEmail[e.Email] = i
			}
			return entries
		}
	}
	if e.Email != "" {
		if i, ok := byEmail[e.Email]; ok {
			if e.Sub != "" && entries[i].Sub == "" {
				entries[i].Sub = e.Sub
				bySub[e.Sub] = i
			}
			return entries
		}
	}
	entries = append(entries, e)
	i := len(entries) - 1
	if e.Sub != "" {
		bySub[e.Sub] = i
	}
	if e.Email != "" {
		byEmail[e.Email] = i
	}
	return entries
}

func allowLookup(entries []Entry) map[string]struct{} {
	allowed := make(map[string]struct{}, len(entries)*2)
	for _, e := range entries {
		if e.Sub != "" {
			allowed[e.Sub] = struct{}{}
		}
		if e.Email != "" {
			allowed[e.Email] = struct{}{}
		}
	}
	return allowed
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-1
}

func frameGeo(ctx *frameContext) *channel.Geo {
	if ctx == nil || ctx.Geo == nil {
		return nil
	}
	g := &channel.Geo{Lat: ctx.Geo.Lat, Lon: ctx.Geo.Lon}
	if ctx.Geo.AccuracyM != nil && *ctx.Geo.AccuracyM > 0 {
		g.AccuracyM = *ctx.Geo.AccuracyM
	}
	return g
}

func silentPin(text string, images []channel.Image, ctx *frameContext) bool {
	if strings.TrimSpace(text) != "" || len(images) > 0 {
		return false
	}
	return ctx != nil && ctx.Geo != nil
}
