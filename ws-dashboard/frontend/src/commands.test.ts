import {
  buildActivityDetailToggleCommand,
  buildActivityRefreshCommand,
  buildActivitySelectItemCommand,
  buildActivityTranscriptLoadMoreCommand,
  buildDashboardRefreshCommand,
  buildDocumentModeSetCommand,
  buildDocumentRevertCommand,
  buildDocumentSaveCommand,
  buildFileExplorerOpenFileCommand,
  buildFileExplorerRefreshCommand,
  buildFileExplorerSelectEntryCommand,
  buildFileExplorerToggleDirectoryCommand,
  buildGitWorktreeAddCloseCommand,
  buildGitWorktreeAddOpenCommand,
  buildGitWorktreeAddSubmitCommand,
  buildGitBranchCreateCloseCommand,
  buildGitBranchCreateOpenCommand,
  buildGitBranchCreateSubmitCommand,
  buildGitBranchMenuOpenCommand,
  buildGitBranchSwitchCommand,
  buildGitFetchCommand,
  buildGitPullFfOnlyCommand,
  buildGitPushCommand,
  buildGitRefreshCommand,
  buildRootPickerCloseCommand,
  buildRootPickerCreateDirectoryCommand,
  buildRootPickerNavigateCommand,
  buildRootPickerOpenCommand,
  buildRootPickerPinDirectoryCommand,
  buildRootPickerSelectDirectoryCommand,
  buildRootPickerUnpinDirectoryCommand,
  buildTerminalCreateCommand,
  buildWorkspaceMenuOpenCommand,
  buildWorkbenchOpenActivityCommand,
  buildWorkspaceRemoveCommand,
  buildWorkRootActivationCommand,
  buildWorkRootCloseCommand,
  buildWorkRootOpenCommand,
  dashboardCommandLabel,
  dispatchDashboardCommand,
  type DashboardCommand,
} from "./commands.js";

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

