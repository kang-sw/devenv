/**
 * System-prompt bootstrap (260904 Phase 1, §1/§4/§5): appends a fixed ws
 * block — the session-start `workflow_manual` snapshot plus the Pi lead
 * guide (`pi-lead-guide.md`) — to every `before_agent_start` system prompt,
 * for the host lead and, later, a `fork` child (worker/explore never see
 * it).
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
 */

import type { BeforeAgentStartEventResult, ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { isLeadOrFork, readSpawnRole, type SpawnRole } from "./process-role.ts";

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
 * the fixed marker line) first, the Pi lead guide second — order per §1.
 * Pure string composition, no IO.
 */
export function buildWsBlock(manualSnapshot: string, guideText: string): string {
  return `${SESSION_START_SNAPSHOT_MARKER}\n\n${manualSnapshot}\n\n${guideText}`;
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
