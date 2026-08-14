package wsnote

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// layeredRecord tags a Record with the layer it was loaded from, purely for
// display in the injected block — the four layers never share a namespace
// on disk, so this is not used for identity/dedup.
type layeredRecord struct {
	Record
	Layer Layer
}

// Compute computes the ambient "# Notes" block: the highest-priority notes
// across the machine, worktree, clone, and repo layers, up to limit items,
// or "" when there are no notes at all. Modeled on computeManuals /
// scopeAnnouncement in package mcp: silent-by-design, never blocks
// workflow_manual from rendering.
//
// A resolution error on any layer (including an empty root, which also
// skips the worktree, clone, and repo layers entirely) is treated the same
// as "no notes on that layer" and degrades to whatever the other layers
// have —
// mirroring scopeAnnouncement's "a resolution error is treated the same as
// inactive" contract — rather than failing the whole block.
func Compute(root string, opts wsconfig.Options, limit int) string {
	var all []layeredRecord

	if machinePath, err := MachinePath(opts); err == nil {
		if records, err := Load(machinePath); err == nil {
			for _, rec := range records {
				all = append(all, layeredRecord{Record: rec, Layer: LayerMachine})
			}
		}
	}

	if root != "" {
		if worktreePath, err := WorktreePath(root); err == nil {
			if records, err := Load(worktreePath); err == nil {
				for _, rec := range records {
					all = append(all, layeredRecord{Record: rec, Layer: LayerWorktree})
				}
			}
		}
	}

	if root != "" {
		if clonePath, err := ClonePath(root); err == nil {
			if records, err := Load(clonePath); err == nil {
				for _, rec := range records {
					all = append(all, layeredRecord{Record: rec, Layer: LayerClone})
				}
			}
		}
	}

	if root != "" {
		if records, err := RepoLoad(RepoDir(root)); err == nil {
			for _, rec := range records {
				all = append(all, layeredRecord{Record: rec, Layer: LayerRepo})
			}
		}
	}

	// The empty-skip check stays keyed on the UNFILTERED collection (muted or
	// not, across every layer): this is what distinguishes the all-muted edge
	// (block still renders — heading + muted line, zero bullet lines) from the
	// truly-empty edge (block skipped entirely).
	if len(all) == 0 {
		return ""
	}

	var visible []layeredRecord
	muted := 0
	for _, note := range all {
		if note.Visible {
			visible = append(visible, note)
		} else {
			muted++
		}
	}

	sort.Slice(visible, func(i, j int) bool {
		return CompareRecords(visible[i].Record, visible[j].Record)
	})

	// Muted notes never consume a limit slot: capping/eliding operates only on
	// the visible subset, so muting a note can free a slot for a previously
	// elided visible one.
	shown := visible
	elided := 0
	if limit > 0 && limit < len(visible) {
		shown = visible[:limit]
		elided = len(visible) - limit
	}

	var sb strings.Builder
	sb.WriteString("# Notes\n")
	for _, note := range shown {
		fmt.Fprintf(&sb, "- [%s] %s (priority %d, %s): %s\n", note.Layer, note.Key, note.Priority, note.WrittenAt, note.Value)
	}
	if elided > 0 {
		fmt.Fprintf(&sb, "(%d lower-priority notes elided — use note.search to retrieve.)\n", elided)
	}
	if muted > 0 {
		fmt.Fprintf(&sb, "(%d muted — use note.search to view.)\n", muted)
	}
	return strings.TrimRight(sb.String(), "\n")
}
