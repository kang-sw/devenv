import {
  createWorkbenchDockviewBridge,
  dockviewBridgeOptions,
  type DockviewBridgePort,
} from "./dockviewBridge.js";
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
    attachmentIds: ["att-agent-main", "att-editor-1"],
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
  "serialized layout contains arrangement and attachment ids only",
);

const serializedJson = JSON.stringify(serialized);
assert(!serializedJson.includes("surfaceKind"), "serialized layout omits surface kind metadata");
assert(!serializedJson.includes("rowPolicy"), "serialized layout omits registry-derived row policy");
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

const addedPanels: unknown[] = [];
let addedGroupCount = 0;
let focusNextCount = 0;
let focusPreviousCount = 0;
const rawPanel = { id: "raw-panel", api: { close: () => undefined }, focus: () => undefined };
const rawGroup = { id: "raw-group", moveTo: () => undefined, maximize: () => undefined };
const fakePort = {
  addPanel(options: unknown) {
    addedPanels.push(options);
    return rawPanel;
  },
  addGroup() {
    addedGroupCount += 1;
    return rawGroup;
  },
  getPanel() {
    return undefined;
  },
  getGroup() {
    return undefined;
  },
  moveToNext() {
    focusNextCount += 1;
  },
  moveToPrevious() {
    focusPreviousCount += 1;
  },
} as unknown as DockviewBridgePort;

const bridge = createWorkbenchDockviewBridge(fakePort);
const panelHandle = bridge.addAttachment(layout.attachments[0]);
const groupHandle = bridge.addGroup();
bridge.focusNext();
bridge.focusPrevious();

assertEqual(addedPanels.length, 1, "bridge adds one Dockview panel through the port");
assertDeepEqual(
  addedPanels[0],
  {
    id: "att-agent-main",
    component: "agent",
    title: "Agent",
    params: { attachmentId: "att-agent-main", surfaceKind: "agent" },
  },
  "bridge maps dashboard attachment metadata to Dockview panel parameters internally",
);
assertEqual(addedGroupCount, 1, "bridge adds one Dockview group through the port");
assertEqual(focusNextCount, 1, "bridge forwards focus-next through the adapter");
assertEqual(focusPreviousCount, 1, "bridge forwards focus-previous through the adapter");

assert(panelHandle !== (rawPanel as unknown), "panel handle is dashboard-owned, not the raw Dockview panel");
assert(groupHandle !== (rawGroup as unknown), "group handle is dashboard-owned, not the raw Dockview group");
assertDeepEqual(
  Object.keys(panelHandle),
  ["type", "attachmentId"],
  "panel handle exposes only dashboard-owned handle fields",
);
assertDeepEqual(
  Object.keys(groupHandle),
  ["type", "groupHandleId"],
  "group handle exposes only dashboard-owned handle fields",
);
assert(!("api" in panelHandle), "panel handle does not expose Dockview panel api");
assert(!("focus" in panelHandle), "panel handle does not expose Dockview panel focus lifecycle");
assert(!("moveTo" in groupHandle), "group handle does not expose Dockview group movement");
assert(!("maximize" in groupHandle), "group handle does not expose Dockview group maximize lifecycle");
assertDeepEqual(
  bridge.serialize(layout),
  serialized,
  "bridge serialization returns the sanitized dashboard workbench layout",
);
