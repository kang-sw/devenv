package wsnote

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestComputeReturnsEmptyWhenNoNotesExist(t *testing.T) {
	opts := wsconfig.Options{ConfigHome: t.TempDir()}

	got := Compute(t.TempDir(), opts, 20)
	if got != "" {
		t.Fatalf("Compute = %q, want empty string when no notes exist", got)
	}
}

func TestComputeRendersMachineAndWorktreeLayers(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
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

	got := Compute(root, opts, 20)
	if !strings.HasPrefix(got, "# Notes") {
		t.Fatalf("Compute = %q, want leading '# Notes' header", got)
	}
	if !strings.Contains(got, "[machine] m") || !strings.Contains(got, "machine note") {
		t.Fatalf("Compute missing machine note: %q", got)
	}
	if !strings.Contains(got, "[worktree] w") || !strings.Contains(got, "worktree note") {
		t.Fatalf("Compute missing worktree note: %q", got)
	}
	if !strings.Contains(got, "[clone] c") || !strings.Contains(got, "clone note") {
		t.Fatalf("Compute missing clone note: %q", got)
	}
	// Highest priority (clone, priority 3) renders first, then worktree (2),
	// then machine (1).
	if strings.Index(got, "[clone] c") > strings.Index(got, "[worktree] w") {
		t.Fatalf("Compute order = %q, want higher-priority clone note before worktree note", got)
	}
	if strings.Index(got, "[worktree] w") > strings.Index(got, "[machine] m") {
		t.Fatalf("Compute order = %q, want higher-priority worktree note before machine note", got)
	}
}

func TestComputeRendersRepoLayer(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}

	root := initGitFixture(t)
	if err := RepoWrite(RepoDir(root), []Record{
		{Key: "r", Value: "repo note", Priority: 5, WrittenAt: "2026-08-03T00:00:00Z"},
	}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}

	got := Compute(root, opts, 20)
	if !strings.Contains(got, "[repo] r") || !strings.Contains(got, "repo note") {
		t.Fatalf("Compute missing repo note: %q", got)
	}
}

func TestComputeSortsAndCapsAcrossAllFourLayers(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
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
	if err := RepoWrite(RepoDir(root), []Record{{Key: "r", Value: "repo note", Priority: 3, WrittenAt: "2026-08-03T00:00:00Z"}}); err != nil {
		t.Fatalf("RepoWrite: %v", err)
	}
	clonePath, err := ClonePath(root)
	if err != nil {
		t.Fatalf("ClonePath: %v", err)
	}
	if err := Write(clonePath, []Record{{Key: "c", Value: "clone note", Priority: 4, WrittenAt: "2026-08-04T00:00:00Z"}}); err != nil {
		t.Fatalf("Write clone: %v", err)
	}

	got := Compute(root, opts, 2)
	if strings.Contains(got, "[machine] m") {
		t.Fatalf("Compute with limit=2 kept the lowest-priority (machine) note, want it elided: %q", got)
	}
	if strings.Contains(got, "[worktree] w") {
		t.Fatalf("Compute with limit=2 kept the third-lowest-priority (worktree) note, want it elided: %q", got)
	}
	if !strings.Contains(got, "[clone] c") || !strings.Contains(got, "[repo] r") {
		t.Fatalf("Compute with limit=2 did not keep the two highest-priority notes across all four layers: %q", got)
	}
	if strings.Index(got, "[clone] c") > strings.Index(got, "[repo] r") {
		t.Fatalf("Compute order = %q, want higher-priority clone note (4) before repo note (3)", got)
	}
	if !strings.Contains(got, "2 lower-priority notes elided") {
		t.Fatalf("Compute with limit=2 missing elision line: %q", got)
	}
}

