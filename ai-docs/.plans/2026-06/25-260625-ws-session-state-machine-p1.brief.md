# Implementation Brief — 260625-feat-ws-session-state-machine, Phase 1

**Ticket:** `260625-feat-ws-session-state-machine` (ready/)
**Phase:** 1 — MCP primitives and session store
**Branch:** `implement/260625-ws-session-state-machine-p1`
**Worktree:** `/home/swkang/devenv/.claude/worktrees/enumerated-discovering-panda`
**Authored by:** brief fork (source-grounded; anchors verified against current tree)

---

## Objective

Implement the storage layer and all MCP tool handlers for the session state
machine: `ws.agenda.*` (freeform session-level blobs), `ws.enter.*` (typed mode
switches that atomically write an agenda blob and replace the todo list), and
`ws.todo.*` (ordered step-level checklist). Register every tool in
`runtime.json`, add unit tests, and add spec entries to
`ai-docs/spec/mcp-tools.md`.

## Scope boundary (Phase 1 ONLY)

- **In scope:** Go storage layer, MCP schema + dispatch, `runtime.json`
  registration, unit tests, `mcp-tools.md` closeout.
- **Out of scope — do NOT touch:**
  - Phase 2 (skill integration: `lead-proceed`, `lead-implement`,
    `lead-forge-spec`, `lead-forge-mental-model`, `lead-sprint`, `lead-salvage`,
    `delegate-orientation.md`).
  - Phase 3 (workflow-manual "Session State" render section, `ws.commit`
    re-injection, `plugin-runtime.md`/`ws-mcp.md` runbook docs).

---

## Storage backing — RESOLVED (verified against the rebased tree)

The ticket's storage premise **holds**. The earlier "unimplemented" reading was a
stale-tree artifact: the branch was based at `2d17c2ed`, which predated the
session-store code. The branch is now rebased onto epic
`260605-epic-ws-playbook-factory-pivot`, where the code exists.

Ground truth:

1. **`ws.ferrule` is fully implemented.** The JSON-RPC tool name is `ws.ferrule`
   (dot); `ws_ferrule` is only the MCP client-side namespace transform. Tool-name
   const `server.go:52` (`bootstrapToolName = "ws.ferrule"`), dispatched at
   `server.go:367` → `handleLeadLogin` (`server.go:1321`). The "Reserved workflow
   primitive" schema text is deliberate obscurity (rename of the former
   `ws.lead.login`), not a stub marker.
2. **It persists one JSON file per session key.** `handleLeadLogin` →
   `s.sessions.mint(...)` (`server.go:1339`) → `sessionStore.mint`
   (`session_auth.go:113`) writes `<cache-root>/keys/<session-key>.json` via
   `O_CREATE|O_EXCL`. The file — not the process — is the source of truth: a fresh
   MCP instance resolves a key by reading its file (`session_auth.go:69-74`).
   **The D2 path is correct.** `CacheRoot` honors `WS_CACHE_HOME` else
   `~/.cache/ws@…` (`session_auth.go:90-99`, `wsstate/paths.go:84`).
3. **The record schema is versioned and additive.** `sessionRecord`
   (`session_auth.go:46-55`): `{schema_version:1, root, scope, parent?,
   overrides?}`. Unknown fields are ignored; the `Overrides map[string]string`
   field was itself added this exact additive way.

**Decision (locked):** extend `sessionRecord` (`session_auth.go:46`) with two
additive fields — `agenda` (map of key → arbitrary JSON object) and `todos`
(ordered array of `{key,title,status}`) — mirroring how `Overrides` was added.
Reuse the existing atomic writer `writeRecordAtomic` (`session_auth.go:347`, temp
`key-*.tmp` + `os.Rename`, Windows-safe) and reader `readRecord`
(`session_auth.go:329`); follow the read-modify-write shape of `setOverride`
(`session_auth.go:311`). This already satisfies the ticket's D2 atomic
write-and-replace requirement — no new atomic-write code is needed.

**Explicitly NOT used:** no new top-level package (`internal/wssession`), no
`Layout.SessionsDir`, no actor-keying. The `ActorID`/`wsstore` SQLite mechanism
is a separate actor model and is not the session-key store.

**Fix-on-contact:** the Phase-1 ticket *body* still names
`.ws/sessions/<session-key>.json`; correct it to the `keys/<session-key>.json`
record on contact. The Design > Storage section (D2) is authoritative.

---

## Reusable infrastructure already present

- **Per-session record store:** `sessionStore` (`session_auth.go`) owns
  `<cache-root>/keys/<session-key>.json`. Writer `writeRecordAtomic` (`:347`,
  temp `key-*.tmp` + `os.Rename`, Windows-safe remove-then-rename fallback),
  reader `readRecord` (`:329`), and read-modify-write example `setOverride`
  (`:311`). This is exactly the "temp file + rename" atomic guarantee the ticket
  asks for — no new atomic-write code is needed.
