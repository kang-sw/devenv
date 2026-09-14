---
title: "Agent-driven branch navigation with summary after lead-local work"
related:
  260907-bug-ws-pi-fork-first-call-prompt-cache-miss: canonical fork solution remains the priority; this is an idea-only alternative, not a prerequisite or replacement
  260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role: dynamic tool loading and lead-local work would interact with the curated lead profile
---

# Agent-driven branch navigation with summary after lead-local work

## Background

The owner wants a lead-caliber agent to perform work with the lead's rich
context and return a summary without retaining all intermediate work in the
active conversation. Forks provide that shape, but a cache-breaking first
request over a large inherited prompt can cost roughly one dollar in the
reported dogfood cases.

Alternative idea: mark a point in the lead session, load the needed tools,
perform the work directly, then navigate back to that point with a summary
of only the intervening work. Preserve the earlier context instead of
compacting the entire conversation.

Owner decision: keep fork as the canonical solution. Capture this workaround
and its feasibility findings only in `idea/`; do not implement it or pivot
fork work to it. No fail-closed versus compatibility-fallback policy was
settled by this discussion.

## Feasibility findings — Pi 0.85.1

Read-only inspection of installed Pi documentation and source found a public
extension path. No live execution, cache measurement or paid probe was run.

The installed terminology is **branch summarization during `/tree`
navigation**, not a `rewind` API:

- `/tree` navigates within one session file and can summarize the branch
  being left.
- `/fork` creates a separate session without this branch summary.
- `/compact` performs context compaction rather than checkpoint-selected
  navigation.

Relevant public surfaces:

- `AgentSession.navigateTree(targetId, options)`.
- `ExtensionCommandContext.navigateTree(targetId, { summarize: true,
  customInstructions: ... })`.
- `session_before_tree`, which can supply a custom branch summary.
- Readonly session entry lookup through `ctx.sessionManager.getEntries()`,
  `getEntry()` and `getLabel()`; labels can be set with `pi.setLabel()`.

An LLM-callable tool receives `ExtensionContext`, which does not itself
expose navigation. The inspected extension bridge pattern is:

1. Save an appropriate checkpoint entry ID.
2. After the lead completes local work, a tool queues an extension command:
   `pi.sendUserMessage("/return-with-summary", { deliverAs: "followUp",
   expandPromptTemplates: true })`.
3. The tool returns `terminate: true` to avoid another model turn before the
   queued command. This requires every result in that tool batch to be
   terminating.
4. The command handler calls `await ctx.waitForIdle()`, then
   `ctx.navigateTree(checkpointId, { summarize: true, ... })`.
5. If continuation is wanted, send a new user prompt afterward; navigation
   alone does not start a model turn.

`/return-with-summary` is an illustrative command name, not an installed
command or an approved interface. This is a feasible API composition, not
an implemented or live-validated workflow.

## Context and lifecycle caveats

- Navigation rebuilds the active message context around the chosen point
  and appends a `branch_summary`. Earlier context remains; the old work
  branch stays in the append-only session tree and can be revisited.
- Navigation rejects while streaming, so an agent tool needs an idle
  command boundary rather than an immediate navigation call.
- Selecting a user entry restores that user message to the editor; select
  an appropriate assistant/non-user checkpoint if that user message must
  remain in the active context.
- `addedToolNames` is persisted on tool-result messages, but tree navigation
  does not visibly restore the active tool set. Removing the work branch
  from active context must not be assumed to restore pre-work schemas or
  preserve deferred-tool markers. Extra tool schemas can still invalidate
  a cached prefix. Dynamic tools also need registration again after reload.
- This is not a cache-hit or cost guarantee. Summary quality/cost, exact
  retained prefix and post-navigation tool state need an isolated probe
  before considering implementation.

## Evidence pointers

Inspection was against the installed `@earendil-works/pi-coding-agent`
package, version 0.85.1. Paths below are package-relative, not repository
source files:

- `docs/sessions.md`, Branch Summaries: tree navigation and summary behavior.
- `docs/compaction.md`, Compaction / Branch Summarization: distinction from
  whole-context compaction.
- `docs/extensions.md`, `ctx.waitForIdle`, `ctx.navigateTree`,
  `pi.sendUserMessage`: command-context navigation and queued command bridge.
- `examples/extensions/reload-runtime.ts`: tool-to-command/idle pattern.
- `dist/core/agent-session.d.ts` and `dist/core/agent-session.js`:
  `navigateTree`, streaming guard and message-context rebuild.
- `docs/session-format.md`: `branch_summary` and append-only session tree.
- The installed `pi-agent-core` type declarations: `addedToolNames` on tool
  results; compare active-tool state separately from session messages.

This capture establishes only that a public extension route exists. Fork
implementation and its own verification remain independent and canonical.
