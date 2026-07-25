---
title: ws dashboard PTY-agent pivot — narrow decorative layer over a vendor agent CLI in a PTY
related:
  260624-feat-ws-dashboard-managed-cli-terminal: pre-written PTY-agent substrate design; effectively the substrate for this pivot
  260622-research-ws-dashboard-ferrule-session-binding: settled ferrule/bootstrap spawn-injection design reused for hook injection
  260620-feat-ws-dashboard-agent-client-activity-sources: suspended structured provider-adapter track this pivot turns away from
  260725-refactor-dashboard-agent-gui-physical-module-isolation: Tier 2 wire-out of the suspended agent-GUI/adapter surface
  260711-idea-dashboard-agent-facing-mcp-control-surface: dashboard-as-MCP-server direction this pivot deliberately does NOT absorb; also the source of the verified claude-CLI hook grounding
  260725-feat-dashboard-nav-row-two-line-open-state: two-line nav row whose deferred agent-counter slot this pivot fills
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

### 1. Spawn-time launch-context injection

Reuse the ferrule/bootstrap injection design:
`260622-research-ws-dashboard-ferrule-session-binding` (settled) + `260624`
Phase 3. The daemon calls `ws.ferrule(root)`, injects a rendered launch context
at the spawn argv/env seam, and stores `wsSessionKey` daemon-private.

Today env-at-spawn is only `TERM` (`terminal_helper_process.rs` L401-406).

INVARIANT to respect: the identity-privacy three-class model from spec
`#260521-ws-dashboard-activity-console-read-model` — `activityId`
(browser-facing) vs `providerSessionId` / `wsSessionKey` (daemon-private). The
injected credential must never become browser route / command / pane identity.

OWNER CLARIFICATION (2026-07-25) on what "daemon-private" means here:
`wsSessionKey` is NOT a security credential. It is private because a key that
leaks into a subagent's context derails the workflow — the subagent starts
acting with the lead's session authority — not because it grants privileged
access to an attacker. The three-class model above still holds, but the
injection mechanism does NOT need to defend against local `ps` inspection.
argv/env is acceptable; a 0600 file is not required for the key's sake.

DECIDED (2026-07-25): MCP injection is OUT OF SCOPE for this pivot. The owner
framed it as binary — either MCP is required for the notification feature, in
which case it must be built as a real extensible dashboard MCP framework
rather than a one-off; or it is not required, in which case the pivot does not
touch MCP at all. It is not required: a vendor notification hook is a command
line in a vendor settings file that posts to an HTTP endpoint, with the
endpoint and token carried in that same injected file. No MCP server, no tool
surface, no protocol work. The dashboard-as-MCP-server direction stays where it
already lives — `260711-idea-dashboard-agent-facing-mcp-control-surface`.

CONFIG MATERIALIZATION: hook config is per-vendor and does not fit argv/env
alone (`claude` takes `--settings <file>`; `codex` takes config-file / `-c`
overrides), so the profile must WRITE a config file at spawn — argv/env
passthrough is necessary but not sufficient as the enabling refactor.
Ownership is deliberately naive per owner direction: the DAEMON owns the file
and reclaims it by GC (sweep files with no live terminal record), not by
precise delete-on-exit. That is the correct trade because the detached helper
outlives the daemon by design (`260723`), so exit-coupled cleanup would require
the helper to know about vendor config — which the no-PTY-reinvention
invariant forbids.

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
- (b) SUPERSEDED 2026-07-25. This originally read: "a PTY-agent has no
  structured turn events, so running vs awaiting-interaction must be derived
  from output-idle timing." That was wrong — feature 1 injects hooks, and the
  hook IS the structured turn event. See `## Notification Path` below, which
  settles the source, transport, and presentation as one mechanism rather than
  two independent features. Output-idle timing survives only as a last-resort
  fallback for vendors with no hook mechanism.

## Spawn Entry Point (owner directive, 2026-07-25)

The PTY agent is spawned from the **top-right toolbar button slot** — it takes
over the role the removed "Open new agent tab" button held. That slot is
currently empty: the Tier 1 suspension (`c3f5b42b`) omits the button entirely
when `AGENT_GUI_SUSPENDED`, so reclaiming it creates no conflict with the
suspension and needs no flag change.

LOAD-BEARING CONSTRAINT: the new spawn path must NOT reuse
`registerNewAgentChatPane`. That primitive is one of the three suspension
guard depths (toolbar render, `a n` hotkey registration, and the primitive
itself), and it early-returns without state change while the flag is true.
Routing a new spawn through it would either no-op silently or, if the guard
were relaxed to let it through, re-open the suspended agent-GUI surface as a
side effect. The spawn must route through the terminal-create path instead —
which is also what the no-PTY-reinvention invariant above requires.

This entry point FORCES open question 1 below rather than merely touching it:
a single toolbar action must produce a pane of one definite `SurfaceKind`, so
the kind/profile-versus-wrapper choice has to be settled before the button can
be wired. Note also that `SurfaceKind` already has an `"agent"` variant, but it
is occupied — it denotes the daemon-discovered singleton main-instance
projection (`workbench/editorGroups.ts`, handled at `App.tsx:5909`/`5931`,
surfaced in the nav as "N pinned main surface(s)"), not a spawnable PTY. It is
not a free slot to claim without deciding what happens to that projection.

## Notification Path (owner discussion, 2026-07-25)

Features 1 and 3 above are NOT independent tracks — the injected hook IS the
attention source. This section settles the path end to end.

### Signal source: vendor hooks (settled)

