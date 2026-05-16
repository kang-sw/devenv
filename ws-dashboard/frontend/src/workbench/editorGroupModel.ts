export type WorkbenchEditorPaneRef = {
  readonly id: string;
};

export type WorkbenchEditorGroupRef<TPane extends WorkbenchEditorPaneRef = WorkbenchEditorPaneRef> = {
  readonly id: string;
  readonly panes: readonly TPane[];
};

export type WorkbenchPaneOrder = Readonly<Record<string, readonly string[]>>;
export type WorkbenchActivePaneState = Readonly<Record<string, string>>;

export type WorkbenchPaneMove = {
  readonly paneId: string;
  readonly targetGroupId: string;
  readonly beforePaneId?: string;
};

export function applyWorkbenchPaneOrder<TGroup extends WorkbenchEditorGroupRef>(
  groups: readonly TGroup[],
  orderByGroup: WorkbenchPaneOrder,
): TGroup[] {
  return groups.map((group) => {
    const requestedOrder = orderByGroup[group.id] ?? [];
    const paneById = new Map(group.panes.map((pane) => [pane.id, pane]));
    const orderedPanes = requestedOrder.flatMap((paneId) => {
      const pane = paneById.get(paneId);
      if (!pane) {
        return [];
      }
      paneById.delete(paneId);
      return [pane];
    });

    return {
      ...group,
      panes: [...orderedPanes, ...paneById.values()],
    };
  });
}

export function deriveWorkbenchPaneOrder(groups: readonly WorkbenchEditorGroupRef[]): Record<string, string[]> {
  return Object.fromEntries(groups.map((group) => [group.id, group.panes.map((pane) => pane.id)]));
}

export function moveWorkbenchPane<TGroup extends WorkbenchEditorGroupRef>(
  groups: readonly TGroup[],
  move: WorkbenchPaneMove,
): TGroup[] {
  let movedPane: WorkbenchEditorPaneRef | undefined;
  const withoutMovedPane = groups.map((group) => {
    const nextPanes = group.panes.filter((pane) => {
      if (pane.id !== move.paneId) {
        return true;
      }
      movedPane = pane;
      return false;
    });

    return {
      ...group,
      panes: nextPanes,
    };
  });

  if (!movedPane || !withoutMovedPane.some((group) => group.id === move.targetGroupId)) {
    return groups.slice();
  }

  return withoutMovedPane.map((group) => {
    if (group.id !== move.targetGroupId) {
      return group;
    }

    const panes = [...group.panes];
    const targetIndex = move.beforePaneId
      ? panes.findIndex((pane) => pane.id === move.beforePaneId)
      : -1;
    const insertAt = targetIndex >= 0 ? targetIndex : panes.length;
    panes.splice(insertAt, 0, movedPane as TGroup["panes"][number]);

    return {
      ...group,
      panes,
    };
  });
}

export function reconcileActiveWorkbenchPanes(
  groups: readonly WorkbenchEditorGroupRef[],
  current: WorkbenchActivePaneState,
  preferred: WorkbenchActivePaneState = {},
): Record<string, string> {
  const next: Record<string, string> = {};

  for (const group of groups) {
    const paneIds = new Set(group.panes.map((pane) => pane.id));
    const preferredPaneId = preferred[group.id];
    const currentPaneId = current[group.id];
    const nextPaneId =
      (preferredPaneId && paneIds.has(preferredPaneId) && preferredPaneId) ||
      (currentPaneId && paneIds.has(currentPaneId) && currentPaneId) ||
      group.panes[0]?.id;

    if (nextPaneId) {
      next[group.id] = nextPaneId;
    }
  }

  return next;
}
