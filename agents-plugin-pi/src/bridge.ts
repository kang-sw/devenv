/**
 * Tool re-registration bridge: spawns the ws-mcp launcher, version-checks
 * it, lists its tools, and re-registers each one on Pi under a sanitized
 * name derived from "ws/" + rawName.
 *
 * SKILL.md prose is written as the literal `ws/playbook.print(...)` /
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
 * rawName.replaceAll(".", "_")`, e.g. `playbook.print` -> `ws__playbook_print`),
 * matching the shape the reference harnesses already use for these same
 * tools (Claude Code registers them as `mcp__plugin_ws_ws__playbook_print`).
 * The model maps the unmodified `ws/playbook.print(...)` SKILL.md prose to
 * the sanitized registered name itself (prose is not rewritten here — it is
 * not this bridge's to rewrite). Dispatch to ws-mcp always uses the RAW
 * dotted `rawName` (`client.callTool(rawName, ...)`) — sanitization is
 * registration-only and never touches the wire call to ws-mcp.
 */

import { keyHint, type ExtensionAPI, type ExtensionUIContext } from "@earendil-works/pi-coding-agent";
import { stringify as stringifyYaml } from "yaml";
import { spawnWsMcpClient, type McpStdioClient, type McpContentItem, type McpToolCallResult } from "./mcp-stdio-client.ts";
import { assertVersionPin, readRuntimeContract } from "./version-check.ts";
import { WS_PI_PARENT_SESSION_KEY_ENV, isLeadOrFork, readSpawnRole, type SpawnRole } from "./process-role.ts";
// Value import from spawner.ts is safe: spawner.ts only imports `type
// BridgeHandle` from this file (type-only, erased at build/runtime), so no
// runtime circular import is created in either direction.
import { resolveModelForAliasViaWsMcp, inheritModelFromToolCtx } from "./spawner.ts";
import { modelCatalogFromToolCtx, formatTierWarning, type ModelCatalogEntry, type TierRejection } from "./model-catalog.ts";

