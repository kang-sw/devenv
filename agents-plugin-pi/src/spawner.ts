/**
 * Self-built delegation spawner: `ws-agent-spawn` / `ws-agent-send` /
 * `ws-agent-list` / `ws-agent-stop` / `ws-agent-transcript` /
 * `ws-report-to-lead` / `explore`.
 *
 * Phase 1 replaces the one-shot `pi --mode json -p` worker spawner with
 * persistent `RpcClient` (`--mode rpc`) children: `ws-agent-spawn` starts a
 * long-lived `pi` subprocess wired through `@earendil-works/pi-coding-agent`'s
 * `RpcClient`, `ws-agent-send` drives it (prompt/followUp/steer, branching on
 * locally-tracked streaming state — see the doc comment on `sendToAgent`),
 * `ws-agent-list` reports live/idle/dormant status plus each agent's last
 * report time, and `ws-agent-stop` gracefully stops a child's process while
 * keeping its `agent_id` -> session/prompt/model mapping registered for a
 * later auto-resume (D-C). `ws-agent-continue` folds into `ws-agent-send` (an
 * id with no live `RpcClient` is "dormant," not a separate tool).
 *
 * 260905 (`260905-feat-ws-pi-push-only-child-reports` Phase 1): there is no
 * harvest tool any more. `ws-agent-wait` is DELETED outright (not deprecated),
 * along with the whole buffered-report machinery it existed to drain
 * (`pendingReports`/`idlePending`/`RpcAgentRecord.waiters`). Every child
 * signal is instead PUSHED into the owning session the instant it happens, as
 * a Pi custom message (`pi.sendMessage(..., {deliverAs, triggerTurn:true})`)
 * in one of six families — `ws-agent-report`, `ws-agent-settled`,
 * `ws-agent-question`, `ws-agent-approval`, `ws-agent-advisory`,
 * `ws-agent-orphaned` — each carrying `details.agent_id`, its own payload, and
 * a fan-in status line (`computeRunningStatusLine`) counting how many
 * delegated agents are still outstanding. The lead therefore ends its turn
 * after dispatching work and is woken by the pushes themselves; it never
 * blocks in a wait call, so an approval request can reach it mid-flight
 * instead of queueing behind an unfinished wait turn.
 *
 * A `followUp` push raised while the owning session is mid-turn is HELD
 * (`heldPushQueue`) and released on that turn's `agent_settled`, so its status
 * line is fresh as of delivery rather than as of arrival; `steer` pushes are
 * sent immediately, since interrupting is their purpose. Symmetrically, a
 * child's `kind:"final"` report is held on the CHILD's side of the same
 * boundary (`pendingFinal`) and pushed when that child's own turn ends, so
 * "done" is never announced while its author is still working.
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
 * Phase 2 adds the per-agent child->lead report channel: `ws-report-to-lead`
 * (child-side, `full-worker`-only) is observed purely from the existing
 * `RpcClient.onEvent()` stream's `tool_execution_start` events — no new
 * transport (see `applyRpcEvent`'s doc comment for the full trace) — and, as
 * of 260905, pushed straight to the lead rather than buffered. Only a bounded
 * `RpcAgentRecord.reportLog` (kind + timestamp, no text) is retained, because
 * two consumers need the history rather than the event: `fork.ts`'s
 * `isIdleWithoutFinal` check (filtered to entries since the record's last LEAD
 * prompt) and `ws-agent-list`'s last-report time.
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
import { WS_PI_PARENT_SESSION_KEY_ENV, WS_PI_SPAWN_ROLE_ENV, isLeadOrFork, readSpawnRole } from "./process-role.ts";

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
 * `waiters: Array<() => void>` field.
 *
 * 260905: now used ONLY by the one-shot `explore` registry (`AgentRecord`),
 * whose `waitForDone` genuinely blocks on child exit. The RPC-backed
 * registry no longer has waiters at all — under the push model a lead never
 * blocks on a child, so `RpcAgentRecord.waiters` and everything that drained
 * it went away with `ws-agent-wait`.
 */
