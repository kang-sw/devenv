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

// TestTicketGraphGatingIsOredAcrossDeduplicatedOccurrences pins the one board
// rule the rest of the suite leaves dead: when several verified tickets share
// an ancestor, the sibling listing must render if *any* of them sits under a
// closed status directory, whichever order the paths arrive in. Deduplication
// keeps the first occurrence's depth label, so it is only the gated flag that
// has to be ORed rather than overwritten.
func TestTicketGraphGatingIsOredAcrossDeduplicatedOccurrences(t *testing.T) {
	build := func(t *testing.T) (*graphFixture, string, string) {
		t.Helper()
		f := newGraphFixture(t)
		f.ticket("todo", "260726-epic-mixed-gating")
		parent := "parent: 260726-epic-mixed-gating"
		openPath := f.ticket("todo", "260726-feat-open-sibling", parent)
		closedPath := f.ticket(".done", "260726-feat-closed-sibling", parent)
		f.ticket("todo", "260726-feat-other-sibling", parent)
		return f, openPath, closedPath
	}

	const header = "Parent [1]: 260726-epic-mixed-gating [todo] - 2 of 3 child tickets still open"

	t.Run("ungated path first", func(t *testing.T) {
		f, openPath, closedPath := build(t)
		requireContains(t, boardAdvisoryText(t, f.verify(openPath, closedPath)), header)
	})

	t.Run("gated path first", func(t *testing.T) {
		f, openPath, closedPath := build(t)
		requireContains(t, boardAdvisoryText(t, f.verify(closedPath, openPath)), header)
	})

	t.Run("ungated path alone stays silent", func(t *testing.T) {
		f, openPath, _ := build(t)
		requireNoAdvisories(t, f.verify(openPath))
	})
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

// TestTicketGraphIntegrityCapIsPerVerifiedTicket pins the cap's granularity.
// Capped per call instead, this fixture would emit five advisories plus
// "... +4 more", and because an advisory names no subject the caller could
// not tell which of the three tickets lost its advisories entirely.
func TestTicketGraphIntegrityCapIsPerVerifiedTicket(t *testing.T) {
	f := newGraphFixture(t)
	var paths []string
	for _, stem := range []string{"260726-feat-cap-a", "260726-feat-cap-b", "260726-feat-cap-c"} {
		paths = append(paths, f.ticket("todo", stem,
			"related:",
			"  260726-nope-1: x",
			"  260726-nope-2: x",
			"  260726-nope-3: x"))
	}

	result := f.verify(paths...)
	if len(result.Advisories) != 9 {
		t.Fatalf("Advisories = %v, want 3 per verified ticket with no cap reached", advisoryKinds(result.Advisories))
	}
	for _, advisory := range result.Advisories {
		if strings.HasPrefix(advisory.Text, "... +") {
			t.Fatalf("cap fired across the call rather than per verified ticket:\n%#v", result.Advisories)
		}
	}
}

// TestTicketGraphIntegritySubjectIsTheVerifiedFileNotTheStem pins the subject
// set against the duplicate-stem guard. byStem keeps the most-open copy for the
// board half, but the integrity checks must read the frontmatter of the file
// actually verified — otherwise a dangling related: on the verified copy goes
// unreported because a different copy of the same stem is clean.
func TestTicketGraphIntegritySubjectIsTheVerifiedFileNotTheStem(t *testing.T) {
	dangling := []string{"related:", "  260726-nope-on-this-copy: dangling"}

	t.Run("dangling on the closed copy", func(t *testing.T) {
		f := newGraphFixture(t)
		f.ticket("todo", "260726-feat-two-copies")
		closed := f.ticket(".done", "260726-feat-two-copies", dangling...)

		advisory := onlyAdvisory(t, f.verify(closed), AdvisoryKindFix)
		requireContainsFlat(t, advisory.Text, "related: `260726-nope-on-this-copy` resolves to no ticket stem")
	})

	t.Run("dangling on the open copy", func(t *testing.T) {
		f := newGraphFixture(t)
		open := f.ticket("todo", "260726-feat-two-copies", dangling...)
		f.ticket(".done", "260726-feat-two-copies")

		advisory := onlyAdvisory(t, f.verify(open), AdvisoryKindFix)
		requireContainsFlat(t, advisory.Text, "related: `260726-nope-on-this-copy` resolves to no ticket stem")
	})
}

// TestTicketGraphIntegrityCapRepeatsPerVerifiedTicket is the combined shape:
// two tickets that each overflow the cap must each get their own five plus
// their own overflow line. This is the assertion that would have caught the
// per-call cap directly.
func TestTicketGraphIntegrityCapRepeatsPerVerifiedTicket(t *testing.T) {
	f := newGraphFixture(t)
	related := []string{"related:"}
	for _, suffix := range []string{"a", "b", "c", "d", "e", "f"} {
		related = append(related, "  260726-nope-"+suffix+": x")
	}
	first := f.ticket("todo", "260726-feat-overflow-one", related...)
	second := f.ticket("todo", "260726-feat-overflow-two", related...)

	result := f.verify(first, second)
	if len(result.Advisories) != 12 {
		t.Fatalf("Advisories = %v, want 5 capped plus 1 overflow for each of two tickets", advisoryKinds(result.Advisories))
	}
	overflows := 0
	for _, advisory := range result.Advisories {
		if advisory.Text == "... +1 more" {
			overflows++
		}
	}
	if overflows != 2 {
		t.Fatalf("overflow lines = %d, want one per verified ticket:\n%#v", overflows, result.Advisories)
	}
}

// TestTicketGraphIgnoresDuplicateStemAcrossStatusDirs covers an abnormal board
// (git mv is atomic, so this should not occur): the same stem in two status
// directories must not duplicate its row, inflate the child count, or let a
// closure nudge fire while an open copy still exists.
func TestTicketGraphIgnoresDuplicateStemAcrossStatusDirs(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-duplicated")
	parent := "parent: 260726-epic-duplicated"
	f.ticket("todo", "260726-feat-two-places", parent)
	f.ticket(".done", "260726-feat-two-places", parent)
	verified := f.ticket(".done", "260726-feat-other", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	if got := strings.Count(board, "260726-feat-two-places"); got != 1 {
		t.Fatalf("duplicated stem rendered %d rows, want 1:\n%s", got, board)
	}
	// The open copy wins, so the epic reads as still in flight rather than
	// attracting a false closure ACTION.
	requireContains(t, board, "Parent [1]: 260726-epic-duplicated [todo] - 1 of 2 child tickets still open")
	requireNotContains(t, board, "ACTION:")
}

// TestTicketGraphOmitsChainEndClaimWhenAncestorParentDangles pins that the
// board never claims a chain ended when the walk was cut. The dangling edge
// belongs to an ancestor, and integrity checks never inspect ancestors by
// design, so nothing else in the output would mention it.
func TestTicketGraphOmitsChainEndClaimWhenAncestorParentDangles(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-mid-dangling", "parent: 260726-epic-never-existed")
	verified := f.ticket(".done", "260726-feat-under-dangling", "parent: 260726-epic-mid-dangling")

	result := f.verify(verified)
	board := boardAdvisoryText(t, result)
	requireContains(t, board, "Parent [1]: 260726-epic-mid-dangling [todo] - all 1 child ticket closed")
	requireNotContains(t, board, "No further ancestors.")

	// The ancestor's own dangling parent: stays that ancestor's problem.
	for _, advisory := range result.Advisories {
		if advisory.Kind == AdvisoryKindFix {
			t.Fatalf("an ancestor's frontmatter was checked: %#v", advisory)
		}
	}
}

// TestTicketGraphChainEndClaimIsPerChain pins that one verified ticket's
// truncated chain cannot suppress the closing claim on another's chain that
// demonstrably ended. The two chains are independent, so the line must track
// the ancestor that terminates its own chain rather than the verify call.
func TestTicketGraphChainEndClaimIsPerChain(t *testing.T) {
	f := newGraphFixture(t)
	// Chain A ends cleanly.
	f.ticket("todo", "260726-epic-complete-top")
	completeChild := f.ticket(".done", "260726-feat-under-complete", "parent: 260726-epic-complete-top")
	// Chain B is cut by an ancestor whose parent: does not resolve.
	f.ticket("todo", "260726-epic-cut-mid", "parent: 260726-epic-never-existed")
	cutChild := f.ticket(".done", "260726-feat-under-cut", "parent: 260726-epic-cut-mid")

	alone := boardAdvisoryText(t, f.verify(completeChild))
	if got := strings.Count(alone, "No further ancestors."); got != 1 {
		t.Fatalf("a complete chain verified alone claimed the end %d times, want 1:\n%s", got, alone)
	}

	together := boardAdvisoryText(t, f.verify(completeChild, cutChild))
	if got := strings.Count(together, "No further ancestors."); got != 1 {
		t.Fatalf("chain-end claims = %d, want exactly one (the complete chain keeps it, the cut chain does not):\n%s", got, together)
	}
	completeAt := strings.Index(together, "260726-epic-complete-top")
	cutAt := strings.Index(together, "260726-epic-cut-mid")
	claimAt := strings.Index(together, "No further ancestors.")
	if completeAt < 0 || cutAt < 0 {
		t.Fatalf("both ancestors must render:\n%s", together)
	}
	if !(completeAt < claimAt && claimAt < cutAt) {
		t.Fatalf("the chain-end claim must sit with the complete chain, not the cut one:\n%s", together)
	}
}

func TestTicketGraphSingleChildEpicHeaderIsSingular(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-solo")
	verified := f.ticket(".done", "260726-feat-solo-child", "parent: 260726-epic-solo")

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "- all 1 child ticket closed")
	requireNotContains(t, board, "1 child tickets")
}

// TestTicketGraphMultiLevelChainCrossReferencesClosableEpic is the only
// depth>1 fixture: it exercises the unbounded walk past one hop, the
// Parent [2] depth label, the plain (epic) parenthetical, and the
// closableAbove cross-reference that turns a nearer ancestor's ACTION line
// into "(epic, closable - see above)" on a farther ancestor's row.
func TestTicketGraphMultiLevelChainCrossReferencesClosableEpic(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-top")
	top := "parent: 260726-epic-top"
	f.ticket("todo", "260726-epic-mid", top)
	f.ticket("todo", "260726-epic-unrelated", top)
	f.ticket("todo", "260726-chore-other", top)
	f.ticket("idea", "260726-research-someday", top)

	mid := "parent: 260726-epic-mid"
	f.ticket(".done", "260726-bug-first", mid)
	verified := f.ticket(".done", "260726-feat-last", mid)

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "Parent [1]: 260726-epic-mid [todo] - all 2 child tickets closed")
	requireContains(t, board, "Parent [2]: 260726-epic-top [todo] - 4 of 4 child tickets still open")
	requireContains(t, board, "    todo    | 260726-epic-mid  (epic, closable - see above)")
	requireContains(t, board, "    todo    | 260726-epic-unrelated  (epic)")
	requireContains(t, board, "    idea    | 260726-research-someday")
	if strings.Index(board, "Parent [1]:") > strings.Index(board, "Parent [2]:") {
		t.Fatalf("ancestors must render nearest first:\n%s", board)
	}
}

