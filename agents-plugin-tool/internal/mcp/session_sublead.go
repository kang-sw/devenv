package mcp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// A sub-lead is a lead-scoped key minted by a render of a `role: sublead`
// playbook (childRoleForPlaybookRole). It carries a marker naming the
// minting playbook, which admits it to a status-only path onto its
// top-level lead's whiteboard (subleadBoardFor) and attributes what it
// posts there.
//
// The marker is its own file in the key directory, outside recordParts:
// writeParts rewrites and removes only part files, so a key.json rewrite by
// any ws version, an older daemon included, never drops it. An older daemon
// never opens the file and refuses the key at the top-level-lead gate, as
// it refuses every spawned key. Only removing the whole key directory
// (prune after the retention age, or a failed claim) removes it.
const subleadMarkerName = "sublead.json"

// subleadPlaybookRole is the playbook frontmatter role whose render mints a
// marked sub-lead key.
const subleadPlaybookRole = "sublead"

// subleadMarker is the marker file's shape.
type subleadMarker struct {
	Playbook string `json:"playbook"`
}

// subleadCacheEntry is one key's memoized marker read (sessionStore.sublead):
// ok false caches an unmarked key too, so a parented non-sub-lead's
// whiteboard calls also skip the file.
type subleadCacheEntry struct {
	playbook string
	ok       bool
}

// subleadMaxDepth bounds the Parent walk from a sub-lead to its top-level
// lead, so a corrupt parent chain cannot loop.
const subleadMaxDepth = 16

// writeSubleadMarker marks key as a sub-lead minted by playbook. It runs
// right after the mint, before the key is handed out.
func (s *sessionStore) writeSubleadMarker(key, playbook string) error {
	d, err := s.dirs()
	if err != nil {
		return err
	}
	if !sessionKeyFilenamePattern.MatchString(key) {
		return errSessionNotFound
	}
	payload, err := json.Marshal(subleadMarker{Playbook: playbook})
	if err != nil {
		return err
	}
	if err := s.writeFileAtomic(filepath.Join(d.keyDir(key), subleadMarkerName), payload); err != nil {
		return err
	}
	playbook = strings.TrimSpace(playbook)
	s.sublead.Store(key, subleadCacheEntry{playbook: playbook, ok: playbook != ""})
	return nil
}

// subleadPlaybook returns the playbook that minted key as a sub-lead; ok is
// false for an unmarked key or an unreadable marker.
//
// The marker is written once, before the key is handed out, and leaves only
// with the whole key directory, so the first read is memoized for the
// process: steady-state whiteboard calls from a parented lead open no file.
// A cached answer cannot go stale for a live key; a key name re-claimed
// after a prune drops its entry in claimKey. Only an absent or malformed
// marker is cached as unmarked; any other read failure (a transient sharing
// violation) is retried on the next call.
func (s *sessionStore) subleadPlaybook(key string) (string, bool) {
	if v, ok := s.sublead.Load(key); ok {
		entry := v.(subleadCacheEntry)
		return entry.playbook, entry.ok
	}
	d, err := s.dirs()
	if err != nil || !sessionKeyFilenamePattern.MatchString(key) {
		return "", false
	}
	entry := subleadCacheEntry{}
	data, err := wsstate.ReadFileOnce(filepath.Join(d.keyDir(key), subleadMarkerName))
	switch {
	case err == nil:
		var marker subleadMarker
		if json.Unmarshal(data, &marker) == nil && strings.TrimSpace(marker.Playbook) != "" {
			entry = subleadCacheEntry{playbook: strings.TrimSpace(marker.Playbook), ok: true}
		}
	case !errors.Is(err, fs.ErrNotExist):
		return "", false
	}
	s.sublead.Store(key, entry)
	return entry.playbook, entry.ok
}

// subleadBoard is a marked sub-lead admitted to its top-level lead's board.
type subleadBoard struct {
	// author is the marker's playbook name, the attribution of every log
	// line and panel recap the sub-lead posts.
	author string
	// boardKey is the top-level lead's key the Parent walk resolved; the
	// write, the board refresh, and the dashboard nudge all target it.
	boardKey string
}

// subleadBoardFor resolves a marked sub-lead key to its top-level lead's
// board by walking Parent. ok is false for an unmarked key (a worker, an
// agent-card child), a non-lead key, and a chain that does not end at a live
// top-level lead.
func (s *Server) subleadBoardFor(key string, entry sessionEntry) (subleadBoard, bool) {
	key = strings.TrimSpace(key)
	if entry.scope != roleLead || entry.parent == "" {
		return subleadBoard{}, false
	}
	author, marked := s.sessions.subleadPlaybook(key)
	if !marked {
		return subleadBoard{}, false
	}
	current := entry
	for depth := 0; depth < subleadMaxDepth; depth++ {
		parentKey := current.parent
		parent, found := s.lookupSession(parentKey)
		if !found {
			return subleadBoard{}, false
		}
		if parent.isTopLevelLead() {
			return subleadBoard{author: author, boardKey: parentKey}, true
		}
		current = parent
	}
	return subleadBoard{}, false
}
