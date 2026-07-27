package wsdoc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Every test here is synthetic. The live board cannot verify this work: all
// four integrity checks sit at zero live hits by design (that is the intended
// steady state for a compile-style guard), and the board block is a function
// of counts that move with every landing commit. The board numbers in the
// ticket's Output Format are illustrative renderings, never assertions.

type graphFixture struct {
	t    *testing.T
	root string
}

// newGraphFixture builds an empty board with an ai-docs/spec/ directory
// present. The directory is not optional scaffolding: scanSpecs errors on a
// missing ai-docs/spec, and the degrade-to-silence path would swallow that
// error along with every advisory, so a fixture without it silently asserts
// nothing. TestTicketGraphMissingSpecDirDegradesToSilence pins that behavior
// deliberately instead.
func newGraphFixture(t *testing.T) *graphFixture {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo spec\n\n## Something {#260101-demo-anchor}\n")
	return &graphFixture{t: t, root: root}
}

func (f *graphFixture) ticket(status, stem string, fields ...string) string {
	f.t.Helper()
	return f.ticketWithBody(status, stem, "", fields...)
}

func (f *graphFixture) ticketWithBody(status, stem, body string, fields ...string) string {
	f.t.Helper()
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + stem + "\n")
	switch status {
	case ".done":
		b.WriteString("completed: 2026-07-27\n")
	case ".dropped":
		b.WriteString("dropped: 2026-07-27\n")
	}
	for _, field := range fields {
		b.WriteString(field + "\n")
	}
	b.WriteString("---\n\n# " + stem + "\n")
	if body != "" {
		b.WriteString("\n" + body)
	}
	rel := "ai-docs/tickets/" + status + "/" + stem + ".md"
	mustWrite(f.t, f.root, rel, b.String())
	return rel
}

func (f *graphFixture) verify(paths ...string) VerifyResult {
	f.t.Helper()
	result, err := TicketVerify(f.root, paths)
	if err != nil {
		f.t.Fatalf("TicketVerify returned error: %v", err)
	}
	return result
}

func advisoryKinds(advisories []VerifyAdvisory) []string {
	out := make([]string, 0, len(advisories))
	for _, advisory := range advisories {
		out = append(out, advisory.Kind)
	}
	return out
}

func onlyAdvisory(t *testing.T, result VerifyResult, kind string) VerifyAdvisory {
	t.Helper()
	if len(result.Advisories) != 1 {
		t.Fatalf("Advisories = %#v, want exactly one", result.Advisories)
	}
	if result.Advisories[0].Kind != kind {
		t.Fatalf("advisory kind = %q, want %q", result.Advisories[0].Kind, kind)
	}
	return result.Advisories[0]
}

func boardAdvisoryText(t *testing.T, result VerifyResult) string {
	t.Helper()
	for _, advisory := range result.Advisories {
		if advisory.Kind == AdvisoryKindBoard {
			return advisory.Text
		}
	}
	t.Fatalf("no board advisory in %#v", result.Advisories)
	return ""
}

func hasBoardAdvisory(result VerifyResult) bool {
	for _, advisory := range result.Advisories {
		if advisory.Kind == AdvisoryKindBoard {
			return true
		}
	}
	return false
}

func requireNoAdvisories(t *testing.T, result VerifyResult) {
	t.Helper()
	if len(result.Advisories) != 0 {
		t.Fatalf("Advisories = %#v, want none", result.Advisories)
	}
}

func requireContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("text missing %q:\n%s", want, text)
	}
}

// requireContainsFlat matches prose after collapsing whitespace, so the
// hanging-indent wrap width stays an implementation detail. Row assertions
// deliberately use requireContains instead: there the exact indentation and the
// fixed-width status column are the contract.
func requireContainsFlat(t *testing.T, text, want string) {
	t.Helper()
	flat := strings.Join(strings.Fields(text), " ")
	if !strings.Contains(flat, want) {
		t.Fatalf("text missing %q (whitespace-collapsed):\n%s", want, text)
	}
}

func requireNotContains(t *testing.T, text, unwanted string) {
	t.Helper()
	if strings.Contains(text, unwanted) {
		t.Fatalf("text unexpectedly contains %q:\n%s", unwanted, text)
	}
}

