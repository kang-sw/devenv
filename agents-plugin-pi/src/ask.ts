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
 * A discussion fork (Entry B) is deliberately NOT wrapped in Entry A's
 * structural anti-bleed frame (`buildForkInitialMessage`) and runs NO
 * anti-bleed loop (§4): a discussion fork is meant to speak AS the lead —
 * persona continuity is the feature there, not a bleed to suppress.
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
 * `closeThreadOnDone`/`handleRespondentFinalReport`'s `"lead-ask"` branches)
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

import { readFileSync, writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { getMarkdownTheme } from "@earendil-works/pi-coding-agent";
import type { BridgeHandle } from "./bridge.ts";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { modelCatalogFromToolCtx, tierWarningNotifierFromToolCtx } from "./model-catalog.ts";
import {
  agentWidgetRefreshRef,
  sendToLead,
  inheritModelFromToolCtx,
  sendToAgent,
  refreshAgentTelemetry,
  spawnAgent,
  storageContextFromToolCtx,
  syncOwnershipProtection,
  startOwnedSessionObserver,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
  type ToolGroup,
} from "./spawner.ts";
import { computeForkToolSurface, getForkSourceSessionFile } from "./fork.ts";
import { readSpawnRole, type SpawnRole } from "./process-role.ts";
import {
  ConversationViewComponent,
  conversationOverlayHeight,
  DONE_COMMAND,
  isEscapeKey,
  wrapInBorder,
  type ChildLiveness,
  type ConversationChannel,
  type ConversationItem,
  type ConversationViewTui,
  type EditorLike,
} from "./conversation-view.ts";
import { loadHostPiTui, wrapTextWithAnsi, type Component, type EditorTheme, type MarkdownTheme } from "./pi-tui.ts";
import { captureForkContext, captureRegisteredTools, captureUnflushedForkSource, effectiveForkDescriptor, type ForkContext } from "./fork-context.ts";
import type { LeadPromptRef } from "./lead-bootstrap.ts";
import { readOwnership, validDescriptor } from "./agent-storage.ts";
import { parseTelemetry, type AgentTelemetry, type TelemetryOrigin } from "./agent-telemetry.ts";

// ---------------------------------------------------------------------------
// Pure helpers. Unit-tested directly (test/ask.test.ts) with no
// filesystem/subprocess/live `pi` session involved.
// ---------------------------------------------------------------------------

/**
 * Lead-facing verb-table tool name (pi-lead-guide.md): queue an async owner
 * question. `260911` renamed this from `ws-ask` — a contract change (async /
 * non-blocking / "answer when ready"), not a cosmetic one: the old name drove
 * the model toward "ask now" framing. See this file's header for the
 * fork-less redesign this rename ships with.
 */
export const ASK_TOOL_NAME = "ws-queue-question";

/**
 * Lead-facing verb-table tool name: withdraw a still-pending queued question.
 * `260911` renamed this from `ws-resolve` for the same reason as
 * `ASK_TOOL_NAME` — "withdraw a queued question" reads correctly against the
 * new async-queue framing where `ws-resolve` did not.
 */
export const RESOLVE_TOOL_NAME = "ws-withdraw-question";

/** `pi.sendMessage` custom-message type for a closed discussion thread's summary (§6). */
export const THREAD_SUMMARY_CUSTOM_TYPE = "ws-thread-summary";

/** Ancestor entries rendered into a post-compaction verbatim excerpt (§7). */
export const EXCERPT_WINDOW = 4;

/** Per-entry character budget inside a verbatim excerpt — keeps a compacted anchor cheap. */
export const EXCERPT_ENTRY_CHARS = 400;

/**
 * §7's "context is bounded" budget for `ws-ask`'s own `context` argument,
 * expressed the way §7 asks for it: an ADAPTER-side length warning, not a
 * truncation. The lead's text is always stored unchanged — silently clipping
 * a question's background would corrupt the very thing the owner needs to
 * answer it — so an over-budget context only produces a `ctx.ui.notify`
 * warning naming the overage.
 */
export const MAX_CONTEXT_CHARS = 400;

/**
 * Pure half of the §7 bound: returns the owner-facing warning text when
 * `context` is over `MAX_CONTEXT_CHARS`, `undefined` otherwise. Never
 * rewrites the context (see `MAX_CONTEXT_CHARS`).
 */
export function checkContextLength(context: string | undefined, limit = MAX_CONTEXT_CHARS): string | undefined {
  if (!context || context.length <= limit) return undefined;
  return `ws: question context is ${context.length} chars (over the ${limit}-char guideline) — it is stored in full, but a shorter one is easier for the owner to answer.`;
}

/**
 * What the overlay's `ctx.ui.custom` `done` callback + the summarize-then-
 * close helper are wrapped as, handed to `onOpened` (moved verbatim from the
 * old per-thread overlay module, now deleted, with one shape addition):
 * `close()` closes the view only (the fork and its thread are untouched);
 * `closeWithSummary(summary)` ends the thread with a supplied summary.
 * Reused unchanged by `handleRespondentFinalReport`'s
 * `overlay.closeWithSummary(message)` path.
 *
 * Review relay #2 C1/I1: `closeWithSummary`'s `alreadyRendered` parameter
 * (default `false`, so every EXISTING caller keeps its old append-then-close
 * behavior unchanged) exists for exactly one caller —
 * `summarizeThenClose`'s own settled-turn completion — whose summary text
 * was ALREADY appended to the view by the component's own internal
 * `agent_settled` handling (the same event, a separate listener registered
 * first). Passing `true` there skips the redundant second append that used
 * to double the summary turn on screen and in `thread.transcript`; the
 * thread-close side effects (`closeThreadOnDone`, `done`) still run exactly
 * as before.
 */
export interface OverlayHandle {
  /** Close the view only (the thread is untouched). */
  close(): void;
  /**
   * End the view with a supplied summary. `alreadyRendered: true` (used only
   * by `summarizeThenClose`) skips appending `summary` to the view — it is
   * already there, appended by the component's own settle handling.
   */
  closeWithSummary(summary: string, alreadyRendered?: boolean): void;
}

/**
 * Review relay #1 I4: owner-facing rendering of a thread's registration time
 * in the overlay's header (moved verbatim from the old, now-deleted per-thread overlay module).
 * Deliberately UTC-and-labeled rather than locale-formatted so the header is
 * identical in a test run, a CI container and the owner's terminal.
 * `undefined` for a missing or unparseable timestamp — an old registry entry
 * must not break the header.
 */
export function formatSpawnTime(iso: string | undefined): string | undefined {
  if (!iso) return undefined;
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return undefined;
  return `${at.toISOString().slice(0, 16).replace("T", " ")} UTC`;
}

/**
 * The fixed message sent to the fork when the owner types `/done` on a
 * `summarizeOnDone` thread. One round-trip only: the fork's next settled
 * turn IS the summary — there is no separate hand-shake protocol. Moved
 * verbatim from the old, now-deleted per-thread overlay module.
 */
export function buildDoneSummaryPrompt(): string {
  return "The owner ended the discussion. Write a concise summary of what was decided now — a few sentences, no preamble, no questions back.";
}

/** Fallback when the fork settles the `/done` turn without producing any text. Moved verbatim from the old, now-deleted per-thread overlay module. */
export const EMPTY_SUMMARY_TEXT = "(the discussion ended without a summary from the thread)";

/**
 * Review relay #1 I6: what the LEAD sees in place of a fork-raised question's
 * own report text, in TUI mode only. Ticket §1 keeps the lead out of a
 * fork-raised question entirely and §8 scopes the lead relay to headless, so
 * in TUI the owner surface is the only answering channel: the lead is told
 * the thread id, who answers it, and to keep waiting rather than relay or
 * answer it itself.
 */
export function buildForkQuestionLeadNotice(agentId: string, threadId: string): string {
  return [
    `[ws] Agent ${agentId} raised a question for the OWNER, registered as thread ${threadId}.`,
    "The owner answers it directly in their own discussion overlay (/answer " + threadId + "); you are not part of that exchange.",
    "Do NOT relay this question, answer it yourself, or ask the owner about it. End your turn — this agent resumes its task once the owner replies, and what was decided reaches you in its own pushed final report's Decisions: line.",
  ].join("\n");
}

/**
 * §1 thread record. Plain data only (no `RpcClient`, no captured `ctx`) so
 * the whole registry round-trips through JSON — §5's "store only plain data
 * in the registry" rule, which is also what makes the file persistence below
 * possible at all.
 */
export interface ThreadRecord {
  threadId: string;
  title: string;
  question?: string;
  context?: string;
  /**
   * The lead-session entry this question was raised at
   * (`sessionManager.getLeafId()` at ask time). Mechanically recorded by the
   * adapter, NEVER authored by the model (§1). Set only for a `ws-ask`-
   * originated thread — a fork-raised thread has no lead entry to anchor to
   * (§7 / the plan's `spawner.ts#L1695` finding).
   */
  entryId?: string;
  /**
   * D3 return-path anchor (`260911`): a best-effort short `git rev-parse
   * --short HEAD` captured at `ws-queue-question` ask time, paired with
   * `entryId` on the return-path injection (`deliverQueuedAnswer` /
   * `buildAskAnchorLine`) so the lead can recover where a queued question
   * came from even past a compaction boundary. `"lead-ask"`-origin only;
   * `undefined` outside a git worktree or on any git-command failure — a
   * missing anchor must never block registering the question.
   */
  askCommitHash?: string;
  status: ThreadStatus;
  /**
   * Which of §1's two entries registered this thread. Load-bearing at
   * `/done` time (review relay #2 C2): the two entries have opposite
   * respondent lifecycles — see `ThreadOrigin`.
   */
  origin: ThreadOrigin;
  /** The agent_id of the fork answering this thread, once one exists. */
  respondentAgentId?: string;
  /** Denormalized resume fields for `respondentAgentId` — see this file's header. */
  forkResume?: PersistedForkResume;
  /**
   * The `ConversationViewComponent` transcript (owner turns, settled child
   * turns, tool calls/results, adapter notes), newest last and capped at
   * `THREAD_TRANSCRIPT_CAP` entries. Persisted with the record so a reopen
   * after Esc — or after a lead restart — shows the conversation so far
   * instead of an empty view (dogfood 2026-09-05). Absent until the thread is
   * first opened. A record written before this ticket carries the legacy
   * `{who,text}[]` shape instead — `normalizeTranscript` converts it on
   * hydrate.
   */
  transcript?: ConversationItem[];
  createdAt: string;
  /** Last open/answer/close touch — orders the "reopen the most recent" shortcut. */
  touchedAt: string;
  /**
   * D-model-side-withdrawal (`260911`, `"lead-ask"`-origin only): set when
   * `ws-withdraw-question` is called while the owner has this thread's
   * fork-less answer view open (`status === "open"`) — the removal cannot be
   * applied immediately without yanking an in-progress edit, so it is
   * deferred to whenever that view closes (Phase 2's
   * `LeadAskQueueComponent.onClose`, via `runLeadAskEscapeAction`). Always
   * `false`/absent otherwise; normalized back to
   * `false` on hydrate (see `hydrateThreadRegistry`) since no view can
   * survive a lead-process restart.
   */
  withdrawnPending?: boolean;
  /**
   * Phase 2 (`260911`, sequential prose-modal tier, `"lead-ask"`-origin
   * only): the owner's current, unsubmitted prose for this question,
   * persisted so closing and reopening the queue resumes typed text (D3
   * "per-question drafts persist" — the persistence MECHANISM already
   * existed pre-Phase-2; this field is what it now carries). Cleared once
   * the answer is delivered (`deliverQueuedAnswer`) or the question is
   * finalized as withdrawn with nothing typed (`runLeadAskEscapeAction`'s
   * `"finalize-withdrawal"` branch); left as-is while the question simply
   * stays `"pending"` between sittings.
   */
  draftAnswer?: string;
}

/**
 * `"pending"` — registered, never opened (costs nothing, §9).
 * `"open"` — a respondent fork exists and the thread is live.
 * `"dormant"` — closed via `/done` but retained and reopenable (§9).
 * `"closed"` — lead self-resolved via `ws-resolve`; no injection, the lead
 * already knows the answer.
 */
export type ThreadStatus = "pending" | "open" | "dormant" | "closed";

/**
 * §1's two entries, recorded at registration because they own their
 * respondent differently (review relay #2 C2):
 *
 * - `"lead-ask"` — Entry B. `ws-ask` registered it and `ensureRespondent`
 *   LAZILY SPAWNED the discussion fork this surface owns end to end. `/done`
 *   is that fork's whole purpose: ask it for a summary, inject the summary
 *   into the lead, then stop it (§6/§9 `ws-agent-stop` semantics).
 * - `"fork-raised"` — Entry A. A live `ws-fork` TASK fork raised the question
 *   mid-task via `ws-report-to-lead(kind:"question")`; its lifecycle belongs
 *   to `ws-fork`/`ws-agent-stop`, not to this surface. `/done`
 *   therefore only detaches the overlay: no summary request, no stop, no
 *   injection. The fork resumes its task and the lead learns the outcome from
 *   its own `kind:"final"` report's `Decisions:` line (§1/§4).
 *
 * A record parsed without this field is treated as `"fork-raised"`: the
 * conservative default, since that is the origin whose respondent must never
 * be stopped by mistake.
 */
export type ThreadOrigin = "lead-ask" | "fork-raised";

/** Normalizes a persisted/unknown `origin` value; see `ThreadOrigin` for why the default is the conservative one. */
export function normalizeThreadOrigin(value: unknown): ThreadOrigin {
  return value === "lead-ask" ? "lead-ask" : "fork-raised";
}

/** Newest transcript entries kept per thread (`ThreadRecord.transcript`); older ones are dropped on write and on parse. */
export const THREAD_TRANSCRIPT_CAP = 200;

/** Maps a legacy `TranscriptEntry.who` value onto its `ConversationItem.kind` equivalent — see `normalizeTranscript`. */
const LEGACY_WHO_TO_KIND: Record<string, "user" | "assistant" | "note"> = {
  you: "user",
  thread: "assistant",
  note: "note",
};

/**
 * One persisted transcript entry, tolerantly converted to a `ConversationItem`
 * or dropped (`undefined`) when malformed. Accepts BOTH shapes: the legacy
 * `{who,text}` entry (`"you"`->`{kind:"user",...}`, `"thread"`->
 * `{kind:"assistant",...}`, `"note"`->`{kind:"note",...}`) written before this
 * ticket, and the native `ConversationItem` `{kind,...}` shape, validated
 * per-kind (`tool-call` needs `id`/`name`; `tool-result` needs `id`/`name`/
 * `content`, `isError` optional; the rest need `text: string`).
 */
function normalizeTranscriptEntry(entry: unknown): ConversationItem | undefined {
  const candidate = entry as
    | { who?: unknown; kind?: unknown; text?: unknown; id?: unknown; name?: unknown; args?: unknown; content?: unknown; isError?: unknown }
    | null;
  if (!candidate || typeof candidate !== "object") return undefined;
  if (typeof candidate.who === "string") {
    const kind = LEGACY_WHO_TO_KIND[candidate.who];
    return kind && typeof candidate.text === "string" ? ({ kind, text: candidate.text } as ConversationItem) : undefined;
  }
  switch (candidate.kind) {
    case "user":
    case "assistant":
    case "lead-message":
    case "note":
      return typeof candidate.text === "string" ? ({ kind: candidate.kind, text: candidate.text } as ConversationItem) : undefined;
    case "tool-call":
      return typeof candidate.id === "string" && typeof candidate.name === "string"
        ? { kind: "tool-call", id: candidate.id, name: candidate.name, args: candidate.args }
        : undefined;
    case "tool-result":
      return typeof candidate.id === "string" && typeof candidate.name === "string" && typeof candidate.content === "string"
        ? {
            kind: "tool-result",
            id: candidate.id,
            name: candidate.name,
            content: candidate.content,
            ...(typeof candidate.isError === "boolean" ? { isError: candidate.isError } : {}),
          }
        : undefined;
    default:
      return undefined;
  }
}

/**
 * Tolerant read of a persisted `transcript`: a non-array is `undefined`
 * (the field is simply absent), malformed entries are dropped, and the
 * result is capped to the newest `THREAD_TRANSCRIPT_CAP` — a hand-edited or
 * older registry file must never make a thread unopenable. See
 * `normalizeTranscriptEntry` for the legacy/native per-entry conversion.
 */
export function normalizeTranscript(value: unknown): ConversationItem[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const entries = value.map((entry) => normalizeTranscriptEntry(entry)).filter((entry): entry is ConversationItem => entry !== undefined);
  return entries.length > THREAD_TRANSCRIPT_CAP ? entries.slice(entries.length - THREAD_TRANSCRIPT_CAP) : entries;
}

/**
 * The `ConversationViewComponent` initial transcript for opening/reopening a
 * thread. The original question is always the first dialogue turn when one is
 * recorded on the thread. Older persisted transcripts either carry it as a
 * leading `Question: ...` note or omit it because the header used to be its
 * only presentation; upgrade/prepend that one turn without disturbing the
 * later history. Exported for direct testing (pure — no component/channel
 * needed).
 */
export function buildInitialConversationItems(thread: Pick<ThreadRecord, "transcript" | "question" | "title">): ConversationItem[] {
  const question = thread.question?.trim();
  const transcript = thread.transcript ?? [];
  if (!question) return transcript;

  const questionTurn: ConversationItem = { kind: "assistant", text: `**Question:** ${question}` };
  const first = transcript[0];
  if (!first) return [questionTurn];

  const firstText = "text" in first ? first.text.trim() : undefined;
  if (first.kind === "note" && (firstText === question || firstText === `Question: ${question}`)) {
    return [questionTurn, ...transcript.slice(1)];
  }
  if (first.kind === "assistant" && (firstText === question || firstText === `Question: ${question}` || firstText === `**Question:** ${question}`)) {
    return transcript;
  }
  return [questionTurn, ...transcript];
}

/** Compact metadata/control header; the question itself belongs in the transcript. */
export function buildThreadHeaderHint(thread: Pick<ThreadRecord, "threadId" | "createdAt">): string {
  const opened = formatSpawnTime(thread.createdAt);
  const metadata = [`ws thread ${thread.threadId}`, ...(opened ? [`opened ${opened}`] : [])].join(" · ");
  return [metadata, `Esc: close view (thread stays open) · ${DONE_COMMAND}: end thread`].join("\n");
}

/**
 * Everything needed to hand-reconstruct an `RpcAgentRecord` for a dormant
 * respondent fork after a lead-process restart, denormalized into the thread
 * registry's own persisted file. Every field is a public `RpcAgentRecord`
 * field (see the plan's `spawner.ts#L647-706` finding) — this is a copy, not
 * a new contract.
 */
export interface PersistedForkResume {
  sessionPath: string;
  systemPromptPath?: string;
  forkContext?: ForkContext;
  explicitTools?: string;
  wsToolNames: string[];
  toolGroup: ToolGroup;
  modelBase?: string;
  modelEffort?: string;
  telemetry?: AgentTelemetry;
  telemetryInputFloor?: TelemetryOrigin;
  observedModel?: string;
  observedEffort?: string;
  observedLatestInput?: number;
  ownership?: import("./agent-storage.ts").AgentOwnership;
}

/**
 * Next free `q<N>` thread id given the ids already in the registry. Short and
 * typeable on purpose — the owner types it as `/answer q3`. Ids are never
 * reused within a registry file: the counter walks past every existing
 * numeric suffix, including those of closed/dormant threads.
 */
export function nextThreadId(existingIds: readonly string[]): string {
  let max = 0;
  for (const id of existingIds) {
    const match = /^q(\d+)$/.exec(id);
    if (match) {
      const n = Number.parseInt(match[1], 10);
      if (Number.isFinite(n) && n > max) max = n;
    }
  }
  return `q${max + 1}`;
}

/**
 * Title for a fork-raised thread: its `ws-report-to-lead` payload is one
 * free-text `message` with no structured `title` field, so a title must be
 * derived (first non-empty line, truncated). Falls back to a fixed label for
 * an empty message rather than producing an unlabelled thread.
 */
export function deriveThreadTitle(message: string, maxLength = 60): string {
  const firstLine = message
    .split("\n")
    .map((line) => line.trim())
    .find((line) => line.length > 0);
  if (!firstLine) return "(untitled question)";
  return firstLine.length <= maxLength ? firstLine : `${firstLine.slice(0, maxLength - 1).trimEnd()}…`;
}

/**
 * The registry's per-lead-session file: a sibling of the lead's own session
 * file (`ctx.sessionManager.getSessionFile()`). Pi never writes this suffix
 * itself, and keying off the session file means a second concurrent lead
 * session gets its own registry for free.
 */
export function threadRegistryPath(sessionFile: string): string {
  return `${sessionFile}.ws-threads.json`;
}

/** Stable, pretty-printed on-disk form (a hand-inspectable adapter data file). */
export function serializeThreadRegistry(records: readonly ThreadRecord[]): string {
  return `${JSON.stringify({ threads: records }, null, 2)}\n`;
}

/**
 * Tolerant parse: anything that is not a well-formed `{threads:[...]}`
 * document degrades to `[]` rather than throwing — same never-throw contract
 * `readGoalLoopConfig` already uses for adapter-owned data files. Individual
 * entries missing a `threadId`/`status` are dropped rather than poisoning
 * the whole registry.
 */
export function parseThreadRegistry(raw: string): ThreadRecord[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  const threads = (parsed as { threads?: unknown } | null)?.threads;
  if (!Array.isArray(threads)) return [];
  return threads
    .filter((entry): entry is ThreadRecord => {
      const candidate = entry as Partial<ThreadRecord> | null;
      return (
        typeof candidate?.threadId === "string" &&
        candidate.threadId.length > 0 &&
        typeof candidate.title === "string" &&
        (candidate.status === "pending" || candidate.status === "open" || candidate.status === "dormant" || candidate.status === "closed")
      );
    })
    // `origin` is normalized rather than validated away: an entry written
    // before the field existed is still a usable thread, and defaulting it to
    // "fork-raised" is the safe direction (see `ThreadOrigin`). `transcript`
    // likewise: absent or malformed simply means "no transcript yet".
    .map((entry) => {
      const { transcript, ...rest } = entry as ThreadRecord & { transcript?: unknown };
      const normalized = normalizeTranscript(transcript);
      return { ...rest, origin: normalizeThreadOrigin(entry.origin), ...(normalized ? { transcript: normalized } : {}) };
    });
}

/** §5 widget wording counts PENDING threads only — an already-open thread is not something the owner still owes an answer to. Also feeds `agent-widget.ts`'s panel-heading question count. */
export function countPending(records: readonly ThreadRecord[]): number {
  return records.filter((record) => record.status === "pending").length;
}

/** `/thread`'s rendering: every thread except lead-self-resolved (`"closed"`) ones, newest touch first. */
export function buildThreadListLines(records: readonly ThreadRecord[]): string[] {
  const listed = records.filter((record) => record.status !== "closed");
  if (listed.length === 0) return ["ws threads: none open or pending."];
  const sorted = [...listed].sort((a, b) => (a.touchedAt < b.touchedAt ? 1 : a.touchedAt > b.touchedAt ? -1 : 0));
  return [
    `ws threads (${sorted.length}):`,
    ...sorted.map((record) => {
      const respondent = record.respondentAgentId ? ` [fork ${record.respondentAgentId.slice(0, 8)}]` : "";
      return `  ${record.threadId}  ${record.status.padEnd(7)}${respondent}  ${record.title}`;
    }),
  ];
}

/**
 * The thread a bare `/answer` (no id) or the reopen shortcut acts on: the
 * most recently touched thread that is still answerable — pending, open, or
 * dormant-but-retained (§9). `undefined` when there is nothing to reopen.
 */
export function mostRecentReopenable(records: readonly ThreadRecord[]): ThreadRecord | undefined {
  const candidates = records.filter((record) => record.status !== "closed");
  if (candidates.length === 0) return undefined;
  return candidates.reduce((best, record) => (record.touchedAt > best.touchedAt ? record : best));
}

/**
 * §7 compaction check: `true` while the recorded `entryId` is still inside
 * the lead's own live context (`ctx.sessionManager.buildContextEntries()`).
 * Sessions are append-only, so an entry that has fallen out of this list has
 * fallen BEHIND a compaction boundary — it still exists in the session tree
 * (`getBranch`), it is just no longer in the model's context, which is
 * exactly when a verbatim excerpt has to be inserted into the fork's first
 * message instead.
 */
export function isEntryLive(entryId: string, liveEntries: readonly { id: string }[]): boolean {
  return liveEntries.some((entry) => entry.id === entryId);
}

/**
 * Tolerant text of one session entry, for the §7 excerpt only. Handles the
 * `SessionMessageEntry` shape (`message.content` as a plain string or as
 * `{type:"text",text}` parts) and the summary-bearing entries; anything else
 * renders as its bare type so the excerpt never crashes on an entry shape
 * this adapter does not model.
 */
export function extractEntryText(entry: unknown): string {
  const e = entry as { type?: string; summary?: unknown; content?: unknown; message?: { role?: string; content?: unknown } } | null;
  if (!e) return "";
  if (typeof e.summary === "string") return e.summary;
  const content = e.message?.content ?? e.content;
  if (typeof content === "string") return content;
  if (Array.isArray(content)) {
    return content
      .map((part) => {
        const p = part as { type?: string; text?: unknown } | null;
        return p?.type === "text" && typeof p.text === "string" ? p.text : "";
      })
      .filter((text) => text.length > 0)
      .join("\n");
  }
  return "";
}

/**
 * §7 verbatim excerpt: a small window of the ancestor chain ENDING at
 * `entryId` (`ctx.sessionManager.getBranch(entryId)`), rendered as plain
 * text for insertion into a discussion fork's first message when the anchor
 * entry has fallen behind a compaction boundary. Returns `""` when the
 * entry is not in the given branch at all — the caller then simply omits the
 * excerpt section (never fabricates one). Rejected alternatives (§7):
 * model-authored time-capsules, and rewinding the lead session.
 */
export function buildVerbatimExcerpt(entryId: string, branch: readonly { id: string }[], windowSize = EXCERPT_WINDOW): string {
  const index = branch.findIndex((entry) => entry.id === entryId);
  if (index < 0 || windowSize <= 0) return "";
  const window = branch.slice(Math.max(0, index - windowSize + 1), index + 1);
  return window
    .map((entry) => {
      const role = (entry as { message?: { role?: string }; type?: string }).message?.role ?? (entry as { type?: string }).type ?? "entry";
      const text = extractEntryText(entry).trim();
      const clipped = text.length > EXCERPT_ENTRY_CHARS ? `${text.slice(0, EXCERPT_ENTRY_CHARS - 1)}…` : text;
      return { role, text: clipped };
    })
    // Entries with no renderable text (model/thinking-level changes, labels)
    // carry nothing for the fork to read — drop them rather than emitting
    // bare role markers.
    .filter((rendered) => rendered.text.length > 0)
    .map((rendered) => `[${rendered.role}] ${rendered.text}`)
    .join("\n\n");
}

/**
 * Role-differentiated `ws-queue-question`/`ws-withdraw-question` active-tools
 * shaping, identical in shape to `fork.ts`'s `addForkToolIfLead` and kept as
 * its own function for the same reason: `execute-gateway.ts`'s shared
 * `computeLeadActiveTools`/`LEAD_ADDED_TOOL_NAMES` are applied to lead AND
 * fork roles alike, so folding these two in there would hand a fork the very
 * tools `FORK_EXCLUDED_TOOL_NAMES` exists to keep away from it.
 *
 * `260911`: lifts the temporary hide (`ac998f77`/`a8cf1183`, superseded here
 * by the fork-less redesign) as part of landing the fork-less lead-raised
 * path — this is a re-enable of the tool surface, not of the old
 * fork-spawning behavior it used to drive. The true top lead (`role ===
 * undefined`) gains both tools (deduped, appended only if missing); every
 * other role's active list passes through unchanged, since a fork never had
 * them appended in the first place (its handler-level refusal in
 * `registerAsk`'s `execute()` is the actual enforcement — see this file's
 * header's golden-rule comment).
 */
export function addAskToolsIfLead(activeTools: readonly string[], role: SpawnRole | undefined): string[] {
  if (role !== undefined) return [...activeTools];
  const result = [...activeTools];
  for (const name of [ASK_TOOL_NAME, RESOLVE_TOOL_NAME]) {
    if (!result.includes(name)) result.push(name);
  }
  return result;
}

/**
 * Entry B's system-prompt directive (`--append-system-prompt`, ephemeral
 * per-spawn file, same as `fork.ts`'s own). Short natural language,
 * conversation constraints only. Framing-free on purpose (§4's
 * directive-style rule): a discussion fork is meant to speak as the lead, so
 * nothing here tries to give it a separate identity.
 *
 * The thread has two exits: the owner's `/done` (which asks for a summary
 * turn), and — post-close dogfood 2026-09-05 — the fork's own
 * `ws-report-to-lead(kind:"final")` once the owner has stated a decision,
 * whose text IS the summary (`handleRespondentFinalReport`). No progress
 * reports, no task frame.
 */
export function buildDiscussionForkDirectiveText(): string {
  return [
    "Side-discussion thread: this session is a clone of the lead's own session, opened so its owner can talk one question through directly.",
    "",
    "Reply conversationally and briefly, in the same voice as the rest of this conversation. Answer what is asked, say plainly when something is genuinely undecided, and ask back only when the answer actually depends on it.",
    "",
    "There is no task to complete and no progress report to file here. Do not start editing files or running work unless the owner explicitly asks for it in this thread.",
    "",
    'The owner may end the thread themselves with /done, in which case you will be asked once for a short summary. When the owner states a decision, or says they will go a certain way, end the thread yourself: call ws-report-to-lead with kind:"final" and a short summary of what was decided — 2 to 4 sentences, the decision first. That summary is delivered to the lead.',
  ].join("\n");
}

/**
 * Entry B's first message. Deliberately NOT wrapped in
 * `buildForkInitialMessage`'s structural frame (the 260905 re-decision is
 * Entry A only — see this file's header): no "# Forked session" header, no
 * "--- Message from the lead ---" fence, no demotion of the inherited
 * conversation. A discussion fork continues the same conversation as itself;
 * that continuity is the feature.
 *
 * `excerpt` carries the §7 post-compaction verbatim window when the anchor
 * entry has fallen out of live context; it is omitted entirely otherwise.
 */
export function buildDiscussionForkInitialMessage(context: string | undefined, question: string, excerpt?: string): string {
  const lines: string[] = [buildDiscussionForkDirectiveText(), "", "The owner opened a side discussion about this question."];
  if (context && context.trim().length > 0) {
    lines.push("", `Context: ${context.trim()}`);
  }
  if (excerpt && excerpt.trim().length > 0) {
    lines.push(
      "",
      "The part of the conversation this refers to is no longer in your live context (it was compacted). Here it is verbatim:",
      "",
      excerpt.trim(),
    );
  }
  lines.push("", `Question: ${question.trim()}`, "", "Answer it directly, then keep talking with the owner until they end the thread.");
  return lines.join("\n");
}

/**
 * §6 injection payload: `context + original question + summary`, delivered as
 * a Pi CUSTOM message (`pi.sendMessage`, not `sendUserMessage`) so the lead
 * can tell it apart from a real owner turn.
 *
 * Review relay #2 (co-located Minor): the opening line must not demote the
 * summary. §6 is explicit that it "carries owner authority: the owner was
 * present" — so it reads as the owner's own decisions, while still being
 * labeled a thread summary rather than a fresh owner turn.
 */
export function buildInjectionMessage(context: string | undefined, question: string | undefined, summary: string): string {
  const lines: string[] = [
    "A side discussion with the owner has closed. These are the owner's decisions from that thread — they carry the owner's authority, delivered as a thread summary rather than as a new owner turn.",
  ];
  if (context && context.trim().length > 0) {
    lines.push("", `Context: ${context.trim()}`);
  }
  if (question && question.trim().length > 0) {
    lines.push("", `Question: ${question.trim()}`);
  }
  lines.push("", "Summary of what was decided:", summary.trim());
  return lines.join("\n");
}

/**
 * `260911` D1/D3 fork-less return-path payload: `context + original question
 * + the owner's own prose answer`, in the same "carries owner authority"
 * spirit as `buildInjectionMessage` above (which this does not replace — see
 * this file's header comment), extended with the D3 anchor line (ask-time
 * short commit hash / `entry_id`) and, when that entry has since fallen
 * behind a compaction boundary, the verbatim excerpt around it — so the lead
 * can recover where the question came from even after losing the live
 * context it was asked in. Delivered the same way, as a Pi CUSTOM message
 * (`THREAD_SUMMARY_CUSTOM_TYPE`) via `sendToLead`'s `followUp` path.
 */
export function buildQueuedAnswerInjectionMessage(
  context: string | undefined,
  question: string | undefined,
  answer: string,
  anchor?: string,
  excerpt?: string,
): string {
  const lines: string[] = [
    "The owner answered a queued question. This is their own prose reply — it carries the owner's authority, delivered as a queued-question answer rather than a new owner turn.",
  ];
  if (anchor) lines.push("", anchor);
  if (context && context.trim().length > 0) lines.push("", `Context: ${context.trim()}`);
  if (question && question.trim().length > 0) lines.push("", `Question: ${question.trim()}`);
  if (excerpt && excerpt.trim().length > 0) {
    lines.push(
      "",
      "The part of the conversation this refers to is no longer in your live context (it was compacted). Here it is verbatim:",
      "",
      excerpt.trim(),
    );
  }
  lines.push("", "Owner's answer:", answer.trim());
  return lines.join("\n");
}

/** Denormalizes the resume-relevant half of a live record into JSON-safe plain data (see `PersistedForkResume`). */
export function captureForkResume(record: RpcAgentRecord): PersistedForkResume {
  return {
    sessionPath: record.sessionPath,
    systemPromptPath: record.systemPromptPath,
    ...(record.forkContext ? { forkContext: record.forkContext } : {}),
    explicitTools: record.explicitTools,
    wsToolNames: [...record.wsToolNames],
    toolGroup: record.toolGroup,
    modelBase: record.modelBase,
    modelEffort: record.modelEffort,
    ...(record.telemetry ? { telemetry: record.telemetry } : {}),
    ...(record.telemetryInputFloor ? { telemetryInputFloor: record.telemetryInputFloor } : {}),
    ...(record.observedModel ? { observedModel: record.observedModel } : {}),
    ...(record.observedEffort ? { observedEffort: record.observedEffort } : {}),
    ...(record.observedLatestInput !== undefined ? { observedLatestInput: record.observedLatestInput } : {}),
    ...(record.ownership ? { ownership: record.ownership } : {}),
  };
}

/**
 * Rebuilds a dormant `RpcAgentRecord` from its persisted resume fields, with
 * every runtime field at its post-stop resting value (`client: undefined` is
 * the one that matters — it is what makes `sendToAgent` take its existing
 * dormant-auto-resume branch and relaunch the child via
 * `--session sessionPath`). No resume logic is duplicated here; this only
 * puts the record back on the shared registry so `sendToAgent` can find it.
 */
export function rehydrateForkRecord(agentId: string, resume: PersistedForkResume): RpcAgentRecord {
  const ownership = resume.ownership && validDescriptor(resume.ownership) && resume.ownership.agentId === agentId && resume.ownership.sessionPath === resume.sessionPath && (() => { const disk = readOwnership(resume.ownership!.home); return !!disk && disk.home === resume.ownership!.home && disk.ownerSessionId === resume.ownership!.ownerSessionId && disk.agentId === agentId && disk.sessionPath === resume.sessionPath && disk.role === resume.ownership!.role && disk.exploreMode === resume.ownership!.exploreMode; })() ? resume.ownership : undefined;
  const record: RpcAgentRecord = {
    agentId,
    client: undefined,
    sessionPath: resume.sessionPath,
    ...(ownership ? { ownership } : {}),
    systemPromptPath: resume.systemPromptPath,
    ...(resume.forkContext ? { forkContext: resume.forkContext } : {}),
    modelBase: resume.modelBase,
    modelEffort: resume.modelEffort,
    ...(parseTelemetry(resume.telemetry) ? { telemetry: parseTelemetry(resume.telemetry) } : {}),
    ...(parseTelemetry({ version: 1, origin: resume.telemetryInputFloor })?.origin ? { telemetryInputFloor: parseTelemetry({ version: 1, origin: resume.telemetryInputFloor })!.origin } : {}),
    ...(typeof resume.observedModel === "string" && resume.observedModel ? { observedModel: resume.observedModel } : {}),
    ...(typeof resume.observedEffort === "string" && resume.observedEffort ? { observedEffort: resume.observedEffort } : {}),
    ...(typeof resume.observedLatestInput === "number" && Number.isFinite(resume.observedLatestInput) && resume.observedLatestInput >= 0 ? { observedLatestInput: resume.observedLatestInput } : {}),
    wsToolNames: [...resume.wsToolNames],
    toolGroup: resume.toolGroup,
    explicitTools: resume.explicitTools,
    spawnRole: "fork",
    streaming: false,
    running: false,
    reportLog: [],
  };
  // Thread-only rows are rendered from forkResume while dormant; reconcile
  // the persisted child file here, never from the widget render path.
  refreshAgentTelemetry(record);
  startOwnedSessionObserver(record);
  return record;
}

/**
 * §5's "`prompt()` when the fork is waiting, `steer()` when it is running",
 * expressed as `sendToAgent`'s own `interrupt` flag: a streaming fork is
 * interrupted (steer), an idle or dormant one is prompted. Pulled out as a
 * one-line predicate so the rule itself is unit-asserted even though the
 * branch it feeds lives inside `sendToAgent` (spawner.ts), which needs a
 * live child to exercise.
 */
export function resolveOwnerSendInterrupt(streaming: boolean): boolean {
  return streaming;
}

/** Tolerant `toolCtx.sessionManager.getLeafId()` read — the §1 mechanically-recorded `entry_id`. Mirrors `getForkSourceSessionFile`'s shape. */
export function getLeafEntryId(toolCtx: unknown): string | undefined {
  const sessionManager = (toolCtx as { sessionManager?: { getLeafId?: () => string | null | undefined } } | undefined)?.sessionManager;
  const id = sessionManager?.getLeafId?.();
  return typeof id === "string" && id.length > 0 ? id : undefined;
}

/**
 * D3 return-path anchor (`260911`): a best-effort short HEAD commit hash,
 * captured at `ws-queue-question` ask time and stored on
 * `ThreadRecord.askCommitHash`. Never throws — matches this file's
 * never-hard-fail convention (`loadThreadRegistryFile`/`saveThreadRegistryFile`):
 * a non-git `cwd` or missing `git` binary simply omits the anchor rather than
 * failing the tool call that registers the question.
 */
export function captureAskCommitHash(cwd: string): string | undefined {
  try {
    const raw = execFileSync("git", ["rev-parse", "--short", "HEAD"], { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] });
    const trimmed = raw.trim();
    return trimmed.length > 0 ? trimmed : undefined;
  } catch {
    return undefined;
  }
}

