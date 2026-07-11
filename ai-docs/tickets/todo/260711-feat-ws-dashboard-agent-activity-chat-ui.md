---
title: ws dashboard agent activity chat UI
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260620-feat-ws-dashboard-agent-client-activity-sources: supplies the AgentClientProvider/Activity source contract, interaction-API methods, and per-harness capability tiering this UI dispatches through; split out 2026-07-11 once this ticket's own layout/UI-UX detail grew large enough to crowd that ticket's provider-adapter scope
  260624-feat-ws-dashboard-managed-cli-recent-sessions: supplies the cross-harness conversation history list surfaced by this ticket's "resume a past conversation" popup
related-mental-model:
  - ws-dashboard-agent-harness
  - ws-web-dashboard
---

# ws dashboard agent activity chat UI

## Background

`260620-feat-ws-dashboard-agent-client-activity-sources` originally carried a
single Phase 5 ("Activity UI and server-scoped integration") covering the
visible UI for the interactive agent-harness surface. Once that ticket's scope
was confirmed to be full-spec interactive control (not read-only projection),
the UI/UX detail grew large enough on its own to crowd the provider-adapter
protocol work in that ticket. This ticket carries the UI/UX design and
implementation for the interactive Activity chat surface; `260620` stays
scoped to the `AgentClientProvider` contract, per-harness adapters, and the
Activity read/write interaction-API methods this UI calls into.

This ticket depends on `260620`'s Phase 1 interaction-API contract
(`activity.session.start/create/send`, per-harness-gated
`compact/rewind/fork/skills`) existing before its interactive flows
(send/resume/fork) can be implemented against real adapters, though the
static layout/shell work can proceed in parallel against a stub provider.

## Decisions

- **Tab/entry-point layout** (owner, 2026-07-11): a top-right "open new agent
  tab" button, mirroring the existing "open new terminal" button, always
  opens a new empty agent tab immediately — it never blocks on a
  harness/session picker first.
  - An empty agent tab shows a top bar with a "current conversation" control,
    defaulting to placeholder text (e.g. "resume a past conversation").
    Clicking it opens a popup listing cross-harness conversation history (the
    vendor-history-scraped list from
    `260624-feat-ws-dashboard-managed-cli-recent-sessions`), showing
    per-entry alias/title (best-effort extraction), last-accessed time, and
    length/size. Selecting an entry starts/resumes that conversation in the
    tab. **Scope of the list (owner, 2026-07-11, interview)**: filtered to
    the current work root/worktree only, not a global cross-work-root list —
    consistent with the existing dashboard pattern of scoping surfaces to
    the currently-open work root rather than showing everything the daemon
    knows about.
  - Below that, an empty tab shows three large per-harness tiles (Codex,
    OpenCode, Claude), each starting a brand-new conversation with that
    harness directly — no path/work-root picker here, since work-root and
    worktree selection is already handled elsewhere in the dashboard before
    an Activity tab is opened.
- **Conversation view** (owner, 2026-07-11): standard messenger layout — user
  messages right-aligned, agent messages left-aligned, markdown rendering
  with real-time per-line streaming formatting (Obsidian-flavored markdown as
  the target dialect).
  - Where a harness exposes extractable thinking/reasoning content, render it
    as a collapsible block (default collapsed) interleaved between the
    surrounding agent content.
  - One chat bubble per agent turn; each tool invocation is its own separate
    bubble. Bubbles show a summary by default with click-to-expand for
    detail; the exact summarization depth is implemented at a reasonable
    common-sense default for the first pass and left as an explicit TBD for
    refinement, not a blocking design question.
  - **"resume from here" vs "fork from here" — distinct semantics (owner,
    2026-07-11, interview)**: these are two separate buttons, not two labels
    for the same action.
    - **resume from here**: in-place rewind. Mutates the *current* session —
      the conversation is rewound to that user bubble and everything after
      it is discarded, and the current tab is replaced in place with the
      rewound state (no new tab). Dispatches through
      `activity.session.rewind`.
    - **fork from here**: branches to a *new* session, preserving the
      original conversation untouched, and opens the result in a **new
      tab**. Dispatches through `activity.session.fork`.
    - Both remain gated by the Cross-Harness Feature Matrix — an
      Unavailable/Hack cell hides or disables its button rather than
      attempting it. **Known risk, flagged not resolved**: "resume from
      here"'s in-place-rewind-to-an-exact-point semantics assumes a
      point-based rewind primitive, but per `260620`'s fixture spike, no
      harness cleanly offers that today — Codex's only rewind primitive
      (`thread/rollback`) is confirmed **deprecated for removal** and drops
      N turns from the *end*, not an arbitrary point (coarser than this
      button implies, and wrong if turns were forked/reordered); OpenCode's
      equivalent is unverified; Claude's only reachable path is a
      transcript-truncation Hack. This means "resume from here" may not be
      implementable as a clean per-harness Passthrough/Overlay action for
      any of the three today — Phase 3 must re-check this against `260620`'s
      matrix before wiring the button live, and may need to ship "resume
      from here" disabled/hidden everywhere at first while only "fork from
      here" (backed by the confirmed-real `thread/fork`) ships in the first
      pass.
  - If the user submits a message while an agent turn is still in progress,
    it is queued rather than rejected or requiring the human to wait
    (**owner, 2026-07-11, interview**):
    - The message immediately appears as its own user chat bubble carrying a
      "pending/queued" badge, and is actually delivered on the next agent
      tool-call batch (Codex: via `turn/steer` where available; other
      harnesses queue for the next turn boundary), at which point the badge
      clears.
    - The prompt input box supports up/down-arrow history traversal across
      previously sent messages, the same as a normal shell/REPL input
      history.
    - The pending bubble itself renders a revert/되돌리기 (undo) button to
      its right. Pressing it — or reaching that pending bubble via the
      prompt box's up-arrow history traversal — pulls its text back into the
      prompt input in an editable state and cancels the queued submission
      (the pending bubble is removed; nothing is sent for it).
  - Every chat bubble (user, agent-turn, and tool-use) has a copy button.
