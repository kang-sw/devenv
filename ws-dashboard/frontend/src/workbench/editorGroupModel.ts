export const workbenchPaneDragMimeType = "application/x-ws-workbench-pane";

export type WorkbenchPaneCategory = "pinned" | "opened";

export type WorkbenchEditorPaneRef = {
  readonly id: string;
};

export type WorkbenchCategorizedPaneRef = WorkbenchEditorPaneRef & {
  readonly category: WorkbenchPaneCategory;
};

export type WorkbenchEditorGroupRef<
  TPane extends WorkbenchEditorPaneRef = WorkbenchEditorPaneRef,
> = {
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

export type WorkbenchPaneMoveResult<
  TGroup extends WorkbenchEditorGroupRef = WorkbenchEditorGroupRef,
> = {
  readonly groups: readonly TGroup[];
  readonly paneOrderByGroup: WorkbenchPaneOrder;
  readonly activePaneByGroup: WorkbenchActivePaneState;
};

export type WorkbenchDynamicGroupRequest = {
  readonly targetGroupId: string;
  /** Dockview display metadata only; dashboard group state persists ids and panes. */
  readonly targetGroupLabel?: string;
};

export type WorkbenchDynamicPaneMove = WorkbenchPaneMove & {
  readonly dynamicTargetGroup?: WorkbenchDynamicGroupRequest;
};

export type WorkbenchDynamicPaneMoveResult<
  TGroup extends WorkbenchEditorGroupRef = WorkbenchEditorGroupRef,
> = WorkbenchPaneMoveResult<TGroup> & {
  readonly createdGroupId: string | null;
};

export function partitionWorkbenchPanesByCategory<
  TPane extends WorkbenchCategorizedPaneRef,
>(panes: readonly TPane[]): Record<WorkbenchPaneCategory, TPane[]> {
  return {
    pinned: panes.filter((pane) => pane.category === "pinned"),
    opened: panes.filter((pane) => pane.category === "opened"),
  };
}

export function applyWorkbenchPaneOrder<TGroup extends WorkbenchEditorGroupRef>(
  groups: readonly TGroup[],
  orderByGroup: WorkbenchPaneOrder,
): TGroup[] {
  const paneById = new Map<string, TGroup["panes"][number]>();
  const originalGroupByPaneId = new Map<string, string>();

  for (const group of groups) {
    for (const pane of group.panes) {
      paneById.set(pane.id, pane as TGroup["panes"][number]);
      originalGroupByPaneId.set(pane.id, group.id);
    }
  }

  const consumedPaneIds = new Set<string>();
  const arrangedGroups = groups.map((group) => {
    const panes = (orderByGroup[group.id] ?? []).flatMap((paneId) => {
      const pane = paneById.get(paneId);
      if (!pane || consumedPaneIds.has(paneId)) {
        return [];
      }
      consumedPaneIds.add(paneId);
      return [pane];
    });

    return {
      ...group,
      panes,
    };
  });

  return arrangedGroups.map((group) => {
    const remainingOriginalPanes = groups
      .find((sourceGroup) => sourceGroup.id === group.id)
      ?.panes.filter(
        (pane) =>
          originalGroupByPaneId.get(pane.id) === group.id &&
          !consumedPaneIds.has(pane.id),
      );

    return {
      ...group,
      panes: [...group.panes, ...(remainingOriginalPanes ?? [])],
    };
  });
}

export function deriveWorkbenchPaneOrder(
  groups: readonly WorkbenchEditorGroupRef[],
): Record<string, string[]> {
  return Object.fromEntries(
    groups.map((group) => [group.id, group.panes.map((pane) => pane.id)]),
  );
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

  if (
    !movedPane ||
    !withoutMovedPane.some((group) => group.id === move.targetGroupId)
  ) {
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

export function selectWorkbenchPane(
  current: WorkbenchActivePaneState,
  groupId: string,
  paneId: string,
): Record<string, string> {
  return {
    ...current,
    [groupId]: paneId,
  };
}

export function resolveWorkbenchPaneDrop({
  dataTransferPaneId,
  fallbackPaneId,
  targetGroupId,
  beforePaneId,
}: {
  readonly dataTransferPaneId: string;
  readonly fallbackPaneId: string | null;
  readonly targetGroupId: string;
  readonly beforePaneId?: string;
}): WorkbenchPaneMove | null {
  const paneId = dataTransferPaneId || fallbackPaneId;
  if (!paneId || paneId === beforePaneId) {
    return null;
  }

  return {
    paneId,
    targetGroupId,
    beforePaneId,
  };
}

export function commitWorkbenchPaneMove<TGroup extends WorkbenchEditorGroupRef>(
  groups: readonly TGroup[],
  currentActivePaneByGroup: WorkbenchActivePaneState,
  move: WorkbenchPaneMove,
): WorkbenchPaneMoveResult<TGroup> {
  const movedGroups = moveWorkbenchPane(groups, move);
  const activePaneByGroup = reconcileActiveWorkbenchPanes(
    movedGroups,
    currentActivePaneByGroup,
    {
      [move.targetGroupId]: move.paneId,
    },
  );

  return {
    groups: movedGroups,
    paneOrderByGroup: deriveWorkbenchPaneOrder(movedGroups),
    activePaneByGroup,
  };
}

export function commitWorkbenchPaneMoveIntoDynamicGroup<
  TGroup extends WorkbenchEditorGroupRef,
>(
  groups: readonly TGroup[],
  currentActivePaneByGroup: WorkbenchActivePaneState,
  move: WorkbenchDynamicPaneMove,
): WorkbenchDynamicPaneMoveResult<TGroup> {
  // CONTRACT: Dockview-created split drops are represented in dashboard state
  // by creating or mapping a dashboard group id before committing the move.
  // The function returns the same serialized pane order and active-pane state
  // as commitWorkbenchPaneMove plus the id of a newly-created dashboard group,
  // if the Dockview target group was not already known.
  // Existing-group moves must preserve commitWorkbenchPaneMove semantics. Unknown
  // dynamicTargetGroup ids must synthesize an empty dashboard group before the
  // move, while targetGroupLabel remains Dockview-only metadata.
  const targetGroupExists = groups.some(
    (group) => group.id === move.targetGroupId,
  );
  const createdGroupId = targetGroupExists
    ? null
    : (move.dynamicTargetGroup?.targetGroupId ?? null);
  const groupsWithDynamicTarget = targetGroupExists
    ? groups
    : createdGroupId
      ? [
          ...groups,
          {
            id: createdGroupId,
            ...(move.dynamicTargetGroup?.targetGroupLabel
              ? { label: move.dynamicTargetGroup.targetGroupLabel }
              : {}),
            panes: [],
          } as unknown as TGroup,
        ]
      : groups;

  const committed = commitWorkbenchPaneMove(
    groupsWithDynamicTarget,
    currentActivePaneByGroup,
    move,
  );
  return {
    ...committed,
    createdGroupId,
  };
}
