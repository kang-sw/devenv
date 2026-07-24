---
title: ws dashboard PTY-agent pivot — narrow decorative layer over a vendor agent CLI in a PTY
related:
  260624-feat-ws-dashboard-managed-cli-terminal: pre-written PTY-agent substrate design; effectively the substrate for this pivot
  260622-research-ws-dashboard-ferrule-session-binding: settled ferrule/bootstrap spawn-injection design reused for hook injection
  260620-feat-ws-dashboard-agent-client-activity-sources: suspended structured provider-adapter track this pivot turns away from
  260725-refactor-dashboard-agent-gui-physical-module-isolation: Tier 2 wire-out of the suspended agent-GUI/adapter surface
  260514-research-ws-web-dashboard-direction: prior steer toward higher-level agent-pane abstraction that this pivot reverses
  260605-research-ws-native-subagent-pivot: broader native-subagent direction anchor
  260605-epic-ws-playbook-factory-pivot: epic coordinating the native-subagent pivot
related-mental-model:
  - developer-environment-tools
  - named-agent-runtime
  - plugin-runtime
---

# ws dashboard PTY-agent pivot — narrow decorative layer over a vendor agent CLI in a PTY

## Status

Research / direction capture. This ticket records the DIRECTION and grounding
for a future design; it is NOT an implementation plan. Owner-directed pivot
captured 2026-07-25 from a delegated history exploration. All facts below are
already verified against source and prior tickets/specs.

## Direction (owner directive, 2026-07-25)

Pivot the dashboard's agent "resource" AWAY from the structured
app-server/CLI provider-adapter chat GUI and BACK to a PTY-based agent: an
agent CLI running in a terminal/PTY, reusing the existing terminal substrate.

Rationale: implementing the full structured agent-GUI spec is too heavy for the
team to own. Scope is deliberately NARROW — the dashboard adds a thin
decorative/convenience layer over a vendor agent CLI running in a PTY, not a
full chat GUI.

This pairs with the agent-GUI suspension already landed:

- Tier 1 flag `AGENT_GUI_SUSPENDED`.
- Tier 2 wire-out ticket
  `260725-refactor-dashboard-agent-gui-physical-module-isolation`.

## Design invariant (load-bearing): no PTY wheel reinvention

THE most important design decision for this pivot: do NOT reinvent a different
PTY wheel. Substantial terminal-usability work already exists in this codebase.
The PTY-agent MUST be an ADDITIVE layer that sits ON TOP OF the existing
terminal infrastructure — reusing it wholesale — plus a few extra features. It
must NOT introduce a parallel or second PTY subsystem, a forked helper process,
or a separate terminal implementation.

Reused wholesale:

- the terminal registry + detached per-terminal helper process
  (`ws-dashboard terminal-helper`),
- NDJSON IPC (Unix socket / Windows named pipe),
- the output ring + cursoring,
- the reconcile/reap lifecycle,
- the frontend xterm pane with owner-auth WebSocket + HTTP-poll fallback
  transport,
- Dockview tabs.

The agent is simply a terminal whose spawned executable is an agent CLI instead
of a shell — via argv/env passthrough at the
`terminal_helper_process.rs::spawn_shell` seam (the enabling refactor already
noted below) — plus the thin decoration/profile layer (hook injection, model
selection, tab-label attention). The plumbing stays single-sourced.

## Recorded Design Reversal (flag prominently for sage/design review)

This pivot REVERSES two recorded steers. That tension must be surfaced, not
hidden.

- `260514-research-ws-web-dashboard-direction` argued agent panes should stay a
  higher-level abstraction where "a PTY is only one possible interface
  type... so future named-agent projections, headless calls, or structured
  agent GUIs do not have to masquerade as terminals," and warned that
  Codex-like TUIs dump conversation state on column resize.
- `260624-feat-ws-dashboard-managed-cli-terminal` (todo/) was originally
  PTY-first but was superseded 2026-07-11 (owner) in favor of the structured
  provider-adapter track (`260620`). This pivot re-reverses that 2026-07-11
  supersession.

The owner has now chosen the narrow PTY path despite these prior steers.
Resolution of the tension is left to design/sage time; this ticket only records
that the reversal is deliberate and known.

## Substrate = re-prioritized 260624

`260624-feat-ws-dashboard-managed-cli-terminal` is effectively the pre-written
PTY-agent substrate design: a managed vendor-CLI-in-PTY terminal. Its Phase 1
already specifies commonizing the PTY I/O substrate:

- spawn from explicit argv/env/cwd,
- output ring + cursoring,
- stdin write,
- resize,
- status,
- close/reap,
- WebSocket forwarding,
- bounded fallback reads,
- vendor differences isolated in thin profiles.

Prerequisite gap (enabling refactor): the spawn seam
`crates/daemon/src/terminal_helper_process.rs::spawn_shell` (L391-407; L399
hardcodes `default_shell()`) and `TerminalHelperArgs` currently carry
rows/cols/cwd but NOT argv/env. argv/env passthrough is the enabling refactor.

Frontend attach seam reused as-is: `terminals.ts` + `terminalPaneBody.tsx`
(xterm) over an owner-auth WebSocket with HTTP poll fallback; Dockview tabs.

## Narrow Decorative Scope — the three entry-point features

The narrow scope is three decorative/convenience entry-point features layered
over the vendor agent CLI. Prior art and gaps for each:

### 1. Spawn-time hook injection