function settleWaiters<T extends { waiters: Array<() => void> }>(record: T): number {
  const waiters = record.waiters;
  record.waiters = [];
  for (const resolve of waiters) resolve();
  return waiters.length;
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
// `ws-agent-list` / `ws-agent-stop` / `ws-agent-transcript`).
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
  /**
   * 260905: which spawn shape produced this record, recorded at spawn time
   * rather than re-derived from `toolGroup`/`explicitTools` heuristics. Read
   * by the shutdown sidecar (`agent-sidecar.ts`) so a `session_start` revival
   * can re-arm the right role wiring for a resurrected orphan.
   */
  spawnRole?: SpawnAgentRole;
  /** `true` while an agent run is actively looping (between `agent_start` and `agent_settled`). */
  streaming: boolean;
  /**
   * 260905 fan-in bookkeeping: `true` from the instant a prompt is ISSUED to
   * this child (`promptAgent`, i.e. before any `agent_start` event can arrive)
   * until it settles, is stopped, exits, or fails to spawn. Deliberately a
   * DIFFERENT, narrower flag than `streaming` (which is event-confirmed and
   * still the right signal for `ws-agent-list`'s display status): this one
   * exists only to feed `computeRunningStatusLine`'s running count, where a
   * just-dispatched child must already count as outstanding.
   */
  running: boolean;
  /**
   * 260905: `true` once this child has sent a `kind:"final"` or
   * `kind:"question"` report during the current turn; cleared by the next
   * `promptAgent`. Two uses: it removes the sender from N on its own push (a
   * child that just filed its final is not "still running" from the lead's
   * point of view), and it suppresses the redundant `ws-agent-settled`
   * `reason:"idle"` push that would otherwise follow the terminal report a
   * few milliseconds later.
   */
  terminalThisTurn?: boolean;
  /**
   * 260905 (Phase 1 Edition): the text of a `kind:"final"` report this child
   * filed during the current turn, stashed instead of pushed. A final is the
   * child's ANSWER, but at the instant the tool call is observed the child is
   * still mid-turn — it may still be committing, cleaning up, or (rarely)
   * filing a corrected final. Pushing then handed the lead a completion signal
   * while its author was still working. So the final is held here and pushed
   * when the child actually leaves the running state, carrying
   * `settled_reason` (`idle`/`stopped`/`exited`) to say how.
   *
   * Last one wins within a turn (a corrected final supersedes the first), and
   * `promptAgent` clears it: an un-pushed final from a previous task must not
   * surface as the answer to a new one. A hook-consumed final (a `lead-ask`
   * thread's decision) is never stashed at all — it is not the lead's message.
   */
  pendingFinal?: string;
  /**
   * 260905: epoch-ms stamp of the last LEAD-issued prompt on this record
   * (`promptAgent` with `isLeadPrompt` not `false`). An internal nudge
   * (`fork.ts`'s anti-bleed loop) deliberately does NOT move it, so a stale
   * pre-nudge `final` cannot be mistaken for a fresh one. `fork.ts` filters
   * `reportLog` by it.
   */
  lastLeadPromptAt?: number;
  /**
   * 260905: `true` for the whole lifetime of an owner discussion thread bound
   * to this agent — set by `ask.ts` on every open/reopen (`ensureRespondent`/
   * `openThread`) and on fork-raised question registration
   * (`handleForkRaisedQuestion`), cleared only when the thread actually closes
   * (`/done`, the respondent's own fork final, `ws-resolve`). While set, this
   * agent produces no settle/advisory push and is left out of the fan-in
   * status line entirely: the exchange belongs to the owner, and the lead is
   * not part of it.
   *
   * Deliberately distinct from `overlayAttached`, which is per-VIEW (an owner
   * pressing Esc clears the overlay while the thread stays bound).
   */
  threadBound?: boolean;
  /** Last-seen final assistant text, cached across `getLastAssistantText()` calls. */
  lastText?: string;
  /** Detaches the current `client.onEvent(...)` listener; re-armed on every (re)start. */
  unsubscribe?: () => void;
  /**
   * 260905: bounded history of `ws-report-to-lead` observations on this
   * record — kind and timestamp only, never the text (the text is pushed to
   * the lead immediately and never retained). Replaces the deleted
   * `pendingReports` FIFO as the input to `fork.ts`'s `isIdleWithoutFinal`
   * and as `ws-agent-list`'s last-report time. Capped at `REPORT_LOG_CAP`
   * (drop-oldest) so a long-lived chatty child cannot grow it without bound.
   */
  reportLog: AgentReportLogEntry[];
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
  /**
   * 260904 Phase 2 (side-thread question surface, review relay #1 C1): `true`
   * while an owner overlay chat VIEW is attached to this agent (`ask.ts`'s
   * `openThread` sets it, closing the view clears it).
   *
   * 260905 narrowed its role: the suppression `fork.ts`'s anti-bleed loop and
   * the push model both need is the THREAD's lifetime, not the view's — an
   * owner pressing Esc must not re-arm nudges on a still-open discussion — so
   * both now read `threadBound`. This flag stays as the per-view record
   * `ask.ts` keeps for its own overlay bookkeeping.
   */
  overlayAttached?: boolean;
  /**
   * 260904 Phase 2 (review relay #1 I6): consulted by `applyRpcEvent` the
   * instant a `kind:"question"` report is observed on this record. It may
   * return a REPLACEMENT message to enqueue for the lead in place of the
   * fork's own question text; returning `undefined` enqueues the original
   * unchanged (the headless baseline, byte-identical to Phase 1).
   *
   * §1 says the lead is not involved in a fork-raised question and §8 scopes
   * the lead relay to headless, so in TUI mode `ask.ts` registers the thread
   * on the owner surface and returns that notice — which, since 260905, is
   * read as a SUPPRESSION signal: a defined return means the question was
   * consumed there and no `ws-agent-question` push reaches the lead at all.
   * Set by `fork.ts`'s `registerFork`; `spawner.ts`
   * stays generic and supplies no implementation, mirroring
   * `onApprovalPending`'s existing callback-injection convention.
   */
  onQuestionReport?: (record: RpcAgentRecord, message: string) => string | undefined;
  /**
   * 260904 Phase 2 (post-close dogfood, 2026-09-05): consulted by
   * `applyRpcEvent` the instant a `kind:"final"` report is observed on this
   * record. A throwing hook is swallowed. `ask.ts` sets it on a thread's
   * respondent so a discussion fork can end its own thread by reporting the
   * decision; the report text is then the thread summary.
   *
   * 260905 (push model) gives it a SUPPRESSION contract: returning `true`
   * means the hook fully consumed the report, so no `ws-agent-report` push is
   * emitted for it. `ask.ts` returns `true` for a `"lead-ask"` discussion
   * thread — the owner's decision already reaches the lead as the
   * `ws-thread-summary` custom message, and pushing the raw report too would
   * deliver the same event twice — and falsy for a `"fork-raised"` task fork,
   * whose final IS the completion signal the lead is meant to see.
   */
  onFinalReport?: (record: RpcAgentRecord, message: string) => boolean | void;
  /**
   * 260905 (review relay #1, I1): fired by `sendToAgent`'s dormant-resume
   * branch right after the fresh client's event listener is attached. It
   * exists for role wiring that needs a LIVE client and therefore cannot be
   * re-armed at revival time — `fork.ts`'s `wireAntiBleedLoop`, which
   * subscribes to `client.onEvent` and returns early when the record is
   * dormant. A record revived from the shutdown sidecar carries this so its
   * first `ws-agent-send` restores the fork wiring rather than silently
   * degrading it to plain-worker behavior. Hook-only state, never serialized.
   */
  onResume?: (record: RpcAgentRecord) => void;
  /**
   * 260905 (review relay #1, I1): a per-RECORD fallback for the approval relay
   * that `RpcSpawnCtx`/`RpcResumeCtx` normally carry per CALL SITE.
   * `attachEventListener` prefers the ctx callback and falls back to this one,
   * so an `execute-worker` keeps its relay even when it is resumed from a call
   * site that has none — the shutdown sidecar's role-keyed revival sets it
   * (making the re-arm explicit rather than a coincidence of which resume path
   * ran), and `ask.ts`'s overlay channel resume, which passes no
   * `onApprovalPending` of its own, gets it for free.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void;
}

/**
 * One observed `ws-report-to-lead` call on a record: its `kind` (260904's
 * `"question"`/`"final"` fork disambiguation; absent for a plain
 * `full-worker`/`execute-worker` progress update) and the epoch-ms time it was
 * seen. The message text is deliberately NOT retained — under the push model
 * it has already been delivered to the lead by the time this entry is
 * appended.
 */
export interface AgentReportLogEntry {
  kind?: "question" | "final";
  at: number;
}

export type RpcAgentRegistry = Map<string, RpcAgentRecord>;

export type AgentStatus = "running" | "idle" | "dormant";

/**
 * 260905: the three RPC-backed spawn shapes, recorded on the record at spawn
 * time (`RpcAgentRecord.spawnRole`). Narrower than `process-role.ts`'s
 * `SpawnRole` on purpose — that one describes the CHILD process's own view of
 * itself (`worker`/`fork`/`explore`) as carried in its env, while this one is
 * the PARENT's classification of what it spawned, and must distinguish an
 * `execute-worker` (approval-gated) from a plain worker so the shutdown
 * sidecar can re-arm the right wiring on revival.
 */
export type SpawnAgentRole = "worker" | "execute-worker" | "fork";

/** Per-agent cap on retained `reportLog` entries (drop-oldest); see `RpcAgentRecord.reportLog`. */
export const REPORT_LOG_CAP = 64;

/**
 * 260905: the six push families. Each is a Pi custom-message `customType`
 * delivered by `pushToLead` into whichever session owns the child:
 *
 * - `ws-agent-report` — a `ws-report-to-lead` progress update (pushed at once)
 *   or a `kind:"final"` completion report (deferred to the end of the child's
 *   turn and then carrying `settled_reason`; see `flushPendingFinal`).
 * - `ws-agent-settled` — the child stopped producing: `reason` is `"idle"`
 *   (settled with no terminal report this turn, carrying `last_message`),
 *   `"stopped"` (an explicit `ws-agent-stop`), `"exited"` (its process died —
 *   see the liveness probe), or `"spawn-failed"`.
 * - `ws-agent-question` — a headless `kind:"question"` report the lead itself
 *   must answer (in TUI the owner surface consumes it and nothing is pushed).
 * - `ws-agent-approval` — an `execute-worker` is blocked on `ws-approve`.
 * - `ws-agent-advisory` — `fork.ts`'s anti-bleed loop has something to say
 *   about a fork's turn shape.
 * - `ws-agent-orphaned` — children that outlived their lead session and are
 *   revivable with `ws-agent-send` (shutdown sidecar, `agent-sidecar.ts`).
 */
export const PUSH_FAMILIES = [
  "ws-agent-report",
  "ws-agent-settled",
  "ws-agent-question",
  "ws-agent-approval",
  "ws-agent-advisory",
  "ws-agent-orphaned",
] as const;

export type PushFamily = (typeof PUSH_FAMILIES)[number];

/** Pi's own `sendMessage` delivery axis (`ExtensionAPI.sendMessage`'s `options.deliverAs`). */
export type PushDeliverAs = "steer" | "followUp" | "nextTurn";

/**
 * The push gate: only the process that OWNS the child pushes into its own
 * session. `isLeadOrFork` is exactly that set — the host lead and a `fork`
 * child (which runs its own `session_start` and may itself spawn workers) —
 * while a `worker`/`explore` child pushes nothing (its own reports travel to
 * ITS parent over the RPC event stream, not through a message into its own
 * transcript). Exported as a pure predicate over an explicit env so the gate
 * is unit-testable without mutating the real process env.
 */
export function shouldPushToLead(env: NodeJS.ProcessEnv = process.env): boolean {
  return isLeadOrFork(readSpawnRole(env));
}

/**
 * The fan-in status line every pushed message carries: `N delegated agents
 * still running`, computed fresh at push time over the shared registry.
 *
 * - The line is PRESENT whenever any registry member is neither dormant nor
 *   stopped/exited (`record.client` present — the one flag all three of those
 *   resting states clear) and NOT `threadBound`. An `explore` leaf is never in
 *   this registry at all (it has its own).
 * - N counts the subset of those that is still `running` and has not yet
 *   filed a `final`/`question` this turn (`terminalThisTurn`), so the agent
 *   whose own terminal report triggered this very push has already removed
 *   itself. `0 delegated agents still running` is the lead's synthesis cue.
 *
 * Presence is deliberately NOT keyed on `running`: a live but idle child is
 * none of dormant/stopped/exited, so it keeps the line present and only
 * leaves N. That is what makes the zero line reachable in the real event
 * order (each child settles moments after filing its final).
 *
 * Owner decision after the second live run (2026-09-05): the former
 * running-out-of-total form with an id suffix is gone. The total only ever
 * grew across a session (an idle child keeps its process, so it stayed counted
 * until stopped/exited) and the id suffix duplicated what `ws-agent-list`
 * already answers, so both were noise. The omission rule survives unchanged:
 * when nothing live is delegated
 * there is no line at all (`undefined`), so a push that has nothing to do with
 * delegation fan-in — a `ws-agent-orphaned` roll-call at session start, a
 * `spawn-failed` for the only child — never ends with a contentless zero line.
 */
/**
 * The shared registry walk behind both `computeRunningStatusLine` (below) and
 * the goal-loop yield predicate (`hasRunningAgents`, 260905 Phase 2): skips
 * `threadBound`/no-`client` records, and reports whether anything counts as
 * "present" (live, non-threadBound) at all plus how many of those are still
 * `running` and not `terminalThisTurn`. Extracted so the two call sites can
 * never drift apart in what they count as fan-in.
 */
function computeFanIn(registry: RpcAgentRegistry | undefined): { present: boolean; running: number } {
  let present = false;
  let running = 0;
  for (const record of registry?.values() ?? []) {
    if (record.threadBound || !record.client) continue;
    present = true;
    if (record.running && !record.terminalThisTurn) running += 1;
  }
  return { present, running };
}

export function computeRunningStatusLine(registry: RpcAgentRegistry | undefined): string | undefined {
  const { present, running } = computeFanIn(registry);
  if (!present) return undefined;
  return `${running} delegated agent${running === 1 ? "" : "s"} still running`;
}

/**
 * Phase 2 (260905) goal-loop yield predicate: true when N > 0 under the
 * same fan-in walk computeRunningStatusLine uses, so the yield decision
 * can never drift from the pushed status line's own arithmetic.
 */
export function hasRunningAgents(registry: RpcAgentRegistry | undefined): boolean {
  return computeFanIn(registry).running > 0;
}

/**
 * Renders a pushed message's human-readable `content`. `details` carries the
 * same fields structurally (that is what a renderer/tool would read); this
 * body is what the lead's model actually sees in its transcript, so it stays
 * plain text with the status line last. An absent status (nothing delegated,
 * see `computeRunningStatusLine`) contributes no line at all.
 */
export function buildPushContent(
  family: PushFamily,
  agentId: string | undefined,
  payload: Record<string, unknown>,
  status: string | undefined,
): string {
  const head = agentId ? `[${family}] agent ${agentId}` : `[${family}]`;
  const body = Object.entries(payload)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => `${key}: ${typeof value === "string" ? value : JSON.stringify(value)}`);
  return [head, ...body, ...(status ? [status] : [])].join("\n");
}

