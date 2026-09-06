/**
 * Lead/fork session-start bootstrap (260904 Phase 1, §1/§4/§5; scope grew in
 * 260906 Phase 1 — see below): appends a fixed ws block — the session-start
 * `workflow_manual` snapshot, the Pi lead guide (`pi-lead-guide.md`), and an
 * `<available_skills>` block — to every `before_agent_start` system prompt,
 * for the host lead and a `fork` child (worker/explore never see it).
 *
 * The manual-snapshot + guide-text portion (`WsBlockBase`) is fetched once
 * per `session_start` (index.ts, after `startBridge` resolves) and held in
 * `wsBlockBaseRef` — a live ref, same convention as
 * `BridgeHandle.defaultSessionKeyRef` (`bridge.ts#L54-59`). The
 * `<available_skills>` portion is deliberately NOT part of that one-time
 * fetch (see `computeSkillsBlockCached`'s doc comment below for the dogfood
 * bug that requires resolving it later, against Pi's live command list) — it
 * is composed fresh inside the `before_agent_start` handler itself, from
 * `wsBlockBaseRef` plus a live `pi.getCommands()` read.
 *
 * Pi re-runs every `before_agent_start` handler from the session's base
 * system prompt on EVERY turn (confirmed against the installed
 * `@earendil-works/pi-coding-agent` package's `agent-session.js` —
 * `this._baseSystemPrompt` is what `event.systemPrompt` starts from each
 * time, not the previous turn's override), so this extension must actually
 * return the override on every call, not rely on a one-time mutation
 * persisting. `BeforeAgentStartEventResult.systemPrompt` is documented as
 * chained when multiple extensions return it, so
 * `event.systemPrompt + "\n\n" + wsBlock` is always additive regardless of
 * this extension's registration order relative to others.
 *
 * §5: this handler depends only on `process.env`'s process-role marker,
 * never on `ctx.ui` or any TUI-only field — a headless `--mode rpc` lead
 * (no spawn marker set, same as an interactively-launched lead) gets the
 * exact same ws block.
 *
 * 260906 Phase 1 grew this module's scope beyond system-prompt composition:
 * `computeSessionBootstrap` is now the single pure function `index.ts` calls
 * once per `session_start` to produce BOTH the ws block's static base above
 * AND the reshaped lead/fork active-tools surface, threading
 * `computeLeadActiveTools` (execute-gateway.ts), `addForkToolIfLead`
 * (fork.ts), `addAskToolsIfLead` (ask.ts), and `addSkillToolIfLeadOrFork`
 * (lead-skills.ts) in sequence — replacing what used to be three separate
 * `pi.setActiveTools()`/`pi.getActiveTools()` round-trips in `index.ts`
 * itself. This module's actual responsibility is therefore "the whole
 * lead/fork session-start bootstrap, including cross-module active-tools
 * reshaping," not only "system-prompt block composition." A later dogfood
 * fix moved the `<available_skills>` sub-piece OUT of that once-per-session
 * computation and into the per-turn `before_agent_start` handler — see
 * `computeSkillsBlockCached` below.
 */

import type { BeforeAgentStartEventResult, ExtensionAPI, SlashCommandInfo } from "@earendil-works/pi-coding-agent";
import { isLeadOrFork, readSpawnRole, type SpawnRole } from "./process-role.ts";
import { computeLeadActiveTools } from "./execute-gateway.ts";
import { addForkToolIfLead } from "./fork.ts";
import { addAskToolsIfLead } from "./ask.ts";
import { addSkillToolIfLeadOrFork, buildSkillsBlock, loadSkillFile, resolveSkillEntries, type LoadedSkill } from "./lead-skills.ts";

/**
 * Fixed marker line prefixed onto the manual snapshot inside the ws block —
 * tells the model the manual body below is a one-time, session-start
 * capture (not refreshed mid-session), matching the workflow_manual mapping's
 * (bridge.ts) parallel "current session state" framing for per-call reads.
 */
export const SESSION_START_SNAPSHOT_MARKER =
  "Session-start snapshot of your ws workflow manual (fetched once; not refreshed mid-session — call ws__workflow_manual for your live current session state):";

/**
 * Builds the full ws system-prompt block: the manual snapshot (prefixed by
 * the fixed marker line) first, the Pi lead guide second, the
 * `<available_skills>` block (260906 Phase 1, see lead-skills.ts) third —
 * order per §1/§4/260906. Pure string composition, no IO. `skillsBlock` may
 * be `""` (no visible skills) — joined in unconditionally, tolerating a
 * harmless extra blank line, the same tolerance Pi's own
 * `formatSkillsForPrompt`-into-`buildSystemPrompt` concatenation already has
 * for an empty skills set.
 */
