package wsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The "no match" rows are the six `🚧` lines ai-docs/spec/documentation-system.md
// carried until 33820249 removed them. ai-docs/spec/ now holds zero markers, so
// these are retained synthetic prose fixtures, not samples of live repo text.
// They still discriminate: two embed the literal marker shapes
// (`## 🚧 Feature Name {#…}` and `> [!note] Planned 🚧`) inside inline code
// mid-line, so a "contains the shape anywhere" predicate — and the bare-emoji
// predicate — both report such prose as marker-carrying. Only a line-start shape
// match keeps it clean, which is what a downstream project still holding marker
// prose needs.
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
		// This line carries no `---` delimiters, so it is not actually inside
		// frontmatter — it only pins the bare list-shape regex at column 0.
		// See TestLegacyMarkerLinesDetectNestedFrontmatterMarker and
		// TestLegacyMarkerLinesCombineFrontmatterAndBodyMarkers for real
		// frontmatter-block coverage.
		{"list marker", "- 🚧 pending [260101-t/p1]", true},
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
		"  ```text",
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

// Each row pins one CommonMark fence rule by its observable consequence: a
// marker line that is inside the block when the rule holds and outside it when
// the rule is dropped. Without these, a tracker that closed on a shorter run, on
// the other fence character, or on a line carrying an info string would flag
// documentation as a live marker with the suite green.
func TestLegacyMarkerLinesFollowFenceOpenAndCloseRules(t *testing.T) {
	const marker = "## 🚧 Example {#260101-x}"
	cases := []struct {
		name  string
		lines []string
		want  int
	}{
		// A closing fence may not carry an info string, so the second ```go
		// keeps the block open and the marker stays inside it.
		{"info string does not close", []string{"```go", "func x() {}", "```go", marker, "```"}, 0},
		{"shorter run does not close", []string{"````", "x", "```", marker, "````"}, 0},
		{"other fence char does not close", []string{"```", "x", "~~~", marker, "```"}, 0},
		{"longer run closes", []string{"```", "x", "````", marker}, 1},
		{"two backticks are not a fence", []string{"``", marker}, 1},
		// "``` a ` b" cannot open a fence: a backtick info string may not
		// contain a backtick, so the marker below it is a real marker.
		{"backtick in info string is not an opener", []string{"``` a ` b", marker}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := legacyMarkerLines(strings.Join(tc.lines, "\n"))
			if len(got) != tc.want {
				t.Fatalf("legacyMarkerLines(%q) = %#v, want %d markers", tc.lines, got, tc.want)
			}
		})
	}
}

// YAML frontmatter is not CommonMark: its keys nest by indentation, so a
// `features:` marker sits four or more columns in as a matter of course.
// Applying the indented-code-block rule there would silently drop it while
// markerContext still reports the same line.
func TestLegacyMarkerLinesDetectNestedFrontmatterMarker(t *testing.T) {
	text := "---\ntitle: Nested\nfeatures:\n  planned:\n    - 🚧 pending prune policy [260113-t/p1]\n---\n# Nested\n"
	got := legacyMarkerLines(text)
	if len(got) != 1 || got[0].Line != 5 {
		t.Fatalf("legacyMarkerLines = %#v, want one marker at line 5", got)
	}
}

// A downstream tree mid-migration typically carries both forms at once: a
// `features:` frontmatter entry not yet rewritten, and body markers still
// pending conversion. 260726 2.7 requires the frontmatter form to be covered
// "not only body forms", and the retained detection surface keeps it as the
// same list-shape regex applied without a frontmatter exemption (see the
// bodyStart comment in legacyMarkerLines). This pins that the two forms are
// folded into one line-ordered result rather than the frontmatter entry being
// silently dropped or reported through a separate path.
func TestLegacyMarkerLinesCombineFrontmatterAndBodyMarkers(t *testing.T) {
	text := "---\ntitle: Mixed\nfeatures:\n  - 🚧 pending prune policy [260118-t/p1]\n---\n" +
		"# Mixed\n\nSome implemented behavior.\n\n" +
		"## 🚧 First Body Marker {#260118-first}\n\nSome more prose.\n\n" +
		"> [!note] Planned 🚧 {#260118-second}\n> Body callout text.\n\n" +
		"- 🚧 pending body list item [260118-t/p2]\n"
	got := legacyMarkerLines(text)
	if len(got) != 4 {
		t.Fatalf("legacyMarkerLines = %#v, want 4 markers", got)
	}
	wantLines := []int{4, 10, 14, 17}
	for i, line := range wantLines {
		if got[i].Line != line {
			t.Fatalf("marker %d line = %d, want %d (%#v)", i, got[i].Line, line, got)
		}
	}
	if len(got[0].Anchors) != 0 {
		t.Fatalf("frontmatter marker anchors = %#v, want none", got[0].Anchors)
	}
	if len(got[1].Anchors) != 1 || got[1].Anchors[0] != "260118-first" {
		t.Fatalf("heading marker anchors = %#v", got[1].Anchors)
	}
	if len(got[2].Anchors) != 1 || got[2].Anchors[0] != "260118-second" {
		t.Fatalf("callout marker anchors = %#v", got[2].Anchors)
	}
	if len(got[3].Anchors) != 0 {
		t.Fatalf("body list marker anchors = %#v, want none", got[3].Anchors)
	}
}