- **Open, not-yet-decided**: whether the dashboard should maintain its own
  skill/capability layer instead of relaying each harness's native skill
  listing as-is, given observed unreliability in per-harness native skill
  discovery (see `260620` Decisions). This affects whether
  `activity.session.skills` ships as a thin per-harness passthrough or a
  dashboard-owned aggregation/override layer, and in turn whether this UI's
  skill-invocation affordance needs its own dashboard-owned picker rather
  than relaying a harness's native list. Resolve before implementing that
  affordance.
- **Broader dashboard layout adjustment** (owner concern raised 2026-07-11,
  not yet detailed): the owner flagged that fitting this interactive chat
  surface well may require adjusting the dashboard's existing layout beyond
  just the Activity tab's own internals. Not yet scoped — surface concrete
  layout changes here as they're identified rather than assuming the
  existing workbench/tab chrome needs no changes.

## Constraints

- This ticket does not re-litigate `260620`'s scope, tiering, or provider
  adapter design; it only consumes the interaction-API contract and
  capability tiering `260620` defines. Any capability gap discovered while
  designing this UI (e.g. an affordance with no backing method) is a `260620`
  change, not a workaround built here.
- Per-harness-gated affordances (rewind/fork/compact/skills/steer) must
  reflect the Cross-Harness Feature Matrix from `260620` at render time —
  do not show a control for a cell classified Unavailable, and label
  Hack-tier controls (none currently in scope for normal phases) as
  experimental if any ever ship here.

## Phases

### Phase 1: Chat surface shell and tab entry points

Implement the top-right "open new agent tab" button, the empty-tab
"current conversation" resume popup wired to the cross-harness history list,
and the three per-harness "start fresh" tiles. This phase can proceed against
a stub/mock provider ahead of `260620`'s adapters landing.

Verification boundary: frontend route/model tests for tab creation and the
resume-popup list rendering; browser-level acceptance evidence for the tile
launch flow.

### Phase 2: Streaming conversation rendering

Implement the messenger-style bubble layout, per-line streaming markdown
rendering, collapsible thinking blocks, per-turn/per-tool-use bubble
separation with summary/expand, and the copy-button affordance on every
bubble.

Verification boundary: frontend component tests for streaming markdown
rendering and collapsible-block default state; browser-level acceptance
evidence for a live streamed turn rendering incrementally.

### Phase 3: Resume/fork and mid-turn submission queuing

Wire the per-user-bubble "resume from here" (in-place rewind,
`activity.session.rewind`, replaces the current tab) and "fork from here"
(new session, `activity.session.fork`, opens a new tab) buttons, gated by
the Cross-Harness Feature Matrix. Per the Decisions above, re-check
"resume from here" against `260620`'s matrix before enabling it live per
harness — it may need to ship disabled/hidden everywhere in the first pass
if no harness's rewind primitive cleanly supports exact-point rewind by
then, shipping only "fork from here" first.

Implement mid-turn user-submission queuing: an immediately-rendered pending
user bubble with a "pending/queued" badge that clears once delivered next
batch (Codex `turn/steer` where available; queue-for-next-turn elsewhere),
prompt-box up/down-arrow history traversal, and a revert/되돌리기 control
that pulls a still-pending bubble back into the editable prompt input and
cancels its queued submission.

Verification boundary: frontend integration tests for gating logic per
harness capability (including a test asserting "resume from here" stays
disabled wherever the underlying rewind cell isn't a clean Passthrough/
Overlay match); browser-level acceptance evidence for a queued mid-turn
submission landing in the next tool-call batch, and for the revert/undo
flow removing a pending bubble without sending it.

### Phase 4: Server-scoped integration

Thread `serverId` through Activity source selection and stream keys for this
UI, following the existing Server Route pattern from `ws-web-dashboard`
(no new special-casing).

Verification boundary: server-scoped route tests showing local compatibility
aliases still map to `server-local`; browser-level acceptance evidence for a
linked remote server's Activity tab behaving identically to the local one.
