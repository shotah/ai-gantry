package telegram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
)

type timeoutNetErr struct{ msg string }

func (e timeoutNetErr) Error() string { return e.msg }

func (e timeoutNetErr) Timeout() bool { return true }

func (e timeoutNetErr) Temporary() bool { return true }

var _ net.Error = timeoutNetErr{}

func TestIsTransientPollErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain", err: errors.New("conflict"), want: false},
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{
			name: "wrapped deadline",
			err:  fmt.Errorf("error get updates, %w", fmt.Errorf("error do request for method getUpdates, %w", context.DeadlineExceeded)),
			want: true,
		},
		{name: "net timeout", err: timeoutNetErr{msg: "i/o timeout"}, want: true},
		{
			name: "client timeout string",
			err:  errors.New(`error get updates, error do request for method getUpdates, Post "https://api.telegram.org/bot***/getUpdates": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`),
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientPollErr(tc.err); got != tc.want {
				t.Fatalf("isTransientPollErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestNew_RequiresAllowlist(t *testing.T) {
	_, err := New(Config{Token: "tok"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsAllowedAndSessionKey(t *testing.T) {
	ch, err := New(Config{Token: "tok", AllowedUsers: []int64{42, 99}})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.isAllowed(42) || ch.isAllowed(7) {
		t.Fatal("allowlist mismatch")
	}
	if got := sessionKey(1, 2, 0); got != "telegram:1:2" {
		t.Fatalf("%q", got)
	}
	if got := sessionKey(1, 2, 9); got != "telegram:1:2:9" {
		t.Fatalf("%q", got)
	}
}
