---
title: "Pi adapter: shared subagent conversation view — owner audit window and owner steering built on the ask overlay"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260904-feat-ws-pi-side-thread-fork-question-surface: owns the ask overlay (`overlay-chat.ts`, `ask.ts`) this epic rebuilds into a shared component; its Esc / `/done` semantics are superseded here and its dead `overlayAttached` flag removed
  260905-feat-ws-pi-overlay-activity-indicator-and-esc-hint: the `working…` marker and header Esc hint read `ForkChannel.isStreaming()`; both are re-expressed on the 3-state liveness input
  260905-feat-ws-pi-agent-alias-park-and-registry-cap: park-at-idle rule and its thread-bound exemption; an owner-held child must be exempt the same way
  260905-feat-ws-pi-push-only-child-reports: settle push suppression is keyed on `threadBound` today; this epic keys settle ownership on the last writer
  260905-feat-ws-pi-live-agent-widget: the `awaiting owner` row state and `ws-agent-list` rows that must carry the owner-held flag
  260903-feat-ws-pi-subagent-rpc-ux: RPC registry, `ws-agent-transcript`, `prompt()`/`steer()` mapping the viewer and steering ride on
  260906-feat-ws-pi-tool-result-yaml-tui-rendering: sibling TUI rendering work; no dependency, but tool-result rendering conventions should not diverge between the lead transcript and the conversation view
related-mental-model:
  - plugin-runtime
sage-review-design: completed
sage-review-design-reviewed: 4cd1ca769f0d6eb8
---

# Pi adapter: shared subagent conversation view — owner audit window and owner steering built on the ask overlay

## Scope

Owner request (2026-09-08): a window inside the Pi lead TUI where the owner
can **audit a subagent's conversation** — every child the lead runs (workers,
execute workers, task forks, discussion forks), live while it streams and after
it settles — and, when needed, **steer** that subagent directly.

The only conversation UI the adapter has today is the ask overlay
(`agents-plugin-pi/src/overlay-chat.ts` + `src/ask.ts`): a `ctx.ui.custom`
overlay that hand-rolls its own wrapping, scrolling and input, keeps a
`TranscriptEntry {who: "you" | "thread" | "note"; text}` list with a hard
24-line tail cut, and is bound to one fork through `ForkChannel`
(prompt-vs-steer on `isStreaming()`). This epic rebuilds it into one **shared
conversation-view component** on `@earendil-works/pi-tui` primitives and puts
both the existing ask flow and the new audit window on it.

Included:

- The shared component: general message model (user / assistant / tool call /
  tool result / lead message / note; structured content; per-item
  expand/collapse), two modes (`view` read-only, `interactive`), liveness
  driven by a 3-state child input, built on pi-tui (`ScrollView`, `Text`,
  `Markdown`, `Editor`) instead of local re-implementations.
- Unblocking pi-tui at test time: `@earendil-works/pi-tui` becomes a direct
  dependency of `ws-pi-bridge` (it is today only nested under
  `pi-coding-agent`'s `node_modules`, so `node --test` cannot resolve it — the
  root cause of the hand-rolled overlay). It is pinned to the same range as
  `@earendil-works/pi-coding-agent` (`^0.84.4`) so npm resolves one copy; the
  runtime keeps importing pi-tui through the loaded host, and child A verifies
  at Phase 1 that the test-time and host copies are the same version.
- Migrating the ask overlay onto the component with no owner-visible
  regression, and deleting the local rendering helpers it replaces.
- The audit window: `/audit` → picker of live and recent children → viewer on
  the component in `view` mode, following a running child's stream live.
- Owner steering from the viewer (`interactive` mode on any child, not only
  forks), the settle-ownership rule that goes with it, and the Esc modal
  (`hold` / `finish` / `interrupt`) that is the only way to leave an
  interactive view.

## Non-Scope

- Replacing or re-rendering the lead's own main transcript window. Pi's
  extension API (0.84.4 `docs/extensions.md`) exposes overlays, widgets,
  header/footer and custom entries, not the main chat container; the audit
  window is an overlay.
- A standalone viewer program outside the Pi TUI (owner rejected: the cost of
  this feature *is* the TUI component; a second program does not remove it).
- Rendering child conversations through `pi.sendMessage` into the lead
  session. `sendMessage` always enters model context via `content` regardless
  of `display`; audit output must never reach the lead model.
