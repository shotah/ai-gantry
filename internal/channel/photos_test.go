package channel_test

import (
	"context"
	"testing"

	"github.com/shotah/ai-gantry/internal/channel"
)

func TestPhotoSink_CollectsURLs(t *testing.T) {
	ctx, sink := channel.AttachPhotoSink(context.Background())
	got := channel.PhotoSinkFrom(ctx)
	if got != sink {
		t.Fatal("sink not on context")
	}
	sink.Add("", "  ", "data:image/png;base64,aaa")
	sink.Add("https://cdn.example/a.png")
	urls := sink.URLs()
	if len(urls) != 2 {
		t.Fatalf("urls=%v", urls)
	}
	urls[0] = "mutated"
	if sink.URLs()[0] == "mutated" {
		t.Fatal("URLs must copy")
	}
}

func TestPhotoURLs_MergesOutbound(t *testing.T) {
	got := channel.PhotoURLs(channel.Outbound{
		PhotoURL: " https://a.png ",
		Photos:   []string{"", "data:image/png;base64,x"},
	})
	if len(got) != 2 || got[0] != "https://a.png" || got[1] != "data:image/png;base64,x" {
		t.Fatalf("%v", got)
	}
}