export function buildWsBlock(manualSnapshot: string, guideText: string, skillsBlock: string): string {
  return `${SESSION_START_SNAPSHOT_MARKER}\n\n${manualSnapshot}\n\n${guideText}\n\n${skillsBlock}`;
}

/** The once-per-`session_start` portion of the ws block (manual snapshot + guide text) — everything EXCEPT the live `<available_skills>` piece. See `computeSkillsBlockCached` for why skills are excluded from this static base. */
export interface WsBlockBase {
  manualSnapshot: string;
  guideText: string;
}

/**
 * Resolves the `<available_skills>` block against Pi's CURRENT
 * `pi.getCommands()` list, called fresh on every `before_agent_start`
 * firing — never a `session_start`-time snapshot.
 *
 * Dogfood bug this fixes: Pi's own startup order (confirmed against the
 * installed `agent-session.js`'s `bindExtensions`) runs `session_start`
 * FIRST and only afterwards awaits `extendResourcesFromExtensions`, which is
 * what merges this adapter's own `resources_discover` skill path into the
 * live resource loader `pi.getCommands()` reads from. A skills block built
 * from a `pi.getCommands()` read taken DURING `session_start` therefore
 * predates every ws skill — the model saw only whatever other extension's
 * skills (e.g. `imagegen`) had already registered by that point. Reading
 * live inside `before_agent_start` (which only ever fires once a user turn
 * is in flight, always after `bindExtensions` has awaited both steps in
 * sequence) sidesteps the ordering entirely.
 *
 * Caching strategy — deliberately the simpler of two considered options:
 * build the block ONCE, on the first `before_agent_start` firing whose live
 * skill-entry list is non-empty, and freeze that result in `cache` for every
 * later firing. The alternative (a per-path cache keyed on the entry list,
 * invalidated whenever the set of paths changes) stays exactly as correct
 * against a mid-session skill-pack hot-reload, but adds a second piece of
 * state (a path->body map plus a diffing step) to buy correctness for an
 * event this adapter has no other reason to support today — nothing else in
 * this codebase reacts to a live-added skill pack mid-session. "Build once
 * after non-empty" needs only one string ref and reads every SKILL.md's
 * frontmatter at most once per session, at the cost of not picking up a
 * skill pack that installs itself after the cache is already frozen (an
 * accepted trade-off; a fresh `session_start`, e.g. `/reload`, gets a fresh
 * `cache` ref from `index.ts` and re-freezes there).
 */
export function computeSkillsBlockCached(
  commands: readonly SlashCommandInfo[],
  loadFile: (path: string) => LoadedSkill,
  cache: { current: string | undefined },
): string {
  if (cache.current !== undefined) {
    return cache.current;
  }
  const entries = resolveSkillEntries(commands);
  if (entries.length === 0) {
    return "";
  }
  cache.current = buildSkillsBlock(entries, loadFile);
  return cache.current;
}

/**
 * Pure decision for one `before_agent_start` firing: `undefined` (no
 * override) for a `worker`/`explore` role, or when `wsBlock` hasn't been
 * filled yet (bootstrap still in flight, or degraded — no snapshot); the
 * chained `{ systemPrompt }` override otherwise. Split out of the handler
 * body (mirrors `decideOnSettle`'s pure-reducer extraction in goal-loop.ts)
 * so this decision is unit-testable without a fake `ExtensionAPI`/`pi.on`
 * capture.
 */
export function computeBeforeAgentStartResult(
  systemPrompt: string,
  wsBlock: string | undefined,
  role: SpawnRole | undefined,
): BeforeAgentStartEventResult | undefined {
  if (!isLeadOrFork(role) || !wsBlock) {
    return undefined;
  }
  return { systemPrompt: `${systemPrompt}\n\n${wsBlock}` };
}

/** Inputs to `computeSessionBootstrap` — everything `index.ts`'s `session_start` otherwise threaded through three separate `pi.getActiveTools()`/`pi.setActiveTools()` round-trips. Skill entries are deliberately NOT here — see `computeSkillsBlockCached`'s header for why that piece moved to `before_agent_start` instead of this once-per-`session_start` call. */
export interface SessionBootstrapInputs {
  role: SpawnRole | undefined;
  /** `handle.manualSnapshotRef.current` — `undefined`/empty means a degraded or not-yet-resolved bootstrap. */
  manualSnapshot: string | undefined;
  guideText: string;
  currentActiveTools: readonly string[];
}

export interface SessionBootstrapResult {
  /** `undefined` means "leave `wsBlockBaseRef.current` untouched" — never a mandate to clear it. */
  wsBlockBase: WsBlockBase | undefined;
  activeTools: string[];
}

