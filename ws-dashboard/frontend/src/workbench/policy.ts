import { attachmentId, type AttachmentId } from "./layoutSerialization.js";
import {
  defaultSurfaceRegistry,
  type SurfaceKind,
  type SurfaceRegistry,
  type WorkbenchCloseConfirmationPolicy,
  type WorkbenchClosePolicy,
  type WorkbenchRowPolicy,
} from "./surfaceRegistry.js";

export type WorkbenchGroupId = string & {
  readonly __workbenchGroupId: unique symbol;
};
export type SurfaceLogicalKey = string & {
  readonly __surfaceLogicalKey: unique symbol;
};

export type WorkbenchPolicyGroup = {
  readonly groupId: WorkbenchGroupId;
};

export type WorkbenchPolicyAttachment = {
  readonly attachmentId: AttachmentId;
  readonly groupId: WorkbenchGroupId;
  readonly surfaceKind: SurfaceKind;
  readonly logicalKey: SurfaceLogicalKey;
};

export type WorkbenchPlacementState = {
  readonly groups: readonly WorkbenchPolicyGroup[];
  readonly attachments: readonly WorkbenchPolicyAttachment[];
  readonly focusedGroupId?: WorkbenchGroupId;
};

export type OpenSurfaceRequest = {
  readonly surfaceKind: SurfaceKind;
  readonly logicalKey: SurfaceLogicalKey;
  readonly attachmentId?: AttachmentId;
};

export type WorkbenchPlacementDecision =
  | {
      readonly type: "focusExisting";
      readonly attachmentId: AttachmentId;
      readonly groupId: WorkbenchGroupId;
      readonly logicalKey: SurfaceLogicalKey;
    }
  | {
      readonly type: "openNew";
      readonly attachmentId: AttachmentId;
      readonly groupId: WorkbenchGroupId;
      readonly logicalKey: SurfaceLogicalKey;
      readonly rowPolicy: WorkbenchRowPolicy;
    };

export type WorkbenchDynamicPlacementDecision = WorkbenchPlacementDecision & {
  readonly nextState: WorkbenchPlacementState;
  readonly createdGroupId: WorkbenchGroupId | null;
};

export type WorkbenchCloseDecision = {
  readonly closePolicy: WorkbenchClosePolicy;
  readonly behavior:
    | "detach"
    | "closeAttachment"
    | "releaseProjection"
    | "deferToProvider";
  readonly terminateReservation: WorkbenchTerminateReservation | null;
};

export type WorkbenchCloseConfirmationDecision = {
  readonly confirmationPolicy: WorkbenchCloseConfirmationPolicy;
  readonly presentation: "none" | "cursorNearPopover";
  readonly confirmLabel: "Yes" | null;
  readonly cancelLabel: "No" | null;
};

export type WorkbenchTerminateReservation = {
  readonly commandId: "workbench.lifecycle.terminate";
  readonly reserved: true;
  readonly surfaceKind: SurfaceKind;
};

export type PtyLogicalSize = {
  readonly columns: number;
  readonly rows: number;
};

export type VisualSplitSize = {
  readonly widthPx: number;
  readonly heightPx: number;
};

export type PtyVisualResizeDecision = {
  readonly logicalSize: PtyLogicalSize;
  readonly visualSize: VisualSplitSize;
  readonly resizeRequest: "deferred";
};

export const defaultPtyLogicalSize: PtyLogicalSize = Object.freeze({
  columns: 80,
  rows: 24,
});

export function workbenchGroupId(value: string): WorkbenchGroupId {
  assertNonEmpty(value, "workbenchGroupId");
  return value as WorkbenchGroupId;
}

export function surfaceLogicalKey(
  ...parts: readonly string[]
): SurfaceLogicalKey {
  if (parts.length === 0) {
    throw new Error("surfaceLogicalKey needs at least one part");
  }

  for (const part of parts) {
    assertNonEmpty(part, "surfaceLogicalKey part");
  }

  return parts.join("/") as SurfaceLogicalKey;
}

export function decideSurfaceOpen(
  state: WorkbenchPlacementState,
  request: OpenSurfaceRequest,
  registry: SurfaceRegistry = defaultSurfaceRegistry(),
): WorkbenchPlacementDecision {
  const existing = state.attachments.find(
    (attachment) => attachment.logicalKey === request.logicalKey,
  );
  if (existing) {
    return {
      type: "focusExisting",
      attachmentId: existing.attachmentId,
      groupId: existing.groupId,
      logicalKey: existing.logicalKey,
    };
  }

  const registryEntry = registry[request.surfaceKind];
  const groupId = selectTargetGroup(state, registryEntry.rowPolicy);
  return {
    type: "openNew",
    attachmentId:
      request.attachmentId ?? attachmentId(`att:${request.logicalKey}`),
    groupId,
    logicalKey: request.logicalKey,
    rowPolicy: registryEntry.rowPolicy,
  };
}

