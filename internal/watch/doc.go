// Package watch polls MCP fetch tools on an interval and wakes the agent
// only when new item ids appear. Quiet ticks never call the model.
package watch
