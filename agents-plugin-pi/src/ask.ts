/**
 * 260904 Phase 2 (`260904-feat-ws-pi-side-thread-fork-question-surface`):
 * the OWNER-question surface — `ws-ask`/`ws-resolve`, the persisted thread
 * registry, the `N pending` widget, `/thread`, `/answer <id>`, the lazy
 * discussion fork, and the `/done` summary injection.
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
  inheritModelFromToolCtx,
  sendToAgent,
  spawnAgent,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
  type ToolGroup,
} from "./spawner.ts";
import { computeForkToolSurface, getForkSourceSessionFile } from "./fork.ts";
import type { SpawnRole } from "./process-role.ts";
import { openOverlayChat, type ForkChannel } from "./overlay-chat.ts";

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

/** `ctx.ui.setWidget` key for the `N pending` counter above the editor. */
export const PENDING_WIDGET_KEY = "ws-threads";

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
    "Do NOT relay this question, answer it yourself, or ask the owner about it. Keep waiting on this agent (ws-agent-wait) — it resumes its task once the owner replies, and what was decided reaches you in its own final report's Decisions: line.",
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
 *   to `ws-fork`/`ws-agent-wait`/`ws-agent-stop`, not to this surface. `/done`
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

/** Stable, pretty-printed on-disk form (a hand-inspectable adapter data file, like model-catalog.json). */
export function serializeThreadRegistry(records: readonly ThreadRecord[]): string {
  return `${JSON.stringify({ threads: records }, null, 2)}\n`;
}

/**
 * Tolerant parse: anything that is not a well-formed `{threads:[...]}`
 * document degrades to `[]` rather than throwing — same never-throw contract
 * `readGoalLoopConfig`/`readModelCatalog` already use for adapter-owned data
 * files. Individual entries missing a `threadId`/`status` are dropped rather
 * than poisoning the whole registry.
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
    // "fork-raised" is the safe direction (see `ThreadOrigin`).
    .map((entry) => ({ ...entry, origin: normalizeThreadOrigin(entry.origin) }));
}

/** §5 widget wording counts PENDING threads only — an already-open thread is not something the owner still owes an answer to. */
export function countPending(records: readonly ThreadRecord[]): number {
  return records.filter((record) => record.status === "pending").length;
}

