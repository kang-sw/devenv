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
 * `ws-agent-continue` / `explore`, see src/spawner.ts) on
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
 * The 260903 ticket's Phase 1 adds the goal-mode arming + `agent_settled`
 * re-injection loop (src/goal-loop.ts, `registerGoalLoop`): a `/goal <goal>`
 * command arms the loop, an armed `agent_settled` re-fire re-injects a
 * reminder naming the goal and its two terminal levers
 * (`goal-achieved`/`goal-blocked`, both model-invoked `pi.registerTool()`
 * calls per the ticket's "explicit skill calls, zero prose parsing" design
 * constraint), and a config-tunable runaway backstop force-stops the loop
 * after N consecutive no-tool-call re-fires. Registered at factory top
 * level alongside the other commands/tools below — no subprocess involved,
 * so it needs no `session_start` gating either.
 *
 * The 260904 ticket's Phase 1 adds the system-prompt bootstrap
 * (src/lead-bootstrap.ts, `registerLeadBootstrap`): a `before_agent_start`
 * handler appends a fixed ws block (the session-start `workflow_manual`
 * snapshot plus `pi-lead-guide.md`) to the system prompt on every turn, for
 * the host lead and a future `fork` child only (never `worker`/`explore`).
 * `registerLeadBootstrap` itself is declarative (factory top level, no
 * subprocess); the actual snapshot fetch happens inside `startBridge`
 * (bridge.ts), and this file fills `wsBlockRef.current` from that result
 * once `session_start`'s `startBridge` call resolves — same seam
 * `registerAgentTools` already uses.
 *
 * The 260904 "execute-approve-gateway" ticket's Phase 1 adds the end-to-end
 * `ws-execute`/`ws-approve` approval gateway (src/execute-gateway.ts,
 * `registerExecuteGateway`): a fixed-prompt `execute-worker` (spawner.ts's
 * new `"execute-worker"` `toolGroup`) whose every shell command elevates
 * through a lead-approval gate. `session_start` builds the approval-request
 * injection callback (`createApprovalRelay`) BEFORE calling
 * `registerAgentTools` (so it can be threaded into that call too — a
 * dormant-resumed execute-worker keeps its relay wired even if later driven
 * through the generic `ws-agent-*` tools), then calls
 * `registerExecuteGateway`, then — lead/fork sessions only —
 * `pi.setActiveTools(computeLeadActiveTools(...))` to remove native
 * `bash`/`read` (and exclude the gated-exec tool itself, the auto-include
 * footgun fix — see execute-gateway.ts's doc comment) while adding
 * `ws-execute`/`ws-approve`/the ugly-named read tool.
 *
 * The 260904 "side-thread fork question surface" ticket's Phase 1 adds
 * `ws-fork` (src/fork.ts, `registerFork`): a `pi --fork <own session>`
 * lateral peer sharing the caller's full context, plus the anti-bleed
 * mechanical loop. `session_start` calls `registerFork` right after
 * `registerExecuteGateway` (same shared `agentTools.rpcRegistry`), then,
 * inside the same lead/fork-only `isLeadOrFork` block as
 * `computeLeadActiveTools` above, applies `addForkToolIfLead` as a
 * SEPARATE, role-differentiated `setActiveTools` step — `role === undefined`
 * (the true top lead) only, never a fork — so a fork's own active-tools
 * surface never regains `ws-fork` (no recursive forking; see fork.ts's own
 * doc comment for the full risk-signal trace).
 *
 * That ticket's Phase 2 adds the owner-question surface (src/ask.ts +
 * src/overlay-chat.ts): `ws-ask`/`ws-resolve`, a persisted per-lead-session
 * thread registry, the `N pending` widget, `/thread`, `/answer <id>` (which
 * lazily forks a discussion thread at the lead's tip AT OPEN TIME and
 * attaches an overlay chat to it), and the `/done` summary injected back
 * into the lead as a Pi custom message. `session_start` re-captures `ctx`
 * into the registry handle on every firing (§5's captured-ctx staleness
 * rule), hydrates the registry from its sibling state file, threads
 * `handleForkRaisedQuestion` into `registerFork` as its new `onQuestion`
 * callback, and applies `addAskToolsIfLead` as a third role-differentiated
 * active-tools step.
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

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { startBridge, type BridgeHandle } from "./bridge.ts";
import { pushToLead, registerAgentTools, type AgentToolsHandle, type RpcAgentRegistry } from "./spawner.ts";
import { buildOrphanSummary, captureOrphans, readAndClearSidecar, rehydrateOrphanRecord, writeSidecar } from "./agent-sidecar.ts";
import { buildDiscussKickoff } from "./discuss.ts";
import { registerGoalLoop } from "./goal-loop.ts";
import { resolveSkillsDir } from "./skills-dir.ts";
import { buildWsBlock, registerLeadBootstrap } from "./lead-bootstrap.ts";
import { isLeadOrFork, readSpawnRole } from "./process-role.ts";
import { computeLeadActiveTools, createApprovalRelay, registerExecuteGateway } from "./execute-gateway.ts";
import { addForkToolIfLead, registerFork } from "./fork.ts";
import {
  addAskToolsIfLead,
  buildForkQuestionLeadNotice,
  createThreadRegistryHandle,
  handleForkRaisedQuestion,
  hydrateThreadRegistry,
  refreshPendingWidget,
  registerAsk,
  registerThreadCommands,
  threadRegistryPath,
} from "./ask.ts";

