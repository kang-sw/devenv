import type { AgentChatPaneState } from "../agentChatSessions.js";
import type { WorkbenchPaneOrder } from "./editorGroupModel.js";
import {
  decideSurfaceOpenWithDynamicGroups,
  surfaceLogicalKey,
  workbenchGroupId,
  type WorkbenchPlacementState,
} from "./policy.js";
import { groupIdForPaneOrder } from "./paneOrder.js";

export function placeAgentChatPane(
  current: WorkbenchPaneOrder,
  existingPanes: Record<string, AgentChatPaneState>,
  pane: AgentChatPaneState,
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
): WorkbenchPaneOrder {
  const placementState = agentChatPlacementState(
    existingPanes,
    groups,
    workbenchPaneOrderByGroup,
    current,
  );
  const decision = decideSurfaceOpenWithDynamicGroups(placementState, {
    surfaceKind: "agentChat",
    logicalKey: surfaceLogicalKey("agentChat", pane.workRootId, pane.tabId),
  });
  if (decision.type !== "openNew") {
    return current;
  }
  return {
    ...current,
    [decision.groupId]: [...(current[decision.groupId] ?? []), pane.paneId],
  };
}

export function agentChatPlacementState(
  panesByLogicalKey: Record<string, AgentChatPaneState>,
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
  agentChatPaneOrderByGroup: WorkbenchPaneOrder,
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
          agentChatPaneOrderByGroup,
          firstGroupId,
        ),
      ),
      surfaceKind: "agentChat",
      logicalKey: surfaceLogicalKey("agentChat", pane.workRootId, pane.tabId),
    })),
  };
}
