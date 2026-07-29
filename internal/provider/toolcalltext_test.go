package provider_test

import (
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/provider"
)

func TestParseToolCallText(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		wantName string
		wantArgs string // substring
	}{{
		name:     "bare object with parameters key",
		content:  "{\n  \"name\": \"garmin__get_daily_activity\",\n  \"parameters\": {\n    \"date\": \"2026-07-28\"\n  }\n}",
		wantName: "garmin__get_daily_activity",
		wantArgs: "2026-07-28",
	}, {
		name:     "arguments key",
		content:  `{"name":"garmin__get_hrv","arguments":{"date":"2026-07-27"}}`,
		wantName: "garmin__get_hrv",
		wantArgs: "2026-07-27",
	}, {
		name:     "stringified arguments",
		content:  `{"name":"garmin__get_hrv","arguments":"{\"date\":\"2026-07-27\"}"}`,
		wantName: "garmin__get_hrv",
		wantArgs: "2026-07-27",
	}, {
		name:     "markdown fence",
		content:  "```json\n{\"name\":\"garmin__get_hrv\",\"parameters\":{}}\n```",
		wantName: "garmin__get_hrv",
		wantArgs: "{}",
	}, {
		name:     "tool_call tags",
		content:  "<tool_call>\n{\"name\": \"math__evaluate\", \"arguments\": {\"expr\": \"2+2\"}}\n</tool_call>",
		wantName: "math__evaluate",
		wantArgs: "2+2",
	}, {
		name:     "embedded in prose",
		content:  `Pulling that up now: {"name":"garmin__get_sleep","parameters":{"date":"2026-07-28"}} — one sec.`,
		wantName: "garmin__get_sleep",
		wantArgs: "2026-07-28",
	}, {
		name:     "no arguments at all still runs",
		content:  `{"name":"garmin__get_hrv"}`,
		wantName: "garmin__get_hrv",
		wantArgs: "{}",
	}, {
		// Braces inside a string value must not end the object early.
		name:     "braces inside string values",
		content:  `{"name":"math__evaluate","arguments":{"expr":"{2+2}"}}`,
		wantName: "math__evaluate",
		wantArgs: "{2+2}",
	}, {
		// Qwen prints MCP calls as {server__tool(k="v")} instead of tool_calls.
		name: "fn-style braced kwargs",
		content: `{google__calendar_list_events(time_min="2026-07-29T00:00:00-07:00", ` +
			`time_max="2026-07-30T00:00:00-07:00", calendar_id="primary", ` +
			`user_google_email="christopherblodgett@gmail.com")}`,
		wantName: "google__calendar_list_events",
		wantArgs: "christopherblodgett@gmail.com",
	}, {
		name:     "fn-style without braces",
		content:  `Calling garmin__get_sleep(date="2026-07-29") now.`,
		wantName: "garmin__get_sleep",
		wantArgs: "2026-07-29",
	}, {
		name: "fn-style duplicated in reply (take first)",
		content: `{google__calendar_list_events(calendar_id="primary", time_min="2026-07-29T00:00:00-07:00")}` +
			"\n\n" +
			`{google__calendar_list_events(calendar_id="primary", time_min="2026-07-29T00:00:00-07:00")}`,
		wantName: "google__calendar_list_events",
		wantArgs: "primary",
	}, {
		// Brat mode: name in prose, args in a markdown json fence.
		name: "args-only json fence with tool name nearby",
		content: "google__calendar_list_events\n```json\n" +
			`{"calendar_id":"primary","time_min":"2026-07-29T00:00:00-07:00","time_max":"2026-07-30T00:00:00-07:00"}` +
			"\n```",
		wantName: "google__calendar_list_events",
		wantArgs: "time_min",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call, ok := provider.ParseToolCallText(tc.content)
			if !ok {
				t.Fatalf("ParseToolCallText(%q) = not a call", tc.content)
			}
			if call.Name != tc.wantName {
				t.Errorf("name = %q, want %q", call.Name, tc.wantName)
			}
			if !strings.Contains(call.Arguments, tc.wantArgs) {
				t.Errorf("arguments = %q, want to contain %q", call.Arguments, tc.wantArgs)
			}
			if call.ID == "" {
				t.Error("id empty; the tool result could not be matched to the call")
			}
		})
	}
}

func TestParseToolCallText_Rejects(t *testing.T) {
	for _, content := range []string{
		"",
		"You rode 21mi yesterday.",
		`{"protein":180,"carbs":220}`,           // JSON, but no name
		`{"name":"","arguments":{}}`,            // empty name
		`{"nickname":"tim","arguments":{}}`,     // name-ish key that is not name
		"{\"name\":\"x\", \"arguments\": {oops", // unbalanced
		// Real answer that mentions a tool and includes unrelated JSON — do not hijack.
		`I already used google__calendar_list_events earlier. Macros: {"protein":180,"carbs":220}`,
	} {
		if call, ok := provider.ParseToolCallText(content); ok {
			t.Errorf("ParseToolCallText(%q) = %+v, want rejected", content, call)
		}
	}
}

func TestParseToolCallTextHinted_ArgsOnlyFromPriorTurn(t *testing.T) {
	content := "```json\n" +
		`{"calendar_id":"primary","time_max":"2026-07-30T00:00:00-07:00","time_min":"2026-07-29T00:00:00-07:00","user_google_email":"christopherblodgett@gmail.com"}` +
		"\n```"
	call, ok := provider.ParseToolCallTextHinted(content, []string{"google__calendar_list_events"})
	if !ok {
		t.Fatal("expected salvage from hint + args-only fence")
	}
	if call.Name != "google__calendar_list_events" {
		t.Fatalf("name = %q", call.Name)
	}
	if !strings.Contains(call.Arguments, "christopherblodgett@gmail.com") {
		t.Fatalf("arguments = %q", call.Arguments)
	}
}

// Ids must not collide: the tool result is matched back by id.
func TestParseToolCallText_UniqueIDs(t *testing.T) {
	a, _ := provider.ParseToolCallText(`{"name":"garmin__get_hrv"}`)
	b, _ := provider.ParseToolCallText(`{"name":"garmin__get_hrv"}`)
	if a.ID == b.ID {
		t.Fatalf("both calls got id %q", a.ID)
	}
}
