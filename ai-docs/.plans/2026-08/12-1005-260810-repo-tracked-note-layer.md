# Plan: 260810-feat-repo-tracked-note-layer — Phase 1: Tracked repo note layer with one-key-per-file storage and injection

## Relevant Ticket Contract

- One key = one file, under `ai-docs/ws-notes/` (tracked), so merge conflicts
  resolve on the filesystem with normal git tooling; no merge/conflict logic
  enters MCP.
- Key→filename encoding is a required planning choice: must round-trip (so
  `note.erase` finds the same file) and be deterministic across clones (same
  key → same filename everywhere); raw keys can contain `/` and `.` and must
  not be used as a path directly.
- No new git-mutation MCP verb; staging/commit of `ai-docs/ws-notes/` files
  rides the caller's normal `git.commit` path.
- Same record shape as the shipped layers: `[key, value, priority,
  written_at]`; `repo`-layer records participate in the existing `# Notes`
  `workflow_manual` injection under the same priority-ordered cap (20) and
  elision line.
- Verification boundary: `note.write(layer: "repo", ...)` writes one tracked
  file per key under `ai-docs/ws-notes/`; the record appears in the next
  `workflow_manual` call's `# Notes` block; the file is genuinely tracked
  (visible to `git status`/a second worktree after commit), unlike
  `machine`/`worktree`.

## Out of Scope

- The `visible`/mute record attribute — owned by
  `260811-feat-note-visibility-mute`; this phase inherits the current
  `[key, value, priority, written_at]` shape unchanged.
- Any git commit/merge automation — writing the file is this layer's job;
  committing it is the caller's ordinary `git.commit` flow, not a new tool.
- `_index.md` `# Session Notes` migration itself —
  `260807-refactor-dissolve-project-index` is the consumer; this phase only
  makes the `repo` layer exist.

## Codebase Findings

- `agents-plugin-tool/internal/wsnote/store.go#L17-L23` — `Layer` is a string
  type with `LayerMachine`/`LayerWorktree` consts; add `LayerRepo = "repo"`
  alongside them.
- `agents-plugin-tool/internal/wsnote/store.go#L29-L48` — `MachinePath`/
  `WorktreePath` are the existing per-layer path resolvers. `RepoDir(root
  string) string` should follow the same naming shape but needs no error
  return (`filepath.Join(root, "ai-docs", "ws-notes")` cannot fail).
- `agents-plugin-tool/internal/wsnote/store.go#L88-L145` — `rmw` is the shared
  flock + temp-file + atomic-rename pattern used by `Write`/`Erase`, keyed on
  a whole-layer JSON file (`map[string]Record`). The repo layer needs a
  **per-key-file** variant of this same pattern (flock on `<file>.lock`,
  `os.CreateTemp` + rename), not a directory-wide RMW — each file is
  independently owned, which is the whole point of "one key = one file"
  filesystem-level conflict resolution. Reuse the write/lock shape, not the
  map-transform shape.
- `agents-plugin-tool/internal/wsnote/inject.go#L29-L80` — `Compute(root,
  opts, limit)` merges `machine` + `worktree` into `layeredRecord` and sorts/
  caps/elides. Add a third merge block for `repo` (guarded on `root != ""`,
  same silent-on-error contract as the worktree block at L40-L48) so it
  participates in the same sort/cap/elision without touching that logic.
- `agents-plugin-tool/internal/mcp/note_tools.go#L25-L51` —
  `resolveNoteStorePath` returns a single file path string per layer
  (`machine`/`worktree` only). This is the one place that must branch to a
  **directory**, not a file, for `repo`. Renaming it to return a small store
  abstraction (see Implementation Plan step 4) keeps `handleNoteWrite`/
  `handleNoteErase`/`handleNoteSearch` (L147-L212) unchanged in shape — they
  already just call `resolve...` then `wsnote.Load/Write/Erase`.
- `agents-plugin-tool/internal/mcp/note_tools.go#L53-L64` — `noteLayerArg`
  validates the `layer` string against the two known layers; add `LayerRepo`
  as a third accepted value here.
- `agents-plugin-tool/internal/mcp/server.go#L4289-L4329` — the three
  `note.write`/`note.erase`/`note.search` schemas each pass a two-element
  `enumStringProperty(..., []string{"machine", "worktree"})` for `layer`, and
  each description text explicitly enumerates the two layers. Both need the
  `"repo"` addition; the schema list is a plain slice literal, not derived
  from `wsnote.Layer` consts anywhere, so it will silently drift if only the
  Go-side consts are updated.
