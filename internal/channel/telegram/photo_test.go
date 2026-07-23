package telegram

import "testing"

func TestExtractImageURLs(t *testing.T) {
	urls, rest := extractImageURLs(`Here you go ![shot](https://cdn.example.com/a.png) and https://cdn.example.com/b.jpg more text`)
	if len(urls) != 2 {
		t.Fatalf("urls=%v", urls)
	}
	if urls[0] != "https://cdn.example.com/a.png" || urls[1] != "https://cdn.example.com/b.jpg" {
		t.Fatalf("urls=%v", urls)
	}
	if rest != "Here you go  and  more text" && rest != "Here you go and more text" {
		// whitespace may collapse differently; require no URLs left
		if containsURL(rest) {
			t.Fatalf("rest still has url: %q", rest)
		}
	}

	urls, rest = extractImageURLs("see https://example.com/docs for help")
	if len(urls) != 0 || rest != "see https://example.com/docs for help" {
		t.Fatalf("non-image url should stay: urls=%v rest=%q", urls, rest)
	}
}

func TestLooksLikeImageURL(t *testing.T) {
	if !looksLikeImageURL("https://x.test/p.JPEG?w=1") {
		t.Fatal("jpeg")
	}
	if looksLikeImageURL("https://x.test/page") {
		t.Fatal("page")
	}
}

func containsURL(s string) bool {
	return bareURLRe.MatchString(s)
}
