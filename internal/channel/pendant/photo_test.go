package pendant

import (
	"strings"
	"testing"
)

func TestReplyFrames_CaptionPlusOneImage(t *testing.T) {
	frames := replyFrames("reply", "1182", "", "drew a red bike", "data:image/png;base64,AQID")
	if len(frames) != 1 {
		t.Fatalf("frames=%d", len(frames))
	}
	f := frames[0]
	if f.Kind != "reply" || f.UserID != "1182" || f.Text != "drew a red bike" {
		t.Fatalf("%+v", f)
	}
	if len(f.Images) != 1 || f.Images[0].URL != "data:image/png;base64,AQID" {
		t.Fatalf("images %+v", f.Images)
	}
}

func TestReplyFrames_PhotoOnlyAndSplit(t *testing.T) {
	frames := replyFrames("reply", "1182", "", "",
		"data:image/png;base64,AQID",
		"https://cdn.example/a.png",
	)
	if len(frames) != 2 {
		t.Fatalf("frames=%d %+v", len(frames), frames)
	}
	if frames[0].Text != "" || len(frames[0].Images) != 1 || frames[0].Images[0].URL != "data:image/png;base64,AQID" {
		t.Fatalf("first %+v", frames[0])
	}
	if frames[1].Text != "" || len(frames[1].Images) != 1 || frames[1].Images[0].URL != "https://cdn.example/a.png" {
		t.Fatalf("second %+v", frames[1])
	}
}

func TestReplyFrames_Empty(t *testing.T) {
	if frames := replyFrames("reply", "1182", "", "  "); len(frames) != 0 {
		t.Fatalf("%+v", frames)
	}
}

func TestFitPendantPhoto_DropsOversizeJunk(t *testing.T) {
	u := "data:image/png;base64," + strings.Repeat("A", pendantImageBytesMax)
	if _, ok := fitPendantPhoto(u); ok {
		t.Fatal("oversize junk must drop (mailbox would refuse the whole frame)")
	}
	if _, ok := fitPendantPhoto("http://cdn.example/a.png"); ok {
		t.Fatal("http is not legal from crane")
	}
	got, ok := fitPendantPhoto(" https://cdn.example/a.png ")
	if !ok || got != "https://cdn.example/a.png" {
		t.Fatalf("https: %q ok=%v", got, ok)
	}
}
