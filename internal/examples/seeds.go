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
		ID:      "first-aim",
		Title:   "First aim: pick a months-scale north-star, store it, and put a wake on the calendar",
		Servers: nil, // harness builtins — always eligible
		Steps: []string{
			"Ask one months-scale question (what should we be aiming at for the next few months?) — do not invent an aim",
			"After they answer: self_note the north-star sentence and memory_store insight subject aim/<area>",
			"Offer to cron_schedule a wake that would move that aim (daily/weekly; live-data jobs name the tools)",
		},
	},
	{
		ID:      "cron-reminder",
		Title:   "Cron: a reminder that survives this chat — later today or every weekday",
		Servers: nil,
		Steps: []string{
			"Pick something they actually forget (timecard, meds, a weekly review)",
			"cron_schedule it (once or daily) with a prompt that says what to do when it fires",
			"Work-only jobs can reply [silent]; otherwise they get the reminder in this chat",
		},
	},
	{
		ID:      "memory-skill",
		Title:   "Memory: save a fiddly recipe as skill/<area> so you stop re-teaching it",
		Servers: nil,
		Steps: []string{
			"Name one annoying workflow (how they like mail labeled, how they log training)",
			"memory_store kind=insight or fact, subject skill/<area>, with exact names and one pitfall",
			"Next time that area comes up, memory_recall skill/<area> before guessing",
		},
	},
	{
		ID:      "mcp-enable",
		Title:   "mcp_enable: turn on a tool prefix for this chat instead of carrying 150 schemas every turn",
		Servers: nil,
		Steps: []string{
			"Look at [mcp prefixes] / /tools for something that's mounted but not in this turn's list",
			"mcp_enable that prefix (short ~27h or brief ~6h)",
			"Then actually use a tool from it in the next step",
		},
	},
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
	{
		ID:      "maps-near-me",
		Title:   "Near me: Telegram pin + place search",
		Servers: []string{"maps"},
		Steps: []string{
			"If [last pin] is missing or hours old, ask them to send a Telegram location pin (a bare pin is silent — it only updates the cursor)",
			"Call maps__place_search from that pin — do not invent a city",
			"Share the Maps URL and a short pick; don't invent hours or ratings",
		},
	},
	{
		ID:      "maps-route-eta",
		Title:   "Directions: last pin to a place, with a leave-by time",
		Servers: []string{"maps"},
		Steps: []string{
			"Use [last pin] as origin, or ask for a Telegram pin if it's stale",
			"Resolve the destination (maps__place_resolve or maps__link_resolve for a share link)",
			"Call maps__route_eta (bike → mode=bicycling) and include the Maps URL",
		},
	},
	{
		ID:      "math-eval",
		Title:   "Numbers: let the math tool do the arithmetic (no mental math)",
		Servers: []string{"math"},
		Steps: []string{
			"State the real numbers (split a bill, a pace, a percent)",
			"Call math__expression_evaluate — don't guess the result",
			"Give the answer in one sentence",
		},
	},
	{
		ID:      "search-decide",
		Title:   "Web search: look up a real decision, then cite what came back",
		Servers: []string{"google-search"},
		Steps: []string{
			"google-search__web_search the thing they need to decide",
			"Summarize what the tool returned — don't invent extra sources",
			"Offer one next step (calendar block, a task, or just the answer)",
		},
	},
	{
		ID:      "youtube-find",
		Title:   "YouTube: find a video worth watching and send the link",
		Servers: []string{"youtube"},
		Steps: []string{
			"youtube videos_search for what they asked",
			"Pick one and share the URL — don't invent views or duration",
			"If Cast is also connected, mention they can say 'put it on the TV'",
		},
	},
	{
		ID:      "commute-to-event",
		Title:   "Leave-by: next calendar event plus a route from the last pin",
		Servers: []string{"google", "maps"},
		Steps: []string{
			"List today's Google Calendar and pick the next place they have to be",
			"Use [last pin] as origin (ask for a Telegram pin if it's stale)",
			"maps__route_eta to that place and tell them when to leave",
		},
	},
	{
		ID:      "garmin-deadman",
		Title:   "Quiet health check: daily Garmin pull that stays [silent] when you're fine",
		Servers: []string{"garmin"},
		Steps: []string{
			"Pull last night's garmin__sleep_get / recovery",
			"If all-clear, reply [silent] — no pep talk",
			"Offer to cron_schedule this at a morning hour they choose (prompt must say [silent] unless something is off)",
		},
	},
	{
		ID:      "ghealth-deadman",
		Title:   "Quiet health check: daily Google Health pull that stays [silent] when you're fine",
		Servers: []string{"ghealth"},
		Steps: []string{
			"Pull last night's ghealth__sleep_get / recovery",
			"If all-clear, reply [silent] — no pep talk",
			"Offer to cron_schedule this at a morning hour they choose (prompt must say [silent] unless something is off)",
		},
	},
	{
		ID:      "google-week-board",
		Title:   "This week: calendar + tasks, what's slipping",
		Servers: []string{"google"},
		Steps: []string{
			"List the next 7 days of Google Calendar",
			"List open Google Tasks",
			"Two sentences: what's packed, what has no time block",
		},
	},
	{
		ID:      "strava-last",
		Title:   "Last session: recap the most recent Strava activity",
		Servers: []string{"strava"},
		Steps: []string{
			"List recent Strava activities and pick the latest",
			"Summarize distance/time/effort in plain language — only numbers the tool returned",
		},
	},
	{
		ID:      "search-place-maps",
		Title:   "Find a place on the web, then a route from the last pin",
		Servers: []string{"google-search", "maps"},
		Steps: []string{
			"Search for the place or venue they named",
			"If [last pin] is stale, ask for a Telegram location pin",
			"maps__route_eta and share the Maps URL",
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
// Seeds with no Servers list are harness builtins (memory / cron / self_note) and
// always match — including a zero-MCP catalog.
func Eligible(seeds []Seed, live map[string]bool) []Seed {
	if len(seeds) == 0 {
		return nil
	}
	if live == nil {
		live = map[string]bool{}
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
		return true
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
	b.WriteString("If the recipe mentions a Telegram pin or [last pin], mention sending a location pin from the phone. ")
	b.WriteString("If the recipe mentions self_note / a north-star / aim/, pitch setting a first months-scale aim — still propose only. ")
	b.WriteString("If the recipe mentions mcp_enable, mention turning a prefix on for this chat. ")
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
