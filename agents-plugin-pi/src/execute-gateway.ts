/**
 * 260904 Phase 1: end-to-end `ws.execute`/`ws.approve` approval gateway.
 *
 * §2 invariant (verbatim, carried into the lead guide's verb table and this
 * module's tool descriptions): the gate exists because `ws.execute` proxies
 * lead-consensus-caliber actions; general `ws-agent-spawn` workers carry no
 * such gate.
 *
 * Delivers the whole gateway through the "no special harness support"
 * fallback relay (§8 baseline):
 *   - `ws-worker-exec` (`GATED_EXEC_TOOL_NAME`, spawner.ts): the gated-exec
 *     worker tool — every free-form command an `execute-worker` runs,
 *     including a "read" that mutates via redirection/`-exec`, elevates
 *     through this tool and pauses for lead approval. Combined with the
 *     mutation-incapable `read-only` builtins (spawner.ts's `TOOL_GROUPS`),
 *     this is §5's "gated-exec worker tool + mutation-incapable read-family"
 *     `--tools` allowlist.
 *   - `ws-execute`: lead-facing. Spawns an `execute-worker` (`toolGroup:
 *     "execute-worker"`, spawner.ts's `spawnAgent`), optionally running
 *     `command` verbatim first (in the LEAD's own trusted process — no gate,
 *     since the lead itself already supplied this exact string as a
 *     tool-call param, already at lead/user-consensus trust per §2) and
 *     handing `{command, output}` to the worker as context. `complex?`
 *     inherits the lead's own model (no `model_name` passed) instead of the
 *     default `"small"` alias. Returns `{agent_id}` immediately — never
 *     awaits worker completion (fire-and-return, §3).
 *   - `ws-approve`: lead-facing. `{agent_id, cmd_id, decision, reason?,
 *     command?}` -> `approve`/`deny(reason)`/`run-instead(command)`. Race-
 *     binds against the SAME agent's `record.pendingApproval` (`cmd_id`
 *     race-binding, §3/§8): rejects when there is no pending approval, or
 *     the given `cmd_id` doesn't match the one currently pending.
 *   - The approval-request relay (child -> lead): a spawned `execute-worker`
 *     child's `tool_execution_start` for `GATED_EXEC_TOOL_NAME` is already
 *     forwarded to this process's `RpcClient.onEvent()` stream and captured
 *     onto `record.pendingApproval` by `applyRpcEvent` (spawner.ts) — no new
 *     transport. `createApprovalRelay` below is the callback
 *     `spawner.ts`'s `attachEventListener` invokes right after that capture:
 *     it scrapes ground-truth working-directory context
 *     (`scrapeWorkingContext`, adapter-side, NOT worker-reported, per §7),
 *     formats the §7 payload (`buildApprovalPromptText`), and pushes it into
 *     the LEAD's own running session as a `ws-agent-approval` custom message
 *     (`pushToLead`, spawner.ts) delivered as `steer`. `steer` (not
 *     `followUp`) is used because this callback fires from a raw
 *     `RpcClient.onEvent()` listener with no `ExtensionContext` available to
 *     check `isIdle()`, and `steer` is valid whether or not the lead is
 *     currently streaming (docs/rpc.md) — an approval request is the one
 *     child signal that must interrupt rather than queue, since the child
 *     cannot progress at all until it is answered. 260905: this push is now
 *     unconditional (the `info.waiterWoken` suppression is gone along with
 *     `ws-agent-wait`); it is the sole notification path for the lead.
 *   - The decision relay (lead -> child): `ws-approve` writes
 *     `<sessionDir>/approvals/<cmd_id>.decision.json`
 *     (`approvalDecisionPath`); `ws-worker-exec`'s blocked `execute()` polls
 *     for that exact file (`waitForDecisionFile`) using the approvals
 *     directory the child learned at spawn time via the
 *     `WS_PI_APPROVAL_DIR` env var (spawner.ts's `buildRpcClientOptions`).
 *     This two-file-polling design is necessary, not incidental: Pi's RPC
 *     surface has no channel that can resolve an IN-FLIGHT tool call —
 *     `steer`/`followUp` are both turn-boundary-only (rpc.md), so nothing
 *     short of a side-channel (here, the filesystem) can unblock the
 *     specific `execute()` promise the gated-exec tool call is holding open.
 *   - `computeLeadActiveTools`: the §8 lead `--tools` reshaping — removes
 *     native `bash`/`read` and (footgun fix, see spawner.ts's Codebase
 *     Findings) `GATED_EXEC_TOOL_NAME` itself from the lead's active list
 *     (a lead-invoked `ws-worker-exec` call would otherwise hang forever
 *     waiting on a decision file nobody will ever write, since nothing
 *     observes the lead's OWN `tool_execution_start` the way a parent
 *     observes a spawned child's), and adds `ws-execute`/`ws-approve`/
 *     `UGLY_READ_TOOL_NAME` if not already present. Applied in index.ts via
 *     `pi.setActiveTools(computeLeadActiveTools(pi.getActiveTools()))`,
 *     gated on `isLeadOrFork` — see that file's `session_start` handler.
 *   - `UGLY_READ_TOOL_NAME`
 *     (`do-i-really-have-to-read-this-myself`, the ticket's own proposed
 *     name, adopted verbatim): a plain file-read tool retained for the lead
 *     once native `read` is removed from its active list — "ugly-named read
 *     retained," not eliminated, per the ticket's lead `--tools` change.
 *
 * Following spawner.ts's own convention (not discuss.ts's single-call-site
 * one): this file mixes pure, unit-tested helpers (test/execute-gateway.test.ts)
 * with the `registerExecuteGateway`/`createApprovalRelay` IO glue, since its
 * IO surface (3 tools + 1 injection callback) is closer in shape to
 * spawner.ts than to discuss.ts.
 *
 * Golden rule: imports FROM spawner.ts only (`spawnAgent`,
 * `inheritModelFromToolCtx`, `GATED_EXEC_TOOL_NAME`, `WS_PI_APPROVAL_DIR_ENV`,
 * types) — spawner.ts never imports from this file, keeping it generic (no
 * `pi.sendUserMessage` dependency there). `agents-plugin-tool/` (ws-mcp Go)
 * and `agents-plugin/skills/` canonical text are both untouched by this
 * ticket.
 */

