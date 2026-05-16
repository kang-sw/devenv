import type { DockviewApi } from "dockview";
import type {
  AttachmentId,
  SerializedWorkbenchLayout,
  WorkbenchAttachment,
  WorkbenchLayoutState,
} from "./layoutSerialization.js";
import { serializeWorkbenchLayout } from "./layoutSerialization.js";

export type DockviewBridgeOptions = {
  readonly disableFloatingGroups: true;
};

export const dockviewBridgeOptions: DockviewBridgeOptions = Object.freeze({
  disableFloatingGroups: true,
});

export type DockviewBridgePort = Pick<
  DockviewApi,
  | "addPanel"
  | "addGroup"
  | "getPanel"
  | "getGroup"
  | "moveToNext"
  | "moveToPrevious"
>;

export type DockviewBridgeEvents = Pick<
  DockviewApi,
  "onWillDrop" | "onWillDragPanel" | "onWillDragGroup" | "onWillShowOverlay"
>;

export type WorkbenchPanelHandle = {
  readonly type: "workbenchPanel";
  readonly attachmentId: AttachmentId;
};

export type WorkbenchGroupHandleId = string & { readonly __workbenchGroupHandleId: unique symbol };

export type WorkbenchGroupHandle = {
  readonly type: "workbenchGroup";
  readonly groupHandleId: WorkbenchGroupHandleId;
};

export type WorkbenchDockviewPanelParams = {
  readonly attachmentId: string;
  readonly surfaceKind: WorkbenchAttachment["surfaceKind"];
};

export type WorkbenchDockviewBridge = {
  readonly options: DockviewBridgeOptions;
  addAttachment(attachment: WorkbenchAttachment): WorkbenchPanelHandle;
  addGroup(): WorkbenchGroupHandle;
  focusNext(): void;
  focusPrevious(): void;
  serialize(layout: WorkbenchLayoutState): SerializedWorkbenchLayout;
};

export function createWorkbenchDockviewBridge(port: DockviewBridgePort): WorkbenchDockviewBridge {
  let nextGroupHandle = 1;

  return {
    options: dockviewBridgeOptions,
    addAttachment(attachment) {
      port.addPanel<WorkbenchDockviewPanelParams>({
        id: attachment.attachmentId,
        component: attachment.surfaceKind,
        title: attachment.title,
        params: {
          attachmentId: attachment.attachmentId,
          surfaceKind: attachment.surfaceKind,
        },
      });

      return Object.freeze({
        type: "workbenchPanel",
        attachmentId: attachment.attachmentId,
      });
    },
    addGroup() {
      port.addGroup();

      return Object.freeze({
        type: "workbenchGroup",
        groupHandleId: `workbench-group-${nextGroupHandle++}` as WorkbenchGroupHandleId,
      });
    },
    focusNext() {
      port.moveToNext();
    },
    focusPrevious() {
      port.moveToPrevious();
    },
    serialize(layout) {
      return serializeWorkbenchLayout(layout);
    },
  };
}
