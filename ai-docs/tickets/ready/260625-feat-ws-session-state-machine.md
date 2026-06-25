---
title: ws session state machine — agenda and todo persistence across compaction
related-mental-model:
  - mcp-runtime
  - plugin-runtime
spec:
  - mcp-tools
sage-review: completed
---

# ws session state machine — agenda and todo persistence across compaction

## Background

After context compaction, routing and implementation context is lost from the
session transcript. The session key (returned by `ws_ferrule`) is the only
stable anchor that survives compaction, because it appears in the compaction
summary. Session key survival across compaction is accepted best-effort risk;
fork-driven operation minimizes compaction frequency in practice.

Workflow manual reload happens after compaction, but there is no mechanism to
restore in-flight mode context (routing decisions, implementation choices) or
task lists built up before the compaction boundary.

The fix is a session-keyed state store on disk, added as fields on the existing
per-session record file (`<cache-root>/keys/<session-key>.json`) that
`ws.ferrule` already mints, holding two namespaces:

- **agenda** — named blobs recording session-level mode context ("what are we
  doing and why"); reminded at workflow-manual load only.
- **todos** — ordered step-level task list; injected at every major checkpoint.

Mode transitions are recorded via typed `ws.enter.*` MCP tools, which
atomically update the agenda blob and replace the todo list with items derived
from the transition parameters.

## Design

### Storage

Agenda and todo state is stored as additional fields in the existing session
key backing file at `<cache-root>/keys/<session-key>.json`, merging into the
file that `ws_ferrule` already manages. No separate file path is introduced.

Added fields:

```json
{
  "agenda": {
    "<key>": { /* arbitrary object */ }
  },
  "todos": [
    { "key": "<short-stem>", "title": "...", "status": "pending|wip|done|defer" }
  ]
}
```

Fields are omitted when empty. Writes use atomic write-and-replace (temp file +
rename) to ensure consistency for concurrent readers such as the ws dashboard.
The session actor is the sole writer; concurrent reads are safe.

### Agenda API (generic, freeform)

- `ws.agenda.set(key: string, value: object)` — upsert agenda blob under `key`.
- `ws.agenda.clear(key: string)` — remove the blob for `key`.

These are fallback primitives for cases not covered by typed `ws.enter.*` tools
(novel modes, supplementary notes). Typed enter tools call `ws.agenda.set`
internally.

**Restoration point:** workflow manual load only — agenda blobs are shown as a
remind section; they are not re-injected at intermediate checkpoints.

### Enter API (typed MCP tools, hidden behind ToolSearch)

Each `ws.enter.*` tool atomically:
1. Stores a typed, schema-validated payload as an agenda blob.
2. Derives appropriate todo items from the payload flags and replaces the entire
   current todo list with those items.

Because enter tools replace the todo list, calling an enter tool is always a
mode switch — any previously derived todo list from a prior mode is discarded.

Available tools:

- `ws.enter.implement(delegation, plan_depth, branch_mode, review_alloc,
  current_branch, merge_target, start_commit, active_agents)` — enter implement
  mode.
- `ws.enter.proceed(ticket, phase, next_skill, conditions)` — enter routing mode.
- `ws.enter.sprint(episode_slug, episode_start, current_edit_context)` — enter
  sprint episode mode.
- `ws.enter.salvage(failure_claim, confirmed_premises, survey_status)` — enter
  salvage mode.

**Todo auto-derivation (ws.enter.implement example):**
- Always included: Route, Prep, Edit, Final action gate, Merge.
- `need_review=true` → Review step included.
- `need_doc=true` → Doc pre-pass, Doc commit gate, Doc closeout steps included.

The derivation logic lives in Go; no skill-side loop over `ws.todo.append` is
needed when an enter tool covers the mode.

### Todo API

Item identity is a caller-provided `key` (short stem such as `"route-verdict"`).
Keys must be unique within the active list; a duplicate key raises an error.
Erased keys may be reused. All mutation calls return void.

```
ws.todo.append(key, title)
ws.todo.insert_before(ref_key, key, title)
ws.todo.insert_after(ref_key, key, title)
ws.todo.check(key, status: "pending"|"wip"|"done"|"defer")
ws.todo.erase(key)
ws.todo.clear(done_only: bool = false)
ws.todo.list(mode: "summary"|"full" = "summary")
ws.todo.reorder(span: {from_key, to_key}, position: {before: ref_key}|{after: ref_key})
```

`clear(done_only=false)` removes all items; `done_only=true` removes only `done`
items (leaves `pending`, `wip`, `defer`).

`reorder` moves a contiguous span `[from_key … to_key]` as a block before or
after `ref_key`. Useful when `ws.enter.*` auto-appended items need repositioning
relative to manually managed items.

**Rendering markers:**

| Status  | Marker |
|---------|--------|
| pending | `- [ ]` |
| wip     | `- [~]` |
| done    | `- [x]` |
| defer   | `- [>]` |

**Summary mode (default):** shows all `pending` and `wip` items. For each
contiguous block of non-done/non-defer items, one adjacent item on each side
(done or defer) is shown for context. All other items are collapsed to `...`.
`defer` collapses the same as `done` in summary mode.

Checkpoint injection always uses summary mode. Explicit `ws.todo.list()` defaults
to summary; pass `mode: "full"` for the complete ordered list.

### Scoping

Agenda and todo state is accessible to any agent that holds a valid session key,
not restricted to the lead session. This enables the lead to populate a todo list
via `ws.enter.*` and pass the session key to a delegate, which can then read,
update, or extend the list during its own execution. Each child actor mints its
own session key via `ws_ferrule`; its agenda/todo state is independent unless
the lead explicitly shares its own session key.

### Behavioral separation rule

| Layer  | Restoration point                                    | Mechanism                         |
|--------|------------------------------------------------------|-----------------------------------|
| agenda | workflow manual load only                            | remind (show blobs at bottom of Session State section) |
| todo   | workflow manual load + `ws.commit` + major checkpoints | inject (summary mode render)    |

`ws.commit` does not auto-mark todos as done. Status transitions are always
explicit via `ws.todo.check`.

## Phases

### Phase 1: MCP primitives and session store

Implement the storage layer and all MCP tool handlers in
`agents-plugin-tool/internal/mcp/`:

- Additive `agenda`/`todos` fields on the existing `sessionRecord`
  (`<cache-root>/keys/<session-key>.json`), reusing its atomic temp+rename writer.
- `ws.agenda.set`, `ws.agenda.clear` handlers.
- `ws.enter.implement`, `ws.enter.proceed`, `ws.enter.sprint`,
  `ws.enter.salvage` handlers; each validates typed parameters, stores agenda
  blob, and derives + replaces todo list.
- `ws.todo.append`, `ws.todo.insert_before`, `ws.todo.insert_after`,
  `ws.todo.check`, `ws.todo.erase`, `ws.todo.clear`, `ws.todo.list`,
  `ws.todo.reorder` handlers.
- `runtime.json` registration for all new tools with version fence
  `>=<next-minor>`.
- Unit tests covering: concurrent write safety, key uniqueness enforcement,
  enter-tool todo derivation per flag combination, reorder correctness.
- Note: todo derivation rules for `ws.enter.proceed`, `ws.enter.sprint`, and
  `ws.enter.salvage` should be derived from the respective skill playbooks
  (`lead-proceed.md`, `lead-sprint.md`, `lead-salvage.md`) during implementation.

Spec closeout: add `ws.agenda.*`, `ws.enter.*`, and `ws.todo.*` tool entries to
`ai-docs/spec/mcp-tools.md`.

Verify via: integration probe confirming all MCP handlers are reachable through
`runtime.json`; unit tests for concurrent write safety, key uniqueness,
enter-tool derivation per flag combination, reorder correctness.

### Result (54f94a53)

Landed the full Phase 1 session state surface: 14 MCP tools — 2 agenda
(`ws.agenda.set`, `ws.agenda.clear`) + 4 enter (`ws.enter.implement`,
`ws.enter.proceed`, `ws.enter.sprint`, `ws.enter.salvage`) + 8 todo
(`ws.todo.append`, `insert_before`, `insert_after`, `check`, `erase`, `clear`,
`list`, `reorder`).

- Storage: additive `agenda` (map) and `todos` (list) fields on the existing
  `sessionRecord` at `<cache-root>/keys/<session-key>.json`, reusing the record's
  atomic temp+rename writer; empty fields omitted, unknown fields ignored. No
  separate store/package introduced (supersedes the brief's Option A after source
  verification showed ferrule/sessionStore already provide the primitives).
- Pure list logic (derivation/append/insert/check/erase/clear/reorder/render,
  key-uniqueness) kept free of disk I/O in `session_state.go`; store-bound atomic
  read-modify-write wrappers and MCP handlers layered on top.
- All 14 tools registered in both `runtime.json` files (ws and wsflow) under the
  version fence `>=0.30.8-dev <0.31.0`.
- Spec section `## Session State Tools {#260625-session-state-tools}` added to
  `ai-docs/spec/mcp-tools.md` (14 tools, storage path, render markers,
  derivations).
- 15 tests passing: pure-logic derivation tables per flag combo, key
  uniqueness/reuse, reorder, summary/full rendering, store concurrency + agenda
  round-trip + enter-replaces-todos, an MCP integration flow, and handler-layer
  arg-parse coverage.

> Forward: version fence used the current-line `>=0.30.8-dev <0.31.0` instead of
> the ticket's literal `>=<next-minor>` (correct: a next-minor fence breaks
> launcher compatibility against the current binary and the wsflow exact-match
> contract; the minor bump is a dev-merge step, not a Phase 1 deliverable).
> Forward (Phase 2): `ws.enter.implement` todo derivation is driven ONLY by the
> `need_review` and `need_doc` booleans, which the ticket's Phase 2 param tuple
> omits; Phase 2 integration must pass them (`need_review` = review-allocation
> != lead-only; `need_doc` = true for the standard pipeline) or the derived list
> is wrong. Also every `ws.enter.*` / `ws.todo.*` tool requires `session_key`,
> which is absent from the ticket tuples.
> Forward (Phase 3): `ws.todo.list` does not expose item keys; a key-surface
> affordance may be wanted for post-compaction mutation by key.

