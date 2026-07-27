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
		{"three-space heading marker", "   ## 🚧 Feature {#260101-x}", true},
		// Four columns opens a CommonMark indented code block, which is the
		// ordinary idiom for showing the marker syntax as an example.
		{"four-space indented example", "    ## 🚧 Indented Example {#260101-x}", false},
		{"tab indented example", "\t> [!note] Planned 🚧 {#260101-x}", false},
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

// A fenced example of the marker syntax is documentation, not a marker. This is
// the same clause-4 promise as the prose table, for the form a spec author
// reaches for when showing more than one shape at once.
func TestLegacyMarkerLinesIgnoreFencedExamples(t *testing.T) {
	text := strings.Join([]string{
		"# Doc",
		"",
		"Write a planned feature like one of these:",
		"",
		"```markdown",
		"## 🚧 Example Feature {#260101-x}",
		"> [!note] Planned 🚧 {#260101-y}",
		"- 🚧 pending [260101-t/p1]",
		"```",
		"",
		"A tilde fence works the same way:",
		"",
		"~~~",
		"## 🚧 Tilde Example {#260101-z}",
		"~~~",
		"",
		"An indented fence, closed by a longer run:",
		"",
		"  ````text",
		"## 🚧 Long Fence Example {#260101-w}",
		"  ````",
		"",
		"## 🚧 Real Marker {#260101-real}",
		"",
	}, "\n")
	got := legacyMarkerLines(text)
	if len(got) != 1 {
		t.Fatalf("legacyMarkerLines reported %d markers: %#v", len(got), got)
	}
	if len(got[0].Anchors) != 1 || got[0].Anchors[0] != "260101-real" {
		t.Fatalf("marker anchors = %#v", got[0].Anchors)
	}
	if got[0].Line != 23 {
		t.Fatalf("marker line = %d, want 23", got[0].Line)
	}
}

func TestLegacyMarkerLinesExtractAllAnchorsAndLineNumbers(t *testing.T) {
	got := legacyMarkerLines("> [!note] Planned 🚧 {#260101-anchor}\n\n## 🚧 Two {#260101-first} {#260101-second}\n\n## 🚧 No anchor here\n")
	if len(got) != 3 {
		t.Fatalf("legacyMarkerLines = %#v", got)
	}
	if len(got[0].Anchors) != 1 || got[0].Anchors[0] != "260101-anchor" || got[0].Line != 1 {
		t.Fatalf("callout marker = %#v", got[0])
	}
	if strings.Join(got[1].Anchors, ",") != "260101-first,260101-second" || got[1].Line != 3 {
		t.Fatalf("multi-anchor marker = %#v", got[1])
	}
	if len(got[2].Anchors) != 0 || got[2].Line != 5 {
		t.Fatalf("anchorless marker = %#v", got[2])
	}
}

// The real corpus files must not be reported: this is the same clause-4 check as
// the tables above, but against the live bytes rather than transcribed lines. A
// missing file is a broken assumption, not an excused environment — skipping
// here would silently retire the strongest guard in the suite.
func TestLegacyMarkerLinesIgnoreMechanismProseFile(t *testing.T) {
	for _, name := range []string{"documentation-system.md", "workflow-skills.md", "mcp-tools.md"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "ai-docs", "spec", name))
		if err != nil {
			t.Fatalf("%s unavailable: %v", name, err)
		}
		if got := legacyMarkerLines(string(raw)); len(got) != 0 {
			t.Fatalf("%s reported markers: %#v", name, got)
		}
	}
}

const legacyMarkerSpecBody = "---\ntitle: Demo\n---\n# Demo\n\n" +
	"## Sibling Behavior {#260101-sibling}\n\n" +
	"Deterministic workspace root pruning is already implemented.\n\n" +
	"> [!note] Planned 🚧 {#260101-anchor}\n> The registry will prune stale roots.\n\n" +
	"## Other Sibling {#260101-other}\n\nAnother implemented behavior.\n"

// legacyMarkerAdvisoryPrefix is the note's leading clause, including the marker
// line number the caller needs in order to act on "strip the marker".
const legacyMarkerAdvisoryPrefix = "legacy planned marker (contract-first planned-entry mechanism being retired by " +
	"260726-refactor-retire-spec-planned-marker-mechanism): 1 marker(s) at line 10"

func legacyMarkerCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", legacyMarkerSpecBody)
	mustWrite(t, root, "ai-docs/spec/clean.md", "---\ntitle: Clean\n---\n# Clean\n\n## Clean {#260101-clean}\n")
	// Unrelated ticket: references sibling anchors in the same file, never the
	// marker's own anchor or the file path. It must not flip the verdict.
	mustWrite(t, root, "ai-docs/tickets/todo/260101-feat-unrelated.md",
		"---\ntitle: Unrelated\nspec:\n  - 260101-sibling\n---\n# Unrelated\n\n## Spec Impact\n\n- 260101-other gains a note.\n")
	// Closed ticket that owns the marker outright. The scan is live-only
	// (idea/todo/ready), so it must never appear: naming a `.done` ticket would
	// tell the caller to move contract text into a ticket nobody will reopen.
	mustWrite(t, root, "ai-docs/tickets/.done/260105-feat-closed.md",
		"---\ntitle: Closed\nspec:\n  - 260101-anchor\n---\n# Closed\n\n## Spec Impact\n\n- ai-docs/spec/demo.md gained the prune policy entry.\n")
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
		legacyMarkerAdvisoryPrefix,
		"no live ticket references this spec",
		"the marker is orphaned",
		"or as an Implementation Gap callout if it did not",
		"Advisory only; this never blocks a commit.",
	}
	// D2 pin: the unrelated ticket names a sibling anchor in the same file, so
	// file-level anchor matching would report it here. The `.done` ticket owns
	// the marker's own anchor, so a scan that included archived statuses would
	// report it here too.
	orphanAbsent := []string{"260101-feat-unrelated", "260105-feat-closed", "move the marker text"}
	assertLegacyMarkerSurfaces(t, root, orphanWant, orphanAbsent)

	addLegacyMarkerOwnerTicket(t, root)
	matchedWant := []string{
		legacyMarkerAdvisoryPrefix,
		"live tickets referencing this spec: 260102-feat-owner [ready]",
		"move the marker text into the ticket's ## Spec Impact, then strip the marker",
		"Advisory only; this never blocks a commit.",
	}
	assertLegacyMarkerSurfaces(t, root, matchedWant, []string{"orphaned", "260101-feat-unrelated", "260105-feat-closed"})

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

// A ticket may name any anchor the marker line declares, not just the first.
func TestLegacyMarkerAdvisoryMatchesSecondAnchorOnMarkerLine(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/two.md",
		"---\ntitle: Two\n---\n# Two\n\n## 🚧 Two Anchors {#260110-first} {#260110-second}\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260111-feat-second.md",
		"---\ntitle: Second\nspec:\n  - 260110-second\n---\n# Second\n")

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("SpecsList = %#v", specs)
	}
	if !strings.Contains(specs[0].LegacyMarkerAdvisory, "260111-feat-second [ready]") {
		t.Fatalf("advisory = %q", specs[0].LegacyMarkerAdvisory)
	}
}

// Multiple owners render as a comma-joined, stem-sorted list. Directory scan
// order is idea/todo/ready, so the todo ticket is discovered first and only the
// sorts put the ready ticket in front — both the resolver sort and the rendered
// sort are load-bearing here. The todo ticket matches through `spec-remove:`,
// which is the other harvest source with no other coverage.
func TestLegacyMarkerAdvisoryJoinsMatchedTicketsInStemOrder(t *testing.T) {
	root := legacyMarkerCorpus(t)
	addLegacyMarkerOwnerTicket(t, root)
	mustWrite(t, root, "ai-docs/tickets/todo/260104-feat-other.md",
		"---\ntitle: Other\nspec-remove:\n  - 260101-anchor\n---\n# Other\n")

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	for _, spec := range specs {
		if spec.Path != "ai-docs/spec/demo.md" {
			continue
		}
		want := "live tickets referencing this spec: 260102-feat-owner [ready], 260104-feat-other [todo] —"
		if !strings.Contains(spec.LegacyMarkerAdvisory, want) {
			t.Fatalf("advisory = %q, want %q", spec.LegacyMarkerAdvisory, want)
		}
		return
	}
	t.Fatal("demo.md missing from SpecsList")
}

