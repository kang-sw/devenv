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
import { spawnWsMcpClient, type McpToolCallResult } from "./mcp-stdio-client.ts";
import { assertVersionPin, readRuntimeContract } from "./version-check.ts";

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
 * Provider-legal registered name for a ws-mcp raw tool name: `ws__` prefix
 * (namespace separator, stands in for the `/` in the `ws/<rawName>` prose
 * form) plus the raw name's `.` separators flattened to `_`. Registration
 * only — never used for the wire call to ws-mcp, which always dispatches on
 * the untouched `rawName`.
 */
function sanitizeToolName(rawName: string): string {
  return `ws__${rawName.replaceAll(".", "_")}`;
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
function withOptionalSessionKey(inputSchema: Record<string, unknown>): Record<string, unknown> {
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
 * session_key fill-or-forward: if the caller omitted session_key (undefined,
 * null, or empty string), splice in the bridge's default-filled key; an
 * explicit session_key passes through completely unchanged. This is what
 * keeps subagent lineage / lead multi-track orchestration viable later even
 * though building that machinery is out of scope for this phase.
 *
 * Never mutates the tool's registered `parameters` schema — only the
 * per-call arguments object.
 */
function resolveSessionKey(
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

  try {
    const initResult = await client.initialize({
      name: "ws-pi-bridge",
      version: "0.1.0",
    });
    assertVersionPin(runtime, initResult.serverInfo.version);

    const tools = await client.listTools();
    const defaultKeyRef: { current: string | undefined } = { current: undefined };

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
          const args = resolveSessionKey(params as Record<string, unknown> | undefined, defaultKeyRef);
          const result = await client.callTool(rawName, args);
          if (result.isError) {
            // Throwing is how Pi's tool contract signals isError: true —
            // returning a value never sets it (docs/extensions.md#L1953-2011).
            throw new Error(firstText(result) ?? `${registeredName} failed with no error text`);
          }
          return { content: result.content, details: result };
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

    notify(opts.ui, `ws-pi-bridge: registered ${tools.length} ws__* tools from ws-mcp ${initResult.serverInfo.version}`);
  } catch (err) {
    shutdown();
    throw err;
  }

  return { shutdown };
}
