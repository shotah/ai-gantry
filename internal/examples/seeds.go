// Package examples provides inventory-aware capability recipe seeds for
// /examples and proactive training-wheels pings.
package examples

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/shotah/ai-gantry/internal/provider"
)

// Seed is a curated multi-step recipe keyed by required MCP server prefixes.
type Seed struct {
	ID      string
	Title   string
	Servers []string // required prefixes (e.g. "google"); all must be live
	Steps   []string
}

// DefaultSeeds is the v1 curated pool. Filter with Eligible / Pick against live tools.
var DefaultSeeds = []Seed{
	{
		ID:      "google-calendar-contact",
		Title:   "Calendar: look up a contact, check free/busy, schedule an event",
		Servers: []string{"google"},
		Steps: []string{
			"Find the contact in Google Contacts / people search",
			"Check calendar free/busy for a sensible slot",
			"Create a calendar event with that contact",
		},
	},
	{
		ID:      "google-gmail-today-tasks",
		Title:   "Inbox triage: today's mail into a task list",
		Servers: []string{"google"},
		Steps: []string{
			"Search Gmail for today's messages",
			"Pull a couple of important threads",
			"Create tasks for anything that needs follow-up",
		},
	},
	{
		ID:      "google-sheets-track",
		Title:   "Sheets: log a quick tracking row",
		Servers: []string{"google"},
		Steps: []string{
			"Open or find the tracking spreadsheet",
			"Append a dated row with what you want remembered",
			"Confirm the update",
		},
	},
	{
		ID:      "morning-brief",
		Title:   "Daily morning brief: calendar + mail, then schedule it as a recurring reminder",
		Servers: []string{"google"},
		Steps: []string{
			"List today's Google Calendar events",
			"Search Gmail for anything urgent from the last day",
			"Deliver a short morning brief (agenda + must-reads)",
			"Offer to cron_schedule it daily at a morning time they choose",
		},
	},
	{
		ID:      "morning-brief-ghealth",
		Title:   "Daily morning brief with Google Health: sleep/recovery + calendar + mail",
		Servers: []string{"google", "ghealth"},
		Steps: []string{
			"Pull last night's sleep via ghealth__sleep_get (wake-up day)",
			"List today's Google Calendar + skim urgent Gmail",
			"Brief: recovery note + agenda + must-reads",
			"Offer to cron_schedule this as a daily morning reminder",
		},
	},
	{
		ID:      "morning-brief-garmin",
		Title:   "Daily morning brief with Garmin: sleep/recovery + calendar + mail",
		Servers: []string{"google", "garmin"},
		Steps: []string{
			"Pull last night's sleep via garmin__sleep_get (wake-up day)",
			"List today's Google Calendar + skim urgent Gmail",
			"Brief: recovery note + agenda + must-reads",
			"Offer to cron_schedule this as a daily morning reminder",
		},
	},
	{
		ID:      "evening-winddown",
		Title:   "Evening wind-down: tomorrow's calendar, open loops, quiet close",
		Servers: []string{"google"},
		Steps: []string{
			"List tomorrow's Google Calendar events",
			"Scan open Google Tasks / leftover mail that still needs a decision",
			"Deliver a short wind-down: what's next, what can wait",
			"Offer to cron_schedule it daily at an evening time they choose",
		},
	},
	{
		ID:      "search-calendar-event",
		Title:   "Web search → calendar: put a real place/event on the calendar",
		Servers: []string{"google-search", "google"},
		Steps: []string{
			"Search the web for the place or event details",
			"Create a Google Calendar event with the useful bits",
		},
	},
	{
		ID:      "housing-sheets",
		Title:   "Housing scout: search listings, compile into Sheets, notify when done",
		Servers: []string{"google-search", "rentals", "google"},
		Steps: []string{
			"Clarify neighborhood / budget constraints",
			"Search rentals for matching listings",
			"Write the best options into a Google Sheet",
			"Tell the human the sheet is ready",
		},
	},
	{
		ID:      "flights-calendar",
		Title:   "Flights: search options and hold a travel block on the calendar",
		Servers: []string{"flights", "google"},
		Steps: []string{
			"Resolve airports if needed",
			"Search flight offers for the trip",
			"Block the travel window on Google Calendar (no ticket purchase)",
		},
	},
	{
		ID:      "weekly-training-strava",
		Title:   "Weekly training summary: roll up Strava, then schedule a Sunday reminder",
		Servers: []string{"strava"},
		Steps: []string{
			"List this week's Strava activities",
			"Pull athlete stats / zones as needed",
			"Write a short weekly training summary (volume, highlights, one suggestion)",
			"Offer to cron_schedule it weekly (e.g. Sunday evening)",
		},
	},
	{
		ID:      "weekly-training-ghealth",
		Title:   "Weekly training summary: Strava load + Google Health recovery",
		Servers: []string{"strava", "ghealth"},
		Steps: []string{
			"List this week's Strava activities and stats",
			"Pull recent ghealth sleep / HRV / weight for recovery context",
			"Summarize load vs recovery for the week",
			"Offer to cron_schedule a weekly training summary reminder",
		},
	},
	{
		ID:      "weekly-training-garmin",
		Title:   "Weekly training summary: Strava load + Garmin recovery",
		Servers: []string{"strava", "garmin"},
		Steps: []string{
			"List this week's Strava activities and stats",
			"Pull recent Garmin sleep / body battery / readiness",
			"Summarize load vs recovery for the week",
			"Offer to cron_schedule a weekly training summary reminder",
		},
	},
	{
		ID:      "garmin-strava-enrich",
		Title:   "Enrich Strava: pull Garmin workout detail and update the Strava activity description",
		Servers: []string{"garmin", "strava"},
		Steps: []string{
			"Find the matching recent activity on Garmin and Strava",
			"Pull Garmin activity detail (laps, HR, notes-worthy stats)",
			"Draft a rich description from that detail",
			"Update the Strava activity description via strava__activities_update",
		},
	},
	{
		ID:      "ghealth-strava-enrich",
		Title:   "Enrich Strava: pull Google Health workout detail and update the Strava description",
		Servers: []string{"ghealth", "strava"},
		Steps: []string{
			"Find the matching recent activity on Google Health and Strava",
			"Pull ghealth__activities_get detail for the session",
			"Draft a rich description from that detail",
			"Update the Strava activity description via strava__activities_update",
		},
	},
	{
		ID:      "fitness-recovery-ghealth",
		Title:   "Training check: yesterday's Strava + last night's Google Health recovery",
		Servers: []string{"strava", "ghealth"},
		Steps: []string{
			"List yesterday's Strava activities",
			"Pull ghealth sleep / HRV for last night",
			"Summarize load vs recovery in plain language",
		},
	},
	{
		ID:      "fitness-recovery-garmin",
		Title:   "Training check: yesterday's Strava + last night's Garmin recovery",
		Servers: []string{"strava", "garmin"},
		Steps: []string{
			"List yesterday's Strava activities",
			"Pull Garmin sleep / recovery for last night",
			"Summarize load vs recovery in plain language",
		},
	},
	{
		ID:      "ghealth-sleep",
		Title:   "Sleep: pull last night's Google Health sleep and interpret it",
		Servers: []string{"ghealth"},
		Steps: []string{
			"Call ghealth__sleep_get for the wake-up day",
			"Summarize duration, stages, and one actionable takeaway",
		},
	},
	{
		ID:      "garmin-sleep",
		Title:   "Sleep: pull last night's Garmin sleep and interpret it",
		Servers: []string{"garmin"},
		Steps: []string{
			"Call garmin__sleep_get for the wake-up day",
			"Summarize duration, stages, and one actionable takeaway",
		},
	},
	{
		ID:      "youtube-cast",
		Title:   "YouTube → Cast: find a video and beam it to a room speaker/TV",
		Servers: []string{"youtube", "cast"},
		Steps: []string{
			"Search YouTube for what they asked",
			"List Cast devices and pick the room",
			"Beam the videoId to that device",
		},
	},
	{
		ID:      "cars-search",
		Title:   "Cars: search inventory and hand off the best listing URLs",
		Servers: []string{"cars"},
		Steps: []string{
			"Search for-sale listings by zip/make/model",
			"Pull detail on the top matches",
			"Share listing links — do not submit dealer leads",
		},
	},
	{
		ID:      "math-sheets",
		Title:   "Numbers: compute with math tools, then log the result in Sheets",
		Servers: []string{"math", "google"},
		Steps: []string{
			"Evaluate the calculation with the math MCP (no mental arithmetic)",
			"Append the inputs + result to a Google Sheet row",
		},
	},
	{
		ID:      "feeds-nws-watch",
		Title:   "RSS watch: subscribe to a feed (NWS alerts, a blog, a GitHub release) and get a text when something posts",
		Servers: []string{"feeds"},
		Steps: []string{
			"Resolve the feed URL with feeds__source_resolve (NWS zone, site, or GitHub repo)",
			"Offer to watch_add feeds__items_list with that url (interval 15m is fine)",
			"Explain quiet ticks stay silent; a new item can Push or [silent] if it's noise",
		},
	},
	{
		ID:      "twitter-watch",
		Title:   "X watch: subscribe to a public account and get a text when they post",
		Servers: []string{"twitter"},
		Steps: []string{
			"Confirm the @handle they care about",
			"Offer to watch_add twitter__posts_list with that handle (prefer 30–60m — X reads are pay-per-use)",
			"Explain the first poll seeds the cursor (no backlog dump); [silent] still skips noise",
		},
	},
}

