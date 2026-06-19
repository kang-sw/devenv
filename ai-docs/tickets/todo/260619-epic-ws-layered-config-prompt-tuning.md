---
title: ws layered config substrate + user prompt-override tuning surface
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: prompt-factory / playbook templating direction this epic extends
  260611-research-ws-per-role-delegation-tuning-config: its deferred "role -> prompt tuning" axis can later ride this epic's override substrate; referenced, not absorbed (its tier-routing half is owned by 260611-refactor-ws-tier-taxonomy-delegate-tier-routing)
  260618-bug-ws-prefer-mercenary-one-way-flip: resolved by child-1 when prefer_mercenary migrates into the layered config as a session-scope item with desired-state get/set
  260513-harness-local-agent-tier-config: existing config.agents_tier surface this epic re-homes under the layered scope model
related-mental-model:
  - prompt-bundle
---

# ws layered config substrate + user prompt-override tuning surface

## Scope

Push the playbook prompt engine toward user-tunable configuration. Two coupled
deliverables:

1. A **layered config substrate** so every config item resolves across
   `session > project > global > builtin` scopes, with a per-item default scope
   and an explicit `scope:` override on set.
2. A **user prompt-override surface** that lets a user/agent override or extend
   named sections of any rendered playbook body (e.g. a delegation-guidance
   section in the workflow manual) without editing shipped rsrc text, plus a
   self-documenting listing tool so the override surface is discoverable from
   inside the MCP.

Motivating first use case: lead "context-saving" delegation tuning via a
`{.DelegationSection}` override-point; generalized into a reusable mechanism
rather than a delegation-specific feature.

## Non-Scope

- Tier taxonomy / role -> tier routing — owned by
  `260611-refactor-ws-tier-taxonomy-delegate-tier-routing`.
- The specific `role.partition` prompt addressing from
  `260611-research-ws-per-role-delegation-tuning-config` (e.g. `reviewer.test`
  vs `reviewer.correctness`). This epic ships the override *mechanism*; that
  granular role-addressing is a possible later consumer, not in scope.
- Config file *format* migration (JSON -> TOML/YAML for multiline friendliness).
  Deferred; revisit only if raw-file readability becomes a real pain.
- Session-scope config storage backend redesign — session-scope items live in
  the existing per-key session store (`keys/<key>.json`, owned by
  `260617-refactor-mcp-stateless-subagent-context`); this epic reuses it, not
  redesigns it.

## Child Tickets

- `260619-feat-ws-layered-config-scope-substrate` (**done**) — 4-layer resolver
  (`session > project > global > builtin`), per-item declared default scope,
  explicit `scope:` arg, scope-reporting get/show, new global config location,
  file-lock read-modify-write, reusable scope primitive + shared schema. P2
  migrated `prefer_mercenary` into it (closed
  `260618-bug-ws-prefer-mercenary-one-way-flip`).
- `260619-feat-ws-prompt-override-marker-engine` (**done**) — A1 block-marker
  override pass (sibling to the product-mode pass); seeded `DelegationSection` in
  the workflow manual (first shipped override-point). Both phases shipped.
- `260619-feat-ws-config-prompt-tool-self-doc` (todo) — **data plane only**:
  `config.prompt.set` setter (own namespace) + no-arg `config.prompt()` listing
  that tree-scans override markers and returns the point list / current values /
  scopes **plus a one-line pointer to `ws:lead-tune`**. The tuning *manual* moved
  out to the new skill child (3b). Depends on the substrate and the marker engine.
- `260619-feat-ws-lead-tune-skill` (todo) — **new**: `ws:lead-tune`, a dedicated
  user-invocable entry skill that is the **umbrella tuning surface** (prompt
  overrides primary; introduces/links `prefer_mercenary` and `config.agents_tier`;
  leaves a slot for future `config.model_alias`/`config.role_tier`). Owns the
  tuning manual and the proactive-proposal trigger. Depends on 3a (data plane) and
  the marker engine (points it documents).

## Cross-Child Decisions

- **Scope resolution order:** `session > project > global > builtin` (builtin =
  code defaults, e.g. `wsconfig` tier defaults, `prefer_mercenary=false`).
- **Per-item default scope, project as blanket fallback.** Each item declares
  its natural default scope in code; items that declare nothing fall back to
  `project`. An explicit `scope:` argument on set always wins. `get`/`show` must
  report *which scope* a value resolved from (debuggability is a hard
  requirement). Mandatory-explicit-scope-per-set was rejected as needless
  friction since per-item declared defaults already remove the "wrong default"
  surprise; flipping to mandatory later is a one-line policy change.
- **`prefer_mercenary` default scope = session.** Migrating it from the bespoke
  in-memory flip into a layered-config session item gives it desired-state
  get/set, which structurally resolves the one-way-flip bug (260618). It stays
  lead-only — the layered setter must honor existing role/capability gating for
  such items (not every item is freely settable).
- **Zero-migration for existing config via precedence.** Today's
  `config.agents_tier` lives in the project-scoped `config.json`. Because
  `project > global`, existing project values keep winning after the global
  layer is added; only the *write default* for future sets follows the item's
  declared scope. No data migration needed.
- **File lock is required regardless of default scope.** Read-modify-write on
  both project and global config must be serialized (flock + temp-write +
  atomic-rename). Global is more exposed (cross-project concurrent writers) but
  project is not lock-free either.
