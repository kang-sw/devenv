export type DashboardCommandId =
  | "dashboard.refresh"
  | "workRoot.open"
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
  | `resource.action.${string}`
  | `workbench.toggle.${string}`
  | `workbench.tab.${string}`;

export type DashboardCommandPayload =
  | { type: "refresh" }
  | { type: "select"; entityId: string }
  | { type: "action"; label: string; entityId: string }
  | { type: "workRoot.open" }
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
  | { type: "activity.detail.toggle"; activityId: string; detailKey: string };

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
    case "workRoot.open":
      return "Open workRoot";
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
    case "action":
      return payload.label;
  }
}
