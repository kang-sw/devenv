/**
 * 260904 Phase 2 (`260904-feat-ws-pi-side-thread-fork-question-surface`):
 * the OWNER-question surface — `ws-ask`/`ws-resolve`, the persisted thread
 * registry, `/thread`, `/answer <id>`, the lazy discussion fork, and the
 * `/done` summary injection.
 *
 * 260905 (`260905-feat-ws-pi-live-agent-widget`): this module's own `N
 * pending` `aboveEditor` widget is gone — a pending/open thread is now a row
 * in `agent-widget.ts`'s merged `belowEditor` live-agent panel. Every former
 * widget-refresh call site here now fires `spawner.ts`'s
 * `agentWidgetRefreshRef` instead (see `refreshAgentWidget` below), so this
 * module stays free of any direct `agent-widget.ts` import.
 *
 * Two entries reach the same registry (§1):
 *   - Entry B (`ws-ask`): the lead registers a question and returns
 *     immediately. NOTHING is spawned at ask time — an unopened question
 *     costs nothing (§9). `/answer <id>` is what spawns a discussion fork,
 *     forked at the lead's tip AT OPEN TIME (not ask time), and attaches the
 *     overlay chat to it.
 *   - Entry A meeting Entry B (`ws-report-to-lead(kind:"question")` from a
 *     task fork): `fork.ts`'s new `onQuestion` seam calls
 *     `handleForkRaisedQuestion` here, which registers a thread whose
 *     `respondent` is ALREADY that live fork. `/answer` then attaches to the
 *     live fork — it never spawns a second one. There is no fork-less
 *     quick-answer path.
 *
 * A legacy discussion-fork prompt is deliberately not wrapped in Entry A's
 * task frame (`buildForkInitialMessage`): it speaks as the lead, so persona
 * continuity rather than task isolation is the feature.
 *
 * Persistence (§5): the registry is written to a sibling file of the lead's
 * own session file (`<sessionFile>.ws-threads.json`), so pending questions
 * and dormant threads survive a lead restart. That is strictly wider than
 * what `spawner.ts` itself persists — its `RpcAgentRegistry` is an in-memory
 * `Map` that does not survive a lead-process restart, contrary to the
 * ticket's §5 assumption. Rather than broaden `spawner.ts`'s own registry
 * persistence (a much larger change than this ticket asks for), each thread
 * carries a denormalized `PersistedForkResume` copy of exactly the fields
 * needed to hand-reconstruct an `RpcAgentRecord`, and `/answer` lazily
 * rehydrates one into `rpcRegistry` the first time a dormant thread is
 * reopened after a restart. `sendToAgent`'s existing dormant-auto-resume
 * branch then does the actual relaunch — no resume logic is reimplemented
 * here.
 *
 * Golden rule / placement: this module imports FROM `spawner.ts`,
 * `fork.ts`, `process-role.ts` and `conversation-view.ts` only, never the
 * reverse (`fork.ts` duplicates the two tool-name literals for exactly this
 * reason — see its `FORK_EXCLUDED_TOOL_NAMES` comment).
 * `agents-plugin-tool/` (ws-mcp Go) and `agents-plugin/skills/` canonical
 * text are untouched.
 *
 * Pure helpers below are unit-tested directly (`test/ask.test.ts`) with no
 * filesystem/subprocess/live `pi` session, and so are `registerAsk`'s two
 * tool bodies and `injectDiscussionSummary` (neither spawns anything — a
 * fake `pi` plus a duck-typed `toolCtx` is enough, the same shape
 * `createApprovalRelay` is tested in). Only the genuinely live glue
 * (`registerThreadCommands`, the lazy discussion-fork spawn, the overlay
 * attach) is left to the plan's tmux/owner-runbook gates.
 *
 * `260911-feat-ws-pi-async-question-queue` Phase 1 (fork-less lead-raised
 * redesign, resolving `260908`'s cost concern by redesign rather than
 * removal): `ws-ask`/`ws-resolve` are renamed `ws-queue-question`/
 * `ws-withdraw-question` (D4 — a contract change, not cosmetic: the old name
 * drove the model toward "ask now"). Entry B above is now genuinely
 * fork-less end to end — `/answer` on a `"lead-ask"` thread never reaches
 * `ensureRespondent`'s discussion-fork spawn branch at all (see
 * `openLeadAskQueue`, dispatched from `openThread` — Phase 2's sequential
 * prose-modal tier, superseding Phase 1's interim `openLeadAskThread`); the
 * owner's one prose reply is delivered straight to the lead via
 * `deliverQueuedAnswer` (the existing `followUp` custom-message path,
 * `sendToLead`, unchanged), carrying
 * the D3 return-path anchor (`ThreadRecord.askCommitHash` + the existing
 * `entryId`, plus a verbatim excerpt when that entry has since fallen behind
 * a compaction boundary) so the lead can recover where the question came
 * from. The pre-redesign discussion-fork machinery this bypasses
 * (`ensureRespondent`'s spawn branch, `buildDiscussionForkDirectiveText`/
 * `buildDiscussionForkInitialMessage`, `resolveDoneAction`/
 * `summarizeThenClose`/`runDoneAction`'s "summarize" branch,
 * `closeThreadOnDone`'s `"lead-ask"` branch)
 * is deliberately left in place rather than deleted: no live code path can
 * reach it for a `"lead-ask"` thread anymore (fork-raised always already has
 * a `respondentAgentId` at registration, so those branches were only ever
 * reachable for `"lead-ask"`), but it is cheap, well-tested insurance against
 * a persisted pre-redesign registry entry rather than churn worth the risk
 * of removing. The `ws-withdraw-question` model-side withdrawal/concurrency
 * contract (`withdrawQueuedQuestion`) is new and `"lead-ask"`-specific;
 * `"fork-raised"` keeps its original unconditional-close behavior verbatim.
 * The fork-raised path, its overlay, and `openThread`'s existing tail are
 * otherwise untouched — this ticket is scoped to the lead-raised path only.
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { sendToLead, sendToAgent, startForkFinish, syncOwnershipProtection, stopAgent, type RpcAgentRecord, type RpcAgentRegistry } from "./spawner.ts";
import { readSpawnRole } from "./process-role.ts";
import type { ChildLiveness, ConversationChannel } from "./conversation-view.ts";
import { createAuditChannel } from "./audit.ts";
import { nowIso, nextThreadId, deriveThreadTitle, captureForkResume, persistThreads, refreshAgentWidget, ASK_TOOL_NAME, getLeafEntryId, captureAskCommitHash, checkContextLength, notify, RESOLVE_TOOL_NAME, THREAD_SUMMARY_CUSTOM_TYPE, buildInjectionMessage, isEntryLive, buildVerbatimExcerpt, buildAskAnchorLine, buildQueuedAnswerInjectionMessage, resolveOwnerSendInterrupt } from "./ask-threads.ts";
import type { ThreadRegistryHandle, ThreadRecord, AskUiCtx, AskSessionCtx } from "./ask-threads.ts";
import { bindThread, repaintActiveQueue } from "./ask-overlay.ts";
export { ASK_TOOL_NAME, RESOLVE_TOOL_NAME, THREAD_SUMMARY_CUSTOM_TYPE, EXCERPT_WINDOW, EXCERPT_ENTRY_CHARS, MAX_CONTEXT_CHARS, checkContextLength, formatSpawnTime, buildDoneSummaryPrompt, EMPTY_SUMMARY_TEXT, buildForkQuestionLeadNotice, normalizeThreadOrigin, buildThreadHeaderHint, nextThreadId, deriveThreadTitle, threadRegistryPath, serializeThreadRegistry, parseThreadRegistry, countPending, buildThreadListLines, mostRecentReopenable, isEntryLive, extractEntryText, buildVerbatimExcerpt, addAskToolsIfLead, buildDiscussionForkDirectiveText, buildDiscussionForkInitialMessage, buildInjectionMessage, buildQueuedAnswerInjectionMessage, captureForkResume, rehydrateForkRecord, resolveOwnerSendInterrupt, getLeafEntryId, captureAskCommitHash, buildAskAnchorLine, createThreadRegistryHandle, loadThreadRegistryFile, saveThreadRegistryFile, hydrateThreadRegistry } from "./ask-threads.ts";
export type { OverlayHandle, ThreadRecord, ThreadStatus, ThreadOrigin, PersistedForkResume, AskUiCtx, ThreadRegistryHandle, AskSessionCtx } from "./ask-threads.ts";
export { ensureRespondent, resolveDoneAction, summarizeThenClose, runDoneAction, buildOverlayHandle, resolveLeadAskEscapeAction, resolveLeadAskQueueEntryAction, runLeadAskEscapeAction, buildLeadAskQueueOnClose, collectLeadAskQueue, countQueueAnswered, buildQueueCoverageLine, buildQueueSubmitConfirmMessage, LeadAskQueueComponent, registerThreadCommands } from "./ask-overlay.ts";
export type { DoneAction, LeadAskEscapeAction, FocusableEditorLike, LeadAskQueueOptions } from "./ask-overlay.ts";

/**
 * 260904 Phase 2, Entry A meeting Entry B: the callback handed to
 * `registerFork` as its `onQuestion`. Registers a thread whose respondent is
 * ALREADY the live fork that raised the question, so `/answer` attaches to
 * that fork rather than spawning a second one. `entryId` is deliberately
 * absent — the lead never authored an entry for a fork-raised question, so
 * there is nothing to anchor (§7).
 */
