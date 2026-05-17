import type { WorkbenchDynamicPlacementDecision } from "./policy.js";

export type DashboardWorkbenchGroup = {
  readonly id: string;
  readonly label: string;
};

export function reconcileDashboardGroupsForPlacement(
  currentGroups: readonly DashboardWorkbenchGroup[],
  placement: WorkbenchDynamicPlacementDecision,
): readonly DashboardWorkbenchGroup[] {
  if (placement.type !== "openNew" || !placement.createdGroupId) {
    return currentGroups;
  }

  const existingById = new Map(currentGroups.map((group) => [group.id, group]));
  return placement.nextState.groups.map((group, index) => {
    const id = String(group.groupId);
    return (
      existingById.get(id) ?? {
        id,
        label: `group ${index + 1}`,
      }
    );
  });
}
