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
	write("SELF.md", "self-body")
	write("SOUL.md", "soul-body")
	write("AAA.md", "extra-a")
	write("notes.txt", "ignored")

	got, err := persona.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	soul := strings.Index(got, "soul-body")
	self := strings.Index(got, "self-body")
	user := strings.Index(got, "user-body")
	aaa := strings.Index(got, "extra-a")
	zzz := strings.Index(got, "extra-z")
	if soul < 0 || self < 0 || user < 0 || aaa < 0 || zzz < 0 {
		t.Fatalf("missing parts in %q", got)
	}
	if soul >= self || self >= user || user >= aaa || aaa >= zzz {
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

func TestLoad_StampsSelfAndRules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SELF.md"), []byte("# SELF.md — Who You Are Becoming\n\n> stale\n\n- dry humor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := "# RULES.md\n\n## Identity lock\n\nhold id\n\n## Self-notes (`self_note` → SELF.md)\n\n- stale rule\n\n## Memory hygiene\n\nhold mem\n"
	if err := os.WriteFile(filepath.Join(dir, "RULES.md"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := persona.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "stale") || strings.Contains(got, "stale rule") {
		t.Fatalf("stale kernel text kept: %q", got)
	}
	if !strings.Contains(got, "Standing aims") || !strings.Contains(got, "- dry humor") {
		t.Fatalf("SELF stamp missing: %q", got)
	}
	if !strings.Contains(got, "A standing aim outlives one task") || !strings.Contains(got, "hold mem") {
		t.Fatalf("RULES stamp missing: %q", got)
	}
}

func TestSyncKernel_RewritesRulesSection(t *testing.T) {
	dir := t.TempDir()
	old := "# RULES.md\n\n## Self-notes (`self_note` → SELF.md)\n\n- stale\n\n## Memory hygiene\n\nok\n"
	if err := os.WriteFile(filepath.Join(dir, "RULES.md"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := persona.SyncKernel(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "RULES.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "- stale") {
		t.Fatalf("stale section kept: %q", got)
	}
	if !strings.Contains(got, "A standing aim outlives one task") || !strings.Contains(got, "## Memory hygiene") {
		t.Fatalf("sync missing kernel or rest of file: %q", got)
	}
}

func TestTimezone_FromUserMarkdown(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"- **Timezone:** America/Los_Angeles\n- **Location:** Seattle",
		"Timezone: America/Los_Angeles",
		"**Timezone:** America/Los_Angeles",
	} {
		if got := persona.Timezone(in); got != "America/Los_Angeles" {
			t.Fatalf("input %q: got %q", in, got)
		}
	}
	if persona.Timezone("no tz here") != "" {
		t.Fatal("expected empty")
	}
	if persona.Timezone("- **Timezone:** Not/AZone") != "" {
		t.Fatal("invalid IANA must be empty")
	}
}

func TestResolveTimezone_PrefersUserMarkdown(t *testing.T) {
	t.Parallel()
	name, loc, source := persona.ResolveTimezone("- **Timezone:** America/Los_Angeles", "UTC")
	if name != "America/Los_Angeles" || source != "USER.md" || loc == nil {
		t.Fatalf("name=%q source=%q loc=%v", name, source, loc)
	}
	name, _, source = persona.ResolveTimezone("", "America/New_York")
	if name != "America/New_York" || source != "CRON_TZ" {
		t.Fatalf("fallback name=%q source=%q", name, source)
	}
	name, loc, source = persona.ResolveTimezone("", "")
	if name != "America/Los_Angeles" || loc == nil {
		t.Fatalf("empty fallback name=%q source=%q", name, source)
	}
}
