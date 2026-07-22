package persona_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/persona"
)

func TestLoad_FixedOrderAndExtras(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ZZZ.md", "extra-z")
	write("USER.md", "user-body")
	write("SOUL.md", "soul-body")
	write("AAA.md", "extra-a")
	write("notes.txt", "ignored")

	got, err := persona.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	soul := strings.Index(got, "soul-body")
	user := strings.Index(got, "user-body")
	aaa := strings.Index(got, "extra-a")
	zzz := strings.Index(got, "extra-z")
	if soul < 0 || user < 0 || aaa < 0 || zzz < 0 {
		t.Fatalf("missing parts in %q", got)
	}
	if soul >= user || user >= aaa || aaa >= zzz {
		t.Fatalf("order wrong in %q", got)
	}
	if strings.Contains(got, "ignored") {
		t.Fatalf("non-md file included: %q", got)
	}
}

func TestLoad_MissingDir(t *testing.T) {
	got, err := persona.Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestLoad_MissingPreferredTolerant(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "USER.md"), []byte("only-user"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := persona.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != "only-user" {
		t.Fatalf("got %q, want only-user", got)
	}
}