### Phase 2: Existing skill integration

Update existing skills to call `ws.enter.*` tools at the point they currently
produce transcript-only routing or implementation context.

#### Priority 1 — immediate integrations

**lead-proceed**
- Call `ws.enter.proceed(ticket, phase, next_skill, conditions)` when a
  next-skill decision is made.
- The enter call records routing context in agenda and replaces todo with a
  routing-phase checklist.

**lead-implement**
- Call `ws.enter.implement(delegation, plan_depth, branch_mode, review_alloc,
  current_branch, merge_target, start_commit, active_agents)` immediately after
  the Route step.
- Include `active_agents: [{name, role, started}]` to preserve agent-name
  context across compaction.
- Todo list is derived automatically by the enter tool; remove any manual
  `ws.todo.append` calls for the standard implement steps.

**lead-forge-spec**
- Replace host task-list dependency with `ws.todo` for domain-task tracking.
- Each `forge-spec-<domain>` task maps to a todo item; resume logic reads from
  `ws.todo` instead of the host task list.

**lead-forge-mental-model**
- Same migration as `lead-forge-spec`: replace `forge-mental-model-<domain>`
  host tasks with `ws.todo` items.

#### Priority 2 — secondary integrations

**lead-sprint**
- Call `ws.enter.sprint(episode_slug, episode_start, current_edit_context)`
  when an episode starts.
