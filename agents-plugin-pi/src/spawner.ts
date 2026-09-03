/**
 * Self-built delegation spawner: `ws-agent-spawn` / `ws-agent-continue` /
 * `ws-agent-wait` / `explore`.
 *
 * Spawns a separate `pi` child process (`--mode json -p`) per delegated
 * agent, capturing its NDJSON event stream to extract the final assistant
 * text and `stopReason`, and gates `state:"done"` on the child's `close`
 * event rather than on an in-stream terminal `stopReason` — the `--session`
 * file is only flush-guaranteed after the process exits (see
 * ai-docs/tickets/idea/260802-research-ws-pi-native-framework.md#L760-766),
 * so an immediate `ws-agent-continue` right after `ws-agent-wait` harvests a
 * "done" agent must be safe to read that file. The last-seen `stopReason` is
 * carried only as metadata, never as the completion signal.
 *
 * Zero on-disk agent-profile files: tool-group curation lives entirely in
 * the in-memory `TOOL_GROUPS` table below plus `pi` CLI flags
 * (`--tools`/`--model`/`--append-system-prompt`/`--session`), matching the
 * ticket's Decisions section (no `.pi/agents/` writes, no global profile
 * files).
 *
 * `--model` resolves as **inherit** this phase: the active model
 * (`ctx.model`) of the *calling* tool-execute context is forwarded verbatim
 * as `provider/id`, or omitted entirely when unset, mirroring the shipped
 * `examples/extensions/subagent/index.ts`'s `dispatchDefaults.model`
 * pattern. A tier -> model catalog is explicit Phase 3 scope, not built
 * here; `tier` is accepted and stored as record metadata only.
 */

import { spawn, type ChildProcess } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { randomUUID } from "node:crypto";
import { StringDecoder } from "node:string_decoder";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { McpStdioClient, McpToolCallResult } from "./mcp-stdio-client.ts";
import type { BridgeHandle } from "./bridge.ts";

// ---------------------------------------------------------------------------
// Pure helpers: tool-group resolution, terminal-stopReason classification,
// spawn-arg building. Unit-tested directly (test/spawner.test.ts) with no
// subprocess involved.
// ---------------------------------------------------------------------------

export type ToolGroup = "read-only" | "recon" | "full-worker";

/**
 * Built-in Pi tool names per curated group (docs/usage.md's `### Tool
 * Options`: `read, bash, edit, write, grep, find, ls`).
 *
 * - `read-only`: pure inspection, no shell at all — the tightest group.
 * - `recon`: read-only plus `bash` (git log/grep -r/etc.) for exploration —
 *   still excludes `edit`/`write` per `rsrc/explore/explore.md`'s own
 *   Constraints ("Use read-only exploration; ... write-capable work
 *   requires {{.SpawnIdiom}}"). Deliberately carries **no** `ws__*` bridge
 *   tool names: an explore leaf's rendered prompt (no `role:` frontmatter)
 *   never gets a scoped child session key spliced in, so any `ws__*` call
 *   from a recon worker would silently fall back to the bridge's own
 *   default-filled key instead of a scoped one.
 * - `full-worker`: everything, plus the live `ws__*` bridge tool names
 *   (passed in by the caller, not hardcoded, so this group tracks ws-mcp's
 *   actual registered tool set instead of drifting from it).
 */
export const TOOL_GROUPS: Record<ToolGroup, readonly string[]> = {
  "read-only": ["read", "grep", "find", "ls"],
  recon: ["read", "grep", "find", "ls", "bash"],
  "full-worker": ["read", "bash", "edit", "write", "grep", "find", "ls"],
};

/**
 * Comma-joined `--tools` value for a curated group. `wsToolNames` (the
 * bridge's sanitized `ws__*` registered names) is only appended for
 * `full-worker` — `read-only` and `recon` are built-in-only by design (see
 * `TOOL_GROUPS` doc comment above).
 */
