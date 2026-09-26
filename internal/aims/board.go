package aims

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shotah/ai-gantry/internal/memory"
)

const sentenceMax = 240

// areaWire is the shape the phone keeps. Anything else is dropped there.
var areaWire = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// DayCell is one local day on the phone board. Events are the same ids
// /aims prints. An empty day is score 0 and an empty events list.
type DayCell struct {
	Day    string  `json:"day"`
	Score  int     `json:"score"`
	Events []int64 `json:"events"`
}

// WeekCell is one Sunday-start week. Metrics is empty, never null.
type WeekCell struct {
	Start   string    `json:"start"`
	Mean    float64   `json:"mean"`
	Up      int       `json:"up"`
	Against int       `json:"against"`
	Metrics []Measure `json:"metrics"`
}

// Link is one next-day correlation the stamps would print.
type Link struct {
	A string  `json:"a"`
	B string  `json:"b"`
	R float64 `json:"r"`
	N int     `json:"n"`
}

// Row is one live aim on the phone board. JSON names are the phone's
// parser (docs/aims-progress.md §4.4.1). Optional objects are omitted
// whole; days and metrics are always arrays.
type Row struct {
	Area     string      `json:"area"`
	Sentence string      `json:"sentence"`
	Rating30 float64     `json:"rating30"`
	Sum7     int         `json:"sum7"`
	Streak   int         `json:"streak"`
	Note     string      `json:"note"`
	NoteAt   string      `json:"note_at,omitempty"`
	Days     []DayCell   `json:"days"`
	Weeks    []WeekCell  `json:"weeks,omitempty"`
	Slope    *float64    `json:"slope,omitempty"`
	Block    *BlockStats `json:"block,omitempty"`
	Effect   *Corr       `json:"effect,omitempty"`
}

// Board renders the phone snapshot from the same math as [progress]
// and /aims. Rows follow capAreas. A row with no sentence, or an area
// the phone's pattern rejects, is dropped. Links are the cross-aim
// lines, strongest first, cap 3. An empty rows slice is a real board.
func (s *Store) Board(ctx context.Context, areas []string, now time.Time) ([]Row, []Link, error) {
	if now.IsZero() {
		now = time.Now()
	}
	capped := capAreas(areas)
	rows := make([]Row, 0, len(capped))
	for _, area := range capped {
		row, ok, err := s.boardRow(ctx, area, now)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		rows = append(rows, row)
	}
	var links []Link
	for _, h := range s.crossHits(ctx, capped, now) {
		links = append(links, Link{A: h.c.A, B: h.c.B, R: h.c.R, N: h.c.N})
	}
	return rows, links, nil
}

func (s *Store) boardRow(ctx context.Context, area string, now time.Time) (Row, bool, error) {
	if !areaWire.MatchString(area) {
		return Row{}, false, nil
	}
	sentence, ok, err := s.sentence(ctx, area)
	if err != nil || !ok {
		return Row{}, false, err
	}
	today := now.In(s.loc).Format(dayLayout)
	from, err := addDays(today, -4)
	if err != nil {
		return Row{}, false, err
	}
	days, err := s.DayScores(ctx, area, from, today)
	if err != nil {
		return Row{}, false, err
	}
	st, err := s.StatsAt(ctx, area, now)
	if err != nil {
		return Row{}, false, err
	}
	weeks, err := s.Weeks(ctx, area, now)
	if err != nil {
		return Row{}, false, err
	}
	row := Row{
		Area:     area,
		Sentence: sentence,
		Rating30: st.Rating30,
		Sum7:     st.Sum7,
		Streak:   st.Streak,
		Note:     st.LastNote,
		Days:     dayCells(days),
	}
	if !st.LastNoteAt.IsZero() {
		row.NoteAt = st.LastNoteAt.In(s.loc).Format(dayLayout)
	}
	if cells := weekCells(weeks); len(cells) > 0 {
		row.Weeks = cells
	}
	if slope, ok := WeekSlope(weeks); ok {
		row.Slope = &slope
	}
	block, ok, err := s.BlockStatsAt(ctx, area, now)
	if err != nil {
		return Row{}, false, err
	}
	if ok && block.Days > 0 {
		row.Block = &block
	}
	effect, effectOK := Effect(area, weeks)
	if StampCorr(effectOK, effect.R) {
		row.Effect = &effect
	}
	return row, true, nil
}

func (s *Store) sentence(ctx context.Context, area string) (string, bool, error) {
	if s.mem == nil {
		return "", false, nil
	}
	e, ok, err := s.mem.ActiveByKindSubject(ctx, memory.KindInsight, memory.SubjectAimPrefix+area)
	if err != nil || !ok {
		return "", false, err
	}
	text := strings.TrimSpace(e.Content)
	if text == "" {
		return "", false, nil
	}
	if utf8.RuneCountInString(text) > sentenceMax {
		text = string([]rune(text)[:sentenceMax])
	}
	return text, true, nil
}

func dayCells(days []DayScore) []DayCell {
	out := make([]DayCell, len(days))
	for i, d := range days {
		ids := make([]int64, 0, len(d.Events))
		for _, ev := range d.Events {
			ids = append(ids, ev.ID)
		}
		out[i] = DayCell{Day: d.Day, Score: d.Score, Events: ids}
	}
	return out
}

func weekCells(weeks []Week) []WeekCell {
	if len(weeks) == 0 {
		return nil
	}
	out := make([]WeekCell, len(weeks))
	for i, w := range weeks {
		out[i] = WeekCell{
			Start:   w.Start,
			Mean:    w.Mean,
			Up:      w.Up,
			Against: w.Against,
			Metrics: flattenMetrics(w.Metrics),
		}
	}
	return out
}

func flattenMetrics(in map[string]Measure) []Measure {
	out := make([]Measure, 0, len(in))
	for _, m := range in {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Metric != out[j].Metric {
			return out[i].Metric < out[j].Metric
		}
		return out[i].Unit < out[j].Unit
	})
	return out
}
