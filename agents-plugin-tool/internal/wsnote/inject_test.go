package wsnote

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

const wantFooter = "_Post-it reminders, higher priority number first: erase stale ones immediately with `note.erase`._"

// writeWorktreeNotes writes n worktree-layer notes "<prefix>.NN" under root,
// priority i for note i, so a higher index sorts first.
func writeWorktreeNotes(t *testing.T, root, prefix string, n int) {
	t.Helper()
	path, err := WorktreePath(root)
	if err != nil {
		t.Fatalf("WorktreePath: %v", err)
	}
	records := make([]Record, 0, n)
	for i := 0; i < n; i++ {
		records = append(records, Record{Key: fmt.Sprintf("%s.%02d", prefix, i), Value: "body-" + prefix, Priority: i, WrittenAt: "t"})
	}
	if err := Write(path, records); err != nil {
		t.Fatalf("Write worktree: %v", err)
	}
}

func isolatedNoteEnv(t *testing.T) wsconfig.Options {
	t.Helper()
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	return wsconfig.Options{ConfigHome: t.TempDir()}
}

func TestComputeRendersEmptyStateHintWhenNoNotesExist(t *testing.T) {
	opts := wsconfig.Options{ConfigHome: t.TempDir()}

	got := Compute(t.TempDir(), opts, 20, 30)
	want := "# Notes\n_No notes. Notes are your short, always-in-context post-it reminders — `note.write` to pin one, `note.erase` when it's no longer needed._"
	if got.Text != want {
		t.Fatalf("Compute = %q, want empty-state hint %q", got.Text, want)
	}
	if got.HiddenTotal() != 0 {
		t.Fatalf("empty Compute hidden = %d, want 0", got.HiddenTotal())
	}
}

