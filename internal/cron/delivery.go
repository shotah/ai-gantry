package cron

import "context"

type deliveryKey struct{}

// Delivery identifies the conversation a job belongs to. Mouths route Push
// through the CHANNEL allowlist; user/chat/thread are not stored or used.
type Delivery struct {
	SessionID string
	UserID    string
	ChatID    string
	ThreadID  int
}

// WithDelivery attaches the conversation id to ctx for cron_* / watch_* calls.
func WithDelivery(ctx context.Context, d Delivery) context.Context {
	return context.WithValue(ctx, deliveryKey{}, d)
}

// DeliveryFrom returns scheduling delivery, if present.
func DeliveryFrom(ctx context.Context) (Delivery, bool) {
	d, ok := ctx.Value(deliveryKey{}).(Delivery)
	return d, ok
}