import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, isAbsolute, join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { BridgeHandle } from "./bridge.ts";
import {
  GATED_EXEC_TOOL_NAME,
  WS_PI_APPROVAL_DIR_ENV,
  inheritModelFromToolCtx,
  pushToLead,
  resolveAgentId,
  spawnAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "./spawner.ts";

// ---------------------------------------------------------------------------
// Pure helpers. Unit-tested directly (test/execute-gateway.test.ts) with no
// filesystem/subprocess/live `pi` session involved.
// ---------------------------------------------------------------------------

/** Lead-facing verb-table tool names (pi-lead-guide.md), registered below. */
export const EXECUTE_TOOL_NAME = "ws-execute";
export const APPROVE_TOOL_NAME = "ws-approve";

/**
 * The ugly-named read tool the ticket itself proposes verbatim — kept
 * available to the lead once native `read` is removed from its active-tools
 * list (§8: "retained," not eliminated).
 */
export const UGLY_READ_TOOL_NAME = "do-i-really-have-to-read-this-myself";

export interface ExecuteWorkerPromptInput {
  /** Verbatim command the lead already ran (via `ws-execute`'s own `command?`), or omitted when `ws-execute` was called with only a `prompt`. */
  command?: string;
  /** That command's captured stdout+stderr, present exactly when `command` is. */
  output?: string;
  /** The lead's task prompt for the spawned execute-worker. */
  prompt: string;
}

/**
 * Pure composer for the initial prompt handed to a `ws-execute`-spawned
 * `execute-worker`: when the lead pre-ran `command` verbatim (in its own
 * trusted process, no gate — §2/§3), prefixes a "verbatim command already
 * run" context block ahead of the lead's own `prompt`; otherwise returns
 * `prompt` unchanged.
 */
export function buildExecuteWorkerPrompt(input: ExecuteWorkerPromptInput): string {
  if (input.command === undefined) {
    return input.prompt;
  }
  const outputBlock = input.output ?? "";
  return [
    "Verbatim command already run (by the lead, before spawning you):",
    "```",
    input.command,
    "```",
    "Output:",
    "```",
    outputBlock,
    "```",
    "",
    input.prompt,
  ].join("\n");
}

/**
 * Pure model-tier selection for `ws-execute`'s `complex?` param: `true`
 * returns `undefined` (no `model_name` at all), so `spawnAgent`'s existing
 * inherit-fallback path (`resolveModelForAliasViaWsMcp` in spawner.ts)
 * resolves to the lead's own model instead of a `config.resolve_agent`
 * round-trip — `complex:true` was always meant to inherit the lead's model
 * unconditionally, not by tier-miss accident. `false`/omitted still picks the
 * existing `"small"` tier (the same key `explore` already resolves through)
 * — zero new curation burden for a user who already configured `small` for
 * harness `pi`.
 */
export function resolveExecuteModelAlias(complex?: boolean): string | undefined {
  return complex ? undefined : "small";
}

export interface PendingApproval {
  cmdId: string;
  command: string;
  rationale?: string;
  /** Worker-supplied per-call cwd override (mirrors `ws-worker-exec`'s own `cwd?` param) — see `resolveApprovalContextCwd`. */
  cwd?: string;
}

export type ValidatePendingApprovalResult = { ok: true } | { ok: false; reason: string };

/**
 * Pure `cmd_id` race-binding check — the ticket's own race-prevention
 * requirement (§3/§8's Phase 1 verification boundary: "`cmd_id` stale-
 * rejection"). Rejects when there is no pending approval on the agent at
 * all, or when the given `cmdId` doesn't match the one actually pending
 * (stale/reused/wrong-agent `cmd_id`); accepts only on an exact match.
 */
export function validatePendingApproval(pending: PendingApproval | undefined, cmdId: string): ValidatePendingApprovalResult {
  if (!pending) {
    return { ok: false, reason: "no pending approval for this agent" };
  }
  if (pending.cmdId !== cmdId) {
    return { ok: false, reason: `cmd_id mismatch: pending cmd_id is "${pending.cmdId}", got "${cmdId}"` };
  }
  return { ok: true };
}

const LEAD_REMOVED_TOOL_NAMES: ReadonlySet<string> = new Set(["bash", "read", GATED_EXEC_TOOL_NAME]);
const LEAD_ADDED_TOOL_NAMES: readonly string[] = [EXECUTE_TOOL_NAME, APPROVE_TOOL_NAME, UGLY_READ_TOOL_NAME];

/**
 * Pure §8 lead `--tools` reshaping: removes native `bash`/`read` and (the
 * auto-include footgun fix, see spawner.ts's Codebase Findings for the full
 * trace) `GATED_EXEC_TOOL_NAME` from `currentActive`, then adds
 * `ws-execute`/`ws-approve`/the ugly-read tool if not already present.
 * Deduped; order is otherwise preserved (kept names first, then any newly
 * added names).
 *
 * The `GATED_EXEC_TOOL_NAME` exclusion is load-bearing, not defensive
 * dressing: `ws-worker-exec` must be registered globally (so a worker's own
 * `pi -e` process, loading the same extension, can activate it via
 * `--tools`), which means Pi's "auto-include every newly-registered tool"
 * behavior on a fresh (non-`setActiveTools`-called-yet) session would
 * otherwise silently make it active on the LEAD's own session too — letting
 * the lead call the worker-only gated-exec tool directly, bypassing
 * `ws-execute`/`ws-approve` entirely, and hanging forever (nothing observes
 * the lead's own `tool_execution_start` the way a parent observes a spawned
 * child's `RpcClient.onEvent()` stream).
 */
export function computeLeadActiveTools(currentActive: readonly string[]): string[] {
  const kept = currentActive.filter((name) => !LEAD_REMOVED_TOOL_NAMES.has(name));
  const result = [...kept];
  for (const name of LEAD_ADDED_TOOL_NAMES) {
    if (!result.includes(name)) {
      result.push(name);
    }
  }
  return [...new Set(result)];
}

export interface WorkingContext {
  cwd: string;
  worktree_root?: string;
  branch?: string;
  /** `"<ahead>/<behind>"` relative to the configured upstream, or undefined when there is none/it can't be determined. */
  ahead_behind?: string;
  dirty?: boolean;
}

export interface ApprovalPayload {
  agent_id: string;
  cmd_id: string;
  command: string;
  rationale?: string;
  context: WorkingContext;
}

/**
 * Pure §7 payload formatter: renders `payload` into the text handed to
 * `pi.sendUserMessage`, instructing the lead to call `ws-approve`. Omits any
 * `context` field that is `undefined` (a best-effort git scrape that
 * couldn't determine it) rather than printing a misleading blank.
 */
export function buildApprovalPromptText(payload: ApprovalPayload): string {
  const ctx = payload.context;
  const contextLines = [
    `  cwd: ${ctx.cwd}`,
    ctx.worktree_root !== undefined ? `  worktree_root: ${ctx.worktree_root}` : undefined,
    ctx.branch !== undefined ? `  branch: ${ctx.branch}` : undefined,
    ctx.ahead_behind !== undefined ? `  ahead_behind: ${ctx.ahead_behind}` : undefined,
    ctx.dirty !== undefined ? `  dirty: ${ctx.dirty}` : undefined,
  ].filter((line): line is string => line !== undefined);

  const lines = [
    `A spawned execute-worker (agent_id: ${payload.agent_id}) wants to run a shell command and needs your approval.`,
    "",
    `cmd_id: ${payload.cmd_id}`,
    `command: ${payload.command}`,
    payload.rationale !== undefined ? `rationale: ${payload.rationale}` : undefined,
    "",
    "context:",
    ...contextLines,
    "",
    `Call ${APPROVE_TOOL_NAME} with {agent_id: "${payload.agent_id}", cmd_id: "${payload.cmd_id}", decision: "approve"|"deny"|"run-instead", reason?, command?} to respond. A stale or mismatched cmd_id is rejected.`,
  ].filter((line): line is string => line !== undefined);

  return lines.join("\n");
}

/**
 * Pure path builder for the decision file both `ws-approve` (writer, parent
 * side) and `ws-worker-exec` (reader, child side — via its own
 * `WS_PI_APPROVAL_DIR`-derived approvals dir) agree on:
 * `<sessionDir>/approvals/<cmdId>.decision.json`.
 */
export function approvalDecisionPath(sessionDir: string, cmdId: string): string {
  return join(sessionDir, "approvals", `${cmdId}.decision.json`);
}

export interface ApprovalDecision {
  decision: "approve" | "deny" | "run-instead";
  reason?: string;
  command?: string;
}

/**
 * Pure cwd-fallback selector for the approval-relay's ground-truth context
 * scrape (review fix, relay #1, CORRECTNESS finding #1): a worker-supplied
 * per-call `cwd` override (`pending.cwd`, captured onto `record.pendingApproval`
 * by `spawner.ts`'s `applyRpcEvent`) takes precedence over the session's base
 * `sessionCwd`, so `createApprovalRelay` scrapes the SAME directory the
 * command will actually run in — matching `ws-worker-exec`'s own `execute()`,
 * which resolves the command's cwd as `p.cwd ?? sessionCtx.cwd`. Extracted
 * (rather than inlined in `createApprovalRelay`) purely for direct unit
 * coverage, since `createApprovalRelay` itself is IO (closes over `pi`).
 */
export function resolveApprovalContextCwd(pending: { cwd?: string }, sessionCwd: string): string {
  return pending.cwd ?? sessionCwd;
}

export type ValidateApprovalDecisionResult = { ok: true } | { ok: false; reason: string };

/**
 * Pure required-field validation for `ws-approve`'s `execute()` (review fix,
 * relay #1, CORRECTNESS finding #2): the tool's own parameter schema already
 * documents `reason` as "Required context for decision:deny" and `command`
 * as needed for `decision:run-instead`, but nothing previously enforced
 * either before writing the decision file — an empty/omitted `command` on
 * `run-instead` silently fell through to `ws-worker-exec`'s own `p.command`
 * fallback (treating it as a no-op approve), and an empty/omitted `reason`
 * on `deny` produced a confusing blank-reason denial. Checked BEFORE
 * `validatePendingApproval`'s race-binding gate is acted on (i.e. before any
 * decision file is written), rejecting with a reason a caller can act on.
 * Whitespace-only values are treated the same as missing (`.trim()`).
 */
export function validateApprovalDecisionInput(decision: "approve" | "deny" | "run-instead", reason: string | undefined, command: string | undefined): ValidateApprovalDecisionResult {
  if (decision === "run-instead" && (!command || command.trim().length === 0)) {
    return { ok: false, reason: "decision \"run-instead\" requires a non-empty command" };
  }
  if (decision === "deny" && (!reason || reason.trim().length === 0)) {
    return { ok: false, reason: "decision \"deny\" requires a non-empty reason" };
  }
  return { ok: true };
}

/**
 * Pure line-slicing logic for the ugly-named read tool (review fix, relay
 * #1, TEST finding #4): extracted out of that tool's `execute()` body so it
 * has direct unit coverage without a live `pi` session. 1-indexed `offset`
 * (matching the tool's own param description); `limit` caps the number of
 * lines returned. Both are optional — omitting both returns the whole file.
 */
export function sliceLines(raw: string, offset?: number, limit?: number): string {
  const lines = raw.split("\n");
  const start = offset ? Math.max(0, offset - 1) : 0;
  const end = limit !== undefined ? Math.min(start + limit, lines.length) : lines.length;
  return lines.slice(start, end).join("\n");
}

/**
 * IO: best-effort git ground-truth scrape of `cwd`'s working context for the
 * §7 payload's `context` field — adapter-scraped, NOT worker-reported (§7).
 * Runs on the PARENT side, same machine/filesystem as the worker's `cwd`
 * (the parent already knows it from spawn time). Each git call is
 * independently try/caught and NEVER throws — matches this adapter's
 * never-hard-fail convention; a non-git `cwd` or missing `git` binary simply
 * degrades every field to `undefined` (except `cwd` itself, always present).
 */
export function scrapeWorkingContext(cwd: string): WorkingContext {
  const worktree_root = tryGit(["rev-parse", "--show-toplevel"], cwd);
  const branch = tryGit(["rev-parse", "--abbrev-ref", "HEAD"], cwd);
  const aheadBehindRaw = tryGit(["rev-list", "--left-right", "--count", "@{upstream}...HEAD"], cwd);
  const ahead_behind = aheadBehindRaw ? aheadBehindRaw.split(/\s+/).join("/") : undefined;
  const statusRaw = tryGit(["status", "--porcelain"], cwd);
  const dirty = statusRaw !== undefined ? statusRaw.length > 0 : undefined;
  return { cwd, worktree_root, branch, ahead_behind, dirty };
}

function tryGit(args: string[], cwd: string): string | undefined {
  try {
    const raw = execFileSync("git", args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] });
    return raw.trim();
  } catch {
    return undefined;
  }
}

