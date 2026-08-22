package examples_test

import (
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/examples"
	"github.com/shotah/ai-gantry/internal/provider"
)

func TestEligibleAndPick_FiltersByLiveServers(t *testing.T) {
	seeds := []examples.Seed{
		{ID: "g", Title: "G", Servers: []string{"google"}, Steps: []string{"a"}},
		{ID: "gs", Title: "GS", Servers: []string{"google", "google-search"}, Steps: []string{"a"}},
		{ID: "yt", Title: "YT", Servers: []string{"youtube", "cast"}, Steps: []string{"a"}},
	}
	defs := []provider.ToolDef{
		{Name: "google__calendar_list_events"},
		{Name: "google-search__web_search"},
		{Name: "memory_recall"}, // builtin — ignored for prefixes
	}
	live := examples.ServerPrefixes(defs)
	if !live["google"] || !live["google-search"] {
		t.Fatalf("live=%v", live)
	}
	if live["youtube"] {
		t.Fatal("youtube should be absent")
	}

	elig := examples.Eligible(seeds, live)
	if len(elig) != 2 {
		t.Fatalf("eligible=%d want 2: %+v", len(elig), elig)
	}
	for _, s := range elig {
		if s.ID == "yt" {
			t.Fatal("youtube+cast seed should be filtered out")
		}
	}

	got, ok := examples.PickFrom(seeds, defs)
	if !ok {
		t.Fatal("expected a pick")
	}
	if got.ID != "g" && got.ID != "gs" {
		t.Fatalf("unexpected pick %q", got.ID)
	}
}

func TestPick_NoMatch(t *testing.T) {
	_, ok := examples.PickFrom(nil, nil)
	if ok {
		t.Fatal("empty seed pool should not match")
	}
}

func TestPick_HarnessMatchesZeroMCP(t *testing.T) {
	got, ok := examples.PickFrom(examples.DefaultSeeds, nil)
	if !ok {
		t.Fatal("harness seeds should match a zero-MCP catalog")
	}
	if len(got.Servers) != 0 {
		t.Fatalf("expected harness seed, got %+v", got)
	}
}

func TestDefaultSeeds_HarnessAlwaysEligible(t *testing.T) {
	elig := examples.Eligible(examples.DefaultSeeds, map[string]bool{})
	ids := map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	for _, want := range []string{"first-aim", "cron-reminder", "memory-skill", "mcp-enable"} {
		if !ids[want] {
			t.Fatalf("zero-MCP catalog missing harness seed %q (eligible=%v)", want, ids)
		}
	}
	if ids["math-eval"] || ids["google-calendar-contact"] {
		t.Fatal("MCP seeds should not match a zero-MCP catalog")
	}
}

func TestPolishAndFallback(t *testing.T) {
	s := examples.Seed{
		ID: "x", Title: "Demo title", Servers: []string{"google"},
		Steps: []string{"step one", "step two"},
	}
	p := examples.PolishPrompt(s)
	if !strings.Contains(p, "Demo title") || !strings.Contains(p, "step one") {
		t.Fatalf("polish missing content: %s", p)
	}
	if !strings.Contains(p, examples.OffHint) {
		t.Fatal("polish missing off hint")
	}
	if !strings.Contains(p, "cron_schedule") {
		t.Fatal("polish should mention recurring reminder guidance")
	}
	if !strings.Contains(p, "watch_add") {
		t.Fatal("polish should mention watch subscription guidance")
	}
	if !strings.Contains(p, "last pin") {
		t.Fatal("polish should mention Telegram last-pin guidance")
	}
	if !strings.Contains(p, "self_note") || !strings.Contains(p, "mcp_enable") {
		t.Fatal("polish should mention first-aim / mcp_enable guidance")
	}
	f := examples.FallbackFormat(s)
	if !strings.Contains(f, "Demo title") || !strings.Contains(f, examples.OffHint) {
		t.Fatalf("fallback: %s", f)
	}
}

