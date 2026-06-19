# Brief: 260619-feat-ws-layered-config-scope-substrate (Phase 1)

## Intent

Build a reusable, scope-aware config substrate for the ws MCP Go tooling so that
config items resolve across `session > project > global > builtin` scopes, with a
per-item declared default write scope and an explicit `scope:` override on set.
This is the prerequisite substrate for the rest of epic
`260619-epic-ws-layered-config-prompt-tuning` (the prompt-override marker engine
and the `config.prompt.*` self-doc tool consume it). Phase 1 ships the substrate
only; Phase 2 (migrate `prefer_mercenary` onto it) is a separate later slice and
is OUT OF SCOPE here.

## Scope Boundary

In scope (Phase 1):
- A `wsconfig` scope resolver implementing precedence `session > project > global > builtin`.
- A per-item default-scope registry; items declaring nothing fall back to `project`.
- An explicit `scope:` argument shape (shared schema fragment) honored on set.
- Scope-reporting reads: `get`/`show` report which scope a value resolved from.
- A NEW global config file at `~/.ws/config.json`, with `WS_CONFIG_HOME` env override.
- File-lock read-modify-write (flock + temp-write + atomic-rename) for BOTH file
  scopes (project `config.json` and the new global `config.json`).
- Session-scope reads/writes route through the EXISTING per-key session store
  (`keys/<key>.json`).
- The shared `scope` schema fragment any config tool can consume.

Explicitly OUT of scope (deferred / other tickets):
- Migrating `prefer_mercenary` onto the layered config — that is Phase 2 of this
  ticket, NOT this slice.
- Retrofitting the existing `config.agents_tier` tool surface onto the scope model
  — owned by the deferred `config.model_alias`/`config.role_tier` rename slice
  (`260611-refactor-ws-tier-taxonomy-delegate-tier-routing`). Coordinate; do not
  fork the scope rule, and do not rename `config.agents_tier` here.
- Adding the `config.prompt.*` namespace or prompt-override storage fields —
  owned by `260619-feat-ws-config-prompt-tool-self-doc` /
  `-prompt-override-marker-engine`.
- Config file FORMAT migration (JSON → TOML/YAML). Stays JSON.

## Caller-Visible Contract

Spec anchors (already authored, ai-docs/spec/mcp-tools.md):
- `#260619-layered-config-scope-model` — the full caller-visible contract.
- `#260619-prefer-mercenary-session-scope-item` — Planned note; Phase 2 only,
  do not implement here.

Observable behavior this slice must deliver:
- A config read returns the value from the highest-precedence scope that holds
  one; `builtin` (code defaults) is the floor.
- A write with no explicit scope lands in the item's declared default scope
  (`project` if the item declares none). An explicit `scope:` arg always wins.
- `get`/`show` output reports the resolved scope of each value (session | project
  | global | builtin) — debuggability is a hard requirement.
- A new global store exists at `~/.ws/config.json` (or `$WS_CONFIG_HOME/config.json`).
- Because `project > global`, pre-existing project-stored values keep winning
  after the global layer is added — NO data migration, no behavior change for
  existing project config.
- Concurrent writers to the project or global file do not corrupt it.

## Contract Instructions

- Module: `agents-plugin-tool/internal/wsconfig/` — add the scope resolver and the
  default-scope registry as NEW files/types beside the existing `config.go`; do
  NOT rewrite `Show`/`SetAgentsTierForHarness` or the existing `Config` struct
  load/save shape. Additive only (merge-safety: this runs in the main worktree
  alongside live pivot work).
- Reuse the existing project config path resolution (`${WS_CACHE_HOME}` →
  `~/.ws@<project-id>/config.json`). Add a parallel global resolver:
  `$WS_CONFIG_HOME` if set, else `~/.ws/config.json`.
- Session scope MUST route through the existing per-key session store
  (`<cache-root>/keys/<key>.json`, owned by `260617`). Do NOT build a new session
  backend. Reuse the store's existing temp-write+rename update idiom (the same
  one `setPreferMercenary` uses). Identify the existing read/update entry points
  before adding a generic scoped-field accessor; if the store lacks a
  general-purpose field read/write, add a minimal additive one, do not refactor
  the store.
- File-lock RMW for project + global files: acquire a file lock, read, modify,
  write to a temp file, atomic-rename. Both file scopes must be serialized.
- The `scope` argument is a shared schema fragment (one definition consumed by
  every scope-aware config tool), not a per-tool re-implementation. Expose it so
  the later children can adopt it.
- Item-level write gating still applies: the scope-aware setter must honor an
  item's existing role/capability restrictions (carry the hook even though no
  gated item is migrated in Phase 1).
- MCP surface: if Phase 1 exposes scope-reporting through `config.show` or a
  get tool, follow the mcp-runtime change recipe — schema in `tools()`, dispatch
  in `callTool`, key-only root resolution via `resolveToolRoot`, and
  `runtime.json` if a tool is added. Keep readable-text default output; JSON on
  request. Do NOT advertise `root`.

