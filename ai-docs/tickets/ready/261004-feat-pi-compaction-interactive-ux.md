---
title: Pi compaction UX for active conversations and display-only history
related:
  261003-feat-pi-lead-compaction-dialog-transcript: retain-none context and summary dialog remain; add independent human-only history
  261002-chore-ws-pi-retire-ws-claude-and-soften-compaction-advisory: refine advisory wording without changing delivery
  261003-feat-pi-lead-resume-after-midrun-compaction: preserve compaction resume and user handoff behavior
  261002-feat-pi-lead-ws-owned-compaction: existing lead compaction mechanism
  260908-research-ws-pi-lifecycle-race-monitoring: adjacent lifecycle verification evidence
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0d1aea13bade20ee
sage-review-completeness-reviewed: 0d1aea13bade20ee
---

# Pi compaction UX for active conversations and display-only history

## Background

The user reports that compaction removes the visible conversation, making
interactive discussion harder to follow, and that GPT-family leads tend to
act immediately on the 50% advisory even while awaiting human input. The
model-specific frequency is a user observation, not an experimentally
established property.

Source investigation found two contributing mechanisms: the adapter retains
no raw pre-compaction entries and Pi rebuilds the main transcript from the
compacted context; the advisory starts a follow-up model turn and includes
preparation instructions without explicitly excluding waits for the human.
The original session records survive. Preserve the context savings while
restoring human-readable recent discussion independently of model messages.

## Decisions

- **D1 — Informational advisory with a narrow safe boundary.** Explicitly label
  the advisory nudge as information, not a task or an instruction to compact.
  Autonomous advisory compaction is appropriate only when there is no active
  discussion with the human and no human answer or clarification being awaited.
  A natural pause after asking the human a question is not permission to
  compact. Continue the interactive exchange instead of treating the nudge as
  the next task. This narrows the advisory's prompt-level judgment, not its
  event-delivery mechanism.
- **D2 — Preserve non-advisory behavior.** Keep existing advisory thresholds,
  once-per-crossing/rearming behavior, delivery and model wake semantics. Do
  not add a deterministic conversational-state gate or change hard-threshold,
  manual /compact, native fallback, or post-compaction goal/resume behavior.
  The change requested is wording and safe-autonomy interpretation.
- **D3 — Human-only inline history.** After successful lead compaction, display
  previous user and assistant message bodies without adding them to the
  model's conversation context or triggering another model turn. Use the
  supported plain custom-entry rendering path, pi.appendEntry plus
  pi.registerEntryRenderer, rather than sendMessage/display:true (which still
  adds a model-context message). Preserve NO_KEPT_ENTRY_ID and the existing
  summary/dialog machinery.
- **D4 — Latest twenty messages, not twenty exchanges.** Select at most the
  twenty most recent eligible user/assistant messages before the compaction,
  counted together, and display their text bodies in chronological order.
  Exclude tool calls/results, thinking, adapter control messages, prior
  display-only history entries and compaction summaries. Do not substitute
  a summary or tool transcript for the selected original message bodies.
- **D5 — Explicit historical boundary, flexible placement.** Add a visible
  previous-conversation/display-only header and separator so the human can
  distinguish restored history from current conversation. Prefer placement
  before the compaction block when supported; placement after it is acceptable.
  The inspected Pi host renders a session_compact-appended history entry
  before the summary immediately but after it on reload. Do not promise a
  stable relative position or use private host mutation to force one.
- **D6 — Persist and refresh without recursive replay.** The display-only
  history remains available on reload. Repeated compactions refresh the
  latest-twenty selection from original messages on the active branch, not
  prior snapshots or summary prose. Associate the display entry with its
  compaction so duplicate event handling does not duplicate the block.
  Failed or cancelled compaction creates no new history block.

## Constraints

- This is a lead-adapter feature. Worker/explore/fork native compaction policy
  is unchanged; do not broaden the lead advisory to children.
