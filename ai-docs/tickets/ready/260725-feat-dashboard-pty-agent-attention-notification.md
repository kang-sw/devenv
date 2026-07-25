---
title: PTY-agent attention notification — hook-injected turn signal to tab, nav row, and browser
related:
  260725-research-ws-dashboard-pty-agent-pivot: direction anchor; this ticket implements its `## Notification Path` section and inherits its verified facts and corrections
  260624-feat-ws-dashboard-managed-cli-terminal: pre-written PTY-agent substrate design; Phase 1 argv/env commonization overlaps this ticket's Phase 1
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: detached-helper model that makes the daemon the only viable owner of injected config
  260725-feat-dashboard-nav-row-two-line-open-state: owns the two-line nav row whose deferred agent-counter slot Phase 7 fills
  260725-refactor-dashboard-agent-gui-physical-module-isolation: Tier 2 wire-out that owns the agent-GUI surface Phase 2 must not route through, and the module holding the existing settings-JSON builder
  260725-bug-dashboard-terminal-platform-macos-unsupported: WAS a block on native macOS verification of every phase here; cleared 2026-07-25 (ticket closed under .done/, driving merges bfbc1a1c and ed255029 landed, `cargo check -p ws-dashboard-daemon` exits 0 on this machine)
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

ACKNOWLEDGEMENT IS KEYED PER TERMINAL, and the nav badge is DERIVED from it.
This lives here rather than in Phase 7 because Phase 6 builds the ack store and
would otherwise have to guess the key and be reworked. Acknowledging a tab
clears that terminal's state; a row's badge clears when no child terminal is
still pending. There is exactly one ack watermark, never two that can disagree.
(Server-row AGGREGATION is a separate question and stays in Phase 7, since it
affects only rendering.)

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
  (`cli.rs:26`, `#[command(hide = true)]`), invoked as
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

**Resolved 2026-07-25, positive.** The Phase 3 step-1 spike (see that phase's
step-1 record) measured `UserPromptSubmit` firing in a real interactive PTY
session. `working` therefore STAYS in the vocabulary and the spinner half of
the nav counter is NOT deferred. This resolves the gate; it does not complete
Phase 3 — steps 2-3 (hook config materialization, the `terminal-notify`
subcommand) are untouched.

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
- `#260516-ws-web-dashboard-inspectable-navigation-shell`
  (`ai-docs/spec/ws-web-dashboard/index.md:983`) — Phase 7 changes the
  secondary-line count SEMANTICS that
  `260725-feat-dashboard-nav-row-two-line-open-state` will already have
  shipped. That ticket's Phase 1 derives terminal counts from `terminalPanes`
  WHOLESALE and its Deferred scope does not anticipate an exclusion, so
  Phase 7's "an agent terminal counts in the agent counter only" amends
  landed, asserted behaviour — including that ticket's acceptance step. Amend
  the spec entry and append a note to that ticket; do not let it be discovered
  as a broken assertion.


## Phases

Phases 1-6 form one vertical slice: after Phase 6 an agent CLI can be spawned
from the UI, finish a turn, and make its tab react. Phases 7-8 are additive
presentation on top of that slice.

Every phase's verification WAS blocked on
`260725-bug-dashboard-terminal-platform-macos-unsupported` while working on
macOS — including the pure unit tests, since the daemon crate did not compile.
That block is CLEARED as of 2026-07-25: the blocking ticket is closed under
`ai-docs/tickets/.done/`, its driving merges landed (`bfbc1a1c` macOS
terminal_platform port Phase 1, `ed255029` native macOS lifecycle acceptance
Phase 2), and `cargo check -p ws-dashboard-daemon` exits 0 on this machine as
of that date. Phases may now be verified and marked done on macOS in the
normal course.

Independently of that (now-cleared) block, one ordering obligation still
applies and should be taken first: Phase 3's step 1 is a verification spike
against a vendor binary under a PTY. It touches no daemon code, and its result
determines the state vocabulary that Phases 4-7 all encode. Run it out of
order, ahead of Phase 1, as soon as this ticket is picked up.

Treat that spike as a SEPARABLE GATE, not merely as Phase 3's first bullet: it
may be completed and its result recorded on its own, ahead of and independently
of every other phase. It is deliberately left inside Phase 3 rather than
renumbered into its own phase because the phase numbering is already stamped;
the ordering obligation is what matters, and it is stated here so it cannot be
missed by an implementer reading phases in order.

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

### Result (9f4a16ca) - 2026-07-26

Done. `TerminalHelperArgs` gained `--command`, repeated `--command-arg`, and
repeated `--env-overlay KEY=VALUE`
(`ws-dashboard/crates/daemon/src/cli.rs`, `1deaa070`), and a vendor-neutral
`EnvScrubProfile` seam (`ws-dashboard/crates/daemon/src/agent_env_profile.rs`,
`dd425a9e`) applies at BOTH spawn hops: the daemon's helper spawn
(`terminal.rs`, `c67d6dfd`) and the helper's own shell spawn
(`terminal_helper_process.rs`, `977baecf`). Only the Claude marker profile is
populated — an ENUMERATED 11-item deny-list (`CLAUDECODE`,
`CLAUDE_CODE_BRIDGE_SESSION_ID`, `CLAUDE_CODE_CHILD_SESSION`,
`CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_EXECPATH`,
`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS`, `CLAUDE_CODE_SESSION_ID`,
`CLAUDE_EFFORT`, `CLAUDE_PID`, `CLAUDE_WATCHER_TOKEN`, `AI_AGENT`) rather than
a `CLAUDE`-prefix rule, because a prefix rule would both over-match any future
unrelated `CLAUDE*` var and silently exclude `AI_AGENT`, which carries no
`CLAUDE` prefix. Scrubbing is subtractive (deny-list) and triggers only on the
presence of explicit argv, so the no-argv default path is untouched by
construction rather than by parity-checking. `TerminalRegistryEntry`
(`terminal_registry_file.rs`) is unchanged — confirmed by an empty diff
against the branch point (`e0668c40^`) — satisfying the ticket's no-registry-
schema-change constraint.

