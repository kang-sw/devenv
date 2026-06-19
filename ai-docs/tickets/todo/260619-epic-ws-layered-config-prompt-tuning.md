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
- A persistent session backend for session-scope config — owned by the pivot
  session-auth work; session-scope items remain in-memory and are lost on
  process restart (consistent with the `unknown_session -> re-login` model).

## Child Tickets

- Planned: **layered config substrate** — 4-layer resolver
  (`session > project > global > builtin`), per-item declared default scope,
  explicit `scope:` arg on set, scope-reporting get/show, new global config
  location, file-lock for read-modify-write. Migrate `prefer_mercenary` into it
  as a session-default item (closes `260618-bug-ws-prefer-mercenary-one-way-flip`).
  Re-home `config.agents_tier`/model aliases under the model (precedence makes
  this zero-migration — see Cross-Child Decisions).
- Planned: **block-marker override engine + DelegationSection seed** — extend
  the product-mode marker pass with an override-block pass; seed
  `{.DelegationSection}` in the workflow manual and consolidate the currently
  scattered/hardcoded delegation guidance text into that named section.
  Depends on the substrate child (override values resolve through layered config).
- Planned: **`config.prompt.*` setter + self-doc listing** — setter that writes
  prompt overrides keyed by `(pointId, harness)` at a chosen scope; a
  no-arg/self-doc `config.prompt()` that tree-scans declared override-points and
  renders the list + current overrides + a short tuning manual so an agent can
  tune in-place. Depends on both prior children.

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

## Completion Criteria

- Done: all three children landed — layered config resolves across the four
  scopes with per-item defaults and scope-reporting get/show; `prefer_mercenary`
  is a session-scope item with working revert (260618 closed); the block-marker
  override engine renders `{.DelegationSection}` with user overrides honored
  per `(pointId, harness)` and scope; `config.prompt()` self-documents the
  override surface.
- Dropped: the pivot abandons user-tunable playbook config, or a simpler
  single-scope override surface is judged sufficient and the layered substrate
  is cut.
- Deferred: `role.partition` prompt addressing (260611 axis-2), config-format
  migration, and a persistent session-config backend.