export function resolveTools(group: ToolGroup, wsToolNames: readonly string[] = []): string {
  const builtins = TOOL_GROUPS[group];
  const extra = group === "full-worker" ? wsToolNames : [];
  return [...builtins, ...extra].join(",");
}

/**
 * Terminal `AssistantMessage.stopReason` values
 * (docs/session-format.md#L88): `"stop" | "length" | "toolUse" | "error" |
 * "aborted"`. `"toolUse"` is NOT terminal (the agent is still working);
 * `"pending"` is stream-only and never appears in a persisted message. This
 * classification is metadata only — never the completion signal, see the
 * module doc comment above.
 */
const TERMINAL_STOP_REASONS: ReadonlySet<string> = new Set(["stop", "length", "error", "aborted"]);

export function isTerminalStopReason(reason: string | undefined): boolean {
  return reason !== undefined && TERMINAL_STOP_REASONS.has(reason);
}

export type SpawnMode = "spawn" | "continue" | "explore";

export interface BuildSpawnArgsOptions {
  mode: SpawnMode;
  /** Required unless `noSession` is true. */
  sessionPath?: string;
  /** `true` only for `explore` (ephemeral, ticket #L216-218). */
  noSession: boolean;
  /** Path to the rendered playbook prompt, passed via `--append-system-prompt`. */
  promptPath?: string;
  /** Comma-joined `--tools` allowlist, from `resolveTools()`. */
  tools?: string;
  /** `provider/id` pattern, or omitted to inherit pi's own default resolution. */
  model?: string;
  task: string;
}

/**
 * Builds the `pi` CLI argv for a spawn/continue/explore invocation, mirroring
 * the shipped reference example's flag-ordering
 * (`examples/extensions/subagent/index.ts#L300-341`):
 * `--mode json -p [--session <path> | --no-session]
 * [--append-system-prompt <path>] [--tools <list>] [--model <pattern>]
 * "<task>"`.
 *
 * `sessionPath` and `noSession` are mutually exclusive by contract (a
 * `--session <path>` continuation must never also carry `--no-session`, and
 * an `explore` leaf must never persist a session file) — passing both, or
 * neither, is a caller bug and throws rather than silently picking one.
 */
export function buildSpawnArgs(opts: BuildSpawnArgsOptions): string[] {
  if (opts.noSession && opts.sessionPath) {
    throw new Error("ws-pi-agent: buildSpawnArgs: sessionPath and noSession are mutually exclusive");
  }
  if (!opts.noSession && !opts.sessionPath) {
    throw new Error(`ws-pi-agent: buildSpawnArgs: mode "${opts.mode}" requires a sessionPath when noSession is false`);
  }

  const args: string[] = ["--mode", "json", "-p"];
  if (opts.noSession) {
    args.push("--no-session");
  } else {
    args.push("--session", opts.sessionPath as string);
  }
  if (opts.promptPath) {
    args.push("--append-system-prompt", opts.promptPath);
  }
  if (opts.tools) {
    args.push("--tools", opts.tools);
  }
  if (opts.model) {
    args.push("--model", opts.model);
  }
  args.push(opts.task);
  return args;
}

/**
 * Buffers raw child-process stdout bytes into newline-delimited
 * `AgentSessionEvent` JSON lines. Mirrors `mcp-stdio-client.ts`'s
 * `JsonRpcLineBuffer` StringDecoder-based, multibyte-safe pattern (a small
 * parallel class, not an import — this buffer parses bare NDJSON event
 * objects, not JSON-RPC envelopes) — deliberately NOT the shipped example's
 * naive per-chunk `data.toString()` split, which corrupts a UTF-8 codepoint
 * split across a `'data'` chunk boundary.
 */
export class AgentEventLineBuffer {
  private readonly decoder = new StringDecoder("utf8");
  private readonly onEvent: (evt: unknown) => void;
  private readonly onParseError?: (line: string) => void;
  private carry = "";

