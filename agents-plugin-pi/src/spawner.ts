/**
 * Self-built delegation spawner: `ws-agent-spawn` / `ws-agent-send` /
 * `ws-agent-wait` / `ws-agent-list` / `ws-agent-stop` / `explore`.
 *
 * Phase 1 replaces the one-shot `pi --mode json -p` worker spawner with
 * persistent `RpcClient` (`--mode rpc`) children: `ws-agent-spawn` starts a
 * long-lived `pi` subprocess wired through `@earendil-works/pi-coding-agent`'s
 * `RpcClient`, `ws-agent-send` drives it (prompt/followUp/steer, branching on
 * locally-tracked streaming state — see the doc comment on `sendToAgent`),
 * `ws-agent-wait` races `agent_settled` events across a set of agent ids,
 * `ws-agent-list` reports live/idle/dormant status, and `ws-agent-stop`
 * gracefully stops a child's process while keeping its `agent_id` ->
 * session/prompt/model mapping registered for a later auto-resume (D-C).
 * `ws-agent-continue` folds into `ws-agent-send` (an id with no live
 * `RpcClient` is "dormant," not a separate tool).
 *
 * The spawn tool carries **no tier** parameter (D-A / Shape A): the caller
 * (the lead) passes an already-rendered `system_prompt_path` — this module
 * never calls `playbook.render` itself for the RPC-backed path — plus an
 * optional `model_name` resolved against the adapter-owned model-catalog
 * alias table (model-catalog.ts), or omits it to inherit the parent
 * session's model.
 *
 * `explore` is the one exception left untouched: it remains the one-shot
 * `pi --mode json -p --no-session --tools=recon` recon leaf from Phases 2-3
 * (`spawnPiProcess`/`AgentEventLineBuffer`/`handleAgentEvent`/`waitForDone`
 * below), self-reaping, non-recursive (depth <= 2: lead -> worker ->
 * explore-leaf; explore cannot spawn explore, since none of the five
 * `ws-agent-*` tools are in `TOOL_GROUPS`). Only its implicit model
 * resolution switches from the old tier lookup to the reframed alias lookup
 * (still keyed on the fixed name `"small"`).
 *
 * `--tools` per-spawn group curation (`read-only`/`recon`/`full-worker`) is
 * retained unchanged — zero on-disk agent-profile files, curation lives in
 * the in-memory `TOOL_GROUPS` table below plus `pi` CLI flags.
 */

import { spawn, type ChildProcess } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { randomUUID } from "node:crypto";
import { StringDecoder } from "node:string_decoder";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { RpcClient, type RpcClientOptions } from "@earendil-works/pi-coding-agent";
import type { McpStdioClient, McpToolCallResult } from "./mcp-stdio-client.ts";
import type { BridgeHandle } from "./bridge.ts";
import { readModelCatalog, resolveAlias, type ModelCatalogConfig } from "./model-catalog.ts";

// ---------------------------------------------------------------------------
// Pure helpers: tool-group resolution, terminal-stopReason classification,
// spawn-arg building. Unit-tested directly (test/spawner.test.ts) with no
// subprocess involved. Shared by both the one-shot `explore` path and the
// RPC-backed path below.
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
 * - `full-worker`: everything, plus the literal `"explore"` custom tool name
 *   and the live `ws__*` bridge tool names (the latter passed in by the
 *   caller, not hardcoded, so this group tracks ws-mcp's actual registered
 *   tool set instead of drifting from it). `explore` is included as a fixed
 *   built-in here (not sourced from `wsToolNames`) because it is a
 *   pi-native `pi.registerTool()` custom tool, not a `ws__*` bridge tool —
 *   Pi's `--tools` allowlist filters "built-in, extension, and custom"
 *   tools alike, so omitting the literal name would silently strip it even
 *   though it is registered. Deliberately excludes every `ws-agent-*`
 *   driving/spawn tool (D-B): a worker can spawn `explore` but never
 *   another worker, so nested spawn depth never exceeds lead -> worker ->
 *   explore-leaf — depth-safe because `explore`'s own `recon` group
 *   excludes both `explore` and every `ws-agent-*` name.
 */
