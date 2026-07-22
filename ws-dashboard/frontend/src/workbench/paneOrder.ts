import type { WorkbenchPaneOrder } from "./editorGroupModel.js";

export function addPaneToGroupOrder(
  orderByGroup: WorkbenchPaneOrder,
  paneId: string,
  groupId: string,
): WorkbenchPaneOrder {
  const withoutPane = removePaneFromOrder(orderByGroup, paneId);
  return {
    ...withoutPane,
    [groupId]: [...(withoutPane[groupId] ?? []), paneId],
  };
}

export function removePaneFromOrder(
  orderByGroup: WorkbenchPaneOrder,
  paneId: string | undefined,
): WorkbenchPaneOrder {
  if (!paneId) {
    return orderByGroup;
  }
  return Object.fromEntries(
    Object.entries(orderByGroup).map(([groupId, paneIds]) => [
      groupId,
      paneIds.filter((candidate) => candidate !== paneId),
    ]),
  );
}

export function activityPaneGroupIdFromOrder(
  paneId: string,
  orderByGroup: WorkbenchPaneOrder,
  groups: ReadonlyArray<{ id: string }>,
): string {
  return groupIdForPaneOrder(
    paneId,
    orderByGroup,
    {},
    groups[1]?.id ?? groups[0]?.id ?? "group-1",
  );
}

export function groupIdForPaneOrder(
  paneId: string,
  primaryOrderByGroup: WorkbenchPaneOrder,
  fallbackOrderByGroup: WorkbenchPaneOrder,
  fallbackGroupId: string,
): string {
  return (
    Object.entries(primaryOrderByGroup).find(([, paneIds]) =>
      paneIds.includes(paneId),
    )?.[0] ??
    Object.entries(fallbackOrderByGroup).find(([, paneIds]) =>
      paneIds.includes(paneId),
    )?.[0] ??
    fallbackGroupId
  );
}
