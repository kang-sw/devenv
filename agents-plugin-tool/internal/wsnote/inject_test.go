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
