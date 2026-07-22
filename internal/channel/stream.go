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

// WithReplyWriter attaches a ReplyWriter for streaming replies.
func WithReplyWriter(ctx context.Context, w ReplyWriter) context.Context {
	return context.WithValue(ctx, replyWriterKey{}, w)
}

// ReplyWriterFrom returns the streaming writer, if any.
func ReplyWriterFrom(ctx context.Context) (ReplyWriter, bool) {
	w, ok := ctx.Value(replyWriterKey{}).(ReplyWriter)
	return w, ok
}