export function handleForkRaisedQuestion(
  handle: ThreadRegistryHandle,
  rpcRegistry: RpcAgentRegistry,
  agentId: string,
  message: string,
  /** Retained for call-site compatibility; registration itself owns no IO. */
  _pi?: ExtensionAPI,
): ThreadRecord {
  const now = nowIso();
  const record: ThreadRecord = {
    threadId: nextThreadId([...handle.threads.keys()]),
    title: deriveThreadTitle(message),
    question: message,
    status: "pending",
    origin: "fork-raised",
    respondentAgentId: agentId,
    createdAt: now,
    touchedAt: now,
  };
  const live = rpcRegistry.get(agentId);
  if (live) {
    record.forkResume = captureForkResume(live);
    // 260905: the thread is bound from REGISTRATION, not from overlay open —
    // the exchange belongs to the owner from the moment the fork raised it,
    // so settled output routes to the owner when that surface exists (and the
    // fork is not counted as one of the lead's children) even before the owner
    // gets around to `/answer`.
    bindThread(rpcRegistry, agentId, true);
  }
  handle.threads.set(record.threadId, record);
  persistThreads(handle);
  refreshAgentWidget();
  return record;
}

/**
 * Registers `ws-ask`/`ws-resolve` (lead-facing; since the owner-question
 * wiring was disconnected no role activates them — `computeSessionBootstrap`
 * filters them out of the top lead's active tools). Registered
 * declaratively/globally like `registerFork`, so a fork child's own
 * `computeForkToolSurface` has these names present to exclude.
 *
 * `ws-ask` REGISTERS ONLY — no spawn (§1/§9). The discussion fork is spawned
 * lazily by `/answer`, at the lead's tip at OPEN time.
 */
