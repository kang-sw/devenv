package wsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestReadConventionUsesBundledDocs(t *testing.T) {
	got, err := ReadConvention("ticket-conventions")
	if err != nil {
		t.Fatalf("ReadConvention returned error: %v", err)
	}
	if !strings.Contains(got, "# Ticket Conventions") {
		t.Fatalf("ReadConvention returned unexpected text: %q", got[:min(len(got), 80)])
	}
	if _, err := ReadConvention("../ticket-conventions"); err == nil {
		t.Fatal("ReadConvention accepted path traversal")
	}
}

func TestGenerateSpecStemAvoidsCollisions(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "## Demo {#260503-demo}\n")

	got, err := GenerateSpecStem(root, "Demo", time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GenerateSpecStem returned error: %v", err)
	}
	if got != "260503-demo-2" {
		t.Fatalf("GenerateSpecStem = %q", got)
	}
}

func TestVerifySpecIndexReportsDuplicates(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/a.md", "## A {#260503-dup}\n")
	mustWrite(t, root, "ai-docs/spec/b.md", "## B {#260503-dup}\n")

	got, err := VerifySpecIndex(root)
	if err != nil {
		t.Fatalf("VerifySpecIndex returned error: %v", err)
	}
	if !strings.Contains(got, "duplicate anchors") || !strings.Contains(got, "260503-dup") {
		t.Fatalf("VerifySpecIndex output missing duplicate report:\n%s", got)
	}
}

func TestMentalModelsListRendersFrontmatter(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/mental-model/demo.md", "---\ndomain: demo\ndescription: \"Demo domain\"\nsources:\n  - demo/\n---\n# Demo\n")

	got, err := MentalModelsList(root)
	if err != nil {
		t.Fatalf("MentalModelsList returned error: %v", err)
	}
	for _, want := range []string{
		"mental-models:",
		"ai-docs/mental-model/demo.md  - demo  # Demo domain",
		"sources: demo/",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("MentalModelsList output missing %q\n%s", want, got)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
