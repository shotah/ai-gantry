package aims_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/aims"
	"github.com/shotah/ai-gantry/internal/memory"
)

func TestBoard_DropsEmptySentenceAndKeepsFiveDays(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	if _, err := f.mem.Store(ctx, memory.KindInsight, "aim/weight", "lose weight"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, loc)
	if _, err := f.store.Log(ctx, aims.Event{What: "weigh", Day: "2026-09-27"}, map[string]int{"weight": 1}); err != nil {
		t.Fatal(err)
	}
	rows, links, err := f.store.Board(ctx, []string{"training", "Not Area", "weight"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 || len(rows) != 1 || rows[0].Area != "weight" {
		t.Fatalf("rows=%+v links=%+v", rows, links)
	}
	if len(rows[0].Days) != 5 || rows[0].Days[0].Day != "2026-09-23" || rows[0].Days[4].Day != "2026-09-27" {
		t.Fatalf("days=%+v", rows[0].Days)
	}
	raw, err := json.Marshal(rows[0].Days[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"events":[]`) {
		t.Fatalf("empty day must send events []: %s", raw)
	}
	if rows[0].Sentence != "lose weight" || rows[0].Note != "" || rows[0].Slope != nil || rows[0].Weeks == nil {
		t.Fatalf("row=%+v", rows[0])
	}
}

func TestBoard_OneWeekOmitsSlopeAndLinks(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	seedAims(t, f.mem, "training")
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, loc)
	if _, err := f.store.Log(ctx, aims.Event{What: "gym", Day: "2026-09-26"}, map[string]int{"training": 2}); err != nil {
		t.Fatal(err)
	}
	rows, links, err := f.store.Board(ctx, []string{"training"}, now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(struct {
		Aims  []aims.Row  `json:"aims"`
		Links []aims.Link `json:"links,omitempty"`
	}{rows, links})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if len(rows) != 1 || len(rows[0].Weeks) != 1 || rows[0].Slope != nil || len(links) != 0 {
		t.Fatalf("one week: %s", body)
	}
	if strings.Contains(body, `"slope"`) || strings.Contains(body, `"links"`) {
		t.Fatalf("optional trend leaked: %s", body)
	}
}

func TestBoard_NineWeeksCarryLinks(t *testing.T) {
	ctx := context.Background()
	loc := mustLA(t)
	f := openFixture(t, loc)
	seedAims(t, f.mem, "drinking", "climbing")
	start := time.Date(2026, 8, 2, 12, 0, 0, 0, loc)
	for w := 0; w < 9; w++ {
		day := start.AddDate(0, 0, w*7).Format("2006-01-02")
		next := start.AddDate(0, 0, w*7+1).Format("2006-01-02")
		score := -2
		if w%2 == 0 {
			score = 2
		}
		if _, err := f.store.Log(ctx, aims.Event{What: "night", Day: day}, map[string]int{"drinking": score}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Log(ctx, aims.Event{What: "session", Day: next}, map[string]int{"climbing": score}); err != nil {
			t.Fatal(err)
		}
	}
	now := start.AddDate(0, 0, 9*7)
	rows, links, err := f.store.Board(ctx, []string{"drinking", "climbing"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	var drink aims.Row
	for _, row := range rows {
		if row.Area == "drinking" {
			drink = row
		}
	}
	if len(drink.Weeks) < 9 || drink.Slope == nil {
		t.Fatalf("weeks=%d slope=%v", len(drink.Weeks), drink.Slope)
	}
	if len(links) == 0 || links[0].A == "" || links[0].N < 8 {
		t.Fatalf("links=%+v", links)
	}
}
