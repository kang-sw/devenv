import type { DockviewApi, DockviewGroupPanel, IDockviewPanel } from "dockview";
import type { WorkbenchLayoutState, SerializedWorkbenchLayout, WorkbenchAttachment } from "./layoutSerialization.js";
import { serializeWorkbenchLayout } from "./layoutSerialization.js";

export type DockviewBridgeOptions = {
  readonly disableDnd: true;
  readonly disableFloatingGroups: true;
};

export const dockviewBridgeOptions: DockviewBridgeOptions = Object.freeze({
  disableDnd: true,
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

export type DockviewPanelHandle = IDockviewPanel;
export type DockviewGroupHandle = DockviewGroupPanel;

export type WorkbenchDockviewPanelParams = {
  readonly attachmentId: string;
  readonly surfaceKind: WorkbenchAttachment["surfaceKind"];
};

export type WorkbenchDockviewBridge = {
  readonly options: DockviewBridgeOptions;
  addAttachment(attachment: WorkbenchAttachment): DockviewPanelHandle;
  addGroup(): DockviewGroupHandle;
  focusNext(): void;
  focusPrevious(): void;
  serialize(layout: WorkbenchLayoutState): SerializedWorkbenchLayout;
};

export function createWorkbenchDockviewBridge(port: DockviewBridgePort): WorkbenchDockviewBridge {
  return {
    options: dockviewBridgeOptions,
    addAttachment(attachment) {
      return port.addPanel<WorkbenchDockviewPanelParams>({
        id: attachment.attachmentId,
        component: attachment.surfaceKind,
        title: attachment.title,
        params: {
          attachmentId: attachment.attachmentId,
          surfaceKind: attachment.surfaceKind,
        },
      });
    },
    addGroup() {
      return port.addGroup();
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
