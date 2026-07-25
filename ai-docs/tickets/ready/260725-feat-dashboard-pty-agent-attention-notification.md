---
title: PTY-agent attention notification — hook-injected turn signal to tab, nav row, and browser
related:
  260725-research-ws-dashboard-pty-agent-pivot: direction anchor; this ticket implements its `## Notification Path` section and inherits its verified facts and corrections
  260624-feat-ws-dashboard-managed-cli-terminal: pre-written PTY-agent substrate design; Phase 1 argv/env commonization overlaps this ticket's Phase 1
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: detached-helper model that makes the daemon the only viable owner of injected config
  260725-feat-dashboard-nav-row-two-line-open-state: owns the two-line nav row whose deferred agent-counter slot Phase 7 fills
  260725-refactor-dashboard-agent-gui-physical-module-isolation: Tier 2 wire-out that owns the agent-GUI surface Phase 2 must not route through, and the module holding the existing settings-JSON builder
  260725-bug-dashboard-terminal-platform-macos-unsupported: blocks native macOS verification of every phase here
  260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers: registry hazard this ticket deliberately avoids by never adding a registry field
  260711-idea-dashboard-agent-facing-mcp-control-surface: dashboard-as-MCP direction explicitly NOT absorbed here
related-mental-model:
  - ws-web-dashboard
  - developer-environment-tools
sage-review-design: completed
sage-review-completeness: completed
---

# PTY-agent attention notification

## Background

The PTY-agent pivot (`260725-research-ws-dashboard-pty-agent-pivot`) names
tab-label attention signalling as its intended PRIMARY entry-point feature: the
dashboard should show when an agent CLI running in a terminal is working and
when it has finished a turn and is waiting for the human.

That research ticket's `## Notification Path` section went through an empirical
and adversarial verification pass on 2026-07-25 which refuted most of its
initial "settled / reuse" claims. This ticket implements what survived. It does
NOT re-derive the analysis; read that section for the evidence.

Facts this ticket builds on, all verified against source or by measurement:

- A `Stop` hook supplied through `claude --settings <file>` FIRES IN A REAL
  INTERACTIVE PTY SESSION (measured 2026-07-25 on macOS via `pty.fork()`, with
  the artifact observed before session teardown). The signal source is settled
  and needs no output-idle heuristic.
- The activity SSE path CANNOT carry this. `work_root_activity_events`
  (`work_root_activity.rs:274`) runs `watch_snapshot` (L166) ->
  `watch_snapshot_blocking` (L546) -> `project_blocking`, which re-reads the
  on-disk wsstate projection every 200 ms (L378) with no in-memory injection
  point; and the frontend subscription is scoped to the SELECTED root's
  Activity Console pane. A nav badge exists precisely to fire for roots the
  user is not looking at.
- The existing `attention` flag is an ERROR signal
  (`work_root_activity.rs:1095-1104`: non-empty diagnostics, a
  `blocked`/`failed`/`unavailable` status, or a call error) and ranks just
  below `live` (`activity_item_rank`, L1192-1200). Reusing it for a benign
  awaiting-input state would render every waiting agent as a failure and
  reorder the ribbon.
- The spawn seam inherits the environment WHOLESALE at both hops. The daemon
  spawns the helper with a `std::process::Command` that sets no env
  (`terminal.rs:817-844`), and `spawn_shell`
  (`terminal_helper_process.rs:391-407`) sets only `cwd` and `TERM` with no
  `env_clear`. A nested `claude` was observed printing
  `Transcript saving is off — inherited CLAUDE_CODE_CHILD_SESSION marker`,
  which also breaks `--resume`.
- Auth is blanket, not scoped: four credential types in `auth.rs:13-64`, none
  resource-scoped, under one `require_owner_auth` layer over the whole
  protected router (`router.rs:96-104`, `:505-525`).

## Decisions

### The signal path is hook -> daemon HTTP -> dedicated stream -> browser

Pinned in the research ticket after owner discussion; restated here because
every phase depends on it.

- **Source**: a vendor turn-boundary hook, injected at spawn. NOT output-idle
  timing, NOT terminal BEL.
