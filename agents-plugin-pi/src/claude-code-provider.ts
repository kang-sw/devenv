/**
 * claude-code-provider.ts — the `claude-code` Pi provider
 * (260908-feat-ws-pi-claude-code-lead-provider, Phase 1).
 *
 * Wraps the `claude` CLI (subscription-authenticated, driven through the
 * Claude Agent SDK's stream-json protocol client) as a Pi model provider so a
 * Pi lead can run on `claude-code/opus|sonnet|haiku` while Pi keeps owning
 * history, tools, hooks, the TUI and the delegation layer. Contract:
 *
 *   - One claude process per Pi session, started lazily on the first
 *     `streamSimple` call. Claude Code's built-in tools, settings, CLAUDE.md,
 *     hooks and connector MCP servers are all off (`tools: []`,
 *     `settingSources: []`, `strictMcpConfig: true`); Pi's system prompt is
 *     passed through verbatim.
 *   - Every Pi tool in `context.tools` is reflected as an in-process SDK MCP
 *     tool (`mcp__pi__<name>`). Its handler does not execute anything: it
 *     PARKS. The `tool_use` block ends the current Pi turn with
 *     `stopReason: "toolUse"`, Pi's own loop executes the tool, and the next
 *     `streamSimple` call whose trailing messages are `toolResult`s resolves
 *     the parked handler by `tool_use` id, so Claude continues inside the
 *     same process. Parallel `tool_use` blocks become sequential Pi turns.
 *   - A trailing `user` message is pushed to the process as a new SDK user
 *     turn; multi-turn in one process is the normal path.
 *   - Resync rule: the process is continued only when the new message list
 *     extends the last-seen one by appended `toolResult`s and/or one trailing
 *     `user` message, every appended `toolResult` has a parked handler, and
 *     system prompt, tool-name set, model id and mapped effort are unchanged.
 *     Anything else (compaction, `--fork` start, history edit, grown tool set,
 *     `/model` or thinking change) closes the process and starts a new one
 *     whose first user message replays Pi's prior history as a deterministic
 *     transcript block (byte-stable for identical histories so Anthropic's
 *     prompt cache absorbs it). Every resync is logged with its reason.
 *   - Thinking: Pi `reasoning` → SDK `effort` (`off`/absent → thinking
 *     disabled, `minimal`/`low` → `low`, the rest one-to-one). SDK thinking,
 *     text and tool_use blocks map to Pi events at block granularity (live
 *     partial deltas are Phase 2).
 *   - Usage: each SDK assistant API message's input/output/cache tokens are
 *     summed into the Pi turn once per `message.id` (the SDK delivers one
 *     frame per content block, all carrying the same usage). Cost stays zero.
 *   - `session_shutdown` → `Query.close()`. `options.signal` abort →
 *     `Query.interrupt()`, the Pi turn ends `aborted`, parked handlers for it
 *     are rejected, the process is kept. Start/SDK failures end the turn as
 *     `error` naming the cause and the remedy; a `result` with `is_error`
 *     ends the turn as `error` with the SDK's text. No silent fallback.
 *
 * The SDK entry points (`query`, `createSdkMcpServer`, `tool`, plus the zod
 * namespace `tool()` needs) go through the `ClaudeCodeSdk` seam so the unit
 * tests script a fake without a claude binary; the default seam dynamically
 * imports `@anthropic-ai/claude-agent-sdk` and `zod` on first use.
 */

import type { ExtensionAPI, ProviderModelConfig } from "@earendil-works/pi-coding-agent";
import type {
  Api, AssistantMessage, AssistantMessageEventStream, Context, ImageContent, Message, Model, SimpleStreamOptions,
  TextContent, ThinkingLevel, Tool, ToolCall, ToolResultMessage, UserMessage,
} from "@earendil-works/pi-ai";
import { createAssistantMessageEventStream } from "@earendil-works/pi-ai";

export const CLAUDE_CODE_PROVIDER = "claude-code";
const MCP_SERVER = "pi";
const TOOL_PREFIX = `mcp__${MCP_SERVER}__`;
const ZERO_COST = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 };

