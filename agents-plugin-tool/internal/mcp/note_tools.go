package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsnote"
)

// noteStore abstracts the per-layer note storage backend so
// handleNoteWrite/handleNoteErase/handleNoteSearch/handleNoteMute/
// handleNoteUnmute stay layer-agnostic: the machine/worktree/clone layers
// store one JSON file per whole layer (fileNoteStore, wrapping
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
// storage shape shared by the machine, worktree, and clone layers.
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
// given the tool's session_key and (worktree/clone/repo layers only) a
// resolved root. Every note.* call requires a valid session_key, matching the
// "every ws tool call carries a session key" invariant, even for the machine
// layer which does not itself need a root:
//   - "worktree", "clone", and "repo" all route through resolveToolRoot, the
//     same root-resolution path every other root-aware tool uses.
//   - "machine" looks the key up directly via s.sessions.lookup — it cannot
//     reuse resolveToolRoot (which would require a resolvable root the
//     machine layer does not need) or requireLeadSessionKey (lead-only; note.*
//     is reachable by any scope, mirroring todo.*/agenda.*).
func (s *Server) resolveNoteStore(toolName string, args map[string]any, meta map[string]any) (noteStore, error) {
	layer, err := noteLayerArg(toolName, args)
	if err != nil {
		return nil, err
	}
	return s.resolveNoteStoreForLayer(toolName, layer, args, meta)
}

// resolveNoteStoreForLayer is resolveNoteStore's per-layer resolution body,
// factored out so handleNoteSearch's multi-layer path can resolve a store
// for an already-known wsnote.Layer (from noteSearchLayersArg) without
// re-parsing args["layer"] through noteLayerArg's required-single-string
// contract, which the other four handlers (write/erase/mute/unmute) still
// own unchanged via resolveNoteStore above.
func (s *Server) resolveNoteStoreForLayer(toolName string, layer wsnote.Layer, args map[string]any, meta map[string]any) (noteStore, error) {
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
	case wsnote.LayerClone:
		root, err := s.resolveToolRoot(args, meta)
		if err != nil {
			return nil, err
		}
		path, err := wsnote.ClonePath(root)
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
		return nil, fmt.Errorf("%s: invalid layer %q: want \"machine\", \"worktree\", \"clone\", or \"repo\"", toolName, layer)
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
	case wsnote.LayerClone:
		return wsnote.LayerClone, nil
	case wsnote.LayerRepo:
		return wsnote.LayerRepo, nil
	default:
		return "", fmt.Errorf(`%s: layer is required and must be "machine", "worktree", "clone", or "repo"`, toolName)
	}
}

// allNoteLayers is the full four-layer set note.search searches when "layer"
// is omitted, mirroring wsnote.Compute's own layer aggregation.
var allNoteLayers = []wsnote.Layer{wsnote.LayerMachine, wsnote.LayerWorktree, wsnote.LayerClone, wsnote.LayerRepo}

// noteLayerEnumValues is the shared 4-value layer enum used both for
// argument validation here and (via server.go) for the tool schema.
var noteLayerEnumValues = []string{"machine", "worktree", "clone", "repo"}

func isValidNoteLayer(raw string) bool {
	switch wsnote.Layer(raw) {
	case wsnote.LayerMachine, wsnote.LayerWorktree, wsnote.LayerClone, wsnote.LayerRepo:
		return true
	default:
		return false
	}
}