Forbidden:
- No temporary/mock/fallback storage. No in-memory-only scope.
- No rewrite of existing config load/save or the session store.
- No rename of `config.agents_tier`; no new `config.prompt.*` namespace.

## Integration Test Instructions

- Boundary: `wsconfig` package unit tests for the resolver + lock; MCP-layer test
  if a tool surface changes.
- New or extended test file under `agents-plugin-tool/internal/wsconfig/`.
- Pass criteria:
  - A value set at each scope resolves with correct precedence
    (session over project over global over builtin).
  - `get`/`show` reports the resolved scope.
  - Explicit `scope:` on set overrides the declared default.
  - An item with no declared default writes to `project`.
  - Global file lands at `$WS_CONFIG_HOME/config.json` when the env var is set.
  - Concurrent writers (lock) do not corrupt the file — a serialized
    read-modify-write test (e.g. N goroutines incrementing) yields no lost writes.
  - Pre-existing project value still wins after a global value is set (zero-migration).

## Implementation Strategy Decisions (settled — do not reopen)

- Storage is INLINE in the single config file per scope (one JSON file per file
  scope); NOT split into per-item resource files. Lock/atomicity stays single-file.
- Scope order is fixed: `session > project > global > builtin`.
- Per-item declared default scope; blanket fallback is `project` (NOT mandatory
  explicit scope — flipping to mandatory later is a one-line policy change).
- Global location reuses the existing convention shape (`WS_CONFIG_HOME` mirrors
  `WS_CACHE_HOME`).

## Rejected Alternatives

- Mandatory explicit scope on every set — rejected as needless friction.
- Per-item split resource files — rejected (reintroduces multi-file lock/atomicity/orphan complexity).
- A new session-config backend — rejected; reuse the `260617` per-key store.
- Lock-free project writes — rejected; both file scopes are serialized.

## Approach

- Add a `scope` type/enum and a resolver that, given an item key, walks
  session → project → global → builtin and returns (value, resolvedScope).
- Add a default-scope registry mapping item keys to their declared default scope,
  with `project` as the fallback.
- Add a setter that picks the target scope (explicit arg > declared default) and
  writes through the appropriate backend (session store vs file with lock).
- Add the global file path resolver + a shared `scope` schema fragment.
- Wire scope-reporting into `get`/`show` output.

## Constraints

- Additive structure only (new files/types/functions); no rewrites of existing
  `Show`, `SetAgentsTierForHarness`, `Config` load/save, or the session store.
- English-only code comments.
- Follow impl-playbook Verify: read full test/build output before claiming pass;
  resolve introduced warnings.

## Out of scope

- prefer_mercenary migration (Phase 2), config.agents_tier rename, config.prompt.*
  namespace, format migration. See Scope Boundary.

## Details

- Project config: `${WS_CACHE_HOME}`-derived `~/.ws@<project-id>/config.json` (existing).
- Global config: `$WS_CONFIG_HOME/config.json`, default `~/.ws/config.json` (new).
- Session config: per-key `<cache-root>/keys/<key>.json` (existing store, additive accessor if needed).
- Resolver return must carry the resolved-scope label for get/show reporting.

## Verification Contract

- `go test ./internal/wsconfig/...` (and any touched MCP package) passes; full
  output read.
- The precedence, scope-reporting, explicit-scope-override, default-fallback,
  global-location, concurrency-lock, and zero-migration assertions above all pass.
- Build is clean with no new warnings.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/spec/mcp-tools.md` #260619-layered-config-scope-model - [Must] core Phase 1 contract (resolver order, declared default write scope with project fallback, explicit `scope:` arg, scope-reporting get/show, `~/.ws/config.json` + `WS_CONFIG_HOME`, atomic-replace-under-lock, single shared scope contract).
- `ai-docs/spec/mcp-tools.md` #260505-config-tools - [Must] existing `config.show`/`config.agents_tier` surface the scope-aware get/set must consume and not break.
- `ai-docs/mental-model/mcp-runtime.md` - [Must] `sessionStore` flat `keys/<key>.json` (temp-write+rename, `WS_CACHE_HOME`), `{root, scope}` association, wsconfig coupling, keyed capability/lead-only gate the scope-aware setter must respect, and the add/change-MCP-tool change recipes.
- `ai-docs/spec/mcp-tools.md` #260513-harness-local-agent-tier-config - [Maybe] `config.agents_tier` — explicitly NOT retrofitted here; read to avoid forking it and honor the coordination note.
- `ai-docs/spec/mcp-tools.md` #260619-prefer-mercenary-session-scope-item - [Maybe] Phase 2 target; informs the session-scope item shape and lead-only gating the substrate must support (do not implement in Phase 1).
- `ai-docs/mental-model/plugin-runtime.md` - [Maybe] launcher environment context for the new `WS_CONFIG_HOME` env override.