const srcDir = dirname(fileURLToPath(import.meta.url));
const pluginDir = dirname(srcDir); // agents-plugin-pi/
const repoRoot = dirname(pluginDir);
const skillsDir = resolveSkillsDir(pluginDir, repoRoot);
const launcherPath = join(pluginDir, "bin", "ws-mcp-launcher.py");
const runtimeJsonPath = join(pluginDir, "runtime.json");
const modelCatalogPath = join(pluginDir, "model-catalog.json");
const goalLoopConfigPath = join(pluginDir, "goal-loop-config.json");
const piLeadGuidePath = join(pluginDir, "pi-lead-guide.md");
const executeWorkerGuidePath = join(pluginDir, "execute-worker-guide.md");

export default function wsPiBridgeExtension(pi: ExtensionAPI) {
  let handle: BridgeHandle | undefined;
  let agentTools: AgentToolsHandle | undefined;
  const wsBlockRef: { current: string | undefined } = { current: undefined };
  // 260905 (push model): the shared RPC registry, published as a mutable ref
  // so `createApprovalRelay` — which must be constructed BEFORE
  // `registerAgentTools` creates that registry — can still read it at push
  // time for the fan-in status line every push carries.
  const rpcRegistryRef: { current: RpcAgentRegistry | undefined } = { current: undefined };
  // The lead's own session file, captured on session_start so the shutdown
  // handler (which gets a ctx of its own, but only after teardown has begun)
  // knows where to write the orphan sidecar.
  let leadSessionFile: string | undefined;
  // 260904 Phase 2 (side-thread question surface): one thread registry per
  // extension instance. Its in-memory map is hydrated from (and written back
  // to) a sibling file of the lead's own session file on every session_start
  // — see ask.ts's header for why that file exists at all.
  const threadHandle = createThreadRegistryHandle();

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

  registerGoalLoop(pi, { goalLoopConfigPath });
  registerLeadBootstrap(pi, wsBlockRef);

  pi.on("session_start", async (_event, ctx) => {
    handle = await startBridge(pi, {
      launcherPath,
      pluginDir,
      runtimeJsonPath,
      modelCatalogPath,
      cwd: ctx.cwd,
      ui: ctx.ui,
    });

    // Built BEFORE registerAgentTools (not after, unlike registerExecuteGateway
    // below) so it can be threaded into that call too — see
    // spawner.ts's registerAgentTools doc comment for why a
    // dormant-resumed execute-worker needs the SAME callback wired through
    // ws-agent-send's auto-resume branch, not just ws-execute's own spawn.
    const onApprovalPending = createApprovalRelay(pi, { cwd: ctx.cwd }, rpcRegistryRef);
    agentTools = registerAgentTools(pi, handle, { cwd: ctx.cwd, modelCatalogPath }, onApprovalPending);
    rpcRegistryRef.current = agentTools.rpcRegistry;
    registerExecuteGateway(pi, handle, agentTools.rpcRegistry, {
      cwd: ctx.cwd,
      modelCatalogPath,
      executeWorkerPromptPath: executeWorkerGuidePath,
      onApprovalPending,
    });
    // 260904 Phase 1 (side-thread fork): registered declaratively/globally,
    // same pattern as registerExecuteGateway above — a fork child re-runs
    // session_start too and needs ws-fork registered so computeForkToolSurface's
    // own exclusion of it has something to exclude. Whether it is ever ACTIVE
    // is addForkToolIfLead's job below, not this registration.
    // 260904 Phase 2: the onQuestion callback is what makes a task fork's own
    // ws-report-to-lead(kind:"question") land in the owner-question registry
    // with `respondent` already set to that live fork (Entry A meets Entry B)
    // — fork.ts stays generic and never imports ask.ts.
    //
    // Review relay #1 I6: its return value replaces what the LEAD sees on that
    // report. In TUI the owner surface is the only answering channel (§1), so
    // the lead gets a notice naming the thread and telling it to keep waiting;
    // in headless there is no owner surface, so `undefined` keeps the Phase 1
    // relay byte-identical (§8).
    registerFork(pi, handle, agentTools.rpcRegistry, { cwd: ctx.cwd, modelCatalogPath }, (agentId, message) => {
      const thread = handleForkRaisedQuestion(threadHandle, agentTools!.rpcRegistry, agentId, message);
      return threadHandle.ctxRef.current?.mode === "tui" ? buildForkQuestionLeadNotice(agentId, thread.threadId) : undefined;
    });

    // 260904 Phase 2 (owner question surface), same declarative/global
    // registration placement as registerFork above: ws-ask/ws-resolve must
    // exist in a fork child's own process too, so computeForkToolSurface has
    // them present to exclude. Whether they are ever ACTIVE is
    // addAskToolsIfLead's job below.
    //
    // §5's captured-ctx staleness rule: re-capture ctx on EVERY session_start
    // (never a factory-scope ctx), and hydrate the persisted registry so
    // pending questions and dormant threads survive a lead restart. Only a
    // lead/fork session owns a thread registry — a worker/explore child has
    // none.
    threadHandle.ctxRef.current = ctx;
    if (isLeadOrFork(readSpawnRole(process.env))) {
      const sessionFile = ctx.sessionManager.getSessionFile();
      leadSessionFile = sessionFile ?? undefined;
      if (sessionFile) {
        hydrateThreadRegistry(threadHandle, threadRegistryPath(sessionFile));
        // 260905 orphan revival: a previous run of THIS lead session died (or
        // was shut down) with children still live. Read-and-delete the
        // sidecar, put each orphan back on the registry as a dormant record
        // (ws-agent-send relaunches it from the same --session file), and tell
        // the lead once. Runs before `registerAsk`/`registerThreadCommands`
        // only incidentally — nothing below depends on it.
        const orphans = readAndClearSidecar(sessionFile);
        if (orphans.length > 0) {
          for (const orphan of orphans) {
            if (!agentTools.rpcRegistry.has(orphan.agentId)) {
              agentTools.rpcRegistry.set(orphan.agentId, rehydrateOrphanRecord(orphan));
            }
          }
          pushToLead(
            pi,
            agentTools.rpcRegistry,
            undefined,
            "ws-agent-orphaned",
            {
              count: orphans.length,
              agents: buildOrphanSummary(orphans),
              detail:
                "A previous run of this session left these delegated agents behind. They are registered as dormant: ws-agent-send revives one from its own session file, ws-agent-transcript reads what it did, ws-agent-stop drops it.",
            },
            "followUp",
          );
        }
      }
      refreshPendingWidget(ctx, threadHandle);
    }
    registerAsk(pi, threadHandle, agentTools.rpcRegistry);
    registerThreadCommands(pi, handle, agentTools.rpcRegistry, threadHandle, { cwd: ctx.cwd, modelCatalogPath });

    // §1/§4: fill the ws system-prompt block only when startBridge actually
    // produced both snapshots (lead/fork role, non-degraded bootstrap — see
    // bridge.ts's all-or-nothing fetch). Otherwise leave wsBlockRef.current
    // unset — computeBeforeAgentStartResult's own guard already treats that
    // as "no override" for every before_agent_start firing, matching §3's
    // degraded-bootstrap behavior (no ws block, no crash).
    if (isLeadOrFork(readSpawnRole(process.env)) && handle.manualSnapshotRef.current) {
      let guideText = "";
      try {
        guideText = readFileSync(piLeadGuidePath, "utf8");
      } catch {
        // Tolerate a missing guide file (e.g. a dev -e run against a source
        // tree that hasn't copied it yet) — the manual snapshot alone is
        // still a strict improvement over no ws block at all.
      }
      wsBlockRef.current = buildWsBlock(handle.manualSnapshotRef.current, guideText);
    }

    // §8: reshape the LEAD's (or a fork's) own tool surface — bash/read
    // removed, ws-execute/ws-approve/the ugly-read tool added, and the
    // gated-exec tool itself excluded even though it was just registered
    // globally (the auto-include footgun fix — see execute-gateway.ts's
    // computeLeadActiveTools doc comment). Never applied to a worker/explore
    // child: those spawn with an explicit `--tools` allowlist already, and
    // this call would otherwise clobber it.
    if (isLeadOrFork(readSpawnRole(process.env))) {
      pi.setActiveTools(computeLeadActiveTools(pi.getActiveTools()));
      // 260904 Phase 1 (side-thread fork): a SEPARATE, role-differentiated
      // step from computeLeadActiveTools above — deliberately not folded
      // into that function's shared LEAD_ADDED_TOOL_NAMES list, which is
      // applied identically to lead and fork roles. addForkToolIfLead only
      // adds ws-fork for the true top lead (role === undefined); a fork
      // never regains it, keeping a fork's own surface excluding ws-fork
      // (no recursive forking) even though isLeadOrFork treats lead/fork
      // identically for the gates above.
      pi.setActiveTools(addForkToolIfLead(pi.getActiveTools(), readSpawnRole(process.env)));
      // 260904 Phase 2: a third role-differentiated step, alongside (never
      // folded into) the two above for the same reason addForkToolIfLead is
      // kept separate — computeLeadActiveTools is applied to lead and fork
      // alike, so folding ws-ask/ws-resolve in there would hand a fork the
      // very tools FORK_EXCLUDED_TOOL_NAMES exists to keep away from it.
      pi.setActiveTools(addAskToolsIfLead(pi.getActiveTools(), readSpawnRole(process.env)));
    }
  });

  pi.on("session_shutdown", async (_event, _ctx) => {
    // 260905: snapshot the still-live children BEFORE stopAll() clears their
    // clients, so the next start of this session can announce them rather
    // than losing them silently (see agent-sidecar.ts's header). Ordering is
    // load-bearing: after stopAll() every record is dormant and
    // `captureOrphans` would return nothing.
    if (leadSessionFile && agentTools) {
      writeSidecar(leadSessionFile, captureOrphans(agentTools.rpcRegistry));
    }
    // Await graceful RPC teardown of any still-live spawned `pi` children
    // before tearing down the bridge connection they were dispatching ws__*
    // tool calls through (agentTools.stopAll() is itself async now that
    // teardown is a graceful RpcClient.stop() rather than a fire-and-forget
    // SIGTERM — see spawner.ts's AgentToolsHandle doc comment).
    await agentTools?.stopAll();
    agentTools = undefined;
    rpcRegistryRef.current = undefined;
    leadSessionFile = undefined;
    handle?.shutdown();
    handle = undefined;
  });
}
