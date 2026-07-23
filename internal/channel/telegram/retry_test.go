package telegram

import (
	"context"
	"errors"
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
