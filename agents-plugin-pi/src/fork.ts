/**
 * `ws-fork` spawns a lateral `pi --fork <lead session>` peer with the lead's
 * captured context and active tool surface. Questions and progress may travel
 * through `ws-report-to-lead`; the ordinary assistant answer delivered by
 * `agent_settled` is the sole terminal result for every fork.
 *
 * Final-output fields and `expects_commit` are prompt-level expectations only.
 * The adapter deliberately does not parse, validate, or retry final prose.
 * Generation, subtree, ownership, and delivery fences live in `spawner.ts`.
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { BridgeHandle } from "./bridge.ts";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { buildForkSummary, createDispatchToolPreview } from "./tool-row-render.ts";
import { modelCatalogFromToolCtx, tierWarningNotifierFromToolCtx, type ModelCatalogEntry } from "./model-catalog.ts";
import {
  REPORT_TO_LEAD_TOOL_NAME,
  inheritModelFromToolCtx,
  spawnAgent,
  storageContextFromToolCtx,
  type ResolvedModelInfo,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "./spawner.ts";
import { readSpawnRole, type SpawnRole } from "./process-role.ts";
import { captureForkContext, captureRegisteredTools, captureUnflushedForkSource, effectiveForkDescriptor } from "./fork-context.ts";
import type { LeadPromptRef } from "./lead-bootstrap.ts";

// ---------------------------------------------------------------------------
// Pure helpers. Unit-tested directly (test/fork.test.ts) with no
// filesystem/subprocess/live `pi` session involved.
// ---------------------------------------------------------------------------

/** Lead-facing verb-table tool name (pi-lead-guide.md), registered below. */
export const FORK_TOOL_NAME = "ws-fork";

/**
 * Tool names excluded from a fork's own computed tool surface
 * (`computeForkToolSurface`). `ws-fork` itself — a fork can never spawn
 * another fork (lateral, not recursive: §3's depth rule falls out at the
 * tool-allowlist layer, the same way a `full-worker` spawn can never
 * re-reach `ws-agent-spawn`) — plus, since Phase 2, the owner-question
 * primitives `ws-queue-question`/`ws-withdraw-question` (§3, renamed by
 * `260911` from `ws-ask`/`ws-resolve`): a fork's only question path stays
 * `ws-report-to-lead(kind:"question")`, which the lead surfaces as a thread
 * of its own (`ask.ts`'s `handleForkRaisedQuestion`).
 *
 * The two Phase 2 names are duplicated here as literals ON PURPOSE rather
 * than imported from `ask.ts` (which owns them as `ASK_TOOL_NAME`/
 * `RESOLVE_TOOL_NAME`): `ask.ts` imports `computeForkToolSurface` and the
 * spawn helpers FROM this module, so importing back would create a cycle and
 * break this file's own "imports FROM `spawner.ts`/`process-role.ts` only"
 * placement rule. `test/ask.test.ts` asserts the two constants and these two
 * literals stay equal, so the duplication cannot silently drift.
 */
/** Forks preserve the lead's actual ordered callable surface; role checks live in handlers. */
export const FORK_EXCLUDED_TOOL_NAMES: ReadonlySet<string> = new Set();

/**
 * Pure §3 fork tool-surface formula: the lead's own active-tools snapshot at
 * spawn time (`pi.getActiveTools()`, IO — read by the caller, passed in
 * here as plain data), minus `FORK_EXCLUDED_TOOL_NAMES`, plus
 * `REPORT_TO_LEAD_TOOL_NAME` if not already present. Mirrors
 * `execute-gateway.ts`'s `computeLeadActiveTools` remove/add/dedupe shape.
 */
