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
	} {
		if call, ok := provider.ParseToolCallText(content); ok {
			t.Errorf("ParseToolCallText(%q) = %+v, want rejected", content, call)
		}
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
