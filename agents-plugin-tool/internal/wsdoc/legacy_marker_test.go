package wsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The "no match" rows are the six real `🚧` lines of
// ai-docs/spec/documentation-system.md, verbatim. Two of them embed the literal
// marker shapes (`## 🚧 Feature Name {#…}` and `> [!note] Planned 🚧`) inside
// inline code mid-line, so a "contains the shape anywhere" predicate — and the
// bare-emoji predicate — both report this prose file as marker-carrying. Only a
// line-start shape match keeps it clean.
func TestLegacyMarkerLinesMatchShapesNotProse(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"prose emoji in inline code", "Contract-first planned spec behavior uses `🚧` markers only when planned", false},
		{"prose heading shape mid-line", "feature is a heading such as `## 🚧 Feature Name {#YYMMDD-slug}`, and a planned", false},
		{"prose callout shape mid-line", "change to an existing feature uses a `> [!note] Planned 🚧` callout. Entries", false},
		{"prose emoji sentence start", "without `🚧` are treated as implemented and must be verified before committing.", false},
		{"prose entries clause", "implemented entries or contract-first `🚧` entries, verifies duplicate anchors,", false},
		{"prose strips clause", "adds missing implemented entries, strips `🚧` markers when implementation has", false},
		{"heading marker", "## 🚧 Feature {#260101-x}", true},
		{"deep heading marker", "###### 🚧 Feature {#260101-x}", true},
		{"callout marker", "> [!note] Planned 🚧 {#260101-x}", true},
		{"frontmatter list marker", "- 🚧 pending [260101-t/p1]", true},
		{"indented callout marker", "  > [!note] Planned 🚧 {#260101-x}", true},
		{"list without marker", "- pending [260101-t/p1]", false},
		{"callout without marker", "> [!note] Planned behavior arrives later.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := legacyMarkerLines(tc.line)
			if (len(got) > 0) != tc.want {
				t.Fatalf("legacyMarkerLines(%q) = %#v, want match=%v", tc.line, got, tc.want)
			}
		})
	}
}

func TestLegacyMarkerLinesExtractOwnAnchor(t *testing.T) {
	got := legacyMarkerLines("> [!note] Planned 🚧 {#260101-anchor}\n\n## 🚧 No anchor here\n")
	if len(got) != 2 {
		t.Fatalf("legacyMarkerLines = %#v", got)
	}
	if got[0].Anchor != "260101-anchor" {
		t.Fatalf("callout anchor = %q", got[0].Anchor)
	}
	if got[1].Anchor != "" {
		t.Fatalf("anchorless marker anchor = %q", got[1].Anchor)
	}
}

// The real corpus file must not be reported: this is the same clause-4 check as
// the table above, but against the live bytes rather than transcribed lines.
func TestLegacyMarkerLinesIgnoreMechanismProseFile(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "ai-docs", "spec", "documentation-system.md"))
	if err != nil {
		t.Skipf("documentation-system.md unavailable: %v", err)
	}
	if got := legacyMarkerLines(string(raw)); len(got) != 0 {
		t.Fatalf("documentation-system.md reported markers: %#v", got)
	}
}

const legacyMarkerSpecBody = "---\ntitle: Demo\n---\n# Demo\n\n" +
	"## Sibling Behavior {#260101-sibling}\n\n" +
	"Deterministic workspace root pruning is already implemented.\n\n" +
	"> [!note] Planned 🚧 {#260101-anchor}\n> The registry will prune stale roots.\n\n" +
	"## Other Sibling {#260101-other}\n\nAnother implemented behavior.\n"

func legacyMarkerCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", legacyMarkerSpecBody)
	mustWrite(t, root, "ai-docs/spec/clean.md", "---\ntitle: Clean\n---\n# Clean\n\n## Clean {#260101-clean}\n")
	// Unrelated ticket: references sibling anchors in the same file, never the
	// marker's own anchor or the file path. It must not flip the verdict.
	mustWrite(t, root, "ai-docs/tickets/todo/260101-feat-unrelated.md",
		"---\ntitle: Unrelated\nspec:\n  - 260101-sibling\n---\n# Unrelated\n\n## Spec Impact\n\n- 260101-other gains a note.\n")
	return root
}

func addLegacyMarkerOwnerTicket(t *testing.T, root string) {
	t.Helper()
	mustWrite(t, root, "ai-docs/tickets/ready/260102-feat-owner.md",
		"---\ntitle: Owner\n---\n# Owner\n\n## Spec Impact\n\n- ai-docs/spec/demo.md gains the prune policy entry.\n\n## Phases\n")
}

func removeLegacyMarkerOwnerTicket(t *testing.T, root string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, "ai-docs", "tickets", "ready", "260102-feat-owner.md")); err != nil {
		t.Fatal(err)
	}
}