  // No TS constructor parameter properties — see mcp-stdio-client.ts's
  // JsonRpcLineBuffer doc comment: Node's native .ts type-stripping cannot
  // erase them (they inject an implicit assignment), so explicit field
  // declarations + explicit assignment are used instead.
  constructor(onEvent: (evt: unknown) => void, onParseError?: (line: string) => void) {
    this.onEvent = onEvent;
    this.onParseError = onParseError;
  }

  feed(chunk: Buffer): void {
    this.carry += this.decoder.write(chunk);
    const lines = this.carry.split("\n");
    this.carry = lines.pop() ?? "";
    for (const line of lines) {
      this.emitLine(line);
    }
  }

  /**
   * Flushes any final partial line left in the decoder/carry buffer once the
   * source stream has ended (e.g. a last event line with no trailing
   * newline). Safe to call at most once per buffer lifetime.
   */
  end(): void {
    const rest = this.decoder.end();
    const remainder = this.carry + rest;
    this.carry = "";
    if (remainder.length > 0) {
      this.emitLine(remainder);
    }
  }

  private emitLine(line: string): void {
    if (!line.trim()) return;
    let evt: unknown;
    try {
      evt = JSON.parse(line);
    } catch {
      this.onParseError?.(line);
      return;
    }
    this.onEvent(evt);
  }
}

// ---------------------------------------------------------------------------
// Module-state registry and async spawn/continue/wait engine.
// ---------------------------------------------------------------------------

export type AgentState = "running" | "done";

export interface AgentRecord {
  agentId: string;
  playbook: string;
  tier?: string;
  /** Absolute path to the ws-owned `--session` file; undefined for `noSession` (explore) records. */
  sessionPath?: string;
  noSession: boolean;
  /** Rendered playbook prompt path (cached, reused unchanged across `continue`). */
  systemPromptPath?: string;
  state: AgentState;
  /** Last-seen `stopReason` — metadata only, never the completion signal. */
  stopReason?: string;
  outputText: string;
  errorMessage?: string;
  exitCode: number | null;
  exitSignal: NodeJS.Signals | null;
  proc?: ChildProcess;
  /** Explore leaves self-reap from the registry once harvested by wait/exploreLeaf; workers never do. */
  selfReap: boolean;
  waiters: Array<() => void>;
}

export type AgentRegistry = Map<string, AgentRecord>;

function firstText(result: McpToolCallResult): string | undefined {
  return result.content.find((item) => item.type === "text")?.text;
}

/**
 * Resolves the `pi` invocation the same way the shipped reference example
 * does (`examples/extensions/subagent/index.ts#L249-263`): prefer
 * re-invoking the currently-running script through the current Node binary
 * (works regardless of whether `pi` is on PATH), falling back to a bare
 * `pi` command otherwise. The Bun-virtual-script branch from the reference
 * is dropped — this package only ever runs under Node (package.json's
 * `"test": "node --test"`), so that branch would be dead code here.
 */
function getPiInvocation(extraArgs: string[]): { command: string; args: string[] } {
  const currentScript = process.argv[1];
  if (currentScript && existsSync(currentScript)) {
    return { command: process.execPath, args: [currentScript, ...extraArgs] };
  }
  return { command: "pi", args: extraArgs };
}

function settleWaiters(record: AgentRecord): void {
  const waiters = record.waiters;
  record.waiters = [];
  for (const resolve of waiters) resolve();
}

function waitForDone(record: AgentRecord): Promise<void> {
  if (record.state === "done") return Promise.resolve();
  return new Promise((resolve) => record.waiters.push(resolve));
}

function handleAgentEvent(record: AgentRecord, evt: unknown): void {
  if (!evt || typeof evt !== "object") return;
  const e = evt as Record<string, unknown>;
  if (e.type !== "message_end") return;
  const msg = e.message as Record<string, unknown> | undefined;
  if (!msg || msg.role !== "assistant") return;

  if (typeof msg.stopReason === "string") {
    record.stopReason = msg.stopReason;
  }
  if (typeof msg.errorMessage === "string") {
    record.errorMessage = msg.errorMessage;
  }
  const content = Array.isArray(msg.content) ? msg.content : [];
  const text = content
    .filter((part): part is Record<string, unknown> => !!part && typeof part === "object" && part.type === "text")
    .map((part) => (typeof part.text === "string" ? part.text : ""))
    .join("");
  if (text) {
    // Last assistant message wins — this is the agent's final answer, not a
    // transcript accumulator.
    record.outputText = text;
  }
}