- `agents-plugin-tool/internal/mcp/note_tools_test.go#L13-L50` — existing
  fixtures (`setupNoteTestEnv`, `mintRootKey`, `twoWorktreesOfOneRepo`) are
  directly reusable for a repo-layer test: `mintRootKey` gives a real git
  worktree root, and `git status`/`git worktree add` helpers already exist in
  the test file (used by `twoWorktreesOfOneRepo`) to assert the written file
  is genuinely tracked-location, not just present on disk.
- `ai-docs/spec/mcp-tools.md#L416-L475` (Note Tools) and `#L667-L691` (Note
  Injection) — both currently say "the two non-tracked note-memory layers"
  and "the tracked `repo` layer is out of scope for this phase" (L425-L426).
  Per the ticket's own Spec Impact section, both must be updated to document
  the third layer, its storage shape (one file per key vs. one file per
  layer), and its participation in injection.
- `agents-plugin-tool/internal/wsnote/record.go#L1-L8` — package doc comment
  explicitly states "The tracked 'repo' layer is out of scope for this
  phase." Update on contact since this phase ships it.
- No existing hex/hash key-encoding helper exists anywhere in `wsnote` or
  `wsdoc` to reuse; this is genuinely new, narrow logic (a single pure
  function), not a missed-reuse risk.

## Implementation Plan

1. `agents-plugin-tool/internal/wsnote/store.go` — add `LayerRepo Layer =
   "repo"` next to the existing consts (L17-L23); add `func RepoDir(root
   string) string { return filepath.Join(root, "ai-docs", "ws-notes") }`
   near `MachinePath`/`WorktreePath`.
