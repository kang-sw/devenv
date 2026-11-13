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
  `SpawnIdiom`), not `pi.spawn`.
- `ticket-worker-elevated` tells the worker to call `config.resolve_agent`,
  which a delegate key refuses (`roleAllowsTool`); a Pi worker picks its tier
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

## Phases

### Phase 1: Correct the Pi spawn idiom and the delegate resolve_agent call

- Point the `pi` SpawnIdiom at `pi.spawn` (`playbook_tools.go:42`).
- Rework the `config.resolve_agent` step in `ticket-worker-elevated` (and any
  other playbook a delegate renders) so a delegate-key worker does not call
  it; use the `pi.spawn` tier argument for Pi.
- Mirror the changes into `agents-plugin-wsflow/` per the mirroring manual.
- Verify: playbook render tests and the wsflow runtime-contract and
  skill-shim drift tests pass.
