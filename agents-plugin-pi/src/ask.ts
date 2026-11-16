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
import { getMarkdownTheme } from "@earendil-works/pi-coding-agent";
import type { BridgeHandle } from "./bridge.ts";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { modelCatalogFromToolCtx, tierWarningNotifierFromToolCtx } from "./model-catalog.ts";
import { sendToLead, inheritModelFromToolCtx, leadParentPolicy, sendToAgent, spawnAgent, storageContextFromToolCtx, startForkFinish, syncOwnershipProtection, stopAgent, type RpcAgentRecord, type RpcAgentRegistry } from "./spawner.ts";
import { computeForkToolSurface, getForkSourceSessionFile } from "./fork.ts";
import { readSpawnRole } from "./process-role.ts";
import { ConversationViewComponent, conversationOverlayHeight, isEscapeKey, wrapInBorder, type ChildLiveness, type ConversationChannel, type ConversationViewTui, type EditorLike } from "./conversation-view.ts";
import { loadHostPiTui, type Component, type EditorTheme, type MarkdownTheme } from "./pi-tui.ts";
import { captureForkContext, captureRegisteredTools, captureUnflushedForkSource, effectiveForkDescriptor } from "./fork-context.ts";
import { removedAgentMessage, validDescriptor } from "./agent-storage.ts";
import { isRemovedAgent } from "./agent-cost.ts";
import { OwnerSteeringComponent, activateOwnerOverlay, clearOwnerOverlay, createAuditChannel, readSessionHistory, reserveOwnerOverlay } from "./audit.ts";
import { nowIso, nextThreadId, deriveThreadTitle, captureForkResume, persistThreads, refreshAgentWidget, ASK_TOOL_NAME, getLeafEntryId, captureAskCommitHash, checkContextLength, notify, RESOLVE_TOOL_NAME, THREAD_SUMMARY_CUSTOM_TYPE, buildInjectionMessage, isEntryLive, buildVerbatimExcerpt, buildAskAnchorLine, buildQueuedAnswerInjectionMessage, resolveOwnerSendInterrupt, rehydrateForkRecord, buildDiscussionForkInitialMessage, EMPTY_SUMMARY_TEXT, buildDoneSummaryPrompt, buildThreadHeaderHint, buildThreadListLines, mostRecentReopenable } from "./ask-threads.ts";
import type { ThreadRegistryHandle, ThreadRecord, AskUiCtx, AskSessionCtx, OverlayHandle } from "./ask-threads.ts";
export { ASK_TOOL_NAME, RESOLVE_TOOL_NAME, THREAD_SUMMARY_CUSTOM_TYPE, EXCERPT_WINDOW, EXCERPT_ENTRY_CHARS, MAX_CONTEXT_CHARS, checkContextLength, formatSpawnTime, buildDoneSummaryPrompt, EMPTY_SUMMARY_TEXT, buildForkQuestionLeadNotice, normalizeThreadOrigin, buildThreadHeaderHint, nextThreadId, deriveThreadTitle, threadRegistryPath, serializeThreadRegistry, parseThreadRegistry, countPending, buildThreadListLines, mostRecentReopenable, isEntryLive, extractEntryText, buildVerbatimExcerpt, addAskToolsIfLead, buildDiscussionForkDirectiveText, buildDiscussionForkInitialMessage, buildInjectionMessage, buildQueuedAnswerInjectionMessage, captureForkResume, rehydrateForkRecord, resolveOwnerSendInterrupt, getLeafEntryId, captureAskCommitHash, buildAskAnchorLine, createThreadRegistryHandle, loadThreadRegistryFile, saveThreadRegistryFile, hydrateThreadRegistry } from "./ask-threads.ts";
export type { OverlayHandle, ThreadRecord, ThreadStatus, ThreadOrigin, PersistedForkResume, AskUiCtx, ThreadRegistryHandle, AskSessionCtx } from "./ask-threads.ts";

/**
 * Phase 2 (`260911`, sequential prose-modal tier): the live host repaint hook
 * for the currently-open batch queue modal, if any — mirrors `activeOverlay`'s
 * module-scope singleton precedent. `undefined` outside a live queue modal.
 * Filled/cleared by `openLeadAskQueue`; consulted (best-effort) by
 * `withdrawQueuedQuestion` so a model withdrawal's banner shows up
 * immediately rather than waiting for the owner's next keystroke.
 */
let activeQueueRepaint: (() => void) | undefined;

let activeQueueOverlayToken: number | undefined;

