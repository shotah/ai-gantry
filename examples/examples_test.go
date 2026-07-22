package examples_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/examples"
	"github.com/shotah/ai-gantry/internal/persona"
)

func TestPersonaExamplesMatchPreferredOrder(t *testing.T) {
	entries, err := fs.ReadDir(examples.FS, "persona")
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]struct{}{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".example.md") {
			t.Fatalf("unexpected persona file %q (want *.example.md)", name)
		}
		base := strings.TrimSuffix(name, ".example.md") + ".md"
		have[base] = struct{}{}
	}
	for _, want := range persona.PreferredOrder {
		if _, ok := have[want]; !ok {
			t.Errorf("missing examples/persona/%s.example.md for PreferredOrder entry %s",
				strings.TrimSuffix(want, ".md"), want)
		}
		delete(have, want)
	}
	for orphan := range have {
		t.Errorf("orphan example not in PreferredOrder: %s", orphan)
	}
}

func TestEmbeddedManifestAndEnv(t *testing.T) {
	for _, path := range []string{"mcp.toml.example", "env.example"} {
		b, err := examples.FS.ReadFile(path)
		if err != nil || len(b) == 0 {
			t.Fatalf("%s: %v len=%d", path, err, len(b))
		}
	}
}
