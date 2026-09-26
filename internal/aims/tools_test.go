package aims_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/aims"
)

func TestAimLog_BadScoreAndUnknownArea(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "climbing")
	tools := aims.Tools{Store: f.store}
	_, err := tools.Call(ctx, aims.ToolLog, json.RawMessage(`{"what":"pizza","aims":{"climbing":4}}`))
	if err == nil || !strings.Contains(err.Error(), "-3..3") && !strings.Contains(err.Error(), "must be") {
		t.Fatalf("err=%v", err)
	}
	_, err = tools.Call(ctx, aims.ToolLog, json.RawMessage(`{"what":"pizza","aims":{"weight":-2}}`))
	if err == nil || !strings.Contains(err.Error(), `unknown area "weight"`) || !strings.Contains(err.Error(), "climbing") {
		t.Fatalf("err=%v", err)
	}
	hist, err := f.store.History(ctx, "", "", "", 10)
	if err != nil || len(hist) != 0 {
		t.Fatalf("wrote on error: %v %v", hist, err)
	}
}

func TestAimHistory_LineFormat(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "drinking", "weight", "climbing")
	tools := aims.Tools{Store: f.store}
	line, err := tools.Call(ctx, aims.ToolLog, json.RawMessage(`{
		"what":"team dinner: 3 beers and a burger",
		"day":"2026-09-22",
		"aims":{"drinking":-2,"weight":-1,"climbing":-1}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "2026-09-22 team dinner: 3 beers and a burger — ") ||
		!strings.Contains(line, "drinking -2") || !strings.Contains(line, "weight -1") || !strings.Contains(line, "climbing -1") {
		t.Fatalf("line %q", line)
	}
	got, err := tools.Call(ctx, aims.ToolHistory, json.RawMessage(`{"area":"drinking","from":"2026-09-22","to":"2026-09-22"}`))
	if err != nil || got != line {
		t.Fatalf("history %q err %v", got, err)
	}
	for _, def := range aims.ToolDefs() {
		if def.Name == "" || def.Parameters["type"] != "object" {
			t.Fatalf("schema %+v", def)
		}
	}
}

func TestAimTools_RejectOverflowingInts(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "climbing")
	tools := aims.Tools{Store: f.store}

	// 2^32+2 truncates to +2 if converted through a 32-bit int first.
	_, err := tools.Call(ctx, aims.ToolLog, json.RawMessage(`{"what":"pizza","aims":{"climbing":4294967298}}`))
	if err == nil || !strings.Contains(err.Error(), "must be -3..3") {
		t.Fatalf("score err=%v", err)
	}
	hist, err := f.store.History(ctx, "", "", "", 10)
	if err != nil || len(hist) != 0 {
		t.Fatalf("wrote on error: %v %v", hist, err)
	}

	_, err = tools.Call(ctx, aims.ToolHistory, json.RawMessage(`{"limit":3000000000}`))
	if err == nil || !strings.Contains(err.Error(), "bad limit") {
		t.Fatalf("limit err=%v", err)
	}
}

func TestAimLog_RewriteByEvent(t *testing.T) {
	ctx := context.Background()
	loc, _ := time.LoadLocation("America/Los_Angeles")
	f := openFixture(t, loc)
	seedAims(t, f.mem, "weight")
	tools := aims.Tools{Store: f.store}
	line, err := tools.Call(ctx, aims.ToolLog, json.RawMessage(`{"what":"team dinner","day":"2026-09-22","aims":{"weight":-2}}`))
	if err != nil {
		t.Fatal(err)
	}
	id := line[1:strings.Index(line, " ")]
	got, err := tools.Call(ctx, aims.ToolLog, json.RawMessage(`{"what":"team dinner","event":`+id+`,"aims":{"weight":0}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "weight 0") || strings.Contains(got, "weight -2") {
		t.Fatalf("rewrite %q", got)
	}
}