/**
 * IO: polls (fixed-interval, cleared on resolve) for `path` to appear,
 * resolving its parsed JSON contents as soon as it does. Also resolves
 * early with `"aborted"` on `signal`'s `"abort"` event — the abort-unblocks-
 * execute path (`ws-agent-stop` -> `client.abort()` -> this tool call's own
 * `AbortSignal` fires). Mirrors the installed package's own `exec.js`
 * `execCommand` abort-listener pattern.
 *
 * Review fix (relay #1, TEST finding #5): despite living next to genuinely
 * live-gate-only code, this function itself needs only a real filesystem +
 * timers — no subprocess, model, or `RpcClient` — so it IS unit-tested
 * directly (test/execute-gateway.test.ts), using a real temp directory, a
 * delayed `writeFileSync`, and a real/aborted `AbortController`.
 */
export function waitForDecisionFile(path: string, signal: AbortSignal | undefined, pollMs = 200): Promise<ApprovalDecision | "aborted"> {
  return new Promise((resolve) => {
    let settled = false;
    let timer: ReturnType<typeof setInterval> | undefined;

    const finish = (result: ApprovalDecision | "aborted") => {
      if (settled) return;
      settled = true;
      if (timer) clearInterval(timer);
      signal?.removeEventListener("abort", onAbort);
      resolve(result);
    };
    const onAbort = () => finish("aborted");

    if (signal) {
      if (signal.aborted) {
        finish("aborted");
        return;
      }
      signal.addEventListener("abort", onAbort);
    }

    timer = setInterval(() => {
      if (!existsSync(path)) return;
      try {
        const raw = readFileSync(path, "utf8");
        finish(JSON.parse(raw) as ApprovalDecision);
      } catch {
        // File may still be mid-write (partial JSON) — try again next tick.
      }
    }, pollMs);
  });
}

