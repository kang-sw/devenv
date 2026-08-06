package wsdoc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Every test here runs real git in a plain t.TempDir() repository. A fake
// runner or a stub board would encode our belief about git's sparse-checkout
// behavior rather than test it, and the residual risk this feature carries is
// exactly the unreproduced "hides too much" hazard, which only real git can
// exhibit. A plain `git init` repo reproduces every property the production
// gate reads (the gate resolves GIT_DIR generically), so no linked worktree is
// constructed.

// scope commits the fixture board and applies a --no-cone sparse-checkout that
// removes every ticket under excludeDirs except the board-relative paths in
// keep.
//
// It then asserts the hide actually happened, in both directions: every
// intended-hidden path is off disk AND every other tracked path is still on
// disk. The failure direction of the hazard is "hides too much", so without
// the second half a fixture that hid the whole board would let the resolution
// tests pass vacuously.
func (f *graphFixture) scope(excludeDirs []string, keep ...string) {
	f.t.Helper()
	runGit(f.t, f.root, "init", "-q")
	runGit(f.t, f.root, "config", "user.email", "test@example.com")
	runGit(f.t, f.root, "config", "user.name", "Test User")
	runGit(f.t, f.root, "add", "-A")
	runGit(f.t, f.root, "commit", "-q", "-m", "board")

	args := []string{"sparse-checkout", "set", "--no-cone", "/*"}
	for _, dir := range excludeDirs {
		args = append(args, "!/ai-docs/tickets/"+dir+"/*")
	}
	kept := map[string]bool{}
	for _, rel := range keep {
		kept[rel] = true
		args = append(args, "/"+rel)
	}
	runGit(f.t, f.root, args...)

	excluded := map[string]bool{}
	for _, dir := range excludeDirs {
		excluded[dir] = true
	}
	for _, rel := range f.trackedPaths() {
		status, _, isTicket := ticketIndexPathParts(rel)
		wantHidden := isTicket && excluded[status] && !kept[rel]
		_, err := os.Stat(filepath.Join(f.root, filepath.FromSlash(rel)))
		if wantHidden && err == nil {
			f.t.Fatalf("fixture: %s should have been hidden by the scope but is on disk", rel)
		}
		if !wantHidden && err != nil {
			f.t.Fatalf("fixture: %s should have stayed on disk but is missing (%v) - the scope hides too much", rel, err)
		}
	}
}

func (f *graphFixture) trackedPaths() []string {
	f.t.Helper()
	return splitNULPaths(gitOutput(f.t, f.root, "ls-files", "-z"))
}

func gitOutput(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s failed: %v", strings.Join(args, " "), err)
	}
	return out
}

// execGitRunner is the real-exec GitRunner the move tests need: mockGitRunner
// fakes `git mv` with os.Rename and can therefore never reproduce a sparse
// refusal. It mirrors wsgit.ExecRunner's CombinedOutput shape, which wsdoc
// cannot import.
type execGitRunner struct{}

func (execGitRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// --- F1: hidden related: target -------------------------------------------

func TestTicketGraphResolvesHiddenRelatedTargetUnderScope(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260101-feat-visible",
		"related:",
		"  260102-feat-hidden: in the index, not in this worktree",
		"  260199-feat-absent: nowhere at all",
	)
	f.ticket("todo", "260102-feat-hidden")
	f.scope([]string{"todo"}, "ai-docs/tickets/todo/260101-feat-visible.md")

	result := f.verify("ai-docs/tickets/todo/260101-feat-visible.md")
	var texts []string
	for _, advisory := range result.Advisories {
		texts = append(texts, advisory.Text)
	}
	text := strings.Join(texts, "\n")

	requireNotContains(t, text, "260102-feat-hidden")
	// Live control: "no FIX for the hidden stem" alone cannot distinguish a
	// correctly resolved hidden stem from an integrity check that silently
	// stopped running over a partial board.
	requireContainsFlat(t, text, "related: `260199-feat-absent` resolves to no ticket stem and no spec anchor.")
}

// --- F2: epic with a hidden open child -------------------------------------

func TestTicketGraphSeesHiddenOpenChildUnderScope(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("ready", "260100-epic-x")
	f.ticket(".done", "260101-feat-a", "parent: 260100-epic-x")
	f.ticket("todo", "260102-feat-b", "parent: 260100-epic-x")
	f.scope([]string{"todo"})

	board := boardAdvisoryText(t, f.verify("ai-docs/tickets/.done/260101-feat-a.md"))
	requireContains(t, board, "1 of 2 child tickets still open")
	requireContains(t, board, "todo    | 260102-feat-b")
	requireNotContains(t, board, actionAllChildrenClosed)
}

