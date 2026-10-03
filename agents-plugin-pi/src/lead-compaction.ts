/**
 * ws-owned lead compaction (261002): the pure builders behind the summary the
 * lead session's `session_before_compact` handler (goal-loop.ts) returns as
 * `{ compaction }`, replacing Pi's native summarizer for the lead only.
 *
 * Summary = adapter-filled deterministic sections + the lead's own prose under
 * fixed headings. Every deterministic section is recomputed from the full
 * session history (`branchEntries`) and the live child registry on every
 * compaction — never parsed back out of the previous summary — so the sections
 * neither compound nor drift. The summary carries no file lists; the stored
 * entry is `fromHook`, which also ends Pi's file-list inheritance chain.
 *
 * The `## Dialog` section (261003) carries the user/lead discussion near-raw:
 * human-typed user text, the lead's assistant text, and one line per tool
 * call, under a byte budget. Tool results never cross; the compaction keeps no
 * raw tail (`NO_KEPT_ENTRY_ID`), and the summary points at the session file
 * where every pre-compaction entry stays searchable.
 *
 * Human-typed user messages are separated from adapter traffic here. Custom
 * messages (`ws-push-batch`, mailbox, preparation messages) are
 * `custom_message` entries and never qualify; the adapter's user-role
 * injections — the push wake line, goal reminders, and Pi's `/skill:` body
 * expansions — are recognized by their exact shape below. Anything else
 * user-role is treated as human text: dropping human text is the worse
 * failure, so an unrecognized shape stays in.
 */

import { readFileSync } from "node:fs";
import type { SessionEntry } from "@earendil-works/pi-coding-agent";
import { isPushWakeLine, type RpcAgentRecord, type RpcAgentRegistry } from "./spawner.ts";

/** The lead-callable compaction lever (replaces 260903's `goal-compact-and-continue`). */
export const LEAD_COMPACT_TOOL_NAME = "ws-compact";

/** `customType` of the adapter's preparation messages (advisory nudge, hard-cut steer, `/compact` reroute). */
export const LEAD_COMPACT_CUSTOM_TYPE = "ws-lead-compact";

/** `details.kind` stamped on every compaction entry this adapter authors; how a stored entry is recognized as ours. */
export const LEAD_COMPACTION_DETAILS_KIND = "ws-pi-lead-compaction";

/** Prefix of the goal reminder's correlation marker (goal-loop.ts appends `<id> -->`). */
export const GOAL_REMINDER_MARKER_PREFIX = "<!-- ws-pi-goal-reminder:";

export interface LeadCompactionDetails {
  kind: typeof LEAD_COMPACTION_DETAILS_KIND;
  version: 1;
  /** `lever`: the lead authored the prose; `fallback`: an in-hook summary call authored it. */
  source: "lever" | "fallback";
}

export function isLeadCompactionDetails(details: unknown): details is LeadCompactionDetails {
  return typeof details === "object" && details !== null && (details as { kind?: unknown }).kind === LEAD_COMPACTION_DETAILS_KIND;
}

/**
 * Default UTF-8 byte budget for the `## Dialog` section (261003). Bytes, not
 * characters or tokens: about 4 bytes per token in English and 3 bytes per
 * Korean character, so one byte budget lands near 10k tokens either way.
 */
export const DEFAULT_DIALOG_BUDGET_BYTES = 40960;
/** A user or assistant message over this many bytes keeps only its head and tail. Fixed, not a setting. */
export const LONG_MESSAGE_THRESHOLD_BYTES = 2560;
/** Bytes kept at each end of an elided message. */
export const LONG_MESSAGE_KEEP_BYTES = 1024;
/** Tool-call arguments JSON over this many bytes keeps only its head and tail. */
export const TOOL_ARGS_THRESHOLD_BYTES = 300;
/** Bytes kept at each end of elided tool-call arguments. */
export const TOOL_ARGS_KEEP_BYTES = 150;
/** Most tool lines kept between two dialog messages; earlier calls in the run fold into one line. */
export const TOOL_RUN_MAX_LINES = 8;

