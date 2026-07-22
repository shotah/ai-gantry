package telegram

import (
	"testing"
)

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
