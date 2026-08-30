package wsreview

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendReadRoundTrip(t *testing.T) {
	root := t.TempDir()

	want := Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictPass}
	if err := Append(root, want); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	got, found, err := Read(root)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !found {
		t.Fatalf("Read did not find the appended entry")
	}
	if got != want {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestAppendReadRoundTripWithRef(t *testing.T) {
	root := t.TempDir()

	want := Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictBlock, Ref: "260901-bug-example"}
	if err := Append(root, want); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	got, found, err := Read(root)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !found {
		t.Fatalf("Read did not find the appended entry")
	}
	if got != want {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestParseLatestSkipsBannerLinesBeforeAndAfter(t *testing.T) {
	content := strings.Join([]string{
		"# banner line before any real entry",
		"aaaaaaa..bbbbbbb: pass",
		"ccccccc..ddddddd: concern -> 260901-concern-example",
		"# tail-anchor comment banner after the last real entry",
		"",
	}, "\n")

	entry, found := ParseLatest(content)
	if !found {
		t.Fatalf("ParseLatest did not find a real entry amid banner lines")
	}
	want := Entry{Base: "ccccccc", Head: "ddddddd", Verdict: VerdictConcern, Ref: "260901-concern-example"}
	if entry != want {
		t.Fatalf("ParseLatest returned wrong entry: got %+v, want %+v", entry, want)
	}
}

func TestReadUnaffectedByUnrelatedEditAfterLastEntry(t *testing.T) {
	root := t.TempDir()

	if err := Append(root, Entry{Base: "aaaaaaa", Head: "bbbbbbb", Verdict: VerdictPass}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Simulate an unrelated edit: append a line that does not match the
	// entry regex after the last real entry.
	path := LedgerPath(root)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open ledger for unrelated edit: %v", err)
	}
	if _, err := f.WriteString("some unrelated note that is not an entry line\n"); err != nil {
		f.Close()
		t.Fatalf("write unrelated edit: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close ledger after unrelated edit: %v", err)
	}

	entry, found, err := Read(root)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !found {
		t.Fatalf("Read did not find the last real entry after an unrelated edit")
	}
	want := Entry{Base: "aaaaaaa", Head: "bbbbbbb", Verdict: VerdictPass}
	if entry != want {
		t.Fatalf("marker resolution affected by unrelated edit: got %+v, want %+v", entry, want)
	}
}

func TestBootstrapCreatesEntryAtHEADAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	sha := reviewTestInitGitWithCommit(t, root)

	entry, created, err := Bootstrap(context.Background(), root)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if !created {
		t.Fatalf("Bootstrap on an absent ledger should report created = true")
	}
	want := Entry{Base: sha, Head: sha, Verdict: VerdictBootstrap}
	if entry != want {
		t.Fatalf("Bootstrap entry mismatch: got %+v, want %+v", entry, want)
	}

	if _, err := os.Stat(LedgerPath(root)); err != nil {
		t.Fatalf("Bootstrap did not create the ledger file: %v", err)
	}

	// Second call must be a no-op: created == false, same entry returned.
	entry2, created2, err := Bootstrap(context.Background(), root)
	if err != nil {
		t.Fatalf("second Bootstrap failed: %v", err)
	}
	if created2 {
		t.Fatalf("second Bootstrap on an already-seeded ledger should report created = false")
	}
	if entry2 != want {
		t.Fatalf("second Bootstrap entry mismatch: got %+v, want %+v", entry2, want)
	}
}

func TestBootstrapRetriggersOnBannerOnlyLedger(t *testing.T) {
	root := t.TempDir()
	sha := reviewTestInitGitWithCommit(t, root)

	// Seed a banner-only ledger: file present, but zero parseable entries.
	banner := "# banner line, no real entry yet\n# another comment line\n"
	if err := os.MkdirAll(filepath.Dir(LedgerPath(root)), 0o755); err != nil {
		t.Fatalf("create ai-docs dir: %v", err)
	}
	if err := os.WriteFile(LedgerPath(root), []byte(banner), 0o644); err != nil {
		t.Fatalf("write banner-only ledger: %v", err)
	}

	// Both Read and ParseLatest must report found == false on a banner-only
	// ledger — this is the "present but zero parseable entries" branch, not
	// the file-missing branch.
	entry, found, err := Read(root)
	if err != nil {
		t.Fatalf("Read on banner-only ledger failed: %v", err)
	}
	if found {
		t.Fatalf("Read on a banner-only ledger should report found = false, got entry %+v", entry)
	}
	if _, found := ParseLatest(banner); found {
		t.Fatalf("ParseLatest on banner-only content should report found = false")
	}

	bootstrapped, created, err := Bootstrap(context.Background(), root)
	if err != nil {
		t.Fatalf("Bootstrap on banner-only ledger failed: %v", err)
	}
	if !created {
		t.Fatalf("Bootstrap on a banner-only (present but zero-parseable) ledger should report created = true")
	}
	want := Entry{Base: sha, Head: sha, Verdict: VerdictBootstrap}
	if bootstrapped != want {
		t.Fatalf("Bootstrap entry mismatch: got %+v, want %+v", bootstrapped, want)
	}

	// The pre-existing banner lines must survive untouched (append-only).
	raw, err := os.ReadFile(LedgerPath(root))
	if err != nil {
		t.Fatalf("read raw ledger file: %v", err)
	}
	if !strings.HasPrefix(string(raw), banner) {
		t.Fatalf("pre-existing banner lines were not preserved; got raw content:\n%s", raw)
	}
}

func TestAppendBlockWithEmptyRefFails(t *testing.T) {
	root := t.TempDir()

	err := Append(root, Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictBlock})
	if err == nil {
		t.Fatalf("Append with Verdict=block and empty Ref should have failed")
	}
}

