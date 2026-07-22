package mcp

import "testing"

func TestFilterTools_AllowAndExclude(t *testing.T) {
	tools := []Tool{
		{OriginalName: "get_sleep"},
		{OriginalName: "get_weight"},
		{OriginalName: "raw_dump"},
		{OriginalName: "get_hrv"},
	}
	kept, before := filterTools(ServerSpec{
		Tools:   []string{"get_sleep", "get_weight", "raw_dump"},
		Exclude: []string{"raw_*"},
	}, tools)
	if before != 4 {
		t.Fatalf("before=%d", before)
	}
	if len(kept) != 2 {
		t.Fatalf("kept=%v", names(kept))
	}
	if kept[0].OriginalName != "get_sleep" || kept[1].OriginalName != "get_weight" {
		t.Fatalf("%v", names(kept))
	}
}

func TestFilterTools_NoFilters(t *testing.T) {
	tools := []Tool{{OriginalName: "a"}, {OriginalName: "b"}}
	kept, before := filterTools(ServerSpec{}, tools)
	if before != 2 || len(kept) != 2 {
		t.Fatalf("before=%d kept=%d", before, len(kept))
	}
}

func TestPrefixFor(t *testing.T) {
	if prefixFor(ServerSpec{Name: "garmin"}) != "garmin" {
		t.Fatal("default")
	}
	if prefixFor(ServerSpec{Name: "garmin", ToolsPrefix: "garm"}) != "garm" {
		t.Fatal("override")
	}
}

func names(tools []Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.OriginalName
	}
	return out
}
