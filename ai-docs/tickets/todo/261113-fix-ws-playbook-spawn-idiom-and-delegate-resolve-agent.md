---
title: "Playbook text: legacy ws-agent-spawn idiom and delegate-refused config.resolve_agent"
related:
  261112-research-ws-hosted-pi-missing-workflow-tools: finding source (decision 390)
  261113-feat-ws-pi-mini-lead-profile: neighbour (Pi mini-lead profile)
sage-review-design: blocked
sage-review-completeness: blocked
---

# Playbook spawn idiom and delegate resolve_agent

## Background

Found during the Pi tool-surface research (decision 390, owner 2026-11-13).
Two shipped playbook text issues, independent of the Pi allowlist:

- The `pi` row of the spawn idiom table is the legacy adapter's
  `ws-agent-spawn` (`agents-plugin-tool/internal/mcp/playbook_tools.go:42`,
  `SpawnIdiom`), not `pi.spawn` (`ws-agent-spawn` is still the live native Pi
  extension tool: `agents-plugin-pi/pi-lead-guide.md#L33`,
  `agents-plugin-pi/src/bridge.ts`; the table row is pinned by
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L301`; the same row's
  `ContinueIdiom` is `ws-agent-send`, `playbook_tools.go:43`).
- `ticket-worker-elevated` tells the worker to call `config.resolve_agent`,
  which a delegate key refuses (`roleAllowsTool`,
  `agents-plugin-tool/internal/mcp/server.go#L5678-L5698`; the call is
  `agents-plugin/rsrc/ticket-worker-elevated/ticket-worker-elevated.md#L92`,
  byte-identical in `agents-plugin-wsflow/rsrc/`); a Pi worker picks its tier
  through `pi.spawn`'s `tier` instead.

## Decisions

- Tier: T0 (playbook content plus one additive delegate-role tool permission, (511)).
- (478)=b (owner, 2026-11-13): the Pi spawn idiom switches at render time.
  An MCP-only Pi session (a `pi.spawn` child, or a hosted Pi lead running on
  the MCP bridge without the `agents-plugin-pi` extension) gets
  `pi.spawn` / `pi.send`; a native Pi session with the extension keeps
  `ws-agent-spawn` / `ws-agent-send`. `ContinueIdiom` follows the same
  switch. The owner-confirmed native terminology of
  `260911-feat-ws-pi-harness-idiom-playbook-overlays` (e4cf74ad) stays.
  Rejected: (a) repointing the `pi` row for everyone (native Pi leads would
  be told a tool they lack, reversing e4cf74ad); (c) leaving the idiom
  unchanged (hosted Pi workers keep the wrong tool name).