func TestAppendWithWhitespaceRefFails(t *testing.T) {
	root := t.TempDir()

	err := Append(root, Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictBlock, Ref: "260901 bug example"})
	if err == nil {
		t.Fatalf("Append with a whitespace-bearing Ref should have failed")
	}

	// A newline-bearing Ref must also be rejected (would otherwise emit two
	// physical lines and corrupt the one-entry-per-line contract).
	err = Append(root, Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictConcern, Ref: "260901-bug\nexample"})
	if err == nil {
		t.Fatalf("Append with a newline-bearing Ref should have failed")
	}
}

func TestAppendConcernWithEmptyRefSucceeds(t *testing.T) {
	root := t.TempDir()

	err := Append(root, Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictConcern})
	if err != nil {
		t.Fatalf("Append with Verdict=concern and empty Ref should succeed (stem requirement is block-only): %v", err)
	}

	got, found, err := Read(root)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !found {
		t.Fatalf("Read did not find the appended concern entry")
	}
	if got.Ref != "" {
		t.Fatalf("expected empty Ref on concern entry, got %q", got.Ref)
	}
}

func TestRoutedCorrectiveEntryAppendsWithoutEditingEarlierBlock(t *testing.T) {
	root := t.TempDir()

	if err := Append(root, Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictBlock, Ref: "260901-bug-example"}); err != nil {
		t.Fatalf("initial block Append failed: %v", err)
	}
	if err := Append(root, Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictRouted, Ref: "260901-bug-example"}); err != nil {
		t.Fatalf("routed corrective Append failed: %v", err)
	}

	raw, err := os.ReadFile(LedgerPath(root))
	if err != nil {
		t.Fatalf("read raw ledger file: %v", err)
	}
	// The first Append physically creates the file, so it must carry the
	// first-creation banner ahead of the two entry lines.
	if !strings.HasPrefix(string(raw), ledgerBanner) {
		t.Fatalf("first-creation Append did not emit the banner at the top of the file; got raw content:\n%s", raw)
	}
	afterBanner := strings.TrimPrefix(string(raw), ledgerBanner)
	lines := strings.Split(strings.TrimRight(afterBanner, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 entry lines below the banner (original block untouched, plus append-only routed follow-up), got %d: %v", len(lines), lines)
	}
	if lines[0] != "abc1234..def5678: block -> 260901-bug-example" {
		t.Fatalf("original block entry was mutated: %q", lines[0])
	}
	if lines[1] != "abc1234..def5678: routed -> 260901-bug-example" {
		t.Fatalf("routed corrective entry line mismatch: %q", lines[1])
	}

	// The latest entry read via the marker parser must be the routed
	// follow-up, not the earlier block.
	entry, found, err := Read(root)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !found {
		t.Fatalf("Read did not find an entry")
	}
	if entry.Verdict != VerdictRouted {
		t.Fatalf("expected latest entry verdict to be routed, got %q", entry.Verdict)
	}
}

