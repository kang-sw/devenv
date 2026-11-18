package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	return s.writeFileAtomic(filepath.Join(d.keyDir(key), subleadMarkerName), payload)
}

// subleadPlaybook returns the playbook that minted key as a sub-lead; ok is
// false for an unmarked key or an unreadable marker.
func (s *sessionStore) subleadPlaybook(key string) (string, bool) {
	d, err := s.dirs()
	if err != nil || !sessionKeyFilenamePattern.MatchString(key) {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(d.keyDir(key), subleadMarkerName))
	if err != nil {
		return "", false
	}
	var marker subleadMarker
	if json.Unmarshal(data, &marker) != nil || strings.TrimSpace(marker.Playbook) == "" {
		return "", false
	}
	return strings.TrimSpace(marker.Playbook), true
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
