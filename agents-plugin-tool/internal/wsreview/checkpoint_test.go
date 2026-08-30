package wsreview

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// checkpointTestCommit adds one trivial commit to root's current branch and
// returns its full SHA.
func checkpointTestCommit(t *testing.T, root, label string) string {
	t.Helper()
	path := label + ".txt"
	if err := os.WriteFile(root+"/"+path, []byte(label+"\n"), 0o644); err != nil {
		t.Fatalf("write commit fixture file: %v", err)
	}
	reviewTestRunGit(t, root, "add", path)
	reviewTestRunGit(t, root, "commit", "-m", label)
	return strings.TrimSpace(string(reviewTestRunGitOutput(t, root, "rev-parse", "HEAD")))
}

// TestCheckpointNudgeNoLedgerReturnsBaselineAdvisoryWithoutWriting is the
// ledger-honesty guard's no-ledger branch: CheckpointNudge must surface a
// baseline-missing advisory but must never create the ledger file itself —
// only the explicit, caller-opted-in review.marker(bootstrap: true) may do
// that.
func TestCheckpointNudgeNoLedgerReturnsBaselineAdvisoryWithoutWriting(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "main")

	got := CheckpointNudge(context.Background(), root)
	if !strings.Contains(got, "no review ledger yet") {
		t.Fatalf("CheckpointNudge = %q, want a baseline-missing advisory", got)
	}
	if _, err := os.Stat(LedgerPath(root)); !os.IsNotExist(err) {
		t.Fatalf("CheckpointNudge must never create the ledger file on the no-ledger path; stat err = %v", err)
	}
}

// TestCheckpointNudgeQuietOnSmallFreshRangeNeverAppends is the ledger-honesty
// guard's quiet branch: a range below both thresholds must return "" and
// must never mutate the (already-seeded) ledger file.
func TestCheckpointNudgeQuietOnSmallFreshRangeNeverAppends(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "main")
	head := strings.TrimSpace(string(reviewTestRunGitOutput(t, root, "rev-parse", "HEAD")))
	if err := Append(root, Entry{Base: head, Head: head, Verdict: VerdictPass}); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	before, err := os.ReadFile(LedgerPath(root))
	if err != nil {
		t.Fatalf("read seeded ledger: %v", err)
	}

	for i := 0; i < 3; i++ { // well below the default staleness (10) and size (20) thresholds
		checkpointTestCommit(t, root, fmt.Sprintf("c%d", i))
	}

	got := CheckpointNudge(context.Background(), root)
	if got != "" {
		t.Fatalf("CheckpointNudge on a small fresh range = %q, want \"\"", got)
	}
	after, err := os.ReadFile(LedgerPath(root))
	if err != nil {
		t.Fatalf("read ledger after CheckpointNudge: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("quiet-range CheckpointNudge mutated the ledger file: before=%q after=%q", before, after)
	}
}

func TestCheckpointNudgeStaleRangeScalesAdvisory(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "main")
	head := strings.TrimSpace(string(reviewTestRunGitOutput(t, root, "rev-parse", "HEAD")))
	if err := Append(root, Entry{Base: head, Head: head, Verdict: VerdictPass}); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}

	for i := 0; i < DefaultStalenessCommits+2; i++ { // past staleness (10), still below size (20)
		checkpointTestCommit(t, root, fmt.Sprintf("c%d", i))
	}

	got := CheckpointNudge(context.Background(), root)
	if got == "" {
		t.Fatalf("CheckpointNudge on a stale range returned \"\", want an advisory")
	}
	if !strings.Contains(got, "staleness threshold") {
		t.Fatalf("CheckpointNudge = %q, want the staleness-threshold advisory (not yet large-accumulation)", got)
	}
}

func TestCheckpointNudgeLargeRangeUsesSizeThreshold(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "main")
	head := strings.TrimSpace(string(reviewTestRunGitOutput(t, root, "rev-parse", "HEAD")))
	if err := Append(root, Entry{Base: head, Head: head, Verdict: VerdictPass}); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}

	for i := 0; i < SizeThresholdCommits; i++ {
		checkpointTestCommit(t, root, fmt.Sprintf("c%d", i))
	}

	got := CheckpointNudge(context.Background(), root)
	if !strings.Contains(got, "large-accumulation threshold") {
		t.Fatalf("CheckpointNudge = %q, want the large-accumulation advisory", got)
	}
}

func TestCheckpointNudgeSkipsSilentlyWhenTrackUnresolvable(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "feature-only")

	got := CheckpointNudge(context.Background(), root)
	if got != "" {
		t.Fatalf("CheckpointNudge with no resolvable track = %q, want \"\" (fail open)", got)
	}
}
