import {
  buildActivityDetailToggleCommand,
  buildActivityRefreshCommand,
  buildActivitySelectItemCommand,
  buildActivityTranscriptLoadMoreCommand,
  buildDashboardRefreshCommand,
  buildFileExplorerOpenFileCommand,
  buildFileExplorerRefreshCommand,
  buildFileExplorerSelectEntryCommand,
  buildFileExplorerToggleDirectoryCommand,
  buildRootPickerCloseCommand,
  buildRootPickerCreateDirectoryCommand,
  buildRootPickerNavigateCommand,
  buildRootPickerOpenCommand,
  buildRootPickerSelectDirectoryCommand,
  buildTerminalCreateCommand,
  buildWorkbenchOpenActivityCommand,
  buildWorkspaceRemoveCommand,
  buildWorkRootActivationCommand,
  buildWorkRootOpenCommand,
  dashboardCommandLabel,
  dispatchDashboardCommand,
  type DashboardCommand,
} from "./commands.js";

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

function assertNotContains(value: string, forbidden: string, label: string) {
  if (value.includes(forbidden)) {
    throw new Error(`${label}: ${JSON.stringify(value)} contained ${JSON.stringify(forbidden)}`);
  }
}

const workRootId = "workRoot:local";
const filePath = "src/App.tsx";
const migratedCommands = [
  buildDashboardRefreshCommand(),
  buildRootPickerOpenCommand(),
  buildRootPickerNavigateCommand("/private/path"),
  buildRootPickerSelectDirectoryCommand("/private/path"),
  buildRootPickerCreateDirectoryCommand("/private", "path"),
  buildRootPickerCloseCommand(),
  buildFileExplorerRefreshCommand(workRootId),
  buildFileExplorerToggleDirectoryCommand(workRootId, "src"),
  buildFileExplorerOpenFileCommand(workRootId, filePath, "singleClick"),
  buildFileExplorerSelectEntryCommand(workRootId, "README.md"),
  buildWorkbenchOpenActivityCommand(workRootId),
  buildTerminalCreateCommand(workRootId),
  buildWorkspaceRemoveCommand("workspace-local-abc"),
  buildWorkRootActivationCommand(workRootId, "offline"),
  buildActivitySelectItemCommand("agent:reviewer"),
  buildActivityTranscriptLoadMoreCommand("agent:reviewer"),
  buildActivityRefreshCommand(workRootId),
  buildActivityDetailToggleCommand("agent:reviewer", "block:1"),
] as const;

assertDeepEqual(
  migratedCommands.map((command) => command.commandId),
  [
    "dashboard.refresh",
    "rootPicker.open",
    "rootPicker.navigate",
    "rootPicker.selectDirectory",
    "rootPicker.createDirectory",
    "rootPicker.close",
    "fileExplorer.refresh",
    "fileExplorer.toggleDirectory",
    "fileExplorer.openFile",
    "fileExplorer.selectEntry",
    "workbench.openActivity",
    "terminal.create",
    "workspace.remove",
    "workRoot.activation.set",
    "activity.selectItem",
    "activity.transcript.loadMore",
    "activity.refresh",
    "activity.detail.toggle",
  ],
  "real command builders preserve migrated command ids",
);

assertDeepEqual(
  migratedCommands.map((command) => command.payload.type),
  [
    "refresh",
    "rootPicker.open",
    "rootPicker.navigate",
    "rootPicker.selectDirectory",
    "rootPicker.createDirectory",
    "rootPicker.close",
    "fileExplorer.refresh",
    "fileExplorer.toggleDirectory",
    "fileExplorer.openFile",
    "fileExplorer.selectEntry",
    "workbench.openActivity",
    "terminal.create",
    "workspace.remove",
    "workRoot.activation.set",
    "activity.selectItem",
    "activity.transcript.loadMore",
    "activity.refresh",
    "activity.detail.toggle",
  ],
  "real command builders emit executable payload variants",
);

const observed: string[] = [];
const executed: string[] = [];
for (const command of migratedCommands) {
  dispatchDashboardCommand(command, {
    observer: (observedCommand) => observed.push(observedCommand.commandId),
    handlers: {
      [command.commandId]: (handledCommand: DashboardCommand) => {
        executed.push(`${handledCommand.commandId}:${handledCommand.payload.type}`);
      },
    },
  });
}

assertDeepEqual(
  observed,
  migratedCommands.map((command) => command.commandId),
  "real migrated builders pass through command observer in order",
);
assertDeepEqual(
  executed,
  migratedCommands.map((command) => `${command.commandId}:${command.payload.type}`),
  "programmatic dispatch executes handlers keyed by real migrated command ids",
);

const openFileCommand: DashboardCommand = buildFileExplorerOpenFileCommand(
  workRootId,
  filePath,
  "singleClick",
);
assertEqual(dashboardCommandLabel(openFileCommand), "Open file", "open file label is stable");

