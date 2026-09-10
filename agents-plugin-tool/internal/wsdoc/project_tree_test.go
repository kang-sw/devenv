package wsdoc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectTreeRendersCoreSections(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/ref/guide.md", "# Guide\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260503-feat-demo.md", "---\ntitle: Demo ticket\nparent: 260503-epic-demo\nrelated:\n  260503-research-demo: source\n---\n# Demo ticket\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-research-demo.md", "---\ntitle: Research demo\n---\n# Research demo\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-epic-demo.md", "---\ntitle: Epic demo\n---\n# Epic demo\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}

	for _, want := range []string{
		"ai-docs/",
		"  ref/",
		"tickets:",
		"  todo/260503-epic-demo",
		"    ready/260503-feat-demo",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ProjectTree output missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "related:") {
		t.Fatalf("ProjectTree output unexpectedly included related: edges\n%s", got)
	}
}

func TestProjectTreeRendersFullBacklogNoOrphanFold(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260503-feat-demo.md", "---\ntitle: Demo ticket\n---\n# Demo ticket\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-epic-demo.md", "---\ntitle: Epic demo\n---\n# Epic demo\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-child-demo.md", "---\ntitle: Child idea\nparent: 260503-epic-demo\nrelated:\n  260503-feat-demo: source\n---\n# Child idea\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-orphan-one.md", "---\ntitle: Orphan one\n---\n# Orphan one\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-orphan-two.md", "---\ntitle: Orphan two\nrelated:\n  260503-feat-demo: related-only\n---\n# Orphan two\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}

	for _, want := range []string{
		"  ready/260503-feat-demo",
		"  todo/260503-epic-demo",
		"    idea/260503-child-demo",
		"  idea/260503-orphan-one",
		"  idea/260503-orphan-two",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ProjectTree output missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "orphan hidden") {
		t.Fatalf("ProjectTree output unexpectedly folded orphan idea tickets\n%s", got)
	}
	if strings.Contains(got, "related:") {
		t.Fatalf("ProjectTree output unexpectedly included related: edges\n%s", got)
	}
}

func TestProjectTreeNoneOnlyWhenNothingRenders(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-child-demo.md", "---\ntitle: Child idea\nparent: 260503-epic-demo\n---\n# Child idea\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if !strings.Contains(got, "?/260503-epic-demo") {
		t.Fatalf("ProjectTree output missing placeholder root\n%s", got)
	}
	if strings.Contains(got, "(none)") {
		t.Fatalf("ProjectTree output unexpectedly included (none) alongside a rendered ticket\n%s", got)
	}
}

func TestProjectTreeNoneWhenTicketDirsEmpty(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	if err := os.MkdirAll(filepath.Join(root, "ai-docs", "tickets", "idea"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if !strings.Contains(got, "  (none)") {
		t.Fatalf("ProjectTree output missing (none) line for an empty backlog\n%s", got)
	}
}

func TestProjectTreeDeadParentAnchorsLiveDescendant(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/.done/260503-done-parent.md", "---\ntitle: Done parent\n---\n# Done parent\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260503-live-child.md", "---\ntitle: Live child\nparent: 260503-done-parent\n---\n# Live child\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if !strings.Contains(got, "  done/260503-done-parent") {
		t.Fatalf("ProjectTree output missing done anchor\n%s", got)
	}
	if !strings.Contains(got, "    ready/260503-live-child") {
		t.Fatalf("ProjectTree output missing nested live child\n%s", got)
	}
}

func TestProjectTreeDeadSubtreeWithNoLiveDescendantOmitted(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/.done/260503-done-parent.md", "---\ntitle: Done parent\n---\n# Done parent\n")
	mustWrite(t, root, "ai-docs/tickets/.done/260503-done-child.md", "---\ntitle: Done child\nparent: 260503-done-parent\n---\n# Done child\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	for _, forbidden := range []string{"260503-done-parent", "260503-done-child"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("ProjectTree output unexpectedly rendered dead-only subtree %q\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "  (none)") {
		t.Fatalf("ProjectTree output missing (none) line for a fully-pruned backlog\n%s", got)
	}
}

func TestProjectTreeMultiLevelDeadAncestorChain(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/.dropped/260503-dropped-grandparent.md", "---\ntitle: Dropped grandparent\n---\n# Dropped grandparent\n")
	mustWrite(t, root, "ai-docs/tickets/.done/260503-done-parent.md", "---\ntitle: Done parent\nparent: 260503-dropped-grandparent\n---\n# Done parent\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260503-live-child.md", "---\ntitle: Live child\nparent: 260503-done-parent\n---\n# Live child\n")
	mustWrite(t, root, "ai-docs/tickets/.done/260503-dead-sibling.md", "---\ntitle: Dead sibling\nparent: 260503-dropped-grandparent\n---\n# Dead sibling\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	for _, want := range []string{
		"  dropped/260503-dropped-grandparent",
		"    done/260503-done-parent",
		"      ready/260503-live-child",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ProjectTree output missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "260503-dead-sibling") {
		t.Fatalf("ProjectTree output unexpectedly rendered dead sibling with no live descendant\n%s", got)
	}
}

func TestProjectTreeSharedPlaceholderForMissingParent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-orphan-a.md", "---\ntitle: Orphan A\nparent: 260503-missing\n---\n# Orphan A\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-orphan-b.md", "---\ntitle: Orphan B\nparent: 260503-missing\n---\n# Orphan B\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if strings.Count(got, "?/260503-missing") != 1 {
		t.Fatalf("ProjectTree output expected exactly one shared placeholder root\n%s", got)
	}
	for _, want := range []string{"idea/260503-orphan-a", "idea/260503-orphan-b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ProjectTree output missing %q\n%s", want, got)
		}
	}
}