// TestParseFrontierTrailingBlockHoldsAtPriorClearingEntry pins the frontier
// resolver's core behavior: a trailing block entry does not advance the
// frontier past the last pass/concern/bootstrap entry that preceded it.
func TestParseFrontierTrailingBlockHoldsAtPriorClearingEntry(t *testing.T) {
	content := strings.Join([]string{
		"aaaaaaa..bbbbbbb: pass",
		"bbbbbbb..ccccccc: block -> 260901-bug-example",
		"",
	}, "\n")

	entry, found := ParseFrontier(content)
	if !found {
		t.Fatalf("ParseFrontier did not find the prior clearing entry")
	}
	want := Entry{Base: "aaaaaaa", Head: "bbbbbbb", Verdict: VerdictPass}
	if entry != want {
		t.Fatalf("ParseFrontier = %+v, want %+v (frontier held at prior pass entry)", entry, want)
	}
}

// TestParseFrontierTrailingRoutedHoldsAtPriorClearingEntry mirrors the
// trailing-block case for a trailing routed corrective entry — routed is
// likewise skipped, never overwriting the running frontier.
func TestParseFrontierTrailingRoutedHoldsAtPriorClearingEntry(t *testing.T) {
	content := strings.Join([]string{
		"aaaaaaa..bbbbbbb: concern -> 260901-concern-example",
		"bbbbbbb..ccccccc: block -> 260901-bug-example",
		"bbbbbbb..ccccccc: routed -> 260901-bug-example",
		"",
	}, "\n")

	entry, found := ParseFrontier(content)
	if !found {
		t.Fatalf("ParseFrontier did not find the prior clearing entry")
	}
	want := Entry{Base: "aaaaaaa", Head: "bbbbbbb", Verdict: VerdictConcern, Ref: "260901-concern-example"}
	if entry != want {
		t.Fatalf("ParseFrontier = %+v, want %+v (frontier held at prior concern entry)", entry, want)
	}
}

// TestParseFrontierBootstrapAloneResolvesAsFloor proves bootstrap is itself a
// resolvable frontier floor when it is the only entry.
func TestParseFrontierBootstrapAloneResolvesAsFloor(t *testing.T) {
	content := "aaaaaaa..aaaaaaa: bootstrap\n"

	entry, found := ParseFrontier(content)
	if !found {
		t.Fatalf("ParseFrontier did not find the bootstrap floor entry")
	}
	want := Entry{Base: "aaaaaaa", Head: "aaaaaaa", Verdict: VerdictBootstrap}
	if entry != want {
		t.Fatalf("ParseFrontier = %+v, want %+v", entry, want)
	}
}

// TestParseFrontierPassAndConcernAdvanceNormally proves the frontier
// advances through consecutive clearing entries exactly like ParseLatest
// does, mirroring TestParseLatestSkipsBannerLinesBeforeAndAfter's coverage
// shape for the non-skipped case.
func TestParseFrontierPassAndConcernAdvanceNormally(t *testing.T) {
	content := strings.Join([]string{
		"# banner line before any real entry",
		"aaaaaaa..bbbbbbb: pass",
		"ccccccc..ddddddd: concern -> 260901-concern-example",
		"",
	}, "\n")

	entry, found := ParseFrontier(content)
	if !found {
		t.Fatalf("ParseFrontier did not find a real entry amid banner lines")
	}
	want := Entry{Base: "ccccccc", Head: "ddddddd", Verdict: VerdictConcern, Ref: "260901-concern-example"}
	if entry != want {
		t.Fatalf("ParseFrontier returned wrong entry: got %+v, want %+v", entry, want)
	}
}

