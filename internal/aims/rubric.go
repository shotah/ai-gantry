package aims

// RubricText is the score scale and the anchor rows. The planner prompt,
// /aims rubric, and aim_log description share it.
const RubricText = `Score an event toward each aim it touches, -3..+3:
+3 did the planned thing and then some, or a milestone (a send, a PR)
+2 did the planned thing
+1 partial, a small win, or a clean day on a quit aim
 0 rest, a weigh-in (a new low is still 0 — the number goes on value), or an agent note
-1 small slip (skipped once, a dessert)
-2 went against the aim (a whole pizza, a night out, skipped the week's key session)
-3 blew it, or an injury that costs the plan

Anchors — score by the nearest row. Grades stay in the human's notation (V, YDS 5.11c, 8a.nu).
training: +3 PR or extra · +2 the planned session · +1 a short session · 0 planned rest · -1 skipped once · -2 skipped the week's key session · -3 injured
climbing: +3 sent the project or a new max · +2 the planned session · +1 got on the wall · 0 rest · -1 skipped · -2 skipped two in a row · -3 injury
weight: +2 the day's food was on plan · +1 mostly on plan · 0 a weigh-in (number on value, not a bonus) · -1 a dessert · -2 a night out · -3 off plan for a week
quit: +1 a clean day (the default) · -1 one drink or one cigarette · -2 a night of it · -3 back on it for a week
habit: +2 the daily block · +1 a few minutes · 0 planned day off · -1 missed once · -2 missed three days · -3 dropped it
On a quit aim a clean day is +1. Unknown domain: nearest column.`