// TestTicketGraphOverflowLineDropsOpenForClosedInclusiveTier pins the wording
// the plan flagged as a judgment call the ticket does not exemplify: where
// hidden rows may be closed, the overflow line omits the word "open" because
// it would be false. It also covers the .done -> .dropped closed sort order
// and the .dropped row that motivates the 8-wide status column.
func TestTicketGraphOverflowLineDropsOpenForClosedInclusiveTier(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-mixed-closed")
	parent := "parent: 260726-epic-mixed-closed"
	verified := f.ticket(".done", "260726-feat-done-a", parent)
	for _, stem := range []string{"260726-feat-done-b", "260726-feat-done-c", "260726-feat-done-d"} {
		f.ticket(".done", stem, parent)
	}
	f.ticket(".dropped", "260726-feat-dropped-a", parent)
	f.ticket(".dropped", "260726-feat-dropped-b", parent)
	f.ticket("idea", "260726-feat-idea-a", parent)
	f.ticket("idea", "260726-feat-idea-b", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "Parent [1]: 260726-epic-mixed-closed [todo] - 6 of 8 closed, 2 idea/ remaining")
	requireContains(t, board, "    .dropped| 260726-feat-dropped-a")
	requireContains(t, board, "    ... +3 more (1 .dropped, 2 idea)")
	requireNotContains(t, board, "more open (")

	if strings.Index(board, "260726-feat-done-d") > strings.Index(board, "260726-feat-dropped-a") {
		t.Fatalf("closed rows must sort .done before .dropped:\n%s", board)
	}
}