func TestProjectTreeParentCycleDegradesToFlatRoots(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-cycle-a.md", "---\ntitle: Cycle A\nparent: 260503-cycle-b\n---\n# Cycle A\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260503-cycle-b.md", "---\ntitle: Cycle B\nparent: 260503-cycle-a\n---\n# Cycle B\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if !strings.Contains(got, "  idea/260503-cycle-a") || !strings.Contains(got, "  idea/260503-cycle-b") {
		t.Fatalf("ProjectTree output expected both cycle members as flat roots\n%s", got)
	}
}

func TestProjectTreeSkipsGitIgnoredEntries(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	mustWrite(t, root, ".gitignore", "ai-docs/presentation/node_modules/\nai-docs/presentation/generated.log\n")
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/presentation/deck.md", "# Deck\n")
	mustWrite(t, root, "ai-docs/presentation/generated.log", "generated\n")
	mustWrite(t, root, "ai-docs/presentation/node_modules/pkg/index.js", "module.exports = {}\n")

	got, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}

	for _, want := range []string{
		"ai-docs/",
		"  presentation/",
		"    deck.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ProjectTree output missing %q\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"node_modules",
		"generated.log",
		"index.js",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("ProjectTree output included ignored %q\n%s", forbidden, got)
		}
	}
}

func TestReadInfraRequiresBareName(t *testing.T) {
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	got, err := ReadInfra("impl-playbook")
	if err != nil {
		t.Fatalf("ReadInfra returned error: %v", err)
	}
	if !strings.Contains(got, "Implementation Playbook") {
		t.Fatalf("ReadInfra = %q", got)
	}

	if _, err := ReadInfra("../impl-playbook"); err == nil {
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
	for _, name := range []string{"ticket", "tickets", "ticket-convention"} {
		got, err := ReadConvention(name)
		if err != nil {
			t.Fatalf("ReadConvention(%q) returned error: %v", name, err)
		}
		if !strings.Contains(got, "# Ticket Conventions") {
			t.Fatalf("ReadConvention(%q) returned unexpected text: %q", name, got[:min(len(got), 80)])
		}
	}
	// The spec and mental-model convention documents retired with their layer;
	// asking for either must miss rather than resolve through a surviving alias.
	for _, name := range []string{"spec", "spec-conventions", "mental-model", "mental-model-conventions"} {
		if _, err := ReadConvention(name); err == nil {
			t.Fatalf("ReadConvention(%q) still resolves a retired convention", name)
		}
	}
	if _, err := ReadConvention("../ticket-conventions"); err == nil {
		t.Fatal("ReadConvention accepted path traversal")
	}
	_, err = ReadConvention("unknown")
	if err == nil || !strings.Contains(err.Error(), "ticket-conventions") {
		t.Fatalf("ReadConvention missing-name error = %v", err)
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

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}
