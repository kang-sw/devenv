/**
 * Self-built delegation spawner: `ws-agent-spawn` / `ws-agent-send` /
 * `ws-agent-wait` / `ws-agent-list` / `ws-agent-stop` /
 * `ws-agent-transcript` / `ws-report-to-lead` / `explore`.
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
 * explore-leaf; explore cannot spawn explore, since none of the `ws-agent-*`
 * tools are in `TOOL_GROUPS`). Only its implicit model resolution switches
 * from the old tier lookup to the reframed alias lookup (still keyed on the
 * fixed name `"small"`).
 *
 * `--tools` per-spawn group curation (`read-only`/`recon`/`full-worker`) is
 * retained unchanged — zero on-disk agent-profile files, curation lives in
 * the in-memory `TOOL_GROUPS` table below plus `pi` CLI flags.
 *
 * Phase 2 adds a bounded per-agent child->lead report channel:
 * `ws-report-to-lead` (child-side, `full-worker`-only) is relayed into a
 * `RpcAgentRecord.pendingReports` FIFO (cap `REPORT_BUFFER_CAP`,
 * drop-oldest-with-marker on overflow) purely by observing the existing
 * `RpcClient.onEvent()` stream's `tool_execution_start` events — no new
 * transport (see `applyRpcEvent`'s doc comment for the full trace).
 * `ws-agent-wait` now also wakes on a report and drains the full buffer FIFO
 * on any wake (`reason: "idle" | "report" | "approval-pending"`, D-D — see
 * `harvestWinner`; the approval-pending wake, 260905, hands a lead blocked in a
 * wait back to a turn boundary so it can ws-approve a stuck child then re-wait).
 * `ws-agent-transcript` (lead-side, not in any `TOOL_GROUPS`) returns the
 * already-tracked `sessionPath` with no RPC round-trip.
 */

import { spawn, type ChildProcess } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { randomUUID } from "node:crypto";
import { StringDecoder } from "node:string_decoder";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { RpcClient, type RpcClientOptions } from "@earendil-works/pi-coding-agent";
import type { McpStdioClient, McpToolCallResult } from "./mcp-stdio-client.ts";
import type { BridgeHandle } from "./bridge.ts";
import { readModelCatalog, resolveAlias, type ModelCatalogConfig } from "./model-catalog.ts";
import { WS_PI_PARENT_SESSION_KEY_ENV, WS_PI_SPAWN_ROLE_ENV } from "./process-role.ts";

// ---------------------------------------------------------------------------
// Pure helpers: tool-group resolution, terminal-stopReason classification,
// spawn-arg building. Unit-tested directly (test/spawner.test.ts) with no
// subprocess involved. Shared by both the one-shot `explore` path and the
// RPC-backed path below.
// ---------------------------------------------------------------------------

export type ToolGroup = "read-only" | "recon" | "full-worker" | "execute-worker";

/** Sole source of truth for the child-side report tool's name, shared by `TOOL_GROUPS`, its registration, and the event-matching branch in `applyRpcEvent`. */
export const REPORT_TO_LEAD_TOOL_NAME = "ws-report-to-lead";

/**
 * Sole source of truth for the gated-exec worker tool's name (260904 Phase 1:
 * end-to-end approval gateway). Defined here rather than in
 * `execute-gateway.ts` to avoid a circular import: `TOOL_GROUPS` below and
 * `applyRpcEvent`'s new pendingApproval-observation branch both need this
 * name, and `execute-gateway.ts` (which registers the tool itself) already
 * imports plumbing FROM `spawner.ts` (`spawnAgent`, `RpcAgentRecord`, etc) —
 * the reverse import would be circular. Mirrors `REPORT_TO_LEAD_TOOL_NAME`'s
 * placement for the same reason.
 */
export const GATED_EXEC_TOOL_NAME = "ws-worker-exec";

/**
 * Spawn-time env var carrying the child's own approvals directory
 * (`<sessionDir>/approvals`, see `buildRpcClientOptions` below) — the only
 * channel `ws-worker-exec`'s `execute()` has to learn where to poll for its
 * decision file, since neither `sessionDir` nor `sessionPath` is otherwise
 * passed to the child process. Parallel to `WS_PI_SPAWN_ROLE_ENV`
 * (`process-role.ts`), but kept here (not there) because it is spawner-owned
 * plumbing specific to the RPC-backed path, not a role marker every spawn
 * kind needs.
 */
export const WS_PI_APPROVAL_DIR_ENV = "WS_PI_APPROVAL_DIR";

/**
 * Pure env-builder for the one-shot `explore` path's `spawn(...)` call
 * (`spawnPiProcess` below): merges the `explore` process-role marker
 * (`WS_PI_SPAWN_ROLE_ENV`, see `process-role.ts`) over `baseEnv` without
 * dropping any inherited variable. Extracted (review fix, cycle 1) so the
 * marker-placement logic is unit-testable without invoking a real child
 * process — mirrors `buildRpcClientOptions`'s export for the same reason on
 * the RPC path.
 *
 * 260904 Phase 1: `WS_PI_SPAWN_ROLE_ENV` subsumes the old boolean
 * `WS_PI_AGENT_CHILD_ENV` marker (a role value instead of a single "is a
 * child" flag) — goal-loop.ts's `isChildProcess` now reads presence of ANY
 * role via `readSpawnRole`/`process-role.ts` instead of equality to `"1"`,
 * so this rename does not change that consuming contract.
 */