// The `- 🚧 ` list shape reaches the advisory surfaces, not only the predicate.
func TestLegacyMarkerListShapeReachesAdvisorySurfaces(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/listed.md",
		"---\ntitle: Listed\nfeatures:\n  - 🚧 pending prune policy [260112-t/p1]\n---\n# Listed\n\n## Listed {#260112-listed}\n")

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	if len(specs) != 1 || !strings.Contains(specs[0].LegacyMarkerAdvisory, "1 marker(s) at line 4") {
		t.Fatalf("SpecsList advisory = %#v", specs)
	}

	tree, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if !strings.Contains(tree, "legacy-marker: ") {
		t.Fatalf("ProjectTree lost the list-shape advisory:\n%s", tree)
	}

	status, err := SpecsStatus(root, SpecStatusOptions{SpecStem: "260112-listed"})
	if err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	if !strings.Contains(status.LegacyMarkerAdvisory, "1 marker(s) at line 4") {
		t.Fatalf("SpecsStatus advisory = %q", status.LegacyMarkerAdvisory)
	}
}

// A fenced commit template inside `## Spec Impact` must not close the section:
// the house template quotes `## AI Context` / `## Spec` lines, and a fence-blind
// scan drops every reference after them, flipping an owned marker to "orphaned;
// strip it".
func TestSpecImpactSectionSurvivesFencedHeadings(t *testing.T) {
	root := legacyMarkerCorpus(t)
	mustWrite(t, root, "ai-docs/tickets/ready/260107-feat-fenced.md",
		"---\ntitle: Fenced\n---\n# Fenced\n\n## Spec Impact\n\nThe commit body uses the house template:\n\n"+
			"```text\n<type>(<scope>): <summary>\n\n## AI Context\n- why\n\n## Spec\n- some-spec\n```\n\n"+
			"- ai-docs/spec/demo.md gains the prune policy entry.\n\n## Phases\n")

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	for _, spec := range specs {
		if spec.Path != "ai-docs/spec/demo.md" {
			continue
		}
		if !strings.Contains(spec.LegacyMarkerAdvisory, "260107-feat-fenced [ready]") {
			t.Fatalf("advisory = %q", spec.LegacyMarkerAdvisory)
		}
		return
	}
	t.Fatal("demo.md missing from SpecsList")
}

// `## Spec Impacts` and `## Spec Impact Analysis` are different sections, and a
// `## Spec Impact` line quoted inside a fence is not a heading at all.
func TestSpecImpactSectionOpensOnlyOnItsOwnHeading(t *testing.T) {
	root := legacyMarkerCorpus(t)
	mustWrite(t, root, "ai-docs/tickets/todo/260108-feat-near-heading.md",
		"---\ntitle: Near\n---\n# Near\n\n## Spec Impacts\n\n- ai-docs/spec/demo.md sits under a plural heading.\n\n"+
			"## Spec Impact Analysis\n\n- 260101-anchor sits under a longer heading.\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260109-feat-quoted-heading.md",
		"---\ntitle: Quoted\n---\n# Quoted\n\n## Notes\n\n```text\n## Spec Impact\n\n- ai-docs/spec/demo.md\n```\n")

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	for _, spec := range specs {
		if spec.Path != "ai-docs/spec/demo.md" {
			continue
		}
		checkAdvisory(t, "SpecsList", spec.LegacyMarkerAdvisory,
			[]string{"no live ticket references this spec"},
			[]string{"260108-feat-near-heading", "260109-feat-quoted-heading"})
		return
	}
	t.Fatal("demo.md missing from SpecsList")
}

// One unreadable ticket must never turn into "the marker is orphaned; strip
// it". An unread ticket is an unknown owner, not an absent one, and a read
// failure must not produce an instruction to delete a contract.
func TestLegacyMarkerAdvisoryReportsIncompleteScanInsteadOfOrphaned(t *testing.T) {
	root := legacyMarkerCorpus(t)
	locked := filepath.Join(root, "ai-docs", "tickets", "todo", "260106-feat-locked.md")
	if err := os.WriteFile(locked, []byte("---\ntitle: Locked\n---\n# Locked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("unreadable-ticket simulation ineffective in this environment")
	}

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	for _, spec := range specs {
		if spec.Path != "ai-docs/spec/demo.md" {
			continue
		}
		checkAdvisory(t, "SpecsList", spec.LegacyMarkerAdvisory,
			[]string{
				"the live ticket scan was incomplete",
				"marker ownership could not be determined",
				"Advisory only; this never blocks a commit.",
			},
			[]string{"the marker is orphaned", "strip it"})
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
