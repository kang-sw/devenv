---
title: "Pi adapter: owner audit window for subagent conversations (`/audit`) and owner steering with last-writer settle ownership"
parent: 260908-epic-ws-pi-subagent-conversation-view
related:
  260908-feat-ws-pi-conversation-view-component: prerequisite — Phase 1 needs its Phase 1 (component, `view` mode, liveness input); Phase 2 needs its Phase 2 (ask bound to the same component so ownership has one implementation)
  260904-feat-ws-pi-side-thread-fork-question-surface: `overlayAttached` settle/nudge suppression and the `/done` semantics this ticket replaces with last-writer ownership and the modal
  260905-feat-ws-pi-push-only-child-reports: the six push families; `ws-agent-settled` / `ws-agent-advisory` become owner toasts while a child is owner-held
  260905-feat-ws-pi-agent-alias-park-and-registry-cap: park-at-idle, alias-reuse rejection and cap eviction — owner-held joins thread-bound as the exempt class
  260905-feat-ws-pi-live-agent-widget: rows gain the owner-held flag and the running / idle-awaiting-owner split
  260903-feat-ws-pi-subagent-rpc-ux: RPC registry, `ws-agent-transcript` path accessor, `prompt()`/`steer()` mapping
spec:
  - pi-adapter-runtime
related-mental-model:
  - plugin-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 40ba2366bd9fd4e7
sage-review-design-reviewed: 40ba2366bd9fd4e7
---

# Pi adapter: owner audit window for subagent conversations (`/audit`) and owner steering with last-writer settle ownership

## Background

Child B of `260908-epic-ws-pi-subagent-conversation-view`. The owner wants to
watch what a subagent is doing — while it streams and after it settles — and
to step in and steer it when the lead's delegation is going wrong, without
that conversation ever entering the lead's model context and without waking
the lead for turns the owner drove.