- Changing where a child's `final` report is delivered. Reports go to the
  lead; owner steering does not redirect them.
- Resolving lead/owner steering conflicts mechanically. Two writers on one
  child is handled by owner convention ("I am steering this one"), as on
  other hosts; the adapter only makes ownership visible.
- Any change to `agents-plugin-tool/` (ws-mcp Go). All work lives in
  `agents-plugin-pi/`.

## Child Tickets

- `260908-feat-ws-pi-conversation-view-component` — done. Shared component
  and `/answer` migration, including the accepted 260909 overlay follow-ups.
- `260908-feat-ws-pi-subagent-audit-window-and-owner-steering` — ready.
  Phase 1 read-only audit is implemented; its distinct owner-live gate and
  Phase 2 owner steering/ownership/modal remain. The component prerequisite
  is now complete.
- `260908-feat-ws-pi-down-arrow-into-agent-picker` — idea. Owner requested
  ready promotion on 2026-09-09, but the local Pi public API does not expose
  the precise post-editor no-action condition needed to preserve native
  autocomplete/history/visual-line Down behavior. Keep blocked on that
  supported hook; `/audit` and `Ctrl+Shift+U` are already available.

The owner requested the remaining children advance, not that their unresolved
live gates or host API constraints be considered passed. Audit Phase 2 depends
on its Phase 1 acceptance; Down-arrow requires the supported input hook before
its exact behavior is implementation-ready.

## Cross-Child Decisions

- **One component, two consumers.** Ask and audit render through the same
  component and the same child binding. Neither child ticket may keep a
  second rendering path (the `260904` `OverlayChatComponent` is replaced, not
  wrapped). Option B (extract-and-reuse of the current overlay as is) was
  rejected: its transcript model is text-only and its rendering is the thing
  being replaced.
- **Human-only surface.** Everything the owner sees in the viewer stays out of
  the lead's model context. Allowed surfaces: `ctx.ui.custom` overlay,
  `ctx.ui.notify`, `setWidget`/`setStatus`, tool-result `details`. Forbidden:
  `pi.sendMessage` for viewer content.