const submittedHostPath = "/Users/kang-sw/private/customer repo";
const workRootOpenCommand = buildWorkRootOpenCommand(submittedHostPath);
let loggableWorkRootOpenCommand: DashboardCommand | null = null;
dispatchDashboardCommand(workRootOpenCommand, {
  observer: (command) => {
    loggableWorkRootOpenCommand = command;
  },
  handlers: {
    "workRoot.open": (command) => {
      assertEqual(command.payload.type, "workRoot.open", "workRoot open handler receives logical payload");
    },
  },
});

assertEqual(workRootOpenCommand.commandId, "workRoot.open", "submitted host path builds workRoot.open command");
assertEqual(workRootOpenCommand.payload.type, "workRoot.open", "workRoot.open payload stays logical");
const serializedPayload = JSON.stringify(workRootOpenCommand.payload);
const serializedLoggableCommand = JSON.stringify(loggableWorkRootOpenCommand);
assertNotContains(serializedPayload, submittedHostPath, "workRoot.open payload omits submitted host path");
assertNotContains(
  serializedLoggableCommand,
  submittedHostPath,
  "workRoot.open loggable command omits submitted host path",
);
assertEqual(
  Object.prototype.hasOwnProperty.call(workRootOpenCommand.payload, "path"),
  false,
  "workRoot.open command payload does not carry a path field",
);

const rootPickerPrivatePath = "/Users/kang-sw/private/root";
const rootPickerCommands = [
  buildRootPickerNavigateCommand(rootPickerPrivatePath),
  buildRootPickerSelectDirectoryCommand(rootPickerPrivatePath),
  buildRootPickerCreateDirectoryCommand(rootPickerPrivatePath, "child"),
];
for (const command of rootPickerCommands) {
  assertNotContains(
    JSON.stringify(command),
    rootPickerPrivatePath,
    `${command.commandId} command omits host paths`,
  );
}
assertEqual(
  dashboardCommandLabel(buildRootPickerOpenCommand()),
  "Open root picker",
  "root picker open label is stable",
);
assertEqual(
  dashboardCommandLabel(buildRootPickerCloseCommand()),
  "Close root picker",
  "root picker close label is stable",
);
assertEqual(
  dashboardCommandLabel(buildRootPickerCreateDirectoryCommand(rootPickerPrivatePath, "child")),
  "Create directory",
  "root picker create-directory label is stable",
);

let terminalCreates = 0;
dispatchDashboardCommand(buildTerminalCreateCommand(workRootId), {
  handlers: {
    "terminal.create": (command) => {
      if (command.payload.type !== "terminal.create") {
        throw new Error("terminal handler received wrong payload");
      }
      terminalCreates += command.payload.workRootId === workRootId ? 1 : 0;
    },
  },
});
assertEqual(terminalCreates, 1, "programmatic terminal.create dispatch reaches executable handler");
let activationChanges = 0;
dispatchDashboardCommand(buildWorkRootActivationCommand(workRootId, "online"), {
  handlers: {
    "workRoot.activation.set": (command) => {
      if (command.payload.type !== "workRoot.activation.set") {
        throw new Error("activation handler received wrong payload");
      }
      activationChanges +=
        command.payload.workRootId === workRootId && command.payload.activation === "online"
          ? 1
          : 0;
    },
  },
});
assertEqual(
  activationChanges,
  1,
  "programmatic workRoot.activation.set dispatch reaches executable handler",
);
assertEqual(
  dashboardCommandLabel(buildWorkRootActivationCommand(workRootId, "offline")),
  "Take workRoot offline",
  "activation command label is stable",
);
assertEqual(
  dashboardCommandLabel(buildWorkspaceRemoveCommand("workspace-local-abc")),
  "Remove workspace",
  "workspace remove command label is stable",
);
const workspaceRemoveCommand = buildWorkspaceRemoveCommand("workspace-local-abc");
assertEqual(
  workspaceRemoveCommand.payload.type === "workspace.remove" &&
    workspaceRemoveCommand.payload.workspaceId,
  "workspace-local-abc",
  "workspace remove command carries only the opaque workspace id",
);
assertNotContains(
  JSON.stringify(workspaceRemoveCommand),
  "/Users/",
  "workspace remove command omits host paths",
);
assertEqual(
  dashboardCommandLabel(buildWorkbenchOpenActivityCommand(workRootId)),
  "Open WorkRoot Activity",
  "activity command label leaves an obvious later command surface",
);

assertEqual(
  dashboardCommandLabel(buildActivitySelectItemCommand("agent:reviewer")),
  "Select activity",
  "activity selection command label is stable",
);
assertEqual(
  dashboardCommandLabel(buildActivityTranscriptLoadMoreCommand("agent:reviewer")),
  "Load transcript",
  "activity transcript load-more command label is stable",
);
assertEqual(
  dashboardCommandLabel(buildActivityRefreshCommand(workRootId)),
  "Refresh",
  "activity refresh command label reuses refresh wording",
);
assertEqual(
  dashboardCommandLabel(buildActivityDetailToggleCommand("agent:reviewer", "block:1")),
  "Toggle detail",
  "activity detail toggle command label is stable",
);