// TestFrontierRoundTripsThroughFile proves Frontier(root) reads the same
// file-handling contract as Read(root) (missing file -> found=false, no
// error) while resolving to the frontier rather than the raw latest entry.
func TestFrontierRoundTripsThroughFile(t *testing.T) {
	root := t.TempDir()

	if _, found, err := Frontier(root); err != nil || found {
		t.Fatalf("Frontier on a missing ledger file = (found=%v, err=%v), want (false, nil)", found, err)
	}

	if err := Append(root, Entry{Base: "aaaaaaa", Head: "bbbbbbb", Verdict: VerdictPass}); err != nil {
		t.Fatalf("Append pass entry failed: %v", err)
	}
	if err := Append(root, Entry{Base: "bbbbbbb", Head: "ccccccc", Verdict: VerdictBlock, Ref: "260901-bug-example"}); err != nil {
		t.Fatalf("Append block entry failed: %v", err)
	}

	entry, found, err := Frontier(root)
	if err != nil {
		t.Fatalf("Frontier failed: %v", err)
	}
	if !found {
		t.Fatalf("Frontier did not find the prior clearing entry")
	}
	want := Entry{Base: "aaaaaaa", Head: "bbbbbbb", Verdict: VerdictPass}
	if entry != want {
		t.Fatalf("Frontier = %+v, want %+v (held at the pass entry, not the trailing block)", entry, want)
	}
}

func TestAppendFirstCreationEmitsBannerOnce(t *testing.T) {
	root := t.TempDir()

	want := Entry{Base: "abc1234", Head: "def5678", Verdict: VerdictPass}
	if err := Append(root, want); err != nil {
		t.Fatalf("first Append failed: %v", err)
	}

	raw, err := os.ReadFile(LedgerPath(root))
	if err != nil {
		t.Fatalf("read raw ledger file: %v", err)
	}
	if !strings.HasPrefix(string(raw), ledgerBanner) {
		t.Fatalf("fresh ledger did not start with the banner; got raw content:\n%s", raw)
	}

	// ParseLatest/Read must resolve to the real entry, never the banner —
	// entryLineRE skips every banner line.
	entry, found, err := Read(root)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !found {
		t.Fatalf("Read did not find the appended entry")
	}
	if entry != want {
		t.Fatalf("Read resolved to the wrong entry amid the banner: got %+v, want %+v", entry, want)
	}

	// A second Append on the now-existing file must not re-emit the banner.
	second := Entry{Base: "aaa1111", Head: "bbb2222", Verdict: VerdictPass}
	if err := Append(root, second); err != nil {
		t.Fatalf("second Append failed: %v", err)
	}
	raw2, err := os.ReadFile(LedgerPath(root))
	if err != nil {
		t.Fatalf("read raw ledger file after second append: %v", err)
	}
	if strings.Count(string(raw2), ledgerBanner) != 1 {
		t.Fatalf("banner must be emitted exactly once; got raw content:\n%s", raw2)
	}
}

