package cron_test

import (
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/cron"
)

func TestParseSchedule_RelativeAndDaily(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, loc)

	p, err := cron.ParseSchedule("in 30m", "once", loc, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != cron.KindOnce {
		t.Fatalf("kind=%s", p.Kind)
	}
	want := now.Add(30 * time.Minute).UTC()
	if !p.NextRun.Equal(want) {
		t.Fatalf("next=%s want %s", p.NextRun, want)
	}

	p, err = cron.ParseSchedule("17:00", "daily", loc, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != cron.KindDaily || p.Expr != "17:00" {
		t.Fatalf("%+v", p)
	}
	if p.NextRun.In(loc).Hour() != 17 {
		t.Fatalf("hour=%d", p.NextRun.In(loc).Hour())
	}

	p, err = cron.ParseSchedule("every:1h", "", loc, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != cron.KindEvery {
		t.Fatalf("%+v", p)
	}
}

func TestAdvanceNext(t *testing.T) {
	from := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	_, ok, err := cron.AdvanceNext(cron.KindOnce, from.Format(time.RFC3339Nano), "UTC", from)
	if err != nil || ok {
		t.Fatalf("once should not repeat: ok=%v err=%v", ok, err)
	}
	next, ok, err := cron.AdvanceNext(cron.KindDaily, "17:00", "UTC", from)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if next.In(time.UTC).Day() != 23 {
		t.Fatalf("next day=%d", next.Day())
	}
}