function repaintActiveQueue(): void {
  try {
    activeQueueRepaint?.();
  } catch {
    // best effort — see doc comment above.
  }
}

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
type LeadAskSessionManager = {
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

/**
 * §5's "one overlay at a time": module-scope, because the overlay outlives
 * the `/answer` handler that opened it (`ctx.ui.custom` resolves only when
 * the overlay closes) and a second `/answer` must be able to close the first
 * one. Closing an overlay never touches its fork — the fork keeps running
 * and the thread stays reopenable (the doom-overlay example's own
 * persistent-state-vs-disposable-view split).
 */
/**
 * Ensures the thread has a live-or-resumable respondent fork on the shared
 * `rpcRegistry`, spawning a discussion fork lazily when it has none.
 * Returns the respondent's agent_id, or `undefined` when it could not be
 * established (already reported to the owner via `notify`).
 *
 * Exported (review relay #1, test partition C5) so the `threadBound`-on-open
 * and `threadBound`-on-REOPEN invariants have direct offline coverage on the
 * two branches that need no subprocess — an already-live respondent and a
 * rehydrate-from-`forkResume` reopen. The third branch (a fresh discussion
 * fork) goes through `spawnAgent` and stays live-gate only.
 */
export async function ensureRespondent(
  pi: ExtensionAPI,
  ctx: AskUiCtx & { sessionManager?: unknown },
  bridge: BridgeHandle,
  rpcRegistry: RpcAgentRegistry,
  handle: ThreadRegistryHandle,
  thread: ThreadRecord,
  sessionCtx: AskSessionCtx,
): Promise<string | undefined> {
  if (thread.respondentAgentId) {
    const agentId = thread.respondentAgentId;
    if (!rpcRegistry.has(agentId)) {
      // Dormant across a lead-process restart: the shared registry is
      // in-memory only, so put a reconstructed record back on it. The actual
      // relaunch happens inside `sendToAgent` on the owner's first message.
      if (!thread.forkResume) {
        notify(ctx, `ws: thread ${thread.threadId}'s fork can no longer be resumed (no persisted session).`, "error");
        return undefined;
      }
      // Retention or eviction removed (or is removing) this fork's home and
      // recorded its cost; a rehydrated relaunch would start an empty session.
      const persistedOwnership = thread.forkResume.ownership;
      if (isRemovedAgent(rpcRegistry, agentId, persistedOwnership && validDescriptor(persistedOwnership) && persistedOwnership.agentId === agentId ? persistedOwnership : undefined)) {
        notify(ctx, `ws: thread ${thread.threadId}'s fork cannot be resumed. ${removedAgentMessage(agentId)}.`, "error");
        return undefined;
      }
      rpcRegistry.set(agentId, rehydrateForkRecord(agentId, thread.forkResume));
    }
    bindThread(rpcRegistry, agentId, true);
    return agentId;
  }

  const forkFrom = getForkSourceSessionFile(ctx);
  if (!forkFrom) {
    notify(ctx, "ws: cannot open a discussion thread — this session has no session file to fork from.", "error");
    return undefined;
  }

  // §7: anchor a compacted entry with a verbatim excerpt of its own window.
  let excerpt: string | undefined;
  const sessionManager = (ctx as { sessionManager?: LeadAskSessionManager }).sessionManager;
  if (thread.entryId && sessionManager) {
    try {
      const liveEntries = sessionManager.buildContextEntries?.() ?? [];
      if (!isEntryLive(thread.entryId, liveEntries)) {
        excerpt = buildVerbatimExcerpt(thread.entryId, sessionManager.getBranch?.(thread.entryId) ?? []);
      }
    } catch {
      // A session-tree read failure must not block opening the thread — the
      // fork simply gets the question without the excerpt.
    }
  }

  const tools = computeForkToolSurface(pi.getActiveTools());
  const captured = sessionCtx.effectivePromptRef?.resolve?.(ctx) ?? sessionCtx.effectivePromptRef?.current;
  const forkContext = captured
    ? captureForkContext({
        kind: "discussion",
        effectiveSystemPrompt: captured.effectiveSystemPrompt,
        basePromptOptions: captured.basePromptOptions,
        wsBlock: captured.wsBlock,
        parentSessionKey: captured.parentSessionKey ?? bridge.defaultSessionKeyRef.current,
        parentSessionKeys: [...new Set([captured.parentSessionKey, bridge.defaultSessionKeyRef.current].filter((key): key is string => typeof key === "string"))],
        parentPiSessionId: (ctx as { sessionManager?: { getSessionId(): string } }).sessionManager?.getSessionId(),
        parentAffinityId: (ctx as { sessionManager?: { getSessionId(): string } }).sessionManager?.getSessionId(),
        thinkingLevel: pi.getThinkingLevel(),
        activeTools: tools,
        registeredTools: captureRegisteredTools(tools, pi.getAllTools()),
        modelDescriptor: await effectiveForkDescriptor(ctx as never, pi.getThinkingLevel()),
      })
    : undefined;
  const result = await spawnAgent(
    rpcRegistry,
    {
      pi,
      cwd: sessionCtx.cwd,
      storage: storageContextFromToolCtx(ctx),
      inheritModel: inheritModelFromToolCtx(ctx),
      catalog: modelCatalogFromToolCtx(ctx),
      notifyTierWarning: tierWarningNotifierFromToolCtx(ctx),
      wsToolNames: bridge.wsToolNames,
      extensionPath: sessionCtx.extensionPath,
      client: bridge.client,
      forkFrom,
      forkSourceEntries: captureUnflushedForkSource(ctx),
      explicitTools: tools.join(","),
      forkContext,
      parentSessionKey: bridge.defaultSessionKeyRef.current,
      parentPolicy: leadParentPolicy(bridge),
      // Entry B's discussion fork belongs to the owner surface, never to the
      // lead's fan-in — bound before its first turn can produce a settle.
      spawnRole: "fork",
    },
    {
      // Entry B remains a conversational message; task-only completion fields stay absent.
      prompt: buildDiscussionForkInitialMessage(thread.context, thread.question ?? thread.title, excerpt),
    },
  );

  bindThread(rpcRegistry, result.agent_id, true);
  thread.respondentAgentId = result.agent_id;
  const record = rpcRegistry.get(result.agent_id);
  if (record) thread.forkResume = captureForkResume(record);
  return result.agent_id;
}

/**
 * Sets/clears `RpcAgentRecord.threadBound` — the thread-lifetime flag (§1's
 * "the lead is not part of this exchange"). Set on every thread open/reopen and on fork-raised
 * registration; cleared only where the thread itself actually closes
 * (`detachForkRaisedThread`, `injectDiscussionSummary`, `ws-resolve`), never
 * on a mere overlay Esc. While set, `spawner.ts` emits no settle push for the
 * record and `computeRunningStatusLine` leaves it out entirely — it is neither
 * counted as running nor keeps the status line present.
 */
function bindThread(rpcRegistry: RpcAgentRegistry, agentId: string, bound: boolean): void {
  const record = rpcRegistry.get(agentId);
  if (!record) return;
  // Publish protection before the local bind: a busy maintenance claim must
  // refuse this transition, not silently leave an accepted owner thread prunable.
  // Releases may remain durably protected on failure; that only retains data.
  const persisted = syncOwnershipProtection({ ...record, threadBound: bound });
  if (bound && !persisted) throw new Error("ws-pi-agent: cannot bind owner thread — owned session home is busy, gone, or unreadable; retry the question");
  record.threadBound = bound;
}

/**
 * Duck-typed slice of `ctx.ui.custom`'s real signature
 * (`ExtensionUIContext.custom`) — kept minimal like every other `ctx` seam in
 * this module, and narrowed to what `openThread` needs: a `tui` structurally
 * compatible with `ConversationViewTui` (the real host `TUI` is a superset),
 * a `theme` exposing semantic foreground/background painters, and a factory that may return its component
 * asynchronously (the real signature allows `Component | Promise<Component>`
 * — needed here because building the live component awaits
 * `loadHostPiTui()`).
 */
interface AskCustomUiCtx {
  ui: {
    custom<T>(
      factory: (
        tui: ConversationViewTui,
        theme: { bg?(color: string, text: string): string; fg?(color: string, text: string): string } | undefined,
        keybindings: unknown,
        done: (result: T) => void,
      ) => Component | Promise<Component>,
      options?: { overlay?: boolean; overlayOptions?: unknown },
    ): Promise<T>;
  };
}

/**
 * §5's `/done`-vs-live-task-fork branching, pulled out as its own pure
 * decision (review relay #2 I3, mirroring `resolveChildLiveness`'s
 * precedent) so `openThread`'s `onDone` closure is a thin dispatch over an
 * independently unit-testable result rather than an untested inline `if`:
 * `"summarize"` asks the (owned) discussion fork for a summary turn before
 * closing; `"close-empty"` closes the view on the spot, with nothing sent to
 * a live task fork this surface does not own (review relay #2 C2).
 */
export type DoneAction = "summarize" | "close-empty";

export function resolveDoneAction(summarizeOnDone: boolean): DoneAction {
  return summarizeOnDone ? "summarize" : "close-empty";
}

/**
 * `/done`'s summarize-then-close state machine — ported from the old,
 * now-deleted per-thread overlay module's `submit()`/`handleEvent()`, with
 * one correctness fix (review relay #2 Critical/Important): appends the
 * "ending the thread…" note, sends the fixed `buildDoneSummaryPrompt()`
 * through the channel, and subscribes ONCE MORE to `channel.onEvent` (a
 * second, independent listener alongside the component's own internal one,
 * registered first) to take the very next `agent_settled` as the summary.
 * Starting this listener fresh at `/done` time — rather than reusing any
 * buffer accumulated before it — is what keeps a half-streamed pre-`/done`
 * turn from ever leaking into the summary (the old M11 guard), with no
 * explicit reset needed: this listener simply never saw those earlier
 * events.
 *
 * On that same `agent_settled`, the component's OWN internal listener (it
 * saw the identical event first — registered in the constructor, before
 * this function is ever called) already appended the settled text as an
 * ordinary `"assistant"` item and persisted it via `onItemsChange` — exactly
 * the render the summary turn needs. `closeWithSummary(..., true)` below
 * tells `buildOverlayHandle` that text is ALREADY on screen, so it must not
 * append it a second time (the review relay #2 Critical: the old
 * single-listener component only ever appended a settled turn once, then
 * called `finish()` with no further append — this restores that same
 * "append at most once" invariant across the new two-listener split).
 *
 * Returns the fresh listener's `unsubscribe`, so a caller that closes the
 * overlay before this settle ever arrives (Esc) can tear it down — otherwise
 * a LATER settle would still fire this callback and call
 * `overlay.closeWithSummary`, which would now be silently absorbed by
 * `buildOverlayHandle`'s own `finished` guard (review relay #2 Important:
 * closing early must not just be swallowed, it must stop listening).
 */
export function summarizeThenClose(component: ConversationViewComponent, channel: ConversationChannel, overlay: OverlayHandle): () => void {
  component.appendItem({ kind: "note", text: "ending the thread — asking for a summary…" });
  let streaming = "";
  const unsubscribe = channel.onEvent((evt) => {
    const e = evt as { type?: string; assistantMessageEvent?: { type?: string; delta?: string } };
    if (e.type === "message_update" && e.assistantMessageEvent?.type === "text_delta" && typeof e.assistantMessageEvent.delta === "string") {
      streaming += e.assistantMessageEvent.delta;
      return;
    }
    if (e.type !== "agent_settled") return;
    unsubscribe();
    const settled = streaming.trim();
    overlay.closeWithSummary(settled.length > 0 ? settled : EMPTY_SUMMARY_TEXT, /* alreadyRendered */ true);
  });
  void channel.send?.(buildDoneSummaryPrompt());
  return unsubscribe;
}

/**
 * `openThread`'s `/done` dispatch, extracted from its inline `onDone` closure
 * (mirroring `resolveDoneAction`/`summarizeThenClose`) so the
 * origin-to-route mapping is unit-lockable end to end rather than live-gate
 * only — the seam 260909 F1 regressed on.
 *
 * - `"summarize"` — a `lead-ask` thread this surface owns: ask the discussion
 *   fork for a summary turn, whose settle drives `overlay.closeWithSummary`,
 *   which routes through `closeThreadOnDone` to the §6 `ws-thread-summary`
 *   injection into the lead. Returns the pending `summarizeThenClose`
 *   unsubscribe so an early Esc can tear the listener down.
 * - `"close-empty"` — a `fork-raised` thread: close the view with an empty
 *   summary, which `closeThreadOnDone` routes to a detach with NO injection
 *   and NO stop (the scope guard). Returns `undefined` — nothing is pending.
 */
export function runDoneAction(
  action: DoneAction,
  component: ConversationViewComponent,
  channel: ConversationChannel,
  overlay: OverlayHandle,
): (() => void) | undefined {
  if (action === "close-empty") {
    overlay.closeWithSummary("");
    return undefined;
  }
  return summarizeThenClose(component, channel, overlay);
}

/**
 * Wraps a live `ConversationViewComponent` + the `ctx.ui.custom` `done`
 * callback as an `OverlayHandle` — the external contract
 * a pending `summarizeThenClose` settle, an owner Esc, and a second `/answer` closing
 * the first overlay all drive without reaching into the component itself.
 * `closeWithSummary` mirrors the old, now-deleted per-thread overlay
 * module's own: a non-empty summary is appended as the child's own turn
 * before the thread itself closes — UNLESS `alreadyRendered` says the
 * component's own settle handling already put it there (see
 * `summarizeThenClose`'s doc comment).
 *
 * Review relay #2 Important (I1a/I1b/I4): a private `finished` flag makes
 * `close`/`closeWithSummary` a no-op after either has already run once —
 * whichever close race wins (an owner Esc during the summary wait, a second
 * owner action, or the summary settle itself) is the ONLY
 * one that runs `closeThreadOnDone`/injects a summary/calls `done`, exactly
 * mirroring the old component's own `finished` guard. `onFinish` — called
 * exactly once, by whichever path wins — is `openThread`'s hook to tear down
 * a still-pending `summarizeThenClose` listener so a late settle never even
 * reaches this guard (belt-and-suspenders with the guard itself).
 */
export function buildOverlayHandle(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  rpcRegistry: RpcAgentRegistry,
  thread: ThreadRecord,
  component: ConversationViewComponent,
  done: (result: undefined) => void,
  onFinish?: () => void,
  resumeCtx?: AskSessionCtx,
): OverlayHandle {
  let finished = false;
  return {
    close: () => {
      if (finished) return;
      finished = true;
      onFinish?.();
      done(undefined);
    },
    closeWithSummary: (summary, alreadyRendered = false) => {
      if (finished) return;
      finished = true;
      onFinish?.();
      if (!alreadyRendered && summary.trim().length > 0) component.appendItem({ kind: "assistant", text: summary.trim() });
      closeThreadOnDone(pi, handle, rpcRegistry, thread, summary, resumeCtx);
      done(undefined);
    },
  };
}

/**
 * The Phase 1 single-question overlay's `onEscape` decision
 * (D-model-side-withdrawal), extracted as its own pure function — mirroring
 * `resolveDoneAction` — so the withdrawal/in-progress-draft interaction is
 * unit-lockable independent of a live `ConversationViewComponent`. Reused
 * unchanged (via `resolveLeadAskQueueEntryAction`) as Phase 2's per-question
 * "preserve" decision inside the sequential prose-modal tier's own close
 * path — see `LeadAskQueueComponent`.
 *
 * `draft` is the owner's current, already-trimmed editor text.
 *
 * - `"deliver"` — a model withdrawal arrived while the view was open
 *   (`thread.withdrawnPending`) AND the owner had already typed something:
 *   the answer is still delivered on Esc, never silently discarded (the
 *   ticket's headline "a withdrawal never discards the owner's typed
 *   prose" contract).
 * - `"finalize-withdrawal"` — a model withdrawal arrived and nothing was
 *   typed: the withdrawal completes with nothing to deliver.
 * - `"revert-pending"` — no withdrawal arrived; the owner is simply closing
 *   an unanswered thread, so it stays available for a later `/answer`. Phase
 *   2 now persists whatever was typed as `ThreadRecord.draftAnswer` (see
 *   `runLeadAskEscapeAction`) so it resumes on a later reopen — the "Phase 1
 *   does not carry a draft forward" limitation this bullet used to describe
 *   is what Phase 2's D3 "per-question drafts persist" lifts.
 */
export type LeadAskEscapeAction = "deliver" | "finalize-withdrawal" | "revert-pending";

export function resolveLeadAskEscapeAction(withdrawnPending: boolean, draft: string): LeadAskEscapeAction {
  if (withdrawnPending) return draft.length > 0 ? "deliver" : "finalize-withdrawal";
  return "revert-pending";
}

/**
 * Phase 2 (`260911`, sequential prose-modal tier): the queue's per-question
 * batch-close decision, generalizing `resolveLeadAskEscapeAction` with a
 * `mode` the single-question overlay never needed — the queue's final
 * confirm and Esc-partial-submit-accept paths both SUBMIT every answered,
 * unwithdrawn question, not just close the view. A still-pending model
 * withdrawal (`withdrawnPending`) takes the same precedence either way: a
 * non-empty draft is always delivered (a withdrawal never discards typed
 * prose) and an empty one is always finalized; `mode` only changes what
 * happens to an UNWITHDRAWN question — deliver it (`"submit"`) or persist it
 * as a draft and leave the question pending (`"preserve"`, i.e. exactly
 * `resolveLeadAskEscapeAction`'s own behavior).
 */
export function resolveLeadAskQueueEntryAction(
  mode: "submit" | "preserve",
  withdrawnPending: boolean,
  draft: string,
): LeadAskEscapeAction {
  if (mode === "submit" && !withdrawnPending && draft.trim().length > 0) return "deliver";
  return resolveLeadAskEscapeAction(withdrawnPending, draft);
}

/**
 * Runs a `resolveLeadAskEscapeAction`/`resolveLeadAskQueueEntryAction` result
 * against the real thread record — the other half of the
 * `resolveDoneAction`/`runDoneAction` split. `"deliver"` routes through
 * `deliverQueuedAnswer` exactly as a normal `send()` would (same
 * anchor/excerpt/`"dormant"` handling, and clears `draftAnswer`); the other
 * two actions mutate `thread` directly and persist.
 */
export function runLeadAskEscapeAction(
  action: LeadAskEscapeAction,
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  thread: ThreadRecord,
  draft: string,
  sessionManager?: LeadAskSessionManager,
): void {
  if (action === "deliver") {
    deliverQueuedAnswer(pi, handle, thread, draft, sessionManager);
    return;
  }
  if (action === "finalize-withdrawal") {
    thread.withdrawnPending = false;
    thread.status = "closed";
    thread.draftAnswer = undefined;
  } else {
    // "revert-pending": Phase 2 (260911) D3 — persist whatever the owner had
    // typed as this question's draft so a later reopen of the queue (or the
    // single-question overlay) resumes it; an empty draft clears any earlier
    // one rather than leaving a stale value behind.
    thread.status = "pending";
    thread.draftAnswer = draft.trim().length > 0 ? draft : undefined;
  }
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshAgentWidget();
}

/** The production modal-close callback, extracted so its one-follow-up batch boundary is directly testable. */
export function buildLeadAskQueueOnClose(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  threads: readonly ThreadRecord[],
  sessionManager?: LeadAskSessionManager,
  onDone?: () => void,
): (mode: "submit" | "preserve", drafts: ReadonlyMap<string, string>) => void {
  return (mode, drafts) => {
    const deliveries: Array<{ thread: ThreadRecord; answer: string }> = [];
    for (const thread of threads) {
      const draft = drafts.get(thread.threadId) ?? "";
      const action = resolveLeadAskQueueEntryAction(mode, thread.withdrawnPending === true, draft);
      if (action === "deliver") deliveries.push({ thread, answer: draft });
      else runLeadAskEscapeAction(action, pi, handle, thread, draft, sessionManager);
    }
    deliverQueuedAnswers(pi, handle, deliveries, sessionManager);
    onDone?.();
  };
}

/**
 * Phase 2 (`260911`, D3) queue order: every answerable
 * (`"pending"`/`"open"`) `"lead-ask"` thread, oldest-asked first — the order
 * the owner is meant to work through them in. `"fork-raised"` threads never
 * enter the batch queue (Scope: "lead-raised only" — see this file's
 * header); they keep the single overlay-chat surface via `openThread`'s
 * existing dispatch, unchanged.
 */
export function collectLeadAskQueue(records: readonly ThreadRecord[]): ThreadRecord[] {
  return records
    .filter((record) => record.origin === "lead-ask" && (record.status === "pending" || record.status === "open"))
    .sort((a, b) => (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : a.threadId.localeCompare(b.threadId)));
}

/** Non-empty (trimmed) drafts across the batch — the queue's live "answered" count for the coverage indicator and both confirm screens. */
export function countQueueAnswered(drafts: ReadonlyMap<string, string>): number {
  let count = 0;
  for (const draft of drafts.values()) {
    if (draft.trim().length > 0) count += 1;
  }
  return count;
}

/** The `"Qn/N · answered"` coverage indicator (D3). `index` is 0-based; `answered` is `countQueueAnswered`'s live count. */
export function buildQueueCoverageLine(index: number, total: number, answered: number): string {
  return `Q${index + 1}/${total} · ${answered} answered`;
}

/**
 * The final-confirm / Esc-partial-submit screen's message: how many answered
 * questions would submit and how many would stay pending. Shared by both
 * confirm screens (`LeadAskQueueComponent`) — they differ only in how they
 * are reached and in the safe default they cancel back to, never in this
 * wording.
 */
export function buildQueueSubmitConfirmMessage(answered: number, total: number): string {
  const pending = Math.max(0, total - answered);
  return `Submit ${answered} answered question${answered === 1 ? "" : "s"}? ${pending} left pending.`;
}

/** `EditorLike` (conversation-view.ts) widened with the `Focusable.focused` flag the real host `Editor` exposes. Needed here — unlike `conversation-view.ts`'s own single-`Editor` use — because `LeadAskQueueComponent` holds one `Editor` per queued question and only the currently focused one should draw its cursor marker. */
export interface FocusableEditorLike extends EditorLike {
  focused: boolean;
}

/** Mirrors `conversation-view.ts`'s private `SHIFT_TAB`/`BORDER_OVERHEAD` constants (not exported — see that file's own `isEscapeKey` precedent for why a small constant is duplicated rather than reached into a private symbol). */
const QUEUE_SHIFT_TAB = "\x1b[Z";

const QUEUE_BORDER_OVERHEAD = 4;

/** Minimal, unpainted `EditorTheme` — the queue draws no host theme colors into the `Editor` itself (its surrounding text uses the same `theme.fg`/`theme.bg` seam every other overlay tier does). */
const QUEUE_IDENTITY_EDITOR_THEME: EditorTheme = {
  borderColor: (text) => text,
  selectList: {
    selectedPrefix: (text) => text,
    selectedText: (text) => text,
    description: (text) => text,
    scrollInfo: (text) => text,
    noMatch: (text) => text,
  },
};

/** One `LeadAskQueueComponent` confirm sub-screen: which one, and the cursor's current side. Both screens default to `"no"` (Decisions D3: "cursor defaulting to the safe No"). */
interface LeadAskQueueConfirmState {
  kind: "final" | "esc";
  choice: "yes" | "no";
}

export interface LeadAskQueueOptions {
  /** Fixed batch snapshot — Phase 2 queue order, from `collectLeadAskQueue`. Never mutated by the component itself; the host applies every close decision afterward. */
  threads: readonly ThreadRecord[];
  /** Which index to focus first (from `/answer <id>` or the reopen shortcut); clamped into range. */
  initialFocusIndex: number;
  /** Constructs one fresh `Editor` bound to the live host TUI — injectable for tests. */
  editorFactory: () => FocusableEditorLike;
  /** The live host's `matchesKey` (resolved through `loadHostPiTui()` by the caller — never the static import; see `pi-tui.ts`'s dual-package-instance note) for terminal-key detection across raw and Kitty-protocol encodings. */
  matchesKey: (data: string, keyId: string) => boolean;
  /** Pi's configured alternate-screen bindings, used for PageUp/PageDown question scrolling. */
  keybindings?: { matches(data: string, id: string): boolean };
  /** Word-wraps one line of plain text (the live host's `wrapTextWithAnsi`, or an injected fake in tests). */
  wrapText: (text: string, width: number) => string[];
  /** Semantic theme painters supplied by the live `ui.custom` callback. Identity defaults keep tests/headless use plain. */
  headerText?: (text: string) => string;
  separatorText?: (text: string) => string;
  helpText?: (text: string) => string;
  overflowText?: (text: string) => string;
  /** Draws the single-line box border, matching every other overlay tier. */
  border?: boolean;
  /** Fires exactly once, with the owner's final decision and every question's live draft text keyed by `threadId`. */
  onClose: (mode: "submit" | "preserve", drafts: ReadonlyMap<string, string>) => void;
}

/**
 * `260911` Phase 2 (D3): the sequential prose-modal tier — distinct from
 * `audit.ts`'s read-only viewer and the fork-raised `ConversationViewComponent`
 * overlay chat. Shows the owner ONE queued `"lead-ask"` question at a time
 * from a fixed batch, prose-only (never parses `#1./#2.` options the lead may
 * have embedded in the question text — those travel to the lead verbatim on
 * delivery via the existing D3 return-path anchor, `deliverQueuedAnswer`),
 * with per-question free-typed answers that persist across navigation and
 * reopen. See `openLeadAskQueue` for the surrounding IO glue (marking the
 * batch `"open"`, the withdrawal-banner repaint hook, and the close/deliver
 * contract) — it supersedes Phase 1's `openLeadAskThread` (removed; see the
 * ticket's Phase 1 Result for why it was scoped as scaffolding).
 *
 * Key contract (ticket Decisions D3):
 *   - `Enter` commits the focused question's current text as its answer and
 *     advances to the next one; on the LAST question it raises the final
 *     confirm instead of advancing further. This reuses the host `Editor`'s
 *     own native `tui.input.submit`/`tui.input.newLine` routing rather than
 *     reimplementing it: `Enter` calls `onSubmit` (submit), `shift+Enter`/
 *     `ctrl+j` insert a newline, and `↑`/`↓`/`pgUp`/`pgDn` move/scroll within
 *     the focused question's own text — all already true of a bare `Editor`
 *     with no extra code here. `Editor.onSubmit` clears its own text before
 *     firing; `handleQuestionSubmit` restores it immediately, since Enter
 *     here COMMITS the answer rather than sending-and-clearing it.
 *   - `Tab`/`shift+Tab` move focus with wrap-around (last -> first) and never
 *     submit or open a confirm screen — intercepted here, ahead of the
 *     `Editor`, so its own tab-autocomplete never fires.
 *   - `Esc` is the exit path: with nothing answered anywhere in the batch it
 *     closes at once (`"preserve"` — nothing to ask about); otherwise it
 *     raises the Esc-partial-submit confirm.
 *   - Both confirm screens default to `"no"` and execute on `Enter`; `Esc`
 *     inside either always takes the `"no"` branch — but the two branches
 *     differ, matching how each screen was reached: the final confirm's "no"
 *     cancels back to editing the last question (Enter did not mean to
 *     leave), while the Esc confirm's "no" exits without submitting (Esc
 *     always means "leave" — only whether it submits on the way out is in
 *     question).
 */
export class LeadAskQueueComponent implements Component {
  private readonly tui: ConversationViewTui;
  private readonly options: LeadAskQueueOptions;
  private readonly threads: readonly ThreadRecord[];
  private readonly editors: FocusableEditorLike[];
  private readonly questionScrollOffsets: number[];
  private readonly questionViewportRows: number[];
  private readonly questionMaxScroll: number[];
  private index: number;
  private confirm: LeadAskQueueConfirmState | undefined;
  private finished = false;

  constructor(tui: ConversationViewTui, options: LeadAskQueueOptions) {
    this.tui = tui;
    this.options = options;
    this.threads = options.threads;
    this.index = Math.min(Math.max(0, options.initialFocusIndex), this.threads.length - 1);
    this.questionScrollOffsets = this.threads.map(() => 0);
    this.questionViewportRows = this.threads.map(() => 1);
    this.questionMaxScroll = this.threads.map(() => 0);
    this.editors = this.threads.map((thread, i) => {
      const editor = options.editorFactory();
      editor.setText(thread.draftAnswer ?? "");
      editor.onSubmit = (text) => this.handleQuestionSubmit(i, text);
      editor.focused = i === this.index;
      return editor;
    });
  }

  invalidate(): void {
    for (const editor of this.editors) editor.invalidate();
  }

  handleInput(data: string): void {
    if (data === "\x03") return; // Ctrl+C swallowed, matching every other overlay tier.
    if (this.confirm) {
      this.handleConfirmInput(data);
      return;
    }
    if (this.handleQuestionScrollInput(data)) return;
    if (isEscapeKey(data)) {
      this.handleEscape();
      return;
    }
    if (data === "\t") {
      this.focusNext(1);
      return;
    }
    if (data === QUEUE_SHIFT_TAB) {
      this.focusNext(-1);
      return;
    }
    this.editors[this.index].handleInput(data);
  }

  render(width: number): string[] {
    const w = Math.max(1, width);
    const border = this.options.border === true;
    const innerW = border ? Math.max(1, w - QUEUE_BORDER_OVERHEAD) : w;
    const inner = this.renderInner(innerW);
    return border ? wrapInBorder(inner, w, innerW) : inner;
  }

  // ---- decisions ------------------------------------------------------------

  /**
   * Review-round-1 correctness Critical fix: trims here, once, so every
   * consumer (the coverage count, both confirm screens, and `onClose`'s
   * drafts map handed to `resolveLeadAskEscapeAction` — whose own contract
   * assumes already-trimmed text, exactly as Phase 1's caller trimmed
   * before it) agrees on what counts as "empty." Reading the raw untrimmed
   * `Editor` buffer here let a whitespace-only draft (e.g. a lone space, or
   * a `shift+Enter` newline) on a `withdrawnPending` question read as
   * non-empty to `resolveLeadAskEscapeAction`'s `draft.length > 0` check,
   * delivering literal whitespace as the owner's "answer" instead of
   * finalizing the withdrawal — trimming outer whitespace does not touch
   * D3's "store answers verbatim" (that decision is about never parsing
   * `#1./#2.` options out of the text, not about preserving invisible
   * padding).
   */
  private liveDrafts(): Map<string, string> {
    const drafts = new Map<string, string>();
    for (let i = 0; i < this.threads.length; i += 1) drafts.set(this.threads[i].threadId, this.editors[i].getText().trim());
    return drafts;
  }

  private focusNext(direction: 1 | -1): void {
    const total = this.threads.length;
    if (total <= 1) return;
    this.editors[this.index].focused = false;
    this.index = (this.index + direction + total) % total;
    this.editors[this.index].focused = true;
    this.tui.requestRender();
  }

  private handleQuestionSubmit(i: number, text: string): void {
    // Restore before the focus guard below (review-round-1 correctness Minor
    // fix): the real `Editor.submitValue()` has already cleared its own
    // buffer by the time this callback fires, so returning early WITHOUT
    // restoring first would discard the owner's just-typed text outright,
    // not merely ignore a stale event.
    this.editors[i].setText(text);
    if (i !== this.index) return; // defensive — input only ever reaches the focused editor.
    if (i === this.threads.length - 1) {
      this.confirm = { kind: "final", choice: "no" };
      this.tui.requestRender();
      return;
    }
    this.focusNext(1);
  }

  private handleEscape(): void {
    if (countQueueAnswered(this.liveDrafts()) === 0) {
      this.finish("preserve");
      return;
    }
    this.confirm = { kind: "esc", choice: "no" };
    this.tui.requestRender();
  }

  private handleConfirmInput(data: string): void {
    const confirm = this.confirm;
    if (!confirm) return;
    const matches = this.options.matchesKey;
    if (isEscapeKey(data)) {
      this.resolveConfirm(confirm.kind, "no");
      return;
    }
    if (matches(data, "left")) {
      if (confirm.choice === "no") {
        confirm.choice = "yes";
        this.tui.requestRender();
      }
      return;
    }
    if (matches(data, "right")) {
      if (confirm.choice === "yes") {
        confirm.choice = "no";
        this.tui.requestRender();
      }
      return;
    }
    if (matches(data, "enter")) this.resolveConfirm(confirm.kind, confirm.choice);
  }

  private handleQuestionScrollInput(data: string): boolean {
    const pageUp = this.options.matchesKey(data, "pageUp") || this.options.keybindings?.matches(data, "tui.altScreen.pageUp") === true;
    const pageDown = this.options.matchesKey(data, "pageDown") || this.options.keybindings?.matches(data, "tui.altScreen.pageDown") === true;
    if (!pageUp && !pageDown) return false;
    const index = this.index;
    const page = Math.max(1, this.questionViewportRows[index] - 1);
    const current = this.questionScrollOffsets[index];
    const next = Math.min(this.questionMaxScroll[index], Math.max(0, current + (pageUp ? -page : page)));
    if (next !== current) {
      this.questionScrollOffsets[index] = next;
      this.tui.requestRender();
    }
    return true;
  }

  private resolveConfirm(kind: "final" | "esc", choice: "yes" | "no"): void {
    if (choice === "yes") {
      this.finish("submit");
      return;
    }
    if (kind === "final") {
      // Decline: return to editing the last question — Enter did not mean to leave.
      this.confirm = undefined;
      this.tui.requestRender();
      return;
    }
    // Esc confirm declined: Esc always means "leave" — exit preserving drafts.
    this.finish("preserve");
  }

  private finish(mode: "submit" | "preserve"): void {
    if (this.finished) return;
    this.finished = true;
    this.options.onClose(mode, this.liveDrafts());
  }

  // ---- rendering --------------------------------------------------------

  private renderInner(w: number): string[] {
    const total = this.threads.length;
    const answered = countQueueAnswered(this.liveDrafts());
    const header = [this.line(this.paint(this.options.headerText, buildQueueCoverageLine(this.index, total, answered)), w), ""];
    if (this.confirm) {
      return [...header, ...this.renderConfirm(w, answered, total)];
    }
    const thread = this.threads[this.index];
    const banner = thread.withdrawnPending
      ? [this.line(this.paint(this.options.overflowText, "⚠ the agent withdrew this question — your answer, if any, is still delivered when you close."), w), ""]
      : [];
    const hint = this.options.wrapText(
      this.paint(this.options.helpText, "Enter: answer & next · Shift+Enter/Ctrl+J: newline · Tab/Shift+Tab: switch question · PgUp/PgDn: scroll question · Esc: exit"),
      w,
    );
    const editorLines = this.editors[this.index].render(w);
    const separatorPlain = `─ Answer ${"─".repeat(Math.max(0, w - 9))}`;
    const separator = [this.line(this.paint(this.options.separatorText, separatorPlain), w)];
    let questionLines = this.options.wrapText(thread.question ?? thread.title, w);
    if (thread.context && thread.context.trim().length > 0) {
      questionLines = [...questionLines, "", ...this.options.wrapText(thread.context, w)];
    }
    const viewportRows = conversationOverlayHeight(this.tui);
    if (Number.isFinite(viewportRows)) {
      const borderOverhead = this.options.border === true ? QUEUE_BORDER_OVERHEAD : 0;
      const fixedRows = borderOverhead + header.length + banner.length + separator.length + editorLines.length + 1 /* blank after editor */ + hint.length;
      questionLines = this.renderQuestionRegion(questionLines, Math.max(1, viewportRows - fixedRows), w);
    } else {
      this.questionScrollOffsets[this.index] = 0;
      this.questionViewportRows[this.index] = Math.max(1, questionLines.length);
      this.questionMaxScroll[this.index] = 0;
    }
    return [...header, ...banner, ...questionLines, ...separator, ...editorLines, "", ...hint];
  }

  private renderQuestionRegion(lines: string[], budget: number, width: number): string[] {
    const index = this.index;
    if (lines.length <= budget) {
      this.questionScrollOffsets[index] = 0;
      this.questionViewportRows[index] = Math.max(1, lines.length);
      this.questionMaxScroll[index] = 0;
      return lines;
    }
    const singleRow = budget === 1;
    const cuePrefix = singleRow && width >= 3 ? "↕ " : "";
    // At a one-row height, reserve the cue's columns before wrapping. Prefixing
    // an already width-filled line and then taking only the first wrapped row
    // would make its tail permanently unreachable.
    const scrollLines = singleRow
      ? lines.flatMap((line) => this.options.wrapText(line, Math.max(1, width - cuePrefix.length)))
      : lines;
    const contentRows = Math.max(1, budget - 1);
    const maxScroll = Math.max(0, scrollLines.length - contentRows);
    const offset = Math.min(maxScroll, this.questionScrollOffsets[index]);
    this.questionScrollOffsets[index] = offset;
    this.questionViewportRows[index] = contentRows;
    this.questionMaxScroll[index] = maxScroll;
    const visible = scrollLines.slice(offset, offset + contentRows);
    if (singleRow) return [this.line(this.paint(this.options.overflowText, `${cuePrefix}${visible[0] ?? ""}`), width)];
    const above = offset > 0 ? "↑ more above · " : "";
    const below = offset < maxScroll ? " · more below ↓" : "";
    const cue = this.line(this.paint(this.options.overflowText, `${above}question lines ${offset + 1}-${offset + visible.length}/${scrollLines.length} · PgUp/PgDn${below}`), width);
    return [...visible, cue];
  }

  private renderConfirm(w: number, answered: number, total: number): string[] {
    const choice = this.confirm?.choice;
    const yes = choice === "yes" ? "[Yes]" : " Yes ";
    const no = choice === "no" ? "[No]" : " No ";
    return [
      ...this.options.wrapText(buildQueueSubmitConfirmMessage(answered, total), w),
      "",
      this.line(`  ${yes}   ${no}`, w),
      "",
      ...this.options.wrapText(this.paint(this.options.helpText, "←/→ select · Enter confirm · Esc cancel"), w),
    ];
  }

  private paint(painter: ((text: string) => string) | undefined, text: string): string {
    return painter?.(text) ?? text;
  }

  private line(text: string, width: number): string {
    return this.options.wrapText(text, width)[0] ?? "";
  }

  /** The owner's current text for question `index`, for a host or test to read without depending on `onClose` having fired. */
  getDraft(index: number): string {
    return this.editors[index]?.getText() ?? "";
  }

  getFocusedIndex(): number {
    return this.index;
  }
}

/**
 * `260911` Phase 2 (D3): opens the sequential prose-modal tier over every
 * answerable `"lead-ask"` thread at once — the async-queue owner surface
 * that supersedes Phase 1's `openLeadAskThread` (removed). Marks every batch
 * member `"open"` up front so a model's `ws-withdraw-question` mid-session
 * correctly takes the "deferred, banner, still-delivers-a-draft" branch
 * (`withdrawQueuedQuestion`) for any of them, exactly as the Phase 1 single-
 * question view did for one thread at a time. `startThreadId` positions the
 * initial focus (from `/answer <id>` or the reopen shortcut); omitted or no
 * longer answerable falls back to the first question in queue order, with a
 * notice in the latter case.
 */
async function openLeadAskQueue(
  pi: ExtensionAPI,
  ctx: AskUiCtx & { ui?: { custom?: unknown }; sessionManager?: unknown },
  handle: ThreadRegistryHandle,
  startThreadId?: string,
): Promise<void> {
  const threads = collectLeadAskQueue([...handle.threads.values()]);
  const requestedIndex = startThreadId ? threads.findIndex((thread) => thread.threadId === startThreadId) : -1;

  if (threads.length === 0) {
    const requested = startThreadId ? handle.threads.get(startThreadId) : undefined;
    if (requested && requested.origin === "lead-ask") {
      notify(ctx, `ws: question ${startThreadId} was already answered or withdrawn.`, "warning");
    } else {
      notify(ctx, "ws: no queued questions to answer.", "info");
    }
    return;
  }
  if (startThreadId && requestedIndex < 0) {
    const requested = handle.threads.get(startThreadId);
    if (requested && requested.origin === "lead-ask") {
      notify(ctx, `ws: question ${startThreadId} was already answered or withdrawn — showing the rest of the queue instead.`, "warning");
    }
  }
  const initialFocusIndex = requestedIndex >= 0 ? requestedIndex : 0;

  const now = nowIso();
  for (const thread of threads) {
    thread.status = "open";
    thread.touchedAt = now;
  }
  persistThreads(handle);
  refreshAgentWidget();

  // One owner overlay at a time across queued answers and audit/thread chat.
  const token = reserveOwnerOverlay();

  const sessionManager = (ctx as { sessionManager?: LeadAskSessionManager }).sessionManager;

  try {
    await (ctx as unknown as { ui: AskCustomUiCtx["ui"] }).ui.custom<undefined>(
      async (tui, theme, keybindings, done) => {
        const hostPiTui = await loadHostPiTui();
        const component = new LeadAskQueueComponent(tui, {
          threads,
          initialFocusIndex,
          border: true,
          matchesKey: hostPiTui.matchesKey as (data: string, keyId: string) => boolean,
          keybindings: keybindings as { matches(data: string, id: string): boolean },
          wrapText: (text, width) => hostPiTui.wrapTextWithAnsi(text, width),
          headerText: (text) => theme.fg("accent", theme.bold(text)),
          separatorText: (text) => theme.fg("borderAccent", text),
          helpText: (text) => theme.fg("dim", text),
          overflowText: (text) => theme.fg("warning", text),
          editorFactory: () => new hostPiTui.Editor(tui as never, QUEUE_IDENTITY_EDITOR_THEME) as unknown as FocusableEditorLike,
          onClose: buildLeadAskQueueOnClose(pi, handle, threads, sessionManager, () => done(undefined)),
        });
        activateOwnerOverlay({
          token,
          threadId: threads[initialFocusIndex].threadId,
          close: () => done(undefined),
          closeWithSummary: () => done(undefined),
        });
        activeQueueRepaint = () => tui.requestRender();
        activeQueueOverlayToken = token;
        return component;
      },
      { overlay: true, overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" } },
    );
  } finally {
    // Review-round-1 correctness Minor fix: guard both clears under the same
    // token check (matching every other `finally` in this file) rather than
    // unconditionally clearing `activeQueueRepaint` — harmless while only one
    // queue can hold focus at a time, but the asymmetry could clear a
    // successor's hook if that single-overlay invariant ever loosens.
    clearOwnerOverlay(token);
    if (activeQueueOverlayToken === token) {
      activeQueueRepaint = undefined;
      activeQueueOverlayToken = undefined;
    }
  }
}

/**
 * Opens (or reopens) one thread's overlay chat. Never auto-popped — only a
 * `/answer`, or the reopen shortcut, reaches here.
 */
async function openThread(
  pi: ExtensionAPI,
  ctx: AskUiCtx & { ui?: { custom?: unknown } },
  bridge: BridgeHandle,
  rpcRegistry: RpcAgentRegistry,
  handle: ThreadRegistryHandle,
  thread: ThreadRecord,
  sessionCtx: AskSessionCtx,
): Promise<void> {
  if (ctx.mode !== "tui") {
    notify(ctx, "ws: discussion threads need interactive mode.", "warning");
    return;
  }

  // `260911` D1: a `"lead-ask"` thread is fork-less end to end — it never
  // reaches `ensureRespondent`'s discussion-fork spawn branch below. See
  // `openLeadAskQueue` (Phase 2's sequential prose-modal tier) and this
  // file's header comment.
  if (thread.origin === "lead-ask") {
    await openLeadAskQueue(pi, ctx, handle, thread.threadId);
    return;
  }

  let agentId: string | undefined;
  try {
    agentId = await ensureRespondent(pi, ctx, bridge, rpcRegistry, handle, thread, sessionCtx);
  } catch (err) {
    notify(ctx, `ws: could not open thread ${thread.threadId}: ${err instanceof Error ? err.message : String(err)}`, "error");
    return;
  }
  if (!agentId) return;

  thread.status = "open";
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshAgentWidget();

  // One owner overlay at a time: the prior audit/thread/queue view closes.
  const token = reserveOwnerOverlay();

  // Review relay #1 C1: mark the respondent as owner-attached for as long as
  // this overlay lives. An Entry-A task fork still runs `wireAntiBleedLoop`
  // on the same record, and every owner exchange is a text-only turn — §4's
  // bleed signal — so without this the loop would nudge the fork mid-
  // conversation and then steer a false "stalled, do not harvest" verdict
  // into the lead. Read (not imported) by `fork.ts`: the reverse import would
  // cycle.
  const resumeCtx = { ...sessionCtx, leadSessionKeyRef: bridge.defaultSessionKeyRef };
  const channel = createForkChannel(pi, rpcRegistry, resumeCtx, agentId, (updated) => {
    thread.forkResume = captureForkResume(updated);
    thread.touchedAt = nowIso();
    persistThreads(handle);
    refreshAgentWidget();
  });
  const sourceRecord = rpcRegistry.get(agentId)!;
  const history = readSessionHistory(sourceRecord.sessionPath, sourceRecord.ownerSends);
  const initialItems = history.status === "available"
    ? history.items
    : [{ kind: "note" as const, text: "History unavailable: the child session file is gone or unreadable." }];
  // Keep identity/time/controls compact. The original question is the first
  // dialogue turn in `initialItems`, where it receives conversation styling.
  const headerHint = buildThreadHeaderHint(thread);
  // Review relay #2 C2: only a discussion fork this surface owns is asked
  // for a summary. A live task fork is mid-task — asking it to summarize
  // (and then acting on that turn) would derail the work the lead is
  // waiting on.
  const summarizeOnDone = thread.origin === "lead-ask";
  // Best effort: `conversation-view.ts`'s `markdownLines()` falls back to its
  // own identity theme when this is `undefined`, so a throwing/missing host
  // theme must not block opening the thread.
  let markdownTheme: MarkdownTheme | undefined;
  try {
    markdownTheme = getMarkdownTheme();
  } catch {
    // best effort — see doc comment above.
  }

  try {
    await (ctx as unknown as AskCustomUiCtx).ui.custom<undefined>(
      async (tui, theme, keybindings, done) => {
        const hostPiTui = await loadHostPiTui();
        let overlayHandle: OverlayHandle | undefined;
        // Review relay #2 I1a: the one `summarizeThenClose` listener that may
        // be waiting on a settle at any given moment. Torn down by
        // `buildOverlayHandle`'s `onFinish` hook the instant ANY close path
        // wins, so an owner Esc (or another close action) during the wait
        // stops this listener rather than leaving it to fire later into an
        // already-guarded (but still leaked) `closeWithSummary`.
        let pendingSummarizeUnsubscribe: (() => void) | undefined;
        let component!: OwnerSteeringComponent;
        const view = new ConversationViewComponent(tui, {
          channel,
          initialItems,
          headerHint,
          markdownTheme,
          border: true,
          viewportHeight: () => conversationOverlayHeight(tui),
          keybindings: keybindings as { matches(data: string, id: string): boolean },
          userLineBg: (text) => theme?.bg?.("userMessageBg", text) ?? text,
          toolTextFg: (text) => theme?.fg?.("muted", text) ?? text,
          workingTextFg: (text) => theme?.fg?.("dim", text) ?? text,
          onSendError: (error) => notify(ctx, `ws: owner send failed: ${error instanceof Error ? error.message : String(error)}`, "error"),
          primitives: { ScrollView: hostPiTui.ScrollView, Markdown: hostPiTui.Markdown, Text: hostPiTui.Text, Editor: hostPiTui.Editor },
          onDone: () => component.finish(),
        });
        view.setMode("interactive");
        overlayHandle = buildOverlayHandle(pi, handle, rpcRegistry, thread, view, done, () => {
          pendingSummarizeUnsubscribe?.();
          pendingSummarizeUnsubscribe = undefined;
        }, resumeCtx);
        component = new OwnerSteeringComponent(tui, view, {
          done: () => overlayHandle!.close(),
          finish: () => {
            pendingSummarizeUnsubscribe = runDoneAction(resolveDoneAction(summarizeOnDone), view, channel, overlayHandle!);
          },
          interrupt: async () => {
            const live = rpcRegistry.get(agentId!);
            if (live?.running && live.client) await live.client.abort();
          },
          interruptEnabled: () => rpcRegistry.get(agentId!)?.running === true && rpcRegistry.get(agentId!)?.client !== undefined,
          notify: (message, type) => notify(ctx, message, type),
          theme,
          matchesKey: hostPiTui.matchesKey as (data: string, keyId: string) => boolean,
        });
        activateOwnerOverlay({ token, threadId: thread.threadId, close: overlayHandle.close, closeWithSummary: overlayHandle.closeWithSummary });
        return component;
      },
      { overlay: true, overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" } },
    );
  } finally {
    // Cleared on every exit path — `/done` (which also stops the fork), a
    // plain close, or a throw out of the overlay.
    clearOwnerOverlay(token);
  }
}

/**
 * Registers the owner-side surface: `/thread` (list), `/answer <id>` (open
 * one, spawning its discussion fork lazily), and a reopen shortcut for the
 * most recently touched thread. `/done` is NOT a Pi command — it is
 * intercepted inside the overlay's own input handling, because
 * `ctx.ui.custom` takes keyboard focus away from the main editor Pi's
 * slash-command dispatch runs on (see `conversation-view.ts`).
 */
export function registerThreadCommands(
  pi: ExtensionAPI,
  bridge: BridgeHandle,
  rpcRegistry: RpcAgentRegistry,
  handle: ThreadRegistryHandle,
  sessionCtx: AskSessionCtx,
  openAnswerThread?: (thread: ThreadRecord, ctx: unknown) => Promise<void>,
): void {
  const openSelected = openAnswerThread ?? ((thread: ThreadRecord, ctx: unknown) => openThread(pi, ctx as never, bridge, rpcRegistry, handle, thread, sessionCtx));
  pi.registerCommand("thread", {
    description: "List ws discussion threads (pending, open, and dormant-but-reopenable).",
    handler: async (_args, ctx) => {
      notify(ctx as AskUiCtx, buildThreadListLines([...handle.threads.values()]).join("\n"), "info");
    },
  });

  pi.registerCommand("answer", {
    description: "Answer queued ws questions (usage: /answer <id>; no id opens the oldest queued lead question).",
    handler: async (args, ctx) => {
      const id = args.trim();
      if (!id) {
        const oldest = collectLeadAskQueue([...handle.threads.values()])[0];
        if (!oldest) {
          notify(ctx as AskUiCtx, "ws: no queued lead questions to answer.", "info");
          return;
        }
        await openSelected(oldest, ctx);
        return;
      }

      const thread = handle.threads.get(id);
      if (!thread) {
        notify(ctx as AskUiCtx, `ws: no thread "${id}" — /thread lists them.`, "error");
        return;
      }
      if (thread.origin === "lead-ask" && thread.status !== "pending" && thread.status !== "open") {
        notify(ctx as AskUiCtx, `ws: question ${id} was ${thread.status === "dormant" ? "already answered" : "withdrawn"}.`, "warning");
        return;
      }
      await openSelected(thread, ctx);
    },
  });

  pi.registerShortcut("ctrl+shift+a" as never, {
    description: "Reopen the most recent ws discussion thread.",
    handler: async (ctx) => {
      const thread = mostRecentReopenable([...handle.threads.values()]);
      if (!thread) {
        notify(ctx as AskUiCtx, "ws: no thread to reopen.", "warning");
        return;
      }
      await openThread(pi, ctx as never, bridge, rpcRegistry, handle, thread, sessionCtx);
    },
  });
}
