import { chmodSync, existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { createHash, randomUUID } from "node:crypto";

/** Durable, immutable snapshot transferred from a lead to one fork child. */
export interface ForkToolDefinition {
  name: string;
  description: string;
  parameters: unknown;
}

export interface ForkContext {
  version: 1;
  kind: "task" | "discussion";
  effectiveSystemPrompt: string;
  /** Pi's rendered append string, retained exactly (including CRLF/trailing whitespace). */
  basePromptOptions?: unknown;
  wsBlock?: string;
  parentSessionKey?: string;
  /** Known parent keys, including a restarted lead's new key beside the captured original. */
  parentSessionKeys?: string[];
  parentPiSessionId?: string;
  parentAffinityId?: string;
  activeTools: string[];
  registeredTools: ForkToolDefinition[];
  /** Non-secret, serialization-affecting model/config fingerprint. */
  modelDescriptor?: Record<string, unknown>;
  thinkingLevel?: string;
}

export { WS_PI_FORK_CONTEXT_ENV as FORK_CONTEXT_ENV, WS_PI_FORK_READY_PATH_ENV as FORK_READY_PATH_ENV, WS_PI_FORK_READY_NONCE_ENV as FORK_READY_NONCE_ENV, WS_PI_FORK_AFFINITY_ENV as FORK_AFFINITY_ENV } from "./process-role.ts";
import { WS_PI_FORK_CONTEXT_ENV as FORK_CONTEXT_ENV } from "./process-role.ts";
export const FORK_CONTEXT_ENTRY = "ws-pi-fork-context";
export const FORK_KEYS_ENTRY = "ws-pi-fork-keys";

function own<T>(value: T): T { return JSON.parse(JSON.stringify(value)) as T; }

function validDefinition(value: unknown): value is ForkToolDefinition {
  const v = value as Partial<ForkToolDefinition> | null;
  return !!v && typeof v.name === "string" && typeof v.description === "string" && Object.prototype.hasOwnProperty.call(v, "parameters");
}

/** Parse present metadata strictly; callers distinguish undefined (legacy) from an Error. */
export function parseForkContext(value: unknown): ForkContext | undefined {
  if (value === undefined) return undefined;
  let raw: unknown = value;
  if (typeof value === "string") {
    try { raw = JSON.parse(value); } catch { throw new Error("ws-pi-fork: malformed delivered fork context"); }
  }
  const c = raw as Partial<ForkContext> | null;
  if (!c || c.version !== 1 || (c.kind !== "task" && c.kind !== "discussion") || typeof c.effectiveSystemPrompt !== "string" ||
      !Array.isArray(c.activeTools) || !c.activeTools.every((name) => typeof name === "string") ||
      !Array.isArray(c.registeredTools) || !c.registeredTools.every(validDefinition)) {
    throw new Error("ws-pi-fork: malformed delivered fork context");
  }
  if (c.wsBlock !== undefined && typeof c.wsBlock !== "string") throw new Error("ws-pi-fork: malformed delivered fork context");
  if (c.parentSessionKey !== undefined && typeof c.parentSessionKey !== "string") throw new Error("ws-pi-fork: malformed delivered fork context");
  if (c.parentSessionKeys !== undefined && (!Array.isArray(c.parentSessionKeys) || !c.parentSessionKeys.every(key => typeof key === "string"))) throw new Error("ws-pi-fork: malformed delivered fork context");
  if (c.parentPiSessionId !== undefined && typeof c.parentPiSessionId !== "string") throw new Error("ws-pi-fork: malformed delivered fork context");
  if (c.parentAffinityId !== undefined && typeof c.parentAffinityId !== "string") throw new Error("ws-pi-fork: malformed delivered fork context");
  if (c.thinkingLevel !== undefined && typeof c.thinkingLevel !== "string") throw new Error("ws-pi-fork: malformed delivered fork context");
  return own(c as ForkContext);
}

/** Pi deliberately does not flush a new session until its first assistant message.
 * Snapshot the readable branch for a private launch source, never create the
 * parent's allocated file (Pi later opens that path with wx).
 */
export function captureUnflushedForkSource(ctx: unknown): unknown[] | undefined {
  const sm = (ctx as { sessionManager?: { getSessionFile(): string | undefined; getHeader(): unknown; getBranch(): unknown[] } }).sessionManager;
  const file = sm?.getSessionFile();
  if (!file || existsSync(file) || !sm?.getHeader || !sm.getBranch) return undefined;
  return own([sm.getHeader(), ...sm.getBranch()]);
}

export function captureForkContext(input: Omit<ForkContext, "version">): ForkContext {
  return own({ ...input, version: 1, activeTools: [...input.activeTools], registeredTools: input.registeredTools.map(own) });
}

export function captureRegisteredTools(activeNames: readonly string[], allTools: readonly { name: string; description?: string; parameters?: unknown }[]): ForkToolDefinition[] {
  const byName = new Map(allTools.map((tool) => [tool.name, tool]));
  return activeNames.map((name) => {
    const tool = byName.get(name);
    if (!tool) throw new Error(`ws-pi-fork: active tool "${name}" is not registered`);
    return { name, description: tool.description ?? "", parameters: own(tool.parameters) };
  });
}

/** Exact order/schema comparison; no sorting, deduplication, or schema replay. */
export function compareForkRegistrations(expected: readonly ForkToolDefinition[], actual: readonly ForkToolDefinition[]): string | undefined {
  if (expected.length !== actual.length) return `expected ${expected.length} callable tools, got ${actual.length}`;
  for (let i = 0; i < expected.length; i += 1) {
    if (JSON.stringify(expected[i]) !== JSON.stringify(actual[i])) return `callable tool registration differs at index ${i} (${expected[i]?.name ?? "missing"})`;
  }
  return undefined;
}

export interface ForkReadiness {
  nonce: string;
  sessionId?: string;
  sessionPath?: string;
  ownSessionKey?: string;
  activeTools: string[];
  registeredTools: ForkToolDefinition[];
  error?: string;
}

export function writePrivateJson(path: string, data: unknown): void {
  mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  const temporary = `${path}.${randomUUID()}.tmp`;
  writeFileSync(temporary, JSON.stringify(data), { mode: 0o600 });
  chmodSync(temporary, 0o600);
  renameSync(temporary, path);
}

export function readForkLaunchContext(env: NodeJS.ProcessEnv): { context?: ForkContext; nonce: string; readinessPath: string } | undefined {
  const path = env[FORK_CONTEXT_ENV];
  if (!path) return undefined;
  let envelope: { context?: unknown; nonce?: unknown; readinessPath?: unknown; legacy?: unknown };
  try { envelope = JSON.parse(readFileSync(path, "utf8")) as typeof envelope; } catch { throw new Error("ws-pi-fork: malformed launch envelope"); }
  if (typeof envelope.nonce !== "string" || !envelope.nonce || typeof envelope.readinessPath !== "string" || !envelope.readinessPath) {
    throw new Error("ws-pi-fork: malformed launch envelope");
  }
  const context = parseForkContext(envelope.context);
  if (!context && !(envelope.context === undefined && envelope.legacy === true)) throw new Error("ws-pi-fork: malformed launch envelope");
  return { context, nonce: envelope.nonce, readinessPath: envelope.readinessPath };
}

/** Reads only data persisted by this child session; inherited parent entries are not trusted. */
export function restoreForkContext(entries: readonly unknown[], ownSessionId: string | undefined): ForkContext | undefined {
  if (!ownSessionId) return undefined;
  for (const entry of [...entries].reverse()) {
    const e = entry as { type?: unknown; customType?: unknown; data?: { sessionId?: unknown; context?: unknown } } | null;
    if (e?.type !== "custom" || e.customType !== FORK_CONTEXT_ENTRY || e.data?.sessionId !== ownSessionId) continue;
    return parseForkContext(e.data.context);
  }
  return undefined;
}

export function restoreForkKeys(entries: readonly unknown[], sessionId: string): string[] {
  const keys = new Set<string>();
  for (const [index, entry] of entries.entries()) {
    const e = entry as { type?: string; customType?: string; data?: { sessionId?: string; current?: unknown; previous?: unknown } };
    const preceding = entries[index - 1] as { type?: string; customType?: string; data?: { sessionId?: string } } | undefined;
    // The first context format wrote an unbound key immediately after its
    // child-bound context entry. Migrate only that provable ownership pair.
    const owner = e.data?.sessionId ?? (preceding?.type === "custom" && preceding.customType === FORK_CONTEXT_ENTRY ? preceding.data?.sessionId : undefined);
    if (e.type !== "custom" || e.customType !== FORK_KEYS_ENTRY || !e.data || owner !== sessionId) continue;
    for (const key of [e.data.current, ...(Array.isArray(e.data.previous) ? e.data.previous : [])]) {
      if (typeof key === "string" && key.trim()) keys.add(key);
    }
  }
  return [...keys];
}

/** Snapshot all model fields by digest so sensitive endpoint/header values never become durable plaintext. */
export function forkModelDescriptor(model: unknown, thinkingLevel: string | undefined): Record<string, unknown> | undefined {
  if (!model || typeof model !== "object") return undefined;
  const m = model as { provider?: string; api?: string };
  return { provider: m.provider, api: m.api, modelDigest: configDigest(model), thinkingLevel };
}

export async function effectiveForkDescriptor(ctx: { model?: unknown; modelRegistry?: { getProviderAuth?: (provider: string) => Promise<unknown> } }, thinkingLevel: string | undefined): Promise<Record<string, unknown> | undefined> {
  const descriptor = forkModelDescriptor(ctx.model, thinkingLevel);
  if (!descriptor || !ctx.modelRegistry?.getProviderAuth) return undefined;
  try {
    const auth = await ctx.modelRegistry.getProviderAuth(descriptor.provider as string);
    return auth ? { ...descriptor, authDigest: configDigest(auth) } : undefined;
  } catch { return undefined; } // Unsupported affinity never discards the prompt.
}

export function frameForkInput(text: string, ownKey: string): string {
  return `Current fork-owned ws session_key: ${ownKey}. Use this key, not the inherited parent or any historical own key. ws-fork, ws-queue-question, and ws-withdraw-question are refused in fork role; report to the lead instead.\n\n${text}`;
}

export function configDigest(value: unknown): string {
  return createHash("sha256").update(JSON.stringify(value)).digest("hex");
}

/** Only the Codex body cache key is affinity-rewritten; all other payload bytes remain Pi-owned. */
export function applyForkAffinity(payload: unknown, context: ForkContext | undefined, currentDescriptor: Record<string, unknown> | undefined, ownAffinityId: string | undefined): unknown {
  if (!context?.parentAffinityId || !ownAffinityId || !context.modelDescriptor || !currentDescriptor ||
      context.modelDescriptor.provider !== "openai-codex" || currentDescriptor.provider !== "openai-codex" ||
      context.modelDescriptor.api !== "openai-codex-responses" || currentDescriptor.api !== "openai-codex-responses" ||
      JSON.stringify(context.modelDescriptor) !== JSON.stringify(currentDescriptor)) return undefined;
  const p = payload as { prompt_cache_key?: unknown; instructions?: unknown; input?: unknown; model?: unknown; tools?: unknown } | null;
  if (!p || typeof p.prompt_cache_key !== "string" || !p.prompt_cache_key || typeof p.instructions !== "string" ||
      typeof p.model !== "string" || !Array.isArray(p.input) || (p.tools !== undefined && !Array.isArray(p.tools))) return undefined;
  return { ...p, prompt_cache_key: context.parentAffinityId };
}

export function removeForkTransport(path: string | undefined): void { if (path) rmSync(path, { force: true }); }
