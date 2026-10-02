---
title: ws-mcp config schema extension for adapter-declared keys
blocked-by: 261002-feat-pi-lead-ws-owned-compaction
related:
  261002-feat-pi-lead-ws-owned-compaction: prerequisite; its compaction knobs are among the keys this ticket migrates
  260905-feat-ws-pi-harness-config-layer: precedent; the adapter reads ws config through a ws-mcp read tool, not by parsing config files
  260903-feat-ws-pi-goal-loop-compaction-hook: reversed; its "goal-loop knobs never live in ws-mcp config" rule no longer holds
  260924-bug-ws-config-project-scope-is-machine-wide: adjacent; the project scope these keys inherit is machine-wide today
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: 0c5360b97988e306
completed: 2026-10-02
---

# ws-mcp config schema extension for adapter-declared keys

## Background

Pi adapter knobs (goal-loop settle and runaway thresholds, the compaction
advisory percent, child retention, the wait animation) are read from the
adapter's `goal-loop-config.json` inside the plugin install directory (the
shipped file sets only `agent_wait_animation` and `child_retention_ttl_days`;
the other four are optional keys with in-code defaults,
agents-plugin-pi/goal-loop-config.json, agents-plugin-pi/src/goal-loop.ts#L92-L107). They cannot be
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
  enum, string, boolean), default, default scope (the scope `config.tune`
  writes when the call names none, the registry's existing
  `configKeyEntry.DefaultScope` concept), description. ws-mcp loads
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
- **Typed reads, string storage.** `config.get` returns a manifest-declared
  key's value as a JSON value of its declared type (number, boolean, or
  string); built-in registry keys and undeclared keys stay strings. Config
  files keep storing strings and the existing resolver is unchanged; the
  conversion happens at read. Rejected: always returning strings (discards
  the type ws-mcp already validated against the manifest, and every adapter
  re-implements parsing that can drift).
- **Invalid and unknown reads never error.** When a stored string fails a
  manifest key's declared type or range, `config.get` returns the manifest
  default, reports the default as the supplying scope, and adds a warning
  naming the invalid stored value and its scope, matching the Pi adapter's
  never-hard-fail resolvers. For a key neither registered nor declared it
  returns the stored string and its scope when one is set and an explicit
  unset result otherwise, as such keys already resolve in `config.list`.
- **Writes stay strings.** `config.tune` takes a manifest key's value as a
  string, as it does for every key but `agents.tier`
  (260928-bug-config-tune-value-schema-untyped), and validates it by parsing
  against the declared type; only `config.get` returns typed values.
- **Migration.** The Pi adapter declares `goal-loop-config.json`'s knobs
  (`agent_wait_animation`, `runaway_threshold`,
  `compaction_advisory_percent`, `context_window_override`,
  `settle_delay_ms`, `child_retention_ttl_days`) and any knob
  261002-feat-pi-lead-ws-owned-compaction adds, and reads them through
  `config.get`.
