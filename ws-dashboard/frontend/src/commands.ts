import { LOCAL_DASHBOARD_SERVER_ROUTE } from "./resourceModel.js";
export type DashboardCommandId =
  | "dashboard.refresh"
  | "workspace.menu.open"
  | "workspace.remove"
  | "gitWorktreeAdd.open"
  | "gitWorktreeAdd.close"
  | "gitWorktreeAdd.submit"
  | "git.refresh"
  | "git.fetch"
  | "git.push"
  | "git.pullFfOnly"
  | "git.branchMenu.open"
  | "git.branch.switch"
  | "git.branchCreate.open"
  | "git.branchCreate.submit"
  | "git.branchCreate.close"
  | "workRoot.open"
  | "workRoot.activation.set"
  | "rootPicker.open"
  | "rootPicker.close"
  | "rootPicker.navigate"
  | "rootPicker.selectDirectory"
  | "rootPicker.createDirectory"
  | "rootPicker.pinDirectory"
  | "rootPicker.unpinDirectory"
  | "fileExplorer.refresh"
  | "fileExplorer.toggleDirectory"
  | "fileExplorer.openFile"
  | "fileExplorer.selectEntry"
  | "workbench.openActivity"
  | "terminal.create"
  | "resource.select"
  | "activity.selectItem"
  | "activity.transcript.loadMore"
  | "activity.refresh"
  | "activity.detail.toggle"
  | "document.translation.toggle"
  | "document.mode.set"
  | "document.save"
  | "document.revert"
  | `resource.action.${string}`
  | `workbench.toggle.${string}`
  | `workbench.tab.${string}`;

export type DashboardCommandPayload =
  | { type: "refresh" }
  | { type: "select"; entityId: string }
  | { type: "action"; label: string; entityId: string }
  | { type: "workspace.menu.open"; workspaceId: string }
  | { type: "workspace.remove"; workspaceId: string }
  | { type: "gitWorktreeAdd.open"; workspaceId: string }
  | { type: "gitWorktreeAdd.close"; workspaceId: string }
  | { type: "gitWorktreeAdd.submit"; workspaceId: string }
  | { type: "git.refresh"; workRootId: string }
  | { type: "git.fetch"; workRootId: string }
  | { type: "git.push"; workRootId: string }
  | { type: "git.pullFfOnly"; workRootId: string }
  | { type: "git.branchMenu.open"; workRootId: string }
  | { type: "git.branch.switch"; workRootId: string; branchName: string }
  | { type: "git.branchCreate.open"; workRootId: string }
  | {
      type: "git.branchCreate.submit";
      workRootId: string;
      branchName: string;
      baseBranch?: string;
    }
  | { type: "git.branchCreate.close"; workRootId: string }
  | { type: "workRoot.open" }
  | {
      type: "workRoot.activation.set";
      workRootId: string;
      activation: "online" | "offline";
    }
  | { type: "rootPicker.open" }
  | { type: "rootPicker.close" }
  | { type: "rootPicker.navigate" }
  | { type: "rootPicker.selectDirectory" }
  | { type: "rootPicker.createDirectory" }
  | { type: "rootPicker.pinDirectory" }
  | { type: "rootPicker.unpinDirectory" }
  | { type: "fileExplorer.refresh"; workRootId: string }
  | { type: "fileExplorer.toggleDirectory"; workRootId: string; path: string }
  | {
      type: "fileExplorer.openFile";
      workRootId: string;
      path: string;
      gesture: "singleClick" | "doubleClick";
    }
  | { type: "fileExplorer.selectEntry"; workRootId: string; path: string }
  | { type: "workbench.openActivity"; workRootId: string }
  | { type: "terminal.create"; workRootId: string }
  | { type: "activity.selectItem"; activityId: string }
  | { type: "activity.transcript.loadMore"; activityId: string }
  | { type: "activity.refresh"; workRootId: string }
  | { type: "activity.detail.toggle"; activityId: string; detailKey: string }
  | { type: "document.translation.toggle"; workRootId: string; path: string }
  | {
      type: "document.mode.set";
      workRootId: string;
      path: string;
      mode: "view" | "edit";
    }
  | { type: "document.save"; workRootId: string; path: string }
  | { type: "document.revert"; workRootId: string; path: string };

export type DashboardCommand = {
  commandId: DashboardCommandId;
  payload: DashboardCommandPayload & { readonly serverRoute?: string };
};

export type DashboardCommandEntry = {
  id: number;
  commandId: string;
  label: string;
};

export type DashboardCommandHandler = (command: DashboardCommand) => void;
export type DashboardCommandHandlers = Partial<
  Record<DashboardCommandId, DashboardCommandHandler>
>;
export type DashboardCommandObserver = (command: DashboardCommand) => void;

export type DashboardCommandDispatcher = (
  command: DashboardCommand,
  handlers?: DashboardCommandHandlers,
) => void;

export function buildDashboardRefreshCommand(): DashboardCommand {
  return { commandId: "dashboard.refresh", payload: { type: "refresh" } };
}

