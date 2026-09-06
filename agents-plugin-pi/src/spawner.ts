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
 * The spawn tool's `model_name` param names one of the four fixed tiers
 * (`small`/`medium`/`large`/`xlarge`); the caller (the lead) passes an
 * already-rendered `system_prompt_path` — this module never calls
 * `playbook.render` itself for the RPC-backed path — plus that optional
 * `model_name`, resolved through ws-mcp's `config.resolve_agent` tool
 * (`resolveModelForAliasViaWsMcp` below), or omits it to inherit the parent
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
 * Async, ws-mcp-backed tier resolution (Phase 4: replaces the old
 * file-catalog-backed `resolveModelForAlias`): when `alias` (one of the four
 * fixed tiers `small|medium|large|xlarge`) is given, calls ws-mcp's
 * `config.resolve_agent` tool (`{tier: alias, format: "json"}`) and accepts
 * its answer only when it is a GENUINE `pi`-labeled hit — `resolved_from ===
 * "pi"` AND the returned model string contains a `/` (Phase 3 Forward (a): a
 * partial `config.tune agents.tier harness:pi` write can seed the `pi`
 * bucket from the codex default, so `resolved_from === "pi"` alone is not
 * proof of a real Pi `provider/id` string — a codex-shaped fallback model
 * carries no `/`).
 *
 * NEVER HARD-FAILS: no alias, an `isError` result, a missing/unparsable text
 * body, a non-`pi` `resolved_from`, or a `pi`-labeled-but-slash-less model
 * all degrade to `{model: inheritModel}` — matching this adapter's existing
 * "the adapter reads the resolution from ws-mcp; missing/unmapped stays
 * inherit, never an error" contract. `effort` is only ever populated
 * alongside a genuine hit, and only when the resolved effort string is
 * non-empty (an empty resolved effort means "no explicit override" — the
 * caller passes no `modelEffort` at all rather than an empty one).
 *
 * Used by both the RPC-backed `spawnAgent`'s `model_name` and `explore`'s
 * fixed `"small"` lookup, and per-tier by `bridge.ts`'s
 * `computePiAliasTableUnset` (the sole "genuine `pi` hit" predicate — see
 * that function's doc comment for why it reuses this resolver instead of
 * reimplementing the guard).
 */
/**
 * Minimal `callTool`-shaped interface `resolveModelForAliasViaWsMcp` needs —
 * a real `McpStdioClient` satisfies this structurally, but so does
 * `bridge.ts`'s own duck-typed `callTool` closure (`WorkflowManualMappingDeps["callTool"]`),
 * letting `computePiAliasTableUnset` call this resolver without a circular
 * import (`spawner.ts` already imports `type BridgeHandle` from
 * `bridge.ts` — a type-only import that is erased at build/runtime, so a
 * VALUE import in the other direction, `bridge.ts` importing this function
 * from `spawner.ts`, does not create a runtime cycle).
 */
export interface ResolveAgentCallToolClient {
  callTool: (name: string, args: Record<string, unknown>) => Promise<McpToolCallResult>;
}

export async function resolveModelForAliasViaWsMcp(
  client: ResolveAgentCallToolClient,
  alias: string | undefined,
  inheritModel: string | undefined,
): Promise<{ model?: string; effort?: string }> {
  if (!alias) return { model: inheritModel };
  try {
    const result = await client.callTool("config.resolve_agent", { tier: alias, format: "json" });
    if (result.isError) return { model: inheritModel };
    const text = result.content.find((item) => item.type === "text")?.text;
    if (!text) return { model: inheritModel };
    let parsed: { model?: string; effort?: string; resolved_from?: string };
    try {
      parsed = JSON.parse(text) as { model?: string; effort?: string; resolved_from?: string };
    } catch {
      return { model: inheritModel };
    }
    if (parsed.resolved_from !== "pi" || !parsed.model || !parsed.model.includes("/")) {
      return { model: inheritModel };
    }
    return { model: parsed.model, effort: parsed.effort || undefined };
  } catch {
    return { model: inheritModel };
  }
}

/**
 * Pure merge rule for `spawnAgent`'s `modelEffort`: an explicit caller
 * `model_effort` always wins over the config-resolved `effort`, but only
 * when it is actually non-empty — `model_effort` is a free-form `string`
 * param with no enum (`SpawnAgentParams.modelEffort`), so a caller-supplied
 * `""` is reachable and must NOT shadow a genuine tier effort (`??` would
 * only guard `null`/`undefined`, not `""`; `applyModelEffort`'s own guard
 * (`if (!modelEffort) return`) already treats `""` as absent, so this
 * matches that convention). The single call site
 * (`record.modelEffort = effectiveModelEffort(...)`) is the one place this
 * rule is computed — both the spawn-time and dormant-resume `applyModelEffort`
 * calls read the already-folded `record.modelEffort` back, never re-deriving
 * it from `params`.
 */