/**
 * Spawns the `pi` child process for `record` and wires deadlock-safe
 * stdout/stderr draining (both streams get a `'data'` listener attached
 * immediately at spawn, per `mcp-stdio-client.ts#L187-199`'s precedent).
 *
 * `state:"done"` is flipped ONLY from the child's `close` event — never from
 * an in-stream terminal `stopReason` — because the `--session` file is only
 * flush-guaranteed complete after the process has actually exited (see the
 * module doc comment). `close` (not `exit`) is used deliberately: it fires
 * after stdio streams have ended, so `record.outputText`/`stopReason` are
 * guaranteed fully drained by the time waiters are settled.
 */
function spawnPiProcess(record: AgentRecord, args: string[], cwd: string): void {
  const invocation = getPiInvocation(args);
  const proc = spawn(invocation.command, invocation.args, {
    cwd,
    stdio: ["ignore", "pipe", "pipe"],
  });
  record.proc = proc;

  const lineBuffer = new AgentEventLineBuffer(
    (evt) => handleAgentEvent(record, evt),
    (line) => console.error(`[ws-pi-agent:${record.agentId}] failed to parse event line: ${line}`),
  );
  proc.stdout.on("data", (chunk: Buffer) => lineBuffer.feed(chunk));
  proc.stderr.on("data", (chunk: Buffer) => {
    console.error(`[ws-pi-agent:${record.agentId}] ${chunk.toString().trimEnd()}`);
  });

  // Spawn-level failure (bad command, missing binary, permissions). Without
  // a listener this becomes an unhandled exception instead of settling the
  // record — mirrors mcp-stdio-client.ts's McpStdioClient 'error' handling.
  proc.on("error", (err) => {
    record.errorMessage = `pi process failed to start: ${err.message}`;
  });

  proc.on("close", (code, signal) => {
    lineBuffer.end();
    record.exitCode = code;
    record.exitSignal = signal;
    record.state = "done";
    settleWaiters(record);
  });
}

export interface SpawnAgentParams {
  playbook: string;
  task: string;
  tier?: string;
}

export interface AgentCallCtx {
  /** The calling (lead) session's session_key, used for `playbook.render`. */
  sessionKey: string;
  cwd: string;
  /** `provider/id`, forwarded from the calling tool-execute ctx.model, or undefined to inherit pi's own default. */
  model?: string;
  /** Bridge's sanitized `ws__*` registered tool names, for the `full-worker` group. */
  wsToolNames: readonly string[];
}

/**
 * Renders `params.playbook` via the already-bridged `playbook.render` (which
 * itself materializes the prompt to a worktree-scoped temp file and, for a
 * role-bearing playbook called by a lead-scoped session_key, mints and
 * splices in a fresh child session key — see
 * `ai-docs/spec/mcp-tools.md#L1871-1896` / `#L2179-2186`), then spawns a
 * `pi --mode json -p --session <ws-owned-path>` child process and registers
 * it as a `state:"running"` entry.
 */
