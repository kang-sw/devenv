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
import type { ActivityTranscript, TranscriptBlock } from "./workRootActivity.js";

export type AgentChatHarness = "codex" | "opencode" | "claude";

export const agentChatHarnesses: readonly AgentChatHarness[] = [
  "codex",
  "opencode",
  "claude",
];

// Phase 3 (`260711-feat-ws-dashboard-agent-activity-chat-ui`) capability
// gating model. Field order/naming mirrors `AgentClientCapabilities` in
// `ws-dashboard/crates/core/src/agent_client_provider.rs#L38-L47` exactly
// (`serde(rename_all = "camelCase")`), so a real per-harness adapter's
// reported capabilities can populate this shape without a reshape once
// `260620` wires it through. Populated today by
// `activitySessionStub.ts#stubCapabilitiesForHarness` — no adapter anywhere
// backs `compact`/`steer`/`goal`/`rewind`/`fork` in a shipped route yet, so
// every value here is a forward-declared, hand-tiered stub reading, not a
// live capability probe.
export type AgentChatCapabilities = {
  readonly compact: boolean;
  readonly steer: boolean;
  readonly goal: boolean;
  readonly rewind: boolean;
  readonly fork: boolean;
  readonly skills: boolean;
};

export type AgentChatSessionView = {
  readonly activityId: string;
  readonly workRootId: string;
  readonly serverRoute?: string | null;
  readonly harness: AgentChatHarness;
  readonly title: string;
  readonly createdAtMs: number;
  readonly transcript: ActivityTranscript;
  readonly capabilities: AgentChatCapabilities;
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

let userTranscriptBlockSequence = 0;

/**
 * Append a real, sent user message to a session's transcript as a fresh
 * `TranscriptBlock` (Phase 3 prerequisite — the base send-input path). Pure:
 * returns a new `AgentChatSessionView` rather than mutating `session`.
 * Mirrors `activitySessionStub.ts`'s `stubTranscriptBlock` shape
 * (`role: "user"`, `renderKind: "markdown"`), with a fresh, unique `cursor`
 * so it never collides with an existing block or a streaming overlay entry.
 */
export function appendUserTranscriptBlock(
  session: AgentChatSessionView,
  text: string,
): AgentChatSessionView {
  userTranscriptBlockSequence += 1;
  const block: TranscriptBlock = {
    cursor: `user-sent-${Date.now().toString(36)}-${userTranscriptBlockSequence}`,
    timestamp: new Date().toISOString(),
    renderKind: "markdown",
    title: null,
    text,
    data: null,
    degraded: false,
    role: "user",
    turnId: undefined,
  };
  return {
    ...session,
    transcript: {
      ...session.transcript,
      blocks: [...session.transcript.blocks, block],
    },
  };
}

// `260720-bug-dashboard-fork-from-here-cutcursor-resolution` Phase 1: an
// optimistic block minted by `appendUserTranscriptBlock` above carries a
// client-only `user-sent-<ts>-<seq>` cursor that the daemon's fork
// cursor-resolution path structurally cannot resolve (real transcript
// cursors are plain sequential strings — "0", "1", "2", ...). Real transcript
// polling (`beginRealStreamingTurn` in `activitySessionClient.ts`) never
// wrote back into the canonical `session.transcript.blocks` array — it only
// fed a separate, render-only `streamingBlocks` overlay
// (`agentChatStreamMerge.ts`'s `mergeStreamingTranscriptBlocks`), so a live
// user bubble kept its optimistic cursor forever, and "fork from here" on it
// silently forked the whole thread instead of cutting.
function isOptimisticUserCursor(cursor: string): boolean {
  return cursor.startsWith("user-sent-");
}

/**
 * Reconcile a session's canonical optimistic (`user-sent-...`) user blocks
 * against a batch of freshly-polled transcript blocks, replacing each
 * still-unresolved optimistic block with the next daemon-confirmed user
 * block from the poll, in encountered order — position/turn-order matching
 * (the ticket's preferred candidate direction), not cursor-identity or text
 * matching, since minor text normalization between the optimistic echo and
 * the daemon's stored copy should not block reconciliation.
 *
 * Pure: returns the same `session` reference (no-op) if there is nothing to
 * reconcile, or a new `AgentChatSessionView` with the same block count/order
 * otherwise — a reconciled block replaces its optimistic predecessor
 * wholesale (the daemon's copy is authoritative for role/timestamp/text too),
 * it is never merely appended.
 */
export function reconcileOptimisticUserCursors(
  session: AgentChatSessionView,
  polledBlocks: readonly TranscriptBlock[],
): AgentChatSessionView {
  const confirmedUserBlocks = polledBlocks.filter(
    (block) => block.role === "user" && !isOptimisticUserCursor(block.cursor),
  );
  if (confirmedUserBlocks.length === 0) {
    return session;
  }
  let nextConfirmedIndex = 0;
  let changed = false;
  const nextBlocks = session.transcript.blocks.map((block) => {
    if (
      nextConfirmedIndex >= confirmedUserBlocks.length ||
      block.role !== "user" ||
      !isOptimisticUserCursor(block.cursor)
    ) {
      return block;
    }
    const confirmed = confirmedUserBlocks[nextConfirmedIndex]!;
    nextConfirmedIndex += 1;
    changed = true;
    return confirmed;
  });
  if (!changed) {
    return session;
  }
  return {
    ...session,
    transcript: {
      ...session.transcript,
      blocks: nextBlocks,
    },
  };
}
