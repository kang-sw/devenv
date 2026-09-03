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
 * Phase 1 scope only: the bridge itself. Spawner/explore (Phase 2), model
 * catalog (Phase 3), and /ws-discuss (Phase 4) are out of scope here.
 */

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { startBridge, type BridgeHandle } from "./bridge.ts";

const srcDir = dirname(fileURLToPath(import.meta.url));
const pluginDir = dirname(srcDir); // agents-plugin-pi/
const repoRoot = dirname(pluginDir);
const skillsDir = join(repoRoot, "agents-plugin", "skills");
const launcherPath = join(pluginDir, "bin", "ws-mcp-launcher.py");
const runtimeJsonPath = join(pluginDir, "runtime.json");

export default function wsPiBridgeExtension(pi: ExtensionAPI) {
  let handle: BridgeHandle | undefined;

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
  });

  pi.on("session_shutdown", () => {
    handle?.shutdown();
    handle = undefined;
  });
}