`260620` Phase 4's live spike verified `PreToolUse` hooks fire via `--settings`
against `claude` 2.1.207 in headless stream-json mode. OWNER RULING
(2026-07-25): interactive hook firing is ASSUMED, not re-verified — vendor
documentation treats interactive as the primary hook use case, so a hook
verified under stream-json is taken to fire in a PTY session. No spike gates
design on this.

The turn-boundary hooks (vendor names vary; the `Stop` / `Notification` class)
are the running -> awaiting-interaction signal. Codex's external `notify`
program is the likely equivalent and still needs confirming per vendor profile.

### Signal transport: daemon HTTP endpoint (settled)

DECIDED: a narrow HTTP endpoint on the daemon, NOT an extension of the helper
IPC protocol.

Rejected — extending the helper IPC surface: the socket is already per-terminal
so filesystem permissions would have supplied authorization for free, but the
owner rejected it on operability. Helpers are detached and long-lived by
design, so changing their IPC protocol forces killing and restarting every live
helper on each dev iteration — exactly the churn that makes dogfooding painful.
Daemon restarts are cheap, helper restarts are not, and that asymmetry decides
it. Notification is cosmetic and does not justify touching the load-bearing
helper protocol.

AUTHORIZATION: the endpoint carries a per-terminal token minted at spawn and
written into the same injected vendor config file, so an arbitrary local
process cannot post attention events for terminals it did not spawn. This is
integrity scoping, not secrecy — consistent with the `wsSessionKey` reading in
feature 1. Today the daemon's only auth is a single owner bearer token plus a
link passphrase (`auth.rs`); handing that to every spawned hook would grant the
agent every dashboard route, so the scoped token is not optional.

CONSEQUENCE to respect: the token and callback URL must live in the PERSISTED
terminal registry, not only in daemon memory. `boot_reconcile` re-adopts
helpers across a daemon restart, so an in-memory-only token would leave a live
agent posting with a token the restarted daemon no longer recognizes. Port
drift has the same shape and is accepted as a known staleness mode — the daemon
is normally started on a fixed port.

Browser delivery reuses what exists: the `work_root_activity_events` SSE route
plus `workRootActivity.ts`'s `attention` flag (L66), live/attention priority
ordering, and the browser-local ack watermark
(`initializeActivityDirtyItems`).

### Presentation

- Tab label (feature 3): net-new indicator at
  `dockviewLayout.tsx::DockviewWorkbenchTab` (L343-401), which renders only
  icon + title + close today.
- Left nav row: NOT a layout change. Owner direction is a Windows-11-style
  orange flash — background / overlay tint pulsing on the row — and/or a
  breakdown inside the second-line counter: working N (spinner) / ready M
  (blinking orange bell glyph).
  IMPLEMENTATION NOTE: `resourceRowTone` already owns both `background` and
  `border-left-color` on `.resource-row*` (styles.css 2746-2757; `-error` also
  sets `background`). A flash must therefore be an independent overlay layer
  (e.g. a pseudo-element) rather than an animation on `background`, or it will
  fight the tone classes.
  SEQUENCING: the agent counter is precisely the slot
  `260725-feat-dashboard-nav-row-two-line-open-state` DEFERRED pending this
  pivot. That deferral resolves here — the counter arrives with the pivot and
  carries the working/ready split.
- Browser: `document.title` flashing + favicon badge as the zero-permission
  default (works over plain-http LAN), with the `Notification` API as an
  explicit Settings opt-in.

### Browser notification constraints

- Secure context required. `localhost` qualifies, so local dogfooding works,
  but the dashboard's linked-server story means plain-http LAN access has no
  `Notification` API at all. Browser notification is inherently a
  localhost-or-TLS feature; the title/favicon fallback is what covers the rest.
- Permission must be requested from a user gesture, so the trigger is a
  Settings toggle. Natural host is the same section registry
  (`settingsSections.tsx`, spec `#260722-ws-dashboard-settings-panel`) already
  chosen for the model picker.
- PWA shell already exists: `frontend/public/manifest.webmanifest`
  (`display: standalone`), `frontend/public/sw.js`, and a
  `serviceWorker.register('/sw.js')` call in `main.tsx`. But `sw.js` is an
  11-line stub with only install/activate/fetch handlers and NO
  push/notification handling — it exists to satisfy installability, nothing
  more.
- OUT OF SCOPE, explicitly: Web Push / VAPID / a push service. Delivery while
  the tab is closed requires unconditional HTTPS plus a push backend, which is
  a different project. Stated here so nobody drifts into it.

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

- How THIN the additive layer is. **Now forced, not optional** — see
  `## Spawn Entry Point`: a single toolbar spawn action must yield one
  definite `SurfaceKind`, so this must be settled before the entry point can
  be wired. The "no PTY wheel reinvention" invariant
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
- RESOLVED 2026-07-25 — was "deriving running / awaiting-interaction from PTY
  output-idle timing without structured turn events". Vendor hooks supply the
  signal; see `## Notification Path`. Residuals: Codex's hook equivalent
  (`notify`) is unconfirmed, and vendors with no hook mechanism have no
  fallback pinned — terminal BEL / OSC 9 is the candidate (xterm.js exposes
  `onBell`; no handler exists in the codebase today) with output-idle timing as
  the last resort.
- Attention aggregation and acknowledgement semantics: does a server row
  aggregate attention from its work roots, and does acknowledging a tab clear
  the nav badge? The ack-watermark precedent exists
  (`initializeActivityDirtyItems`) but the propagation rule is unpinned.
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