/**
 * D3 return-path anchor line: pairs the ask-time short commit hash with the
 * mechanically-recorded `entryId` so the lead can recover where a queued
 * question came from even past its own compaction boundary. `undefined`
 * (the line is omitted entirely) when neither half is present.
 */
export function buildAskAnchorLine(commitHash: string | undefined, entryId: string | undefined): string | undefined {
  if (!commitHash && !entryId) return undefined;
  const parts: string[] = [];
  if (commitHash) parts.push(`commit ${commitHash}`);
  if (entryId) parts.push(`entry ${entryId}`);
  return `Asked at: ${parts.join(", ")}`;
}

// ---------------------------------------------------------------------------
// IO glue: the persisted registry file, the widget, tool/command
// registration, and the lazy discussion-fork spawn/attach path. Not unit
// tested here — see this file's header comment.
// ---------------------------------------------------------------------------

/**
 * Minimal structural view of the `ctx`/`toolCtx` surface this module needs,
 * kept duck-typed (rather than importing `ExtensionContext`) so every seam
 * below is drivable from a plain object in tests — the same convention
 * `getForkSourceSessionFile`/`inheritModelFromToolCtx` already use.
 */
export interface AskUiCtx {
  mode?: string;
  ui?: {
    notify?(message: string, type?: "info" | "warning" | "error"): void;
  };
}