func TestComputeDegradesSilentlyOnAbsentRepoDir(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{{Key: "m", Value: "machine note", Priority: 1, WrittenAt: "t"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	root := initGitFixture(t)
	// No RepoWrite call: ai-docs/ws-notes/ never exists under root.
	got := Compute(root, opts, 20)
	if !strings.Contains(got, "[machine] m") {
		t.Fatalf("Compute with absent repo dir = %q, want machine note still rendered (no error surfaced)", got)
	}
	if strings.Contains(got, "[repo]") {
		t.Fatalf("Compute with absent repo dir unexpectedly rendered a repo note: %q", got)
	}
}

func TestComputeElidesBeyondLimit(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}

	records := make([]Record, 0, 5)
	for i := 0; i < 5; i++ {
		records = append(records, Record{Key: string(rune('a' + i)), Value: "v", Priority: i, WrittenAt: "t"})
	}
	if err := Write(machinePath, records); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := Compute("", opts, 2)
	if strings.Count(got, "- [machine]") != 2 {
		t.Fatalf("Compute with limit=2 = %q, want exactly 2 rendered lines", got)
	}
	if !strings.Contains(got, "3 lower-priority notes elided") {
		t.Fatalf("Compute with limit=2 missing elision line: %q", got)
	}
	// Highest priorities (4, 3) must be the ones shown, not elided.
	if !strings.Contains(got, "priority 4") || !strings.Contains(got, "priority 3") {
		t.Fatalf("Compute with limit=2 did not keep the highest-priority notes: %q", got)
	}
}

// TestComputeExcludesMutedNotesFromCapBudget verifies muted notes never
// consume a notesInjectionCap slot: with limit=2 and three notes where the
// lowest-priority one would normally be elided, muting the highest-priority
// note instead frees a slot for the previously-elided one, which now shows.
func TestComputeExcludesMutedNotesFromCapBudget(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{
		{Key: "high", Value: "v", Priority: 3, WrittenAt: "t"},
		{Key: "mid", Value: "v", Priority: 2, WrittenAt: "t"},
		{Key: "low", Value: "v", Priority: 1, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Baseline: limit=2 elides "low" (lowest priority).
	baseline := Compute("", opts, 2)
	if strings.Contains(baseline, "[machine] low") {
		t.Fatalf("baseline Compute unexpectedly included the lowest-priority note: %q", baseline)
	}
	if !strings.Contains(baseline, "1 lower-priority notes elided") {
		t.Fatalf("baseline Compute missing elision line: %q", baseline)
	}

	// Mute the highest-priority note: it must free its slot for "low".
	if err := SetVisible(machinePath, []string{"high"}, false); err != nil {
		t.Fatalf("SetVisible(mute high): %v", err)
	}
	got := Compute("", opts, 2)
	if strings.Contains(got, "[machine] high") {
		t.Fatalf("Compute after muting \"high\" still rendered it: %q", got)
	}
	if !strings.Contains(got, "[machine] mid") || !strings.Contains(got, "[machine] low") {
		t.Fatalf("Compute after muting \"high\" did not show both remaining visible notes (mid, low): %q", got)
	}
	if strings.Contains(got, "lower-priority notes elided") {
		t.Fatalf("Compute after muting \"high\" still shows an elision line with only 2 visible notes left under limit=2: %q", got)
	}
	if !strings.Contains(got, "1 muted") {
		t.Fatalf("Compute after muting \"high\" missing the muted-count line: %q", got)
	}
}

// TestComputeRendersMutedLineIndependentOfElisionLine verifies both the
// muted-count line and the over-cap elision line render together when both
// conditions apply, and that neither line depends on the other's presence.
func TestComputeRendersMutedLineIndependentOfElisionLine(t *testing.T) {
	configHome := t.TempDir()
	opts := wsconfig.Options{ConfigHome: configHome}
	machinePath, err := MachinePath(opts)
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := Write(machinePath, []Record{
		{Key: "a", Value: "v", Priority: 4, WrittenAt: "t"},
		{Key: "b", Value: "v", Priority: 3, WrittenAt: "t"},
		{Key: "c", Value: "v", Priority: 2, WrittenAt: "t"},
		{Key: "muted1", Value: "v", Priority: 1, WrittenAt: "t"},
		{Key: "muted2", Value: "v", Priority: 0, WrittenAt: "t"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := SetVisible(machinePath, []string{"muted1", "muted2"}, false); err != nil {
		t.Fatalf("SetVisible(mute): %v", err)
	}

	// limit=2: 3 visible notes (a, b, c) -> 1 elided (c). 2 muted (muted1, muted2).
	got := Compute("", opts, 2)
	if !strings.Contains(got, "1 lower-priority notes elided") {
		t.Fatalf("Compute missing elision line: %q", got)
	}
	if !strings.Contains(got, "2 muted") {
		t.Fatalf("Compute missing muted-count line: %q", got)
	}
	if strings.Index(got, "lower-priority notes elided") > strings.Index(got, "muted") {
		t.Fatalf("Compute rendered muted line before elision line, want elision line first (house style): %q", got)
	}
	if strings.Contains(got, "muted1") || strings.Contains(got, "muted2") {
		t.Fatalf("Compute rendered a muted note's bullet line, want it excluded entirely: %q", got)
	}
}

// TestComputeRendersHeadingOnlyWhenAllNotesMuted verifies the all-muted edge:
// the block still renders (heading + muted line) when every note on a layer
// is muted, even though zero visible notes remain — distinct from the
// truly-empty edge (TestComputeReturnsEmptyWhenNoNotesExist) where the block
// is skipped entirely.
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

	got := Compute("", opts, 20)
	if !strings.HasPrefix(got, "# Notes") {
		t.Fatalf("Compute with all notes muted = %q, want the block to still render (heading present)", got)
	}
	if strings.Contains(got, "- [machine]") {
		t.Fatalf("Compute with all notes muted rendered a bullet line, want zero: %q", got)
	}
	if !strings.Contains(got, "2 muted") {
		t.Fatalf("Compute with all notes muted missing the muted-count line: %q", got)
	}
	if strings.Contains(got, "lower-priority notes elided") {
		t.Fatalf("Compute with all notes muted (zero visible, zero over-cap) unexpectedly rendered an elision line: %q", got)
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

	got := Compute("", opts, 20)
	if !strings.Contains(got, "[machine] m") {
		t.Fatalf("Compute with empty root = %q, want machine note still rendered", got)
	}
}