Follow-on commits from a review cycle: `4a20a378`, `8de998b9`, `9f4a16ca`.
Related docs on this branch: `e751e89a` (mental model), `1fbf9a0f` (idea
ticket).

**Verification (re-run and confirmed on this machine, 2026-07-26):**
`cargo test -p ws-dashboard-daemon --lib` → 140 passed, 0 failed, 2 ignored,
exit 0 (baseline before this phase was 134 — net +6 tests). `cargo test -p
ws-dashboard-daemon --test terminal_lifetime` → 4 passed, 0 failed, exit 0.
`cargo check -p ws-dashboard-daemon --tests` → exit 0.

**Non-vacuity.** Each new regression guard was proven by mutating production
source, observing the intended failure, then reverting: an unconditional
env-clear added to the default (no-argv) branch fails the default-path guard
at both hops; a narrow allowlist keeping only `PATH` fails the deny-list
assertion at both hops; removing the "scrub wins over overlay" guard lets an
overlay resurrect `CLAUDECODE` and fails that assertion.

**Review outcome.** fit: 0 Critical / 0 Important / 2 Minor, both accepted
as-is — the missing profile-selector parameter is Phase 2's additive surface,
and the `host_env: impl IntoIterator` shape on the scrub function legitimately
diverges from a point-lookup closure pattern because a scrub needs full
enumeration, not single-key lookups. correctness: 0 Critical / 3 Important,
all fixed. test: 1 Critical / 2 Important, all fixed. Findings that carry
forward:

1. `CommandBuilder::env_clear()` (hop 2, `portable-pty`) destroyed
   `portable-pty`'s Windows base-env construction (system+user `PATH` merged
   from `HKLM`/`HKCU`), so a Windows agent-profile terminal would have
   resolved its program against a narrower `PATH` than a shell terminal in
   the same helper. Fixed by switching hop 2 to per-marker
   `command.env_remove(marker)` (`terminal_helper_process.rs:452`,
   `8de998b9`). Hop 1 deliberately KEEPS `env_clear()` + repopulate:
   `std::process::Command` has no comparable base-env construction to
   destroy, so this is not a correctness fix there, and an independent probe
   showed switching hop 1's mechanism would not close hop 1's own test blind
   spot either (see point 4).
2. `--command-arg` / `--env-overlay` were silently dropped when `--command`
   was absent; now a hard failure — clap `requires = "command"` at hop 2
   (`cli.rs:70,82`) and a `TerminalError::BadRequest` guard at hop 1
   (`terminal.rs:896`).
3. `--env-overlay` is an argv channel carrying arbitrary env VALUES, which
   cuts against this ticket's load-bearing invariant (`## Constraints`,
   "identity privacy" / "token never touches the helper") that helper argv is
   world-readable via `ps` and therefore must carry file PATHS only. Nothing
   populates `--env-overlay` yet. A `CONTRACT` note at `cli.rs:72` now names
   that invariant and names Phase 4's callback token as the specific value it
   must never carry. RECORDING PROMINENTLY: this is a constraint on Phase 4,
   not a footnote on Phase 1 — the callback token must never be threaded
   through `--env-overlay`.
4. The mandatory "default spawn unchanged" guard was originally VACUOUS:
   `get_envs()` / `iter_extra_env_as_str()` cannot distinguish "no env
   manipulation" from "`env_clear()` then re-add `TERM`" — verified
   empirically with isolated probes. Now fixed at both hops, but hop 1's
   replacement guard infers the clear from `std::process::Command`'s unstable
   `Debug` rendering and is `#[cfg(unix)]`-gated
   (`terminal.rs:2064-2085`), so Windows has no coverage for that specific
   regression at hop 1. Captured as
   `260726-chore-dashboard-terminal-hop1-env-clear-guard-fragile` in `idea/`
   — cross-referenced from that ticket back to this one.
5. Decision, now settled rather than left open: an env overlay may NOT
   resurrect a scrubbed marker — the scrub wins, with a warning log on the
   attempt. Recorded here so a later phase does not re-litigate it.

**Deferred / not done.** Phase 2 (profile registry and the browser spawn
path), Phase 3 steps 2-3, and all later phases. Only the Claude marker set is
populated; the seam is vendor-neutral so Phase 2 extends it rather than
rewriting it.

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

### Result (5bc8ad28) - 2026-07-26