- **Agent -> daemon**: a narrow HTTP endpoint on the daemon. NOT an extension
  of the helper IPC protocol — helpers are detached and long-lived, so a
  protocol change forces killing every live helper on each dev iteration.
  Daemon restarts are cheap, helper restarts are not.
- **Daemon -> browser**: a NEW dedicated always-on event stream, SERVER-SCOPED
  with a forwarding sibling like every other terminal route (see Phase 5 — an
  earlier draft said "dashboard scope", which would have dropped linked
  servers). Rejected alternative: adding an in-memory injection point to the
  activity projector's watch path. That would drag this ticket into repairing
  the Activity Console's streaming architecture (live Codex/Claude sessions
  already fail to stream today — they are merged only into the snapshot route
  at `work_root_activity.rs:202-221`), and the consumer shape is wrong anyway:
  nav rows and dockview tabs are global, the activity stream is per-root and
  pane-gated.

### The token never touches the helper or the registry

The callback token lives in DAEMON-owned persisted state keyed by
`terminal_id`, and reaches the agent only inside the `0600` `callback.json`
the daemon writes — never inside the vendor config file, and never in argv.
Helper argv carries file PATHS only.

This is load-bearing in two directions. Everything the daemon passes the helper
is clap `--long` argv (`cli.rs:31-49`), world-readable via `ps`, so a token
routed that way would be forgeable by any local process — defeating the only
thing the token exists for. And it keeps this ticket clear of
`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`:
adding a non-`Option` field to `TerminalRegistryEntry` would make older
still-running helpers' entries unparseable and orphan them permanently, and
this design never adds one.

### Ready-for-input gets its own vocabulary

Because `attention` already means "error", the new state is carried on the new
stream with its own field, not by overloading `attention`. Reusable from the
existing model: the browser-local ack-watermark PATTERN
(`initializeActivityDirtyItems`, `workRootActivity.ts:568`) and
`ActivityItem.kind` being an open string vocabulary by contract
(`crates/core/src/activity.rs:92-100`).

### Concrete mechanics, pinned so an implementer does not have to invent them

- **Turn states.** `working` / `ready` / `idle`. See the next decision for how
  `working` is sourced — it is NOT verified yet and gates the vocabulary.
- **Callback endpoint.** A SINGLE local route,
  `POST /api/dashboard/terminals/{terminal_id}/turn-state`. Body
  `{ "token": "<opaque>", "state": "working|ready|idle" }`. The token is
  checked in the handler against the daemon's store; the route is registered
  outside `require_owner_auth`.
  Deliberately NOT paired with a server-scoped sibling, unlike the other
  terminal routes. An agent always posts to the daemon that spawned it — a
  terminal on a linked server runs under the remote daemon — so the sibling
  would have no caller, and being outside owner auth it would be an
  unauthenticated forwarding route. Server scoping belongs to the STREAM
  (Phase 5), which is browser-facing, not to the callback, which is not.
- **What the hook actually runs.** A hidden `ws-dashboard` subcommand, in the
  same style as the existing hidden `terminal-helper`
  (`cli.rs:~31`, `#[command(hide = true)]`), invoked as
  `ws-dashboard terminal-notify --callback <path> --state ready`. This is
  chosen over `curl` deliberately: `curl` is not guaranteed present, is awkward
  to quote portably inside a vendor JSON config, and does not exist by default
  on Windows in the form the command line would assume.
- **On-disk layout**, all under the daemon state dir, all `0600`:
  - `terminal-tokens/<terminal_id>.json` — daemon-owned token store.
  - `agent-profiles/<terminal_id>/settings.json` — the materialized VENDOR
    config. Contains the hook command line and nothing secret.
  - `agent-profiles/<terminal_id>/callback.json` — the `--callback` target:
    `{ "baseUrl": ..., "terminalId": ..., "token": ... }`. Kept SEPARATE from
    the vendor config on purpose. The token must not sit inside a file a vendor
    binary parses and may echo, log, or forward; only `terminal-notify` reads
    this one.
  - `agent-profiles/<terminal_id>/` is the GC sweep's scan root.
- **Ephemeral port.** `callback.json` is per-terminal and the daemon REWRITES
  every live terminal's copy after `boot_reconcile` whenever its bound base URL
  changes. Deliberately not one well-known file: the state dir is per-user with
  no override flag (`persistent_state.rs:495-510`), so a shared file would let
  a second concurrent daemon silently steal every agent's callback target — and
  the acceptance harness runs its own daemon.
