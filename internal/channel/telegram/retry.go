package telegram

import (
	"context"
	"errors"
	"time"

	"github.com/go-telegram/bot"
)

// Overridable for tests.
var (
	retryBase     = 500 * time.Millisecond
	retryMaxWait  = 10 * time.Second
	retryAttempts = 4 // total tries = 1 + retryAttempts
)

// doWith429Retry runs op, backing off on Telegram 429 (TooManyRequests).
// Honors retry_after when present; otherwise uses exponential backoff.
func doWith429Retry(ctx context.Context, op func() error) error {
	backoff := retryBase
	var last error
	for attempt := 0; attempt <= retryAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := op()
		if err == nil {
			return nil
		}
		last = err
		wait, ok := retryWait(err, backoff)
		if !ok || attempt == retryAttempts {
			return err
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return err
		}
		backoff *= 2
		if backoff > retryMaxWait {
			backoff = retryMaxWait
		}
	}
	return last
}

func retryWait(err error, backoff time.Duration) (time.Duration, bool) {
	var tm *bot.TooManyRequestsError
	if !errors.As(err, &tm) {
		return 0, false
	}
	wait := backoff
	if tm.RetryAfter > 0 {
		wait = time.Duration(tm.RetryAfter) * time.Second
	}
	if wait > retryMaxWait {
		wait = retryMaxWait
	}
	if wait < time.Millisecond {
		wait = time.Millisecond
	}
	return wait, true
}

func isTooManyRequests(err error) bool {
	var tm *bot.TooManyRequestsError
	return errors.As(err, &tm)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
