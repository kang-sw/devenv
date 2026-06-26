# Survey: 260619-feat-ws-layered-config-scope-substrate

Scope: Phase 1 reusable scope-aware config substrate. All paths under
`agents-plugin-tool/`. Additive only; no rewrite of `Show`,
`SetAgentsTierForHarness`, `Config` load/save, or the session store.

## Reusable Components

- `internal/wsstate/paths.go#L84-L96` — `CacheRoot(Options)`: project path
  resolution seam. Resolves `opts.CacheHome` → `$WS_CACHE_HOME` → `~/.cache/ws@...`.
  The new global resolver should mirror this exact shape with `$WS_CONFIG_HOME` →
  `~/.ws/config.json` (note `~/.ws`, not `~/.cache`).
- `internal/wsconfig/config.go#L225-L231` — `Path(opts)`: existing project
  `config.json` resolver (joins `CacheRoot` + `config.json`). Reuse as-is for the
  `project` file scope; add a parallel `globalPath()` beside it.
- `internal/wsconfig/config.go#L393-L407` — `save(opts, cfg)`: existing
  non-atomic `MkdirAll`+`MarshalIndent`+`os.WriteFile`. NOT lock-safe; the new
  file-lock RMW path is a parallel additive writer, leave `save` untouched
  (`SetAgentsTierForHarness` still uses it).
- `internal/wsconfig/config.go#L40-L68` — `Load(opts)`: parse project
  `config.json`, returns `defaultConfig()` on `os.IsNotExist`. Pattern to copy
  for reading the global file (missing file → empty/builtin layer).
- `internal/mcp/session_auth.go#L188-L212` — `writeRecordAtomic(dir,key,record)`:
  the temp-write+rename idiom the brief names (`setPreferMercenary` uses it).
  Session-scope writes must route through this store, not a new backend.
- `internal/mcp/session_auth.go#L170-L183` — `readRecord(dir,key)`: existing
  session read entry point; key path-safety guard at L41/L171.
- `internal/mcp/server.go#L1079-L1101` — `formatConfigView(view)`: readable-text
  formatter for `config.show`; extend additively to print resolved-scope labels.

## Existing Patterns

- MCP tool dispatch: `internal/mcp/server.go#L452-L457` (`config.show`) and
  `#L458-L473` (`config.agents_tier`) show the switch-case shape in `callTool`.
  A new scope-aware get/set case would slot here; readable-text default,
  `wantsJSON` opt-in (see L454-L456).
- Tool schema registration: `internal/mcp/server.go#L1870-L1894` — the `tools()`
  entries for `config.show`/`config.agents_tier`. A shared `scope` schema fragment
  would be a helper (cf. `enumStringProperty` used at L1886) consumed by each
  config tool's `inputSchema`.
- Runtime version registry: `agents-plugin/runtime.json#L13-L14` and
  `agents-plugin-wsflow/runtime.json#L67` list `config.show`/`config.agents_tier`
  with version ranges. A NEW tool name must be added in both files (per the
  add-MCP-tool recipe); reusing/extending existing tools needs no entry.
- Per-scope JSON file load (missing→default) pattern: `config.go#L40-L68`.
- wsconfig test pattern: `internal/wsconfig/config_test.go#L9-L24` uses
  `Options{CacheHome: t.TempDir()}` to isolate the file. The global resolver
  needs an analogous test seam — see Risk Signals (no `Options` field for global
  yet).

## Relevant Interfaces

- `internal/wsconfig/config.go#L15-L17` — `Options{CacheHome string}`: the only
  injection seam today. A global-path test seam likely needs an additive field
  (e.g. `ConfigHome string`) or env-only resolution; flag for planner (Risk).
- `internal/wsconfig/config.go#L19-L38` — `Config`/`AgentsConfig`/`View`/
  `AgentTier`: existing struct shapes. The resolver returns must carry a
  resolved-scope label; `View` (L29-L32) is the get/show return type to extend
  additively for scope reporting.
- `internal/mcp/server.go#L36-L41` — `toolRole` (`lead`/`delegate`/`leaf`) and
  `#L2449-L2463` `roleAllowsTool`: the role gate the scope-aware setter must honor
  (`config.*` is lead-only for non-lead scopes today). The "carry the gating hook"
  requirement maps here.
- `internal/mcp/server.go#L59-L61` — `isLeadOnlyTool`: `ws.lead.*` + bootstrap.
  Setter gating reference.
- `internal/mcp/session_auth.go#L18-L34` — `sessionEntry`/`sessionRecord`
  (`SchemaVersion`,`Root`,`Scope`,`PreferMercenary`): the on-disk session shape.
  A generic session-scope field accessor would extend this record additively
  (the brief explicitly anticipates adding a minimal additive accessor).
