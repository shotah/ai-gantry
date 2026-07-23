package slack

import (
	"context"
	"io"
	"strconv"
	"sync/atomic"
	"testing"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestNew_RequiresTokensAndAllowlist(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("bot token")
	}
	if _, err := New(Config{BotToken: "xoxb-1"}); err == nil {
		t.Fatal("app token")
	}
	if _, err := New(Config{BotToken: "bad", AppToken: "xapp-1"}); err == nil {
		t.Fatal("prefix bot")
	}
	if _, err := New(Config{BotToken: "xoxb-1", AppToken: "bad"}); err == nil {
		t.Fatal("prefix app")
	}
	if _, err := New(Config{BotToken: "xoxb-1", AppToken: "xapp-1"}); err == nil {
		t.Fatal("allowlist")
	}
	ch, err := New(Config{
		BotToken:     "xoxb-1",
		AppToken:     "xapp-1",
		AllowedUsers: []string{"U1", " U2 "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.isAllowed("U2") {
		t.Fatal("allowlist trim")
	}
}

func TestSessionKeyStripMention(t *testing.T) {
	if got := sessionKey("C1", "U1", ""); got != "slack:C1:U1" {
		t.Fatal(got)
	}
	if got := sessionKey("C1", "U1", "123.456"); got != "slack:C1:U1:123.456" {
		t.Fatal(got)
	}
	if got := stripMention("hi <@B99> there", "B99"); got != "hi  there" {
		t.Fatalf("%q", got)
	}
}

func TestSplitMessage(t *testing.T) {
	if len(splitMessage("hi", 3500)) != 1 {
		t.Fatal()
	}
	long := make([]rune, 4000)
	for i := range long {
		long[i] = 'a'
	}
	if len(splitMessage(string(long), 3500)) != 2 {
		t.Fatal()
	}
}

type posted struct {
	channel string
	text    string
	thread  string
	blocks  int
}

type capturingPoster struct {
	authID      string
	posts       []posted
	updates     []string
	uploads     []slackapi.UploadFileParameters
	fileBody    []byte
	lastFileURL string
	tsSeq       atomic.Int64
}

func (c *capturingPoster) AuthTest() (*slackapi.AuthTestResponse, error) {
	return &slackapi.AuthTestResponse{UserID: c.authID, Team: "T"}, nil
}

func (c *capturingPoster) OpenConversation(params *slackapi.OpenConversationParameters) (*slackapi.Channel, bool, bool, error) {
	return &slackapi.Channel{GroupConversation: slackapi.GroupConversation{Conversation: slackapi.Conversation{ID: "D-" + params.Users[0]}}}, false, false, nil
}

func (c *capturingPoster) PostMessageContext(_ context.Context, channelID string, options ...slackapi.MsgOption) (string, string, error) {
	_, msg, err := slackapi.UnsafeApplyMsgOptions("token", channelID, "https://slack.com/api/", options...)
	if err != nil {
		return "", "", err
	}
	blocks := 0
	if b := msg.Get("blocks"); b != "" && b != "[]" {
		blocks = 1
	}
	c.posts = append(c.posts, posted{channel: channelID, text: msg.Get("text"), thread: msg.Get("thread_ts"), blocks: blocks})
	n := c.tsSeq.Add(1)
	return channelID, "ts-" + strconv.FormatInt(n, 10), nil
}

func (c *capturingPoster) UpdateMessageContext(_ context.Context, channelID, timestamp string, options ...slackapi.MsgOption) (string, string, string, error) {
	_, msg, err := slackapi.UnsafeApplyMsgOptions("token", channelID, "https://slack.com/api/", options...)
	if err != nil {
		return "", "", "", err
	}
	c.updates = append(c.updates, timestamp+":"+msg.Get("text"))
	return channelID, timestamp, "", nil
}

func (c *capturingPoster) GetFileContext(_ context.Context, downloadURL string, writer io.Writer) error {
	c.lastFileURL = downloadURL
	body := c.fileBody
	if body == nil {
		body = []byte("x")
	}
	_, err := writer.Write(body)
	return err
}

func (c *capturingPoster) UploadFileContext(_ context.Context, params slackapi.UploadFileParameters) (*slackapi.FileSummary, error) {
	c.uploads = append(c.uploads, params)
	return &slackapi.FileSummary{ID: "Fup"}, nil
}

func TestPushAndDispatch(t *testing.T) {
	api := &capturingPoster{authID: "BOT"}
	ch, err := New(Config{
		BotToken:     "xoxb-1",
		AppToken:     "xapp-1",
		AllowedUsers: []string{"U42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.newPoster = func(_, _ string) poster { return api }
	ch.api = api
	ch.botUser = "BOT"

	err = ch.Push(context.Background(), channel.Outbound{
		UserID: "U42",
		ChatID: "D99",
		Text:   "cron hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.posts) != 1 || api.posts[0].text != "cron hi" {
		t.Fatalf("%+v", api.posts)
	}

	if err := ch.Push(context.Background(), channel.Outbound{UserID: "U9", ChatID: "D1", Text: "x"}); err == nil {
		t.Fatal("allowlist")
	}

	handled := make(chan channel.Message, 1)
	ch.handleMessage(context.Background(), &slackevents.MessageEvent{
		User:        "U42",
		Channel:     "D99",
		Text:        "/status",
		ChannelType: "im",
		TimeStamp:   "1.0",
	}, func(_ context.Context, msg channel.Message) (string, error) {
		handled <- msg
		return "ok", nil
	})
	msg := <-handled
	if msg.SessionID != "slack:D99:U42" || msg.Text != "/status" {
		t.Fatalf("%+v", msg)
	}
	if len(api.posts) < 2 || api.posts[len(api.posts)-1].text != "ok" {
		t.Fatalf("posts=%+v", api.posts)
	}

	before := len(api.posts)
	ch.handleMessage(context.Background(), &slackevents.MessageEvent{
		User:        "U42",
		Channel:     "C1",
		Text:        "hi",
		ChannelType: "channel",
	}, func(context.Context, channel.Message) (string, error) {
		t.Fatal("should not handle")
		return "", nil
	})
	if len(api.posts) != before {
		t.Fatal("guild plain message should be ignored")
	}

	ch.handleAppMention(context.Background(), &slackevents.AppMentionEvent{
		User:      "U42",
		Channel:   "C1",
		Text:      "<@BOT> hello",
		TimeStamp: "9.0",
	}, func(_ context.Context, msg channel.Message) (string, error) {
		if msg.Text != "hello" {
			t.Fatalf("text=%q", msg.Text)
		}
		if msg.SessionID != "slack:C1:U42:9.0" {
			t.Fatalf("session=%q", msg.SessionID)
		}
		return "pong", nil
	})
	if api.posts[len(api.posts)-1].text != "pong" || api.posts[len(api.posts)-1].thread != "9.0" {
		t.Fatalf("%+v", api.posts[len(api.posts)-1])
	}
}

func TestResolveDest(t *testing.T) {
	api := &capturingPoster{}
	ch, thread, err := resolveDest(api, channel.Outbound{SessionID: "slack:C1:U1:1.2"})
	if err != nil || ch != "C1" || thread != "1.2" {
		t.Fatalf("%q %q %v", ch, thread, err)
	}
	ch, _, err = resolveDest(api, channel.Outbound{UserID: "U9"})
	if err != nil || ch != "D-U9" {
		t.Fatalf("%q %v", ch, err)
	}
}

func TestRunUsesAuthAndSocketHook(t *testing.T) {
	api := &capturingPoster{authID: "BOT"}
	ch, err := New(Config{
		BotToken:     "xoxb-1",
		AppToken:     "xapp-1",
		AllowedUsers: []string{"U1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.newPoster = func(_, _ string) poster { return api }
	ran := false
	ch.runSocket = func(_ context.Context, c *Channel, _ channel.Handler) error {
		ran = true
		if c.botUser != "BOT" {
			t.Fatalf("botUser=%q", c.botUser)
		}
		return nil
	}
	if err := ch.Run(context.Background(), func(context.Context, channel.Message) (string, error) {
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("socket runner not called")
	}
}
