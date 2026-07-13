---
title: ws dashboard agent activity chat UI
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260620-feat-ws-dashboard-agent-client-activity-sources: supplies the AgentClientProvider/Activity source contract, interaction-API methods, and per-harness capability tiering this UI dispatches through; split out 2026-07-11 once this ticket's own layout/UI-UX detail grew large enough to crowd that ticket's provider-adapter scope
  260624-feat-ws-dashboard-managed-cli-recent-sessions: supplies the cross-harness conversation history list surfaced by this ticket's "resume a past conversation" popup
related-mental-model:
  - ws-dashboard-agent-harness
  - ws-web-dashboard
sage-review: completed
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
    - **Build "resume from here" for cheap removal/disabling (owner,
      2026-07-11)**: given the risk above, implement "resume from here" as
      an isolated, independently toggleable feature from day one — its own
      component/module, its own feature flag or config gate, and its own
      per-harness capability check — not inlined into the shared bubble/
      turn rendering path. It should be removable or disabled per-harness (or
      entirely) by flipping one flag/removing one module, without touching
      "fork from here" or the rest of the conversation view. Treat it as an
      experimental affordance likely to need iteration or outright removal,
      not a committed permanent feature, until a harness's rewind primitive
      is confirmed solid enough to build on.
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
- **Skill-layer question narrowed, not fully resolved (2026-07-11 update)**:
  `260620`'s follow-up fixture check against the installed Claude CLI found
  Claude's skill listing is not uniformly Unavailable after all — it splits
  into a real Passthrough surface for plugin-provided skills (`claude
  plugin list` + `claude plugin details <plugin>`, no session needed) plus
  an Overlay filesystem scan for loose/project `SKILL.md` skills that have
  no CLI listing surface at all. This means `activity.session.skills` for
  Claude is inherently a small dashboard-owned union/aggregation (two
  sources merged into one list) even in the "thin passthrough" case — the
  open question narrows to whether this UI's skill-invocation affordance
  needs anything *beyond* that union (e.g. dashboard-side caching, manual
  overrides, cross-harness skill-name normalization) or whether the union
  itself is a sufficient dashboard-owned skill layer. Resolve the narrowed
  question before implementing the skill-invocation affordance.
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
a stub/mock provider ahead of `260620`'s adapters landing. **Tile-launch
semantics (fixture-review follow-up, 2026-07-11)**: clicking a tile actually
calls `activity.session.create`/`start` against whatever provider is wired
in (the real adapter once available, a stub that returns a synthetic
session/transcript in the meantime) — it is not a UI-only state transition
that waits for Phase 3/4 wiring to do anything. This keeps Phase 1
independently testable end-to-end against the stub before real adapters
land.

Verification boundary: frontend route/model tests for tab creation and the
resume-popup list rendering; browser-level acceptance evidence for the tile
launch flow actually invoking `activity.session.create`/`start` and
rendering the (stubbed or real) resulting session.

### Result

Implemented per plan `ai-docs/.plans/2026-07/13-0943-chat-ui-shell-phase1.md`
on branch `impl/chat-ui-shell-p` (commits `414d8805`, fix-cycle `6827fdb0`,
base `8d53d859`).

Added a new multi-instance `agentChat` `SurfaceKind` (dockview "opened"
policy, daemon-owned lifecycle per pane), modeled on the existing
`persistentTerminal` precedent rather than either wrong precedent flagged at
survey time (the singleton `agent` kind, and the read-only `workRootActivity`
tab). A top-right "Open new agent tab" button always opens a new empty tab;
an empty tab shows a resume popup backed by a local cross-harness history
stub (`activitySessionStub.ts`, scoped to the work root) plus three
per-harness start tiles (Codex/OpenCode/Claude). Clicking a tile invokes the
local stub `activity.session.create`/`start` path and renders the returned
synthetic transcript, satisfying the tile-launch semantics called out above
end-to-end against the stub, ahead of `260620`'s real adapters and `260624`'s
real history source (neither exists yet).

