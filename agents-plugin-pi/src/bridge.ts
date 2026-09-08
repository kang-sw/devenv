/**
 * Tool re-registration bridge: spawns the ws-mcp launcher, version-checks
 * it, lists its tools, and re-registers each one on Pi under a sanitized
 * name derived from "ws/" + rawName.
 *
 * SKILL.md prose is written as the literal `ws/playbook.read(...)` /
 * `ws/workflow_manual(...)` call syntax (see
 * ai-docs/spec/mcp-tools.md's McpNamespace template and
 * agents-plugin/skills/*), but that prose form is not itself a legal
 * provider tool name: OpenAI-compatible tool-calling APIs (confirmed live
 * against this repo's only reachable provider, openrouter) reject any
 * character outside `[a-zA-Z0-9_-]` in a tool name, so a literal `/` (or a
 * raw `.` from ws-mcp's own dotted names) breaks the entire tool-bearing
 * turn, not just one call.
 *
 * The REGISTERED name is therefore sanitized (`/` -> `__` namespace
 * separator, `.` -> `_` within-tool separator: `registeredName = "ws__" +
 * rawName.replaceAll(".", "_")`, e.g. `playbook.read` -> `ws__playbook_read`),
 * matching the shape the reference harnesses already use for these same
 * tools (Claude Code registers them as `mcp__plugin_ws_ws__playbook_read`).
 * The model maps the unmodified `ws/playbook.read(...)` SKILL.md prose to
 * the sanitized registered name itself (prose is not rewritten here — it is
 * not this bridge's to rewrite). Dispatch to ws-mcp always uses the RAW
 * dotted `rawName` (`client.callTool(rawName, ...)`) — sanitization is
 * registration-only and never touches the wire call to ws-mcp.
 */