/**
 * In-memory registry plus the two refs the deferred, event-driven paths need
 * (§5's captured-`ctx` staleness rule: the `ctx` is re-captured on EVERY
 * `session_start` and only plain data is kept in `threads`).
 */
export interface ThreadRegistryHandle {
  threads: Map<string, ThreadRecord>;
  ctxRef: { current: AskUiCtx | undefined };
  pathRef: { current: string | undefined };
}

export function createThreadRegistryHandle(): ThreadRegistryHandle {
  return { threads: new Map(), ctxRef: { current: undefined }, pathRef: { current: undefined } };
}

/** Never-throw read (`readGoalLoopConfig`'s contract): a missing/corrupt file degrades to an empty registry. */
export function loadThreadRegistryFile(path: string): ThreadRecord[] {
  let raw: string;
  try {
    raw = readFileSync(path, "utf8");
  } catch {
    return [];
  }
  return parseThreadRegistry(raw);
}

/** Never-throw write: a failed save degrades to a no-op (the in-memory registry stays authoritative for this process). */
export function saveThreadRegistryFile(path: string, records: readonly ThreadRecord[]): void {
  try {
    writeFileSync(path, serializeThreadRegistry(records));
  } catch {
    // best effort — a read-only/unwritable session dir must never turn a
    // question registration into a crashed tool call.
  }
}

