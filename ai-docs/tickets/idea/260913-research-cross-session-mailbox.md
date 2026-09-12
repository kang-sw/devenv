---
title: Cross-session mailbox for session-to-session messaging and remote agent-loop control
related:
  260909-epic-ws-worker-interpreter-refoundation: constraint — wake design must honor Cross-Child Decision 16 (no blocking in a tool call, no polling; harness owns parent→child waiting)
  260605-research-ws-native-subagent-pivot: context — native subagents share the lead's single multiplexed ws-mcp process; session-key auth model
  260504-feat-ws-mcp-hook-driven-interrupt: prior-art — retired hook-driven interrupt/outbox mailbox (delivered into a running subprocess turn via PostToolUse)
---

# Cross-session mailbox for session-to-session messaging and remote agent-loop control

## Background

The user wants a mailbox primitive in ws MCP so that independent harness
sessions in one project can exchange messages, and so that one session can
drive another session's agent loop out-of-band.

Motivating scenario: run two harnesses at once — a Claude session for
discussion / data-driven work, and a Codex session as the primary `lead-run`
executor. The user talks only to Claude and remote-controls the executor:
"send mail to the executor to run ticket X." The executor must therefore be
able to wait for and act on inbound control messages while otherwise idle.

Endpoint granularity is **harness-session to harness-session** (inter-session).
Addressing individual subagents inside one harness is intra-session and belongs
to the harness's own native messaging (e.g. Claude `SendMessage`), not to this
mailbox.

## Prior art and why this is a revival, not greenfield

`260504-feat-ws-mcp-hook-driven-interrupt` already built essentially this:
`agents.interrupt` durable messages delivered into a running Codex/Claude
subprocess turn via `PostToolUse` hooks, with a `WS_AGENT_OUTBOX` env for the
Claude adapter and "hook-triggered mailbox checks that drain queued outbox
content." It was **fully retired** with the "mercenary" (ws-spawned
subprocess) machinery.

The decisive point is *why* it died: its scope was lead ↔ its-own-spawned
subprocess, and the whole subprocess-ownership model was abandoned in favor of
harness-native delegation. The **hook-driven delivery** was not the problem;
the **ws-owns-the-process premise** was. This ticket revives the delivery
mechanism decoupled from that retired premise, now for harness-native peer
sessions. Recording this here so a future reader does not re-litigate "we
already deleted this."

## Architectural framing: two axes

The refoundation epic deliberately keeps ws out of the wait/notify business —
Cross-Child Decision 16/20: the worker "neither blocks in a tool call nor
polls," and cross-session *parent→child* waiting is delegated to the harness
(`Agent`/`SendMessage`). This mailbox does not reverse that. It splits by axis:

- **parent↔child axis** stays owned by the harness (native subagent spawn /
  SendMessage).
- **peer axis** — independent sessions with no spawn relationship, across
  worktrees/clones on a machine — is the genuine gap the harness does not
  cover, and is the sole reason this mailbox exists.

## Delivery model: pull-based spine + best-effort push

MCP is request/response; the server cannot push to an idle agent. Delivery is
layered, all best-effort where noted:

- **Response piggyback (host-neutral spine).** A central ws-MCP dispatch
  wrapper appends an "unread N: from X → call `mailbox.recv`" badge to the
  owner session's tool responses. One interception point, not per-tool wiring.
  This makes polling free but is still fundamentally pull: it only fires when
  the owner makes a ws call.
- **Model-driven wake (the active path).** Genuine wake of an idle executor
  requires something to block until mail arrives. That blocking lives in a CLI
  process, never an MCP tool (see invariants). The model launches
  `ws-mcp mailbox wait --timeout` through the harness's background-task
  mechanism; its exit re-invokes the agent. Only harness-owned background tasks
  produce a wakeup — the ws-mcp server cannot route one, so arming the wait is
  the one obligation that cannot be made implicit.
- **Harness hook adapters (best-effort).** Codex `Stop` /`PostToolUse` hooks,
  Claude hooks, pi native active push. These are adapter/fallback surfaces per
  `shipped-surface-boundary.md`, never the host-neutral contract.

Truly idle interactive sessions (user away) remain best-effort: a background
task's completion may not fire until the user next interacts.

## Identity, opt-in, and self-registration

Identity is a launch-time env var `WS_MAILBOX=slug@scope`, which the ws-mcp
server inherits at spawn (the harness spawns the stdio server as its child).
The server reads it at startup and self-registers presence into the shared
store. This dodges the hook-identity problem entirely and gives free
containment: the mailbox name is fixed by the user at launch, so an agent
cannot pick an arbitrary name at runtime or impersonate a peer.