export function buildWorkRootOpenCommand(
  _submittedHostPath: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "workRoot.open",
    payload: { type: "workRoot.open", serverRoute },
  };
}

export function buildRootPickerOpenCommand(
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "rootPicker.open",
    payload: { type: "rootPicker.open", serverRoute },
  };
}

export function buildRootPickerCloseCommand(
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "rootPicker.close",
    payload: { type: "rootPicker.close", serverRoute },
  };
}

export function buildRootPickerNavigateCommand(
  _targetPath: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "rootPicker.navigate",
    payload: { type: "rootPicker.navigate", serverRoute },
  };
}

export function buildRootPickerSelectDirectoryCommand(
  _targetPath: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "rootPicker.selectDirectory",
    payload: { type: "rootPicker.selectDirectory", serverRoute },
  };
}

export function buildRootPickerCreateDirectoryCommand(
  _parentPath: string,
  _name: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "rootPicker.createDirectory",
    payload: { type: "rootPicker.createDirectory", serverRoute },
  };
}

export function buildRootPickerPinDirectoryCommand(
  _path: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "rootPicker.pinDirectory",
    payload: { type: "rootPicker.pinDirectory", serverRoute },
  };
}

export function buildRootPickerUnpinDirectoryCommand(
  _path: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "rootPicker.unpinDirectory",
    payload: { type: "rootPicker.unpinDirectory", serverRoute },
  };
}

export function buildWorkspaceMenuOpenCommand(
  workspaceId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "workspace.menu.open",
    payload: { type: "workspace.menu.open", serverRoute, workspaceId },
  };
}

export function buildGitWorktreeAddOpenCommand(
  workspaceId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "gitWorktreeAdd.open",
    payload: { type: "gitWorktreeAdd.open", serverRoute, workspaceId },
  };
}

export function buildGitWorktreeAddCloseCommand(
  workspaceId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "gitWorktreeAdd.close",
    payload: { type: "gitWorktreeAdd.close", serverRoute, workspaceId },
  };
}

export function buildGitWorktreeAddSubmitCommand(
  workspaceId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "gitWorktreeAdd.submit",
    payload: { type: "gitWorktreeAdd.submit", serverRoute, workspaceId },
  };
}

export function buildGitRefreshCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.refresh",
    payload: { type: "git.refresh", serverRoute, workRootId },
  };
}
export function buildGitFetchCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.fetch",
    payload: { type: "git.fetch", serverRoute, workRootId },
  };
}
export function buildGitPushCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.push",
    payload: { type: "git.push", serverRoute, workRootId },
  };
}
export function buildGitPullFfOnlyCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.pullFfOnly",
    payload: { type: "git.pullFfOnly", serverRoute, workRootId },
  };
}
export function buildGitBranchMenuOpenCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.branchMenu.open",
    payload: { type: "git.branchMenu.open", serverRoute, workRootId },
  };
}
export function buildGitBranchSwitchCommand(
  workRootId: string,
  branchName: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.branch.switch",
    payload: { type: "git.branch.switch", serverRoute, workRootId, branchName },
  };
}
export function buildGitBranchCreateOpenCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.branchCreate.open",
    payload: { type: "git.branchCreate.open", serverRoute, workRootId },
  };
}
export function buildGitBranchCreateSubmitCommand(
  workRootId: string,
  branchName: string,
  baseBranch?: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.branchCreate.submit",
    payload: {
      type: "git.branchCreate.submit",
      serverRoute,
      workRootId,
      branchName,
      baseBranch,
    },
  };
}
export function buildGitBranchCreateCloseCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "git.branchCreate.close",
    payload: { type: "git.branchCreate.close", serverRoute, workRootId },
  };
}

export function buildWorkspaceRemoveCommand(
  workspaceId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "workspace.remove",
    payload: { type: "workspace.remove", serverRoute, workspaceId },
  };
}

export function buildWorkRootActivationCommand(
  workRootId: string,
  activation: "online" | "offline",
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "workRoot.activation.set",
    payload: {
      type: "workRoot.activation.set",
      serverRoute,
      workRootId,
      activation,
    },
  };
}

export function buildFileExplorerRefreshCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "fileExplorer.refresh",
    payload: { type: "fileExplorer.refresh", serverRoute, workRootId },
  };
}

export function buildFileExplorerToggleDirectoryCommand(
  workRootId: string,
  path: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "fileExplorer.toggleDirectory",
    payload: {
      type: "fileExplorer.toggleDirectory",
      serverRoute,
      workRootId,
      path,
    },
  };
}

export function buildFileExplorerOpenFileCommand(
  workRootId: string,
  path: string,
  gesture: "singleClick" | "doubleClick",
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "fileExplorer.openFile",
    payload: {
      type: "fileExplorer.openFile",
      serverRoute,
      workRootId,
      path,
      gesture,
    },
  };
}