/** Hydrates the in-memory map from the registry file; safe to call on a path that does not exist yet. */
export function hydrateThreadRegistry(handle: ThreadRegistryHandle, path: string): void {
  handle.pathRef.current = path;
  handle.threads.clear();
  for (const record of loadThreadRegistryFile(path)) {
    if (record.respondentAgentId && record.forkResume) {
      // Thread-only rows render this persisted object directly, so reconcile
      // and normalize it at hydration rather than waiting for /answer.
      record.forkResume = captureForkResume(rehydrateForkRecord(record.respondentAgentId, record.forkResume));
    }
    // `260911`: no fork-less lead-ask answer VIEW can survive a lead-process
    // restart, so a persisted "open" lead-ask thread reverts to "pending"
    // (still answerable via a fresh /answer) — unless a model withdrawal was
    // left deferred behind it, in which case that removal now finalizes
    // since there is no longer a view whose in-progress edit it must not
    // yank (see `withdrawQueuedQuestion`/`ThreadRecord.withdrawnPending`).
    if (record.origin === "lead-ask") {
      if (record.status === "open") record.status = record.withdrawnPending ? "closed" : "pending";
      record.withdrawnPending = false;
    }
    handle.threads.set(record.threadId, record);
  }
}

function persistThreads(handle: ThreadRegistryHandle): void {
  const path = handle.pathRef.current;
  if (!path) return;
  saveThreadRegistryFile(path, [...handle.threads.values()]);
}