Done. `agent_profile_registry.rs` (new) is a pure static lookup table -
`AgentProfile { id, command, args, scrub, hook_config }` - with two entries:
`"claude"` (composes Phase 1's `agent_env_profile::CLAUDE`) and a
UI-invisible `"dummy-echo"` test profile (a real, always-registered entry
per the survey's design answer 3, reachable only through the daemon's
production route, never a user-facing control). `CreateTerminalRequest`
gained `profileId: Option<String>` (`#[serde(default)]`); a new pure
`resolve_create_command` (`terminal.rs`) resolves it before
`TerminalSession::spawn` - unknown id is a `BadRequest`, absent id is a
literal no-branch-taken path producing the exact prior `(None, Vec::new())`
call. Provenance (`profile_id: Option<String>`) was added to
`TerminalSession`/`TerminalSessionView` (camelCase `profileId`), NOT to
`TerminalRegistryEntry` (hard constraint untouched - confirmed no diff to
that struct). Frontend: `TerminalCreateOptions.profileId`,
`TerminalSessionView.profileId`, a new `terminal.create.agent` command
(profile id fixed at `"claude"` in the handler, not carried in the
payload), and a `ChromeIconButton` ("New agent terminal", icon `Bot`) added
beside the existing "New terminal" button, OUTSIDE the
`AGENT_GUI_SUSPENDED ? null : (...)` block.

**Deviation from a plan Codebase Finding, reported as directed.** The
plan's Codebase Findings said `build_helper_command`'s hop-1 scrub call
(hardcoded `&CLAUDE`) needed no signature change for Phase 2. Leaving that
hardcoding in place would have made every profile's own `scrub` field dead
code - unread by anything, for both current profiles, which is exactly the
class of "merely plausible replacement" vacuity Phase 1's review already
flagged once (finding 4 below). `build_helper_command` now takes
`scrub: Option<&EnvScrubProfile>` and applies the CALLER-SUPPLIED resolved
profile's own list (falling back to `CLAUDE` only as a defensive default).
Proven with a unit test supplying a synthetic non-CLAUDE scrub profile and
asserting ITS marker - not a CLAUDE marker - is what gets stripped.

**Restart provenance loss, made visible per this phase's own instruction.**
`reconcile_entry`'s adopt arm passes `profile_id: None` with an explicit
CONTRACT comment: unlike turn state (self-corrects on the next hook after
adoption), profile provenance has NO self-correction signal, so a
re-adopted agent terminal permanently under-counts against Phase 7's
counter until it is closed and a fresh one is spawned. This is a candidate
follow-up `idea/` ticket, not fixed here.

Follow-on commits from a review cycle: `0c788730`, `87214f93`, `5bc8ad28`
(the last is a documentation-only correction to this Result's own
verification numbers, see below).

**Known open item, not a Phase 2 defect.** The fit review noted that
`## Spec Impact` bullet part (a) — the Phase 1 spawn-seam description
(argv/env passthrough and the scrub/allowlist at the daemon-helper spawn
seam itself, independent of the profile registry) — remains unamended in
the spec. Phase 2's own spec edit (`74a668b3`) covers part (b), the
browser-facing profile parameter on `CreateTerminalRequest`; it does not
touch part (a). Phase 1 did not introduce this gap either — that phase's
own doc pass concluded no spec change was owed because the scrub is not
caller-visible. Recorded here rather than silently left, so a later phase
can settle whether part (a) is owed at all.

**Verification (this machine, 2026-07-26).** `cargo test -p
ws-dashboard-daemon --lib` -> 147 passed, 0 failed, 2 ignored (Phase 1
baseline 140, net +7: 3 registry-resolve tests, 3 `resolve_create_command`
tests, 1 scrub-wiring non-vacuity test). `cargo test -p ws-dashboard-daemon
--test terminal_lifetime` -> 4 passed, 0 failed (unchanged). `cargo check -p
ws-dashboard-daemon --tests` -> exit 0. `npm run build` (tsc -b + vite
build) -> clean. All 21 pure-TS `npm run test:*` suites -> exit 0.
Browser: `npx playwright test --grep "agent spawn profile"` (new dedicated
sibling spec `e2e/agent-spawn-profile.spec.ts`, own daemon/workRoot - see
that file's CONTRACT comment for why it is not a `test.step` inside
`dashboard-acceptance.spec.ts`) -> 1 passed, run twice for stability. It
proves the plan's own browser step never covered: clicking the real
toolbar button dispatches `terminal.create.agent` (via the toolbar's
`data-last-command-id` command-observer attribute), the resulting pane is
an ordinary `persistentTerminal` recorded `profileId: "claude"`, and no
`data-surface-kind="agentChat"` pane is ever registered. It does not wait
for or assert that the underlying `claude` binary starts successfully, so
it acquires no dependency on a vendor binary, credentials, or network. Full
regression: `npx playwright test dashboard-acceptance.spec.ts` fails only
at the pre-existing, already-tracked fitNow short-viewport assertion
(`todo/260725-bug-dashboard-fitnow-short-viewport-shrink`); every other
step, including toolbar/command-log assertions the new button could have
disturbed, passes.

**Final verification, re-run on the final committed state after the review
cycle's fixes landed (this machine, 2026-07-26; the lead independently
reproduced the Rust figure).** `cargo test -p ws-dashboard-daemon --lib` ->
148 passed, 0 failed, 2 ignored, exit 0 (Phase 1 baseline 140; net +8 over
Phase 1, one more than this Result's own pre-review 147 - the additional
lib test is the hop-2 synthetic-marker regression guard added in
`0c788730`). `cargo test -p ws-dashboard-daemon --test terminal_lifetime`
-> 4 passed, exit 0 (unchanged). `npm run build` -> exit 0. All 21 pure-TS
suites -> exit 0 each. `npx playwright test --grep "agent spawn profile"`
-> 1 passed, exit 0. Full `dashboard-acceptance.spec.ts` -> exit 1 with the
sole failure at `:3779`, the same pre-existing, already-tracked fitNow
short-viewport regression named above - judged by failure site, untouched
by this review cycle's fixes.