// TestTicketGraphAllChildrenClosedStillFiresUnderScope is F2's inversion
// control: with the second child genuinely gone from index and disk, the same
// board must emit the closure ACTION. Together the two tests pin the exact
// inversion rather than just the absence of an advisory.
func TestTicketGraphAllChildrenClosedStillFiresUnderScope(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("ready", "260100-epic-x")
	f.ticket(".done", "260101-feat-a", "parent: 260100-epic-x")
	f.scope([]string{"todo"})

	board := boardAdvisoryText(t, f.verify("ai-docs/tickets/.done/260101-feat-a.md"))
	requireContains(t, board, "all 1 child ticket closed")
	requireContainsFlat(t, board, "Check whether this epic can be closed.")
}

// --- F3: blocked mutations name the scope ----------------------------------

func TestTicketsMoveBlockedByScopeNamesTheScope(t *testing.T) {
	f := newGraphFixture(t)
	// A research stem is used for the destination-blocked case because it is
	// exempt from both sage-review stages, so TicketsMove performs no
	// frontmatter write before the move and the no-op claim can be asserted
	// with `git status --porcelain`.
	f.ticket("idea", "260103-research-c")
	f.ticket("todo", "260104-feat-hidden")
	f.ticket("todo", "260105-feat-kept")
	f.scope([]string{"todo"}, "ai-docs/tickets/todo/260105-feat-kept.md")

	t.Run("hidden destination", func(t *testing.T) {
		_, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260103-research-c", To: "todo"})
		if err == nil {
			t.Fatal("TicketsMove into a hidden status succeeded, want a scope error")
		}
		requireContains(t, err.Error(), "outside this worktree's sparse-checkout scope (core.sparseCheckout)")
		requireContains(t, err.Error(), "ai-docs/tickets/todo/260103-research-c.md")
		requireContains(t, err.Error(), "git sparse-checkout add")
		// git's own advice text is gettext-localized and must never be relayed.
		requireNotContains(t, err.Error(), "outside of your sparse-checkout definition")
		if out := gitOutput(t, f.root, "status", "--porcelain"); len(strings.TrimSpace(string(out))) != 0 {
			t.Fatalf("blocked move was not a no-op:\n%s", out)
		}
	})

	t.Run("hidden source", func(t *testing.T) {
		_, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260104-feat-hidden", To: "ready"})
		if err == nil {
			t.Fatal("TicketsMove of a hidden source succeeded, want a scope error")
		}
		requireContains(t, err.Error(), "source ticket ai-docs/tickets/todo/260104-feat-hidden.md")
		requireContains(t, err.Error(), "outside this worktree's sparse-checkout scope (core.sparseCheckout)")
		if out := gitOutput(t, f.root, "status", "--porcelain"); len(strings.TrimSpace(string(out))) != 0 {
			t.Fatalf("blocked move was not a no-op:\n%s", out)
		}
	})

	t.Run("absent stem still reports not found", func(t *testing.T) {
		_, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260199-feat-nope", To: "todo"})
		if err == nil || !strings.Contains(err.Error(), "ticket not found: 260199-feat-nope") {
			t.Fatalf("err = %v, want ticket not found", err)
		}
	})

	// Positive control: an in-scope destination must still move, or the
	// pre-flight would be indistinguishable from blocking every mutation once a
	// scope is active.
	t.Run("in-scope move still succeeds", func(t *testing.T) {
		result, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260105-feat-kept", To: "ready"})
		if err != nil {
			t.Fatalf("TicketsMove returned error: %v", err)
		}
		if result.NewPath != "ai-docs/tickets/ready/260105-feat-kept.md" {
			t.Fatalf("NewPath = %q", result.NewPath)
		}
		if _, err := os.Stat(filepath.Join(f.root, "ai-docs/tickets/ready/260105-feat-kept.md")); err != nil {
			t.Fatalf("moved ticket missing on disk: %v", err)
		}
	})
}

func TestTicketsCloseHiddenSourceNamesTheScope(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260104-feat-hidden")
	f.ticket("todo", "260105-feat-kept")
	f.scope([]string{"todo"}, "ai-docs/tickets/todo/260105-feat-kept.md")

	_, err := TicketsClose(f.root, execGitRunner{}, TicketCloseOptions{TicketStem: "260104-feat-hidden", Status: "done", Today: "2026-08-06"})
	if err == nil {
		t.Fatal("TicketsClose of a hidden source succeeded, want a scope error")
	}
	requireContains(t, err.Error(), "source ticket ai-docs/tickets/todo/260104-feat-hidden.md")
	requireContains(t, err.Error(), "outside this worktree's sparse-checkout scope (core.sparseCheckout)")
	if out := gitOutput(t, f.root, "status", "--porcelain"); len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("blocked close was not a no-op:\n%s", out)
	}
}