- **`child_retention_ttl_days` becomes a plain integer.** It is declared as
  an integer with minimum 0, where `0` disables retention (today's `false`)
  and the shipped default stays 30. Today a non-positive value falls back to
  the default; that fallback is replaced by the `0` = disabled meaning.
  Rejected: a union type in the manifest (the only knob that needs one);
  splitting it into an enabled boolean and a days integer (one more knob for
  no gain).
- **Guidance.** Pi lead guidance tells the lead to tune these knobs with
  `ws/config.tune`, and the lead-tune playbook gains the adapter-declared
  settings.
- **Manifest discovery.** Each adapter's launcher sets an environment
  variable listing manifest paths, pointing at its own package's manifest;
  ws-mcp loads every listed manifest. Rejected: a `config` section in each
  package's `runtime.json` (a new file convention inside ws-mcp); a runtime
  `config.declare` call at bridge startup (it would add a mutable,
  call-order-dependent registry to ws-mcp for no reach the environment
  variable lacks). Either way, only a ws-mcp instance launched by the
  declaring adapter knows its keys: a lead on another harness cannot tune
  `pi.*` keys through `config.tune`, and that is intended — the knobs are
  tuned from the Pi lead that uses them.
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
- **Sequential landing.** The whole ticket stays `blocked-by:`
  261002-feat-pi-lead-ws-owned-compaction and is dispatched after that
  ticket reaches `.done/`. Rejected: splitting the Pi migration into its own
  ticket blocked on the compaction ticket's Phase 2 so the ws-mcp phase could
  start early (the parallelism is not needed); dropping `blocked-by:` for a
  prose-only dependency (no hard gate).
- **Reversal recorded.** This replaces the rule in
  260903-feat-ws-pi-goal-loop-compaction-hook and the `goal-loop.ts` module
  doc that goal-loop knobs never live in ws-mcp config.

## Constraints

- Shipped text and ws-mcp output stay downstream-neutral
  (`ai-docs/manuals/shipped-surface-boundary.md`, AGENTS.md Architecture
  Rule 4).
- The repo scope stays hand-edit only; this ticket does not make it
  writable through `config.tune`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Matching manuals from AGENTS.md `### Implementation Conventions`:
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, `ai-docs/manuals/ws-mcp.md`.

## Prior Decisions

- 261001-feat-config-repo-scope-and-tune-weight-guidance (2026-10-01, Decisions): "Repo scope stays read-only to `config.tune`. When the user wants a team-wide setting, lead-tune drafts the `.ws-workflow/config.json` edit from `config.list`'s repo information" — bearing: constrains
- 261001-feat-config-repo-scope-and-tune-weight-guidance (2026-10-01, Decisions): "Lever effects are single-sourced in `config.list`. Each weight-lever knob's Go description states what each value does ... lead-tune translates that into the user's language" — bearing: constrains
- 260928-bug-config-tune-value-schema-untyped (2026-09-28, Result): "Keys other than `agents.tier` reject a present non-null non-string `value` via `tuneStringValueRejection`" — bearing: constrains
- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-23, Result): "`session_key` is no longer a schema-static `required` field on any config write ... the per-key rule ... is enforced at dispatch" — bearing: constrains
- 260905-feat-ws-pi-harness-config-layer (2026-09-06, Result): "`pi` is a member of both `normalizedHarness` copies, `promptHarnessEnum`, the prompt-override listing buckets" — bearing: supports
- 260917-feat-ws-committed-project-config-scope (2026-09-17, Result): "Allowed-key schema / `.gitignore`: left minimal (no schema validation added); a consumer adds keys as needed." — bearing: supports
- 260625-feat-lead-tune-schema-backed-knob-catalog (2026-06-25, Resolution): "Implemented schema-backed config.tuning catalog and updated lead-tune to consume catalog metadata." — bearing: supports
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, 0cda382d): "Config schema extension ticketed separately with the compaction ticket as prerequisite, reversing 260903's rule that goal-loop knobs never live in ws-mcp config." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/, agents-plugin-tool/internal/wsconfig/, agents-plugin-pi/, agents-plugin/rsrc/lead-tune, agents-plugin-wsflow mirrors, runtime.json manifests |
| scope.surface | cross-module | new MCP tool config.get plus manifest env var consumed by Go core and the Pi adapter |
| scope.new_public_symbol | yes | config.get MCP tool and the manifest-path environment variable |
| scope.new_type_contract | yes | adapter key manifest schema (key, type, range, default, scope, namespace) and typed config.get return |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/config_repo_scope_test.go, agents-plugin-pi/test/goal-loop.test.ts; new manifest tests likely added |
| complexity.reuse_points | confirmed | wsconfig Resolver and configRegistry/buildTuningCatalog in internal/mcp/config_registry.go and server.go; config.resolve_agent precedent |
| complexity.side_effect_risk | moderate | touches config.tune validation path, tool inventory in every runtime manifest, and Pi runtime reads of retired file |
| risk.correctness | moderate | typed-vs-string reads, scope precedence, and 0=disabled semantic change for child retention |
| risk.fit | moderate | reverses a recorded rule (260903) and adds a manifest discovery mechanism to ws-mcp |
| risk.test | moderate | cross-package suites (Go, wsflow runtime-contract, Pi node tests) must all stay green; unreachable-ws-mcp fallback is hard to test |
| risk.security_or_contract | moderate | new tool contract, env-var-loaded manifests from the filesystem, namespace collision rules |

## Phases

### Phase 1: Adapter-declared keys in ws-mcp

In `agents-plugin-tool`:

- Left to the implementer: the manifest file format, the environment
  variable's name and path separator (Phase 1 defines them and Phase 2's
  launcher uses them as defined), and the channel that reports a rejected
  manifest.
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
  parent chain included), typed per the declared manifest type for
  manifest keys and as a string otherwise; registered wherever the tool inventory is declared
  (MCP schema, every runtime manifest, the Pi delegation policy).

