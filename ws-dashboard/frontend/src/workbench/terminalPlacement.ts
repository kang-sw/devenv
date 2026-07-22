import {
  terminalPaneFromSession,
  type TerminalPaneState,
  type TerminalSessionView,
} from "../terminals.js";
import type { WorkbenchPaneOrder } from "./editorGroupModel.js";
import {
  decideSurfaceOpenWithDynamicGroups,
  surfaceLogicalKey,
  workbenchGroupId,
  type WorkbenchPlacementState,
} from "./policy.js";
import { groupIdForPaneOrder } from "./paneOrder.js";

export function placeTerminalSessions(
  current: WorkbenchPaneOrder,
  existingPanes: Record<string, TerminalPaneState>,
  sessions: TerminalSessionView[],
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
): WorkbenchPaneOrder {
  let next = { ...current };
  let placementState = terminalPlacementState(
    existingPanes,
    groups,
    workbenchPaneOrderByGroup,
    current,
  );
  for (const session of sessions) {
    const decision = decideSurfaceOpenWithDynamicGroups(placementState, {
      surfaceKind: "persistentTerminal",
      logicalKey: surfaceLogicalKey(
        "persistentTerminal",
        session.workRootId,
        session.terminalId,
      ),
    });
    if (decision.type === "openNew") {
      const pane = terminalPaneFromSession(session);
      next = {
        ...next,
        [decision.groupId]: [...(next[decision.groupId] ?? []), pane.paneId],
      };
      placementState = {
        ...placementState,
        attachments: [
          ...placementState.attachments,
          {
            attachmentId:
              pane.paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
            groupId: decision.groupId,
            surfaceKind: "persistentTerminal",
            logicalKey: decision.logicalKey,
          },
        ],
      };
    }
  }
  return next;
}

export function terminalPlacementState(
  panesByLogicalKey: Record<string, TerminalPaneState>,
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
): WorkbenchPlacementState {
  const firstGroupId = groups[0]?.id ?? "group-1";
  return {
    groups: groups.map((group) => ({ groupId: workbenchGroupId(group.id) })),
    focusedGroupId: workbenchGroupId(firstGroupId),
    attachments: Object.values(panesByLogicalKey).map((pane) => ({
      attachmentId:
        pane.paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
      groupId: workbenchGroupId(
        groupIdForPaneOrder(
          pane.paneId,
          workbenchPaneOrderByGroup,
          terminalPaneOrderByGroup,
          firstGroupId,
        ),
      ),
      surfaceKind: "persistentTerminal",
      logicalKey: surfaceLogicalKey(
        "persistentTerminal",
        pane.session.workRootId,
        pane.session.terminalId,
      ),
    })),
  };
}