/**
 * 260905 (live-agent widget ticket): the standalone `N pending` `aboveEditor`
 * widget this function used to repaint is gone — a pending/open question is
 * now a row in `agent-widget.ts`'s merged `belowEditor` panel instead. Every
 * former call site now fires the merged refresh through
 * `spawner.ts`'s `agentWidgetRefreshRef` — the same ref `spawner.ts`'s own
 * registry-transition points use — so this module never has to import
 * `agent-widget.ts` directly (golden rule: this module imports FROM
 * `spawner.ts`, never the reverse). A guarded no-op outside a TUI lead
 * session (`ref.current` is only ever filled there) and best-effort
 * (swallows a throw), matching every other push/refresh call site's
 * convention.
 */
function refreshAgentWidget(): void {
  try {
    agentWidgetRefreshRef.current?.();
  } catch {
    // best effort — see doc comment above.
  }
}

/**
 * Phase 2 (`260911`, sequential prose-modal tier): the live host repaint hook
 * for the currently-open batch queue modal, if any — mirrors `activeOverlay`'s
 * module-scope singleton precedent. `undefined` outside a live queue modal.
 * Filled/cleared by `openLeadAskQueue`; consulted (best-effort) by
 * `withdrawQueuedQuestion` so a model withdrawal's banner shows up
 * immediately rather than waiting for the owner's next keystroke.
 */