export function decideSurfaceOpenWithDynamicGroups(
  state: WorkbenchPlacementState,
  request: OpenSurfaceRequest,
  registry: SurfaceRegistry = defaultSurfaceRegistry(),
): WorkbenchDynamicPlacementDecision {
  // CONTRACT: Dynamic placement is the caller-facing policy for Dockview-backed
  // workbenches. Existing logical keys still focus their attachment. New
  // persistent terminal panes prefer group 1. New editor/read-only file panes
  // prefer group 2, creating group 2 when only group 1 exists. Groups 3+ are
  // user-created groups and are not automatic placement targets.
  // Duplicate logical keys must keep decideSurfaceOpen focusExisting behavior.
  // Generated dashboard groups use the ordered browser-state seed `group-N`
  // (next index after the current groups), never raw Dockview handles.
  const existing = state.attachments.find(
    (attachment) => attachment.logicalKey === request.logicalKey,
  );
  if (existing) {
    return {
      type: "focusExisting",
      attachmentId: existing.attachmentId,
      groupId: existing.groupId,
      logicalKey: existing.logicalKey,
      nextState: state,
      createdGroupId: null,
    };
  }

  if (state.groups.length === 0) {
    throw new Error("workbench placement needs at least one group");
  }

  const registryEntry = registry[request.surfaceKind];
  const { groupId, groups, createdGroupId } = selectDynamicTargetGroup(
    state,
    registryEntry.rowPolicy,
  );
  const attachment = {
    attachmentId:
      request.attachmentId ?? attachmentId(`att:${request.logicalKey}`),
    groupId,
    surfaceKind: request.surfaceKind,
    logicalKey: request.logicalKey,
  };

  return {
    type: "openNew",
    attachmentId: attachment.attachmentId,
    groupId,
    logicalKey: request.logicalKey,
    rowPolicy: registryEntry.rowPolicy,
    nextState: {
      ...state,
      groups,
      attachments: [...state.attachments, attachment],
    },
    createdGroupId,
  };
}

export function decideSurfaceClose(
  surfaceKind: SurfaceKind,
  registry: SurfaceRegistry = defaultSurfaceRegistry(),
): WorkbenchCloseDecision {
  const closePolicy = registry[surfaceKind].closePolicy;

  if (closePolicy === "detachDaemonResource") {
    return {
      closePolicy,
      behavior: "detach",
      terminateReservation: reserveTerminateCommand(surfaceKind),
    };
  }

  return {
    closePolicy,
    behavior: closePolicy,
    terminateReservation: null,
  };
}

export function decideSurfaceCloseConfirmation(
  surfaceKind: SurfaceKind,
  registry: SurfaceRegistry = defaultSurfaceRegistry(),
): WorkbenchCloseConfirmationDecision {
  // CONTRACT: Tab close confirmation is separate from close side effects.
  // Agent and persistent terminal tabs require a cursor-near Yes/No popover
  // before daemon-backed session close. Reversible views close immediately and
  // must not show a browser-native modal or custom confirmation popover.
  const confirmationPolicy = registry[surfaceKind].closeConfirmationPolicy;
  if (confirmationPolicy === "confirmSessionClose") {
    return {
      confirmationPolicy,
      presentation: "cursorNearPopover",
      confirmLabel: "Yes",
      cancelLabel: "No",
    };
  }

  return {
    confirmationPolicy,
    presentation: "none",
    confirmLabel: null,
    cancelLabel: null,
  };
}

export function reserveTerminateCommand(
  surfaceKind: SurfaceKind,
): WorkbenchTerminateReservation {
  return {
    commandId: "workbench.lifecycle.terminate",
    reserved: true,
    surfaceKind,
  };
}

export function preservePtyLogicalSize(
  logicalSize: PtyLogicalSize,
  visualSize: VisualSplitSize,
): PtyVisualResizeDecision {
  assertPositiveInteger(logicalSize.columns, "logical columns");
  assertPositiveInteger(logicalSize.rows, "logical rows");
  assertPositiveInteger(visualSize.widthPx, "visual width");
  assertPositiveInteger(visualSize.heightPx, "visual height");

  return {
    logicalSize,
    visualSize,
    resizeRequest: "deferred",
  };
}

function selectDynamicTargetGroup(
  state: WorkbenchPlacementState,
  rowPolicy: WorkbenchRowPolicy,
): {
  readonly groupId: WorkbenchGroupId;
  readonly groups: readonly WorkbenchPolicyGroup[];
  readonly createdGroupId: WorkbenchGroupId | null;
} {
  if (rowPolicy === "opened") {
    const secondGroup = state.groups[1]?.groupId;
    if (secondGroup) {
      return {
        groupId: secondGroup,
        groups: state.groups,
        createdGroupId: null,
      };
    }

    const createdGroupId = nextOrderedWorkbenchGroupId(state.groups);
    const groups = [...state.groups, { groupId: createdGroupId }];
    return { groupId: createdGroupId, groups, createdGroupId };
  }

  return {
    groupId: state.groups[0].groupId,
    groups: state.groups,
    createdGroupId: null,
  };
}

function nextOrderedWorkbenchGroupId(
  groups: readonly WorkbenchPolicyGroup[],
): WorkbenchGroupId {
  const used = new Set(groups.map((group) => group.groupId));
  for (let index = groups.length + 1; ; index += 1) {
    const candidate = workbenchGroupId(`group-${index}`);
    if (!used.has(candidate)) {
      return candidate;
    }
  }
}

function selectTargetGroup(
  state: WorkbenchPlacementState,
  rowPolicy: WorkbenchRowPolicy,
): WorkbenchGroupId {
  if (state.groups.length === 0) {
    throw new Error("workbench placement needs at least one group");
  }

  if (rowPolicy === "opened") {
    return state.groups[1]?.groupId ?? state.groups[0].groupId;
  }

  if (
    state.focusedGroupId &&
    state.groups.some((group) => group.groupId === state.focusedGroupId)
  ) {
    return state.focusedGroupId;
  }

  return state.groups[0].groupId;
}

function assertNonEmpty(value: string, label: string) {
  if (value.trim().length === 0) {
    throw new Error(`${label} must be non-empty`);
  }
}

function assertPositiveInteger(value: number, label: string) {
  if (!Number.isInteger(value) || value <= 0) {
    throw new Error(`${label} must be a positive integer`);
  }
}