- **Child liveness is a 3-state input**, not a boolean:
  `running` | `idle-awaiting-owner` | `settled`. It replaces
  `ForkChannel.isStreaming()` (`ask.ts`) as what the component reads — today
  that boolean feeds only the overlay's render cache and `working…` marker —
  and it is the input the ownership rule keys off. The live-agent widget
  derives its row state from `threadBound` / `pendingApproval` today and has
  no running/idle distinction; giving its rows one is new work under this
  epic, not a migration. The states are computed from the record, not from
  the view: `running` = the child's turn is in progress (`record.running`);
  `idle-awaiting-owner` = the turn ended while the owner is the last writer
  (the child is not parked because it is owner-held, and it waits for the
  owner's next line — whether or not a viewer is open on it); `settled` =
  the turn ended with the lead as last writer (parked to dormant, or
  reported). `idle-awaiting-owner` therefore fires only for owner-held
  children, never for an ordinary parked worker. It is rendered prominently —
  clearly more salient than the other two — so the owner notices a child
  waiting on them; exact styling is implementation detail.
- **Last writer owns settle.** A child record carries `lastWriter: "lead" |
  "owner"`. The owner's first message from the viewer sets `owner`; a lead
  send (`ws-agent-send`, nudge, any lead-side prompt) sets `lead`. When the
  child settles, the side that wrote last is the side notified: `owner` → a
  `ctx.ui.notify` toast to the human, no lead wake; `lead` → the existing push
  path. Of the adapter's push families
  (`260905-feat-ws-pi-push-only-child-reports`), the rule governs
  `ws-agent-settled` and `ws-agent-advisory` only: while owner-held those two
  become owner toasts. `ws-agent-report` (final and progress),
  `ws-agent-approval`, `ws-agent-question` and `ws-agent-orphaned` keep their
  lead path unchanged (the viewer has no approve verb; the lead still answers
  approvals). The settles a child produces while the owner is talking to it
  are never replayed to the lead — they were addressed to the owner.
  This generalizes the thread-lifetime suppression keyed on `threadBound`
  today (`260905-…-alias-park`, `260905-…-push-only`) to any child; the
  view-scoped `overlayAttached` flag of `260904` (set but no longer read) is
  removed. **Closing the viewer does not hand the child back to the lead.**
  Ownership returns to the lead only through `finish` (below) or a lead-side
  send.
  While owner-held, a child is treated like a thread-bound one on these
  `threadBound`-keyed surfaces: exempt from park-at-idle
  (`260905-…-alias-park`), outside the lead's fan-in set (the `N delegated
  agents still running` count excludes it while held and counts it again
  once ownership is released), protected from alias reuse and cap eviction.
  The shutdown sidecar is **not** one of them: an owner-held worker has no
  `.ws-threads.json` counterpart, so it is persisted like any other child and
  survives `/reload`. Its `ws-agent-list` row and live-agent widget row show
  it as owner-held; `session_children` is a ws-mcp tool outside this epic's
  boundary and is not changed.
- **Leaving an interactive view is a forced choice.** Esc in an interactive
  view opens a modal `[ hold ] [ finish ] [ interrupt ]` with an `Esc: cancel`
  hint line; Esc again closes the modal (toggle). `hold` (default) closes the
  view and keeps owner ownership; `finish` closes the view and hands the
  child back to the lead by sending it **one lead-attributed handoff message**
  ("the owner has left; continue your task" — wording is implementation
  detail) that sets `lastWriter: "lead"`, so the child runs one more turn and
  its next settle or report reaches the lead through the normal path (queued
  after the current turn when the child is running, a fresh prompt when it is
  idle). The handoff is sent only when the child is actually owner-held; a
  `finish` on a child the owner never wrote to just closes the view. Settle
  routing reads `lastWriter` at settle time: a handoff queued into a running
  run is processed before that run ends, so the run's settle carries the
  child's reply to the handoff and takes the lead path. Threads keep today's
  origin routing: a `lead-ask` discussion
  thread's `finish` is `/done` (summary turn, `ws-thread-summary` injection —
  the same one-more-turn shape, no generic handoff message); a `fork-raised`
  thread gets the generic handoff like any other child (today `/done` only
  detaches) and the fork's own `final` still reaches the lead as a report.
  Rejected: replaying the settles deferred during owner steering to the lead
  at `finish` (they had no lead audience). `interrupt` aborts the child's
  current turn only,
  keeps the view open, changes nothing about ownership, and is disabled when
  the child is not running. The modal is canonical; the `/done` slash command
  is kept as an alias for now and may be removed later. `view` mode (the
  read-only audit window) closes on Esc directly; the modal exists only where
  ownership is at stake.
- **Ctrl+C never reaches Pi from the viewer or the modal.** Pi's editor exits
  the process on a double Ctrl+C within 500 ms (`pi-coding-agent` 0.84.4,
  `dist/core/keybindings.js` maps `app.clear` to ctrl+c and
  `dist/modes/interactive/interactive-mode.js` `handleCtrlC` calls
  `shutdown()` on the second press; a focused overlay receives raw input via
  `handleInput`, and pi-tui has no global Ctrl+C intercept). The overlay
  swallows it and gives it no meaning. Rejected: Ctrl+C as a modal key; an
  Esc-inert modal.
- **Audit entry is `/audit`.** A `registerCommand` picker of live and recent
  children (alias > title > short uuid, as the widget names them); selection
  opens the viewer. Never auto-pops.
- **Tool-result rendering does not fork.** The component renders a child's
  tool results through the same formatter function the lead transcript uses
  — `260906-feat-ws-pi-tool-result-yaml-tui-rendering` attaches it to the
  bridge's per-tool `renderResult` hook, which does not apply to a child's
  calls reconstructed from its transcript or event stream, so that formatter
  is extracted to a module both call sites import. One implementation; a
  child-specific rendering dialect is not introduced.

## Completion Criteria

- Done: both children closed to `.done/`; the ask overlay and the audit
  window render through one component; `overlay-chat.ts`'s local
  `visibleWidth`/`wrapLine`/transcript helpers are gone; the owner has run
  the live checks of both children (audit a streaming worker, steer a fork,
  hold and reopen, finish, interrupt) and reported them passing.
- Dropped: the owner decides the audit window is not worth a pi-tui component
  after A Phase 1, or Pi ships a native subagent-conversation surface that
  makes the adapter's own obsolete.
- Deferred: replacing the lead's main window; mechanical lead/owner conflict
  resolution; report re-routing to the owner; removal of the `/done` alias.
