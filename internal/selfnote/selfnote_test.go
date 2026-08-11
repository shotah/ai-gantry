package selfnote_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/selfnote"
)

func TestStore_AppendCreatesHeaderAndNotifies(t *testing.T) {
	dir := t.TempDir()
	s, err := selfnote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	s.OnChange = func() { changed++ }

	if err := s.Append("opens 20-questions with a dramatic accusation"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "# SELF.md") {
		t.Fatalf("missing header: %q", got)
	}
	if !strings.Contains(got, "- opens 20-questions with a dramatic accusation") {
		t.Fatalf("missing note: %q", got)
	}
	if changed != 1 {
		t.Fatalf("OnChange fired %d times, want 1", changed)
	}
	if err := s.Append("  "); err == nil {
		t.Fatal("empty note accepted")
	}
}

func TestStore_AppendRefusesWhenFull(t *testing.T) {
	dir := t.TempDir()
	s, err := selfnote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", selfnote.MaxChars-10)
	if err := os.WriteFile(filepath.Join(dir, selfnote.FileName), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	err = s.Append("one more line that will not fit")
	if err == nil || !strings.Contains(err.Error(), "full") {
		t.Fatalf("err = %v, want full-file refusal", err)
	}
}

func TestStore_WriteReplacesClipsAndRefusesEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := selfnote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(""); err == nil {
		t.Fatal("empty write accepted")
	}
	if err := s.Write("- no heading line"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Read()
	if !strings.HasPrefix(got, "# SELF.md") {
		t.Fatalf("header not prepended: %q", got)
	}
	if err := s.Write("# SELF.md — Who You Are Becoming\n" + strings.Repeat("y", 2*selfnote.MaxChars)); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Read()
	if len(got) > selfnote.MaxChars {
		t.Fatalf("content not clipped: %d chars", len(got))
	}
}

func TestTools_Call(t *testing.T) {
	dir := t.TempDir()
	s, err := selfnote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	tools := selfnote.Tools{Store: s}
	out, err := tools.Call(context.Background(), selfnote.ToolNote, json.RawMessage(`{"note":"calls the human Boss"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "noted") {
		t.Fatalf("out = %q", out)
	}
	got, _ := s.Read()
	if !strings.Contains(got, "- calls the human Boss") {
		t.Fatalf("note not written: %q", got)
	}
	if _, err := tools.Call(context.Background(), "self_bogus", nil); err == nil {
		t.Fatal("unknown tool accepted")
	}
}

type stubRunner struct{ defs []provider.ToolDef }

func (r stubRunner) Tools() []provider.ToolDef { return r.defs }

func (r stubRunner) ToolCount() int { return len(r.defs) }

func (r stubRunner) Call(context.Context, string, json.RawMessage) (string, error) {
	return "other-ok", nil
}

func TestComposite_MergesAndRoutes(t *testing.T) {
	dir := t.TempDir()
	s, err := selfnote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := selfnote.Composite{
		Self:  selfnote.Tools{Store: s},
		Other: stubRunner{defs: []provider.ToolDef{{Name: "demo__echo"}}},
	}
	defs := c.Tools()
	if len(defs) != 2 || defs[0].Name != selfnote.ToolNote || defs[1].Name != "demo__echo" {
		t.Fatalf("defs = %+v", defs)
	}
	if c.ToolCount() != 2 {
		t.Fatalf("ToolCount = %d", c.ToolCount())
	}
	if out, err := c.Call(context.Background(), "demo__echo", nil); err != nil || out != "other-ok" {
		t.Fatalf("route to other: %q %v", out, err)
	}
	if _, err := c.Call(context.Background(), selfnote.ToolNote, json.RawMessage(`{"note":"n"}`)); err != nil {
		t.Fatalf("route to self: %v", err)
	}
}
