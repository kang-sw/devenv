import {
  createWorkbenchDockviewBridge,
  dockviewBridgeOptions,
  type DockviewBridgePort,
} from "./dockviewBridge.js";
import {
  defaultSurfaceKinds,
  defaultSurfaceRegistry,
} from "./surfaceRegistry.js";
import {
  applyWorkbenchPaneOrder,
  commitWorkbenchPaneMove,
  commitWorkbenchPaneMoveIntoDynamicGroup,
  deriveWorkbenchPaneOrder,
  moveWorkbenchPane,
  partitionWorkbenchPanesByCategory,
  reconcileActiveWorkbenchPanes,
  resolveWorkbenchPaneDrop,
  selectWorkbenchPane,
  workbenchPaneDragMimeType,
} from "./editorGroupModel.js";
import {
  attachmentId,
  daemonResourceId,
  serializeWorkbenchLayout,
  type WorkbenchLayoutState,
} from "./layoutSerialization.js";
import { reconcileDashboardGroupsForPlacement } from "./placementGroups.js";
import {
  decideSurfaceClose,
  decideSurfaceCloseConfirmation,
  decideSurfaceOpen,
  decideSurfaceOpenWithDynamicGroups,
  decideWorkbenchTabClosePresentation,
  defaultPtyLogicalSize,
  preservePtyLogicalSize,
  reserveTerminateCommand,
  surfaceLogicalKey,
  workbenchGroupId,
  type WorkbenchPlacementState,
} from "./policy.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
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
    "workRootActivity",
    "taskView",
    "inspector",
  ],
  "default registry exposes required surface kinds in stable order",
);

assertEqual(registry.agent.rowPolicy, "pinned", "agent row policy");
assertEqual(
  registry.persistentTerminal.rowPolicy,
  "pinned",
  "terminal row policy",
);
assertEqual(
  registry.agent.lifecycleOwner,
  "daemonProcess",
  "agent lifecycle owner",
);
assertEqual(
  registry.agent.closePolicy,
  "detachDaemonResource",
  "agent close policy",
);
assertEqual(
  registry.persistentTerminal.closePolicy,
  "detachDaemonResource",
  "terminal close policy",
);

for (const kind of [
  "editor",
  "viewer",
  "diff",
  "diagnostics",
  "eventsLog",
  "workRootActivity",
  "taskView",
  "inspector",
] as const) {
  assertEqual(
    registry[kind].rowPolicy,
    "opened",
    `${kind} uses the opened row`,
  );
}

// CONTRACT: Phase 3 must add behavior tests proving workRootActivity opens in
// group 1 despite its opened row policy, duplicate logical keys focus the
// existing activity pane, and close presentation stays immediate with no
// cursor-near confirmation.
// Placement assertions live with the existing terminal/editor dynamic-group
// assertions below and use surfaceLogicalKey("workRootActivity", root id).
// App uses surfaceLogicalKey("workRootActivity", root id) for one activity pane
// per selected workRoot. App-level pane storage/rendering is left to the
// implementation slice; policy behavior is locked below.

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
assert(
  !serializedJson.includes("surfaceKind"),
  "serialized layout omits surface kind metadata",
);
assert(
  !serializedJson.includes("rowPolicy"),
  "serialized layout omits registry-derived row policy",
);
assert(
  !serializedJson.includes("server-local"),
  "serialized layout omits daemon server identity",
);
assert(
  !serializedJson.includes("workspace-devenv"),
  "serialized layout omits daemon workspace identity",
);
assert(
  !serializedJson.includes("workroot-devenv"),
  "serialized layout omits daemon workRoot identity",
);
assert(
  !serializedJson.includes("instance-agent-main"),
  "serialized layout omits daemon instance identity",
);

