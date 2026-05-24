export type DashboardCommandId =
  | "dashboard.refresh"
  | "workspace.remove"
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
  | `resource.action.${string}`
  | `workbench.toggle.${string}`
  | `workbench.tab.${string}`;

export type DashboardCommandPayload =
  | { type: "refresh" }
  | { type: "select"; entityId: string }
  | { type: "action"; label: string; entityId: string }
  | { type: "workspace.remove"; workspaceId: string }
  | { type: "workRoot.open" }
  | { type: "workRoot.activation.set"; workRootId: string; activation: "online" | "offline" }
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
  | { type: "document.translation.toggle"; workRootId: string; path: string };

export type DashboardCommand = {
  commandId: DashboardCommandId;
  payload: DashboardCommandPayload;
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

export function buildWorkRootOpenCommand(_submittedHostPath: string): DashboardCommand {
  return { commandId: "workRoot.open", payload: { type: "workRoot.open" } };
}

export function buildRootPickerOpenCommand(): DashboardCommand {
  return { commandId: "rootPicker.open", payload: { type: "rootPicker.open" } };
}

export function buildRootPickerCloseCommand(): DashboardCommand {
  return { commandId: "rootPicker.close", payload: { type: "rootPicker.close" } };
}

export function buildRootPickerNavigateCommand(_targetPath: string): DashboardCommand {
  return {
    commandId: "rootPicker.navigate",
    payload: { type: "rootPicker.navigate" },
  };
}

export function buildRootPickerSelectDirectoryCommand(_targetPath: string): DashboardCommand {
  return {
    commandId: "rootPicker.selectDirectory",
    payload: { type: "rootPicker.selectDirectory" },
  };
}

export function buildRootPickerCreateDirectoryCommand(
  _parentPath: string,
  _name: string,
): DashboardCommand {
  return {
    commandId: "rootPicker.createDirectory",
    payload: { type: "rootPicker.createDirectory" },
  };
}

export function buildRootPickerPinDirectoryCommand(_path: string): DashboardCommand {
  return {
    commandId: "rootPicker.pinDirectory",
    payload: { type: "rootPicker.pinDirectory" },
  };
}

export function buildRootPickerUnpinDirectoryCommand(_path: string): DashboardCommand {
  return {
    commandId: "rootPicker.unpinDirectory",
    payload: { type: "rootPicker.unpinDirectory" },
  };
}

export function buildWorkspaceRemoveCommand(workspaceId: string): DashboardCommand {
  return {
    commandId: "workspace.remove",
    payload: { type: "workspace.remove", workspaceId },
  };
}

export function buildWorkRootActivationCommand(
  workRootId: string,
  activation: "online" | "offline",
): DashboardCommand {
  return {
    commandId: "workRoot.activation.set",
    payload: { type: "workRoot.activation.set", workRootId, activation },
  };
}

export function buildFileExplorerRefreshCommand(workRootId: string): DashboardCommand {
  return {
    commandId: "fileExplorer.refresh",
    payload: { type: "fileExplorer.refresh", workRootId },
  };
}

export function buildFileExplorerToggleDirectoryCommand(
  workRootId: string,
  path: string,
): DashboardCommand {
  return {
    commandId: "fileExplorer.toggleDirectory",
    payload: { type: "fileExplorer.toggleDirectory", workRootId, path },
  };
}

export function buildFileExplorerOpenFileCommand(
  workRootId: string,
  path: string,
  gesture: "singleClick" | "doubleClick",
): DashboardCommand {
  return {
    commandId: "fileExplorer.openFile",
    payload: { type: "fileExplorer.openFile", workRootId, path, gesture },
  };
}

export function buildFileExplorerSelectEntryCommand(
  workRootId: string,
  path: string,
): DashboardCommand {
  return {
    commandId: "fileExplorer.selectEntry",
    payload: { type: "fileExplorer.selectEntry", workRootId, path },
  };
}

export function buildWorkbenchOpenActivityCommand(workRootId: string): DashboardCommand {
  return {
    commandId: "workbench.openActivity",
    payload: { type: "workbench.openActivity", workRootId },
  };
}

export function buildTerminalCreateCommand(workRootId: string): DashboardCommand {
  return {
    commandId: "terminal.create",
    payload: { type: "terminal.create", workRootId },
  };
}

export function buildActivitySelectItemCommand(
  activityId: string,
): DashboardCommand {
  return {
    commandId: "activity.selectItem",
    payload: { type: "activity.selectItem", activityId },
  };
}

export function buildActivityTranscriptLoadMoreCommand(
  activityId: string,
): DashboardCommand {
  return {
    commandId: "activity.transcript.loadMore",
    payload: { type: "activity.transcript.loadMore", activityId },
  };
}

export function buildActivityRefreshCommand(workRootId: string): DashboardCommand {
  return {
    commandId: "activity.refresh",
    payload: { type: "activity.refresh", workRootId },
  };
}

export function buildActivityDetailToggleCommand(
  activityId: string,
  detailKey: string,
): DashboardCommand {
  return {
    commandId: "activity.detail.toggle",
    payload: { type: "activity.detail.toggle", activityId, detailKey },
  };
}

export function buildDocumentTranslationToggleCommand(
  workRootId: string,
  path: string,
): DashboardCommand {
  return {
    commandId: "document.translation.toggle",
    payload: { type: "document.translation.toggle", workRootId, path },
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
    case "workspace.remove":
      return "Remove workspace";
    case "workRoot.open":
      return "Open workRoot";
    case "workRoot.activation.set":
      return payload.activation === "online" ? "Bring workRoot online" : "Take workRoot offline";
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
      return payload.gesture === "doubleClick" ? "Open pinned file" : "Open file";
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
    case "action":
      return payload.label;
  }
}
