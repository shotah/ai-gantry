package slash

import (
	"strings"
	"testing"
)

func TestCatalog_UniqueAndHelp(t *testing.T) {
	cmds := Catalog()
	if len(cmds) < 10 {
		t.Fatalf("catalog too small: %d", len(cmds))
	}
	seen := map[string]struct{}{}
	help := HelpText()
	for _, c := range cmds {
		if c.Name == "" || strings.Contains(c.Name, "/") || strings.Contains(c.Name, " ") {
			t.Fatalf("bad name %+v", c)
		}
		if len(c.Hint) < 3 {
			t.Fatalf("hint too short %+v", c)
		}
		if _, ok := seen[c.Name]; ok {
			t.Fatalf("dup %s", c.Name)
		}
		seen[c.Name] = struct{}{}
		if !strings.Contains(help, "/"+c.Name+" —") {
			t.Fatalf("help missing /%s:\n%s", c.Name, help)
		}
	}
	if _, ok := seen["new"]; !ok {
		t.Fatalf("missing new: %v", seen)
	}
	if _, ok := seen["brief"]; !ok {
		t.Fatalf("missing brief: %v", seen)
	}
	if _, ok := seen["help"]; !ok {
		t.Fatalf("missing help: %v", seen)
	}
	ready := ReadyLine()
	if !strings.Contains(ready, "/new") || !strings.HasSuffix(ready, "/quit") {
		t.Fatalf("ready=%q", ready)
	}
}
