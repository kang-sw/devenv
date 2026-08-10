package wsdoc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	f.commitBoard()

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

// commitBoard makes the fixture a git repository with the board committed and
// no sparse-checkout applied. It is the filter-off baseline the scope-gated
// behavior must stay byte-identical to.
func (f *graphFixture) commitBoard() {
	f.t.Helper()
	runGit(f.t, f.root, "init", "-q")
	runGit(f.t, f.root, "config", "user.email", "test@example.com")
	runGit(f.t, f.root, "config", "user.name", "Test User")
	runGit(f.t, f.root, "add", "-A")
	runGit(f.t, f.root, "commit", "-q", "-m", "board")
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

// TestScopeBlockedMutationIsANoOpOnSageBearingTicket covers the ticket's
// declared hot path, where the no-op claim is load-bearing rather than
// incidental: a `bug` stem carries both sage-review stages, so TicketsMove's
// prepareSageReviewForUpwardMove persists frontmatter on an upward move. If the
// destination pre-flight ran after that write, the call would report a refusal
// while leaving the working tree dirty. The research-stem case in
// TestTicketsMoveBlockedByScopeNamesTheScope cannot catch that: it is exempt
// from both stages, so it never writes at all.
func TestScopeBlockedMutationIsANoOpOnSageBearingTicket(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("idea", "260101-bug-capture")
	f.ticket("todo", "260105-feat-kept")
	f.scope([]string{"todo"}, "ai-docs/tickets/todo/260105-feat-kept.md")

	before := readFileString(t, filepath.Join(f.root, "ai-docs/tickets/idea/260101-bug-capture.md"))

	_, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260101-bug-capture", To: "todo"})
	if err == nil {
		t.Fatal("TicketsMove into a hidden status succeeded, want a scope error")
	}
	requireContains(t, err.Error(), "outside this worktree's sparse-checkout scope (core.sparseCheckout)")
	if out := gitOutput(t, f.root, "status", "--porcelain"); len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("blocked move of a sage-bearing ticket was not a no-op:\n%s", out)
	}
	if after := readFileString(t, filepath.Join(f.root, "ai-docs/tickets/idea/260101-bug-capture.md")); after != before {
		t.Fatalf("blocked move rewrote the source frontmatter:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}

// TestTicketsCloseBlockedByScopeThenWidenedRetryIsClean is the other half of
// the same property: TicketsClose's pre-move writes are non-idempotent
// (appendResolution appends unconditionally), so a refusal landing after them
// would make the error's own widen-then-retry remedy corrupt the ticket with a
// second ## Resolution section.
func TestTicketsCloseBlockedByScopeThenWidenedRetryIsClean(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260105-feat-kept")
	// .done/ is excluded, so the close destination is out of scope while the
	// source stays checked out.
	f.scope([]string{".done"}, "ai-docs/tickets/todo/260105-feat-kept.md")

	source := filepath.Join(f.root, "ai-docs/tickets/todo/260105-feat-kept.md")
	before := readFileString(t, source)

	_, err := TicketsClose(f.root, execGitRunner{}, TicketCloseOptions{
		TicketStem: "260105-feat-kept", Status: "done", Today: "2026-08-06", Resolution: "Closed for the fixture.",
	})
	if err == nil {
		t.Fatal("TicketsClose into a hidden status succeeded, want a scope error")
	}
	requireContains(t, err.Error(), "destination path ai-docs/tickets/.done/260105-feat-kept.md")
	requireContains(t, err.Error(), "outside this worktree's sparse-checkout scope (core.sparseCheckout)")
	if out := gitOutput(t, f.root, "status", "--porcelain"); len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("blocked close was not a no-op:\n%s", out)
	}
	if after := readFileString(t, source); after != before {
		t.Fatalf("blocked close rewrote the source:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}

	// Follow the remedy the error itself gives, then retry.
	runGit(t, f.root, "sparse-checkout", "disable")
	result, err := TicketsClose(f.root, execGitRunner{}, TicketCloseOptions{
		TicketStem: "260105-feat-kept", Status: "done", Today: "2026-08-06", Resolution: "Closed for the fixture.",
	})
	if err != nil {
		t.Fatalf("TicketsClose after widening the scope returned error: %v", err)
	}
	closed := readFileString(t, filepath.Join(f.root, filepath.FromSlash(result.NewPath)))
	if count := strings.Count(closed, "## Resolution"); count != 1 {
		t.Fatalf("retry produced %d ## Resolution sections, want exactly 1:\n%s", count, closed)
	}
	if count := strings.Count(closed, "completed: 2026-08-06"); count != 1 {
		t.Fatalf("retry produced %d completed: fields, want exactly 1:\n%s", count, closed)
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

// blockStatusDir occupies a status directory with a regular file, so
// atomicGitMove's MkdirAll fails and the mutation reaches its post-write failure
// branch on every git version. The real trigger for that branch is a cross-scope
// `git mv` on git < 2.42, where the destination pre-flight fails open — which is
// unreachable on any host whose git has check-rules, i.e. the host this suite
// runs on. The failure mode is deliberately not a sparse one: what these tests
// pin is which failures carry a PartialMutationNotice, not how the error reads.
func blockStatusDir(t *testing.T, root, status string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "ai-docs", "tickets", status), []byte("occupied\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGitMoveFailureCarriesNoNoticeWithoutAScope pins the filter-off half of the
// ticket's byte-identical constraint on the one path this feature added a return
// value to: with no sparse-checkout at all, a failing git move must return the
// same empty result it returned before this branch, so no caller renders a
// `partial-mutation:` line it never rendered before.
func TestGitMoveFailureCarriesNoNoticeWithoutAScope(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260101-feat-a", "sage-review-design: skipped", "sage-review-completeness: skipped")
	f.commitBoard()

	blockStatusDir(t, f.root, ".done")
	result, err := TicketsClose(f.root, execGitRunner{}, TicketCloseOptions{
		TicketStem: "260101-feat-a", Status: "done", Today: "2026-08-06", Resolution: "Closed for the fixture.",
	})
	if err == nil {
		t.Fatal("TicketsClose succeeded with an occupied destination directory")
	}
	if result.PartialMutationNotice != "" {
		t.Fatalf("unscoped close reported PartialMutationNotice = %q, want empty (filter-off behavior must not change)", result.PartialMutationNotice)
	}
	if strings.Contains(err.Error(), "sparse-checkout scope is active") {
		t.Fatalf("unscoped close error mentions a scope: %v", err)
	}

	blockStatusDir(t, f.root, "ready")
	moved, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260101-feat-a", To: "ready"})
	if err == nil {
		t.Fatal("TicketsMove succeeded with an occupied destination directory")
	}
	if moved.PartialMutationNotice != "" {
		t.Fatalf("unscoped move reported PartialMutationNotice = %q, want empty (filter-off behavior must not change)", moved.PartialMutationNotice)
	}
	if strings.Contains(err.Error(), "sparse-checkout scope is active") {
		t.Fatalf("unscoped move error mentions a scope: %v", err)
	}
}

// TestScopedGitMoveFailureNoticeAssertsOnlyThisCallsWrites covers the other
// direction: under an active scope the residual write-then-reject window must be
// reported, but only for the writes this call actually made. The move notice
// used to be derived from the postures read back off disk, so it fired for a
// ticket that merely carried postures; it is now gated on the file really
// differing from its pre-call bytes.
func TestScopedGitMoveFailureNoticeAssertsOnlyThisCallsWrites(t *testing.T) {
	t.Run("close reports its non-idempotent writes", func(t *testing.T) {
		f := newGraphFixture(t)
		f.ticket("todo", "260101-feat-a")
		f.ticket("idea", "260102-feat-hidden")
		f.scope([]string{"idea"})
		blockStatusDir(t, f.root, ".done")

		result, err := TicketsClose(f.root, execGitRunner{}, TicketCloseOptions{
			TicketStem: "260101-feat-a", Status: "done", Today: "2026-08-06", Resolution: "Closed for the fixture.",
		})
		if err == nil {
			t.Fatal("TicketsClose succeeded with an occupied destination directory")
		}
		requireContains(t, result.PartialMutationNotice, "the close date was already written to ai-docs/tickets/todo/260101-feat-a.md")
		requireContains(t, result.PartialMutationNotice, "## Resolution")
	})

	t.Run("move stays silent when its re-persist changed nothing", func(t *testing.T) {
		f := newGraphFixture(t)
		// Both required stages already hold terminal values, so
		// prepareSageReviewForUpwardMove re-persists them byte-identically. The
		// file did not change, so no notice may claim it did.
		f.ticket("todo", "260101-feat-a", "sage-review-design: skipped", "sage-review-completeness: skipped")
		f.ticket("idea", "260102-feat-hidden")
		f.scope([]string{"idea"})
		before := readFileString(t, filepath.Join(f.root, "ai-docs/tickets/todo/260101-feat-a.md"))
		blockStatusDir(t, f.root, "ready")

		result, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260101-feat-a", To: "ready"})
		if err == nil {
			t.Fatal("TicketsMove succeeded with an occupied destination directory")
		}
		if after := readFileString(t, filepath.Join(f.root, "ai-docs/tickets/todo/260101-feat-a.md")); after != before {
			t.Fatalf("fixture: the re-persist was supposed to be content-identical:\n--- before ---\n%s\n--- after ---\n%s", before, after)
		}
		if result.PartialMutationNotice != "" {
			t.Fatalf("PartialMutationNotice = %q, want empty: the ticket carried postures but this call changed nothing", result.PartialMutationNotice)
		}
	})

	t.Run("move reports a frontmatter change it made", func(t *testing.T) {
		f := newGraphFixture(t)
		// No sage-review fields yet, so the same call stamps them: a real change
		// on disk, and the notice must say so.
		f.ticket("todo", "260101-feat-a")
		f.ticket("idea", "260102-feat-hidden")
		f.scope([]string{"idea"})
		blockStatusDir(t, f.root, "ready")

		result, err := TicketsMove(f.root, execGitRunner{}, TicketMoveOptions{TicketStem: "260101-feat-a", To: "ready"})
		if err == nil {
			t.Fatal("TicketsMove succeeded with an occupied destination directory")
		}
		requireContains(t, result.PartialMutationNotice, "sage review posture:")
	})
}

// TestGateDefersWhenWorktreeConfigDisagreesWithRepositoryConfig pins the
// negative fast path's stated defer-on-ambiguity posture. git honors
// $GIT_DIR/config.worktree only while extensions.worktreeConfig is set, so
// trusting that file alone reports "no scope" where git still filters the
// worktree — the inversion the index path exists to prevent, since every
// resolution surface would silently revert to a partial board and start
// emitting false dangling-reference advisories.
func TestGateDefersWhenWorktreeConfigDisagreesWithRepositoryConfig(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260101-feat-a")
	f.commitBoard()

	runGit(t, f.root, "sparse-checkout", "set", "--no-cone", "/*")
	runGit(t, f.root, "sparse-checkout", "disable")
	if _, err := os.Stat(filepath.Join(f.root, ".git", "info", "sparse-checkout")); err != nil {
		t.Skipf("this git removes the pattern file on disable (%v); the gate never reaches the config files", err)
	}
	// The divergence, exactly as an unrelated tool or a hand edit can leave it.
	runGit(t, f.root, "config", "--unset", "extensions.worktreeConfig")
	runGit(t, f.root, "config", "core.sparseCheckout", "true")

	worktreeConfig := readFileString(t, filepath.Join(f.root, ".git", "config.worktree"))
	if !strings.Contains(worktreeConfig, "sparseCheckout = false") {
		t.Skipf("this git did not leave a false core.sparseCheckout in config.worktree; nothing to disagree about:\n%s", worktreeConfig)
	}
	if resolved := strings.TrimSpace(string(gitOutput(t, f.root, "config", "--type=bool", "--get", "core.sparseCheckout"))); resolved != "true" {
		t.Fatalf("fixture: git resolves core.sparseCheckout = %q, want true", resolved)
	}

	if scope := newTicketScope(f.root); scope == nil {
		t.Fatal("the gate returned nil while git still resolves core.sparseCheckout=true; the config-file fast path must defer when the two files disagree")
	}
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
		if len(result.Advisories) == 0 {
			t.Fatalf("%s: no advisories; the dangling related: check stopped running", label)
		}
		requireContainsFlat(t, result.Advisories[0].Text, "related: `260199-feat-absent` resolves to no ticket stem")
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
	requireGateSpawnsNoGit(t, f.root, "in a repository that never enabled sparse-checkout")

	// `git sparse-checkout disable` leaves $GIT_DIR/info/sparse-checkout on
	// disk with its patterns intact and only flips core.sparseCheckout to false
	// (measured, git 2.43). A pattern-file stat alone would therefore keep the
	// gate "maybe active" forever after a restore, spawning a git config
	// process on every gated call in a repository the user believes is
	// unscoped - which is exactly the cost the byte-identical constraint
	// forbids. A nil scope here is what makes that call count zero.
	runGit(t, f.root, "sparse-checkout", "set", "--no-cone", "/*", "!/ai-docs/tickets/todo/*")
	runGit(t, f.root, "sparse-checkout", "disable")
	if _, err := os.Stat(filepath.Join(f.root, ".git", "info", "sparse-checkout")); err != nil {
		t.Skipf("this git removes the pattern file on disable (%v); the regression this pins is unreachable", err)
	}
	assertUnchanged("git repository after sparse-checkout disable")
	requireGateSpawnsNoGit(t, f.root, "after sparse-checkout disable")
}

// TestSparseCheckoutActiveMatchesTicketScopeActive pins SparseCheckoutActive
// (the #260810 lighter gate internal/mcp's git.commit dispatch uses to decide
// `--sparse` staging) against the same three states TicketScope's Active
// field already covers: no repository, a plain repository with no
// sparse-checkout, and an active sparse-checkout scope. The two must never
// disagree, since SparseCheckoutActive is defined as newTicketScope(root) !=
// nil — the identical gate TicketScope itself starts from.
func TestSparseCheckoutActiveMatchesTicketScopeActive(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260101-feat-a")

	assertMatches := func(label string) {
		t.Helper()
		scope, err := TicketScope(f.root, nil)
		if err != nil {
			t.Fatalf("%s: TicketScope returned error: %v", label, err)
		}
		if got := SparseCheckoutActive(f.root); got != scope.Active {
			t.Fatalf("%s: SparseCheckoutActive = %v, want %v (TicketScope.Active)", label, got, scope.Active)
		}
	}

	assertMatches("not a git repository")

	runGit(t, f.root, "init", "-q")
	runGit(t, f.root, "config", "user.email", "test@example.com")
	runGit(t, f.root, "config", "user.name", "Test User")
	runGit(t, f.root, "add", "-A")
	runGit(t, f.root, "commit", "-q", "-m", "board")
	assertMatches("git repository without sparse-checkout")

	runGit(t, f.root, "sparse-checkout", "set", "--no-cone", "/*", "!/ai-docs/tickets/todo/*")
	assertMatches("git repository with an active sparse-checkout scope")
}

// requireGateSpawnsNoGit is how the zero-process half of the byte-identical
// constraint is asserted without counting internal calls: PATH is replaced by a
// shim that records any invocation and fails, so a surviving `git config` spawn
// shows up as a marker file. Answering "inactive" is not enough - the gate has
// always answered that; what regresses is answering it by paying a subprocess.
func requireGateSpawnsNoGit(t *testing.T, root, label string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH shim assumes a POSIX shell")
	}
	t.Run("gate spawns no git "+label, func(t *testing.T) {
		shimDir := t.TempDir()
		marker := filepath.Join(shimDir, "invoked")
		script := "#!/bin/sh\necho called >> " + marker + "\nexit 1\n"
		if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", shimDir)

		if scope := newTicketScope(root); scope != nil {
			t.Fatalf("newTicketScope returned a scope %s", label)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("the gate executed git %s; the filter-off path must cost zero processes", label)
		}
	})
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

// --- cat-file --batch framing ----------------------------------------------

// TestParseCatFileBatchRejectsPartialDecode pins the propagate-then-swallow
// posture at the decoder. Returning what was decoded so far would hand every
// later hidden ticket an empty body, and in resolveGraph mode that silently
// drops its `parent:` - which is how the all-children-closed ACTION could fire
// on an epic whose hidden children are still open.
func TestParseCatFileBatchRejectsPartialDecode(t *testing.T) {
	requested := []string{"ai-docs/tickets/todo/260101-feat-a.md", "ai-docs/tickets/todo/260102-feat-b.md"}

	t.Run("well formed", func(t *testing.T) {
		out := []byte("aaa blob 5\nfirst\nbbb blob 6\nsecond\n")
		bodies, err := parseCatFileBatch(out, requested)
		if err != nil {
			t.Fatalf("parseCatFileBatch returned error: %v", err)
		}
		if bodies[requested[0]] != "first" || bodies[requested[1]] != "second" {
			t.Fatalf("bodies = %#v", bodies)
		}
	})

	t.Run("missing record is skipped, not an error", func(t *testing.T) {
		out := []byte(":ai-docs/tickets/todo/260101-feat-a.md missing\nbbb blob 6\nsecond\n")
		bodies, err := parseCatFileBatch(out, requested)
		if err != nil {
			t.Fatalf("parseCatFileBatch returned error: %v", err)
		}
		if _, ok := bodies[requested[0]]; ok {
			t.Fatalf("a missing record produced a body: %#v", bodies)
		}
		if bodies[requested[1]] != "second" {
			t.Fatalf("bodies = %#v", bodies)
		}
	})

	for name, out := range map[string]string{
		"truncated stream":  "aaa blob 5\nfirst\n",
		"short final body":  "aaa blob 5\nfirst\nbbb blob 99\nsecond\n",
		"unexpected header": "aaa blob 5\nfirst\nbbb tree 6\nsecond\n",
		"unreadable size":   "aaa blob 5\nfirst\nbbb blob six\nsecond\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCatFileBatch([]byte(out), requested); err == nil {
				t.Fatalf("parseCatFileBatch accepted a malformed stream (%s)", name)
			}
		})
	}
}

// TestScopeBodiesRejectsNewlineInPath guards the positional pairing: records
// come back in request order, so a path carrying a newline would split into two
// stdin lines and shift every later body onto the wrong ticket - silently
// handing one ticket's `parent:` to another.
func TestScopeBodiesRejectsNewlineInPath(t *testing.T) {
	f := newGraphFixture(t)
	f.ticket("todo", "260101-feat-visible")
	f.ticket("todo", "260102-feat-hidden")
	f.scope([]string{"todo"}, "ai-docs/tickets/todo/260101-feat-visible.md")

	scope := newTicketScope(f.root)
	if scope == nil {
		t.Fatal("newTicketScope returned nil inside a scoped worktree")
	}
	if _, err := scope.bodies([]string{"ai-docs/tickets/todo/260102-feat-hidden.md\nevil.md"}); err == nil {
		t.Fatal("scope.bodies accepted a path containing a newline")
	}
}
