import type {
  DashboardResourcesView,
  InstanceView,
  WorkRootView,
} from "../resourceModel.js";

// Re-resolves a previously-opened work root against the current resources
// snapshot without depending on `selectedId`/tree-walk selection state. Used
// to keep every mounted per-root `DockviewWorkbenchLayout` instance in sync
// with live resource updates even while a different root is selected.
export function findOpenWorkRoot(
  resources: DashboardResourcesView | null,
  ref: { rootId: string; serverRoute: string },
): { root: WorkRootView; mainInstance: InstanceView | null } | null {
  if (!resources) {
    return null;
  }
  for (const workspace of resources.workspaces) {
    for (const root of workspace.workRoots) {
      if (
        root.id === ref.rootId &&
        root.resourcePath.serverId === ref.serverRoute
      ) {
        return { root, mainInstance: root.mainInstances[0] ?? null };
      }
    }
  }
  return null;
}
