import type { SurfaceKind } from "./surfaceRegistry.js";

export type AttachmentId = string & { readonly __attachmentId: unique symbol };
export type DaemonResourceId = string & { readonly __daemonResourceId: unique symbol };

export type DaemonResourceRef = {
  readonly serverId: DaemonResourceId;
  readonly workspaceId?: DaemonResourceId;
  readonly workRootId?: DaemonResourceId;
  readonly instanceId?: DaemonResourceId;
};

export type WorkbenchAttachment = {
  readonly attachmentId: AttachmentId;
  readonly surfaceKind: SurfaceKind;
  readonly title?: string;
  readonly daemonResource?: DaemonResourceRef;
};

export type WorkbenchArrangementNode =
  | {
      readonly type: "attachment";
      readonly attachmentId: AttachmentId;
    }
  | {
      readonly type: "group";
      readonly orientation: "horizontal" | "vertical";
      readonly children: readonly WorkbenchArrangementNode[];
    };

export type WorkbenchLayoutState = {
  readonly attachments: readonly WorkbenchAttachment[];
  readonly arrangement: WorkbenchArrangementNode | null;
  readonly activeAttachmentId?: AttachmentId;
};

export type SerializedWorkbenchArrangementNode =
  | {
      readonly type: "attachment";
      readonly attachmentId: string;
    }
  | {
      readonly type: "group";
      readonly orientation: "horizontal" | "vertical";
      readonly children: readonly SerializedWorkbenchArrangementNode[];
    };

export type SerializedWorkbenchLayout = {
  readonly version: 1;
  readonly attachmentIds: readonly string[];
  readonly arrangement: SerializedWorkbenchArrangementNode | null;
  readonly activeAttachmentId?: string;
};

export function attachmentId(value: string): AttachmentId {
  assertNonEmptyId(value, "attachmentId");
  return value as AttachmentId;
}

export function daemonResourceId(value: string): DaemonResourceId {
  assertNonEmptyId(value, "daemonResourceId");
  return value as DaemonResourceId;
}

export function serializeWorkbenchLayout(layout: WorkbenchLayoutState): SerializedWorkbenchLayout {
  const attachmentIds = new Set<string>();
  const serializedAttachmentIds = layout.attachments.map((attachment) => {
    const id = attachment.attachmentId;
    if (attachmentIds.has(id)) {
      throw new Error(`duplicate attachmentId: ${id}`);
    }
    attachmentIds.add(id);
    return id;
  });

  if (layout.activeAttachmentId && !attachmentIds.has(layout.activeAttachmentId)) {
    throw new Error(`active attachment is not part of layout: ${layout.activeAttachmentId}`);
  }

  return {
    version: 1,
    attachmentIds: serializedAttachmentIds,
    arrangement: serializeArrangement(layout.arrangement, attachmentIds),
    ...(layout.activeAttachmentId ? { activeAttachmentId: layout.activeAttachmentId } : {}),
  };
}

function serializeArrangement(
  node: WorkbenchArrangementNode | null,
  attachmentIds: ReadonlySet<string>,
): SerializedWorkbenchArrangementNode | null {
  if (!node) {
    return null;
  }

  if (node.type === "attachment") {
    if (!attachmentIds.has(node.attachmentId)) {
      throw new Error(`arrangement references unknown attachmentId: ${node.attachmentId}`);
    }
    return { type: "attachment", attachmentId: node.attachmentId };
  }

  return {
    type: "group",
    orientation: node.orientation,
    children: node.children
      .map((child) => serializeArrangement(child, attachmentIds))
      .filter(isArrangementNode),
  };
}

function isArrangementNode(
  value: SerializedWorkbenchArrangementNode | null,
): value is SerializedWorkbenchArrangementNode {
  return value !== null;
}

function assertNonEmptyId(value: string, label: string) {
  if (value.trim().length === 0) {
    throw new Error(`${label} must be non-empty`);
  }
}
