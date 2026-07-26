package telegram

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestComposeInboundText_LocationAndCaption(t *testing.T) {
	got := composeInboundText(&models.Message{
		Caption:  "meet here",
		Location: &models.Location{Latitude: 10.5, Longitude: 106.7},
	})
	if !strings.Contains(got, "[location] lat=10.500000 lon=106.700000") {
		t.Fatalf("got %q", got)
	}
	if !strings.HasSuffix(got, "meet here") {
		t.Fatalf("got %q", got)
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
