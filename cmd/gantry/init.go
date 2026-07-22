package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shotah/ai-gantry/examples"
)

// initCmd scaffolds persona + mcp.toml (+ .env.example) from embedded templates.
//
//	PERSONA_DIR   default deploy/persona (local) — use /persona in containers
//	MCP_MANIFEST  default deploy/mcp.toml
//
// Existing files are skipped. Fail-fast if a target directory is not writable.
func initCmd() int {
	personaDir := envOr("PERSONA_DIR", "deploy/persona")
	manifestPath := envOr("MCP_MANIFEST", "deploy/mcp.toml")

	if err := ensureWritableDir(personaDir); err != nil {
		fmt.Fprintf(os.Stderr, "init: persona dir: %v\n", err)
		return 1
	}
	manifestDir := filepath.Dir(manifestPath)
	if manifestDir != "." && manifestDir != "" {
		if err := ensureWritableDir(manifestDir); err != nil {
			fmt.Fprintf(os.Stderr, "init: mcp manifest dir: %v\n", err)
			return 1
		}
	}

	wrote := 0
	skipped := 0

	entries, err := fs.ReadDir(examples.FS, "persona")
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: read embedded persona: %v\n", err)
		return 1
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".example.md") {
			continue
		}
		destName := strings.TrimSuffix(name, ".example.md") + ".md"
		dest := filepath.Join(personaDir, destName)
		n, err := copyEmbeddedIfMissing(path.Join("persona", name), dest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "init: %v\n", err)
			return 1
		}
		if n {
			wrote++
			fmt.Fprintf(os.Stderr, "wrote %s\n", dest)
		} else {
			skipped++
			fmt.Fprintf(os.Stderr, "skip  %s (exists)\n", dest)
		}
	}

	n, err := copyEmbeddedIfMissing("mcp.toml.example", manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}
	if n {
		wrote++
		fmt.Fprintf(os.Stderr, "wrote %s\n", manifestPath)
	} else {
		skipped++
		fmt.Fprintf(os.Stderr, "skip  %s (exists)\n", manifestPath)
	}

	envDest := ".env.example"
	n, err = copyEmbeddedIfMissing("env.example", envDest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}
	if n {
		wrote++
		fmt.Fprintf(os.Stderr, "wrote %s\n", envDest)
	} else {
		skipped++
		fmt.Fprintf(os.Stderr, "skip  %s (exists)\n", envDest)
	}

	fmt.Fprintf(os.Stderr, "init done: wrote=%d skipped=%d\n", wrote, skipped)
	fmt.Fprintf(os.Stderr, "next: copy .env.example → .env, edit secrets, mount %s + %s\n", personaDir, manifestPath)
	return 0
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	probe := filepath.Join(dir, ".gantry-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("%s is not writable: %w", dir, err)
	}
	_ = os.Remove(probe)
	return nil
}

func copyEmbeddedIfMissing(src, dest string) (wrote bool, err error) {
	if _, err := os.Stat(dest); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	data, err := examples.FS.ReadFile(src)
	if err != nil {
		return false, fmt.Errorf("read embedded %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", dest, err)
	}
	return true, nil
}
