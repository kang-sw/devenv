---
title: ws-owned lead compaction for the Pi adapter
related:
  260903-feat-ws-pi-goal-loop-compaction-hook: predecessor; its observe-only session_before_compact stance and goal-compact-and-continue lever are superseded for the lead session
  260913-bug-ws-pi-inactive-goal-compact-and-continue-aborts: reversed; its inactive-goal rejection no longer applies to the lever
  261002-feat-ws-config-adapter-schema-extension: dependent; migrates this ticket's knobs from goal-loop-config.json into ws-mcp config
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: 45f2cfb52f96daa6
completed: 2026-10-02
---

# ws-owned lead compaction for the Pi adapter

## Background

Long-running Pi lead sessions lose context badly across compaction.

- Pi's native compaction (`@earendil-works/pi-coding-agent` 0.84.4,
  `dist/core/compaction/`) appends `<read-files>`/`<modified-files>` lists
  gathered only from `read`/`write`/`edit` tool calls, inherits them from the
  previous non-hook compaction's `details` forever, and never caps or
  normalizes them (`compaction.js` file-op inheritance, `utils.js`
  `computeFileLists`/`formatFileOperations`). Its update prompt says
  "PRESERVE all existing information", so the summary only grows: a
  2026-09-08 lead session went 10k -> 15k -> 23k summary characters over
  three compactions. Tool results are cut to 2000 characters when serialized
  for the summarizer, the summary call rejects tool calls, and automatic
  compaction never receives custom instructions.
- The user-installed third-party extension `@lll9p/pi-better-compaction`
  replaced lead compaction with OpenAI Responses V2 opaque blobs and keeps
  user/developer/system items verbatim up to 65k tokens. Pi converts every
  extension-injected custom message to `role: "user"`
  (`dist/core/messages.js` `convertToLlm`), so the adapter's `ws-push-batch`
  reports, wake lines, and skill expansions were retained verbatim and
  accumulated across compactions (a 2026-09-21 lead session: 57 -> 142
  retained items, about 20k -> 33k tokens, mostly worker reports). The user
  is disabling that extension.
- The adapter today only observes compaction: goal-loop's
  `session_before_compact` handler never overrides, the
  `goal-compact-and-continue` lever passes `carry_forward` to pi's native
  summarizer as custom instructions, and nothing re-establishes ws session
  state after compaction beyond the system prompt's state pointer line.

This ticket makes the adapter own lead compaction so that durable state lives
in ws tooling and the summary carries only what compaction alone can carry.

## Decisions

- **Scope: the lead session only.** Spawned worker and explore sessions keep
  Pi's native compaction; they are structurally short-lived and rarely
  compact. The adapter owns lead compaction by returning
  `{ compaction: CompactionResult }` from `session_before_compact`.
- **Two-stage, LLM-active compaction authored by the lead itself.** A
  preparation turn asks the lead to move durable detail into ws tooling with
  its ordinary tools (agenda, todos, notes; open decisions into tickets or an
  Open Decision Queue) and then call a lever tool carrying prose for what
  only compaction can carry: the user's style and preferences, working
  guidance, current work context, mood and rapport, and residual details
  with no tool home. The lead authors this in-session, so the whole context
  is a prompt-cache hit and the author has seen full tool results. Rejected:
  a separate summarizer call over serialized history as the primary path
  (cache miss, 2000-character tool-result truncation, an author that never
  saw the work).