let activeQueueRepaint: (() => void) | undefined;

function repaintActiveQueue(): void {
  try {
    activeQueueRepaint?.();
  } catch {
    // best effort — see doc comment above.
  }
}

function notify(ctx: AskUiCtx | undefined, message: string, type?: "info" | "warning" | "error"): void {
  ctx?.ui?.notify?.(message, type);
}

function nowIso(): string {
  return new Date().toISOString();
}

export interface AskSessionCtx {
  cwd: string;
  /** Exact manifest entry module loaded by this parent, used by dormant fork resumes. */
  extensionPath: string;
  effectivePromptRef?: LeadPromptRef;
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
  /**
   * Review relay #1 (I2): only used to arm the respondent's final-report hook
   * here (see below). Optional so the registration itself still works from a
   * call site with no extension API — the thread is registered either way; the
   * bind is then released by the other close paths.
   */
  pi?: ExtensionAPI,
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
    // so the lead must not be pushed this fork's settles/advisories (nor
    // count it as one of its own outstanding children) even before the owner
    // gets around to `/answer`.
    live.threadBound = true;
    syncOwnershipProtection(live);
  }
  handle.threads.set(record.threadId, record);
  // Review relay #1 (I2): arm the final-report hook HERE, not only from
  // `ensureRespondent`. `ensureRespondent` runs on `/answer`, so before this
  // fix the bind set above could only ever be released by an owner who
  // actually opened the thread — and in headless (§8) there is no owner
  // surface at all, so a fork-raised question latched `threadBound` forever:
  // permanently outside the fan-in count, settles permanently suppressed,
  // anti-bleed permanently disarmed, and the lead's fan-in showing no status
  // line while the fork was still working. Armed at registration, the fork's OWN
  // `kind:"final"` closes the thread and releases the bind with no owner
  // involvement (`handleRespondentFinalReport` -> `detachForkRaisedThread`).
  if (pi) armFinalReportHook(pi, handle, rpcRegistry, record.threadId, agentId);
  persistThreads(handle);
  refreshAgentWidget();
  return record;
}

/**
 * Registers `ws-ask`/`ws-resolve` (lead-facing; reachable only after
 * `index.ts`'s role-differentiated `addAskToolsIfLead` step). Registered
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
 * The thread's close, routed on its `origin` — reached from the overlay's
 * `/done` (with the fork's summary turn) and from the respondent's own
 * `kind:"final"` report (`handleRespondentFinalReport`, with the report
 * text). The two entries own their respondent differently (review relay #2
 * C2, see `ThreadOrigin`):
 *
 * - `"lead-ask"`: this surface spawned the discussion fork, so `/done` runs
 *   the full §6/§9 close — summary into the lead, then stop the fork.
 * - `"fork-raised"`: the respondent is a LIVE Entry A task fork the lead is
 *   parked on. Stopping it would destroy its in-flight task and hang the
 *   lead's own delegation of it, so `/done` only
 *   detaches: the thread goes dormant and the fork carries on, reporting what
 *   was decided through its own `kind:"final"` report (§1/§4).
 */
export function closeThreadOnDone(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  rpcRegistry: RpcAgentRegistry,
  thread: ThreadRecord,
  summary: string,
): void {
  if (thread.origin === "lead-ask") {
    injectDiscussionSummary(pi, handle, rpcRegistry, thread, summary);
    return;
  }
  detachForkRaisedThread(handle, rpcRegistry, thread);
}

/**
 * `/done` on a fork-raised thread: close the overlay, keep the task fork
 * running. No summary was ever requested from it, nothing is injected into
 * the lead (§1 keeps the lead out of this exchange entirely), and the thread
 * stays dormant-and-reopenable for as long as the fork lives.
 */