- **Cache/layout resolution:** `CacheRoot` (`session_auth.go:90-99`,
  `wsstate/paths.go:84`) honors `WS_CACHE_HOME` / `WS_MCP_*` overrides; tests can
  point it at a temp sandbox dir (see `wsstate/generated_paths_test.go`,
  `wsstore/store_test.go` for the `CacheHome`/`Options` pattern).

## MCP tool addition recipe (from mcp-runtime / plugin-runtime mental models, verified)

Adding each tool requires all of:

1. **Schema** — append an entry to `tools()` (`server.go:2302`), shape:
   `{"name","description","inputSchema":{"type":"object","properties":{...},
   "required":[]string{...}}}`. Use existing helpers `stringProperty(...)`,
   `integerProperty(...)`, and (confirm/​add) a bool/object helper as needed.
2. **Dispatch** — add a `case "<tool>":` in the `callTool` switch
   (`server.go:328-1072`, default error at `:1069`). Root-aware tools resolve via
   `s.resolveToolRoot(params.Arguments, params.Meta)`. Mutations should return
   compact text (`toolTextResponse`); `ws.todo.list` returns rendered text.
3. **`runtime.json`** — add each tool name to the `"tools"` map
   (`agents-plugin/runtime.json:33`) with the next-minor fence. Current version
   is `0.30.0`, fence string `">=0.30.0-dev <0.31.0"`. Per ticket "version fence
   `>=<next-minor>`", new tools use **`">=0.31.0-dev <0.32.0"`**. Confirm the
   minor-bump intent with the lead (the version bump itself is a dev-merge
   concern via `bump-ws-version.sh`, but the per-tool fence string must be set
   here). `runtime.capabilities` derives MCP tool names from `tools()`, but
   `runtime.json` must still be updated by hand.
4. **Profile/visibility** — these tools are session-state primitives. Per D3
   scoping they must be reachable by any agent holding a valid session key (not
   lead-only), so they should NOT be gated behind lead-only profile filters.
   Confirm placement against `toolAllowed` (`server.go:3010`) and
   `filteredTools` (`server.go:2954`); add visibility tests if any profile gate
   applies. They are not agent-backed, so they are not hidden in wsflow
   no-agent mode.

## API surface to implement (storage-location independent)

All mutations return void (compact confirmation text); `ws.todo.list` returns
rendered text.

**Agenda (freeform):**
- `ws.agenda.set(key: string, value: object)` — upsert blob under `key`.
- `ws.agenda.clear(key: string)` — remove blob.

**Enter (typed; each atomically: store typed payload as agenda blob AND derive +
REPLACE the entire todo list):**
- `ws.enter.implement(delegation, plan_depth, branch_mode, review_alloc,
  current_branch, merge_target, start_commit, active_agents)`
- `ws.enter.proceed(ticket, phase, next_skill, conditions)`
- `ws.enter.sprint(episode_slug, episode_start, current_edit_context)`
- `ws.enter.salvage(failure_claim, confirmed_premises, survey_status)`

Calling any enter tool is always a mode switch — the prior derived todo list is
discarded.

**Todo:**
- `ws.todo.append(key, title)`
- `ws.todo.insert_before(ref_key, key, title)`
- `ws.todo.insert_after(ref_key, key, title)`
- `ws.todo.check(key, status: "pending"|"wip"|"done"|"defer")`
- `ws.todo.erase(key)`
- `ws.todo.clear(done_only: bool = false)`
- `ws.todo.list(mode: "summary"|"full" = "summary")` → rendered text
- `ws.todo.reorder(span: {from_key, to_key}, position: {before: ref_key}|{after: ref_key})`

**Identity / validation rules:**
- `key` is caller-provided, unique within the active list; duplicate `key` →
  error. Erased keys are reusable.
- `clear(done_only=false)` removes all; `done_only=true` removes only `done`
  (leaves pending/wip/defer).
- `reorder` moves contiguous span `[from_key … to_key]` as a block.

**Rendering:** pending `- [ ]`, wip `- [~]`, done `- [x]`, defer `- [>]`.
Summary mode = every pending+wip item, plus one adjacent context item on each
side of each contiguous active block; everything else collapses to `...`;
`defer` collapses like `done`. Checkpoint injection always uses summary mode.

**enter.implement todo derivation (verified against ticket):**
- Always: Route, Prep, Edit, Final action gate, Merge.
- `need_review=true` → add Review.
- `need_doc=true` → add Doc pre-pass, Doc commit gate, Doc closeout.
- Derivation logic lives in Go (no skill-side `ws.todo.append` loop).

## Stored schema (fields added to the chosen session-state JSON)

```json
{
  "agenda": { "<key>": { /* arbitrary object */ } },
  "todos": [
    { "key": "<short-stem>", "title": "...", "status": "pending|wip|done|defer" }
  ]
}
```