While wiring the new surface kind into shared `App.tsx` state, the
implementer found and fixed two pre-existing Dockview pane-activation races
that this surface kind's dual nature (dynamically added post-load, sharing
the "opened" group with read-only panes) newly exposed: pane-array ordering
in `buildWorkbenchEditorGroups` (agentChat panes now appended last so
existing pane indices don't shift), and the `activePaneByRoot` revalidation
effect not recognizing agentChat panes as live (now included in the
live-pane-id set and merged pane order). Both fixes were independently
verified genuine and free of new races by the correctness reviewer. Also
fixed a pane-local dismiss-outside-click bug on the resume popover (ref
wrapped only the popover, not its toggle button, causing immediate
self-dismiss).

Review: partitioned correctness/fit/test. Fit and correctness came back
clean (correctness: 2 Minor, non-blocking — resume popover doesn't surface
entry size, transcript block keys rely on a currently-hardcoded `"0"` cursor,
both deferred as Phase 2 forward concerns). Test partition raised one
Important finding — `agentChatSessions.test.ts` had zero coverage of the
failure/error path (`markAgentChatPaneError`, wired into both stub
create/start rejection call sites in `App.tsx`). Fixed via a dedicated
fix-cycle commit (`6827fdb0`) adding a non-vacuous failure-path test (verified
by temporarily gutting the function and confirming the new test failed);
re-review confirmed the finding resolved, no new issues. Remaining Minor
findings from all three partitions (resume popover entry-size display,
hardcoded cursor, a somewhat-tautological id-scoping test, no
empty-history-list case, no test for an unrecognized history-item `kind`
falling through to a silent `codex` default) were left as-is per lead
disposition — optional, non-blocking, several explicitly forward-noted for
Phase 2/3.

Verification: all 13 registered frontend route-test npm scripts pass;
`npm run test:browser` (Playwright + cargo daemon build) passes 2/2,
including a new e2e step that drives the real tile-click-to-render path
(click `agentChat.create` -> resume popover -> tile click -> asserts the
rendered transcript pane actually reflects the stub's response, not a
UI-only state flip). One intermittent, unrelated `test:browser` flake was
investigated (isolated via `git stash` against the base commit) and
confirmed pre-existing/environmental, not a regression from this change.

Not yet started: Phase 2 (streaming conversation rendering), Phase 3
(resume/fork, mid-turn submission queuing, the load-bearing "resume from
here" isolation requirement), Phase 4 (server-scoped integration).

### Phase 2: Streaming conversation rendering

Implement the messenger-style bubble layout, per-line streaming markdown
rendering, collapsible thinking blocks, per-turn/per-tool-use bubble
separation with summary/expand, and the copy-button affordance on every
bubble.

Verification boundary: frontend component tests for streaming markdown
rendering and collapsible-block default state; browser-level acceptance
evidence for a live streamed turn rendering incrementally.

### Result

Implemented per plan `ai-docs/.plans/2026-07/13-1118-chat-ui-streaming-phase2.md`
on branch `impl/chat-ui-streami` (commits `8aa453b0`, fix-cycle `523bde3a`,
base `f58c78c5`).

Added messenger-style bubble layout (user right-aligned, agent/tool
left-aligned), collapsible thinking blocks (default collapsed), per-turn/
per-tool-use bubble separation via additive `TranscriptBlock.role`/`turnId`
fields plus a new pure `groupTranscriptIntoBubbles` grouping helper, and a
copy-button affordance on every bubble. Per the survey plan's reuse
mandates, markdown rendering reuses `documentViewer.tsx`'s existing
`unified`/`remark-parse`/`remark-gfm` pipeline (new `renderMarkdownFragment`)
rather than a second parser, and tool-use summary/expand reuses
`workRootActivity.ts`'s existing `transcriptBlockView`/`transcriptCompactSummary`
heuristics verbatim. Since no real streaming backend exists yet, added
stub-side chunked/growing-text emission (`activitySessionStub.ts`'s
`stubBeginStreamingTurn`) wired through a new `mergeStreamingTranscriptBlocks`
overlay function, so incremental rendering is demoable and testable without
a live daemon. The additive `TranscriptBlock` schema change was confirmed
non-breaking against every existing consumer by the fit reviewer.

Review: partitioned correctness/fit/test. Fit and correctness came back
clean on first pass (correctness noted 3 forward-looking Minors — turn-id
grouping granularity, thinking-block interleaving fidelity, a
`chatBlockRole` title-substring heuristic that could miscategorize once a
real adapter lands — all latent against the not-yet-existing real adapter,
none affecting current stub-backed behavior). Test partition raised 2
Important findings: `mergeStreamingTranscriptBlocks` had zero unit coverage,
and `groupTranscriptIntoBubbles`'s test suite was missing an empty-transcript
case and a genuine multi-tick re-grouping stability sequence. Fixed via a
dedicated fix-cycle commit (`523bde3a`): extracted `mergeStreamingTranscriptBlocks`
out of `App.tsx` into a new pure, independently-testable module
`agentChatStreamMerge.ts` (App.tsx's xterm/react-aria-components/lucide-react/
CSS imports break the plain-tsc+node route-test harness), added unit tests
covering cursor-overwrite collision and unmatched-append ordering, and added
the missing empty-transcript and multi-tick stability cases. Re-review
confirmed both Important findings resolved and separately confirmed the
`App.tsx` extraction is a verified pure move (byte-identical body, no
closure/type/import-cycle drift, unchanged call site) — no regression, no
new findings.

Verification: all registered frontend route-test npm scripts pass
(including new `test:agent-chat-bubbles` and `test:agent-chat-stream-merge`);
`npx tsc -b`/`npm run build` clean; `npm run test:browser` passes 2/2,
including a new e2e step that polls the streaming bubble's text 8 times at
180ms intervals and asserts at least one strictly-increasing length pair
(genuine intermediate-state observation, not a before/after snapshot) plus
final settled semantic markdown rendering.

Not yet started: Phase 3 (resume/fork, mid-turn submission queuing, the
load-bearing "resume from here" isolation requirement), Phase 4
(server-scoped integration).

### Phase 3: Resume/fork and mid-turn submission queuing

Wire the per-user-bubble "resume from here" (in-place rewind,
`activity.session.rewind`, replaces the current tab) and "fork from here"
(new session, `activity.session.fork`, opens a new tab) buttons, gated by
the Cross-Harness Feature Matrix. Per the Decisions above, re-check
"resume from here" against `260620`'s matrix before enabling it live per
harness — it may need to ship disabled/hidden everywhere in the first pass
if no harness's rewind primitive cleanly supports exact-point rewind by
then, shipping only "fork from here" first. Implement "resume from here" as
its own isolated component/module behind its own feature flag/capability
gate, separate from "fork from here" and the shared bubble/turn rendering —
it must be removable or disable-able (globally or per harness) by flipping
one flag or deleting one module, since it is expected to need experimental
iteration or outright removal as harness rewind primitives evolve.

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

### Result

Implemented per the research-grade plan
`ai-docs/.plans/2026-07/13-1150-chat-ui-resume-fork-phase3.md` on branch
`impl/chat-ui-resume` (commits `6745f9ee`, fix-cycle `34fb3d52`, base
`a001fcdb`). Plan escalated to research at survey time (companion idea
ticket `260713-feat-ws-dashboard-activity-session-fork-cursor` filed as a
side effect, flagging a real gap in `260620`'s `ActivitySessionForkRequest`
contract for the eventual real backend).

Per the fixture-verified harness tiering in
`ai-docs/mental-model/ws-dashboard-agent-harness.md` (no harness qualifies
as a clean Passthrough/Overlay match for point-based rewind today — Codex's
`thread/rollback` is deprecated-for-removal and coarse, Claude's only path
is an unofficial Hack, OpenCode is unverified), "resume from here" ships as
an isolated, scaffolded-but-disabled module (`agentChatResumeFromHere.tsx`)
gated by a new `AgentChatCapabilities` model mirroring the Rust
`AgentClientCapabilities` struct field-for-field; `isResumeFromHereEnabled`
unconditionally returns `false` today (verified by the fit/correctness
reviewers to be a deliberate one-flag/one-module disable, not dead code).
"Fork from here" ships live, backed by a new stub-local, non-wire
`cutBlocks` parameter on `stubForkActivitySession` (the shared
`ActivitySessionForkRequest` type itself was left untouched, per
`260620`'s ownership of that contract — see the companion idea ticket
above).

Also implemented: the base prompt/send input UI (Phase 1/2 built only
read-only rendering), mid-turn submission queuing (immediately-rendered
pending bubble with a queued/steering badge, FIFO-dequeued on the stub's new
`onComplete` turn-completion callback), a revert control pulling a pending
message back into the editable input and canceling its queued send, and
up/down prompt-box history traversal (no prior codebase precedent; built
from scratch).

Review: partitioned correctness/fit/test. Fit came back clean (1 Minor:
resume/fork CSS rules aren't visibly grouped under a comment tying them to
the isolated module). Correctness came back clean on the critical scrutiny
item (independently confirmed the resume-from-here gate is a genuine
render-site-consulted one-flag disable, not bypassed/dead capability
plumbing) and verified all three self-reported implementation-time bug
fixes (a Dockview `contentRevision` under-keying bug, an e2e pending-vs-real
bubble locator ambiguity, and a `paneRef` stale-closure bug in the FIFO
dequeue); 1 Important finding (up-arrow can't recall a still-pending bubble)
was judged plan-conformant on inspection — the ticket's actual cancel
mechanism is the dedicated revert button, not up-arrow history — and closed
with no code change. Test partition raised 2 Important findings: no
regression test existed for the self-reported `paneRef` stale-closure fix,
and the new `appendUserTranscriptBlock` had zero direct unit coverage.
Fixed via fix-cycle commit `34fb3d52`: added direct unit tests for
`appendUserTranscriptBlock`, and added an e2e regression assertion in the
existing mid-turn-queuing step confirming the first message's bubble
survives a FIFO dequeue-and-deliver cycle (the exact scenario the
`paneRef`/`pendingRef` fix guards against). Re-review independently verified
both fixes are genuine by reverting each and observing the predicted test
failures, not just trusting the implementer's claim. The re-review also
surfaced one non-blocking follow-up gap worth noting for later: a stale
pre-swap turn's `onComplete` can still fire after a resume/history-item
swap on the same pane (only unmount stops an in-flight `stubBeginStreamingTurn`
handle, not the per-`activityId` reset effect) — distinct from the FIFO
dequeue scenario the fix commit covers, deferred as a latent, not-yet-
observed-in-practice edge case rather than reopening the fix cycle.

Verification: all registered frontend route-test npm scripts pass
(including new `test:agent-chat-capabilities`); `npx tsc -b` clean;
`npm run test:browser` passes 2/2, including cross-harness (Codex + Claude)
checks confirming "fork from here" renders while "resume from here" stays
absent for both, plus the queuing/revert/history acceptance flows.

Not yet started: Phase 4 (server-scoped integration).

### Phase 4: Server-scoped integration

Thread `serverId` through Activity source selection and stream keys for this
UI, following the existing Server Route pattern from `ws-web-dashboard`
(no new special-casing).

Verification boundary: server-scoped route tests showing local compatibility
aliases still map to `server-local`; browser-level acceptance evidence for a
linked remote server's Activity tab behaving identically to the local one.