2. New file `agents-plugin-tool/internal/wsnote/repo_store.go` (keeps
   `store.go` focused on the single-file layers):
   - `func repoKeyFilename(key string) string` — `hex.EncodeToString([]byte(key))
     + ".json"`. Chosen encoding: hex of the raw UTF-8 key bytes. It is a pure
     function of the key (deterministic across every clone/OS/locale — no
     hashing, no timezone/case dependence), fully collision-free (distinct
     keys never hex-encode to the same string), and trivially round-trips for
     `erase` (recomputes the identical filename from the same key). It also
     sidesteps the slash/dot-in-key hazard entirely: hex output contains only
     `[0-9a-f]`, so a slash-bearing or dotted key can never nest a directory
     or collide with `..`/hidden-file handling.
   - `func RepoLoad(dir string) (map[string]Record, error)` — `os.ReadDir`;
     `os.IsNotExist` → return empty non-nil map (mirrors `Load`'s missing-file
     contract). For each entry, skip non-`.json`-suffixed names (excludes
     `.lock` files and `*.tmp` temp-write artifacts), read + `json.Unmarshal`
     into a single `Record` (not a map — this file holds exactly one record),
     and key the result map by `rec.Key` from the file content (content is
     source of truth, not the filename).
   - `func RepoWrite(dir string, records []Record) error` — `os.MkdirAll(dir,
     0o755)`; for each record, write `filepath.Join(dir,
     repoKeyFilename(rec.Key))` via the same flock + `os.CreateTemp` +
     atomic-rename shape as `rmw` (L88-L145 of `store.go`), but scoped to one
     record per file (lock path `<file>.lock`, no map-transform — this is a
     full overwrite of that one file, matching `note.write`'s existing
     full-overwrite-per-key contract). Use `json.MarshalIndent` (2-space, like
     `rmw`) so the tracked file diffs cleanly in git.
   - `func RepoErase(dir string, keys []string) error` — for each key,
     `os.Remove(filepath.Join(dir, repoKeyFilename(key)))`, ignoring
     `os.IsNotExist` (no-op on a missing key, matching `Erase`'s contract).
3. `agents-plugin-tool/internal/wsnote/inject.go` — in `Compute` (L29-L80),
   add a third block after the worktree block (L40-L48):
   ```go
   if root != "" {
       if records, err := RepoLoad(RepoDir(root)); err == nil {
           for _, rec := range records {
               all = append(all, layeredRecord{Record: rec, Layer: LayerRepo})
           }
       }
   }
   ```
   Same silent-on-error, no new branching in the sort/cap/elision logic below.
4. `agents-plugin-tool/internal/mcp/note_tools.go`:
   - `noteLayerArg` (L53-L64): accept `wsnote.LayerRepo` as a third valid
     value.
   - Replace `resolveNoteStorePath` (L25-L51, returns `(string, error)`) with
     `resolveNoteStore(toolName string, args map[string]any, meta
     map[string]any) (noteStore, error)`, where `noteStore` is a small local
     interface:
     ```go
     type noteStore interface {
         Load() (map[string]wsnote.Record, error)
         Write(records []wsnote.Record) error
         Erase(keys []string) error
     }
     ```
     with two unexported implementations: `fileNoteStore{path string}`
     (wraps `wsnote.Load/Write/Erase(path, ...)`, used for `machine` and
     `worktree`, preserving today's `MachinePath`/`WorktreePath` + session/
     root resolution exactly) and `repoNoteStore{dir string}` (wraps
     `wsnote.RepoLoad/RepoWrite/RepoErase(dir, ...)`). The repo branch
     resolves `dir` via `s.resolveToolRoot(args, meta)` +
     `wsnote.RepoDir(root)` — same root-resolution path the worktree branch
     already uses, so no new auth/session logic is introduced.
   - Update `handleNoteWrite`/`handleNoteErase`/`handleNoteSearch`
     (L147-L212) to call `s.resolveNoteStore(...)` and then
     `store.Write(records)` / `store.Erase(keys)` / `store.Load()` in place
     of the current `resolveNoteStorePath` + `wsnote.Write/Erase/Load(path,
     ...)` calls. `noteRecordsArg`/`noteKeysArg`/`wsnote.Search` and all
     formatting helpers stay unchanged — they already operate on
     `[]Record`/`map[string]Record`, not on the path.
5. `agents-plugin-tool/internal/mcp/server.go#L4289-L4329` — add `"repo"` to
   all three `enumStringProperty(..., []string{"machine", "worktree", ...})`
   calls for `note.write`/`note.erase`/`note.search`, and extend each
   description string to mention the third, git-tracked `repo` layer (one
   file per key under `ai-docs/ws-notes/`).
6. Doc hygiene on contact:
   - `agents-plugin-tool/internal/wsnote/record.go#L1-L8` — drop "the tracked
     'repo' layer is out of scope for this phase" from the package doc
     comment; note it now ships as a third layer.
   - `ai-docs/spec/mcp-tools.md#L416-L475` — describe the `repo` layer: git-
     tracked, one file per key under `ai-docs/ws-notes/` (vs. one file per
     *layer* for `machine`/`worktree`), same record shape, no CLI mirror
     change, no new git-mutation verb (staging/commit rides the caller's
     normal `git.commit`). State the key→filename encoding decision (hex of
     the UTF-8 key bytes) since it is now caller-observable behavior (a
     caller inspecting `ai-docs/ws-notes/` sees hex filenames, not literal
     keys).
   - `ai-docs/spec/mcp-tools.md#L667-L691` — update the injection section's
     "across both the `machine` and `worktree` layers" to "across the
     `machine`, `worktree`, and `repo` layers."

## Verification Plan

- `go test ./agents-plugin-tool/internal/wsnote/... ./agents-plugin-tool/internal/mcp/...`
  after adding:
  - `wsnote` package: round-trip test for `RepoWrite`/`RepoLoad`/`RepoErase`
    (write → load returns the record under its original `Key`, not the
    filename; erase removes the file — assert via `os.Stat`); a slash-bearing
    and a dotted key both land as flat files directly under the repo dir
    (no nested subdirectories created); `repoKeyFilename` is deterministic
    (same key → same string across repeated calls, i.e. no host/time
    dependence); `RepoLoad` on a missing directory returns an empty non-nil
    map, matching `Load`'s contract.
  - `mcp` package `note_tools_test.go`: extend the per-layer round-trip test
    (or add a sibling) for `layer: "repo"` using `mintRootKey` — after
    `note.write`, assert a real file exists on disk under
    `<root>/ai-docs/ws-notes/`, and (reusing the git helpers already present
    in this test file, e.g. `runGit`) that `git status --porcelain` on that
    root reports the new file as untracked/added — the ticket's "genuinely
    tracked, unlike machine/worktree" verification boundary, without needing
    a full commit+second-clone round trip. `note.search(layer: "repo")`
    finds it; `note.erase` removes the file from disk.
  - `inject.go`/`note_announcement.go`: extend or add a `Compute` test
    asserting a repo-layer record appears in the `# Notes` output tagged
    `[repo]`, sorts/caps/elides identically alongside machine/worktree
    records, and that an unreadable or absent repo dir degrades silently
    (no error surfaced, no injection).
- Manual/CLI-adjacent check (optional, matches the ticket's cross-clone
  wording): `note.write(layer: "repo", notes: [...])` on a real worktree,
  then `git status` shows a new file at `ai-docs/ws-notes/<hex>.json`.

## Escalations

- None.