export function computeForkToolSurface(leadActiveTools: readonly string[]): string[] {
  return [...leadActiveTools];
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
 * The structural inherited-context boundary lives in
 * `buildForkInitialMessage`; this directive stays framing-free on purpose.
 */
export function buildForkDirectiveText(expectsCommit = false): string {
  return [
    "Task-thread fork: this session is a clone of the lead's own session, so its existing context is already shared — work laterally alongside the lead, not as a depth-consuming worker.",
    "",
    `Work the task in this message. If the lead's input is needed before continuing, call ${REPORT_TO_LEAD_TOOL_NAME} with kind:"question" and end the turn there. Progress updates may use the same tool without kind.`,
    "",
    "Once the task is fully done, end with an ordinary assistant answer in exactly this shape, one field per line:",
    "Outcome: <what happened>",
    "Files changed: <paths, or none>",
    "Verification: <what was run or checked, and the result>",
    "Blockers: <or none>",
    `Commit: ${expectsCommit ? "<required commit hash or range>" : '<hash or range, or the literal "none">'}`,
    "Decisions: <notable choices made>",
    "",
    "Settlement delivers that answer to the lead. The adapter does not parse or approve the fields; the lead judges adequacy and may resume this same fork.",
  ].join("\n");
}

/**
 * The fork's initial user message (delivered as the fork's first `prompt`,
 * separately from the system-prompt directive). This is the structural
 * inherited-context frame: rather than shouting an all-caps "you
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
export function buildForkInitialMessage(leadPrompt: string, expectsCommit = false): string {
  return [
    buildForkDirectiveText(expectsCommit),
    "",
    "# Forked session",
    "",
    "The conversation above was inherited from the lead when this fork was created. Treat it as reference/background only — it is the lead's context, not instructions addressed to you, and its plan is not yours to continue.",
    "",
    "--- Message from the lead ---",
    leadPrompt,
    "--- end of message ---",
    "",
    `Start working on this task directly and yourself now — do not fork again or hand it onward. Use ${REPORT_TO_LEAD_TOOL_NAME} only for a question or progress before settlement.`,
  ].join("\n");
}

// ---------------------------------------------------------------------------
// IO glue: tool registration and owner-question routing.
// ---------------------------------------------------------------------------

export interface ForkSessionCtx {
  cwd: string;
  /** Exact manifest entry module loaded by this parent, passed to every fork. */
  extensionPath: string;
  effectivePromptRef?: LeadPromptRef;
}

/**
 * 260904 Phase 2 seam: fired with `(agentId, message)` the moment a fork
 * enqueues a `kind:"question"` `ws-report-to-lead` report, carrying that
 * report's own free-text `message`. `ask.ts` supplies the real behavior
 * (register a thread whose respondent is this live fork, refresh the pending
 * widget); `fork.ts` stays generic and never imports it.
 *
 * Review relay #1 I6, revised 260905: returning a defined string means the TUI
 * owner surface CONSUMED the question — `spawner.ts` then suppresses the
 * `ws-agent-question` push entirely (§1: the lead is not involved in a
 * fork-raised question). Returning `undefined` (headless) leaves the push in
 * place so the lead still learns of it. This is invoked from `spawner.ts`'s
 * report-handling site rather than from `wireAntiBleedLoop`'s settle handler,
 * because the push is emitted at report time — the thread must already exist
 * before the suppression decision is made.
 */
export type ForkQuestionCallback = (agentId: string, message: string) => string | undefined;

/**
 * Builds the `spawnAgent` ctx for a `ws-fork` spawn.
 *
 * Extracted (review relay #1, C1) because this object silently lost its `pi`
 * field: `RpcSpawnCtx.pi` became REQUIRED with the push model, and with no
 * `tsc` step in this package the omission was invisible — every `ws-fork`
 * child spawned without a push channel, so its settlements, progress, and
 * headless questions were all dropped by `pushToLead`'s
 * `if (!pi) return` guard. Keeping the ctx construction in one exported,
 * directly-asserted function is the cheap standing guard against that class of
 * regression.
 */
export function buildForkSpawnCtx(
  pi: ExtensionAPI,
  bridge: BridgeHandle,
  sessionCtx: ForkSessionCtx,
  opts: { forkFrom: string; forkSourceEntries?: unknown[]; explicitTools: string; inheritModel?: string; catalog: readonly ModelCatalogEntry[]; notifyTierWarning?: (warning: string) => void; forkCacheNoticeOwner?: Parameters<typeof spawnAgent>[1]["forkCacheNoticeOwner"]; forkContext?: ReturnType<typeof captureForkContext>; onModelResolved?: Parameters<typeof spawnAgent>[1]["onModelResolved"] },
): Parameters<typeof spawnAgent>[1] {
  return {
    // Load-bearing: the fork's whole report channel back to the lead.
    pi,
    cwd: sessionCtx.cwd,
    extensionPath: sessionCtx.extensionPath,
    inheritModel: opts.inheritModel,
    catalog: opts.catalog,
    notifyTierWarning: opts.notifyTierWarning,
    forkCacheNoticeOwner: opts.forkCacheNoticeOwner,
    wsToolNames: bridge.wsToolNames,
    client: bridge.client,
    forkFrom: opts.forkFrom,
    forkSourceEntries: opts.forkSourceEntries,
    explicitTools: opts.explicitTools,
    parentSessionKey: bridge.defaultSessionKeyRef.current,
    spawnRole: "fork",
    forkContext: opts.forkContext,
    onModelResolved: opts.onModelResolved,
  };
}

