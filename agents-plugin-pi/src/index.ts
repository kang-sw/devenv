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
 * top of the Phase 1 bridge. Phase 3 added (and the
 * `260905-feat-ws-pi-harness-config-layer` ticket's Phase 4 retired) an
 * adapter-owned tier-curation data file: tier-aware `--model` resolution now
 * goes through ws-mcp's `config.resolve_agent` tool
 * (`resolveModelForAliasViaWsMcp`, spawner.ts) instead of a hand-edited JSON
 * file, the unset-tier advisory in bridge.ts is sourced from the same tool,
 * and the read-only `ws-model-catalog-list` command below still exercises
 * Pi's `ctx.scopedModels` read API but now points the user at `config.tune
 * agents.tier harness:pi` / `lead-tune` for curation instead of a data file.
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
 * (bridge.ts), and this file fills `wsBlockBaseRef.current` from that result
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
 * thread registry, `/thread`, `/answer <id>` (which
 * lazily forks a discussion thread at the lead's tip AT OPEN TIME and
 * attaches an overlay chat to it), and the `/done` summary injected back
 * into the lead as a Pi custom message. `session_start` re-captures `ctx`
 * into the registry handle on every firing (§5's captured-ctx staleness
 * rule), hydrates the registry from its sibling state file, threads
 * `handleForkRaisedQuestion` into `registerFork` as its new `onQuestion`
 * callback, and applies `addAskToolsIfLead` as a third role-differentiated
 * active-tools step.
 *
 * 260905 Phase 1 Edition (push delivery): three factory/session_start hooks
 * serve the pushed child-report channel. `registerPushFlush` (factory scope)
 * releases the pushes the spawner held while this session was mid-turn, on
 * this session's own `agent_settled`; `session_start` fills
 * `spawner.ts`'s `leadIdleRef` with this session's `ctx.isIdle` (the seam that
 * decides hold vs send) and, in TUI only, registers the compact
 * `push-render.ts` renderers for the six families; `session_shutdown` drops
 * the held queue with the session it belonged to.
 *
 * The `260905-feat-ws-pi-live-agent-widget` ticket's Phase 1 adds the
 * live-agent widget (src/agent-widget.ts, `createAgentWidgetController`): one
 * compact `belowEditor` panel listing every live agent and owner discussion
 * thread, plus a `setStatus` footer segment (`ws: N agents · M questions`).
 * It also folds the 260904 owner-question surface's standalone `N pending`
 * `aboveEditor` widget into this one panel — `ask.ts` no longer owns any
 * widget of its own. `session_start`, in the same TUI-lead-only block that
 * hydrates the thread registry, (re)creates the controller over
 * `agentTools.rpcRegistry`/`threadHandle.threads` and points
 * `spawner.ts`'s `agentWidgetRefreshRef` at its `refresh()`; every
 * registry-transition point in `spawner.ts` and every widget-refresh call
 * site left in `ask.ts` fire through that same ref, so neither module
 * imports `agent-widget.ts` directly. `session_shutdown` stops its 10-second
 * elapsed timer and clears both the widget and the status segment.
 *
 * The `260906-bug-ws-pi-lead-cannot-see-or-load-skills` ticket's Phase 1 adds
 * the lead/fork skill surface (src/lead-skills.ts): removing native
 * `read`/`bash` from the reshaped lead/fork tool surface (above) also
 * silently dropped Pi's own `<available_skills>` system-prompt block and its
 * `read`-the-SKILL.md loading path, leaving the lead with no way to see or
 * load a skill it was not told about via `/skill:<name>`. The fix is
 * adapter-owned, mirroring the workflow-manual/tool-reshape split already in
 * this file: a dedicated `<available_skills>` block (pointing at `ws-skill`,
 * never `read`) is appended as the third ordered item of the ws
 * system-prompt block, and `ws-skill(name, args?)` is registered globally
 * and added to the active-tools surface for lead AND fork alike
 * (`isLeadOrFork`, not the narrower lead-only gate
 * `addForkToolIfLead`/`addAskToolsIfLead` use). This ticket also collapses
 * the previously separate `buildWsBlock` call and three sequential
 * `pi.setActiveTools()`/`pi.getActiveTools()` reshape steps below into one
 * `computeSessionBootstrap` call (`lead-bootstrap.ts`) — a single pure
 * function producing the whole lead/fork session-start outcome, testable
 * end-to-end without a fake `ExtensionAPI`.
 *
 * A later dogfood fix corrected this ticket's original `session_start`-time
 * `pi.getCommands()` snapshot: Pi actually runs `session_start` FIRST and
 * only afterwards merges an extension's own `resources_discover` skills into
 * the live command list (`extendResourcesFromExtensions`, confirmed against
 * the installed `agent-session.js`), so that snapshot predated every ws
 * skill and both `ws-skill` and the `<available_skills>` block saw only
 * whatever other extension (e.g. `imagegen`) had registered first. Both now
 * resolve `pi.getCommands()` LIVE instead — `ws-skill` inside its own
 * `execute()` (`lead-skills.ts`), the block inside `before_agent_start`
 * (`lead-bootstrap.ts`'s `computeSkillsBlockCached`, cached in
 * `skillsBlockCacheRef` below) — so no `session_start`-scoped skill snapshot
 * exists in this file at all any more.
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
import {
  agentWidgetRefreshRef,
  heldPushQueue,
  leadIdleRef,
  pushToLead,
  registerAgentTools,
  registerPushFlush,
  type AgentToolsHandle,
  type RpcAgentRegistry,
} from "./spawner.ts";
import { createAgentWidgetController, shouldArmAgentWidget, type AgentWidgetController } from "./agent-widget.ts";
import { registerPushMessageRenderers } from "./push-render.ts";
import { buildOrphanPush, captureOrphans, readAndClearSidecar, reviveOrphans, writeSidecar } from "./agent-sidecar.ts";
import { buildDiscussKickoff } from "./discuss.ts";
import { registerGoalLoop, readGoalLoopConfig, resolveSettleDelayMs } from "./goal-loop.ts";
import { resolveSkillsDir } from "./skills-dir.ts";
import { computeSessionBootstrap, registerLeadBootstrap, type SkillsBlockCache, type WsBlockBase } from "./lead-bootstrap.ts";
import { isLeadOrFork, readSpawnRole } from "./process-role.ts";
import { createApprovalRelay, registerExecuteGateway } from "./execute-gateway.ts";
import { armForkRoleWiring, registerFork } from "./fork.ts";
import {
  buildForkQuestionLeadNotice,
  createThreadRegistryHandle,
  handleForkRaisedQuestion,
  hydrateThreadRegistry,
  registerAsk,
  registerThreadCommands,
  threadRegistryPath,
} from "./ask.ts";
import { registerWsSkillTool } from "./lead-skills.ts";

const srcDir = dirname(fileURLToPath(import.meta.url));
const pluginDir = dirname(srcDir); // agents-plugin-pi/
const repoRoot = dirname(pluginDir);
const skillsDir = resolveSkillsDir(pluginDir, repoRoot);
const launcherPath = join(pluginDir, "bin", "ws-mcp-launcher.py");
const runtimeJsonPath = join(pluginDir, "runtime.json");
const goalLoopConfigPath = join(pluginDir, "goal-loop-config.json");
const piLeadGuidePath = join(pluginDir, "pi-lead-guide.md");
const executeWorkerGuidePath = join(pluginDir, "execute-worker-guide.md");

export default function wsPiBridgeExtension(pi: ExtensionAPI) {
  let handle: BridgeHandle | undefined;
  let agentTools: AgentToolsHandle | undefined;
  // The manual-snapshot + guide-text half of the ws block, filled once per
  // `session_start`. The `<available_skills>` half is deliberately NOT held
  // here — see `skillsBlockCacheRef` below and `lead-bootstrap.ts`'s
  // `computeSkillsBlockCached` for why that piece is resolved live instead.
  const wsBlockBaseRef: { current: WsBlockBase | undefined } = { current: undefined };
  // Dogfood fix: the `<available_skills>` block cache, built lazily inside
  // `registerLeadBootstrap`'s `before_agent_start` handler against a LIVE
  // `pi.getCommands()` read (never a `session_start`-time snapshot — Pi
  // merges this adapter's own skills into `pi.getCommands()` AFTER
  // `session_start` returns, so a snapshot taken here would predate them,
  // same live-ref convention as `wsBlockBaseRef`/`rpcRegistryRef`). Lives at
  // this factory scope (survives across every `session_start` on this loaded
  // extension instance), but that is fine: `computeSkillsBlockCached` keys
  // its cache on the live entry-path set and rebuilds on its own whenever
  // that set changes, so no external reset is needed here.
  const skillsBlockCacheRef: { current: SkillsBlockCache | undefined } = { current: undefined };
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
  // 260905 (live-agent widget ticket): the IO controller behind
  // `spawner.ts`'s `agentWidgetRefreshRef` — created once per TUI-lead
  // `session_start`, torn down (timer stopped, widget/status cleared) on
  // `session_shutdown`. `undefined` in every non-TUI or non-lead/fork process,
  // which is also what keeps `agentWidgetRefreshRef.current` unset there.
  let agentWidgetHandle: AgentWidgetController | undefined;

  pi.on("resources_discover", () => ({
    skillPaths: [skillsDir],
  }));

  // Read-only: lists Pi's currently scoped (or, if unscoped, all available)
  // models as `provider/id` candidates for the user to hand-copy into a
  // `config.tune agents.tier harness:pi` write (see lead-tune). No writes —
  // curation stays a ws-mcp config edit, not an adapter-owned data file.
  pi.registerCommand("ws-model-catalog-list", {
    description: "List provider/id model candidates for curating harness pi's agents.tier entries via config.tune / lead-tune.",
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

  const goalLoopHandle = registerGoalLoop(pi, { goalLoopConfigPath, rpcRegistryRef });
  registerLeadBootstrap(pi, wsBlockBaseRef, skillsBlockCacheRef);
  // 260906 Phase 1: declarative/global, same placement as registerFork/
  // registerAsk above it — a fork child re-runs session_start too and needs
  // ws-skill registered so addSkillToolIfLeadOrFork has something to
  // activate. Whether it is ever ACTIVE is that gate's job, not this call's.
  // Dogfood fix: takes only `pi` now — it reads `pi.getCommands()` live
  // inside its own `execute()`, never a ref filled at `session_start`.
  registerWsSkillTool(pi);
  // 260905 Edition: releases the child pushes that arrived while this session
  // was mid-turn, each with a status line computed at release time. Factory
  // scope (like registerGoalLoop above, never inside session_start) so a
  // /reload cannot stack duplicate agent_settled handlers.
  registerPushFlush(pi, { delayMs: () => resolveSettleDelayMs(readGoalLoopConfig(goalLoopConfigPath)) });
  // Whether the compact push renderers have been registered in THIS process.
  // Registration is per-process and idempotent (Pi keys renderers by
  // customType), but it costs a dynamic import, so a second session_start
  // does not repeat it.
  let pushRenderersRegistered = false;

  pi.on("session_start", async (_event, ctx) => {
    // 260905 Edition: hand the spawner this session's idleness accessor (the
    // same captured-ctx-per-session_start seam wsBlockBaseRef uses), so a
    // followUp push raised while this session is mid-turn is held until its
    // turn settles instead of going out with an already-stale status line.
    leadIdleRef.current = () => ctx.isIdle();
    // TUI only: replace Pi's default custom-message rendering for the six
    // push families, whose own content already opens with the family label
    // the default would print again. No-op (default rendering stands) when
    // pi-tui cannot be loaded — see push-render.ts.
    if (!pushRenderersRegistered && ctx.mode === "tui" && isLeadOrFork(readSpawnRole(process.env))) {
      pushRenderersRegistered = true;
      void registerPushMessageRenderers(pi)
        .then((registered) => {
          pushRenderersRegistered = registered;
        })
        .catch(() => {
          // A rejection (e.g. Pi's assertActive() during teardown) must not
          // surface as an unhandled rejection nor pin the flag at `true`,
          // which would permanently skip the retry on the next session_start.
          pushRenderersRegistered = false;
        });
    }

    handle = await startBridge(pi, {
      launcherPath,
      pluginDir,
      runtimeJsonPath,
      cwd: ctx.cwd,
      ui: ctx.ui,
    });

    // Built BEFORE registerAgentTools (not after, unlike registerExecuteGateway
    // below) so it can be threaded into that call too — see
    // spawner.ts's registerAgentTools doc comment for why a
    // dormant-resumed execute-worker needs the SAME callback wired through
    // ws-agent-send's auto-resume branch, not just ws-execute's own spawn.
    const onApprovalPending = createApprovalRelay(pi, { cwd: ctx.cwd }, rpcRegistryRef);
    agentTools = registerAgentTools(pi, handle, { cwd: ctx.cwd }, onApprovalPending);
    rpcRegistryRef.current = agentTools.rpcRegistry;
    registerExecuteGateway(pi, handle, agentTools.rpcRegistry, {
      cwd: ctx.cwd,
      executeWorkerPromptPath: executeWorkerGuidePath,
      onApprovalPending,
    });
    // 260904 Phase 1 (side-thread fork): registered declaratively/globally,
    // same pattern as registerExecuteGateway above — a fork child re-runs
    // session_start too and needs ws-fork registered so computeForkToolSurface's
    // own exclusion of it has something to exclude. Whether it is ever ACTIVE
    // is addForkToolIfLead's job, applied below via computeSessionBootstrap,
    // not this registration.
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
    //
    // Hoisted to a named callback (review relay #1, I1) because the shutdown
    // sidecar's orphan revival below re-arms the SAME hook on a revived fork.
    const onForkQuestion = (agentId: string, message: string): string | undefined => {
      const thread = handleForkRaisedQuestion(threadHandle, agentTools!.rpcRegistry, agentId, message, pi);
      return threadHandle.ctxRef.current?.mode === "tui" ? buildForkQuestionLeadNotice(agentId, thread.threadId) : undefined;
    };
    registerFork(pi, handle, agentTools.rpcRegistry, { cwd: ctx.cwd }, onForkQuestion);

    // 260904 Phase 2 (owner question surface), same declarative/global
    // registration placement as registerFork above: ws-ask/ws-resolve must
    // exist in a fork child's own process too, so computeForkToolSurface has
    // them present to exclude. Whether they are ever ACTIVE is
    // addAskToolsIfLead's job, applied below via computeSessionBootstrap.
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
        // (ws-agent-send relaunches it from the same --session file), and —
        // when any of them was cut off mid-turn — tell the lead once. Runs
        // before `registerAsk`/`registerThreadCommands` only incidentally —
        // nothing below depends on it.
        const orphans = readAndClearSidecar(sessionFile);
        if (orphans.length > 0) {
          // Role-keyed wiring re-arm (review relay #1, I1): `spawnRole` is
          // persisted precisely so a revived FORK comes back with its question
          // routing (§1 keeps a fork-raised question on the owner surface) and
          // its anti-bleed loop, rather than silently degrading to plain-worker
          // behavior on the next ws-agent-send. A revived execute-worker gets
          // the approval relay pinned to the record itself, so it no longer
          // depends on which call site happens to resume it.
          reviveOrphans(agentTools.rpcRegistry, orphans, {
            fork: (record) => armForkRoleWiring(pi, agentTools!.rpcRegistry, record, onForkQuestion),
            executeWorker: (record) => {
              record.onApprovalPending = onApprovalPending;
            },
          });
          // Edition: EVERY entry is re-registered above (an idle reviewer
          // must stay reachable through ws-agent-send), but only a set
          // containing cut-off work is announced — see buildOrphanPush.
          const orphanPush = buildOrphanPush(orphans);
          if (orphanPush) {
            pushToLead(pi, agentTools.rpcRegistry, undefined, "ws-agent-orphaned", orphanPush, "followUp");
          }
        }
      }
      // 260905 (live-agent widget ticket): TUI-lead-only, via
      // `shouldArmAgentWidget` (review relay #1 Important #5: extracted into
      // agent-widget.ts's own pure predicate, directly unit tested, so this
      // gate is no longer only a doc comment) — mirrors the same
      // `ctx.mode === "tui"` check already used at #L249 for the push
      // renderers. The outer `isLeadOrFork` block above also runs headless
      // (hydration/orphan revival apply there too), but the widget itself
      // must not. A prior controller (a `/reload`) is stopped first so its
      // timer never outlives the registry/threads it closed over.
      if (shouldArmAgentWidget(readSpawnRole(process.env), ctx.mode)) {
        agentWidgetHandle?.stop();
        agentWidgetHandle = createAgentWidgetController(ctx, agentTools.rpcRegistry, threadHandle.threads);
        agentWidgetRefreshRef.current = () => agentWidgetHandle?.refresh();
        agentWidgetHandle.refresh();
      }
    }
    registerAsk(pi, threadHandle, agentTools.rpcRegistry);
    registerThreadCommands(pi, handle, agentTools.rpcRegistry, threadHandle, { cwd: ctx.cwd });

    // §1/§4/260906: one pure call produces BOTH the ws block's static base
    // (manual snapshot + Pi lead guide) AND the fully reshaped lead/fork
    // tool surface — see lead-bootstrap.ts's `computeSessionBootstrap` doc
    // comment for why this replaced three separate
    // `pi.setActiveTools()`/`pi.getActiveTools()` round-trips. The
    // `<available_skills>` piece is NOT computed here (dogfood fix — see
    // this file's header and `computeSkillsBlockCached`): it is resolved
    // live inside `registerLeadBootstrap`'s `before_agent_start` handler
    // instead, against a `pi.getCommands()` read taken well after Pi's own
    // post-`session_start` skill merge. `wsBlockBase` stays `undefined` for
    // `worker`/`explore`, or when `startBridge` produced no manual snapshot
    // (degraded bootstrap) — `wsBlockBaseRef.current` is then left
    // untouched, and `computeBeforeAgentStartResult`'s own guard already
    // treats that as "no override" for every `before_agent_start` firing.
    //
    // Review cycle 1 (Minor): the guide read is gated the same way the
    // pre-260906 code gated it — `isLeadOrFork` AND a present manual
    // snapshot — rather than running for every role. `guideText` is only
    // ever consumed inside `buildWsBlock` (now called from
    // `before_agent_start`), which itself only runs when `manualSnapshot` is
    // present, so reading it for `worker`/`explore` or a degraded bootstrap
    // was pure waste with no observable effect.
    const bootstrapRole = readSpawnRole(process.env);
    let guideText = "";
    if (isLeadOrFork(bootstrapRole) && handle.manualSnapshotRef.current) {
      try {
        guideText = readFileSync(piLeadGuidePath, "utf8");
      } catch {
        // Tolerate a missing guide file (e.g. a dev -e run against a source
        // tree that hasn't copied it yet) — the manual snapshot alone is
        // still a strict improvement over no ws block at all.
      }
    }
    const bootstrap = computeSessionBootstrap({
      role: bootstrapRole,
      manualSnapshot: handle.manualSnapshotRef.current,
      guideText,
      currentActiveTools: pi.getActiveTools(),
    });
    if (bootstrap.wsBlockBase !== undefined) {
      wsBlockBaseRef.current = bootstrap.wsBlockBase;
    }
    // Review cycle 1 (Minor): gated on `isLeadOrFork` again, matching the
    // pre-260906 code — for `worker`/`explore`, `computeSessionBootstrap`
    // already returns `activeTools` unchanged, so calling `setActiveTools`
    // there was a semantic no-op that still re-ran Pi's internal
    // `_rebuildSystemPrompt` for a role that previously never took that
    // path at all.
    if (isLeadOrFork(bootstrapRole)) {
      pi.setActiveTools(bootstrap.activeTools);
    }
  });

  pi.on("session_shutdown", async (_event, _ctx) => {
    // 260905: snapshot the children BEFORE stopAll() tears down their live
    // clients, so the next start of this session can announce them rather
    // than losing them silently (see agent-sidecar.ts's header). Ordering
    // still matters for the roll-call's accuracy — `captureOrphans` now also
    // captures already-dormant (parked) records, but only a live-at-shutdown
    // snapshot correctly reports which ones were still `running` at that
    // instant; after stopAll() every record reads as dormant/idle.
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
    // Held pushes die with the session, exactly like the Pi followUp queue
    // they stand in for: their registry is about to be discarded, so a status
    // line computed after this point would describe nothing. The sidecar
    // written above carries child IDENTITIES forward; reports are not
    // persisted (see spawner.ts's heldPushQueue).
    heldPushQueue.length = 0;
    // Review relay #1 (Minor, 260906): reset the compaction-in-flight flag
    // and both of goal-loop.ts's private markers beside the held-push queue
    // they gate — otherwise a shutdown/`/reload` that lands mid-compaction
    // leaves `leadCompactingRef` stuck `true` into the replacement session,
    // where every `followUp` push and `injectDiscussionSummary` would hold
    // forever with nothing left to release them.
    goalLoopHandle.resetCompactionStateForShutdown();
    leadIdleRef.current = undefined;
    // 260905 (live-agent widget ticket): stop the elapsed timer and clear the
    // widget/status segment (mirrors `leadIdleRef.current = undefined` above)
    // — the registries the controller closed over are about to be discarded.
    agentWidgetHandle?.stop();
    agentWidgetHandle = undefined;
    agentWidgetRefreshRef.current = undefined;
    handle?.shutdown();
    handle = undefined;
  });
}
