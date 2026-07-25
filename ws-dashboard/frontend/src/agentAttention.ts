// Pure endpoint-builder and parse helpers for the daemon's server-wide,
// work-root-independent per-terminal attention (turn-state) event stream.
// See ai-docs/spec/ws-web-dashboard/index.md#260726-dashboard-terminal-attention-event-stream.
//
// CONTRACT: mirrors `workRootActivity.ts`'s split of pure parsing from
// `App.tsx`'s live-wiring effect - this module stays free of React/DOM/
// `EventSource` so it is unit-testable without a browser. Unlike every other
// endpoint builder in this codebase, `attentionEventsEndpoint` addresses no
// per-work-root path segment: attention is keyed by terminal id across the
// whole daemon, not scoped to a workRoot.

import { localCompatibleDashboardApiRoute } from "./resourceModel.js";

export type AgentAttentionState = "working" | "ready" | "idle";

export type AgentAttentionEntry = {
  terminalId: string;
  workRootId: string;
  state: AgentAttentionState;
  updatedAtMs: number;
};

export function attentionEventsEndpoint(serverRoute?: string | null): string {
  return localCompatibleDashboardApiRoute(serverRoute, [
    "terminals",
    "attention",
    "events",
  ]);
}

// CONTRACT (260725 Phase 5 review cycle 1, finding C): the subscription
// effect's `EventSource` had no `onerror` handler. Per the WHATWG
// `EventSource` algorithm, a non-2xx response status (a 401 from an expired
// owner session, or the forwarder's bounded 502 when a linked server is
// momentarily unreachable) FAILS the connection PERMANENTLY - `readyState`
// goes to `CLOSED` and the UA never auto-reconnects on its own, unlike a
// clean stream end (this stream's `Lagged` case), which IS a reestablish
// condition the browser retries by itself. Combined with the effect's
// `sources.has(serverRoute)` re-creation guard, a permanently-failed source
// was never replaced, silently defeating the lag -> reconnect -> snapshot
// resync that is this design's only resync path.
//
// This module stays DOM-free (see the header comment), so the numeric
// `EventSource.CLOSED` readyState (`2`) is named here rather than imported,
// and the predicate takes a plain `readyState: number` rather than an
// `EventSource` - the one piece of the fix worth unit-testing without a
// browser. `App.tsx`'s `onerror` handler calls this, and on `true` closes
// the dead source and removes it from the subscription map so the next
// `serversView` poll's effect re-run (`resourceAvailabilityPollIntervalMs`,
// 5s) recreates it - a natural backoff bound against hammering a
// persistently-401/502 server, since recreation only ever happens on the
// NEXT poll tick, never synchronously inside this handler. A transient
// `CONNECTING` readyState means `EventSource` is already retrying with its
// own backoff and must be left alone, or this would fight the browser's own
// reconnect logic.
export const ATTENTION_SOURCE_CLOSED_READY_STATE = 2;

export function shouldReplaceAttentionSourceOnError(readyState: number): boolean {
  return readyState === ATTENTION_SOURCE_CLOSED_READY_STATE;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isAgentAttentionState(value: unknown): value is AgentAttentionState {
  return value === "working" || value === "ready" || value === "idle";
}

// Parses a single `event: attention` frame body.
export function parseAgentAttentionEntry(
  value: unknown,
): AgentAttentionEntry | null {
  if (
    !isObject(value) ||
    typeof value.terminalId !== "string" ||
    typeof value.workRootId !== "string" ||
    !isAgentAttentionState(value.state) ||
    typeof value.updatedAtMs !== "number"
  ) {
    return null;
  }
  return {
    terminalId: value.terminalId,
    workRootId: value.workRootId,
    state: value.state,
    updatedAtMs: value.updatedAtMs,
  };
}

// Parses an `event: attentionSnapshot` frame body (`{ items: [...] }`),
// emitted once on every fresh connection so a reconnect never loses a
// pending state that changed while no stream was open. Returns `null` (not
// a partial list) if any item fails to parse, so a malformed snapshot never
// silently drops entries the caller would otherwise trust as complete.
export function parseAgentAttentionSnapshot(
  value: unknown,
): AgentAttentionEntry[] | null {
  if (!isObject(value) || !Array.isArray(value.items)) {
    return null;
  }
  const entries: AgentAttentionEntry[] = [];
  for (const raw of value.items) {
    const entry = parseAgentAttentionEntry(raw);
    if (!entry) {
      return null;
    }
    entries.push(entry);
  }
  return entries;
}
