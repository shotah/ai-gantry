package cron

import "context"

type deliveryKey struct{}

// Delivery binds a scheduled job to a chat/session (from the scheduling turn).
type Delivery struct {
	SessionID string
	UserID    string
	ChatID    string
	ThreadID  int
}

// WithDelivery attaches outbound routing to ctx for cron_* tool calls.
func WithDelivery(ctx context.Context, d Delivery) context.Context {
	return context.WithValue(ctx, deliveryKey{}, d)
}

// DeliveryFrom returns scheduling delivery, if present.
func DeliveryFrom(ctx context.Context) (Delivery, bool) {
	d, ok := ctx.Value(deliveryKey{}).(Delivery)
	return d, ok
}
