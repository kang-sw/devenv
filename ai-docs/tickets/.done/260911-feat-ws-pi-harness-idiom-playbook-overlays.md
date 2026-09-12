---
title: "Render shared ws playbooks Pi-correct: pi terminology entry and selected .pi.md overlays"
parent: 260911-epic-ws-pi-refound-resync-harness-peer
related:
  260905-feat-ws-pi-harness-config-layer: landed the harness-peer infra this ticket fills the last data/text gaps of
  260906-bug-ws-pi-rsrc-mirror-drift: overlays are authored upstream under agents-plugin/rsrc and reach Pi via its mirror, never authored in the mirror
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b6356fde220ffe6a
sage-review-completeness-reviewed: b6356fde220ffe6a
completed: 2026-09-12
---

# Render shared ws playbooks Pi-correct: pi terminology entry and selected .pi.md overlays

## Background

`260905` landed Pi as a full member of ws-mcp's harness-keyed surfaces (the
harness enum, `ws-pi-bridge` detection, `agents.tier`, prompt override-points,
and `.pi.md` variant selection). Two gaps leave shared playbooks rendering Pi
with host-neutral fallbacks:

1. `playbookTerminologyTable`
   (`agents-plugin-tool/internal/mcp/playbook_tools.go`) has no `"pi"` key, so
   `SpawnIdiom` / `ExploreAgent` / `ContinueIdiom` resolve to the host-neutral
   `""` table ("spawning a subagent", ...) for Pi sessions.
2. No `.pi.md` overlays exist under `agents-plugin/rsrc/` (only `.codex.md`),
   though the loader already selects them for a `harness=pi` session.

This ticket fills both so a Pi session renders the correct spawn idiom and the
Pi-specific deltas of the delegation-flow playbooks.

## Decisions

