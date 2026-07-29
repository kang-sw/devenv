import {
  TerminalPaneBody,
  type TerminalPaneActions,
} from "../terminalPaneBody.js";
import type { TerminalPaneState } from "../terminals.js";
import { serverScopedIdentity } from "../resourceModel.js";
import type { ViewState, WorkRootView } from "../resourceModel.js";
import {
  pendingAttentionStateFor,
  type AgentAttentionAcknowledgements,
  type AgentAttentionEntry,
} from "../agentAttention.js";
import type { WorkbenchPane } from "./editorGroups.js";
import type { WorkbenchPaneOrder } from "./editorGroupModel.js";

// The two App-level maps the tab-label indicator derives from (260725
// Phase 6): live attention entries and the tab-click ack watermark, both
// keyed by `serverScopedIdentity(serverRoute, terminalId)`.
export type TerminalAttentionInput = {
  readonly attentionByKey: Readonly<Record<string, AgentAttentionEntry>>;
  readonly acknowledgements: AgentAttentionAcknowledgements;
};

export const EMPTY_TERMINAL_ATTENTION: TerminalAttentionInput = {
  attentionByKey: {},
  acknowledgements: {},
};

// CONTRACT (260725 Phase 6): the ONE key-builder both the writer
// (`App.tsx`'s attention-stream handlers) and this reader use. Hand-rolling
// the join here would silently desync the moment either side changes.
export function terminalAttentionKey(pane: TerminalPaneState): string {
  return serverScopedIdentity(
    pane.session.serverRoute,
    pane.session.terminalId,
  );
}

export function terminalWorkbenchPanesByGroup(
  root: WorkRootView,
  terminalPanes: TerminalPaneState[],
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
  terminalActions: TerminalPaneActions,
  groups: ReadonlyArray<{ id: string; label: string }>,
  attention: TerminalAttentionInput = EMPTY_TERMINAL_ATTENTION,
): Record<string, WorkbenchPane[]> {
  const panes = terminalPanes
    .filter(
      (pane) =>
        pane.session.workRootId === root.id &&
        (pane.session.serverRoute ?? "server-local") ===
          root.resourcePath.serverId,
    )
    .map((pane) => terminalWorkbenchPane(pane, terminalActions, attention));
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
  attention: TerminalAttentionInput = EMPTY_TERMINAL_ATTENTION,
): WorkbenchPane {
  const state: ViewState = {
    status: pane.session.status,
    loading: pane.session.status === "starting",
    stale: false,
    error: pane.error,
  };
  const attentionKey = terminalAttentionKey(pane);
  // `?? undefined` (not `?? null`): `WorkbenchPane.attentionState` is an
  // OPTIONAL field, and `shouldUpdateDockviewWorkbenchPanelParams` compares
  // it by `!==` - a `null` here and an absent field there would read as a
  // spurious change on the very first sync of every non-attention pane.
  const attentionState =
    pendingAttentionStateFor(
      attention.attentionByKey[attentionKey],
      attention.acknowledgements[attentionKey],
      pane.session.status,
    ) ?? undefined;
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
    attentionState,
    body: <TerminalPaneBody key={pane.paneId} pane={pane} actions={actions} />,
  };
}
