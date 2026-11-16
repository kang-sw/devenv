import { chmodSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { isDeepStrictEqual } from "node:util";
import { renameWithWindowsRetry, type RenameRetryHooks } from "./atomic-write.ts";

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

export { WS_PI_FORK_CONTEXT_ENV as FORK_CONTEXT_ENV, WS_PI_FORK_AFFINITY_ENV as FORK_AFFINITY_ENV } from "./process-role.ts";
import { WS_PI_FORK_CONTEXT_ENV as FORK_CONTEXT_ENV } from "./process-role.ts";
export const FORK_CONTEXT_ENTRY = "ws-pi-fork-context";
export const FORK_KEYS_ENTRY = "ws-pi-fork-keys";
/** Readiness kind the fork child publishes on the control channel and the parent validates. */
export const FORK_READINESS_KIND = "fork";

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

/** Strip prose only at schema nodes, never in property names or literal const/enum/default data. */
function forkValidationSchema(schema: unknown): unknown {
  if (Array.isArray(schema)) return schema.map(forkValidationSchema);
  if (!schema || typeof schema !== "object") return schema;
  const result = { ...schema } as Record<string, unknown>;
  delete result.description;
  for (const key of ["properties", "patternProperties", "$defs", "definitions", "dependentSchemas"]) {
    const map = result[key];
    if (map && typeof map === "object" && !Array.isArray(map)) {
      result[key] = Object.fromEntries(Object.entries(map).map(([name, child]) => [name, forkValidationSchema(child)]));
    }
  }
  for (const key of ["items", "additionalItems", "additionalProperties", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema", "allOf", "anyOf", "oneOf", "prefixItems"]) {
    if (Object.prototype.hasOwnProperty.call(result, key)) result[key] = forkValidationSchema(result[key]);
  }
  return result;
}

function schemaRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function schemaPath(parent: string, key: string): string {
  return `${parent}/${key.replace(/~/g, "~0").replace(/\//g, "~1")}`;
}

/**
 * Directional call-shape check: runtime may add optional properties and drop
 * requirements, recursively through existing properties. Other keywords stay
 * exact after prose stripping. This cannot detect handler semantic changes.
 */
function forkSchemaMismatch(captured: unknown, runtime: unknown, path = "/parameters"): string | undefined {
  if (isDeepStrictEqual(captured, runtime)) return undefined;
  if (!schemaRecord(captured) || !schemaRecord(runtime)) return `${path}: changed`;

  const oldRequired = captured.required === undefined ? [] : captured.required;
  const newRequired = runtime.required === undefined ? [] : runtime.required;
  if (!Array.isArray(oldRequired) || !Array.isArray(newRequired) ||
      !oldRequired.every(name => typeof name === "string") || !newRequired.every(name => typeof name === "string")) {
    return `${path}/required: invalid requirements`;
  }
  for (const name of newRequired) {
    if (!oldRequired.includes(name)) return `${schemaPath(`${path}/required`, name)}: new required property`;
  }

  const oldProperties = captured.properties === undefined ? {} : captured.properties;
  const newProperties = runtime.properties === undefined ? {} : runtime.properties;
  if (!schemaRecord(oldProperties) || !schemaRecord(newProperties)) return `${path}/properties: changed`;
  for (const [name, schema] of Object.entries(oldProperties)) {
    const propertyPath = schemaPath(`${path}/properties`, name);
    if (!Object.prototype.hasOwnProperty.call(newProperties, name)) return `${propertyPath}: missing runtime property`;
    const mismatch = forkSchemaMismatch(schema, newProperties[name], propertyPath);
    if (mismatch) return mismatch;
  }
  for (const name of Object.keys(newProperties)) {
    if (!Object.prototype.hasOwnProperty.call(oldProperties, name) && newRequired.includes(name)) {
      return `${schemaPath(`${path}/properties`, name)}: added required property`;
    }
  }
  for (const key of new Set([...Object.keys(captured), ...Object.keys(runtime)])) {
    if (key === "required" || key === "properties") continue;
    if (!isDeepStrictEqual(captured[key], runtime[key]) ||
        Object.prototype.hasOwnProperty.call(captured, key) !== Object.prototype.hasOwnProperty.call(runtime, key)) {
      return `${schemaPath(path, key)}: changed`;
    }
  }
  return undefined;
}

/** Compatibility comparison, retaining the parent-captured order in every result list. */
export interface ForkRegistrationComparison {
  missing: ForkToolDefinition[];
  extra: ForkToolDefinition[];
  reordered: boolean;
  changed: string[];
}

export function classifyForkRegistrations(expected: readonly ForkToolDefinition[], actual: readonly ForkToolDefinition[]): ForkRegistrationComparison {
  const expectedNames = new Set(expected.map((tool) => tool.name));
  const actualByName = new Map(actual.map((tool) => [tool.name, tool]));
  const missing = expected.filter((tool) => !actualByName.has(tool.name));
  const extra = actual.filter((tool) => !expectedNames.has(tool.name));
  const changed = expected.flatMap((tool) => {
    const registered = actualByName.get(tool.name);
    if (!registered) return [];
    // Compare only; the child retains its actual runtime registration.
    const mismatch = forkSchemaMismatch(forkValidationSchema(tool.parameters), forkValidationSchema(registered.parameters));
    return mismatch ? [`${tool.name} (${mismatch})`] : [];
  });
  const expectedCommonOrder = expected.filter((tool) => actualByName.has(tool.name)).map((tool) => tool.name);
  const actualCommonOrder = actual.filter((tool) => expectedNames.has(tool.name)).map((tool) => tool.name);
  const reordered = JSON.stringify(expectedCommonOrder) !== JSON.stringify(actualCommonOrder);
  return { missing, extra, reordered, changed };
}

export function formatForkRegistrationMismatch(comparison: ForkRegistrationComparison): string | undefined {
  const parts: string[] = [];
  if (comparison.missing.length) parts.push(`missing callable tools: ${comparison.missing.map((tool) => tool.name).join(", ")}`);
  if (comparison.extra.length) parts.push(`extra callable tools: ${comparison.extra.map((tool) => tool.name).join(", ")}`);
  if (comparison.reordered) parts.push("reordered callable tools");
  if (comparison.changed.length) parts.push(`changed callable tools: ${comparison.changed.join(", ")}`);
  return parts.length ? parts.join("; ") : undefined;
}

/** Preserve callable order; tolerate prose and compatible object-schema additions without replacing registrations. */
export function compareForkRegistrations(expected: readonly ForkToolDefinition[], actual: readonly ForkToolDefinition[]): string | undefined {
  return formatForkRegistrationMismatch(classifyForkRegistrations(expected, actual));
}

/** Stage-2 readiness payload a fork child sends over its control channel after `session_start`. */
export interface ForkReadiness {
  sessionId?: string;
  sessionPath?: string;
  ownSessionKey?: string;
  activeTools: string[];
  registeredTools: ForkToolDefinition[];
  error?: string;
}

export interface PrivateJsonWriteHooks extends RenameRetryHooks {
  temporaryName?: () => string;
}

const PRIVATE_JSON_DIAGNOSTIC_FIELD_CAP = 512;
function boundedDiagnosticField(value: unknown, fallback: string): string {
  if (typeof value !== "string" || !value) return fallback;
  return value.length <= PRIVATE_JSON_DIAGNOSTIC_FIELD_CAP ? value : `${value.slice(0, PRIVATE_JSON_DIAGNOSTIC_FIELD_CAP)}…`;
}

/** Builds transport-only exhaustion detail; private payload fields are never rendered. */
function privateJsonRetryExhausted(path: string, data: unknown, attempts: number, cause: NodeJS.ErrnoException): Error {
  const envelope = data && typeof data === "object" ? data as { nonce?: unknown; revision?: unknown } : undefined;
  const nonce = boundedDiagnosticField(envelope?.nonce, "unavailable");
  const revision = Number.isSafeInteger(envelope?.revision) && (envelope?.revision as number) >= 0 ? String(envelope?.revision) : "unavailable";
  const diagnosticPath = boundedDiagnosticField(path, "unavailable");
  const error = new Error(`ws-pi-private-json: replacement retries exhausted path=${JSON.stringify(diagnosticPath)} writer_pid=${process.pid} channel_nonce=${JSON.stringify(nonce)} snapshot_revision=${revision} attempts=${attempts}`);
  Object.defineProperties(error, {
    cause: { value: cause, enumerable: false },
    code: { value: cause.code, enumerable: false },
  });
  return error;
}

/** The optional hooks are a focused deterministic test seam; production callers use two arguments. */
export function writePrivateJson(path: string, data: unknown, hooks: PrivateJsonWriteHooks = {}): void {
  mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  const temporary = `${path}.${(hooks.temporaryName ?? randomUUID)()}.tmp`;
  try {
    writeFileSync(temporary, JSON.stringify(data), { mode: 0o600 });
    chmodSync(temporary, 0o600);
    renameWithWindowsRetry(temporary, path, hooks, (attempts, cause) => privateJsonRetryExhausted(path, data, attempts, cause));
  } finally {
    try { rmSync(temporary, { force: true }); } catch { /* Preserve the write failure. */ }
  }
}

/** The launch envelope carries the captured context in; readiness goes back over the control channel, not a file. */
export function readForkLaunchContext(env: NodeJS.ProcessEnv): { context?: ForkContext } | undefined {
  const path = env[FORK_CONTEXT_ENV];
  if (!path) return undefined;
  let envelope: { context?: unknown; legacy?: unknown };
  try { envelope = JSON.parse(readFileSync(path, "utf8")) as typeof envelope; } catch { throw new Error("ws-pi-fork: malformed launch envelope"); }
  if (!envelope || typeof envelope !== "object") throw new Error("ws-pi-fork: malformed launch envelope");
  const context = parseForkContext(envelope.context);
  if (!context && !(envelope.context === undefined && envelope.legacy === true)) throw new Error("ws-pi-fork: malformed launch envelope");
  return { context };
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

export function frameForkInput(text: string, ownKey: string, unavailableTools: readonly string[] = []): string {
  const frame = `Current fork-owned ws session_key: ${ownKey}. Use this key, not the inherited parent or any historical own key. ws-fork is refused in fork role.\n\n${text}`;
  if (!unavailableTools.length) return frame;
  return `${frame}\n\nUnavailable tools in this fork: ${unavailableTools.join(", ")}. Their parent extension was not loaded; calling one fails deterministically.`;
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