- **Terminology home is ws-mcp Go, not a Pi-side override.** Reserved terminology
  vars are deterministically overwritten by the tool-injected terminology layer
  over caller context (`buildPlaybookVars` layer ordering; the "tool-injected
  wins on collision" invariant), so a Pi adapter cannot set them through
  `playbook.render(context:)`. The `"pi"` entry (`ExploreAgent` / `SpawnIdiom` /
  `ContinueIdiom`) is added to the Go table with the exact literals
  ``the `explore` researcher``, `ws-agent-spawn`, and
  `ws-agent-send to the same alias or agent id`, respectively. These name
  the current Pi mechanisms (`agents-plugin-pi/src/spawner.ts#L3508-L3511`,
  `#L3613-L3616`, `#L3689-L3702`) and fit the shared playbooks' sentence grammar.
  This is data consistent with the existing `codex` / `claude` entries, not
  host-specific logic — compatible with AGENTS.md clause (1) and the
  harness-peer clause.
- **No overlay without a demonstrated delta.** The current shared bases need no
  `.pi.md` overlay merely to substitute terminology: `lead-discuss`, `lead-run`,
  and `ticket-worker` use the terminology variables, while `lead-delegate` is a
  non-delegating print playbook with none (`agents-plugin/rsrc/`). Do not
  pre-author an overlay. Add one under
  `agents-plugin/rsrc/<name>/<name>.pi.md` only if Pi dogfood identifies exact
  wrong shared-base text that terminology cannot express; then mirror it into
  `agents-plugin-pi/rsrc` per `260906`.
- **Pure-Pi-mechanism text is out of scope.** execute-worker / orchestrator
  guides describing `ws-execute` / `ws-worker-exec` / approval and
  `ws-report-to-lead` have no shared base and stay adapter-owned
  `systemPromptPath` guides (epic Cross-Child Decision 4).
- Excluded unless Pi dogfood reproduces a defect: `detectHarnessFromRaw` has no
  `"pi"` substring fallback (non-initialize paths rely on the sticky
  initialize-time `clientInfo.name=="ws-pi-bridge"` value); pi model tier
  defaults are not seeded in `applyDefaultModelAliases` (pure data via
  `config.tune` / `defaultModelAliases`). Do not fold either into this ticket
  without that dogfood evidence (`agents-plugin-tool/internal/mcp/server.go#L2809-L2851`,
  `agents-plugin-tool/internal/wsconfig/config_test.go#L688-L693`).

## Constraints

- **Develop-authored (AGENTS.md clause 1).** The terminology entry and any
  `.pi.md` overlay are authored on `develop` and reach the Pi track by
  cherry-pick plus the `260906` mirror. The current `track/pi-agent` worktree's
  `b2509f01` develop snapshot permits verification, not authorship.
- **Ready gate satisfied:** `epic/refound` merged to `develop` as `77409c56`,
  and `260911-chore-ws-pi-track-sync-to-refound` is done with the settled shared
  `rsrc/` absorbed (`ai-docs/tickets/.done/260911-chore-ws-pi-track-sync-to-refound.md`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/playbook_tools.go, playbook_tools_test.go; conditional agents-plugin/rsrc and agents-plugin-pi/rsrc overlay |
| scope.surface | public-interface | playbook.render output changes for detected Pi sessions |
| scope.new_public_symbol | no | existing internal terminology table and Pi native tools |
| scope.new_type_contract | no | existing map[string]map[string]string and playbook.render variables |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go; agents-plugin-pi/test/version-check.test.ts |
| complexity.reuse_points | confirmed | existing terminology table, generic overlay loader, and recursive mirror guard |
| complexity.side_effect_risk | moderate | a Pi overlay replaces a shared playbook body, so add one only after dogfood evidence |
| risk.correctness | moderate | rendered Pi terminology must name actual explore, spawn, and continuation mechanisms |
| risk.fit | moderate | the selected display strings must remain grammar-compatible across every variable use site |
| risk.test | moderate | Go coverage exists; Pi dogfood is needed before an overlay is justified |
| risk.security_or_contract | moderate | host-rendered playbook wording is an externally consumed workflow contract |

## Phases

### Phase 1: Pi terminology entry

Add a `"pi"` entry to `playbookTerminologyTable` with `ExploreAgent` =
``the `explore` researcher``, `SpawnIdiom` = `ws-agent-spawn`, and
`ContinueIdiom` = `ws-agent-send to the same alias or agent id`. Verify that
a `harness=pi` render of the
delegation-flow playbooks (`ticket-worker`, `lead-run`, ...) substitutes the Pi
idioms instead of the host-neutral fallback, and that `reservedToolVarNames`
coverage still holds.

### Result (93b4591a) - 2026-09-12

- Cherry-picked the independently reviewed develop implementation (`ab718fa5`) as
  `93b4591a`: the shared terminology table now supplies the exact Pi explore,
  spawn, and continuation idioms while retaining the host-neutral fallback.
- `TestPlaybookPrintPiHarnessUsesPiTerminology`,
  `TestPlaybookPrintPiDelegationFlowPlaybooks`, overlay-selection coverage, and
  terminology-table coverage pass; the full Go suite and MCP smoke test pass.

### Phase 2: Conditional .pi.md overlay

Depends on Phase 1 and the settled shared `rsrc/` supplied by the completed sync
child. Dogfood `lead-discuss`, `lead-run`, and `ticket-worker` with the Pi
terminology entry. If no concrete shared-base delta remains, add no overlay. Only
for a reproduced delta that terminology vars cannot express, author the one
needed `.pi.md` overlay under `agents-plugin/rsrc/`; mirror it per `260906`.
Verify any overlay is selected only for `harness=pi`, the `260906` mirror guard
stays green, and no overlay is authored in the mirror.

### Result (93b4591a) - 2026-09-12

- Source renders of `lead-discuss`, `lead-run`, and `ticket-worker` need only
  terminology substitution; no concrete shared-base delta justified a `.pi.md`
  overlay.
- The Pi runtime contract and `rsrc/` mirror are byte-identical to their
  canonical `agents-plugin/` counterparts; no `.pi.md` exists in either tree.
  `npm test` passes (1647 pass, 1 expected skip), including the 260906 mirror
  checks.


## Resolution (2026-09-12)

Implemented on develop and cherry-picked to the Pi track as 93b4591a. Pi terminology renders through the shared table; no .pi.md overlay was justified, and the canonical runtime and rsrc mirrors remain synchronized.
