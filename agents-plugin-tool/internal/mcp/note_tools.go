package mcp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsnote"
)

// noteStore abstracts the per-layer note storage backend so
// handleNoteWrite/handleNoteErase/handleNoteSearch/handleNoteMute/
// handleNoteUnmute stay layer-agnostic: the machine/worktree layers store one
// JSON file per whole layer (fileNoteStore, wrapping
// wsnote.Load/Write/Erase/SetVisible), while the repo layer stores one JSON
// file per key (repoNoteStore, wrapping
// wsnote.RepoLoad/RepoWrite/RepoErase/RepoSetVisible).
type noteStore interface {
	Load() (map[string]wsnote.Record, error)
	Write(records []wsnote.Record) error
	Erase(keys []string) error
	SetVisible(keys []string, visible bool) error
}

// fileNoteStore implements noteStore over a single whole-layer JSON file, the
// storage shape shared by the machine and worktree layers.
type fileNoteStore struct {
	path string
}

func (f fileNoteStore) Load() (map[string]wsnote.Record, error) { return wsnote.Load(f.path) }
func (f fileNoteStore) Write(records []wsnote.Record) error     { return wsnote.Write(f.path, records) }
func (f fileNoteStore) Erase(keys []string) error               { return wsnote.Erase(f.path, keys) }
func (f fileNoteStore) SetVisible(keys []string, visible bool) error {
	return wsnote.SetVisible(f.path, keys, visible)
}

// repoNoteStore implements noteStore over the tracked repo-layer directory,
// one JSON file per key.
type repoNoteStore struct {
	dir string
}

func (r repoNoteStore) Load() (map[string]wsnote.Record, error) { return wsnote.RepoLoad(r.dir) }
func (r repoNoteStore) Write(records []wsnote.Record) error     { return wsnote.RepoWrite(r.dir, records) }
func (r repoNoteStore) Erase(keys []string) error               { return wsnote.RepoErase(r.dir, keys) }
func (r repoNoteStore) SetVisible(keys []string, visible bool) error {
	return wsnote.RepoSetVisible(r.dir, keys, visible)
}

// resolveNoteStore resolves the noteStore backend for the "layer" argument,
// given the tool's session_key and (worktree/repo layers only) a resolved
// root. Every note.* call requires a valid session_key, matching the "every
// ws tool call carries a session key" invariant, even for the machine layer
// which does not itself need a root:
//   - "worktree" and "repo" both route through resolveToolRoot, the same
//     root-resolution path every other root-aware tool uses.
//   - "machine" looks the key up directly via s.sessions.lookup — it cannot
//     reuse resolveToolRoot (which would require a resolvable root the
//     machine layer does not need) or requireLeadSessionKey (lead-only; note.*
//     is reachable by any scope, mirroring todo.*/agenda.*).
func (s *Server) resolveNoteStore(toolName string, args map[string]any, meta map[string]any) (noteStore, error) {
	layer, err := noteLayerArg(toolName, args)
	if err != nil {
		return nil, err
	}
	switch layer {
	case wsnote.LayerMachine:
		key, _ := args["session_key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s: session_key is required", toolName)
		}
		if _, found := s.sessions.lookup(key); !found {
			return nil, fmt.Errorf("%s: unknown_session: session key not found; "+
				"if you are the lead, re-bootstrap your session per ws:workflow-manual with your known root and retry the call", toolName)
		}
		path, err := wsnote.MachinePath(wsconfig.Options{})
		if err != nil {
			return nil, err
		}
		return fileNoteStore{path: path}, nil
	case wsnote.LayerWorktree:
		root, err := s.resolveToolRoot(args, meta)
		if err != nil {
			return nil, err
		}
		path, err := wsnote.WorktreePath(root)
		if err != nil {
			return nil, err
		}
		return fileNoteStore{path: path}, nil
	case wsnote.LayerRepo:
		root, err := s.resolveToolRoot(args, meta)
		if err != nil {
			return nil, err
		}
		return repoNoteStore{dir: wsnote.RepoDir(root)}, nil
	default:
		return nil, fmt.Errorf("%s: invalid layer %q: want \"machine\", \"worktree\", or \"repo\"", toolName, layer)
	}
}

func noteLayerArg(toolName string, args map[string]any) (wsnote.Layer, error) {
	raw, _ := args["layer"].(string)
	raw = strings.TrimSpace(raw)
	switch wsnote.Layer(raw) {
	case wsnote.LayerMachine:
		return wsnote.LayerMachine, nil
	case wsnote.LayerWorktree:
		return wsnote.LayerWorktree, nil
	case wsnote.LayerRepo:
		return wsnote.LayerRepo, nil
	default:
		return "", fmt.Errorf(`%s: layer is required and must be "machine", "worktree", or "repo"`, toolName)
	}
}

// noteNow is the injectable clock noteRecordsArg stamps WrittenAt from. Tests
// override it to assert exact, controlled instants (e.g. proving a
// full-overwrite re-stamps rather than echoing the prior value) without
// depending on real wall-clock gaps at RFC3339's second granularity, which
// would make a stale-echo regression indistinguishable from a fresh
// same-second re-stamp. Always restore the original via t.Cleanup.
var noteNow = func() time.Time { return time.Now().UTC() }

