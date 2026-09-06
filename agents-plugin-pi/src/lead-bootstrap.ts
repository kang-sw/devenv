/**
 * Lead/fork session-start bootstrap (260904 Phase 1, §1/§4/§5; scope grew in
 * 260906 Phase 1 — see below): appends a fixed ws block — the session-start
 * `workflow_manual` snapshot, the Pi lead guide (`pi-lead-guide.md`), and an
 * `<available_skills>` block — to every `before_agent_start` system prompt,
 * for the host lead and a `fork` child (worker/explore never see it).
 *
 * Fetched once per `session_start` (index.ts, after `startBridge` resolves)
 * and held in `wsBlockRef` — a live ref, same convention as
 * `BridgeHandle.defaultSessionKeyRef` (`bridge.ts#L54-59`). Pi re-runs every
 * `before_agent_start` handler from the session's base system prompt on
 * EVERY turn (confirmed against the installed
 * `@earendil-works/pi-coding-agent` package's `agent-session.js` —
 * `this._baseSystemPrompt` is what `event.systemPrompt` starts from each
 * time, not the previous turn's override), so this extension must actually
 * return the override on every call from `wsBlockRef.current`, not rely on
 * a one-time mutation persisting. `BeforeAgentStartEventResult.systemPrompt`
 * is documented as chained when multiple extensions return it, so
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
 * once per `session_start` to produce BOTH the ws block above AND the
 * reshaped lead/fork active-tools surface, threading
 * `computeLeadActiveTools` (execute-gateway.ts), `addForkToolIfLead`
 * (fork.ts), `addAskToolsIfLead` (ask.ts), and `addSkillToolIfLeadOrFork`
 * (lead-skills.ts) in sequence — replacing what used to be three separate
 * `pi.setActiveTools()`/`pi.getActiveTools()` round-trips in `index.ts`
 * itself. This module's actual responsibility is therefore "the whole
 * lead/fork session-start bootstrap, including cross-module active-tools
 * reshaping," not only "system-prompt block composition."
 */

import type { BeforeAgentStartEventResult, ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { isLeadOrFork, readSpawnRole, type SpawnRole } from "./process-role.ts";
import { computeLeadActiveTools } from "./execute-gateway.ts";
import { addForkToolIfLead } from "./fork.ts";
import { addAskToolsIfLead } from "./ask.ts";
import { addSkillToolIfLeadOrFork, buildSkillsBlock, type LoadedSkill, type SkillEntry } from "./lead-skills.ts";

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

/** Inputs to `computeSessionBootstrap` — everything `index.ts`'s `session_start` otherwise threaded through three separate `pi.getActiveTools()`/`pi.setActiveTools()` round-trips plus a standalone `buildWsBlock` call. */
export interface SessionBootstrapInputs {
  role: SpawnRole | undefined;
  /** `handle.manualSnapshotRef.current` — `undefined`/empty means a degraded or not-yet-resolved bootstrap. */
  manualSnapshot: string | undefined;
  guideText: string;
  skillEntries: readonly SkillEntry[];
  loadSkillFile: (path: string) => LoadedSkill;
  currentActiveTools: readonly string[];
}

export interface SessionBootstrapResult {
  /** `undefined` means "leave `wsBlockRef.current` untouched" — never a mandate to clear it. */
  wsBlock: string | undefined;
  activeTools: string[];
}

/**
 * 260906 Phase 1 testability extraction: the single pure function that
 * produces the WHOLE lead/fork session-start outcome — the ws system-prompt
 * block AND the reshaped tool surface — for a given role, so a test can
 * drive `index.ts`'s actual sequencing (role gate -> skills block ->
 * `buildWsBlock` -> `computeLeadActiveTools` -> `addForkToolIfLead` ->
 * `addAskToolsIfLead` -> `addSkillToolIfLeadOrFork`) without re-implementing
 * a second copy of that order inside the test itself. `index.ts` calls this
 * once per `session_start` and applies the result (`wsBlockRef.current =
 * result.wsBlock` only when it is not `undefined`; a single
 * `pi.setActiveTools(result.activeTools)`), collapsing what used to be three
 * separate `pi.setActiveTools()`/`pi.getActiveTools()` round-trips into one.
 *
 * `worker`/`explore` short-circuit to `{ wsBlock: undefined, activeTools:
 * [...currentActiveTools] }` — no block, no reshape, tool surface passed
 * through unchanged (matches the pre-260906 behavior: those roles were never
 * touched by any of the four reshape steps). `buildSkillsBlock` (one
 * `readFile` per installed skill) is skipped entirely when `manualSnapshot`
 * is absent (degraded bootstrap — its output would be discarded anyway,
 * since no `wsBlock` is built without a manual snapshot to prefix): review
 * cycle 1 Minor fix, avoiding wasted per-skill IO on a path that never uses
 * the result.
 */
export function computeSessionBootstrap(inputs: SessionBootstrapInputs): SessionBootstrapResult {
  const { role, manualSnapshot, guideText, skillEntries, loadSkillFile, currentActiveTools } = inputs;
  if (!isLeadOrFork(role)) {
    return { wsBlock: undefined, activeTools: [...currentActiveTools] };
  }

  const wsBlock = manualSnapshot ? buildWsBlock(manualSnapshot, guideText, buildSkillsBlock(skillEntries, loadSkillFile)) : undefined;

  let activeTools = computeLeadActiveTools(currentActiveTools);
  activeTools = addForkToolIfLead(activeTools, role);
  activeTools = addAskToolsIfLead(activeTools, role);
  activeTools = addSkillToolIfLeadOrFork(activeTools, role);

  return { wsBlock, activeTools };
}

/**
 * Thin IO glue: registers the `before_agent_start` handler, reading the
 * current process-role from `process.env` and the current ws block from the
 * live `wsBlockRef` on every call (never captured once at registration
 * time). Registered at extension factory top level (declarative, no
 * subprocess involved) — the actual `wsBlockRef.current` fill happens inside
 * `session_start` (index.ts), same seam `startBridge`/`registerAgentTools`
 * already use.
 */
export function registerLeadBootstrap(pi: ExtensionAPI, wsBlockRef: { current: string | undefined }): void {
  pi.on("before_agent_start", (event) => computeBeforeAgentStartResult(event.systemPrompt, wsBlockRef.current, readSpawnRole(process.env)));
}