assertThrows(
  () =>
    serializeWorkbenchLayout({
      attachments: layout.attachments,
      arrangement: {
        type: "attachment",
        attachmentId: attachmentId("missing"),
      },
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
    disableFloatingGroups: true,
  },
  "bridge defaults allow Dockview tab movement while disabling floating group controls",
);
assert(
  !("disableDnd" in dockviewBridgeOptions),
  "bridge does not hard-disable Dockview tab drag/reorder behavior",
);

const editorGroups = [
  {
    id: "primary",
    panes: [{ id: "agent" }, { id: "terminal" }, { id: "viewer" }],
  },
  {
    id: "support",
    panes: [{ id: "editor" }, { id: "tasks" }, { id: "diagnostics" }],
  },
] as const;

assertDeepEqual(
  partitionWorkbenchPanesByCategory([
    { id: "main-agent", category: "pinned" },
    { id: "persistent-terminal", category: "pinned" },
    { id: "selected-viewer", category: "opened" },
    { id: "diagnostics", category: "opened" },
  ]),
  {
    pinned: [
      { id: "main-agent", category: "pinned" },
      { id: "persistent-terminal", category: "pinned" },
    ],
    opened: [
      { id: "selected-viewer", category: "opened" },
      { id: "diagnostics", category: "opened" },
    ],
  },
  "visible workbench header preserves compact pinned and opened pane categories",
);

const reorderedEditorGroups = moveWorkbenchPane(editorGroups, {
  paneId: "viewer",
  targetGroupId: "primary",
  beforePaneId: "agent",
});
assertDeepEqual(
  deriveWorkbenchPaneOrder(reorderedEditorGroups),
  {
    primary: ["viewer", "agent", "terminal"],
    support: ["editor", "tasks", "diagnostics"],
  },
  "visible workbench movement model reorders tabs inside a split group",
);

const crossSplitEditorGroups = moveWorkbenchPane(reorderedEditorGroups, {
  paneId: "terminal",
  targetGroupId: "support",
  beforePaneId: "diagnostics",
});
assertDeepEqual(
  deriveWorkbenchPaneOrder(crossSplitEditorGroups),
  {
    primary: ["viewer", "agent"],
    support: ["editor", "tasks", "terminal", "diagnostics"],
  },
  "visible workbench movement model moves tabs across split groups",
);

assertDeepEqual(
  applyWorkbenchPaneOrder(editorGroups, {
    primary: ["terminal", "missing"],
    support: ["diagnostics"],
  }).map((group) => ({
    id: group.id,
    panes: group.panes.map((pane) => pane.id),
  })),
  [
    { id: "primary", panes: ["terminal", "agent", "viewer"] },
    { id: "support", panes: ["diagnostics", "editor", "tasks"] },
  ],
  "visible workbench movement model reapplies saved tab order without dropping new panes",
);

assertDeepEqual(
  reconcileActiveWorkbenchPanes(
    crossSplitEditorGroups,
    { primary: "terminal", support: "editor" },
    { support: "terminal" },
  ),
  {
    primary: "viewer",
    support: "terminal",
  },
  "visible workbench active state follows moved panes and falls back per group",
);

assertDeepEqual(
  deriveWorkbenchPaneOrder(
    applyWorkbenchPaneOrder(
      editorGroups,
      deriveWorkbenchPaneOrder(crossSplitEditorGroups),
    ),
  ),
  {
    primary: ["viewer", "agent"],
    support: ["editor", "tasks", "terminal", "diagnostics"],
  },
  "visible workbench movement model preserves cross-split group membership across rerenders",
);

const emptiedSourceGroups = moveWorkbenchPane(
  [
    { id: "primary", panes: [{ id: "only" }] },
    { id: "support", panes: [{ id: "editor" }] },
  ],
  { paneId: "only", targetGroupId: "support" },
);
assertDeepEqual(
  deriveWorkbenchPaneOrder(emptiedSourceGroups),
  {
    primary: [],
    support: ["editor", "only"],
  },
  "visible workbench movement model can represent an empty source split for the empty drop target UI",
);
assertDeepEqual(
  reconcileActiveWorkbenchPanes(
    emptiedSourceGroups,
    { primary: "only", support: "editor" },
    { support: "only" },
  ),
  {
    support: "only",
  },
  "visible workbench active state omits empty split groups instead of preserving stale active panes",
);

assertDeepEqual(
  selectWorkbenchPane({ primary: "agent" }, "support", "tasks"),
  {
    primary: "agent",
    support: "tasks",
  },
  "visible workbench click selection updates group-local active pane state",
);

assertEqual(
  workbenchPaneDragMimeType,
  "application/x-ws-workbench-pane",
  "visible workbench drag wiring uses a stable pane dataTransfer MIME key",
);
assertDeepEqual(
  resolveWorkbenchPaneDrop({
    dataTransferPaneId: "terminal",
    fallbackPaneId: "agent",
    targetGroupId: "support",
    beforePaneId: "editor",
  }),
  {
    paneId: "terminal",
    targetGroupId: "support",
    beforePaneId: "editor",
  },
  "visible workbench drop wiring prefers the dataTransfer pane id for cross-split moves",
);
assertDeepEqual(
  resolveWorkbenchPaneDrop({
    dataTransferPaneId: "",
    fallbackPaneId: "agent",
    targetGroupId: "primary",
    beforePaneId: "viewer",
  }),
  {
    paneId: "agent",
    targetGroupId: "primary",
    beforePaneId: "viewer",
  },
  "visible workbench drop wiring falls back to local drag state for reorder moves",
);
assertEqual(
  resolveWorkbenchPaneDrop({
    dataTransferPaneId: "agent",
    fallbackPaneId: null,
    targetGroupId: "primary",
    beforePaneId: "agent",
  }),
  null,
  "visible workbench drop wiring ignores self-drops on the same tab",
);

const committedMove = commitWorkbenchPaneMove(
  editorGroups,
  { primary: "terminal", support: "editor" },
  {
    paneId: "terminal",
    targetGroupId: "support",
    beforePaneId: "diagnostics",
  },
);
assertDeepEqual(
  committedMove,
  {
    groups: [
      { id: "primary", panes: [{ id: "agent" }, { id: "viewer" }] },
      {
        id: "support",
        panes: [
          { id: "editor" },
          { id: "tasks" },
          { id: "terminal" },
          { id: "diagnostics" },
        ],
      },
    ],
    paneOrderByGroup: {
      primary: ["agent", "viewer"],
      support: ["editor", "tasks", "terminal", "diagnostics"],
    },
    activePaneByGroup: {
      primary: "agent",
      support: "terminal",
    },
  },
  "visible workbench move commit covers drag move, serialized group membership, and active body reconciliation",
);

assertDeepEqual(
  commitWorkbenchPaneMoveIntoDynamicGroup(
    editorGroups,
    { primary: "terminal", support: "editor" },
    {
      paneId: "viewer",
      targetGroupId: "group-3",
      dynamicTargetGroup: {
        targetGroupId: "group-3",
        targetGroupLabel: "group 3",
      },
    },
  ),
  {
    groups: [
      { id: "primary", panes: [{ id: "agent" }, { id: "terminal" }] },
      {
        id: "support",
        panes: [{ id: "editor" }, { id: "tasks" }, { id: "diagnostics" }],
      },
      { id: "group-3", label: "group 3", panes: [{ id: "viewer" }] },
    ],
    paneOrderByGroup: {
      primary: ["agent", "terminal"],
      support: ["editor", "tasks", "diagnostics"],
      "group-3": ["viewer"],
    },
    activePaneByGroup: {
      primary: "terminal",
      support: "editor",
      "group-3": "viewer",
    },
    createdGroupId: "group-3",
  },
  "dynamic workbench move commit creates a durable dashboard group for Dockview split drops",
);

assertDeepEqual(
  commitWorkbenchPaneMoveIntoDynamicGroup(
    editorGroups,
    { primary: "terminal", support: "editor" },
    {
      paneId: "terminal",
      targetGroupId: "support",
      beforePaneId: "diagnostics",
    },
  ),
  {
    ...committedMove,
    createdGroupId: null,
  },
  "dynamic workbench move commit preserves existing-group move semantics",
);

const addedPanels: unknown[] = [];
let addedGroupCount = 0;
let focusNextCount = 0;
let focusPreviousCount = 0;
const rawPanel = {
  id: "raw-panel",
  api: { close: () => undefined },
  focus: () => undefined,
};
const rawGroup = {
  id: "raw-group",
  moveTo: () => undefined,
  maximize: () => undefined,
};
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

assertEqual(
  addedPanels.length,
  1,
  "bridge adds one Dockview panel through the port",
);
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
assertEqual(
  addedGroupCount,
  1,
  "bridge adds one Dockview group through the port",
);
assertEqual(
  focusNextCount,
  1,
  "bridge forwards focus-next through the adapter",
);
assertEqual(
  focusPreviousCount,
  1,
  "bridge forwards focus-previous through the adapter",
);

assert(
  panelHandle !== (rawPanel as unknown),
  "panel handle is dashboard-owned, not the raw Dockview panel",
);
assert(
  groupHandle !== (rawGroup as unknown),
  "group handle is dashboard-owned, not the raw Dockview group",
);
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
assert(
  !("api" in panelHandle),
  "panel handle does not expose Dockview panel api",
);
assert(
  !("focus" in panelHandle),
  "panel handle does not expose Dockview panel focus lifecycle",
);
assert(
  !("moveTo" in groupHandle),
  "group handle does not expose Dockview group movement",
);
assert(
  !("maximize" in groupHandle),
  "group handle does not expose Dockview group maximize lifecycle",
);
assertDeepEqual(
  bridge.serialize(layout),
  serialized,
  "bridge serialization returns the sanitized dashboard workbench layout",
);

const groupOne = workbenchGroupId("group-1");
const groupTwo = workbenchGroupId("group-2");
const agentKey = surfaceLogicalKey(
  "agent",
  "workroot-devenv",
  "instance-agent-main",
);
const editorKey = surfaceLogicalKey("editor", "workroot-devenv", "README.md");
const workRootActivityKey = surfaceLogicalKey(
  "workRootActivity",
  "workroot-devenv",
);
const placementState: WorkbenchPlacementState = {
  groups: [{ groupId: groupOne }, { groupId: groupTwo }],
  focusedGroupId: groupTwo,
  attachments: [
    {
      attachmentId: attachmentId("att-existing-editor"),
      groupId: groupTwo,
      surfaceKind: "editor",
      logicalKey: editorKey,
    },
  ],
};

assertDeepEqual(
  decideSurfaceOpen(placementState, {
    surfaceKind: "editor",
    logicalKey: editorKey,
    attachmentId: attachmentId("att-duplicate-editor"),
  }),
  {
    type: "focusExisting",
    attachmentId: "att-existing-editor",
    groupId: "group-2",
    logicalKey: "editor/workroot-devenv/README.md",
  },
  "surface open focuses an existing logical key instead of creating a duplicate attachment",
);

assertDeepEqual(
  decideSurfaceOpen(placementState, {
    surfaceKind: "viewer",
    logicalKey: surfaceLogicalKey("viewer", "workroot-devenv", "preview"),
    attachmentId: attachmentId("att-viewer-preview"),
  }),
  {
    type: "openNew",
    attachmentId: "att-viewer-preview",
    groupId: "group-2",
    logicalKey: "viewer/workroot-devenv/preview",
    rowPolicy: "opened",
  },
  "opened/support surfaces prefer the second split group when it exists",
);

assertDeepEqual(
  decideSurfaceOpen(placementState, {
    surfaceKind: "agent",
    logicalKey: agentKey,
    attachmentId: attachmentId("att-agent-main"),
  }),
  {
    type: "openNew",
    attachmentId: "att-agent-main",
    groupId: "group-2",
    logicalKey: "agent/workroot-devenv/instance-agent-main",
    rowPolicy: "pinned",
  },
  "durable pinned surfaces prefer the focused group",
);

assertDeepEqual(
  decideSurfaceOpen(
    { groups: [{ groupId: groupOne }], attachments: [] },
    {
      surfaceKind: "persistentTerminal",
      logicalKey: surfaceLogicalKey("terminal", "workroot-devenv"),
      attachmentId: attachmentId("att-terminal"),
    },
  ),
  {
    type: "openNew",
    attachmentId: "att-terminal",
    groupId: "group-1",
    logicalKey: "terminal/workroot-devenv",
    rowPolicy: "pinned",
  },
  "durable pinned surfaces fall back to the first group",
);

assertDeepEqual(
  decideSurfaceOpenWithDynamicGroups(placementState, {
    surfaceKind: "workRootActivity",
    logicalKey: workRootActivityKey,
    attachmentId: attachmentId("att-workroot-activity"),
  }),
  {
    type: "openNew",
    attachmentId: "att-workroot-activity",
    groupId: "group-2",
    logicalKey: "workRootActivity/workroot-devenv",
    rowPolicy: "opened",
    nextState: {
      ...placementState,
      attachments: [
        ...placementState.attachments,
        {
          attachmentId: "att-workroot-activity",
          groupId: "group-2",
          surfaceKind: "workRootActivity",
          logicalKey: "workRootActivity/workroot-devenv",
        },
      ],
    },
    createdGroupId: null,
  },
  "WorkRoot Activity uses opened-row support-split placement for new panes",
);

assertDeepEqual(
  decideSurfaceOpenWithDynamicGroups(
    {
      groups: [{ groupId: groupOne }, { groupId: groupTwo }],
      focusedGroupId: groupTwo,
      attachments: [
        {
          attachmentId: attachmentId("att-workroot-activity"),
          groupId: groupOne,
          surfaceKind: "workRootActivity",
          logicalKey: workRootActivityKey,
        },
      ],
    },
    {
      surfaceKind: "workRootActivity",
      logicalKey: workRootActivityKey,
      attachmentId: attachmentId("att-workroot-activity-duplicate"),
    },
  ),
  {
    type: "focusExisting",
    attachmentId: "att-workroot-activity",
    groupId: "group-1",
    logicalKey: "workRootActivity/workroot-devenv",
    nextState: {
      groups: [{ groupId: "group-1" }, { groupId: "group-2" }],
      focusedGroupId: "group-2",
      attachments: [
        {
          attachmentId: "att-workroot-activity",
          groupId: "group-1",
          surfaceKind: "workRootActivity",
          logicalKey: "workRootActivity/workroot-devenv",
        },
      ],
    },
    createdGroupId: null,
  },
  "duplicate WorkRoot Activity opens focus the existing logical key",
);

assertDeepEqual(
  decideWorkbenchTabClosePresentation("workRootActivity", {
    clientX: 12,
    clientY: 34,
  }),
  {
    type: "closeImmediately",
    confirmation: {
      confirmationPolicy: "none",
      presentation: "none",
      confirmLabel: null,
      cancelLabel: null,
    },
  },
  "WorkRoot Activity panes close immediately with no confirmation popover",
);

const readmeFileKey = surfaceLogicalKey(
  "editor",
  "root-local-abc",
  "README.md",
);
const nestedFileKey = surfaceLogicalKey(
  "editor",
  "root-local-abc",
  "src/main.rs",
);
assertDeepEqual(
  decideSurfaceOpen(
    {
      groups: [{ groupId: groupOne }, { groupId: groupTwo }],
      attachments: [
        {
          attachmentId: attachmentId("att-readme"),
          groupId: groupTwo,
          surfaceKind: "editor",
          logicalKey: readmeFileKey,
        },
      ],
    },
    {
      surfaceKind: "editor",
      logicalKey: readmeFileKey,
      attachmentId: attachmentId("att-readme-duplicate"),
    },
  ),
  {
    type: "focusExisting",
    attachmentId: "att-readme",
    groupId: "group-2",
    logicalKey: "editor/root-local-abc/README.md",
  },
  "read-only file logical key dedupes the same workRootId and relative path",
);
assertDeepEqual(
  decideSurfaceOpen(
    { groups: [{ groupId: groupOne }, { groupId: groupTwo }], attachments: [] },
    {
      surfaceKind: "editor",
      logicalKey: nestedFileKey,
      attachmentId: attachmentId("att-main-rs"),
    },
  ),
  {
    type: "openNew",
    attachmentId: "att-main-rs",
    groupId: "group-2",
    logicalKey: "editor/root-local-abc/src/main.rs",
    rowPolicy: "opened",
  },
  "new read-only file panes prefer the second split group",
);
assertDeepEqual(
  decideSurfaceOpen(
    { groups: [{ groupId: groupOne }], attachments: [] },
    {
      surfaceKind: "editor",
      logicalKey: nestedFileKey,
      attachmentId: attachmentId("att-main-rs"),
    },
  ),
  {
    type: "openNew",
    attachmentId: "att-main-rs",
    groupId: "group-1",
    logicalKey: "editor/root-local-abc/src/main.rs",
    rowPolicy: "opened",
  },
  "new read-only file panes fall back to the first split when it is the only group",
);
assertDeepEqual(
  decideSurfaceOpenWithDynamicGroups(
    { groups: [{ groupId: groupOne }], attachments: [] },
    {
      surfaceKind: "editor",
      logicalKey: nestedFileKey,
      attachmentId: attachmentId("att-main-rs"),
    },
  ),
  {
    type: "openNew",
    attachmentId: "att-main-rs",
    groupId: "group-2",
    logicalKey: "editor/root-local-abc/src/main.rs",
    rowPolicy: "opened",
    nextState: {
      groups: [{ groupId: "group-1" }, { groupId: "group-2" }],
      attachments: [
        {
          attachmentId: "att-main-rs",
          groupId: "group-2",
          surfaceKind: "editor",
          logicalKey: "editor/root-local-abc/src/main.rs",
        },
      ],
    },
    createdGroupId: "group-2",
  },
  "dynamic placement creates group 2 for editor/file opens when only group 1 exists",
);
const oneGroupEditorPlacement = decideSurfaceOpenWithDynamicGroups(
  { groups: [{ groupId: groupOne }], attachments: [] },
  {
    surfaceKind: "editor",
    logicalKey: nestedFileKey,
    attachmentId: attachmentId("att-main-rs"),
  },
);
assertDeepEqual(
  reconcileDashboardGroupsForPlacement(
    [{ id: "group-1", label: "group 1" }],
    oneGroupEditorPlacement,
  ),
  [
    { id: "group-1", label: "group 1" },
    { id: "group-2", label: "group 2" },
  ],
  "read-only App wiring persists policy-created group 2 from group-1-only placement",
);
assertDeepEqual(
  reconcileDashboardGroupsForPlacement(
    [
      { id: "group-1", label: "group 1" },
      { id: "group-2", label: "custom editors" },
    ],
    decideSurfaceOpenWithDynamicGroups(
      { groups: [{ groupId: groupOne }, { groupId: groupTwo }], attachments: [] },
      {
        surfaceKind: "editor",
        logicalKey: surfaceLogicalKey("editor", "root-local-abc", "src/lib.rs"),
        attachmentId: attachmentId("att-lib-rs"),
      },
    ),
  ),
  [
    { id: "group-1", label: "group 1" },
    { id: "group-2", label: "custom editors" },
  ],
  "read-only App wiring preserves existing group labels when no group is created",
);
assertDeepEqual(
  decideSurfaceOpenWithDynamicGroups(
    {
      groups: [
        { groupId: groupOne },
        { groupId: groupTwo },
        { groupId: workbenchGroupId("group-3") },
      ],
      attachments: [],
    },
    {
      surfaceKind: "editor",
      logicalKey: surfaceLogicalKey("editor", "root-local-abc", "src/lib.rs"),
      attachmentId: attachmentId("att-lib-rs"),
    },
  ).groupId,
  "group-2",
  "dynamic placement excludes group 3 from automatic editor placement",
);

const activityLogicalKey = surfaceLogicalKey("workRootActivity", "root-local-abc");
assertDeepEqual(
  decideSurfaceOpenWithDynamicGroups(
    { groups: [{ groupId: groupOne }, { groupId: groupTwo }], attachments: [] },
    {
      surfaceKind: "workRootActivity",
      logicalKey: activityLogicalKey,
      attachmentId: attachmentId("att-activity"),
    },
  ),
  {
    type: "openNew",
    attachmentId: "att-activity",
    groupId: "group-2",
    logicalKey: "workRootActivity/root-local-abc",
    rowPolicy: "opened",
    nextState: {
      groups: [{ groupId: "group-1" }, { groupId: "group-2" }],
      attachments: [
        {
          attachmentId: "att-activity",
          groupId: "group-2",
          surfaceKind: "workRootActivity",
          logicalKey: "workRootActivity/root-local-abc",
        },
      ],
    },
    createdGroupId: null,
  },
  "dynamic placement sends a new WorkRoot Activity pane to the support split",
);
assertDeepEqual(
  decideSurfaceOpenWithDynamicGroups(
    { groups: [{ groupId: groupOne }], attachments: [] },
    {
      surfaceKind: "workRootActivity",
      logicalKey: activityLogicalKey,
      attachmentId: attachmentId("att-activity"),
    },
  ).createdGroupId,
  "group-2",
  "dynamic placement creates the support split for WorkRoot Activity when needed",
);
assertDeepEqual(
  decideSurfaceOpenWithDynamicGroups(
    {
      groups: [{ groupId: groupOne }, { groupId: groupTwo }],
      attachments: [
        {
          attachmentId: attachmentId("att-activity"),
          groupId: groupTwo,
          surfaceKind: "workRootActivity",
          logicalKey: activityLogicalKey,
        },
      ],
    },
    {
      surfaceKind: "workRootActivity",
      logicalKey: activityLogicalKey,
      attachmentId: attachmentId("att-activity-duplicate"),
    },
  ),
  {
    type: "focusExisting",
    attachmentId: "att-activity",
    groupId: "group-2",
    logicalKey: "workRootActivity/root-local-abc",
    nextState: {
      groups: [{ groupId: "group-1" }, { groupId: "group-2" }],
      attachments: [
        {
          attachmentId: "att-activity",
          groupId: "group-2",
          surfaceKind: "workRootActivity",
          logicalKey: "workRootActivity/root-local-abc",
        },
      ],
    },
    createdGroupId: null,
  },
  "dynamic placement focuses an existing WorkRoot Activity pane in its support group",
);

assert(
  readmeFileKey !== nestedFileKey,
  "different read-only file paths open distinct logical panes",
);
assert(
  !String(readmeFileKey).includes("/Users/"),
  "read-only logical key omits raw host paths",
);

const terminalSessionKey = surfaceLogicalKey(
  "persistentTerminal",
  "root-local-abc",
  "term_abc",
);
assertDeepEqual(
  decideSurfaceOpen(
    {
      groups: [{ groupId: groupOne }, { groupId: groupTwo }],
      focusedGroupId: groupTwo,
      attachments: [
        {
          attachmentId: attachmentId("att-terminal-existing"),
          groupId: groupTwo,
          surfaceKind: "persistentTerminal",
          logicalKey: terminalSessionKey,
        },
      ],
    },
    {
      surfaceKind: "persistentTerminal",
      logicalKey: terminalSessionKey,
      attachmentId: attachmentId("att-terminal-duplicate"),
    },
  ),
  {
    type: "focusExisting",
    attachmentId: "att-terminal-existing",
    groupId: "group-2",
    logicalKey: "persistentTerminal/root-local-abc/term_abc",
  },
  "persistent terminal duplicate logical key focuses existing session pane",
);
assertDeepEqual(
  decideSurfaceOpen(
    {
      groups: [{ groupId: groupOne }, { groupId: groupTwo }],
      focusedGroupId: groupTwo,
      attachments: [],
    },
    {
      surfaceKind: "persistentTerminal",
      logicalKey: terminalSessionKey,
      attachmentId: attachmentId("att-terminal-new"),
    },
  ),
  {
    type: "openNew",
    attachmentId: "att-terminal-new",
    groupId: "group-2",
    logicalKey: "persistentTerminal/root-local-abc/term_abc",
    rowPolicy: "pinned",
  },
  "persistent terminal opens into the focused group",
);
assertDeepEqual(
  decideSurfaceOpenWithDynamicGroups(
    {
      groups: [{ groupId: groupOne }, { groupId: groupTwo }],
      focusedGroupId: groupTwo,
      attachments: [],
    },
    {
      surfaceKind: "persistentTerminal",
      logicalKey: terminalSessionKey,
      attachmentId: attachmentId("att-terminal-new"),
    },
  ),
  {
    type: "openNew",
    attachmentId: "att-terminal-new",
    groupId: "group-1",
    logicalKey: "persistentTerminal/root-local-abc/term_abc",
    rowPolicy: "pinned",
    nextState: {
      groups: [{ groupId: "group-1" }, { groupId: "group-2" }],
      focusedGroupId: "group-2",
      attachments: [
        {
          attachmentId: "att-terminal-new",
          groupId: "group-1",
          surfaceKind: "persistentTerminal",
          logicalKey: "persistentTerminal/root-local-abc/term_abc",
        },
      ],
    },
    createdGroupId: null,
  },
  "dynamic placement sends terminals to group 1 even when group 2 is focused",
);
assertDeepEqual(
  decideSurfaceCloseConfirmation("persistentTerminal"),
  {
    confirmationPolicy: "confirmSessionClose",
    presentation: "cursorNearPopover",
    confirmLabel: "Yes",
    cancelLabel: "No",
  },
  "persistent terminal tab close requires cursor-near confirmation",
);
assertDeepEqual(
  decideSurfaceCloseConfirmation("agent"),
  {
    confirmationPolicy: "confirmSessionClose",
    presentation: "cursorNearPopover",
    confirmLabel: "Yes",
    cancelLabel: "No",
  },
  "agent tab close requires cursor-near confirmation",
);
assertDeepEqual(
  decideSurfaceCloseConfirmation("diagnostics"),
  {
    confirmationPolicy: "none",
    presentation: "none",
    confirmLabel: null,
    cancelLabel: null,
  },
  "reversible diagnostics tabs close immediately",
);
assertDeepEqual(
  decideWorkbenchTabClosePresentation("persistentTerminal", {
    clientX: 320,
    clientY: 80,
  }),
  {
    type: "requestConfirmation",
    confirmation: {
      confirmationPolicy: "confirmSessionClose",
      presentation: "cursorNearPopover",
      confirmLabel: "Yes",
      cancelLabel: "No",
    },
    anchor: { clientX: 320, clientY: 80 },
  },
  "terminal tab close requests a cursor-anchored confirmation presentation",
);
assertDeepEqual(
  decideWorkbenchTabClosePresentation("viewer", { clientX: 12, clientY: 34 }),
  {
    type: "closeImmediately",
    confirmation: {
      confirmationPolicy: "none",
      presentation: "none",
      confirmLabel: null,
      cancelLabel: null,
    },
  },
  "reversible viewer tab close has no confirmation presentation",
);
assertDeepEqual(
  decideSurfaceClose("persistentTerminal").terminateReservation?.commandId,
  "workbench.lifecycle.terminate",
  "persistent terminal close reserves terminate command",
);

assertDeepEqual(
  decideSurfaceClose("agent"),
  {
    closePolicy: "detachDaemonResource",
    behavior: "detach",
    terminateReservation: {
      commandId: "workbench.lifecycle.terminate",
      reserved: true,
      surfaceKind: "agent",
    },
  },
  "daemon-backed close resolves to detach and reserves terminate separately",
);
assertDeepEqual(
  reserveTerminateCommand("persistentTerminal"),
  {
    commandId: "workbench.lifecycle.terminate",
    reserved: true,
    surfaceKind: "persistentTerminal",
  },
  "explicit terminate command reservation is separate from close",
);
assertDeepEqual(
  decideSurfaceClose("inspector"),
  {
    closePolicy: "closeAttachment",
    behavior: "closeAttachment",
    terminateReservation: null,
  },
  "browser-owned inspector closes the attachment without daemon termination",
);

const resizeDecision = preservePtyLogicalSize(defaultPtyLogicalSize, {
  widthPx: 1440,
  heightPx: 640,
});
assertDeepEqual(
  resizeDecision,
  {
    logicalSize: { columns: 80, rows: 24 },
    visualSize: { widthPx: 1440, heightPx: 640 },
    resizeRequest: "deferred",
  },
  "visual split size is recorded without rewriting PTY logical dimensions",
);
assert(
  resizeDecision.logicalSize === defaultPtyLogicalSize,
  "PTY logical size object is preserved",
);
