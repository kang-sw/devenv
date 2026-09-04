package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maybePrune removes stale session key record files from the flat keys/
// directory, at most once per pruneScanCadence. It is called from the
// session-bootstrap handler (ferrule) so pruning piggybacks on normal
// activity rather than requiring a background goroutine or a dedicated tool.
//
// Cadence is bookkept with a marker file (pruneMarkerName) rather than an
// in-memory timestamp: the keys/ store is a single flat directory shared by
// every server process across every worktree, so an in-memory guard would not
// prevent concurrent processes from all scanning on their first ferrule call.
// Claiming the marker before scanning means a duplicate scan by a racing
// process is merely wasteful, never harmful (deletes are keyed on mtime, which
// is idempotent to re-derive).
//
// Pruning is mtime-only: a record's JSON body is never parsed to decide
// whether to keep it. This is deliberate — a malformed record still has a
// valid file mtime, so mtime-only pruning cannot be tripped up by unreadable
// content, sidestepping the "malformed files must not crash pruning"
// requirement entirely rather than adding error-handling around it.
func (s *sessionStore) maybePrune() {
	dir, err := s.keysDir()
	if err != nil {
		return
	}

	markerPath := filepath.Join(dir, pruneMarkerName)
	if info, err := os.Stat(markerPath); err == nil {
		if time.Since(info.ModTime()) < pruneScanCadence {
			return
		}
	}

	// Claim the scan: creating/truncating the marker first means a racing
	// process that checks the marker after this point sees a fresh mtime and
	// skips its own scan. O_EXCL is unnecessary here because a duplicate scan
	// is harmless, just wasteful.
	f, err := os.OpenFile(markerPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	f.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > keyRetentionAge {
			_ = os.Remove(path)
		}
	}
}