/** Model ids pass straight through as the SDK `model` option (the CLI resolves the alias). */
export const CLAUDE_CODE_MODELS: ProviderModelConfig[] = [
  { id: "opus", name: "Claude Opus (Claude Code)", reasoning: true, input: ["text"], cost: ZERO_COST, contextWindow: 1_000_000, maxTokens: 128_000 },
  { id: "sonnet", name: "Claude Sonnet (Claude Code)", reasoning: true, input: ["text"], cost: ZERO_COST, contextWindow: 1_000_000, maxTokens: 128_000 },
  { id: "haiku", name: "Claude Haiku (Claude Code)", reasoning: true, input: ["text"], cost: ZERO_COST, contextWindow: 200_000, maxTokens: 64_000 },
];

export const CLAUDE_CODE_REMEDY = "Remedies: run `claude auth login` (subscription login), install Claude Code (`npm install -g @anthropic-ai/claude-code`), or switch the tier back to another provider (`/model`, or `config.tune agents.tier harness:pi`).";

// ---- SDK seam ----

export type SdkEffort = "low" | "medium" | "high" | "xhigh" | "max";
export interface SdkUserMessage { type: "user"; message: { role: "user"; content: string }; parent_tool_use_id: null }
export interface SdkContentBlock { type: string; id?: string; name?: string; input?: unknown; text?: string; thinking?: string; signature?: string }
export interface SdkUsage { input_tokens?: number; output_tokens?: number; cache_read_input_tokens?: number; cache_creation_input_tokens?: number }
export interface SdkMessage {
  type: string;
  subtype?: string;
  message?: { id?: string; model?: string; content?: SdkContentBlock[]; usage?: SdkUsage };
  error?: string;
  is_error?: boolean;
  result?: string;
  errors?: string[];
  [key: string]: unknown;
}
export interface SdkQuery extends AsyncIterable<SdkMessage> { interrupt(): Promise<unknown>; close(): void }
export interface SdkToolResult { content: Array<{ type: "text"; text: string }>; isError?: boolean }
export type SdkToolHandler = (args: Record<string, unknown>, extra: unknown) => Promise<SdkToolResult>;
/** Minimal zod surface `jsonSchemaToZodShape` needs; the real `z` satisfies it. */
export interface ZodLike {
  string(): ZodTypeLike; number(): ZodTypeLike; boolean(): ZodTypeLike; any(): ZodTypeLike; enum(values: [string, ...string[]]): ZodTypeLike;
}
export interface ZodTypeLike { optional(): ZodTypeLike; describe(description: string): ZodTypeLike }
export interface ClaudeCodeSdk {
  query(params: { prompt: AsyncIterable<SdkUserMessage>; options: Record<string, unknown> }): SdkQuery;
  createSdkMcpServer(options: { name: string; version: string; tools: unknown[]; alwaysLoad?: boolean }): unknown;
  tool(name: string, description: string, shape: Record<string, ZodTypeLike>, handler: SdkToolHandler): unknown;
  z: ZodLike;
}

async function loadDefaultSdk(): Promise<ClaudeCodeSdk> {
  const [sdk, zod] = await Promise.all([import("@anthropic-ai/claude-agent-sdk"), import("zod")]);
  return { query: sdk.query as unknown as ClaudeCodeSdk["query"], createSdkMcpServer: sdk.createSdkMcpServer as unknown as ClaudeCodeSdk["createSdkMcpServer"], tool: sdk.tool as unknown as ClaudeCodeSdk["tool"], z: zod.z as unknown as ZodLike };
}

// ---- pure helpers ----

/** Pi thinking level → SDK effort; absent/`off` disables thinking. */
export function effortForThinking(level: ThinkingLevel | "off" | undefined): SdkEffort | "off" {
  if (!level || level === "off") return "off";
  if (level === "minimal" || level === "low") return "low";
  return level;
}

