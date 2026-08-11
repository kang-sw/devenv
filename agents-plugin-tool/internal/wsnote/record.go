// Package wsnote implements the two non-tracked note-memory layers (260807
// Phase 1): "machine" (PC-global, project-agnostic; lives beside the global
// ws config file) and "worktree" (worktree-local, ephemeral; lives under the
// existing wsstate worktree cache directory). Both layers share the same
// record shape and the same flock-serialized read-modify-write storage
// pattern; only the resolved file path differs. The tracked "repo" layer is
// out of scope for this phase.
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
