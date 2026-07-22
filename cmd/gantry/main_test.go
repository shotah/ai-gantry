package main

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/channel/stdio"
	"github.com/shotah/ai-gantry/internal/config"
)

func TestStatus(t *testing.T) {
	if code := status(); code != 1 {
		t.Fatalf("status() = %d, want 1", code)
	}
}

func TestPrintHelp(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	printHelp()
	_ = w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if !strings.Contains(buf.String(), "gantry") {
		t.Fatalf("help = %q", buf.String())
	}
}

func TestNewLogger_Levels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "other"} {
		l := newLogger(level)
		if l == nil {
			t.Fatalf("newLogger(%q) nil", level)
		}
	}
}

func TestNewChannel(t *testing.T) {
	logger := slog.Default()

	ch, err := newChannel(&config.Config{Channel: config.ChannelStdio}, logger)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ch.(*stdio.Channel); !ok {
		t.Fatalf("got %T", ch)
	}

	ch, err = newChannel(&config.Config{
		Channel:              config.ChannelTelegram,
		TelegramBotToken:     "1:tok",
		TelegramAllowedUsers: []int64{1},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	if ch == nil {
		t.Fatal("nil telegram channel")
	}

	_, err = newChannel(&config.Config{Channel: "nope"}, logger)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_BadConfig(t *testing.T) {
	t.Setenv("CHANNEL", "stdio")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")
	if code := run(); code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
}