/**
 * `firstKeptEntryId` of every compaction this adapter authors (261003): an id
 * no session entry can carry (Pi mints 8-hex or UUID ids), so no raw entry
 * from before the compaction is kept after the summary. Pi's
 * `buildContextEntries` keeps pre-compaction entries only from the one whose
 * id matches, so a match-nothing id keeps none; `prepareCompaction` falls back
 * to the entry after the compaction as the next boundary, which is exactly
 * where the kept range starts.
 */
export const NO_KEPT_ENTRY_ID = "ws-pi-lead-compaction:no-kept-tail";

// ---------------------------------------------------------------------------
// The lead's prose: fixed headings, one lever field each.
// ---------------------------------------------------------------------------

/**
 * The fixed prose headings, in summary order. One lever parameter per heading
 * (rather than one free-form field) so a heading cannot silently go missing
 * between compactions.
 */
export const LEAD_PROSE_SECTIONS = [
  { key: "user_preferences", heading: "User preferences and style", description: "How the user wants to be worked with: tone, language, verbosity, approval habits, standing preferences." },
  { key: "working_practices", heading: "Agreed working practices", description: "Practices agreed in this session that no project document records yet." },
  { key: "unrecorded_decisions", heading: "Decisions not yet recorded", description: "Decisions made in this session that no ticket, commit, note, agenda, or todo holds yet." },
  { key: "current_work", heading: "Current work", description: "What is in progress: pointers (ticket stem, branch, commit, agent alias) for persisted state, precise prose for conversation-only state, and the active playbook's current step." },
  { key: "next_step", heading: "Immediate next step", description: "The very next action, quoting the user's latest request verbatim." },
  { key: "mood_rapport", heading: "Mood and rapport", description: "The user's current mood and the working rapport." },
  { key: "residual_details", heading: "Residual details", description: "Anything else only this conversation holds that has no tool home." },
] as const;

export type LeadProseKey = (typeof LEAD_PROSE_SECTIONS)[number]["key"];
export type LeadProse = Partial<Record<LeadProseKey, string>>;

/** JSON-schema `properties` for the lever, one required string per heading. */
export function leadProseParameterSchema(): { type: "object"; properties: Record<string, { type: "string"; description: string }>; required: string[] } {
  const properties: Record<string, { type: "string"; description: string }> = {};
  for (const section of LEAD_PROSE_SECTIONS) {
    properties[section.key] = { type: "string", description: `${section.heading}: ${section.description} Empty when there is nothing.` };
  }
  return { type: "object", properties, required: LEAD_PROSE_SECTIONS.map((section) => section.key) };
}

/** Renders the prose under its fixed `###` headings; a missing or blank field renders `(none)`. Field text is kept verbatim. */
export function renderLeadProse(prose: LeadProse): string {
  return LEAD_PROSE_SECTIONS.map((section) => {
    const value = prose[section.key];
    const body = typeof value === "string" && value.trim() !== "" ? value : "(none)";
    return `### ${section.heading}\n${body}`;
  }).join("\n\n");
}

// ---------------------------------------------------------------------------
// Session-entry helpers (duck-typed: tests pass plain objects).
// ---------------------------------------------------------------------------

interface ContentPart {
  type?: unknown;
  text?: unknown;
  id?: unknown;
  name?: unknown;
  arguments?: unknown;
}

function contentParts(content: unknown): ContentPart[] {
  if (typeof content === "string") return [{ type: "text", text: content }];
  return Array.isArray(content) ? (content as ContentPart[]) : [];
}

function messageText(content: unknown): string {
  const parts = contentParts(content);
  return parts
    .map((part) => (part?.type === "text" && typeof part.text === "string" ? part.text : part?.type === "image" ? "[image]" : ""))
    .filter((text) => text !== "")
    .join("\n");
}

function entryMessage(entry: SessionEntry): { role?: unknown; content?: unknown; toolCallId?: unknown; isError?: unknown } | undefined {
  return entry?.type === "message" ? (entry.message as { role?: unknown; content?: unknown }) : undefined;
}

