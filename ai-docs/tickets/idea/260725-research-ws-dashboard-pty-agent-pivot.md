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

ENVIRONMENT SCRUB — NET-NEW REQUIREMENT (found during 2026-07-25 verification;
not previously recorded anywhere). `spawn_shell`
(`terminal_helper_process.rs:391-408`) sets only `cwd` and `TERM` on the
`CommandBuilder` and never calls `env_clear`, so `portable-pty` hands the
child the daemon's environment WHOLESALE. Today that is harmless because the
child is a shell. It stops being harmless the moment the child is an agent CLI:
if the daemon was itself launched from inside an agent harness, that harness's
env markers propagate down into every spawned agent.

This is not hypothetical. During the hook verification above, a `claude`
started from inside a Claude Code session printed:

```text
⚠ Transcript saving is off — inherited CLAUDE_CODE_CHILD_SESSION marker
```

Transcript saving off also means `--resume` has nothing to resume. The
dogfooding path — owner starts the daemon from a terminal inside their agent
session — is exactly the path that triggers it. The argv/env passthrough
refactor must therefore include a scrub/allowlist step, not just a
pass-through, and the vendor profile owns the list of markers to strip.

The scrub must be applied at BOTH hops, not just the visible one. The daemon
spawns the helper with a plain `std::process::Command` that sets no env
(`terminal.rs:817-844`; `terminal_platform.rs` only adds `setsid` + double-fork),
so the helper already inherits the daemon's environment before `spawn_shell`
runs. Scrubbing only inside `spawn_shell` leaves the helper's own env dirty.
Note also that the existing line "Today env-at-spawn is only `TERM`" is true
only about EXPLICIT env — it reads as if the environment were otherwise clean,
and it is not.

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

Two things the GC decision does NOT get for free:

- There is no existing daemon-side directory sweep to hang it on.
  `delete_registry_entry` (`terminal_registry_file.rs:68-71`) is per-entry and
  is only called from boot reconcile / verified kill. A sweep is a net-new
  background task, not a reuse.
- "No live terminal record" keys off TERMINAL liveness, not AGENT liveness.
  The agent CLI is a grandchild the daemon has no handle on (see the open
  question below), so a config file survives as long as its terminal does even
  after the agent has exited.

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

### Signal source: vendor hooks (VERIFIED, not assumed)

The owner ruled on 2026-07-25 that interactive hook firing could be ASSUMED
from `260620` Phase 4's headless stream-json verification. That assumption was
then discharged empirically rather than carried — it is now a measured fact:

- `--settings <file>` DOES accept a `hooks` block. A doc-derived claim that
  hooks can only come from `.claude/settings.json` was checked and is wrong;
  `claude --help` lists `--settings <file-or-json>`, and the repo's own
  `260620` Phase 4 spike had already injected a `PreToolUse` hook this way.
- A `Stop` hook supplied via `--settings` FIRES IN A REAL INTERACTIVE PTY
  SESSION. Verified 2026-07-25 on macOS by driving `claude` under
  `pty.fork()`, sending a prompt, and observing the hook artifact written
  BEFORE the session was terminated (so it cannot be confused with a
  session-teardown write).

`Stop` is therefore the turn-boundary signal, and it needs no fallback tier for
the Claude profile.

STILL UNVERIFIED (do not present as settled):

- The `Notification` event and its `idle_prompt` matcher — reported by a docs
  lookup as "Claude is done and waiting for your next prompt", which would be
  a more precise awaiting-interaction signal than `Stop`. It did NOT fire in
  the PTY run above, but that run ended at the 60s mark and never idled long
  enough to be a real test. Worth a second spike only if `Stop` proves too
  coarse.
