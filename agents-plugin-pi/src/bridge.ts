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

import type { ExtensionAPI, ExtensionUIContext } from "@earendil-works/pi-coding-agent";
import { spawnWsMcpClient, type McpStdioClient, type McpContentItem, type McpToolCallResult } from "./mcp-stdio-client.ts";
import { assertVersionPin, readRuntimeContract } from "./version-check.ts";
import { WS_PI_PARENT_SESSION_KEY_ENV, isLeadOrFork, readSpawnRole, type SpawnRole } from "./process-role.ts";

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
 * Advisory appended to every `workflow_manual` response while harness `pi`'s
 * `agents.tier` alias table has no genuine `pi` entries (per
 * `computePiAliasTableUnset` below). Mirrors the Go core's blockquote
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
 * The four fixed tiers `config.resolve_agent` understands — shared by
 * `computePiAliasTableUnset` here and `resolveModelForAliasViaWsMcp`
 * (spawner.ts), which applies the identical guard for a single tier lookup.
 */
const PI_TIERS = ["small", "medium", "large", "xlarge"] as const;

/**
 * `true` only when NONE of the four fixed tiers resolves to a GENUINE
 * `pi`-labeled `config.resolve_agent` hit — the trigger condition for
 * `MODEL_CATALOG_ADVISORY`. Applies the same `resolved_from === "pi" &&
 * model.includes("/")` guard spawner.ts's `resolveModelForAliasViaWsMcp`
 * uses for a single tier (Phase 3 Forward (a): a codex-seeded default can
 * answer under the `pi` label with no `/`, so `resolved_from === "pi"` alone
 * is not proof of a real Pi model string). NEVER HARD-FAILS: an `isError`
 * result, missing/unparsable text, or a thrown call are all treated as "not
 * a hit" for that tier and the loop continues — never surfaced as an error.
 * Four local stdio round-trips per `workflow_manual` call (only) is
 * acceptable — mirrors the advisory's existing per-call, no-caching
 * contract.
 */
export async function computePiAliasTableUnset(callTool: WorkflowManualMappingDeps["callTool"]): Promise<boolean> {
  for (const tier of PI_TIERS) {
    try {
      const result = await callTool("config.resolve_agent", { tier, format: "json" });
      if (result.isError) continue;
      const text = firstText(result);
      if (!text) continue;
      const parsed = JSON.parse(text) as { model?: string; resolved_from?: string };
      if (parsed.resolved_from === "pi" && parsed.model?.includes("/")) {
        return false;
      }
    } catch {
      // Never hard-fail this advisory computation — skip to the next tier.
    }
  }
  return true;
}

/**
 * Appends `MODEL_CATALOG_ADVISORY` as a new `{type:"text"}` item onto a
 * COPY of `content` (never mutated in place) when `rawName ===
 * "workflow_manual"` and `piAliasTableUnset` is true; otherwise returns
 * `content` unchanged (same reference). Extracted as a pure function — the
 * `config.resolve_agent` round-trips that compute `piAliasTableUnset` are the
 * caller's job (`computePiAliasTableUnset`) — so the
 * append-not-prepend/copy-not-mutate/gated contract stays directly
 * unit-testable, same as resolveSessionKey/withOptionalSessionKey above.
 */
export function maybeAppendModelCatalogAdvisory(rawName: string, content: McpContentItem[], piAliasTableUnset: boolean): McpContentItem[] {
  if (rawName !== "workflow_manual" || !piAliasTableUnset) {
    return content;
  }
  return [...content, { type: "text", text: MODEL_CATALOG_ADVISORY }];
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

  const piAliasTableUnset = await computePiAliasTableUnset(deps.callTool);

  if (cut.found) {
    const content = replaceFirstTextItem(manualResult.content, prependWorkflowStateLine(cut.text));
    return { content: maybeAppendModelCatalogAdvisory("workflow_manual", content, piAliasTableUnset), details: manualResult };
  }

  deps.notifyMappingDegraded();
  const stateArgs: Record<string, unknown> = args.session_key === undefined ? {} : { session_key: args.session_key };
  const stateResult = await deps.callTool("workflow_state", stateArgs);
  if (stateResult.isError) {
    throw new Error(firstText(stateResult) ?? "workflow_state failed with no error text");
  }
  const stateText = firstText(stateResult) ?? "";
  const content = replaceFirstTextItem(stateResult.content, prependWorkflowStateLine(stateText));
  return { content: maybeAppendModelCatalogAdvisory("workflow_manual", content, piAliasTableUnset), details: stateResult };
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
        async execute(_toolCallId, params) {
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

          // §3 workflow_manual -> workflow_state mapping: only for lead/fork
          // roles, and only once a static-body snapshot actually exists
          // (both are the "degraded bootstrap" escape hatch — worker/explore
          // roles and a failed/skipped snapshot fetch both forward
          // workflow_manual verbatim, exactly as today).
          if (shouldMapWorkflowManual(rawName, Boolean(staticBodySnapshotRef.current), readSpawnRole(process.env))) {
            return await dispatchMappedWorkflowManual(args, {
              callTool: (name, callArgs) => client.callTool(name, callArgs),
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
          // call pays for an unrelated MCP round-trip.
          const piAliasTableUnset =
            rawName === "workflow_manual" ? await computePiAliasTableUnset((name, callArgs) => client.callTool(name, callArgs)) : false;
          const content = maybeAppendModelCatalogAdvisory(rawName, result.content, piAliasTableUnset);
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