// noteSearchLayersArg parses note.search's "layer" argument, which — unlike
// the other four note.* tools — is optional and accepts either a single
// string or an array of strings:
//   - absent/nil: search all four layers; tagged = true (multi-layer result,
//     per the ticket's tagging rule).
//   - a single string: search exactly that one layer; tagged = false (sub-
//     decision a: preserves today's plain []wsnote.Record result shape
//     exactly, byte-for-byte, for this the most common call shape).
//   - a non-empty []any of strings: search exactly those layers; tagged =
//     true even for a one-element array, per the ticket's own wording that
//     array results get tagged regardless of length.
//
// Any other shape (empty array, non-string array entries, unknown layer
// name) is a caller error, mirroring noteLayerArg's error text style.
func noteSearchLayersArg(toolName string, args map[string]any) (layers []wsnote.Layer, tagged bool, err error) {
	raw, present := args["layer"]
	if !present || raw == nil {
		return allNoteLayers, true, nil
	}
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		if !isValidNoteLayer(v) {
			return nil, false, fmt.Errorf(`%s: layer must be "machine", "worktree", "clone", or "repo" (or an array of those), or omitted to search all four`, toolName)
		}
		return []wsnote.Layer{wsnote.Layer(v)}, false, nil
	case []any:
		items := stringList(v)
		if len(items) == 0 {
			return nil, false, fmt.Errorf(`%s: layer array must be non-empty and contain only "machine", "worktree", "clone", or "repo"`, toolName)
		}
		out := make([]wsnote.Layer, 0, len(items))
		for _, item := range items {
			item = strings.TrimSpace(item)
			if !isValidNoteLayer(item) {
				return nil, false, fmt.Errorf(`%s: layer array entry %q must be "machine", "worktree", "clone", or "repo"`, toolName, item)
			}
			out = append(out, wsnote.Layer(item))
		}
		return out, true, nil
	default:
		return nil, false, fmt.Errorf(`%s: layer must be "machine", "worktree", "clone", or "repo" (or an array of those), or omitted to search all four`, toolName)
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

// taggedNoteRecord is a Record tagged with the layer it was loaded from, for
// note.search's cross-layer/array result shape (sub-decision a: only this
// tagged shape carries "layer" — a single-string layer call keeps returning
// plain []wsnote.Record). Embedding Record lets it marshal via ordinary
// field promotion with no custom MarshalJSON, mirroring inject.go's
// layeredRecord shape (kept as a separate, mcp-local type since
// layeredRecord itself is private to wsnote and scoped to Compute).
type taggedNoteRecord struct {
	wsnote.Record
	Layer wsnote.Layer `json:"layer,omitempty"`
}

func (s *Server) handleNoteSearch(id json.RawMessage, args map[string]any, meta map[string]any) response {
	const tool = "note.search"
	layers, tagged, err := noteSearchLayersArg(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	glob, _ := args["glob"].(string)
	from, _ := args["from"].(string)
	then, _ := args["then"].(string)
	glob = strings.TrimSpace(glob)
	from = strings.TrimSpace(from)
	then = strings.TrimSpace(then)

	if !tagged {
		// Single-string layer: unchanged behavior — resolve that one store,
		// load, search, and return plain []wsnote.Record exactly as before
		// (now ordered by wsnote.Search's 3-key comparator).
		store, err := s.resolveNoteStoreForLayer(tool, layers[0], args, meta)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		records, err := store.Load()
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		result, err := wsnote.Search(records, glob, from, then)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, result, nil)
		}
		return toolTextResponse(id, formatNoteSearch(result), nil)
	}

	result, err := s.searchNoteLayers(tool, layers, glob, from, then, args, meta)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, result, nil)
	}
	return toolTextResponse(id, formatNoteSearchTagged(result), nil)
}

// searchNoteLayers resolves, loads, and filters each of layers independently
// (a resolution/load failure on one layer is not degraded away here — unlike
// wsnote.Compute's ambient block, note.search is an explicit query the
// caller is entitled to see fail loudly), tags each surviving record with
// its origin layer, and returns the combined set ordered by the same 3-key
// comparator wsnote.Search uses (sub-decision b), so layer:"x" and
// layer:["x"] never diverge in order.
func (s *Server) searchNoteLayers(tool string, layers []wsnote.Layer, glob, from, then string, args, meta map[string]any) ([]taggedNoteRecord, error) {
	var combined []taggedNoteRecord
	for _, layer := range layers {
		store, err := s.resolveNoteStoreForLayer(tool, layer, args, meta)
		if err != nil {
			return nil, err
		}
		records, err := store.Load()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tool, err)
		}
		matched, err := wsnote.FilterRecords(records, glob, from, then)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tool, err)
		}
		for _, rec := range matched {
			combined = append(combined, taggedNoteRecord{Record: rec, Layer: layer})
		}
	}
	sort.Slice(combined, func(i, j int) bool {
		return wsnote.CompareRecords(combined[i].Record, combined[j].Record)
	})
	return combined, nil
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

// formatNoteSearchTagged mirrors formatNoteSearch but prefixes each line
// with "[<layer>] ", matching inject.go's "# Notes" block line style
// ("- [%s] %s (priority %d, %s): %s\n"), for note.search's omitted/array
// layer result (sub-decision a: only the tagged multi-layer shape gets a
// layer prefix; single-string layer calls keep formatNoteSearch's untagged
// line style).
func formatNoteSearchTagged(records []taggedNoteRecord) string {
	if len(records) == 0 {
		return "no notes matched\n"
	}
	var b strings.Builder
	for _, rec := range records {
		fmt.Fprintf(&b, "[%s] %s (priority %d, %s): %s\n", rec.Layer, rec.Key, rec.Priority, rec.WrittenAt, rec.Value)
	}
	return b.String()
}