// A marker shape inside an HTML comment is documentation, not a marker.
func TestLegacyMarkerLinesIgnoreHTMLCommentedMarkers(t *testing.T) {
	text := "# Doc\n\n<!--\n## 🚧 Commented {#260101-x}\n-->\n\n<!-- one-liner: ## 🚧 Inline {#260101-y} -->\n\n## 🚧 Real Marker {#260101-real}\n"
	got := legacyMarkerLines(text)
	if len(got) != 1 || got[0].Line != 9 {
		t.Fatalf("legacyMarkerLines = %#v, want one marker at line 9", got)
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

// Multiple owners render as a comma-joined, stem-sorted list. TicketsList orders
// by status rank first (ready < todo), so it hands the resolver the ready ticket
// ahead of the todo ticket even though `260100-feat-other` sorts before
// `260102-feat-owner`. The rendered sort is therefore the sole carrier of the
// output order here: deleting it, or reversing it, produces the wrong string.
// The todo ticket matches through `spec-remove:`, which is the other harvest
// source with no other coverage.
func TestLegacyMarkerAdvisoryJoinsMatchedTicketsInStemOrder(t *testing.T) {
	root := legacyMarkerCorpus(t)
	addLegacyMarkerOwnerTicket(t, root)
	mustWrite(t, root, "ai-docs/tickets/todo/260100-feat-other.md",
		"---\ntitle: Other\nspec-remove:\n  - 260101-anchor\n---\n# Other\n")

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	for _, spec := range specs {
		if spec.Path != "ai-docs/spec/demo.md" {
			continue
		}
		want := "live tickets referencing this spec: 260100-feat-other [todo], 260102-feat-owner [ready] —"
		if !strings.Contains(spec.LegacyMarkerAdvisory, want) {
			t.Fatalf("advisory = %q, want %q", spec.LegacyMarkerAdvisory, want)
		}
		return
	}
	t.Fatal("demo.md missing from SpecsList")
}

// A spec carrying more than one marker must report every line, the real count,
// and the plural label: the advisory's whole added value is telling the caller
// where to strip, and that instruction is only non-trivial above N=1.
func TestLegacyMarkerAdvisoryRendersEveryMarkerLine(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/multi.md",
		"---\ntitle: Multi\n---\n# Multi\n\n## 🚧 First {#260120-first}\n\n## 🚧 Second {#260120-second}\n")

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("SpecsList = %#v", specs)
	}
	want := "2 marker(s) at lines 6, 8;"
	if !strings.Contains(specs[0].LegacyMarkerAdvisory, want) {
		t.Fatalf("advisory = %q, want %q", specs[0].LegacyMarkerAdvisory, want)
	}

	tree, err := ProjectTree(root)
	if err != nil {
		t.Fatalf("ProjectTree returned error: %v", err)
	}
	if !strings.Contains(tree, want) {
		t.Fatalf("ProjectTree lost the multi-marker line list:\n%s", tree)
	}
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

// A `## Spec Impact` line quoted inside a fence is not a heading at all, so the
// ticket quoting one owns nothing. This is also the `.done` guard: the closed
// ticket in the corpus names the marker's own anchor, so a scan that included
// archived statuses would break the orphaned assertion.
func TestSpecImpactSectionIgnoresFencedHeading(t *testing.T) {
	root := legacyMarkerCorpus(t)
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
			[]string{"260109-feat-quoted-heading", "260105-feat-closed"})
		return
	}
	t.Fatal("demo.md missing from SpecsList")
}

// The advisory and `readyGateWarning` must agree on what a Spec Impact section
// is. The ready gate opens on the loose prefix `## Spec Impact`, so a ticket
// headed `## Spec Impact and Phases` is promoted as spec-addressed; if the
// advisory required a stricter form, that same ticket's markers would then be
// reported as orphaned and the caller told to delete a live contract.
func TestSpecImpactSectionOpensOnTheSameLoosePrefixAsTheReadyGate(t *testing.T) {
	for _, heading := range []string{
		"## Spec Impact",
		"## Spec Impact and Phases",
		"## Spec Impact Analysis",
		"## Spec Impacts",
		"## Spec Impact — notes",
	} {
		t.Run(heading, func(t *testing.T) {
			root := legacyMarkerCorpus(t)
			mustWrite(t, root, "ai-docs/tickets/ready/260115-feat-loose-heading.md",
				"---\ntitle: Loose\n---\n# Loose\n\n"+heading+"\n\n- ai-docs/spec/demo.md gains the prune policy entry.\n")

			// The ready gate's own predicate, applied to the same heading.
			if !strings.HasPrefix(strings.TrimSpace(heading), "## Spec Impact") {
				t.Fatalf("fixture heading %q is not one the ready gate accepts", heading)
			}
			assertDemoAdvisoryNames(t, root, "260115-feat-loose-heading [ready]")
		})
	}
}