function toolCalls(entry: SessionEntry): Array<{ name: string; args: Record<string, unknown> }> {
  const message = entryMessage(entry);
  if (message?.role !== "assistant") return [];
  return contentParts(message.content).flatMap((part) =>
    part?.type === "toolCall" && typeof part.name === "string"
      ? [{ name: part.name, args: typeof part.arguments === "object" && part.arguments !== null ? (part.arguments as Record<string, unknown>) : {} }]
      : [],
  );
}

/** Epoch-ms timestamp of the newest compaction entry, or `undefined` before the first compaction. */
export function previousCompactionAt(entries: readonly SessionEntry[]): number | undefined {
  for (let i = entries.length - 1; i >= 0; i--) {
    const entry = entries[i]!;
    if (entry?.type === "compaction") {
      const at = Date.parse(entry.timestamp);
      return Number.isFinite(at) ? at : undefined;
    }
  }
  return undefined;
}

// ---------------------------------------------------------------------------
// Human-typed user text.
// ---------------------------------------------------------------------------

const SKILL_EXPANSION_RE = /^<skill name="([^"]*)" location="[^"]*">[\s\S]*?\n<\/skill>(?:\n\n([\s\S]*))?$/;

/**
 * The human-typed part of one user-role message, or `undefined` when the
 * whole message is adapter traffic. A Pi `/skill:` expansion collapses back to
 * the `/skill:<name> <args>` the human typed.
 */
export function humanTextOf(text: string): string | undefined {
  if (text.includes(GOAL_REMINDER_MARKER_PREFIX)) return undefined;
  if (isPushWakeLine(text.trim())) return undefined;
  const skill = SKILL_EXPANSION_RE.exec(text);
  if (skill) {
    const args = skill[2]?.trim();
    return args ? `/skill:${skill[1]} ${args}` : `/skill:${skill[1]}`;
  }
  return text.trim() === "" ? undefined : text;
}

// ---------------------------------------------------------------------------
// Dialog transcript (261003): the user/lead discussion as close to raw as the
// byte budget allows, one line per tool call, no tool results.
// ---------------------------------------------------------------------------

export type DialogItem =
  | { kind: "user" | "assistant"; timestamp: string; text: string }
  /** `args` is the call's arguments as JSON, before elision. */
  | { kind: "tool"; name: string; args: string; failed: boolean }
  /** Earlier calls of an over-long tool run, counted by tool name, highest count first. */
  | { kind: "fold"; total: number; counts: Array<{ name: string; count: number }> };

/**
 * Every dialog item in the full branch history, oldest first: human-typed
 * user text (`humanTextOf`), the lead's assistant text, and one item per tool
 * call. Thinking parts, tool results, custom messages, and bash executions
 * are left out; a call whose result is an error is marked failed. The
 * `ws-compact` call is left out because its arguments are the summary's prose.
 */
export function collectDialogItems(entries: readonly SessionEntry[]): DialogItem[] {
  const failed = new Set<string>();
  for (const entry of entries) {
    const message = entryMessage(entry);
    if (message?.role === "toolResult" && message.isError === true && typeof message.toolCallId === "string") failed.add(message.toolCallId);
  }
  const items: DialogItem[] = [];
  for (const entry of entries) {
    const message = entryMessage(entry);
    if (message?.role === "user") {
      const text = humanTextOf(messageText(message.content));
      if (text !== undefined) items.push({ kind: "user", timestamp: entry.timestamp, text });
    } else if (message?.role === "assistant") {
      let pending: string[] = [];
      const flush = (): void => {
        const text = pending.join("\n");
        pending = [];
        if (text.trim() !== "") items.push({ kind: "assistant", timestamp: entry.timestamp, text });
      };
      for (const part of contentParts(message.content)) {
        if (part?.type === "text" && typeof part.text === "string") {
          pending.push(part.text);
        } else if (part?.type === "toolCall" && typeof part.name === "string" && part.name !== LEAD_COMPACT_TOOL_NAME) {
          flush();
          items.push({ kind: "tool", name: part.name, args: JSON.stringify(part.arguments ?? {}), failed: typeof part.id === "string" && failed.has(part.id) });
        }
      }
      flush();
    }
  }
  return items;
}

/** Keeps the last `TOOL_RUN_MAX_LINES` calls of each consecutive tool run; the earlier ones become one fold item ahead of them. */
export function foldToolRuns(items: readonly DialogItem[]): DialogItem[] {
  const result: DialogItem[] = [];
  let run: Array<Extract<DialogItem, { kind: "tool" }>> = [];
  const flushRun = (): void => {
    const foldedCount = run.length - TOOL_RUN_MAX_LINES;
    if (foldedCount > 0) {
      const counts = new Map<string, number>();
      for (const call of run.slice(0, foldedCount)) counts.set(call.name, (counts.get(call.name) ?? 0) + 1);
      result.push({
        kind: "fold",
        total: foldedCount,
        counts: [...counts].map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0)),
      });
      result.push(...run.slice(foldedCount));
    } else {
      result.push(...run);
    }
    run = [];
  };
  for (const item of items) {
    if (item.kind === "tool") {
      run.push(item);
    } else {
      flushRun();
      result.push(item);
    }
  }
  flushRun();
  return result;
}

