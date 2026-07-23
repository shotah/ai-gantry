package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/shotah/ai-gantry/internal/channel"
)

type apiMock struct {
	token string
	mu    sync.Mutex
	calls map[string]int
	srv   *httptest.Server
}

const testBotToken = "1:test-token"

func newAPIMock(t *testing.T) *apiMock {
	t.Helper()
	m := &apiMock{token: testBotToken, calls: map[string]int{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// File downloads use /file/bot<token>/<path>
		if strings.HasPrefix(r.URL.Path, "/file/bot"+testBotToken+"/") {
			m.mu.Lock()
			m.calls["fileDownload"]++
			m.mu.Unlock()
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xff, 0xd8, 0xff, 0xd9}) // minimal JPEG
			return
		}
		method := strings.TrimPrefix(r.URL.Path, "/bot"+testBotToken+"/")
		m.mu.Lock()
		m.calls[method]++
		m.mu.Unlock()

		_, _ = io.ReadAll(r.Body)
		switch method {
		case "getMe":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"g","username":"gantry_bot"}}`))
		case "getUpdates":
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		case "setMyCommands", "sendChatAction":
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "editMessageText":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"edited"}}`))
		case "sendPhoto":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":2,"date":1,"chat":{"id":1,"type":"private"}}}`))
		case "getFile":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"pic","file_unique_id":"u","file_path":"photos/pic.jpg"}}`))
		default:
			t.Errorf("unexpected method %q", method)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":404,"description":"unknown"}`))
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *apiMock) count(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[method]
}

func testBot(t *testing.T, serverURL string) *bot.Bot {
	t.Helper()
	b, err := bot.New(testBotToken,
		bot.WithServerURL(serverURL),
		bot.WithSkipGetMe(),
	)
	if err != nil {
		t.Fatalf("bot.New: %v", err)
	}
	return b
}

func testChannel(t *testing.T) *Channel {
	t.Helper()
	ch, err := New(Config{Token: testBotToken, AllowedUsers: []int64{42}})
	if err != nil {
		t.Fatal(err)
	}
	ch.chunkMax = 20
	return ch
}

func TestMakeHandler_AllowlistAndReply(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)

	var got channel.Message
	handler := ch.makeHandler(func(_ context.Context, msg channel.Message) (string, error) {
		got = msg
		return "hello reply", nil
	})

	handler(context.Background(), b, &models.Update{
		Message: &models.Message{
			ID:   7,
			Text: " hi ",
			Chat: models.Chat{ID: 99, Type: "private"},
			From: &models.User{ID: 42, Username: "chris"},
		},
	})

	if got.Text != "hi" || got.UserID != "42" || got.SessionID != "telegram:99:42" {
		t.Fatalf("got %+v", got)
	}
	// typing + reply
	if api.count("sendChatAction") < 1 {
		t.Fatal("expected typing action")
	}
	if api.count("sendMessage") < 1 {
		t.Fatal("expected sendMessage")
	}
}

func TestMakeHandler_Unauthorized(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)
	called := false
	handler := ch.makeHandler(func(context.Context, channel.Message) (string, error) {
		called = true
		return "", nil
	})
	handler(context.Background(), b, &models.Update{
		Message: &models.Message{
			Text: "nope",
			Chat: models.Chat{ID: 1},
			From: &models.User{ID: 7},
		},
	})
	if called {
		t.Fatal("handler should not run for unauthorized users")
	}
	if api.count("sendMessage") != 0 {
		t.Fatal("should not reply to unauthorized users")
	}
}

func TestMakeHandler_ErrorSendsApology(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)
	handler := ch.makeHandler(func(context.Context, channel.Message) (string, error) {
		return "", errors.New("boom")
	})
	handler(context.Background(), b, &models.Update{
		Message: &models.Message{
			Text: "hi",
			Chat: models.Chat{ID: 1},
			From: &models.User{ID: 42},
		},
	})
	if api.count("sendMessage") < 1 {
		t.Fatal("expected apology message")
	}
}

func TestMakeHandler_IgnoresEmptyAndNil(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)
	handler := ch.makeHandler(func(context.Context, channel.Message) (string, error) {
		t.Fatal("should not be called")
		return "", nil
	})
	handler(context.Background(), b, &models.Update{})
	handler(context.Background(), b, &models.Update{
		Message: &models.Message{Text: "  ", Chat: models.Chat{ID: 1}, From: &models.User{ID: 42}},
	})
	if api.count("sendMessage") != 0 {
		t.Fatal("unexpected sends")
	}
}

func TestSendChunks_Splits(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	ch.chunkMax = 10
	b := testBot(t, api.srv.URL)

	text := strings.Repeat("abcdefghij", 3) // 30 runes -> 3 chunks
	if err := ch.sendChunks(context.Background(), b, 1, 0, text); err != nil {
		t.Fatal(err)
	}
	if got := api.count("sendMessage"); got != 3 {
		t.Fatalf("sendMessage calls = %d, want 3", got)
	}
}

func TestRun_BotCreateError(t *testing.T) {
	ch := testChannel(t)
	ch.newBot = func(string, ...bot.Option) (*bot.Bot, error) {
		return nil, errors.New("nope")
	}
	err := ch.Run(context.Background(), func(context.Context, channel.Message) (string, error) {
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "create bot") {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_Cancel(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	ch.newBot = func(token string, opts ...bot.Option) (*bot.Bot, error) {
		opts = append(opts,
			bot.WithServerURL(api.srv.URL),
			bot.WithSkipGetMe(),
			bot.WithHTTPClient(50*time.Millisecond, api.srv.Client()),
		)
		return bot.New(token, opts...)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ch.Run(ctx, func(context.Context, channel.Message) (string, error) {
			return "", nil
		})
	}()

	deadline := time.After(2 * time.Second)
	for api.count("getMe") == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for getMe")
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestNew_MissingToken(t *testing.T) {
	_, err := New(Config{AllowedUsers: []int64{1}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMakeHandler_PhotoInboundAndSendPhotoReply(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)

	var got channel.Message
	handler := ch.makeHandler(func(_ context.Context, msg channel.Message) (string, error) {
		got = msg
		return `nice ![x](https://cdn.example.com/out.png)`, nil
	})

	handler(context.Background(), b, &models.Update{
		Message: &models.Message{
			ID:      9,
			Caption: "what is this?",
			Photo: []models.PhotoSize{
				{FileID: "small", Width: 10, Height: 10, FileSize: 10},
				{FileID: "pic", Width: 100, Height: 100, FileSize: 100},
			},
			Chat: models.Chat{ID: 99, Type: "private"},
			From: &models.User{ID: 42, Username: "chris"},
		},
	})

	if got.Text != "what is this?" || len(got.Images) != 1 {
		t.Fatalf("got %+v", got)
	}
	if !strings.HasPrefix(got.Images[0].URL, "data:image/jpeg;base64,") {
		t.Fatalf("image url=%q", got.Images[0].URL)
	}
	if api.count("getFile") < 1 || api.count("fileDownload") < 1 {
		t.Fatal("expected photo download")
	}
	if api.count("sendPhoto") < 1 {
		t.Fatal("expected sendPhoto for reply image")
	}
}

func TestSendReply_PhotoURL(t *testing.T) {
	api := newAPIMock(t)
	ch := testChannel(t)
	b := testBot(t, api.srv.URL)
	err := ch.sendReply(context.Background(), b, 1, 0, "caption text", "https://cdn.example.com/x.png")
	if err != nil {
		t.Fatal(err)
	}
	if api.count("sendPhoto") < 1 {
		t.Fatal("sendPhoto")
	}
}
