package cron

// KindExamples seeds capability-example pings. The daily planner is a separate
// once-a-day job (KindDailyPlanner). Scheduling the qty window is Spread, not
// an examples type.
const KindExamples = "examples"

// KindExamplesPing is one propose-only example ping created by the planner.
const KindExamplesPing = "examples_ping"