export function buildFileExplorerSelectEntryCommand(
  workRootId: string,
  path: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "fileExplorer.selectEntry",
    payload: { type: "fileExplorer.selectEntry", serverRoute, workRootId, path },
  };
}

export function buildWorkbenchOpenActivityCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "workbench.openActivity",
    payload: { type: "workbench.openActivity", serverRoute, workRootId },
  };
}

export function buildTerminalCreateCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "terminal.create",
    payload: { type: "terminal.create", serverRoute, workRootId },
  };
}

export function buildActivitySelectItemCommand(
  activityId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "activity.selectItem",
    payload: { type: "activity.selectItem", serverRoute, activityId },
  };
}

export function buildActivityTranscriptLoadMoreCommand(
  activityId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "activity.transcript.loadMore",
    payload: { type: "activity.transcript.loadMore", serverRoute, activityId },
  };
}

export function buildActivityRefreshCommand(
  workRootId: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "activity.refresh",
    payload: { type: "activity.refresh", serverRoute, workRootId },
  };
}

export function buildActivityDetailToggleCommand(
  activityId: string,
  detailKey: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "activity.detail.toggle",
    payload: {
      type: "activity.detail.toggle",
      serverRoute,
      activityId,
      detailKey,
    },
  };
}

export function buildDocumentTranslationToggleCommand(
  workRootId: string,
  path: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "document.translation.toggle",
    payload: {
      type: "document.translation.toggle",
      serverRoute,
      workRootId,
      path,
    },
  };
}

export function buildDocumentModeSetCommand(
  workRootId: string,
  path: string,
  mode: "view" | "edit",
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "document.mode.set",
    payload: { type: "document.mode.set", serverRoute, workRootId, path, mode },
  };
}

export function buildDocumentSaveCommand(
  workRootId: string,
  path: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "document.save",
    payload: { type: "document.save", serverRoute, workRootId, path },
  };
}

export function buildDocumentRevertCommand(
  workRootId: string,
  path: string,
  serverRoute: string = LOCAL_DASHBOARD_SERVER_ROUTE,
): DashboardCommand {
  return {
    commandId: "document.revert",
    payload: { type: "document.revert", serverRoute, workRootId, path },
  };
}

export function dispatchDashboardCommand(
  command: DashboardCommand,
  options: {
    handlers?: DashboardCommandHandlers;
    observer?: DashboardCommandObserver;
  } = {},
) {
  options.observer?.(command);
  options.handlers?.[command.commandId]?.(command);
}

export function dashboardCommandLabel(command: DashboardCommand): string {
  const { payload } = command;
  switch (payload.type) {
    case "refresh":
    case "fileExplorer.refresh":
    case "activity.refresh":
      return "Refresh";
    case "select":
      return "Select";
    case "workspace.menu.open":
      return "Open workspace menu";
    case "workspace.remove":
      return "Remove workspace";
    case "gitWorktreeAdd.open":
      return "Add worktree";
    case "gitWorktreeAdd.close":
      return "Close add worktree";
    case "gitWorktreeAdd.submit":
      return "Create worktree";
    case "git.refresh":
      return "Refresh Git status";
    case "git.fetch":
      return "Fetch Git";
    case "git.push":
      return "Push Git";
    case "git.pullFfOnly":
      return "Pull Git ff-only";
    case "git.branchMenu.open":
      return "Open branch menu";
    case "git.branch.switch":
      return "Switch branch";
    case "git.branchCreate.open":
      return "Open new branch";
    case "git.branchCreate.submit":
      return "Create branch";
    case "git.branchCreate.close":
      return "Close new branch";
    case "workRoot.open":
      return "Open workRoot";
    case "workRoot.activation.set":
      return payload.activation === "online"
        ? "Bring workRoot online"
        : "Take workRoot offline";
    case "rootPicker.open":
      return "Open root picker";
    case "rootPicker.close":
      return "Close root picker";
    case "rootPicker.navigate":
      return "Navigate root picker";
    case "rootPicker.selectDirectory":
      return "Select directory";
    case "rootPicker.createDirectory":
      return "Create directory";
    case "rootPicker.pinDirectory":
      return "Pin directory";
    case "rootPicker.unpinDirectory":
      return "Unpin directory";
    case "fileExplorer.toggleDirectory":
      return "Toggle directory";
    case "fileExplorer.openFile":
      return payload.gesture === "doubleClick"
        ? "Open pinned file"
        : "Open file";
    case "fileExplorer.selectEntry":
      return "Select file entry";
    case "workbench.openActivity":
      return "Open WorkRoot Activity";
    case "terminal.create":
      return "Create terminal";
    case "activity.selectItem":
      return "Select activity";
    case "activity.transcript.loadMore":
      return "Load transcript";
    case "activity.detail.toggle":
      return "Toggle detail";
    case "document.translation.toggle":
      return "Toggle translation";
    case "document.mode.set":
      return payload.mode === "edit" ? "Edit document" : "View document";
    case "document.save":
      return "Save document";
    case "document.revert":
      return "Revert document";
    case "action":
      return payload.label;
  }
}