// TestCanaryConcurrentFirstCreationConflictsSerialAppendDoesNot reproduces
// the Phase-3 canary in a real git repo: two branches that each Append
// independently — without either absorbing the other's write first —
// produce a real git conflict in the ledger's tail, with the explanatory
// banner surviving in the merged file (the concurrent-first-landing case
// named in the plan's Lead Adjudications: neither branch's common ancestor
// has a ledger file yet — an add/add conflict). Whether git places the
// banner inside the conflict hunk or keeps it as common context above the
// markers is git-version-dependent (Apple git 2.50 swept it into the hunk;
// Linux git 2.55 in CI diffs it out), so the assertion checks the banner
// against the whole file, not the hunk. A serial single-writer sequence,
// with no competing concurrent creation, must merge cleanly.
func TestCanaryConcurrentFirstCreationConflictsSerialAppendDoesNot(t *testing.T) {
	root := t.TempDir()
	reviewTestInitGitWithCommit(t, root)
	mainBranch := strings.TrimSpace(string(reviewTestRunGitOutput(t, root, "rev-parse", "--abbrev-ref", "HEAD")))

	relLedger, err := filepath.Rel(root, LedgerPath(root))
	if err != nil {
		t.Fatalf("relativize ledger path: %v", err)
	}

	reviewTestRunGit(t, root, "branch", "branch-a")
	reviewTestRunGit(t, root, "branch", "branch-b")

	// Branch A: the first-ever Append on this branch. The ledger does not
	// exist in the common ancestor, so this is the bare-Append-with-no-
	// prior-Bootstrap first-creation path (the risk signal from the plan's
	// Codebase Findings).
	reviewTestRunGit(t, root, "checkout", "branch-a")
	if err := Append(root, Entry{Base: "aaaaaaa", Head: "bbbbbbb", Verdict: VerdictPass}); err != nil {
		t.Fatalf("branch-a Append failed: %v", err)
	}
	reviewTestRunGit(t, root, "add", relLedger)
	reviewTestRunGit(t, root, "commit", "-m", "branch-a review")

	// Branch B: independently, the first-ever Append on this branch too —
	// two branches racing to create the ledger from nothing, neither seeing
	// the other's write. This is the canary's precondition.
	reviewTestRunGit(t, root, "checkout", "branch-b")
	if err := Append(root, Entry{Base: "aaaaaaa", Head: "ccccccc", Verdict: VerdictConcern}); err != nil {
		t.Fatalf("branch-b Append failed: %v", err)
	}
	reviewTestRunGit(t, root, "add", relLedger)
	reviewTestRunGit(t, root, "commit", "-m", "branch-b review")

	// Merge branch-a into branch-b: an add/add conflict, since the ledger
	// exists on neither side's common ancestor — the canary firing.
	mergeCmd := exec.Command("git", "merge", "branch-a", "--no-edit")
	mergeCmd.Dir = root
	mergeOut, mergeErr := mergeCmd.CombinedOutput()
	if mergeErr == nil {
		t.Fatalf("expected a git conflict between two branches independently creating the ledger, got a clean merge:\n%s", mergeOut)
	}

	conflicted, err := os.ReadFile(LedgerPath(root))
	if err != nil {
		t.Fatalf("read conflicted ledger file: %v", err)
	}
	content := string(conflicted)
	startMarker := strings.Index(content, "<<<<<<<")
	endMarker := strings.Index(content, ">>>>>>>")
	if startMarker == -1 || endMarker == -1 || endMarker < startMarker {
		t.Fatalf("expected git conflict markers in the ledger file, got:\n%s", content)
	}
	// The banner is a top-of-file block; git keeps it as common context above
	// the conflict region rather than inside the markers. Whether it lands
	// inside the conflict hunk is git-version-dependent — Apple git 2.50 swept
	// it in, Linux git 2.55 (CI) does not — so asserting against the hunk is
	// non-portable. The canary's contract is that a conflict fires (markers
	// present, asserted above) and the explanatory banner survives in the
	// merged file; assert that against the whole file, not the hunk.
	if !strings.Contains(content, ledgerBanner) {
		t.Fatalf("expected the ledger banner to survive in the conflicted file, got:\n%s", content)
	}

	reviewTestRunGit(t, root, "merge", "--abort")

	// --- Serial path: a single writer appending once, with no competing
	// concurrent creation on the other side, must merge cleanly (no
	// false-positive conflict).
	reviewTestRunGit(t, root, "checkout", mainBranch)
	serialMergeCmd := exec.Command("git", "merge", "branch-b", "--no-edit")
	serialMergeCmd.Dir = root
	serialOut, serialErr := serialMergeCmd.CombinedOutput()
	if serialErr != nil {
		t.Fatalf("serial single-writer merge should not conflict: %v\n%s", serialErr, serialOut)
	}
	if strings.Contains(string(serialOut), "CONFLICT") {
		t.Fatalf("serial single-writer merge reported a conflict:\n%s", serialOut)
	}
}

// reviewTestInitGitWithCommit initializes a real temp git repo (reusing the
// wsgit package's sparseTestInitGit/sparseTestRunGit fixture idiom from
// internal/wsgit/git_test.go) with one commit, and returns HEAD's full SHA.
func reviewTestInitGitWithCommit(t *testing.T, root string) string {
	t.Helper()

	reviewTestRunGit(t, root, "init")
	reviewTestRunGit(t, root, "config", "core.autocrlf", "false")
	reviewTestRunGit(t, root, "config", "user.email", "test@example.com")
	reviewTestRunGit(t, root, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed fixture file: %v", err)
	}
	reviewTestRunGit(t, root, "add", "seed.txt")
	reviewTestRunGit(t, root, "commit", "-m", "seed commit")

	out := reviewTestRunGitOutput(t, root, "rev-parse", "HEAD")
	return strings.TrimSpace(string(out))
}

func reviewTestRunGit(t *testing.T, root string, args ...string) {
	t.Helper()
	reviewTestRunGitOutput(t, root, args...)
}

func reviewTestRunGitOutput(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return out
}