// ServerPrefixes returns the set of MCP server prefixes present in defs
// (same split as mcp.EstimateSchemaBudget: name before "__").
func ServerPrefixes(defs []provider.ToolDef) map[string]bool {
	out := make(map[string]bool)
	for _, d := range defs {
		if i := strings.Index(d.Name, "__"); i > 0 {
			out[d.Name[:i]] = true
		}
	}
	return out
}

// Eligible returns seeds whose required servers are all present in live.
func Eligible(seeds []Seed, live map[string]bool) []Seed {
	if len(seeds) == 0 || live == nil {
		return nil
	}
	out := make([]Seed, 0, len(seeds))
	for _, s := range seeds {
		if seedOK(s, live) {
			out = append(out, s)
		}
	}
	return out
}

func seedOK(s Seed, live map[string]bool) bool {
	if len(s.Servers) == 0 {
		return false
	}
	for _, srv := range s.Servers {
		if !live[srv] {
			return false
		}
	}
	return true
}

// Pick chooses one eligible seed at random from DefaultSeeds ∩ live tool prefixes.
func Pick(defs []provider.ToolDef) (Seed, bool) {
	return PickFrom(DefaultSeeds, defs)
}

// PickFrom is like Pick but with an explicit seed pool (tests).
func PickFrom(seeds []Seed, defs []provider.ToolDef) (Seed, bool) {
	eligible := Eligible(seeds, ServerPrefixes(defs))
	if len(eligible) == 0 {
		return Seed{}, false
	}
	if len(eligible) == 1 {
		return eligible[0], true
	}
	return eligible[rand.IntN(len(eligible))], true
}