// A 4-space-indented quote of the house commit template must not close the
// section: the block-indent rule the predicate already applies to marker lines
// governs the section heading too, otherwise the verdict flips to "orphaned;
// strip it" on the presence of an unrelated indented block.
func TestSpecImpactSectionSurvivesIndentedHeadings(t *testing.T) {
	root := legacyMarkerCorpus(t)
	mustWrite(t, root, "ai-docs/tickets/ready/260116-feat-indented.md",
		"---\ntitle: Indented\n---\n# Indented\n\n## Spec Impact\n\nThe commit body uses the house template:\n\n"+
			"    <type>(<scope>): <summary>\n\n    ## AI Context\n    - why\n\n    ## Spec\n    - some-spec\n\n"+
			"- ai-docs/spec/demo.md gains the prune policy entry.\n\n## Phases\n")

	assertDemoAdvisoryNames(t, root, "260116-feat-indented [ready]")
}

// An unbalanced fence anywhere in the ticket must not hide the section. The
// markdown-in-markdown idiom — a fenced example that itself contains a fence,
// written without widening the outer fence — leaves the fence count odd, and a
// tracker left open at EOF swallows the heading and every reference under it.
// When the fences do not balance, fence tracking is not trustworthy for that
// document, so the scan runs again without it.
func TestSpecImpactSectionRecoversFromUnbalancedFences(t *testing.T) {
	root := legacyMarkerCorpus(t)
	mustWrite(t, root, "ai-docs/tickets/ready/260117-feat-unbalanced.md",
		"---\ntitle: Unbalanced\n---\n# Unbalanced\n\n## Context\n\n"+
			"```markdown\n<type>(<scope>): <summary>\n\n```go\nx := 1\n```\n```\n\n"+
			"## Spec Impact\n\n- ai-docs/spec/demo.md gains the prune policy entry.\n")

	assertDemoAdvisoryNames(t, root, "260117-feat-unbalanced [ready]")
}

func assertDemoAdvisoryNames(t *testing.T, root, want string) {
	t.Helper()
	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	for _, spec := range specs {
		if spec.Path != "ai-docs/spec/demo.md" {
			continue
		}
		checkAdvisory(t, "SpecsList", spec.LegacyMarkerAdvisory,
			[]string{want, "move the marker text"},
			[]string{"the marker is orphaned", "strip it,"})
		return
	}
	t.Fatal("demo.md missing from SpecsList")
}

// A live-ticket scan that fails must never turn into "the marker is orphaned;
// strip it". An unscanned ticket tree is an unknown owner, not an absent one,
// and a read failure must not produce an instruction to delete a contract.
//
// The failure is driven by a tickets path that exists but is not a directory.
// That is permission-independent by construction: unlike `chmod 000`, which is a
// no-op on DrvFs/9p mounts, as root, and on Windows, it fails identically on
// every platform and filesystem, so this guard can never degrade into a skip.
func TestLegacyMarkerAdvisoryReportsIncompleteScanInsteadOfOrphaned(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", legacyMarkerSpecBody)
	if err := os.WriteFile(filepath.Join(root, "ai-docs", "tickets"), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
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

// A repository with no tickets tree has no live tickets. That is a determinate
// answer, not a failed scan: reporting it as incomplete would suppress a correct
// orphaned verdict permanently.
func TestLegacyMarkerAdvisoryReportsOrphanedWithoutTicketsDirectory(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", legacyMarkerSpecBody)

	specs, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	for _, spec := range specs {
		if spec.Path != "ai-docs/spec/demo.md" {
			continue
		}
		checkAdvisory(t, "SpecsList", spec.LegacyMarkerAdvisory,
			[]string{"no live ticket references this spec", "the marker is orphaned"},
			[]string{"the live ticket scan was incomplete"})
		return
	}
	t.Fatal("demo.md missing from SpecsList")
}

// A nil resolver has no ownership knowledge, which is the same epistemic state
// as a failed scan. It must never default into the delete instruction.
func TestLegacyMarkerAdviseOnNilResolverNeverSaysStripIt(t *testing.T) {
	var resolver *legacyMarkerResolver
	got := resolver.advise("ai-docs/spec/demo.md", []legacyMarker{{Line: 3}})
	checkAdvisory(t, "nil resolver", got,
		[]string{"the live ticket scan was incomplete", "marker ownership could not be determined"},
		[]string{"the marker is orphaned", "strip it"})
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