import { execFile } from "node:child_process";
import type { ExtensionAPI, ExtensionUIContext } from "@earendil-works/pi-coding-agent";
import { spawnWsMcpClient, type McpStdioClient, type McpContentItem, type McpToolCallResult } from "./mcp-stdio-client.ts";
import { assertVersionPin, readRuntimeContract } from "./version-check.ts";
import { buildLocalDevenvBootstrap, type LocalDevenvContext } from "./local-devenv.ts";
import { WS_PI_PARENT_SESSION_KEY_ENV, isLeadOrFork, readSpawnRole, type SpawnRole } from "./process-role.ts";
// Value import from spawner.ts is safe: spawner.ts only imports `type
// BridgeHandle` from this file (type-only, erased at build/runtime), so no
// runtime circular import is created in either direction.
import { resolveModelForAliasViaWsMcp, inheritModelFromToolCtx } from "./spawner.ts";
import { modelCatalogFromToolCtx, formatTierWarning, type ModelCatalogEntry, type TierRejection } from "./model-catalog.ts";
import { registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";

import type { ForkContext } from "./fork-context.ts";

export interface BridgeOptions {
  forkContext?: ForkContext;
  previousOwnKeys?: readonly string[];
  launcherPath: string;
  pluginDir: string;
  runtimeJsonPath: string;
  /** Working directory of the Pi session — used as the ferrule bootstrap root. */
  cwd: string;
  /** Filled independently before MCP startup; native fallback remains available when absent. */
  toolPreviewTuiRef: ToolPreviewTuiRef;
  ui?: ExtensionUIContext;
}

export interface BridgeHandle {
  /** Idempotent — safe to call more than once (e.g. a duplicate session_shutdown). */
  shutdown(): void;
  /**
   * The bridge's already-connected ws-mcp client, threaded out so Phase 2's
   * spawner can issue its own `playbook.render` / `ferrule` calls without
   * opening a second connection to the launcher.
   */
  client: McpStdioClient;
  /**
   * The same default-filled session_key ref used by every bridged tool's
   * fill-or-forward path (`resolveSessionKey`). A live object reference, not
   * a snapshot — reads the current value even if the bootstrap ferrule call
   * resolves after a caller has already captured this handle.
   */
  defaultSessionKeyRef: { current: string | undefined };
  /** Sanitized `ws__*` registered tool names (see `sanitizeToolName`), for the spawner's `full-worker` tool group. */
  wsToolNames: readonly string[];
  /**
   * The full `workflow_manual` CONTINUE-response text, fetched once
   * (lead/fork roles only) right after the ferrule bootstrap succeeds. A
   * live ref, same convention as `defaultSessionKeyRef` — `undefined` for a
   * worker/explore role, or when the fetch fails/returns no text (degraded
   * bootstrap). Consumed by `lead-bootstrap.ts` (system-prompt injection)
   * and, indirectly, by the workflow_manual->workflow_state mapping below
   * (via `staticBodySnapshotRef`, not this ref).
   */
  manualSnapshotRef: { current: string | undefined };
  /**
   * The static manual-body snapshot (`playbook.read("lead-workflow-manual")`),
   * fetched in lockstep with `manualSnapshotRef` (same gate, same
   * all-or-nothing degraded fallback). Used by the workflow_manual->
   * workflow_state mapping's `cutStaticBody` call — `undefined` disables the
   * mapping entirely (forward every workflow_manual call verbatim).
   */
  staticBodySnapshotRef: { current: string | undefined };
}

function notify(ui: ExtensionUIContext | undefined, message: string, level: "info" | "warning" | "error" = "info"): void {
  if (ui) {
    ui.notify(message, level);
  } else {
    console.error(`[ws-pi-bridge] ${message}`);
  }
}

function firstText(result: McpToolCallResult): string | undefined {
  return result.content.find((item) => item.type === "text")?.text;
}

/**
 * Advisory appended to every `workflow_manual` response while harness `pi`'s
 * `agents.tier` alias table has no genuine `pi` entries (per
 * `computePiAliasTableReport` below). Mirrors the Go core's blockquote
 * bootstrap-staleness-advisory convention (`bootstrap_alarm.go`) — a `>
 * [!note]`-style block, re-warning on every read while the condition holds
 * rather than once per session (no ticket-mandated exact copy).
 */
export const MODEL_CATALOG_ADVISORY =
  "> [!note]\n" +
  "> **Pi's model tier table has no entries.** ws-agent-spawn (and ws-fork/" +
  "ws-execute) now refuse a named tier that has no genuine pi entry instead " +
  "of silently inheriting the parent session's model; simple explore refuses " +
  "the same way. Configure at " +
  "least a `small` tier for harness `pi` via `config.tune agents.tier " +
  "harness:pi` (see lead-tune) to route explore/recon to a cheaper model; " +
  "the other three fixed tiers (`medium`/`large`/`xlarge`) are yours to " +
  "curate too. Changes apply immediately — no restart needed.";

/**
 * The four fixed tiers `config.resolve_agent` understands — used by
 * `computePiAliasTableReport` below.
 */
const PI_TIERS = ["small", "medium", "large", "xlarge"] as const;

/**
 * Scan every tier through the spawn resolver, without an inherited model so an
 * accepted hit is distinguishable. Never stop at the first hit: later rejected
 * tiers still need diagnostics. No accepted/rejected hits (including all
 * transport failures) retains the empty-table advisory. No notifications here.
 */
export interface PiAliasTableReport {
  /**
   * True when every one of the four fixed tiers is rejected `unset` (i.e.
   * every `config.resolve_agent` call answers with a non-`pi` `resolved_from`
   * — no tier has ever been targeted at harness pi at all). A genuine pi
   * accept, or an `unknown`/`no-auth` rejection on ANY tier, flips this to
   * `false` even while other tiers remain unset — the all-unset guidance
   * block and the per-tier rejected-rows table are mutually exclusive (see
   * `maybeAppendModelCatalogAdvisory`).
   */
  unset: boolean;
  rejected: Array<{ alias: string; rejected: TierRejection }>;
}

export async function computePiAliasTableReport(callTool: WorkflowManualMappingDeps["callTool"], catalog: readonly ModelCatalogEntry[] = []): Promise<PiAliasTableReport> {
  const report: PiAliasTableReport = { unset: true, rejected: [] };
  for (const alias of PI_TIERS) {
    const { model, rejected } = await resolveModelForAliasViaWsMcp({ callTool }, alias, undefined, catalog);
    // A genuine pi accept, or any rejection OTHER than "unset", proves the
    // table isn't all-unset; a "unset" rejection (resolved_from !== "pi")
    // still gets a row in `report.rejected` but does not by itself disprove
    // "every tier is rejected unset" — see `PiAliasTableReport.unset`'s
    // redefinition.
    if (model !== undefined || (rejected && rejected.why !== "unset")) report.unset = false;
    if (rejected) report.rejected.push({ alias, rejected });
  }
  return report;
}

/**
 * The raw-dispatch advisory gate: only a `workflow_manual` call pays for the
 * extra `config.resolve_agent` round-trips `computePiAliasTableReport` needs
 * — every other bridged tool call skips it entirely (`callTool` is never
 * invoked, the report has neither an unset flag nor rejections). Extracted out of
 * `startBridge`'s raw-dispatch closure as its own function specifically so
 * the gate is directly unit-testable: `startBridge` itself spawns a real
 * ws-mcp subprocess and cannot be exercised in a unit test (review relay #1,
 * Important/test).
 */
export async function computeRawDispatchPiAliasTableReport(
  rawName: string,
  callTool: WorkflowManualMappingDeps["callTool"],
  catalog: readonly ModelCatalogEntry[] = [],
): Promise<PiAliasTableReport> {
  return rawName === "workflow_manual" ? await computePiAliasTableReport(callTool, catalog) : { unset: false, rejected: [] };
}

/**
 * Per-session dedup memory for `maybeAppendModelCatalogAdvisory`: `current`
 * holds the last emitted advisory key (see `buildAdvisoryKey`), or
 * `undefined` when no advisory has been emitted since the last reset (a
 * clean table, or a compaction boundary). Owned by `startBridge` (one holder
 * per session) and passed in as a parameter so the function it gates stays
 * IO-free/pure for the existing direct-call tests — an omitted holder means
 * "no dedup memory," i.e. always append while a rejection exists (today's
 * behavior, unchanged).
 */
export type AdvisoryKeyHolder = { current: string | undefined };

/**
 * Stable string key for a `PiAliasTableReport`'s rejected set, used to dedupe
 * advisory emission per session. Three distinct shapes:
 * - Clean table (no `unset`, no rejections): `""` — a dedicated sentinel so a
 *   holder can be reset back to "no advisory emitted."
 * - `report.unset === true` with an EMPTY `rejected` array (every tier hit a
 *   transport/parse miss, not a genuine `unset` `config.resolve_agent`
 *   answer — see `computePiAliasTableReport`'s doc comment): `"unset"`, its
 *   own sentinel, distinct from the clean-table `""` key even though both
 *   have an empty `rejected` array.
 * - Otherwise (rejected has one or more rows, whether or not `unset` is also
 *   true): a sorted, joined `<alias>=<model or "->:<why>` key so tier order
 *   in `report.rejected` never causes a spurious "changed" key.
 */
export function buildAdvisoryKey(report: PiAliasTableReport): string {
  if (!report.unset && report.rejected.length === 0) return ""; // clean table
  if (report.rejected.length === 0) return "unset"; // all-miss/empty-catalog, no per-tier detail
  return [...report.rejected]
    .sort((a, b) => a.alias.localeCompare(b.alias))
    .map(({ alias, rejected }) => `${alias}=${rejected.model || "-"}:${rejected.why}`)
    .join(",");
}

/**
 * Append one advisory text item on a copy, only for workflow_manual.
 * `report.unset` (every tier rejected `unset`, or every tier a bare
 * transport/parse miss) selects the guidance block; any other
 * non-all-unset table with at least one rejection (including a `unset` one
 * sitting alongside a genuine hit or an `unknown`/`no-auth` rejection)
 * selects the per-tier rows instead — the two forms are exclusive, keyed on
 * `unset`, NOT on whether `rejected` happens to be non-empty (an all-unset
 * table's `rejected` is non-empty too, once `unset` rejections are tracked).
 * An accepted-only table (no rejections, not all-unset) returns the
 * original content reference. No human command pointer.
 *
 * `holder`, when supplied, gates emission to once per distinct rejected set
 * (see `buildAdvisoryKey`): a repeat call with the same key is a no-op
 * (returns `content` unchanged, same reference); a changed key re-appends
 * and updates the holder; a clean table resets the holder to `undefined` so
 * a later rejection warns again. Omitting `holder` preserves today's
 * behavior exactly — always append while a rejection exists — so every
 * existing direct-call test keeps passing unchanged.
 */
export function maybeAppendModelCatalogAdvisory(
  rawName: string,
  content: McpContentItem[],
  report: PiAliasTableReport,
  inheritModel?: string,
  catalogEmpty = true,
  holder?: AdvisoryKeyHolder,
): McpContentItem[] {
  if (rawName !== "workflow_manual") return content;
  const hasRejection = report.unset || report.rejected.length > 0;
  if (!hasRejection) {
    if (holder) holder.current = undefined;
    return content;
  }
  if (holder) {
    const key = buildAdvisoryKey(report);
    if (key === holder.current) return content;
    holder.current = key;
  }
  const text = report.unset
    ? MODEL_CATALOG_ADVISORY
    : "> [!note]\n" + report.rejected.map(({ alias, rejected }) => `> ${formatTierWarning(alias, rejected, inheritModel, catalogEmpty)}`).join("\n");
  return [...content, { type: "text", text }];
}

/**
 * Fixed line prepended to every mapped `workflow_manual` response — both the
 * cut-success branch and the workflow_state-fallback branch (§3). Wording is
 * pinned by the ticket contract.
 */
const WORKFLOW_STATE_MAPPING_LINE = "Workflow manual is in your system prompt; this is your current session state.";

export function prependWorkflowStateLine(text: string): string {
  return `${WORKFLOW_STATE_MAPPING_LINE}\n\n${text}`;
}

/**
 * The reasons `cutStaticBody` can fail to produce a cut. `"no-body"` is not
 * a fallback trigger — it means ws-mcp rendered no manual body at all (its
 * no-restorable-state notice), so the caller forwards the response
 * unchanged instead of dispatching `workflow_state`.
 */
export type StaticBodyCutMissReason = "start-anchor" | "end-anchor" | "order" | "no-body";

/**
 * Anchor-cut, not substring-cut: the start anchor is `staticBodySnapshot`'s
 * first non-empty line, matched as a whole line at its first occurrence in
 * `response`; the end anchor is the literal `## Session Key` heading line
 * (ws-mcp always appends `\n\n## Session Key\n<key>` after the manual body,
 * per `injectSessionKeyLine` — see `workflow_manual.go`). Everything ws-mcp
 * prepends (warnings, `# Manuals`, skeptical-posture block) stays ahead of
 * the start anchor; everything it appends (`## Session Key` onward: session
 * state, notes) stays from the end anchor onward — the byte-identical
 * substring assumption this replaced could not survive ws-mcp re-wrapping
 * or re-flowing the manual body between the session-start snapshot and a
 * later call, even though the render always keeps this line-anchored shape.
 *
 * Four outcomes:
 * - Both anchors found, in order (end anchor at or after the start anchor's
 *   line): `found: true`, `text` is the response with everything from the
 *   start-anchor line up to (not including) the end-anchor line removed —
 *   the end-anchor line itself is kept, in the retained tail.
 * - Only the start anchor missing: `reason: "start-anchor"`.
 * - Only the end anchor missing: `reason: "end-anchor"`.
 * - Both found but out of order (end anchor's line is at or before the
 *   start anchor's line): `reason: "order"`.
 * - Neither anchor present: `reason: "no-body"` — ws-mcp's
 *   no-restorable-state notice shape; the caller must NOT treat this as a
 *   renderer-drift fallback trigger (see `dispatchMappedWorkflowManual`).
 *
 * Pure, synchronous, no IO — the mapping's IO wrapper below calls this on an
 * already-fetched response body.
 */
export function cutStaticBody(response: string, staticBodySnapshot: string): { text: string; found: boolean; reason?: StaticBodyCutMissReason } {
  const startLine = staticBodySnapshot.split("\n").find((line) => line.length > 0);

  const lines = response.split("\n");
  let startOffset = -1;
  let endOffset = -1;
  let offset = 0;
  for (const line of lines) {
    if (startOffset === -1 && startLine !== undefined && line === startLine) {
      startOffset = offset;
    }
    if (endOffset === -1 && line === "## Session Key") {
      endOffset = offset;
    }
    offset += line.length + 1; // +1 for the "\n" split away by String.split.
  }

  const startFound = startOffset !== -1;
  const endFound = endOffset !== -1;

  if (!startFound && !endFound) {
    return { text: response, found: false, reason: "no-body" };
  }
  if (!startFound) {
    return { text: response, found: false, reason: "start-anchor" };
  }
  if (!endFound) {
    return { text: response, found: false, reason: "end-anchor" };
  }
  if (endOffset <= startOffset) {
    return { text: response, found: false, reason: "order" };
  }
  return { text: response.slice(0, startOffset) + response.slice(endOffset), found: true };
}

/**
 * Replaces the first `{type:"text"}` item's text with `mappedText` on a COPY
 * of `content` (never mutated in place); if `content` carries no text item at
 * all, `mappedText` is unshifted as a new leading item instead. Mirrors
 * `maybeAppendModelCatalogAdvisory`'s copy-not-mutate contract so the two
 * compose safely (this function's output is always fed into that one next).
 */
function replaceFirstTextItem(content: McpContentItem[], mappedText: string): McpContentItem[] {
  let replaced = false;
  const next = content.map((item) => {
    if (!replaced && item.type === "text") {
      replaced = true;
      return { ...item, text: mappedText };
    }
    return item;
  });
  if (!replaced) {
    next.unshift({ type: "text", text: mappedText });
  }
  return next;
}

/**
 * Pure role-gate predicate for the §3 workflow_manual -> workflow_state
 * mapping: `true` only when `rawName` is the literal `"workflow_manual"`
 * dispatch name, a static-body snapshot actually exists, AND the caller's
 * role is lead-or-fork. Extracted (review relay #1, cycle 1) from
 * `execute()`'s previously-inlined closure condition specifically so this
 * gate is unit-testable without a live `pi -e` session — mirrors
 * `lead-bootstrap.ts`'s `computeBeforeAgentStartResult` extraction for the
 * exact same reason (an inlined boolean-logic gate with no test seam is a
 * silent-regression risk: swapping `&&` for `||`, or dropping the role
 * check, would leak the mapping into a worker/explore `workflow_manual`
 * call with zero coverage to catch it).
 */
export function shouldMapWorkflowManual(rawName: string, hasSnapshot: boolean, role: SpawnRole | undefined): boolean {
  return rawName === "workflow_manual" && hasSnapshot && isLeadOrFork(role);
}

export interface WorkflowManualMappingDeps {
  /** Duck-typed subset of `McpStdioClient` — lets tests supply a stub with no real subprocess. */
  callTool: (name: string, args: Record<string, unknown>) => Promise<McpToolCallResult>;
  /** The `playbook.read("lead-workflow-manual")` snapshot fetched once at session_start. */
  staticBodySnapshot: string;
  catalog?: readonly ModelCatalogEntry[];
  inheritModel?: string;
  /**
   * Invoked on a cut-miss fallback (one anchor missing, or the anchors are
   * out of order) — never on `reason: "no-body"`, which is not a fallback
   * trigger. The caller is responsible for the "notify once per session"
   * dedupe (a closure flag in `startBridge`), not this function.
   */
  notifyMappingDegraded: (reason: Exclude<StaticBodyCutMissReason, "no-body">) => void;
  /** Threaded into every `maybeAppendModelCatalogAdvisory` call this dispatch makes, so all three call sites share one per-session dedup holder. */
  advisoryKeyHolder?: AdvisoryKeyHolder;
}

/**
 * IO wrapper for the `workflow_manual` -> `workflow_state` mapping (§3).
 * Dispatches `workflow_manual` with `args` (already normalized/resolved by
 * the caller) and anchor-cuts `deps.staticBodySnapshot`'s start line out of
 * the response (see `cutStaticBody`), branching three ways on the result:
 *
 * - Cut found: returns `prependWorkflowStateLine(cut text)`, re-wrapped
 *   through `maybeAppendModelCatalogAdvisory` keyed on the literal
 *   `"workflow_manual"` name (§3: the advisory still rides the mapped
 *   response, keyed on the tool's *registered* — i.e. ws-mcp's own raw
 *   dotted — name, not on which tool was actually dispatched to).
 * - `reason: "no-body"` (ws-mcp rendered no manual body at all — its
 *   no-restorable-state notice): forwards the original response unchanged
 *   (through the same fixed-line prepend and advisory keying) — no
 *   `notifyMappingDegraded` call, no `workflow_state` dispatch. There is no
 *   manual body to fall back away from.
 * - Any other miss (`"start-anchor"` / `"end-anchor"` / `"order"` —
 *   renderer drift): calls `deps.notifyMappingDegraded(cut.reason)`, then
 *   dispatches `workflow_state` instead — dropping `root` and any other
 *   `workflow_manual`-only arg by only forwarding `session_key` — prepends
 *   the same fixed line, and applies the same advisory keying.
 *
 * An `isError` result on either dispatch is thrown, matching the bridge's
 * existing non-mapped dispatch contract (`execute()`'s own `if
 * (result.isError) throw ...`).
 */
export async function dispatchMappedWorkflowManual(
  args: Record<string, unknown>,
  deps: WorkflowManualMappingDeps,
): Promise<{ content: McpContentItem[]; details: McpToolCallResult }> {
  const manualResult = await deps.callTool("workflow_manual", args);
  if (manualResult.isError) {
    throw new Error(firstText(manualResult) ?? "workflow_manual failed with no error text");
  }
  const manualText = firstText(manualResult) ?? "";
  const cut = cutStaticBody(manualText, deps.staticBodySnapshot);

  const piAliasTableReport = await computePiAliasTableReport(deps.callTool, deps.catalog);

  if (cut.found) {
    const content = replaceFirstTextItem(manualResult.content, prependWorkflowStateLine(cut.text));
    return { content: maybeAppendModelCatalogAdvisory("workflow_manual", content, piAliasTableReport, deps.inheritModel, !deps.catalog?.length, deps.advisoryKeyHolder), details: manualResult };
  }

  if (cut.reason === "no-body") {
    const content = replaceFirstTextItem(manualResult.content, prependWorkflowStateLine(manualText));
    return { content: maybeAppendModelCatalogAdvisory("workflow_manual", content, piAliasTableReport, deps.inheritModel, !deps.catalog?.length, deps.advisoryKeyHolder), details: manualResult };
  }

  deps.notifyMappingDegraded(cut.reason as Exclude<StaticBodyCutMissReason, "no-body">);
  const stateArgs: Record<string, unknown> = args.session_key === undefined ? {} : { session_key: args.session_key };
  const stateResult = await deps.callTool("workflow_state", stateArgs);
  if (stateResult.isError) {
    throw new Error(firstText(stateResult) ?? "workflow_state failed with no error text");
  }
  const stateText = firstText(stateResult) ?? "";
  const content = replaceFirstTextItem(stateResult.content, prependWorkflowStateLine(stateText));
  return { content: maybeAppendModelCatalogAdvisory("workflow_manual", content, piAliasTableReport, deps.inheritModel, !deps.catalog?.length, deps.advisoryKeyHolder), details: stateResult };
}

/**
 * Provider-legal registered name for a ws-mcp raw tool name: `ws__` prefix
 * (namespace separator, stands in for the `/` in the `ws/<rawName>` prose
 * form) plus the raw name's `.` separators flattened to `_`. Registration
 * only — never used for the wire call to ws-mcp, which always dispatches on
 * the untouched `rawName`.
 */
export function sanitizeToolName(rawName: string): string {
  return `ws__${rawName.replaceAll(".", "_")}`;
}

/**
 * Prefixes local-devenv source/commit/built-path context onto a launcher or
 * `initialize`/`assertVersionPin`/`listTools` failure, so the launcher's own
 * generic "incompatible ws-mcp runtime after repair" (or any other startup
 * failure) becomes actionable for a developer running against a source
 * build instead of a release download. `context` is `undefined` whenever
 * the local-devenv marker was absent or the caller's role skipped it
 * (worker/explore) — in that case `err` passes through unchanged (coerced to
 * `Error` if it wasn't already one, so the return type is always an `Error`
 * regardless of what was thrown).
 *
 * Extracted as its own pure function (matching this file's established
 * gate-extraction convention, e.g. `shouldMapWorkflowManual`) because
 * `startBridge` spawns a real subprocess and cannot be exercised in
 * `node --test`.
 */
export function wrapLaunchErrorWithLocalDevenvContext(err: unknown, context: LocalDevenvContext | undefined): Error {
  const original = err instanceof Error ? err : new Error(String(err));
  if (!context) return original;
  return new Error(
    `ws-pi-bridge: local-devenv build active (source_root=${context.sourceRoot}, commit=${context.sourceCommit}, ` +
      `built=${context.builtPath}): ${original.message}`,
  );
}

/**
 * Real `LocalDevenvBuildDeps.runBuild` implementation for `startBridge`'s
 * call site. Deviates from an earlier `stdio: "inherit"` sketch: Pi's TUI
 * owns the terminal at session-start time, so inheriting stdio here would
 * corrupt the TUI's rendering instead of showing build output. Captures
 * stdout/stderr via pipe instead and folds both streams into the thrown
 * `Error`'s message on a non-zero exit, so a build failure stays fully
 * diagnosable from the error alone (surfaced to the user via
 * `wrapLaunchErrorWithLocalDevenvContext` above once the build succeeds but
 * a later launch step fails, or directly when the build itself fails).
 *
 * Review fix (relay #1, Important #1): genuinely async (`execFile`, not
 * `execFileSync`) rather than merely returning a resolved-later Promise
 * around a blocking call. `buildLocalDevenvBootstrap`'s call site is
 * `notify(...)` immediately followed by `await deps.runBuild(...)`; Pi's
 * `ui.notify` defers its paint via `process.nextTick(() =>
 * this.scheduleRender())`, and a `nextTick` callback cannot run while a
 * synchronous `execFileSync` call still owns the stack. With the old
 * `execFileSync` version, the "building ws-mcp from ..." notification and
 * the "finished in Nms" notification both landed together only once the
 * whole build (and event loop) had already been blocked and released —
 * exactly the cold-build-vs-hang ambiguity the ticket's notify Decision
 * exists to prevent. `execFile` spawns without blocking, so the awaited
 * Promise genuinely suspends `buildLocalDevenvBootstrap` at that `await`,
 * letting the queued render (and the rest of the event loop) run while the
 * build is in flight.
 */
function runGoBuild(argv: string[], opts: { cwd: string }): Promise<void> {
  return new Promise((resolve, reject) => {
    execFile(argv[0], argv.slice(1), { cwd: opts.cwd, encoding: "buffer" }, (err, stdout, stderr) => {
      if (err) {
        const stdoutText = stdout ? stdout.toString() : "";
        const stderrText = stderr ? stderr.toString() : "";
        reject(new Error(`ws-pi-bridge: go build failed (${err.message})\n--- stdout ---\n${stdoutText}\n--- stderr ---\n${stderrText}`));
        return;
      }
      resolve();
    });
  });
}

const MERCENARY_RAW_PREFIX = "mercenary.";

/**
 * Module-level (not `startBridge`-local) one-time-registration guard for the
 * `pi.on("session_compact", ...)` listener that resets the active session's
 * advisory-key holder. `startBridge` runs inside `session_start`, which can
 * fire more than once per process (`/reload`), and `pi.on` returns no
 * unsubscribe handle — registering unconditionally inside `startBridge`
 * would stack one listener per reload. `activeAdvisoryKeyHolder` is
 * reassigned by every `startBridge` call so the (singly-registered) listener
 * always resets whichever session's holder is currently live.
 */
let compactionListenerRegistered = false;
let activeAdvisoryKeyHolder: AdvisoryKeyHolder | undefined;

/**
 * Drops every ws-mcp tool whose raw (pre-sanitization) name starts with
 * `mercenary.` from the list the bridge registers with Pi and exposes via
 * `wsToolNames` — independent of the server-side `workflow.prefer_mercenary`
 * knob (Open Decision #3, 260905-feat-ws-pi-harness-config-layer): no Pi
 * process, lead or child, can see or call the mercenary surface. Pure so it
 * is unit-testable without a live ws-mcp subprocess.
 */
export function filterOutMercenaryTools<T extends { name: string }>(tools: readonly T[]): T[] {
  return tools.filter((tool) => !tool.name.startsWith(MERCENARY_RAW_PREFIX));
}

/**
 * Drops `session_key` from a JSON-Schema's `required` array (keeping it in
 * `properties`, unchanged, so a caller can still supply it explicitly).
 *
 * Discovered live: Pi validates tool-call arguments against the registered
 * `parameters` schema *before* `execute()` ever runs (a typebox/JSON-Schema
 * checker walks the raw schema's `required` array structurally — no typebox
 * `Kind` wrapping needed for this either, consistent with the step-7 spike).
 * ws-mcp's own inputSchema marks `session_key` required on every root-aware
 * tool, so passing it through unmodified silences the session_key
 * fill-or-forward path entirely: Pi rejects an omitted-session_key call
 * with "must have required properties session_key" before the bridge's
 * `resolveSessionKey()` default-fill ever gets a chance to run. This is the
 * one schema edit the bridge makes — it does not add a synthetic
 * session_key property (ws-mcp's own inputSchema already declares it), it
 * only lifts the artificial requirement so "session_key stays optional and
 * caller-controllable" (ticket constraint) is actually true at the Pi
 * tool-call layer, not just inside execute().
 */
export function withOptionalSessionKey(inputSchema: Record<string, unknown>): Record<string, unknown> {
  const required = inputSchema.required;
  if (!Array.isArray(required) || !required.includes("session_key")) {
    return inputSchema;
  }
  return {
    ...inputSchema,
    required: required.filter((name) => name !== "session_key"),
  };
}

/**
 * The fresh-bootstrap sentinel a caller sends in `session_key` when it has no
 * lead session key yet (mirrors ws-mcp's own `freshBootstrapKey` constant,
 * `agents-plugin-tool/internal/mcp/workflow_manual.go`, and the identical
 * literal already shipped verbatim in `agents-plugin/rsrc/lead-proceed/
 * lead-proceed.md` and sibling skill text — this is the live, already-shipped
 * value, not an invented one).
 */
const FRESH_BOOTSTRAP_SENTINEL = "obsidian-latch";

export interface NormalizeSessionKeyOptions {
  /** The bridge's own default-filled session key (`defaultSessionKeyRef.current`), or undefined pre-bootstrap. */
  ownKey: string | undefined;
  /** The fresh-bootstrap sentinel value that rewrites to `ownKey`. */
  sentinel: string;
  /** The env-delivered parent lead key (fork-only, `WS_PI_PARENT_SESSION_KEY`), when set. */
  parentLeadKey?: string;
  parentLeadKeys?: readonly string[];
  /** Earlier child-owned keys, never inferred from transcript text. */
  previousOwnKeys?: readonly string[];
}

/** Forks never forward parent/stale keys; the caller turns this into a Pi tool error. */
export function forkSessionKeyRefusal(provided: unknown, opts: NormalizeSessionKeyOptions): string | undefined {
  if (readSpawnRole(process.env) !== "fork" || typeof provided !== "string") return undefined;
  const current = opts.ownKey ? ` Use current fork key "${opts.ownKey}".` : " Fork bootstrap is not ready yet.";
  if ((opts.parentLeadKey && provided === opts.parentLeadKey) || opts.parentLeadKeys?.includes(provided)) return `ws-pi-agent: fork refuses its parent session key.${current}`;
  if (opts.previousOwnKeys?.includes(provided) && provided !== opts.ownKey) return `ws-pi-agent: fork refuses its stale prior session key.${current}`;
  return undefined;
}

/**
 * Refuses parent and historical own keys in fork role, even before bootstrap.
 * Only the fresh-bootstrap sentinel is rewritten to a ready own key. Ordinary
 * leads and unrelated explicit worker keys retain fill-or-forward behavior.
 * Without an own key, the sentinel still reaches ws-mcp's bootstrap fallback.
 *
 * Never mutates `params` — copy-on-write, same contract as
 * `resolveSessionKey`.
 */
export function normalizeSessionKey(
  params: Record<string, unknown> | undefined,
  opts: NormalizeSessionKeyOptions,
): Record<string, unknown> | undefined {
  const provided = params?.session_key;
  const refusal = forkSessionKeyRefusal(provided, opts);
  if (refusal) throw new Error(refusal);
  if (!opts.ownKey) return params;
  if (provided === opts.sentinel) {
    return { ...(params ?? {}), session_key: opts.ownKey };
  }
  return params;
}

/**
 * session_key fill-or-forward: if the caller omitted session_key (undefined,
 * null, or empty string), splice in the bridge's default-filled key; an
 * explicit session_key passes through completely unchanged. This is what
 * keeps subagent lineage / lead multi-track orchestration viable later even
 * though building that machinery is out of scope for this phase.
 *
 * Never mutates the tool's registered `parameters` schema — only the
 * per-call arguments object.
 */
export function resolveSessionKey(
  params: Record<string, unknown> | undefined,
  defaultKeyRef: { current: string | undefined },
): Record<string, unknown> {
  const args: Record<string, unknown> = params && typeof params === "object" ? { ...params } : {};
  const provided = args.session_key;
  if (provided === undefined || provided === null || provided === "") {
    if (defaultKeyRef.current) {
      args.session_key = defaultKeyRef.current;
    }
    // else: leave omitted so ws-mcp's own mandatory_session_key recovery
    // guidance surfaces to the caller instead of the bridge swallowing it.
  }
  return args;
}

export async function startBridge(pi: ExtensionAPI, opts: BridgeOptions): Promise<BridgeHandle> {
  const runtime = readRuntimeContract(opts.runtimeJsonPath);

  // Lead/fork-only: a worker/explore child never consults the local-devenv
  // marker or builds ws-mcp itself — it reuses whatever the launcher already
  // installed for the lead via the compatibility stamp. Runs before the
  // launcher is spawned so a defined result's env fragment can be threaded
  // into spawnWsMcpClient below.
  let localDevenvContext: LocalDevenvContext | undefined;
  let launcherEnv: Record<string, string> | undefined;
  if (isLeadOrFork(readSpawnRole(process.env))) {
    const bootstrap = await buildLocalDevenvBootstrap(opts.pluginDir, runtime.plugin_version, {
      runBuild: runGoBuild,
      notify: (m) => notify(opts.ui, `ws-pi-bridge: ${m}`),
    });
    if (bootstrap) {
      launcherEnv = bootstrap.env;
      localDevenvContext = bootstrap.context;
    }
  }

  const client = spawnWsMcpClient(
    opts.launcherPath,
    opts.pluginDir,
    (line) => {
      console.error(`[ws-mcp] ${line.trimEnd()}`);
    },
    launcherEnv,
  );

  let shutdownCalled = false;
  const shutdown = () => {
    if (shutdownCalled) return;
    shutdownCalled = true;
    client.close();
  };

  // Declared outside the try block (not just `const` inside it) so the
  // Phase 2 spawner's return-value fields below can still see them after a
  // successful try — `try { const x = ... }` block-scopes `x` to the try
  // block itself.
  let tools: Awaited<ReturnType<typeof client.listTools>> = [];
  const defaultKeyRef: { current: string | undefined } = { current: undefined };
  const manualSnapshotRef: { current: string | undefined } = { current: undefined };
  const staticBodySnapshotRef: { current: string | undefined } = { current: undefined };
  // Per-session "notify once" dedupe (§3) for the workflow_manual mapping's
  // cut-miss fallback — a closure flag scoped to this startBridge call
  // (one bridge per Pi session), not a module-level global.
  let notifiedMappingDegraded = false;
  // Per-session advisory-key dedup memory (see `maybeAppendModelCatalogAdvisory`).
  // The listener that resets it on compaction is module-scoped (see
  // `compactionListenerRegistered`'s doc comment) so a `/reload`-driven
  // re-entry into `session_start` (and thus this function) never stacks a
  // second `pi.on` registration; this holder itself is still fresh per call.
  const advisoryKeyHolder: AdvisoryKeyHolder = { current: undefined };
  activeAdvisoryKeyHolder = advisoryKeyHolder;
  if (!compactionListenerRegistered) {
    compactionListenerRegistered = true;
    pi.on("session_compact", () => {
      if (activeAdvisoryKeyHolder) activeAdvisoryKeyHolder.current = undefined;
    });
  }

  try {
    const initResult = await client.initialize({
      name: "ws-pi-bridge",
      version: "0.1.0",
    });
    assertVersionPin(runtime, initResult.serverInfo.version);

    tools = filterOutMercenaryTools(await client.listTools());
    for (const tool of tools) {
      const rawName = tool.name;
      const registeredName = sanitizeToolName(rawName);
      // The common seam supplies this sanitized provider-visible title while
      // preserving bridge dispatch and all original tool definition fields.
      registerWsTool(pi, {
        name: registeredName,
        label: rawName,
        description: tool.description ?? rawName,
        // Raw JSON-Schema pass-through — confirmed empirically (see the
        // step-7 spike note in the implementation plan / commit history):
        // pi's own tool-to-provider-schema conversion
        // (getJsonSchemaToolParameters in its bundled provider chunks) treats
        // `parameters` as a plain JSON-Schema object and forwards it
        // verbatim to the provider API; it does not require typebox's Kind
        // symbols at runtime. ws-mcp's inputSchema is already a plain
        // {type, properties, required} object, so no typebox shim is needed.
        parameters: withOptionalSessionKey(tool.inputSchema) as never,
        async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
          // Dispatch always uses the RAW dotted name — sanitization is
          // registration-only, never part of the ws-mcp wire call.
          // Refuse inherited/stale fork keys before sentinel normalization and
          // omitted-key fill. Explicit unrelated keys are never rewritten.
          const rawParams = params as Record<string, unknown> | undefined;
          const refusal = forkSessionKeyRefusal(rawParams?.session_key, {
            ownKey: defaultKeyRef.current,
            sentinel: FRESH_BOOTSTRAP_SENTINEL,
            parentLeadKey: opts.forkContext?.parentSessionKey ?? process.env[WS_PI_PARENT_SESSION_KEY_ENV],
            previousOwnKeys: opts.previousOwnKeys,
            parentLeadKeys: opts.forkContext?.parentSessionKeys,
          });
          if (refusal) throw new Error(refusal);
          const normalized = normalizeSessionKey(rawParams, {
            ownKey: defaultKeyRef.current,
            sentinel: FRESH_BOOTSTRAP_SENTINEL,
            // WS_PI_PARENT_SESSION_KEY_ENV is unset until a future
            // fork-spawning ticket sets it (out of scope here — this phase
            // only reserves the env var name via process-role.ts).
            parentLeadKey: process.env[WS_PI_PARENT_SESSION_KEY_ENV],
          });
          const args = resolveSessionKey(normalized, defaultKeyRef);
          const catalog = rawName === "workflow_manual" ? modelCatalogFromToolCtx(toolCtx) : [];
          const inheritModel = inheritModelFromToolCtx(toolCtx);

          // §3 workflow_manual -> workflow_state mapping: only for lead/fork
          // roles, and only once a static-body snapshot actually exists
          // (both are the "degraded bootstrap" escape hatch — worker/explore
          // roles and a failed/skipped snapshot fetch both forward
          // workflow_manual verbatim, exactly as today).
          if (shouldMapWorkflowManual(rawName, Boolean(staticBodySnapshotRef.current), readSpawnRole(process.env))) {
            return await dispatchMappedWorkflowManual(args, {
              callTool: (name, callArgs) => client.callTool(name, callArgs),
              catalog,
              inheritModel,
              // shouldMapWorkflowManual already asserted this is truthy — TS
              // can't see through the predicate call, so this cast is safe
              // and load-bearing only for the type checker, not runtime.
              staticBodySnapshot: staticBodySnapshotRef.current as string,
              advisoryKeyHolder,
              notifyMappingDegraded: (reason) => {
                if (!notifiedMappingDegraded) {
                  notifiedMappingDegraded = true;
                  const missing =
                    reason === "start-anchor"
                      ? "the manual body's start heading (its first non-empty line) is missing from the response"
                      : reason === "end-anchor"
                        ? "the '## Session Key' end heading is missing from the response"
                        : "the '## Session Key' end heading appears before the manual body's start heading";
                  notify(
                    opts.ui,
                    `ws-pi-bridge: workflow_manual's response could not be anchor-cut (${missing}) — falling back to workflow_state; per-call advisories are unavailable for the rest of this session`,
                    "warning",
                  );
                }
              },
            });
          }

          const result = await client.callTool(rawName, args);
          if (result.isError) {
            // Throwing is how Pi's tool contract signals isError: true —
            // returning a value never sets it (docs/extensions.md#L1953-2011).
            throw new Error(firstText(result) ?? `${registeredName} failed with no error text`);
          }
          // The extra config.resolve_agent round-trips this needs are gated
          // on rawName === "workflow_manual" first, so no other bridged tool
          // call pays for an unrelated MCP round-trip — see
          // computeRawDispatchPiAliasTableReport's doc comment.
          const piAliasTableReport = await computeRawDispatchPiAliasTableReport(rawName, (name, callArgs) => client.callTool(name, callArgs), catalog);
          const content = maybeAppendModelCatalogAdvisory(rawName, result.content, piAliasTableReport, inheritModel, catalog.length === 0, advisoryKeyHolder);
          return { content, details: result };
        },
      }, opts.toolPreviewTuiRef);
    }

    // Default-fill key bootstrap: mint a session_key via ferrule so that
    // omitted-session_key calls resolve instead of failing outright.
    try {
      const ferruleResult = await client.callTool("ferrule", { root: opts.cwd, format: "json" });
      if (ferruleResult.isError) {
        notify(opts.ui, `ws-pi-bridge: ferrule bootstrap failed: ${firstText(ferruleResult)}`, "warning");
      } else {
        const text = firstText(ferruleResult);
        if (text) {
          const parsed = JSON.parse(text) as { session_key?: string };
          if (parsed.session_key) {
            defaultKeyRef.current = parsed.session_key;
          } else {
            notify(opts.ui, "ws-pi-bridge: ferrule response carried no session_key", "warning");
          }
        }
      }
    } catch (err) {
      // Leave defaultKeyRef.current unset — a subsequent omitted-session_key
      // call then surfaces ws-mcp's own mandatory_session_key guidance
      // rather than the bridge swallowing the failure silently.
      notify(opts.ui, `ws-pi-bridge: ferrule bootstrap threw: ${(err as Error).message}`, "warning");
    }

    // §1/§3 session-start snapshot fetch: the full workflow_manual response
    // (for lead-bootstrap.ts's system-prompt block) and the static
    // manual-body snapshot (for this bridge's own workflow_manual mapping),
    // fetched once here, right after the ferrule bootstrap. Gated on
    // isLeadOrFork so a worker/explore child — which loads this same
    // extension but never uses either — skips the extra round-trip
    // entirely. Also gated on defaultKeyRef.current actually being set: a
    // failed ferrule bootstrap is already the degraded-bootstrap case, and
    // both refs staying unset is exactly what that case needs (§3).
    //
    // All-or-nothing: per the plan, a failure or empty-text response on
    // EITHER call leaves BOTH refs unset (never a manual snapshot with no
    // static-body snapshot, or vice versa) — the ws system-prompt block and
    // the workflow_manual mapping degrade together, not independently.
    if (defaultKeyRef.current && isLeadOrFork(readSpawnRole(process.env))) {
      // A delivered fork prompt is immutable and must not be replaced by a child mapping fetch.
      const inheritedPrompt = readSpawnRole(process.env) === "fork" && Boolean(opts.forkContext);
      if (!inheritedPrompt) {
        try {
          const manualResult = await client.callTool("workflow_manual", { session_key: defaultKeyRef.current });
          const manualText = !manualResult.isError ? firstText(manualResult) : undefined;
          if (manualText) manualSnapshotRef.current = manualText;
        } catch (err) {
          notify(opts.ui, `ws-pi-bridge: session-start manual snapshot fetch threw: ${(err as Error).message}`, "warning");
        }
      }
      try {
        const staticBodyResult = await client.callTool("playbook.read", { name: "lead-workflow-manual", session_key: defaultKeyRef.current });
        const staticBodyText = !staticBodyResult.isError ? firstText(staticBodyResult) : undefined;
        if (staticBodyText) staticBodySnapshotRef.current = staticBodyText;
      } catch (err) {
        notify(opts.ui, `ws-pi-bridge: session-start static-body fetch threw: ${(err as Error).message}`, "warning");
      }
    }

    notify(opts.ui, `ws-pi-bridge: registered ${tools.length} ws__* tools from ws-mcp ${initResult.serverInfo.version}`);
  } catch (err) {
    shutdown();
    throw wrapLaunchErrorWithLocalDevenvContext(err, localDevenvContext);
  }

  return {
    shutdown,
    client,
    defaultSessionKeyRef: defaultKeyRef,
    wsToolNames: tools.map((tool) => sanitizeToolName(tool.name)),
    manualSnapshotRef,
    staticBodySnapshotRef,
  };
}
