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
 * `fork.ts`, `process-role.ts` and `overlay-chat.ts` only, never the
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
 */

import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { BridgeHandle } from "./bridge.ts";
import {
  agentWidgetRefreshRef,
  heldPushQueue,
  inheritModelFromToolCtx,
  isOwningAgentIdle,
  sendToAgent,
  spawnAgent,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
  type ToolGroup,
} from "./spawner.ts";
import { computeForkToolSurface, getForkSourceSessionFile } from "./fork.ts";
import type { SpawnRole } from "./process-role.ts";
import { openOverlayChat, type ForkChannel, type OverlayHandle, type TranscriptEntry } from "./overlay-chat.ts";

// ---------------------------------------------------------------------------
// Pure helpers. Unit-tested directly (test/ask.test.ts) with no
// filesystem/subprocess/live `pi` session involved.
// ---------------------------------------------------------------------------

/** Lead-facing verb-table tool name (pi-lead-guide.md): register an owner question. */
export const ASK_TOOL_NAME = "ws-ask";

/** Lead-facing verb-table tool name: self-resolve a still-pending owner question. */
export const RESOLVE_TOOL_NAME = "ws-resolve";

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
   * The overlay transcript (owner lines, settled thread turns, adapter notes),
   * newest last and capped at `THREAD_TRANSCRIPT_CAP` entries. Persisted with
   * the record so a reopen after Esc — or after a lead restart — shows the
   * conversation so far instead of an empty view (dogfood 2026-09-05). Absent
   * until the thread is first opened.
   */
  transcript?: TranscriptEntry[];
  createdAt: string;
  /** Last open/answer/close touch — orders the "reopen the most recent" shortcut. */
  touchedAt: string;
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

/**
 * Tolerant read of a persisted `transcript`: a non-array is `undefined`
 * (the field is simply absent), malformed entries are dropped, and the
 * result is capped to the newest `THREAD_TRANSCRIPT_CAP` — a hand-edited or
 * older registry file must never make a thread unopenable.
 */
