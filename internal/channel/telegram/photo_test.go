package telegram

import (
	"testing"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestExtractImageURLs_ViaChannel(t *testing.T) {
	urls, rest := channel.ExtractImageURLs(`![shot](https://cdn.example.com/a.png)`)
	if len(urls) != 1 || urls[0] != "https://cdn.example.com/a.png" {
		t.Fatalf("urls=%v rest=%q", urls, rest)
	}
}
