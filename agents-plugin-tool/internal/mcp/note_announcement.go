package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsnote"
)

// notesInjectionCap bounds how many non-machine notes the workflow_manual
// "# Notes" block inlines with their body; notesKeyOnlyCap bounds how many
// more it lists key-only before the rest collapse into a "+N hidden" line.
// Visible machine notes are pinned and count against neither. Arbitrary,
// trivially-tunable implementation constants, not contract forks.
const (
	notesInjectionCap = 20
	notesKeyOnlyCap   = 30
)

// computeNotesBlock computes the ambient "# Notes" block for root, merging the
// machine, worktree, clone, and repo layers. Thin wrapper over
// wsnote.Compute, mirroring computeManuals's shape (see
// manuals_announcement.go): pure, root-in, and silent on any resolution error.
func computeNotesBlock(root string) wsnote.Block {
	return wsnote.Compute(root, wsconfig.Options{}, notesInjectionCap, notesKeyOnlyCap)
}

// computeNotes is computeNotesBlock's text, for renders that raise no warning.
func computeNotes(root string) string {
	return computeNotesBlock(root).Text
}

// notesCleanupThrottleFile, in the git common dir, records when the
// hidden-notes cleanup warning last rendered, so it renders at most once per
// notesCleanupInterval per clone (the worktree pool nudge's throttle shape).
const (
	notesCleanupThrottleFile = "ws-notes-cleanup-nudge.json"
	notesCleanupInterval     = 24 * time.Hour
)

// notesCleanupNow is the injectable clock the cleanup throttle reads. Tests
// override it and restore it via t.Cleanup.
var notesCleanupNow = func() time.Time { return time.Now().UTC() }

// notesCleanupWarning returns the hidden-notes cleanup warning for root's
// clone when block hides notes and the clone's daily slot is free, and
// consumes that slot; otherwise it returns "". Only a top-level lead's
// workflow_manual calls this: a worker cannot erase notes, so it must not
// spend the slot.
//
// Accepted residual: a session-start setup call whose response is discarded
// before it reaches the model (a setup that finishes after its caller stopped
// waiting) still consumes the slot. The next day's first lead call shows the
// warning again, so this self-heals.
func notesCleanupWarning(root string, block wsnote.Block, now time.Time) string {
	total := block.HiddenTotal()
	if total == 0 || strings.TrimSpace(root) == "" {
		return ""
	}
	out, err := wsgit.ExecRunner{}.RunGit(context.Background(), root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return ""
	}
	throttlePath := filepath.Join(strings.TrimSpace(string(out)), notesCleanupThrottleFile)
	var last struct {
		ShownAt string `json:"shown_at"`
	}
	if data, err := os.ReadFile(throttlePath); err == nil {
		_ = json.Unmarshal(data, &last) // a corrupt file only loses the throttle
	}
	if at, err := time.Parse(time.RFC3339, last.ShownAt); err == nil && now.Sub(at) < notesCleanupInterval {
		return ""
	}
	last.ShownAt = now.UTC().Format(time.RFC3339)
	if data, err := json.Marshal(last); err == nil {
		tmp := throttlePath + ".tmp"
		if os.WriteFile(tmp, data, 0o644) == nil {
			_ = os.Rename(tmp, throttlePath)
		}
	}

	var parts []string
	for _, layer := range []wsnote.Layer{wsnote.LayerWorktree, wsnote.LayerClone, wsnote.LayerRepo} {
		if n := block.Hidden[layer]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", layer, n))
		}
	}
	return fmt.Sprintf("> **Notes overflow: %d hidden notes** (%s) are past the Notes block's limits, so no session sees even their keys. Clean up now: `note.query` the hidden notes, `note.erase` every stale one, and move still-needed detail into a ticket or a manual. Muting does not fix this.", total, strings.Join(parts, ", "))
}
