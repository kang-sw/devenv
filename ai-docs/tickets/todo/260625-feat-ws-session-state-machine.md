---
title: ws session state machine — verdict and todo persistence across compaction
related-mental-model:
  - mcp-runtime
  - plugin-runtime
---

# ws session state machine — verdict and todo persistence across compaction

## Background

After context compaction, implementation and routing verdicts are lost from the
session transcript. The session key (returned by `ws_ferrule`) is the only stable
anchor that survives compaction, because it appears in the compaction summary.

Workflow manual reload happens after compaction, but there is no mechanism to
restore in-flight verdicts (routing decisions, implementation choices) or task
lists that were built up before the compaction boundary.

The fix is a session-keyed state store on disk (`.ws/sessions/<session-key>.json`)
that holds two namespaces:

- **verdicts** — schemaless key/value blobs keyed by verdict type string; schema
  is enforced by the calling skill, not by MCP.
- **todos** — ordered list of task items with caller-provided key identifiers.

The workflow manual reload procedure injects both namespaces so the agent
resumes with full state visibility.

## Design

### Storage

`.ws/sessions/<session-key>.json` is the single backing file per session.
Fields:

```json
{
  "verdicts": {
    "<type>": { /* arbitrary object */ }
  },
  "todos": [
    { "key": "<short-stem>", "title": "...", "status": "pending|wip|done|defer" }
  ]
}
```

The file is created on first write and ignored if absent (no state = clean session).

### Verdict API (MCP primitives)

- `ws.verdict.create(type: string, data: object)` — upsert verdict blob under
  `type`. Overwrites any prior blob for the same type.
- `ws.verdict.clear(type: string)` — remove the blob for `type`.

Schema is the skill's responsibility. `ws.verdict.create` accepts any JSON object.

**Restoration point:** workflow manual load only (remind, not inject).

### Todo API (MCP primitives)

Item identity is a caller-provided `key` (short stem such as `"route-verdict"`).
Keys must be unique within the active list; a duplicate key returns an error.
Erased keys may be reused. Sequential indices are not exposed because they break
under insert/erase.

All mutation calls return void; errors are raised as exceptions.

- `ws.todo.append(key: string, title: string)` — append item at end.
- `ws.todo.insert_before(ref_key: string, key: string, title: string)` — insert
  before the item identified by `ref_key`.
- `ws.todo.insert_after(ref_key: string, key: string, title: string)` — insert
  after the item identified by `ref_key`.
- `ws.todo.check(key: string, status: "pending" | "wip" | "done" | "defer")` —
  update item status. Any status transition is allowed.
- `ws.todo.erase(key: string)` — hard delete; does not shift other keys.
- `ws.todo.clear(done_only: bool = false)` — `done_only=false` removes all items;
  `done_only=true` removes only items with status `done` (leaves pending/wip/defer).
- `ws.todo.list(mode: "summary" | "full" = "summary")` — render the todo list.

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
`defer` items are treated as collapsed in summary mode (same as done). Full
item key and title are always shown; status marker indicates state.

**Restoration points:** workflow manual load, `ws.commit`, and any other major
checkpoint that renders session context to the agent. Checkpoint injection always
uses summary mode. Explicit `ws.todo.list()` calls default to summary; pass
`mode: "full"` for the complete ordered list.

### Skill layer

MCP primitives carry no behavioral meaning. Meaning is encoded in skills:

- New skills such as `ws.verdict.implement` call `ws.verdict.create("implement",
  {...})` with a defined payload shape and provide the behavioral guidance
  associated with recording that verdict type.
- Existing skills (`lead-implement`, `lead-proceed`, etc.) are updated to call
  the appropriate verdict MCP tool at the point where they currently produce a
  transcript-only verdict.

The skill owns the schema; the MCP layer only persists the blob.

### Behavioral separation rule

| Namespace | Restoration point | Mechanism |
|-----------|------------------|-----------|
| verdict   | workflow manual load only | remind (show current verdicts at the bottom of the manual section) |
| todo      | workflow manual load + `ws.commit` + major checkpoints | inject (render full list inline) |