// assertLegacyMarkerSurfaces checks all four advisory surfaces at once so a
// render point cannot regress in isolation. specs.find is exercised through its
// query path, which is a different formatter from the no-query fallback.
func assertLegacyMarkerSurfaces(t *testing.T, root string, wantContains []string, wantAbsent []string) {
	t.Helper()

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	byPath := map[string]SpecInfo{}
	for _, spec := range specs {
		byPath[spec.Path] = spec
	}
	checkAdvisory(t, "SpecsList", byPath["ai-docs/spec/demo.md"].LegacyMarkerAdvisory, wantContains, wantAbsent)
	if got := byPath["ai-docs/spec/clean.md"].LegacyMarkerAdvisory; got != "" {
		t.Fatalf("SpecsList advisory on marker-free spec = %q", got)
	}

	status, err := SpecsStatus(root, SpecStatusOptions{SpecStem: "260101-anchor"})
	if err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	checkAdvisory(t, "SpecsStatus", status.LegacyMarkerAdvisory, wantContains, wantAbsent)

	clean, err := SpecsStatus(root, SpecStatusOptions{SpecStem: "260101-clean"})
	if err != nil {
		t.Fatalf("SpecsStatus(clean) returned error: %v", err)
	}
	if clean.LegacyMarkerAdvisory != "" {
		t.Fatalf("SpecsStatus advisory on marker-free spec = %q", clean.LegacyMarkerAdvisory)
	}

	found, err := SpecsFind(root, SpecFindOptions{Query: "deterministic workspace root pruning"})
	if err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	advisory := ""
	for _, spec := range found {
		if spec.Path == "ai-docs/spec/demo.md" {
			advisory = spec.LegacyMarkerAdvisory
		}
	}
	if advisory == "" && len(wantContains) > 0 {
		t.Fatalf("SpecsFind query path returned no advisory: %#v", found)
	}
	checkAdvisory(t, "SpecsFind", advisory, wantContains, wantAbsent)

	tree, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if strings.Count(tree, "legacy-marker: ") != 1 {
		t.Fatalf("ProjectTree legacy-marker count != 1:\n%s", tree)
	}
	treeAdvisory := ""
	for _, line := range strings.Split(tree, "\n") {
		if _, rest, ok := strings.Cut(line, "legacy-marker: "); ok {
			treeAdvisory = rest
		}
	}
	// The rendered advisory must hang under demo.md, not clean.md.
	if !strings.Contains(tree, "demo.md  - Demo\n    legacy-marker: ") {
		t.Fatalf("ProjectTree legacy-marker not attached to demo.md:\n%s", tree)
	}
	checkAdvisory(t, "ProjectTree", treeAdvisory, wantContains, wantAbsent)
}

func checkAdvisory(t *testing.T, surface, advisory string, wantContains, wantAbsent []string) {
	t.Helper()
	for _, want := range wantContains {
		if !strings.Contains(advisory, want) {
			t.Fatalf("%s advisory missing %q: %q", surface, want, advisory)
		}
	}
	for _, absent := range wantAbsent {
		if strings.Contains(advisory, absent) {
			t.Fatalf("%s advisory contained %q: %q", surface, absent, advisory)
		}
	}
}

func TestLegacyMarkerAdvisoryFlipsWithLiveTicketState(t *testing.T) {
	root := legacyMarkerCorpus(t)

	orphanWant := []string{
		"legacy planned marker (retired mechanism): 1 marker(s)",
		"no live ticket references this spec",
		"the marker is orphaned",
		"Advisory only; this never blocks a commit.",
	}
	// D2 pin: the unrelated ticket names a sibling anchor in the same file, so
	// file-level anchor matching would report it here.
	orphanAbsent := []string{"260101-feat-unrelated", "move the marker text"}
	assertLegacyMarkerSurfaces(t, root, orphanWant, orphanAbsent)

	addLegacyMarkerOwnerTicket(t, root)
	matchedWant := []string{
		"legacy planned marker (retired mechanism): 1 marker(s)",
		"live tickets referencing this spec: 260102-feat-owner [ready]",
		"move the marker text into the ticket's ## Spec Impact, then strip the marker",
		"Advisory only; this never blocks a commit.",
	}
	assertLegacyMarkerSurfaces(t, root, matchedWant, []string{"orphaned", "260101-feat-unrelated"})

	// Ticket state is read per call, not cached across calls.
	removeLegacyMarkerOwnerTicket(t, root)
	assertLegacyMarkerSurfaces(t, root, orphanWant, orphanAbsent)
}

func TestLegacyMarkerAdvisoryMatchesOwnAnchorReference(t *testing.T) {
	root := legacyMarkerCorpus(t)
	mustWrite(t, root, "ai-docs/tickets/idea/260103-feat-anchor-owner.md",
		"---\ntitle: Anchor owner\nspec:\n  - 260101-anchor\n---\n# Anchor owner\n")

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	for _, spec := range specs {
		if spec.Path != "ai-docs/spec/demo.md" {
			continue
		}
		if !strings.Contains(spec.LegacyMarkerAdvisory, "260103-feat-anchor-owner [idea]") {
			t.Fatalf("advisory = %q", spec.LegacyMarkerAdvisory)
		}
		return
	}
	t.Fatal("demo.md missing from SpecsList")
}

// The advisory is a migration note, never a gate: no surface may fail because a
// marker exists.
func TestLegacyMarkerAdvisoryNeverBlocks(t *testing.T) {
	root := legacyMarkerCorpus(t)
	if _, err := SpecsList(root); err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	if _, err := SpecsFind(root, SpecFindOptions{}); err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	if _, err := SpecsStatus(root, SpecStatusOptions{SpecStem: "260101-anchor"}); err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	if _, err := ProjectTree(root); err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if _, err := VerifySpecIndex(root); err != nil {
		t.Fatalf("VerifySpecIndex returned error: %v", err)
	}
}
