// Package wsnote implements three note-memory layers: the two non-tracked
// layers from 260807 Phase 1 — "machine" (PC-global, project-agnostic; lives
// beside the global ws config file) and "worktree" (worktree-local,
// ephemeral; lives under the existing wsstate worktree cache directory) —
// plus the git-tracked "repo" layer from 260810 Phase 1, which stores one
// file per key under ai-docs/ws-notes/ so merge conflicts resolve on the
// filesystem with normal git tooling. All three layers share the same
// record shape; the non-tracked layers share one flock-serialized
// read-modify-write file per layer, while the repo layer serializes
// per-key-file writes independently (see repo_store.go).
package wsnote

// Record is one stored note entry: an arbitrary key/value pair with an
// integer priority (higher = more important) and an RFC3339 write
// timestamp. WrittenAt is set by the writer (internal/mcp's note.write
// handler), not by this package, so Load/Write/Erase stay pure storage
// operations with no wall-clock dependency.
type Record struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Priority  int    `json:"priority"`
	WrittenAt string `json:"written_at"`
}