/** Largest UTF-8 prefix of `bytes` within `limit` bytes that ends on a code-point boundary. */
function utf8HeadEnd(bytes: Buffer, limit: number): number {
  let end = Math.min(limit, bytes.length);
  // A continuation byte (10xxxxxx) at the cut means the cut splits a code point.
  while (end > 0 && end < bytes.length && (bytes[end]! & 0xc0) === 0x80) end--;
  return end;
}

/** Start of the largest UTF-8 suffix of `bytes` within `limit` bytes that begins on a code-point boundary. */
function utf8TailStart(bytes: Buffer, limit: number): number {
  let start = Math.max(0, bytes.length - limit);
  while (start < bytes.length && (bytes[start]! & 0xc0) === 0x80) start++;
  return start;
}

/**
 * `text` unchanged when it is at most `thresholdBytes` UTF-8 bytes; otherwise
 * its first and last `keepBytes` bytes (cut on code-point boundaries, so each
 * end may hold a few bytes less) around `[... N bytes skipped ...]`, where N
 * is the elided byte count. `join` separates the marker from both ends.
 */
export function elideMiddle(text: string, thresholdBytes: number, keepBytes: number, join: string): string {
  const bytes = Buffer.from(text, "utf8");
  if (bytes.length <= thresholdBytes) return text;
  const headEnd = utf8HeadEnd(bytes, keepBytes);
  const tailStart = utf8TailStart(bytes, keepBytes);
  const skipped = tailStart - headEnd;
  return `${bytes.subarray(0, headEnd).toString("utf8")}${join}[... ${skipped} bytes skipped ...]${join}${bytes.subarray(tailStart).toString("utf8")}`;
}

/** One item as the `## Dialog` section renders it, elision applied. */
export function renderDialogItem(item: DialogItem): string {
  switch (item.kind) {
    case "user":
    case "assistant":
      return `--- ${item.kind} (${item.timestamp}) ---\n${elideMiddle(item.text, LONG_MESSAGE_THRESHOLD_BYTES, LONG_MESSAGE_KEEP_BYTES, "\n")}`;
    case "tool":
      return `→ ${item.name} ${elideMiddle(item.args, TOOL_ARGS_THRESHOLD_BYTES, TOOL_ARGS_KEEP_BYTES, " ")}${item.failed ? " ✗failed" : ""}`;
    case "fold":
      return `→ (+${item.total} more: ${item.counts.map((entry) => `${entry.name}×${entry.count}`).join(", ")})`;
  }
}

export interface SelectedDialog {
  /** Rendered kept items, oldest first. */
  lines: string[];
  /** Every item, kept or not. */
  total: number;
  /** Older items dropped by the budget. */
  omitted: number;
}

