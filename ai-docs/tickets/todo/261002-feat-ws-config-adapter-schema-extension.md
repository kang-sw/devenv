---
title: ws-mcp config schema extension for adapter-declared keys
blocked-by: 261002-feat-pi-lead-ws-owned-compaction
related:
  261002-feat-pi-lead-ws-owned-compaction: prerequisite; its compaction knobs are among the keys this ticket migrates
  260905-feat-ws-pi-harness-config-layer: precedent; the adapter reads ws config through a ws-mcp read tool, not by parsing config files
  260903-feat-ws-pi-goal-loop-compaction-hook: reversed; its "goal-loop knobs never live in ws-mcp config" rule no longer holds
  260924-bug-ws-config-project-scope-is-machine-wide: adjacent; the project scope these keys inherit is machine-wide today
---

# ws-mcp config schema extension for adapter-declared keys

## Background

Pi adapter knobs (goal-loop settle and runaway thresholds, the compaction
advisory percent, child retention, the wait animation) live in the adapter's
`goal-loop-config.json` inside the plugin install directory. They cannot be
set per session, per repository, or globally, `ws/config.tune` cannot touch
them, and lead-tune cannot list them.

ws-mcp config already has the scope stack these knobs need — session (parent
chain inherited by spawned children) > project > repo > global > builtin
(`internal/wsconfig` `scope.go`, `resolver.go`) — but its key registry is
closed: `config.tune` accepts only `configRegistry` rows
(`internal/mcp/config_registry.go`) plus the dynamic `prompt.*` family,
values are strings validated only against an optional enum, and adding a
scalar key takes hand edits across `scope.go`, `server.go`
(`builtinConfigDefaults`, `buildTuningCatalog`), and `config_registry.go`.
Unregistered keys already resolve and appear in `config.list` when written
into a config file by hand. There is no single-key read tool; `config.list`
also scans the playbook tree.

## Decisions

- **Schema extension, not a new scope layer.** An adapter ships a
  declaration manifest of its own keys — key, type (integer with range,
  enum, string, boolean), default, default scope, description. ws-mcp loads
  it so `config.tune` validates writes against the declared type, and the
  tuning catalog that lead-tune reads lists the keys, across the existing
  scopes. Rejected: hard-coding Pi keys in the Go registry (every new knob
  needs a ws-mcp release, and Go source would name one harness).
- **ws-mcp stays harness-neutral.** No harness name appears in the Go
  change; any adapter can declare keys the same way.
- **Narrow read tool.** A new read-only `config.get(key, session_key?)`
  returns the resolved value and its scope, so adapters read one key without
  `config.list`'s playbook scan, following the `config.resolve_agent`
  precedent of reading ws config through a tool.
- **Migration.** The Pi adapter declares `goal-loop-config.json`'s knobs
  (`agent_wait_animation`, `runaway_threshold`,
  `compaction_advisory_percent`, `context_window_override`,
  `settle_delay_ms`, `child_retention_ttl_days`) and any knob
  261002-feat-pi-lead-ws-owned-compaction adds, and reads them through
  `config.get`.
- **Guidance.** Pi lead guidance tells the lead to tune these knobs with
  `ws/config.tune`, and the lead-tune playbook gains the adapter-declared
  settings.
- **Manifest discovery.** Each adapter's launcher sets an environment
  variable listing manifest paths, pointing at its own package's manifest;
  ws-mcp loads every listed manifest. Rejected: a `config` section in each
  package's `runtime.json` (a new file convention inside ws-mcp); a runtime
  `config.declare` call at bridge startup (ws-mcp sessions not started by
  that adapter would not know its keys).
- **Namespaces.** Each manifest declares one namespace prefix (for example
  `pi.`) and every key it declares sits under it; ws-mcp rejects, at load, a
  key that collides with a built-in registry key or with another manifest's
  namespace. Rejected: free key names with first-wins collisions.
- **Retire `goal-loop-config.json`.** Manifest defaults replace its shipped
  values; when ws-mcp is unreachable the adapter uses the manifest defaults.
  Rejected: keeping the file as a lower-precedence fallback (two sources for
  one knob drift, and edits inside the install directory are overwritten on
  plugin update).
- **No adapter-side cache.** The adapter reads a knob through `config.get`
  at each use, as `readGoalLoopConfig` re-reads its file on every call today.
- **Reversal recorded.** This replaces the rule in
  260903-feat-ws-pi-goal-loop-compaction-hook and the `goal-loop.ts` module
  doc that goal-loop knobs never live in ws-mcp config.

## Constraints

- Shipped text and ws-mcp output stay downstream-neutral
  (`ai-docs/manuals/shipped-surface-boundary.md`, AGENTS.md Architecture
  Rule 4).
- The repo scope stays hand-edit only; this ticket does not make it
  writable through `config.tune`.
- Matching manuals from AGENTS.md `### Implementation Conventions`:
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, `ai-docs/manuals/ws-mcp.md`.

## Phases

### Phase 1: Adapter-declared keys in ws-mcp

In `agents-plugin-tool`:

- Load the manifests named by the launcher environment variable at startup;
  validate each manifest (namespace prefix, key types, defaults inside their
  declared range or enum, collisions with built-in keys and other
  namespaces) and report a rejected manifest without failing the server.
- `config.tune` accepts manifest keys at the scopes it already writes,
  validates values against the declared type (integer range, enum, string,
  boolean), and the tuning catalog lists them with description, default,
  and current value.
- New read-only tool `config.get(key, session_key?)` returning the resolved
  value and the scope that supplied it, using the existing resolver (session
  parent chain included); registered wherever the tool inventory is declared
  (MCP schema, every runtime manifest, the Pi delegation policy).

Done when `go test ./...` passes with tests covering manifest loading and
rejection cases, typed validation through `config.tune`, catalog listing,
`config.get` precedence across session, project, repo, global, and the
manifest default, and when the wsflow runtime-contract tests and the
`agents-plugin-pi` suite pass with `config.get` registered.

### Phase 2: Pi adapter migration

Depends on Phase 1.

- Ship the Pi adapter's manifest under the `pi.` namespace declaring every
  `goal-loop-config.json` knob and the compaction knobs from
  261002-feat-pi-lead-ws-owned-compaction; set the manifest variable in the
  Pi launcher.
- Replace `goal-loop-config.json` reads with `config.get` through the
  bridge's MCP client, falling back to manifest defaults when ws-mcp is
  unreachable; remove the file and its readers.
- Pi lead guidance tells the lead to tune these knobs with
  `ws/config.tune`; the lead-tune playbook describes the adapter-declared
  settings, with wsflow and Pi rsrc mirrors updated.

Done when the `agents-plugin-pi` suite passes with tests showing a tuned
value changes goal-loop and compaction behavior and that the defaults apply
when ws-mcp is unreachable, and the playbook render, surface, wsflow-mirror,
and downstream-neutrality suites pass.