export function detachForkRaisedThread(handle: ThreadRegistryHandle, rpcRegistry: RpcAgentRegistry, thread: ThreadRecord): void {
  const agentId = thread.respondentAgentId;
  if (agentId) {
    const record = rpcRegistry.get(agentId);
    if (record) {
      record.overlayAttached = false;
      // The thread itself is closing here, so the thread-lifetime bind is
      // released too: the fork rejoins the lead's fan-in and its own
      // kind:"final" is pushed to the lead as any other child's would be.
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
      record.overlayAttached = false;
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
export function deliverQueuedAnswer(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  thread: ThreadRecord,
  answer: string,
  sessionManager?: { buildContextEntries?: () => { id: string }[]; getBranch?: (id: string) => { id: string }[] },
): void {
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
  const message = {
    customType: THREAD_SUMMARY_CUSTOM_TYPE,
    content: buildQueuedAnswerInjectionMessage(thread.context, thread.question, answer, anchor, excerpt),
    display: true,
    details: { threadId: thread.threadId, title: thread.title },
  };
  sendToLead(pi, message, "followUp");

  thread.status = "dormant";
  thread.withdrawnPending = false;
  // Phase 2 (260911): the draft is delivered, not merely persisted — clear it
  // so a later hand-edited/inspected registry never shows a stale one.
  thread.draftAnswer = undefined;
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshAgentWidget();
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
/** `ConversationChannel.liveness()`'s two-state read off the registry's `streaming` flag — `"idle-awaiting-owner"` is child B's ownership rule, not this ticket's. Pulled out as a small exported pure helper matching the file's own `resolveOwnerSendInterrupt` precedent. */
export function resolveChildLiveness(streaming: boolean): ChildLiveness {
  return streaming ? "running" : "settled";
}

function createForkChannel(pi: ExtensionAPI, rpcRegistry: RpcAgentRegistry, cwd: string, extensionPath: string, agentId: string): ConversationChannel {
  const listeners = new Set<(evt: unknown) => void>();
  let attached: unknown;
  let detach: (() => void) | undefined;

  function sync(): void {
    const record = rpcRegistry.get(agentId);
    const client = record?.client;
    if (!client || client === attached) return;
    detach?.();
    attached = client;
    detach = client.onEvent((evt) => {
      for (const listener of listeners) listener(evt);
    });
  }

  return {
    onEvent(listener) {
      listeners.add(listener);
      sync();
      return () => {
        listeners.delete(listener);
        if (listeners.size === 0) {
          detach?.();
          detach = undefined;
          attached = undefined;
        }
      };
    },
    liveness() {
      return resolveChildLiveness(rpcRegistry.get(agentId)?.streaming === true);
    },
    async send(text) {
      await sendToAgent(rpcRegistry, { pi, cwd, extensionPath }, agentId, text, resolveOwnerSendInterrupt(rpcRegistry.get(agentId)?.streaming === true));
      sync();
    },
  };
}

/**
 * §5's "one overlay at a time": module-scope, because the overlay outlives
 * the `/answer` handler that opened it (`ctx.ui.custom` resolves only when
 * the overlay closes) and a second `/answer` must be able to close the first
 * one. Closing an overlay never touches its fork — the fork keeps running
 * and the thread stays reopenable (the doom-overlay example's own
 * persistent-state-vs-disposable-view split).
 */
let activeOverlay: { token: number; threadId: string; handle: OverlayHandle } | undefined;
/** Identifies one overlay INSTANCE, not one thread: reopening the same thread must not let the closing instance clear its successor's entry. */
let overlayToken = 0;

/** The live overlay handle for `threadId`, if the one open overlay is attached to that thread. */
function attachedOverlayFor(threadId: string): OverlayHandle | undefined {
  return activeOverlay?.threadId === threadId ? activeOverlay.handle : undefined;
}

/**
 * A respondent's own `kind:"final"` report (post-close dogfood 2026-09-05):
 * the discussion fork ends the thread itself once the owner has stated a
 * decision, and the report text is the summary. Routed exactly like `/done`
 * — through the attached overlay's `closeWithSummary` (whose `onDone` is
 * `closeThreadOnDone`) when one is open, directly through `closeThreadOnDone`
 * when the owner had already pressed Esc — so a `lead-ask` thread gets the
 * §6 injection, the stop, `dormant`, persistence and the widget refresh in
 * one place, with no summary turn. A `fork-raised` thread's final report
 * closes an attached overlay and detaches the thread (its lifecycle belongs
 * to `ws-fork`, and the lead reads the pushed report itself). A `lead-ask`
 * thread is ignored unless it is `open` — a late duplicate from an
 * already-closed thread must not re-inject — while a `fork-raised` thread also
 * accepts `pending` (review relay #1 I2: the owner may never open it, and in
 * headless never can, so this is that bind's only release path).
 *
 * 260905 return value = `spawner.ts`'s `onFinalReport` SUPPRESSION contract:
 * `true` means "consumed, do not push this report to the lead". Only a
 * `lead-ask` thread returns true — the owner's decision already reaches the
 * lead as the `ws-thread-summary` message, so a `ws-agent-report` push on top
 * would deliver the same event twice. A `fork-raised` fork's final IS the
 * completion signal the lead is meant to see, so it returns `false` and the
 * push goes out. A non-`open` thread also returns `false`: nothing was
 * consumed.
 *
 * `overlay` is injectable for tests; the default is the module-scope active
 * overlay.
 */
export function handleRespondentFinalReport(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  rpcRegistry: RpcAgentRegistry,
  thread: ThreadRecord,
  message: string,
  overlay: OverlayHandle | undefined = attachedOverlayFor(thread.threadId),
): boolean {
  if (thread.origin === "fork-raised") {
    // Review relay #1 (I2): `"pending"` counts here, unlike for `lead-ask`. A
    // fork-raised thread is bound from REGISTRATION, and in headless (§8) no
    // owner surface will ever open it — so the fork's own final is the only
    // event that can release the bind, and refusing it while the thread is
    // merely pending is exactly the permanent latch this branch must not
    // create. The fork answered itself or finished the task; either way the
    // owner has nothing left to answer.
    if (thread.status !== "open" && thread.status !== "pending") return false;
    // Close the view if one is open, then run the thread close itself
    // (previously only reachable via `/done`) so `threadBound` is released and
    // the fork rejoins the lead's fan-in on the very report that ends the
    // thread.
    overlay?.closeWithSummary("");
    detachForkRaisedThread(handle, rpcRegistry, thread);
    return false;
  }
  if (thread.status !== "open") return false;
  if (overlay) {
    overlay.closeWithSummary(message);
    return true;
  }
  closeThreadOnDone(pi, handle, rpcRegistry, thread, message);
  return true;
}

/**
 * Arms `handleRespondentFinalReport` on the thread's respondent record. The
 * thread is re-read from the registry by id at fire time, since a
 * `session_start` re-hydration replaces the record objects.
 */
function armFinalReportHook(pi: ExtensionAPI, handle: ThreadRegistryHandle, rpcRegistry: RpcAgentRegistry, threadId: string, agentId: string): void {
  const record = rpcRegistry.get(agentId);
  if (!record) return;
  record.onFinalReport = (_record, message) => {
    const thread = handle.threads.get(threadId);
    if (!thread) return false;
    // The boolean propagates verbatim: it is `spawner.ts`'s
    // report-push suppression signal, not a local status.
    return handleRespondentFinalReport(pi, handle, rpcRegistry, thread, message);
  };
}

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
      rpcRegistry.set(agentId, rehydrateForkRecord(agentId, thread.forkResume));
    }
    // Idempotent: a live or rehydrated respondent (either origin) reports its
    // own final into this thread — see `handleRespondentFinalReport`.
    armFinalReportHook(pi, handle, rpcRegistry, thread.threadId, agentId);
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
  const sessionManager = (ctx as { sessionManager?: { buildContextEntries?: () => { id: string }[]; getBranch?: (id: string) => { id: string }[] } })
    .sessionManager;
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
      // Entry B's discussion fork belongs to the owner surface, never to the
      // lead's fan-in — bound before its first turn can produce a settle.
      spawnRole: "fork",
    },
    {
      // Entry B remains a conversational message; task-only completion fields stay absent.
      prompt: buildDiscussionForkInitialMessage(thread.context, thread.question ?? thread.title, excerpt),
    },
  );

  thread.respondentAgentId = result.agent_id;
  const record = rpcRegistry.get(result.agent_id);
  if (record) thread.forkResume = captureForkResume(record);
  armFinalReportHook(pi, handle, rpcRegistry, thread.threadId, result.agent_id);
  bindThread(rpcRegistry, result.agent_id, true);
  return result.agent_id;
}

/**
 * Sets/clears `RpcAgentRecord.threadBound` — the thread-LIFETIME flag (§1's
 * "the lead is not part of this exchange"), as opposed to `overlayAttached`'s
 * per-VIEW lifetime. Set on every thread open/reopen and on fork-raised
 * registration; cleared only where the thread itself actually closes
 * (`detachForkRaisedThread`, `injectDiscussionSummary`, `ws-resolve`), never
 * on a mere overlay Esc. While set, `spawner.ts` emits no settle push for the
 * record and `computeRunningStatusLine` leaves it out entirely — it is neither
 * counted as running nor keeps the status line present.
 */