export interface BridgeOptions {
  launcherPath: string;
  pluginDir: string;
  runtimeJsonPath: string;
  /** Working directory of the Pi session — used as the ferrule bootstrap root. */
  cwd: string;
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
   * The static manual-body snapshot (`playbook.print("lead-workflow-manual")`),
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
 * Converts JSON containers to YAML for Pi's display only. Scalars are left
 * alone: ws-mcp often returns JSON strings/numbers as intentional prose-like
 * values, and parsing them is not permission to reformat them.
 */
export function yamlDisplayText(text: string): string {
  try {
    const value: unknown = JSON.parse(text);
    if (!Array.isArray(value) && (typeof value !== "object" || value === null)) return text;
    return stringifyYaml(value);
  } catch {
    return text;
  }
}

/**
 * Builds the display body from MCP's original ordered content. Only the first
 * text block is eligible for YAML conversion; later text remains byte-for-byte
 * raw and non-text content stays in the result for Pi's own image handling.
 */
export function renderResultText(content: ReadonlyArray<Pick<McpContentItem, "type" | "text">>, isError: boolean): string {
  const firstTextIndex = content.findIndex((item) => item.type === "text");
  return content
    .filter((item) => item.type === "text")
    .map((item, textIndex) => textIndex === 0 && firstTextIndex !== -1 && !isError ? yamlDisplayText(item.text ?? "") : (item.text ?? ""))
    .join("\n");
}

const ANSI_SEQUENCE = /\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;
const graphemeSegmenter = new Intl.Segmenter(undefined, { granularity: "grapheme" });

function graphemeDisplayWidth(grapheme: string): number {
  if (/^(?:\p{Control}|\p{Mark}|\p{Default_Ignorable_Code_Point})+$/u.test(grapheme)) return 0;
  const base = grapheme.replace(/^[\p{Control}\p{Mark}\p{Default_Ignorable_Code_Point}]+/u, "");
  const code = base.codePointAt(0);
  if (code === undefined) return 0;
  // Emoji sequences and regional indicators render as a single two-cell glyph.
  if (/\p{Extended_Pictographic}/u.test(grapheme) || (code >= 0x1f1e6 && code <= 0x1f1ff)) return 2;
  return (
    (code >= 0x1100 && code <= 0x115f) ||
    (code >= 0x2329 && code <= 0x232a) ||
    (code >= 0x2e80 && code <= 0xa4cf) ||
    (code >= 0xac00 && code <= 0xd7a3) ||
    (code >= 0xf900 && code <= 0xfaff) ||
    (code >= 0xfe10 && code <= 0xfe6f) ||
    (code >= 0xff01 && code <= 0xff60) ||
    (code >= 0xffe0 && code <= 0xffe6) ||
    (code >= 0x20000 && code <= 0x3fffd)
  ) ? 2 : 1;
}

/** Width in terminal cells, with ANSI sequences zero-width and graphemes intact. */
export function visibleDisplayWidth(text: string): number {
  const plain = text.replace(ANSI_SEQUENCE, "").replace(/\t/g, "   ");
  let width = 0;
  for (const { segment } of graphemeSegmenter.segment(plain)) width += graphemeDisplayWidth(segment);
  return width;
}

function wrapDisplayLine(text: string, width: number): string[] {
  const maxWidth = Math.max(1, Math.floor(width));
  if (text.length === 0) return [""];
  const rows: string[] = [];
  let row = "";
  let rowWidth = 0;
  let cursor = 0;
  const appendGraphemes = (chunk: string) => {
    for (const { segment } of graphemeSegmenter.segment(chunk)) {
      const segmentWidth = graphemeDisplayWidth(segment);
      if (rowWidth > 0 && rowWidth + segmentWidth > maxWidth) {
        rows.push(row);
        row = "";
        rowWidth = 0;
      }
      // A one-column viewport cannot represent a wide grapheme. Match the
      // terminal's only width-safe option rather than overflowing its row.
      if (rowWidth === 0 && segmentWidth > maxWidth) {
        rows.push("?");
        continue;
      }
      row += segment;
      rowWidth += segmentWidth;
    }
  };
  for (const match of text.matchAll(ANSI_SEQUENCE)) {
    appendGraphemes(text.slice(cursor, match.index));
    row += match[0];
    cursor = (match.index ?? 0) + match[0].length;
  }
  appendGraphemes(text.slice(cursor));
  if (row.length > 0) rows.push(row);
  return rows.length > 0 ? rows : [""];
}

/**
 * Returns visual display rows for a tool result. The collapsed marker counts
 * wrapped body rows (not source newlines), so the number remains truthful at
 * narrow widths and for wide Unicode.
 */
export function renderResultRows(
  content: ReadonlyArray<Pick<McpContentItem, "type" | "text">>,
  isError: boolean,
  expanded: boolean,
  width: number,
  expandHint = keyHint("app.tools.expand", "to expand"),
): string[] {
  const text = renderResultText(content, isError);
  if (!text) return [];
  const logicalLines = text.replace(/\r\n?/g, "\n").split("\n");
  if (logicalLines.at(-1) === "") logicalLines.pop();
  const bodyRows = logicalLines.flatMap((line) => wrapDisplayLine(line, width));
  if (expanded || bodyRows.length <= 10) return bodyRows;
  const remaining = bodyRows.length - 10;
  return [...bodyRows.slice(0, 10), ...wrapDisplayLine(`… ${remaining} more rows (${expandHint})`, width)];
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
  "> **Pi's model tier table has no entries.** ws-agent-spawn and explore " +
  "currently silently inherit the parent session's model for every " +
  "delegated agent — costly for cheap recon/explore work. Configure at " +
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
  unset: boolean;
  rejected: Array<{ alias: string; rejected: TierRejection }>;
}

export async function computePiAliasTableReport(callTool: WorkflowManualMappingDeps["callTool"], catalog: readonly ModelCatalogEntry[] = []): Promise<PiAliasTableReport> {
  const report: PiAliasTableReport = { unset: true, rejected: [] };
  for (const alias of PI_TIERS) {
    const { model, rejected } = await resolveModelForAliasViaWsMcp({ callTool }, alias, undefined, catalog);
    if (model !== undefined || rejected) report.unset = false;
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
 * Append one advisory text item on a copy, only for workflow_manual.
 * Rejections take precedence over the old empty-table copy; accepted-only
 * tables return the original content reference. No human command pointer.
 */
export function maybeAppendModelCatalogAdvisory(rawName: string, content: McpContentItem[], report: PiAliasTableReport, inheritModel?: string, catalogEmpty = true): McpContentItem[] {
  if (rawName !== "workflow_manual" || (!report.unset && report.rejected.length === 0)) return content;
  const text = report.rejected.length
    ? "> [!note]\n" + report.rejected.map(({ alias, rejected }) => `> ${formatTierWarning(alias, rejected, inheritModel, catalogEmpty)}`).join("\n")
    : MODEL_CATALOG_ADVISORY;
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
 * Removes the first exact-substring occurrence of `staticBodySnapshot` from
 * `response`. `found: false` (the static manual body no longer appears
 * byte-identical inside a live `workflow_manual` response — e.g. renderer
 * drift between the session-start snapshot and a later call) is the
 * ticket's own trigger for the `workflow_state` fallback dispatch. Pure,
 * synchronous, no IO — the mapping's IO wrapper below calls this on an
 * already-fetched response body.
 */
export function cutStaticBody(response: string, staticBodySnapshot: string): { text: string; found: boolean } {
  const index = response.indexOf(staticBodySnapshot);
  if (index === -1) {
    return { text: response, found: false };
  }
  return { text: response.slice(0, index) + response.slice(index + staticBodySnapshot.length), found: true };
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
  /** The `playbook.print("lead-workflow-manual")` snapshot fetched once at session_start. */
  staticBodySnapshot: string;
  catalog?: readonly ModelCatalogEntry[];
  inheritModel?: string;
  /** Invoked on a cut-miss fallback (renderer drift) — the caller is responsible for the "notify once per session" dedupe (a closure flag in `startBridge`), not this function. */
  notifyMappingDegraded: () => void;
}

/**
 * IO wrapper for the `workflow_manual` -> `workflow_state` mapping (§3).
 * Dispatches `workflow_manual` with `args` (already normalized/resolved by
 * the caller) and cuts `deps.staticBodySnapshot` out of the response:
 *
 * - Cut found: returns `prependWorkflowStateLine(cut text)`, re-wrapped
 *   through `maybeAppendModelCatalogAdvisory` keyed on the literal
 *   `"workflow_manual"` name (§3: the advisory still rides the mapped
 *   response, keyed on the tool's *registered* — i.e. ws-mcp's own raw
 *   dotted — name, not on which tool was actually dispatched to).
 * - Cut miss (renderer drift): calls `deps.notifyMappingDegraded()`, then
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
    return { content: maybeAppendModelCatalogAdvisory("workflow_manual", content, piAliasTableReport, deps.inheritModel, !deps.catalog?.length), details: manualResult };
  }

  deps.notifyMappingDegraded();
  const stateArgs: Record<string, unknown> = args.session_key === undefined ? {} : { session_key: args.session_key };
  const stateResult = await deps.callTool("workflow_state", stateArgs);
  if (stateResult.isError) {
    throw new Error(firstText(stateResult) ?? "workflow_state failed with no error text");
  }
  const stateText = firstText(stateResult) ?? "";
  const content = replaceFirstTextItem(stateResult.content, prependWorkflowStateLine(stateText));
  return { content: maybeAppendModelCatalogAdvisory("workflow_manual", content, piAliasTableReport, deps.inheritModel, !deps.catalog?.length), details: stateResult };
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

const MERCENARY_RAW_PREFIX = "mercenary.";

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
}

/**
 * Rewrites exactly two explicit `session_key` values to the bridge's own key,
 * ahead of `resolveSessionKey`'s fill-or-forward: the fresh-bootstrap sentinel
 * (`opts.sentinel`), and — fork-only — the env-delivered parent lead key
 * (`opts.parentLeadKey`). Every other explicit key (including an explicit
 * child key) passes through completely untouched — widening this to any other
 * rewrite case is explicitly rejected by the ticket contract.
 *
 * When `opts.ownKey` is unset (degraded bootstrap: the bridge's own ferrule
 * mint hasn't resolved, or failed), both rewrites are disabled and `params` is
 * returned unchanged — the sentinel self-heals exactly as today (ws-mcp's own
 * fresh-bootstrap path still recognizes it directly).
 *
 * Never mutates `params` — copy-on-write, same contract as
 * `resolveSessionKey`.
 */
export function normalizeSessionKey(
  params: Record<string, unknown> | undefined,
  opts: NormalizeSessionKeyOptions,
): Record<string, unknown> | undefined {
  if (!opts.ownKey) {
    return params;
  }
  const provided = params?.session_key;
  if (provided === opts.sentinel || (opts.parentLeadKey !== undefined && provided === opts.parentLeadKey)) {
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
  const client = spawnWsMcpClient(opts.launcherPath, opts.pluginDir, (line) => {
    console.error(`[ws-mcp] ${line.trimEnd()}`);
  });

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
      pi.registerTool({
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
        // Structural component intentionally avoids a direct pi-tui import:
        // Pi owns image blocks from the unmodified content array, while this
        // component supplies only width-safe text rows for the tool shell.
        renderResult(result, options, theme, context) {
          return {
            render: (width: number) => renderResultRows(
              result.content as McpContentItem[],
              context.isError,
              options.expanded,
              width,
            ).map((row) => theme.fg("toolOutput", row)),
            invalidate: () => {},
          };
        },
        async execute(_toolCallId, params, _signal, _onUpdate, toolCtx) {
          // Dispatch always uses the RAW dotted name — sanitization is
          // registration-only, never part of the ws-mcp wire call.
          // normalizeSessionKey runs in front of resolveSessionKey's own
          // fill-or-forward: it rewrites the two ticket-mandated sentinel/
          // parent-key explicit cases to the bridge's own key, then
          // resolveSessionKey handles the (separate) omitted-key fill.
          const normalized = normalizeSessionKey(params as Record<string, unknown> | undefined, {
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
              notifyMappingDegraded: () => {
                if (!notifiedMappingDegraded) {
                  notifiedMappingDegraded = true;
                  notify(
                    opts.ui,
                    "ws-pi-bridge: workflow_manual's static manual body no longer matches the session-start snapshot (renderer drift) — falling back to workflow_state; per-call advisories are unavailable for the rest of this session",
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
          const content = maybeAppendModelCatalogAdvisory(rawName, result.content, piAliasTableReport, inheritModel, catalog.length === 0);
          return { content, details: result };
        },
      });
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
      try {
        const manualResult = await client.callTool("workflow_manual", { session_key: defaultKeyRef.current });
        const staticBodyResult = await client.callTool("playbook.print", {
          name: "lead-workflow-manual",
          session_key: defaultKeyRef.current,
        });
        const manualText = !manualResult.isError ? firstText(manualResult) : undefined;
        const staticBodyText = !staticBodyResult.isError ? firstText(staticBodyResult) : undefined;
        if (manualText && staticBodyText) {
          manualSnapshotRef.current = manualText;
          staticBodySnapshotRef.current = staticBodyText;
        } else {
          notify(
            opts.ui,
            "ws-pi-bridge: session-start manual/static-body snapshot fetch returned no text — ws system-prompt block and workflow_manual mapping disabled for this session",
            "warning",
          );
        }
      } catch (err) {
        notify(
          opts.ui,
          `ws-pi-bridge: session-start manual/static-body snapshot fetch threw: ${(err as Error).message} — ws system-prompt block and workflow_manual mapping disabled for this session`,
          "warning",
        );
      }
    }

    notify(opts.ui, `ws-pi-bridge: registered ${tools.length} ws__* tools from ws-mcp ${initResult.serverInfo.version}`);
  } catch (err) {
    shutdown();
    throw err;
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