func TestDefaultSeeds_GHealthVsGarminAndReminders(t *testing.T) {
	crystal := examples.ServerPrefixes([]provider.ToolDef{
		{Name: "google__calendar_list_events"},
		{Name: "ghealth__sleep_get"},
		{Name: "strava__activities_list"},
		{Name: "strava__activities_update"},
	})
	elig := examples.Eligible(examples.DefaultSeeds, crystal)
	ids := map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	for _, want := range []string{
		"morning-brief", "morning-brief-ghealth", "evening-winddown",
		"weekly-training-strava", "weekly-training-ghealth",
		"ghealth-strava-enrich", "ghealth-sleep", "fitness-recovery-ghealth",
	} {
		if !ids[want] {
			t.Fatalf("Crystal/Sara catalog missing seed %q (eligible=%v)", want, ids)
		}
	}
	for _, ban := range []string{"morning-brief-garmin", "garmin-sleep", "garmin-strava-enrich", "fitness-recovery-garmin"} {
		if ids[ban] {
			t.Fatalf("garmin seed %q should not match ghealth-only catalog", ban)
		}
	}

	garminLab := examples.ServerPrefixes([]provider.ToolDef{
		{Name: "garmin__sleep_get"},
		{Name: "garmin__activities_get"},
		{Name: "strava__activities_list"},
		{Name: "strava__activities_update"},
	})
	elig = examples.Eligible(examples.DefaultSeeds, garminLab)
	ids = map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	if !ids["garmin-strava-enrich"] || !ids["weekly-training-strava"] {
		t.Fatalf("garmin+strava catalog missing enrich/week seeds: %v", ids)
	}
	if ids["ghealth-strava-enrich"] {
		t.Fatal("ghealth enrich should not match garmin-only health catalog")
	}
}

func TestDefaultSeeds_Maps(t *testing.T) {
	mapsOnly := examples.ServerPrefixes([]provider.ToolDef{
		{Name: "maps__place_search"},
		{Name: "maps__route_eta"},
	})
	elig := examples.Eligible(examples.DefaultSeeds, mapsOnly)
	ids := map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	if !ids["maps-near-me"] || !ids["maps-route-eta"] {
		t.Fatalf("maps catalog missing pin seeds: %v", ids)
	}
	if ids["feeds-nws-watch"] {
		t.Fatal("feeds seed should not match maps-only catalog")
	}
}

func TestDefaultSeeds_FeedsAndTwitter(t *testing.T) {
	feedsOnly := examples.ServerPrefixes([]provider.ToolDef{
		{Name: "feeds__items_list"},
		{Name: "feeds__source_resolve"},
	})
	elig := examples.Eligible(examples.DefaultSeeds, feedsOnly)
	ids := map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	if !ids["feeds-nws-watch"] {
		t.Fatalf("feeds catalog missing feeds-nws-watch: %v", ids)
	}
	if ids["twitter-watch"] {
		t.Fatal("twitter-watch should not match feeds-only catalog")
	}

	twitterOnly := examples.ServerPrefixes([]provider.ToolDef{
		{Name: "twitter__posts_list"},
	})
	elig = examples.Eligible(examples.DefaultSeeds, twitterOnly)
	ids = map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	if !ids["twitter-watch"] {
		t.Fatalf("twitter catalog missing twitter-watch: %v", ids)
	}
	if ids["feeds-nws-watch"] {
		t.Fatal("feeds-nws-watch should not match twitter-only catalog")
	}
}

func TestDefaultSeeds_MathSearchYoutube(t *testing.T) {
	mathOnly := examples.ServerPrefixes([]provider.ToolDef{
		{Name: "math__expression_evaluate"},
	})
	elig := examples.Eligible(examples.DefaultSeeds, mathOnly)
	ids := map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	if !ids["math-eval"] || !ids["first-aim"] {
		t.Fatalf("math catalog missing math-eval/first-aim: %v", ids)
	}
	if ids["math-sheets"] {
		t.Fatal("math-sheets needs google")
	}

	searchOnly := examples.ServerPrefixes([]provider.ToolDef{
		{Name: "google-search__web_search"},
	})
	elig = examples.Eligible(examples.DefaultSeeds, searchOnly)
	ids = map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	if !ids["search-decide"] {
		t.Fatalf("search catalog missing search-decide: %v", ids)
	}
	if ids["search-calendar-event"] {
		t.Fatal("search-calendar-event needs google")
	}

	ytOnly := examples.ServerPrefixes([]provider.ToolDef{
		{Name: "youtube__videos_search"},
	})
	elig = examples.Eligible(examples.DefaultSeeds, ytOnly)
	ids = map[string]bool{}
	for _, s := range elig {
		ids[s.ID] = true
	}
	if !ids["youtube-find"] {
		t.Fatalf("youtube catalog missing youtube-find: %v", ids)
	}
	if ids["youtube-cast"] {
		t.Fatal("youtube-cast needs cast")
	}
}
