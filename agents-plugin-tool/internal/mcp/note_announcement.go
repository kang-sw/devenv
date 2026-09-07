package mcp

import (
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsnote"
)

// notesInjectionCap bounds how many highest-priority notes are inlined into
// the workflow_manual "# Notes" block before the remainder are elided behind
// a visible count (still retrievable via note.query). An arbitrary,
// trivially-tunable implementation constant, not a contract fork (260807
// Phase 1 plan).
const notesInjectionCap = 20

// computeNotes computes the ambient "# Notes" block for root, merging the
// machine, worktree, clone, and repo layers. Thin wrapper over
// wsnote.Compute, mirroring computeManuals's shape (see
// manuals_announcement.go): pure, root-in string-out, silent ("") on any
// resolution error or empty result.
func computeNotes(root string) string {
	return wsnote.Compute(root, wsconfig.Options{}, notesInjectionCap)
}