- Preserve original session history and branch isolation. Display entries add
  storage/rendering work, but no direct model-context messages or automatic
  model run. Existing human-only history must not be replayed into later
  compaction summaries as conversational content.
- Use supported extension APIs and the existing host-compatible Pi UI loading
  seam; do not mutate chatContainer, rewrite stored compaction boundaries,
  retain raw model context for UI purposes, or patch the installed host.
- Read ai-docs/manuals/skill-authoring.md before modifying compaction guidance
  or prompt strings. Read ai-docs/manuals/shipped-surface-boundary.md before
  modifying shipped text. Apply matching AGENTS.md Implementation Conventions
  if shared full/wsflow surfaces are touched. No change to shared host-neutral
  workflow semantics is requested.
- Pi API work requires complete reading of relevant installed Pi documentation
  and its applicable Markdown cross-references, including extensions,
  compaction, sessions/session format, messages and TUI APIs.

## Prior Art

- agents-plugin-pi/src/goal-loop.ts: search buildLeadCompactionResult,
  sendPreparation, fireCompactionTriggers, session_compact and compaction
  completion handling.
- agents-plugin-pi/src/lead-compaction.ts and agents-plugin-pi/lead-compact-guide.md:
  existing advisory preparation wording, retain-none sentinel and summary dialog
  (agents-plugin-pi/src/lead-compaction.ts#L81, #L622-L650;
  agents-plugin-pi/lead-compact-guide.md).
- agents-plugin-pi/src/pi-tui.ts: host-compatible UI primitive resolution;
  avoid creating incompatible duplicate package instances.
- Installed Pi extension APIs: appendEntry, registerEntryRenderer and the
  session_compact event. Plain custom entries differ from custom_message
  entries in sessionEntryToContextMessages; only the latter reach the model.
- Installed Pi interactive renderer: compaction_end and renderInitialMessages
  select context entries differently, accounting for live/reload placement.

## Relations

| stem | declared as | status |
|---|---|---|
| 261003-feat-pi-lead-compaction-dialog-transcript | related | done |
| 261002-chore-ws-pi-retire-ws-claude-and-soften-compaction-advisory | related | done |
| 261003-feat-pi-lead-resume-after-midrun-compaction | related | done |
| 261002-feat-pi-lead-ws-owned-compaction | related | done |
| 260908-research-ws-pi-lifecycle-race-monitoring | related | todo |

## Prior Decisions

