---
title: "Playbook text: legacy ws-agent-spawn idiom and delegate-refused config.resolve_agent"
related:
  261112-research-ws-hosted-pi-missing-workflow-tools: finding source (decision 390)
  261113-feat-ws-pi-mini-lead-profile: neighbour (Pi mini-lead profile)
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
| scope.span | multi-file | playbook_tools.go, playbook_tools_test.go, agents-plugin/rsrc/ticket-worker-elevated/ticket-worker-elevated.md, agents-plugin-wsflow/rsrc/ticket-worker-elevated/ticket-worker-elevated.md |
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

- Point the `pi` SpawnIdiom at `pi.spawn` (`playbook_tools.go:42`).
- Rework the `config.resolve_agent` step in `ticket-worker-elevated` (and any
  other playbook a delegate renders) so a delegate-key worker does not call
  it; use the `pi.spawn` tier argument for Pi.
- Mirror the changes into `agents-plugin-wsflow/` per the mirroring manual.
- Verify: playbook render tests and the wsflow runtime-contract and
  skill-shim drift tests pass.
