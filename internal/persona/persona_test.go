package persona_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/persona"
)

func TestLoad_PersonaThenSelfIgnoresExtras(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ZZZ.md", "extra-z")
	write("PERSONA.md", "persona-body")
	write("SELF.md", "self-body")
	write("SOUL.md", "soul-leftover")
	write("AAA.md", "extra-a")
	write("notes.txt", "ignored")

	got, err := persona.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	personaIdx := strings.Index(got, "persona-body")
	selfIdx := strings.Index(got, "self-body")
	if personaIdx < 0 || selfIdx < 0 {
		t.Fatalf("missing parts in %q", got)
	}
	if personaIdx >= selfIdx {
		t.Fatalf("order wrong in %q", got)
	}
	for _, bad := range []string{"extra-z", "soul-leftover", "extra-a", "ignored"} {
		if strings.Contains(got, bad) {
			t.Fatalf("unexpected %q in %q", bad, got)
		}
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
	if err := os.WriteFile(filepath.Join(dir, "PERSONA.md"), []byte("only-persona"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := persona.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(got, "only-persona") {
		t.Fatalf("got %q, want to contain only-persona", got)
	}
}

func TestLoad_StampsSelfAndPersona(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SELF.md"), []byte("# SELF.md — Who You Are Becoming\n\n> stale\n\n- dry humor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "# PERSONA.md\n\n## Identity lock\n\nhold id\n\n## Self-notes (`self_note` → SELF.md)\n\n- stale rule\n\n## Memory hygiene\n\nhold mem\n"
	if err := os.WriteFile(filepath.Join(dir, "PERSONA.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := persona.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "stale") || strings.Contains(got, "stale rule") {
		t.Fatalf("stale kernel text kept: %q", got)
	}
	if !strings.Contains(got, "north-star aims") || !strings.Contains(got, "- dry humor") {
		t.Fatalf("SELF stamp missing: %q", got)
	}
	if !strings.Contains(got, "A north-star is one sentence") || !strings.Contains(got, "hold mem") ||
		!strings.Contains(got, "A vibe word is not a joke") {
		t.Fatalf("PERSONA stamp missing: %q", got)
	}
	if !strings.Contains(got, "## Location pins") || !strings.Contains(got, "[last pin]") {
		t.Fatalf("location stamp missing: %q", got)
	}
}

func TestSyncKernel_RewritesPersonaSection(t *testing.T) {
	dir := t.TempDir()
	old := "# PERSONA.md\n\n## Self-notes (`self_note` → SELF.md)\n\n- stale\n\n## Memory hygiene\n\nok\n"
	if err := os.WriteFile(filepath.Join(dir, "PERSONA.md"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := persona.SyncKernel(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed=%v, want none", removed)
	}
	b, err := os.ReadFile(filepath.Join(dir, "PERSONA.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "- stale") {
		t.Fatalf("stale section kept: %q", got)
	}
	if !strings.Contains(got, "A north-star is one sentence") || !strings.Contains(got, "## Memory hygiene") {
		t.Fatalf("sync missing kernel or rest of file: %q", got)
	}
	if !strings.Contains(got, "## Location pins") {
		t.Fatalf("sync missing location section: %q", got)
	}
}

func TestSyncKernel_MigratesAndRemovesLegacy(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("SOUL.md", "soul-body")
	write("RULES.md", "## Memory hygiene\n\nrules-body")
	write("USER.md", "user-body")
	write("TOOLS.md", "tools-body")
	write("SELF.md", "self-body")
	write("keep.md", "should-stay")

	removed, err := persona.SyncKernel(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != len(persona.LegacyFiles) {
		t.Fatalf("removed=%v", removed)
	}
	for _, name := range persona.LegacyFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still present: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.md")); err != nil {
		t.Fatalf("unrelated file deleted: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "PERSONA.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"soul-body", "rules-body", "user-body", "tools-body"} {
		if !strings.Contains(got, want) {
			t.Fatalf("migrated PERSONA.md missing %q: %q", want, got)
		}
	}
	if !strings.Contains(got, "A north-star is one sentence") {
		t.Fatalf("stamp missing after migrate: %q", got)
	}

	loaded, err := persona.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded, "soul-body") || !strings.Contains(loaded, "self-body") {
		t.Fatalf("Load missing migrated content: %q", loaded)
	}
	if strings.Contains(loaded, "should-stay") {
		t.Fatalf("Load included extra file: %q", loaded)
	}
}

func TestSyncKernel_KeepsExistingPersonaWhenRemovingLegacy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PERSONA.md"), []byte("keep-me\n\n## Memory hygiene\n\nok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("old-soul"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := persona.SyncKernel(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "SOUL.md" {
		t.Fatalf("removed=%v", removed)
	}
	b, err := os.ReadFile(filepath.Join(dir, "PERSONA.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "keep-me") {
		t.Fatalf("existing PERSONA.md overwritten: %q", b)
	}
	if strings.Contains(string(b), "old-soul") {
		t.Fatalf("legacy content merged into existing PERSONA.md: %q", b)
	}
}

func TestTimezone_FromPersonaMarkdown(t *testing.T) {
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

func TestResolveTimezone_PrefersPersonaMarkdown(t *testing.T) {
	t.Parallel()
	name, loc, source := persona.ResolveTimezone("- **Timezone:** America/Los_Angeles", "UTC")
	if name != "America/Los_Angeles" || source != "PERSONA.md" || loc == nil {
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