- **A turn boundary crossed while the daemon is down is LOST.** Hooks do not
  re-fire and the Phase 5 snapshot cannot reconstruct what it never received.
  Pinned default: on adoption through `boot_reconcile` an agent terminal's
  state is `idle`, and the next hook corrects it. Do not attempt to infer the
  missed transition.
- **Prior art to reuse, not reinvent.** `DocumentEventHub`
  (`work_root_files.rs:45-57`) is an existing `tokio::sync::broadcast` hub
  fronting an SSE route — the event stream should follow its shape rather than
  invent another. `claude_cli.rs` already builds a settings JSON containing a
  hook block (`:473-497`, injected at `:752-764`) — but it passes that JSON
  INLINE AS ARGV, so this ticket's `0600`-file requirement is a deliberate
  divergence from it, not an extension of it. Read it for the hook block shape,
  not for the delivery mechanism. Note also that it lives in the
  agent-GUI surface being wired out by
  `260725-refactor-dashboard-agent-gui-physical-module-isolation`, so extract
  rather than depend on it.

### The turn-START signal is not yet verified and gates the payload vocabulary

Only the turn-END signal is measured. `Stop` was verified firing in an
interactive PTY; nothing has established how `working` begins.

The candidate is a prompt-submission hook (`UserPromptSubmit` in the Claude
event set), which would fire when the human sends a turn. It has NOT been
verified in a PTY session, and it is not the same class of evidence as `Stop`.

Pinned resolution rather than an open question: Phase 3 must verify a
turn-start hook by the same method used for `Stop` (drive the real binary under
a PTY, observe the artifact) BEFORE the stream payload is fixed. If no
turn-start hook fires, the first slice ships a two-state vocabulary
(`ready` / `idle`) and `working` moves to `## Deferred scope` — the spinner
half of the nav counter goes with it. Do not infer `working` from output-idle
timing; that is the heuristic this whole design exists to avoid.

### The first spawn produces an ordinary terminal pane

The research ticket's open question 1 ("how thin the additive layer is —
`kind`/profile flag versus wrapper component") is NOT a prerequisite here. A
toolbar spawn action must yield one definite `SurfaceKind`, and for this ticket
that kind is `persistentTerminal`: an ordinary terminal whose command happens
to be an agent CLI. Whether the agent later earns a distinct `SurfaceKind` is a
separate decision that this ticket neither makes nor blocks.

Consequence: attention is keyed by `terminal_id` and works for any terminal, so
Phases 3-8 are independent of that open question. Phase 2 is precisely the
phase that settles it, so it is not independent — it is where the decision is
cashed in.

This resolves an open question the research anchor had assigned to sage/design
time. Record the resolution back on
`260725-research-ws-dashboard-pty-agent-pivot` when this ticket lands, rather
than leaving that ticket asserting the question is still open.

## Constraints

