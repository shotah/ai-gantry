package channel

import "context"

type replyWriterKey struct{}

// ReplyWriter updates a progressive outbound reply (Telegram edit / stdio).
type ReplyWriter interface {
	// Update replaces the visible reply with fullText so far.
	Update(ctx context.Context, fullText string) error
	// Started reports whether any Update was applied.
	Started() bool
	// Finish ensures the final text is shown (no-op if never started).
	Finish(ctx context.Context, final string) error
}

// ThinkingWriter is an optional ReplyWriter that can show model chain-of-thought
// separately from the answer (e.g. Telegram expandable italics).
type ThinkingWriter interface {
	ReplyWriter
	// UpdateThinking replaces the visible reply using accumulated thinking + content.
	UpdateThinking(ctx context.Context, thinking, content string) error
}

// ProgressWriter is an optional ReplyWriter that can append a trace line while
// tools run (e.g. "→ garmin__activities_list"). Long local-model tool chains
// otherwise look frozen, especially with chain-of-thought disabled.
type ProgressWriter interface {
	ReplyWriter
	// UpdateProgress appends one status line to the visible trace.
	UpdateProgress(ctx context.Context, note string) error
}

// StatusWriter is an optional ReplyWriter that can show a transient status line
// (e.g. "spinning up") while the model is still silent. Unlike a progress trace
// the line is disposable: real output replaces it, so it never survives into the
// finished reply.
type StatusWriter interface {
	ReplyWriter
	// UpdateStatus sets the transient status line; an empty note clears it.
	UpdateStatus(ctx context.Context, note string) error
}

// WithReplyWriter attaches a ReplyWriter for streaming replies.
func WithReplyWriter(ctx context.Context, w ReplyWriter) context.Context {
	return context.WithValue(ctx, replyWriterKey{}, w)
}

// ReplyWriterFrom returns the streaming writer, if any.
func ReplyWriterFrom(ctx context.Context) (ReplyWriter, bool) {
	w, ok := ctx.Value(replyWriterKey{}).(ReplyWriter)
	return w, ok
}

// ProgressWriterFrom returns the streaming writer when it supports trace lines.
func ProgressWriterFrom(ctx context.Context) (ProgressWriter, bool) {
	w, ok := ctx.Value(replyWriterKey{}).(ProgressWriter)
	return w, ok
}

// StatusWriterFrom returns the streaming writer when it supports a status line.
func StatusWriterFrom(ctx context.Context) (StatusWriter, bool) {
	w, ok := ctx.Value(replyWriterKey{}).(StatusWriter)
	return w, ok
}