// TestTicketsMoveWrapsRawErrorWhenCheckRulesUnavailable covers the git < 2.42
// backstop: includes fails open, so the refusal must arrive as the wrapped
// post-hoc message instead. The case is probed rather than version-asserted -
// the question is whether the subcommand exists, which is what the production
// fail-open branch keys on.
func TestTicketsMoveWrapsRawErrorWhenCheckRulesUnavailable(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("idea", "260103-research-c")
	f.ticket("todo", "260104-feat-hidden")
	f.scope([]string{"todo"})

	scope := newTicketScope(f.root)
	if scope == nil {
		t.Fatal("newTicketScope returned nil inside a scoped worktree")
	}
	if _, err := scope.run("", "sparse-checkout", "check-rules"); err == nil {
		t.Skip("git supports sparse-checkout check-rules; the fail-open path is unreachable here")
	}

	_, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260103-research-c", To: "todo"})
	if err == nil {
		t.Fatal("TicketsMove into a hidden status succeeded, want an error")
	}
	requireContains(t, err.Error(), "a sparse-checkout scope is active in this worktree (core.sparseCheckout)")
	requireContains(t, err.Error(), "git sparse-checkout add")
}

// --- Resolution surfaces ---------------------------------------------------

func TestTicketsStatusAndFindResolveHiddenTickets(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260101-feat-visible")
	f.ticketWithBody("todo", "260102-feat-hidden", "Body mentioning 260505-spec-sasquatch.\n")
	f.scope([]string{"todo"}, "ai-docs/tickets/todo/260101-feat-visible.md")

	t.Run("status resolves", func(t *testing.T) {
		info, err := TicketsStatus(f.root, TicketStatusOptions{TicketStem: "260102-feat-hidden", Resolve: true})
		if err != nil {
			t.Fatalf("TicketsStatus returned error: %v", err)
		}
		if !info.Hidden {
			t.Fatalf("Hidden = false, want true: %#v", info)
		}
		if info.Status != "todo" || info.Path != "ai-docs/tickets/todo/260102-feat-hidden.md" {
			t.Fatalf("unexpected ticket: %#v", info)
		}
		if info.Title != "260102-feat-hidden" {
			t.Fatalf("Title = %q, want the frontmatter title parsed from the index blob", info.Title)
		}
	})

	t.Run("status without resolve stays absent", func(t *testing.T) {
		if _, err := TicketsStatus(f.root, TicketStatusOptions{TicketStem: "260102-feat-hidden"}); err == nil {
			t.Fatal("discovery-mode TicketsStatus found a hidden ticket, want ticket not found")
		}
	})

	// The index-body branch is what references.trace's spec branch depends on:
	// the query matches text that exists only inside the hidden ticket's body.
	t.Run("find resolves bodies", func(t *testing.T) {
		tickets, err := TicketsFind(f.root, TicketFindOptions{Query: "260505-spec-sasquatch", Resolve: true})
		if err != nil {
			t.Fatalf("TicketsFind returned error: %v", err)
		}
		if len(tickets) != 1 || tickets[0].Stem != "260102-feat-hidden" || !tickets[0].Hidden {
			t.Fatalf("unexpected results: %#v", tickets)
		}
	})

	t.Run("find without resolve stays discovery", func(t *testing.T) {
		tickets, err := TicketsFind(f.root, TicketFindOptions{Query: "260505-spec-sasquatch"})
		if err != nil {
			t.Fatalf("TicketsFind returned error: %v", err)
		}
		if len(tickets) != 0 {
			t.Fatalf("discovery-mode TicketsFind matched a hidden body: %#v", tickets)
		}
	})

	t.Run("scope reports the hidden count", func(t *testing.T) {
		info, err := TicketScope(f.root, nil)
		if err != nil {
			t.Fatalf("TicketScope returned error: %v", err)
		}
		if !info.Active || info.Hidden != 1 || len(info.HiddenStems) != 1 || info.HiddenStems[0] != "260102-feat-hidden" {
			t.Fatalf("TicketScope = %#v", info)
		}
	})
}

// --- Gate no-op ------------------------------------------------------------