func TestComputeAppendsStaleNoteFooterWhenVisible(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{{Key: "m", Value: "machine note", Priority: 1, WrittenAt: "t"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := Compute("", opts, 20, 30).Text
	if !strings.HasSuffix(got, "\n"+wantFooter) {
		t.Fatalf("Compute with visible notes = %q, want last line %q", got, wantFooter)
	}
	if strings.Count(got, "Post-it reminders") != 1 {
		t.Fatalf("Compute footer must be one line: %q", got)
	}
}

func TestComputeAppendsStaleNoteFooterWhenAllMuted(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{{Key: "m", Value: "v", Priority: 1, WrittenAt: "t"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := SetVisible(machinePath, []string{"m"}, false); err != nil {
		t.Fatalf("SetVisible(mute): %v", err)
	}

	got := Compute("", opts, 20, 30).Text
	if !strings.HasSuffix(got, wantFooter) {
		t.Fatalf("Compute with all-muted notes = %q, want suffix %q", got, wantFooter)
	}
	if strings.Contains(got, "No notes.") {
		t.Fatalf("Compute with all-muted notes = %q, must not render the empty-state hint", got)
	}
}

func TestComputeRendersAllLayersWithMachineFirst(t *testing.T) {
	opts := isolatedNoteEnv(t)
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{{Key: "m", Value: "machine note", Priority: 1, WrittenAt: "2026-08-01T00:00:00Z"}}); err != nil {
		t.Fatalf("Write machine: %v", err)
	}

	root := initGitFixture(t)
	worktreePath, err := WorktreePath(root)
	if err != nil {
		t.Fatalf("WorktreePath: %v", err)
	}
	if err := Write(worktreePath, []Record{{Key: "w", Value: "worktree note", Priority: 2, WrittenAt: "2026-08-02T00:00:00Z"}}); err != nil {
		t.Fatalf("Write worktree: %v", err)
	}
	clonePath, err := ClonePath(root)
	if err != nil {
		t.Fatalf("ClonePath: %v", err)
	}
	if err := Write(clonePath, []Record{{Key: "c", Value: "clone note", Priority: 3, WrittenAt: "2026-08-03T00:00:00Z"}}); err != nil {
		t.Fatalf("Write clone: %v", err)
	}

	got := Compute(root, opts, 20, 30).Text
	if !strings.HasPrefix(got, "# Notes") {
		t.Fatalf("Compute = %q, want leading '# Notes' header", got)
	}
	for _, want := range []string{"[machine] m", "machine note", "[worktree] w", "worktree note", "[clone] c", "clone note"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Compute missing %q: %q", want, got)
		}
	}
	// Machine is pinned first despite the lowest priority; the rest follow
	// CompareRecords: clone (3) before worktree (2).
	if !(strings.Index(got, "[machine] m") < strings.Index(got, "[clone] c") && strings.Index(got, "[clone] c") < strings.Index(got, "[worktree] w")) {
		t.Fatalf("Compute order = %q, want machine, then clone (3), then worktree (2)", got)
	}
}

func TestComputeRendersRepoLayer(t *testing.T) {
	opts := isolatedNoteEnv(t)

	root := initGitFixture(t)
	if err := RepoWrite(RepoDir(root), []Record{
		{Key: "r", Value: "repo note", Priority: 5, WrittenAt: "2026-08-03T00:00:00Z"},
	}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}

	got := Compute(root, opts, 20, 30).Text
	if !strings.Contains(got, "[repo] r") || !strings.Contains(got, "repo note") {
		t.Fatalf("Compute missing repo note: %q", got)
	}
}

// TestComputeTiersSpanNonMachineLayers: the body, key-only, and hidden tiers
// share one CompareRecords order across worktree, clone, and repo, and the
// hidden count is broken down by layer.
func TestComputeTiersSpanNonMachineLayers(t *testing.T) {
	opts := isolatedNoteEnv(t)
	root := initGitFixture(t)
	worktreePath, err := WorktreePath(root)
	if err != nil {
		t.Fatalf("WorktreePath: %v", err)
	}
	if err := Write(worktreePath, []Record{
		{Key: "w4", Value: "body-w4", Priority: 4, WrittenAt: "t"},
		{Key: "w1", Value: "body-w1", Priority: 1, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("Write worktree: %v", err)
	}
	clonePath, err := ClonePath(root)
	if err != nil {
		t.Fatalf("ClonePath: %v", err)
	}
	if err := Write(clonePath, []Record{{Key: "c3", Value: "body-c3", Priority: 3, WrittenAt: "t"}}); err != nil {
		t.Fatalf("Write clone: %v", err)
	}
	if err := RepoWrite(RepoDir(root), []Record{
		{Key: "r2", Value: "body-r2", Priority: 2, WrittenAt: "t"},
		{Key: "r0", Value: "body-r0", Priority: 0, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}

	got := Compute(root, opts, 1, 2)
	text := got.Text
	if !strings.Contains(text, "- [worktree] w4 (priority 4, t): body-w4\n") {
		t.Fatalf("body tier must hold the top note w4: %q", text)
	}
	if !strings.Contains(text, "Key only (note.query for the body): [clone] c3, [repo] r2\n") {
		t.Fatalf("key-only tier must list c3 then r2 on one line: %q", text)
	}
	for _, leaked := range []string{"body-c3", "body-r2", "w1", "r0"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("Compute leaked %q past its tier: %q", leaked, text)
		}
	}
	if !strings.Contains(text, "+2 hidden — note.query lists them.\n") {
		t.Fatalf("Compute missing +2 hidden line: %q", text)
	}
	if got.HiddenTotal() != 2 || got.Hidden[LayerWorktree] != 1 || got.Hidden[LayerRepo] != 1 {
		t.Fatalf("Hidden = %v, want worktree 1, repo 1", got.Hidden)
	}
}

// TestComputeTierBoundaries pins the 20/30 split: exactly 20 notes show no
// key-only line, 21 put one key on it, 50 fill it with nothing hidden, and 51
// hide one.
func TestComputeTierBoundaries(t *testing.T) {
	cases := []struct {
		total, keyOnly, hidden int
	}{
		{20, 0, 0},
		{21, 1, 0},
		{50, 30, 0},
		{51, 30, 1},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.total), func(t *testing.T) {
			opts := isolatedNoteEnv(t)
			root := initGitFixture(t)
			writeWorktreeNotes(t, root, "n", tc.total)

			got := Compute(root, opts, 20, 30)
			text := got.Text
			if n := strings.Count(text, "\n- [worktree] "); n != 20 {
				t.Fatalf("body lines = %d, want 20: %q", n, text)
			}
			keyLine := ""
			for _, line := range strings.Split(text, "\n") {
				if strings.HasPrefix(line, "Key only") {
					keyLine = line
				}
			}
			if n := strings.Count(keyLine, "[worktree] "); n != tc.keyOnly {
				t.Fatalf("key-only keys = %d, want %d: %q", n, tc.keyOnly, text)
			}
			if strings.Contains(keyLine, "body-n") {
				t.Fatalf("key-only line carries a body: %q", keyLine)
			}
			if got.HiddenTotal() != tc.hidden {
				t.Fatalf("HiddenTotal = %d, want %d", got.HiddenTotal(), tc.hidden)
			}
			if tc.hidden == 0 {
				if strings.Contains(text, " hidden —") {
					t.Fatalf("unexpected hidden line: %q", text)
				}
				return
			}
			if !strings.Contains(text, fmt.Sprintf("+%d hidden — ", tc.hidden)) {
				t.Fatalf("missing +%d hidden line: %q", tc.hidden, text)
			}
			if strings.Contains(text, "n.00") {
				t.Fatalf("the lowest-priority note n.00 must be hidden, not shown: %q", text)
			}
		})
	}
}

// TestComputePinsMachineNotesOutsideEveryCap: visible machine notes render
// first with their body even when lower priority than every other note, use
// no tier slot, and never count as hidden; muted machine notes stay hidden.
func TestComputePinsMachineNotesOutsideEveryCap(t *testing.T) {
	opts := isolatedNoteEnv(t)
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{
		{Key: "m.a", Value: "machine-a", Priority: -5, WrittenAt: "t"},
		{Key: "m.b", Value: "machine-b", Priority: -1, WrittenAt: "t"},
		{Key: "m.muted", Value: "machine-muted", Priority: 99, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("Write machine: %v", err)
	}
	if err := SetVisible(machinePath, []string{"m.muted"}, false); err != nil {
		t.Fatalf("SetVisible: %v", err)
	}
	root := initGitFixture(t)
	writeWorktreeNotes(t, root, "w", 3)

	got := Compute(root, opts, 1, 1)
	text := got.Text
	// CompareRecords among machine notes: m.b (-1) before m.a (-5).
	wantHead := "# Notes\n- [machine] m.b (priority -1, t): machine-b\n- [machine] m.a (priority -5, t): machine-a\n- [worktree] w.02 "
	if !strings.HasPrefix(text, wantHead) {
		t.Fatalf("Compute = %q, want prefix %q", text, wantHead)
	}
	if !strings.Contains(text, "Key only (note.query for the body): [worktree] w.01\n") {
		t.Fatalf("machine notes consumed a tier slot: %q", text)
	}
	if !strings.Contains(text, "+1 hidden") || got.HiddenTotal() != 1 || got.Hidden[LayerMachine] != 0 {
		t.Fatalf("hidden must count only the one overflowing worktree note: %q, %v", text, got.Hidden)
	}
	if strings.Contains(text, "machine-muted") || strings.Contains(text, "m.muted") {
		t.Fatalf("muted machine note rendered: %q", text)
	}
	if !strings.Contains(text, "(1 muted — use note.query to view.)") {
		t.Fatalf("muted machine note missing from the muted count: %q", text)
	}
}

func TestComputeDegradesSilentlyOnAbsentRepoDir(t *testing.T) {
	opts := isolatedNoteEnv(t)
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{{Key: "m", Value: "machine note", Priority: 1, WrittenAt: "t"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	root := initGitFixture(t)
	// No RepoWrite call: ai-docs/ws-notes/ never exists under root.
	got := Compute(root, opts, 20, 30).Text
	if !strings.Contains(got, "[machine] m") {
		t.Fatalf("Compute with absent repo dir = %q, want machine note still rendered (no error surfaced)", got)
	}
	if strings.Contains(got, "[repo]") {
		t.Fatalf("Compute with absent repo dir unexpectedly rendered a repo note: %q", got)
	}
}

// TestComputeExcludesMutedNotesFromTierBudget verifies muted notes never
// consume a tier slot: muting the top note frees its body slot for the next.
func TestComputeExcludesMutedNotesFromTierBudget(t *testing.T) {
	opts := isolatedNoteEnv(t)
	root := initGitFixture(t)
	worktreePath, err := WorktreePath(root)
	if err != nil {
		t.Fatalf("WorktreePath: %v", err)
	}
	if err := Write(worktreePath, []Record{
		{Key: "high", Value: "v", Priority: 3, WrittenAt: "t"},
		{Key: "mid", Value: "v", Priority: 2, WrittenAt: "t"},
		{Key: "low", Value: "v", Priority: 1, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	baseline := Compute(root, opts, 2, 0).Text
	if strings.Contains(baseline, "[worktree] low") || !strings.Contains(baseline, "+1 hidden") {
		t.Fatalf("baseline Compute must hide the lowest-priority note: %q", baseline)
	}

	if err := SetVisible(worktreePath, []string{"high"}, false); err != nil {
		t.Fatalf("SetVisible(mute high): %v", err)
	}
	got := Compute(root, opts, 2, 0).Text
	if strings.Contains(got, "[worktree] high") {
		t.Fatalf("Compute after muting \"high\" still rendered it: %q", got)
	}
	if !strings.Contains(got, "[worktree] mid") || !strings.Contains(got, "[worktree] low") {
		t.Fatalf("Compute after muting \"high\" did not show both remaining visible notes (mid, low): %q", got)
	}
	if strings.Contains(got, "hidden —") {
		t.Fatalf("Compute after muting \"high\" still shows a hidden line: %q", got)
	}
	if !strings.Contains(got, "1 muted") {
		t.Fatalf("Compute after muting \"high\" missing the muted-count line: %q", got)
	}
}

// TestComputeRendersMutedLineIndependentOfHiddenLine verifies the muted-count
// line and the hidden line render together, hidden first.
func TestComputeRendersMutedLineIndependentOfHiddenLine(t *testing.T) {
	opts := isolatedNoteEnv(t)
	root := initGitFixture(t)
	worktreePath, err := WorktreePath(root)
	if err != nil {
		t.Fatalf("WorktreePath: %v", err)
	}
	if err := Write(worktreePath, []Record{
		{Key: "a", Value: "v", Priority: 4, WrittenAt: "t"},
		{Key: "b", Value: "v", Priority: 3, WrittenAt: "t"},
		{Key: "c", Value: "v", Priority: 2, WrittenAt: "t"},
		{Key: "muted1", Value: "v", Priority: 1, WrittenAt: "t"},
		{Key: "muted2", Value: "v", Priority: 0, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := SetVisible(worktreePath, []string{"muted1", "muted2"}, false); err != nil {
		t.Fatalf("SetVisible(mute): %v", err)
	}

	got := Compute(root, opts, 2, 0).Text
	if !strings.Contains(got, "+1 hidden") {
		t.Fatalf("Compute missing hidden line: %q", got)
	}
	if !strings.Contains(got, "2 muted") {
		t.Fatalf("Compute missing muted-count line: %q", got)
	}
	if strings.Index(got, "+1 hidden") > strings.Index(got, "2 muted") {
		t.Fatalf("Compute rendered muted line before hidden line: %q", got)
	}
	if strings.Contains(got, "muted1") || strings.Contains(got, "muted2") {
		t.Fatalf("Compute rendered a muted note, want it excluded entirely: %q", got)
	}
}

// TestComputeRendersHeadingOnlyWhenAllNotesMuted verifies the all-muted edge:
// the block still renders (heading + muted line) with zero bullet lines.
func TestComputeRendersHeadingOnlyWhenAllNotesMuted(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{
		{Key: "only1", Value: "v", Priority: 1, WrittenAt: "t"},
		{Key: "only2", Value: "v", Priority: 2, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := SetVisible(machinePath, []string{"only1", "only2"}, false); err != nil {
		t.Fatalf("SetVisible(mute all): %v", err)
	}

	got := Compute("", opts, 20, 30).Text
	if !strings.HasPrefix(got, "# Notes") {
		t.Fatalf("Compute with all notes muted = %q, want the block to still render (heading present)", got)
	}
	if strings.Contains(got, "- [machine]") {
		t.Fatalf("Compute with all notes muted rendered a bullet line, want zero: %q", got)
	}
	if !strings.Contains(got, "2 muted") {
		t.Fatalf("Compute with all notes muted missing the muted-count line: %q", got)
	}
	if strings.Contains(got, "hidden —") || strings.Contains(got, "Key only") {
		t.Fatalf("Compute with all notes muted unexpectedly rendered a tier line: %q", got)
	}
}

func TestComputeDegradesToMachineOnlyWithEmptyRoot(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{{Key: "m", Value: "machine note", Priority: 1, WrittenAt: "t"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := Compute("", opts, 20, 30).Text
	if !strings.Contains(got, "[machine] m") {
		t.Fatalf("Compute with empty root = %q, want machine note still rendered", got)
	}
}