export function normalizeTranscript(value: unknown): TranscriptEntry[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const entries = value
    .filter((entry): entry is TranscriptEntry => {
      const candidate = entry as Partial<TranscriptEntry> | null;
      return (candidate?.who === "you" || candidate?.who === "thread" || candidate?.who === "note") && typeof candidate.text === "string";
    })
    .map((entry) => ({ who: entry.who, text: entry.text }));
  return entries.length > THREAD_TRANSCRIPT_CAP ? entries.slice(entries.length - THREAD_TRANSCRIPT_CAP) : entries;
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
  systemPromptPath: string;
  explicitTools?: string;
  wsToolNames: string[];
  toolGroup: ToolGroup;
  modelBase?: string;
  modelEffort?: string;
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

/** §5 widget wording counts PENDING threads only — an already-open thread is not something the owner still owes an answer to. Also feeds `agent-widget.ts`'s `buildStatusSegment` question count. */
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
 * Role-differentiated `ws-ask`/`ws-resolve` active-tools addition, identical
 * in shape to `fork.ts`'s `addForkToolIfLead` and kept as its own function
 * for the same reason: `execute-gateway.ts`'s shared
 * `computeLeadActiveTools`/`LEAD_ADDED_TOOL_NAMES` are applied to lead AND
 * fork roles alike, so folding these two in there would hand a fork the very
 * tools `FORK_EXCLUDED_TOOL_NAMES` exists to keep away from it. Only the
 * true top lead (`role === undefined`) ever gains them.
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
  const lines: string[] = ["The owner opened a side discussion about this question."];
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

/** Denormalizes the resume-relevant half of a live record into JSON-safe plain data (see `PersistedForkResume`). */
export function captureForkResume(record: RpcAgentRecord): PersistedForkResume {
  return {
    sessionPath: record.sessionPath,
    systemPromptPath: record.systemPromptPath,
    explicitTools: record.explicitTools,
    wsToolNames: [...record.wsToolNames],
    toolGroup: record.toolGroup,
    modelBase: record.modelBase,
    modelEffort: record.modelEffort,
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
  return {
    agentId,
    client: undefined,
    sessionPath: resume.sessionPath,
    systemPromptPath: resume.systemPromptPath,
    modelBase: resume.modelBase,
    modelEffort: resume.modelEffort,
    wsToolNames: [...resume.wsToolNames],
    toolGroup: resume.toolGroup,
    explicitTools: resume.explicitTools,
    spawnRole: "fork",
    streaming: false,
    running: false,
    reportLog: [],
  };
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

function notify(ctx: AskUiCtx | undefined, message: string, type?: "info" | "warning" | "error"): void {
  ctx?.ui?.notify?.(message, type);
}

function nowIso(): string {
  return new Date().toISOString();
}

export interface AskSessionCtx {
  cwd: string;
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
export function registerAsk(pi: ExtensionAPI, handle: ThreadRegistryHandle, rpcRegistry?: RpcAgentRegistry): void {
  pi.registerTool({
    name: ASK_TOOL_NAME,
    label: ASK_TOOL_NAME,
    description:
      "Register a question for the owner without blocking or interrupting them. Returns {question_id} immediately and spawns nothing — the owner opens it themselves with /answer <id>, which is when a discussion thread is created. Use it for a decision only the owner can make; keep working on anything that does not depend on the answer. Call ws-resolve if you work the answer out yourself before they open it.",
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
      const p = params as { title: string; question: string; context?: string };
      const now = nowIso();
      const record: ThreadRecord = {
        threadId: nextThreadId([...handle.threads.keys()]),
        title: p.title,
        question: p.question,
        context: p.context,
        entryId: getLeafEntryId(toolCtx),
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
  });

  pi.registerTool({
    name: RESOLVE_TOOL_NAME,
    label: RESOLVE_TOOL_NAME,
    description:
      "Withdraw a question you registered with ws-ask because you no longer need the owner's answer. Removes it from their pending count. Does not notify them and injects nothing — you already know the answer.",
    parameters: {
      type: "object",
      properties: {
        question_id: { type: "string", description: "The question_id ws-ask returned." },
      },
      required: ["question_id"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, _toolCtx) {
      const p = params as { question_id: string };
      const record = handle.threads.get(p.question_id);
      if (!record) {
        throw new Error(`ws-pi-agent: ${RESOLVE_TOOL_NAME}: unknown question_id "${p.question_id}"`);
      }
      record.status = "closed";
      record.touchedAt = nowIso();
      // The thread is closed for good, so release the thread-lifetime bind on
      // its respondent (a fork-raised thread's fork keeps running and rejoins
      // the lead's fan-in).
      if (record.respondentAgentId && rpcRegistry) bindThread(rpcRegistry, record.respondentAgentId, false);
      persistThreads(handle);
      refreshAgentWidget();
      return { content: [{ type: "text", text: JSON.stringify({ question_id: record.threadId, status: record.status }) }] };
    },
  });
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

/**
 * §6 injection, the `"lead-ask"` half of `/done`: the fork's summary
 * reaches the lead as a Pi CUSTOM message (distinguishable from an owner
 * turn) delivered via `followUp` — never `steer` — so a streaming lead only
 * sees it after its current turn and multiple closes queue in order, and
 * with `triggerTurn: true` so an IDLE lead starts a turn on it at once rather
 * than leaving the owner's decision queued until their next prompt. The lead
 * session is never rewound. The thread itself goes `"dormant"`: retained and
 * reopenable (§9), not deleted.
 *
 * Review relay #1 I5: "dormant" is 260903's `ws-agent-stop` semantics —
 * dormant AND retained — so the respondent's child process is actually
 * stopped here rather than left running idle for the rest of the lead
 * session. `stopAgent` keeps the registry entry (it only drops `client`), and
 * the resume snapshot is captured BEFORE the stop, while the record still
 * carries its live fields, so `/answer` can rehydrate and `sendToAgent`'s
 * dormant branch can relaunch from the same `--session` file. Review relay #2
 * C2 narrowed that stop to `"lead-ask"` threads — it is `closeThreadOnDone`'s
 * job to keep an Entry A task fork out of here.
 *
 * 260906 (compaction push-hold ticket, Phase 1): the outbound
 * `ws-thread-summary` message is held on `spawner.ts`'s `heldPushQueue`
 * (as a `kind: "raw"` entry) whenever `isOwningAgentIdle()` is false — mid-turn
 * OR an in-flight compaction, the same predicate `pushToLead`'s `followUp`
 * hold uses — and released with the rest of that queue on the owning turn's
 * `agent_settled` or on `releaseAfterCompaction`. This call site never had a
 * mid-turn guard before (it never carried a computed status line, so
 * staleness never applied), but an in-flight compaction can abort the very
 * turn a synchronous send here would otherwise race into. The thread's own
 * dormant transition / respondent stop / persistence below runs immediately
 * either way — those side effects are not part of the race being fixed and
 * delaying them would add no safety.
 */
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
  // `triggerTurn` is what makes an IDLE lead act on the decision: without
  // it Pi only queues the message and nothing starts a turn until the
  // owner's next prompt (dogfood 2026-09-05, Pi 0.84.4). `followUp` keeps
  // the ordering contract for a lead mid-turn — delivered after that turn.
  const options = { deliverAs: "followUp" as const, triggerTurn: true };
  if (isOwningAgentIdle()) {
    pi.sendMessage(message, options);
  } else {
    heldPushQueue.push({ kind: "raw", send: (p) => p.sendMessage(message, options) });
  }

  const agentId = thread.respondentAgentId;
  if (agentId) {
    const record = rpcRegistry.get(agentId);
    if (record) {
      record.overlayAttached = false;
      record.threadBound = false;
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
function createForkChannel(pi: ExtensionAPI, rpcRegistry: RpcAgentRegistry, cwd: string, agentId: string): ForkChannel {
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
    isStreaming() {
      return rpcRegistry.get(agentId)?.streaming === true;
    },
    async send(text) {
      await sendToAgent(rpcRegistry, { pi, cwd }, agentId, text, resolveOwnerSendInterrupt(rpcRegistry.get(agentId)?.streaming === true));
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

  const directiveDir = mkdtempSync(join(tmpdir(), "ws-pi-discuss-"));
  const directivePath = join(directiveDir, "discussion-directive.md");
  writeFileSync(directivePath, buildDiscussionForkDirectiveText());

  const result = await spawnAgent(
    rpcRegistry,
    {
      pi,
      cwd: sessionCtx.cwd,
      inheritModel: inheritModelFromToolCtx(ctx),
      wsToolNames: bridge.wsToolNames,
      client: bridge.client,
      forkFrom,
      explicitTools: computeForkToolSurface(pi.getActiveTools()).join(","),
      parentSessionKey: bridge.defaultSessionKeyRef.current,
      // Entry B's discussion fork belongs to the owner surface, never to the
      // lead's fan-in — bound before its first turn can produce a settle.
      spawnRole: "fork",
    },
    {
      systemPromptPath: directivePath,
      // Entry B: plain text, no structural frame, and no wireAntiBleedLoop
      // call afterwards — see this file's header.
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
  if (record) record.threadBound = bound;
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

  try {
    await openOverlayChat(ctx as never, {
      title: thread.title,
      threadId: thread.threadId,
      question: thread.question,
      createdAt: thread.createdAt,
      // Review relay #2 C2: only a discussion fork this surface owns is asked
      // for a summary. A live task fork is mid-task — asking it to summarize
      // (and then acting on that turn) would derail the work the lead is
      // waiting on.
      summarizeOnDone: thread.origin === "lead-ask",
      channel: createForkChannel(pi, rpcRegistry, sessionCtx.cwd, agentId),
      onDone: (summary) => closeThreadOnDone(pi, handle, rpcRegistry, thread, summary),
      // The transcript lives on the record, not in the view: restored here,
      // and persisted on every append so Esc/reopen and a lead restart both
      // show the conversation so far.
      initialEntries: thread.transcript,
      onTranscriptChange: (entries) => {
        thread.transcript = entries.length > THREAD_TRANSCRIPT_CAP ? entries.slice(entries.length - THREAD_TRANSCRIPT_CAP) : entries;
        persistThreads(handle);
      },
      onOpened: (overlay) => {
        activeOverlay = { token, threadId: thread.threadId, handle: overlay };
      },
    });
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
 * slash-command dispatch runs on (see overlay-chat.ts).
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