- Replaces `Sprint-Edit:` commit-marker resume logic; enables in-progress
  episode recovery without requiring a prior commit.

**lead-salvage**
- Call `ws.enter.salvage(failure_claim, confirmed_premises, survey_status)`
  after the failure-claim confirmation step.
- Prevents user re-confirmation after compaction when premises are already
  locked.

#### Excluded

- `lead-write-skeleton` — deprecated; no integration needed.

#### Shared update

- Update `delegate-orientation.md` to document the agenda/todo/enter contract
  (layer purposes, restoration points, typed vs. freeform entry) for all
  delegate agents.

Verify via: per-skill smoke test confirming the enter tool is called at the
correct point and the derived todo list matches expected items; review of
`delegate-orientation.md` update for accuracy.

### Phase 3: Workflow manual integration

- Add a "Session State" section to the workflow manual render, injected at load.
- Agenda subsection: show all active agenda blobs (remind only; not repeated at
  intermediate checkpoints).
- Todo subsection: show the todo list in summary mode (injected at every
  restoration point).
- Update `ws.commit` checkpoint logic to re-inject the todo list after commit.
- Document restoration behavior in `ai-docs/spec/plugin-runtime.md` and
  `ai-docs/ref/ws-mcp.md`.

Verify via: compaction simulation confirming the agenda remind section appears
in workflow-manual reload output; checkpoint probe confirming todo summary
injects after `ws.commit`.
