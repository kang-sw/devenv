import { readFileSync, rmSync, writeFileSync } from "node:fs";
import { randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import type { BridgeHandle } from "./bridge.ts";
import { agentWidgetRefreshRef, refreshAgentTelemetry, startOwnedSessionObserver, type RpcAgentRecord, type ToolGroup } from "./spawner.ts";
import type { SpawnRole } from "./process-role.ts";
import { DONE_COMMAND } from "./conversation-view.ts";
import type { ForkContext } from "./fork-context.ts";
import { renameWithWindowsRetry, type RenameRetryHooks } from "./atomic-write.ts";
import type { LeadPromptRef } from "./lead-bootstrap.ts";
import { restoreDescendantUsage } from "./agent-usage-rollup.ts";
import { readOwnership, validDescriptor } from "./agent-storage.ts";
import { parseTelemetry, type AgentTelemetry, type TelemetryOrigin } from "./agent-telemetry.ts";
import { parseDelegationPolicy } from "./delegation-policy.ts";

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
 *
 * Review relay #2 C1/I1: `closeWithSummary`'s `alreadyRendered` parameter
 * (default `false`, so every EXISTING caller keeps its old append-then-close
 * behavior unchanged) exists for exactly one caller —
 * `summarizeThenClose`'s own settled-turn completion — whose summary text
 * was ALREADY appended to the view by the component's own internal
 * `agent_settled` handling (the same event, a separate listener registered
 * first). Passing `true` there skips the redundant second append that used
 * to double the summary turn on screen; the
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
    "Do NOT relay this question, answer it yourself, or ask the owner about it. End your turn — this agent resumes its task once the owner replies, and its later ordinary settled answer carries the outcome.",
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
 *   injection. The fork resumes its task and its later ordinary settled
 *   answer reaches the lead through the shared terminal lifecycle.
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
export interface PersistedForkResume extends Pick<RpcAgentRecord, "delegation" | "waitingOnChildren" | "lastWriter" | "ownerSends"> {
  sessionPath: string;
  systemPromptPath?: string;
  forkContext?: ForkContext;
  explicitTools?: string;
  wsToolNames: string[];
  toolGroup: ToolGroup;
  modelBase?: string;
  modelEffort?: string;
  telemetry?: AgentTelemetry;
  telemetryContextFloor?: TelemetryOrigin;
  observedModel?: string;
  observedEffort?: string;
  observedContextTokens?: number;
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
  const threads = records.map((record) => {
    const { transcript: _retiredTranscript, ...persisted } = record as ThreadRecord & { transcript?: unknown };
    return persisted;
  });
  return `${JSON.stringify({ threads }, null, 2)}\n`;
}

/**
 * Tolerant parse: anything that is not a well-formed `{threads:[...]}`
 * document degrades to `[]` rather than throwing — the never-throw contract
 * for adapter-owned data files. Individual
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
    // "fork-raised" is the safe direction (see `ThreadOrigin`). Legacy
    // thread-local transcripts are discarded: the child record/session is
    // now the single conversation source.
    .map((entry) => {
      const { transcript: _retiredTranscript, ...rest } = entry as ThreadRecord & { transcript?: unknown };
      return { ...rest, origin: normalizeThreadOrigin(entry.origin) };
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
// Dormant: no production caller since the owner-question wiring was
// disconnected; `lead-bootstrap.ts`'s `withoutOwnerQuestionTools` now filters
// both tools out of the top lead's active list instead.
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
 * The owner ends the thread with `/done`; ordinary settled answers before
 * that remain part of the owner conversation. No completion report tool and
 * no task frame are involved.
 */
export function buildDiscussionForkDirectiveText(): string {
  return [
    "Side-discussion thread: this session is a clone of the lead's own session, opened so its owner can talk one question through directly.",
    "",
    "Reply conversationally and briefly, in the same voice as the rest of this conversation. Answer what is asked, say plainly when something is genuinely undecided, and ask back only when the answer actually depends on it.",
    "",
    "There is no task to complete and no progress report to file here. Do not start editing files or running work unless the owner explicitly asks for it in this thread.",
    "",
    "The owner ends the thread with /done. Until then, each ordinary settled answer belongs to the owner conversation and is not a completion report to the lead.",
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
    ...(record.delegation ? { delegation: record.delegation, waitingOnChildren: record.waitingOnChildren } : {}),
    ...(record.lastWriter ? { lastWriter: record.lastWriter } : {}),
    ...(record.ownerSends?.length ? { ownerSends: record.ownerSends.map((send) => ({ ...send })) } : {}),
    wsToolNames: [...record.wsToolNames],
    toolGroup: record.toolGroup,
    modelBase: record.modelBase,
    modelEffort: record.modelEffort,
    ...(record.telemetry ? { telemetry: record.telemetry } : {}),
    ...(record.telemetryContextFloor ? { telemetryContextFloor: record.telemetryContextFloor } : {}),
    ...(record.observedModel ? { observedModel: record.observedModel } : {}),
    ...(record.observedEffort ? { observedEffort: record.observedEffort } : {}),
    ...(record.observedContextTokens !== undefined ? { observedContextTokens: record.observedContextTokens } : {}),
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
    ...(parseTelemetry({ version: 1, origin: resume.telemetryContextFloor })?.origin ? { telemetryContextFloor: parseTelemetry({ version: 1, origin: resume.telemetryContextFloor })!.origin } : {}),
    ...(typeof resume.observedModel === "string" && resume.observedModel ? { observedModel: resume.observedModel } : {}),
    ...(typeof resume.observedEffort === "string" && resume.observedEffort ? { observedEffort: resume.observedEffort } : {}),
    ...(typeof resume.observedContextTokens === "number" && Number.isFinite(resume.observedContextTokens) && resume.observedContextTokens >= 0 ? { observedContextTokens: resume.observedContextTokens } : {}),
    wsToolNames: [...resume.wsToolNames],
    toolGroup: resume.toolGroup,
    explicitTools: resume.explicitTools,
    // An older record may still carry the retired `subtreeChannel`; it is
    // ignored, and the relaunch binds a fresh control channel.
    ...(resume.delegation ? { delegation: parseDelegationPolicy(resume.delegation), waitingOnChildren: resume.waitingOnChildren } : {}),
    ...(resume.lastWriter === "lead" || resume.lastWriter === "owner" ? { lastWriter: resume.lastWriter } : {}),
    ...(Array.isArray(resume.ownerSends) ? { ownerSends: resume.ownerSends.flatMap((send) => send && typeof send.text === "string" && typeof send.at === "number" ? [{ text: send.text, at: send.at }] : []) } : {}),
    spawnRole: "fork",
    streaming: false,
    running: false,
    reportLog: [],
  };
  // Thread-only rows are rendered from forkResume while dormant; reconcile
  // the persisted child file here, never from the widget render path.
  refreshAgentTelemetry(record);
  restoreDescendantUsage(record);
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

/** Never-throw read: a missing/corrupt file degrades to an empty registry. */
export function loadThreadRegistryFile(path: string): ThreadRecord[] {
  let raw: string;
  try {
    raw = readFileSync(path, "utf8");
  } catch {
    return [];
  }
  return parseThreadRegistry(raw);
}

/**
 * Never-throw write: a failed save degrades to a no-op (the in-memory registry
 * stays authoritative for this process). The temp-then-rename replacement means
 * a crash mid-write leaves the previous registry intact, never a truncated one.
 * `hooks` is a focused deterministic test seam; production callers pass two arguments.
 */
export function saveThreadRegistryFile(path: string, records: readonly ThreadRecord[], hooks?: RenameRetryHooks): void {
  const temporary = `${path}.${randomUUID()}.tmp`;
  try {
    writeFileSync(temporary, serializeThreadRegistry(records));
    renameWithWindowsRetry(temporary, path, hooks);
  } catch {
    // best effort — a read-only/unwritable session dir must never turn a
    // question registration into a crashed tool call.
    try { rmSync(temporary, { force: true }); } catch { /* the save already failed */ }
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

export function persistThreads(handle: ThreadRegistryHandle): void {
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
export function refreshAgentWidget(): void {
  try {
    agentWidgetRefreshRef.current?.();
  } catch {
    // best effort — see doc comment above.
  }
}

export function notify(ctx: AskUiCtx | undefined, message: string, type?: "info" | "warning" | "error"): void {
  ctx?.ui?.notify?.(message, type);
}

export function nowIso(): string {
  return new Date().toISOString();
}

export interface AskSessionCtx {
  cwd: string;
  /** Exact manifest entry module loaded by this parent, used by dormant fork resumes. */
  extensionPath: string;
  effectivePromptRef?: LeadPromptRef;
  /** Read at each owner send or /done, since a re-login can replace the key. */
  leadSessionKeyRef?: BridgeHandle["defaultSessionKeyRef"];
}