// ---------------------------------------------------------------------------
// IO glue: tool registration + the approval-request injection callback.
// ---------------------------------------------------------------------------

export interface ExecuteGatewaySessionCtx {
  cwd: string;
  /** Fixed, adapter-authored execute-worker system prompt (execute-worker-guide.md) — NOT lead-rendered, see that file's header comment. */
  executeWorkerPromptPath: string;
  /** See spawner.ts's `RpcSpawnCtx.onApprovalPending` — threaded into every `spawnAgent` call this module makes for `ws-execute`. */
  onApprovalPending?: (record: RpcAgentRecord) => void;
}

/**
 * Builds the approval-request-relay callback: reacts to a freshly-set
 * `record.pendingApproval` (spawner.ts's `applyRpcEvent`) by scraping ground-
 * truth working-directory context, formatting the §7 payload, and pushing it
 * into the LEAD's own running session as a `ws-agent-approval` custom message
 * delivered as `steer` — see this file's header comment for the full design
 * trace. Constructed once in index.ts's `session_start` handler (before
 * `registerAgentTools`, so it can be threaded into that call too — see
 * spawner.ts's `registerAgentTools` doc comment) and passed to both
 * `registerAgentTools` and `registerExecuteGateway`.
 *
 * `registryRef` is a mutable holder rather than the registry itself because
 * the relay must exist BEFORE `registerAgentTools` creates that registry; the
 * ref is filled immediately afterwards. Its only use is the fan-in status
 * line every push carries, so a still-empty ref degrades to no status line,
 * never to a crash.
 *
 * 260905: the push is unconditional. The former `info.waiterWoken`
 * suppression existed only because a blocked `ws-agent-wait` was a second
 * notification path; with that tool deleted, this push is the only way the
 * lead ever learns a child is blocked on approval.
 */