Today the owner has: the live-agent widget rows (name · role · state ·
elapsed), pushed reports rendered in the lead transcript, `ws-agent-transcript`
(a path accessor the *lead model* would have to read — which is exactly the
context pollution to avoid), and the `/answer` overlay, which reaches only a
thread respondent and only for the question exchange. There is no way to open
an arbitrary worker's conversation, and ownership exists only for threads:
`RpcAgentRecord.threadBound` (thread lifetime, cleared by `/done` /
`ws-resolve` / the fork's own `final`) keys the settle-push suppression, the
anti-bleed suspension and the park exemption, so a plain worker or execute
worker can never be owner-held. (`overlayAttached` is a dead field today —
set and cleared in `ask.ts`, read nowhere.)

Child A delivers the shared `ConversationViewComponent` (`view` /
`interactive` modes, `ConversationChannel` with a 3-state `liveness()`). This
ticket puts the audit window and the steering rule on it.

## Decisions

### Audit window (Phase 1)

- **`/audit` command.** `pi.registerCommand("audit", …)` opens a picker of the
  registry's children — running and dormant — one row per child in the widget's
  naming (alias > title > short uuid) and three-state ordering (awaiting owner,
  then awaiting approval, then running by elapsed), plus a fourth,
  picker-only tier the widget does not have: dormant children by last
  activity, most recent first (the widget never rows a dormant child);
  `/audit <id-or-alias>` opens one directly. The picker is itself a focused
  `ctx.ui.custom` modal (arrow keys move, Enter opens, Esc cancels) and is
  also bound to a keyboard shortcut (`pi.registerShortcut`, key chosen at
  implementation like the `/answer` reopen shortcut) so the owner reaches a
  child in two keystrokes without typing a command. Selection opens the
  viewer as a `ctx.ui.custom` overlay with the ask overlay's geometry. Never auto-pops;
  the existing one-overlay-at-a-time rule applies (opening a second closes the
  first, the child is unaffected).
- **Source of the conversation.** History comes from the child's own Pi
  session file (`record.sessionPath`, the same path `ws-agent-transcript`
  returns), parsed by the adapter into `ConversationItem`s (assistant /
  tool-call / tool-result, and user-side entries); the live tail comes from
  the child's RPC event stream (`text_delta`, `tool_execution_start`/end,
  `agent_start`, `agent_settled`) through the same `ConversationChannel`
  shape the ask binding uses. **Attribution comes from the adapter, not the
  file**: every send reaches the child as raw text through `promptAgent`, so
  the session file cannot tell an owner line from a lead send. The record
  keeps `ownerSends: Array<{ text: string; at: number }>` (appended by the
  viewer's send path, persisted in the shutdown sidecar with the record); a
  user-side entry that matches a logged owner send (text, in order) renders
  as `user`, every other user-side entry (spawn prompt, `ws-agent-send`,
  nudge, `finish` handoff) as `lead-message`. This record-based source is the
  single source for both `/audit` and `/answer`; the ask thread's own
  `transcript` persistence is retired in Phase 2 when the two share one
  binding. The adapter reads the transcript; the lead model never does. A
  dormant child shows its history with `liveness() === "settled"` and no
  process is resumed by opening the viewer.
- **`view` mode.** Read-only; follows the stream; Esc closes the viewer
  directly (no modal — nothing about ownership is at stake). Reopen shows
  the conversation so far. Nothing in the viewer changes the child's record.

### Owner steering (Phase 2)

- **Entering interactive mode from the viewer.** The `/audit` viewer opens
  in `view` mode; pressing Enter there calls the component's
  `setMode("interactive")` (child A: mode may be raised from `view` to
  `interactive` on the same instance, never lowered) — the same component,
  history, scroll position, expand state and event subscription survive; only
  the editor appears. The switch alone changes nothing on the record —
  ownership flips only on the first send. `/answer` opens `interactive`
  from the start, as today. A child whose record `sendToAgent` refuses (a
  one-shot explore record, per `260906-feat-ws-pi-lead-explore-as-async-rpc-child`)
  opens in `view` mode only: Enter shows a one-line hint and does not
  switch. Rejected: a separate `/steer <id>` command (one more verb for the
  same window); rebuilding the component on switch (resets the view).
- **`lastWriter` on the record.** `RpcAgentRecord.lastWriter?: "lead" |
  "owner"` (absent = lead). Set to `owner` by a send from the viewer; set to
  `lead` by every lead-side prompt (`ws-agent-send`, the fork nudge, spawn,
  the `finish` handoff). The dead `overlayAttached` field is removed.
  `threadBound` keeps its thread-lifetime meaning; every suppression and
  exemption keyed on it today widens to `threadBound || lastWriter === "owner"`.
  Ownership flips on a **send**, not on opening the viewer or switching mode.
- **Liveness.** `ConversationChannel.liveness()` for any child:
  `running` when `record.running`; else `idle-awaiting-owner` when
  `lastWriter === "owner"`; else `settled`. The widget row state gains the
  same split for owner-held rows.
- **Settle routing while owner-held** (`lastWriter === "owner"`):
  `ws-agent-settled` and `ws-agent-advisory` are not pushed to the lead — the
  settle becomes a `ctx.ui.notify` toast naming the child; the fork anti-bleed
  loop (`wireAntiBleedLoop`, today suspended while `threadBound`) is suspended
  for the same span and re-arms when ownership returns to the lead. `ws-agent-report` (final and progress),
  `ws-agent-approval`, `ws-agent-question` and `ws-agent-orphaned` are
  unchanged. Owner-time settles are never replayed to the lead.
- **Owner-held is exempt like thread-bound** on: park-at-idle, the fan-in
  presence/count (`computeFanIn` excludes it), alias-reuse rejection, cap
  eviction. The shutdown sidecar persists it like any other child (it has no
  `.ws-threads.json` counterpart) **including `lastWriter` and
  `ownerSends`**, so an owner-held child is still owner-held after `/reload`
  or a lead restart. `ws-agent-list` rows carry `owner_held: true`; widget
  rows show the flag.
- **The modal.** In `interactive` mode Esc opens
  `[ hold ] [ finish ] [ interrupt ]` with an `Esc: cancel` hint line; Esc
  again closes the modal; `hold` is the default highlight.
  - `hold`: close the view; ownership unchanged.
  - `finish`: close the view and, **only if `lastWriter === "owner"`**, send
    the child one lead-attributed handoff message through `sendToAgent`'s
    existing branches (`followUp` into a running run, `prompt` to an idle or
    dormant child) and set `lastWriter = "lead"`. Settle routing reads
    `lastWriter` at settle time: a `followUp` handoff joins the in-flight run
    and that run ends only after the handoff turn is processed, so its one
    settle carries the child's reply to the handoff and correctly takes the
    lead path; no second field is needed. `finish` on a child the owner
    never wrote to just closes the view (no handoff, no turn consumed). On a
    `lead-ask` discussion thread `finish` is `/done` (summary turn,
    `ws-thread-summary` injection, no generic handoff); on a `fork-raised`
    thread it follows the general rule (handoff only if owner-held) and the
    fork's own `final` still reaches the lead as a report.
  - `interrupt`: abort the child's current turn only — never a stop/park;
    the view stays open; ownership unchanged; disabled (greyed, no-op) when
    the child is not `running`. Phase 2 first verifies that pi-coding-agent's
    RPC command set (`RpcCommand` in `dist/modes/rpc/rpc-types.d.ts` /
    `RpcClient`) exposes an abort; if it does not, the verb ships permanently
    disabled with a `not supported by this Pi` hint — a stop-and-resume
    substitute is rejected because it parks the child and loses the run.
  - `/done` typed in the editor remains an alias of `finish`.
- **Ctrl+C** is swallowed in the viewer and the modal in both modes.
- **Rejected.** Releasing ownership on view close (the `overlayAttached`
  model this replaces); replaying deferred settles to the lead at `finish`;
  an Esc-inert modal; Ctrl+C as a modal key; routing owner toasts through
  `pi.sendMessage` (enters model context).

## Constraints

- Golden rule: `agents-plugin-tool/` untouched; `session_children` (ws-mcp)
  is not changed.
- Human-only surfaces only: `ctx.ui.custom`, `ctx.ui.notify`,
  `setWidget`/`setStatus`, tool-result `details`. No viewer content, toast
  or transcript excerpt through `pi.sendMessage`.
- The viewer never resumes a dormant child on its own; only a send does
  (through `sendToAgent`'s existing dormant branch).
- Lead-only, TUI-only: nothing is registered in child processes or headless
  leads (`ctx.mode !== "tui"`).
- Ask behaviour that is not about ownership (thread registry, persistence,
  `/answer`, `/thread`, injection) is unchanged.

## Prior Art

- `ask.ts`: `openThread` (overlay lifetime, `activeOverlay` token,
  `overlayAttached` set/clear in `finally`), `createForkChannel`,
  `closeThreadOnDone` origin routing.
- `spawner.ts`: `pushToLead`, settle handling and park, `computeFanIn`,
  `sendToAgent` dormant branch, `ws-agent-transcript`.
- `agent-sidecar.ts`: `captureOrphans` (the thread-bound skip).
- `fork.ts`: `wireAntiBleedLoop` reading `threadBound` (already widened off
  `overlayAttached` by `260905-…-alias-park`).
- `agent-widget.ts`: `buildAgentRows`, `AgentRowState`.
- `260904` Phase 2 verification: the two-tier agent-driven TUI loop (unit
  `render(width)` tests; tmux probe on an isolated socket
  `tmux -L ws-probe-<pid>`, never the default server).

## Spec Impact

`pi-adapter-runtime`:

- New anchor: audit window — `/audit`, picker, transcript-plus-event source,
  view mode, never auto-pops, human-only.
- New anchor: owner steering — `lastWriter`, liveness derivation, settle
  routing by family, owner-held exemptions and flags, the modal contract,
  `finish` handoff, Ctrl+C.
- `{#260905-pi-side-thread-owner-question-surface}`: replace the
  "while an owner overlay is attached … re-arms the moment the overlay
  detaches" suppression sentence (already stale — the code keys on
  `threadBound`) and the "Esc closes the view only" bullet with the ownership
  rule and the modal; `/done` = `finish` alias.
- `{#260903-pi-spawner-completion-gating}` and
  `{#260904-pi-report-to-lead-channel}`: owner-held joins thread-bound in the
  park exemption and the fan-in set; settle push routed by `lastWriter`.
- `{#260905-pi-live-agent-widget}`: owner-held flag and the
  running / idle-awaiting-owner row split.

## Phases

### Phase 1: `/audit` picker and read-only viewer

Depends on `260908-feat-ws-pi-conversation-view-component` Phase 1.

Add `agents-plugin-pi/src/audit.ts`: the `/audit` command, the picker modal
and its shortcut, the
session-file parser to `ConversationItem`s, the `ConversationChannel` over a
registry record (history + live events; `liveness()` = `running` /
`settled` only in this phase), and the `view`-mode overlay. Register from
`index.ts` on `session_start` for TUI leads only.

Tests: parser maps assistant / tool-call / tool-result entries and user-side
entries (all `lead-message` in this phase — no owner sends exist yet) from a
fixture session file; picker rows follow the widget naming and
ordering; the shortcut opens the same picker; opening a dormant child
resumes nothing; live events append to the
tail; Esc closes without a modal; Enter in `view` mode switches the overlay to
`interactive` without touching the record (the send path itself is Phase 2);
the second `/audit` closes the first overlay. Live check (owner-run, isolated tmux socket): `/audit` a worker
mid-task, watch a tool call appear and expand it, Esc, reopen, and confirm
the lead transcript received nothing from the viewer.

### Result (33ae460e) - 2026-09-09

Added `agents-plugin-pi/src/audit.ts`: `parseSessionFile` (best-effort JSONL
read — missing/unreadable file or malformed line yields `[]`/skips, never
throws — mapping `assistant` text+`toolCall` blocks in order, `toolResult` via
the shared `toolResultContentText`, and every user-side entry as `lead-message`
per the Phase-1 no-owner-sends contract; non-chat entry types and non-chat
message roles are skipped), `createAuditChannel` (the `ask.ts` `createForkChannel`
shape minus `send`, `liveness()` delegating to `resolveChildLiveness`; a dormant
record with `client === undefined` attaches no listener, so opening it resumes
nothing), `buildAuditPickerItems` (the three live tiers reusing
`classifyRegistryRowState` + `rowName` ordering plus a fourth picker-only dormant
tier by `lastActivityAt` descending), `shouldRegisterAudit` (true only for a true
lead `readSpawnRole(process.env) === undefined` and `mode === "tui"` — stricter
than `isLeadOrFork`, so no fork/worker/explore/headless registration), and
`openPicker`/`openViewer` (a `ctx.ui.custom` `SelectList` picker and the
`view`-mode `ConversationViewComponent` overlay with `ask.ts`'s geometry and a
module-level active-overlay singleton so a second `/audit` closes the first; Esc
closes directly with no modal, Enter raises to `interactive` via `setMode`
without mutating the record; no `pi.sendMessage`/no send path). Registered from
`index.ts` on `session_start`. Supporting behavior-preserving extractions:
`spawner.ts` `lastActivityAt`, `agent-widget.ts` `rowName`/
`classifyRegistryRowState` exports, `conversation-view.ts` `toolResultContentText`
export, `pi-tui.ts` `SelectList`/`SelectItem` re-export.

Deviations: `openPicker`/`openViewer` dropped the plan's descriptive
`sessionCtx`/`pi` params (Phase 1 needs neither — no `send`, no cwd-dependent
spawn); `STATE_RANK`/`STATE_LABEL` were re-declared locally in `audit.ts` rather
than shared from `agent-widget.ts` (flagged Minor by fit review — deferred, no
behavior impact).

Verification: 28 new tests in `test/audit.test.ts` cover the full Phase-1 test
list (parser mapping, picker naming/ordering incl. dormant tier, shortcut opens
picker, dormant resumes nothing, live events append, Esc-no-modal,
Enter-raises-without-touching-record, second-`/audit`-closes-first,
`shouldRegisterAudit` truth table). Targeted run (`audit` + `agent-widget` +
`ask` + `conversation-view` + `spawner`) green; full-suite failing-test-name set
byte-identical to the ~130-failure pre-existing baseline (missing installed Pi
SDK bundle) — DELTA = 0 regressions, +28 passing. Test review independently
mutation-verified the parser and gate assertions catch realistic defects (not
tautological). Review: partitioned (correctness/fit/test) all clean — 4 Minor
recorded, 0 Critical/Important, no relay.

The owner-run isolated-tmux live check remains pending (see `## Blocked`).

### Phase 2: owner steering, ownership, modal

Depends on Phase 1 and on `260908-feat-ws-pi-conversation-view-component`
Phase 2.

Verify the RPC abort command first (see `interrupt`). Add `lastWriter` and
`ownerSends` to `RpcAgentRecord` and the sidecar, stamp `lastWriter` at every
prompt site;
derive `liveness()` from it; route `ws-agent-settled` / `ws-agent-advisory`
to a toast while owner-held and suspend the anti-bleed loop for that span;
extend the thread-bound exemptions (park, fan-in, alias reuse, cap eviction)
to owner-held; add `owner_held` to `ws-agent-list` and the widget rows with
the running / idle-awaiting-owner split; build the modal and wire `hold` /
`finish` / `interrupt`; remove `overlayAttached`; make the ask overlay and
the audit viewer share the binding and retire the thread record's own
`transcript` in favour of the record-based source. Amend the spec per Spec
Impact.

Tests: `lastWriter` transitions for owner send, lead send, nudge, `finish`;
settle while owner-held produces a toast and no `ws-agent-settled`/advisory
push, while a `final` report still pushes; park, fan-in, alias reuse and cap
eviction all treat owner-held as thread-bound; the sidecar persists
`lastWriter` and `ownerSends` and a reloaded owner-held child stays
owner-held; modal render at 40/80/120 with the default highlight, toggle on
Esc, `interrupt` disabled when not running and permanently disabled when the
abort verification failed; `finish` on an owner-held child sends exactly one
handoff and the following settle takes the lead path, `finish` on a child
the owner never wrote to sends nothing; `finish` on a `lead-ask` thread
takes the `/done` route; parsed history attributes logged owner sends as
`user` and everything else as `lead-message`; Ctrl+C in viewer and modal
changes nothing. Live check
(owner-run): steer a running worker, `hold`, see the idle-awaiting-owner
rendering and the toast when it settles with no lead turn, reopen and
`finish`, confirm the lead receives the child's next settle or report and
the widget row returns to the lead's fan-in; `interrupt` a streaming child
and confirm it stops mid-turn with the view still open.

## Blocked (2026-09-09)

Phase 1 automated slice is complete and merged (`### Result (33ae460e)`), but
its **owner-run live check is a post-build acceptance gate that only a human on
a real TUI can clear**: `/audit` a worker mid-task on an isolated tmux socket
(`tmux -L ws-probe-<pid>`), watch a tool call appear and expand it, Esc, reopen,
and confirm the lead transcript received nothing from the viewer. No automated
harness can drive the live TUI overlay + RPC event stream end-to-end, so this
sign-off is owner-only.

The ticket also stays out of `.done/` because **Phase 2 (owner steering,
ownership, modal) is not yet implemented** — and Phase 2 additionally depends on
`260908-feat-ws-pi-conversation-view-component` Phase 2, whose own owner
acceptance was subsequently completed on 2026-09-09 and that prerequisite is
now closed. This ticket therefore remains in `ready/` with
Phase 1 code landed; a drain selector should skip it (blocked note present)
until the owner clears the Phase 1 live check and Phase 2's prerequisites are
met.
