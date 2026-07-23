package slack

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestIsImageFile(t *testing.T) {
	if !isImageFile(slackapi.File{Mimetype: "image/png"}) {
		t.Fatal("mimetype")
	}
	if !isImageFile(slackapi.File{Name: "pic.JPG"}) {
		t.Fatal("name")
	}
	if !isImageFile(slackapi.File{Filetype: "webp"}) {
		t.Fatal("filetype")
	}
	if isImageFile(slackapi.File{Name: "notes.txt"}) {
		t.Fatal("non-image")
	}
}

func TestInboundImages(t *testing.T) {
	api := &capturingPoster{
		fileBody: []byte("pngbytes"),
	}
	imgs, err := inboundImages(context.Background(), api, []slackapi.File{{
		ID:                 "F1",
		Mimetype:           "image/png",
		URLPrivateDownload: "https://files.slack.com/a.png",
	}})
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
	if api.lastFileURL != "https://files.slack.com/a.png" {
		t.Fatalf("url=%q", api.lastFileURL)
	}
}

func TestSendReply_HTTPSImageAndData(t *testing.T) {
	api := &capturingPoster{}
	ch, err := New(Config{BotToken: "xoxb-1", AppToken: "xapp-1", AllowedUsers: []string{"U1"}})
	if err != nil {
		t.Fatal(err)
	}
	err = ch.sendReply(context.Background(), api, "D1", "", "see ![x](https://cdn.example/x.png) ok", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(api.posts) != 2 {
		t.Fatalf("posts=%+v", api.posts)
	}
	if !strings.Contains(api.posts[0].text, "see") || !strings.Contains(api.posts[0].text, "ok") {
		t.Fatalf("text post=%+v", api.posts[0])
	}
	// Image posts use MsgOptionBlocks; UnsafeApplyMsgOptions omits blocks from values,
	// so we assert the fallback text URL instead.
	if api.posts[1].text != "https://cdn.example/x.png" {
		t.Fatalf("image post=%+v", api.posts[1])
	}

	data := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	err = ch.sendReply(context.Background(), api, "D1", "1.2", "", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(api.uploads) != 1 || api.uploads[0].Channel != "D1" || api.uploads[0].ThreadTimestamp != "1.2" {
		t.Fatalf("uploads=%+v", api.uploads)
	}
}

func TestDispatch_FileShare(t *testing.T) {
	api := &capturingPoster{authID: "BOT", fileBody: []byte("img")}
	ch, err := New(Config{
		BotToken:     "xoxb-1",
		AppToken:     "xapp-1",
		AllowedUsers: []string{"U42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.api = api
	ch.botUser = "BOT"

	handled := make(chan channel.Message, 1)
	ch.handleMessage(context.Background(), &slackevents.MessageEvent{
		User:        "U42",
		Channel:     "D99",
		Text:        "",
		ChannelType: "im",
		SubType:     "file_share",
		TimeStamp:   "1.0",
		Message: &slackapi.Msg{
			Files: []slackapi.File{{
				ID:                 "F9",
				Mimetype:           "image/jpeg",
				URLPrivateDownload: "https://files.slack.com/x.jpg",
			}},
		},
	}, func(_ context.Context, msg channel.Message) (string, error) {
		handled <- msg
		return "saw it", nil
	})
	msg := <-handled
	if len(msg.Images) != 1 {
		t.Fatalf("images=%+v", msg.Images)
	}
	if !strings.HasPrefix(msg.Images[0].URL, "data:image/jpeg;base64,") {
		t.Fatalf("url=%q", msg.Images[0].URL)
	}
}
