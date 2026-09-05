package pendant

import (
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/here"
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
}

type outboundFrame struct {
	Text     string          `json:"text,omitempty"`
	Kind     string          `json:"kind,omitempty"`
	Commands []slash.Command `json:"commands,omitempty"`
}

func cmdsFrame() outboundFrame {
	return outboundFrame{Kind: "cmds", Commands: slash.Catalog()}
}

func sessionID(slug, sub string) string {
	slug = strings.TrimSpace(slug)
	sub = strings.TrimSpace(sub)
	if slug == "" {
		slug = "crane"
	}
	return "pendant:" + slug + ":" + sub
}

// normalizeSub keeps the Google sub. Email after a colon is a Worker label.
func normalizeSub(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.IndexByte(id, ':'); i >= 0 {
		id = strings.TrimSpace(id[:i])
	}
	return id
}

func applyGeo(sid string, ctx *frameContext, now time.Time) {
	if ctx == nil || ctx.Geo == nil {
		return
	}
	at := now
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(ctx.At)); err == nil {
		at = t
	}
	here.Set(sid, here.Pin{Lat: ctx.Geo.Lat, Lon: ctx.Geo.Lon, At: at})
}

func silentPin(text string, images []channel.Image, ctx *frameContext) bool {
	if strings.TrimSpace(text) != "" || len(images) > 0 {
		return false
	}
	return ctx != nil && ctx.Geo != nil
}