// --- Integrity checks -------------------------------------------------------

func TestTicketGraphFlagsUnresolvableParent(t *testing.T) {
	f := newGraphFixture(t)
	child := f.ticket("todo", "260726-feat-orphan", "parent: 260726-epic-does-not-exist")

	result := f.verify(child)
	advisory := onlyAdvisory(t, result, AdvisoryKindFix)
	requireContains(t, advisory.Text, "FIX:")
	requireContainsFlat(t, advisory.Text, "parent: `260726-epic-does-not-exist` resolves to no ticket stem.")
	requireContainsFlat(t, advisory.Text, "Correct or remove the entry.")
	// The commit-path amend recipe is never part of the check text.
	requireNotContains(t, advisory.Text, "--amend")
}

func TestTicketGraphFlagsUnresolvableRelated(t *testing.T) {
	f := newGraphFixture(t)
	child := f.ticket("todo", "260726-feat-dangling",
		"related:",
		"  260726-nope-not-a-thing: why it matters")

	result := f.verify(child)
	advisory := onlyAdvisory(t, result, AdvisoryKindFix)
	requireContainsFlat(t, advisory.Text, "related: `260726-nope-not-a-thing` resolves to no ticket stem and no spec anchor. Correct or remove the entry.")
}

func TestTicketGraphParentCycleSuppressesBoard(t *testing.T) {
	f := newGraphFixture(t)
	a := f.ticket("todo", "260726-epic-cycle-a", "parent: 260726-epic-cycle-b")
	f.ticket("todo", "260726-epic-cycle-b", "parent: 260726-epic-cycle-a")

	result := f.verify(a)
	advisory := onlyAdvisory(t, result, AdvisoryKindCheck)
	requireContains(t, advisory.Text, "CHECK:")
	requireContainsFlat(t, advisory.Text, "forms a cycle")
	requireContainsFlat(t, advisory.Text, "`260726-epic-cycle-a` -> `260726-epic-cycle-b` -> `260726-epic-cycle-a`")
	if hasBoardAdvisory(result) {
		t.Fatalf("cyclic chain still produced a board block: %#v", result.Advisories)
	}
}

func TestTicketGraphFlagsNonEpicParent(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-refactor-not-an-epic")
	child := f.ticket("todo", "260726-feat-child", "parent: 260726-refactor-not-an-epic")

	result := f.verify(child)
	advisory := onlyAdvisory(t, result, AdvisoryKindCheck)
	requireContainsFlat(t, advisory.Text, "parent: `260726-refactor-not-an-epic` resolves to a ticket whose category is `refactor`, not `epic`. A parent must be an epic; confirm the intended parent.")
}

// --- Board block, all five renderings ---------------------------------------

func TestTicketGraphBoardAllChildrenClosed(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-closable")
	parent := "parent: 260726-epic-closable"
	f.ticket(".done", "260726-feat-one", parent)
	f.ticket(".done", "260726-feat-two", parent)
	verified := f.ticket(".done", "260726-feat-three", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "## Parent Board")
	requireContains(t, board, "Parent [1]: 260726-epic-closable [todo] - all 3 child tickets closed")
	requireContains(t, board, "    .done   | 260726-feat-one")
	requireContains(t, board, "    .done   | 260726-feat-three  (just now)")
	requireContains(t, board, "  ACTION: Check whether this epic can be closed.")
	requireContainsFlat(t, board, "Read its `## Completion Criteria` first")
	requireContains(t, board, "  No further ancestors.")
}

func TestTicketGraphBoardIdeaOnlyRemaining(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-idea-tier")
	parent := "parent: 260726-epic-idea-tier"
	f.ticket(".done", "260726-feat-landed-a", parent)
	verified := f.ticket(".done", "260726-feat-landed-b", parent)
	f.ticket("idea", "260726-feat-deferred", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "Parent [1]: 260726-epic-idea-tier [todo] - 2 of 3 closed, 1 idea/ remaining")
	requireContains(t, board, "    .done   | 260726-feat-landed-a")
	requireContains(t, board, "    .done   | 260726-feat-landed-b  (just now)")
	requireContains(t, board, "    idea    | 260726-feat-deferred")
	requireContainsFlat(t, board, "ACTION: Every accepted child has landed; only idea/ children remain.")

	// Closed rows precede the idea rows in this tier.
	if strings.Index(board, "260726-feat-landed-b") > strings.Index(board, "260726-feat-deferred") {
		t.Fatalf("closed rows must precede idea rows:\n%s", board)
	}
}