/**
 * The owning session's idleness accessor, filled from the `session_start` ctx
 * (`ctx.isIdle`) by `index.ts` — the same mutable-ref seam `wsBlockRef` uses
 * for a value only a live ctx can supply. `pushToLead` reads it to decide
 * whether a `followUp` push can go out now or must be HELD (see
 * `heldPushQueue`). Unset (a test, a headless path that never ran
 * `session_start`, a torn-down session) is deliberately treated as IDLE: the
 * pre-hold behavior of sending straight through is the safe default, since a
 * held push that nothing ever flushes would be a lost report.
 */
export const leadIdleRef: { current: (() => boolean) | undefined } = { current: undefined };

/** Whether the owning session's agent is between turns right now. See `leadIdleRef`. */
function isOwningAgentIdle(): boolean {
  const isIdle = leadIdleRef.current;
  if (!isIdle) return true;
  try {
    return isIdle() !== false;
  } catch {
    return true;
  }
}

/** One `followUp` push deferred until the owning session's current turn settles. */
interface HeldPush {
  registry: RpcAgentRegistry | undefined;
  record: RpcAgentRecord | undefined;
  family: PushFamily;
  payload: Record<string, unknown>;
}

/**
 * Pushes that arrived while the owning session was mid-turn, in arrival order.
 *
 * Phase 1 Edition (live-run fix): Pi queues a `followUp` sent mid-turn in its
 * own `PendingMessageQueue` and delivers it after the turn ends — but there is
 * no extension hook at that delivery (`before_agent_start` fires only for
 * owner-typed prompts; see `agent-session.ts`'s `prompt()` vs
 * `sendCustomMessage`), so a message built at ARRIVAL time carried a status
 * line already stale by the time the lead read it. A worker that finished
 * while the lead was still spawning its siblings reported zero still running
 * (its siblings were not registered yet), and so did the next — three
 * separate invitations to synthesize early. Holding the push here and building
 * the message at FLUSH
 * time is the fix: the status line is then computed against the registry as it
 * stands when the lead actually reads it.
 *
 * Exported for the flush handler and for test isolation only; nothing else may
 * write it. Held pushes are in-memory and die with the process, exactly like
 * the Pi queue they stand in for — `session_shutdown` drops them rather than
 * persisting them (the sidecar carries child IDENTITIES, never reports).
 */
