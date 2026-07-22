import type { ReadOnlyFilePane } from "../workRootFiles.js";
import type { WorkbenchPaneOrder } from "./editorGroupModel.js";
import {
  surfaceLogicalKey,
  workbenchGroupId,
  type WorkbenchPlacementState,
} from "./policy.js";
import { groupIdForPaneOrder } from "./paneOrder.js";

export function readOnlyFilePlacementState(
  panesByLogicalKey: Record<string, ReadOnlyFilePane>,
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder,
): WorkbenchPlacementState {
  const fallbackGroupId = groups[1]?.id ?? groups[0]?.id ?? "group-2";
  return {
    groups: groups.map((group) => ({ groupId: workbenchGroupId(group.id) })),
    attachments: Object.values(panesByLogicalKey).map((pane) => ({
      attachmentId:
        pane.id as WorkbenchPlacementState["attachments"][number]["attachmentId"],
      groupId: workbenchGroupId(
        groupIdForPaneOrder(
          pane.id,
          workbenchPaneOrderByGroup,
          readOnlyFilePaneOrderByGroup,
          fallbackGroupId,
        ),
      ),
      surfaceKind: "editor",
      logicalKey: surfaceLogicalKey(...pane.logicalKey.split("/")),
    })),
  };
}

export function sameReadOnlyOpenRequest(
  current: ReadOnlyFilePane | undefined,
  requested: ReadOnlyFilePane,
): current is ReadOnlyFilePane {
  return (
    current !== undefined &&
    current.workRootId === requested.workRootId &&
    current.path === requested.path &&
    current.mode === requested.mode
  );
}

export function readOnlyFilePaneRevision(pane: ReadOnlyFilePane) {
  return [
    "readonly",
    pane.status,
    pane.path,
    pane.sizeBytes ?? "",
    pane.languageHint ?? "",
    pane.extension ?? "",
    pane.error ?? "",
    hashText(pane.content),
  ].join(":");
}

export function hashText(value: string) {
  let hash = 5381;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 33) ^ value.charCodeAt(index);
  }
  return `${value.length}:${(hash >>> 0).toString(36)}`;
}