- Codex. Confirmed that Codex HAS a hook subsystem — `codex --help` exposes
  `--dangerously-bypass-hook-trust` ("Run enabled hooks without requiring
  persisted hook trust for this invocation"). The earlier guess that Codex
  offers a `notify` external program was NOT confirmed; no `notify` key
  appears in the binary's strings or in a live `~/.codex/config.toml`.
  IMPLICATION worth flagging early: Codex gates hooks behind a PERSISTED TRUST
  record. A daemon that injects a hook config at spawn may therefore need an
  explicit trust step, which the Claude profile does not need — the per-vendor
  profile is less symmetric than this ticket previously assumed.

### Signal transport: daemon HTTP endpoint (settled)

DECIDED: a narrow HTTP endpoint on the daemon, NOT an extension of the helper
IPC protocol.

Rejected — extending the helper IPC surface. The owner rejected it on
operability: helpers are detached and long-lived by design, so changing their
IPC protocol forces killing and restarting every live helper on each dev
iteration — exactly the churn that makes dogfooding painful. Daemon restarts
are cheap, helper restarts are not, and that asymmetry decides it. A cosmetic
feature does not justify touching the load-bearing helper protocol.

CORRECTION (design review, 2026-07-25): an earlier draft of this ticket also
credited the IPC option with getting authorization "for free" from filesystem
permissions on the per-terminal socket. That was false and is struck — nothing
chmods the socket, so its mode is umask-governed, and the handshake carries no
secret (`HelperToDaemonMessage::Handshake { pid, start_time }`,
`terminal_helper_protocol.rs:43-46`). Only the registry JSON gets a deliberate
`0600` (`terminal_registry_file.rs:48-52`); the asymmetry is simply an absence
on the socket side. Filesystem permissions therefore buy CROSS-USER isolation
only — any same-user process can already connect and drive any terminal today,
which is precisely the threat the scoped token exists to address. The
operability argument is the whole reason for the rejection; the security
argument never existed.

AUTHORIZATION: the endpoint carries a per-terminal token minted at spawn and
written into the injected vendor config file, so an arbitrary local process
cannot post attention events for terminals it did not spawn. This is integrity
scoping, not secrecy — consistent with the `wsSessionKey` reading in feature 1.
Today the daemon has no per-resource auth at all: `auth.rs:13-64` defines four
credential types (pairing token, link passphrase, owner session cookie, bearer
token) and none of them are resource-scoped, and `router.rs:96-104` applies a
single `require_owner_auth` layer over the ENTIRE protected router — only
`/pair` and `/api/dashboard/link-auth` sit outside it. `require_owner_auth`
(`:505-525`) branches on websocket-upgrade versus browser entrypoint and never
on path. So handing an agent any existing credential grants it every dashboard
route. The scoped token is not optional.

(Incidental, relevant to any later push work: `/sw.js` and
`/manifest.webmanifest` are served from inside the authenticated router.)

CAVEAT: `--no-auth` disables the owner-auth middleware, which is precisely the
dogfooding configuration. The scoped token is therefore unenforced in exactly
the mode it will first be exercised in. Acceptable for a cosmetic feature, but
it means `--no-auth` runs cannot be used as evidence that token scoping works.

TOKEN STORE — CORRECTED (design review, 2026-07-25). An earlier draft said the
token must live in "the PERSISTED terminal registry". That is the WRONG store
and would have leaked the token:

- The registry is HELPER-owned, not daemon-owned — create-on-spawn,
  delete-on-exit, with the daemon only pruning entries it has confirmed dead
  (`terminal_registry_file.rs:1-6`), and the helper writes its entry before the
  IPC listener even binds (`terminal_helper_process.rs:174-193`).
- Everything the daemon tells the helper travels as clap `--long` argv
  (`cli.rs:31-49`), which is world-readable via `ps`.

So a token routed into the registry would have to pass through helper argv
first. Correct shape instead: the token lives in DAEMON-owned persisted state
keyed by `terminal_id`, and reaches the agent only inside the `0600` vendor
config file the daemon itself writes. Helper argv carries the config file PATH
(not secret) and nothing else. The durability requirement that motivated the
original claim still holds — `boot_reconcile` re-adopts helpers across a daemon
restart, so an in-memory-only token would leave a live agent posting with a
token the restarted daemon no longer recognizes — it just has to be satisfied
in the daemon's own store. Port drift has the same shape and is accepted as a
known staleness mode; the daemon is normally started on a fixed port.

Where the code change actually lands: `boot_reconcile` (`terminal.rs:196-220`)
must REPOPULATE the in-memory token -> terminal lookup from the daemon's store
during boot, before serving. Today's `TerminalSession` construction path has no
slot for auth material at all, so "persist the token" is only half the change.

REGISTRY SCHEMA CONSTRAINT (applies to the argv/env passthrough refactor too,
not just to tokens). `TerminalRegistryEntry` has no `version` field and no
`#[serde(default)]` on anything, and `scan_registry_dir`
(`terminal_registry_file.rs:77`) warns-and-SKIPS an entry it cannot
deserialize. Because helpers are detached and outlive daemon upgrades by
design, adding a non-`Option` field makes every still-running older helper's
entry invisible to the upgraded daemon — which means boot reconcile never sees
it, the drop path never runs, and the helper plus its socket are orphaned
permanently. Any new registry field must be `Option<T>` + `#[serde(default)]`.
This hazard is not specific to this pivot and is reported separately.

### Browser delivery: OPEN, not reuse (design review, 2026-07-25)

An earlier draft asserted that browser delivery "reuses what exists" via the
`work_root_activity_events` SSE route. That was checked against source and is
WRONG. The activity SSE path cannot carry a terminal-originated attention
event as built:

- The snapshot route `work_root_activity` (`work_root_activity.rs:187`) DOES
  merge live in-memory Codex/Claude sessions into the unified feed
  (L202-221) — which is presumably where the reuse intuition came from.
- But the SSE route `work_root_activity_events` (L274) goes through
  `watch_snapshot` (L166) -> `watch_snapshot_blocking` (L546) ->
  `project_blocking`, which reads the on-disk wsstate projection ONLY, on a
  200 ms re-poll (L378). The live-session merge is not on the watch path.
  There is no in-memory injection point.
- The frontend subscription is additionally scoped to the Activity Console for
  the SELECTED root — which defeats the purpose, since a nav badge has to
  light up for roots the user is NOT currently looking at.

Nor is the browser-side `attention` flag reusable as-is, which an earlier draft
also claimed. `attention` is currently an ERROR signal, not an
awaiting-interaction signal — `work_root_activity.rs:1095-1104` sets it from
`!diagnostics.is_empty()`, a status of `blocked`/`failed`/`unavailable`, or a
`current_call` error, and `activity_item_rank` (L1192-1200) gives it rank 1,
just below `live`. Overloading it with a benign "turn finished, awaiting your
input" would make every waiting agent render as a failure in the Activity
Console and would silently reorder the ribbon. A ready-for-input state needs
its own field or its own vocabulary.

What IS genuinely reusable: the browser-local ack watermark shape
(`initializeActivityDirtyItems`, `workRootActivity.ts:568`) as a PATTERN, and
`ActivityItem.kind`, which is an open string vocabulary by contract
(`crates/core/src/activity.rs:92-100`) — so a terminal-originated kind is
additive at the type level even though the transport is not.

The transport is therefore an OPEN question with three candidates, none picked:
adding an in-memory injection point to the activity projector's watch path; a
small dedicated daemon-owned attention event stream independent of the activity
projection; or piggybacking the existing per-terminal WebSocket (rejected on
first look — it is per-pane, so it cannot notify about a pane that is not
open).

### Presentation

- Tab label (feature 3): net-new indicator at
  `dockviewLayout.tsx::DockviewWorkbenchTab` (L343-401), which renders only
  icon + title + close today.
- Left nav row: NOT a layout change. Owner direction is a Windows-11-style
  orange flash — background / overlay tint pulsing on the row — and/or a
  breakdown inside the second-line counter: working N (spinner) / ready M
  (blinking orange bell glyph).
  IMPLEMENTATION NOTE (corrected after design review — the original overstated
  which rules own `background`). In the EFFECTIVE CSS block (styles.css 2727+;
  the earlier 1030-1110 block is overridden), `background` on `.resource-row`
  is written by three separate rules: the base rule (2729,
  `--ws-color-surface-context`), `:hover` (2743, `--ws-color-panel-hover`), and
  the `-error` tone (2757, `--ws-color-notice-error`). The other tone classes
  (`-ready`, `-muted`) set only `border-left-color`. So `resourceRowTone` does
  NOT own `background` in general.
  The conclusion is unchanged and in fact stronger. The hard case is not the
  tone classes at all but `.resource-row-selected` (styles.css 1081-1095 —
  defined ONCE, so not overridden despite living in the early block), which
  paints a two-layer gradient plus a `box-shadow` and also sets
  `border-left-color`. An attention flash animated on `background` would have
  to fight the base rule, `:hover`, `-error`, AND that gradient, so it must be
  an independent overlay layer (e.g. a pseudo-element).
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
  signal, now verified in an interactive PTY rather than assumed; see
  `## Notification Path`. Residuals: Codex hooks exist but are gated behind a
  persisted trust record whose config shape is unknown, and vendors with no
  hook mechanism have no fallback pinned — terminal BEL / OSC 9 is the
  candidate (xterm.js exposes `onBell`; no handler exists in the codebase
  today) with output-idle timing as the last resort.
- HOW the attention event reaches the browser. Reopened by design review after
  the "reuse the activity SSE" assertion was refuted against source — see
  `### Browser delivery: OPEN, not reuse`. Three candidates listed there, none
  picked. This is the largest remaining unknown in the notification path.
- Whether the daemon needs a handle on the AGENT process specifically. The
  daemon tracks only the helper (pid + start_time,
  `terminal_helper_protocol.rs:43-46`) and the helper tracks only the shell it
  spawned (`SharedState.child`, `terminal_helper_process.rs:80`). The agent CLI
  is a grandchild launched by the shell, invisible to both. Three consequences
  the design has not addressed: attention events can be correlated ONLY by the
  injected token, since there is no process identity to check them against;
  config-file GC keys off terminal liveness, so a file outlives its agent; and
  `TerminalHelperStatus` reports only the shell's state, so "the agent finished
  a turn" can never be inferred from existing helper signals — the hook is the
  only source.
- Whether a ready-for-input state gets its own field or reuses/extends
  `attention`. Reuse is currently blocked because `attention` means "error" —
  see `### Browser delivery`.
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
