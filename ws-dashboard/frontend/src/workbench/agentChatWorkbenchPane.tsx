import {
  AgentChatPaneBody,
  type AgentChatPaneActions,
} from "../agentChatPaneBody.js";
import { agentChatHarnessLabel } from "../activitySessionStub.js";
import type { AgentChatPaneState } from "../agentChatSessions.js";
import { closeContractLabel } from "../resourcePresentation.js";
import type { ViewState, WorkRootView } from "../resourceModel.js";
import type { WorkbenchPane } from "./editorGroups.js";
import type { WorkbenchPaneOrder } from "./editorGroupModel.js";

export function agentChatWorkbenchPanesByGroup(
  root: WorkRootView,
  agentChatPanes: AgentChatPaneState[],
  agentChatPaneOrderByGroup: WorkbenchPaneOrder,
  actions: AgentChatPaneActions,
  groups: ReadonlyArray<{ id: string; label: string }>,
): Record<string, WorkbenchPane[]> {
  const panes = agentChatPanes
    .filter(
      (pane) =>
        pane.workRootId === root.id &&
        (pane.serverRoute ?? "server-local") === root.resourcePath.serverId,
    )
    .map((pane) => agentChatWorkbenchPane(pane, actions));
  const paneById = new Map(panes.map((pane) => [pane.id, pane]));
  const consumed = new Set<string>();
  const byGroup: Record<string, WorkbenchPane[]> = Object.fromEntries(
    groups.map((group) => [group.id, []]),
  );
  for (const groupId of groups.map((group) => group.id)) {
    for (const paneId of agentChatPaneOrderByGroup[groupId] ?? []) {
      const pane = paneById.get(paneId);
      if (pane && !consumed.has(paneId)) {
        byGroup[groupId].push(pane);
        consumed.add(paneId);
      }
    }
  }
  for (const pane of panes) {
    if (!consumed.has(pane.id))
      (byGroup[groups[0]?.id ?? "group-1"] ??= []).push(pane);
  }
  return byGroup;
}

export function agentChatWorkbenchPane(
  pane: AgentChatPaneState,
  actions: AgentChatPaneActions,
): WorkbenchPane {
  const session = pane.session;
  const state: ViewState = {
    status: pane.starting ? "starting" : session ? "running" : "idle",
    loading: pane.starting,
    stale: false,
    error: pane.error,
  };
  return {
    id: pane.paneId,
    kind: "agentChat",
    category: "opened",
    title: session
      ? `${agentChatHarnessLabel[session.harness]}${session.title ? ` — ${session.title}` : ""}`
      : "New agent chat",
    detail: session ? session.activityId : "no conversation started yet",
    state,
    meta: session
      ? [session.harness, closeContractLabel("agentChat")]
      : ["empty", closeContractLabel("agentChat")],
    // CONTRACT (Phase 3 fix): must change whenever the *canonical*
    // transcript changes (a real send/fork/resume), not just when
    // `activityId` changes - `shouldUpdateDockviewWorkbenchPanelParams`
    // (`workbench/dockviewLayoutModel.ts`) skips pushing a fresh `body`
    // element into Dockview unless `contentRevision` (or `meta`) differs,
    // so a `session.activityId`-only key left a real user send invisible:
    // `AgentChatPaneBody`'s local state (`turnInFlight`, `streamingBlocks`)
    // kept working since it lives inside the already-mounted component
    // instance, but the *prop* carrying the newly-appended transcript block
    // never reached Dockview's still-stale `pane`. In-flight streaming
    // overlay ticks intentionally do *not* bump this (they're local
    // component state, not part of `session.transcript.blocks`), preserving
    // the existing "avoid Dockview param churn on high-frequency updates"
    // intent this field already documents for terminals.
    contentRevision: session
      ? `agentChat:${pane.paneId}:${session.activityId}:${session.transcript.blocks.length}:${
          session.transcript.blocks[session.transcript.blocks.length - 1]?.cursor ?? ""
        }`
      : `agentChat:${pane.paneId}:empty`,
    body: <AgentChatPaneBody key={pane.paneId} pane={pane} actions={actions} />,
  };
}