// noteRecordsArg parses the "notes" argument into []wsnote.Record. Each item
// must be a {"key", "value", "priority"} object (the wire shape is
// array-of-objects, not positional array-of-tuples — the universal MCP
// tool-arg convention in this codebase). WrittenAt is stamped here at write
// time, not supplied by the caller.
func noteRecordsArg(toolName string, args map[string]any) ([]wsnote.Record, error) {
	raw, ok := args["notes"].([]any)
	if !ok {
		return nil, fmt.Errorf(`%s: notes must be an array of {"key","value","priority"} objects`, toolName)
	}
	now := noteNow().Format(time.RFC3339)
	out := make([]wsnote.Record, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: notes[%d] must be an object", toolName, i)
		}
		key, _ := obj["key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s: notes[%d].key is required", toolName, i)
		}
		value, _ := obj["value"].(string)
		priority, err := notePriorityValue(obj["priority"])
		if err != nil {
			return nil, fmt.Errorf("%s: notes[%d].%s", toolName, i, err.Error())
		}
		out = append(out, wsnote.Record{Key: key, Value: value, Priority: priority, WrittenAt: now})
	}
	return out, nil
}

func notePriorityValue(raw any) (int, error) {
	switch v := raw.(type) {
	case nil:
		return 0, nil
	case float64:
		return int(v), nil
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("priority must be an integer: %w", err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("priority must be an integer")
	}
}

func noteKeysArg(toolName string, args map[string]any) ([]string, error) {
	raw, ok := args["keys"].([]any)
	if !ok {
		return nil, fmt.Errorf("%s: keys must be an array of strings", toolName)
	}
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		key, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s: keys[%d] must be a string", toolName, i)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s: keys[%d] must be non-empty", toolName, i)
		}
		out = append(out, key)
	}
	return out, nil
}

func (s *Server) handleNoteWrite(id json.RawMessage, args map[string]any, meta map[string]any) response {
	const tool = "note.write"
	store, err := s.resolveNoteStore(tool, args, meta)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	records, err := noteRecordsArg(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if len(records) == 0 {
		return toolTextResponse(id, "", fmt.Errorf("%s: notes must be a non-empty array", tool))
	}
	if err := store.Write(records); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, records, nil)
	}
	return toolTextResponse(id, formatNoteWrite(records), nil)
}

func (s *Server) handleNoteErase(id json.RawMessage, args map[string]any, meta map[string]any) response {
	const tool = "note.erase"
	store, err := s.resolveNoteStore(tool, args, meta)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	keys, err := noteKeysArg(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if len(keys) == 0 {
		return toolTextResponse(id, "", fmt.Errorf("%s: keys must be a non-empty array", tool))
	}
	if err := store.Erase(keys); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, keys, nil)
	}
	return toolTextResponse(id, formatNoteErase(keys), nil)
}

// handleNoteMute and handleNoteUnmute mirror handleNoteErase's shape exactly:
// resolve the layer's store, parse keys, reject an empty list, and dispatch
// to the store's SetVisible. They never call noteRecordsArg/noteNow — mute
// and unmute set-state on Visible only and must NOT restamp WrittenAt.
func (s *Server) handleNoteMute(id json.RawMessage, args map[string]any, meta map[string]any) response {
	return s.handleNoteSetVisible(id, args, meta, "note.mute", false, formatNoteMute)
}

func (s *Server) handleNoteUnmute(id json.RawMessage, args map[string]any, meta map[string]any) response {
	return s.handleNoteSetVisible(id, args, meta, "note.unmute", true, formatNoteUnmute)
}

func (s *Server) handleNoteSetVisible(id json.RawMessage, args map[string]any, meta map[string]any, tool string, visible bool, format func([]string) string) response {
	store, err := s.resolveNoteStore(tool, args, meta)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	keys, err := noteKeysArg(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if len(keys) == 0 {
		return toolTextResponse(id, "", fmt.Errorf("%s: keys must be a non-empty array", tool))
	}
	if err := store.SetVisible(keys, visible); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, keys, nil)
	}
	return toolTextResponse(id, format(keys), nil)
}

func (s *Server) handleNoteSearch(id json.RawMessage, args map[string]any, meta map[string]any) response {
	const tool = "note.search"
	store, err := s.resolveNoteStore(tool, args, meta)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	records, err := store.Load()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	glob, _ := args["glob"].(string)
	from, _ := args["from"].(string)
	then, _ := args["then"].(string)
	result, err := wsnote.Search(records, strings.TrimSpace(glob), strings.TrimSpace(from), strings.TrimSpace(then))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, result, nil)
	}
	return toolTextResponse(id, formatNoteSearch(result), nil)
}

func formatNoteWrite(records []wsnote.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "wrote %d note(s):\n", len(records))
	for _, rec := range records {
		fmt.Fprintf(&b, "- %s (priority %d)\n", rec.Key, rec.Priority)
	}
	return b.String()
}

func formatNoteErase(keys []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "erased %d note(s):\n", len(keys))
	for _, key := range keys {
		fmt.Fprintf(&b, "- %s\n", key)
	}
	return b.String()
}

func formatNoteMute(keys []string) string   { return formatNoteSetVisible("muted", keys) }
func formatNoteUnmute(keys []string) string { return formatNoteSetVisible("unmuted", keys) }

func formatNoteSetVisible(verb string, keys []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %d note(s):\n", verb, len(keys))
	for _, key := range keys {
		fmt.Fprintf(&b, "- %s\n", key)
	}
	return b.String()
}

func formatNoteSearch(records []wsnote.Record) string {
	if len(records) == 0 {
		return "no notes matched\n"
	}
	var b strings.Builder
	for _, rec := range records {
		fmt.Fprintf(&b, "%s (priority %d, %s): %s\n", rec.Key, rec.Priority, rec.WrittenAt, rec.Value)
	}
	return b.String()
}