func TestTicketGraphBoardSiblingsRemainOnClosedPath(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-in-flight")
	parent := "parent: 260726-epic-in-flight"
	verified := f.ticket(".done", "260726-feat-just-landed", parent)
	f.ticket("todo", "260726-feat-still-open", parent)
	f.ticket("idea", "260726-feat-someday", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "Parent [1]: 260726-epic-in-flight [todo] - 2 of 3 child tickets still open")
	requireContains(t, board, "    todo    | 260726-feat-still-open")
	requireContains(t, board, "    idea    | 260726-feat-someday")
	// Closed children are omitted here: the question is what remains.
	requireNotContains(t, board, "260726-feat-just-landed")
	// Not closable, so no ACTION line.
	requireNotContains(t, board, "ACTION:")
}

func TestTicketGraphBoardAppliesRowCap(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-crowded")
	parent := "parent: 260726-epic-crowded"
	verified := f.ticket(".done", "260726-feat-landed", parent)
	for _, stem := range []string{
		"260726-feat-todo-a", "260726-feat-todo-b", "260726-feat-todo-c",
		"260726-feat-todo-d", "260726-feat-todo-e", "260726-feat-todo-f",
	} {
		f.ticket("todo", stem, parent)
	}
	f.ticket("idea", "260726-feat-idea-a", parent)
	f.ticket("idea", "260726-feat-idea-b", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "Parent [1]: 260726-epic-crowded [todo] - 8 of 9 child tickets still open")
	requireContains(t, board, "    ... +3 more open (1 todo, 2 idea)")

	rows := 0
	for _, line := range strings.Split(board, "\n") {
		if strings.HasPrefix(line, "    ") && strings.Contains(line, "| 260726-") {
			rows++
		}
	}
	if rows != 5 {
		t.Fatalf("rendered %d child rows, want the 5-row cap:\n%s", rows, board)
	}
}

func TestTicketGraphBoardAncestorAlreadyClosed(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket(".done", "260726-epic-already-closed")
	verified := f.ticket("todo", "260726-feat-late-child", "parent: 260726-epic-already-closed")

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "Parent [1]: 260726-epic-already-closed [.done] - parent already closed")
	requireContainsFlat(t, board, "NOTE: This parent is already closed. No action needed. If its `### Result` should mention this work, edit that Result; do not reopen the parent.")
	// No rows at all under a closed ancestor.
	requireNotContains(t, board, "|")
	// The NOTE renders on an ordinary todo/-path commit too, so it must not
	// assert that anything closed just now.
	requireNotContains(t, board, "just now")
	requireNotContains(t, board, "child tickets")
	requireNotContains(t, board, "ACTION:")
}

// --- Rules ------------------------------------------------------------------

func TestTicketGraphRelatedResolvesSpecAnchorButParentDoesNot(t *testing.T) {
	f := newGraphFixture(t)
	mustWrite(t, f.root, "ai-docs/spec/harness.md",
		"# Harness\n\n## Local agent tier config {#260513-harness-local-agent-tier-config}\n\nText.\n")

	related := f.ticket("todo", "260726-feat-points-at-spec",
		"related:",
		"  260513-harness-local-agent-tier-config: deliberate spec reference")
	requireNoAdvisories(t, f.verify(related))

	parented := f.ticket("todo", "260726-feat-parents-at-spec",
		"parent: 260513-harness-local-agent-tier-config")
	advisory := onlyAdvisory(t, f.verify(parented), AdvisoryKindFix)
	requireContainsFlat(t, advisory.Text, "parent: `260513-harness-local-agent-tier-config` resolves to no ticket stem.")
}