/**
 * Selects rendered items newest-first within `budgetBytes`, each costing its
 * rendered UTF-8 bytes plus its line break. An item that does not fit whole is
 * dropped whole and selection stops there: the section is always a contiguous
 * newest run.
 */
export function selectDialogItems(items: readonly DialogItem[], budgetBytes: number): SelectedDialog {
  const lines: string[] = [];
  let used = 0;
  for (let i = items.length - 1; i >= 0; i--) {
    const line = renderDialogItem(items[i]!);
    const cost = Buffer.byteLength(line, "utf8") + 1;
    if (used + cost > budgetBytes) break;
    used += cost;
    lines.unshift(line);
  }
  return { lines, total: items.length, omitted: items.length - lines.length };
}

export const DIALOG_SECTION_HEADING = "## Dialog";

/** The whole `## Dialog` section: header, the session-file search pointer, then the kept items. */
export function buildDialogSection(entries: readonly SessionEntry[], budgetBytes: number, sessionFile: string | undefined): string {
  const selected = selectDialogItems(foldToolRuns(collectDialogItems(entries)), budgetBytes);
  const header = selected.omitted > 0
    ? `The newest ${selected.lines.length} of ${selected.total} dialog items (older ones are carried by the prose below).`
    : `All ${selected.total} dialog items of this session.`;
  const search = sessionFile
    ? `Full tool output and every earlier message remain in the session file \`${sessionFile}\`; search it (for example with grep) when you need them.`
    : "This session has no session file, so tool output from before this compaction cannot be searched.";
  return [DIALOG_SECTION_HEADING, header, search, ...selected.lines].join("\n");
}

// ---------------------------------------------------------------------------
// Child agents (spawner registry).
// ---------------------------------------------------------------------------

const TICKET_PATH_RE = /ai-docs\/tickets\/[A-Za-z._-]+\/\d{6}-[a-z0-9][a-z0-9-]*\.md/;
const TICKET_STEM_RE = /\b\d{6}-[a-z]+-[a-z0-9][a-z0-9-]*\b/;

function ticketRefIn(text: string | undefined): string | undefined {
  if (!text) return undefined;
  return TICKET_PATH_RE.exec(text)?.[0] ?? TICKET_STEM_RE.exec(text)?.[0];
}

/** One `<field>: <value>` line from a worker Report block. */
function reportField(text: string | undefined, field: string): string | undefined {
  if (!text) return undefined;
  const match = new RegExp(`^\\s*${field}:\\s*(.+?)\\s*$`, "m").exec(text);
  return match?.[1];
}

function childName(record: RpcAgentRecord): string {
  const label = record.alias ?? record.title;
  return label ? `${label} (${record.agentId})` : record.agentId;
}

function childTicket(record: RpcAgentRecord): string {
  const reported = reportField(record.lastText, "ticket");
  return ticketRefIn(reported) ?? reported ?? ticketRefIn(record.prompt) ?? "none named";
}

function isInFlight(record: RpcAgentRecord): boolean {
  return record.running || record.streaming || record.waitingOnChildren === true;
}

/** In-flight children: one line each with role, ticket, and state. Owner-thread agents are not the lead's and are left out. */
export function describeInFlightChildren(registry: RpcAgentRegistry | undefined): string[] {
  if (!registry) return [];
  return [...registry.values()]
    .filter((record) => !record.threadBound && isInFlight(record))
    .map((record) => {
      const state = record.waitingOnChildren ? "waiting on its own children" : "running";
      return `- ${childName(record)} [${record.spawnRole ?? "agent"}]: ticket ${childTicket(record)}; ${state}`;
    });
}

/**
 * Children whose run ended after the previous compaction (all settled ones
 * before the first): one line each — name, ticket, terminal status, commit —
 * read from the worker Report block in the child's last assistant text when
 * present. Each child appears in exactly one compaction's list, because the
 * cut-off moves forward with every compaction.
 */
