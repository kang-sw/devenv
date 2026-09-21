---
title: Allow global scope override for agents.tier model-tier tuning
related:
  260619-feat-ws-layered-config-scope-substrate: predecessor context (deferred the tier resolver fold)
  260814-refactor-config-collapse-tuning-knobs-to-list-tune: predecessor context (built config.list/config.tune, preserved the bypass)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f867e66063b53f6a
sage-review-completeness-reviewed: f867e66063b53f6a
---

# Allow global scope override for agents.tier model-tier tuning

## Background

`config.tune` hard-restricts `agents.tier` to project scope. The restriction is
a hardcoded per-key veto in the `config.tune` dispatch path (search
`agents-plugin-tool/internal/mcp/server.go` for the `entry.Key == "agents.tier"`
branch that rejects any explicit scope other than project) — the only per-key
scope veto in the tool. It is not a considered policy that tier must be
machine-local; it is deferred technical debt from a resolver bypass.

Every other tunable key routes through the generic `wsconfig.Resolver`
(`session > project > repo > global > builtin`) whose stored shape is a flat
`map[string]string` overlay, so those keys already accept an explicit `global`
scope. `agents.tier` is `ResolverBacked: false`: its value is a structured
`AgentTier{Backend, Model, Effort}` nested at `Config.Agents.ModelAliases[tier]
[harness]`, incompatible with the flat overlay, so it writes directly to the
single project config file via `wsconfig.SetAgentsTierForHarness`, bypassing the
resolver. The registry entry for `agents.tier` therefore exposes only a
`harness` selector and no `scope` selector, and the catalog never advertises a
scope axis for it.

The bypass was preserved deliberately, twice, as out-of-scope for the ticket at
hand — not because tier was judged project-only:
- `.done/260619-feat-ws-layered-config-scope-substrate` introduced the layered
  resolver and left the tier predecessor tool byte-for-byte unchanged.
- `.done/260814-refactor-config-collapse-tuning-knobs-to-list-tune` built today's
  `config.list`/`config.tune` and recorded that a resolver fold of `agents.tier`
  would be a capability extension out of bounds for a behavior-preserving phase,
  with a forward note that the tier adapter must stay a compound writer, not be
  flattened into the resolver.

The tier-taxonomy design frames the concrete tier→model mapping (exactly what
`agents.tier` stores) as user-level, cross-project config, which argues *for*
global support rather than a project-only restriction.

This ticket removes the restriction by giving the existing compound writer a
scope axis, without folding tier into the resolver.

## Decisions

- **Keep the compound (non-resolver) writer; add a scope axis to it.** Do not
  fold `agents.tier` into `wsconfig.Resolver`. The structured `ModelAliases`
  shape is incompatible with the resolver's flat overlay, and the two prior
  tickets explicitly deferred that fold. Rejected alternative — full resolver
  fold: larger change, contradicts the recorded forward note, and unnecessary to
  reach global support.
- **Scopes: project and global only.** `session` is not meaningful for
  `agents.tier` because it is not resolver-backed / not session-layered; do not
  invent a session write path for tier. `repo` remains hand-edited/read-only as
  for every other key.
- **Read-time merge is a per-`(tier, harness)` leaf overlay:**
  `builtin < global < project`. A project entry for a given `(tier, harness)`
  overrides the global entry for the same `(tier, harness)`; where project has
  no entry for that leaf, the global value stands; builtin defaults remain the
  floor. Rejected alternative — whole tier-map replacement (any project entry
  for a tier replaces the entire global definition of that tier): breaks partial
  specialization and makes `agents.tier` an anomaly versus the resolver's
  per-key overlay semantics. The chosen leaf overlay mirrors the resolver's
  per-key overlay one level deeper, matching how every other key already
  behaves.

## Constraints

- Editing `agents-plugin-tool/internal/mcp/` requires reading
  `ai-docs/manuals/ws-mcp.md` first (per Implementation Conventions).
- The compound writer must stay a compound writer; do not flatten `agents.tier`
  into `Config.Overrides` / the resolver.
- Existing tests assert the `agents.tier` knob's `SelectorFields` contain only
  `harness` (enum: claude/codex/pi/default) — see the assertions in
  `agents-plugin-tool/internal/mcp/prompt_override_test.go`. These must be
  updated to reflect the added `scope` selector.
- A CLI mirror may re-implement the same project-only veto — check
  `agents-plugin-tool/cmd/ws-mcp/main.go` (or the equivalent CLI entry) and keep
  it consistent with the tool behavior.
- This changes `config.tune`'s scope contract, an observable protocol/API
  semantics change under AGENTS.md; the direction was approved in discussion.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Non-goals

- `sage_review_design_tier` / `sage_review_completeness_tier`: conceptual
  cousins (model-tier settings, resolver-backed, currently with no write path at
  all) but tied to an as-yet-undefined role model. Deferred to a separate future
  ticket; out of scope here.
- No full resolver fold of `agents.tier`.

## Prior Decisions

- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-23, Result): "the `agents.tier` adapter (`ResolverBacked:false`) must stay a compound writer, not be flattened" — bearing: supports
- 260619-feat-ws-layered-config-scope-substrate (2026-06-19, Result): "`config.agents_tier` byte-for-byte unchanged" — bearing: supports
- 260917-feat-ws-committed-project-config-scope (2026-09-17, Result): "`ScopeRepo` is excluded from `ScopeSchemaEnum`, so `config.tune` never offers `repo` as a writable scope; the committed file is hand-edited" — bearing: supports
- 260626-bug-sage-review-config-setter-missing (2026-09-11, commit c1699e1e): "Advertise session, project, and global scope explicitly for the Sage knob while preserving the resolver's project default and auto builtin" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go#L862-867, agents-plugin-tool/internal/mcp/config_registry.go#L143-181, agents-plugin-tool/internal/wsconfig/config.go#L150-293, agents-plugin-tool/internal/mcp/prompt_override_test.go#L1278, agents-plugin-tool/cmd/ws-mcp/main.go#L255-292 |
| scope.surface | public-interface | config.tune's scope contract for agents.tier is caller-visible; ticket's own Constraints call it an observable protocol/API semantics change |
| scope.new_public_symbol | yes | planned global-scope writer parallel to wsconfig.SetAgentsTierForHarness (config.go#L150); exact symbol name not decided by the ticket |
| scope.new_type_contract | no | reuses existing AgentTier{Backend,Model,Effort} (config.go#L60) and wsconfig.Scope (scope.go#L9-21); no new type named |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/prompt_override_test.go (SelectorFields assertions L1278, L1402), agents-plugin-tool/internal/wsconfig/config_test.go (tier resolution tests) |
| complexity.reuse_points | confirmed | wsconfig.GlobalPath/loadGlobalConfig global-file pattern (global.go#L17-34) and wsconfig.SetAgentsTierForHarness project writer (config.go#L150) are the parallel templates the ticket names |
| complexity.side_effect_risk | moderate | wsconfig.ResolveAgentTierForHarness (config.go#L273) is called from both internal/mcp/playbook_tools.go and internal/mcp/server.go; a read-time merge change touches every tier-resolution consumer |
| risk.correctness | moderate | the per-(tier, harness) leaf overlay merge is new logic with no existing equivalent in the compound (non-resolver) reader to diff against |
| risk.fit | low | parallels the already-landed project/global file pattern (global.go) and the repo-scope precedent (260917-feat-ws-committed-project-config-scope) for adding a scope axis, while keeping agents.tier outside the resolver as the ticket requires |
| risk.test | moderate | requires new precedence/overlay test coverage beyond the existing SelectorFields assertions, per the ticket's own Verification bullet |
| risk.security_or_contract | moderate | ticket's own Constraints flag this as an observable protocol/API semantics change to config.tune, approved in discussion but still a contract shift |

## Phases

### Phase 1: Add project/global scope axis to agents.tier tuning

Goal: `config.tune` accepts `scope: "global"` (and `"project"`, the current
default) for `agents.tier`, persists the structured `ModelAliases` value to the
matching config file, and the compound reader merges `builtin < global <
project` at `(tier, harness)` leaf granularity.

Work:
- **Narrow** (do not simply delete) the `entry.Key == "agents.tier"` scope check
  in the `config.tune` dispatch path so it accepts `{project, global}` and
  rejects everything else. This veto is currently the *only* scope check for
  `agents.tier` (unlike `harness`, there is no generic scope-enum enforcement in
  the dispatch path), so a bare deletion would let `scope: "session"`/`"repo"`
  fall through to the write branch and silently write project instead of
  erroring.
- Add a `scope` selector (project, global) to the `agents.tier` registry entry
  so the catalog advertises it; leave the `harness` selector intact.
- Add a new global-scope writer for the structured `ModelAliases` shape,
  mirroring the existing project writer `wsconfig.SetAgentsTierForHarness` but
  targeting the global config file via the `GlobalPath`/`loadGlobalConfig`
  pattern (there is no tier-specific global writer today; this creates one).
- Apply the read-time overlay at **every** tier-resolution read site, not one
  reader. Tier resolution has two independent `Load()` → `resolveAliasMapping`
  entry points: `ResolveAgentTierForHarness` (introspection: `config.resolve_agent`,
  playbook validation) **and** `ResolveAgentForHarnessConfig` (the actual
  agent-spawn / `playbook.render` path). The `builtin < global < project`
  per-`(tier, harness)` leaf overlay must take effect at both, or a global
  override would surface in `config.list`/`config.resolve_agent` yet be ignored
  when agents actually spawn — the feature's primary purpose silently failing.
  The merge must NOT live in `Load()` (the project writer reuses `Load` before
  saving to the project file, so merging global there would pollute project
  writes) nor purely inside `resolveAliasMapping` (it receives an already-loaded
  `cfg` and never reads the global file); introduce a shared merge step both read
  sites call.
- Update the `prompt_override_test.go` `SelectorFields` assertions for the added
  `scope` selector, and add coverage for a global write and the
  global→project leaf overlay on read — asserting the overlay through the
  spawn-path resolver, not only the introspection reader.
- CLI mirror (`cmd/ws-mcp/main.go` `configTune`): today it has no scope flag and
  no veto — it unconditionally calls the project writer. Leaving the CLI
  project-only is acceptable for this ticket (the MCP tool is the contract
  surface); do not add a `--scope` flag unless trivial. Just confirm the CLI
  path is not broken by the writer/reader changes.

Verification: `go test ./...` under `agents-plugin-tool/` (at minimum the mcp
and wsconfig packages) passes; new tests cover global write + leaf overlay
precedence; catalog output for `agents.tier` advertises the scope selector.
