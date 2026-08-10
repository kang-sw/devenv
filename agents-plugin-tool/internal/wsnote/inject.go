package wsnote

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// layeredRecord tags a Record with the layer it was loaded from, purely for
// display in the injected block — the two layers never share a namespace on
// disk, so this is not used for identity/dedup.
type layeredRecord struct {
	Record
	Layer Layer
}

// Compute computes the ambient "# Notes" block: the highest-priority notes
// across both the machine and worktree layers, up to limit items, or "" when
// there are no notes at all. Modeled on computeManuals / scopeAnnouncement in
// package mcp: silent-by-design, never blocks workflow_manual from rendering.
//
// A resolution error on either layer (including an empty root, or a
// worktree-path resolution failure) is treated the same as "no notes on that
// layer" and degrades to whatever the other layer has — mirroring
// scopeAnnouncement's "a resolution error is treated the same as inactive"
// contract — rather than failing the whole block.
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

	if len(all) == 0 {
		return ""
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].Priority != all[j].Priority {
			return all[i].Priority > all[j].Priority
		}
		if all[i].WrittenAt != all[j].WrittenAt {
			return all[i].WrittenAt > all[j].WrittenAt
		}
		return all[i].Key < all[j].Key
	})

	shown := all
	elided := 0
	if limit > 0 && limit < len(all) {
		shown = all[:limit]
		elided = len(all) - limit
	}

	var sb strings.Builder
	sb.WriteString("# Notes\n")
	for _, note := range shown {
		fmt.Fprintf(&sb, "- [%s] %s (priority %d, %s): %s\n", note.Layer, note.Key, note.Priority, note.WrittenAt, note.Value)
	}
	if elided > 0 {
		fmt.Fprintf(&sb, "(%d lower-priority notes elided — use note.search to retrieve.)\n", elided)
	}
	return strings.TrimRight(sb.String(), "\n")
}