**Non-vacuity (mutated production source, observed the intended failure,
reverted).** (a) `resolve_create_command`'s absent-`profile_id` branch
mutated to fall through to `"claude"` - failed the no-branch-taken unit
test (`left: Some(("claude", [])) right: None`). (b) `build_helper_command`
mutated to always apply `agent_env_profile::NONE` regardless of the
supplied `scrub` - failed both the Claude-marker deny-list test and the
new synthetic-scrub-profile test. Both reverted; `cargo test -p
ws-dashboard-daemon --lib` clean afterward. (c) Review-cycle addendum:
`apply_scrub_and_overlay` (hop 2) mutated to ignore its threaded `markers`
argument and scrub the hardcoded `CLAUDE` list instead, simulating the
pre-fix behavior - failed the new
`apply_scrub_and_overlay_uses_the_supplied_marker_list_not_a_hardcoded_claude_one`
assertion (`left: Some("scrub-me") right: None`); reverted, `cargo test -p
ws-dashboard-daemon --lib` back to 148 passed.

**Review outcome.** fit: 0 Critical / 0 Important / 0 Minor - clean.
correctness: 0 Critical / 2 Important / 5 Minor. test: 0 Critical /
1 Important / 1 Minor. All four non-minor findings were dispositioned
[fixed] and are fixed in `0c788730`, `87214f93`, and (for the factual
correction to this Result's own verification numbers) `5bc8ad28`. The four
recorded below each change what a later phase must know; the remaining
Minor findings (four correctness, none test) carry no forward obligation
and are not repeated here.

1. **Scrub asymmetry (correctness, Important).** Hop 1
   (`build_helper_command`) honoured the resolved profile's own scrub list,
   but hop 2 (the helper's own `apply_claude_scrub_and_overlay`) still
   iterated a hardcoded `agent_env_profile::CLAUDE` unconditionally,
   contradicting both hop 2's own CONTRACT comment and this ticket's
   "the scrub applies at BOTH hops" constraint. Latent at the time of the
   original Result above: every registered profile either used the Claude
   marker set or declared an empty scrub, so the two-hop gap produced no
   observable bug yet — but the first profile with non-Claude markers (the
   deferred Codex profile, e.g.) would have leaked its markers at hop 1 only.
   Fixed by threading hop 1's resolved scrub list to hop 2 through a new
   repeated `--scrub-marker` helper argv flag; `apply_claude_scrub_and_overlay`
   is renamed `apply_scrub_and_overlay` and now scrubs the caller-supplied
   list instead of a hardcoded one, with a new hop-2 test
   (`apply_scrub_and_overlay_uses_the_supplied_marker_list_not_a_hardcoded_claude_one`)
   mirroring the existing hop-1 synthetic-profile assertion. RECORDING
   PROMINENTLY for later phases: helper argv now carries scrub marker NAMES
   in addition to file paths. That is deliberate and safe — marker names are
   env-var KEYS, never values — and it does NOT relax the standing rule that
   argv must never carry secrets. Phase 4's callback token is still forbidden
   from argv.
2. **Restore-intent respawn dropped `profileId` (correctness, Important).**
   After a daemon restart drops a live helper, the browser's restore-intent
   respawn path (`App.tsx`, fires when `listTerminals` returns zero sessions
   for a workRoot with a persisted `TerminalRestoreIntent`) rebuilt the
   terminal via `createTerminalPane({ title, cwdHint })` with no
   `profileId`, so an agent terminal came back as a PLAIN SHELL under its
   original, unchanged agent-style title — nothing in the UI told the owner
   the wrong process was now running. Worse than the provenance loss already
   captured in `idea/260726-bug-dashboard-agent-profile-provenance-lost-on-restart`,
   which loses only the daemon-side metadata; this ran the wrong process
   behind an unchanged label. Fixed in `87214f93`: `TerminalRestoreIntent`
   gains `profileId?: string | null`, round-tripping through the existing
   version-1 localStorage schema additively so an older persisted intent
   with no `profileId` key still parses. Verified at browser level by a new
   step that closes the other terminals through the normal UI flow, waits
   for that write to settle, direct-DELETEs the agent terminal (bypassing
   the browser's own close-tab flow, leaving a stale restore intent to
   simulate a daemon restart), reloads, and asserts the respawned pane
   carries `data-profile-id="claude"`.
3. **Vendor independence, now proven rather than argued (test, Important).**
   The plan called for the browser step to skip gracefully when `claude` is
   not installed; the ticket's own acceptance constraint has no such
   dependency to begin with. Rather than add a now-dead skip guard, the
   deviation is documented in the `agent-spawn-profile.spec.ts` header, and
   the underlying question was settled empirically: with the `"claude"`
   profile's command temporarily repointed at a nonexistent binary, the
   browser step still passed (1 passed, 1.4s, exit 0) — because the
   daemon's `create_terminal` HTTP response is already built and returned
   before the helper's `spawn_shell` ever attempts to run the resolved
   command, so a failed spawn can only flip the helper's internal status,
   never retroactively fail the already-returned response. The acceptance
   suite therefore has NO dependency on a vendor binary, credentials, or
   network — exactly as the ticket requires — proven rather than merely
   asserted.
4. **An assertion's comment claimed more than the assertion proved (test,
   Minor).** The agentChat-count-zero checks in
   `agent-spawn-profile.spec.ts` were annotated as proof that
   `registerNewAgentChatPane` was never routed to, but that function already
   no-ops under `AGENT_GUI_SUSPENDED` regardless of routing, so a
   hypothetical mis-wiring to `agentChat.create` would still produce zero
   agentChat panes there too. The load-bearing check is the adjacent
   `data-last-command-id` assertion, which reads the fired command id
   directly. Comments corrected in `87214f93` to say so.

**Deferred / not done.** Phase 3 steps 2-3 (hook config materialization,
`terminal-notify` subcommand) and all later phases (4-8). No
`TerminalRegistryEntry` change. No Codex profile (deferred scope). The
restart-provenance-loss gap above is flagged, not fixed.

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

#### Step 1 spike record — 2026-07-25 (SEPARABLE GATE result; NOT a Phase 3 `### Result`)

This is a partial record against Phase 3 step 1 only. Steps 2-3 (hook config
materialization, `terminal-notify` subcommand) are UNTOUCHED and Phase 3 as a
whole is NOT done. The ticket's `## Phases` preamble permits this step-1 gate
to be completed and recorded independently of the rest of the phase; this
section is that record, placed inline (rather than as a phase `### Result`,
which the ticket conventions reserve for a completed phase) because Phase 3
has no `### Result` yet. All backing artifacts lived under a session-scoped
scratchpad that does not survive the session, so this record is
self-contained: method, raw data, and cross-checks are inlined below rather
than cited by path.

**Result: POSITIVE.** `UserPromptSubmit` DOES fire at human turn submission for
the Claude CLI (v2.1.220) under a real interactive PTY. Consequence, per the
`## Decisions` gate this resolves: `working` STAYS in the three-state
`working`/`ready`/`idle` vocabulary, and the spinner half of the Phase 7 nav
counter is NOT deferred.

**Raw artifact (verbatim, both lines, one run, one prompt/reply turn):**

```
USER_PROMPT_SUBMIT 1784991563.367513000
STOP 1784991570.994170000
```

7.626657 s apart.

**Method (sufficient to reproduce without the scratchpad):** Python stdlib
`pty.fork()`; child `os.execve()` of the resolved absolute path to the
`claude` binary (execve does not search `PATH`) with argv
`["claude", "--settings", <path>]`. The settings file registers BOTH
`UserPromptSubmit` and `Stop` as
`{"matcher": "*", "hooks": [{"type": "command", "command": "echo \"<LABEL> $(date +%s.%N)\" >> <shared events.log>"}]}`.
The parent process waits ~8 s (one poll loop, 0.5 s `select()` tick), writes a
short prompt (`"Reply with exactly the word done and stop.\r"`) to the PTY
master fd, keeps polling, then — BEFORE any teardown (`SIGTERM`, fd close, or
`waitpid`) — reads `events.log` from the parent process and only then sends
`SIGTERM`. That read-before-teardown ordering is load-bearing: it is what
makes the timestamps trustworthy as fire-time observations rather than
teardown-time artifacts, and it is the same method that originally verified
`Stop`. CLI version 2.1.220. Exactly one CLI invocation, one short
prompt/reply turn, one settings file shared by both hooks.

**Why the timestamps are fire-time, not settings-write-time:** the settings
file holds `$(date +%s.%N)` as an UNEXPANDED shell literal (verified by
reading the settings file back: the command string contains the literal
`$(date +%s.%N)`, not a baked-in number) — the hook's own shell expands it only
when the hook actually runs. The two lines are 7.627 s apart, which is
inconsistent with both being written at settings-parse or session-teardown
time. Independent cross-check at a layer the hook cannot influence:
`events.log`'s own filesystem mtime (`1784991570.995319`) matches the `STOP`
line's embedded timestamp (`1784991570.994170`) to 1.1 ms.

**Why the fire is pinned to the human submission and not to session start,
config load, replay, or teardown:** exactly one prompt was ever written to the
PTY in the run, at script-relative `t≈8.3s` (the driver script's own progress
line: `[t=8.3] sent prompt`). Exactly one `UserPromptSubmit` line and one
`Stop` line appear in `events.log`, in that order, both timestamped after the
process had been running for several seconds — there is no second, unrelated
`UserPromptSubmit` fire that a session-start or config-load explanation would
need to account for. Independent corroboration from the PTY transcript's own
self-reported elapsed time for that turn — the CLI's live status line showed
`(7s · ↓1 tokens)` and the completed-turn line read `Crunched for 7s` — which
matches the 7.627 s gap measured between the two hook-written timestamps
within the overhead of hook dispatch and process scheduling.

**Positive control (why the result is trustworthy, not merely favorable):**
`Stop` was registered in the same run specifically so an absent
`UserPromptSubmit` could be distinguished from a dead harness (e.g. `claude`
failing to start, the settings file being ignored, or the PTY drive script
losing the child). That design criterion was met: both hooks fired in the
same run, `Stop` confirming the harness was live and hook delivery worked at
all, `UserPromptSubmit` confirming the specific candidate signal.

**Environment note (stated precisely):** the spike ran inside a Claude Code
session, and the child was exec'd via a filtered environment with all ten
`CLAUDE`/`CLAUDECODE`-named variables stripped (`CLAUDECODE`,
`CLAUDE_CODE_BRIDGE_SESSION_ID`, `CLAUDE_CODE_CHILD_SESSION`,
`CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_EXECPATH`,
`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS`, `CLAUDE_CODE_SESSION_ID`,
`CLAUDE_EFFORT`, `CLAUDE_PID`, `CLAUDE_WATCHER_TOKEN`), ruling out "inherited
env suppressed the hook" as a confound. Residual `AI_AGENT=claude-code_2-1-219_agent`
and several `~/.claude/plugins/...` entries in the child's `PATH` remained
unstripped. This does not weaken the result — residual environment could only
have suppressed a fire, never fabricated one, and a fire was observed anyway.

**Scope boundary — what this DOES and does NOT prove (must not drift):**
- PROVEN: the `UserPromptSubmit` event exists in CLI 2.1.220; it fires at
  human turn submission under a real interactive PTY; a `type: "command"` hook
  delivered via a `--settings` FILE PATH executes and writes an observable
  artifact.
- NOT PROVEN: `0600` file permissions specifically — the spike's settings file
  was `0644` (same-uid read is expected to work identically, but was not
  tested at `0600`) — and delivery through the daemon -> helper ->
  `portable-pty` seam with the Phase 1 environment scrub applied. Phase 3 step
  2 (hook config materialization under `agent-profiles/<terminal_id>/`) and
  step 3 (`terminal-notify` subcommand) still own that plumbing and remain
  unverified.
- Correction to a premise recorded elsewhere: the spike did NOT use
  inline-argv JSON delivery. The `## Decisions` note about `claude_cli.rs`
  passing its hook settings JSON inline as argv (around the "Prior art to
  reuse, not reinvent" bullet) describes that module's existing mechanism, not
  this spike — this spike used a `--settings <file>` path throughout, and does
  not validate step 2's delivery plumbing.

Scratchpad paths referenced during this spike
(`turn_start_spike/events.log`, `turn_start_spike/settings.json`,
`turn_start_spike/child_env.txt`, `turn_start_spike/pty_transcript.txt`,
`turn_start_spike.py`, `turn_start_spike_run.log`) are ephemeral and already
gone; they are named here only as historical context, not as evidence this
record depends on.

Verification: the spike result recorded explicitly (including a negative
result); a test that `terminal-notify` resolves a base URL written after the
config file, proving the ephemeral-port path works.

### Result (4ffb22c8) - 2026-07-26

Done. Step 1's gate was already closed and recorded separately (see the
`#### Step 1 spike record` above); this Result covers steps 2-3, completing
the phase as a whole.

The daemon materializes a per-terminal vendor hook settings file at
`agent-profiles/<terminal_id>/settings.json`, mode `0600` on Unix, and passes
only its PATH through helper argv — never the settings content itself. It
also writes `agent-profiles/<terminal_id>/bound-base-url.json` immediately
after binding, on EVERY bind (not only at spawn), so the ephemeral-port
constraint in `## Decisions` holds across `boot_reconcile` re-binds. A new
hidden `ws-dashboard terminal-notify` subcommand
(`#[command(hide = true)]`, `terminal_notify.rs`) resolves that file AT FIRE
TIME — never cached — and POSTs the turn state to the daemon's callback
route. The hook encodes `UserPromptSubmit` -> `working` and `Stop` ->
`ready`, the three-state vocabulary (`working`/`ready`/`idle`) the closed
Phase 3 step-1 spike established as live rather than deferred.

Commits: `7efac08b` (plan), `d86d7094` (implementation), `4ffb22c8` (review
fixes).

**Verification (this machine, 2026-07-26; independently re-run and
confirmed).** `cargo test -p ws-dashboard-daemon --lib` -> 168 passed, 0
failed, 2 ignored, exit 0 (Phase 2 baseline 148, net +20). `cargo test -p
ws-dashboard-daemon --test server` -> 16 passed, 0 failed. `cargo test -p
ws-dashboard-daemon --test terminal_notify` -> 2 passed, 0 failed. `cargo
test -p ws-dashboard-daemon --test terminal_lifetime` -> 4 passed, 0 failed
(unchanged). `cargo test -p ws-dashboard-daemon --test
agent_hook_missing_state_dir` -> 1 passed, 0 failed. `cargo check -p
ws-dashboard-daemon --tests` -> exit 0. `cargo clippy -p ws-dashboard-daemon
--all-targets` -> no new warnings (the only warnings present are pre-existing
ones in unrelated test files, none touching `terminal_notify.rs`,
`agent_hook_config.rs`, or `agent_callback.rs`).

**Non-vacuity.** The hook-ordering assertion (`UserPromptSubmit` firing
before `Stop` yields distinct resolved base URLs) was proven by a
memoization mutation RE-RUN IN ISOLATION (`--test-threads=1`) so the failure
showed this test's own fixture values (`left: "http://127.0.0.1:1111" right:
"http://127.0.0.1:2222"`) rather than a sibling's — the first attempt's
evidence had been cross-test-contaminated by the process-global mutation and
was rejected as insufficient. The `0600` mode assertion on the vendor
settings file was proven by mutating the mode to `0644` and observing the
failure (`left: 420 right: 384`).

**Review outcome.** fit: 0 Critical / 1 Important / 0 Minor - the unbounded
hand-rolled failure log, recorded as finding 3 below. correctness: 0
Critical / 6 Important / 7 Minor. test: 1 Critical / 3 Important / 2 Minor.
All dispositioned findings were fixed in `4ffb22c8`, except one deliberately
routed to a follow-up ticket (below). Findings that carry forward:

1. **A test run was writing into the developer's REAL
   `~/.local/state/ws-dashboard/`.** The new unconditional
   `bound-base-url.json` write made pre-existing in-process `tests/server.rs`
   tests pollute the real state dir — confirmed on disk, not inferred. Fixed
   by scoping those tests to a temp state home; proven by before/after mtimes
   on the real file being byte-identical across a full test run.
2. **`terminal-notify` had no HTTP timeout**, so one hook fire could block an
   interactive agent turn until the vendor's own hook timeout fires — the
   stale-callback-on-a-reused-port case this design anticipates. Now
   `connect_timeout` 750ms, `timeout` 2s.
3. **The failure log was an unbounded hand-rolled appender** in the same
   `logs/` directory as the daemon's own bounded one, under a filename its
   pruner would never reclaim — and in the window before Phase 4, every hook
   fire fails, twice per turn. Now writes through the crate's existing
   daily-rotating appender (`logging::build_file_appender`).
4. **`settings.json` — whose content is an executed command line — fell back
   to the predictable, world-writable `/tmp/agent-profiles/`** when no state
   dir resolved. Now degrades to a hookless spawn instead, proven by a new
   integration test (`agent_hook_missing_state_dir.rs`) that removes every
   state-home variable and asserts the profile dir is absent and the daemon
   warns.
5. **The hook command's POSIX single-quoting was provably wrong on
   Windows** (`cmd.exe` does not treat `'` as a quote character) — and the
   ticket chose this subcommand over `curl` specifically ON
   WINDOWS-PORTABILITY GROUNDS, so the implementation was undermining its own
   rationale. Replaced with a `cfg`-split quoting scheme (POSIX single-quote
   unchanged, Windows double-quote). STATE PLAINLY: the Windows branch is
   plausible but NOT proven — no Windows machine was available this cycle —
   and it wants a Windows spike mirroring the macOS spike step 1 used. Do
   not let this read as verified.
6. **The temp file for `bound-base-url.json` had a fixed name**, so two
   daemons sharing a state home could interleave and publish a torn file;
   the registry precedent it was modelled on avoids this by deriving temp
   names per terminal. Now suffixed per writer (pid + counter + timestamp);
   `agent_hook_config.rs`'s own writer was checked and found not exposed to
   the same hazard (its temp path is already namespaced under a
   per-terminal-id directory).

Deferred to `260726-bug-dashboard-terminal-notify-silent-failure-no-expiry`
(`c69a7ff5`), cross-referenced from there back to this ticket: the
deliberate silent-exit-0 design has no expiry, so after Phase 4 a broken
callback path is indistinguishable from an unfinished turn; the temp-file
create-then-chmod window that matters once a token is written; and the
constraint that Phase 4 must NOT derive `callback.json`'s `baseUrl` by
reading the shared `bound-base-url.json` (that would reinstate the
multi-daemon steal `## Decisions` rejects).

Also recorded: `agent-profiles/<terminal_id>/` is created but never removed
in this phase — Phase 4 owns that GC sweep, per `## Decisions`'s "On-disk
layout" bullet and Phase 4's own bullet requiring the sweep to run strictly
after `boot_reconcile`.

**Deferred / not done.** Phase 4 and all later phases (4-8).

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

### Result (f134aa8a) - 2026-07-26

Done. Commits: `60f74c5d` (plan), `12604d64` (token store), `384b1924`
(route), `00576d6f` (GC sweep), then review-cycle fixes `4952822c`,
`f4ee632c`, `eec756f2`, `f134aa8a`.

A daemon-owned per-terminal token lives at `terminal-tokens/<id>.json`,
created at mode `0600` from the moment it is written rather than chmod'd
after the fact. `POST /api/dashboard/terminals/{terminal_id}/turn-state` is
registered outside `require_owner_auth` with a handler-level constant-time
token check; an unknown terminal id and a wrong token return the identical
401. `boot_reconcile` recovers the token on adopt and rewrites
`callback.json` with the restarted daemon's fresh base URL, so a
re-adopted helper's already-materialized `--callback` argv keeps resolving
a live target. The GC sweep over `agent-profiles/` keys on terminal
liveness and is spawned strictly after the awaited `boot_reconcile`
completes.

**Verification (this machine, 2026-07-26; the lead independently
reproduced the lib figure and the end-to-end test).** `cargo test -p
ws-dashboard-daemon --lib` -> 190 passed, 0 failed, 2 ignored (Phase 3
baseline 168, net +22). `cargo test -p ws-dashboard-daemon --test
terminal_notify_callback_restart` -> 1 passed. `cargo test -p
ws-dashboard-daemon --test terminal_notify_end_to_end` -> 1 passed. `cargo
test -p ws-dashboard-daemon --test terminal_notify` -> 2 passed. `cargo
test -p ws-dashboard-daemon --test server` -> 16 passed. `cargo test -p
ws-dashboard-daemon --test terminal_lifetime` -> 4 passed. `cargo test -p
ws-dashboard-daemon --test routes` -> 170 passed, 2 failed, both
independently confirmed pre-existing at the base commit in a separate
worktree. `cargo check --tests` -> exit 0. Owner auth was ENABLED in every
test, as this phase requires.

**Non-vacuity.** Each guard was proven by mutation and revert: reverting
the pending-registration-set union fails the concurrent-spawn test;
skipping the callback-URL rewrite fails the restart test; racing the sweep
ahead of reconcile fails the ordering test; making delivery a silent no-op
fails the end-to-end test; accepting any known token fails the
cross-terminal test.

**Review outcome.** Four significant findings were dispositioned and
fixed; three Minors were reviewed and accepted without action (recorded
below). Findings that carry forward:

1. **A concurrent-spawn GC race, Critical.** The sweep snapshotted liveness
   BEFORE listing directories, so a terminal whose profile directory
   existed but whose session was not yet inserted into `sessions` — the
   insert happens only after helper spawn AND the IPC handshake — was
   classified an orphan and had its live config and token deleted. This is
   the SAME invariant the boot-ordering guard protects, reappearing in the
   steady-state case where nothing guarded it. Fixed with an explicit
   pending-registration set marked before the directory is created and
   cleared only after the id is visible in `sessions`, so the sweep's
   liveness view is always a superset. A minimum-age guard was considered
   and rejected: it would need a principled upper bound on
   spawn-plus-handshake latency, and there is none.
2. **The GC task leaked.** It was a bare spawn with a discarded handle and
   no self-termination, unlike the daemon's one other tracked task. Now
   aborted on shutdown in both select arms.
3. **The `0600` helper was duplicated across two modules**, justified by a
   precedent that did not actually support it. This is the
   security-sensitive sequence the phase was told to get right, and two
   copies means a future fix lands on one. Now a single shared helper.
4. **The end-to-end delivery assertion was missing**, though reported as
   done. Every new test hand-built the JSON POST rather than driving the
   real `terminal-notify` CLI against the real route. That mattered
   specifically because the CLI is silent by design with no expiry, so a
   broken delivery path leaves no signal anywhere and only a test can
   catch it. Now covered by a test that interposes a transparent TCP relay
   between the real CLI and the real daemon, asserting the real 204 the
   route returns — with no production change made to create that
   observability.

**Minors accepted without action.** No narrow unit test of the adopt-arm
token recovery; untested unreadable-directory branches in the sweep; no
concurrent same-terminal POST test. Recorded so a later reader knows these
were seen and judged, not missed.

**Deferred / not done.** Phase 5 and all later phases.

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

### Result (79f21023) - 2026-07-26

Done. Commits: `4b89a13d` (plan), `c9582bfb` (contract-first spec),
`8ed291c6` (daemon hub + SSE route pair + turn-state wiring), `d5f858c9`
(frontend subscription), then review fixes `9336c8ba` and `79f21023`.

An `AttentionHub` holds a broadcast sender plus a per-terminal snapshot map,
following `DocumentEventHub` rather than inventing a second pattern. The
SSE route pair — local plus a forwarding sibling — lives inside the
owner-auth protected router and is modelled on the existing
`server_scoped_work_root_activity_events`/document-events precedent. The
Phase 4 turn-state handler, which previously accepted and discarded, now
publishes to the hub. The browser opens one `EventSource` per eligible
linked server rather than one global subscription, since a terminal on a
linked server runs under that remote daemon. The stream carries an initial
snapshot on connect and ends the stream on broadcast lag so the client
reconnects and re-reads the snapshot. Nothing renders in this phase — the
state lands in `App.tsx`'s new `attentionByKey` where Phase 6 can consume
it; confirmed from the diff (`App.tsx` diffs to +140 lines of state/effect
wiring only, no JSX/return changes, no `frontend/e2e/` changes), so no
browser gate applies to this phase.

**Verification (this machine, 2026-07-26).** `cargo test -p
ws-dashboard-daemon --lib > out 2>&1; echo $?` → 201 passed, 0 failed, 2
ignored, exit 0 (Phase 4 baseline was 190, net +11). `cargo test -p
ws-dashboard-daemon --test routes > out 2>&1; echo $?` → 174 passed, 2
failed, exit 101 — the two failures are the known pre-existing pair
`dashboard_resources_refresh_prunes_workspace_without_available_work_roots`
and `online_missing_work_root_returns_bounded_unavailable_without_path_leak`.
`cargo check -p ws-dashboard-daemon --tests > out 2>&1; echo $?` → exit 0,
no diagnostics.

**Review outcome.** Three significant findings were dispositioned and
fixed in `9336c8ba`/`79f21023`. Findings that carry forward:

1. **A fifth session-removal path leaked attention entries.** Four choke
   points were wired to forget a terminal's attention state; `insert`'s own
   eviction `retain` was not, so a helper that exited without a browser
   `DELETE` kept its last state in every future snapshot for the daemon's
   lifetime — directly contradicting the spec sentence this phase had just
   landed contract-first. Fixed by making the code true rather than
   weakening the sentence, with a test watching that specific path. The
   callback-token half of the same gap is pre-existing Phase 4 debt and was
   deliberately left alone.
2. **The map write and the broadcast were not one critical section**, so
   two concurrent posts for one terminal could publish in the opposite
   order to the stored state — subscribers pinned on a stale `working`
   while the snapshot said `ready`, which is the never-clearing spinner
   this feature exists to prevent. Fixed by making them a single critical
   section. The evidence is unusually strong: reverting the fix and
   running an invariant test fifty times reproduced the failure on run 42
   from real thread scheduling, not from a staged delay.
3. **The browser had no `onerror`**, so a 401 or a forwarder 502 killed a
   stream permanently while the dedup guard prevented replacement —
   silently defeating the lag-to-reconnect-to-snapshot resync that is this
   design's only resync path. Fixed by dropping a permanently-`CLOSED`
   source so the next poll tick recreates it, bounded by that existing 5s
   tick rather than a new timer.

**Non-vacuity.** Each guard was proven by mutation and revert: removing the
`forget` call fails the eviction test; the pre-fix lock ordering fails the
concurrency invariant; forcing the replace-predicate to false fails the
frontend test.

**Known gap, recorded honestly.** The frontend suite covers the
`readyState` decision as a pure predicate but NOT the surrounding
`EventSource` open/close/dedup wiring end to end, because this repo has no
jsdom harness. This is the same subscription-lifecycle gap the test review
raised, accepted as Minor and inherited by Phase 6 rather than widened
here.

**Deferred / not done.** Phase 6 and all later phases.

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
