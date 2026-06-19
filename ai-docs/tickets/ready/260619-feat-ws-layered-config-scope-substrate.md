---
title: Layered config scope substrate (session>project>global>builtin) + prefer_mercenary migration
parent: 260619-epic-ws-layered-config-prompt-tuning
spec:
  - 260619-layered-config-scope-model
  - 260619-prefer-mercenary-session-scope-item
related:
  260618-bug-ws-prefer-mercenary-one-way-flip: closed by Phase 2 (prefer_mercenary becomes a session-scope layered-config item with desired-state get/set)
  260617-refactor-mcp-stateless-subagent-context: owns the per-key session store (keys/<key>.json) that backs the session scope layer
  260611-refactor-ws-tier-taxonomy-delegate-tier-routing: deferred config.model_alias/config.role_tier rename slice that must adopt this child's reusable scope primitive (coordinate; do not fork)
related-mental-model:
  - prompt-bundle
---

# Layered config scope substrate (session>project>global>builtin) + prefer_mercenary migration

## Background

Today ws config has a single store (project-scoped `config.json` under
`${WS_CACHE_HOME}` ≈ `~/.ws@<project-id>/`) and `prefer_mercenary` is a bespoke
one-way in-memory flip with no revert. There is no reusable, scope-aware config
mechanism. This child builds the substrate the rest of the epic
(`260619-epic-ws-layered-config-prompt-tuning`) consumes.

## Decisions

- **Scope resolution order:** `session > project > global > builtin`
  (builtin = code defaults; e.g. `wsconfig` tier defaults, `prefer_mercenary=false`).
- **Scope storage map:**
  - session → the per-key session store (`keys/<key>.json`, owned by
    `260617-refactor-mcp-stateless-subagent-context`); file-backed and tied to
    the session key's lifetime.
  - project → existing `config.json` under `${WS_CACHE_HOME}` (`~/.ws@<id>/`).
  - global → **new** project-agnostic location: `~/.ws/config.json` with a
    `WS_CONFIG_HOME` env override (mirrors the `WS_CACHE_HOME → ~/.ws@<id>/`
    convention).
- **Per-item declared default scope; project as blanket fallback.** Each config
  item declares its natural default scope in code; items declaring nothing fall
  back to `project`. An explicit `scope:` argument on set always wins.
- **`get`/`show` must report which scope a value resolved from** (hard
  requirement for debuggability).
- **Reusable internal primitive, shared schema.** The scope resolution + the
  `scope` parameter shape are an internalized reusable `wsconfig` method plus a
  shared `scope` schema fragment that any config tool consumes. This child
  applies it to the items the epic owns (`prefer_mercenary`; the prompt-override
  storage field added by `260619-feat-ws-config-prompt-tool-self-doc` /
  `-prompt-override-marker-engine`). It does NOT retrofit the
  `config.agents_tier` tool surface — the `config.model_alias`/`config.role_tier`
  rename slice (`260611-refactor-...`) adopts the same primitive when it touches
  those tools. Coordinate; do not fork the scope rule.
- **Zero-migration via precedence.** Because `project > global`, existing
  project-stored values keep winning after the global layer is added; only the
  *write default* for future sets follows the item's declared scope. No data
  migration.
- **File lock required, both file scopes.** Read-modify-write on project and
  global config must be serialized (flock + temp-write + atomic-rename). Global
  is more exposed (cross-project concurrent writers); project is not lock-free
  either.
- **Additive structure (merge-safety).** New `wsconfig` resolver module + new
  fields; do not rewrite existing `Show`/`SetAgentsTierForHarness`. This child
  runs in the main worktree alongside live pivot work.

Rejected: mandatory explicit scope on every set (needless friction — per-item
declared defaults already remove the wrong-default surprise; flipping to
mandatory later is a one-line policy change).

## Phases

### Phase 1: Reusable scope substrate

Build the scope-aware config primitive: the `session>project>global>builtin`
resolver, a per-item default-scope registry, an explicit `scope:` parameter, a
scope-reporting `get`/`show`, the new global config file
(`~/.ws/config.json` + `WS_CONFIG_HOME`), and file-lock read-modify-write for
both file scopes. Expose the shared `scope` schema fragment for other config
tools to adopt. Session-scope reads/writes route through the per-key session
store from `260617`.

Verification: a value set at each scope resolves with correct precedence; `show`
reports the resolved scope; concurrent writers do not corrupt the file (lock).

### Result (acf1be70) - 2026-06-19

Substrate landed (impl `6b3ea800`, fix-cycle `acf1be70`). All Phase 1 decisions
honored:

- `wsconfig.Resolver` resolves `session > project > global > builtin`, returning
  the source scope (`resolver.go`). Per-item default-scope registry with
  `project` fallback (`scope.go`); explicit `SetOptions.ExplicitScope` wins.
- New global store `~/.ws/config.json` / `$WS_CONFIG_HOME` (`global.go`); missing
  file → empty layer, not an error.
- File-scope RMW serialized with `gofrs/flock` (temp-write + atomic-rename) for
  both project and global; the session store is unchanged (mutex + `O_EXCL`/
  rename, NOT flocked).
- Session scope routes through the existing per-key store via additive
  `getOverride`/`setOverride`/`listOverrideKeys` accessors (`session_auth.go`) —
  no new session backend.
- `config.show` reports each value's resolved scope when `session_key` is supplied
  (`ScopedShow` → `View.ResolvedOverrides`); plain `Show` otherwise.
  `config.agents_tier` byte-for-byte unchanged. Shared `scope` schema fragment
  (`ScopeSchemaEnum` / `scopeSchemaProperty`) exposed for later adopters.

Verification: `go test ./internal/wsconfig/...` passes (incl. precedence,
scope-reporting, explicit-scope override, default fallback, global location via
`$WS_CONFIG_HOME` and `ConfigHome`, zero-migration project-over-global, capability
hook, and two concurrency tests — distinct-key and contended shared-key
increment, final == N). `go test ./internal/mcp/...` shows only two pre-existing
golden-hash failures (`lead-workflow-manual.md`), confirmed unrelated to this
slice. `go build ./...` clean.

Spec `260619-layered-config-scope-model` 🚧 stripped (`a3969d4b`); mental model
`mcp-runtime` updated with the locking invariant (`a0b5571c`).

Forward (Phase 2): `prefer_mercenary` can now ride the `Overrides` overlay as a
session-default desired-state item with the role/capability gating hook already
present in `Resolver.Set` (`CapabilityCheck`).

#### Minor follow-up (deferred, non-blocking)

`setOverrideInFileRMW` and `setOverrideInFile` share ~40 lines of flock+temp+
rename boilerplate; the latter could delegate to the former. Correct and tested;
unify opportunistically.

### Phase 2: Migrate prefer_mercenary onto the layered config

Replace the bespoke one-way `setPreferMercenary` with a `prefer_mercenary`
config item whose declared default scope is `session`, set/got through the
layered primitive with desired-state (enable AND disable). It stays lead-only —
the layered setter must honor the existing role/capability gating (not every
item is freely settable). This closes `260618-bug-ws-prefer-mercenary-one-way-flip`.

Depends on Phase 1.

Verification: flip prefer_mercenary on, then off, on the same session key (the
260618 repro) and confirm the render guidance follows both transitions; non-lead
keys are rejected.
