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
