import { dockviewBridgeOptions } from "./dockviewBridge.js";
import { defaultSurfaceKinds, defaultSurfaceRegistry } from "./surfaceRegistry.js";
import {
  attachmentId,
  daemonResourceId,
  serializeWorkbenchLayout,
  type WorkbenchLayoutState,
} from "./layoutSerialization.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

function assert(condition: boolean, label: string) {
  if (!condition) {
    throw new Error(label);
  }
}

function assertThrows(action: () => unknown, pattern: RegExp, label: string) {
  try {
    action();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (!pattern.test(message)) {
      throw new Error(`${label}: error ${message} did not match ${pattern}`);
    }
    return;
  }

  throw new Error(`${label}: expected an error`);
}

const registry = defaultSurfaceRegistry();

assertDeepEqual(
  defaultSurfaceKinds,
  [
    "agent",
    "persistentTerminal",
    "editor",
    "viewer",
    "diff",
    "diagnostics",
    "eventsLog",
    "taskView",
    "inspector",
  ],
  "default registry exposes required surface kinds in stable order",
);

assertEqual(registry.agent.rowPolicy, "pinned", "agent row policy");
assertEqual(registry.persistentTerminal.rowPolicy, "pinned", "terminal row policy");
assertEqual(registry.agent.lifecycleOwner, "daemonProcess", "agent lifecycle owner");
assertEqual(registry.agent.closePolicy, "detachDaemonResource", "agent close policy");
assertEqual(
  registry.persistentTerminal.closePolicy,
  "detachDaemonResource",
  "terminal close policy",
);

for (const kind of ["editor", "viewer", "diff", "diagnostics", "eventsLog", "taskView", "inspector"] as const) {
  assertEqual(registry[kind].rowPolicy, "opened", `${kind} uses the opened row`);
}

const layout: WorkbenchLayoutState = {
  attachments: [
    {
      attachmentId: attachmentId("att-agent-main"),
      surfaceKind: "agent",
      title: "Agent",
      daemonResource: {
        serverId: daemonResourceId("server-local"),
        workspaceId: daemonResourceId("workspace-devenv"),
        workRootId: daemonResourceId("workroot-devenv"),
        instanceId: daemonResourceId("instance-agent-main"),
      },
    },
    {
      attachmentId: attachmentId("att-editor-1"),
      surfaceKind: "editor",
      title: "README.md",
      daemonResource: {
        serverId: daemonResourceId("server-local"),
        workspaceId: daemonResourceId("workspace-devenv"),
        workRootId: daemonResourceId("workroot-devenv"),
      },
    },
  ],
  arrangement: {
    type: "group",
    orientation: "horizontal",
    children: [
      { type: "attachment", attachmentId: attachmentId("att-agent-main") },
      { type: "attachment", attachmentId: attachmentId("att-editor-1") },
    ],
  },
  activeAttachmentId: attachmentId("att-editor-1"),
};

const serialized = serializeWorkbenchLayout(layout);
assertDeepEqual(
  serialized,
  {
    version: 1,
    attachments: [
      { attachmentId: "att-agent-main", surfaceKind: "agent", rowPolicy: "pinned" },
      { attachmentId: "att-editor-1", surfaceKind: "editor", rowPolicy: "opened" },
    ],
    arrangement: {
      type: "group",
      orientation: "horizontal",
      children: [
        { type: "attachment", attachmentId: "att-agent-main" },
        { type: "attachment", attachmentId: "att-editor-1" },
      ],
    },
    activeAttachmentId: "att-editor-1",
  },
  "serialized layout contains arrangement, attachment ids, surface kinds, and row policy only",
);

const serializedJson = JSON.stringify(serialized);
assert(!serializedJson.includes("server-local"), "serialized layout omits daemon server identity");
assert(
  !serializedJson.includes("workspace-devenv"),
  "serialized layout omits daemon workspace identity",
);
assert(!serializedJson.includes("workroot-devenv"), "serialized layout omits daemon workRoot identity");
assert(
  !serializedJson.includes("instance-agent-main"),
  "serialized layout omits daemon instance identity",
);

assertThrows(
  () =>
    serializeWorkbenchLayout({
      attachments: layout.attachments,
      arrangement: { type: "attachment", attachmentId: attachmentId("missing") },
    }),
  /unknown attachmentId/,
  "unknown arrangement attachment ids are rejected",
);

assertThrows(
  () =>
    serializeWorkbenchLayout({
      attachments: [layout.attachments[0], layout.attachments[0]],
      arrangement: null,
    }),
  /duplicate attachmentId/,
  "duplicate attachment ids are rejected",
);

assertDeepEqual(
  dockviewBridgeOptions,
  {
    disableDnd: true,
    disableFloatingGroups: true,
  },
  "bridge defaults disable raw Dockview drag/drop and floating group controls",
);
