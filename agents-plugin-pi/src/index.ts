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
 *   - Exposes ws skills through resources_discover via a package-local-first
 *     resolver (src/skills-dir.ts): prefers a pack-time-copied
 *     `agents-plugin-pi/skills/` (baked into the published/installed tarball,
 *     gitignored, never committed — see scripts/copy-skills.mjs) and falls
 *     back to the monorepo canonical `agents-plugin/skills/` for dev `-e`
 *     runs from the source tree.
 *
 * Phase 2 adds the self-built delegation spawner (`ws-agent-spawn` /
 * `ws-agent-continue` / `ws-agent-wait` / `explore`, see src/spawner.ts) on
 * top of the Phase 1 bridge. Phase 3 adds the adapter-owned model-catalog
 * curation data file (`model-catalog.json`, see src/model-catalog.ts):
 * tier-aware `--model` resolution in the spawner, an unset-tier advisory
 * appended to every `workflow_manual` bridge response (bridge.ts), and a
 * read-only `ws-model-catalog-list` command exercising Pi's
 * `ctx.scopedModels` read API to help the user hand-curate the catalog.
 *
 * Phase 4 ships the `/ws-discuss` proof-of-concept command (kickoff built by
 * src/discuss.ts): a single `pi.sendUserMessage` that loads the lead-discuss
 * skill (skills-load), whose body drives the bridged `ws__*` tools (bridge),
 * and instructs the model to dispatch one `explore` recon leaf (spawner) —
 * proving skills-load + bridge + spawner compose end-to-end on Pi.
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
 *
 * `skills/` is a separate, fourth carried copy with a different sync model:
 * it is generated at pack time by scripts/copy-skills.mjs (prepack/prepare),
 * gitignored, and never hand-synced — see src/skills-dir.ts's
 * package-local-first resolver above.
 */

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { startBridge, type BridgeHandle } from "./bridge.ts";
import { registerAgentTools, type AgentToolsHandle } from "./spawner.ts";
import { buildDiscussKickoff } from "./discuss.ts";
import { resolveSkillsDir } from "./skills-dir.ts";

const srcDir = dirname(fileURLToPath(import.meta.url));
const pluginDir = dirname(srcDir); // agents-plugin-pi/
const repoRoot = dirname(pluginDir);
const skillsDir = resolveSkillsDir(pluginDir, repoRoot);
const launcherPath = join(pluginDir, "bin", "ws-mcp-launcher.py");
const runtimeJsonPath = join(pluginDir, "runtime.json");
const modelCatalogPath = join(pluginDir, "model-catalog.json");

export default function wsPiBridgeExtension(pi: ExtensionAPI) {
  let handle: BridgeHandle | undefined;
  let agentTools: AgentToolsHandle | undefined;

  pi.on("resources_discover", () => ({
    skillPaths: [skillsDir],
  }));

  // Read-only: lists Pi's currently scoped (or, if unscoped, all available)
  // models as `provider/id` candidates for the user to hand-copy into
  // model-catalog.json's `tiers`/`catalog` fields. No writes — curation
  // stays a hand-edited data file (see model-catalog.ts's doc comment).
  pi.registerCommand("ws-model-catalog-list", {
    description: "List provider/id model candidates for curating model-catalog.json's tiers.",
    handler: async (_args, ctx) => {
      const models = ctx.scopedModels.length > 0 ? ctx.scopedModels.map((sm) => sm.model) : ctx.modelRegistry.getAvailable();
      const lines = models.map((m) => `${m.provider}/${m.id}`);
      const header = ctx.scopedModels.length > 0 ? `Scoped models (${lines.length}):` : `All available models (${lines.length}):`;
      ctx.ui.notify([header, ...lines].join("\n"));
    },
  });

  // Phase 4 proof-of-concept command: one message that loads the lead-discuss
  // skill (skills-load), whose body calls the bridged ws__* tools (bridge), and
  // instructs the model to dispatch one `explore` recon leaf (spawner) — proving
  // all three MVP surfaces compose. expandPromptTemplates:true expands the
  // leading `/skill:lead-discuss <topic>` (docs/extensions.md#L1439-1467); the
  // idle guard mirrors examples/extensions/send-user-message.ts so the plain
  // (no deliverAs) send is always safe.
  pi.registerCommand("ws-discuss", {
    description: "PoC gate: load the ws discuss skill and dispatch one explore leaf, proving skills-load + bridge + spawner compose.",
    handler: async (args, ctx) => {
      if (!ctx.isIdle()) {
        ctx.ui.notify("Agent is busy — try again when idle.", "warning");
        return;
      }
      pi.sendUserMessage(buildDiscussKickoff(args), { expandPromptTemplates: true });
    },
  });

  pi.on("session_start", async (_event, ctx) => {
    handle = await startBridge(pi, {
      launcherPath,
      pluginDir,
      runtimeJsonPath,
      modelCatalogPath,
      cwd: ctx.cwd,
      ui: ctx.ui,
    });
    agentTools = registerAgentTools(pi, handle, { cwd: ctx.cwd, modelCatalogPath });
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
