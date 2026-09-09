/** Stateless active-context dedupe for repeat playbook and skill reads. */

export type ReadFamily = "playbook.read" | "ws-skill";

export interface ReadDedupeDecision {
  text: string;
  deduped: boolean;
}

const PROVENANCE_PREFIX = "<!-- ws-pi-read-dedupe-v1 ";
const PROVENANCE_SUFFIX = " -->";

function stable(value: unknown): string {
  if (value === null || typeof value !== "object") return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(stable).join(",")}]`;
  const record = value as Record<string, unknown>;
  return `{${Object.keys(record).sort().map((key) => `${JSON.stringify(key)}:${stable(record[key])}`).join(",")}}`;
}

function withoutSessionKey(value: unknown): unknown {
  if (!value || typeof value !== "object" || Array.isArray(value)) return value;
  const { session_key: _ignored, ...rest } = value as Record<string, unknown>;
  return rest;
}

export function playbookReadKey(params: Record<string, unknown>): string {
  return stable({ name: params.name, context: withoutSessionKey(params.context) });
}

export function wsSkillKey(params: Record<string, unknown>): string {
  return stable({ name: params.name, args: params.args });
}

function headings(body: string): string {
  const found = body.match(/^#{1,3}\s+.+$/gm) ?? [];
  return found.length > 0 ? found.join("; ") : "(no headings)";
}

interface Provenance { originalToolCallId: string; family: ReadFamily; key: string }

function pointerText(originalToolCallId: string, family: ReadFamily, key: string, body: string): string {
  const provenance: Provenance = { originalToolCallId, family, key };
  return `The unchanged result is available from successful tool call \`${originalToolCallId}\` (headings: ${headings(body)}).\n${PROVENANCE_PREFIX}${JSON.stringify(provenance)}${PROVENANCE_SUFFIX}`;
}

function parsePointer(text: string): Provenance | undefined {
  const start = text.lastIndexOf(PROVENANCE_PREFIX);
  if (start < 0 || !text.endsWith(PROVENANCE_SUFFIX)) return undefined;
  try {
    const value = JSON.parse(text.slice(start + PROVENANCE_PREFIX.length, -PROVENANCE_SUFFIX.length)) as Partial<Provenance>;
    return (typeof value.originalToolCallId === "string" && (value.family === "playbook.read" || value.family === "ws-skill") && typeof value.key === "string") ? value as Provenance : undefined;
  } catch {
    return undefined;
  }
}

function textResult(entry: any): string | undefined {
  if (entry?.type !== "message" || entry.message?.role !== "toolResult" || entry.message.isError) return undefined;
  const text = entry.message.content?.find((item: any) => item?.type === "text")?.text;
  return typeof text === "string" ? text : undefined;
}

function callInfo(entries: readonly unknown[], currentToolCallId: string): Map<string, { family: ReadFamily; key: string }> {
  const calls = new Map<string, { family: ReadFamily; key: string }>();
  for (const entry of entries as any[]) {
    if (entry?.type !== "message" || entry.message?.role !== "assistant") continue;
    for (const call of entry.message.content ?? []) {
      if (call?.type !== "toolCall" || call.id === currentToolCallId || !call.arguments || typeof call.arguments !== "object") continue;
      if (call.name === "ws__playbook_read") calls.set(call.id, { family: "playbook.read", key: playbookReadKey(call.arguments) });
      if (call.name === "ws-skill") calls.set(call.id, { family: "ws-skill", key: wsSkillKey(call.arguments) });
    }
  }
  return calls;
}

/**
 * Reads only Pi's supplied active context. A full response counts when it is
 * byte-identical to `freshBody`; a pointer counts only when its envelope
 * resolves to that same visible full response. Therefore stale or forged
 * markers never suppress a read.
 */
export function dedupeRead(entries: readonly unknown[], currentToolCallId: string, family: ReadFamily, key: string, freshBody: string): ReadDedupeDecision {
  const calls = callInfo(entries, currentToolCallId);
  const matching = new Map<string, string>();
  for (const entry of entries as any[]) {
    const text = textResult(entry);
    const call = calls.get(entry?.message?.toolCallId);
    if (text !== undefined && call?.family === family && call.key === key) matching.set(entry.message.toolCallId, text);
  }
  const fullIds = new Set([...matching].filter(([, text]) => text === freshBody).map(([id]) => id));
  let count = fullIds.size;
  for (const text of matching.values()) {
    const marker = parsePointer(text);
    if (marker?.family === family && marker.key === key && fullIds.has(marker.originalToolCallId)) count += 1;
  }
  if (count === 1) {
    const originalToolCallId = [...fullIds][0]!;
    return { text: pointerText(originalToolCallId, family, key, freshBody), deduped: true };
  }
  return { text: freshBody, deduped: false };
}