- **Prompt-override mechanism = block markers, not frontmatter defaults.** Use an
  override-block marker pass (`<!-- ws:override:<id> -->...seed default...<!--
  ws:/override:<id> -->`) extending the existing `selectProductModeBlocks`
  marker machinery. Shipped default text stays inline in the `.md` (PR-readable);
  no multiline-in-YAML default. A user override replaces the block content; an
  empty seed block is a pure extension slot — so `{.DelegationSection}`
  (override existing) and `{.WorkflowManualExt}` (append) are the *same*
  primitive. Rejected: variable-with-frontmatter-default (engine fails loud on
  unprovided placeholders today, and multiline YAML defaults are ugly and move
  seed text out of the body). **Consequence: the `{{.DelegationSection}}`-style
  template-variable placeholder is NOT used for override-points.** The
  `DelegationSection` identifier survives only as the marker `<id>` and as the
  `config.prompt.set` `pointId` key — the user-facing addressing API
  (`config.prompt.set("DelegationSection", harness, prompt)`) is preserved; only
  the in-body carrier changes from a `{{.Var}}` placeholder to a marker-delimited
  block whose inline content is the seed default.
- **Two orthogonal axes for prompt overrides:** `(pointId, harness)` selects
  *what* is overridden (harness in `claude | codex | "*"/all`, mirroring the
  existing per-harness `ModelAliases` map shape); scope selects *where* it is
  stored. Resolution for an override-point: user override (harness match) ->
  user override (`*`) -> inline seed default.
- **Coordinate with the deferred config-surface rename slice.** A separate
  deferred slice (live follow-up of `260611-refactor-ws-tier-taxonomy-delegate-tier-routing`)
  renames `config.agents_tier` -> `config.model_alias` and adds a
  `config.role_tier` `(skill,role)->tier` override surface. Child-1's re-homing
  of the tier/alias config under the layered scope model touches the same config
  struct and tool names — it must coordinate with (not duplicate or fork) that
  rename. Sequence so one does not clobber the other's caller-visible names.
- **Additive merge-safety (this epic runs in the main worktree alongside live
  pivot M3 work).** Implement as new files/fields/functions to minimize conflict
  on shared hot files (`wsconfig/config.go`, `mcp/playbook_tools.go`,
  `mcp/server.go`): a new `wsconfig` resolver module, a new `AgentsConfig`
  sibling field for overrides, a single inserted override-layer in
  `buildPlaybookVars`, and a new marker-pass function beside
  `selectProductModeBlocks` — not rewrites of existing functions.
- **Prompt-override storage = inline in the single config file (confirmed).**
  Overrides live inline in the one config JSON, not split into per-override
  resource-files, so the lock/atomicity story stays single-file. Config size is
  not a functional problem (config is already loaded per call; tens of KB is
  negligible) and raw-file readability is recovered by the self-doc render.
  Split-to-resource-files rejected: reintroduces multi-file
  lock/atomicity/orphan complexity.
- **Tuning discovery = dedicated entry skill, not workflow-manual prose
  (confirmed).** The proactive-proposal surface (so an agent offers tuning when a
  user signals "let's tune the prompts/workflow") is a new `ws:lead-tune` entry
  skill whose *description* is the runtime trigger (mental model
  `workflow-skills` #260508 — skill descriptions are the trigger surface). The
  always-on `lead-workflow-manual` gets at most a one-line pointer (or none), so
  the tuning surface does not tax general-task routing attention. Rejected:
  putting the tuning manual inline in `lead-workflow-manual` (always-on bloat) or
  inline in `config.prompt()` output (bloats the data surface, weak trigger).
- **Three planes, kept separate (confirmed).** (1) Data plane = `config.prompt.*`
  MCP tools (3a). (2) Procedure/manual plane = the `ws:lead-tune` skill playbook
  (3b) — owns the tuning manual and when/how to propose. (3) Discovery plane =
  the skill description + minimal manual pointer. `config.prompt()` returns data +
  a pointer; the manual lives in 3b.
- **`ws:lead-tune` is an umbrella, implemented to what exists (confirmed).** It
  conceptually covers all workflow tuning; the first build covers prompt overrides
  (primary), `prefer_mercenary`, and `config.agents_tier` (introduce/link their
  existing set paths — do not reimplement), and leaves a slot for the future
  `config.model_alias`/`config.role_tier` axis (`260611` research). 3a (MCP data
  plane) is NOT blocked on 3b. Naming follows the `lead-*` entry-skill convention
  (`ws:lead-tune`); the user-facing framing is "workflow tuning."

## Completion Criteria

- Done: all children landed — layered config resolves across the four
  scopes with per-item defaults and scope-reporting get/show (done);
  `prefer_mercenary` is a session-scope item with working revert (260618 closed,
  done); the block-marker override engine renders `DelegationSection` with user
  overrides honored per `(pointId, harness)` and scope (done); `config.prompt()`
  returns the override surface data; and `ws:lead-tune` surfaces the umbrella
  tuning manual plus the proactive-proposal trigger.
- Dropped: the pivot abandons user-tunable playbook config, or a simpler
  single-scope override surface is judged sufficient and the layered substrate
  is cut.
- Deferred: `role.partition` prompt addressing (260611 axis-2), config-format
  migration, and a persistent session-config backend.