- 261002-chore-ws-pi-retire-ws-claude-and-soften-compaction-advisory (2026-10-03, commit 0910a053): "Advisory head now matches the user-approved light-nudge text and interpolates the hard percent; delivery (followUp+triggerTurn) and re-arm behavior are unchanged." — bearing: constrains
- 261003-feat-pi-lead-resume-after-midrun-compaction (2026-10-03, commit 0e56a275): "Route is stored on CompactionOperation because the abort's agent_end clears preparation state before the deferred release" — bearing: constrains
- 26047862 (2026-10-03, commit): "User confirmed route-dependent resume: hard steer and autonomous lever calls resume; advisory and /compact reroute do not; goal-active and Pi auto/overflow paths unchanged." — bearing: constrains
- 261003-feat-pi-lead-compaction-dialog-transcript (2026-10-03, commit 300b5371): "Pi's `buildContextEntries` keeps pre-compaction entries only from the entry whose id equals `firstKeptEntryId`" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, commit f065de0d): "Crossing flags also re-arm when usage is observed below the threshold, which keeps \"once per crossing\" literal without depending only on compaction." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/lead-compaction.ts, agents-plugin-pi/lead-compact-guide.md; renderer and tests within the same adapter |
| scope.surface | cross-module | goal-loop lifecycle hooks, lead-compaction prompt builders, and Pi entry rendering/persistence; no ws-mcp interface change |
| scope.new_public_symbol | no | no new tool, config knob or exported symbol required by the ticket; display customType is adapter-local |
| scope.new_type_contract | yes | new persisted display-entry data must associate original message bodies and the successful compaction; supported CustomEntry/EntryRenderer contract in installed dist/core/extensions/types.d.ts#L1208-L1227 |
| scope.test_surface | existing | agents-plugin-pi/test/lead-compaction.test.ts#L433-L462 and test/goal-loop.test.ts#L2075-L2105, #L2126-L2175, #L2363-L2526; real SessionManager fixture already exists |
| complexity.reuse_points | confirmed | humanTextOf in agents-plugin-pi/src/lead-compaction.ts#L185-L194; loadHostPiTui in src/pi-tui.ts; installed examples/extensions/entry-renderer.ts demonstrates appendEntry plus registerEntryRenderer. collectDialogItems splits assistant text around tools and includes branch summaries, so it is not a twenty-message selector |
| complexity.side_effect_risk | moderate | append-only display storage and session_compact handling must not duplicate snapshots, trigger runs, or alter existing compaction release/resume |
| risk.correctness | high | combined-message counting, raw-text filtering, active-branch selection, repeated compaction and duplicate success events must preserve retain-none context |
| risk.fit | moderate | supported entry API fits display-only history; live/reload ordering differs and existing prompt autonomy/resume contracts must remain intact |
| risk.test | high | existing offline fixtures cover compaction context and triggers, but real interactive placement/reload and model compliance require the explicitly requested live acceptance |
| risk.security_or_contract | moderate | raw human/assistant history is persisted for display; must never enter model context or summaries as a custom_message and must exclude control traffic and thinking |

Installed-host evidence (root: `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/`): `dist/core/session-manager.js#L162-L191` maps plain custom entries to zero context messages; `dist/core/agent-session.js#L2685-L2691` appends and emits entry_appended without prompting. `dist/core/agent-session.js#L2199-L2230` awaits session_compact before compaction_end. `dist/modes/interactive/interactive-mode.js#L2943-L2951` renders entries after the compaction first, then its summary live; `#L3354-L3359` renders buildContextEntries in order on reload (summary first, then the appended display entry). These are source-level ordering facts, not a live UI acceptance result. Relevant installed extensions, compaction, sessions, session-format, message-types, TUI and SDK documentation and entry-renderer/custom-compaction examples were read.

## Phases

### Phase 1: Preserve interactive compaction UX without retaining model history

**Intended behavior.** Refine the lead advisory to state its informational role
and the absence-of-active-human-discussion/awaited-human-answer safe boundary.
Add a supported display-only history entry and renderer that restore the latest
up-to-twenty combined user/assistant text messages around successful lead
compaction, with an explicit historical header and separator. Preserve
retain-none model context, current delivery/resume behavior and supported
live/reload placement differences.

**Deferred scope.** Exact tool/thinking replay, image/attachment rendering,
perfect equivalence to native message selection/copy controls, fixed placement
before the compaction summary, installed-host changes, deterministic advisory
state gates, threshold/delivery changes, child compaction policy, and empirical
claims that GPT always or never chooses compaction.

**Verification boundary.** Add prompt tests distinguishing active human
interaction and awaited answers from genuinely safe autonomous pauses; ensure
hard/manual wording and delivery/rearming tests remain valid. With a real
SessionManager fixture, prove history persists but produces zero model-context
messages, retains the original records, and emits no model continuation.
Test twenty-message counting/chronological order, fewer-than-twenty and empty
history, exclusion of thinking/tools/custom traffic/old snapshots, branch
isolation, repeated compaction and duplicate handling, reload, and no entry on
failed/cancelled compaction. Test the renderer's previous-conversation header,
separator and user/assistant text presentation. Run relevant Pi suites and
perform a live lead UI acceptance check of successful compaction and reload;
record any unverified live check rather than claiming source analysis proves
pixels, copy/selection behavior or model compliance.