- (511)=b (owner, 2026-11-13: "이게 리드쪽 플레이북이랑 일관된다고
  봅니다."): the delegate role is allowed `config.resolve_agent`, read-only
  like `config.get` (`server.go` `roleAllowsTool`), so the
  `ticket-worker-elevated` step that resolves the child tier through it
  stays, for every harness, consistent with the lead playbooks. Tier stays
  T0 (an additive tool permission). Rejected: (a) rendering with
  `playbook.render(tier_override)` and spawning at the returned model
  (rewrites the playbook step, diverges from the lead-side flow); (c) no
  tier choice for delegate-key workers (narrows the 260911 D1 "tier
  resolution is mandatory").
- **Switch signal and full MCP-only row** (design review 2026-11-13,
  autonomous): MCP-only Pi sessions probably do not render as harness `pi`:
  the server returns `pi` only for clientInfo `ws-pi-bridge` (sent by the
  native extension, `agents-plugin-pi/src/bridge.ts:1071`) or a launcher
  `WS_MCP_HARNESS` that nothing sets for `pi.spawn` children (`pi.go:262-265`)
  or the hosted bridge fallback (`pi_lead.go:44`), which connect through Pi's
  `builtin:mcp` client (its clientInfo is unverified). So the switch keys on
  the caller's recorded launch signal regardless of the detected harness,
  and the MCP-only branch supplies a full terminology row: besides
  `SpawnIdiom` and `ContinueIdiom`, `ExploreAgent` becomes a read-only
  `pi.spawn` at the small tier, because the native `explore` tool
  (`playbook_tools.go:41`) is just as absent there (the (478) rationale:
  never name a tool the session lacks). The signal is derived from existing
  records (a non-lead Pi backend runs `--no-extensions` plus `builtin:mcp`
  unless `Profile.Plugins` adds the extension; a hosted lead with a non-empty
  `PluginWarning` runs on the bridge), not from the entry path (a hosted
  lead running the committed bundle of
  `261113-bug-ws-hosted-pi-start-misses-uninstalled-extension-deps` is
  native). If the worker must add a Record field instead, it is an additive
  store field (still T0) and the Result corrects the tier note.
- **Mirrors** (design review 2026-11-13, autonomous):
  `ticket-worker-elevated.md` has three byte-identical copies (ws, wsflow,
  `agents-plugin-pi/rsrc/`), guarded by `TestPiMirrorUpToDate`
  (`internal/wsrsrc/pi_mirror_test.go`) and the CI `diff -rq`
  (`ws-mcp-release.yml:95`); all three change, with both rsrc manifests
  re-hashed.
- Separate ticket from the mini-lead profile because this changes shipped
  playbook text. Rejected: fixing it inside the profile ticket.

## Constraints

- Shipped text must not depend on anything repo-specific
  (`ai-docs/manuals/shipped-surface-boundary.md`).
- Playbook and skill edits follow `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (wsflow shims and mirrors must stay
  in sync).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/rsrc/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261112-research-ws-hosted-pi-missing-workflow-tools (2026-11-12, Confirmed Decisions): "(390) playbook text issues go to `261113-fix-ws-playbook-spawn-idiom-and-delegate-resolve-agent`." — bearing: supports
- 260911-feat-ws-pi-harness-idiom-playbook-overlays (2026-09-12, commit e4cf74ad): "The owner confirmed exact Pi mechanism terminology for explore, ws-agent-spawn, and ws-agent-send." — bearing: contradiction-candidate
- 260911-refactor-lead-delegate-executor-tier-resolution (2026-09-11, Decisions): "Tier resolution is mandatory, not advisory (D1). The delegate must resolve its executor model through `config.resolve_agent(tier)`" — bearing: constrains
- 260609-feat-ws-playbook-surface-mvp (2026-06-09, Decisions): "Harness differences ship as data. Shared playbook body + a per-harness terminology table (`explore_agent`, `spawn_idiom`, `continue_idiom`...)" — bearing: constrains
- 260912-bug-ws-pi-agent-spawn-concrete-model-override (2026-09-13, Result 41dcbb31): "Concrete selection bypasses `config.resolve_agent`" (native `ws-agent-spawn` model selection) — bearing: supports
- 261113-feat-ws-pi-mini-lead-profile (2026-11-13, Decisions): "`pi-mini-lead` = `pi-worker` tools plus `tickets.query/acquire/release/ close/move/verify/checklist`, read-only `note.query`, `playbook.read`, `playbook.render`..." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | server.go roleAllowsTool and its role test, playbook_tools.go, playbook_tools_test.go, ticket-worker-elevated.md in agents-plugin/rsrc, agents-plugin-wsflow/rsrc and agents-plugin-pi/rsrc, both rsrc manifests |
| scope.surface | internal | no exported symbol changes; SpawnIdiom value and playbook prose only (T0) |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go#L288-L313 pins the Pi SpawnIdiom and must change |
| complexity.reuse_points | confirmed | pi.spawn tier argument (pi_tools.go#L285-L299) and existing SpawnIdiom table |
| complexity.side_effect_risk | moderate | Pi SpawnIdiom renders into every playbook with a SpawnIdiom site; rsrc edits touch wsflow mirror and manifest |
| risk.correctness | moderate | the pi harness row also serves native Pi where ws-agent-spawn is live, so the right idiom is unsettled |
| risk.fit | moderate | native-Pi versus hosted-Pi distinction is not modelled by the single pi terminology key |
| risk.test | moderate | the pinned test and Pi delegation-flow tests change; wsflow mirror drift tests must pass |
| risk.security_or_contract | low | T0: playbook text plus an additive read-only config.resolve_agent permission for the delegate role ((511)=b); no tool or argument shape change |

## Phases

### Phase 1: Correct the Pi spawn idiom and the delegate resolve_agent call

- Add the render-time switch of Decision (478): keep the `pi` row
  (`playbook_tools.go:40-43`) for native Pi and select `pi.spawn` /
  `pi.send` wording when the rendering caller is an MCP-only Pi session.
  Derive that signal from what ws already records for the caller (the Pi
  agent's launch mode in `wsagent`: `--no-extensions` child or bridge
  fallback), not from a new caller-supplied argument; name the signal in the
  Result. Update the pinned row test (`playbook_tools_test.go`) and add a
  test per branch.
- Allow `config.resolve_agent` for the delegate role in `roleAllowsTool`
  ((511)=b) and add a role test; the `ticket-worker-elevated` resolve step
  stays. In the MCP-only Pi row, the spawn idiom passes the resolved tier to
  `pi.spawn`.
- Mirror the changes into `agents-plugin-wsflow/` per the mirroring manual
  and into `agents-plugin-pi/rsrc/`; re-hash both rsrc manifests.
- Overlap: ready `261113-feat-ws-ticket-visual-reference-and-ask-classes`
  Phase 2 (in flight) edits the worker playbooks, including all three
  elevated copies; whichever lands second rebases its text and manifest
  hashes.
- Verify: playbook render tests (both idiom branches, including
  `ExploreAgent`), at least one test through the real detection path (from
  the caller's session key or initialize to the rendered idiom, not a
  synthetic harness value), `TestPiMirrorUpToDate`, and the wsflow
  runtime-contract and skill-shim drift tests pass; a
  delegate-key session's `config.resolve_agent` call succeeds (role test)
  while other `config.*` writes stay refused; a rendered
  `ticket-worker-elevated` for an MCP-only Pi caller names `pi.spawn` with the
  resolved tier.

## Blocked (2026-11-13)

### Design Reviewer — concern

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | MCP-only Pi sessions are probably not detected as harness pi | important | autonomous |
| 2 | ExploreAgent also names a tool MCP-only sessions lack | important | autonomous |
| 3 | The resolve_agent refusal hits every harness, but only the Pi replacement is named | minor | autonomous |
| 4 | Third mirror not named | minor | autonomous |
| 5 | No launch-mode field exists, and adding one changes the tier note | minor | autonomous |
| 6 | Same file as a ready ticket | minor | autonomous |
| 7 | Cross-ticket [261113-fix-ws-playbook-spawn-idiom-and-delegate-resolve-agent, 261113-bug-ws-hosted-pi-start-misses-uninstalled-extension-deps]: single bridge-fallback signal; bundle-loaded counts as native | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Non-Pi delegate worker child-tier selection unspecified (settled by owner (511)=b) | important |