export function describeFinishedChildren(registry: RpcAgentRegistry | undefined, since: number | undefined): string[] {
  if (!registry) return [];
  return [...registry.values()]
    .filter((record) => !record.threadBound && !isInFlight(record) && typeof record.settledAt === "number" && (since === undefined || record.settledAt > since))
    .sort((a, b) => a.settledAt! - b.settledAt!)
    .map((record) => {
      const status = reportField(record.lastText, "status") ?? "settled (no report block)";
      const stop = reportField(record.lastText, "stop");
      const commits = reportField(record.lastText, "commits") ?? "none reported";
      const stopPart = stop && stop !== "none" ? ` stop ${stop};` : "";
      return `- ${childName(record)}: ticket ${childTicket(record)}; status ${status};${stopPart} commits ${commits}`;
    });
}

// ---------------------------------------------------------------------------
// Active ticket and playbook (from the lead's own tool calls).
// ---------------------------------------------------------------------------

const TICKET_ARG_KEYS = ["ticket_path", "ticket_stem", "stem", "ticket", "path"] as const;

export interface ActiveTicket {
  ref: string;
  phase?: string;
  tool: string;
}

function ticketFromArgs(args: Record<string, unknown>): { ref: string; phase?: string } | undefined {
  for (const scope of [args, typeof args.target === "object" && args.target !== null ? (args.target as Record<string, unknown>) : undefined]) {
    if (!scope) continue;
    for (const key of TICKET_ARG_KEYS) {
      const value = scope[key];
      const ref = typeof value === "string" ? ticketRefIn(value) : undefined;
      if (ref) {
        const phase = scope.phase ?? args.phase;
        return { ref, phase: typeof phase === "string" || typeof phase === "number" ? String(phase) : undefined };
      }
    }
  }
  return undefined;
}

/** The ticket the lead's newest ticket-naming tool call addressed. */
export function findActiveTicket(entries: readonly SessionEntry[]): ActiveTicket | undefined {
  for (let i = entries.length - 1; i >= 0; i--) {
    const calls = toolCalls(entries[i]!);
    for (let j = calls.length - 1; j >= 0; j--) {
      const found = ticketFromArgs(calls[j]!.args);
      if (found) return { ...found, tool: calls[j]!.name };
    }
  }
  return undefined;
}

/** Recovery skills load nothing to resume, so they never become the active playbook. */
const NON_RESUMABLE_PLAYBOOKS = new Set(["lead-revive"]);

export interface ActivePlaybook {
  name: string;
  /** How it was loaded: `ws-skill`, `ws__playbook_read`, or a typed `/skill:` expansion. */
  via: "ws-skill" | "ws__playbook_read" | "/skill";
}

/** The newest playbook the lead loaded, from `ws-skill` / `ws__playbook_read` calls or a typed `/skill:` expansion. */
export function findActivePlaybook(entries: readonly SessionEntry[]): ActivePlaybook | undefined {
  for (let i = entries.length - 1; i >= 0; i--) {
    const entry = entries[i]!;
    const calls = toolCalls(entry);
    for (let j = calls.length - 1; j >= 0; j--) {
      const call = calls[j]!;
      const name = typeof call.args.name === "string" ? call.args.name.replace(/^.*:/, "") : undefined;
      if (!name || NON_RESUMABLE_PLAYBOOKS.has(name)) continue;
      if (call.name === "ws-skill") return { name, via: "ws-skill" };
      if (call.name === "ws__playbook_read") return { name, via: "ws__playbook_read" };
    }
    const message = entryMessage(entry);
    if (message?.role === "user") {
      const skill = /^<skill name="([^"]*)"/.exec(messageText(message.content));
      if (skill && !NON_RESUMABLE_PLAYBOOKS.has(skill[1]!)) return { name: skill[1]!, via: "/skill" };
    }
  }
  return undefined;
}

// ---------------------------------------------------------------------------
// Summary assembly.
// ---------------------------------------------------------------------------

