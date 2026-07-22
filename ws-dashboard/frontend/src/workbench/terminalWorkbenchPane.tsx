import {
  TerminalPaneBody,
  type TerminalPaneActions,
} from "../terminalPaneBody.js";
import type { TerminalPaneState } from "../terminals.js";
import type { ViewState, WorkRootView } from "../resourceModel.js";
import type { WorkbenchPane } from "./editorGroups.js";
import type { WorkbenchPaneOrder } from "./editorGroupModel.js";

export function terminalWorkbenchPanesByGroup(
  root: WorkRootView,
  terminalPanes: TerminalPaneState[],
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
  terminalActions: TerminalPaneActions,
  groups: ReadonlyArray<{ id: string; label: string }>,
): Record<string, WorkbenchPane[]> {
  const panes = terminalPanes
    .filter(
      (pane) =>
        pane.session.workRootId === root.id &&
        (pane.session.serverRoute ?? "server-local") ===
          root.resourcePath.serverId,
    )
    .map((pane) => terminalWorkbenchPane(pane, terminalActions));
  const paneById = new Map(panes.map((pane) => [pane.id, pane]));
  const consumed = new Set<string>();
  const byGroup: Record<string, WorkbenchPane[]> = Object.fromEntries(
    groups.map((group) => [group.id, []]),
  );
  for (const groupId of groups.map((group) => group.id)) {
    for (const paneId of terminalPaneOrderByGroup[groupId] ?? []) {
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

export function terminalWorkbenchPane(
  pane: TerminalPaneState,
  actions: TerminalPaneActions,
): WorkbenchPane {
  const state: ViewState = {
    status: pane.session.status,
    loading: pane.session.status === "starting",
    stale: false,
    error: pane.error,
  };
  return {
    id: pane.paneId,
    kind: "persistentTerminal",
    category: "pinned",
    title: pane.session.title,
    detail: pane.session.terminalId,
    state,
    meta: [
      pane.session.status,
      pane.socketStatus,
      `${pane.session.columns}x${pane.session.rows}`,
    ],
    contentRevision: `terminal:${pane.paneId}`,
    body: <TerminalPaneBody key={pane.paneId} pane={pane} actions={actions} />,
  };
}