/** Top-level properties become typed zod fields; nested objects/arrays pass through as `z.any()`, never dropped. */
export function jsonSchemaToZodShape(schema: unknown, z: ZodLike): Record<string, ZodTypeLike> {
  const shape: Record<string, ZodTypeLike> = {};
  const s = (schema ?? {}) as { properties?: Record<string, { type?: unknown; description?: unknown; enum?: unknown }>; required?: unknown };
  const required = new Set(Array.isArray(s.required) ? s.required.map(String) : []);
  for (const [key, prop] of Object.entries(s.properties ?? {})) {
    const type = Array.isArray(prop?.type) ? prop.type[0] : prop?.type;
    const enumValues = Array.isArray(prop?.enum) && prop.enum.length && prop.enum.every(v => typeof v === "string") ? prop.enum as [string, ...string[]] : undefined;
    let field = enumValues ? z.enum(enumValues)
      : type === "string" ? z.string()
      : type === "number" || type === "integer" ? z.number()
      : type === "boolean" ? z.boolean()
      : z.any();
    if (typeof prop?.description === "string" && prop.description) field = field.describe(prop.description);
    shape[key] = required.has(key) ? field : field.optional();
  }
  return shape;
}

function textOf(content: string | Array<TextContent | ImageContent>): string {
  if (typeof content === "string") return content;
  return content.map(part => part.type === "text" ? part.text : `[image ${part.mimeType}]`).join("\n");
}

const REPLAY_PREAMBLE = "The following is the prior conversation of this session, replayed verbatim after a context resync. Continue it as the assistant; do not summarize or repeat it.";
const REPLAY_CONTINUE = "Continue from the end of the transcript above.";

/** Deterministic (byte-stable for identical histories) transcript of every message but a trailing user one. */
export function buildReplayPrompt(messages: Message[]): string {
  const last = messages[messages.length - 1];
  const trailingUser = last?.role === "user" ? last : undefined;
  const history = trailingUser ? messages.slice(0, -1) : messages;
  const tail = trailingUser ? textOf(trailingUser.content) : REPLAY_CONTINUE;
  if (history.length === 0) return tail;
  const lines: string[] = ["<conversation_transcript>", REPLAY_PREAMBLE, ""];
  for (const message of history) {
    if (message.role === "user") {
      lines.push("[user]", textOf(message.content), "");
    } else if (message.role === "assistant") {
      lines.push("[assistant]");
      for (const block of message.content) {
        if (block.type === "text") lines.push(block.text);
        else if (block.type === "toolCall") lines.push(`[tool_call id="${block.id}" name="${block.name}"]`, JSON.stringify(block.arguments), "[/tool_call]");
      }
      lines.push("");
    } else if (message.role === "toolResult") {
      lines.push(`[tool_result id="${message.toolCallId}" name="${message.toolName}" error="${message.isError}"]`, textOf(message.content), "[/tool_result]", "");
    }
  }
  lines.push("</conversation_transcript>", "", tail);
  return lines.join("\n");
}

function messageKey(message: Message): string {
  return JSON.stringify([message.role, message.content, message.role === "toolResult" ? message.toolCallId : null]);
}

function makeInputQueue() {
  const items: SdkUserMessage[] = [];
  let waiter: ((message: SdkUserMessage) => void) | undefined;
  return {
    push(message: SdkUserMessage) {
      if (waiter) { const w = waiter; waiter = undefined; w(message); } else items.push(message);
    },
    async *iter(): AsyncGenerator<SdkUserMessage> {
      for (;;) {
        if (items.length) { yield items.shift()!; continue; }
        yield await new Promise<SdkUserMessage>(resolve => { waiter = resolve; });
      }
    },
  };
}

function describeError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

// ---- provider ----