export const heldPushQueue: HeldPush[] = [];

/** Builds and sends one push immediately, computing its status line right now. */
function sendPush(
  pi: ExtensionAPI,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord | undefined,
  family: PushFamily,
  payload: Record<string, unknown>,
  deliverAs: PushDeliverAs,
): void {
  const status = computeRunningStatusLine(registry);
  const base: Record<string, unknown> = record ? { agent_id: record.agentId, ...payload } : { ...payload };
  const details = status ? { ...base, status } : base;
  try {
    pi.sendMessage(
      {
        customType: family,
        content: buildPushContent(family, record?.agentId, payload, status),
        display: true,
        details: details as never,
      },
      { deliverAs, triggerTurn: true },
    );
  } catch {
    // Best effort: a push that cannot be delivered (a torn-down session, a
    // host that rejected the message) must never turn a child's routine
    // report into a crashed event listener.
  }
}

/**
 * Releases every held push, in arrival order, each with a status line computed
 * NOW. Called from the owning session's `agent_settled` (see
 * `registerPushFlush`): the agent is idle at that instant, so the first send
 * starts a fresh run and the rest land in Pi's own followUp queue behind it —
 * one lead run that sees all of them in order, which is what the one-at-a-time
 * drain was always meant to produce.
 *
 * The queue is drained BEFORE the first send so a push issued from inside that
 * run is held again for the next settle rather than re-entering this drain.
 */
export function flushHeldPushes(pi: ExtensionAPI | undefined): number {
  const pending = heldPushQueue.splice(0, heldPushQueue.length);
  if (!pi) return 0;
  for (const held of pending) {
    sendPush(pi, held.registry, held.record, held.family, held.payload, "followUp");
  }
  return pending.length;
}

/**
 * Arms the held-push release on the owning session's own `agent_settled`.
 * Registered at factory scope (like `registerGoalLoop`) rather than inside
 * `session_start`, so a `/reload` cannot stack duplicate handlers. The role
 * gate is re-checked at fire time for the same reason `pushToLead` checks it:
 * a `worker`/`explore` process holds nothing, so it has nothing to flush.
 */
export function registerPushFlush(pi: ExtensionAPI): void {
  pi.on("agent_settled", () => {
    if (!shouldPushToLead()) return;
    flushHeldPushes(pi);
  });
}

/**
 * Emits the deferred `kind:"final"` report for a child that has just left the
 * running state, and says whether there was one.
 *
 * Phase 1 Edition: a `final` observed mid-turn is stashed on the record
 * (`pendingFinal`) rather than pushed, because at that instant the child is
 * still working — it filed its answer through a tool call and its turn
 * continues. This is the release point, reached from every way a child can
 * stop running: the RPC `agent_settled` (`idle`), the `ws-agent-stop` tool
 * (`stopped`), and the liveness probe's exit detection (`exited`).
 * `details.settled_reason` records which, so a final released by a stop or a
 * process death is not read as an orderly completion.
 *
 * A `true` return means the caller must NOT also push `ws-agent-settled` for
 * the same transition — one child turn is one message to the lead.
 */
export function flushPendingFinal(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  settledReason: "idle" | "stopped" | "exited",
): boolean {
  const report = record.pendingFinal;
  record.pendingFinal = undefined;
  if (report === undefined) return false;
  pushToLead(pi, registry, record, "ws-agent-report", { kind: "final", report, settled_reason: settledReason }, "followUp");
  return true;
}

/**
 * Pushes one child signal into the owning session as a Pi custom message.
 * This is the whole replacement for the deleted `ws-agent-wait` harvest: the
 * lead ends its turn and is woken by these instead of blocking.
 *
 * `triggerTurn: true` is what makes an IDLE lead act on the signal at once
 * rather than leaving it queued until the owner's next prompt (the same
 * lesson `ask.ts`'s `injectDiscussionSummary` already encodes). `deliverAs`
 * is per-family: `"steer"` for the two things a lead must act on mid-turn (a
 * blocked approval, a headless question), `"followUp"` for everything else.
 *
 * A `followUp` push that arrives while the owning session is MID-TURN is not
 * sent now — it is held (`heldPushQueue`) and released on that turn's
 * `agent_settled` with its status line computed at release time. See
 * `heldPushQueue` for why: Pi's own queue would deliver the message later but
 * with an arrival-time status line, which read stale by the time the lead saw
 * it. `steer` pushes are never held; their whole point is to interrupt.
 *
 * Guarded on `shouldPushToLead` and on `pi` being present (a `sendToAgent`
 * resume path may run without one), so a worker-role process and an
 * un-threaded call site are both silent no-ops rather than errors.
 */
export function pushToLead(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord | undefined,
  family: PushFamily,
  payload: Record<string, unknown>,
  deliverAs: PushDeliverAs,
): void {
  if (!pi || !shouldPushToLead()) return;
  if (deliverAs === "followUp" && !isOwningAgentIdle()) {
    heldPushQueue.push({ registry, record, family, payload });
    return;
  }
  sendPush(pi, registry, record, family, payload, deliverAs);
}

/**
 * Single funnel for every `client.prompt(...)` call in this adapter, so the
 * fan-in bookkeeping cannot drift from the actual dispatches: marks the child
 * `running` from the instant the prompt is ISSUED (not when `agent_start`
 * arrives — a lead ending its turn immediately after dispatch must already
 * see it counted), clears the previous turn's `terminalThisTurn` and any
 * un-pushed `pendingFinal`, and stamps `lastLeadPromptAt`.
 *
 * `isLeadPrompt: false` is passed by exactly one caller — `fork.ts`'s
 * anti-bleed nudge — because an internal re-prompt is not a new task
 * boundary: moving `lastLeadPromptAt` there would hide a stale
 * idle-without-final from the very check the nudge exists to serve.
 */
export async function promptAgent(
  record: RpcAgentRecord,
  client: RpcClient,
  message: string,
  opts?: { isLeadPrompt?: boolean },
): Promise<void> {
  record.running = true;
  record.terminalThisTurn = false;
  // A final that never reached a settle belongs to the task being replaced,
  // not to the one starting now.
  record.pendingFinal = undefined;
  if (opts?.isLeadPrompt !== false) {
    record.lastLeadPromptAt = Date.now();
  }
  await client.prompt(message);
}

/**
 * Transitions `record` to the dead/stopped resting state: no client, not
 * running, not streaming, listener detached. Shared by the liveness probe,
 * the in-flight-rejection paths, and `stopAgent`, so "what a stopped record
 * looks like" is defined once.
 */