// NoMatchMessage is returned when no seed matches the live catalog.
const NoMatchMessage = "No multi-step recipes match your connected tools right now. Try /tools to see what's available."

// OffHint is appended to polished suggestions so users know how to opt out.
const OffHint = "Turn these off anytime with /examples off"

// PolishPrompt asks the model to localize a seed into a short invitation (no tools).
func PolishPrompt(s Seed) string {
	var b strings.Builder
	b.WriteString("Propose one concrete capability example for this human based on this recipe.\n")
	fmt.Fprintf(&b, "Title: %s\n", s.Title)
	b.WriteString("Steps:\n")
	for i, step := range s.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step)
	}
	b.WriteString("\nRules: 2–3 sentences. Localize with a plausible everyday scenario. ")
	b.WriteString("Invite them to try it (e.g. want me to do that?). ")
	b.WriteString("If the recipe mentions cron_schedule / a recurring reminder, pitch scheduling it (daily or weekly) — still propose only. ")
	b.WriteString("If the recipe mentions watch_add / a feed or X subscription, pitch setting up the watch — still propose only. ")
	b.WriteString("Do not call tools. Do not invent tools outside this recipe. ")
	fmt.Fprintf(&b, "End with a short note: %s", OffHint)
	return b.String()
}

// FallbackFormat renders a seed without LLM polish (completer failure path).
func FallbackFormat(s Seed) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", s.Title)
	for i, step := range s.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step)
	}
	fmt.Fprintf(&b, "\nWant me to try that?\n%s", OffHint)
	return strings.TrimSpace(b.String())
}