export function buildChildProcessEnv(baseEnv: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  return { ...baseEnv, [WS_PI_SPAWN_ROLE_ENV]: "explore" };
}

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
 * - `full-worker`: everything, plus the literal `"explore"` and
 *   `"ws-report-to-lead"` custom tool names and the live `ws__*` bridge tool
 *   names (the latter passed in by the caller, not hardcoded, so this group
 *   tracks ws-mcp's actual registered tool set instead of drifting from it).
 *   `explore` and `ws-report-to-lead` are included as fixed built-ins here
 *   (not sourced from `wsToolNames`) because both are pi-native
 *   `pi.registerTool()` custom tools, not `ws__*` bridge tools — Pi's
 *   `--tools` allowlist filters "built-in, extension, and custom" tools
 *   alike, so omitting either literal name would silently strip it even
 *   though it is registered. Deliberately excludes every `ws-agent-*`
 *   driving/spawn tool (D-B): a worker can spawn `explore` but never
 *   another worker, so nested spawn depth never exceeds lead -> worker ->
 *   explore-leaf — depth-safe because `explore`'s own `recon` group
 *   excludes `explore`, `ws-report-to-lead`, and every `ws-agent-*` name.
 *   `ws-report-to-lead` is added only here, per the ticket's "the only
 *   child-side tool ADDED by this ticket" — `recon`/`read-only` are
 *   untouched.
 * - `execute-worker` (260904 Phase 1): the ticket's §5 "mutation-incapable
 *   read family" worker spawned by `ws-execute` — reuses the exact
 *   `read-only` builtins array (`READ_ONLY_BUILTINS` below), never
 *   re-derived, plus the gated-exec tool (`GATED_EXEC_TOOL_NAME` — every
 *   free-form shell command, including a "read" that mutates via
 *   redirection/`-exec`, must go through it and pause for lead approval),
 *   `ws-report-to-lead` (progress updates), and `explore` (scoped read-only
 *   sub-questions). Deliberately excludes `bash`/`edit`/`write` — those would
 *   let the worker bypass the approval gate entirely.
 */
const READ_ONLY_BUILTINS: readonly string[] = ["read", "grep", "find", "ls"];

export const TOOL_GROUPS: Record<ToolGroup, readonly string[]> = {
  "read-only": READ_ONLY_BUILTINS,
  recon: ["read", "grep", "find", "ls", "bash"],
  "full-worker": ["read", "bash", "edit", "write", "grep", "find", "ls", "explore", REPORT_TO_LEAD_TOOL_NAME],
  "execute-worker": [...READ_ONLY_BUILTINS, GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME, "explore"],
};

/**
 * Comma-joined `--tools` value for a curated group. `wsToolNames` (the
 * bridge's sanitized `ws__*` registered names) is only appended for
 * `full-worker` — `read-only`, `recon`, and `execute-worker` are
 * built-in/custom-only by design (see `TOOL_GROUPS` doc comment above); an
 * execute-worker never gets a scoped child session key spliced into its
 * prompt (same reasoning as `recon`), so it never calls a `ws__*` bridge tool
 * either.
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

/**
 * Pure extraction of the calling tool-execute `toolCtx`'s current model as a
 * `provider/id` string, or `undefined` when absent/malformed. Exported
 * (260904 Phase 1) so `execute-gateway.ts`'s `ws-execute` tool can resolve
 * its own inherit-fallback model the same way `ws-agent-spawn`/`explore`
 * already do here, without duplicating this shape-tolerant extraction.
 */
export function inheritModelFromToolCtx(toolCtx: unknown): string | undefined {
  const model = (toolCtx as { model?: { provider?: string; id?: string } } | undefined)?.model;
  return model?.provider && model?.id ? `${model.provider}/${model.id}` : undefined;
}

/**
 * Review fix (relay #1, TEST finding #3): pure extraction of `spawnAgent`'s
 * `ctx.toolGroup ?? "full-worker"` default — previously inlined directly in
 * `spawnAgent`, leaving no seam a unit test could exercise independent of a
 * real `RpcClient` spawn. `RpcSpawnCtx.toolGroup`'s own doc comment already
 * states the contract this codifies: "Omitted (or explicit `"full-worker"`)
 * preserves every existing `ws-agent-spawn` caller's behavior unchanged."
 * Mirrors `inheritModelFromToolCtx`'s own extraction-for-testability shape
 * immediately above.
 */
export function resolveSpawnToolGroup(explicit: ToolGroup | undefined): ToolGroup {
  return explicit ?? "full-worker";
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
    env: buildChildProcessEnv(process.env),
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
  /** Curated `--tools` group this record was spawned with; reused unchanged on a dormant resume so `resolveTools` never silently widens/narrows a resumed child's tool surface. Set at spawn (`ctx.toolGroup ?? "full-worker"`), never mutated afterward. */
  toolGroup: ToolGroup;
  /**
   * 260904 Phase 1 (side-thread fork): a pre-computed `--tools` value that
   * bypasses `resolveTools(toolGroup, wsToolNames)` entirely when set —
   * a fork's tool surface is dynamic (`computeForkToolSurface` over the
   * lead's own `pi.getActiveTools()` at spawn time, `fork.ts`), not one of
   * the static `TOOL_GROUPS` entries `toolGroup` indexes. Cached verbatim at
   * spawn and reused unchanged on every dormant resume (mirrors
   * `systemPromptPath`/`modelBase`'s existing cache-and-reuse contract).
   * `undefined` for every non-fork spawn — those keep resolving tools from
   * `toolGroup`/`wsToolNames` exactly as before.
   */
  explicitTools?: string;
  /** `true` while an agent run is actively looping (between `agent_start` and `agent_settled`). */
  streaming: boolean;
  /** Edge-consume flag: set by the `agent_settled` listener, cleared by whichever `ws-agent-wait` call harvests it first. */
  idlePending: boolean;
  waiters: Array<() => void>;
  /** Last-seen final assistant text, cached across `getLastAssistantText()` calls. */
  lastText?: string;
  /** Detaches the current `client.onEvent(...)` listener; re-armed on every (re)start. */
  unsubscribe?: () => void;
  /** FIFO buffer of undrained `ws-report-to-lead` messages, capped at `REPORT_BUFFER_CAP` (drop-oldest). */
  pendingReports: AgentReport[];
  /** Count of reports dropped from `pendingReports` due to overflow since the last drain. */
  reportsDropped: number;
  /**
   * 260904 Phase 1: set by `applyRpcEvent` the instant a `tool_execution_start`
   * for `GATED_EXEC_TOOL_NAME` is observed on this record's child; cleared by
   * `ws-approve` once a decision is written. `undefined` means "no gated
   * command is currently awaiting lead approval on this agent" — the
   * condition `validatePendingApproval` (execute-gateway.ts) rejects against.
   *
   * Review fix (relay #1): also carries `cwd`, captured from the gated-exec
   * tool call's own `args.cwd` override when the worker supplied one (the
   * same `ws-worker-exec` `cwd?` param `execute-gateway.ts`'s `execute()`
   * itself falls back on via `p.cwd ?? sessionCtx.cwd`) — the approval-relay
   * callback (`createApprovalRelay`) must scrape the SAME directory the
   * command will actually run in, not unconditionally the worker's base
   * `sessionCtx.cwd`, or a `cwd`-overridden command's ground-truth git
   * context (branch/dirty/ahead_behind) would silently describe the wrong
   * directory to the lead.
   */
  pendingApproval?: { cmdId: string; command: string; rationale?: string; cwd?: string };
}

/**
 * A single buffered `ws-report-to-lead` message. `kind` is optional
 * (260904 Phase 1, side-thread fork ticket): `"question"`/`"final"`
 * disambiguate a fork's task-thread turn (see `fork.ts`'s anti-bleed
 * predicates); existing `full-worker`/`execute-worker` callers omit it
 * entirely and are unaffected (additive, not a breaking rename of
 * `pendingReports`/`WaitForAgentsResult.reports`'s element shape from a bare
 * `string` — every existing consumer reads `.message` instead of the string
 * directly).
 */
export interface AgentReport {
  message: string;
  kind?: "question" | "final";
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
  /** Curated `--tools` group for this spawn. Omitted (or explicit `"full-worker"`) preserves every existing `ws-agent-spawn` caller's behavior unchanged — only `ws-execute` (execute-gateway.ts) passes `"execute-worker"`. */
  toolGroup?: ToolGroup;
  /**
   * 260904 Phase 1 (side-thread fork): the lead's own session file to fork
   * from (`toolCtx.sessionManager.getSessionFile()`, `fork.ts`'s
   * `registerFork`). When set, `buildRpcClientOptions` emits `--fork
   * <forkFrom>` instead of `--session <sessionPath>` for THIS initial spawn
   * only — never threaded through `RpcResumeCtx`/`sendToAgent`'s
   * dormant-resume branch, which always resumes via the fork's own
   * already-discovered `sessionPath` (see `spawnAgent`'s `client.getState()`
   * overwrite below). `undefined` for every non-fork spawn (unchanged
   * `--session` behavior).
   */
  forkFrom?: string;
  /**
   * 260904 Phase 1 (side-thread fork): pre-computed `--tools` value that
   * bypasses `resolveTools(toolGroup, wsToolNames)` when set — see
   * `RpcAgentRecord.explicitTools`'s doc comment for the full rationale.
   * `undefined` for every non-fork spawn.
   */
  explicitTools?: string;
  /**
   * 260904 Phase 1 (side-thread fork): the lead's own default-filled
   * session key (`BridgeHandle.defaultSessionKeyRef.current`), forwarded to
   * `buildRpcClientOptions` so it can set `WS_PI_PARENT_SESSION_KEY_ENV` on
   * the fork child's env — the marker `normalizeSessionKey` (bridge.ts)
   * already rewrites an explicit sentinel `session_key` from. Only
   * meaningful alongside `forkFrom`; ignored otherwise.
   */
  parentSessionKey?: string;
  /**
   * 260904 Phase 1: fired right after `attachEventListener` observes a
   * freshly-set `record.pendingApproval` on this spawn's record — the
   * approval-request-relay injection hook. `spawner.ts` stays generic (no
   * `pi.sendUserMessage` import) by taking this as a plain callback;
   * `execute-gateway.ts`'s `createApprovalRelay` is the only real
   * implementation. Never fires for a non-`"execute-worker"` spawn in
   * practice, since `GATED_EXEC_TOOL_NAME` is reachable only from that
   * group's `--tools` list.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void;
}

export interface RpcResumeCtx {
  cwd: string;
  /** See `RpcSpawnCtx.onApprovalPending` — threaded through `sendToAgent`'s dormant-auto-resume branch so a resumed `execute-worker`'s approval relay keeps working. */
  onApprovalPending?: (record: RpcAgentRecord) => void;
}

/**
 * Exported (review fix, cycle 1) so `test/spawner.test.ts` can assert the
 * `WS_PI_SPAWN_ROLE_ENV` marker directly against the built options object
 * instead of leaving it covered only by a manual spot-check — `RpcClient`'s
 * own `env: {...process.env, ...this.options.env}` merge means this function
 * only needs to carry the marker itself (no `process.env` spread here).
 *
 * 260904 Phase 1: carries `WS_PI_SPAWN_ROLE_ENV: "worker"` (see
 * `process-role.ts`), replacing the old boolean `WS_PI_AGENT_CHILD_ENV: "1"`
 * marker this function used to set. Also now carries `WS_PI_APPROVAL_DIR`,
 * derived from `sessionPath`'s own directory (`dirname(sessionPath)`, the
 * same `sessionDir` `spawnAgent`/`sendToAgent` already `mkdtempSync`'d or
 * cached — no new parameter needed since the two paths are always siblings:
 * `sessionPath` is unconditionally `join(sessionDir, "session.jsonl")`).
 * Inert for a non-`"execute-worker"` spawn (nothing in its `--tools` list can
 * ever dispatch `ws-worker-exec` to read this var), so it is folded into the
 * env unconditionally rather than threaded as an extra opt-in parameter.
 *
 * 260904 Phase 1 (side-thread fork) adds the `forkFrom`/`parentSessionKey`
 * params: when `forkFrom` is given, the emitted args swap `["--session",
 * sessionPath, ...]` for `["--fork", forkFrom, ...]` (initial spawn only —
 * `sendToAgent`'s dormant-resume call site never passes `forkFrom`) and the
 * role marker becomes `"fork"` instead of `"worker"`; `parentSessionKey`
 * (only meaningful alongside `forkFrom`) additionally sets
 * `WS_PI_PARENT_SESSION_KEY_ENV` on the child's env, the marker
 * `normalizeSessionKey` (bridge.ts) already rewrites an explicit sentinel
 * `session_key` from — the fork's own bridge instance uses it to mint its
 * own lead-scope key instead of a fresh one. `--fork`'s exact composition
 * with `--mode rpc`/`--tools`/`--append-system-prompt` and its at-leaf vs.
 * before-a-message clone semantics is the ticket's own named live-
 * verification item (not resolvable offline) — this function only builds
 * the argv, it does not confirm Pi's own `--fork` behavior.
 */
export function buildRpcClientOptions(
  cwd: string,
  model: string | undefined,
  sessionPath: string,
  systemPromptPath: string,
  tools: string,
  forkFrom?: string,
  parentSessionKey?: string,
): RpcClientOptions {
  const env: Record<string, string> = {
    [WS_PI_SPAWN_ROLE_ENV]: forkFrom ? "fork" : "worker",
    [WS_PI_APPROVAL_DIR_ENV]: join(dirname(sessionPath), "approvals"),
  };
  if (forkFrom && parentSessionKey) {
    env[WS_PI_PARENT_SESSION_KEY_ENV] = parentSessionKey;
  }
  const args = forkFrom
    ? ["--fork", forkFrom, "--append-system-prompt", systemPromptPath, "--tools", tools]
    : ["--session", sessionPath, "--append-system-prompt", systemPromptPath, "--tools", tools];
  return {
    cliPath: RPC_CLI_PATH,
    cwd,
    env,
    model,
    args,
  };
}

/** Per-agent bounded FIFO cap on undrained `ws-report-to-lead` messages (drop-oldest-with-marker on overflow). */
export const REPORT_BUFFER_CAP = 32;

/**
 * Pushes `message` (optionally tagged with `kind` — 260904 Phase 1's
 * `"question"`/`"final"` disambiguation for a fork's task-thread turn, see
 * `fork.ts`) onto `record.pendingReports`, drop-oldest once the buffer
 * exceeds `REPORT_BUFFER_CAP` (incrementing `reportsDropped` as a truncation
 * marker), then settles any pending waiters — a report is a wake condition
 * exactly like `agent_settled` (reused `settleWaiters`; draining an empty
 * `waiters` array when nobody is currently waiting is already a no-op).
 *
 * `kind` is omitted from the pushed entry entirely (not stored as an
 * explicit `undefined` property) when the caller omits it, so an existing
 * `full-worker`/`execute-worker` report round-trips as `{message}` — no
 * `kind` key at all — unchanged in shape from before this field existed.
 */
export function enqueueReport(record: RpcAgentRecord, message: string, kind?: "question" | "final"): void {
  const entry: AgentReport = kind === undefined ? { message } : { message, kind };
  record.pendingReports.push(entry);
  if (record.pendingReports.length > REPORT_BUFFER_CAP) {
    record.pendingReports.shift();
    record.reportsDropped += 1;
  }
  settleWaiters(record);
}

/**
 * Edge-consume drain: swaps `record.pendingReports`/`reportsDropped` for
 * empty/zero and returns the previous values. Pure, mirrors the edge/consume
 * shape of the existing `idlePending` clear in `waitForAgents`.
 */
export function drainReports(record: RpcAgentRecord): { reports: AgentReport[]; reports_dropped: number } {
  const reports = record.pendingReports;
  const reports_dropped = record.reportsDropped;
  record.pendingReports = [];
  record.reportsDropped = 0;
  return { reports, reports_dropped };
}

/**
 * Applies an `agent_start`/`agent_settled`/`ws-report-to-lead`-tool RPC event
 * onto `record`'s locally-tracked streaming/report state. Exported so the
 * idle-edge-consume waiter logic (`ws-agent-send`'s prompt-vs-followUp/steer
 * branch, `ws-agent-wait`'s `idlePending` fast path) and the report-relay
 * branch have direct unit coverage without a real `RpcClient` subprocess —
 * mirrors `handleAgentEvent`'s test-injection pattern for the one-shot
 * `explore` path above.
 *
 * The report branch matches a raw `tool_execution_start` event (the same
 * event Pi's own extension-hook fan-out emits the instant the LLM dispatches
 * a tool call, forwarded verbatim to the parent's `RpcClient.onEvent()` — see
 * the plan's Codebase Findings for the full trace) whose `toolName` is
 * `REPORT_TO_LEAD_TOOL_NAME`; `evt.args.message`, if a string, is enqueued
 * (260904 Phase 1, side-thread fork ticket: along with `evt.args.kind` when
 * it is exactly `"question"` or `"final"` — any other value, including a
 * malformed one, is dropped, same as an absent `kind`).
 * All other event types (including a `tool_execution_start` for any other
 * tool name) are ignored here — they only matter to a live streaming UI or
 * to the tool's own execution, not to this module's bookkeeping.
 *
 * 260904 Phase 1 (execute-approve gateway) adds a 2nd `tool_execution_start` branch, matched on
 * `GATED_EXEC_TOOL_NAME`: sets `record.pendingApproval` from the event's
 * `toolCallId` (this IS the `cmd_id` — no new id needs minting) plus the
 * gated-exec tool's own `{command, rationale}` args. Requires both a string
 * `toolCallId` and a string `args.command`; a missing/malformed event is
 * silently ignored (never throws) — matches the report branch's own
 * best-effort shape-tolerance above.
 *
 * Review fix (relay #1): also captures `args.cwd` (the same optional
 * per-call working-directory override `ws-worker-exec`'s own `execute()`
 * accepts) onto `record.pendingApproval.cwd` when it is a string — omitted
 * (`undefined`) otherwise, so a caller falls back to the worker's base cwd
 * exactly as `execute-gateway.ts`'s `execute()` itself does.
 */
export function applyRpcEvent(record: RpcAgentRecord, evt: { type?: string; toolName?: string; args?: unknown; toolCallId?: string }): void {
  if (evt.type === "agent_start") {
    record.streaming = true;
  } else if (evt.type === "agent_settled") {
    record.streaming = false;
    record.idlePending = true;
    settleWaiters(record);
  } else if (evt.type === "tool_execution_start" && evt.toolName === REPORT_TO_LEAD_TOOL_NAME) {
    const args = evt.args as { message?: unknown; kind?: unknown } | undefined;
    const message = args?.message;
    if (typeof message === "string") {
      const kind = args?.kind === "question" || args?.kind === "final" ? args.kind : undefined;
      enqueueReport(record, message, kind);
    }
  } else if (evt.type === "tool_execution_start" && evt.toolName === GATED_EXEC_TOOL_NAME) {
    const args = evt.args as { command?: unknown; rationale?: unknown; cwd?: unknown } | undefined;
    const command = args?.command;
    if (typeof command === "string" && typeof evt.toolCallId === "string") {
      record.pendingApproval = {
        cmdId: evt.toolCallId,
        command,
        rationale: typeof args?.rationale === "string" ? args.rationale : undefined,
        cwd: typeof args?.cwd === "string" ? args.cwd : undefined,
      };
      // 260905 (approval-relay deadlock fix): wake any lead already blocked in
      // ws-agent-wait on this agent, exactly like the agent_settled/report
      // branches above. Without this the lead stays blocked, and the
      // turn-boundary-only approval steer queues behind the unfinished wait
      // turn — a circular deadlock broken only by the wait timeout.
      settleWaiters(record);
    }
  }
}

/**
 * Wires `client.onEvent()` into `applyRpcEvent` for `record`, then — 260904
 * Phase 1 — additionally invokes `onApprovalPending(record)` right after any
 * `tool_execution_start` for `GATED_EXEC_TOOL_NAME` (the same event
 * `applyRpcEvent` just used to set `record.pendingApproval`). Kept as a
 * second, independent check on the raw event (not a "did pendingApproval
 * change" diff) so this function stays a thin, generic wire-up: `spawner.ts`
 * never imports `pi.sendUserMessage` itself (golden-rule-adjacent — keeps the
 * approval-relay's actual injection behavior owned entirely by
 * `execute-gateway.ts`, which supplies the real callback).
 */
function attachEventListener(record: RpcAgentRecord, client: RpcClient, onApprovalPending?: (record: RpcAgentRecord) => void): void {
  record.unsubscribe = client.onEvent((evt) => {
    const e = evt as { type?: string; toolName?: string; args?: unknown; toolCallId?: string };
    applyRpcEvent(record, e);
    if (onApprovalPending && e.type === "tool_execution_start" && e.toolName === GATED_EXEC_TOOL_NAME) {
      onApprovalPending(record);
    }
  });
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
 *
 * 260904 Phase 1 (side-thread fork): when `ctx.forkFrom` is set, the
 * mkdtemp'd `sessionPath` computed below is only a PLACEHOLDER (it still
 * backs the approvals-dir derivation `buildRpcClientOptions` folds into the
 * child's env) — `pi --fork <forkFrom>` has Pi itself create/name the real
 * forked session file, discoverable only after `client.start()` via
 * `client.getState().sessionFile`. `record.sessionPath` is overwritten with
 * that real path immediately after `start()` resolves (before any event
 * listener/model-effort/prompt call), so every downstream consumer
 * (`getAgentTranscriptPath`, a dormant resume's own `--session
 * record.sessionPath`) sees the actual forked file, never the placeholder.
 * Throws — never silently degrades — when `getState()` returns no
 * `sessionFile`, mirroring the codebase's existing never-silently-degrade
 * convention (e.g. `bridge.ts`'s ferrule-mint failure handling).
 */
export async function spawnAgent(registry: RpcAgentRegistry, ctx: RpcSpawnCtx, params: SpawnAgentParams): Promise<{ agent_id: string }> {
  const agentId = randomUUID();
  const sessionDir = mkdtempSync(join(tmpdir(), "ws-pi-agent-"));
  const sessionPath = join(sessionDir, "session.jsonl");

  const catalogConfig = params.modelName ? readModelCatalog(ctx.modelCatalogPath) : undefined;
  const modelBase = resolveModelForAlias(catalogConfig, params.modelName, ctx.inheritModel);

  const toolGroup: ToolGroup = resolveSpawnToolGroup(ctx.toolGroup);
  const tools = ctx.explicitTools ?? resolveTools(toolGroup, ctx.wsToolNames);
  const record: RpcAgentRecord = {
    agentId,
    sessionPath,
    systemPromptPath: params.systemPromptPath,
    modelBase,
    modelEffort: params.modelEffort,
    wsToolNames: ctx.wsToolNames,
    toolGroup,
    explicitTools: ctx.explicitTools,
    streaming: false,
    idlePending: false,
    waiters: [],
    pendingReports: [],
    reportsDropped: 0,
  };
  registry.set(agentId, record);

  const client = new RpcClient(
    buildRpcClientOptions(ctx.cwd, modelBase, sessionPath, params.systemPromptPath, tools, ctx.forkFrom, ctx.parentSessionKey),
  );
  record.client = client;

  await client.start();

  if (ctx.forkFrom) {
    const state = await client.getState();
    const forkedSessionFile = state?.sessionFile;
    if (!forkedSessionFile) {
      throw new Error(
        "ws-pi-agent: fork spawn: RpcClient.getState() returned no sessionFile — cannot determine the forked session's actual path",
      );
    }
    record.sessionPath = forkedSessionFile;
  }

  await applyModelEffort(client, params.modelEffort);
  attachEventListener(record, client, ctx.onApprovalPending);
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
    // 260904 Phase 1 (side-thread fork): `forkFrom` is deliberately never
    // passed here — a dormant resume (including a stopped fork) always
    // resumes via `--session record.sessionPath` (the fork's own
    // already-discovered real session file, see `spawnAgent`'s
    // `getState()` overwrite), exactly like a normal worker resume.
    // `record.explicitTools` (when set) is reused verbatim, same
    // cache-and-reuse contract as `systemPromptPath`/`modelBase`.
    const client = new RpcClient(
      buildRpcClientOptions(
        ctx.cwd,
        record.modelBase,
        record.sessionPath,
        record.systemPromptPath,
        record.explicitTools ?? resolveTools(record.toolGroup, record.wsToolNames),
      ),
    );
    record.client = client;
    record.idlePending = false;
    await client.start();
    await applyModelEffort(client, record.modelEffort);
    attachEventListener(record, client, ctx.onApprovalPending);
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

/**
 * Pure selection helper mirroring `firstIdlePendingAgentId`: the first entry
 * (in caller-given order) whose `pendingApproval` is set, or `undefined` when
 * none is (260905). Unlike `idlePending` this is not an edge-consume flag —
 * it is real state cleared by `ws-approve`, so a re-wait before approving
 * correctly re-selects the same agent.
 */
export function firstPendingApprovalAgentId(records: ReadonlyArray<{ id: string; record: RpcAgentRecord }>): string | undefined {
  return records.find(({ record }) => record.pendingApproval)?.id;
}

/**
 * Pure selection helper mirroring `firstIdlePendingAgentId`'s shape: the
 * first entry (in caller-given order) with at least one undrained buffered
 * report, or `undefined` when none has any. A dormant record can still carry
 * an undrained report from before it stopped, so this must be checked
 * independently of `client`/`idlePending` state.
 */
export function firstReportPendingAgentId(records: ReadonlyArray<{ id: string; record: RpcAgentRecord }>): string | undefined {
  return records.find(({ record }) => record.pendingReports.length > 0)?.id;
}

export interface WaitForAgentsResult {
  agent_id?: string;
  last_message?: string;
  /** Present only on a non-timeout harvest: "idle" when the agent settled, "report" when only a buffered report woke the wait, "approval-pending" when the agent is blocked awaiting a lead approval (260905). */
  reason?: "idle" | "report" | "approval-pending";
  /** Present only on reason:"approval-pending": the gated command the agent is blocked on. The lead calls ws-approve with this cmd_id, then re-waits to harvest the eventual report. */
  pending_approval?: { cmd_id: string; command: string; rationale?: string };
  /** Buffered `ws-report-to-lead` messages drained for the woken agent, FIFO order. Always present ([] when nothing was harvested, e.g. on timeout). */
  reports: AgentReport[];
  /** Count of reports dropped from the buffer due to overflow since the last drain. Always present (0 when nothing was harvested). */
  reports_dropped: number;
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
 * Harvests a woken/already-pending winner: idle takes priority over a
 * same-agent buffered report when both are true at harvest time (an
 * implementation-level tie-break, not a ticket ambiguity — see the plan) —
 * `reason: "idle"` is reported, but any buffered reports are still drained
 * and returned either way (D-D: a waking lead sees the full report queue
 * regardless of what triggered the wake).
 */
async function harvestWinner(record: RpcAgentRecord, agentId: string): Promise<WaitForAgentsResult> {
  // 260905: a pending approval takes priority — the agent is actively blocked
  // waiting for the lead, so surface it first so the lead can ws-approve and
  // unblock it. Not edge-consumed: record.pendingApproval is cleared by
  // ws-approve itself (execute-gateway.ts), so a re-wait before approving
  // correctly re-reports approval-pending rather than looping forever. Reports
  // are still drained (mirroring the idle branch) so a report buffered before
  // the gate is not stranded. An agent cannot be simultaneously mid-gated-call
  // and idlePending, so this never races the idle branch on the same record.
  if (record.pendingApproval) {
    const { cmdId, command, rationale } = record.pendingApproval;
    const drained = drainReports(record);
    return {
      agent_id: agentId,
      reason: "approval-pending",
      pending_approval: { cmd_id: cmdId, command, rationale },
      ...drained,
      timed_out: false,
    };
  }
  if (record.idlePending) {
    record.idlePending = false;
    const drained = drainReports(record);
    return { agent_id: agentId, reason: "idle", last_message: await harvestLastMessage(record), ...drained, timed_out: false };
  }
  const drained = drainReports(record);
  return { agent_id: agentId, reason: "report", ...drained, timed_out: false };
}

/**
 * Races `agentIds` for the first to settle (`agent_settled`) or report
 * (`ws-report-to-lead`), NEVER killing a still-running agent on timeout — a
 * timed-out wait simply leaves every agent registered exactly as it was for
 * a later wait/send. Drops the old `policy: "any"|"all"` axis entirely
 * (Phase 1's `ws-agent-wait(agent_ids[], timeout?)` signature always behaves
 * as first-finisher).
 *
 * An agent whose `idlePending` flag is already latched at call time (it
 * settled since the last wait/send) is harvested immediately with no race
 * at all — see `firstIdlePendingAgentId`. Likewise, an agent already holding
 * an undrained buffered report (even a dormant one — see
 * `firstReportPendingAgentId`) is harvested immediately with `reason:
 * "report"`.
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

  // 260905: an agent already blocked on a lead approval is the most urgent
  // fast path — it is holding up a child and (unlike idle/report) cannot make
  // any progress until the lead acts. Checked before idle/report; the three
  // states never co-occur on one record (a mid-gated-call agent is neither
  // settled nor, for that call, reporting), so ordering only picks among
  // distinct agents, and unblocking a stuck child first is the right choice.
  const alreadyPendingApproval = firstPendingApprovalAgentId(records);
  if (alreadyPendingApproval) {
    return harvestWinner(registry.get(alreadyPendingApproval) as RpcAgentRecord, alreadyPendingApproval);
  }

  const alreadyIdle = firstIdlePendingAgentId(records);
  if (alreadyIdle) {
    return harvestWinner(registry.get(alreadyIdle) as RpcAgentRecord, alreadyIdle);
  }

  // A dormant record can still carry an undrained report from before it
  // stopped; this must resolve before the `allDormant` hang-guard below
  // would otherwise (incorrectly) refuse the wait.
  const alreadyReported = firstReportPendingAgentId(records);
  if (alreadyReported) {
    return harvestWinner(registry.get(alreadyReported) as RpcAgentRecord, alreadyReported);
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
    return { timed_out: true, reports: [], reports_dropped: 0 };
  }

  return harvestWinner(registry.get(winnerId) as RpcAgentRecord, winnerId);
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

/**
 * Lead-side introspection accessor (same family as `ws-agent-list`/
 * `ws-agent-stop`, not a driving/spawn tool): returns the absolute path to
 * `agentId`'s Pi session JSONL, unchanged since `spawnAgent` first computed
 * it. No RPC round-trip, no content marshalling — the lead greps the file
 * directly. Throws when `agentId` is unknown, same message convention as
 * `sendToAgent`/`stopAgent`.
 */
export function getAgentTranscriptPath(registry: RpcAgentRegistry, agentId: string): { transcript_path: string } {
  const record = registry.get(agentId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  return { transcript_path: record.sessionPath };
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
  /**
   * 260904 Phase 1: the same RPC-backed registry `ws-agent-*` already reads
   * and writes, exposed so `execute-gateway.ts`'s `ws-execute`/`ws-approve`
   * tools can spawn/inspect agents on ONE shared map — §4's "agent_id
   * disambiguates among all live and dormant/retained agents" requires this,
   * not a second parallel registry.
   */
  rpcRegistry: RpcAgentRegistry;
}

/**
 * Registers the seven RPC-backed delegation tools (`ws-agent-spawn`,
 * `ws-agent-send`, `ws-agent-wait`, `ws-agent-list`, `ws-agent-stop`,
 * `ws-agent-transcript`, `ws-report-to-lead`) plus the unchanged one-shot
 * `explore` tool, against two separate in-extension registries: `rpcRegistry`
 * (RPC-backed, persistent children) and `exploreRegistry` (one-shot,
 * self-reaping recon leaves). They are kept separate rather than unified
 * because their completion signals are fundamentally different (an
 * `agent_settled` RPC event vs. a child process's `close` event) — see
 * `waitForAgents` vs `waitForDone`.
 *
 * Phase 2 adds a child->lead report channel: `ws-report-to-lead` is the only
 * child-side tool this ticket adds (registered here but reachable only from
 * a worker's `full-worker` `--tools` allowlist, per `TOOL_GROUPS`); its
 * `execute()` is a no-op ack — the relay to the parent's per-agent
 * `pendingReports` buffer rides the existing `RpcClient.onEvent()` wire via
 * `applyRpcEvent`'s new `tool_execution_start` branch, not the tool's return
 * value (see that function's doc comment). `ws-agent-transcript` is a
 * lead-side introspection tool (same family as `ws-agent-list`/
 * `ws-agent-stop`), never added to any `TOOL_GROUPS` entry, so it is not
 * reachable from a worker's own `--tools`.
 *
 * MVP depth is 0->1 leaf (D-B): none of the `ws-agent-*` tools are
 * themselves part of any `TOOL_GROUPS` entry, so a worker spawned through
 * `ws-agent-spawn` never receives a nested-spawn tool in its own `--tools`
 * allowlist even though its own `pi` process loads this same extension —
 * only the non-recursive `explore` leaf (and now `ws-report-to-lead`, a
 * non-spawning report call) is reachable from a worker.
 */
export function registerAgentTools(
  pi: ExtensionAPI,
  bridge: BridgeHandle,
  sessionCtx: { cwd: string; modelCatalogPath: string },
  /**
   * 260904 Phase 1: see `RpcSpawnCtx.onApprovalPending`'s doc comment.
   * Threaded into both `ws-agent-spawn`'s `spawnAgent` call and
   * `ws-agent-send`'s `sendToAgent` call (dormant-resume branch) so a
   * `ws-execute`-spawned `execute-worker` keeps its approval relay wired even
   * if it is later driven through the generic `ws-agent-*` tools (shared
   * registry, §4). A no-op for every other `toolGroup` — `GATED_EXEC_TOOL_NAME`
   * is unreachable from `full-worker`/`recon`/`read-only`'s `--tools` lists,
   * so this callback simply never fires for them.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void,
): AgentToolsHandle {
  const rpcRegistry: RpcAgentRegistry = new Map();
  const exploreRegistry: AgentRegistry = new Map();

  /**
   * IO wrapper around `resolveModelForAlias` for `explore`'s implicit
   * `"small"` lookup: re-reads the model-catalog file fresh on every call
   * (no caching) so a hand-edit applies without restarting Pi, matching
   * bridge.ts's workflow_manual advisory's no-caching choice.
   */
  function resolveExploreModel(toolCtx: unknown): string | undefined {
    const config = readModelCatalog(sessionCtx.modelCatalogPath);
    return resolveModelForAlias(config, "small", inheritModelFromToolCtx(toolCtx));
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
          inheritModel: inheritModelFromToolCtx(toolCtx),
          wsToolNames: bridge.wsToolNames,
          modelCatalogPath: sessionCtx.modelCatalogPath,
          onApprovalPending,
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
      const result = await sendToAgent(rpcRegistry, { cwd: sessionCtx.cwd, onApprovalPending }, p.agent_id, p.message, p.interrupt);
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "ws-agent-wait",
    label: "ws-agent-wait",
    description:
      "Wait for the first of the given agent_ids to settle (agent_settled) OR report (a child calling ws-report-to-lead) OR need a lead approval, returning agent_id, reason (idle|report|approval-pending), and all buffered reports for that agent drained in FIFO order (plus reports_dropped if the buffer overflowed). On reason:idle, last_message is also included. On reason:approval-pending, pending_approval:{cmd_id,command,rationale} is included — call ws-approve with that cmd_id, then call ws-agent-wait again to harvest the result (do not keep blocking). Never kills a running agent on timeout.",
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
    name: "ws-agent-transcript",
    label: "ws-agent-transcript",
    description:
      "Lead-side introspection: returns {transcript_path}, the absolute path to a spawned agent's Pi session JSONL. No content marshalling — grep/read the file directly.",
    parameters: {
      type: "object",
      properties: { agent_id: { type: "string", description: "agentId returned by ws-agent-spawn." } },
      required: ["agent_id"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { agent_id: string };
      const result = getAgentTranscriptPath(rpcRegistry, p.agent_id);
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: REPORT_TO_LEAD_TOOL_NAME,
    label: REPORT_TO_LEAD_TOOL_NAME,
    description:
      "Surface a buffered async status update or intermediate finding to the lead immediately, distinct from your final answer (which the lead harvests separately once you settle). The lead receives this the next time it calls ws-agent-wait on you (reason: report), draining every buffered report in FIFO order.",
    parameters: {
      type: "object",
      properties: {
        message: { type: "string", description: "Status update or intermediate finding to surface to the lead immediately." },
        kind: {
          type: "string",
          enum: ["question", "final"],
          description:
            "Optional disambiguation for a task-thread fork's turn-end (ws-fork): \"question\" ends your turn awaiting the lead's input; \"final\" marks the task fully complete, using the required Outcome/Files changed/Verification/Blockers/Commit/Decisions report shape. Omit for a normal full-worker/execute-worker progress update.",
        },
      },
      required: ["message"],
    } as never,
    async execute() {
      // No-op ack: the relay to the parent's per-agent report buffer already
      // happens via the existing RpcClient.onEvent() stream (Pi emits
      // tool_execution_start the instant the LLM dispatches this call,
      // forwarded verbatim to the parent's applyRpcEvent) — see that
      // function's doc comment for the full trace. This execute() body does
      // not touch the registry.
      return { content: [{ type: "text", text: "reported" }] };
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
    rpcRegistry,
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
