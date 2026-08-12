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
	// Higher priority (worktree, priority 2) renders before machine (priority 1).
	if strings.Index(got, "[worktree] w") > strings.Index(got, "[machine] m") {
		t.Fatalf("Compute order = %q, want higher-priority worktree note first", got)
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

func TestComputeSortsAndCapsAcrossAllThreeLayers(t *testing.T) {
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

	got := Compute(root, opts, 2)
	if strings.Contains(got, "[machine] m") {
		t.Fatalf("Compute with limit=2 kept the lowest-priority (machine) note, want it elided: %q", got)
	}
	if !strings.Contains(got, "[repo] r") || !strings.Contains(got, "[worktree] w") {
		t.Fatalf("Compute with limit=2 did not keep the two highest-priority notes across all three layers: %q", got)
	}
	if strings.Index(got, "[repo] r") > strings.Index(got, "[worktree] w") {
		t.Fatalf("Compute order = %q, want higher-priority repo note (3) before worktree note (2)", got)
	}
	if !strings.Contains(got, "1 lower-priority notes elided") {
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