function clearLiveState(record: RpcAgentRecord): void {
  record.unsubscribe?.();
  record.unsubscribe = undefined;
  record.client = undefined;
  record.streaming = false;
  record.running = false;
}

/**
 * 260905 liveness probe. `RpcClient` exposes no public exit event, but its
 * `send()` throws synchronously once the child process has exited (the
 * bundled client sets `exitError` on the process's own `'exit'`/`'error'`),
 * so a `getState()` round-trip is a reliable liveness test: if it rejects,
 * the child is gone. Called on registry transitions (settle), on a periodic
 * timer while anything is outstanding, and implicitly by every in-flight
 * request rejection routed through `markAgentExited`.
 *
 * Returns `true` when the agent is (still) alive, `false` when this call
 * transitioned it to exited and pushed `ws-agent-settled` `reason:"exited"`.
 */
export async function probeAgentLiveness(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
): Promise<boolean> {
  const client = record.client;
  if (!client) return false;
  try {
    await client.getState();
    return true;
  } catch {
    markAgentExited(pi, registry, record);
    return false;
  }
}

/**
 * Records that `record`'s child process is gone and tells the lead once. Safe
 * to call repeatedly — a record already cleared of its client pushes nothing
 * a second time.
 */
export function markAgentExited(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
): void {
  if (!record.client) return;
  clearLiveState(record);
  // A child that filed a final and then died before settling still answered;
  // `settled_reason: "exited"` is what tells the lead the death, not silence.
  if (flushPendingFinal(pi, registry, record, "exited")) return;
  pushToLead(pi, registry, record, "ws-agent-settled", { reason: "exited" }, "followUp");
}

/** Interval of the background liveness sweep, while at least one agent is outstanding. */
export const LIVENESS_PROBE_INTERVAL_MS = 30_000;

/**
 * Starts the periodic half of the liveness probe: every
 * `LIVENESS_PROBE_INTERVAL_MS`, probe each still-`running` record. Skipped
 * entirely while nothing is outstanding (N === 0), so an idle lead pays
 * nothing. `unref()`'d so a pending sweep never holds the process open.
 * Returns the stopper (`AgentToolsHandle.stopAll` calls it).
 */
export function startLivenessProbe(
  pi: ExtensionAPI,
  registry: RpcAgentRegistry,
  intervalMs: number = LIVENESS_PROBE_INTERVAL_MS,
): () => void {
  const timer = setInterval(() => {
    for (const record of [...registry.values()]) {
      if (record.running && record.client) {
        void probeAgentLiveness(pi, registry, record);
      }
    }
  }, intervalMs);
  timer.unref?.();
  return () => clearInterval(timer);
}

/**
 * The `spawn-failed` half of `spawnAgent`'s launch-failure handling: park the
 * half-registered record in its resting state and tell the owning session
 * once, so the fan-in count is not left waiting on a child that never started.
 * The caller re-throws the original error unchanged afterwards.
 *
 * Extracted (review relay #1, test partition C2) so this branch has offline
 * coverage — `spawnAgent` itself constructs a real `RpcClient` and is
 * live-gate only.
 */
export function pushSpawnFailed(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  err: unknown,
): void {
  clearLiveState(record);
  pushToLead(pi, registry, record, "ws-agent-settled", { reason: "spawn-failed", error: err instanceof Error ? err.message : String(err) }, "followUp");
}

export interface SpawnAgentParams {
  systemPromptPath: string;
  prompt: string;
  modelName?: string;
  modelEffort?: string;
}

export interface RpcSpawnCtx {
  /**
   * 260905: the spawning session's own `ExtensionAPI`, needed so every signal
   * this child produces can be PUSHED back into that session (`pushToLead`).
   * Required rather than optional: a spawn with no push channel would leave
   * its child's reports unreachable now that `ws-agent-wait` is gone.
   */
  pi: ExtensionAPI;
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
   * approval-request-relay injection hook, now told whether a lead-side
   * waiter was already woken by this event (260904 Phase 2, re-scoped
   * 2026-09-05). `spawner.ts` stays generic (no `pi.sendUserMessage` import)
   * by taking this as a plain callback; `execute-gateway.ts`'s
   * `createApprovalRelay` is the only real implementation. Never fires for a
   * non-`"execute-worker"` spawn in practice, since `GATED_EXEC_TOOL_NAME` is
   * reachable only from that group's `--tools` list.
   *
   * 260905 dropped the `info: {waiterWoken}` argument: with `ws-agent-wait`
   * deleted there is no wait return for the relay to be a stale duplicate of,
   * so it now pushes unconditionally.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void;
  /**
   * 260905: recorded on the record as `spawnRole` so the shutdown sidecar can
   * re-arm the right wiring on revival. Defaults to `"fork"` when `forkFrom`
   * is set, `"execute-worker"` for that tool group, `"worker"` otherwise.
   */
  spawnRole?: SpawnAgentRole;
}

