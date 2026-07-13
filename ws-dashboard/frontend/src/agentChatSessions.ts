// Phase-1 pane-state module for
// `260711-feat-ws-dashboard-agent-activity-chat-ui` — the per-click,
// multi-instance interactive Activity chat tab. Mirrors the subset of
// `terminals.ts`'s pane-state shape/helpers needed for a new
// `"agentChat"` `SurfaceKind` (see `workbench/surfaceRegistry.ts`), kept
// separate from the singleton `"agent"` pane (`mainInstance`) and the
// read-only `"workRootActivity"` projection tab.
//
// CONTRACT: a chat tab exists in the "empty" state (`session: null`)
// immediately on creation — the top-right "open new agent tab" button never
// blocks on a harness/session picker. `session` is only populated once a
// tile or history entry is picked and the stub/real
// `activity.session.create`/`start` call resolves (see
// `activitySessionStub.ts`).

import {
  LOCAL_DASHBOARD_SERVER_ROUTE,
  serverScopedIdentity,
} from "./resourceModel.js";
import type { ActivityTranscript } from "./workRootActivity.js";

export type AgentChatHarness = "codex" | "opencode" | "claude";

export const agentChatHarnesses: readonly AgentChatHarness[] = [
  "codex",
  "opencode",
  "claude",
];

export type AgentChatSessionView = {
  readonly activityId: string;
  readonly workRootId: string;
  readonly serverRoute?: string | null;
  readonly harness: AgentChatHarness;
  readonly title: string;
  readonly createdAtMs: number;
  readonly transcript: ActivityTranscript;
};

export type AgentChatPaneState = {
  readonly paneId: string;
  readonly logicalKey: string;
  readonly tabId: string;
  readonly workRootId: string;
  readonly serverRoute?: string | null;
  readonly localCreatedAtMs: number;
  readonly session: AgentChatSessionView | null;
  readonly starting: boolean;
  readonly error: string | null;
};

let agentChatTabSequence = 0;

export function nextAgentChatTabId(): string {
  agentChatTabSequence += 1;
  return `chat-${Date.now().toString(36)}-${agentChatTabSequence}`;
}

export function agentChatPaneLogicalKey(
  workRootId: string,
  tabId: string,
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
) {
  return [
    "agentChat",
    serverScopedIdentity(serverRoute, workRootId),
    tabId,
  ].join("/");
}

export function agentChatPaneId(
  tabId: string,
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
) {
  return `agentChat:${encodeURIComponent(serverScopedIdentity(serverRoute, tabId))}`;
}

export function createEmptyAgentChatPane(
  workRootId: string,
  serverRoute?: string | null,
  tabId: string = nextAgentChatTabId(),
): AgentChatPaneState {
  return {
    paneId: agentChatPaneId(tabId, serverRoute),
    logicalKey: agentChatPaneLogicalKey(workRootId, tabId, serverRoute),
    tabId,
    workRootId,
    serverRoute: serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE,
    localCreatedAtMs: Date.now(),
    session: null,
    starting: false,
    error: null,
  };
}

export function markAgentChatPaneStarting(
  pane: AgentChatPaneState,
): AgentChatPaneState {
  return { ...pane, starting: true, error: null };
}

export function attachAgentChatSession(
  pane: AgentChatPaneState,
  session: AgentChatSessionView,
): AgentChatPaneState {
  return { ...pane, session, starting: false, error: null };
}

export function markAgentChatPaneError(
  pane: AgentChatPaneState,
  error: string,
): AgentChatPaneState {
  return { ...pane, starting: false, error };
}

export function removeAgentChatPane(
  current: Record<string, AgentChatPaneState>,
  logicalKey: string,
): Record<string, AgentChatPaneState> {
  const next = { ...current };
  delete next[logicalKey];
  return next;
}

/**
 * Drop every agent-chat pane belonging to a specific work root/server pair.
 * Used when a work root is closed from the left panel — mirrors
 * `removeTerminalPanesForWorkRoot`. Chat sessions here are stub/local-only in
 * Phase 1, so unlike terminals there is no daemon-side session to leave
 * running; a future real-adapter phase may need a close/detach call here.
 */
export function removeAgentChatPanesForWorkRoot(
  current: Record<string, AgentChatPaneState>,
  rootId: string,
  serverRoute: string | undefined,
): Record<string, AgentChatPaneState> {
  const normalizedServerRoute = serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE;
  const next: Record<string, AgentChatPaneState> = {};
  for (const [key, pane] of Object.entries(current)) {
    const matches =
      pane.workRootId === rootId &&
      (pane.serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE) ===
        normalizedServerRoute;
    if (!matches) {
      next[key] = pane;
    }
  }
  return next;
}