Storage is nearly free: the ws session-key store is already a machine-global
flat directory (`<cache-root>/keys/`, no project/worktree segment), so all
sessions on a machine co-locate; only the record's `Root` distinguishes them.
The proposed `clone|worktree|machine` scopes map onto existing wsnote layer
roots (worktree cache / per-project shared / `~/.ws`). Cross-machine is out of
scope (records carry no machine identity and no non-tracked layer crosses
machines by default).

## Owner binding and misfire prevention

Native subagents share the lead's single multiplexed ws-mcp process, which
carries one inherited `WS_MAILBOX`. Env alone therefore cannot tell the lead
from a subagent — misfire (a subagent draining the owner's mail, or a leaf
agent getting arm-reminders) is not automatically prevented. It is closed at
two layers using the mandatory per-call `session_key`:

- **Server layer:** `recv` / `send` / piggyback are hard-gated on
  `caller_session_key == owner_key`. Other keys sharing the server are inert
  w.r.t. the mailbox.
- **Owner binding by `ferrule`, not bind-once:** the top lead key is lost
  fairly often, so `ferrule` re-login (the `unknown_session → re-login`
  recovery path, invoked via the `obsidian-latch` bootstrap) happens
  frequently. Rebind the owner on each `ferrule` call where `WS_MAILBOX` is set
  **and `parent_session_key` is null**. This self-heals on every re-login and
  rides the existing recovery path. Worker/delegate `ferrule` calls carry a
  parent and so never steal ownership — critically, the trigger is
  "parent-less ferrule," NOT "any lead-capability key," because
  worker mini-leads are minted with lead-capability child keys.
- **Address = name, owner = internal pointer:** peers send to the stable
  `slug@scope` name; the owner `session_key` is a rebindable internal pointer.
  Senders never see top-key churn, and queued mail survives key loss (drained
  by whoever currently owns the name). Queue / presence / listening markers are
  all keyed by the name.
- **Hook layer:** the arm-reminder must fire only in the owner/root-turn
  context, keyed off the harness's structural main-vs-subagent hook distinction
  (Claude: `Stop`, never `SubagentStop`). Subagent stop/tool events must not
  emit it. This is the hook-layer counterpart of the server-layer owner gate.

## Probe plan (must precede any actionable promotion)

The whole executor path rests on harness hook behavior that is currently
unverified. Repo notes flag "Codex hook feedback semantics" as an open item,
and the codex-integration manual's hook probe is dated 2026-07-08. Before this
design becomes actionable, empirically re-verify on the current CLIs:

1. Codex `Stop` + `decision: block` keep-alive and `PostToolUse` `exit 2`
   feedback injection still behave as the manual records.
2. Whether Codex subagent turns fire `Stop`, and what classifier (agent name?)
   distinguishes a main-turn stop from a subagent stop.
3. Claude hook payload contents — enough to confirm `Stop` vs `SubagentStop`
   structural separation and any owner-context signal.
4. That a harness background task's completion re-invokes the agent (Claude
   `run_in_background` → task notification confirmed; Codex TBD).

## Probe results (2026-09-13, Codex 0.154.0 + Claude docs)

Ran the probe plan in an isolated `codex exec`
(`--ignore-user-config --ephemeral`, inline `-c` hooks, no ws plugin; the
running executor untouched) and confirmed the Claude payload from docs.

- **(1) Codex hooks fire + `Stop` `decision:block` keep-alive — confirmed.**
  Both `Stop` and `PostToolUse` fire on 0.154.0. A `Stop` hook returning
  `{"decision":"block","reason":"<instruction>"}` re-invokes the model with the
  reason as an instruction (verified: the model ran the injected command), then
  concludes; the re-entry `Stop` carries `stop_hook_active: true` as a loop
  guard. This is the turn-boundary wake mechanism (Decision 6).