Reuse the ferrule/bootstrap injection design:
`260622-research-ws-dashboard-ferrule-session-binding` (settled) + `260624`
Phase 3. The daemon calls `ws.ferrule(root)`, injects a rendered launch context
at the spawn argv/env seam, and stores `wsSessionKey` daemon-private.

Today env-at-spawn is only `TERM` (`terminal_helper_process.rs` L401-406).

INVARIANT to respect: the identity-privacy three-class model from spec
`#260521-ws-dashboard-activity-console-read-model` — `activityId`
(browser-facing) vs `providerSessionId` / `wsSessionKey` (daemon-private). The
injected credential must never become browser route / command / pane identity.

### 2. Model selection exposure

NET-NEW: no agent model UI exists (both adapters report `model: None` —
`codex_app_server.rs:838`, `claude_cli.rs:839`). Natural host: the Settings
section registry (spec `#260722-ws-dashboard-settings-panel`;
`frontend/src/settingsSections.tsx`).

DESIGN TENSION to flag: spec Daemon Foundation (index.md ~L47-49) says the
dashboard must NOT be the canonical model backend/authority; model aliasing
(light/core/deep → provider) lives in ws runtime
(`260508-feat-harness-aware-model-aliases`). So a picker must delegate to
ws-runtime aliases / vendor profiles, not own a model backend.

### 3. Tab-label attention signaling (intended PRIMARY entry point)

Blink / mark the tab label when the agent is running, and when it finishes a
turn and is awaiting interaction.

Prior art to build on: `frontend/src/workRootActivity.ts` already has an
`attention: boolean` flag (L66), live/attention priority ordering, a
browser-local dirty-item watermark (`initializeActivityDirtyItems`, cleared on
acknowledge), and an `"attention"` badge tone. Spec
`#260521-ws-dashboard-activity-console-ui-shell` already describes a
"short-lived green breathing indicator... until the user selects or otherwise
acknowledges them" + browser-local ack watermark — the exact running/awaiting
semantics wanted.

GAPS:

- (a) That indicator is workRoot-level top-bar today, NOT per-tab. The
  tab-label seam
  `frontend/src/workbench/dockviewLayout.tsx::DockviewWorkbenchTab` (L343-401)
  renders only icon + title + close; NO attention/blink indicator exists —
  net-new.
- (b) A PTY-agent has no structured turn events, so "running vs
  awaiting-interaction" must be derived from output-idle timing (net-new
  heuristic; no terminal bell / activity-dot handling exists today).

## Remove vs Keep

Wire-out of the adapter surface is tracked in
`260725-refactor-dashboard-agent-gui-physical-module-isolation` — reference it;
do not duplicate its full seam list here.

KEEP / reuse:

- the whole terminal/PTY substrate (`terminal*.rs`, `terminals.ts`,
  `terminalPaneBody.tsx`, WS + HTTP transport, Dockview tabs),
- `workRootActivity.ts` shared Activity types,
- the SQLite named-agent Activity source
  (`#260525-ws-dashboard-sqlite-agent-activity-source`).

NOTE: spec `#260516-ws-web-dashboard-terminal-pane` states "The terminal pane
is a shell terminal substrate only; it does not hardcode Codex, Claude, or
other agent presets" (index.md ~L1964-1965). Per the "no PTY wheel
reinvention" invariant above, the additive layer must not FORK the terminal
pane's substrate; it may add a sibling profile/kind over the same
single-sourced plumbing. The remaining choice — profile/kind flag vs thin
wrapper component — is called out below as an open design choice.

## Open Questions (for sage/design; do NOT resolve here)

- How THIN the additive layer is. The "no PTY wheel reinvention" invariant
  above SETTLES the substrate question — same substrate, no new PTY plumbing.
  What remains open is only the shape of the thin layer: whether the agent is
  expressed as a `kind`/profile flag on the same terminal pane vs a thin
  wrapper component over the identical infra. The
  shell-substrate-only spec line (`#260516-ws-web-dashboard-terminal-pane`)
  tension remains noted, but framed as: the additive layer must not FORK the
  terminal pane's substrate — it may add a sibling profile/kind over the same
  plumbing, but the plumbing stays single-sourced.
- Model picker vs the "dashboard is not model authority" invariant — how to
  delegate to ws-runtime aliases.
- Deriving running / awaiting-interaction from PTY output-idle timing without
  structured turn events.
- Whether/how 260624's 2026-07-11 supersession is formally reversed (edit
  260624 vs supersede-by-this-ticket).
- Scope boundary: how much of 260624 Phase 1 argv/env commonization is in-scope
  for the narrow pivot.

## Relations

Tickets: `260624-feat-ws-dashboard-managed-cli-terminal`,
`260622-research-ws-dashboard-ferrule-session-binding`,
`260620-feat-ws-dashboard-agent-client-activity-sources` (suspended),
`260725-refactor-dashboard-agent-gui-physical-module-isolation`,
`260514-research-ws-web-dashboard-direction`,
`260605-research-ws-native-subagent-pivot` / epic
`260605-epic-ws-playbook-factory-pivot`.

Spec anchors:
`#260620-ws-dashboard-agent-client-provider-contract` (suspended),
`#260521-ws-dashboard-activity-console-ui-shell`,
`#260521-ws-dashboard-activity-console-read-model`,
`#260722-ws-dashboard-settings-panel`,
`#260516-ws-web-dashboard-terminal-pane`,
`#260516-ws-web-dashboard-terminal-registry-pty-spawn`,
`#260525-ws-dashboard-sqlite-agent-activity-source`.
