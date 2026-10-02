/** Stateless active-context dedupe for repeat playbook, skill, and workflow_manual reads. */

export type ReadFamily = "playbook.read" | "ws-skill" | "workflow_manual";

const READ_FAMILIES: readonly ReadFamily[] = ["playbook.read", "ws-skill", "workflow_manual"];

export interface ReadDedupeOptions {
  /**
   * Resolves a prior `ws__workflow_manual` call's raw arguments to the
   * session key the bridge actually dispatched with (omitted or sentinel
   * keys are default-filled at execute time, so the raw arguments alone do
   * not name it). Absent, the raw `session_key` argument is used.
   */
  workflowManualSessionKey?: (args: Record<string, unknown>) => unknown;
}

export interface ReadDedupeDecision {
  text: string;
  deduped: boolean;
}

const PROVENANCE_PREFIX = "<!-- ws-pi-read-dedupe-v1 ";
const PROVENANCE_SUFFIX = " -->";
const POINTER_PROSE_PREFIX = "The result is unchanged. Continue using the full result already present ";
const LEGACY_POINTER_PROSE_PREFIX = "The unchanged result is available from successful tool call `";

function stable(value: unknown): string {
  if (value === null || typeof value !== "object") return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(stable).join(",")}]`;
  const record = value as Record<string, unknown>;
  return `{${Object.keys(record).sort().map((key) => `${JSON.stringify(key)}:${stable(record[key])}`).join(",")}}`;
}

export function playbookReadKey(params: Record<string, unknown>): string {
  // Routing's outer session_key does not participate because this key only
  // projects name/context. A context.session_key is playbook substitution
  // data, so it remains part of the semantic context map.
  return stable({ name: params.name, context: params.context });
}

export function wsSkillKey(params: Record<string, unknown>): string {
  return stable({ name: params.name, args: params.args });
}

/** workflow_manual reads are keyed only by the resolved session key. */
export function workflowManualKey(sessionKey: unknown): string {
  return stable({ session_key: sessionKey ?? null });
}

/**
 * The text a workflow_manual result is compared by: every text item joined,
 * because the bridge appends advisories as separate text items and the
 * compared text is the full text the tool returns. Used for both the fresh
 * result and prior visible results so the two sides always agree.
 */
export function workflowManualResultText(content: readonly unknown[] | undefined): string | undefined {
  const texts = (content ?? []).flatMap((item: any) => (item?.type === "text" && typeof item.text === "string" ? [item.text as string] : []));
  return texts.length > 0 ? texts.join("\n") : undefined;
}

function headings(body: string): string {
  const found = body.match(/^#{1,3}\s+.+$/gm) ?? [];
  return found.length > 0 ? found.join("; ") : "(no headings)";
}

interface Provenance { originalToolCallId: string; family: ReadFamily; key: string }

function pointerText(originalToolCallId: string, family: ReadFamily, key: string, body: string, distance: number): string {
  const provenance: Provenance = { originalToolCallId, family, key };
  return `${POINTER_PROSE_PREFIX}${distance} tool call${distance === 1 ? "" : "s"} ago. Do not read it again (headings: ${headings(body)}).\n${PROVENANCE_PREFIX}${JSON.stringify(provenance)}${PROVENANCE_SUFFIX}`;
}

function parsePointer(text: string): Provenance | undefined {
  const start = text.lastIndexOf(PROVENANCE_PREFIX);
  if (start < 0 || !text.endsWith(PROVENANCE_SUFFIX)) return undefined;
  try {
    const value = JSON.parse(text.slice(start + PROVENANCE_PREFIX.length, -PROVENANCE_SUFFIX.length)) as Partial<Provenance>;
    return (typeof value.originalToolCallId === "string" && READ_FAMILIES.includes(value.family as ReadFamily) && typeof value.key === "string") ? value as Provenance : undefined;
  } catch {
    return undefined;
  }
}

function textResult(entry: any, family: ReadFamily): string | undefined {
  if (entry?.type !== "message" || entry.message?.role !== "toolResult" || entry.message.isError) return undefined;
  if (family === "workflow_manual") return workflowManualResultText(entry.message.content);
  const text = entry.message.content?.find((item: any) => item?.type === "text")?.text;
  return typeof text === "string" ? text : undefined;
}

function callInfo(entries: readonly unknown[], currentToolCallId: string, options: ReadDedupeOptions): Map<string, { family: ReadFamily; key: string }> {
  const calls = new Map<string, { family: ReadFamily; key: string }>();
  for (const entry of entries as any[]) {
    if (entry?.type !== "message" || entry.message?.role !== "assistant") continue;
    for (const call of entry.message.content ?? []) {
      if (call?.type !== "toolCall" || call.id === currentToolCallId || !call.arguments || typeof call.arguments !== "object") continue;
      if (call.name === "ws__playbook_read") calls.set(call.id, { family: "playbook.read", key: playbookReadKey(call.arguments) });
      if (call.name === "ws-skill") calls.set(call.id, { family: "ws-skill", key: wsSkillKey(call.arguments) });
      if (call.name === "ws__workflow_manual") {
        const sessionKey = options.workflowManualSessionKey ? options.workflowManualSessionKey(call.arguments) : call.arguments.session_key;
        calls.set(call.id, { family: "workflow_manual", key: workflowManualKey(sessionKey) });
      }
    }
  }
  return calls;
}

function toolCallDistance(entries: readonly unknown[], originalToolCallId: string, currentToolCallId: string): number {
  const ids: string[] = [];
  for (const entry of entries as any[]) {
    if (entry?.type !== "message" || entry.message?.role !== "assistant") continue;
    for (const call of entry.message.content ?? []) if (call?.type === "toolCall" && typeof call.id === "string") ids.push(call.id);
  }
  const original = ids.lastIndexOf(originalToolCallId);
  const current = ids.lastIndexOf(currentToolCallId);
  return original >= 0 && current > original ? current - original : 1;
}

/**
 * Reads only Pi's supplied active context. A full response counts when it is
 * byte-identical to `freshBody`; a pointer counts only when its envelope
 * resolves to that same visible full response. Therefore stale or forged
 * markers never suppress a read.
 */
export function dedupeRead(entries: readonly unknown[], currentToolCallId: string, family: ReadFamily, key: string, freshBody: string, options: ReadDedupeOptions = {}): ReadDedupeDecision {
  const calls = callInfo(entries, currentToolCallId, options);
  const matching = new Map<string, string[]>();
  for (const entry of entries as any[]) {
    const text = textResult(entry, family);
    const call = calls.get(entry?.message?.toolCallId);
    if (text !== undefined && call?.family === family && call.key === key) {
      const prior = matching.get(entry.message.toolCallId) ?? [];
      prior.push(text);
      matching.set(entry.message.toolCallId, prior);
    }
  }
  const fullIds = new Set([...matching].filter(([, texts]) => texts.length === 1 && texts[0] === freshBody).map(([id]) => id));
  let count = fullIds.size;
  for (const texts of matching.values()) for (const text of texts) {
    const marker = parsePointer(text);
    // A prior pointer's human paragraph may survive a transcript transform
    // while its provenance line is removed. It is still a pointer candidate,
    // never evidence for emitting another pointer.
    const candidate = text.startsWith(POINTER_PROSE_PREFIX) || text.startsWith(LEGACY_POINTER_PROSE_PREFIX) || text.includes(PROVENANCE_PREFIX);
    if (!candidate) continue;
    if (!marker || marker.family !== family || marker.key !== key || !fullIds.has(marker.originalToolCallId)) return { text: freshBody, deduped: false };
    count += 1;
  }
  if (count === 1) {
    const originalToolCallId = [...fullIds][0]!;
    return { text: pointerText(originalToolCallId, family, key, freshBody, toolCallDistance(entries, originalToolCallId, currentToolCallId)), deduped: true };
  }
  return { text: freshBody, deduped: false };
}