- `internal/mcp/server.go#L1510-L1523` — `resolveToolRoot`: key→root resolution;
  any new key-only config tool wires through it (brief: "key-only root resolution
  via resolveToolRoot"). Session-scope config reads need the key, which this path
  already extracts from `arguments["session_key"]`.

## Constraints

- `save` (`config.go#L393-L407`) is NOT atomic and NOT locked; the brief's
  file-lock RMW is a NEW parallel writer for both project and global files. Do not
  retrofit `save`/`SetAgentsTierForHarness` onto the lock (additive-only +
  agents_tier-no-touch).
- Session store serialization is in-process `sync.Mutex` only
  (`session_auth.go#L53-L58`,#L160-L161); cross-process safety rests on
  `O_EXCL` (mint) and temp+rename (update). It does NOT use flock. The brief's
  flock requirement applies to the project/global FILE scopes, not the session
  store — session writes reuse the existing mutex+rename path as-is.
- `config.*` tools are blocked for non-lead session scopes
  (`roleAllowsTool` L2457/L2459). A session-scope SET reachable by a delegate/leaf
  key would contradict this gate; the setter must respect existing per-item gating.
- Existing project values outrank global by spec (`#260619-layered-config-scope-model`),
  so zero-migration holds only if `project` is resolved strictly above `global`.
- Go module is `go 1.23` (`go.mod#L3`); no flock/file-lock dependency is vendored
  (`require modernc.org/sqlite` only). See Risk Signals.

## Risk Signals

- `go.mod` (`agents-plugin-tool/go.mod#L1-L6`) — Possible reuse/dependency risk:
  there is no file-lock library in the tree and no `syscall.Flock`/`gofrs/flock`
  usage anywhere under `internal/`. The brief mandates "flock + temp-write +
  atomic-rename" for both file scopes; implementer must either add a dependency or
  hand-roll `syscall.Flock` (POSIX-only — Windows/WSL2 path behavior unverified).
  Lead/planner should confirm the locking primitive choice before implementation.
- `internal/wsconfig/config.go#L15-L17` — Possible test-seam risk:
  `Options` exposes only `CacheHome`. Global-file tests need to point
  `$WS_CONFIG_HOME` somewhere; if resolution is env-only the
  `Options{CacheHome:t.TempDir()}` isolation pattern (config_test.go#L9) won't
  cover the global file, and parallel tests touching a real `~/.ws` would be
  non-hermetic. An additive `Options` field for the global home is likely needed;
  flag whether it stays env-only or gains a struct seam.
- `internal/mcp/server.go#L458-L473` (`config.agents_tier`) and
  `#L2538-L2539` (noAgentHiddenTool) — Possible contract-boundary risk: the brief
  forbids renaming/retrofitting `config.agents_tier` (owned by `260611`). The
  resolver/registry must not be wired into this case. If Phase 1 exposes scope
  reporting it should be on `config.show` or a NEW get tool, keeping
  `config.agents_tier` byte-for-byte unchanged.
- `internal/mcp/session_auth.go#L155-L168` (`setPreferMercenary`) — Possible
  reuse-shape risk: the only session mutator is hardcoded to flip one boolean
  field. A generic scoped-field accessor (the brief's "add a minimal additive
  one") has no precedent shape here; the implementer is inventing the
  session-scope storage layout (map field vs. typed fields on `sessionRecord`).
  This is additive but is a genuine design choice, not a pure copy — surface to
  planner if the field layout is contested.

## Opinion

- No existing `config.get` tool exists (no `config.get`/`"get"` case in
  `callTool`); the brief leaves get-vs-extend-`show` open ("if Phase 1 exposes
  scope-reporting through `config.show` or a get tool"). Both wiring points are
  identified above; the choice is an implementation decision, not a missing
  reference.
- The substrate is genuinely additive against current code: every named reuse
  point (`Path`, `Load`, `save` pattern, `writeRecordAtomic`, `formatConfigView`,
  `tools()`/`callTool` slots) is extendable without editing existing behavior.
- The one true design gap (not a brief contradiction) is the locking primitive:
  the repo has no file-lock mechanism today. This is flagged as the highest-value
  item for lead/planner confirmation before implementation, but it is a means
  decision the brief already mandates (flock), so it does not require escalation
  to research — only a primitive choice.

## Lead Decisions (resolves Risk Signals — binding, do not reopen)

1. **Locking primitive = `github.com/gofrs/flock`.** Add it as a dependency to
   `agents-plugin-tool/go.mod`. Rationale: OS-level advisory lock that
   auto-releases on process exit (no stale-lock file to garbage-collect) and is
   cross-platform (Windows + POSIX), honoring the documented Windows target. Do
   NOT hand-roll `syscall.Flock` (POSIX-only — would drop Windows config
   locking). RMW order for both project + global files: acquire flock on a
   sibling `<config>.lock` (or the config file itself) → read → modify →
   temp-write → atomic-rename → release. The session store is UNCHANGED: it keeps
   its in-process `sync.Mutex` + `O_EXCL`/temp+rename; do NOT flock the session
   store.
2. **Global-path test seam = additive `Options.ConfigHome string`.** Mirror
   `CacheHome`. Global path resolves `opts.ConfigHome` → `$WS_CONFIG_HOME` →
   `~/.ws/config.json` (note `~/.ws`, not `~/.cache`). Tests use
   `Options{ConfigHome: t.TempDir()}` for hermetic global-file coverage. Keep
   resolution precedence identical in shape to `wsstate.CacheRoot`.
3. **MCP surface for Phase 1 = minimal, additive.** Do NOT add a new generic
   `config.set`/`config.get` MCP tool in Phase 1 — there is no scoped config item
   to set over MCP yet (`prefer_mercenary` migration is Phase 2; prompt-override
   items belong to the sibling children). Phase 1's set/resolve capability is the
   INTERNAL `wsconfig` API, exercised by `wsconfig` unit tests with arbitrary test
   keys. The only MCP-layer change is extending `config.show` / `View` /
   `formatConfigView` to report each value's resolved scope (additive). Keep
   `config.agents_tier` byte-for-byte unchanged (its dispatch, schema, and
   `noAgentHiddenTool` entry untouched). Add the shared `scope` schema-fragment
   helper (cf. `enumStringProperty`) as substrate infra so the sibling children
   adopt it later.
4. **Generic scoped storage layout = additive overlay.** The resolver reads a
   generic scoped key→value overlay: an additive `Overrides map[string]string`
   (or equivalently-named) field on the project/global `Config` and on
   `sessionRecord`, plus a `defaultScope(itemKey) Scope` registry (fallback
   `project`). `builtin` is the code-default floor (resolver returns it when no
   scope holds the key). This keeps `prefer_mercenary` and prompt-override items
   able to ride the same overlay later without another storage change.
