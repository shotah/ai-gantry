package telegram

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-telegram/bot"
)

// Overridable for tests.
var (
	retryBase          = 500 * time.Millisecond
	retryMaxWait       = 10 * time.Second
	finishRetryMaxWait = 3 * time.Minute // Finish may wait out flood-control benches
	retryAttempts      = 4               // total tries = 1 + retryAttempts
)

// doWith429Retry runs op, backing off on Telegram 429 (TooManyRequests).
// Honors retry_after when present; otherwise uses exponential backoff.
func doWith429Retry(ctx context.Context, op func() error) error {
	return doWith429RetryMax(ctx, op, retryMaxWait)
}

func doWith429RetryMax(ctx context.Context, op func() error, maxWait time.Duration) error {
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
		wait, ok := retryAfterDuration(err)
		if !ok {
			return err
		}
		if wait < backoff {
			wait = backoff
		}
		if maxWait > 0 && wait > maxWait {
			wait = maxWait
		}
		if attempt == retryAttempts {
			return err
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return err
		}
		backoff *= 2
		if maxWait > 0 && backoff > maxWait {
			backoff = maxWait
		}
	}
	return last
}

func retryWait(err error, backoff time.Duration) (time.Duration, bool) {
	wait, ok := retryAfterDuration(err)
	if !ok {
		return 0, false
	}
	if wait < backoff {
		wait = backoff
	}
	if wait > retryMaxWait {
		wait = retryMaxWait
	}
	return wait, true
}

// retryAfterDuration returns Telegram's retry_after with no artificial cap.
func retryAfterDuration(err error) (time.Duration, bool) {
	var tm *bot.TooManyRequestsError
	if !errors.As(err, &tm) {
		return 0, false
	}
	if tm.RetryAfter > 0 {
		return time.Duration(tm.RetryAfter) * time.Second, true
	}
	return retryBase, true
}

func isTooManyRequests(err error) bool {
	var tm *bot.TooManyRequestsError
	return errors.As(err, &tm)
}

// isMessageNotModified is Telegram's "edit noop" — content already matches.
// Treat as success so Finish does not fall back to a duplicate SendMessage.
func isMessageNotModified(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "message is not modified")
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