function assertNotContains(value: string, forbidden: string, label: string) {
  if (value.includes(forbidden)) {
    throw new Error(
      `${label}: ${JSON.stringify(value)} contained ${JSON.stringify(forbidden)}`,
    );
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
  buildRootPickerPinDirectoryCommand("/private/path"),
  buildRootPickerUnpinDirectoryCommand("/private/path"),
  buildRootPickerCloseCommand(),
  buildFileExplorerRefreshCommand(workRootId),
  buildFileExplorerToggleDirectoryCommand(workRootId, "src"),
  buildFileExplorerOpenFileCommand(workRootId, filePath, "singleClick"),
  buildFileExplorerSelectEntryCommand(workRootId, "README.md"),
  buildWorkbenchOpenActivityCommand(workRootId),
  buildTerminalCreateCommand(workRootId),
  buildWorkspaceMenuOpenCommand("workspace-local-abc"),
  buildWorkspaceRemoveCommand("workspace-local-abc"),
  buildWorkRootCloseCommand(workRootId),
  buildGitWorktreeAddOpenCommand("workspace-local-abc"),
  buildGitWorktreeAddCloseCommand("workspace-local-abc"),
  buildGitWorktreeAddSubmitCommand("workspace-local-abc"),
  buildWorkRootActivationCommand(workRootId, "offline"),
  buildGitRefreshCommand(workRootId),
  buildGitFetchCommand(workRootId),
  buildGitPushCommand(workRootId),
  buildGitPullFfOnlyCommand(workRootId),
  buildGitBranchMenuOpenCommand(workRootId),
  buildGitBranchSwitchCommand(workRootId, "feature/private"),
  buildGitBranchCreateOpenCommand(workRootId),
  buildGitBranchCreateSubmitCommand(workRootId, "new-private", "main"),
  buildGitBranchCreateCloseCommand(workRootId),
  buildActivitySelectItemCommand("agent:reviewer"),
  buildActivityTranscriptLoadMoreCommand("agent:reviewer"),
  buildActivityRefreshCommand(workRootId),
  buildActivityDetailToggleCommand("agent:reviewer", "block:1"),
  buildDocumentModeSetCommand(workRootId, filePath, "edit"),
  buildDocumentSaveCommand(workRootId, filePath),
  buildDocumentRevertCommand(workRootId, filePath),
] as const;

assertDeepEqual(
  migratedCommands.map((command) => command.commandId),
  [
    "dashboard.refresh",
    "rootPicker.open",
    "rootPicker.navigate",
    "rootPicker.selectDirectory",
    "rootPicker.createDirectory",
    "rootPicker.pinDirectory",
    "rootPicker.unpinDirectory",
    "rootPicker.close",
    "fileExplorer.refresh",
    "fileExplorer.toggleDirectory",
    "fileExplorer.openFile",
    "fileExplorer.selectEntry",
    "workbench.openActivity",
    "terminal.create",
    "workspace.menu.open",
    "workspace.remove",
    "workRoot.close",
    "gitWorktreeAdd.open",
    "gitWorktreeAdd.close",
    "gitWorktreeAdd.submit",
    "workRoot.activation.set",
    "git.refresh",
    "git.fetch",
    "git.push",
    "git.pullFfOnly",
    "git.branchMenu.open",
    "git.branch.switch",
    "git.branchCreate.open",
    "git.branchCreate.submit",
    "git.branchCreate.close",
    "activity.selectItem",
    "activity.transcript.loadMore",
    "activity.refresh",
    "activity.detail.toggle",
    "document.mode.set",
    "document.save",
    "document.revert",
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
    "rootPicker.pinDirectory",
    "rootPicker.unpinDirectory",
    "rootPicker.close",
    "fileExplorer.refresh",
    "fileExplorer.toggleDirectory",
    "fileExplorer.openFile",
    "fileExplorer.selectEntry",
    "workbench.openActivity",
    "terminal.create",
    "workspace.menu.open",
    "workspace.remove",
    "workRoot.close",
    "gitWorktreeAdd.open",
    "gitWorktreeAdd.close",
    "gitWorktreeAdd.submit",
    "workRoot.activation.set",
    "git.refresh",
    "git.fetch",
    "git.push",
    "git.pullFfOnly",
    "git.branchMenu.open",
    "git.branch.switch",
    "git.branchCreate.open",
    "git.branchCreate.submit",
    "git.branchCreate.close",
    "activity.selectItem",
    "activity.transcript.loadMore",
    "activity.refresh",
    "activity.detail.toggle",
    "document.mode.set",
    "document.save",
    "document.revert",
  ],
  "real command builders emit executable payload variants",
);

const remoteFileCommand = buildFileExplorerOpenFileCommand(
  "root-same",
  "src/App.tsx",
  "singleClick",
  "server-a",
);
const otherRemoteFileCommand = buildFileExplorerOpenFileCommand(
  "root-same",
  "src/App.tsx",
  "singleClick",
  "server-b",
);
assertEqual(
  remoteFileCommand.payload.serverRoute,
  "server-a",
  "file command payload carries server id for execution routing",
);
assertEqual(
  JSON.stringify(remoteFileCommand.payload) ===
    JSON.stringify(otherRemoteFileCommand.payload),
  false,
  "same bare file command identity does not collapse across servers",
);
assertEqual(
  buildTerminalCreateCommand("root-same", "server-a").payload.serverRoute,
  "server-a",
  "terminal command payload carries server id",
);
assertEqual(
  buildActivitySelectItemCommand("activity-same", "server-a").payload.serverRoute,
  "server-a",
  "activity command payload carries server id",
);
assertEqual(
  buildWorkRootOpenCommand("/private/path", "server-a").payload.serverRoute,
  "server-a",
  "open workRoot command payload keeps server id while omitting host path",
);
assertEqual(
  buildGitWorktreeAddOpenCommand("workspace-same", "server-remote").payload
    .serverRoute,
  "server-remote",
  "git worktree open command carries remote server route",
);
assertEqual(
  buildGitWorktreeAddSubmitCommand("workspace-same", "server-remote").payload
    .serverRoute,
  "server-remote",
  "git worktree submit command carries remote server route",
);
assertEqual(
  buildWorkRootCloseCommand("root-same", "server-remote").payload.serverRoute,
  "server-remote",
  "workRoot close command carries remote server route",
);
assertEqual(
  buildWorkspaceRemoveCommand("workspace-same", "server-remote").payload
    .serverRoute,
  "server-remote",
  "workspace remove command carries remote server route",
);

const branchCreateCommand = buildGitBranchCreateSubmitCommand(
  workRootId,
  "new-private",
  "main",
);
if (branchCreateCommand.payload.type !== "git.branchCreate.submit") {
  throw new Error("branch create command payload type mismatch");
}
assertEqual(
  branchCreateCommand.payload.baseBranch,
  "main",
  "branch create command records selected base branch",
);

const observed: string[] = [];
const executed: string[] = [];
for (const command of migratedCommands) {
  dispatchDashboardCommand(command, {
    observer: (observedCommand) => observed.push(observedCommand.commandId),
    handlers: {
      [command.commandId]: (handledCommand: DashboardCommand) => {
        executed.push(
          `${handledCommand.commandId}:${handledCommand.payload.type}`,
        );
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
  migratedCommands.map(
    (command) => `${command.commandId}:${command.payload.type}`,
  ),
  "programmatic dispatch executes handlers keyed by real migrated command ids",
);

const openFileCommand: DashboardCommand = buildFileExplorerOpenFileCommand(
  workRootId,
  filePath,
  "singleClick",
);
assertEqual(
  dashboardCommandLabel(openFileCommand),
  "Open file",
  "open file label is stable",
);

const submittedHostPath = "/Users/kang-sw/private/customer repo";
const workRootOpenCommand = buildWorkRootOpenCommand(submittedHostPath);
let loggableWorkRootOpenCommand: DashboardCommand | null = null;
dispatchDashboardCommand(workRootOpenCommand, {
  observer: (command) => {
    loggableWorkRootOpenCommand = command;
  },
  handlers: {
    "workRoot.open": (command) => {
      assertEqual(
        command.payload.type,
        "workRoot.open",
        "workRoot open handler receives logical payload",
      );
    },
  },
});

assertEqual(
  workRootOpenCommand.commandId,
  "workRoot.open",
  "submitted host path builds workRoot.open command",
);
assertEqual(
  workRootOpenCommand.payload.type,
  "workRoot.open",
  "workRoot.open payload stays logical",
);
const serializedPayload = JSON.stringify(workRootOpenCommand.payload);
const serializedLoggableCommand = JSON.stringify(loggableWorkRootOpenCommand);
assertNotContains(
  serializedPayload,
  submittedHostPath,
  "workRoot.open payload omits submitted host path",
);
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
  buildRootPickerPinDirectoryCommand(rootPickerPrivatePath),
  buildRootPickerUnpinDirectoryCommand(rootPickerPrivatePath),
];
for (const command of rootPickerCommands) {
  assertNotContains(
    JSON.stringify(command),
    rootPickerPrivatePath,
    `${command.commandId} command omits host paths`,
  );
}

const remoteRootPickerCommands = [
  buildRootPickerOpenCommand("server-a"),
  buildRootPickerNavigateCommand(rootPickerPrivatePath, "server-a"),
  buildRootPickerSelectDirectoryCommand(rootPickerPrivatePath, "server-a"),
  buildRootPickerCreateDirectoryCommand(
    rootPickerPrivatePath,
    "child",
    "server-a",
  ),
  buildRootPickerPinDirectoryCommand(rootPickerPrivatePath, "server-a"),
  buildRootPickerUnpinDirectoryCommand(rootPickerPrivatePath, "server-a"),
  buildRootPickerCloseCommand("server-a"),
  buildWorkRootOpenCommand(rootPickerPrivatePath, "server-a"),
];
for (const command of remoteRootPickerCommands) {
  assertEqual(
    command.payload.serverRoute,
    "server-a",
    `${command.commandId} command carries remote server route`,
  );
  assertNotContains(
    JSON.stringify(command),
    rootPickerPrivatePath,
    `${command.commandId} command keeps host path out of logical payload`,
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
  dashboardCommandLabel(
    buildRootPickerCreateDirectoryCommand(rootPickerPrivatePath, "child"),
  ),
  "Create directory",
  "root picker create-directory label is stable",
);
assertEqual(
  dashboardCommandLabel(
    buildRootPickerPinDirectoryCommand(rootPickerPrivatePath),
  ),
  "Pin directory",
  "root picker pin-directory label is stable",
);
assertEqual(
  dashboardCommandLabel(
    buildRootPickerUnpinDirectoryCommand(rootPickerPrivatePath),
  ),
  "Unpin directory",
  "root picker unpin-directory label is stable",
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
assertEqual(
  terminalCreates,
  1,
  "programmatic terminal.create dispatch reaches executable handler",
);
let activationChanges = 0;
dispatchDashboardCommand(buildWorkRootActivationCommand(workRootId, "online"), {
  handlers: {
    "workRoot.activation.set": (command) => {
      if (command.payload.type !== "workRoot.activation.set") {
        throw new Error("activation handler received wrong payload");
      }
      activationChanges +=
        command.payload.workRootId === workRootId &&
        command.payload.activation === "online"
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
const workspaceRemoveCommand = buildWorkspaceRemoveCommand(
  "workspace-local-abc",
);
assertEqual(
  workspaceRemoveCommand.payload.type === "workspace.remove" &&
    workspaceRemoveCommand.payload.workspaceId,
  "workspace-local-abc",
  "workspace remove command carries the opaque workspace id",
);
assertEqual(
  workspaceRemoveCommand.payload.serverRoute,
  "server-local",
  "workspace remove command defaults to the local server route",
);
assertNotContains(
  JSON.stringify(workspaceRemoveCommand),
  "/Users/",
  "workspace remove command omits host paths",
);
assertEqual(
  dashboardCommandLabel(buildWorkRootCloseCommand(workRootId)),
  "Close work root",
  "workRoot close command label is stable",
);
const workRootCloseCommand = buildWorkRootCloseCommand(workRootId);
assertEqual(
  workRootCloseCommand.payload.type === "workRoot.close" &&
    workRootCloseCommand.payload.workRootId,
  workRootId,
  "workRoot close command carries the work root id",
);
assertEqual(
  workRootCloseCommand.payload.serverRoute,
  "server-local",
  "workRoot close command defaults to the local server route",
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
  dashboardCommandLabel(
    buildActivityTranscriptLoadMoreCommand("agent:reviewer"),
  ),
  "Load transcript",
  "activity transcript load-more command label is stable",
);
assertEqual(
  dashboardCommandLabel(buildActivityRefreshCommand(workRootId)),
  "Refresh",
  "activity refresh command label reuses refresh wording",
);
assertEqual(
  dashboardCommandLabel(
    buildActivityDetailToggleCommand("agent:reviewer", "block:1"),
  ),
  "Toggle detail",
  "activity detail toggle command label is stable",
);

assertEqual(
  dashboardCommandLabel(
    buildDocumentModeSetCommand(workRootId, filePath, "edit"),
  ),
  "Edit document",
  "document edit command label is stable",
);
assertEqual(
  dashboardCommandLabel(
    buildDocumentModeSetCommand(workRootId, filePath, "view"),
  ),
  "View document",
  "document view command label is stable",
);
assertEqual(
  dashboardCommandLabel(buildDocumentSaveCommand(workRootId, filePath)),
  "Save document",
  "document save command label is stable",
);
assertEqual(
  dashboardCommandLabel(buildDocumentRevertCommand(workRootId, filePath)),
  "Revert document",
  "document revert command label is stable",
);

const gitPrivateRootPath = "/Users/kang-sw/private/git-root";
const gitCommands = [
  buildGitRefreshCommand(workRootId),
  buildGitFetchCommand(workRootId),
  buildGitPushCommand(workRootId),
  buildGitPullFfOnlyCommand(workRootId),
  buildGitBranchMenuOpenCommand(workRootId),
  buildGitBranchSwitchCommand(workRootId, "feature/private"),
  buildGitBranchCreateOpenCommand(workRootId),
  buildGitBranchCreateSubmitCommand(workRootId, "new-private"),
  buildGitBranchCreateCloseCommand(workRootId),
];
for (const command of gitCommands) {
  assertNotContains(
    JSON.stringify(command),
    gitPrivateRootPath,
    `${command.commandId} omits host paths`,
  );
}
assertEqual(
  dashboardCommandLabel(buildGitPullFfOnlyCommand(workRootId)),
  "Pull Git ff-only",
  "safe pull label is stable",
);