export const LEAD_PROSE_SECTION_HEADING = "## Carried forward by the lead";
const RESUME_SECTION_HEADING = "## Resume";

export interface LeadCompactionSummaryInput {
  sessionKey: string | undefined;
  branchEntries: readonly SessionEntry[];
  registry: RpcAgentRegistry | undefined;
  /** The lead's prose, already rendered under the fixed headings. */
  prose: string;
  /** UTF-8 byte budget of the `## Dialog` section. */
  dialogBudgetBytes: number;
  /** The session JSONL path (`sessionManager.getSessionFile()`), `undefined` for an unpersisted session. */
  sessionFile: string | undefined;
}

function rereadInstruction(playbook: ActivePlaybook): string {
  return playbook.via === "ws__playbook_read"
    ? `re-read it with \`ws__playbook_read\` (name \`${playbook.name}\`)`
    : `re-read it with \`ws-skill ${playbook.name}\``;
}

/** Builds the whole summary text: deterministic sections, the lead's prose, and the closing `lead-revive` instruction. No file lists. */
export function buildLeadCompactionSummary(input: LeadCompactionSummaryInput): string {
  const { sessionKey, branchEntries, registry, prose, dialogBudgetBytes, sessionFile } = input;
  const keyText = sessionKey?.trim() ? `\`${sessionKey}\` (preserve verbatim)` : "unknown (recover it with `ws__workflow_manual`)";
  const ticket = findActiveTicket(branchEntries);
  const playbook = findActivePlaybook(branchEntries);
  const since = previousCompactionAt(branchEntries);
  const inFlight = describeInFlightChildren(registry);
  const finished = describeFinishedChildren(registry, since);

  const ticketLine = ticket
    ? `- Active ticket: \`${ticket.ref}\` (from the latest \`${ticket.tool}\` call); phase: ${ticket.phase ?? "the ticket's first phase without a `### Result`"}.`
    : "- Active ticket: none named by a tool call this session.";
  const playbookLine = playbook
    ? `- Active playbook: \`${playbook.name}\`. Its body is not re-attached: ${rereadInstruction(playbook)} before continuing it; the current step is named under Current work below.`
    : "- Active playbook: none loaded this session.";

  const sections = [
    "The ws Pi adapter wrote this summary. Durable state lives in ws tooling (tickets, commits, notes, agenda, todos); the sections below point at it and carry only what the conversation alone held.",
    ["## Session", `- ws session key: ${keyText}`, ticketLine, playbookLine].join("\n"),
    ["## Child agents in flight", ...(inFlight.length ? inFlight : ["(none)"])].join("\n"),
    ["## Child agents finished since the previous compaction", ...(finished.length ? finished : ["(none)"])].join("\n"),
    buildDialogSection(branchEntries, dialogBudgetBytes, sessionFile),
    `${LEAD_PROSE_SECTION_HEADING}\n${prose}`,
    `${RESUME_SECTION_HEADING}\nBefore any other workflow action, invoke \`lead-revive\` (\`ws-skill lead-revive\`) with session key ${sessionKey?.trim() ? `\`${sessionKey}\`` : "(recover it first)"}; it restores agenda, todos, and notes through \`workflow_manual\`.`,
  ];
  return sections.join("\n\n");
}

/**
 * The lead's prose out of a previous ws summary, or the whole text when it is
 * not one (another summarizer wrote it). Searched from the end: the prose and
 * `## Resume` are the summary's last sections, and the `## Dialog` section
 * before them may quote either heading verbatim.
 */
export function extractLeadProse(summary: string): string {
  const resume = summary.lastIndexOf(`\n\n${RESUME_SECTION_HEADING}\n`);
  const end = resume < 0 ? summary.length : resume;
  const start = summary.lastIndexOf(`${LEAD_PROSE_SECTION_HEADING}\n`, end);
  if (start < 0) return summary;
  return summary.slice(start + LEAD_PROSE_SECTION_HEADING.length + 1, end);
}

// ---------------------------------------------------------------------------
// Preparation guide and messages.
// ---------------------------------------------------------------------------