Fields omitted when empty. Single writer (the session actor); concurrent reads
safe via atomic replace.

## Concrete file-level change targets

- `agents-plugin-tool/internal/mcp/server.go`
  - `tools()` (`:2302`) — add schema entries for all `ws.agenda.*`,
    `ws.enter.*`, `ws.todo.*` tools.
  - `callTool` switch (`:361-1069`) — add dispatch cases for each.
  - Possibly a new property helper if an object/bool/array property helper is
    missing (confirm `stringProperty`/`integerProperty` neighbors during survey).
- `agents-plugin-tool/internal/mcp/session_auth.go`
  - Extend `sessionRecord` (`:46`) with additive `Agenda`/`Todos` fields (JSON
    tags `agenda`/`todos`, `omitempty`), mirroring `Overrides`.
  - Add agenda/todo mutation helpers on `sessionStore` following `setOverride`
    (`:311`) — read-modify-write via `readRecord`/`writeRecordAtomic`.
- **New file** `agents-plugin-tool/internal/mcp/session_state.go` (or similar,
  SAME `internal/mcp` package) for the PURE logic — todo list type,
  enter-derivation, summary/full rendering, reorder, key-uniqueness validation —
  kept separable from disk I/O for table-testability (Code Standards #4). No new
  top-level package.
- `agents-plugin/runtime.json` (`:33` tools map) — register every new tool with
  the next-minor fence.
- `ai-docs/spec/mcp-tools.md` — new section (see closeout).
- Tests — new `*_test.go` (see test plan).

## Unit-test plan

Pure-logic table tests (no MCP round-trip needed) for:
- **Key uniqueness** — duplicate `append`/`insert_*` key errors; erased key
  reusable.
- **enter derivation per flag combo** — `implement` with each of {need_review,
  need_doc} × {true,false} produces exactly the expected ordered todo set;
  enter replaces (not appends to) any prior list.
- **reorder correctness** — contiguous span moves as a block before/after a
  ref_key; span endpoints, single-item span, and invalid ref handling.
- **rendering** — markers per status; summary-mode collapse with the one-adjacent
  -context-item rule; defer collapses like done; full mode shows all.
- **concurrent write safety** — atomic replace leaves a complete, parseable file
  under concurrent writers/readers (mirror the `replaceFile`/temp-rename
  guarantee; can exercise via the storage helper directly).

Integration test (MCP dispatch) in the `server_test.go` style:
`NewServer(root, "test").ServeStdio(ctx, strings.NewReader(<JSONL>), &buf)` then
decode responses by id and assert `toolText(t, byID["n"])` — confirm each new
tool is reachable and a happy-path enter→list flow renders as expected. Set
`CacheHome`/`WS_CACHE_HOME` to a temp dir so the store writes under a sandbox.

## Spec closeout

Add a section to `ai-docs/spec/mcp-tools.md` (heading style `## Title {#anchor}`,
anchor e.g. `{#260625-session-state-tools}`) documenting `ws.agenda.*`,
`ws.enter.*`, and `ws.todo.*`: behavior, identity/validation rules, derivation
table, rendering markers, summary-mode rule, and the storage location finally
chosen. Do not copy live JSON schema verbatim (per reference-document-ownership
rule — schema lives in `tools()`); describe durable behavior.

## Verification

- `go build ./...` and `go test ./internal/...` from `agents-plugin-tool/`.
- Integration probe: confirm all new tools appear in `tools/list` and dispatch
  without "unknown tool".
- `runtime.capabilities` / launcher contract: new tool names present in
  `runtime.json` so the compatibility check passes.

## Open survey questions (for plan-populator-survey)

1. ~~Storage backing decision~~ — **RESOLVED:** extend `sessionRecord`
   (`session_auth.go:46`) with additive `agenda`/`todos`, reuse
   `writeRecordAtomic`/`readRecord`. `key` semantics = the `ws.ferrule` session
   key (the `keys/<session-key>.json` record), not an actor id.
2. **proceed / sprint / salvage todo-derivation rules** — must be derived from
   `lead-proceed`, `lead-sprint`, `lead-salvage` playbooks (read via
   `playbook.print` / bundled prompts). Do NOT invent. Only `enter.implement`
   derivation is specified by the ticket.
3. ~~New package vs. extend `wsstate`~~ — **RESOLVED:** no new package; extend
   `sessionRecord` in `internal/mcp/session_auth.go`, pure logic in a sibling
   file in the same package. No `upsertJSON`/`wsstate` involvement.
4. **Object/bool/array property helpers** — confirm what `tools()` schema helpers
   exist beside `stringProperty`/`integerProperty`; `ws.enter.*` and
   `ws.todo.reorder` need object/array/bool-shaped params.
5. **Version fence** — confirm next-minor (`0.31.0`) is the intended fence for
   these tools and coordinate the bump surface (`bump-ws-version.sh`) at merge.