func TestTicketGraphDeduplicatesAncestorsAcrossPaths(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-shared")
	parent := "parent: 260726-epic-shared"
	first := f.ticket(".done", "260726-feat-shared-a", parent)
	second := f.ticket(".done", "260726-feat-shared-b", parent)

	board := boardAdvisoryText(t, f.verify(first, second))
	if got := strings.Count(board, "Parent [1]:"); got != 1 {
		t.Fatalf("ancestor rendered %d times, want 1:\n%s", got, board)
	}
	if got := strings.Count(board, "## Parent Board"); got != 1 {
		t.Fatalf("board header rendered %d times, want 1:\n%s", got, board)
	}
}

func TestTicketGraphNoParentEmitsNoBoardAtAll(t *testing.T) {
	f := newGraphFixture(t)
	standalone := f.ticket("todo", "260726-feat-standalone")

	result := f.verify(standalone)
	requireNoAdvisories(t, result)
}

func TestTicketGraphSiblingListingGatesOnClosedPath(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-gating")
	parent := "parent: 260726-epic-gating"
	openPath := f.ticket("todo", "260726-feat-alpha", parent)
	f.ticket("todo", "260726-feat-beta", parent)
	closedPath := f.ticket(".done", "260726-feat-gamma", parent)

	// Identical fixture, open path: the sibling listing is the high-volume
	// output, so it stays silent on an ordinary ticket-touching commit.
	requireNoAdvisories(t, f.verify(openPath))

	board := boardAdvisoryText(t, f.verify(closedPath))
	requireContains(t, board, "Parent [1]: 260726-epic-gating [todo] - 2 of 3 child tickets still open")
	requireContains(t, board, "    todo    | 260726-feat-alpha")
	requireContains(t, board, "    todo    | 260726-feat-beta")
}

func TestTicketGraphAppliesIntegrityCap(t *testing.T) {
	f := newGraphFixture(t)
	child := f.ticket("todo", "260726-feat-many-dangles",
		"related:",
		"  260726-nope-a: a",
		"  260726-nope-b: b",
		"  260726-nope-c: c",
		"  260726-nope-d: d",
		"  260726-nope-e: e",
		"  260726-nope-f: f")

	result := f.verify(child)
	if len(result.Advisories) != 6 {
		t.Fatalf("Advisories = %v, want 5 capped entries plus one overflow line", advisoryKinds(result.Advisories))
	}
	for i, advisory := range result.Advisories[:5] {
		if advisory.Kind != AdvisoryKindFix {
			t.Fatalf("advisory %d kind = %q, want %q", i, advisory.Kind, AdvisoryKindFix)
		}
	}
	overflow := result.Advisories[5]
	if overflow.Text != "... +1 more" {
		t.Fatalf("overflow advisory = %q, want %q", overflow.Text, "... +1 more")
	}
	// The overflow marker carries no remedy, so it must never attract the
	// commit path's amend recipe.
	if overflow.Kind == AdvisoryKindFix {
		t.Fatalf("overflow advisory kind = %q, want a non-fix kind", overflow.Kind)
	}
}

