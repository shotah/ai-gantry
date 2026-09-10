package telegram

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestBareLocation(t *testing.T) {
	if !bareLocation(&models.Message{Location: &models.Location{Latitude: 1, Longitude: 2}}) {
		t.Fatal("bare pin should hold")
	}
	if bareLocation(&models.Message{
		Location: &models.Location{Latitude: 1, Longitude: 2},
		Caption:  "tacos",
	}) {
		t.Fatal("captioned pin should run a turn")
	}
	if bareLocation(&models.Message{
		Location:       &models.Location{Latitude: 1, Longitude: 2},
		ReplyToMessage: &models.Message{Text: "where?"},
	}) {
		t.Fatal("reply pin should run a turn")
	}
	if bareLocation(&models.Message{Text: "hi"}) {
		t.Fatal("text is not a pin")
	}
}

func TestInboundGeo(t *testing.T) {
	g := inboundGeo(&models.Message{
		Venue: &models.Venue{
			Title:    "Cafe",
			Location: models.Location{Latitude: 37.5, Longitude: -122.2},
		},
	})
	if g == nil || g.Label != "Cafe" || g.Lat != 37.5 || g.Lon != -122.2 {
		t.Fatalf("venue geo = %+v", g)
	}
	g = inboundGeo(&models.Message{Location: &models.Location{Latitude: 10.5, Longitude: 106.7}})
	if g == nil || g.Lat != 10.5 || g.Label != "" {
		t.Fatalf("pin geo = %+v", g)
	}
	if inboundGeo(&models.Message{Text: "hi"}) != nil {
		t.Fatal("text has no geo")
	}
}

func TestComposeInboundText_LocationAndCaption(t *testing.T) {
	got := composeInboundText(&models.Message{
		Caption:  "meet here",
		Location: &models.Location{Latitude: 10.5, Longitude: 106.7},
	})
	if got != "meet here" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "10.5") || strings.Contains(got, "[location]") {
		t.Fatalf("coords must not be stuffed into text: %q", got)
	}
}

func TestComposeInboundText_VenueContactDocument(t *testing.T) {
	got := composeInboundText(&models.Message{
		Venue: &models.Venue{
			Title:    "Cafe",
			Address:  "1 Main",
			Location: models.Location{Latitude: 1, Longitude: 2},
		},
		Contact: &models.Contact{
			FirstName:   "Ada",
			LastName:    "Lovelace",
			PhoneNumber: "+1000",
		},
		Document: &models.Document{
			FileName: "labs.pdf",
			MimeType: "application/pdf",
			FileSize: 12,
		},
	})
	for _, want := range []string{
		"[venue] Cafe — 1 Main",
		"[contact] Ada Lovelace, +1000",
		"[document] labs.pdf (application/pdf) 12 bytes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "lat=") || strings.Contains(got, "1.000000") {
		t.Fatalf("venue coords must not be in text: %q", got)
	}
}

func TestComposeInboundText_ForwardAndReply(t *testing.T) {
	got := composeInboundText(&models.Message{
		Text: "thoughts?",
		ForwardOrigin: &models.MessageOrigin{
			Type: models.MessageOriginTypeUser,
			MessageOriginUser: &models.MessageOriginUser{
				SenderUser: models.User{Username: "alice"},
			},
		},
		ReplyToMessage: &models.Message{Text: "original idea"},
	})
	if !strings.Contains(got, "[forwarded from @alice]") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "[reply to] original idea") {
		t.Fatalf("got %q", got)
	}
	if !strings.HasSuffix(got, "thoughts?") {
		t.Fatalf("got %q", got)
	}
}

func TestComposeInboundText_Sticker(t *testing.T) {
	got := composeInboundText(&models.Message{
		Sticker: &models.Sticker{Emoji: "🧗"},
	})
	if got != "[sticker] 🧗" {
		t.Fatalf("got %q", got)
	}
}
