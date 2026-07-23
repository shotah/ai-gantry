package discord

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestIsImageAttachment(t *testing.T) {
	if !isImageAttachment(&discordgo.MessageAttachment{ContentType: "image/png"}) {
		t.Fatal("content-type")
	}
	if !isImageAttachment(&discordgo.MessageAttachment{Filename: "pic.JPG"}) {
		t.Fatal("filename")
	}
	if isImageAttachment(&discordgo.MessageAttachment{Filename: "notes.txt"}) {
		t.Fatal("non-image")
	}
}

func TestInboundImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("pngbytes"))
	}))
	defer srv.Close()

	imgs, err := inboundImages(context.Background(), &discordgo.Message{
		Attachments: []*discordgo.MessageAttachment{{
			URL:         srv.URL + "/a.png",
			ContentType: "image/png",
			Filename:    "a.png",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 {
		t.Fatalf("len=%d", len(imgs))
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("pngbytes"))
	if imgs[0].URL != want {
		t.Fatalf("got %q", imgs[0].URL)
	}
}

func TestSendReply_ExtractsHTTPSImage(t *testing.T) {
	mock := &mockSession{}
	ch, err := New(Config{Token: "tok", AllowedUsers: []string{"1"}})
	if err != nil {
		t.Fatal(err)
	}
	err = ch.sendReply(context.Background(), mock, "dm", "see ![x](https://cdn.example/x.png) ok", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.msgs) != 1 || !strings.Contains(mock.msgs[0], "see") || !strings.Contains(mock.msgs[0], "ok") {
		t.Fatalf("msgs=%v", mock.msgs)
	}
	if mock.embeds != 1 {
		t.Fatalf("embeds=%d", mock.embeds)
	}
}

func TestSendReply_DataURLUpload(t *testing.T) {
	mock := &mockSession{}
	ch, err := New(Config{Token: "tok", AllowedUsers: []string{"1"}})
	if err != nil {
		t.Fatal(err)
	}
	data := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	err = ch.sendReply(context.Background(), mock, "dm", "", data)
	if err != nil {
		t.Fatal(err)
	}
	if mock.files != 1 {
		t.Fatalf("files=%d", mock.files)
	}
}

func TestLooksLikeImageURL_Shared(t *testing.T) {
	if !channel.LooksLikeImageURL("https://x.com/a.webp") {
		t.Fatal("expected true")
	}
}