Done when `go test ./...` passes with tests covering manifest loading and
rejection cases, typed validation through `config.tune`, catalog listing,
`config.get` precedence across session, project, repo, global, and the
manifest default, typed values for manifest keys and strings for other
keys, the invalid-stored-value fallback with its warning, and the unset
result for an unknown key, and when the wsflow runtime-contract tests and the
`agents-plugin-pi` suite pass with `config.get` registered.

### Result (1ff2e25ad) - 2026-10-02

- Manifest format: JSON `{schema_version: 1, namespace: "<seg>.", keys: [{key, type (integer|boolean|string|enum), minimum, maximum, enum, default, default_scope (session|project|global, default project), description}]}`; keys carry the full namespaced name; unknown fields reject. Loader in `agents-plugin-tool/internal/mcp/config_manifest.go`.
- Env var `WS_MCP_CONFIG_MANIFESTS`, an OS path list. A rejected manifest is rejected whole, logged to ws-mcp stderr, and listed by `config.list` (text trailer, JSON `manifest_errors`); the earlier-listed manifest keeps a contested namespace.
- `config.get(key, session_key?)` returns `{key, value, scope, warnings?}`: typed for declared keys, string otherwise, `scope: "unset"` with null value for an unknown unset key, and for an invalid stored value the declared default at scope `builtin` plus a warning. Callable by delegate and leaf keys so children read the lead's session-scope values.
- Declared keys stay off the wsconfig global scope registry; `config.tune` applies the manifest default scope as the explicit scope.
- Verification: `go test ./...` passes; wsflow package tests (14) pass; the `agents-plugin-pi` suite passes once its runtime cache holds a ws-mcp build carrying `config.get` (the published v0.46.26 release asset lacks it, so real-child integration tests time out against it until the next release, as for any runtime.json tool addition).

### Phase 2: Pi adapter migration

Depends on Phase 1.

- Ship the Pi adapter's manifest under the `pi.` namespace declaring every
  `goal-loop-config.json` knob and the compaction knobs from
  261002-feat-pi-lead-ws-owned-compaction; set the manifest variable in the
  Pi launcher.
- Replace `goal-loop-config.json` reads with `config.get` through the
  bridge's MCP client (`child_retention_ttl_days` `0` disables retention), falling back to manifest defaults when ws-mcp is
  unreachable; remove the file and its readers.
- Pi lead guidance tells the lead to tune these knobs with
  `ws/config.tune`; the lead-tune playbook describes the adapter-declared
  settings, with wsflow and Pi rsrc mirrors updated.

Done when the `agents-plugin-pi` suite passes with tests showing a tuned
value changes goal-loop and compaction behavior and that the defaults apply
when ws-mcp is unreachable, and the playbook render, surface, wsflow-mirror,
and downstream-neutrality suites pass.

### Result (1949b0e50) - 2026-10-02

- `agents-plugin-pi/config-manifest.json` declares nine `pi.*` knobs: the six `goal-loop-config.json` knobs plus `compaction_hard_percent`, `compaction_user_messages_budget_tokens`, `compaction_user_message_cap_tokens`. All are integers except the boolean animation knob. `agent_wait_animation` and `child_retention_ttl_days` default to global scope.
- The bridge sets `WS_MCP_CONFIG_MANIFESTS` for every role's ws-mcp. `src/adapter-config.ts` reads knobs with one `config.get` per knob per use (2s timeout); an absent, failed, or slow read resolves to the built-in default, which a test pins equal to the manifest default.
- The file, `readGoalLoopConfig`, and their package/test references are removed. `child_retention_ttl_days: 0` disables pruning.
- Read granularity: one settle cycle reads at arm and carries the values to its fire; the agent widget reads per `refresh()` and its animation ticks reuse that answer. The goal-loop settle arm, the compaction triggers, and the wake-recovery timer guard the async gap (cancel or compaction during the read schedules nothing).
- `pi-lead-guide.md` routes adapter tuning to `ws__config_tune`; `lead-tune` handles `adapter_setting` knobs; rsrc manifest plus the wsflow and Pi rsrc mirrors are regenerated.
- Review (lite) fixes in f16371bfa: deferred-reader tests for the async guards, quoted-integer manifest default rejected.
- Verification: `agents-plugin-pi` `npm test` 1997 tests, 0 fail (runtime cache primed with a local build, see Phase 1); `go test ./...` passes including the shipped-manifest load test, rsrc mirror and manifest drift tests.
