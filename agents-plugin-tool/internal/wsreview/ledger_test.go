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
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 lines (original block untouched, plus append-only routed follow-up), got %d: %v", len(lines), lines)
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