/** `withdrawQueuedQuestion`'s result, echoed in the tool's own JSON reply. */
export type WithdrawOutcome = "removed" | "deferred" | "no-op";

/**
 * `ws-withdraw-question`'s model-side withdrawal, keyed on the thread's
 * CURRENT owner-facing state. Both this call and the owner's answer view run
 * in the same adapter process, so the two are already serialized — no
 * separate lock is needed.
 *
 * - `"fork-raised"`: unchanged from the pre-`260911` `ws-resolve` — an
 *   unconditional immediate close, releasing the thread-lifetime bind so the
 *   respondent rejoins the lead's fan-in. This path is out of `260911`'s
 *   scope.
 * - `"lead-ask"` `"pending"` (no answer view open): removed immediately —
 *   the widget's pending count decrements right away.
 * - `"lead-ask"` `"open"` (the owner has the fork-less answer view open right
 *   now): the removal cannot be applied without yanking an in-progress edit,
 *   so it is deferred (`withdrawnPending`) to whenever that view closes
 *   (Phase 2's `LeadAskQueueComponent.onClose`, via
 *   `runLeadAskEscapeAction`) — an already-typed, unsubmitted answer is
 *   still delivered to the lead at that point, never silently discarded.
 * - `"lead-ask"` `"dormant"`/`"closed"` (already answered, or already
 *   withdrawn): a no-op — the answer, if any, was already injected.
 */
