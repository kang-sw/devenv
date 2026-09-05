/**
 * 260904 Phase 1 (`260904-feat-ws-pi-side-thread-fork-question-surface`):
 * `ws-fork`, the first side-thread task-thread primitive.
 *
 * A fork is a `pi --fork <lead session>` spawn (Decision §2): a lateral peer
 * that inherits the LEAD's full current context (cloned session), not a
 * depth-consuming worker (§3 — no depth-budget consumption, termination
 * unchanged). Its own tool surface is the lead's exact active-tools snapshot
 * at spawn time, minus the small excluded set (`FORK_EXCLUDED_TOOL_NAMES` —
 * Phase 1 excludes only `ws-fork` itself; the ticket's own forward-compat
 * note reserves room for `ws-ask`/`ws-resolve` in Phase 2), plus
 * `ws-report-to-lead` (Decision §3). Approval routing for a fork's own
 * `ws-execute`-spawned worker falls out for free from the existing
 * per-process registration pattern (see `spawner.ts`'s Codebase Findings in
 * the plan — every spawned child, fork included, re-runs `session_start`
 * fresh and gets its OWN `registerAgentTools`/`registerExecuteGateway`
 * closures) — no new relay code needed here for that.
 *
 * The anti-bleed mechanical loop (§4, task threads / Entry A only) is this
 * module's other half: turn-end-with-no-tool-call auto-nudges up to
 * `MAX_FORK_NUDGES` times before failing loud with a transcript tail;
 * idle-without-a-`kind:"final"` report is never harvested as a silent
 * success; `kind:"question"` then end-of-turn disambiguates as a question,
 * `kind:"final"` as completion, neither as a bare turn-end (`"no-signal"` or
 * `"acknowledge-and-return"` when a tool call happened but no report did);
 * a `kind:"final"` report is checked against the required
 * Outcome/Files-changed/Verification/Blockers/Commit/Decisions shape and,
 * when `expects_commit:true`, against a `Commit: none` non-completion rule.
 * Every predicate below is pure and independently unit-tested
 * (`test/fork.test.ts`) against plain data — none constructs a real
 * `RpcClient`. The IO glue (`registerFork`, `wireAntiBleedLoop`) wires them
 * onto the SAME `RpcClient.onEvent()` stream `spawner.ts`'s own
 * `attachEventListener` already observes for a spawned child (a second,
 * independent subscription — `RpcClient.onEvent()` supports multiple
 * listeners, same assumption `spawner.ts`'s own approval-relay callback
 * already rides) — this file never reaches into `spawner.ts`'s internals to
 * do it, keeping that module's own event-wiring untouched. This IO layer is
 * NOT unit-tested here (mirrors `execute-gateway.ts`'s own
 * `createApprovalRelay`/tool-`execute()` split): it needs a live `RpcClient`
 * subprocess, and the ticket's own Phase 1 task instruction defers the live
 * Bleed PoC / `--fork` composition confirmation to a manual gate (no
 * provider credentials in this sandbox).
 *
 * Golden rule: imports FROM `spawner.ts` and `process-role.ts` only, never
 * the reverse — mirrors `execute-gateway.ts`'s own placement note.
 * `agents-plugin-tool/` (ws-mcp Go) and `agents-plugin/skills/` canonical
 * text are both untouched by this ticket.
 */

