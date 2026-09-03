/**
 * ws-pi-bridge: Pi extension entry point.
 *
 * Bridges ws-mcp (the harness-neutral MCP server backing the ws workflow —
 * Go source untouched, see AGENTS.md's golden rule) onto Pi:
 *   - Spawns the ws-mcp launcher as a subprocess on session_start, closes it
 *     on session_shutdown (docs/extensions.md#L220-224, #L516-526 — no
 *     background processes from the top-level factory).
 *   - Re-registers every ws-mcp tool via pi.registerTool (bridge.ts) under a
 *     provider-legal sanitized name derived from the `ws/<tool>` prose form
 *     (`/` -> `__`, `.` -> `_`, e.g. `ws__playbook_print`) — SKILL.md prose
 *     stays untouched as literal `ws/playbook.print(...)` calls; the model
 *     maps that prose to the sanitized registered name itself.
 *   - Exposes agents-plugin/skills/ through resources_discover, pointing at
 *     the existing directory directly rather than copying it (sibling root,
 *     same repo — unlike bin/ws-mcp-launcher.py + runtime.json, which have
 *     repo precedent for copying instead, see agents-plugin-wsflow).
 *
 * Phase 2 adds the self-built delegation spawner (`ws-agent-spawn` /
 * `ws-agent-continue` / `ws-agent-wait` / `explore`, see src/spawner.ts) on
 * top of the Phase 1 bridge. Model catalog (Phase 3) and /ws-discuss
 * (Phase 4) remain out of scope here.
 *
 * HAND-SYNC NOTE: bin/ws-mcp-launcher.py, runtime.json, and rsrc/ in this
 * package are byte-identical copies of the same-named files under
 * agents-plugin/ (same precedent as agents-plugin-wsflow's copies — no
 * cross-root relative reference, no shared sync tooling exists yet). This
 * note lives here rather than as a header comment inside
 * bin/ws-mcp-launcher.py so the copy stays byte-identical with
 * agents-plugin/bin/ws-mcp-launcher.py (verifiable via `diff`) instead of
 * silently drifting from it. When agents-plugin/bin/ws-mcp-launcher.py,
 * agents-plugin/runtime.json, or agents-plugin/rsrc/ change, re-copy the
 * changed file(s) here verbatim — all three copies must stay in lockstep.
 * A stale/missing rsrc/ copy surfaces at call time, e.g. workflow_manual:
 * "render playbook: rsrc manifest missing".
 */

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { startBridge, type BridgeHandle } from "./bridge.ts";
import { registerAgentTools, type AgentToolsHandle } from "./spawner.ts";

const srcDir = dirname(fileURLToPath(import.meta.url));
const pluginDir = dirname(srcDir); // agents-plugin-pi/
const repoRoot = dirname(pluginDir);
const skillsDir = join(repoRoot, "agents-plugin", "skills");
const launcherPath = join(pluginDir, "bin", "ws-mcp-launcher.py");
const runtimeJsonPath = join(pluginDir, "runtime.json");

export default function wsPiBridgeExtension(pi: ExtensionAPI) {
  let handle: BridgeHandle | undefined;
  let agentTools: AgentToolsHandle | undefined;

  pi.on("resources_discover", () => ({
    skillPaths: [skillsDir],
  }));

  pi.on("session_start", async (_event, ctx) => {
    handle = await startBridge(pi, {
      launcherPath,
      pluginDir,
      runtimeJsonPath,
      cwd: ctx.cwd,
      ui: ctx.ui,
    });
    agentTools = registerAgentTools(pi, handle, { cwd: ctx.cwd });
  });

  pi.on("session_shutdown", () => {
    // Kill any still-running spawned `pi` children before tearing down the
    // bridge connection they were dispatching ws__* tool calls through.
    agentTools?.killRunning();
    agentTools = undefined;
    handle?.shutdown();
    handle = undefined;
  });
}