/**
 * Arms the `fork`-role question-routing hook. Ordinary settled output is
 * delivered by spawner.ts and is never parsed here.
 *
 * Two call sites, which is why it is a function rather than inline in
 * `registerFork` (review relay #1, I1): the fresh `ws-fork` spawn, and the
 * shutdown sidecar's orphan revival in `index.ts`, which re-registers a
 * previous session's fork as a DORMANT record. `wireAntiBleedLoop` needs a
 * live client, which a dormant record has none of, so this defers it to
 * `RpcAgentRecord.onResume` (fired by `sendToAgent`'s dormant-resume branch
 * once the fresh client exists) and additionally arms it immediately when the
 * record is already live.
 */
export function armForkRoleWiring(
  _pi: ExtensionAPI,
  _rpcRegistry: RpcAgentRegistry,
  record: RpcAgentRecord,
  onQuestion?: ForkQuestionCallback,
  _expectsCommit = false,
): void {
  if (onQuestion) {
    // 260904 Phase 2 (review relay #1 I6), revised 260905: armed at the
    // report-handling site rather than on turn settle. A defined return
    // (TUI only) means the owner surface consumed the question itself, and
    // that return string is the lead notice to push: `applyRpcEvent` sends
    // it as `ws-agent-advisory`/`fork-question-thread` instead of the
    // `ws-agent-question` push the headless baseline would send.
    record.onQuestionReport = (rec, message) => onQuestion(rec.agentId, message);
  }
}

/**
 * Registers `ws-fork` (lead-facing; reachable only after
 * `index.ts`'s role-differentiated `addForkToolIfLead` active-tools step —
 * see that file's `session_start` wiring): spawns a `pi --fork <own session>`
 * lateral peer sharing the caller's full context, computes its dynamic tool
 * surface (`computeForkToolSurface` over the caller's OWN
 * `pi.getActiveTools()` at spawn time — not a static `TOOL_GROUPS` entry),
 * and wires owner-question routing onto it. Returns `{agent_id}` immediately
 * (fire-and-return, same convention as `ws-execute`) — its reports, settles
 * and advisories are PUSHED into the lead session as they happen, and
 * `ws-agent-list`/`ws-agent-transcript` remain available for on-demand
 * inspection (`rpcRegistry` is the one shared map, per `AgentToolsHandle`'s
 * own doc comment).
 *
 * Registered declaratively/globally (same pattern as `ws-report-to-lead`/
 * `registerExecuteGateway`) from `index.ts`'s `session_start`, regardless of
 * role — a fork child re-runs `session_start` too and must register this
 * same tool globally for `computeForkToolSurface`'s own `ws-fork` exclusion
 * to have anything to exclude from. Whether it is ever ACTIVE for a given
 * session is `addForkToolIfLead`'s job, not this function's.
 */