interface Turn {
  stream: AssistantMessageEventStream;
  partial: AssistantMessage;
  nextIndex: number;
  countedMessageIds: Set<string>;
  /** A `tool_use` block was applied; the turn ends `toolUse` once its handler has parked. */
  sealedToolUseId?: string;
  finished: boolean;
  responded: boolean;
  options: SimpleStreamOptions | undefined;
  model: Model<Api>;
  owner?: ClaudeProcess;
  detachAbort?: () => void;
}
interface Parked { resolve: (result: SdkToolResult) => void; reject: (error: Error) => void }
type InboxItem = { kind: "block"; block: SdkContentBlock; usage: SdkUsage | undefined; messageId: string | undefined; model: string | undefined; error: string | undefined } | { kind: "result"; isError: boolean; text: string };
interface ClaudeProcess {
  query: SdkQuery;
  input: ReturnType<typeof makeInputQueue>;
  alive: boolean;
  systemPrompt: string;
  toolNames: string;
  modelId: string;
  effort: string;
  lastMessages: Message[];
  lastKeys: string[];
  /** Handler parked and paired to its `tool_use` id, awaiting Pi's toolResult. */
  parked: Map<string, Parked>;
  /** Handlers invoked before their `tool_use` block was paired, FIFO per tool name. */
  unpaired: Map<string, Parked[]>;
  /** `tool_use` blocks seen before their handler was invoked, FIFO per tool name. */
  unhandled: Map<string, string[]>;
  /** `tool_use` ids whose Pi turn was aborted before the handler parked; a late handler is rejected on arrival. */
  dead: Set<string>;
  inbox: InboxItem[];
  current: Turn | undefined;
  turnsInFlight: number;
  resultsToDiscard: number;
  announced: boolean;
}
type Plan = { kind: "continue"; toolResults: ToolResultMessage[]; user: UserMessage | undefined } | { kind: "restart"; reason: string };

export interface ClaudeCodeProviderDeps {
  sdk?: () => Promise<ClaudeCodeSdk>;
  cwd?: () => string;
  log?: (line: string) => void;
}

export interface ClaudeCodeProviderHandle {
  streamSimple: (model: Model<Api>, context: Context, options?: SimpleStreamOptions) => AssistantMessageEventStream;
  /** Close the live claude process (session_shutdown); idempotent. */
  close: () => void;
  hasProcess: () => boolean;
}

