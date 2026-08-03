package telegram

import (
	"strings"
	"testing"
)

func TestMarkdownToTelegramHTML_Basics(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string // all must be contained
		deny []string // none may be contained
	}{
		{
			name: "bold italic code",
			in:   "Say **hello** and *world* plus `code`",
			want: []string{"<b>hello</b>", "<i>world</i>", "<code>code</code>"},
		},
		{
			name: "heading",
			in:   "# Title\n\nBody",
			want: []string{"<b>Title</b>", "Body"},
			deny: []string{"# Title"},
		},
		{
			name: "link",
			in:   "See [docs](https://example.com/x).",
			want: []string{`<a href="https://example.com/x">docs</a>`},
		},
		{
			name: "fenced code",
			in:   "```go\nfmt.Println(\"<hi>\")\n```",
			want: []string{`<pre><code class="language-go">`, "&lt;hi&gt;", "</code></pre>"},
			deny: []string{"<hi>"},
		},
		{
			name: "list",
			in:   "- one\n- two",
			want: []string{"• one", "• two"},
		},
		{
			name: "strikethrough",
			in:   "~~old~~ new",
			want: []string{"<s>old</s>", "new"},
		},
		{
			name: "escapes raw html",
			in:   "use <script>alert(1)</script> please",
			want: []string{"&lt;script&gt;"},
			deny: []string{"<script>"},
		},
		{
			name: "dangerous link dropped to text",
			in:   "[x](javascript:alert(1))",
			want: []string{"x"},
			deny: []string{"javascript:", "<a href"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := markdownToTelegramHTML(tt.in)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Fatalf("missing %q in %q", w, got)
				}
			}
			for _, d := range tt.deny {
				if strings.Contains(got, d) {
					t.Fatalf("unexpected %q in %q", d, got)
				}
			}
		})
	}
}

func TestMarkdownToTelegramHTML_PlainUnchangedShape(t *testing.T) {
	got := markdownToTelegramHTML("Sleep score 78 — solid deep sleep.")
	if strings.ContainsAny(got, "<>") {
		t.Fatalf("plain text grew tags: %q", got)
	}
	if !strings.Contains(got, "Sleep score 78") {
		t.Fatalf("lost content: %q", got)
	}
}

func TestMarkdownToTelegramHTML_TableNoHTMLTable(t *testing.T) {
	in := "| A | B |\n| --- | --- |\n| 1 | 2 |"
	got := markdownToTelegramHTML(in)
	if strings.Contains(got, "<table") || strings.Contains(got, "<td") {
		t.Fatalf("telegram-illegal table tags: %q", got)
	}
	if strings.Contains(got, "<pre>") {
		t.Fatalf("tables should not use <pre>: %q", got)
	}
	if !strings.Contains(got, "<b>A | B</b>") || !strings.Contains(got, "1 | 2") {
		t.Fatalf("expected pipe-separated table: %q", got)
	}
}

func TestMarkdownToTelegramHTML_TablePreservesLinks(t *testing.T) {
	in := "| Car | Link |\n| --- | --- |\n| Sport | [View Sport](https://example.com/sport) |"
	got := markdownToTelegramHTML(in)
	if strings.Contains(got, "<pre>") {
		t.Fatalf("tables must not use <pre> (links cannot nest): %q", got)
	}
	if !strings.Contains(got, `<a href="https://example.com/sport">View Sport</a>`) {
		t.Fatalf("expected clickable link preserved: %q", got)
	}
	if !strings.Contains(got, "Sport | ") {
		t.Fatalf("expected pipe-separated row: %q", got)
	}
}

func TestMarkdownToTelegramHTML_StrikethroughTelegramSafe(t *testing.T) {
	got := markdownToTelegramHTML("~~old~~")
	if !strings.Contains(got, "<s>old</s>") {
		t.Fatalf("strikethrough not telegram-safe: %q", got)
	}
}

func TestBuildStreamDisplay_FinalFormatsMarkdown(t *testing.T) {
	text, useHTML := buildStreamDisplay("", "Try **bold** and `x`", 4096, true)
	if !useHTML {
		t.Fatal("expected HTML on final")
	}
	if !strings.Contains(text, "<b>bold</b>") || !strings.Contains(text, "<code>x</code>") {
		t.Fatalf("markdown not converted: %q", text)
	}
}

func TestBuildStreamDisplay_LiveSkipsMarkdown(t *testing.T) {
	text, useHTML := buildStreamDisplay("", "Try **bold**", 4096, false)
	if useHTML {
		t.Fatal("live stream should stay plain")
	}
	if text != "Try **bold**" {
		t.Fatalf("live altered text: %q", text)
	}
}

func TestBuildStreamDisplay_FinalWithThinkingFormatsAnswer(t *testing.T) {
	text, useHTML := buildStreamDisplay("step one", "Answer with **bold**", 4096, true)
	if !useHTML {
		t.Fatal("expected HTML")
	}
	if !strings.Contains(text, `<blockquote expandable><i>step one</i></blockquote>`) {
		t.Fatalf("thinking block: %q", text)
	}
	if !strings.Contains(text, "<b>bold</b>") {
		t.Fatalf("answer not converted: %q", text)
	}
	// Must not double-escape the tags we emit.
	if strings.Contains(text, "&lt;b&gt;") {
		t.Fatalf("answer HTML was escaped: %q", text)
	}
}
