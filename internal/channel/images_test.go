package channel_test

import (
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestExtractImageURLs(t *testing.T) {
	urls, rest := channel.ExtractImageURLs(`Here you go ![shot](https://cdn.example.com/a.png) and https://cdn.example.com/b.jpg more text`)
	if len(urls) != 2 {
		t.Fatalf("urls=%v", urls)
	}
	if urls[0] != "https://cdn.example.com/a.png" || urls[1] != "https://cdn.example.com/b.jpg" {
		t.Fatalf("urls=%v", urls)
	}
	if strings.Contains(rest, "http") {
		t.Fatalf("rest still has url: %q", rest)
	}

	urls, rest = channel.ExtractImageURLs("see https://example.com/docs for help")
	if len(urls) != 0 || rest != "see https://example.com/docs for help" {
		t.Fatalf("non-image url should stay: urls=%v rest=%q", urls, rest)
	}
}

func TestMergePhotoURLs(t *testing.T) {
	urls, rest := channel.MergePhotoURLs("caption ![x](https://cdn.example.com/a.png)", "data:image/png;base64,aaa", "")
	if len(urls) != 2 || urls[0] != "data:image/png;base64,aaa" || urls[1] != "https://cdn.example.com/a.png" {
		t.Fatalf("urls=%v", urls)
	}
	if strings.Contains(rest, "http") {
		t.Fatalf("rest=%q", rest)
	}
}

func TestDecodeDataURL(t *testing.T) {
	raw, mime, ext, err := channel.DecodeDataURL("data:image/png;base64,AQID")
	if err != nil || mime != "image/png" || ext != ".png" {
		t.Fatalf("mime=%q ext=%q err=%v", mime, ext, err)
	}
	if string(raw) != string([]byte{1, 2, 3}) {
		t.Fatalf("raw=%v", raw)
	}
	if _, _, _, err := channel.DecodeDataURL("https://x.png"); err == nil {
		t.Fatal("https must fail")
	}
}

func TestLooksLikeImageURL(t *testing.T) {
	if !channel.LooksLikeImageURL("https://x.test/p.JPEG?w=1") {
		t.Fatal("jpeg")
	}
	if channel.LooksLikeImageURL("https://x.test/page") {
		t.Fatal("page")
	}
	if !channel.LooksLikeImageURL("data:image/png;base64,aaa") {
		t.Fatal("data")
	}
}