export function createApprovalRelay(
  pi: ExtensionAPI,
  sessionCtx: { cwd: string },
  registryRef?: { current: RpcAgentRegistry | undefined },
): (record: RpcAgentRecord) => void {
  return (record) => {
    const pending = record.pendingApproval;
    if (!pending) return;
    const context = scrapeWorkingContext(resolveApprovalContextCwd(pending, sessionCtx.cwd));
    const text = buildApprovalPromptText({
      agent_id: record.agentId,
      cmd_id: pending.cmdId,
      command: pending.command,
      rationale: pending.rationale,
      context,
    });
    pushToLead(pi, registryRef?.current, record, "ws-agent-approval", { cmd_id: pending.cmdId, request: text }, "steer");
  };
}

/**
 * Registers `ws-worker-exec` (child-side, gated), `ws-execute` (lead-side,
 * spawns an `execute-worker`), `ws-approve` (lead-side, resolves a pending
 * gated command), and the ugly-named read tool. All four are registered
 * declaratively/globally (same pattern as `ws-report-to-lead`,
 * spawner.ts) — `ws-worker-exec` is reachable only from an `execute-worker`
 * child's own `--tools` list (`TOOL_GROUPS`, spawner.ts); `ws-execute`/
 * `ws-approve`/the ugly-read tool are reachable by the lead only after
 * index.ts's `pi.setActiveTools(computeLeadActiveTools(...))` call (§8).
 *
 * Call once, unconditionally, from index.ts's `session_start` handler —
 * mirrors `registerAgentTools`'s own unconditional call (the tools are
 * simply never exposed to a worker/explore's own `--tools`).
 */