export function createClaudeCodeProvider(deps: ClaudeCodeProviderDeps = {}): ClaudeCodeProviderHandle {
  const loadSdk = deps.sdk ?? loadDefaultSdk;
  const cwd = deps.cwd ?? (() => process.cwd());
  const log = deps.log ?? ((line: string) => console.error(`[ws-pi-claude-code] ${line}`));
  let proc: ClaudeProcess | undefined;
  let sdkPromise: Promise<ClaudeCodeSdk> | undefined;

  function finish(turn: Turn, reason: "stop" | "toolUse" | "aborted" | "error", errorMessage?: string): void {
    if (turn.finished) return;
    turn.finished = true;
    turn.detachAbort?.();
    turn.partial.stopReason = reason;
    if (errorMessage) turn.partial.errorMessage = errorMessage;
    if (reason === "aborted" || reason === "error") turn.stream.push({ type: "error", reason, error: turn.partial });
    else turn.stream.push({ type: "done", reason, message: turn.partial });
    const owner = turn.owner;
    if (owner) {
      // Pi appends this assistant message to its history, so the next call's
      // extension check must treat it as already seen.
      owner.lastMessages.push(turn.partial);
      owner.lastKeys.push(messageKey(turn.partial));
      if (owner.current === turn) owner.current = undefined;
    }
  }

  function respondOnce(turn: Turn): void {
    if (turn.responded) return;
    turn.responded = true;
    void Promise.resolve(turn.options?.onResponse?.({ status: 200, headers: {} }, turn.model)).catch(error => log(`onResponse failed: ${describeError(error)}`));
  }

  function rejectAllParked(p: ClaudeProcess, why: string): void {
    const error = new Error(why);
    for (const parked of p.parked.values()) parked.reject(error);
    p.parked.clear();
    for (const list of p.unpaired.values()) for (const parked of list) parked.reject(error);
    p.unpaired.clear();
    for (const ids of p.unhandled.values()) for (const id of ids) p.dead.add(id);
  }

  function closeProcess(p: ClaudeProcess, why: string): void {
    if (!p.alive) return;
    p.alive = false;
    rejectAllParked(p, why);
    p.inbox.length = 0;
    try { p.query.close(); } catch (error) { log(`close failed: ${describeError(error)}`); }
    if (p.current) finish(p.current, "error", `${why}. ${CLAUDE_CODE_REMEDY}`);
  }

  /** Pair an invoked handler with its `tool_use` block: by the id the MCP request carries when present, else FIFO per tool name. */
  function toolInvoked(p: ClaudeProcess, name: string, extra: unknown): Promise<SdkToolResult> {
    return new Promise<SdkToolResult>((resolve, reject) => {
      const parked: Parked = { resolve, reject };
      if (!p.alive) { reject(new Error("claude process is closed")); return; }
      // Claude Code stamps the originating `tool_use` id on the MCP request as `_meta["claudecode/toolUseId"]` (verified live).
      const metaId = (extra as { _meta?: Record<string, unknown> } | undefined)?._meta?.["claudecode/toolUseId"];
      log(`tool ${name} invoked (tool_use id ${typeof metaId === "string" ? metaId : "absent from request meta"})`);
      const queued = p.unhandled.get(name) ?? [];
      let id = typeof metaId === "string" ? metaId : undefined;
      if (id === undefined) id = queued.shift();
      else { const index = queued.indexOf(id); if (index >= 0) queued.splice(index, 1); }
      if (id === undefined) {
        const list = p.unpaired.get(name) ?? [];
        list.push(parked);
        p.unpaired.set(name, list);
        return;
      }
      if (p.dead.delete(id)) { reject(new Error("Pi aborted the turn")); return; }
      p.parked.set(id, parked);
      if (p.current?.sealedToolUseId === id) finish(p.current, "toolUse");
    });
  }

  function applyBlock(p: ClaudeProcess, turn: Turn, item: Extract<InboxItem, { kind: "block" }>): void {
    respondOnce(turn);
    const { block, usage } = item;
    if (usage && (!item.messageId || !turn.countedMessageIds.has(item.messageId))) {
      if (item.messageId) turn.countedMessageIds.add(item.messageId);
      const u = turn.partial.usage;
      u.input += usage.input_tokens ?? 0;
      u.output += usage.output_tokens ?? 0;
      u.cacheRead += usage.cache_read_input_tokens ?? 0;
      u.cacheWrite += usage.cache_creation_input_tokens ?? 0;
      u.totalTokens = u.input + u.output + u.cacheRead + u.cacheWrite;
    }
    if (item.model) turn.partial.responseModel = item.model;
    const index = turn.nextIndex++;
    if (block.type === "text") {
      const text = block.text ?? "";
      turn.partial.content.push({ type: "text", text });
      turn.stream.push({ type: "text_start", contentIndex: index, partial: turn.partial });
      turn.stream.push({ type: "text_delta", contentIndex: index, delta: text, partial: turn.partial });
      turn.stream.push({ type: "text_end", contentIndex: index, content: text, partial: turn.partial });
    } else if (block.type === "thinking" || block.type === "redacted_thinking") {
      const thinking = block.thinking ?? "";
      turn.partial.content.push({ type: "thinking", thinking, ...(block.signature ? { thinkingSignature: block.signature } : {}), ...(block.type === "redacted_thinking" ? { redacted: true } : {}) });
      turn.stream.push({ type: "thinking_start", contentIndex: index, partial: turn.partial });
      if (thinking) turn.stream.push({ type: "thinking_delta", contentIndex: index, delta: thinking, partial: turn.partial });
      turn.stream.push({ type: "thinking_end", contentIndex: index, content: thinking, partial: turn.partial });
    } else if (block.type === "tool_use") {
      const rawName = String(block.name ?? "");
      const name = rawName.startsWith(TOOL_PREFIX) ? rawName.slice(TOOL_PREFIX.length) : rawName;
      const id = String(block.id ?? "");
      const call: ToolCall = { type: "toolCall", id, name, arguments: (block.input ?? {}) as Record<string, unknown> };
      turn.partial.content.push(call);
      turn.stream.push({ type: "toolcall_start", contentIndex: index, partial: turn.partial });
      turn.stream.push({ type: "toolcall_delta", contentIndex: index, delta: JSON.stringify(call.arguments), partial: turn.partial });
      turn.stream.push({ type: "toolcall_end", contentIndex: index, toolCall: call, partial: turn.partial });
      turn.sealedToolUseId = id;
      const waiting = p.unpaired.get(name)?.shift();
      if (waiting) {
        p.parked.set(id, waiting);
        finish(turn, "toolUse");
      } else if (p.parked.has(id)) {
        finish(turn, "toolUse");
      } else {
        const list = p.unhandled.get(name) ?? [];
        list.push(id);
        p.unhandled.set(name, list);
      }
    } else {
      turn.nextIndex--;
      log(`ignoring SDK content block of type ${block.type}`);
    }
    if (item.error) log(`SDK assistant message error: ${item.error}`);
  }

  function pump(p: ClaudeProcess): void {
    for (;;) {
      const turn = p.current;
      if (!turn || turn.finished || turn.sealedToolUseId || p.inbox.length === 0) return;
      const item = p.inbox.shift()!;
      if (item.kind === "block") { applyBlock(p, turn, item); continue; }
      respondOnce(turn);
      p.turnsInFlight = Math.max(0, p.turnsInFlight - 1);
      if (item.isError) finish(turn, "error", item.text);
      else if (p.turnsInFlight === 0) finish(turn, "stop");
    }
  }

  async function consume(p: ClaudeProcess): Promise<void> {
    try {
      for await (const message of p.query) {
        if (!p.alive) return;
        if (p.resultsToDiscard > 0) {
          if (message.type === "result") p.resultsToDiscard--;
          continue;
        }
        if (message.type === "assistant") {
          const m = message.message ?? {};
          for (const block of m.content ?? []) p.inbox.push({ kind: "block", block, usage: m.usage, messageId: m.id, model: m.model, error: message.error });
        } else if (message.type === "result") {
          const text = message.is_error ? [message.result, ...(message.errors ?? [])].filter(Boolean).join("\n") || `claude result ${message.subtype ?? "error"}` : "";
          p.inbox.push({ kind: "result", isError: Boolean(message.is_error), text });
        } else if (message.type === "system" && message.subtype === "init" && !p.announced) {
          // The SDK emits one init per user turn; announce the process once.
          p.announced = true;
          log(`claude process ready: model=${String(message.model)} mcp=${JSON.stringify(message.mcp_servers ?? [])}`);
        }
        pump(p);
      }
      if (p.alive) closeProcess(p, "claude process ended");
    } catch (error) {
      log(`claude process failed: ${describeError(error)}`);
      if (p.alive) closeProcess(p, `claude process failed: ${describeError(error)}`);
    }
  }

  function plan(context: Context, model: Model<Api>, effort: string, toolNames: string): Plan {
    const p = proc;
    if (!p) return { kind: "restart", reason: "no live process" };
    if (!p.alive) return { kind: "restart", reason: "process is not alive" };
    if (p.systemPrompt !== (context.systemPrompt ?? "")) return { kind: "restart", reason: "system prompt changed" };
    if (p.toolNames !== toolNames) return { kind: "restart", reason: "tool set changed" };
    if (p.modelId !== model.id) return { kind: "restart", reason: `model changed (${p.modelId} -> ${model.id})` };
    if (p.effort !== effort) return { kind: "restart", reason: `effort changed (${p.effort} -> ${effort})` };
    const messages = context.messages;
    if (messages.length <= p.lastMessages.length) return { kind: "restart", reason: "history is not an extension of the last-seen list" };
    for (let i = 0; i < p.lastMessages.length; i++) {
      if (messages[i] !== p.lastMessages[i] && messageKey(messages[i]) !== p.lastKeys[i]) return { kind: "restart", reason: `history diverged at message ${i}` };
    }
    const tail = messages.slice(p.lastMessages.length);
    const toolResults = tail.filter((m): m is ToolResultMessage => m.role === "toolResult");
    const users = tail.filter((m): m is UserMessage => m.role === "user");
    if (toolResults.length + users.length !== tail.length) return { kind: "restart", reason: "appended history carries a non-user, non-toolResult message" };
    if (users.length > 1 || (users.length === 1 && tail[tail.length - 1] !== users[0])) return { kind: "restart", reason: "appended history carries more than one trailing user message" };
    for (const result of toolResults) if (!p.parked.has(result.toolCallId)) return { kind: "restart", reason: `no parked handler for ${result.toolName} (${result.toolCallId})` };
    return { kind: "continue", toolResults, user: users[0] };
  }

  async function start(sdk: ClaudeCodeSdk, model: Model<Api>, context: Context, effort: SdkEffort | "off", toolNames: string, options: Record<string, unknown>): Promise<ClaudeProcess> {
    const input = makeInputQueue();
    const p: ClaudeProcess = {
      query: undefined as unknown as SdkQuery, input, alive: true, systemPrompt: context.systemPrompt ?? "", toolNames, modelId: model.id, effort,
      lastMessages: [], lastKeys: [], parked: new Map(), unpaired: new Map(), unhandled: new Map(), dead: new Set(), inbox: [], current: undefined, turnsInFlight: 0, resultsToDiscard: 0, announced: false,
    };
    const tools = (context.tools ?? []).map((t: Tool) => sdk.tool(t.name, t.description, jsonSchemaToZodShape(t.parameters, sdk.z), (_args, extra) => toolInvoked(p, t.name, extra)));
    const server = sdk.createSdkMcpServer({ name: MCP_SERVER, version: "1.0.0", tools, alwaysLoad: true });
    p.query = sdk.query({ prompt: input.iter(), options: { ...options, mcpServers: { [MCP_SERVER]: server } } });
    void consume(p);
    return p;
  }

  function sdkOptions(model: Model<Api>, context: Context, effort: SdkEffort | "off"): Record<string, unknown> {
    return {
      systemPrompt: context.systemPrompt ?? "",
      tools: [],
      settingSources: [],
      strictMcpConfig: true,
      persistSession: false,
      includePartialMessages: false,
      permissionMode: "bypassPermissions",
      allowDangerouslySkipPermissions: true,
      model: model.id,
      cwd: cwd(),
      stderr: (data: string) => { const line = data.trim(); if (line) log(`claude stderr: ${line.slice(0, 500)}`); },
      ...(effort === "off" ? { thinking: { type: "disabled" } } : { effort }),
    };
  }

  async function run(turn: Turn, context: Context, options: SimpleStreamOptions | undefined): Promise<void> {
    const model = turn.model;
    const effort = effortForThinking(options?.reasoning);
    const toolNames = (context.tools ?? []).map(t => t.name).sort().join("\n");
    const decision = plan(context, model, effort, toolNames);
    const messages = context.messages;
    const last = messages[messages.length - 1];
    if (decision.kind === "restart") {
      if (proc) log(`resync: ${decision.reason}`);
      else log(`starting claude process for ${model.id} (${decision.reason})`);
    }
    const userMessage = (text: string): SdkUserMessage => ({ type: "user", message: { role: "user", content: text }, parent_tool_use_id: null });
    let payload: { start?: Record<string, unknown>; messages: SdkUserMessage[]; toolResults: Array<{ toolCallId: string; toolName: string; text: string; isError: boolean }> } = decision.kind === "restart"
      ? { start: sdkOptions(model, context, effort), messages: [userMessage(buildReplayPrompt(messages))], toolResults: [] }
      : {
        messages: decision.user ? [userMessage(textOf(decision.user.content))] : [],
        toolResults: decision.toolResults.map(r => ({ toolCallId: r.toolCallId, toolName: r.toolName, text: textOf(r.content), isError: r.isError })),
      };
    const replacement = await options?.onPayload?.(payload, model);
    if (replacement !== undefined) payload = replacement as typeof payload;
    if (turn.finished) return;
    let p: ClaudeProcess;
    if (decision.kind === "restart") {
      if (proc) { closeProcess(proc, `resync: ${decision.reason}`); proc = undefined; }
      const sdk = await (sdkPromise ??= loadSdk().catch(error => { sdkPromise = undefined; throw error; }));
      p = await start(sdk, model, context, effort, toolNames, payload.start ?? sdkOptions(model, context, effort));
      proc = p;
    } else {
      p = proc!;
    }
    if (turn.finished) return;
    p.current = turn;
    turn.owner = p;
    const signal = options?.signal;
    if (signal) {
      const onAbort = () => abort(p, turn);
      signal.addEventListener("abort", onAbort, { once: true });
      turn.detachAbort = () => signal.removeEventListener("abort", onAbort);
      if (signal.aborted) { onAbort(); return; }
    }
    for (const result of payload.toolResults) {
      const parked = p.parked.get(result.toolCallId);
      p.parked.delete(result.toolCallId);
      if (!parked) { log(`toolResult ${result.toolCallId} (${result.toolName}) had no parked handler`); continue; }
      parked.resolve({ content: [{ type: "text", text: result.text }], ...(result.isError ? { isError: true } : {}) });
    }
    for (const message of payload.messages) { p.turnsInFlight++; p.input.push(message); }
    p.lastMessages = messages.slice();
    p.lastKeys = messages.map(messageKey);
    pump(p);
  }

  function abort(p: ClaudeProcess, turn: Turn): void {
    if (turn.finished) return;
    log("abort: interrupting claude process");
    p.resultsToDiscard += p.turnsInFlight;
    p.turnsInFlight = 0;
    p.inbox.length = 0;
    rejectAllParked(p, "Pi aborted the turn");
    void Promise.resolve().then(() => p.query.interrupt()).catch(error => log(`interrupt failed: ${describeError(error)}`));
    finish(turn, "aborted", "aborted");
  }

  function streamSimple(model: Model<Api>, context: Context, options?: SimpleStreamOptions): AssistantMessageEventStream {
    const stream = createAssistantMessageEventStream();
    const partial: AssistantMessage = {
      role: "assistant", content: [], api: model.api, provider: model.provider, model: model.id,
      usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { ...ZERO_COST, total: 0 } },
      stopReason: "pending", timestamp: Date.now(),
    };
    const turn: Turn = { stream, partial, nextIndex: 0, countedMessageIds: new Set(), finished: false, responded: false, options, model };
    stream.push({ type: "start", partial });
    run(turn, context, options).catch(error => {
      const message = `Claude Code provider could not start or drive the claude process: ${describeError(error)}. ${CLAUDE_CODE_REMEDY}`;
      log(message);
      finish(turn, "error", message);
    });
    return stream;
  }

  return {
    streamSimple,
    close: () => { if (proc) { closeProcess(proc, "session shutdown"); proc = undefined; } },
    hasProcess: () => Boolean(proc?.alive),
  };
}

/** Register `claude-code` in every process that loads the extension; the claude child only starts on first use. */
export function registerClaudeCodeProvider(pi: ExtensionAPI, deps: ClaudeCodeProviderDeps = {}): ClaudeCodeProviderHandle {
  const handle = createClaudeCodeProvider(deps);
  pi.registerProvider(CLAUDE_CODE_PROVIDER, {
    name: "Claude Code (subscription)",
    api: "anthropic-messages",
    baseUrl: "http://claude-code.invalid/",
    // Placeholder so hasConfiguredAuth reports true; real auth is the claude CLI's own login.
    apiKey: "claude-code-cli-login",
    streamSimple: handle.streamSimple,
    models: CLAUDE_CODE_MODELS,
  });
  pi.on("session_shutdown", async () => { handle.close(); });
  return handle;
}
