---
title: "Render shared ws playbooks Pi-correct: pi terminology entry and selected .pi.md overlays"
parent: 260911-epic-ws-pi-refound-resync-harness-peer
related:
  260905-feat-ws-pi-harness-config-layer: landed the harness-peer infra this ticket fills the last data/text gaps of
  260906-bug-ws-pi-rsrc-mirror-drift: overlays are authored upstream under agents-plugin/rsrc and reach Pi via its mirror, never authored in the mirror
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
  `ContinueIdiom`, carrying Pi's `ws-agent-spawn` / Pi explore / continue idioms)
  is added to the Go table. This is data consistent with the existing
  `codex` / `claude` entries, not host-specific logic — compatible with AGENTS.md
  clause (1) and the harness-peer clause.
- **Overlay only where the neutral render is actually wrong.** Author a `.pi.md`
  overlay under `agents-plugin/rsrc/<name>/<name>.pi.md` for a delegation-flow
  playbook (candidates: `lead-discuss`, `lead-delegate`, `lead-run`,
  `ticket-worker`) only where the Pi delta exceeds the terminology vars; do not
  pre-author an overlay per playbook. Overlays are authored upstream and mirrored
  into `agents-plugin-pi/rsrc` per `260906`.
- **Pure-Pi-mechanism text is out of scope.** execute-worker / orchestrator
  guides describing `ws-execute` / `ws-worker-exec` / approval and
  `ws-report-to-lead` have no shared base and stay adapter-owned
  `systemPromptPath` guides (epic Cross-Child Decision 4).
- Noted, not necessarily bundled: `detectHarnessFromRaw` has no `"pi"` substring
  fallback (non-initialize paths rely on the sticky initialize-time
  `clientInfo.name=="ws-pi-bridge"` value); pi model tier defaults are not seeded
  in `applyDefaultModelAliases` (pure data via `config.tune` /
  `defaultModelAliases`). Fold in only if dogfood shows they bite.

## Constraints

- **Develop-authored (AGENTS.md clause 1).** The terminology entry and the
  `.pi.md` overlays are authored on `develop` and reach the Pi track by
  cherry-pick plus the `260906` mirror. Not authored on the Pi track.
- **Ready gate:** do not promote to `ready/` until `epic/refound` has merged to
  `develop`; the overlay phase also depends on `260911-chore-ws-pi-track-sync-to-refound`
  giving a settled shared `rsrc/` to overlay against.

## Phases

### Phase 1: Pi terminology entry

Add a `"pi"` entry to `playbookTerminologyTable` with Pi's `ExploreAgent` /
`SpawnIdiom` / `ContinueIdiom`. Verify that a `harness=pi` render of the
delegation-flow playbooks (`ticket-worker`, `lead-run`, ...) substitutes the Pi
idioms instead of the host-neutral fallback, and that `reservedToolVarNames`
coverage still holds.

### Phase 2: Selected .pi.md overlays

Depends on Phase 1 and on a settled shared `rsrc/` (the sync child). For each
delegation-flow playbook whose Pi delta exceeds the terminology vars, author a
`.pi.md` overlay under `agents-plugin/rsrc/`; mirror it per `260906`. Verify the
overlay is selected only for `harness=pi`, the `260906` mirror guard stays green,
and no overlay is authored in the mirror.
