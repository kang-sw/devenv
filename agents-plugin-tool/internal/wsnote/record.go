// Package wsnote implements four note-memory layers: the two non-tracked
// layers from 260807 Phase 1 — "machine" (PC-global, project-agnostic; lives
// beside the global ws config file) and "worktree" (worktree-local,
// ephemeral; lives under the existing wsstate worktree cache directory) —
// the git-tracked "repo" layer from 260810 Phase 1, which stores one file
// per key under ai-docs/ws-notes/ so merge conflicts resolve on the
// filesystem with normal git tooling, and the non-tracked "clone" layer from
// 260814 Phase 1 (project-scoped, worktree-agnostic; lives under the
// existing per-project wsstate cache directory, shared across every worktree
// of the same project but never staged by git). All four layers share the
// same record shape; the non-tracked layers (machine/worktree/clone) each
// share one flock-serialized read-modify-write file per layer, while the
// repo layer serializes per-key-file writes independently (see
// repo_store.go).
package wsnote

import "encoding/json"

// Record is one stored note entry: an arbitrary key/value pair with an
// integer priority (higher = more important) and an RFC3339 write
// timestamp. WrittenAt is set by the writer (internal/mcp's note.write
// handler), not by this package, so Load/Write/Erase stay pure storage
// operations with no wall-clock dependency. Visible gates whether the
// record is eligible for the injected "# Notes" block (see inject.go);
// note.mute/note.unmute are the sole intended mutators (via
// SetVisible/RepoSetVisible) — note.write always preserves the existing
// value on an overwrite and initializes true on a brand new key (see
// Write/writeRepoRecordFile), never accepting it from the caller.
type Record struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Priority  int    `json:"priority"`
	WrittenAt string `json:"written_at"`
	Visible   bool   `json:"visible"`
}

// UnmarshalJSON decodes a Record with a migration-safe default for Visible:
// a record stored before this field existed has no "visible" key at all, and
// that absence must read as true (visible). encoding/json's ordinary zero-value
// behavior for a plain bool field defaults an absent key to false — the
// OPPOSITE of the required migration contract — so this decodes "visible"
// through a *bool shadow field instead: nil (key absent, or explicit JSON
// null) defaults to true, while an explicit true/false decodes as given.
func (r *Record) UnmarshalJSON(data []byte) error {
	var shadow struct {
		Key       string `json:"key"`
		Value     string `json:"value"`
		Priority  int    `json:"priority"`
		WrittenAt string `json:"written_at"`
		Visible   *bool  `json:"visible"`
	}
	if err := json.Unmarshal(data, &shadow); err != nil {
		return err
	}
	r.Key = shadow.Key
	r.Value = shadow.Value
	r.Priority = shadow.Priority
	r.WrittenAt = shadow.WrittenAt
	if shadow.Visible == nil {
		r.Visible = true
	} else {
		r.Visible = *shadow.Visible
	}
	return nil
}
