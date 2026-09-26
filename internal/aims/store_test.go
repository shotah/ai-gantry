package aims_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/aims"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/session"
)

type fixture struct {
	store *aims.Store
	mem   *memory.Builtin
}

func openFixture(t *testing.T, loc *time.Location) fixture {
	t.Helper()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	mem, err := memory.OpenDB(sess.DB())
	if err != nil {
		t.Fatal(err)
	}
	store, err := aims.OpenDB(sess.DB(), loc, mem)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{store: store, mem: mem}
}

func seedAims(t *testing.T, mem *memory.Builtin, areas ...string) {
	t.Helper()
	ctx := context.Background()
	for _, a := range areas {
		if _, err := mem.Store(ctx, memory.KindInsight, memory.SubjectAimPrefix+a, a+" aim"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenDB_Nil(t *testing.T) {
	if _, err := aims.OpenDB(nil, nil, nil); err == nil {
		t.Fatal("want error")
	}
}

func TestLog_UnknownAreaWritesNothing(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "climbing")
	_, err := f.store.Log(ctx, aims.Event{What: "pizza", Day: "2026-09-22"}, map[string]int{"weight": -2})
	if err == nil || !strings.Contains(err.Error(), `unknown area "weight"`) || !strings.Contains(err.Error(), "climbing") {
		t.Fatalf("err=%v", err)
	}
	hist, err := f.store.History(ctx, "", "", "", 10)
	if err != nil || len(hist) != 0 {
		t.Fatalf("hist=%v err=%v", hist, err)
	}
}

func TestLog_ThreeScoresOneEvent(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "climbing", "drinking", "weight")
	ev, err := f.store.Log(ctx, aims.Event{
		What: "team dinner: 3 beers and a burger",
		Day:  "2026-09-22",
	}, map[string]int{"drinking": -2, "weight": -1, "climbing": -1})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID == 0 || ev.What != "team dinner: 3 beers and a burger" || ev.Day != "2026-09-22" {
		t.Fatalf("event=%+v", ev)
	}
	if len(ev.Scores) != 3 {
		t.Fatalf("scores=%v", ev.Scores)
	}
	got := map[string]int{}
	for _, sc := range ev.Scores {
		got[sc.Area] = sc.Value
	}
	if got["drinking"] != -2 || got["weight"] != -1 || got["climbing"] != -1 {
		t.Fatalf("got=%v", got)
	}
	hist, err := f.store.History(ctx, "drinking", "", "", 10)
	if err != nil || len(hist) != 1 || hist[0].ID != ev.ID {
		t.Fatalf("hist=%v err=%v", hist, err)
	}
	if len(hist[0].Scores) != 3 {
		t.Fatalf("filter still returns every score, got %v", hist[0].Scores)
	}
}

func TestLog_UnrefAppendsSameDay(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "training", "weight")
	a, err := f.store.Log(ctx, aims.Event{What: "ran five miles", Day: "2026-09-22"}, map[string]int{"training": 2})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.store.Log(ctx, aims.Event{What: "dessert", Day: "2026-09-22"}, map[string]int{"weight": -1})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatal("unref'd events on one day must append")
	}
	hist, err := f.store.History(ctx, "", "2026-09-22", "2026-09-22", 10)
	if err != nil || len(hist) != 2 {
		t.Fatalf("hist=%v err=%v", hist, err)
	}
}

func TestLog_RefDedupes(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "training", "weight")
	first, err := f.store.Log(ctx, aims.Event{
		What: "ran five miles", Day: "2026-09-22", Ref: "garmin:20481773",
	}, map[string]int{"training": 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.Log(ctx, aims.Event{
		What: "ran five miles", Day: "2026-09-22", Ref: "garmin:20481773",
	}, map[string]int{"training": 2, "weight": 1})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("rewrite must insert a new row")
	}
	old, err := f.store.Get(ctx, first.ID)
	if err != nil || old.SupersededBy == nil || *old.SupersededBy != second.ID {
		t.Fatalf("old=%+v err=%v", old, err)
	}
	hist, err := f.store.History(ctx, "", "", "", 10)
	if err != nil || len(hist) != 1 || hist[0].ID != second.ID {
		t.Fatalf("live hist=%v err=%v", hist, err)
	}
	got := map[string]int{}
	for _, sc := range second.Scores {
		got[sc.Area] = sc.Value
	}
	if got["training"] != 2 || got["weight"] != 1 {
		t.Fatalf("merged scores=%v", got)
	}
}

func TestLog_EventRewriteMergesScore(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "climbing", "drinking", "weight")
	ev, err := f.store.Log(ctx, aims.Event{What: "team dinner", Day: "2026-09-22"}, map[string]int{
		"drinking": -2, "weight": -1, "climbing": -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := f.store.Log(ctx, aims.Event{ID: ev.ID}, map[string]int{"weight": 0})
	if err != nil {
		t.Fatal(err)
	}
	if rewritten.What != "team dinner" || rewritten.Day != "2026-09-22" {
		t.Fatalf("copied fields: %+v", rewritten)
	}
	got := map[string]int{}
	for _, sc := range rewritten.Scores {
		got[sc.Area] = sc.Value
	}
	if got["drinking"] != -2 || got["climbing"] != -1 || got["weight"] != 0 {
		t.Fatalf("merged=%v", got)
	}
	old, err := f.store.Get(ctx, ev.ID)
	if err != nil || old.SupersededBy == nil || *old.SupersededBy != rewritten.ID {
		t.Fatalf("old=%+v err=%v", old, err)
	}
}

func TestLog_BackDatedDay(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "training")
	ev, err := f.store.Log(ctx, aims.Event{What: "ran (self-reported)", Day: "2026-09-16"}, map[string]int{"training": 2})
	if err != nil || ev.Day != "2026-09-16" {
		t.Fatalf("ev=%+v err=%v", ev, err)
	}
}

