package telegram

import (
	"strings"
	"testing"
)

func TestBuildStreamDisplay_Plain(t *testing.T) {
	text, html := buildStreamDisplay("", "hello", 4096, false)
	if html {
		t.Fatal("expected plain")
	}
	if text != "hello" {
		t.Fatalf("text=%q", text)
	}
}

func TestBuildStreamDisplay_LiveItalics(t *testing.T) {
	text, html := buildStreamDisplay("step one <tag>", "answer & more", 4096, false)
	if !html {
		t.Fatal("expected HTML")
	}
	if strings.Contains(text, "blockquote") {
		t.Fatalf("live stream must not use expandable blockquote: %s", text)
	}
	if !strings.Contains(text, "<i>step one &lt;tag&gt;</i>") {
		t.Fatalf("thinking not italic/escaped: %s", text)
	}
	if !strings.Contains(text, "answer &amp; more") {
		t.Fatalf("answer not escaped: %s", text)
	}
}

func TestBuildStreamDisplay_FinalExpandable(t *testing.T) {
	text, html := buildStreamDisplay("step one <tag>", "answer & more", 4096, true)
	if !html {
		t.Fatal("expected HTML")
	}
	if !strings.Contains(text, `<blockquote expandable><i>`) {
		t.Fatalf("missing expandable block: %s", text)
	}
	if !strings.Contains(text, "step one &lt;tag&gt;") {
		t.Fatalf("thinking not escaped: %s", text)
	}
	if !strings.Contains(text, "</i></blockquote>\n\n") {
		t.Fatalf("missing separator: %s", text)
	}
}

func TestBuildStreamDisplay_DedupesPromotedThinking(t *testing.T) {
	same := "Sleep score 78 — solid deep sleep."
	text, useHTML := buildStreamDisplay(same, same, 4096, true)
	if !useHTML {
		t.Fatal("final plain answer still uses HTML parse mode after MD convert")
	}
	if strings.Contains(text, "<blockquote") || strings.Contains(text, "<i>") {
		t.Fatalf("promoted thinking must not render as CoT box: %q", text)
	}
	if !strings.Contains(text, same) {
		t.Fatalf("text=%q", text)
	}
}