export function effectiveModelEffort(callerEffort: string | undefined, resolvedEffort: string | undefined): string | undefined {
  return callerEffort || resolvedEffort;
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
  /**
   * 260905 (alias/park/cap ticket): caller-supplied, human-chosen short name
   * for THIS agent record — a different concept from
   * `resolveModelForAliasViaWsMcp`'s "alias," which names a fixed *tier*
   * resolved through ws-mcp's `config.resolve_agent` tool (see above).
   * Optional; the adapter never derives one from the prompt. Resolved
   * through `resolveAgentId` alongside the raw `agentId` uuid on every
   * `ws-agent-send`/`ws-agent-stop`/`ws-agent-transcript`/`ws-approve` call.
   * Reusing an alias on a new spawn overwrites a dormant/idle holder (clearing
   * the holder's `alias`, not its `title`) or rejects the spawn outright when
   * the holder is `running`/`threadBound` — see `spawnAgent`.
   */
  alias?: string;
  /**
   * 260905: caller-supplied free-text label, independent of `alias` — never
   * used for resolution, purely descriptive (roll-call/list display). Reusing
   * an `alias` on a new spawn clears the prior holder's `alias` but leaves its
   * `title` untouched.
   */
  title?: string;
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
   * 260905 (live-agent widget ticket): epoch-ms stamp of the most recent
   * prompt ISSUED to this child, stamped unconditionally by `promptAgent` —
   * including the anti-bleed nudge (`isLeadPrompt: false`), unlike
   * `lastLeadPromptAt` below, because the widget's "running" row is meant to
   * show how long THIS turn has been going, and a nudge starts a new turn on
   * the wire even though it is not a new lead-issued task boundary. Read by
   * `agent-widget.ts`'s `buildAgentRows` as the running-row elapsed clock;
   * left untouched by `sendToAgent`'s `steer`/`followUp` join (see that
   * function's doc comment) — a mid-stream steer keeps ticking from the
   * turn's original start, by design.
   */
  runStartedAt?: number;
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
  /**
   * 260905: the head-truncated (`truncatePromptForStorage`,
   * `PROMPT_STORAGE_CAP_BYTES`) copy of the spawn's initial `prompt`, stashed
   * for `ws-agent-list`'s opt-in `include_prompt` reply and the shutdown
   * sidecar. Set once at spawn, never updated by later `ws-agent-send` calls.
   */
  prompt?: string;
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
   * 260905 (list-model/last-report-fidelity ticket): the sidecar-revival-only
   * fallback for `last_report_at` — `rehydrateOrphanRecord` fills this from
   * `PersistedOrphan.lastReportAt` (the newest `reportLog` entry at
   * shutdown, already an ISO string). `listAgents` and `evictForCapacity`
   * both prefer a real `reportLog` entry over this value whenever one is
   * present, so a revived record that has reported since falls back to its
   * own history rather than the stale shutdown snapshot.
   */
  lastReportAtOverride?: string;
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
   * the LEAD NOTICE to push: a defined return means the question itself is
   * answered on the owner surface, not by the lead, but the returned string
   * is still pushed to the lead as a `ws-agent-advisory`/`fork-question-thread`
   * message in place of the `ws-agent-question` the headless baseline would
   * send. Set by `fork.ts`'s `registerFork`; `spawner.ts`
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

/**
 * 260905 (alias/park/cap ticket): the ONE alias-or-uuid resolution helper
 * every `agent_id` param goes through (`sendToAgent`, `stopAgent`,
 * `getAgentTranscriptPath`, and `execute-gateway.ts`'s `ws-approve`), so a
 * caller may pass either the raw uuid or a record's `alias` interchangeably.
 * Direct `registry.has(idOrAlias)` wins first (the uuid path, cheap and
 * unambiguous); otherwise scans for a record whose `alias` matches exactly.
 * Returns `undefined` on a genuine miss — each call site's existing "unknown
 * agentId" error path is left unchanged by falling back to the original
 * input (`resolveAgentId(registry, id) ?? id`).
 */
export function resolveAgentId(registry: RpcAgentRegistry, idOrAlias: string): string | undefined {
  if (registry.has(idOrAlias)) return idOrAlias;
  for (const record of registry.values()) {
    if (record.alias === idOrAlias) return record.agentId;
  }
  return undefined;
}

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
 * 260905 (alias/park/cap ticket): env var the LEAD process reads at spawn
 * time to override the registry cap (`DEFAULT_AGENT_REGISTRY_CAP`). Never
 * forwarded to a child's env — this is lead-process-only config, unlike the
 * child-carried role markers in `process-role.ts`.
 */
export const WS_PI_AGENT_REGISTRY_CAP_ENV = "WS_PI_AGENT_REGISTRY_CAP";

/** Default registry cap when `WS_PI_AGENT_REGISTRY_CAP_ENV` is unset or unparsable. */
export const DEFAULT_AGENT_REGISTRY_CAP = 256;

/**
 * Pure env-param resolver for the registry cap, mirroring `shouldPushToLead`'s
 * testable-default-param shape. A missing, non-numeric, or non-positive value
 * falls back to `DEFAULT_AGENT_REGISTRY_CAP` rather than throwing or producing
 * a cap of zero/negative that would reject every spawn.
 */
export function resolveAgentRegistryCap(env: NodeJS.ProcessEnv = process.env): number {
  const raw = env[WS_PI_AGENT_REGISTRY_CAP_ENV];
  if (typeof raw !== "string" || raw.trim() === "") return DEFAULT_AGENT_REGISTRY_CAP;
  const parsed = Number(raw);
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : DEFAULT_AGENT_REGISTRY_CAP;
}

/**
 * Byte cap on the stored copy of a spawn's `prompt` (`RpcAgentRecord.prompt`),
 * applied once at spawn time — bounds the record itself, the shutdown sidecar
 * that serializes it, and the `ws-agent-list` `include_prompt` reply alike.
 */
export const PROMPT_STORAGE_CAP_BYTES = 4096;

/** Marker line appended by `truncatePromptForStorage` when a prompt was cut. */
const PROMPT_TRUNCATION_MARKER = "\n…[truncated for storage]";

/**
 * Byte-safe head-truncation of a prompt for storage: cuts at `capBytes` UTF-8
 * bytes (never mid-codepoint — a naive `Buffer.slice` cut can straddle a
 * multibyte sequence and corrupt the tail) and appends a cut-marker line when
 * truncation actually happened. Mirrors `AgentEventLineBuffer`'s existing
 * `StringDecoder`-based multibyte-safe convention.
 */
export function truncatePromptForStorage(prompt: string, capBytes: number = PROMPT_STORAGE_CAP_BYTES): string {
  const buf = Buffer.from(prompt, "utf8");
  if (buf.length <= capBytes) return prompt;
  const decoder = new StringDecoder("utf8");
  const head = decoder.write(buf.subarray(0, capBytes));
  return `${head}${PROMPT_TRUNCATION_MARKER}`;
}

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
 *   must answer (in TUI the owner surface consumes it, and the lead instead
 *   gets the `fork-question-thread` advisory below).
 * - `ws-agent-approval` — an `execute-worker` is blocked on `ws-approve`.
 * - `ws-agent-advisory` — the adapter's own statement about a child: emitted
 *   by `fork.ts`'s anti-bleed loop (a fork's turn shape) and, since 260905,
 *   by this module's question branch registering a fork-raised thread
 *   (`advisory: "fork-question-thread"`, `followUp`).
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
 * The shared registry walk behind both `computeRunningStatusLine` (below) and
 * the goal-loop yield predicate (`hasRunningAgents`, 260905 Phase 2): skips
 * only `threadBound` records, and reports whether anything counts as
 * "present" (any non-threadBound registry member — dormant/parked included,
 * see the alias/park/cap ticket's presence-rule change) at all, plus how many
 * of those are still `running` and not `terminalThisTurn`. Extracted so the
 * two call sites can never drift apart in what they count as fan-in.
 */
function computeFanIn(registry: RpcAgentRegistry | undefined): { present: boolean; running: number } {
  let present = false;
  let running = 0;
  for (const record of registry?.values() ?? []) {
    if (record.threadBound) continue;
    present = true;
    if (record.running && !record.terminalThisTurn) running += 1;
  }
  return { present, running };
}

/**
 * The fan-in status line every pushed message carries: `N delegated agents
 * still running`, computed fresh at push time over the shared registry.
 *
 * - The line is PRESENT whenever the registry holds any member that is NOT
 *   `threadBound` — 260905 (alias/park/cap ticket) keys this on registry
 *   membership, not on a live client: a dormant/parked record still counts as
 *   present, since automatic parking (see the settle handler below) now
 *   routinely turns a settled, non-threadBound child dormant. An `explore`
 *   leaf is never in this registry at all (it has its own).
 * - N counts the subset of those that is still `running` and has not yet
 *   filed a `final`/`question` this turn (`terminalThisTurn`), so the agent
 *   whose own terminal report triggered this very push has already removed
 *   itself. `0 delegated agents still running` is the lead's synthesis cue —
 *   and, per the presence rule above, `0 …` stays visible (not omitted) as
 *   long as any non-threadBound record — dormant included — remains
 *   registered; cap eviction is what eventually removes it.
 *
 * Owner decision after the second live run (2026-09-05): the former
 * running-out-of-total form with an id suffix is gone. The total only ever
 * grew across a session (an idle child keeps its process, so it stayed counted
 * until stopped/exited) and the id suffix duplicated what `ws-agent-list`
 * already answers, so both were noise. The omission rule survives unchanged:
 * when nothing at all is registered (or every member is threadBound) there is
 * no line at all (`undefined`), so a push that has nothing to do with
 * delegation fan-in — a `ws-agent-orphaned` roll-call at session start, a
 * `spawn-failed` for the only child — never ends with a contentless zero line.
 */
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
 * (`ctx.isIdle`) by `index.ts` — the same mutable-ref seam `wsBlockBaseRef` uses
 * for a value only a live ctx can supply. `pushToLead` reads it to decide
 * whether a `followUp` push can go out now or must be HELD (see
 * `heldPushQueue`). Unset (a test, a headless path that never ran
 * `session_start`, a torn-down session) is deliberately treated as IDLE: the
 * pre-hold behavior of sending straight through is the safe default, since a
 * held push that nothing ever flushes would be a lost report.
 */
export const leadIdleRef: { current: (() => boolean) | undefined } = { current: undefined };

/**
 * 260906 (compaction push-hold ticket, Phase 1): tracks whether the owning
 * session has an in-flight `ctx.compact()` right now. Set by the goal-loop's
 * `goal-compact-and-continue` lever before it calls `ctx.compact`, and
 * defensively by `session_before_compact` for ANY compaction reason (owner
 * `/compact`, Pi's own threshold/overflow auto-compaction) — not just the
 * lever's own. Cleared only by `goal-loop.ts`'s `releaseAfterCompaction`
 * (deferred past Pi's own compaction flag) or, as a backstop, by the next
 * `agent_start`. `isOwningAgentIdle()` folds this in so a `followUp` push
 * held during ordinary mid-turn work is ALSO held across the whole
 * compaction window, closing the race where a push or a
 * `goal-compact-and-continue` reminder could be delivered into the doomed
 * turn `ctx.compact()`'s internal abort just settled.
 */
export const leadCompactingRef: { current: boolean } = { current: false };

/**
 * 260906 Phase 1 (settle-timer reminder race ticket): true only between the
 * goal-loop's settle timer calling `pi.sendUserMessage(reminder, { deliverAs:
 * "followUp" })` (the reinject reminder's own `prompt()` pre-run awaits,
 * before the run is actually streaming) and one of that boundary guard's
 * three clear points landing: `agent_start`, `agent_settled` (a real settle
 * is proof the reminder's run at least started), or the guard's own fallback
 * timeout (no `agent_start`/`agent_settled` arrived within the settle delay
 * of the send). Owned and mutated by `goal-loop.ts`'s settle timer; read
 * here, at send time, by `sendPush` so a push racing the reminder's own
 * pre-run await is sent with `triggerTurn: false` — landing via Pi's
 * `_appendCustomMessage` instead of colliding with the reminder's own
 * `prompt()` call ("Agent is already processing…").
 */
export const leadReminderStartPendingRef: { current: boolean } = { current: false };

/**
 * 260905 (live-agent widget ticket): the same mutable-ref seam as
 * `leadIdleRef`, filled by `index.ts`'s `session_start` (TUI lead only) with
 * a closure that recomputes `agent-widget.ts`'s rows and repaints the
 * `belowEditor` widget + `setStatus` segment. Lets every registry-transition
 * point in this module (spawn, settle, stop, exit, spawn-failed, an approval
 * or report event) trigger a re-render WITHOUT importing `agent-widget.ts` or
 * `ask.ts` — this module must stay the lower layer, mirroring `ask.ts`'s own
 * "imports FROM spawner.ts, never the reverse" rule. `undefined` outside a
 * TUI lead session (a worker/explore child, a headless lead, a test), in
 * which case `triggerAgentWidgetRefresh` is a silent no-op.
 */
export const agentWidgetRefreshRef: { current: (() => void) | undefined } = { current: undefined };

/**
 * Best-effort fire of `agentWidgetRefreshRef`, swallowing a throw — matches
 * every other push call site's swallow-and-continue convention. Called from
 * every registry-transition point a live-agent-widget row depends on: a
 * throwing or absent refresh must never turn a routine spawn/settle/stop into
 * a crashed event listener.
 */
function triggerAgentWidgetRefresh(): void {
  try {
    agentWidgetRefreshRef.current?.();
  } catch {
    // best effort — see doc comment above.
  }
}

/**
 * Whether the owning session's agent is between turns right now. See
 * `leadIdleRef`. 260906 (compaction push-hold ticket, Phase 1): an in-flight
 * compaction (`leadCompactingRef`) forces `false` regardless of Pi's own
 * idle read — `ctx.compact()`'s internal abort settles the invoking turn
 * before the compaction flag is even set, so relying on `leadIdleRef` alone
 * would read a compacting session as idle for the entire compaction window.
 * Exported for `goal-loop.ts`'s `releaseAfterCompaction`, which needs the
 * same read to decide whether to flush now or leave it to the next settle.
 */
export function isOwningAgentIdle(): boolean {
  if (leadCompactingRef.current) return false;
  const isIdle = leadIdleRef.current;
  if (!isIdle) return true;
  try {
    return isIdle() !== false;
  } catch {
    return true;
  }
}

/** One `followUp`/`steer` push deferred until the owning session's current turn (or compaction) settles. */
interface HeldPush {
  kind: "push";
  registry: RpcAgentRegistry | undefined;
  record: RpcAgentRecord | undefined;
  family: PushFamily;
  payload: Record<string, unknown>;
  /** Recorded so `flushHeldPushes` resends with the SAME delivery mode it was held under, not a hardcoded one. */
  deliverAs: PushDeliverAs;
}

/**
 * 260906 (compaction push-hold ticket, Phase 1): a pre-built raw send held
 * alongside family-shaped pushes on the same queue — `ask.ts`'s
 * `injectDiscussionSummary` builds its `ws-thread-summary` message itself
 * (it has no `PushFamily`/payload shape to reconstruct at flush time), so it
 * holds a closure that performs its own `pi.sendMessage` call instead.
 */
interface HeldRawSend {
  kind: "raw";
  send: (pi: ExtensionAPI) => void;
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
 *
 * 260906 (compaction push-hold ticket, Phase 1): also holds `HeldRawSend`
 * entries (see that type's doc comment) so `ask.ts`'s
 * `injectDiscussionSummary` can share this same queue/flush-ordering
 * mechanism instead of maintaining a parallel one.
 */
export const heldPushQueue: Array<HeldPush | HeldRawSend> = [];

/**
 * Composes the `pi.sendMessage`/`sendCustomMessage` options for an
 * adapter-initiated lead turn start, reading `leadReminderStartPendingRef`
 * FRESH at call time: `triggerTurn` is `false` while the ref is `true` — the
 * boundary guard against colliding with the goal-loop's settle-timer
 * reminder mid-await in its own `prompt()` call (`sendUserMessage` has no
 * `triggerTurn` option of its own to race against). With the ref set, the
 * send instead lands via Pi's `_appendCustomMessage` (a synchronous,
 * turn-less append onto `agent.state.messages`) and is picked up once the
 * reminder's own turn actually starts, rather than throwing "Agent is
 * already processing…" or silently dropping.
 *
 * 260906 Phase 1 review relay #1 (Important #2): extracted so every
 * adapter-initiated send that can start a lead turn goes through the same
 * guard, not just `sendPush`'s family-shaped pushes — `ask.ts`'s
 * `injectDiscussionSummary` also calls this, both for its immediate send and
 * inside its held `kind: "raw"` closure, so a closure built while the flag
 * was clear still reads the flag's value AT FLUSH TIME rather than a stale
 * snapshot captured when the send was held.
 */
export function composeLeadTurnStartOptions(deliverAs: PushDeliverAs): { deliverAs: PushDeliverAs; triggerTurn: boolean } {
  return { deliverAs, triggerTurn: !leadReminderStartPendingRef.current };
}

/**
 * Builds and sends one push immediately, computing its status line right
 * now.
 *
 * 260906 Phase 1 (settle-timer reminder race ticket): `triggerTurn` is
 * composed by `composeLeadTurnStartOptions` — see its doc comment for the
 * boundary-guard rationale.
 */
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
  // 260905 (alias/park/cap ticket): pushed-message heads print the alias when
  // there is one, followed by the uuid — a one-line, zero-signature-churn
  // change: compose the display id here and pass it through
  // `buildPushContent`'s existing bare-`agentId` parameter, leaving that
  // helper (and `details.agent_id`, which stays the raw uuid) untouched.
  const displayId = record?.alias ? `${record.alias} (${record.agentId})` : record?.agentId;
  try {
    pi.sendMessage(
      {
        customType: family,
        content: buildPushContent(family, displayId, payload, status),
        display: true,
        details: details as never,
      },
      composeLeadTurnStartOptions(deliverAs),
    );
  } catch {
    // Best effort: a push that cannot be delivered (a torn-down session, a
    // host that rejected the message) must never turn a child's routine
    // report into a crashed event listener.
  }
}

/**
 * Releases every held push, in arrival order — a `HeldPush` with its status
 * line computed NOW and resent with its recorded `deliverAs`, or a
 * `HeldRawSend` invoked as-is. Called from the owning session's
 * `agent_settled` (see `registerPushFlush`) or from `goal-loop.ts`'s
 * `releaseAfterCompaction`: the agent is idle at that instant, so the first
 * send starts a fresh run and the rest land in Pi's own followUp queue behind
 * it — one lead run that sees all of them in order, which is what the
 * one-at-a-time drain was always meant to produce.
 *
 * The queue is drained BEFORE the first send so a push issued from inside that
 * run is held again for the next settle rather than re-entering this drain.
 */
export function flushHeldPushes(pi: ExtensionAPI | undefined): number {
  const pending = heldPushQueue.splice(0, heldPushQueue.length);
  if (!pi) return 0;
  for (const held of pending) {
    if (held.kind === "raw") {
      held.send(pi);
      continue;
    }
    sendPush(pi, held.registry, held.record, held.family, held.payload, held.deliverAs);
  }
  return pending.length;
}

/**
 * Arms the held-push release on the owning session's own `agent_settled`.
 * Registered at factory scope (like `registerGoalLoop`) rather than inside
 * `session_start`, so a `/reload` cannot stack duplicate handlers. The role
 * gate is re-checked at fire time for the same reason `pushToLead` checks it:
 * a `worker`/`explore` process holds nothing, so it has nothing to flush.
 *
 * 260906 (compaction push-hold ticket, Phase 1): also gated on
 * `leadCompactingRef` — the abort inside `ctx.compact()` fires an
 * `agent_settled` for the turn it just aborted BEFORE the compaction flag is
 * set on Pi's own side, so this is the only thing stopping this handler from
 * flushing the held queue into that doomed turn. `goal-loop.ts`'s
 * `releaseAfterCompaction` is what flushes it once the compaction actually
 * finishes.
 */
export function registerPushFlush(pi: ExtensionAPI): void {
  pi.on("agent_settled", () => {
    if (!shouldPushToLead()) return;
    if (leadCompactingRef.current) return;
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
 * it. `steer` pushes are normally never held — their whole point is to
 * interrupt — with one 260906 exception: while `leadCompactingRef.current` is
 * true, a `steer` push is held too, since `ctx.compact()`'s
 * `sendCustomMessage` path bypasses Pi's own `prompt()` compaction guard
 * entirely (there is no live turn to interrupt into during a compaction), and
 * is released once `goal-loop.ts`'s `releaseAfterCompaction` runs. A `steer`
 * push mid-turn (not compacting) still bypasses the hold as before.
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
  const holdFollowUp = deliverAs === "followUp" && !isOwningAgentIdle();
  const holdSteerWhileCompacting = deliverAs === "steer" && leadCompactingRef.current;
  if (holdFollowUp || holdSteerWhileCompacting) {
    heldPushQueue.push({ kind: "push", registry, record, family, payload, deliverAs });
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
 *
 * 260905 (live-agent widget ticket): `runStartedAt` is stamped
 * unconditionally, unlike `lastLeadPromptAt` — the widget's elapsed clock
 * resets on a nudge too, since the nudge really did start a fresh turn on
 * the wire even though it is not a new lead-issued task boundary.
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
  record.runStartedAt = Date.now();
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
  triggerAgentWidgetRefresh();
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
  triggerAgentWidgetRefresh();
}

export interface SpawnAgentParams {
  systemPromptPath: string;
  prompt: string;
  modelName?: string;
  modelEffort?: string;
  /**
   * 260905 (alias/park/cap ticket): optional human-chosen short name for this
   * agent, resolved alongside the raw uuid by `resolveAgentId`. Reusing an
   * alias already held by a `running`/`threadBound` record rejects the spawn;
   * a dormant/idle holder's alias is overwritten (its `title` is untouched).
   * Never derived from `prompt` — the adapter only ever uses what the caller
   * passed.
   */
  alias?: string;
  /** 260905: optional free-text label, independent of `alias` — see `RpcAgentRecord.title`. */
  title?: string;
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
  /** ws-mcp client used to resolve `model_name` through `config.resolve_agent` (`resolveModelForAliasViaWsMcp`), read fresh per spawn. */
  client: McpStdioClient;
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
 * 260905 Phase 1 (fork-question lead notice, was 260904 Phase 2 / review
 * relay #1 I6): a `kind:"question"` report is passed through
 * `record.onQuestionReport` (when set) first. That hook thread-binds the
 * record and, in TUI mode, returns the registration-notice string built by
 * `buildForkQuestionLeadNotice` — the lead must still be told a thread now
 * exists, just not asked to answer it. A DEFINED (string) return is pushed to
 * the lead as `ws-agent-advisory`/`fork-question-thread` (`detail` is that
 * notice text) so the lead is told the thread exists rather than seeing
 * nothing at all — `followUp` delivery only guarantees the notice is queued
 * for the lead's next turn boundary, not that it precedes the owner's
 * `/answer`; `undefined` is the headless baseline and the question is pushed
 * as `ws-agent-question`/`steer` for the lead to answer directly. A throwing
 * hook degrades to that same headless baseline rather than dropping the
 * report.
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
        // A defined (string) return is the registration notice for a fork
        // thread the hook just bound: push it to the lead as an advisory
        // instead of the raw question. `undefined` is the headless case;
        // a throwing hook degrades to that same baseline rather than
        // dropping the report.
        let notice: string | undefined;
        if (record.onQuestionReport) {
          try {
            notice = record.onQuestionReport(record, message);
          } catch {
            notice = undefined;
          }
        }
        return notice !== undefined
          ? { push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: notice }, deliverAs: "followUp" } }
          : { push: { family: "ws-agent-question", payload: { question: message }, deliverAs: "steer" } };
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
 * 260905 (alias/park/cap ticket): **automatic park** is the last step of this
 * settle handling, run AFTER `probeAgentLiveness` resolves (so a park never
 * races the liveness check) and after the (separately-registered) anti-bleed
 * advisory/nudge judgment has had its chance to re-prompt the child — see the
 * plan's Codebase Findings for why no explicit ordering call into `fork.ts` is
 * needed: a synchronous nudge already flips `record.running` before this IIFE
 * resumes past its own `await`. Parks (silent `stopAgent`) iff
 * `!record.threadBound && !record.running` at that point: a `threadBound`
 * record is never parked, and a record the nudge re-prompted is not parked
 * either. No grace period, and no extra push for the park itself — the
 * settle/final push above already told the lead the child stopped; parking is
 * pure resource cleanup, silently resumed later by `sendToAgent`'s dormant
 * branch. A park failure is swallowed (best effort), matching every other
 * best-effort branch in this handler.
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
    if (e.type === "agent_start") {
      // 260905 (live-agent widget ticket): streaming just flipped true — a
      // fresh "running" row transition.
      triggerAgentWidgetRefresh();
    }
    if (outcome.settled) {
      void (async () => {
        // The deferred final, if this turn filed one, IS the settle message.
        if (!flushPendingFinal(pi, registry, record, "idle") && !record.threadBound && !record.terminalThisTurn) {
          const lastMessage = await harvestLastMessage(record);
          pushToLead(pi, registry, record, "ws-agent-settled", { reason: "idle", last_message: lastMessage }, "followUp");
        }
        await probeAgentLiveness(pi, registry, record);
        // Automatic park: the last step, after the liveness probe and (by
        // event-loop ordering) after any synchronous nudge has had its chance
        // to re-prompt this record. See the doc comment above.
        if (registry && !record.threadBound && !record.running) {
          try {
            await stopAgent(registry, record.agentId, pi, { silent: true });
          } catch {
            // best effort — a park failure must not crash the settle handler.
          }
        }
        // 260905 (live-agent widget ticket): fired after the liveness probe
        // and the possible automatic park above, so the widget's re-render
        // sees the record's fully-settled state (dormant if parked, still
        // running if a synchronous nudge re-prompted it first).
        triggerAgentWidgetRefresh();
      })();
    }
    if (e.type === "tool_execution_start" && (e.toolName === GATED_EXEC_TOOL_NAME || e.toolName === REPORT_TO_LEAD_TOOL_NAME)) {
      // 260905 (live-agent widget ticket): a gated command just went pending
      // approval, or a report (progress/question/final) was just observed —
      // both are widget-relevant transitions per the ticket's own list.
      triggerAgentWidgetRefresh();
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
 * 260905 (alias/park/cap ticket): the alias half of `spawnAgent`'s guard
 * clauses, run before any side effect (`mkdtempSync`/`randomUUID`). No-op
 * (`{ ok: true }`) when `alias` is unset. Otherwise scans for the current
 * holder of `alias`: a `running`/`threadBound` holder blocks the spawn
 * outright (rejection, not silent skip — the ticket's own wording); any other
 * holder (dormant/idle) is returned as `holder` for the caller to clear.
 *
 * Review relay #1 (Critical): this function does **not** mutate `holder`
 * itself — it only locates and validates. `runSpawnGuards` below commits
 * `holder.alias = undefined` only once every guard (this one and
 * `evictForCapacity`) has actually succeeded, so a rejected spawn (this
 * guard, `evictForCapacity`, or anything else `spawnAgent` throws before
 * registering) leaves the previous holder's alias completely untouched —
 * still resolvable by `ws-agent-send <alias>`. Exported for direct unit
 * coverage without a real `RpcClient`.
 */
export function reserveAgentAlias(
  registry: RpcAgentRegistry,
  alias: string | undefined,
): { ok: true; holder?: RpcAgentRecord } | { ok: false; error: string } {
  if (!alias) return { ok: true };
  for (const holder of registry.values()) {
    if (holder.alias !== alias) continue;
    if (holder.running || holder.threadBound) {
      const state = holder.running ? "running" : "threadBound";
      return {
        ok: false,
        error: `ws-pi-agent: ws-agent-spawn rejected: alias "${alias}" is held by agent ${holder.agentId}, which is ${state}`,
      };
    }
    return { ok: true, holder };
  }
  return { ok: true };
}

/**
 * 260905 (alias/park/cap ticket): the registry-cap half of `spawnAgent`'s
 * guard clauses. While the registry is at or over `cap`, evicts the dormant
 * (`!record.client`), non-`running`, non-`threadBound` record with the
 * oldest last-activity stamp (`max(lastLeadPromptAt, last reportLog entry, or —
 * for a revived orphan with no reportLog yet — its lastReportAtOverride)`)
 * until the new spawn fits. Never evicts a `running`/`threadBound` record —
 * when none remain to evict, the spawn fails outright rather than silently
 * exceeding the cap. Only forgets the registry entry (never touches the
 * evicted agent's on-disk session file). Exported for direct unit coverage
 * without a real `RpcClient`.
 */
export function evictForCapacity(registry: RpcAgentRegistry, cap: number): { ok: true; evictedLabel?: string } | { ok: false; error: string } {
  const evictedLabels: string[] = [];
  while (registry.size >= cap) {
    let candidate: RpcAgentRecord | undefined;
    let candidateActivity = Number.POSITIVE_INFINITY;
    for (const record of registry.values()) {
      if (record.client || record.running || record.threadBound) continue;
      const lastReportActivity = record.reportLog.at(-1)?.at ?? (record.lastReportAtOverride ? Date.parse(record.lastReportAtOverride) : 0);
      const activity = Math.max(record.lastLeadPromptAt ?? 0, lastReportActivity);
      if (activity < candidateActivity) {
        candidate = record;
        candidateActivity = activity;
      }
    }
    if (!candidate) {
      return {
        ok: false,
        error: `ws-pi-agent: ws-agent-spawn rejected: registry cap (${cap}) reached and every remaining record is running/threadBound — nothing can be evicted to fit`,
      };
    }
    registry.delete(candidate.agentId);
    evictedLabels.push(candidate.alias ?? candidate.agentId);
  }
  return evictedLabels.length > 0 ? { ok: true, evictedLabel: evictedLabels.join(", ") } : { ok: true };
}

/**
 * 260905 (alias/park/cap ticket, review relay #1 CRITICAL fix): the single
 * gate `spawnAgent` calls before any side effect (`mkdtempSync`/
 * `randomUUID`/`registry.set`). Runs `reserveAgentAlias` then
 * `evictForCapacity` and returns the first failure unmutated — neither guard
 * commits anything on its own. Only once BOTH guards succeed does this
 * function clear the previous alias holder's `alias` (the actual transfer).
 * This ordering is load-bearing: the original implementation cleared the
 * holder's alias as a side effect of `reserveAgentAlias` itself, so a
 * subsequent `evictForCapacity` rejection (e.g. every other record is
 * running/threadBound/live) still left the spawn un-registered but had
 * already destroyed the previous holder's alias, making it unresolvable by
 * name even though its record was never touched otherwise. Exported for
 * direct unit coverage of the exact ordering guarantee without a real
 * `RpcClient`.
 */
export function runSpawnGuards(
  registry: RpcAgentRegistry,
  alias: string | undefined,
  cap: number,
): { ok: true; evictedLabel?: string } | { ok: false; error: string } {
  const aliasReservation = reserveAgentAlias(registry, alias);
  if (!aliasReservation.ok) return aliasReservation;
  const eviction = evictForCapacity(registry, cap);
  if (!eviction.ok) return eviction;
  if (aliasReservation.holder) {
    aliasReservation.holder.alias = undefined;
  }
  return eviction;
}

/**
 * Spawns a persistent `RpcClient` child from an already-rendered system
 * prompt file. Unlike the Phase 2-3 spawner, this performs **no**
 * `playbook.render` call itself (D-A): the caller (the lead) renders the
 * playbook and passes the resulting path directly as `systemPromptPath`.
 *
 * `modelBase` resolves `model_name`-first through `config.resolve_agent`
 * (`resolveModelForAliasViaWsMcp`), falling back to `ctx.inheritModel`
 * unchanged when `model_name` is unset or resolves to a non-genuine `pi`
 * answer. A config-resolved `effort` is folded into `record.modelEffort` via
 * `effectiveModelEffort` — an explicit, non-empty caller `params.modelEffort`
 * always wins. `record.modelEffort` is then the single value both the
 * spawn-time and dormant-resume `applyModelEffort` calls apply — neither
 * re-reads `params.modelEffort` directly.
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
 *
 * 260905 (alias/park/cap ticket): the very first thing this function does is
 * call `runSpawnGuards` (alias reservation + cap eviction, committed only
 * once both succeed) — see that function's doc comment for why the ordering
 * there is load-bearing. A guard rejection throws before any of the above
 * side effects (`mkdtempSync`, `randomUUID`, `registry.set`) run, so a
 * rejected spawn leaves no trace and no other record touched.
 */
export async function spawnAgent(
  registry: RpcAgentRegistry,
  ctx: RpcSpawnCtx,
  params: SpawnAgentParams,
): Promise<{ agent_id: string; alias?: string; evicted?: string }> {
  // Guard clauses first, before any side effect (mkdtempSync/randomUUID) and
  // before `registry.set` — a rejected spawn leaves no trace, including on
  // the previous alias holder (see `runSpawnGuards`'s doc comment).
  const eviction = runSpawnGuards(registry, params.alias, resolveAgentRegistryCap());
  if (!eviction.ok) {
    throw new Error(eviction.error);
  }

  const agentId = randomUUID();
  const sessionDir = mkdtempSync(join(tmpdir(), "ws-pi-agent-"));
  const sessionPath = join(sessionDir, "session.jsonl");

  const { model: modelBase, effort: resolvedEffort } = await resolveModelForAliasViaWsMcp(ctx.client, params.modelName, ctx.inheritModel);

  const toolGroup: ToolGroup = resolveSpawnToolGroup(ctx.toolGroup);
  const tools = ctx.explicitTools ?? resolveTools(toolGroup, ctx.wsToolNames);
  const record: RpcAgentRecord = {
    agentId,
    alias: params.alias,
    title: params.title,
    sessionPath,
    systemPromptPath: params.systemPromptPath,
    modelBase,
    // Explicit caller effort always wins over the config-resolved one
    // (effectiveModelEffort's `||` semantics — an empty-string caller value
    // is treated as absent). This is the single fold point: both the
    // spawn-time and dormant-resume `applyModelEffort` calls read
    // `record.modelEffort` back rather than re-deriving it from `params`.
    modelEffort: effectiveModelEffort(params.modelEffort, resolvedEffort),
    wsToolNames: ctx.wsToolNames,
    toolGroup,
    explicitTools: ctx.explicitTools,
    spawnRole: ctx.spawnRole ?? (ctx.forkFrom ? "fork" : toolGroup === "execute-worker" ? "execute-worker" : "worker"),
    streaming: false,
    running: false,
    reportLog: [],
    prompt: truncatePromptForStorage(params.prompt),
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

    // Read the already-folded record value, not params.modelEffort directly
    // — record.modelEffort is the single source of truth for what effort a
    // spawned/resumed child should receive (see effectiveModelEffort above
    // and the dormant-resume call site in sendToAgent, which reads the same
    // field).
    await applyModelEffort(client, record.modelEffort);
    attachEventListener(ctx.pi, registry, record, client, ctx.onApprovalPending);
    await promptAgent(record, client, params.prompt);
  } catch (err) {
    pushSpawnFailed(ctx.pi, registry, record, err);
    throw err;
  }

  // 260905 (live-agent widget ticket): a brand-new registry member, live and
  // running from its initial prompt — the widget's first sighting of it.
  triggerAgentWidgetRefresh();
  return { agent_id: agentId, alias: record.alias, evicted: eviction.evictedLabel };
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
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid through the one
  // shared helper first; an unresolvable input falls back to the original
  // string so the existing "unknown agentId" error path is unchanged.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
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
    return { agent_id: record.agentId };
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
  return { agent_id: record.agentId };
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
 *
 * 260905 (alias/park/cap ticket) adds `alias`/`title` on every row (both
 * omitted when unset), and an opt-in `opts.includePrompt` that additionally
 * includes the record's stored (already head-truncated at spawn) `prompt`.
 * Off by default — a full-registry dump with every prompt inlined would be
 * needlessly large for the common "what's out there" check.
 *
 * 260905 (list-model/last-report-fidelity ticket) adds `model`: the
 * effective resolved model the child was launched with (`modelBase`, plus
 * `/<effort>` when `modelEffort` is set), omitted only when the record
 * carries no `modelBase` at all — either a sidecar written before the field
 * existed, or a fresh spawn whose `resolveModelForAliasViaWsMcp` fallback
 * (`ctx.inheritModel`, via `inheritModelFromToolCtx`) itself came back
 * `undefined` because the tool-context model was absent or malformed.
 * Because that resolver otherwise falls back to the parent's own concrete
 * model on any catalog miss, an inheriting child's `modelBase` already IS
 * the parent's model name — no separate inherited-vs-explicit flag is
 * needed. `last_report_at` now falls back to `record.lastReportAtOverride`
 * (set only by `rehydrateOrphanRecord`) when `reportLog` is empty, so a
 * revived orphan does not read as never-reported.
 */
export function listAgents(
  registry: RpcAgentRegistry,
  opts?: { includePrompt?: boolean },
): Array<{ agent_id: string; status: AgentStatus; alias?: string; title?: string; model?: string; last_report_at?: string; prompt?: string }> {
  return [...registry.entries()].map(([agentId, record]) => {
    const lastReport = record.reportLog[record.reportLog.length - 1];
    const lastReportAt = lastReport ? new Date(lastReport.at).toISOString() : record.lastReportAtOverride;
    const model = record.modelBase ? (record.modelEffort ? `${record.modelBase}/${record.modelEffort}` : record.modelBase) : undefined;
    return {
      agent_id: agentId,
      status: (record.client ? (record.streaming ? "running" : "idle") : "dormant") as AgentStatus,
      ...(record.alias ? { alias: record.alias } : {}),
      ...(record.title ? { title: record.title } : {}),
      ...(model ? { model } : {}),
      ...(lastReportAt ? { last_report_at: lastReportAt } : {}),
      ...(opts?.includePrompt && record.prompt ? { prompt: record.prompt } : {}),
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
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid first — see
  // `sendToAgent`'s identical resolve-then-`.get()` shape.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  const client = record.client;
  if (client) {
    // Review relay #1 (Important, alias/park/cap): clear live state
    // SYNCHRONOUSLY, before either await below, not after both resolve. A
    // record that still reads `client`-live during `abort()`/`stop()` is a
    // race window: a concurrent `ws-agent-send` (or overlay `ForkChannel`
    // send) sees a live-idle record and calls `promptAgent` on a client
    // that's mid-teardown, losing the turn silently once `stop()` lands.
    // Clearing here first means that same racing send instead takes
    // `sendToAgent`'s dormant-resume branch, exactly as if this record had
    // already finished parking — the automatic park path (which now runs
    // after every settle, not just on an explicit stop) makes this window
    // hot enough to close rather than accept.
    clearLiveState(record);
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
    // 260905 (live-agent widget ticket): the record just left the live state
    // (or lost its thread bind) — either way a widget-relevant transition.
    triggerAgentWidgetRefresh();
  }
  return { agent_id: record.agentId };
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
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid first — see
  // `sendToAgent`'s identical resolve-then-`.get()` shape.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
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
  sessionCtx: { cwd: string },
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
   * IO wrapper around `resolveModelForAliasViaWsMcp` for `explore`'s implicit
   * `"small"` lookup: re-resolves through `config.resolve_agent` fresh on
   * every call (no caching) so a `config.tune agents.tier harness:pi` edit
   * applies without restarting Pi, matching bridge.ts's advisory's
   * no-caching choice.
   */
  async function resolveExploreModel(toolCtx: unknown): Promise<string | undefined> {
    const { model } = await resolveModelForAliasViaWsMcp(bridge.client, "small", inheritModelFromToolCtx(toolCtx));
    return model;
  }

  pi.registerTool({
    name: "ws-agent-spawn",
    label: "ws-agent-spawn",
    description:
      "Spawn a persistent RPC-backed pi subagent from an already-rendered system-prompt file (e.g. via ws/playbook.render). Returns {agent_id, alias?, evicted?} immediately after the initial prompt is sent. Do not wait for it: end your turn, and its reports, questions and completion arrive on their own as ws-agent-* messages carrying a running-count status line.",
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
            "Optional tier name (small|medium|large|xlarge) resolved against harness pi's config.tune agents.tier entries (see lead-tune/config.list); omitted or unmapped inherits the parent session's model.",
        },
        model_effort: {
          type: "string",
          description:
            "Optional Pi thinking level (off|minimal|low|medium|high|xhigh|max), applied via setThinkingLevel after start; an unsupported value degrades to a no-op.",
        },
        alias: {
          type: "string",
          description:
            "Optional short name for this agent, usable in place of agent_id on ws-agent-send/stop/transcript/ws-approve. Reusing an alias held by a running/threadBound agent rejects this spawn; a dormant/idle holder's alias is overwritten (its title is kept). Never derived automatically — omit to address this agent by uuid only.",
        },
        title: {
          type: "string",
          description: "Optional free-text label for this agent, independent of alias (display only, never used for resolution).",
        },
      },
      required: ["system_prompt_path", "prompt"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as {
        system_prompt_path: string;
        prompt: string;
        model_name?: string;
        model_effort?: string;
        alias?: string;
        title?: string;
      };
      const result = await spawnAgent(
        rpcRegistry,
        {
          pi,
          cwd: sessionCtx.cwd,
          inheritModel: inheritModelFromToolCtx(toolCtx),
          wsToolNames: bridge.wsToolNames,
          client: bridge.client,
          onApprovalPending,
        },
        {
          systemPromptPath: p.system_prompt_path,
          prompt: p.prompt,
          modelName: p.model_name,
          modelEffort: p.model_effort,
          alias: p.alias,
          title: p.title,
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
      "List every tracked agent_id, its alias/title (when set), status (running/idle/dormant — most agents park to dormant shortly after settling, so idle is transient), model (the model the agent runs on; an inheriting child shows its parent's model), and last_report_at (ISO, absent if it has never reported). Use it to check on a quiet agent — there is no wait tool; every report, question, approval request and completion is pushed to you as a ws-agent-* message on its own.",
    parameters: {
      type: "object",
      properties: {
        include_prompt: {
          type: "boolean",
          description: "When true, also include each agent's stored (head-truncated) initial prompt. Off by default.",
        },
      },
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { include_prompt?: boolean };
      const result = listAgents(rpcRegistry, { includePrompt: p.include_prompt });
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
        { sessionKey: bridge.defaultSessionKeyRef.current ?? "", cwd: sessionCtx.cwd, model: await resolveExploreModel(toolCtx) },
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