func TestTicketGraphSiblingListingGatesOnDroppedPath(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-dropped-gate")
	parent := "parent: 260726-epic-dropped-gate"
	verified := f.ticket(".dropped", "260726-feat-abandoned", parent)
	f.ticket("todo", "260726-feat-open-one", parent)
	f.ticket("idea", "260726-feat-open-two", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "Parent [1]: 260726-epic-dropped-gate [todo] - 2 of 3 child tickets still open")
	requireContains(t, board, "    todo    | 260726-feat-open-one")
	requireContains(t, board, "    idea    | 260726-feat-open-two")
}

// TestTicketGraphActionFiresOnOpenPathWithoutJustNow separates two rules that
// every other ACTION fixture leaves co-varying: ACTION lines fire regardless of
// path gating (only the sibling listing is gated), and "(just now)" attaches to
// closed rows only — on an open row it would read as a closure that did not
// happen.
func TestTicketGraphActionFiresOnOpenPathWithoutJustNow(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-open-path-action")
	parent := "parent: 260726-epic-open-path-action"
	f.ticket(".done", "260726-feat-accepted-landed", parent)
	verified := f.ticket("idea", "260726-feat-deferred-idea", parent)

	board := boardAdvisoryText(t, f.verify(verified))
	requireContains(t, board, "Parent [1]: 260726-epic-open-path-action [todo] - 1 of 2 closed, 1 idea/ remaining")
	requireContainsFlat(t, board, "ACTION: Every accepted child has landed; only idea/ children remain.")
	requireContains(t, board, "    idea    | 260726-feat-deferred-idea")
	requireNotContains(t, board, "just now")
}