export const TOOL_GROUPS: Record<ToolGroup, readonly string[]> = {
  "read-only": ["read", "grep", "find", "ls"],
  recon: ["read", "grep", "find", "ls", "bash"],
  "full-worker": ["read", "bash", "edit", "write", "grep", "find", "ls", "explore"],
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
 * classification is metadata only, used by the one-shot `explore` path's
 * event handling — never the completion signal, see `handleAgentEvent`'s
 * doc comment below.
 */
const TERMINAL_STOP_REASONS: ReadonlySet<string> = new Set(["stop", "length", "error", "aborted"]);

export function isTerminalStopReason(reason: string | undefined): boolean {
  return reason !== undefined && TERMINAL_STOP_REASONS.has(reason);
}

export type SpawnMode = "explore";

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
 * Builds the `pi` CLI argv for a one-shot `explore` invocation, mirroring
 * the shipped reference example's flag-ordering
 * (`examples/extensions/subagent/index.ts#L300-341`):
 * `--mode json -p [--session <path> | --no-session]
 * [--append-system-prompt <path>] [--tools <list>] [--model <pattern>]
 * "<task>"`.
 *
 * `sessionPath` and `noSession` are mutually exclusive by contract (an
 * `explore` leaf must never persist a session file) — passing both, or
 * neither, is a caller bug and throws rather than silently picking one.
 * Phase 1 narrows `SpawnMode` to `"explore"` only: the RPC-backed
 * spawn/send path builds its `pi` argv directly via `RpcClientOptions.args`
 * (see `buildRpcClientOptions` below), not through this helper.
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
 * split across a `'data'` chunk boundary. Used only by the one-shot
 * `explore` path; the RPC-backed path gets its events from `RpcClient`'s
 * own `onEvent()`.
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

/**
 * Pure alias-first, inherit-fallback model resolution: when `alias` is
 * given and mapped in `config`, that `provider/id` wins; otherwise (no
 * `alias`, catalog unset, or that specific alias unmapped) falls back to
 * `inheritModel` unchanged. Split out from the IO wrapper that re-reads the
 * catalog file fresh per call so the resolution rule itself is directly
 * unit-testable with no file I/O involved. Used by both the RPC-backed
 * `spawnAgent`'s `model_name` and `explore`'s fixed `"small"` lookup.
 */
export function resolveModelForAlias(
  config: ModelCatalogConfig | undefined,
  alias: string | undefined,
  inheritModel: string | undefined,
): string | undefined {
  if (alias) {
    const aliasModel = resolveAlias(config, alias);
    if (aliasModel) return aliasModel;
  }
  return inheritModel;
}

// ---------------------------------------------------------------------------
// One-shot `explore` machinery (unchanged from Phase 2-3, only its model
// resolution call switches to the reframed alias lookup).
// ---------------------------------------------------------------------------

export type AgentState = "running" | "done";

export interface AgentRecord {
  agentId: string;
  playbook: string;
  /** Absolute path to the ws-owned `--session` file; undefined for `noSession` (explore) records. */
  sessionPath?: string;
  noSession: boolean;
  /** Rendered playbook prompt path (cached, reused unchanged across resumes). */
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
 * `"test": "node --test"`), so that branch would be dead code here. Used
 * only by the one-shot `explore` path; the RPC-backed path always passes
 * `cliPath: process.argv[1]` directly with no fallback (see
 * `RPC_CLI_PATH` below) since `RpcClient` itself has no bare-`pi` fallback.
 */
function getPiInvocation(extraArgs: string[]): { command: string; args: string[] } {
  const currentScript = process.argv[1];
  if (currentScript && existsSync(currentScript)) {
    return { command: process.execPath, args: [currentScript, ...extraArgs] };
  }
  return { command: "pi", args: extraArgs };
}

/**
 * Resolves every pending one-shot waiter on `record` (its `waiters` array is
 * drained and each resolved). Generic over any record shape carrying a
 * `waiters: Array<() => void>` field so it is reusable verbatim by both the
 * one-shot `explore` registry (`AgentRecord`) and the RPC-backed registry
 * (`RpcAgentRecord`) below.
 */
function settleWaiters<T extends { waiters: Array<() => void> }>(record: T): void {
  const waiters = record.waiters;
  record.waiters = [];
  for (const resolve of waiters) resolve();
}

function waitForDone(record: AgentRecord): Promise<void> {
  if (record.state === "done") return Promise.resolve();
  return new Promise((resolve) => record.waiters.push(resolve));
}

/**
 * Extracts `stopReason`/`errorMessage`/final text from a `message_end`
 * event onto `record`. Deliberately NEVER touches `record.state` — the
 * load-bearing invariant is that `state:"done"` is flipped only by the
 * child's `close` event, never by observing a terminal `stopReason` in the
 * stream (the `--session` file is only flush-guaranteed complete after the
 * process exits). Exported so that invariant has direct unit coverage
 * (test/spawner.test.ts) instead of only being reachable through a live
 * subprocess.
 */
export function handleAgentEvent(record: AgentRecord, evt: unknown): void {
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
 * flush-guaranteed complete after the process has actually exited. `close`
 * (not `exit`) is used deliberately: it fires after stdio streams have
 * ended, so `record.outputText`/`stopReason` are guaranteed fully drained by
 * the time waiters are settled.
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
  // Explicitly flips state:"done" and settles waiters here rather than
  // relying on Node also emitting 'close' after 'error' (a dual-emit
  // assumption) — 'close' still fires afterward in the common case and
  // re-applies the same done/settled state, which is idempotent (settleWaiters
  // drains an already-empty waiters array; state/exitCode get overwritten
  // with equivalent values).
  proc.on("error", (err) => {
    record.errorMessage = `pi process failed to start: ${err.message}`;
    record.state = "done";
    settleWaiters(record);
  });

  proc.on("close", (code, signal) => {
    lineBuffer.end();
    record.exitCode = code;
    record.exitSignal = signal;
    record.state = "done";
    settleWaiters(record);
  });
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

export interface ExploreParams {
  query: string;
  /**
   * `false`/omitted (default): block until the leaf finishes and return its
   * output directly. `true`: register a running entry and return
   * immediately, for the caller to harvest later.
   */
  async?: boolean;
}

function harvestExplore(id: string, record: AgentRecord, registry: AgentRegistry): { output: string; stopReason?: string } {
  const entry = { output: record.outputText, stopReason: record.stopReason };
  // Explore leaves have no continue path — reap right after the first
  // harvest instead of lingering in the registry forever.
  if (record.selfReap) {
    registry.delete(id);
  }
  return entry;
}

/**
 * Thin one-shot `explore` preset: fixed `playbook: "explore"`,
 * `--tools=recon`, `--no-session`, no continuation. The leaf self-reaps from
 * the registry once its output has been harvested — synchronously here by
 * default (Phase 1 drops the shared `ws-agent-wait` integration this used to
 * lean on for `async: true`, since that tool now only tracks the RPC-backed
 * registry below; `async: true` still registers the running entry, it just
 * has no dedicated harvesting tool of its own in this phase).
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
  if (!systemPromptPath) {
    throw new Error('ws-pi-agent: playbook.render("explore") returned no prompt path');
  }

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
  const harvested = harvestExplore(agentId, record, registry);
  return { agentId, state: "done", output: harvested.output, stopReason: harvested.stopReason };
}

// ---------------------------------------------------------------------------
// RPC-backed persistent-child engine (`ws-agent-spawn` / `ws-agent-send` /
// `ws-agent-wait` / `ws-agent-list` / `ws-agent-stop`).
// ---------------------------------------------------------------------------

/**
 * Every `new RpcClient(...)` construction passes `cliPath: process.argv[1]`
 * explicitly — spawn and resume-from-dormant alike. `RpcClient.start()` in
 * the installed `@earendil-works/pi-coding-agent` package always does
 * `spawn("node", [cliPath, ...args])` with `cliPath = this.options.cliPath
 * ?? "dist/cli.js"`; there is no bare-`pi` fallback inside `RpcClient`
 * itself (unlike this module's own `getPiInvocation()` used by the one-shot
 * `explore` path above). If `process.argv[1]` is ever missing/non-existent
 * there is no client-side fallback path — this is the ticket's settled
 * answer, carried forward as-is, not something to re-derive.
 */
const RPC_CLI_PATH = process.argv[1];

export interface RpcAgentRecord {
  agentId: string;
  /** The live RPC child, or `undefined` when dormant (stopped but resumable — D-C). */
  client?: RpcClient;
  /** Absolute path to the ws-owned `--session` file, reused unchanged across every (re)start. */
  sessionPath: string;
  /** Lead-rendered playbook prompt path, passed via `--append-system-prompt`; reused unchanged across resumes (no re-render). */
  systemPromptPath: string;
  /** Resolved `provider/id`, or undefined to inherit pi's own default resolution. Cached so a dormant resume reuses the same model. */
  modelBase?: string;
  /** Caller-supplied thinking level, applied via `setThinkingLevel()` after every (re)start. */
  modelEffort?: string;
  /** Cached bridge `ws__*` tool names, for `--tools` re-resolution on a dormant resume. */
  wsToolNames: readonly string[];
  /** `true` while an agent run is actively looping (between `agent_start` and `agent_settled`). */
  streaming: boolean;
  /** Edge-consume flag: set by the `agent_settled` listener, cleared by whichever `ws-agent-wait` call harvests it first. */
  idlePending: boolean;
  waiters: Array<() => void>;
  /** Last-seen final assistant text, cached across `getLastAssistantText()` calls. */
  lastText?: string;
  /** Detaches the current `client.onEvent(...)` listener; re-armed on every (re)start. */
  unsubscribe?: () => void;
}

export type RpcAgentRegistry = Map<string, RpcAgentRecord>;

export type AgentStatus = "running" | "idle" | "dormant";

export interface SpawnAgentParams {
  systemPromptPath: string;
  prompt: string;
  modelName?: string;
  modelEffort?: string;
}

export interface RpcSpawnCtx {
  cwd: string;
  /** `provider/id`, forwarded from the calling tool-execute ctx.model, or undefined to inherit pi's own default. */
  inheritModel?: string;
  /** Bridge's sanitized `ws__*` registered tool names, for the `full-worker` group. */
  wsToolNames: readonly string[];
  /** Path to the adapter-owned model-catalog data file, read fresh per spawn. */
  modelCatalogPath: string;
}

export interface RpcResumeCtx {
  cwd: string;
}

function buildRpcClientOptions(
  cwd: string,
  model: string | undefined,
  sessionPath: string,
  systemPromptPath: string,
  tools: string,
): RpcClientOptions {
  return {
    cliPath: RPC_CLI_PATH,
    cwd,
    model,
    args: ["--session", sessionPath, "--append-system-prompt", systemPromptPath, "--tools", tools],
  };
}

/**
 * Applies an `agent_start`/`agent_settled` RPC event onto `record`'s
 * locally-tracked streaming state. Exported so the idle-edge-consume waiter
 * logic (`ws-agent-send`'s prompt-vs-followUp/steer branch, `ws-agent-wait`'s
 * `idlePending` fast path) has direct unit coverage without a real
 * `RpcClient` subprocess — mirrors `handleAgentEvent`'s test-injection
 * pattern for the one-shot `explore` path above. All other event types are
 * ignored here; they only matter to a live streaming UI, not to this
 * module's idle/running bookkeeping.
 */
export function applyRpcEvent(record: RpcAgentRecord, evt: { type?: string }): void {
  if (evt.type === "agent_start") {
    record.streaming = true;
  } else if (evt.type === "agent_settled") {
    record.streaming = false;
    record.idlePending = true;
    settleWaiters(record);
  }
}

function attachEventListener(record: RpcAgentRecord, client: RpcClient): void {
  record.unsubscribe = client.onEvent((evt) => applyRpcEvent(record, evt as { type?: string }));
}

/**
 * Best-effort `setThinkingLevel()` after `start()`: Pi has no separate
 * `--reasoning-effort` CLI launch flag, so `model_effort` is applied as a
 * live post-start RPC call instead (decoupled from whether `model_name` was
 * also given). Never hard-fails — an unsupported/unrecognized level string
 * degrades to a no-op, matching the ticket's own fallback wording (Out of
 * Scope: validating against Pi's exact `ThinkingLevel` enum is not this
 * phase's job; the caller's string is forwarded as-is).
 */
async function applyModelEffort(client: RpcClient, modelEffort: string | undefined): Promise<void> {
  if (!modelEffort) return;
  try {
    await client.setThinkingLevel(modelEffort as Parameters<RpcClient["setThinkingLevel"]>[0]);
  } catch {
    // never-hard-fail — see doc comment above.
  }
}

/**
 * Spawns a persistent `RpcClient` child from an already-rendered system
 * prompt file. Unlike the Phase 2-3 spawner, this performs **no**
 * `playbook.render` call itself (D-A): the caller (the lead) renders the
 * playbook and passes the resulting path directly as `systemPromptPath`.
 *
 * `modelBase` resolves `model_name`-first (against the alias table), falling
 * back to `ctx.inheritModel` unchanged when `model_name` is unset or
 * unmapped — same shape as `resolveModelForAlias` (and the old tier-based
 * `resolveModelForTier` it replaces).
 *
 * Returns as soon as `client.prompt()` has sent the initial message — that
 * call only awaits transmission, not full-run completion (docs/rpc.md) — so
 * this satisfies the ticket's "returns immediately" contract while still
 * surfacing a synchronous spawn-time failure (bad `cliPath`, provider auth)
 * to the caller instead of swallowing it.
 */
export async function spawnAgent(registry: RpcAgentRegistry, ctx: RpcSpawnCtx, params: SpawnAgentParams): Promise<{ agent_id: string }> {
  const agentId = randomUUID();
  const sessionDir = mkdtempSync(join(tmpdir(), "ws-pi-agent-"));
  const sessionPath = join(sessionDir, "session.jsonl");

  const catalogConfig = params.modelName ? readModelCatalog(ctx.modelCatalogPath) : undefined;
  const modelBase = resolveModelForAlias(catalogConfig, params.modelName, ctx.inheritModel);

  const record: RpcAgentRecord = {
    agentId,
    sessionPath,
    systemPromptPath: params.systemPromptPath,
    modelBase,
    modelEffort: params.modelEffort,
    wsToolNames: ctx.wsToolNames,
    streaming: false,
    idlePending: false,
    waiters: [],
  };
  registry.set(agentId, record);

  const client = new RpcClient(
    buildRpcClientOptions(ctx.cwd, modelBase, sessionPath, params.systemPromptPath, resolveTools("full-worker", ctx.wsToolNames)),
  );
  record.client = client;

  await client.start();
  await applyModelEffort(client, params.modelEffort);
  attachEventListener(record, client);
  await client.prompt(params.prompt);

  return { agent_id: agentId };
}

/**
 * Delivers `message` to `agentId`, branching on locally-tracked streaming
 * state — this is a real behavior gap in the ticket's literal
 * `followUp()`/`steer()` tool mapping, traced through the installed
 * package's RPC mode and agent-loop source: `followUp`/`steer` only
 * *enqueue*; the queue is drained solely inside an *active* agent-loop run.
 * A freshly-started or freshly-resumed idle client has no active run, so
 * calling `followUp()`/`steer()` against it would silently queue a message
 * that is never delivered. So:
 *
 * - Dormant (`!record.client`): rebuild a fresh `RpcClient` against the
 *   SAME cached session/prompt/model (auto-resume, D-C), clear any latched
 *   `idlePending` from a prior run, then deliver via `prompt()` regardless
 *   of `interrupt` — nothing is running yet to interrupt or queue behind.
 *   This is also where D-C's "auto-resumed child on the SAME ws
 *   session_key" lineage falls out for free: the reused `systemPromptPath`
 *   already has any session key spliced in by the lead's own prior
 *   `playbook.render` call, and passing it unchanged via
 *   `--append-system-prompt` on every relaunch never re-derives or
 *   duplicates it.
 * - Live and idle (including the instant after this function's own
 *   auto-resume branch, or right after `spawnAgent`'s initial `prompt()`
 *   settles): also `prompt()`, regardless of `interrupt` — and likewise
 *   clears `idlePending` first (D-D): the PREVIOUS run's completion left it
 *   latched, and leaving it set would let a `ws-agent-wait` racing this new
 *   run busy-return the stale prior finish/last-message instead of waiting
 *   for the run this send just started.
 * - Live and streaming: `interrupt ? steer() : followUp()`, per the
 *   ticket's literal flag semantics — this is the one case where an active
 *   run actually exists for the queue to drain into. `idlePending` is
 *   already `false` here (the agent never settled since it started
 *   streaming), so there is nothing to clear.
 */
export async function sendToAgent(
  registry: RpcAgentRegistry,
  ctx: RpcResumeCtx,
  agentId: string,
  message: string,
  interrupt?: boolean,
): Promise<{ agent_id: string }> {
  const record = registry.get(agentId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }

  if (!record.client) {
    const client = new RpcClient(
      buildRpcClientOptions(ctx.cwd, record.modelBase, record.sessionPath, record.systemPromptPath, resolveTools("full-worker", record.wsToolNames)),
    );
    record.client = client;
    record.idlePending = false;
    await client.start();
    await applyModelEffort(client, record.modelEffort);
    attachEventListener(record, client);
    await client.prompt(message);
    return { agent_id: agentId };
  }

  if (record.streaming) {
    if (interrupt) {
      await record.client.steer(message);
    } else {
      await record.client.followUp(message);
    }
  } else {
    // Live-idle: starting a fresh run here must clear any latched
    // idlePending from the PREVIOUS run before prompt() fires — otherwise a
    // ws-agent-wait racing this new run would busy-return with the stale
    // finished/last_message from before this send (D-D violation). Mirrors
    // the dormant-resume branch above, which clears it for the same reason.
    record.idlePending = false;
    await record.client.prompt(message);
  }
  return { agent_id: agentId };
}

/**
 * Pure selection helper: the first entry (in caller-given order) whose
 * `idlePending` edge-consume flag is already latched, or `undefined` when
 * none is. Split out of `waitForAgents` so the "already-settled since the
 * last wait/send" fast path is directly unit-testable against fake records
 * with no async plumbing or real `RpcClient` involved.
 */
export function firstIdlePendingAgentId(records: ReadonlyArray<{ id: string; record: RpcAgentRecord }>): string | undefined {
  return records.find(({ record }) => record.idlePending)?.id;
}

export interface WaitForAgentsResult {
  agent_id?: string;
  last_message?: string;
  timed_out: boolean;
}

async function harvestLastMessage(record: RpcAgentRecord): Promise<string | undefined> {
  if (!record.client) return record.lastText;
  try {
    const text = await record.client.getLastAssistantText();
    if (text !== null && text !== undefined) {
      record.lastText = text;
    }
  } catch {
    // best effort — fall back to whatever lastText was last cached.
  }
  return record.lastText;
}

/**
 * Races `agentIds` for the first to settle (`agent_settled`), NEVER killing
 * a still-running agent on timeout — a timed-out wait simply leaves every
 * agent registered exactly as it was for a later wait/send. Drops the old
 * `policy: "any"|"all"` axis entirely (Phase 1's `ws-agent-wait(agent_ids[],
 * timeout?)` signature always behaves as first-finisher).
 *
 * An agent whose `idlePending` flag is already latched at call time (it
 * settled since the last wait/send) is harvested immediately with no race
 * at all — see `firstIdlePendingAgentId`.
 */
export async function waitForAgents(registry: RpcAgentRegistry, agentIds: string[], timeoutMs?: number): Promise<WaitForAgentsResult> {
  if (agentIds.length === 0) {
    // Racing zero promises would never settle — an empty agentIds with no
    // timeout would otherwise hang ws-agent-wait forever. Fail fast instead.
    throw new Error('ws-pi-agent: waitForAgents requires at least one agentId in "agent_ids"');
  }

  const records = agentIds.map((id) => {
    const record = registry.get(id);
    if (!record) {
      throw new Error(`ws-pi-agent: unknown agentId "${id}"`);
    }
    return { id, record };
  });

  const alreadyIdle = firstIdlePendingAgentId(records);
  if (alreadyIdle) {
    const record = registry.get(alreadyIdle) as RpcAgentRecord;
    record.idlePending = false;
    return { agent_id: alreadyIdle, last_message: await harvestLastMessage(record), timed_out: false };
  }

  // Guard: every listed record is dormant (no live client, per `stopAgent`)
  // and none has `idlePending` latched (the fast path above already would
  // have returned if so) — with no `timeoutMs`, nothing will EVER fire an
  // `agent_settled` event to resolve the race below, so this would hang
  // forever. Fail fast instead, mirroring the empty-`agentIds` guard above.
  const allDormant = records.every(({ record }) => !record.client);
  if (allDormant && !(timeoutMs && timeoutMs > 0)) {
    throw new Error(
      'ws-pi-agent: waitForAgents: every listed agentId is dormant (stopped) with no timeout given — nothing can ever settle this wait; pass a timeout or ws-agent-send one of them first to resume it',
    );
  }

  const winner = new Promise<string>((resolve) => {
    for (const { id, record } of records) {
      record.waiters.push(() => resolve(id));
    }
  });

  let winnerId: string | undefined;
  if (timeoutMs && timeoutMs > 0) {
    let timer: NodeJS.Timeout | undefined;
    const timeoutPromise = new Promise<"timeout">((resolve) => {
      timer = setTimeout(() => resolve("timeout"), timeoutMs);
    });
    const result = await Promise.race([winner, timeoutPromise]);
    clearTimeout(timer);
    if (result !== "timeout") winnerId = result;
  } else {
    winnerId = await winner;
  }

  if (!winnerId) {
    return { timed_out: true };
  }

  const winnerRecord = registry.get(winnerId) as RpcAgentRecord;
  winnerRecord.idlePending = false;
  return { agent_id: winnerId, last_message: await harvestLastMessage(winnerRecord), timed_out: false };
}

/**
 * Maps every registered agent to a `{agent_id, status}` pair: `"dormant"`
 * when there is no live client (stopped, resumable — D-C), else
 * `"running"`/`"idle"` from the locally-tracked streaming flag. Pure — no
 * IO, no RPC round trip — so directly unit-testable against fake records.
 */
export function listAgents(registry: RpcAgentRegistry): Array<{ agent_id: string; status: AgentStatus }> {
  return [...registry.entries()].map(([agentId, record]) => ({
    agent_id: agentId,
    status: record.client ? (record.streaming ? "running" : "idle") : "dormant",
  }));
}

/**
 * Best-effort graceful stop of `agentId`'s live RPC child (`abort()` then
 * `stop()`, both best-effort — a child that already exited or never
 * finished starting must not turn a routine stop into an unhandled
 * rejection). The registry entry is NEVER deleted here — per D-C, a stopped
 * agent stays registered as dormant/resumable; `ws-agent-send` auto-resumes
 * it later via the same cached session file. Throws only when `agentId`
 * itself is unknown.
 */
export async function stopAgent(registry: RpcAgentRegistry, agentId: string): Promise<{ agent_id: string }> {
  const record = registry.get(agentId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  const client = record.client;
  if (client) {
    try {
      await client.abort();
    } catch {
      // best effort
    }
    try {
      await client.stop();
    } catch {
      // best effort
    }
    record.unsubscribe?.();
    record.unsubscribe = undefined;
    record.client = undefined;
    record.streaming = false;
  }
  return { agent_id: agentId };
}

// ---------------------------------------------------------------------------
// Pi tool registration.
// ---------------------------------------------------------------------------

export interface AgentToolsHandle {
  /**
   * Graceful teardown for `session_shutdown`: awaits a best-effort
   * `client.stop()` over every live RPC-backed child (renamed from the old
   * synchronous `killRunning` — the shutdown semantics changed from a fire-
   * and-forget SIGTERM to an awaited graceful RPC stop), plus a SIGTERM of
   * any still-running one-shot `explore` process (that machinery is
   * untouched, so it keeps the old kill style).
   */
  stopAll(): Promise<void>;
}

/**
 * Registers the five RPC-backed delegation tools (`ws-agent-spawn`,
 * `ws-agent-send`, `ws-agent-wait`, `ws-agent-list`, `ws-agent-stop`) plus
 * the unchanged one-shot `explore` tool, against two separate in-extension
 * registries: `rpcRegistry` (RPC-backed, persistent children) and
 * `exploreRegistry` (one-shot, self-reaping recon leaves). They are kept
 * separate rather than unified because their completion signals are
 * fundamentally different (an `agent_settled` RPC event vs. a child
 * process's `close` event) — see `waitForAgents` vs `waitForDone`.
 *
 * MVP depth is 0->1 leaf (D-B): none of the five `ws-agent-*` tools are
 * themselves part of any `TOOL_GROUPS` entry, so a worker spawned through
 * `ws-agent-spawn` never receives a nested-spawn tool in its own `--tools`
 * allowlist even though its own `pi` process loads this same extension —
 * only the non-recursive `explore` leaf is reachable from a worker.
 */
export function registerAgentTools(
  pi: ExtensionAPI,
  bridge: BridgeHandle,
  sessionCtx: { cwd: string; modelCatalogPath: string },
): AgentToolsHandle {
  const rpcRegistry: RpcAgentRegistry = new Map();
  const exploreRegistry: AgentRegistry = new Map();

  function inheritModel(toolCtx: unknown): string | undefined {
    const model = (toolCtx as { model?: { provider?: string; id?: string } } | undefined)?.model;
    return model?.provider && model?.id ? `${model.provider}/${model.id}` : undefined;
  }

  /**
   * IO wrapper around `resolveModelForAlias` for `explore`'s implicit
   * `"small"` lookup: re-reads the model-catalog file fresh on every call
   * (no caching) so a hand-edit applies without restarting Pi, matching
   * bridge.ts's workflow_manual advisory's no-caching choice.
   */
  function resolveExploreModel(toolCtx: unknown): string | undefined {
    const config = readModelCatalog(sessionCtx.modelCatalogPath);
    return resolveModelForAlias(config, "small", inheritModel(toolCtx));
  }

  pi.registerTool({
    name: "ws-agent-spawn",
    label: "ws-agent-spawn",
    description:
      "Spawn a persistent RPC-backed pi subagent from an already-rendered system-prompt file (e.g. via ws/playbook.render). Returns {agent_id} immediately after the initial prompt is sent; harvest progress with ws-agent-wait.",
    parameters: {
      type: "object",
      properties: {
        system_prompt_path: {
          type: "string",
          description: "Path to the lead-rendered playbook prompt file, appended as the child's system prompt via --append-system-prompt.",
        },
        prompt: { type: "string", description: "Initial task prompt sent to the spawned agent." },
        model_name: {
          type: "string",
          description:
            "Optional alias name resolved against model-catalog.json's aliases map; omitted or unmapped inherits the parent session's model.",
        },
        model_effort: {
          type: "string",
          description:
            "Optional Pi thinking level (off|minimal|low|medium|high|xhigh|max), applied via setThinkingLevel after start; an unsupported value degrades to a no-op.",
        },
      },
      required: ["system_prompt_path", "prompt"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as { system_prompt_path: string; prompt: string; model_name?: string; model_effort?: string };
      const result = await spawnAgent(
        rpcRegistry,
        {
          cwd: sessionCtx.cwd,
          inheritModel: inheritModel(toolCtx),
          wsToolNames: bridge.wsToolNames,
          modelCatalogPath: sessionCtx.modelCatalogPath,
        },
        {
          systemPromptPath: p.system_prompt_path,
          prompt: p.prompt,
          modelName: p.model_name,
          modelEffort: p.model_effort,
        },
      );
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "ws-agent-send",
    label: "ws-agent-send",
    description:
      "Send a message to a spawned agent. Delivers via prompt() when idle (including immediately after auto-resuming a dormant agent); while mid-stream, interrupt:true steers it and interrupt:false/omitted queues a follow-up. A dormant (ws-agent-stop'd) agent_id is auto-resumed from its cached session file first.",
    parameters: {
      type: "object",
      properties: {
        agent_id: { type: "string", description: "agentId returned by ws-agent-spawn." },
        message: { type: "string", description: "Message text to deliver." },
        interrupt: {
          type: "boolean",
          description: "While the agent is mid-stream: true steers (interrupts) it, false/omitted queues a follow-up. Ignored while idle or dormant.",
        },
      },
      required: ["agent_id", "message"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { agent_id: string; message: string; interrupt?: boolean };
      const result = await sendToAgent(rpcRegistry, { cwd: sessionCtx.cwd }, p.agent_id, p.message, p.interrupt);
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "ws-agent-wait",
    label: "ws-agent-wait",
    description:
      "Wait for the first of the given agent_ids to settle (agent_settled), returning its agent_id and last assistant message. Never kills a running agent on timeout.",
    parameters: {
      type: "object",
      properties: {
        agent_ids: { type: "array", items: { type: "string" }, description: "agentIds to race for first-finisher." },
        timeout: { type: "number", description: "Optional timeout in milliseconds." },
      },
      required: ["agent_ids"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { agent_ids: string[]; timeout?: number };
      const result = await waitForAgents(rpcRegistry, p.agent_ids, p.timeout);
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "ws-agent-list",
    label: "ws-agent-list",
    description: "List every tracked agent_id and its status (running/idle/dormant).",
    parameters: { type: "object", properties: {} } as never,
    async execute() {
      const result = listAgents(rpcRegistry);
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "ws-agent-stop",
    label: "ws-agent-stop",
    description:
      "Gracefully stop a spawned agent's live RPC process. It stays registered as dormant/resumable — ws-agent-send auto-resumes it later via its cached session file.",
    parameters: {
      type: "object",
      properties: { agent_id: { type: "string", description: "agentId returned by ws-agent-spawn." } },
      required: ["agent_id"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { agent_id: string };
      const result = await stopAgent(rpcRegistry, p.agent_id);
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
          description: "When true, returns immediately with a running registry entry instead of blocking until done.",
        },
      },
      required: ["query"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as ExploreParams;
      const result = await exploreLeaf(
        bridge.client,
        exploreRegistry,
        // explore resolves implicitly through the "small" alias — no
        // caller-facing model param on ExploreParams (ticket: explore is a
        // role, not a caller-supplied alias/tier).
        { sessionKey: bridge.defaultSessionKeyRef.current ?? "", cwd: sessionCtx.cwd, model: resolveExploreModel(toolCtx) },
        p,
      );
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  return {
    async stopAll(): Promise<void> {
      const rpcStops = [...rpcRegistry.values()]
        .filter((record): record is RpcAgentRecord & { client: RpcClient } => !!record.client)
        .map((record) => record.client.stop().catch(() => {}));
      await Promise.allSettled(rpcStops);

      for (const record of exploreRegistry.values()) {
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
