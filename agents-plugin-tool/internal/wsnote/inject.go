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

// Block is the rendered ambient "# Notes" block plus the per-layer count of
// notes it hides entirely (past both display tiers), so a caller can raise a
// cleanup warning without re-reading the stores.
type Block struct {
	Text   string
	Hidden map[Layer]int
}

// HiddenTotal sums Hidden across layers.
func (b Block) HiddenTotal() int {
	total := 0
	for _, n := range b.Hidden {
		total += n
	}
	return total
}

// Compute computes the ambient "# Notes" block across the machine, worktree,
// clone, and repo layers. Modeled on computeManuals in package mcp:
// silent-by-design, never blocks workflow_manual from rendering.
//
// Visible machine notes are pinned: they render first, with their body, and
// are exempt from both tiers and from the hidden count. Visible non-machine
// notes, in CompareRecords order, render the first bodyLimit with their body,
// the next keyLimit as one key-only line, and the rest as a "+N hidden" line.
// Muted notes on every layer stay out of the listing and the tiers and show
// only in the muted-count line.
//
// A resolution error on any layer (including an empty root, which also
// skips the worktree, clone, and repo layers entirely) is treated the same
// as "no notes on that layer" and degrades to whatever the other layers
// have, rather than failing the whole block.
func Compute(root string, opts wsconfig.Options, bodyLimit, keyLimit int) Block {
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
	// truly-empty edge (which now also renders — a deliberate divergence from
	// sibling injections' silent-when-empty contract: "# Notes" always
	// renders so the empty-state hint is a discoverable, always-in-context
	// affordance rather than something the caller must already know to ask
	// for).
	if len(all) == 0 {
		return Block{Text: "# Notes\n_No notes. Notes are your short, always-in-context post-it reminders — `note.write` to pin one, `note.erase` when it's no longer needed._"}
	}

	var machine, rest []layeredRecord
	muted := 0
	for _, note := range all {
		switch {
		case !note.Visible:
			muted++
		case note.Layer == LayerMachine:
			machine = append(machine, note)
		default:
			rest = append(rest, note)
		}
	}
	byRecord := func(notes []layeredRecord) {
		sort.Slice(notes, func(i, j int) bool {
			return CompareRecords(notes[i].Record, notes[j].Record)
		})
	}
	byRecord(machine)
	byRecord(rest)

	bodyLimit = max(bodyLimit, 0)
	keyLimit = max(keyLimit, 0)
	withBody := rest[:min(bodyLimit, len(rest))]
	keyOnly := rest[len(withBody):min(bodyLimit+keyLimit, len(rest))]
	hidden := rest[len(withBody)+len(keyOnly):]

	var sb strings.Builder
	sb.WriteString("# Notes\n")
	for _, note := range append(append([]layeredRecord{}, machine...), withBody...) {
		fmt.Fprintf(&sb, "- [%s] %s (priority %d, %s): %s\n", note.Layer, note.Key, note.Priority, note.WrittenAt, note.Value)
	}
	if len(keyOnly) > 0 {
		keys := make([]string, 0, len(keyOnly))
		for _, note := range keyOnly {
			keys = append(keys, fmt.Sprintf("[%s] %s", note.Layer, note.Key))
		}
		fmt.Fprintf(&sb, "Key only (note.query for the body): %s\n", strings.Join(keys, ", "))
	}
	block := Block{}
	if len(hidden) > 0 {
		block.Hidden = map[Layer]int{}
		for _, note := range hidden {
			block.Hidden[note.Layer]++
		}
		fmt.Fprintf(&sb, "+%d hidden — note.query lists them.\n", len(hidden))
	}
	if muted > 0 {
		fmt.Fprintf(&sb, "(%d muted — use note.query to view.)\n", muted)
	}
	sb.WriteString("_Post-it reminders, higher priority number first: erase stale ones immediately with `note.erase`._\n")
	block.Text = strings.TrimRight(sb.String(), "\n")
	return block
}