/**
 * 260906 Phase 1 testability extraction: the single pure function that
 * produces the WHOLE lead/fork session-start outcome — the ws block's static
 * base AND the reshaped tool surface — for a given role, so a test can drive
 * `index.ts`'s actual sequencing (role gate -> `computeLeadActiveTools` ->
 * `addForkToolIfLead` -> `addAskToolsIfLead` -> `addSkillToolIfLeadOrFork`)
 * without re-implementing a second copy of that order inside the test
 * itself. `index.ts` calls this once per `session_start` and applies the
 * result (`wsBlockBaseRef.current = result.wsBlockBase` only when it is not
 * `undefined`; a single `pi.setActiveTools(result.activeTools)`), collapsing
 * what used to be three separate `pi.setActiveTools()`/`pi.getActiveTools()`
 * round-trips into one.
 *
 * `worker`/`explore` short-circuit to `{ wsBlockBase: undefined, activeTools:
 * [...currentActiveTools] }` — no block, no reshape, tool surface passed
 * through unchanged (matches the pre-260906 behavior: those roles were never
 * touched by any of the four reshape steps).
 *
 * A later dogfood fix removed the `<available_skills>` piece from this
 * function entirely: `computeSessionBootstrap` no longer takes
 * `skillEntries`/`loadSkillFile` and no longer calls `buildSkillsBlock` or
 * `buildWsBlock` at all — those now run per-turn inside
 * `registerLeadBootstrap`'s `before_agent_start` handler, against a LIVE
 * `pi.getCommands()` read, because a `session_start`-time skill snapshot
 * predates Pi's own post-`session_start` skill-resource merge (see
 * `computeSkillsBlockCached`). This function now only produces `WsBlockBase`
 * (manual snapshot + guide text) — the two inputs `before_agent_start` still
 * combines with the live skills block via `buildWsBlock`.
 */
export function computeSessionBootstrap(inputs: SessionBootstrapInputs): SessionBootstrapResult {
  const { role, manualSnapshot, guideText, currentActiveTools } = inputs;
  if (!isLeadOrFork(role)) {
    return { wsBlockBase: undefined, activeTools: [...currentActiveTools] };
  }

  const wsBlockBase: WsBlockBase | undefined = manualSnapshot ? { manualSnapshot, guideText } : undefined;

  let activeTools = computeLeadActiveTools(currentActiveTools);
  activeTools = addForkToolIfLead(activeTools, role);
  activeTools = addAskToolsIfLead(activeTools, role);
  activeTools = addSkillToolIfLeadOrFork(activeTools, role);

  return { wsBlockBase, activeTools };
}

/**
 * Thin IO glue: registers the `before_agent_start` handler, reading the
 * current process-role from `process.env`, the static ws-block base from the
 * live `wsBlockBaseRef`, and the live `<available_skills>` block (via
 * `computeSkillsBlockCached`, cached in `skillsBlockCacheRef`) on every call
 * — never captured once at registration time. Registered at extension
 * factory top level (declarative, no subprocess involved) — the actual
 * `wsBlockBaseRef.current` fill happens inside `session_start` (index.ts),
 * same seam `startBridge`/`registerAgentTools` already use.
 *
 * Bails out before touching `pi.getCommands()`/`computeSkillsBlockCached` at
 * all when there is no role/base to act on (`worker`/`explore`, or a
 * still-degraded bootstrap) — `wsBlockBaseRef.current` stays `undefined` for
 * those processes for the life of the session (`computeSessionBootstrap`
 * never fills it for them), so this is not just an optimization: it is the
 * same "no override" outcome `computeBeforeAgentStartResult` would reach
 * anyway, reached without the wasted per-skill frontmatter IO on every turn
 * of a role that never uses the result.
 */
export function registerLeadBootstrap(
  pi: ExtensionAPI,
  wsBlockBaseRef: { current: WsBlockBase | undefined },
  skillsBlockCacheRef: { current: string | undefined },
): void {
  pi.on("before_agent_start", (event) => {
    const role = readSpawnRole(process.env);
    const base = wsBlockBaseRef.current;
    if (!isLeadOrFork(role) || !base) {
      return computeBeforeAgentStartResult(event.systemPrompt, undefined, role);
    }
    const skillsBlock = computeSkillsBlockCached(pi.getCommands(), (path) => loadSkillFile(path), skillsBlockCacheRef);
    const wsBlock = buildWsBlock(base.manualSnapshot, base.guideText, skillsBlock);
    return computeBeforeAgentStartResult(event.systemPrompt, wsBlock, role);
  });
}
