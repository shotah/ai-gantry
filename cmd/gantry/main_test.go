package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/channel/stdio"
	"github.com/shotah/ai-gantry/internal/config"
	"github.com/shotah/ai-gantry/internal/doctor"
	"github.com/shotah/ai-gantry/internal/heartbeat"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestStatus(t *testing.T) {
	dir := t.TempDir()
	store, err := session.Open(dir, 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := heartbeat.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	if err := hb.Touch(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if err := doctor.WriteSnapshot(dir, nil); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DATA_DIR", dir)
	t.Setenv("PERSONA_DIR", dir)
	t.Setenv("MCP_MANIFEST", filepath.Join(dir, "missing.toml"))
	t.Setenv("CHANNEL", "stdio")

	code, out := captureStdout(t, status)
	if code != 0 {
		t.Fatalf("status() = %d, want 0; out=%s", code, out)
	}
	var rep doctor.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("json %q: %v", out, err)
	}
	if !rep.Alive || !rep.OK || rep.Channel != "stdio" {
		t.Fatalf("%+v from %s", rep, out)
	}

	t.Setenv("DATA_DIR", filepath.Join(dir, "missing"))
	code, _ = captureStdout(t, status)
	if code != 1 {
		t.Fatalf("status() = %d, want 1", code)
	}
}

func TestStatus_AllSkippedStillExitZero(t *testing.T) {
	dir := t.TempDir()
	store, err := session.Open(dir, 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := heartbeat.OpenDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	if err := hb.Touch(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if err := doctor.WriteSnapshot(dir, []mcp.ServerStatus{
		{Name: "google", State: mcp.ServerSkipped, Reason: mcp.ReasonNoOAuth, Auth: true},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_DIR", dir)
	t.Setenv("PERSONA_DIR", dir)
	t.Setenv("MCP_MANIFEST", filepath.Join(dir, "missing.toml"))
	t.Setenv("CHANNEL", "telegram")

	code, out := captureStdout(t, status)
	if code != 0 {
		t.Fatalf("alive-but-skipped must exit 0 (docker health), got %d %s", code, out)
	}
	var rep doctor.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.Alive || rep.OK || rep.Reason != "mcp_all_skipped" {
		t.Fatalf("%+v", rep)
	}
}

func captureStdout(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return code, buf.String()
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
	help := buf.String()
	if !strings.Contains(help, "gantry") || !strings.Contains(help, "JSON doctor") {
		t.Fatalf("help = %q", help)
	}
}

func TestNewLogger_Levels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "other"} {
		l, fwd := newLogger(level, "off")
		if l == nil {
			t.Fatalf("newLogger(%q) nil", level)
		}
		if fwd != nil {
			t.Fatalf("newLogger(%q, off) unexpected forwarder", level)
		}
	}
	l, fwd := newLogger("info", "error")
	if l == nil || fwd == nil {
		t.Fatal("want logfwd handler when TELEGRAM_ERROR_REPORTING=error")
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