export interface RpcResumeCtx {
  /** See `RpcSpawnCtx.pi`. Optional here only because a resume can be driven from a call site with no push channel of its own; pushes are then skipped rather than erroring. */
  pi?: ExtensionAPI;
  cwd: string;
  /** See `RpcSpawnCtx.onApprovalPending` — threaded through `sendToAgent`'s dormant-auto-resume branch so a resumed `execute-worker`'s approval relay keeps working. */
  onApprovalPending?: (record: RpcAgentRecord) => void;
  /**
   * 260905 (review relay #1, I2): `true` only for the LEAD-facing
   * `ws-agent-send` tool. The lead driving a child directly is the headless
   * answer path for a fork-raised question — the fork was thread-bound from
   * registration and, with no owner surface to ever open or close that thread,
   * nothing else would release the bind before its own final. So a lead send
   * unbinds it: the exchange is now the lead's, and the child rejoins the
   * fan-in immediately rather than staying invisible until it completes.
   *
   * Deliberately NOT set by `ask.ts`'s overlay channel (`createForkChannel`),
   * which routes the OWNER's messages through this same function — an owner
   * typing into an open thread must leave the bind exactly as it is.
   */
  leadSend?: boolean;
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

/**
 * Appends one observed `ws-report-to-lead` call to `record.reportLog`,
 * drop-oldest past `REPORT_LOG_CAP`. `kind` is omitted from the entry
 * entirely (not stored as an explicit `undefined` property) when the caller
 * omits it, so a plain progress update round-trips as `{at}`.
 */
export function recordReport(record: RpcAgentRecord, kind: "question" | "final" | undefined, at: number = Date.now()): void {
  record.reportLog.push(kind === undefined ? { at } : { kind, at });
  if (record.reportLog.length > REPORT_LOG_CAP) {
    record.reportLog.shift();
  }
}

/**
 * The report kinds `record` has filed since its last LEAD prompt — the input
 * `fork.ts`'s `isIdleWithoutFinal` judges a turn against. Filtering by
 * `lastLeadPromptAt` is what makes a `final` from a PREVIOUS task stop
 * counting once the lead sends a new one; a nudge deliberately does not move
 * that stamp (see `promptAgent`), so it cannot un-flag a stale record.
 */
export function reportKindsSinceLeadPrompt(record: RpcAgentRecord): Array<"question" | "final" | undefined> {
  const since = record.lastLeadPromptAt ?? 0;
  return record.reportLog.filter((entry) => entry.at >= since).map((entry) => entry.kind);
}

/**
 * What `attachEventListener` must DO about an event `applyRpcEvent` just
 * applied. 260905: `applyRpcEvent` stays pure (no `pi`, no `RpcClient` — the
 * convention its existing plain-fake-record tests depend on), so it describes
 * the push instead of performing it, and the IO glue one layer up
 * (`attachEventListener`, which already read this return value to fire
 * `onApprovalPending`) turns that description into a `pi.sendMessage`.
 */
export interface RpcEventOutcome {
  /** A push to emit verbatim, already resolved against the record's suppression hooks. */
  push?: { family: PushFamily; payload: Record<string, unknown>; deliverAs: PushDeliverAs };
  /** `true` on `agent_settled`: the caller decides whether a `ws-agent-settled` push follows (it needs an async `harvestLastMessage`). */
  settled?: boolean;
}

/**
 * Applies an `agent_start`/`agent_settled`/`ws-report-to-lead`-tool RPC event
 * onto `record`'s locally-tracked streaming/report state, and returns what the
 * IO layer should push for it. Exported so the streaming/turn bookkeeping and
 * the report-classification branch have direct unit coverage without a real
 * `RpcClient` subprocess — mirrors `handleAgentEvent`'s test-injection pattern
 * for the one-shot `explore` path above.
 *
 * The report branch matches a raw `tool_execution_start` event (the same
 * event Pi's own extension-hook fan-out emits the instant the LLM dispatches
 * a tool call, forwarded verbatim to the parent's `RpcClient.onEvent()` — see
 * the plan's Codebase Findings for the full trace) whose `toolName` is
 * `REPORT_TO_LEAD_TOOL_NAME`; `evt.args.message`, if a string, becomes a push
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
 *
 * 260904 Phase 2 (side-thread question surface, review relay #1 I6): a
 * `kind:"question"` report is passed through `record.onQuestionReport` (when
 * set) first. Under the push model that hook's existing return contract IS
 * the suppression signal: a DEFINED return means a TUI owner surface has
 * taken the question (§1 keeps the lead out of that exchange entirely), so
 * nothing is pushed at all; `undefined` is the headless baseline and the
 * question is pushed as `ws-agent-question`/`steer` for the lead to answer.
 *
 * `record.onFinalReport` gained the parallel contract: returning `true` means
 * the hook fully consumed the report (a `lead-ask` discussion thread, whose
 * decision reaches the lead as its own `ws-thread-summary` message), so
 * nothing is stashed; falsy stashes it as `record.pendingFinal`.
 *
 * Phase 1 Edition: an un-consumed `final` produces NO push from here. It is
 * stashed and released by `flushPendingFinal` when the child leaves the
 * running state — see that function and `RpcAgentRecord.pendingFinal`.
 * `question` and untagged progress reports keep their immediate push: a
 * question blocks the child until it is answered, and progress is only
 * meaningful while the work is still going.
 */
export function applyRpcEvent(
  record: RpcAgentRecord,
  evt: { type?: string; toolName?: string; args?: unknown; toolCallId?: string },
): RpcEventOutcome {
  if (evt.type === "agent_start") {
    record.streaming = true;
  } else if (evt.type === "agent_settled") {
    record.streaming = false;
    // The run is over: the child stops counting toward the fan-in the instant
    // it settles, whatever the caller decides to push about it.
    record.running = false;
    return { settled: true };
  } else if (evt.type === "tool_execution_start" && evt.toolName === REPORT_TO_LEAD_TOOL_NAME) {
    const args = evt.args as { message?: unknown; kind?: unknown } | undefined;
    const message = args?.message;
    if (typeof message === "string") {
      const kind = args?.kind === "question" || args?.kind === "final" ? args.kind : undefined;
      recordReport(record, kind);
      if (kind !== undefined) {
        // A terminal report for this turn: the sender is out of N from here
        // on (including on its own push), and the `agent_settled` that
        // follows must not emit a redundant idle-settle push.
        record.terminalThisTurn = true;
      }

      if (kind === "question") {
        // A defined return means the owner surface consumed it (TUI); only
        // the headless `undefined` case reaches the lead. A throwing hook
        // degrades to the headless baseline rather than dropping the report.
        let consumed = false;
        if (record.onQuestionReport) {
          try {
            consumed = record.onQuestionReport(record, message) !== undefined;
          } catch {
            consumed = false;
          }
        }
        return consumed ? {} : { push: { family: "ws-agent-question", payload: { question: message }, deliverAs: "steer" } };
      }

      if (kind === "final") {
        let consumed = false;
        if (record.onFinalReport) {
          try {
            consumed = record.onFinalReport(record, message) === true;
          } catch {
            // swallowed: a hook failure must not drop the report
            consumed = false;
          }
        }
        // Phase 1 Edition: NOT pushed here. The child is still mid-turn at
        // this instant; the final is stashed and pushed when it actually
        // leaves the running state (`flushPendingFinal`), so the lead is told
        // "done" only once the author has stopped working. A consumed report
        // belongs to an owner thread and is not stashed at all.
        if (!consumed) record.pendingFinal = message;
        return {};
      }

      return { push: { family: "ws-agent-report", payload: { report: message }, deliverAs: "followUp" } };
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
      // The approval PUSH itself is `createApprovalRelay`'s job
      // (execute-gateway.ts owns the §7 payload and the working-context
      // scrape); this branch only records what is pending. Fired from
      // `attachEventListener` below via `onApprovalPending`, unconditionally
      // now that there is no wait return for it to duplicate.
      return {};
    }
  }
  return {};
}

/**
 * Wires `client.onEvent()` into `applyRpcEvent` for `record` and performs the
 * IO half of whatever it reports: emits the described push, and — on
 * `agent_settled` — decides whether an idle-settle push follows. It also,
 * since 260904 Phase 1, invokes `onApprovalPending(record)` right after any
 * `tool_execution_start` for `GATED_EXEC_TOOL_NAME` (the same event
 * `applyRpcEvent` just used to set `record.pendingApproval`). Kept as a
 * second, independent check on the raw event (not a "did pendingApproval
 * change" diff) so the approval-relay's actual payload/injection behavior
 * stays owned entirely by `execute-gateway.ts`, which supplies that callback.
 *
 * The settle push is deliberately conditional and asynchronous:
 * - replaced by the deferred `kind:"final"` report when this turn filed one
 *   (`flushPendingFinal`) — that report IS the completion signal, and the
 *   settle notice would be the same event a second time;
 * - suppressed while `record.threadBound` — an owner discussion thread's turn
 *   boundaries are not the lead's business (§1);
 * - suppressed when `record.terminalThisTurn` — the child already filed a
 *   `final`/`question` this turn, and that push IS the signal; a settle
 *   notice milliseconds later would just be a duplicate wake;
 * - otherwise pushed with `last_message` from `harvestLastMessage` (the
 *   former `ws-agent-wait` `reason:"idle"` payload, reused verbatim), which
 *   needs an RPC round-trip and so cannot happen inline in the listener.
 *
 * The settle is also a registry transition, so it doubles as a liveness-probe
 * point (`probeAgentLiveness`) — a child that died mid-turn is reported as
 * `exited` rather than silently going quiet.
 *
 * Exported (review relay #1, test partition C2) purely so the suppression
 * conditions above have direct offline coverage: this listener, not the pure
 * `applyRpcEvent`, is where the `!threadBound && !terminalThisTurn` gate
 * actually lives, and `spawnAgent`/`sendToAgent` — its only production call
 * sites — both construct a real `RpcClient` and are live-gate only. Tests
 * drive it with a duck-typed `client` exposing `onEvent`/`getState`/
 * `getLastAssistantText`.
 */
export function attachEventListener(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  client: RpcClient,
  onApprovalPending?: (record: RpcAgentRecord) => void,
): void {
  record.unsubscribe = client.onEvent((evt) => {
    const e = evt as { type?: string; toolName?: string; args?: unknown; toolCallId?: string };
    const outcome = applyRpcEvent(record, e);
    if (outcome.push) {
      pushToLead(pi, registry, record, outcome.push.family, outcome.push.payload, outcome.push.deliverAs);
    }
    if (outcome.settled) {
      void (async () => {
        // The deferred final, if this turn filed one, IS the settle message.
        if (!flushPendingFinal(pi, registry, record, "idle") && !record.threadBound && !record.terminalThisTurn) {
          const lastMessage = await harvestLastMessage(record);
          pushToLead(pi, registry, record, "ws-agent-settled", { reason: "idle", last_message: lastMessage }, "followUp");
        }
        await probeAgentLiveness(pi, registry, record);
      })();
    }
    // Ctx callback first, per-record fallback second (see
    // `RpcAgentRecord.onApprovalPending`).
    const approvalHook = onApprovalPending ?? record.onApprovalPending;
    if (approvalHook && e.type === "tool_execution_start" && e.toolName === GATED_EXEC_TOOL_NAME) {
      approvalHook(record);
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
    spawnRole: ctx.spawnRole ?? (ctx.forkFrom ? "fork" : toolGroup === "execute-worker" ? "execute-worker" : "worker"),
    streaming: false,
    running: false,
    reportLog: [],
  };
  registry.set(agentId, record);

  const client = new RpcClient(
    buildRpcClientOptions(ctx.cwd, modelBase, sessionPath, params.systemPromptPath, tools, ctx.forkFrom, ctx.parentSessionKey),
  );
  record.client = client;

  // 260905: the record is registered BEFORE `start()`, so a failure anywhere
  // in the launch sequence would otherwise leave a half-registered zombie the
  // lead's fan-in count keeps waiting on. Push `spawn-failed` and re-throw
  // unchanged — the thrown error still surfaces to the `ws-agent-spawn` caller
  // exactly as before; the push is additive, for the M/N bookkeeping.
  try {
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
    attachEventListener(ctx.pi, registry, record, client, ctx.onApprovalPending);
    await promptAgent(record, client, params.prompt);
  } catch (err) {
    pushSpawnFailed(ctx.pi, registry, record, err);
    throw err;
  }

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
 *   SAME cached session/prompt/model (auto-resume, D-C), then deliver via
 *   `promptAgent()` regardless of `interrupt` — nothing is running yet to
 *   interrupt or queue behind. This is also where D-C's "auto-resumed child
 *   on the SAME ws session_key" lineage falls out for free: the reused
 *   `systemPromptPath` already has any session key spliced in by the lead's
 *   own prior `playbook.render` call, and passing it unchanged via
 *   `--append-system-prompt` on every relaunch never re-derives or
 *   duplicates it. This branch is also how an ORPHANED child from a previous
 *   lead session is revived (260905's shutdown sidecar re-registers it as a
 *   plain dormant record; `ws-agent-send` needs no special case for it).
 * - Live and idle (including the instant after this function's own
 *   auto-resume branch, or right after `spawnAgent`'s initial prompt
 *   settles): also `promptAgent()`, regardless of `interrupt`.
 * - Live and streaming: `interrupt ? steer() : followUp()`, per the
 *   ticket's literal flag semantics — this is the one case where an active
 *   run actually exists for the queue to drain into.
 *
 * 260905: every delivery goes through `promptAgent` (or re-marks `running`
 * for the steer/followUp branch), so the fan-in count reflects a send the
 * moment it is issued; and any rejection from the live client is treated as
 * the child having exited (`markAgentExited`) before being re-thrown.
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

  // See `RpcResumeCtx.leadSend`: the lead taking over the exchange releases a
  // thread bind the owner surface will never close (the headless
  // fork-raised-question path).
  if (ctx.leadSend && record.threadBound) record.threadBound = false;

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
    await client.start();
    await applyModelEffort(client, record.modelEffort);
    attachEventListener(ctx.pi, registry, record, client, ctx.onApprovalPending);
    // Role wiring that needs a live client (a revived fork's anti-bleed loop —
    // see `RpcAgentRecord.onResume`). Best effort: a wiring failure must not
    // turn a routine resume into a failed send.
    try {
      record.onResume?.(record);
    } catch {
      // ignored — see above.
    }
    await promptAgent(record, client, message);
    return { agent_id: agentId };
  }

  const live = record.client;
  try {
    if (record.streaming) {
      if (interrupt) {
        await live.steer(message);
      } else {
        await live.followUp(message);
      }
      // A steer/followUp joins the run already in flight, so the child is
      // outstanding again from the lead's point of view even though no fresh
      // prompt was issued — including for `terminalThisTurn` (review relay #1,
      // minor): new work was just dispatched, so a `final` filed before it no
      // longer keeps the child out of N.
      record.running = true;
      record.terminalThisTurn = false;
      // Mirror `promptAgent`: a final stashed before this instruction answers
      // the task being replaced, not the one just dispatched — a later settle
      // must not flush it as the reply to the new message.
      record.pendingFinal = undefined;
    } else {
      await promptAgent(record, live, message);
    }
  } catch (err) {
    // 260905: an in-flight request rejection is the other deterministic
    // "the child is gone" signal (`RpcClient.send()` throws once the process
    // has exited) — treat it exactly like a failed liveness probe so the lead
    // is told rather than left counting a dead agent, then re-throw so the
    // caller still sees the failure.
    markAgentExited(ctx.pi, registry, record);
    throw err;
  }
  return { agent_id: agentId };
}

/**
 * The agent's last assistant text, refreshed over RPC when the child is still
 * live and falling back to the cached `lastText` otherwise. This is the
 * former `ws-agent-wait` `reason:"idle"` payload, reused verbatim as the
 * `last_message` field of the `ws-agent-settled` push.
 */
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
 * Maps every registered agent to a `{agent_id, status}` pair: `"dormant"`
 * when there is no live client (stopped, resumable — D-C), else
 * `"running"`/`"idle"` from the locally-tracked streaming flag. Pure — no
 * IO, no RPC round trip — so directly unit-testable against fake records.
 *
 * 260905 adds `last_report_at` (ISO, omitted when the agent has never
 * reported): with `ws-agent-wait` gone, a lead that missed or compacted a
 * pushed report needs some way to see how long an agent has been quiet.
 * `status` deliberately keeps deriving from `streaming` rather than the new
 * `running` flag — `streaming` is event-confirmed and is the right thing to
 * DISPLAY, while `running` is the narrower, earlier-set fan-in counter.
 */
export function listAgents(registry: RpcAgentRegistry): Array<{ agent_id: string; status: AgentStatus; last_report_at?: string }> {
  return [...registry.entries()].map(([agentId, record]) => {
    const lastReport = record.reportLog[record.reportLog.length - 1];
    return {
      agent_id: agentId,
      status: (record.client ? (record.streaming ? "running" : "idle") : "dormant") as AgentStatus,
      ...(lastReport ? { last_report_at: new Date(lastReport.at).toISOString() } : {}),
    };
  });
}

/**
 * Best-effort graceful stop of `agentId`'s live RPC child (`abort()` then
 * `stop()`, both best-effort — a child that already exited or never
 * finished starting must not turn a routine stop into an unhandled
 * rejection). The registry entry is NEVER deleted here — per D-C, a stopped
 * agent stays registered as dormant/resumable; `ws-agent-send` auto-resumes
 * it later via the same cached session file. Throws only when `agentId`
 * itself is unknown.
 *
 * 260905: a stop is one of the four `ws-agent-settled` reasons, so a
 * non-silent stop pushes `reason:"stopped"` — the lead asked for it, but the
 * push is what removes the agent from its fan-in count in the same place
 * every other terminal transition does. `opts.silent` suppresses that for the
 * internal stops that are not a delegation outcome at all: `ask.ts` closing a
 * discussion thread (the owner's summary is the signal, not a stop notice)
 * and `stopAll()`'s shutdown sweep (the session is going away).
 */
export async function stopAgent(
  registry: RpcAgentRegistry,
  agentId: string,
  pi?: ExtensionAPI,
  opts?: { silent?: boolean },
): Promise<{ agent_id: string }> {
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
    clearLiveState(record);
    // Review relay #1 (I2): a stop is a thread-close path too — the ticket
    // names "lead stop" alongside `/done`/fork final/`ws-resolve`. Releasing
    // the bind here keeps a stopped agent from carrying a latched flag into a
    // later `ws-agent-send` revival, where it would silently suppress every
    // settle push for the rest of the session.
    record.threadBound = false;
    if (opts?.silent) {
      // An adapter-internal stop (a thread close, session shutdown) is not a
      // lead-facing event at all, so an un-pushed final dies with it rather
      // than arriving out of nowhere.
      record.pendingFinal = undefined;
    } else if (!flushPendingFinal(pi, registry, record, "stopped")) {
      pushToLead(pi, registry, record, "ws-agent-settled", { reason: "stopped" }, "followUp");
    }
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
 * Registers the six RPC-backed delegation tools (`ws-agent-spawn`,
 * `ws-agent-send`, `ws-agent-list`, `ws-agent-stop`, `ws-agent-transcript`,
 * `ws-report-to-lead`) plus the unchanged one-shot `explore` tool, against two
 * separate in-extension registries: `rpcRegistry` (RPC-backed, persistent
 * children) and `exploreRegistry` (one-shot, self-reaping recon leaves). They
 * are kept separate rather than unified because their completion signals are
 * fundamentally different (an `agent_settled` RPC event, pushed to the lead,
 * vs. a child process's `close` event awaited inline by `waitForDone`).
 *
 * Phase 2 adds a child->lead report channel: `ws-report-to-lead` is the only
 * child-side tool this ticket adds (registered here but reachable only from
 * a worker's `full-worker` `--tools` allowlist, per `TOOL_GROUPS`); its
 * `execute()` is a no-op ack — the relay to the lead rides the existing
 * `RpcClient.onEvent()` wire via `applyRpcEvent`'s `tool_execution_start`
 * branch, not the tool's return value (see that function's doc comment).
 * `ws-agent-transcript` is a lead-side introspection tool (same family as
 * `ws-agent-list`/`ws-agent-stop`), never added to any `TOOL_GROUPS` entry, so
 * it is not reachable from a worker's own `--tools`.
 *
 * 260905: there is no seventh, harvesting tool — `ws-agent-wait` is deleted.
 * Everything a lead used to block for now arrives as a pushed custom message
 * (see this module's header and `pushToLead`), and the background liveness
 * probe started here is what turns a child that dies without settling into an
 * `exited` push instead of silence.
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
  const stopLivenessProbe = startLivenessProbe(pi, rpcRegistry);

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
      "Spawn a persistent RPC-backed pi subagent from an already-rendered system-prompt file (e.g. via ws/playbook.render). Returns {agent_id} immediately after the initial prompt is sent. Do not wait for it: end your turn, and its reports, questions and completion arrive on their own as ws-agent-* messages carrying a running-count status line.",
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
          pi,
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
      "Send a message to a spawned agent. Delivers via prompt() when idle (including immediately after auto-resuming a dormant agent); while mid-stream, interrupt:true steers it and interrupt:false/omitted queues a follow-up. A dormant (ws-agent-stop'd) agent_id — including one revived from a ws-agent-orphaned message after a session restart — is auto-resumed from its cached session file first. Returns as soon as the message is delivered; the agent's answer arrives later as a pushed ws-agent-* message.",
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
      // `leadSend: true`: this tool is the lead's own channel to a child (see
      // `RpcResumeCtx.leadSend`), unlike ask.ts's overlay channel.
      const result = await sendToAgent(
        rpcRegistry,
        { pi, cwd: sessionCtx.cwd, onApprovalPending, leadSend: true },
        p.agent_id,
        p.message,
        p.interrupt,
      );
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: "ws-agent-list",
    label: "ws-agent-list",
    description:
      "List every tracked agent_id, its status (running/idle/dormant), and last_report_at (ISO, absent if it has never reported). Use it to check on a quiet agent — there is no wait tool; every report, question, approval request and completion is pushed to you as a ws-agent-* message on its own.",
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
      // Non-silent: an explicit stop is a delegation outcome the lead should
      // see land in its transcript like every other settle reason.
      const result = await stopAgent(rpcRegistry, p.agent_id, pi);
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
      "Surface an async status update or intermediate finding to the lead immediately, distinct from your final answer. It is delivered to the lead the moment you call this — there is nothing to wait for on either side.",
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
      // No-op ack: the relay to the parent already happens via the existing
      // RpcClient.onEvent() stream (Pi emits tool_execution_start the instant
      // the LLM dispatches this call, forwarded verbatim to the parent's
      // applyRpcEvent, which pushes it into the lead's session) — see that
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
      stopLivenessProbe();
      // Silent by construction: the session itself is going away, so a
      // per-agent "stopped" push would have nowhere to land. Routed through
      // stopAgent so shutdown leaves records in the same resting shape every
      // other stop does (the sidecar snapshot, index.ts, is taken BEFORE this
      // runs, while the records are still marked live).
      const rpcStops = [...rpcRegistry.keys()].map((agentId) => stopAgent(rpcRegistry, agentId, pi, { silent: true }).catch(() => undefined));
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