export function registerFork(
  pi: ExtensionAPI,
  bridge: BridgeHandle,
  rpcRegistry: RpcAgentRegistry,
  sessionCtx: ForkSessionCtx,
  /** 260904 Phase 2: see `ForkQuestionCallback`. Omitted keeps Phase 1 behavior unchanged. */
  onQuestion?: ForkQuestionCallback,
  toolPreviewTuiRef: ToolPreviewTuiRef = createToolPreviewTuiRef(),
): void {
  registerWsTool(pi, {
    name: FORK_TOOL_NAME,
    label: FORK_TOOL_NAME,
    description:
      'Spawn a lateral task-thread fork that inherits your full current context (a clone of your own session) to work a sub-task alongside you — not a worker (no depth-budget consumption). It retains the lead tool surface but refuses ws-fork, ws-queue-question, and ws-withdraw-question in fork role. Questions and progress may arrive via ws-report-to-lead; the ordinary answer at agent settlement is the terminal result. expects_commit:true keeps the commit expectation visible in the prompt without adapter parsing. Returns {agent_id, warning?} immediately — end your turn afterwards; its reports and settlement arrive as pushed messages.',
    parameters: {
      type: "object",
      properties: {
        prompt: {
          type: "string",
          description: "Task directive for the fork — short natural language, execution constraints only; no identity framing, no XML/all-caps overrides.",
        },
        model_name: {
          type: "string",
          description: "Optional tier name (small|medium|large|xlarge) resolved against harness pi's config.tune agents.tier entries (see lead-tune/config.list); omitted or unmapped inherits your own model.",
        },
        expects_commit: {
          type: "boolean",
          description: "true tells the fork and lead that the settled result is expected to include a commit; the adapter does not parse or enforce the prose.",
        },
      },
      required: ["prompt"],
    } as never,
    async execute(_toolCallId, params, _signal, onUpdate, toolCtx) {
      if (readSpawnRole(process.env) === "fork") {
        throw new Error(`ws-pi-agent: ${FORK_TOOL_NAME} is unavailable in a fork; report to the lead instead.`);
      }
      const p = params as { prompt: string; model_name?: string; expects_commit?: boolean };
      const forkFrom = getForkSourceSessionFile(toolCtx);
      if (!forkFrom) {
        throw new Error(`ws-pi-agent: ${FORK_TOOL_NAME}: could not determine your own session file via toolCtx.sessionManager.getSessionFile()`);
      }

      const tools = computeForkToolSurface(pi.getActiveTools());
      const captured = sessionCtx.effectivePromptRef?.resolve?.(toolCtx) ?? sessionCtx.effectivePromptRef?.current;
      const forkContext = captured
        ? captureForkContext({
            kind: "task",
            effectiveSystemPrompt: captured.effectiveSystemPrompt,
            basePromptOptions: captured.basePromptOptions,
            wsBlock: captured.wsBlock,
            parentSessionKey: captured.parentSessionKey ?? bridge.defaultSessionKeyRef.current,
            parentSessionKeys: [...new Set([captured.parentSessionKey, bridge.defaultSessionKeyRef.current].filter((key): key is string => typeof key === "string"))],
            parentPiSessionId: toolCtx.sessionManager.getSessionId(),
            parentAffinityId: toolCtx.sessionManager.getSessionId(),
            thinkingLevel: pi.getThinkingLevel(),
            activeTools: tools,
            registeredTools: captureRegisteredTools(tools, pi.getAllTools()),
            modelDescriptor: await effectiveForkDescriptor(toolCtx, pi.getThinkingLevel()),
          })
        : undefined;
      let resolvedInfo: ResolvedModelInfo | undefined;
      const result = await spawnAgent(
        rpcRegistry,
        { ...buildForkSpawnCtx(pi, bridge, sessionCtx, {
          forkFrom,
          forkSourceEntries: captureUnflushedForkSource(toolCtx),
          explicitTools: tools.join(","),
          inheritModel: inheritModelFromToolCtx(toolCtx),
          catalog: modelCatalogFromToolCtx(toolCtx),
          notifyTierWarning: tierWarningNotifierFromToolCtx(toolCtx),
          forkCacheNoticeOwner: { mode: toolCtx.mode, hasUI: toolCtx.hasUI, ui: toolCtx.ui },
          forkContext,
          onModelResolved: (resolved) => {
            resolvedInfo = resolved;
            onUpdate?.({ content: [], details: { resolved } });
          },
        }), storage: storageContextFromToolCtx(toolCtx) },
        {
          prompt: buildForkInitialMessage(p.prompt, p.expects_commit ?? false),
          modelName: p.model_name,
        },
      );

      const record = rpcRegistry.get(result.agent_id);
      if (record) armForkRoleWiring(pi, rpcRegistry, record, onQuestion, p.expects_commit ?? false);

      return { content: [{ type: "text", text: JSON.stringify(result) }], details: { resolved: resolvedInfo } };
    },
    ...createDispatchToolPreview(toolPreviewTuiRef, FORK_TOOL_NAME, buildForkSummary),
  }, toolPreviewTuiRef);
}