- **No PTY wheel reinvention** (research ticket's load-bearing invariant). This
  ticket adds argv/env passthrough and a config-file write at the EXISTING
  spawn seam. It must not fork the terminal substrate, add a second helper
  kind, or introduce a parallel PTY implementation.
- **Env scrub applies at BOTH hops.** Scrubbing only inside `spawn_shell`
  leaves the helper's own inherited environment dirty, and the helper's env is
  what `portable-pty` seeds the child from.
- **No registry schema change.** See the decision above.
- **The callback route sits OUTSIDE `require_owner_auth`, and that is a
  contract change.** Only `/pair` and `/api/dashboard/link-auth` are outside
  the blanket layer today (`router.rs:466-469`). Adding a third unauthenticated
  route touches the daemon-foundation auth boundary and must be spec-addressed,
  not slipped in. A corollary correction: an earlier draft claimed `--no-auth`
  "voids the scoped token". It does not — the token is checked in the handler,
  not by the middleware, so `--no-auth` changes nothing about it.
- **The callback URL cannot be baked at spawn time.** `--port` defaults to `0`
  (`cli.rs:71-72`), i.e. an ephemeral port chosen at bind. A URL written into a
  config file at spawn would be wrong for any helper re-adopted by a restarted
  daemon through `boot_reconcile`. The bound base URL must be resolvable at
  hook-FIRE time, not frozen at spawn time.
- **Identity privacy.** The callback token is a new daemon-private credential.
  It must never become browser route, command, or pane identity, per
  `#260521-ws-dashboard-activity-console-read-model`. Note it is a genuine
  credential, unlike `wsSessionKey` — whose privacy is a workflow-integrity
  concern rather than an access-control one.
- **Attention flash must be an overlay layer.** `background` on
  `.resource-row` is written by the base rule (styles.css 2729), `:hover`
  (2743), and `-error` (2757). An animation on `background` would fight all
  three, so the flash must be an independent layer (e.g. a pseudo-element).
  NOTE for whoever implements this — the selected-row gradient is ALREADY
  dead, which is itself the strongest argument for the overlay approach.
  `App.tsx:7431` puts `resource-row`, `resource-row-<tone>` and
  `resource-row-selected` on the SAME element. `.resource-row-selected`
  (styles.css 1081-1090) and `.ws-row-selected` (232-235) both declare
  `background`, but the base `.resource-row` rule at 2729 comes later at equal
  specificity (0,1,0), so source order wins and only `border-left-color` and
  `box-shadow` survive from the selected rule. Confirm this before building on
  either rule, and report it to
  `260725-feat-dashboard-nav-row-two-line-open-state` rather than silently
  working around it.
- **macOS blocks verification of every phase, including unit tests.** The
  daemon does not compile on macOS at all
  (`260725-bug-dashboard-terminal-platform-macos-unsupported`), so
  `cargo test -p ws-dashboard-daemon` cannot run here either. An earlier draft
  softened this to "not a prerequisite for writing the code" — that is true
  only in the narrowest sense. On this machine, nothing in this ticket can be
  verified until the macOS ticket lands. Treat it as a hard sequencing
  prerequisite, not a caveat. The one escape hatch is the same one the macOS
  ticket already names: cross-compile checking against a Linux target, or a
  container. Use it knowingly — it does not exercise the macOS platform leaf,
  so it proves compilation, not behaviour.
- **Scope boundary with `260624-feat-ws-dashboard-managed-cli-terminal`.**
  Phase 1 lands part of that ticket's Phase 1 (argv/env commonization). It does
  NOT land the rest of it (output ring, cursoring, resize, status, bounded
  fallback reads are all already present; vendor profiles are only partially
  covered here). On completing Phase 1, append an Edition note to `260624`
  recording which part is absorbed, so it does not sit in `todo/` silently
  half-done. Whether `260624`'s 2026-07-11 supersession is formally reversed
  stays an open question on the research anchor and is NOT decided here.

## Deferred scope

- **Model selection exposure.** The pivot's feature 2. Independent of the
  notification path; not in this ticket.
- **MCP injection of any kind.** Ruled out of the pivot by owner decision. The
  dashboard-as-MCP-server direction stays in
  `260711-idea-dashboard-agent-facing-mcp-control-surface`.
- **Web Push / VAPID / service-worker push.** Delivery while the tab is closed
  needs unconditional HTTPS plus a push backend. Explicitly out of scope; the
  existing `sw.js` stays an 11-line installability stub.
- **A distinct `SurfaceKind` for agent terminals.** See the decision above.
- **Codex profile.** Codex has a hook subsystem gated by persisted trust
  (`--dangerously-bypass-hook-trust`), but its config shape is unknown and the
  trust step has no design. Phase 1 keeps the profile seam vendor-neutral;
  only the Claude profile is implemented here.

## Spec Impact

Contract-first: no — with one recorded reservation. The completeness review
flagged that a net-new browser-facing event stream normally warrants a
contract-first spec entry rather than a phase-owned one. The reservation is
accepted rather than dismissed: Phase 5's spec entry is a hard merge gate, not
a follow-up, and the phase is written so the entry lands before the route does.
If a later reviewer wants this converted to contract-first, that is a
legitimate correction and costs one spec-writing pass.

Expected caller-visible change: the browser gains an optional profile parameter
on terminal creation, one new token-authed callback route that is not
owner-authenticated, one new server-scoped attention event stream, and a
Settings entry for notification permission. Existing terminal creation, output,
and transport contracts are unchanged when no profile is requested.

- `#260516-ws-web-dashboard-terminal-registry-pty-spawn` — amend BOTH ends.
  (a) the spawn seam, for argv/env passthrough and the environment
  scrub/allowlist; today the spec describes spawn as carrying rows/cols/cwd
  only. (b) the browser-facing create contract, for the profile parameter —
  `CreateTerminalRequest` (`terminal.rs:563-570`), `TerminalSession::spawn`
  (`terminal.rs:805-813`), and `createTerminal` (`terminals.ts:205-232`) all
  carry the same four fields today, so this is a caller-visible change and not
  an internal one.
- `#260516-ws-web-dashboard-terminal-pane` — this spec states the terminal pane
  "is a shell terminal substrate only; it does not hardcode Codex, Claude, or
  other agent presets". Phase 2 must amend it to permit a sibling profile over
  the SAME single-sourced plumbing while keeping the anti-fork rule explicit.
  Do not delete the sentence; tier it.
- `#260515-ws-web-daemon-foundation` — the spec currently states "The pairing
  route is the only unauthenticated browser entrypoint"
  (`ai-docs/spec/ws-web-dashboard/index.md:38-39`). The callback route
  contradicts that sentence, so it must be tiered into a named second class
  (token-authed, non-browser, per-terminal) rather than left implicit. The
  matching in-source CONTRACT comment at `router.rs:97-102`, which enumerates
  what stays outside the protected router, must be amended in the same change.
  Phase 4 owns this; it is the riskiest edit in the ticket and should not be
  batched with anything else.
- NEW entry for the attention event stream (Phase 5). Net-new browser-facing
  contract — route pair (local and server-scoped), payload shape, the initial
  snapshot-on-connect behaviour, and an explicit statement that it is
  independent of the Activity Console projection. Phase 5 does not ship
  without it.
- `#260521-ws-dashboard-activity-console-read-model` — extend the identity
  three-class model with the per-terminal callback token as a fourth
  daemon-private class, distinguishing it from `wsSessionKey` (a genuine
  credential versus a workflow-integrity concern).
- `#260722-ws-dashboard-settings-panel` — Phase 8 adds a
  notification-permission section entry.
- Whatever spec entry `260725-feat-dashboard-nav-row-two-line-open-state`
  lands for the secondary-line counts — Phase 7 changes their SEMANTICS (an
  agent terminal leaves the terminal count and joins the agent count), which
  amends behaviour that ticket will have already shipped and asserted. Amend
  that entry and append a note to that ticket; do not diverge silently.


## Phases

Phases 1-6 form one vertical slice: after Phase 6 an agent CLI can be spawned
from the UI, finish a turn, and make its tab react. Phases 7-8 are additive
presentation on top of that slice.

Every phase's verification is blocked on
`260725-bug-dashboard-terminal-platform-macos-unsupported` while working on
macOS — including the pure unit tests, since the daemon crate does not compile.
Phases may be written before it lands; none may be marked done.

ONE EXCEPTION, and it should be taken first: Phase 3's step 1 is a
verification spike against a vendor binary under a PTY. It touches no daemon
code, so macOS does not block it, and its result determines the state
vocabulary that Phases 4-7 all encode. Run it out of order, ahead of Phase 1,
as soon as this ticket is picked up.

### Phase 1: argv/env passthrough and environment scrub at the spawn seam

Extend `TerminalHelperArgs` and `spawn_shell`
(`terminal_helper_process.rs:391-407`) to carry an explicit command argv and env
overlay instead of hardcoding `default_shell()` at L399, and add a
scrub/allowlist step applied at BOTH hops — the daemon's helper spawn
(`terminal.rs:817-844`) and the helper's shell spawn.

The scrub list belongs to a vendor-neutral profile seam; only the Claude marker
set is populated here.

Do NOT add a field to `TerminalRegistryEntry`. If one ever becomes unavoidable
it must be `Option<T>` + `#[serde(default)]` — see
`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`.

Verification: unit tests asserting scrubbed markers are absent from the
constructed env and that a default (no-argv) spawn still produces the existing
shell behaviour, so ordinary terminals are provably unchanged.

### Phase 2: agent spawn path from the browser

Depends on Phase 1. Owns the `#260516-ws-web-dashboard-terminal-pane` spec
amendment.

`CreateTerminalRequest` (`terminal.rs:563-570`) carries only columns, rows,
title, and cwd_hint — there is no way to ask for anything but a shell. Without
this phase nothing downstream can be exercised end to end.

- Add an optional profile selector to `CreateTerminalRequest` and resolve it
  against a small vendor profile registry (command argv, env scrub list, hook
  config shape). Absent profile keeps today's shell behaviour byte for byte.
- Wire the top-right toolbar slot vacated by the Tier 1 agent-GUI suspension
  (`c3f5b42b`) to this path. The spawn MUST NOT route through
  `registerNewAgentChatPane`, which is one of the three `AGENT_GUI_SUSPENDED`
  guard depths and would either no-op or re-open the suspended surface.
- The resulting pane is `SurfaceKind: "persistentTerminal"`. The pane must
  record which profile produced it, because Phase 7 needs to tell an agent
  terminal from a shell terminal and Phase 7's counter must not double-count it
  against the nav ticket's terminal count.

Verification: a browser acceptance step spawning a terminal under a DUMMY
profile — a trivial local command, never a real vendor CLI — and asserting the
pane opens with the profile recorded; plus an existing-shell-terminal
regression step. The acceptance suite must not acquire a dependency on a vendor
binary, credentials, or network; Phase 6 states the same rule and this phase
must not quietly break it.

### Phase 3: turn-start verification, hook config materialization, notify subcommand

Depends on Phase 2.

1. FIRST, verify a turn-start hook by the method that verified `Stop`: drive
   the real vendor binary under a PTY and observe the artifact. If none fires,
   drop `working` from the vocabulary per `## Decisions` and record that here.
   Everything downstream depends on the answer, so this precedes the code.
2. Daemon materializes the vendor hook config under
   `agent-profiles/<terminal_id>/` at `0600` and passes its PATH through helper
   argv.
3. Add the hidden `ws-dashboard terminal-notify` subcommand and the
   bound-base-URL file the daemon rewrites on every bind.

Verification: the spike result recorded explicitly (including a negative
result); a test that `terminal-notify` resolves a base URL written after the
config file, proving the ephemeral-port path works.

### Phase 4: token store and the callback endpoint

Depends on Phase 3. Owns the `#260515-ws-web-daemon-foundation` spec amendment,
which is the riskiest edit in this ticket and should not be batched.

- Daemon-owned token store at `terminal-tokens/<terminal_id>.json`, with
  `boot_reconcile` (`terminal.rs:196-220`) repopulating the in-memory
  token -> terminal lookup BEFORE serving. The current `TerminalSession`
  construction path has no slot for auth material; adding one is part of this
  phase.
- The route pair from `## Decisions`, registered outside `require_owner_auth`
  with a handler-level token check.
- A GC sweep over `agent-profiles/` reclaiming directories with no live
  terminal record. No existing daemon-side directory sweep exists to reuse
  (`delete_registry_entry`, `terminal_registry_file.rs:68-71`, is per-entry),
  so this is a net-new background task. It keys off TERMINAL liveness, so a
  config legitimately outlives an agent that exited inside a surviving
  terminal.
  ORDERING, load-bearing: the sweep must run strictly AFTER `boot_reconcile`
  completes. If it runs first, every helper the daemon is about to re-adopt
  looks recordless, and the sweep deletes the config files of agents that are
  still running against them. Trigger is boot-after-reconcile plus a periodic
  tick.

Verification, all with owner auth ENABLED — feasible because the acceptance
harness already scrapes the one-time owner pairing URL
(`frontend/e2e/daemonHarness.ts`): a valid token is accepted, a wrong and an
absent token are rejected, a token for terminal A cannot post for terminal B,
an orphaned profile directory is reclaimed, and the sweep does not touch a
directory belonging to a helper adopted by `boot_reconcile`.

Restart verification must cover the URL, not just the token: after a daemon
restart on a DIFFERENT port with a live helper still running, a callback from
that helper must still arrive. Testing only token survival would pass while
pointing at a dead port.

### Phase 5: server-scoped attention event stream

Depends on Phase 4. Owes the NEW spec entry named in `## Spec Impact` before
the route merges.

A `tokio::sync::broadcast` hub plus an SSE route, following `DocumentEventHub`
(`work_root_files.rs:45-57`) rather than inventing a second pattern.

- The stream is SERVER-SCOPED with a forwarding sibling. The exact precedent to
  copy is `server_scoped_work_root_activity_events`
  (`servers.rs:1174-1185`), which dispatches on `server_route ==
  LOCAL_SERVER_ID` and otherwise forwards; `DocumentEventHub` has the same
  shape via `server_scoped_document_events`. Consequently the frontend holds
  ONE SUBSCRIPTION PER LINKED SERVER, not one global subscription — a terminal
  on a linked server runs under the remote daemon, so its hook posts to the
  remote daemon and its broadcast lands in the remote daemon's channel. An
  earlier draft called this "dashboard-scope" with a single `App`-level
  subscription, which would have silently dropped every remote agent.
- It carries an initial snapshot on connect. Without one, a browser refresh
  loses every pending attention state, since a broadcast channel has no
  history.
- It does NOT touch `work_root_activity.rs`. The Activity Console's own
  streaming gap is real but is not this ticket's to fix.

Verification: a state transition for a NON-selected work root reaches the
client with no Activity Console pane open — the exact case the rejected reuse
path could not serve; and a reconnect receives pending state via the snapshot.

### Phase 6: tab-label indicator — end-to-end slice closes here

Depends on Phase 5.

`DockviewWorkbenchTab` (`workbench/dockviewLayout.tsx:343-401`) renders only
icon, title, and close. Add a state affordance driven by the Phase 5 stream,
with acknowledgement clearing it — reuse the ack-watermark PATTERN, not the
`attention` field.

Verification is split deliberately, because the acceptance suite does not
invoke real vendor CLIs and should not start:
- Automated: drive the Phase 4 callback endpoint directly to synthesize a turn
  boundary, then assert the indicator appears and clears on acknowledgement.
- Recorded manual: one real run with an actual agent CLI, captured in the
  phase Result. This is the only step that proves hook injection and the
  browser path work together.

### Phase 7: nav-row presentation

Depends on Phase 5 and on `260725-feat-dashboard-nav-row-two-line-open-state`
Phase 1 having landed the two-line row, whose Deferred scope already reserves
this slot.

Fills the deferred agent-counter slot with a split count — working N (spinner)
/ ready M (orange bell) — and adds the owner-requested Windows-11-style orange
flash as an independent overlay layer per `## Constraints`. If Phase 3's spike
found no turn-start hook, the spinner half is dropped with it.

An agent terminal counts in the AGENT counter only, never also in the terminal
count, or the two tickets double-count the same pane. The carrier is the
profile recorded on the pane in Phase 2 — NOT "terminals that have posted a
hook event", which would read zero for a freshly spawned agent that has not
finished a turn yet.

The two rules the review flagged as unpinned are PINNED here rather than
forwarded to implementation time:

- **Aggregation**: a server row shows the highest-priority state among its work
  roots (`ready` outranks `working` outranks none), because a server row exists
  to tell the user whether anything under it needs them.
- **Acknowledgement**: the nav badge is DERIVED, never separately acknowledged.
  Acknowledging a tab clears that terminal's state; the row's badge clears when
  no child terminal is still pending. This avoids two independent ack
  watermarks that can disagree.

Verification: browser acceptance asserting the counter split, no double count,
a badge on a work root that is not selected, and that acknowledging the last
pending tab clears the row badge without a separate action.

### Phase 8: browser-level notification

Depends on Phase 5. Independent of Phases 6-7. Owns the
`#260722-ws-dashboard-settings-panel` spec entry.

`document.title` flashing plus a favicon badge as the zero-permission default —
the only tier that works over plain-http LAN access, where `Notification` is
unavailable because the page is not a secure context.

Then `Notification` as an explicit opt-in requested from a user gesture (a
Settings toggle in `settingsSections.tsx`), never on load. State plainly in the
Settings copy that OS-level notification requires localhost or TLS, so the
limitation is visible rather than surprising.

Verification: the title/favicon tier asserted in browser acceptance; the
permission tier verified manually and recorded, since driving a real permission
prompt in the harness is not worth its cost.
