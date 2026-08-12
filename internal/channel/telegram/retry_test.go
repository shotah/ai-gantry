package telegram

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

func TestRetryWait_HonorsRetryAfter(t *testing.T) {
	wait, ok := retryWait(&bot.TooManyRequestsError{RetryAfter: 3}, time.Second)
	if !ok {
		t.Fatal("expected retryable")
	}
	if wait != 3*time.Second {
		t.Fatalf("wait=%v", wait)
	}
}

func TestRetryWait_FallsBackToBackoff(t *testing.T) {
	wait, ok := retryWait(&bot.TooManyRequestsError{RetryAfter: 0}, 750*time.Millisecond)
	if !ok || wait != 750*time.Millisecond {
		t.Fatalf("ok=%v wait=%v", ok, wait)
	}
}

func TestRetryWait_Non429(t *testing.T) {
	if _, ok := retryWait(errors.New("nope"), time.Second); ok {
		t.Fatal("expected non-retryable")
	}
}

func TestDoWith429Retry_SucceedsAfterThrottle(t *testing.T) {
	prevBase, prevAttempts := retryBase, retryAttempts
	retryBase = time.Millisecond
	retryAttempts = 3
	t.Cleanup(func() {
		retryBase, retryAttempts = prevBase, prevAttempts
	})

	var n int
	err := doWith429Retry(context.Background(), func() error {
		n++
		if n < 3 {
			return &bot.TooManyRequestsError{Message: "slow down", RetryAfter: 0}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if n != 3 {
		t.Fatalf("calls=%d", n)
	}
}

func TestDoWith429Retry_GivesUp(t *testing.T) {
	prevBase, prevAttempts := retryBase, retryAttempts
	retryBase = time.Millisecond
	retryAttempts = 2
	t.Cleanup(func() {
		retryBase, retryAttempts = prevBase, prevAttempts
	})

	err := doWith429Retry(context.Background(), func() error {
		return &bot.TooManyRequestsError{Message: "slow down", RetryAfter: 0}
	})
	if !isTooManyRequests(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestIsMessageNotModified(t *testing.T) {
	err := fmt.Errorf(`telegram: stream edit: bad request, Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message`)
	if !isMessageNotModified(err) {
		t.Fatal("expected not-modified detect")
	}
	if isMessageNotModified(nil) || isMessageNotModified(errors.New("other")) {
		t.Fatal("false positive not-modified")
	}
}

func TestIsTelegramEntityError(t *testing.T) {
	err := fmt.Errorf(`telegram: send: bad request, Bad Request: can't parse entities: Unsupported start tag "table" at byte offset 0`)
	if !isTelegramEntityError(err) {
		t.Fatal("expected entity error")
	}
	if isTelegramEntityError(nil) || isTelegramEntityError(errors.New("other")) {
		t.Fatal("false positive entity error")
	}
}

func TestDoWith429Retry_Non429NoRetry(t *testing.T) {
	var n int
	want := errors.New("boom")
	err := doWith429Retry(context.Background(), func() error {
		n++
		return want
	})
	if !errors.Is(err, want) || n != 1 {
		t.Fatalf("err=%v calls=%d", err, n)
	}
}
