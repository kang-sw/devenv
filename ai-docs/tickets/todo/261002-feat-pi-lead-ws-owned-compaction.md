---
title: ws-owned lead compaction for the Pi adapter
related:
  260903-feat-ws-pi-goal-loop-compaction-hook: predecessor; its observe-only session_before_compact stance and goal-compact-and-continue lever are superseded for the lead session
  261002-feat-ws-config-adapter-schema-extension: dependent; migrates this ticket's knobs from goal-loop-config.json into ws-mcp config
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
  where the agenda and todos live; in-flight child agents from the spawner
  registry; human-typed user messages (the analogue of Claude Code's "All
  user messages" section), excluding adapter-injected traffic such as
  `ws-push-batch` reports, wake lines, goal reminders, and skill expansions;
  and the active playbook. The summary carries no file lists. Hook results
  are stored with `fromHook=true`, which also ends Pi's file-list
  inheritance chain.
- **Deterministic-section budgets and shape.** The human-typed
  user-message section keeps messages newest-first within about 8k tokens;
  older messages beyond the budget drop out of the section, and the lead's
  prose and earlier summaries carry their gist. Rejected: keeping every human
  message (grows without bound like Pi's file lists); no deterministic
  section, leaving it to the prose. The active playbook is carried as a
  pointer — its name and current step, to be re-read with `playbook.read` —
  not as a re-attached body. Rejected: re-attaching the body within a budget
  as Claude Code does for skills (costs tokens on every compaction, and
  `playbook.read` is cheap and always current for the session's overrides).
- **Kept raw tail.** The compaction result uses
  `preparation.firstKeptEntryId` unchanged; Pi computes valid cut points and
  its `compaction.keepRecentTokens` setting tunes the size.
- **One lever.** `goal-compact-and-continue` is folded into the new lever:
  one tool and one summary shape, with `carry_forward` becoming the lead's
  prose; goal-loop keeps only its compaction hold/release and reminder
  logic. Rejected: a separate goal-only tool calling the new path.
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

## Phases

### Phase 1: Lever, summary, and the preparation guide

- In `agents-plugin-pi`, replace `goal-compact-and-continue` with one
  compaction lever tool carrying the lead's prose. On a lever-initiated
  compaction the lead session's `session_before_compact` handler returns
  `{ compaction }` with the adapter-filled deterministic sections (session
  key, active ticket and phase, agenda/todo location, in-flight children,
  human-typed user messages newest-first within about 8k tokens, active
  playbook pointer), the lead's prose, no file lists, and
  `preparation.firstKeptEntryId`. Spawned worker and explore sessions are
  untouched. `leadCompactingRef` and goal-loop's hold/release keep working.
- Add `agents-plugin-pi/lead-compact-guide.md`: tidy durable state into
  ws tooling, write the prose sections, then call the lever. Ship it in the
  package's published files.

Done when the `agents-plugin-pi` suite passes with tests showing the lever
path produces the summary shape above (no file lists, adapter-injected
traffic such as `ws-push-batch` reports, wake lines, goal reminders, and
skill expansions excluded from the user-message section, the budget
honored), worker sessions keep native compaction, and the hold/release
behavior is unchanged.

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
