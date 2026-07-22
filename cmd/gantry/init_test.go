package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCmd_ScaffoldsAndSkips(t *testing.T) {
	root := t.TempDir()
	persona := filepath.Join(root, "persona")
	manifest := filepath.Join(root, "mcp.toml")
	t.Setenv("PERSONA_DIR", persona)
	t.Setenv("MCP_MANIFEST", manifest)

	// Run from temp so .env.example lands here.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if code := initCmd(); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	soul := filepath.Join(persona, "SOUL.md")
	if _, err := os.Stat(soul); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".env.example")); err != nil {
		t.Fatal(err)
	}

	// Second run skips existing.
	if code := initCmd(); code != 0 {
		t.Fatalf("re-init exit %d", code)
	}
}

func TestInitCmd_UnwritablePersona(t *testing.T) {
	root := t.TempDir()
	fileAsDir := filepath.Join(root, "notadir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PERSONA_DIR", filepath.Join(fileAsDir, "persona"))
	t.Setenv("MCP_MANIFEST", filepath.Join(root, "mcp.toml"))
	if code := initCmd(); code == 0 {
		t.Fatal("expected failure for unwritable persona dir")
	}
}