import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { BridgeHandle } from "./bridge.ts";
import {
  REPORT_TO_LEAD_TOOL_NAME,
  inheritModelFromToolCtx,
  spawnAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "./spawner.ts";
import type { SpawnRole } from "./process-role.ts";

// ---------------------------------------------------------------------------
// Pure helpers. Unit-tested directly (test/fork.test.ts) with no
// filesystem/subprocess/live `pi` session involved.
// ---------------------------------------------------------------------------

/** Lead-facing verb-table tool name (pi-lead-guide.md), registered below. */
export const FORK_TOOL_NAME = "ws-fork";

/**
 * Tool names excluded from a fork's own computed tool surface
 * (`computeForkToolSurface`). Phase 1 excludes only `ws-fork` itself — a
 * fork can never spawn another fork (lateral, not recursive: §3's depth
 * rule falls out at the tool-allowlist layer, the same way a `full-worker`
 * spawn can never re-reach `ws-agent-spawn`). Phase 2 adds `ws-ask`/
 * `ws-resolve` here once those primitives exist (kept as its own named
 * constant, not folded into a shared exclusion set, so this stays
 * forward-compatible without touching `computeForkToolSurface`'s own logic).
 */
export const FORK_EXCLUDED_TOOL_NAMES: ReadonlySet<string> = new Set([FORK_TOOL_NAME]);

/**
 * Pure §3 fork tool-surface formula: the lead's own active-tools snapshot at
 * spawn time (`pi.getActiveTools()`, IO — read by the caller, passed in
 * here as plain data), minus `FORK_EXCLUDED_TOOL_NAMES`, plus
 * `REPORT_TO_LEAD_TOOL_NAME` if not already present. Mirrors
 * `execute-gateway.ts`'s `computeLeadActiveTools` remove/add/dedupe shape.
 */
export function computeForkToolSurface(leadActiveTools: readonly string[]): string[] {
  const kept = leadActiveTools.filter((name) => !FORK_EXCLUDED_TOOL_NAMES.has(name));
  const result = [...kept];
  if (!result.includes(REPORT_TO_LEAD_TOOL_NAME)) {
    result.push(REPORT_TO_LEAD_TOOL_NAME);
  }
  return [...new Set(result)];
}

/**
 * Pure, role-differentiated `ws-fork` active-tools addition — the fix for
 * the plan's own risk-signal finding: `role === undefined` is the true top
 * lead ONLY. A `"fork"` (or `"worker"`/`"explore"`) role never regains
 * `ws-fork`, even though `isLeadOrFork` treats lead and fork identically for
 * the system-prompt/workflow_manual gates elsewhere — this is deliberately
 * a DIFFERENT, narrower gate than `isLeadOrFork`, not a reuse of it. Never
 * folded into `execute-gateway.ts`'s shared `LEAD_ADDED_TOOL_NAMES` /
 * `computeLeadActiveTools`, which IS applied uniformly to both lead and fork
 * roles — doing so would leak `ws-fork` back into a fork's own surface.
 */
export function addForkToolIfLead(activeTools: readonly string[], role: SpawnRole | undefined): string[] {
  if (role !== undefined || activeTools.includes(FORK_TOOL_NAME)) {
    return [...activeTools];
  }
  return [...activeTools, FORK_TOOL_NAME];
}

/** Max consecutive no-tool-call turn-ends tolerated before failing loud (§4). */
export const MAX_FORK_NUDGES = 2;

/** `true` while another auto-nudge is still allowed; `false` once `nudgeCount` has reached `MAX_FORK_NUDGES` (the fail-loud transition). */
export function shouldNudge(nudgeCount: number): boolean {
  return nudgeCount < MAX_FORK_NUDGES;
}

export type ForkTurnOutcome = "question" | "final" | "acknowledge-and-return" | "no-signal";

/**
 * Pure §4 disambiguation table for a single fork turn's end:
 * - `reportKind: "question"` -> `"question"` (ends the turn awaiting the
 *   lead's input — a valid, non-bleed stop).
 * - `reportKind: "final"` -> `"final"` (task complete — subject to the
 *   report-shape/`expects_commit` checks elsewhere, not this predicate).
 * - No report, but a tool call happened this turn -> `"acknowledge-and-
 *   return"` (real progress, just not yet reported — not itself a bleed
 *   signal; the ticket explicitly rules out prose-only bleed mitigation, so
 *   this case is not auto-nudged).
 * - Neither a report nor any tool call -> `"no-signal"` (the actual bleed
 *   condition the nudge loop targets).
 */
export function classifyForkTurnOutcome(input: { hadToolCall: boolean; reportKind?: "question" | "final" }): ForkTurnOutcome {
  if (input.reportKind === "question") return "question";
  if (input.reportKind === "final") return "final";
  return input.hadToolCall ? "acknowledge-and-return" : "no-signal";
}

/**
 * `true` when none of `reportKinds` (the buffered/observed report kinds for
 * a fork, in whatever order the caller collected them) is `"final"` — the
 * §4 "idle-without-`kind:"final"` is reported as incomplete, never harvested
 * as a result" rule. An empty list is vacuously `true` (no final report was
 * ever seen).
 */
export function isIdleWithoutFinal(reportKinds: readonly (string | undefined)[]): boolean {
  return !reportKinds.includes("final");
}

/** §4's required `kind:"final"` report field order, each expected as its own `"<Field>:"`-prefixed line. */
export const REQUIRED_FINAL_REPORT_FIELDS = ["Outcome", "Files changed", "Verification", "Blockers", "Commit", "Decisions"] as const;

/**
 * Line-anchored extraction of a `"<field>: <value>"` line's value from a
 * multi-line report `message` — trims surrounding whitespace on both the
 * matched line and the returned value. Returns `undefined` when no line
 * starts with the exact `"<field>:"` prefix (after trimming). Shared by
 * `validateFinalReportShape` (presence check) and `checkExpectsCommitCompletion`'s
 * caller (`wireAntiBleedLoop`, to pull the `Commit:` value out of an
 * already-shape-valid report).
 */
export function extractReportField(message: string, field: string): string | undefined {
  const prefix = `${field}:`;
  for (const rawLine of message.split("\n")) {
    const line = rawLine.trim();
    if (line.startsWith(prefix)) {
      return line.slice(prefix.length).trim();
    }
  }
  return undefined;
}

/**
 * Pure §4 required-shape check for a `kind:"final"` report: every field in
 * `REQUIRED_FINAL_REPORT_FIELDS` must appear as its own `"<Field>:"`-prefixed
 * line (via `extractReportField`) somewhere in `message`. `Commit:` must
 * always be present (the literal value `"none"` is valid shape-wise — see
 * `checkExpectsCommitCompletion` for the separate `expects_commit`
 * non-completion rule layered on top).
 */
export function validateFinalReportShape(message: string): { ok: true } | { ok: false; missing: string[] } {
  const missing = REQUIRED_FINAL_REPORT_FIELDS.filter((field) => extractReportField(message, field) === undefined);
  return missing.length === 0 ? { ok: true } : { ok: false, missing: [...missing] };
}

/**
 * Pure §4 `expects_commit` non-completion rule: `expects_commit:true` paired
 * with an absent `Commit:` line, or one whose value is (case-insensitively,
 * whitespace-tolerantly) the literal `"none"`, is flagged as NOT a
 * completion — `expects_commit:false`/omitted never triggers this, and any
 * other non-empty `Commit:` value is accepted regardless of `expects_commit`.
 */
export function checkExpectsCommitCompletion(expectsCommit: boolean, commitLine: string | undefined): { ok: true } | { ok: false; reason: string } {
  if (expectsCommit && (commitLine === undefined || /^\s*none\s*$/i.test(commitLine))) {
    return { ok: false, reason: 'expects_commit:true but Commit: none (or missing) — not treated as a completion' };
  }
  return { ok: true };
}

/**
 * Pure last-`n`-lines tail, used to surface a stalled fork's transcript tail
 * to the lead on the fail-loud path (§4) instead of a full-file dump.
 * `n <= 0` returns an empty string; `n` beyond the line count returns the
 * whole text unchanged.
 */
export function tailLines(text: string, n: number): string {
  if (n <= 0) return "";
  const lines = text.split("\n");
  return lines.slice(Math.max(0, lines.length - n)).join("\n");
}

/**
 * Pure, shape-tolerant extraction of the calling tool-execute `toolCtx`'s
 * own session file path — the fork's `--fork <path>` target — via
 * `toolCtx.sessionManager.getSessionFile()` (confirmed reachable:
 * `ExtensionContext.sessionManager: ReadonlySessionManager`,
 * `SessionManager.getSessionFile()`, see the plan's Codebase Findings).
 * Mirrors `inheritModelFromToolCtx`'s own extraction-for-testability shape
 * (spawner.ts) so this seam is unit-testable against a fake `toolCtx` with
 * no live `ExtensionContext`.
 */
export function getForkSourceSessionFile(toolCtx: unknown): string | undefined {
  const sessionManager = (toolCtx as { sessionManager?: { getSessionFile?: () => string | undefined } } | undefined)?.sessionManager;
  const file = sessionManager?.getSessionFile?.();
  return typeof file === "string" && file.length > 0 ? file : undefined;
}

/**
 * Pure directive text appended to a fork's system prompt (via
 * `--append-system-prompt`, an ephemeral per-spawn file — `registerFork`
 * writes no new checked-in guide asset for this, unlike
 * `execute-worker-guide.md`). Short natural language, execution constraints
 * only — no identity framing, no XML/all-caps overrides, per §4's directive-
 * style rule. The lead's own task text is delivered separately as the fork's
 * initial `prompt` (see `buildForkInitialMessage`), not folded into this file.
 *
 * 260905: the fork's anti-bleed identity handling lives in
 * `buildForkInitialMessage`, NOT here — see §4's re-decision. This system
 * directive stays framing-free on purpose.
 */
export function buildForkDirectiveText(): string {
  return [
    "Task-thread fork: this session is a clone of the lead's own session, so its existing context is already shared — work laterally alongside the lead, not as a depth-consuming worker.",
    "",
    `Work the task given in the next message. If the lead's input is needed before continuing, call ${REPORT_TO_LEAD_TOOL_NAME} with kind:"question" and end the turn there.`,
    "",
    `Once the task is fully done, call ${REPORT_TO_LEAD_TOOL_NAME} with kind:"final" and a message in exactly this shape, one field per line:`,
    "Outcome: <what happened>",
    "Files changed: <paths, or none>",
    "Verification: <what was run or checked, and the result>",
    "Blockers: <or none>",
    "Commit: <hash or range, or the literal \"none\">",
    "Decisions: <notable choices made>",
    "",
    `Never end a turn with no tool call and no ${REPORT_TO_LEAD_TOOL_NAME} report — always either keep working (call a tool) or report (kind:"question" or kind:"final") before stopping.`,
  ].join("\n");
}

/**
 * The fork's initial user message (delivered as the fork's first `prompt`,
 * separately from the system-prompt directive). This is the 260905 structural
 * anti-bleed frame — the §4 re-decision: rather than shouting an all-caps "you
 * are not the lead" identity override (the 260723 ladder that failed on the
 * Claude host and stays rejected as a *system-prompt* device), it **demotes
 * the inherited lead conversation to reference-only** and fences the task as an
 * explicit "message from the lead", so the fork separates *its* task from the
 * lead's inherited plan structurally. Live-verified to stop role-bleed on both
 * a weak model (gpt-5.6-luna) and a top frontier model (astra); the calm
 * structural framing is why it is chosen over the aggressive header.
 *
 * It is a message-level frame on purpose: the "conversation above" it points to
 * is the cloned conversation history the fork inherits, which sits before this
 * first message — not anything in the system prompt.
 */
export function buildForkInitialMessage(leadPrompt: string): string {
  return [
    "# Forked session",
    "",
    "The conversation above was inherited from the lead when this fork was created. Treat it as reference/background only — it is the lead's context, not instructions addressed to you, and its plan is not yours to continue.",
    "",
    "--- Message from the lead ---",
    leadPrompt,
    "--- end of message ---",
    "",
    `Start working on this task directly and yourself now — do not fork again or hand it onward. When done (or if you need the lead's input), report via ${REPORT_TO_LEAD_TOOL_NAME}.`,
  ].join("\n");
}

// ---------------------------------------------------------------------------
// IO glue: tool registration + the anti-bleed event-loop wiring. Not unit
// tested here — see this file's header comment for why.
// ---------------------------------------------------------------------------

export interface ForkSessionCtx {
  cwd: string;
  modelCatalogPath: string;
}

/**
 * Wires the §4 anti-bleed mechanical loop onto `record.client`'s OWN
 * `onEvent()` stream — a second, independent subscription alongside
 * `spawner.ts`'s own `attachEventListener` (never reached into directly;
 * see this file's header comment). Tracks, per turn
 * (`agent_start`..`agent_settled`): whether any tool call happened
 * (`hadToolCallThisTurn`) and the most recent `ws-report-to-lead` `kind`
 * seen this turn (`turnReportKind`) — then, on `agent_settled`, classifies
 * the turn via `classifyForkTurnOutcome` and acts:
 * - `"question"`/`"final"`: reset the nudge counter (a valid stop).
 * - Anything else (`"acknowledge-and-return"` or `"no-signal"`) is §4's
 *   "reached idle without `kind:"final"`" case — checked via
 *   `isIdleWithoutFinal` against `record.pendingReports`' own undrained
 *   kinds (so an already-buffered-but-not-yet-harvested `"final"` is not
 *   re-flagged):
 *   - `"acknowledge-and-return"`: reset the nudge counter — a tool call is
 *     real progress, not itself a bleed signal (the ticket rules out
 *     prose-only bleed mitigation as a *substitute* for this check, not as
 *     a reason to treat a working turn as silence) — but still surface an
 *     incomplete-run notice to the LEAD so an idle fork with no
 *     completion/question signal is never mistaken for a finished result.
 *   - `"no-signal"`: `shouldNudge` — re-prompt the FORK itself via
 *     `record.client.prompt(...)` (never the lead's session — a spawned
 *     child only runs another turn when re-prompted on its OWN RPC handle;
 *     this mirrors `sendToAgent`'s live-idle branch, including clearing
 *     `record.idlePending` first so a racing `ws-agent-wait` cannot
 *     busy-return a stale pre-nudge snapshot). Once `MAX_FORK_NUDGES` is
 *     exhausted, fail loud to the LEAD with a transcript tail (`tailLines`
 *     over `record.sessionPath`, best-effort) and tell it not to harvest a
 *     result from this fork.
 *
 * A `kind:"final"` report is additionally validated inline, the instant its
 * `tool_execution_start` is observed: `validateFinalReportShape` (rejects a
 * malformed report — never treated as a valid completion) and, when shape
 * is valid, `checkExpectsCommitCompletion` against the extracted `Commit:`
 * line. Both surface a steer notice to the lead on failure; neither mutates
 * `record.pendingReports` itself (that FIFO is `spawner.ts`'s own, drained
 * by `ws-agent-wait` as normal — this loop only ever ADDS advisory
 * `pi.sendUserMessage` notices to the lead, or a nudge `prompt()` to the
 * fork itself; it never blocks or rewrites the report).
 */
function wireAntiBleedLoop(pi: ExtensionAPI, agentId: string, record: RpcAgentRecord, expectsCommit: boolean): void {
  const client = record.client;
  if (!client) return;

  let nudgeCount = 0;
  let hadToolCallThisTurn = false;
  let turnReportKind: "question" | "final" | undefined;

  client.onEvent((evt) => {
    const e = evt as { type?: string; toolName?: string; args?: unknown };

    if (e.type === "agent_start") {
      hadToolCallThisTurn = false;
      turnReportKind = undefined;
      return;
    }

    if (e.type === "tool_execution_start") {
      hadToolCallThisTurn = true;
      if (e.toolName === REPORT_TO_LEAD_TOOL_NAME) {
        const args = e.args as { kind?: unknown; message?: unknown } | undefined;
        const kind = args?.kind === "question" || args?.kind === "final" ? args.kind : undefined;
        turnReportKind = kind;

        if (kind === "final" && typeof args?.message === "string") {
          const shape = validateFinalReportShape(args.message);
          if (!shape.ok) {
            pi.sendUserMessage(
              `Fork ${agentId}'s kind:"final" report is missing required field(s): ${shape.missing.join(", ")}. Required shape: ${REQUIRED_FINAL_REPORT_FIELDS.join(" / ")}. This is NOT treated as a valid completion — ask the fork to resubmit.`,
              { deliverAs: "steer" },
            );
          } else {
            const commitLine = extractReportField(args.message, "Commit");
            const commitCheck = checkExpectsCommitCompletion(expectsCommit, commitLine);
            if (!commitCheck.ok) {
              pi.sendUserMessage(
                `Fork ${agentId} reported kind:"final" but ${commitCheck.reason}. This is NOT treated as a valid completion.`,
                { deliverAs: "steer" },
              );
            }
          }
        }
      }
      return;
    }

    if (e.type !== "agent_settled") return;

    const outcome = classifyForkTurnOutcome({ hadToolCall: hadToolCallThisTurn, reportKind: turnReportKind });

    if (outcome === "question" || outcome === "final") {
      nudgeCount = 0;
      return;
    }

    // Neither "question" nor "final" this turn. §4: a fork reaching idle
    // (agent_settled) without a completion/question signal is
    // non-completion. Check the record's own undrained report queue (not
    // just this turn) so an already-buffered-but-not-yet-harvested
    // kind:"final" is not re-flagged as idle-without-final.
    if (!isIdleWithoutFinal(record.pendingReports.map((r) => r.kind))) {
      nudgeCount = 0;
      return;
    }

    if (outcome === "acknowledge-and-return") {
      // A tool call happened this turn — real progress, not itself a bleed
      // signal, so it is not auto-nudged. It is still §4's
      // idle-without-final case, though: surface it to the lead so an idle
      // fork with no completion/question signal is never mistaken for a
      // finished result.
      nudgeCount = 0;
      pi.sendUserMessage(
        `Fork ${agentId} went idle after doing work, but has not reported completion (kind:"final") or asked a question (kind:"question"). Do not treat this as a finished result yet.`,
        { deliverAs: "steer" },
      );
      return;
    }

    // "no-signal": the actual bleed condition — no tool call, no report.
    // Re-prompt the FORK itself (never the lead — see this function's own
    // doc comment).
    if (shouldNudge(nudgeCount)) {
      nudgeCount += 1;
      record.idlePending = false; // mirrors sendToAgent's live-idle clearing: an internal re-prompt must not let a racing ws-agent-wait busy-return with the stale pre-nudge snapshot.
      client
        .prompt(
          `Your last turn ended with no tool call and no ${REPORT_TO_LEAD_TOOL_NAME} report (nudge ${nudgeCount}/${MAX_FORK_NUDGES}). Continue the task, or call ${REPORT_TO_LEAD_TOOL_NAME} with kind:"question" or kind:"final".`,
        )
        .catch((err: unknown) => {
          pi.sendUserMessage(
            `Fork ${agentId}: failed to deliver an anti-bleed nudge to it (${err instanceof Error ? err.message : String(err)}). Treat this fork as potentially stalled.`,
            { deliverAs: "steer" },
          );
        });
      return;
    }

    let transcriptTail = "(transcript unavailable)";
    try {
      transcriptTail = tailLines(readFileSync(record.sessionPath, "utf8"), 40);
    } catch {
      // best effort — see doc comment above; a missing/unreadable transcript
      // must not turn a fail-loud notice into an unhandled exception.
    }
    pi.sendUserMessage(
      [
        `Fork ${agentId} stalled: ${MAX_FORK_NUDGES} consecutive turns with no tool call and no report. Treat this fork as failed — do NOT harvest a result from it.`,
        "Transcript tail:",
        transcriptTail,
      ].join("\n"),
      { deliverAs: "steer" },
    );
  });
}

/**
 * Registers `ws-fork` (lead-facing; reachable only after
 * `index.ts`'s role-differentiated `addForkToolIfLead` active-tools step —
 * see that file's `session_start` wiring): spawns a `pi --fork <own session>`
 * lateral peer sharing the caller's full context, computes its dynamic tool
 * surface (`computeForkToolSurface` over the caller's OWN
 * `pi.getActiveTools()` at spawn time — not a static `TOOL_GROUPS` entry),
 * and wires the anti-bleed loop onto it. Returns `{agent_id}` immediately
 * (fire-and-return, same convention as `ws-execute`) — harvest progress with
 * the same shared `ws-agent-wait`/`ws-agent-list`/`ws-agent-transcript`
 * tools any other spawned agent uses (`rpcRegistry` is the one shared map,
 * per `AgentToolsHandle`'s own doc comment).
 *
 * Registered declaratively/globally (same pattern as `ws-report-to-lead`/
 * `registerExecuteGateway`) from `index.ts`'s `session_start`, regardless of
 * role — a fork child re-runs `session_start` too and must register this
 * same tool globally for `computeForkToolSurface`'s own `ws-fork` exclusion
 * to have anything to exclude from. Whether it is ever ACTIVE for a given
 * session is `addForkToolIfLead`'s job, not this function's.
 */
export function registerFork(pi: ExtensionAPI, bridge: BridgeHandle, rpcRegistry: RpcAgentRegistry, sessionCtx: ForkSessionCtx): void {
  pi.registerTool({
    name: FORK_TOOL_NAME,
    label: FORK_TOOL_NAME,
    description:
      'Spawn a lateral task-thread fork that inherits your full current context (a clone of your own session) to work a sub-task alongside you — not a worker (no depth-budget consumption). Its own tool surface excludes ws-fork (no recursive forking). It reports back only via ws-report-to-lead(kind:"question"|"final"); expects_commit:true flags a kind:"final" report whose Commit field is missing or "none" as incomplete. Returns {agent_id} immediately — harvest with ws-agent-wait/ws-agent-list like any other spawned agent.',
    parameters: {
      type: "object",
      properties: {
        prompt: {
          type: "string",
          description: "Task directive for the fork — short natural language, execution constraints only; no identity framing, no XML/all-caps overrides.",
        },
        model_name: {
          type: "string",
          description: "Optional alias resolved against model-catalog.json's aliases map; omitted or unmapped inherits your own model.",
        },
        expects_commit: {
          type: "boolean",
          description: 'true flags a kind:"final" report whose Commit field is missing or the literal "none" as an incomplete run.',
        },
      },
      required: ["prompt"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
      const p = params as { prompt: string; model_name?: string; expects_commit?: boolean };
      const forkFrom = getForkSourceSessionFile(toolCtx);
      if (!forkFrom) {
        throw new Error(`ws-pi-agent: ${FORK_TOOL_NAME}: could not determine your own session file via toolCtx.sessionManager.getSessionFile()`);
      }

      const tools = computeForkToolSurface(pi.getActiveTools());
      const directiveDir = mkdtempSync(join(tmpdir(), "ws-pi-fork-"));
      const directivePath = join(directiveDir, "fork-directive.md");
      writeFileSync(directivePath, buildForkDirectiveText());

      const result = await spawnAgent(
        rpcRegistry,
        {
          cwd: sessionCtx.cwd,
          inheritModel: inheritModelFromToolCtx(toolCtx),
          wsToolNames: bridge.wsToolNames,
          modelCatalogPath: sessionCtx.modelCatalogPath,
          forkFrom,
          explicitTools: tools.join(","),
          parentSessionKey: bridge.defaultSessionKeyRef.current,
        },
        {
          systemPromptPath: directivePath,
          prompt: buildForkInitialMessage(p.prompt),
          modelName: p.model_name,
        },
      );

      const record = rpcRegistry.get(result.agent_id);
      if (record) {
        wireAntiBleedLoop(pi, result.agent_id, record, p.expects_commit ?? false);
      }

      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  });
}