- **Summary = adapter-filled deterministic sections + the lead's prose.**
  Deterministic sections: the ws session key; the active ticket and phase;
  in-flight child agents from the spawner registry; child agents finished
  since the previous compaction, one line each (name, ticket, terminal
  status, commit) from the same registry, since their reports are excluded
  from the user-message section; human-typed user messages (the analogue of
  Claude Code's "All user messages" section), excluding adapter-injected
  traffic such as `ws-push-batch` reports, wake lines, goal reminders, and
  skill expansions; and the active playbook. The summary carries no file
  lists. Hook results are stored with `fromHook=true`, which also ends Pi's
  file-list inheritance chain. The summary closes by telling the lead to
  invoke `lead-revive` with the session key, which restores agenda, todos,
  and notes through `workflow_manual`. Rejected: inlining agenda and todo
  contents (duplicates what revive already restores, and goes stale in the
  summary).
- **Deterministic-section budgets and shape.** The human-typed
  user-message section keeps messages newest-first within about 8k tokens,
  and caps any single message at about 1.5k tokens with a truncation marker
  so one pasted log cannot take the whole budget; older messages beyond the
  budget drop out of the section, and the lead's prose carries their gist.
  Every compaction recomputes the deterministic sections from the full
  session history (`branchEntries`), not from the previous summary, so they
  neither compound nor drift. Rejected: keeping every human message (grows
  without bound like Pi's file lists); no deterministic section, leaving it
  to the prose. The active playbook is carried as a pointer — its name and
  current step, to be re-read with `playbook.read` — not as a re-attached
  body. Rejected: re-attaching the body within a budget as Claude Code does
  for skills (costs tokens on every compaction, and `playbook.read` is cheap
  and always current for the session's overrides). Nothing else is
  re-attached after compaction: the Pi lead's system prompt already carries
  the root instructions, the Pi lead guide, and the workflow-manual
  snapshot.
- **The lead's prose has fixed headings and is rewritten each time.**
  Headings: user preferences and style; agreed working practices; decisions
  made in the session that no durable record holds yet; current work; the
  immediate next step, quoting the user's latest request verbatim; mood and
  rapport; residual details. Each compaction rewrites the prose from
  scratch, carrying forward what in the previous summary is still live and
  dropping what is resolved. The writing rule is: for content already
  persisted (tickets, commits, notes, agenda, todos), give its path or
  pointer; for content that lives only in the conversation, summarize it as
  precisely as possible. A soft length target of about 2-4k tokens guides
  the prose; there is no hard cap. Rejected: appending to the previous
  summary (Pi's "PRESERVE all existing information" is what made its
  summaries grow monotonically); free-form prose (sections silently go
  missing between compactions); a blacklist of what not to write.
- **Kept raw tail.** The compaction result uses
  `preparation.firstKeptEntryId` unchanged; Pi computes valid cut points and
  its `compaction.keepRecentTokens` setting tunes the size.
- **One lever.** `goal-compact-and-continue` is folded into the new lever:
  one tool and one summary shape, with `carry_forward` becoming the lead's
  prose; goal-loop keeps only its compaction hold/release and reminder
  logic. Rejected: a separate goal-only tool calling the new path.
- **The lever works without an active goal.** This reverses
  260913-bug-ws-pi-inactive-goal-compact-and-continue-aborts's inactive-goal
  rejection for the lever: lead compaction is not goal-specific, and
  trigger (b) lets the lead call it at any time. The goal-specific effects
  (goal re-injection after compaction and any goal-loop state the lever
  touches) apply only while a goal is active, so with no goal active the
  lever touches no goal-loop state, keeping 260913's no-side-effect
  property for those parts. Rejected: keeping the rejection (the lead could
  never compact through the lever outside a goal, contradicting triggers
  (a)-(c)).
- **Triggers.**
  - (a) Two thresholds. At the advisory threshold
    (`compaction_advisory_percent`, default 50) the adapter nudges the lead
    once, at a turn boundary (`agent_end`). At the hard threshold
    (`compaction_hard_percent`, default 80) the adapter does not wait for the
    run to settle: it sends a steer message, so at the next tool-call
    boundary the lead goes straight into the preparation sequence. The nudge
    fires once per threshold crossing and re-arms after a compaction; neither
    trigger fires while a preparation turn or a compaction is in progress;
    nothing re-nudges between the two, and if the lead ignores the hard-cut
    steer, Pi's automatic compaction and the fallback (d) are the backstop.
    Rejected: a single advisory knob at 50 for every session (no forcing
    point before Pi's own automatic compaction).
  - (b) The lead may call the lever autonomously at any time.
  - (c) A user `/compact` (Pi handles it in interactive mode before extension
    commands and before the `input` event, so only `session_before_compact`
    with `reason: "manual"` can intercept it) is answered with
    `{ cancel: true }` when it did not come from the lever, and the
    preparation turn is queued immediately carrying the `/compact` focus
    text. The user accepted Pi's resulting "Compaction cancelled" line.
  - (d) A Pi automatic or overflow compaction that fires before any
    preparation is handled inside the hook as a fallback: a separate LLM
    summary with the same template, without tools, plus the deterministic
    sections. The fallback call uses the session model through Pi's exported
    summary helpers; Pi has no summarization-model setting.
- **Competing compaction extensions.** When the entry stored after
  `session_compact` is not the adapter's (another extension's result won,
  since Pi keeps the last non-empty `session_before_compact` result), the
  adapter tells the user once per session to disable the competing
  extension. Rejected: documenting it only (a silently lost ws compaction is
  invisible).
- **No system- or developer-role injection.** ws messages stay user-role.
  Pi offers no role choice for injected messages, Anthropic has no
  mid-conversation system role, and raising untrusted subagent output above
  user authority would open a prompt-injection channel.
- **The preparation sequence is an adapter-owned guide.** Its text lives
  in `agents-plugin-pi/lead-compact-guide.md`, beside the adapter's other
  guides (`pi-lead-guide.md`, `execute-worker-guide.md`), and the advisory
  nudge, the hard-cut steer, and the `/compact` reroute carry its body
  directly in their message, ending with the call to the adapter's
  compaction lever. Only the Pi adapter owns a lead-callable lever: on
  Claude Code or Codex compaction is user-run, so a lead-side preparation
  would go stale before the user compacted, and nothing outside the Pi
  adapter points a lead at this sequence. Rejected: a shared `lead-compact`
  playbook in `agents-plugin/rsrc` with a Pi overlay for the final step
  (ships a body no other harness uses into every package and its mirrors,
  leaves a base file that only says to stop, and makes the hard cut spend
  an extra `playbook.read` round trip at high context; its one remaining
  benefit, `prompt.*` override of the text, is rarely needed); a Pi-only
  skill (breaks the 1:1 mirror between Pi skills and the shared ws skills).
- **Knob location.** `compaction_advisory_percent` and the new
  `compaction_hard_percent` (and any budget this ticket introduces) live in
  `goal-loop-config.json` for now;
  261002-feat-ws-config-adapter-schema-extension migrates them into ws-mcp
  config.

## Constraints

- Pi extension API facts the design relies on (`dist/core/extensions/types.d.ts`):
  `SessionBeforeCompactEvent { preparation, branchEntries, customInstructions?,
  reason: "manual" | "threshold" | "overflow", willRetry, signal }`;
  `SessionBeforeCompactResult { cancel?, compaction? }`; the last non-empty
  handler result wins across extensions; `ctx.compact({ customInstructions,
  onComplete, onError })` runs the manual path (`reason: "manual"`).
- The lead's compaction push-hold (`leadCompactingRef`) and goal-loop's
  release-after-compaction handling must keep working for every trigger.
- Reference for the summary shape: Claude Code's compaction (nine sections
  including "All user messages", an analysis scratchpad, and post-compaction
  re-attachment of root instructions, recent files, invoked skill bodies,
  and running background agents) — a reference, not a contract.
- Out of scope: microcompact-style clearing of old tool results before
  compaction (Pi `context` event); the Pi lead already exposes execution
  tools that keep large outputs out of context, so the need is small.
- Matching manuals from AGENTS.md `### Implementation Conventions`: none
  (`agents-plugin-pi/` has no declared row).

## Prior Decisions

- 260903-feat-ws-pi-goal-loop-compaction-hook (2026-09-04, Result): "`goal-compact-and-continue(carry_forward)` — a non-terminal `pi.registerTool` lever that calls `ctx.compact({ customInstructions: carry_forward })` once and returns without `disarmGoal()`." — bearing: constrains
- 260903-feat-ws-pi-goal-loop-compaction-hook (2026-09-04, commit ea11442e): "session_before_compact is observe-only because the installed Pi build offers no partial-inject hook on the auto/threshold path." — bearing: supports
- 260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction (2026-09-06, Result): "`leadCompactingRef` sits beside `leadIdleRef` in `spawner.ts`; it is set by the lever before `ctx.compact` and by `session_before_compact` for every compaction" — bearing: constrains
- 260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction (2026-09-06, commit 81463a7d): "`registerPushFlush`'s `agent_settled` flush is gated on `leadCompactingRef` because `ctx.compact()`'s internal `abort()` fires an `agent_settled` ... BEFORE Pi's own compaction flag is set" — bearing: constrains
- 260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction (2026-09-06, Result bfcf850b): "summary custom instructions are steering, not a verbatim delivery guarantee; preserve the raw string independently for model-visible reminder delivery." — bearing: constrains
- 260913-bug-ws-pi-inactive-goal-compact-and-continue-aborts (2026-09-13, Decisions): "Keep the guard in the authoritative lever path so tool calls and any equivalent adapter entry point cannot diverge." — bearing: contradiction-candidate
- 260909-feat-ws-pi-goal-stop-controls (2026-09-13, commit c6a87734): "Keep shared wake and compaction ownership independent: stopping goal scheduling does not release child-report reservations or prematurely clear in-flight compaction." — bearing: constrains
- 260905-chore-ws-pi-goal-announcement-wording (2026-09-05, Decisions): "Compaction lever result names the in-flight compaction ... `Compaction requested; the conversation will resume from a summary carrying: <carry_forward>`" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/lead-compact-guide.md (new), agents-plugin-pi/goal-loop-config.json, agents-plugin-pi/package.json files list |
| scope.surface | cross-module | lever tool replaces goal-compact-and-continue; session_before_compact handler (goal-loop.ts#L1056-L1062) interacts with leadCompactingRef in spawner.ts#L1060 |
| scope.new_public_symbol | yes | new compaction lever tool replacing goal-compact-and-continue; new config key compaction_hard_percent |
| scope.new_type_contract | yes | new CompactionResult summary shape and GoalLoopConfig field in goal-loop.ts#L90-L100 |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts exists; new test files likely for summary builders |
| complexity.reuse_points | confirmed | leadCompactingRef hold/release, resolveCompactionAdvisoryPercent, spawner child registry, Pi preparation.firstKeptEntryId |
| complexity.side_effect_risk | high | session_before_compact result replaces Pi compaction and a /compact cancel path; races with push-hold and goal re-arm are a known defect class |
| risk.correctness | high | summary assembly from branchEntries, budgets, trigger races, last-non-empty-result-wins across extensions |
| risk.fit | moderate | follows adapter-owned guide pattern (pi-lead-guide.md, execute-worker-guide.md) but folds an existing lever |
| risk.test | high | live Pi compaction path unverified in prior tickets; needs fake-pi harness for hooks and steer timing |
| risk.security_or_contract | moderate | ws messages stay user-role; filtering adapter-injected traffic from the user-message section must not leak or drop human text |

## Phases

### Phase 1: Lever, summary, and the preparation guide

Left to the implementer: the lever's tool name and parameter shape (one
prose field or one per fixed heading; the result wording pinned by
260905-chore-ws-pi-goal-announcement-wording is updated to match), where the
adapter reads the active ticket, phase, and playbook step, and, in Phase 2,
how context usage is measured (reuse `resolveCompactionAdvisoryPercent`'s
inputs) and how a lever-initiated manual compaction is told apart from a
user `/compact`.

- In `agents-plugin-pi`, replace `goal-compact-and-continue` with one
  compaction lever tool carrying the lead's prose. On a lever-initiated
  compaction the lead session's `session_before_compact` handler returns
  `{ compaction }` with the adapter-filled deterministic sections (session
  key, active ticket and phase, in-flight and recently finished children,
  human-typed user messages within the budgets above, active playbook
  pointer, closing `lead-revive` instruction), the lead's prose under the
  fixed headings, no file lists, and `preparation.firstKeptEntryId`. Spawned worker and explore sessions are
  untouched. `leadCompactingRef` and goal-loop's hold/release keep working.
- Add `agents-plugin-pi/lead-compact-guide.md`: tidy durable state into
  ws tooling, rewrite the prose under the fixed headings with the
  persisted-pointer / conversation-precise rule, then call the lever. Ship it in the
  package's published files.

Done when the `agents-plugin-pi` suite passes with tests showing the lever
path produces the summary shape above (no file lists, adapter-injected
traffic such as `ws-push-batch` reports, wake lines, goal reminders, and
skill expansions excluded from the user-message section, the budget
and per-message caps honored, finished children listed once, the sections
recomputed rather than inherited across two compactions), worker sessions
keep native compaction, the lever compacts with no active goal without
touching goal-loop state, and the hold/release
behavior is unchanged.

### Result (94cdf8bd2) - 2026-10-02

- `ws-compact` replaces `goal-compact-and-continue`: lead-only, one required
  string parameter per fixed prose heading, usable with or without a goal;
  goal effects (re-arm, carry) apply only while a goal is active.
- `src/lead-compaction.ts` builds the summary from `branchEntries` and the
  spawner registry: session key, active ticket and phase (newest
  ticket-naming lead tool call), playbook pointer (`ws-skill`,
  `ws__playbook_read`, or a `/skill:` expansion; `lead-revive` excluded),
  children in flight (owner-thread agents excluded) and finished since the
  previous compaction entry, human-typed user messages (push batches, wake
  lines, goal reminders excluded; skill expansions collapsed to the typed
  command; newest-first under the budget, shown chronologically, per-message
  cap with a truncation marker), the lead prose, and a closing `lead-revive`
  instruction. No file lists; Pi's `firstKeptEntryId` and `tokensBefore`
  pass through unchanged; details are stamped `ws-pi-lead-compaction`.
- Budgets are tunable via `compaction_user_messages_budget_tokens` and
  `compaction_user_message_cap_tokens` in `goal-loop-config.json`.
- The guide ships as `agents-plugin-pi/lead-compact-guide.md` (package
  `files`); `pi-lead-guide.md` points at `ws-compact`.
- Verification: `npm test` in `agents-plugin-pi`: 1975 pass, 0 fail,
  3 skipped (new `test/lead-compaction.test.ts`; goal-loop and push-wake
  suites migrated to the new lever).

### Phase 2: Triggers and backstops

Depends on Phase 1.

- Advisory nudge and hard-cut steer per the Triggers decision, with
  `compaction_hard_percent` (default 80) added to `goal-loop-config.json`;
  both carry the preparation guide's body.
- `/compact` reroute: a manual compaction not started by the lever returns
  `{ cancel: true }` and queues the preparation turn carrying the guide's body and
  the `/compact` focus text.
- Fallback for threshold and overflow compactions that arrive without
  preparation: an in-hook summary with the session model through Pi's
  exported summary helpers, the same template, and the deterministic
  sections.
- Competing-extension notice once per session when the stored compaction
  entry is not the adapter's.

Done when the `agents-plugin-pi` suite passes with tests covering the
nudge firing once per crossing and re-arming after compaction, the hard cut
delivered as a steer at a tool-call boundary, neither firing during
preparation or compaction, the `/compact` cancel-and-reroute with focus
text, the fallback summary shape, and the competing-extension notice.

### Result (f065de0da) - 2026-10-02

- Advisory nudge at `agent_end` (followUp with `triggerTurn`) and hard-cut
  steer at `turn_end` (`compaction_hard_percent`, default 80, added to
  `goal-loop-config.json` beside `compaction_advisory_percent: 50`). Each
  fires once per crossing, re-arms after a compaction or when usage is seen
  below the threshold, and is silent while a compaction or a preparation run
  is in progress. The preparation block clears at every `agent_end`, since
  Pi drains its queues before that event (review fix a5ff0b278).
- A manual compaction not started by the lever returns `{ cancel: true }`;
  after `session_compact_failed` releases the hold, the preparation turn is
  queued with the `/compact` focus text.
- Threshold and overflow compactions without lever prose get an in-hook
  fallback: `ctx.modelRegistry.complete` on the session model with
  `convertToLlm`/`serializeConversation`, the fixed-heading template, and
  the deterministic sections; a failure degrades to native compaction with
  a warning.
- The competing-extension notice fires once per session when the stored
  entry is not the adapter's.
- Review (lite): 1 Important (stuck preparation block) fixed in a5ff0b278;
  Minor cap clamp fixed; two Minors kept as design choices.
- Verification: `npm test` in `agents-plugin-pi`: 1984 pass, 0 fail,
  3 skipped.