- **(1b) `PostToolUse` output never reaches the Codex model on 0.154.0.** The
  hook fires (side effects work), but nothing it emits reaches the model: not
  `exit 2` + stderr, not JSON `decision:block` + `reason`, not
  `hookSpecificOutput.additionalContext`. Tested even as a *passive* note
  (exit 0, `additionalContext` asking the model to append a codeword to its
  final message) — the codeword never surfaced, so the context did not reach the
  model at all, not merely "reached but ignored." Note the asymmetry: `Stop`'s
  `reason` does reach the model (see (1)), `PostToolUse`'s does not. So a Codex
  `PostToolUse` hook cannot notify or soft-steer the model mid-turn at all — it
  is side-effect-only (write a marker, check mail). (This reverses the
  codex-integration manual's dated 2026-05-04 `exit 2` claim, corrected there.)
  **Design consequence: mid-turn awareness on Codex must ride the
  response-piggyback spine — the unread-badge appended to ws MCP tool responses,
  which the model always reads — not hooks. Hooks contribute only the `Stop`
  boundary wake; `PostToolUse` is demoted to side-effect (marker) use on Codex.**
  This probe independently validates the piggyback layer's necessity.
- **(2) Codex main-vs-subagent classifier — no payload signal.** The `Stop`
  payload has no `agent_id`/`agent_type`/agent-name field, so a Codex hook
  cannot structurally distinguish a main turn from a subagent turn from the
  payload (contrast Claude). Subagent spawn in `--ephemeral` `codex exec` fails
  (needs the shared app-server daemon), so whether Codex subagent turns fire
  hooks at all is still open (needs a daemon-backed probe). Mitigation stands:
  the server-layer `caller == owner` gate (Decision 3) is the real misfire
  guard; the Codex hook-layer guard (Decision 9) is best-effort and must bake
  owner identity into the hook command args.
- **(3) Claude `Stop` vs `SubagentStop` — confirmed structurally separable.**
  Docs (code.claude.com/docs/en/hooks.md): `Stop` carries
  `agent_id`/`agent_type` only in a subagent context, `SubagentStop` always; so
  a single Claude `Stop` hook can gate on `hook_event_name == "Stop"` &&
  `agent_id` absent → root lead turn. Decision 9's mechanism holds on Claude.
- **(4) Background-task wake:** Claude `run_in_background` → task-notification
  re-invoke confirmed (observed live this session). Codex `exec` is one-shot
  headless with no equivalent re-invoke; its wake equivalent is the `Stop`
  `decision:block` drain-loop from (1), so this collapses into (1) for Codex.
- **New: Codex 0.154.0 gates hooks behind persisted trust**
  (`--dangerously-bypass-hook-trust` bypasses for automation). A real adapter
  must persist hook trust; a deployment step the design must account for.

Net: the turn-boundary wake path (Decision 6) is empirically solid on both
harnesses; mid-turn preemption of a busy Codex executor is not achievable via
hooks on 0.154.0 and is out of scope (the primary "idle executor waits for
'run ticket X'" scenario only needs turn-boundary wake).

## Outcome Ledger

### Verified Findings

- Prior art `260504-feat-ws-mcp-hook-driven-interrupt` implemented a
  hook-driven interrupt/outbox mailbox and is fully retired with the mercenary
  surface; grepping current Go source for `agents.interrupt` / mercenary
  returns nothing.
- Native subagents share the lead's single ws-mcp process over one multiplexed
  stdio connection (`260605-research-ws-native-subagent-pivot`). A blocking
  MCP tool would therefore freeze the shared channel for the lead and all
  subagents.
- The ws session-key store is a machine-global flat directory
  (`<cache-root>/keys/`) with no project/worktree segment; all sessions on a
  machine co-locate, distinguished only by each record's `Root`. Not shared
  across machines by default.
- Note layers already provide the storage roots the proposed scopes need:
  `machine` (`~/.ws`), `worktree` (per-worktree cache), `clone` (per-project
  shared dir); only the git-tracked `repo` layer crosses machines, and only via
  git, not live.
- No existing ws mechanism pushes/notifies/signals one session to another;
  `session.children`/`session.note` are pull-only over the parent chain.
  Refoundation Decision 16/20 keeps ws out of wait/notify and delegates
  parent→child waiting to the harness.
- `ferrule` is the sole session-mint primitive; it takes an optional
  `parent_session_key`, so a parent-less mint is distinguishable from a
  worker/delegate mint.
- Codex exposes `Stop` (turn-conclude, `decision: block` drain-loop
  alternative) and `PostToolUse` (`exit 2` + stderr injects feedback and
  continues the turn) hooks per the codex-integration manual — dated, and
  flagged for re-verification.

### Confirmed Decisions

1. **opt-in / identity:** `WS_MAILBOX=slug@scope` launch-time env. Absent →
   mailbox fully inert (no presence, no piggyback, zero overhead).
2. **self-registration:** the server self-registers presence from the env at
   startup, keyed by the mailbox name, with liveness (heartbeat/pidfile) and
   duplicate-live-name detection. No agent-facing `register` call.
3. **owner binding:** rebind owner on each `ferrule` call where `WS_MAILBOX` is
   set and `parent_session_key == null`; worker/delegate ferrule calls (parent
   present) never bind. `recv`/`send`/piggyback are hard-gated on
   `caller == owner` at the server. Trigger is parent-less ferrule, never "any
   lead-capability key."
4. **address = name, owner = internal pointer:** peers address the stable
   `slug@scope` name; queue/presence/listening markers are name-keyed; the
   owner `session_key` is a rebindable pointer, so key churn never loses mail
   or breaks senders.
5. **MCP tools:** `send`, `recv`, `lookup-peers`, all non-blocking. No blocking
   `wait` on the MCP surface.
6. **wake:** blocking lives only in the CLI `ws-mcp mailbox wait --timeout`,
   launched by the model as a harness background task (the one non-implicit
   model obligation), with a listening marker. **The wait is level-triggered,
   not edge-triggered: on startup it synchronously checks the durable
   name-keyed queue for unread mail and returns immediately if any exists,
   blocking only when the mailbox is empty.** Waiting on a bare arrival event
   would lose mail deposited in the arm/drain gap (lost wakeup); since the store
   is durable (Decision 4), the wait just reads it on startup. The `Stop` hook
   is an arm-reminder / backstop safety net only. Wake coverage is bounded: an
   *idle* session's armed wait fires promptly (immediately if mail is already
   present, else on arrival); a *busy* session (mid-turn) learns of mail via the
   response-piggyback badge on its ws calls and, at worst, at the next `Stop`
   boundary — a one-turn latency ceiling. On Claude a background-task completion
   can additionally surface mid-turn (observed this session: task-completion
   notifications arrive within the running turn); on Codex it lands at the `Stop`
   boundary. Truly idle interactive sessions with the user away remain
   best-effort (a background task's completion may not fire until the user next
   interacts).
7. **harness adapters:** Codex `Stop`/`PostToolUse` hooks, Claude hooks, pi
   native push — all adapter/best-effort per the shipped-surface boundary,
   never the host-neutral contract.
8. **endpoint granularity:** one endpoint per harness root session per
   `WS_MAILBOX`; sub-agent-level addressing is intra-session and out of scope
   (owned by native harness messaging).
9. **hook-layer misfire guard:** the arm-reminder fires only in the
   owner/root-turn context, keyed off the harness's structural main-vs-subagent
   hook distinction (Claude: `Stop`, never `SubagentStop`); subagent stop/tool
   events must not emit it.

### Proposals

- Consumption/ack semantics: piggyback shows only an "unread N" badge; actual
  body consumption is via explicit `recv`. Whether `recv` is
  drain-on-read vs requires a separate ack, and dedup-cursor handling, is not
  yet settled.
- A new `lead-use-mailbox` skill to guide the user/lead through registering
  (launching with `WS_MAILBOX`), arming the wait, and the send/recv flow.

### Open Questions

- **Resolved by the 2026-09-13 probe:** Codex hook re-verification (item 1),
  Claude `Stop` vs `SubagentStop` payload (item 3), and background-task wake
  (item 4) — see `## Probe results`. Item 1 also produced a new finding:
  `PostToolUse` no longer steers the model on 0.154.0, so wake is turn-boundary
  (`Stop`) only and mid-turn preemption of a busy Codex executor is dropped.
- **Still open:** whether Codex subagent turns fire hooks at all, and under what
  event name — the isolated `exec` probe could not spawn a subagent (needs the
  shared app-server daemon). Resolve with a daemon-backed probe before relying
  on any Codex hook-layer subagent guard.
- Codex has no main-vs-subagent signal in the hook payload, so owner-context
  gating on Codex must bake identity into the hook command args (the server-layer
  `caller == owner` gate remains the real guard). Direction confirmed; adapter
  wiring unspecified.

### Rejected Alternatives

- **Blocking `mailbox.wait` as an MCP tool.** Rejected: it would hold the
  shared multiplexed stdio channel and freeze ws for the lead and all
  subagents; it also re-crosses the Decision 16 line. Blocking belongs in the
  CLI process instead.
- **Bind-once owner binding.** Rejected: the top lead key is lost frequently,
  so a one-time bind goes stale after the first re-login. Rebind at `ferrule`
  instead.
- **Dynamic per-call `export MAILBOX_...` to coordinate with hooks.** Rejected:
  shell state is ephemeral (AGENTS.md Architecture Rule 6) and env flows only
  parent→child at spawn, so a value exported inside a tool call never reaches a
  later hook process. Use launch-time env for the static identity and the
  shared file store for dynamic state.
- **`Stop`-hook process-park keep-alive.** Rejected: parking the turn inside
  the hook turns the harness into a mailbox-only zombie and needs messy env
  control. Keep the model as the active-wait owner; the hook only reminds.
- **Explicit `register` MCP call.** Rejected: superseded by env + server
  self-registration (invariant 2), which also removes "agent forgot to
  register" drift.