export async function spawnAgent(
  client: McpStdioClient,
  registry: AgentRegistry,
  ctx: AgentCallCtx,
  params: SpawnAgentParams,
): Promise<{ agentId: string; state: AgentState }> {
  const renderResult = await client.callTool("playbook.render", {
    session_key: ctx.sessionKey,
    name: params.playbook,
  });
  const text = firstText(renderResult);
  if (renderResult.isError || !text) {
    throw new Error(`ws-pi-agent: playbook.render("${params.playbook}") failed: ${text ?? "no content returned"}`);
  }
  const systemPromptPath = text.split("\n")[0]?.trim();
  if (!systemPromptPath) {
    throw new Error(`ws-pi-agent: playbook.render("${params.playbook}") returned no prompt path`);
  }

  const agentId = randomUUID();
  const sessionDir = mkdtempSync(join(tmpdir(), "ws-pi-agent-"));
  const sessionPath = join(sessionDir, "session.jsonl");

  const record: AgentRecord = {
    agentId,
    playbook: params.playbook,
    tier: params.tier,
    sessionPath,
    noSession: false,
    systemPromptPath,
    state: "running",
    outputText: "",
    exitCode: null,
    exitSignal: null,
    selfReap: false,
    waiters: [],
  };
  registry.set(agentId, record);

  const args = buildSpawnArgs({
    mode: "spawn",
    sessionPath,
    noSession: false,
    promptPath: systemPromptPath,
    tools: resolveTools("full-worker", ctx.wsToolNames),
    model: ctx.model,
    task: params.task,
  });
  spawnPiProcess(record, args, ctx.cwd);

  return { agentId, state: record.state };
}

/**
 * Resumes a `state:"done"` worker's session file with a follow-up task, via
 * a fresh `pi --session <same-path>` process. Reuses the cached
 * `systemPromptPath` unchanged — no re-render — so the append-system-prompt
 * text does not duplicate across resumes.
 */