func TestTicketScopeGateIsInertWithoutSparseCheckout(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260101-feat-a")
	f.ticket("idea", "260102-feat-b", "related:", "  260199-feat-absent: nowhere")

	assertUnchanged := func(label string) {
		t.Helper()
		list, err := TicketsList(f.root, TicketListOptions{})
		if err != nil {
			t.Fatalf("%s: TicketsList returned error: %v", label, err)
		}
		if len(list) != 2 {
			t.Fatalf("%s: TicketsList = %#v", label, list)
		}
		for _, ticket := range list {
			if ticket.Hidden {
				t.Fatalf("%s: ticket marked hidden without a scope: %#v", label, ticket)
			}
		}
		found, err := TicketsFind(f.root, TicketFindOptions{Query: "260101-feat-a", Resolve: true})
		if err != nil || len(found) != 1 {
			t.Fatalf("%s: TicketsFind = %#v, err = %v", label, found, err)
		}
		info, err := TicketsStatus(f.root, TicketStatusOptions{TicketStem: "260101-feat-a", Resolve: true})
		if err != nil || info.Hidden {
			t.Fatalf("%s: TicketsStatus = %#v, err = %v", label, info, err)
		}
		if _, err := TicketsStatus(f.root, TicketStatusOptions{TicketStem: "260199-feat-absent", Resolve: true}); err == nil {
			t.Fatalf("%s: an absent stem resolved", label)
		}
		result := f.verify("ai-docs/tickets/idea/260102-feat-b.md")
		requireContainsFlat(t, strings.Join([]string{result.Advisories[0].Text}, ""), "related: `260199-feat-absent` resolves to no ticket stem")
		scope, err := TicketScope(f.root, nil)
		if err != nil {
			t.Fatalf("%s: TicketScope returned error: %v", label, err)
		}
		if scope.Active || scope.Hidden != 0 {
			t.Fatalf("%s: TicketScope = %#v, want inactive", label, scope)
		}
	}

	assertUnchanged("not a git repository")

	runGit(t, f.root, "init", "-q")
	runGit(t, f.root, "config", "user.email", "test@example.com")
	runGit(t, f.root, "config", "user.name", "Test User")
	runGit(t, f.root, "add", "-A")
	runGit(t, f.root, "commit", "-q", "-m", "board")
	assertUnchanged("git repository without sparse-checkout")
}

// --- TicketCreate collision -------------------------------------------------

func TestTicketCreateRejectsHiddenCollision(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260101-feat-visible")
	f.ticket("todo", "260102-feat-hidden")
	f.scope([]string{"todo"}, "ai-docs/tickets/todo/260101-feat-visible.md")

	_, err := TicketCreate(f.root, TicketCreateOptions{Stem: "feat-hidden", InitialState: "todo", Today: "260102"})
	if err == nil {
		t.Fatal("TicketCreate overwrote a ticket hidden by the scope")
	}
	requireContains(t, err.Error(), "outside this worktree's sparse-checkout scope (core.sparseCheckout)")
	requireContains(t, err.Error(), "ai-docs/tickets/todo/260102-feat-hidden.md")

	// A stem the index does not carry is still creatable.
	if _, err := TicketCreate(f.root, TicketCreateOptions{Stem: "feat-brand-new", InitialState: "idea", Today: "260102"}); err != nil {
		t.Fatalf("TicketCreate of an uncollided stem returned error: %v", err)
	}
}

// --- Fully excluded status directories --------------------------------------

func TestScanTicketsSurvivesFullyExcludedDirectories(t *testing.T) {
	t.Run("one status directory", func(t *testing.T) {
		f := newGraphFixture(t)
		f.ticket("ready", "260100-feat-r")
		f.ticket("todo", "260101-feat-a")
		f.ticket("todo", "260102-feat-b")
		f.scope([]string{"todo"})

		if _, err := os.Stat(filepath.Join(f.root, "ai-docs/tickets/todo")); !os.IsNotExist(err) {
			t.Fatalf("fixture: todo/ should have vanished from disk, stat err = %v", err)
		}
		// resolveOff: the discovery path must neither error nor lose the
		// statuses that are still on disk.
		list, err := TicketsList(f.root, TicketListOptions{})
		if err != nil {
			t.Fatalf("TicketsList returned error: %v", err)
		}
		if len(list) != 1 || list[0].Stem != "260100-feat-r" {
			t.Fatalf("TicketsList = %#v", list)
		}
		// resolveGraph: the whole board is still reachable.
		graph, err := scanTickets(f.root, ticketScanOptions{Resolve: resolveGraph})
		if err != nil {
			t.Fatalf("scanTickets(resolveGraph) returned error: %v", err)
		}
		if len(graph) != 3 {
			t.Fatalf("scanTickets(resolveGraph) = %#v, want 3 tickets", graph)
		}
	})

	t.Run("whole board", func(t *testing.T) {
		f := newGraphFixture(t)
		f.ticket("ready", "260100-feat-r")
		f.ticket("todo", "260101-feat-a")
		f.scope([]string{"ready", "todo"})

		if _, err := os.Stat(filepath.Join(f.root, "ai-docs/tickets")); !os.IsNotExist(err) {
			t.Fatalf("fixture: ai-docs/tickets/ should have vanished from disk, stat err = %v", err)
		}
		info, err := TicketsStatus(f.root, TicketStatusOptions{TicketStem: "260101-feat-a", Resolve: true})
		if err != nil {
			t.Fatalf("TicketsStatus returned error: %v", err)
		}
		if !info.Hidden || info.Status != "todo" {
			t.Fatalf("unexpected ticket: %#v", info)
		}
	})
}