func TestLog_DefaultDayUsesLoc(t *testing.T) {
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training")
	ev, err := f.store.Log(ctx, aims.Event{What: "rest"}, map[string]int{"training": 0})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Now().In(loc).Format("2006-01-02")
	if ev.Day != want {
		t.Fatalf("day=%s want %s", ev.Day, want)
	}
}

func TestLog_ScoreOutOfRange(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "training")
	if _, err := f.store.Log(ctx, aims.Event{What: "x", Day: "2026-09-22"}, map[string]int{"training": 4}); err == nil {
		t.Fatal("want range error")
	}
}

func TestForget_CascadeAndKeepShared(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "climbing", "drinking", "weight")
	shared, err := f.store.Log(ctx, aims.Event{What: "team dinner", Day: "2026-09-22"}, map[string]int{
		"drinking": -2, "weight": -1, "climbing": -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	only, err := f.store.Log(ctx, aims.Event{What: "one beer", Day: "2026-09-23"}, map[string]int{"drinking": -1})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Forget(ctx, "drinking"); err != nil {
		t.Fatal(err)
	}
	orphan, err := f.store.Get(ctx, only.ID)
	if err != nil || orphan.SupersededBy == nil {
		t.Fatalf("orphan should be superseded: %+v err=%v", orphan, err)
	}
	kept, err := f.store.Get(ctx, shared.ID)
	if err != nil || kept.SupersededBy != nil {
		t.Fatalf("shared event should stay live: %+v err=%v", kept, err)
	}
	got := map[string]int{}
	for _, sc := range kept.Scores {
		got[sc.Area] = sc.Value
	}
	if _, ok := got["drinking"]; ok {
		t.Fatalf("drinking score still on event: %v", got)
	}
	if got["weight"] != -1 || got["climbing"] != -1 {
		t.Fatalf("kept=%v", got)
	}
	hist, err := f.store.History(ctx, "drinking", "", "", 10)
	if err != nil || len(hist) != 0 {
		t.Fatalf("drinking hist=%v err=%v", hist, err)
	}
}

func TestHistory_NewestFirstAndRange(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "training")
	for _, day := range []string{"2026-09-20", "2026-09-22", "2026-09-21"} {
		if _, err := f.store.Log(ctx, aims.Event{What: day, Day: day}, map[string]int{"training": 1}); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := f.store.History(ctx, "training", "2026-09-21", "2026-09-22", 10)
	if err != nil || len(hist) != 2 {
		t.Fatalf("hist=%v err=%v", hist, err)
	}
	if hist[0].Day != "2026-09-22" || hist[1].Day != "2026-09-21" {
		t.Fatalf("order %+v %+v", hist[0], hist[1])
	}
}

func TestBlock_SetAndGet(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	if _, ok, err := f.store.Block(ctx, "climbing"); err != nil || ok {
		t.Fatalf("empty block ok=%v err=%v", ok, err)
	}
	if err := f.store.SetBlock(ctx, "climbing", "2026-09-01", "2027-02-28"); err != nil {
		t.Fatal(err)
	}
	b, ok, err := f.store.Block(ctx, "climbing")
	if err != nil || !ok || b.FromDay != "2026-09-01" || b.ToDay != "2027-02-28" {
		t.Fatalf("block=%+v ok=%v err=%v", b, ok, err)
	}
	if err := f.store.SetBlock(ctx, "climbing", "2027-01-01", "2026-01-01"); err == nil {
		t.Fatal("want from-after-to error")
	}
}

func TestAreas_SkipsBootstrap(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	if _, err := f.mem.Store(ctx, memory.KindInsight, memory.SubjectAimBootstrap, "asked"); err != nil {
		t.Fatal(err)
	}
	seedAims(t, f.mem, "training")
	areas, err := f.store.Areas(ctx)
	if err != nil || len(areas) != 1 || areas[0] != "training" {
		t.Fatalf("areas=%v err=%v", areas, err)
	}
}

func TestLog_MeasurementRidesAlong(t *testing.T) {
	ctx := context.Background()
	f := openFixture(t, time.UTC)
	seedAims(t, f.mem, "weight")
	v := 191.9
	ev, err := f.store.Log(ctx, aims.Event{
		What: "weighed in", Day: "2026-09-22", Metric: "weight", Value: &v, Unit: "lb", Source: "tool",
	}, map[string]int{"weight": 0})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Metric != "weight" || ev.Unit != "lb" || ev.Source != "tool" || ev.Value == nil || *ev.Value != 191.9 {
		t.Fatalf("measurement=%+v", ev)
	}
}