`ws.commit` does not auto-mark todos as done. Status transitions are always
explicit via `ws.todo.check`.

## Phases

### Phase 1: MCP primitives and session store

Implement the storage layer and all MCP tool handlers:

- `.ws/sessions/` directory convention and `<session-key>.json` schema.
- `ws.verdict.create`, `ws.verdict.clear` handlers in
  `agents-plugin-tool/internal/mcp/`.
- `ws.todo.append`, `ws.todo.insert_before`, `ws.todo.insert_after`,
  `ws.todo.check`, `ws.todo.erase`, `ws.todo.clear`, `ws.todo.list` handlers.
- `runtime.json` registration for all new tools with version fence
  `>=<next-minor>`.
- Unit tests covering concurrent write safety and key uniqueness enforcement.

### Phase 2: Skill wrapper authoring and existing skill integration

Author new verdict-type skills following the invariant checklist in
`agents-plugin/skills/lead-skill-authoring/SKILL.md`, and update existing
skills to call MCP primitives at the point they currently produce
transcript-only verdicts.

#### Priority 1 — immediate integrations

**lead-proceed** (verdict)
- Emit `ws.verdict.create("routing", {...})` when a next-skill decision is made.
- Payload schema (enforced by skill): `{next, ticket, phase, conditions: {discussion_needed, actionable, freshness}}`.
- Clear with `ws.verdict.clear("routing")` when routing is resolved and the
  target skill is entered.

**lead-implement** (verdict + todo)
- Emit `ws.verdict.create("implement", {...})` immediately after the Route step.
- Payload schema: `{delegation, plan_depth, branch_mode, review_alloc, current_branch, merge_target, start_commit}`.
- Include `active_agents: [{name, role, started}]` in the payload to survive
  agent-name loss across compaction (no separate API needed).
- Populate `ws.todo` with the 9-step task list (Route, Prep, Edit, Review,
  Doc pre-pass, Doc commit gate, Doc closeout compaction, Final action gate,
  Merge) at session start using `ws.todo.append(key, title)` for each step;
  mark items via `ws.todo.check(key, status)` as they complete.

**lead-forge-spec** (todo)
- Replace host task-list dependency with `ws.todo` for domain-task tracking.
- Each `forge-spec-<domain>` task maps to a todo item; existing resume logic
  reads from `ws.todo` instead of the host task list.

**lead-forge-mental-model** (todo)
- Same migration as `lead-forge-spec`: replace `forge-mental-model-<domain>`
  host tasks with `ws.todo` items.

#### Priority 2 — secondary integrations

**lead-sprint** (verdict)
- Replace `Sprint-Edit:` commit-marker resume logic with
  `ws.verdict.create("sprint-episode", {...})`.
- Payload schema: `{episode_slug, episode_start, current_edit_context}`.
- Enables in-progress episode recovery without requiring a prior commit.

**lead-salvage** (verdict)
- Emit `ws.verdict.create("salvage", {...})` after the failure-claim confirmation
  step.
- Payload schema: `{failure_claim, confirmed_premises, survey_status}`.
- Prevents user re-confirmation after compaction when premises are already locked.

#### Excluded

- `lead-write-skeleton` — deprecated; no integration needed.

#### Shared update

- Update `delegate-orientation.md` to document the verdict/todo contract
  (payload conventions, restoration behaviour, skill ownership of schema)
  for all delegate agents.

### Phase 3: Workflow manual integration

- Add a "Session State" section to the workflow manual render that is injected
  at load time.
- Verdict subsection: show all active verdict blobs (remind only, not repeated
  at checkpoints).
- Todo subsection: show the full todo list with IDs and statuses (injected at
  every restoration point).
- Update `ws.commit` checkpoint logic to re-inject the todo list after commit.
- Document restoration behavior in `ai-docs/spec/plugin-runtime.md` and
  `ai-docs/ref/ws-mcp.md`.