/** Widget content for `ctx.ui.setWidget`; `undefined` clears the widget entirely at zero pending. */
export function buildWidgetLines(count: number): string[] | undefined {
  if (count <= 0) return undefined;
  return [`ws: ${count} pending question${count === 1 ? "" : "s"} — /answer <id>, /thread to list`];
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
 * conversation constraints only — and deliberately NO `kind:"final"` report
 * instruction: a discussion thread's exit is the owner's `/done` in the
 * overlay, not a report. Framing-free on purpose (§4's directive-style rule):
 * a discussion fork is meant to speak as the lead, so nothing here tries to
 * give it a separate identity.
 */
export function buildDiscussionForkDirectiveText(): string {
  return [
    "Side-discussion thread: this session is a clone of the lead's own session, opened so its owner can talk one question through directly.",
    "",
    "Reply conversationally and briefly, in the same voice as the rest of this conversation. Answer what is asked, say plainly when something is genuinely undecided, and ask back only when the answer actually depends on it.",
    "",
    "There is no task to complete and no report to file here. Do not start editing files or running work unless the owner explicitly asks for it in this thread.",
    "",
    "The owner ends the thread themselves; when they do, you will be asked once for a short summary of what was decided.",
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
    streaming: false,
    idlePending: false,
    waiters: [],
    pendingReports: [],
    reportsDropped: 0,
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
    setWidget?(key: string, content: string[] | undefined, options?: { placement?: string }): void;
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
 * Repaints the `N pending` widget above the editor. A guarded no-op outside
 * TUI mode (§8: headless never grows a widget) and whenever no `ctx` has been
 * captured yet — the race window between a lead restart and its first
 * `session_start`, which the ticket's own captured-`ctx` note calls out.
 */
export function refreshPendingWidget(ctx: AskUiCtx | undefined, handle: ThreadRegistryHandle): void {
  if (!ctx || ctx.mode !== "tui") return;
  ctx.ui?.setWidget?.(PENDING_WIDGET_KEY, buildWidgetLines(countPending([...handle.threads.values()])), { placement: "aboveEditor" });
}

function notify(ctx: AskUiCtx | undefined, message: string, type?: "info" | "warning" | "error"): void {
  ctx?.ui?.notify?.(message, type);
}

function nowIso(): string {
  return new Date().toISOString();
}

export interface AskSessionCtx {
  cwd: string;
  modelCatalogPath: string;
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
  if (live) record.forkResume = captureForkResume(live);
  handle.threads.set(record.threadId, record);
  persistThreads(handle);
  refreshPendingWidget(handle.ctxRef.current, handle);
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
export function registerAsk(pi: ExtensionAPI, handle: ThreadRegistryHandle): void {
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
        refreshPendingWidget(ctx, handle);
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
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as { question_id: string };
      const record = handle.threads.get(p.question_id);
      if (!record) {
        throw new Error(`ws-pi-agent: ${RESOLVE_TOOL_NAME}: unknown question_id "${p.question_id}"`);
      }
      record.status = "closed";
      record.touchedAt = nowIso();
      persistThreads(handle);
      refreshPendingWidget((toolCtx as AskUiCtx | undefined) ?? handle.ctxRef.current, handle);
      return { content: [{ type: "text", text: JSON.stringify({ question_id: record.threadId, status: record.status }) }] };
    },
  });
}

/**
 * The overlay's `/done` exit, routed on the thread's `origin` — the two
 * entries own their respondent differently (review relay #2 C2, see
 * `ThreadOrigin`):
 *
 * - `"lead-ask"`: this surface spawned the discussion fork, so `/done` runs
 *   the full §6/§9 close — summary into the lead, then stop the fork.
 * - `"fork-raised"`: the respondent is a LIVE Entry A task fork the lead is
 *   parked on. Stopping it would destroy its in-flight task and hang the
 *   lead's `ws-agent-wait` (`stopAgent` settles no waiters), so `/done` only
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
      // Refresh the resume snapshot while the record is still live, so a
      // reopen after a lead restart can rehydrate it.
      thread.forkResume = captureForkResume(record);
    }
  }
  thread.status = "dormant";
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshPendingWidget(handle.ctxRef.current, handle);
}

/**
 * §6 injection, the `"lead-ask"` half of `/done`: the fork's summary
 * reaches the lead as a Pi CUSTOM message (distinguishable from an owner
 * turn) delivered via `followUp` — never `steer` — so it lands when the lead
 * is idle and multiple closes queue in order. The lead session is never
 * rewound. The thread itself goes `"dormant"`: retained and reopenable (§9),
 * not deleted.
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
 */
export function injectDiscussionSummary(
  pi: ExtensionAPI,
  handle: ThreadRegistryHandle,
  rpcRegistry: RpcAgentRegistry,
  thread: ThreadRecord,
  summary: string,
): void {
  pi.sendMessage(
    {
      customType: THREAD_SUMMARY_CUSTOM_TYPE,
      content: buildInjectionMessage(thread.context, thread.question, summary),
      display: true,
      details: { threadId: thread.threadId, title: thread.title },
    },
    { deliverAs: "followUp" },
  );

  const agentId = thread.respondentAgentId;
  if (agentId) {
    const record = rpcRegistry.get(agentId);
    if (record) {
      record.overlayAttached = false;
      // Snapshot first: `stopAgent` clears `client`, and a later reopen needs
      // the session/tool fields this copy carries.
      thread.forkResume = captureForkResume(record);
    }
    // Best effort — a failed stop must not lose the summary the owner just
    // produced, nor strand the thread in "open".
    void stopAgent(rpcRegistry, agentId).catch(() => undefined);
  }

  thread.status = "dormant";
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshPendingWidget(handle.ctxRef.current, handle);
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
function createForkChannel(rpcRegistry: RpcAgentRegistry, cwd: string, agentId: string): ForkChannel {
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
      await sendToAgent(rpcRegistry, { cwd }, agentId, text, resolveOwnerSendInterrupt(rpcRegistry.get(agentId)?.streaming === true));
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
let activeOverlay: { token: number; close: () => void } | undefined;
/** Identifies one overlay INSTANCE, not one thread: reopening the same thread must not let the closing instance clear its successor's entry. */
let overlayToken = 0;

/**
 * Ensures the thread has a live-or-resumable respondent fork on the shared
 * `rpcRegistry`, spawning a discussion fork lazily when it has none.
 * Returns the respondent's agent_id, or `undefined` when it could not be
 * established (already reported to the owner via `notify`).
 */
async function ensureRespondent(
  pi: ExtensionAPI,
  ctx: AskUiCtx & { sessionManager?: unknown },
  bridge: BridgeHandle,
  rpcRegistry: RpcAgentRegistry,
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
      cwd: sessionCtx.cwd,
      inheritModel: inheritModelFromToolCtx(ctx),
      wsToolNames: bridge.wsToolNames,
      modelCatalogPath: sessionCtx.modelCatalogPath,
      forkFrom,
      explicitTools: computeForkToolSurface(pi.getActiveTools()).join(","),
      parentSessionKey: bridge.defaultSessionKeyRef.current,
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
  return result.agent_id;
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
    agentId = await ensureRespondent(pi, ctx, bridge, rpcRegistry, thread, sessionCtx);
  } catch (err) {
    notify(ctx, `ws: could not open thread ${thread.threadId}: ${err instanceof Error ? err.message : String(err)}`, "error");
    return;
  }
  if (!agentId) return;

  thread.status = "open";
  thread.touchedAt = nowIso();
  persistThreads(handle);
  refreshPendingWidget(ctx, handle);

  // One overlay at a time (§5): the previous one is closed first; its own
  // fork is untouched and its thread stays reopenable.
  activeOverlay?.close();
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
      channel: createForkChannel(rpcRegistry, sessionCtx.cwd, agentId),
      onDone: (summary) => closeThreadOnDone(pi, handle, rpcRegistry, thread, summary),
      onOpened: (close) => {
        activeOverlay = { token, close };
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