func TestTicketGraphRowsSortReadyTodoIdea(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-sorted")
	parent := "parent: 260726-epic-sorted"
	f.ticket("idea", "260726-feat-zzz-idea", parent)
	f.ticket("todo", "260726-feat-mmm-todo", parent)
	f.ticket("ready", "260726-feat-aaa-ready", parent)
	verified := f.ticket(".done", "260726-feat-landed", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	ready := strings.Index(board, "    ready   | 260726-feat-aaa-ready")
	todo := strings.Index(board, "    todo    | 260726-feat-mmm-todo")
	idea := strings.Index(board, "    idea    | 260726-feat-zzz-idea")
	if ready < 0 || todo < 0 || idea < 0 {
		t.Fatalf("missing a status row:\n%s", board)
	}
	if !(ready < todo && todo < idea) {
		t.Fatalf("rows must sort ready -> todo -> idea:\n%s", board)
	}
}

func TestTicketGraphChecksListFormRelated(t *testing.T) {
	f := newGraphFixture(t)
	// The list form is legal frontmatter that used to parse to []string and be
	// dropped by a bare map[string]string assertion, so the dangling-stem check
	// never saw it. The trailing comment is stripped as part of normalising.
	child := f.ticket("todo", "260726-feat-list-related",
		"related:",
		"  - 260726-nope-list-form  # historical reference")

	result := f.verify(child)
	advisory := onlyAdvisory(t, result, AdvisoryKindFix)
	requireContainsFlat(t, advisory.Text, "related: `260726-nope-list-form` resolves to no ticket stem and no spec anchor.")
}

// --- Degrade to silence -----------------------------------------------------

func TestTicketGraphMissingSpecDirDegradesToSilence(t *testing.T) {
	root := t.TempDir()
	// No ai-docs/spec at all, so scanSpecs errors. The ticket carries a
	// dangling related: that would otherwise produce a FIX:, which makes the
	// silence assertion non-vacuous.
	mustWrite(t, root, "ai-docs/tickets/.done/260726-feat-no-specs.md",
		"---\ntitle: No specs\ncompleted: 2026-07-27\nrelated:\n  260726-nope-dangling: n\n---\n\n# No specs\n")

	result, err := TicketVerify(root, []string{"ai-docs/tickets/.done/260726-feat-no-specs.md"})
	if err != nil {
		t.Fatalf("a graph-load failure became an error return: %v", err)
	}
	if !result.OK {
		t.Fatalf("result.OK = false, want true; findings = %#v", result.Findings)
	}
	if len(result.Advisories) != 0 {
		t.Fatalf("Advisories = %#v, want none after a graph-load failure", result.Advisories)
	}
}

func TestTicketGraphUnreadableTicketDegradesToSilence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not make a file unreadable on Windows")
	}
	f := newGraphFixture(t)
	verified := f.ticket("todo", "260726-feat-readable",
		"related:",
		"  260726-nope-dangling: n")
	broken := f.ticket("todo", "260726-feat-unreadable")

	brokenAbs := filepath.Join(f.root, filepath.FromSlash(broken))
	if err := os.Chmod(brokenAbs, 0o000); err != nil {
		t.Skipf("cannot make a file unreadable here: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(brokenAbs, 0o644) })
	if _, err := os.ReadFile(brokenAbs); err == nil {
		t.Skip("mode 000 is still readable (running as root?)")
	}

	result, err := TicketVerify(f.root, []string{verified})
	if err != nil {
		t.Fatalf("an unreadable board file became an error return: %v", err)
	}
	if !result.OK {
		t.Fatalf("result.OK = false, want true; findings = %#v", result.Findings)
	}
	if len(result.Advisories) != 0 {
		t.Fatalf("Advisories = %#v, want none after a graph-load failure", result.Advisories)
	}
}

// --- Deliberate negative cases ----------------------------------------------

func TestTicketGraphRelatedSpecAnchorEmitsNothing(t *testing.T) {
	f := newGraphFixture(t)
	mustWrite(t, f.root, "ai-docs/spec/namespace.md",
		"# Namespace\n\n## Lead skill namespace surface {#260505-lead-skill-namespace-surface}\n")
	ticket := f.ticket("todo", "260726-feat-spec-related",
		"related:",
		"  260505-lead-skill-namespace-surface: deliberate reference")

	requireNoAdvisories(t, f.verify(ticket))
}

func TestTicketGraphEpicWithoutChildrenEmitsNothing(t *testing.T) {
	f := newGraphFixture(t)
	epic := f.ticket("todo", "260726-epic-pre-decomposition")

	requireNoAdvisories(t, f.verify(epic))
}

func TestTicketGraphIgnoresChildStemsNamedOnlyInTheEpicBody(t *testing.T) {
	f := newGraphFixture(t)
	// The child exists but carries no parent: back-link. `parent:` is the sole
	// authority for the edge set and no check reads a body, so nothing fires.
	f.ticket("todo", "260726-feat-body-mentioned")
	epic := f.ticketWithBody("todo", "260726-epic-prose-board",
		"## Child Tickets\n\n- 260726-feat-body-mentioned: named in prose only\n- Planned: something that does not exist yet\n")

	requireNoAdvisories(t, f.verify(epic))
}
