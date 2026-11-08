package mcp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maybePrune removes stale session key records from both layouts of the key
// store (session_layout.go), at most once per pruneScanCadence. It is called
// from the session-bootstrap handler (ferrule) so pruning piggybacks on
// normal activity rather than requiring a background goroutine or a
// dedicated tool.
//
// Cadence is bookkept with a marker file (pruneMarkerName) rather than an
// in-memory timestamp: the key store is shared by every server process across
// every worktree, so an in-memory guard would not prevent concurrent processes
// from all scanning on their first ferrule call. Claiming the marker before
// scanning means a duplicate scan by a racing process is merely wasteful,
// never harmful (deletes are keyed on mtime, which is idempotent to
// re-derive).
//
// The same pass reaps mailbox history directories idle past the same age
// (pruneMailboxHistory), so stored message bodies do not outlive the key
// records that could reach them.
//
// Pruning is mtime-only: a record's JSON body is never parsed to decide
// whether to keep it. This is deliberate — a malformed record still has a
// valid file mtime, so mtime-only pruning cannot be tripped up by unreadable
// content, sidestepping the "malformed files must not crash pruning"
// requirement entirely rather than adding error-handling around it. Legacy
// record files age out here too; there is no bulk migration.
func (s *sessionStore) maybePrune() {
	d, err := s.dirs()
	if err != nil {
		return
	}

	markerPath := filepath.Join(d.sessions, pruneMarkerName)
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

	now := time.Now()
	pruneSessionDirs(d.sessions, now)
	pruneLegacyRecords(d.legacy, now)
	pruneMailboxHistory(now)
}

// pruneSessionDirs removes, whole, every key directory whose key.json mtime is
// older than keyRetentionAge. A directory with no key.json (a claim or a
// migration a crash cut short) goes by its own mtime instead, which every
// file created or renamed in it refreshes. A name that is not a path-safe key
// is not the store's and is left alone. Errors are ignored.
func pruneSessionDirs(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !sessionKeyFilenamePattern.MatchString(entry.Name()) {
			continue
		}
		keyDir := filepath.Join(dir, entry.Name())
		info, err := os.Stat(filepath.Join(keyDir, partKey.fileName()))
		if errors.Is(err, fs.ErrNotExist) {
			info, err = os.Stat(keyDir)
		}
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > keyRetentionAge {
			_ = os.RemoveAll(keyDir)
		}
	}
}

// pruneLegacyRecords removes every legacy record file (keys/<key>.json) whose
// mtime is older than keyRetentionAge. A missing legacy root is a no-op.
func pruneLegacyRecords(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
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

// pruneMailboxHistory removes per-session mailbox history directories whose
// mtime is older than keyRetentionAge, mtime-only like the key records:
// creating, renaming, and removing entry files refresh the directory's mtime,
// so an active ring is never reaped. Errors are ignored.
func pruneMailboxHistory(now time.Time) {
	root, err := mailboxHistoryRoot()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > keyRetentionAge {
			_ = os.RemoveAll(path)
		}
	}
}