export function registerExecuteGateway(pi: ExtensionAPI, bridge: BridgeHandle, rpcRegistry: RpcAgentRegistry, sessionCtx: ExecuteGatewaySessionCtx): void {
  pi.registerTool({
    name: GATED_EXEC_TOOL_NAME,
    label: GATED_EXEC_TOOL_NAME,
    description:
      "Run a shell command that must first be approved by the lead. Every free-form command elevates through this gate — including a \"read\" that mutates via redirection or -exec. Blocks until the lead calls ws-approve with a matching cmd_id; on deny, re-plan and resubmit a revised command; on run-instead, treat the substituted command's output as authoritative.",
    parameters: {
      type: "object",
      properties: {
        command: { type: "string", description: "Shell command to run (sh -c semantics: &&, redirection, pipes, etc. all work)." },
        rationale: { type: "string", description: "Why this command is needed — shown to the lead as part of the approval request." },
        cwd: { type: "string", description: "Optional working directory override for this command; defaults to the worker's own cwd." },
      },
      required: ["command", "rationale"],
    } as never,
    async execute(toolCallId, params, signal) {
      const p = params as { command: string; rationale: string; cwd?: string };
      const approvalDir = process.env[WS_PI_APPROVAL_DIR_ENV];
      if (!approvalDir) {
        throw new Error(`ws-pi-agent: ${GATED_EXEC_TOOL_NAME}: ${WS_PI_APPROVAL_DIR_ENV} is unset — this tool only runs inside a ws-execute-spawned execute-worker.`);
      }
      const decisionPath = join(approvalDir, `${toolCallId}.decision.json`);
      const outcome = await waitForDecisionFile(decisionPath, signal);

      if (outcome === "aborted") {
        return { content: [{ type: "text", text: "Aborted (ws-agent-stop) before the lead responded." }] };
      }
      if (outcome.decision === "deny") {
        const suffix = outcome.reason ? `: ${outcome.reason}` : ".";
        return { content: [{ type: "text", text: `Lead denied this command${suffix} Re-plan and resubmit a revised command via ${GATED_EXEC_TOOL_NAME}.` }] };
      }

      const runInstead = outcome.decision === "run-instead" && outcome.command;
      const commandToRun = runInstead ? outcome.command! : p.command;
      const execResult = await pi.exec("sh", ["-c", commandToRun], { cwd: p.cwd ?? sessionCtx.cwd, signal });
      const note = runInstead ? "Lead substituted a different command; treat its output below as authoritative.\n\n" : "";
      return {
        content: [
          {
            type: "text",
            text: `${note}exit code: ${execResult.code}\nstdout:\n${execResult.stdout}\nstderr:\n${execResult.stderr}`,
          },
        ],
      };
    },
  });

  pi.registerTool({
    name: EXECUTE_TOOL_NAME,
    label: EXECUTE_TOOL_NAME,
    description:
      "Spawn an execute-worker to carry out `prompt`; every shell command it runs elevates through a lead-approval gate (see ws-approve) because it proxies lead-consensus-caliber actions, unlike a general ws-agent-spawn worker. Optionally runs `command` verbatim FIRST, in your own trusted context (no gate), and hands its output to the worker. complex:true inherits your own model instead of the default light one. Returns {agent_id} immediately — end your turn afterwards; its approval requests, reports and settles arrive as pushed messages.",
    parameters: {
      type: "object",
      properties: {
        command: {
          type: "string",
          description: "Optional shell command to run verbatim FIRST, in your own trusted process (no approval gate) — its {command, output} is handed to the worker as context.",
        },
        prompt: { type: "string", description: "Task prompt for the spawned execute-worker." },
        complex: { type: "boolean", description: "true inherits your own model instead of the default 'small' alias." },
      },
      required: ["prompt"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as { command?: string; prompt: string; complex?: boolean };
      let output: string | undefined;
      if (p.command !== undefined) {
        const execResult = await pi.exec("sh", ["-c", p.command], { cwd: sessionCtx.cwd });
        output = `${execResult.stdout}${execResult.stderr}`;
      }
      const initialPrompt = buildExecuteWorkerPrompt({ command: p.command, output, prompt: p.prompt });
      const result = await spawnAgent(
        rpcRegistry,
        {
          pi,
          cwd: sessionCtx.cwd,
          inheritModel: inheritModelFromToolCtx(toolCtx),
          wsToolNames: bridge.wsToolNames,
          client: bridge.client,
          toolGroup: "execute-worker",
          onApprovalPending: sessionCtx.onApprovalPending,
        },
        {
          systemPromptPath: sessionCtx.executeWorkerPromptPath,
          prompt: initialPrompt,
          modelName: resolveExecuteModelAlias(p.complex),
        },
      );
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });

  pi.registerTool({
    name: APPROVE_TOOL_NAME,
    label: APPROVE_TOOL_NAME,
    description:
      "Respond to a pending ws-worker-exec approval request from a ws-execute-spawned agent. decision:approve runs the command as proposed; deny(reason) rejects it (the worker re-plans); run-instead(command) substitutes a different command whose output the worker treats as authoritative. Rejected if cmd_id doesn't match the currently pending one (stale/reused cmd_id).",
    parameters: {
      type: "object",
      properties: {
        agent_id: { type: "string", description: "agentId of the execute-worker (from ws-execute's {agent_id})." },
        cmd_id: { type: "string", description: "cmd_id from the approval-request prompt; must match the currently pending one." },
        decision: { type: "string", enum: ["approve", "deny", "run-instead"], description: "approve | deny | run-instead." },
        reason: { type: "string", description: "Required context for decision:deny; optional otherwise." },
        command: { type: "string", description: "Substituted command for decision:run-instead; ignored otherwise." },
      },
      required: ["agent_id", "cmd_id", "decision"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { agent_id: string; cmd_id: string; decision: "approve" | "deny" | "run-instead"; reason?: string; command?: string };
      // 260905 (alias/park/cap ticket): the fourth alias-or-uuid resolution
      // call site, alongside sendToAgent/stopAgent/getAgentTranscriptPath in
      // spawner.ts.
      const resolvedAgentId = resolveAgentId(rpcRegistry, p.agent_id) ?? p.agent_id;
      const record = rpcRegistry.get(resolvedAgentId);
      const validation = validatePendingApproval(record?.pendingApproval, p.cmd_id);
      if (!validation.ok || !record) {
        const reason = validation.ok ? "unknown agent_id" : validation.reason;
        throw new Error(`ws-pi-agent: ${APPROVE_TOOL_NAME} rejected: ${reason}`);
      }
      const inputValidation = validateApprovalDecisionInput(p.decision, p.reason, p.command);
      if (!inputValidation.ok) {
        throw new Error(`ws-pi-agent: ${APPROVE_TOOL_NAME} rejected: ${inputValidation.reason}`);
      }

      const sessionDir = dirname(record.sessionPath);
      const decisionPath = approvalDecisionPath(sessionDir, p.cmd_id);
      mkdirSync(dirname(decisionPath), { recursive: true });
      writeFileSync(decisionPath, JSON.stringify({ decision: p.decision, reason: p.reason, command: p.command }));
      record.pendingApproval = undefined;

      return { content: [{ type: "text", text: JSON.stringify({ ok: true }) }] };
    },
  });

  pi.registerTool({
    name: UGLY_READ_TOOL_NAME,
    label: "read",
    description:
      "Read a text file's contents (offset/limit by line, 1-indexed). Kept available under this deliberately unappealing name once native `read` is removed from your active tools — reading files yourself is a fallback, not your first move; prefer delegating to a spawned agent (ws-agent-spawn/ws-execute/explore).",
    parameters: {
      type: "object",
      properties: {
        path: { type: "string", description: "Path to the file to read (relative or absolute)." },
        offset: { type: "number", description: "Line number to start reading from (1-indexed)." },
        limit: { type: "number", description: "Maximum number of lines to read." },
      },
      required: ["path"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { path: string; offset?: number; limit?: number };
      const absolutePath = isAbsolute(p.path) ? p.path : join(sessionCtx.cwd, p.path);
      const raw = readFileSync(absolutePath, "utf8");
      return { content: [{ type: "text", text: sliceLines(raw, p.offset, p.limit) }] };
    },
  });
}
