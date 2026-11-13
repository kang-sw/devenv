---
title: "Playbook text: legacy ws-agent-spawn idiom and delegate-refused config.resolve_agent"
related:
  261112-research-ws-hosted-pi-missing-workflow-tools: finding source (decision 390)
  261113-feat-ws-pi-mini-lead-profile: neighbour (Pi mini-lead profile)
sage-review-design: required
sage-review-completeness: required
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

- Tier: T0 (playbook content only).
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
- The `config.resolve_agent` fix covers `ticket-worker-elevated` (ws and
  wsflow copies), the only delegate-rendered playbook found calling it; the
  worker greps the rsrc trees for any other delegate-rendered site
  (technical, (482)-(488) acknowledged 2026-11-13).
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

## Open Decision Queue

Last ID: (511)

(511) [open] [policy] How a non-Pi delegate worker picks its child tier once `config.resolve_agent` is gone from `ticket-worker-elevated` (completeness review: missing)
- `roleDelegate` refuses every `config.*` tool except `config.get` (`server.go` `roleAllowsTool`), so Claude and Codex workers rendering `ticket-worker-elevated` hit the same refusal as Pi; the ticket named only the Pi replacement (`pi.spawn` tier). The `claudeSpawnIdiom` comment (`playbook_tools.go` ~L113) also says workers spawn from `config.resolve_agent` output.
- On MAIN's board as (511); options and recommendation as posted there (rec a: the worker gets its child tier through `playbook.render` `tier_override` / the implicit fixed-tier variables, `wsrsrc.ImplicitVariableNames`).
> a, per the board.

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
| scope.span | multi-file | playbook_tools.go, playbook_tools_test.go, ticket-worker-elevated.md in agents-plugin/rsrc, agents-plugin-wsflow/rsrc and agents-plugin-pi/rsrc, both rsrc manifests |
| scope.surface | internal | no exported symbol changes; SpawnIdiom value and playbook prose only (T0) |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go#L288-L313 pins the Pi SpawnIdiom and must change |
| complexity.reuse_points | confirmed | pi.spawn tier argument (pi_tools.go#L285-L299) and existing SpawnIdiom table |
| complexity.side_effect_risk | moderate | Pi SpawnIdiom renders into every playbook with a SpawnIdiom site; rsrc edits touch wsflow mirror and manifest |
| risk.correctness | moderate | the pi harness row also serves native Pi where ws-agent-spawn is live, so the right idiom is unsettled |
| risk.fit | moderate | native-Pi versus hosted-Pi distinction is not modelled by the single pi terminology key |
| risk.test | moderate | the pinned test and Pi delegation-flow tests change; wsflow mirror drift tests must pass |
| risk.security_or_contract | low | playbook text only, T0, no tool or argument surface change |

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
- Rework the `config.resolve_agent` step in `ticket-worker-elevated` (and any
  other playbook a delegate renders) so a delegate-key worker does not call
  it; use the `pi.spawn` tier argument for Pi.
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
  runtime-contract and skill-shim drift tests pass; a rendered
  `ticket-worker-elevated` shows no `config.resolve_agent` step for a
  delegate key and tells a Pi worker to pick the tier through `pi.spawn`.
