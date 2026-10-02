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

/** Chars-per-token heuristic, the same one Pi's own `estimateTokens` uses. */
export const CHARS_PER_TOKEN = 4;
/** Default budget for the whole human-typed user-message section. */
export const DEFAULT_USER_MESSAGES_BUDGET_TOKENS = 8000;
/** Default cap for one human-typed message inside that section. */
export const DEFAULT_USER_MESSAGE_CAP_TOKENS = 1500;

export interface UserMessageBudgets {
  totalTokens: number;
  perMessageTokens: number;
}

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

function entryMessage(entry: SessionEntry): { role?: unknown; content?: unknown } | undefined {
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
// Human-typed user messages.
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

export interface HumanMessage {
  timestamp: string;
  text: string;
}

/** Every human-typed user message in the full branch history, oldest first. */
export function collectHumanMessages(entries: readonly SessionEntry[]): HumanMessage[] {
  const result: HumanMessage[] = [];
  for (const entry of entries) {
    const message = entryMessage(entry);
    if (message?.role !== "user") continue;
    const text = humanTextOf(messageText(message.content));
    if (text !== undefined) result.push({ timestamp: entry.timestamp, text });
  }
  return result;
}

export interface SelectedHumanMessages {
  /** Kept messages, oldest first, each already capped. */
  kept: HumanMessage[];
  /** Older messages dropped by the section budget. */
  omitted: number;
}

/**
 * Selects newest-first within the section budget after capping each message
 * at the per-message cap (head kept, truncation marker appended), so one
 * pasted log cannot take the whole budget. Selection stops at the first
 * message that no longer fits: the section is always a contiguous newest run.
 */
export function selectHumanMessages(messages: readonly HumanMessage[], budgets: UserMessageBudgets): SelectedHumanMessages {
  const totalChars = budgets.totalTokens * CHARS_PER_TOKEN;
  const capChars = budgets.perMessageTokens * CHARS_PER_TOKEN;
  const kept: HumanMessage[] = [];
  let used = 0;
  for (let i = messages.length - 1; i >= 0; i--) {
    const message = messages[i]!;
    const text = message.text.length > capChars
      ? `${message.text.slice(0, capChars)}\n[... truncated: ${message.text.length - capChars} more characters]`
      : message.text;
    if (used + text.length > totalChars) break;
    used += text.length;
    kept.unshift({ timestamp: message.timestamp, text });
  }
  return { kept, omitted: messages.length - kept.length };
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
  budgets: UserMessageBudgets;
}

function rereadInstruction(playbook: ActivePlaybook): string {
  return playbook.via === "ws__playbook_read"
    ? `re-read it with \`ws__playbook_read\` (name \`${playbook.name}\`)`
    : `re-read it with \`ws-skill ${playbook.name}\``;
}

/** Builds the whole summary text: deterministic sections, the lead's prose, and the closing `lead-revive` instruction. No file lists. */
export function buildLeadCompactionSummary(input: LeadCompactionSummaryInput): string {
  const { sessionKey, branchEntries, registry, prose, budgets } = input;
  const keyText = sessionKey?.trim() ? `\`${sessionKey}\` (preserve verbatim)` : "unknown (recover it with `ws__workflow_manual`)";
  const ticket = findActiveTicket(branchEntries);
  const playbook = findActivePlaybook(branchEntries);
  const since = previousCompactionAt(branchEntries);
  const inFlight = describeInFlightChildren(registry);
  const finished = describeFinishedChildren(registry, since);
  const human = selectHumanMessages(collectHumanMessages(branchEntries), budgets);

  const ticketLine = ticket
    ? `- Active ticket: \`${ticket.ref}\` (from the latest \`${ticket.tool}\` call); phase: ${ticket.phase ?? "the ticket's first phase without a `### Result`"}.`
    : "- Active ticket: none named by a tool call this session.";
  const playbookLine = playbook
    ? `- Active playbook: \`${playbook.name}\`. Its body is not re-attached: ${rereadInstruction(playbook)} before continuing it; the current step is named under Current work below.`
    : "- Active playbook: none loaded this session.";

  const humanHeader = human.omitted > 0
    ? `Human-typed messages, the newest ${human.kept.length} of ${human.kept.length + human.omitted} (older ones are carried by the prose below).`
    : `All ${human.kept.length} human-typed messages of this session.`;

  const sections = [
    "The ws Pi adapter wrote this summary. Durable state lives in ws tooling (tickets, commits, notes, agenda, todos); the sections below point at it and carry only what the conversation alone held.",
    ["## Session", `- ws session key: ${keyText}`, ticketLine, playbookLine].join("\n"),
    ["## Child agents in flight", ...(inFlight.length ? inFlight : ["(none)"])].join("\n"),
    ["## Child agents finished since the previous compaction", ...(finished.length ? finished : ["(none)"])].join("\n"),
    ["## User messages", humanHeader, ...human.kept.map((message) => `--- user message (${message.timestamp}) ---\n${message.text}`)].join("\n"),
    `${LEAD_PROSE_SECTION_HEADING}\n${prose}`,
    `${RESUME_SECTION_HEADING}\nBefore any other workflow action, invoke \`lead-revive\` (\`ws-skill lead-revive\`) with session key ${sessionKey?.trim() ? `\`${sessionKey}\`` : "(recover it first)"}; it restores agenda, todos, and notes through \`workflow_manual\`.`,
  ];
  return sections.join("\n\n");
}

/** The lead's prose out of a previous ws summary, or the whole text when it is not one (another summarizer wrote it). */
export function extractLeadProse(summary: string): string {
  const start = summary.indexOf(`${LEAD_PROSE_SECTION_HEADING}\n`);
  if (start < 0) return summary;
  const body = summary.slice(start + LEAD_PROSE_SECTION_HEADING.length + 1);
  const end = body.indexOf(`\n\n${RESUME_SECTION_HEADING}\n`);
  return end < 0 ? body : body.slice(0, end);
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
  | { kind: "advisory"; percent: number; threshold: number }
  | { kind: "hard"; percent: number; threshold: number }
  | { kind: "reroute"; focus?: string };

/** The preparation message body: one trigger line, then the guide verbatim. */
export function buildPreparationMessage(trigger: PreparationTrigger, guide: string): string {
  let head: string;
  if (trigger.kind === "advisory") {
    head = `Context usage is ${Math.round(trigger.percent)}% of the window, past the compaction advisory point (${trigger.threshold}%). The run has ended, so prepare for compaction now with the guide below before taking up new work.`;
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
