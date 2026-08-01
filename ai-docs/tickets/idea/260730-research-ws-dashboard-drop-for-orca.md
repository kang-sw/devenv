---
title: Drop ws-dashboard development in favor of Orca ADE
related:
  260730-research-orca-plugin-ws-workflow-surface: successor concern — the one capability gap this drop leaves open
---

# Drop ws-dashboard development in favor of Orca ADE

## Background

`ws-dashboard/` was built as a personal ws-aware browser control plane:
workspace persistence, worktree management, integrated terminal, document
exploration, git management, plus a ws-specific Activity Console. Partway
through, the owner found Orca ADE (`stablyai/orca`), which already implements
substantially all of the generic half — and, on the owner's own daily use,
implements it better than the target this project was aiming at.

This ticket records the decision to stop ws-dashboard feature development, the
evidence behind it, and what that costs.

## Decision

Stop ws-dashboard feature development. Orca supersedes it. The completed sweep
deleted the dashboard code, documentation domain, and dashboard board; recovery
is through the annotated `archive/ws-dashboard` tag.

The ws/wsflow plugin itself is unaffected and remains the project's core
deliverable. It installs into the Claude Code / Codex CLIs, and Orca runs those
CLIs in ordinary terminals — owner-verified that an Orca terminal behaves no
differently from a normal shell, so ws skills load unchanged inside it.

## Evidence: What Orca Covers

Surveyed 2026-07-30 against `stablyai/orca` at `main`, plus owner daily use.

- Repository facts: MIT, TypeScript/Electron, created 2026-03-17, 33,379 stars,
  2,667 open issues, pushed same day as this survey.
- Feature overlap: terminals, worktree management, workspace persistence,
  document exploration, git management, editor. Owner-verified as a daily
  driver on Windows ("clean and excellent"); macOS use had just started at the
  time of this decision.
- Remote/headless model: `orca serve --port` runs a headless daemon on a Linux
  VPS (`docs/reference/headless-linux-server.md`), advertising a WebSocket
  runtime endpoint with a pairing URL, QR, `webClientUrl`, E2EE keys, and a
  device registry. This is the same ground ws-dashboard's Server Route, linked
  server registry, SSH tunnel reconnect, gateway forwarding, and PWA work
  covered — roughly the largest single block of its spec.
- Harness integration: `src/main/codex/codex-app-server-*` already implements a
  Codex app-server client, the surface ws-dashboard had been researching.
- Agent attention: Orca handles agent status natively and broadly — an
  `agent-status` subsystem spanning `src/shared/agent-status-*.ts`,
  `src/main/agent-hooks/agent-status-pane-index.ts`, and
  `src/renderer/src/lib/agent-status.ts`, plus attention surfacing in
  `src/main/tray/tray-attention-icon.ts`,
  `src/renderer/src/components/sidebar/smart-attention.ts`, and
  `src/renderer/src/components/terminal-pane/terminal-pane-attention-subscriptions.ts`.
  This duplicates the dashboard's turn-state hook and attention-indicator work.

Timing note, to keep the record honest: the first ws-dashboard commit is
2026-05-14 (`92377b44`), when Orca was a two-month-old, then-obscure
repository. Its current shape postdates the start of this project.

## What Does Not Come Back

Programmatic harness session control — compact, steer, goal, fork — is
permanently given up. Orca owns the agent process, and its plugin API exposes
only `terminal.sendText` toward a running agent. No plugin can recover this.
See `260730-research-orca-plugin-ws-workflow-surface` for the plugin capability
survey behind that claim.

## Rejected Alternative

Merging the dashboard development line into `main`. The branch
`goal/ws-dashboard-dev/velvet-arbor-quill` was already merged into
`ws-dashboard-dev`, with later tips above it, but merging that line into `main`
would still be forced integration work for code no longer being developed. It
was instead preserved by the annotated `archive/ws-dashboard` tag. Recover it
with `git fetch --tags && git checkout -b revive archive/ws-dashboard`.

The reusable `(harness, capability)` four-tier matrix (Passthrough / Overlay /
Hack / Unavailable) with fixture-verified Codex app-server findings was
recovered to `ai-docs/ref/agent-harness-capability-tiers.md`. It remains valid
independently of the removed UI.

## Cleanup Scope

Observed on `main`, 2026-07-30. These figures differ substantially from the
dangling worktree's (e.g. 73 live tickets and a 2,804-line spec there), so
`main` is the only correct basis for cleanup.

- `ws-dashboard/` code tree: present.
- `ai-docs/spec/ws-web-dashboard/index.md`: 1,305 lines.
- `ai-docs/mental-model/ws-web-dashboard.md`: single file (the branch has a
  three-file directory instead).
- Live dashboard tickets: 13 across `idea/`, `todo/`, `ready/`, including
  `260729-bug-dashboard-submodule-workroot-empty-projection`, i.e. `main` had
  dashboard activity the day before this decision.
- `ai-docs/_index.md` references the dashboard in the inventory line and
  related sections.

This inventory was resolved by `260730-chore-ws-dashboard-drop-sweep`: the code,
specification, mental model, and 13 dashboard tickets were removed rather than
archived in the live document tree; `_index.md` was reconciled; and the harness
research was recovered as a reference.

## Unverified

Named so a later session does not mistake them for checked facts.

- macOS behavior under multiple concurrent worktrees running goal-pursuit.
- Orca's `webClientUrl` remote/phone access actually exercised end to end;
  confirmed to exist in the documented ready payload, not yet used.