export function withdrawQueuedQuestion(
  handle: ThreadRegistryHandle,
  rpcRegistry: RpcAgentRegistry | undefined,
  thread: ThreadRecord,
): WithdrawOutcome {
  if (thread.origin === "fork-raised") {
    thread.status = "closed";
    thread.touchedAt = nowIso();
    if (thread.respondentAgentId && rpcRegistry) bindThread(rpcRegistry, thread.respondentAgentId, false);
    persistThreads(handle);
    refreshAgentWidget();
    return "removed";
  }
  if (thread.status === "dormant" || thread.status === "closed") return "no-op";
  if (thread.status === "open") {
    thread.withdrawnPending = true;
    thread.touchedAt = nowIso();
    persistThreads(handle);
    refreshAgentWidget();
    // Phase 2 (260911): if a batch queue modal has this thread open right
    // now, repaint it immediately so the non-destructive withdrawal banner
    // shows up without waiting for the owner's next keystroke.
    repaintActiveQueue();
    return "deferred";
  }
  // "pending" — never opened.
  thread.status = "closed";
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshAgentWidget();
  return "removed";
}

export function registerAsk(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  rpcRegistry?: RpcAgentRegistry,
  toolPreviewTuiRef: ToolPreviewTuiRef = createToolPreviewTuiRef(),
): void {
  registerWsTool(pi, {
    name: ASK_TOOL_NAME,
    label: ASK_TOOL_NAME,
    description:
      "Queue a question for the owner to answer async, without blocking or interrupting them. Returns {question_id} immediately and spawns nothing. The owner opens it themselves with /answer <id> whenever they're ready and replies in prose; their reply is delivered to you as an injected message once you're idle — no discussion thread, no live back-and-forth. Use it for a decision only the owner can make; keep working on anything that does not depend on the answer. Call ws-withdraw-question if you work the answer out yourself before they open it.",
    parameters: {
      type: "object",
      properties: {
        title: { type: "string", description: "Short label for the owner's thread list — a few words, not a sentence." },
        question: { type: "string", description: "The question itself, phrased so the owner can answer it without re-reading the session." },
        context: {
          type: "string",
          description:
            "2-3 sentences of your own words giving the owner just enough background to answer. No file paths, no commit hashes, no line numbers.",
        },
      },
      required: ["title", "question"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      if (readSpawnRole(process.env) === "fork") {
        throw new Error(`ws-pi-agent: ${ASK_TOOL_NAME} is unavailable in a fork; report to the lead instead.`);
      }
      const p = params as { title: string; question: string; context?: string };
      const now = nowIso();
      const record: ThreadRecord = {
        threadId: nextThreadId([...handle.threads.keys()]),
        title: p.title,
        question: p.question,
        context: p.context,
        entryId: getLeafEntryId(toolCtx),
        // D3 return-path anchor: best-effort, never blocks registration.
        askCommitHash: captureAskCommitHash(process.cwd()),
        status: "pending",
        origin: "lead-ask",
        createdAt: now,
        touchedAt: now,
      };
      handle.threads.set(record.threadId, record);
      persistThreads(handle);

      const ctx = toolCtx as AskUiCtx | undefined;

      // §7: the context is bounded by an adapter-side warning, never by
      // truncation — `record.context` above already stored the lead's text
      // unchanged.
      const overage = checkContextLength(p.context);
      if (overage) notify(ctx, overage, "warning");

      if (ctx?.mode === "tui") {
        refreshAgentWidget();
      } else {
        // §8 headless baseline: no widget, no discussion fork — just a
        // fire-and-forget notify. The owner's answer then arrives as an
        // ordinary lead turn.
        notify(ctx, `ws: question ${record.threadId} registered for the owner — ${record.title}`, "info");
      }

      return { content: [{ type: "text", text: JSON.stringify({ question_id: record.threadId }) }] };
    },
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: RESOLVE_TOOL_NAME,
    label: RESOLVE_TOOL_NAME,
    description:
      "Withdraw a question you queued with ws-queue-question because you no longer need the owner's answer. A still-pending question is removed from their count immediately; one the owner already has open is only removed once they close it (an in-progress edit is never yanked, and any content they already typed is still delivered to you); one already answered is a no-op. Never notifies the owner and injects nothing on its own — you already know the answer.",
    parameters: {
      type: "object",
      properties: {
        question_id: { type: "string", description: "The question_id ws-queue-question returned." },
      },
      required: ["question_id"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, _toolCtx) {
      if (readSpawnRole(process.env) === "fork") {
        throw new Error(`ws-pi-agent: ${RESOLVE_TOOL_NAME} is unavailable in a fork; report to the lead instead.`);
      }
      const p = params as { question_id: string };
      const record = handle.threads.get(p.question_id);
      if (!record) {
        throw new Error(`ws-pi-agent: ${RESOLVE_TOOL_NAME}: unknown question_id "${p.question_id}"`);
      }
      const outcome = withdrawQueuedQuestion(handle, rpcRegistry, record);
      return { content: [{ type: "text", text: JSON.stringify({ question_id: record.threadId, status: record.status, outcome }) }] };
    },
  }, toolPreviewTuiRef);
}

/**
 * The thread's `/done` close, routed on its `origin`. The two entries own
 * their respondent differently (review relay #2
 * C2, see `ThreadOrigin`):
 *
 * - `"lead-ask"`: this surface spawned the discussion fork, so `/done` runs
 *   the full §6/§9 close — summary into the lead, then stop the fork.
 * - `"fork-raised"`: the respondent is a LIVE Entry A task fork the lead is
 *   parked on. Stopping it would destroy its in-flight task and hang the
 *   lead's own delegation of it, so `/done` only
 *   starts a fresh lead-owned Finish handoff whose ordinary settled answer is
 *   delivered through the shared terminal lifecycle.
 */
export function closeThreadOnDone(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  rpcRegistry: RpcAgentRegistry,
  thread: ThreadRecord,
  summary: string,
  resumeCtx?: AskSessionCtx,
): void {
  if (thread.origin === "lead-ask") {
    injectDiscussionSummary(pi, handle, rpcRegistry, thread, summary);
    return;
  }
  finishForkRaisedThread(pi, handle, rpcRegistry, thread, resumeCtx);
}

/** `/done` on a fork-raised thread: close the view immediately, then let the
 * shared same-process coordinator wait for/obtain one terminal outcome. */
function finishForkRaisedThread(pi: ExtensionAPI, handle: ThreadRegistryHandle, rpcRegistry: RpcAgentRegistry, thread: ThreadRecord, resumeCtx?: AskSessionCtx): void {
  const record = thread.respondentAgentId ? rpcRegistry.get(thread.respondentAgentId) : undefined;
  if (!record) {
    // A missing live record has no in-process lifecycle to reconcile.
    detachForkRaisedThread(handle, rpcRegistry, thread);
    return;
  }
  // A completed coordinator has already made this thread dormant and
  // released its temporary bind. A repeated `/done` is then a no-op, not a
  // fresh closeout against the parked record.
  if (thread.status === "dormant" && !record.threadBound) return;
  if (record.forkFinish) return;

  bindThread(rpcRegistry, record.agentId, true);
  thread.status = "dormant";
  thread.touchedAt = nowIso();
  thread.forkResume = captureForkResume(record);
  persistThreads(handle);
  refreshAgentWidget();

  record.onForkFinishComplete = (finished, failure) => {
    // A replacement send can supersede this operation; only its own callback
    // may release the temporary bind and refresh this thread snapshot.
    finished.threadBound = false;
    syncOwnershipProtection(finished);
    thread.forkResume = captureForkResume(finished);
    thread.status = "dormant";
    thread.touchedAt = nowIso();
    persistThreads(handle);
    refreshAgentWidget();
    if (failure) notify(handle.ctxRef.current, `ws: fork finish for ${thread.threadId} ended without confirmed terminal admission (${failure}).`, "warning");
  };
  startForkFinish(record, rpcRegistry, pi, {
    cwd: resumeCtx?.cwd ?? process.cwd(),
    extensionPath: resumeCtx?.extensionPath ?? process.argv[1] ?? "",
    leadSessionKey: resumeCtx?.leadSessionKeyRef?.current,
  });
}

/** Release a fork-raised thread bind when its respondent is gone or the thread is withdrawn. */
export function detachForkRaisedThread(handle: ThreadRegistryHandle, rpcRegistry: RpcAgentRegistry, thread: ThreadRecord): void {
  const agentId = thread.respondentAgentId;
  if (agentId) {
    const record = rpcRegistry.get(agentId);
    if (record) {
      // The thread itself is closing here, so the thread-lifetime bind is
      // released too and any later turn belongs to the lead route.
      record.threadBound = false;
      syncOwnershipProtection(record);
      // Refresh the resume snapshot while the record is still live, so a
      // reopen after a lead restart can rehydrate it.
      thread.forkResume = captureForkResume(record);
    }
  }
  thread.status = "dormant";
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshAgentWidget();
}

/** Send the summary through the shared hold/wake path. Thread close, respondent stop, snapshot, and persistence remain immediate even when delivery waits. */
export function injectDiscussionSummary(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  rpcRegistry: RpcAgentRegistry,
  thread: ThreadRecord,
  summary: string,
): void {
  const message = {
    customType: THREAD_SUMMARY_CUSTOM_TYPE,
    content: buildInjectionMessage(thread.context, thread.question, summary),
    display: true,
    details: { threadId: thread.threadId, title: thread.title },
  };
  // The shared path wakes idle leads through user preflight. `followUp`
  // remains this summary's busy-time admission mode; confirmed start releases
  // the shared held batch as steering. Thread side effects stay immediate.
  sendToLead(pi, message, "followUp");

  const agentId = thread.respondentAgentId;
  if (agentId) {
    const record = rpcRegistry.get(agentId);
    if (record) {
      record.threadBound = false;
      syncOwnershipProtection(record);
      // Snapshot first: `stopAgent` clears `client`, and a later reopen needs
      // the session/tool fields this copy carries.
      thread.forkResume = captureForkResume(record);
    }
    // Best effort — a failed stop must not lose the summary the owner just
    // produced, nor strand the thread in "open". `silent: true`: this stop is
    // an internal consequence of the owner's `/done`, and the lead already
    // received the decision as the `ws-thread-summary` message above — a
    // `ws-agent-settled` push on top would be the same event twice.
    void stopAgent(rpcRegistry, agentId, pi, { silent: true }).catch(() => undefined);
  }

  thread.status = "dormant";
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshAgentWidget();
}

/**
 * `260911` D1: delivers a `"lead-ask"` thread's owner-authored answer
 * straight to the lead — fork-less end to end, so there is no respondent to
 * stop or summarize (contrast `injectDiscussionSummary` above, kept for the
 * pre-redesign path this bypasses — see this file's header). Sent through
 * the same `followUp` custom-message path (`sendToLead`, unchanged from
 * `260904` §6), extended with the D3 return-path anchor (ask-time short
 * commit hash / `entry_id`) and, when that entry has since fallen behind a
 * compaction boundary, the verbatim excerpt around it (the same
 * `isEntryLive`/`buildVerbatimExcerpt` mechanism `ensureRespondent` used to
 * run at fork-SPAWN time — now run here, at answer-DELIVERY time, since a
 * fork-less thread never spawns).
 *
 * Terminal state is `"dormant"` (delivered, retained) regardless of how
 * delivery was triggered — a normal submit, or the queue modal's exit path
 * still-delivering an in-progress, already-typed answer behind a deferred
 * model withdrawal (`ThreadRecord.withdrawnPending`, cleared here).
 */
export type LeadAskSessionManager = {
  buildContextEntries?: () => { id: string }[];
  getBranch?: (id: string) => { id: string }[];
};

function queuedAnswerContent(thread: ThreadRecord, answer: string, sessionManager?: LeadAskSessionManager): string {
  let excerpt: string | undefined;
  if (thread.entryId && sessionManager) {
    try {
      const liveEntries = sessionManager.buildContextEntries?.() ?? [];
      if (!isEntryLive(thread.entryId, liveEntries)) {
        excerpt = buildVerbatimExcerpt(thread.entryId, sessionManager.getBranch?.(thread.entryId) ?? []);
      }
    } catch {
      // A session-tree read failure must not block delivering the answer —
      // the lead simply gets it without the excerpt.
    }
  }
  const anchor = buildAskAnchorLine(thread.askCommitHash, thread.entryId);
  return buildQueuedAnswerInjectionMessage(thread.context, thread.question, answer, anchor, excerpt);
}

/**
 * Deliver one modal submission as one lead follow-up. Entries must already be
 * in deterministic queue order; blank questions are excluded by the caller.
 * The single-answer shape retains the original details contract, while a
 * multi-answer submission carries ordered ids/titles beside one combined
 * model payload.
 */
export function deliverQueuedAnswers(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  entries: readonly { thread: ThreadRecord; answer: string }[],
  sessionManager?: LeadAskSessionManager,
): void {
  const deliverable = entries.filter(({ answer }) => answer.trim().length > 0);
  if (deliverable.length === 0) return;
  const content = deliverable.map(({ thread, answer }) => queuedAnswerContent(thread, answer, sessionManager)).join("\n\n---\n\n");
  const details = deliverable.length === 1
    ? { threadId: deliverable[0].thread.threadId, title: deliverable[0].thread.title }
    : { threadIds: deliverable.map(({ thread }) => thread.threadId), titles: deliverable.map(({ thread }) => thread.title) };
  sendToLead(pi, { customType: THREAD_SUMMARY_CUSTOM_TYPE, content, display: true, details }, "followUp");

  const touchedAt = nowIso();
  for (const { thread } of deliverable) {
    thread.status = "dormant";
    thread.withdrawnPending = false;
    thread.draftAnswer = undefined;
    thread.touchedAt = touchedAt;
  }
  persistThreads(handle);
  refreshAgentWidget();
}

export function deliverQueuedAnswer(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  thread: ThreadRecord,
  answer: string,
  sessionManager?: LeadAskSessionManager,
): void {
  deliverQueuedAnswers(pi, handle, [{ thread, answer }], sessionManager);
}

/**
 * The overlay's send path. Every owner message goes through `sendToAgent`,
 * which already owns the whole branch table this surface needs: dormant ->
 * relaunch via `--session` then `prompt()`, live-idle -> `prompt()`,
 * live-streaming -> `steer()` when interrupting. §5's "prompt() when the
 * fork is waiting, steer() when it is running" is therefore expressed as the
 * `interrupt` flag, not as a second implementation of the same branch.
 *
 * The event subscription is re-synced on every send because a dormant thread
 * has no `client` at open time — it only gets one once `sendToAgent`
 * relaunches the child.
 */
/** Pure owner-steering liveness decision retained as a test seam. */
export function resolveChildLiveness(running: boolean, ownerHeld = false): ChildLiveness {
  return running ? "running" : ownerHeld ? "idle-awaiting-owner" : "settled";
}

export function createForkChannel(
  pi: ExtensionAPI,
  rpcRegistry: RpcAgentRegistry,
  sessionCtx: AskSessionCtx,
  agentId: string,
  onSent?: (record: RpcAgentRecord) => void,
): ConversationChannel {
  return createAuditChannel(rpcRegistry, agentId, async (text) => {
    const record = rpcRegistry.get(agentId);
    await sendToAgent(rpcRegistry, { pi, cwd: sessionCtx.cwd, extensionPath: sessionCtx.extensionPath, leadSessionKey: sessionCtx.leadSessionKeyRef?.current, writer: "owner" }, agentId, text, resolveOwnerSendInterrupt(record?.streaming === true));
    const updated = rpcRegistry.get(agentId);
    if (updated) onSent?.(updated);
  });
}