// TestTicketGraphIntegrityIgnoresAncestorFrontmatter pins the settled subject
// set: integrity checks read the verified ticket's own frontmatter and never an
// ancestor's. A dangling related: on an ancestor is that ancestor's problem,
// reported when a commit touches it.
func TestTicketGraphIntegrityIgnoresAncestorFrontmatter(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-dirty-frontmatter",
		"related:",
		"  260726-nope-on-the-ancestor: dangling on the parent")
	verified := f.ticket(".done", "260726-feat-clean-child", "parent: 260726-epic-dirty-frontmatter")

	result := f.verify(verified)
	requireContains(t, boardAdvisoryText(t, result), "Parent [1]: 260726-epic-dirty-frontmatter")
	for _, advisory := range result.Advisories {
		if advisory.Kind != AdvisoryKindBoard {
			t.Fatalf("an ancestor's frontmatter produced an advisory: %#v", advisory)
		}
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

// TestTicketGraphEpicWithoutChildrenEmitsNothing guards a regression, not a
// branch. Childlessness is structurally unreachable by this pass: a verified
// ticket's own children are never rendered, only its ancestors are, so adding
// children to this epic changes nothing. What the test actually asserts is
// that verifying an epic on an ungated todo/ path stays silent, which is what
// a future "childless epic -> advisory" check would break. The epic is given a
// parent so the pass genuinely runs rather than returning on an empty chain
// (which would make this a duplicate of the no-parent case), and the closed
// sibling is a positive control proving the fixture is live rather than inert.
func TestTicketGraphEpicWithoutChildrenEmitsNothing(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260726-epic-umbrella")
	childless := f.ticket("todo", "260726-epic-pre-decomposition", "parent: 260726-epic-umbrella")
	sibling := f.ticket(".done", "260726-feat-umbrella-sibling", "parent: 260726-epic-umbrella")

	requireNoAdvisories(t, f.verify(childless))

	// Positive control: the same board does produce a block, so the assertion
	// above is about the childless epic and not about an inert fixture.
	board := boardAdvisoryText(t, f.verify(sibling))
	requireContains(t, board, "Parent [1]: 260726-epic-umbrella [todo] - 1 of 2 child tickets still open")
	requireContains(t, board, "    todo    | 260726-epic-pre-decomposition  (epic)")
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