/** Minimal stand-in used only when the packaged guide file is unreadable. */
const FALLBACK_GUIDE = `Prepare for compaction: move durable detail into ws tooling (agenda, todos, notes; open decisions into tickets), then call \`${LEAD_COMPACT_TOOL_NAME}\` with every heading filled.`;

/** Reads `lead-compact-guide.md` fresh (never cached), falling back to a one-line stand-in. */
export function readLeadCompactGuide(path: string | undefined): string {
  if (!path) return FALLBACK_GUIDE;
  try {
    const text = readFileSync(path, "utf8").trim();
    return text || FALLBACK_GUIDE;
  } catch {
    return FALLBACK_GUIDE;
  }
}

export type PreparationTrigger =
  | { kind: "advisory"; percent: number; threshold: number; hardPercent: number }
  | { kind: "hard"; percent: number; threshold: number }
  | { kind: "reroute"; focus?: string };

/** The preparation message body: one trigger line, then the guide verbatim. */
export function buildPreparationMessage(trigger: PreparationTrigger, guide: string): string {
  let head: string;
  if (trigger.kind === "advisory") {
    head = [
      `Context usage is ${Math.round(trigger.percent)}% of the window (advisory point: ${trigger.threshold}%). This is a light`,
      "nudge, not an instruction to stop.",
      "",
      "Consider compacting now if this is a quiet point \u2014 for example, you are only",
      "waiting on background agents, or a piece of work just landed and what comes",
      "next is weakly related to what you are holding. If you are mid-task or holding",
      "context that would be costly to rebuild (an unsettled discussion, a",
      "half-applied change, a diagnosis in progress), keep going and compact at the",
      "next quiet point instead. You will not be nudged again before the hard point",
      `(${trigger.hardPercent}%), where compaction is no longer optional.`,
      "",
      "If you decide not to compact now, end this turn without replying. If you do,",
      "follow the guide below.",
    ].join("\n");
  } else if (trigger.kind === "hard") {
    head = `Context usage is ${Math.round(trigger.percent)}% of the window, past the hard compaction point (${trigger.threshold}%). Stop the current work now and prepare for compaction with the guide below before anything else.`;
  } else {
    const focus = trigger.focus?.trim();
    head = `The user ran /compact; the adapter cancelled Pi's native compaction so you can prepare it. Prepare for compaction now with the guide below.${focus ? `\nThe user's /compact focus text, to honor in your prose:\n${focus}` : ""}`;
  }
  return `${head}\n\n${guide}`;
}

// ---------------------------------------------------------------------------
// Fallback summary prompt (threshold/overflow compaction with no preparation).
// ---------------------------------------------------------------------------

export const FALLBACK_SYSTEM_PROMPT = "You are a context summarization assistant. You read a conversation between a user and an AI lead agent and write the lead's carry-forward prose under fixed headings. Do not continue the conversation and do not answer questions in it; output only the prose.";

/** The user prompt for the in-hook fallback summary call. */
export function buildFallbackSummaryPrompt(conversationText: string, previousProse: string | undefined): string {
  const headings = LEAD_PROSE_SECTIONS.map((section) => `### ${section.heading}\n(${section.description})`).join("\n\n");
  const previous = previousProse?.trim()
    ? `<previous-prose>\n${previousProse}\n</previous-prose>\n\nThe previous prose above came from the last compaction. Carry forward what is still live and drop what is resolved.\n\n`
    : "";
  return (
    `<conversation>\n${conversationText}\n</conversation>\n\n${previous}` +
    "Write the lead's carry-forward prose for the conversation above, from scratch, under exactly these headings, in this order, each starting with `### `:\n\n" +
    `${headings}\n\n` +
    "For content already persisted (tickets, commits, notes, agenda, todos), give its path or pointer; for content that lives only in the conversation, summarize it as precisely as possible. Write `(none)` under a heading with nothing to say. Aim for about 2-4k tokens. Do not list files that were read or modified."
  );
}
