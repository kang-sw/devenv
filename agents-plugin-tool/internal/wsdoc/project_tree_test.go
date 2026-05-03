package wsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectTreeRendersCoreSections(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/ref/guide.md", "# Guide\n")
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\nfeatures:\n  - done {#260503-done}\n  - 🚧 pending [260503-feat-demo/p1]\n---\n# Demo\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-feat-demo.md", "---\ntitle: Demo ticket\nparent: 260503-epic-demo\nrelated:\n  260503-research-demo: source\n---\n# Demo ticket\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-research-demo.md", "---\ntitle: Research demo\n---\n# Research demo\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-epic-demo.md", "---\ntitle: Epic demo\n---\n# Epic demo\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}

	for _, want := range []string{
		"ai-docs/",
		"  ref/",
		"spec:",
		"  demo.md  - Demo  [2f, WIP 1 -> 260503-feat-demo/p1]",
		"tickets:",
		"  [todo] 260503-feat-demo",
		"      parent: 260503-epic-demo  # Epic demo",
		"      related: 260503-research-demo  # source · Research demo",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ProjectTree output missing %q\n%s", want, got)
		}
	}
}

func TestReadInfraRequiresBareName(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "claude-plugin/infra/example.md", "hello")

	got, err := ReadInfra(root, "example")
	if err != nil {
		t.Fatalf("ReadInfra returned error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("ReadInfra = %q", got)
	}

	if _, err := ReadInfra(root, "../example"); err == nil {
		t.Fatal("ReadInfra accepted path traversal")
	}
}

func mustWrite(t *testing.T, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