function bindThread(rpcRegistry: RpcAgentRegistry, agentId: string, bound: boolean): void {
  const record = rpcRegistry.get(agentId);
  if (record) { record.threadBound = bound; syncOwnershipProtection(record); }
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
 * `handleRespondentFinalReport`'s `overlay.closeWithSummary` path, a pending
 * `summarizeThenClose` settle, an owner Esc, and a second `/answer` closing
 * the first overlay all drive without reaching into the component itself.
 * `closeWithSummary` mirrors the old, now-deleted per-thread overlay
 * module's own: a non-empty summary is appended as the child's own turn
 * before the thread itself closes — UNLESS `alreadyRendered` says the
 * component's own settle handling already put it there (see
 * `summarizeThenClose`'s doc comment).
 *
 * Review relay #2 Important (I1a/I1b/I4): a private `finished` flag makes
 * `close`/`closeWithSummary` a no-op after either has already run once —
 * whichever of the three real races wins (an owner Esc during the summary
 * wait, the fork's own `kind:"final"` report arriving mid-wait via
 * `handleRespondentFinalReport`, or the summary settle itself) is the ONLY
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
      closeThreadOnDone(pi, handle, rpcRegistry, thread, summary);
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
  sessionManager?: { buildContextEntries?: () => { id: string }[]; getBranch?: (id: string) => { id: string }[] },
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
  /** The live host's `matchesKey` (resolved through `loadHostPiTui()` by the caller — never the static import; see `pi-tui.ts`'s dual-package-instance note) for confirm-screen arrow/enter detection across raw and Kitty-protocol encodings. */
  matchesKey: (data: string, keyId: string) => boolean;
  /** Word-wraps one line of plain text (the live host's `wrapTextWithAnsi`, or an injected fake in tests). */
  wrapText: (text: string, width: number) => string[];
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
  private index: number;
  private confirm: LeadAskQueueConfirmState | undefined;
  private finished = false;

  constructor(tui: ConversationViewTui, options: LeadAskQueueOptions) {
    this.tui = tui;
    this.options = options;
    this.threads = options.threads;
    this.index = Math.min(Math.max(0, options.initialFocusIndex), this.threads.length - 1);
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

  private liveDrafts(): Map<string, string> {
    const drafts = new Map<string, string>();
    for (let i = 0; i < this.threads.length; i += 1) drafts.set(this.threads[i].threadId, this.editors[i].getText());
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
    if (i !== this.index) return; // defensive — input only ever reaches the focused editor.
    this.editors[i].setText(text);
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
    if (matches(data, "left") || matches(data, "up")) {
      confirm.choice = "no";
      this.tui.requestRender();
      return;
    }
    if (matches(data, "right") || matches(data, "down")) {
      confirm.choice = "yes";
      this.tui.requestRender();
      return;
    }
    if (matches(data, "enter")) this.resolveConfirm(confirm.kind, confirm.choice);
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
    const lines: string[] = [this.line(buildQueueCoverageLine(this.index, total, answered), w), ""];
    if (this.confirm) {
      lines.push(...this.renderConfirm(w, answered, total));
      return lines;
    }
    const thread = this.threads[this.index];
    if (thread.withdrawnPending) {
      lines.push(this.line("⚠ the agent withdrew this question — your answer, if any, is still delivered when you close.", w), "");
    }
    lines.push(...this.options.wrapText(thread.question ?? thread.title, w));
    if (thread.context && thread.context.trim().length > 0) {
      lines.push("", ...this.options.wrapText(thread.context, w));
    }
    lines.push("", ...this.editors[this.index].render(w), "");
    lines.push(this.line("Enter: answer & next · Shift+Enter/Ctrl+J: newline · Tab/Shift+Tab: switch question · Esc: exit", w));
    return lines;
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
      this.line("←/→ select · Enter confirm · Esc cancel", w),
    ];
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

  // One overlay at a time (§5, carried over from Phase 1): a live
  // fork-raised chat, if any, is closed first.
  activeOverlay?.handle.close();
  activeOverlay = undefined;
  const token = ++overlayToken;

  const sessionManager = (ctx as { sessionManager?: { buildContextEntries?: () => { id: string }[]; getBranch?: (id: string) => { id: string }[] } })
    .sessionManager;

  try {
    await (ctx as unknown as { ui: AskCustomUiCtx["ui"] }).ui.custom<undefined>(
      async (tui, _theme, _keybindings, done) => {
        const hostPiTui = await loadHostPiTui();
        const component = new LeadAskQueueComponent(tui, {
          threads,
          initialFocusIndex,
          border: true,
          matchesKey: hostPiTui.matchesKey as (data: string, keyId: string) => boolean,
          wrapText: (text, width) => hostPiTui.wrapTextWithAnsi(text, width),
          editorFactory: () => new hostPiTui.Editor(tui as never, QUEUE_IDENTITY_EDITOR_THEME) as unknown as FocusableEditorLike,
          onClose: (mode, drafts) => {
            for (const thread of threads) {
              const draft = drafts.get(thread.threadId) ?? "";
              const action = resolveLeadAskQueueEntryAction(mode, thread.withdrawnPending === true, draft);
              runLeadAskEscapeAction(action, pi, handle, thread, draft, sessionManager);
            }
            done(undefined);
          },
        });
        activeOverlay = {
          token,
          threadId: threads[initialFocusIndex].threadId,
          handle: { close: () => done(undefined), closeWithSummary: () => done(undefined) },
        };
        activeQueueRepaint = () => tui.requestRender();
        return component;
      },
      { overlay: true, overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" } },
    );
  } finally {
    if (activeOverlay?.token === token) activeOverlay = undefined;
    activeQueueRepaint = undefined;
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

  // One overlay at a time (§5): the previous one is closed first; its own
  // fork is untouched and its thread stays reopenable.
  activeOverlay?.handle.close();
  activeOverlay = undefined;
  const token = ++overlayToken;

  // Review relay #1 C1: mark the respondent as owner-attached for as long as
  // this overlay lives. An Entry-A task fork still runs `wireAntiBleedLoop`
  // on the same record, and every owner exchange is a text-only turn — §4's
  // bleed signal — so without this the loop would nudge the fork mid-
  // conversation and then steer a false "stalled, do not harvest" verdict
  // into the lead. Read (not imported) by `fork.ts`: the reverse import would
  // cycle.
  const attachedRecord = rpcRegistry.get(agentId);
  if (attachedRecord) attachedRecord.overlayAttached = true;

  const channel = createForkChannel(pi, rpcRegistry, sessionCtx.cwd, sessionCtx.extensionPath, agentId);
  // The transcript lives on the record, not in the view: restored here (or
  // seeded from the question when there is no transcript yet), and persisted
  // on every append so Esc/reopen and a lead restart both show the
  // conversation so far.
  const initialItems = buildInitialConversationItems(thread);
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
        // wins, so an owner Esc (or a racing final report) during the wait
        // stops this listener rather than leaving it to fire later into an
        // already-guarded (but still leaked) `closeWithSummary`.
        let pendingSummarizeUnsubscribe: (() => void) | undefined;
        const component: ConversationViewComponent = new ConversationViewComponent(tui, {
          channel,
          initialItems,
          headerHint,
          markdownTheme,
          // 260909 V1/V2: the overlay draws its own border + horizontal margin
          // so it separates from the lead's background behind it.
          border: true,
          viewportHeight: () => conversationOverlayHeight(tui),
          keybindings: keybindings as { matches(data: string, id: string): boolean },
          userLineBg: (text) => theme?.bg?.("userMessageBg", text) ?? text,
          toolTextFg: (text) => theme?.fg?.("muted", text) ?? text,
          workingTextFg: (text) => theme?.fg?.("dim", text) ?? text,
          primitives: { ScrollView: hostPiTui.ScrollView, Markdown: hostPiTui.Markdown, Text: hostPiTui.Text, Editor: hostPiTui.Editor },
          // Routed through `overlayHandle.close()` (rather than the raw
          // `done` callback) so an Esc during a pending summary wait
          // participates in the SAME `finished` guard `closeWithSummary`
          // uses — otherwise Esc would close the view here while a later
          // settle still injected a summary into the lead behind it.
          onEscape: () => overlayHandle?.close(),
          onDone: () => {
            pendingSummarizeUnsubscribe = runDoneAction(resolveDoneAction(summarizeOnDone), component, channel, overlayHandle!);
          },
          onItemsChange: (items) => {
            thread.transcript = items.length > THREAD_TRANSCRIPT_CAP ? items.slice(-THREAD_TRANSCRIPT_CAP) : [...items];
            persistThreads(handle);
          },
        });
        component.setMode("interactive");
        overlayHandle = buildOverlayHandle(pi, handle, rpcRegistry, thread, component, done, () => {
          pendingSummarizeUnsubscribe?.();
          pendingSummarizeUnsubscribe = undefined;
        });
        activeOverlay = { token, threadId: thread.threadId, handle: overlayHandle };
        return component;
      },
      { overlay: true, overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" } },
    );
  } finally {
    // Cleared on every exit path — `/done` (which also stops the fork), a
    // plain close, or a throw out of the overlay.
    const record = rpcRegistry.get(agentId);
    if (record) record.overlayAttached = false;
    if (activeOverlay?.token === token) activeOverlay = undefined;
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
): void {
  pi.registerCommand("thread", {
    description: "List ws discussion threads (pending, open, and dormant-but-reopenable).",
    handler: async (_args, ctx) => {
      notify(ctx as AskUiCtx, buildThreadListLines([...handle.threads.values()]).join("\n"), "info");
    },
  });

  pi.registerCommand("answer", {
    description: "Open a ws question thread in a chat overlay (usage: /answer <id>; no id opens the most recent).",
    handler: async (args, ctx) => {
      const id = args.trim();
      const thread = id ? handle.threads.get(id) : mostRecentReopenable([...handle.threads.values()]);
      if (!thread) {
        notify(ctx as AskUiCtx, id ? `ws: no thread "${id}" — /thread lists them.` : "ws: no thread to open.", "warning");
        return;
      }
      await openThread(pi, ctx as never, bridge, rpcRegistry, handle, thread, sessionCtx);
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