export async function continueAgent(
  registry: AgentRegistry,
  ctx: Omit<AgentCallCtx, "sessionKey">,
  agentId: string,
  task: string,
): Promise<{ agentId: string; state: AgentState }> {
  const record = registry.get(agentId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  if (record.noSession) {
    throw new Error(`ws-pi-agent: agent "${agentId}" has no persisted session (explore leaves cannot be continued)`);
  }
  if (record.state !== "done") {
    throw new Error(`ws-pi-agent: agent "${agentId}" is not done yet (state="${record.state}"); wait for completion before continuing`);
  }

  record.state = "running";
  record.exitCode = null;
  record.exitSignal = null;
  record.outputText = "";
  record.stopReason = undefined;
  record.errorMessage = undefined;

  const args = buildSpawnArgs({
    mode: "continue",
    sessionPath: record.sessionPath,
    noSession: false,
    promptPath: record.systemPromptPath,
    tools: resolveTools("full-worker", ctx.wsToolNames),
    model: ctx.model,
    task,
  });
  spawnPiProcess(record, args, ctx.cwd);

  return { agentId, state: record.state };
}

export type WaitPolicy = "any" | "all";

export interface WaitResultEntry {
  agentId: string;
  state: AgentState;
  stopReason?: string;
  output: string;
  exitCode: number | null;
  errorMessage?: string;
}

export interface WaitAgentsResult {
  done: WaitResultEntry[];
  pending: string[];
  timedOut: boolean;
}

async function raceOrAll(promises: Promise<void>[], policy: WaitPolicy): Promise<void> {
  if (policy === "any") {
    await Promise.race(promises);
  } else {
    await Promise.all(promises);
  }
}

function harvest(id: string, record: AgentRecord, registry: AgentRegistry): WaitResultEntry {
  const entry: WaitResultEntry = {
    agentId: id,
    state: record.state,
    stopReason: record.stopReason,
    output: record.outputText,
    exitCode: record.exitCode,
    errorMessage: record.errorMessage,
  };
  // Explore leaves have no continue path — reap right after the first
  // harvest instead of lingering in the registry forever.
  if (record.selfReap) {
    registry.delete(id);
  }
  return entry;
}

/**
 * Partitions `agentIds` into done/pending, waiting up to `timeoutMs` (if
 * given) for `policy` ("any" one, or "all") of them to reach `state:"done"`.
 * NEVER kills a running process on timeout (ticket #L215) — a timed-out
 * agent simply stays in the registry as `running` for a later wait/continue.
 */
export async function waitAgents(
  registry: AgentRegistry,
  agentIds: string[],
  policy: WaitPolicy,
  timeoutMs?: number,
): Promise<WaitAgentsResult> {
  const records = agentIds.map((id) => {
    const record = registry.get(id);
    if (!record) {
      throw new Error(`ws-pi-agent: unknown agentId "${id}"`);
    }
    return { id, record };
  });

  const waitPromise = raceOrAll(
    records.map(({ record }) => waitForDone(record)),
    policy,
  );

  let timedOut = false;
  if (timeoutMs && timeoutMs > 0) {
    let timer: NodeJS.Timeout | undefined;
    const timeoutPromise = new Promise<"timeout">((resolve) => {
      timer = setTimeout(() => resolve("timeout"), timeoutMs);
    });
    const result = await Promise.race([waitPromise.then(() => "done" as const), timeoutPromise]);
    clearTimeout(timer);
    timedOut = result === "timeout";
  } else {
    await waitPromise;
  }

  const done: WaitResultEntry[] = [];
  const pending: string[] = [];
  for (const { id, record } of records) {
    if (record.state === "done") {
      done.push(harvest(id, record, registry));
    } else {
      pending.push(id);
    }
  }
  return { done, pending, timedOut };
}

export interface ExploreParams {
  query: string;
  /**
   * `false`/omitted (default): block until the leaf finishes and return its
   * output directly. `true`: register a running entry and return
   * immediately, for the caller to harvest later via `ws-agent-wait`
   * alongside other in-flight agents.
   */
  async?: boolean;
}

/**
 * Thin one-shot `explore` preset: fixed `playbook: "explore"`,
 * `--tools=recon`, `--no-session`, no continuation. The leaf self-reaps from
 * the registry once its output has been harvested — either synchronously
 * here (default) or later via `ws-agent-wait` (when `async: true`).
 */
export async function exploreLeaf(
  client: McpStdioClient,
  registry: AgentRegistry,
  ctx: Omit<AgentCallCtx, "wsToolNames">,
  params: ExploreParams,
): Promise<{ agentId: string; state: AgentState; output?: string; stopReason?: string }> {
  const renderResult = await client.callTool("playbook.render", {
    session_key: ctx.sessionKey,
    name: "explore",
  });
  const text = firstText(renderResult);
  if (renderResult.isError || !text) {
    throw new Error(`ws-pi-agent: playbook.render("explore") failed: ${text ?? "no content returned"}`);
  }
  const systemPromptPath = text.split("\n")[0]?.trim();

  const agentId = randomUUID();
  const record: AgentRecord = {
    agentId,
    playbook: "explore",
    noSession: true,
    systemPromptPath,
    state: "running",
    outputText: "",
    exitCode: null,
    exitSignal: null,
    selfReap: true,
    waiters: [],
  };
  registry.set(agentId, record);

  const args = buildSpawnArgs({
    mode: "explore",
    noSession: true,
    promptPath: systemPromptPath,
    tools: resolveTools("recon"),
    model: ctx.model,
    task: params.query,
  });
  spawnPiProcess(record, args, ctx.cwd);

  if (params.async) {
    return { agentId, state: record.state };
  }

  await waitForDone(record);
  const harvested = harvest(agentId, record, registry);
  return { agentId, state: "done", output: harvested.output, stopReason: harvested.stopReason };
}

// ---------------------------------------------------------------------------
// Pi tool registration.
// ---------------------------------------------------------------------------

export interface AgentToolsHandle {
  /** Best-effort SIGTERM over any still-`running` registry entries (session_shutdown cleanup). */
  killRunning(): void;
}

/**
 * Registers the four delegation-spawner tools (`ws-agent-spawn`,
 * `ws-agent-continue`, `ws-agent-wait`, `explore`) against the extension's
 * own module-state registry, reusing the bridge's already-connected
 * `McpStdioClient` and default session key (threaded out via `BridgeHandle`)
 * rather than opening a second ws-mcp connection.
 *
 * MVP depth is 0->1 leaf: none of these four tools are themselves part of
 * any `TOOL_GROUPS` entry, so a depth-1 worker/leaf spawned through them
 * never receives a nested-spawn tool in its own `--tools` allowlist even
 * though its own `pi` process loads this same extension.
 */
export function registerAgentTools(pi: ExtensionAPI, bridge: BridgeHandle, sessionCtx: { cwd: string }): AgentToolsHandle {
  const registry: AgentRegistry = new Map();

  function resolveModel(toolCtx: unknown): string | undefined {
    const model = (toolCtx as { model?: { provider?: string; id?: string } } | undefined)?.model;
    return model?.provider && model?.id ? `${model.provider}/${model.id}` : undefined;
  }

  pi.registerTool({
    name: "ws-agent-spawn",
    label: "ws-agent-spawn",
    description:
      "Spawn a delegated pi subagent as a background process, rendering `playbook` via ws/playbook.render as its system-prompt append. Returns immediately with a running registry entry; harvest with ws-agent-wait.",
    parameters: {
      type: "object",
      properties: {
        playbook: { type: "string", description: 'Playbook stem to render, e.g. "implementer".' },
        task: { type: "string", description: "Task text passed as the spawned process's initial prompt." },
        tier: { type: "string", description: "Advisory tier hint; not yet resolved to a model (Phase 3 scope)." },
      },
      required: ["playbook", "task"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as SpawnAgentParams;
      const result = await spawnAgent(
        bridge.client,
        registry,
        {
          sessionKey: bridge.defaultSessionKeyRef.current ?? "",
          cwd: sessionCtx.cwd,
          model: resolveModel(toolCtx),
          wsToolNames: bridge.wsToolNames,
        },
        p,
      );
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "ws-agent-continue",
    label: "ws-agent-continue",
    description: "Resume a done agent's session with a follow-up task, reusing its cached system prompt and session file.",
    parameters: {
      type: "object",
      properties: {
        agentId: { type: "string", description: "agentId returned by ws-agent-spawn." },
        task: { type: "string", description: "Follow-up task text for the resumed session." },
      },
      required: ["agentId", "task"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as { agentId: string; task: string };
      const result = await continueAgent(
        registry,
        { cwd: sessionCtx.cwd, model: resolveModel(toolCtx), wsToolNames: bridge.wsToolNames },
        p.agentId,
        p.task,
      );
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "ws-agent-wait",
    label: "ws-agent-wait",
    description:
      "Wait for any or all of the given agentIds to finish (gated on their process exit, not merely an in-stream stopReason). Never kills a running process on timeout.",
    parameters: {
      type: "object",
      properties: {
        "agent-ids": { type: "array", items: { type: "string" }, description: "agentIds to wait on." },
        policy: { type: "string", enum: ["any", "all"], description: 'Wait for "any" one, or "all", of the given agents.' },
        timeout: { type: "number", description: "Optional timeout in milliseconds." },
      },
      required: ["agent-ids", "policy"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { "agent-ids": string[]; policy: WaitPolicy; timeout?: number };
      const result = await waitAgents(registry, p["agent-ids"], p.policy, p.timeout);
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "explore",
    label: "explore",
    description:
      "One-shot read-only exploration leaf: answers a single scoped question via the explore playbook with the recon tool group, no session persisted, no continuation.",
    parameters: {
      type: "object",
      properties: {
        query: { type: "string", description: "One-shot exploration question." },
        async: {
          type: "boolean",
          description: "When true, returns immediately with a running registry entry for ws-agent-wait instead of blocking until done.",
        },
      },
      required: ["query"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as ExploreParams;
      const result = await exploreLeaf(
        bridge.client,
        registry,
        { sessionKey: bridge.defaultSessionKeyRef.current ?? "", cwd: sessionCtx.cwd, model: resolveModel(toolCtx) },
        p,
      );
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  return {
    killRunning() {
      for (const record of registry.values()) {
        if (record.state === "running" && record.proc && !record.proc.killed) {
          try {
            record.proc.kill();
          } catch {
            // best effort
          }
        }
      }
    },
  };
}
