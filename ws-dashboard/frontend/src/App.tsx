import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, Dispatch, ReactNode, SetStateAction } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { normalizeServerRouteLocation } from "./routeBasis";
import {
  buildDashboardRefreshCommand,
  buildFileExplorerOpenFileCommand,
  buildFileExplorerRefreshCommand,
  buildFileExplorerSelectEntryCommand,
  buildFileExplorerToggleDirectoryCommand,
  buildTerminalCreateCommand,
  buildWorkbenchOpenActivityCommand,
  buildWorkRootActivationCommand,
  buildWorkRootOpenCommand,
  dashboardCommandLabel,
  dispatchDashboardCommand,
  type DashboardCommand,
  type DashboardCommandDispatcher,
  type DashboardCommandEntry,
  type DashboardCommandHandlers,
  type DashboardCommandPayload,
} from "./commands";
import {
  decideSurfaceClose,
  decideWorkbenchTabClosePresentation,
  decideSurfaceOpenWithDynamicGroups,
  applyWorkbenchPaneOrder,
  commitWorkbenchPaneMoveIntoDynamicGroup,
  reconcileActiveWorkbenchPanes,
  reconcileDashboardGroupsForPlacement,
  selectWorkbenchPane,
  surfaceLogicalKey,
  workbenchGroupId,
  DockviewWorkbenchLayout,
  type SurfaceKind,
  type WorkbenchPaneCategory,
  type WorkbenchPaneOrder,
  type DockviewTabCloseRequest,
  type WorkbenchPlacementState,
} from "./workbench";
import {
  applyReadOnlyFilePaneContent,
  applyReadOnlyFilePaneError,
  createLoadingReadOnlyFilePane,
  fetchWorkRootFiles,
  fetchWorkRootTextFile,
  flattenWorkRootFileTree,
  idleDirectoryLoadState,
  toggleExpandedPath,
  workRootExplorerInitialLoadPath,
  workRootExplorerRefreshPaths,
  workRootExplorerShouldLoadOnExpand,
  type DirectoryLoadState,
  readOnlyFilePaneLogicalKey,
  readOnlyFilePaneModeForOpenGesture,
  type ReadOnlyFileOpenGesture,
  type ReadOnlyFilePane,
  type WorkRootFileEntryView,
} from "./workRootFiles";
import {
  appendTerminalOutput,
  appendTerminalWebSocketMessage,
  canApplyTerminalOutputPoll,
  clampTerminalSize,
  closeTerminal,
  createTerminal,
  fetchTerminalOutput,
  listTerminals,
  markTerminalPaneCloseError,
  markTerminalSocketStatus,
  loadTerminalRestoreIntents,
  reconcileListedTerminalSessions,
  removeClosedTerminalPane,
  replaceTerminalRestoreIntentsForWorkRoot,
  resizeTerminal,
  sendTerminalInput,
  saveTerminalRestoreIntents,
  shouldPollTerminalOutput,
  terminalOutputPollChangedState,
  terminalPaneFromSession,
  terminalRestoreIntentsForWorkRoot,
  terminalRestoreIntentsFromPanes,
  terminalPaneLogicalKey,
  terminalWebSocketCursor,
  terminalWebSocketUrl,
  type TerminalCreateOptions,
  type TerminalPaneState,
  type TerminalWebSocketServerMessage,
  type TerminalSessionView,
} from "./terminals";
import {
  flattenEntities,
  reconcileSelectedId,
  type ActionHint,
  type DashboardResourcesView,
  type InstanceView,
  type ResourceEntity,
  type ResourcePath,
  type ServerView,
  type ViewState,
  type WorkRootView,
  type WorkspaceView,
} from "./resourceModel";
import { requestOpenWorkRoot } from "./openWorkRoot";
import { ActivityConsole } from "./ActivityConsole";
import {
  createResourceRefreshCoordinator,
  resourceAvailabilityPollIntervalMs,
  type ResourceRefreshCoordinator,
} from "./resourceRefresh";
import {
  applyActivityConsoleEvent,
  fetchWorkRootActivity,
  mergeWorkRootActivityViews,
  parseActivityConsoleEvent,
  shouldApplyActivityStreamRequest,
  workRootActivityBadge,
  workRootActivityEventsEndpoint,
  type ActivityConsoleEvent,
  type ActivityConsoleStreamRequest,
  type WorkRootActivityBadgeInput,
  type WorkRootActivityBadgeView,
} from "./workRootActivity";

type CommandPayload = DashboardCommandPayload;
type CommandEntry = DashboardCommandEntry;

type WorkRootExplorerSnapshot = {
  expandedPaths: Set<string>;
  directories: Record<string, DirectoryLoadState>;
  selectedPath: string | null;
};

type WorkbenchSelection = {
  workspace: WorkspaceView;
  root: WorkRootView;
  mainInstance: InstanceView | null;
  selectedInstance: InstanceView | null;
};

async function requestWorkRootActivation(
  workRootId: string,
  activation: "online" | "offline",
): Promise<DashboardResourcesView> {
  const response = await fetch(
    `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/activation`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ activation }),
    },
  );
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }
  return (await response.json()) as DashboardResourcesView;
}

// Terminal output is short-polled over HTTP (the daemon output route returns
// immediately). A snappy interval keeps keystroke echo latency low; idle polls
// are guarded below so they do not re-render the workbench.
const terminalOutputPollIntervalMs = 120;
const workRootActivityRefreshIntervalMs = 3_000;
const workRootActivityRecentRefreshLimit = 30;
const initialWorkbenchGroups = [
  { id: "group-1", label: "group 1" },
  { id: "group-2", label: "group 2" },
] as const;

export function App() {
  const [resources, setResources] = useState<DashboardResourcesView | null>(
    null,
  );
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [commandLog, setCommandLog] = useState<CommandEntry[]>([]);
  const [readOnlyFilePanes, setReadOnlyFilePanes] = useState<
    Record<string, ReadOnlyFilePane>
  >({});
  const [activeReadOnlyFilePaneRequest, setActiveReadOnlyFilePaneRequest] =
    useState<{
      paneId: string;
      sequence: number;
    } | null>(null);
  const [readOnlyFilePaneOrderByGroup, setReadOnlyFilePaneOrderByGroup] =
    useState<WorkbenchPaneOrder>({});
  const [workbenchGroupsByRoot, setWorkbenchGroupsByRoot] = useState<
    Record<string, ReadonlyArray<{ id: string; label: string }>>
  >({});
  const [paneOrderByRoot, setPaneOrderByRoot] = useState<
    Record<string, WorkbenchPaneOrder>
  >({});
  const commandSequence = useRef(0);
  const fileOpenSequence = useRef(0);
  const resourceRefreshCoordinatorRef =
    useRef<ResourceRefreshCoordinator | null>(null);

  if (!resourceRefreshCoordinatorRef.current) {
    resourceRefreshCoordinatorRef.current = createResourceRefreshCoordinator({
      applyResources: setResources,
      setLoading,
      setError,
    });
  }

  const loadResources = useCallback(
    (reason: "initial" | "explicit" | "poll" | "open" = "explicit") =>
      resourceRefreshCoordinatorRef.current?.refresh(reason),
    [],
  );

  useEffect(() => {
    resourceRefreshCoordinatorRef.current?.resume();
    void loadResources("initial");
  }, [loadResources]);

  useEffect(() => {
    resourceRefreshCoordinatorRef.current?.resume();
    const interval = window.setInterval(() => {
      void loadResources("poll");
    }, resourceAvailabilityPollIntervalMs);

    return () => {
      window.clearInterval(interval);
      resourceRefreshCoordinatorRef.current?.dispose();
    };
  }, [loadResources]);

  const handleWorkRootOpened = useCallback(
    (openedView: DashboardResourcesView, requestedWorkRootId?: string) => {
      // Identify the just-opened workRoot: the workRoot present in the
      // aggregated open response but absent from the prior resource view.
      const priorWorkRootIds = new Set(
        flattenEntities(resources)
          .filter((entity) => entity.type === "workRoot")
          .map((entity) => entity.id),
      );
      const openedWorkRootId = requestedWorkRootId ?? flattenEntities(openedView).find(
        (entity) =>
          entity.type === "workRoot" && !priorWorkRootIds.has(entity.id),
      )?.id;

      // Reconcile immediately with the aggregated open response and select the
      // opened workRoot, then re-fetch the canonical endpoint so it stays the
      // source of truth for refresh and re-entry.
      resourceRefreshCoordinatorRef.current?.applyExternalResources(openedView);
      if (openedWorkRootId) {
        setSelectedId(openedWorkRootId);
      }
      void loadResources("open");
    },
    [loadResources, resources],
  );

  useEffect(() => {
    if (!resources) {
      return;
    }

    normalizeServerRoute(resources.server.id);
  }, [resources]);

  const entities = useMemo(() => flattenEntities(resources), [resources]);

  useEffect(() => {
    // Reconcile after every resource change so a selection that left the
    // entity set (the mock workspace once the tree turns live) cannot remain
    // active.
    const nextSelectedId = reconcileSelectedId(entities, selectedId);
    if (nextSelectedId !== selectedId) {
      setSelectedId(nextSelectedId);
    }
  }, [entities, selectedId]);

  const selectedEntity =
    entities.find((entity) => entity.id === selectedId) ?? entities[0] ?? null;
  const workbenchSelection = useMemo(
    () => resolveWorkbenchSelection(resources, selectedId),
    [resources, selectedId],
  );

  const openReadOnlyFile = useCallback(
    (
      workRoot: WorkRootView,
      entry: WorkRootFileEntryView,
      gesture: ReadOnlyFileOpenGesture = "singleClick",
    ) => {
      const mode = readOnlyFilePaneModeForOpenGesture(gesture);
      const pane = createLoadingReadOnlyFilePane(
        workRoot.id,
        entry.path,
        mode,
      );
      const pinnedLogicalKey = readOnlyFilePaneLogicalKey(
        workRoot.id,
        entry.path,
        "pinned",
      );
      const previewLogicalKey = readOnlyFilePaneLogicalKey(
        workRoot.id,
        entry.path,
        "preview",
      );
      const existingPinnedPane = readOnlyFilePanes[pinnedLogicalKey];
      const focusPane = (paneId: string) =>
        setActiveReadOnlyFilePaneRequest({
          paneId,
          sequence: fileOpenSequence.current++,
        });

      if (mode === "pinned" && existingPinnedPane) {
        setReadOnlyFilePanes((current) => {
          const next = { ...current };
          delete next[previewLogicalKey];
          return next;
        });
        setReadOnlyFilePaneOrderByGroup((current) =>
          removePaneFromOrder(current, readOnlyFilePanes[previewLogicalKey]?.id),
        );
        focusPane(existingPinnedPane.id);
        return;
      }

      const groupsForPlacement =
        workbenchGroupsByRoot[workRoot.id] ?? initialWorkbenchGroups;
      const placement = decideSurfaceOpenWithDynamicGroups(
        readOnlyFilePlacementState(
          readOnlyFilePanes,
          groupsForPlacement,
          paneOrderByRoot[workRoot.id] ?? {},
          readOnlyFilePaneOrderByGroup,
        ),
        {
          surfaceKind: "editor",
          logicalKey: surfaceLogicalKey(...pane.logicalKey.split("/")),
          attachmentId:
            pane.id as WorkbenchPlacementState["attachments"][number]["attachmentId"],
        },
      );

      if (placement.type === "openNew" && placement.createdGroupId) {
        setWorkbenchGroupsByRoot((current) => ({
          ...current,
          [workRoot.id]: reconcileDashboardGroupsForPlacement(
            current[workRoot.id] ?? groupsForPlacement,
            placement,
          ),
        }));
      }

      setReadOnlyFilePanes((current) => {
        const next = { ...current };
        if (mode === "pinned") {
          delete next[previewLogicalKey];
        }
        next[pane.logicalKey] = pane;
        return next;
      });
      setReadOnlyFilePaneOrderByGroup((current) => {
        let next = mode === "pinned"
          ? removePaneFromOrder(current, readOnlyFilePanes[previewLogicalKey]?.id)
          : current;
        if (placement.type === "openNew") {
          next = {
            ...next,
            [placement.groupId]: [
              ...(next[placement.groupId] ?? []).filter((id) => id !== pane.id),
              pane.id,
            ],
          };
        }
        return next;
      });
      focusPane(pane.id);

      void fetchWorkRootTextFile(workRoot.id, entry.path)
        .then((file) => {
          setReadOnlyFilePanes((current) => {
            const currentPane = current[pane.logicalKey];
            if (!sameReadOnlyOpenRequest(currentPane, pane)) {
              return current;
            }
            return {
              ...current,
              [pane.logicalKey]: applyReadOnlyFilePaneContent(
                currentPane,
                file,
              ),
            };
          });
        })
        .catch((error) => {
          setReadOnlyFilePanes((current) => {
            const currentPane = current[pane.logicalKey];
            if (!sameReadOnlyOpenRequest(currentPane, pane)) {
              return current;
            }
            return {
              ...current,
              [pane.logicalKey]: applyReadOnlyFilePaneError(
                currentPane,
                error instanceof Error ? error.message : "file read failed",
              ),
            };
          });
        });
    },
    [
      paneOrderByRoot,
      readOnlyFilePaneOrderByGroup,
      readOnlyFilePanes,
      workbenchGroupsByRoot,
    ],
  );

  const executeCommand = useCallback<DashboardCommandDispatcher>(
    (command: DashboardCommand, handlers: DashboardCommandHandlers = {}) => {
      const executableHandlers: DashboardCommandHandlers = { ...handlers };
      if (command.payload.type === "select") {
        const { entityId } = command.payload;
        executableHandlers[command.commandId] = () => setSelectedId(entityId);
      } else if (command.payload.type === "refresh") {
        executableHandlers[command.commandId] = () => {
          void loadResources("explicit");
        };
      } else if (command.payload.type === "workRoot.activation.set") {
        const { workRootId, activation } = command.payload;
        executableHandlers[command.commandId] = () => {
          void requestWorkRootActivation(workRootId, activation)
            .then((nextResources) => {
              resourceRefreshCoordinatorRef.current?.applyExternalResources(nextResources);
            })
            .catch((nextError) => {
              setError(nextError instanceof Error ? nextError.message : "activation failed");
            });
        };
      }

      dispatchDashboardCommand(command, {
        handlers: executableHandlers,
        observer: (observedCommand) => {
          setCommandLog((entries) =>
            [
              {
                id: commandSequence.current++,
                commandId: observedCommand.commandId,
                label: dashboardCommandLabel(observedCommand),
              },
              ...entries,
            ].slice(0, 6),
          );
        },
      });
    },
    [loadResources],
  );

  return (
    <main className="app-shell" aria-label="ws dashboard">
      <div className="shell-grid shell-grid-workbench">
        <aside className="shell-panel shell-panel-nav" aria-label="Resources">
          <PanelHeader
            title={resources?.server.label ?? "ws dashboard"}
            state={resources?.server.state}
            actions={resources?.server.actions ?? []}
            entityId={resources?.server.id ?? "server"}
            onCommand={executeCommand}
          />
          <OpenWorkRootControl
            onOpened={handleWorkRootOpened}
            onCommand={executeCommand}
          />
          <ResourceNavigation
            resources={resources}
            loading={loading}
            error={error}
            selectedId={selectedEntity?.id ?? null}
            selectedWorkRoot={workbenchSelection?.root ?? null}
            onCommand={executeCommand}
            onOpenFile={openReadOnlyFile}
          />
        </aside>

        <section
          className="shell-panel shell-panel-workbench"
          aria-label="WorkRoot workbench"
        >
          <WorkbenchShell
            commandLog={commandLog}
            error={error}
            loading={loading}
            resources={resources}
            selectedEntity={selectedEntity}
            selection={workbenchSelection}
            workbenchGroupsByRoot={workbenchGroupsByRoot}
            paneOrderByRoot={paneOrderByRoot}
            onCommand={executeCommand}
            onWorkbenchGroupsByRootChange={setWorkbenchGroupsByRoot}
            onPaneOrderByRootChange={setPaneOrderByRoot}
            readOnlyFilePanes={Object.values(readOnlyFilePanes)}
            readOnlyFilePaneOrderByGroup={readOnlyFilePaneOrderByGroup}
            activeReadOnlyFilePaneRequest={activeReadOnlyFilePaneRequest}
            onReadOnlyFilePanesChange={setReadOnlyFilePanes}
            onReadOnlyFilePaneOrderByGroupChange={setReadOnlyFilePaneOrderByGroup}
          />
        </section>
      </div>
    </main>
  );
}

function PanelHeader({
  title,
  state,
  actions = [],
  entityId = "panel",
  onCommand,
}: {
  title: string;
  state?: ViewState;
  actions?: ActionHint[];
  entityId?: string;
  onCommand?: DashboardCommandDispatcher;
}) {
  return (
    <div className="panel-header">
      <div className="panel-title-block">
        <div className="panel-title">{title}</div>
        {state ? <StateLine state={state} /> : null}
      </div>
      {actions.length > 0 && onCommand ? (
        <div className="action-strip">
          {actions.map((action) => (
            <button
              className="action-button"
              data-command-id={
                action.id === "refresh"
                  ? "dashboard.refresh"
                  : `resource.action.${action.id}`
              }
              disabled={!action.enabled}
              key={action.id}
              title={action.label}
              type="button"
              onClick={() =>
                onCommand(
                  action.id === "refresh"
                    ? buildDashboardRefreshCommand()
                    : {
                        commandId: `resource.action.${action.id}`,
                        payload: { type: "action", label: action.label, entityId },
                      },
                )
              }
            >
              {action.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function OpenWorkRootControl({
  onOpened,
  onCommand,
}: {
  onOpened: (view: DashboardResourcesView, requestedWorkRootId?: string) => void;
  onCommand: DashboardCommandDispatcher;
}) {
  const [path, setPath] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    const requestedPath = path.trim();
    if (requestedPath.length === 0 || pending) {
      return;
    }

    onCommand(
      buildWorkRootOpenCommand(requestedPath),
      {
        "workRoot.open": () => {
          setPending(true);
          setError(null);
          void requestOpenWorkRoot(requestedPath)
            .then((result) => {
              setPath("");
              onOpened(result.view, result.openedWorkRootId ?? undefined);
            })
            .catch((nextError) => {
              setError(nextError instanceof Error ? nextError.message : "open failed");
            })
            .finally(() => {
              setPending(false);
            });
        },
      },
    );
  };

  return (
    <form
      className="open-work-root"
      aria-label="Open workRoot"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <label className="section-label" htmlFor="open-work-root-path">
        Open workRoot
      </label>
      <div className="open-work-root-row">
        <input
          id="open-work-root-path"
          className="open-work-root-input"
          type="text"
          autoComplete="off"
          spellCheck={false}
          placeholder="/path/to/workRoot"
          value={path}
          disabled={pending}
          onChange={(event) => setPath(event.target.value)}
        />
        <button
          className="action-button action-button-primary"
          data-command-id="workRoot.open"
          disabled={pending || path.trim().length === 0}
          type="submit"
        >
          {pending ? "Opening" : "Open"}
        </button>
      </div>
      {error ? (
        <InlineNotice tone="error" title="Open failed" detail={error} />
      ) : null}
    </form>
  );
}

function ResourceNavigation({
  resources,
  loading,
  error,
  selectedId,
  selectedWorkRoot,
  onCommand,
  onOpenFile,
}: {
  resources: DashboardResourcesView | null;
  loading: boolean;
  error: string | null;
  selectedId: string | null;
  selectedWorkRoot: WorkRootView | null;
  onCommand: DashboardCommandDispatcher;
  onOpenFile: (
    workRoot: WorkRootView,
    entry: WorkRootFileEntryView,
    gesture: ReadOnlyFileOpenGesture,
  ) => void;
}) {
  if (loading && !resources) {
    return <StatusPane title="Loading" detail="resources" />;
  }

  if (error && !resources) {
    return (
      <StatusPane
        title="Fetch failed"
        detail={error}
        action={
          <button
            className="action-button action-button-primary"
            data-command-id="dashboard.refresh"
            type="button"
            onClick={() => onCommand(buildDashboardRefreshCommand())}
          >
            Refresh
          </button>
        }
      />
    );
  }

  if (!resources || resources.workspaces.length === 0) {
    return <StatusPane title="Empty" detail="no workspaces" />;
  }

  return (
    <div className="nav-stack">
      <div className="resource-list resource-list-region">
        {error ? (
          <InlineNotice tone="error" title="Refresh failed" detail={error} />
        ) : null}
        {loading ? (
          <InlineNotice tone="info" title="Refreshing" detail="resources" />
        ) : null}
        {resources.workspaces.map((workspace) => (
          <WorkspaceRows
            key={workspace.id}
            workspace={workspace}
            selectedId={selectedId}
            onCommand={onCommand}
          />
        ))}
      </div>
      <WorkRootFileExplorer
        workRoot={selectedWorkRoot}
        onCommand={onCommand}
        onOpenFile={onOpenFile}
      />
    </div>
  );
}

function WorkRootFileExplorer({
  workRoot,
  onCommand,
  onOpenFile,
}: {
  workRoot: WorkRootView | null;
  onCommand: DashboardCommandDispatcher;
  onOpenFile: (
    workRoot: WorkRootView,
    entry: WorkRootFileEntryView,
    gesture: ReadOnlyFileOpenGesture,
  ) => void;
}) {
  const [snapshots, setSnapshots] = useState<
    Record<string, WorkRootExplorerSnapshot>
  >({});

  const snapshot = workRoot
    ? (snapshots[workRoot.id] ?? initialExplorerSnapshot())
    : null;

  const updateSnapshot = useCallback(
    (
      workRootId: string,
      updater: (snapshot: WorkRootExplorerSnapshot) => WorkRootExplorerSnapshot,
    ) => {
      setSnapshots((current) => ({
        ...current,
        [workRootId]: updater(current[workRootId] ?? initialExplorerSnapshot()),
      }));
    },
    [],
  );

  const loadDirectory = useCallback(
    async (workRootId: string, path: string) => {
      updateSnapshot(workRootId, (current) => ({
        ...current,
        directories: {
          ...current.directories,
          [path]: { ...idleDirectoryLoadState(), status: "loading" },
        },
      }));

      try {
        const listing = await fetchWorkRootFiles(workRootId, path);
        updateSnapshot(workRootId, (current) => ({
          ...current,
          directories: {
            ...current.directories,
            [listing.path]: {
              status: "loaded",
              entries: listing.entries,
              error: null,
            },
          },
        }));
      } catch (error) {
        updateSnapshot(workRootId, (current) => ({
          ...current,
          directories: {
            ...current.directories,
            [path]: {
              status: "error",
              entries: [],
              error: error instanceof Error ? error.message : "listing failed",
            },
          },
        }));
      }
    },
    [updateSnapshot],
  );

  useEffect(() => {
    if (!workRoot) {
      return;
    }

    const initialPath = workRootExplorerInitialLoadPath(snapshots[workRoot.id]);
    if (initialPath !== null) {
      void loadDirectory(workRoot.id, initialPath);
    }
  }, [loadDirectory, snapshots, workRoot]);

  if (!workRoot) {
    return (
      <section className="file-explorer" aria-label="WorkRoot files">
        <div className="file-explorer-header">
          <div>
            <div className="section-label">Files</div>
            <div className="file-explorer-title">Select a workRoot</div>
          </div>
        </div>
        <div className="file-explorer-body">
          <div className="file-explorer-state">No workRoot selected</div>
        </div>
      </section>
    );
  }

  const rows = flattenWorkRootFileTree({
    expandedPaths: snapshot?.expandedPaths ?? new Set([""]),
    directories: snapshot?.directories ?? {},
    selectedPath: snapshot?.selectedPath ?? null,
  });

  const selectEntry = (entry: WorkRootFileEntryView) => {
    onCommand(
      buildFileExplorerSelectEntryCommand(workRoot.id, entry.path),
      {
        "fileExplorer.selectEntry": () => {
          updateSnapshot(workRoot.id, (current) => ({
            ...current,
            selectedPath: entry.path,
          }));
        },
      },
    );
  };

  const toggleDirectory = (entry: WorkRootFileEntryView) => {
    const isExpanded = snapshot?.expandedPaths.has(entry.path) ?? false;
    onCommand(
      buildFileExplorerToggleDirectoryCommand(workRoot.id, entry.path),
      {
        "fileExplorer.toggleDirectory": () => {
          updateSnapshot(workRoot.id, (current) => ({
            ...current,
            expandedPaths: toggleExpandedPath(current.expandedPaths, entry.path),
            selectedPath: entry.path,
          }));

          if (workRootExplorerShouldLoadOnExpand(snapshot, entry.path, isExpanded)) {
            void loadDirectory(workRoot.id, entry.path);
          }
        },
      },
    );
  };

  const openFile = (
    entry: WorkRootFileEntryView,
    gesture: ReadOnlyFileOpenGesture = "singleClick",
  ) => {
    onCommand(
      buildFileExplorerOpenFileCommand(workRoot.id, entry.path, gesture),
      {
        "fileExplorer.openFile": () => {
          updateSnapshot(workRoot.id, (current) => ({
            ...current,
            selectedPath: entry.path,
          }));
          onOpenFile(workRoot, entry, gesture);
        },
      },
    );
  };

  const refreshExplorer = () => {
    onCommand(
      buildFileExplorerRefreshCommand(workRoot.id),
      {
        "fileExplorer.refresh": () => {
          const paths = workRootExplorerRefreshPaths(
            snapshot?.expandedPaths ?? new Set([""]),
          );
          for (const path of paths) {
            void loadDirectory(workRoot.id, path);
          }
        },
      },
    );
  };

  return (
    <section
      className="file-explorer"
      aria-label={`Files for ${workRoot.label}`}
    >
      <div className="file-explorer-header">
        <div className="file-explorer-heading">
          <div className="section-label">Files</div>
          <div className="file-explorer-title" title={workRoot.label}>
            {workRoot.label}
          </div>
        </div>
        <button
          className="action-button file-explorer-refresh"
          data-command-id="fileExplorer.refresh"
          type="button"
          onClick={refreshExplorer}
        >
          Refresh
        </button>
      </div>
      <div
        className="file-explorer-body"
        role="tree"
        aria-label="WorkRoot file tree"
      >
        {rows.length === 0 ? (
          <div className="file-explorer-state">Loading</div>
        ) : (
          rows.map((row) =>
            row.type === "state" ? (
              <div
                className={`file-explorer-state file-explorer-state-${row.status}`}
                key={`${row.path}:${row.status}`}
                style={{ "--depth": row.depth } as CSSProperties}
              >
                {row.label}
              </div>
            ) : (
              <FileExplorerRow
                entry={row.entry}
                expanded={row.expanded}
                key={row.entry.path}
                depth={row.depth}
                selected={row.selected}
                onSelect={selectEntry}
                onToggleDirectory={toggleDirectory}
                onOpenFile={openFile}
              />
            ),
          )
        )}
      </div>
    </section>
  );
}

function FileExplorerRow({
  entry,
  depth,
  expanded,
  selected,
  onSelect,
  onToggleDirectory,
  onOpenFile,
}: {
  entry: WorkRootFileEntryView;
  depth: number;
  expanded: boolean;
  selected: boolean;
  onSelect: (entry: WorkRootFileEntryView) => void;
  onToggleDirectory: (entry: WorkRootFileEntryView) => void;
  onOpenFile: (entry: WorkRootFileEntryView, gesture: ReadOnlyFileOpenGesture) => void;
}) {
  const isDirectory = entry.kind === "directory";
  const isOk = entry.status === "ok";
  const canOpen = !isDirectory && entry.previewEligible && isOk;

  // Conventional tree interaction: the whole row is the control. A directory
  // row toggles expansion, a previewable file row opens its read-only preview,
  // and any other row simply selects. The emitted command id matches that
  // action so the keyboard command layer stays aligned with the click.
  const commandId = isDirectory
    ? "fileExplorer.toggleDirectory"
    : canOpen
      ? "fileExplorer.openFile"
      : "fileExplorer.selectEntry";

  const activate = () => {
    if (isDirectory) {
      onToggleDirectory(entry);
    } else if (canOpen) {
      onOpenFile(entry, "singleClick");
    } else {
      onSelect(entry);
    }
  };

  const title = isDirectory
    ? `${entry.path || entry.name} (${expanded ? "expanded" : "collapsed"})`
    : canOpen
      ? `Open read-only preview of ${entry.name}`
      : isOk
        ? `${entry.name} (preview unavailable)`
        : `${entry.name} (${entry.status})`;

  return (
    <button
      className={`file-explorer-row ${isDirectory ? "file-explorer-row-directory" : "file-explorer-row-file"} ${
        selected ? "file-explorer-row-selected" : ""
      } ${!isOk ? "file-explorer-row-muted" : ""}`}
      role="treeitem"
      aria-expanded={isDirectory ? expanded : undefined}
      aria-selected={selected}
      data-command-id={commandId}
      type="button"
      style={{ "--depth": depth } as CSSProperties}
      title={title}
      onClick={activate}
      onDoubleClick={() => {
        if (canOpen) {
          onOpenFile(entry, "doubleClick");
        }
      }}
    >
      <span className="file-explorer-twisty" aria-hidden="true">
        {isDirectory ? (expanded ? "▾" : "▸") : ""}
      </span>
      <span className="file-explorer-name">
        {entry.name}
        {isDirectory ? "/" : ""}
      </span>
      {!isOk ? (
        <span className="file-explorer-row-status">{entry.status}</span>
      ) : null}
      {canOpen ? (
        <span className="file-explorer-row-hint" aria-hidden="true">
          open
        </span>
      ) : null}
    </button>
  );
}

function initialExplorerSnapshot(): WorkRootExplorerSnapshot {
  return {
    expandedPaths: new Set([""]),
    directories: {},
    selectedPath: null,
  };
}

function InlineNotice({
  tone,
  title,
  detail,
}: {
  tone: "error" | "info";
  title: string;
  detail: string;
}) {
  return (
    <div className={`inline-notice inline-notice-${tone}`} role="status">
      <strong>{title}</strong>
      <span>{detail}</span>
    </div>
  );
}

function WorkbenchShell({
  resources,
  selection,
  selectedEntity,
  commandLog,
  loading,
  error,
  onCommand,
  readOnlyFilePanes,
  readOnlyFilePaneOrderByGroup,
  activeReadOnlyFilePaneRequest,
  workbenchGroupsByRoot,
  paneOrderByRoot,
  onWorkbenchGroupsByRootChange,
  onPaneOrderByRootChange,
  onReadOnlyFilePanesChange,
  onReadOnlyFilePaneOrderByGroupChange,
}: {
  resources: DashboardResourcesView | null;
  selection: WorkbenchSelection | null;
  selectedEntity: ResourceEntity | null;
  commandLog: CommandEntry[];
  loading: boolean;
  error: string | null;
  onCommand: DashboardCommandDispatcher;
  readOnlyFilePanes: ReadOnlyFilePane[];
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder;
  activeReadOnlyFilePaneRequest: { paneId: string; sequence: number } | null;
  workbenchGroupsByRoot: Record<
    string,
    ReadonlyArray<{ id: string; label: string }>
  >;
  paneOrderByRoot: Record<string, WorkbenchPaneOrder>;
  onWorkbenchGroupsByRootChange: Dispatch<
    SetStateAction<Record<string, ReadonlyArray<{ id: string; label: string }>>>
  >;
  onPaneOrderByRootChange: Dispatch<
    SetStateAction<Record<string, WorkbenchPaneOrder>>
  >;
  onReadOnlyFilePanesChange: Dispatch<
    SetStateAction<Record<string, ReadOnlyFilePane>>
  >;
  onReadOnlyFilePaneOrderByGroupChange: Dispatch<
    SetStateAction<WorkbenchPaneOrder>
  >;
}) {
  const [activePaneByRoot, setActivePaneByRoot] = useState<
    Record<string, Record<string, string>>
  >({});
  const [terminalPanes, setTerminalPanes] = useState<
    Record<string, TerminalPaneState>
  >({});
  const [activeTerminalPaneRequest, setActiveTerminalPaneRequest] = useState<{
    paneId: string;
    sequence: number;
  } | null>(null);
  const [terminalPaneOrderByGroup, setTerminalPaneOrderByGroup] =
    useState<WorkbenchPaneOrder>({});
  const [pendingCloseRequest, setPendingCloseRequest] = useState<
    | (DockviewTabCloseRequest & {
        readonly anchor: { clientX: number; clientY: number };
        readonly workRootId: string;
      })
    | null
  >(null);
  const [closedAgentPaneByRoot, setClosedAgentPaneByRoot] = useState<
    Record<string, readonly string[]>
  >({});
  const focusedReadOnlyRequest = useRef<number | null>(null);
  const focusedTerminalRequest = useRef<number | null>(null);
  const terminalOpenSequence = useRef(0);
  const restoredTerminalIntentRoots = useRef<Set<string>>(new Set());
  const [focusedTerminalPaneId, setFocusedTerminalPaneId] = useState<
    string | null
  >(null);
  const focusedTerminalPaneIdRef = useRef<string | null>(null);
  focusedTerminalPaneIdRef.current = focusedTerminalPaneId;
  // Selected-workRoot named-agent activity for the compact top-bar badge.
  // Keep the owning root id beside the fetch state so a root switch never
  // renders the previous root's activity for a frame before the effect resets.
  const [workRootActivityState, setWorkRootActivityState] = useState<{
    rootId: string | null;
    activity: WorkRootActivityBadgeInput;
  }>({ rootId: null, activity: { phase: "loading" } });
  const workRootActivityStateRef = useRef(workRootActivityState);
  workRootActivityStateRef.current = workRootActivityState;
  const activityStreamRequestSeq = useRef(0);
  const currentActivityStreamRequest = useRef<ActivityConsoleStreamRequest>({
    workRootId: "",
    requestId: 0,
  });
  const activitySnapshotRequestSeq = useRef(0);
  const [activityPollFallbackRootId, setActivityPollFallbackRootId] = useState<string | null>(null);
  const [activityTranscriptRefresh, setActivityTranscriptRefresh] = useState<{
    rootId: string;
    activityId: string;
    cursor: string | null;
    sequence: number;
  } | null>(null);
  // Whether the reversible WorkRoot Activity pane is open, scoped per workRoot
  // like terminal/read-only pane state. The badge entrypoint toggles this on;
  // immediate close toggles it off.
  const [activityPaneOpenByRoot, setActivityPaneOpenByRoot] = useState<
    Record<string, boolean>
  >({});
  const selectedWorkRootId = selection?.root.id ?? null;
  const workbenchGroups = selectedWorkRootId
    ? (workbenchGroupsByRoot[selectedWorkRootId] ?? initialWorkbenchGroups)
    : initialWorkbenchGroups;
  const paneOrderByGroup = selectedWorkRootId
    ? (paneOrderByRoot[selectedWorkRootId] ?? {})
    : {};
  const activePaneByGroup = selectedWorkRootId
    ? (activePaneByRoot[selectedWorkRootId] ?? {})
    : {};
  const activityPaneOpenForSelected = selectedWorkRootId
    ? (activityPaneOpenByRoot[selectedWorkRootId] ?? false)
    : false;

  const setActivePaneByGroupForSelected = (
    next:
      | Record<string, string>
      | ((current: Record<string, string>) => Record<string, string>),
  ) => {
    if (!selectedWorkRootId) {
      return;
    }
    setActivePaneByRoot((currentByRoot) => {
      const current = currentByRoot[selectedWorkRootId] ?? {};
      return {
        ...currentByRoot,
        [selectedWorkRootId]: typeof next === "function" ? next(current) : next,
      };
    });
  };

  const workbenchModel =
    resources && selection
      ? (() => {
          const { workspace, root, mainInstance, selectedInstance } = selection;
          const supportEntity =
            selectedEntity ?? resourceEntityForWorkRoot(root);
          const editorGroups = applyWorkbenchPaneOrder(
            buildWorkbenchEditorGroups(
              root,
              workbenchGroups,
              mainInstance,
              selectedInstance,
              supportEntity,
              readOnlyFilePanes,
              readOnlyFilePaneOrderByGroup,
              paneOrderByGroup,
              Object.values(terminalPanes),
              terminalPaneOrderByGroup,
              {
                onSendData: sendTerminalData,
                onClose: closeTerminalPane,
                onResize: forwardTerminalResize,
                onSocketStatus: updateTerminalSocketStatus,
                onSocketMessage: applyTerminalSocketMessage,
                onSocketResize: acceptTerminalSocketResize,
                onFocusInput: (pane) => setFocusedTerminalPaneId(pane.paneId),
                isActivePane: (pane) =>
                  focusedTerminalPaneIdRef.current === pane.paneId,
              },
              closedAgentPaneByRoot[root.id] ?? [],
              activityPaneOpenByRoot[root.id] ?? false,
              workRootActivityState.rootId === root.id
                ? workRootActivityState.activity
                : { phase: "loading" },
              activityTranscriptRefresh?.rootId === root.id
                ? activityTranscriptRefresh
                : null,
              onCommand,
            ),
            paneOrderByGroup,
          );
          return {
            workspace,
            root,
            mainInstance,
            selectedInstance,
            editorGroups,
          };
        })()
      : null;
  const editorGroups = workbenchModel?.editorGroups ?? [];

  useEffect(() => {
    if (!workbenchModel) {
      return;
    }
    const rootId = workbenchModel.root.id;
    const listStartedAtMs = Date.now();
    void listTerminals(rootId)
      .then((sessions) => {
        const restoreIntents = terminalRestoreIntentsForWorkRoot(
          loadTerminalRestoreIntents(),
          rootId,
        );
        if (
          sessions.length === 0 &&
          restoreIntents.length > 0 &&
          !restoredTerminalIntentRoots.current.has(rootId)
        ) {
          restoredTerminalIntentRoots.current.add(rootId);
          for (const intent of restoreIntents) {
            onCommand(buildTerminalCreateCommand(rootId), {
              "terminal.create": () =>
                createTerminalPane({
                  title: intent.title,
                  cwdHint: intent.cwdHint,
                }),
            });
          }
          return;
        }
        setTerminalPanes((current) =>
          persistTerminalPanesForWorkRoot(
            rootId,
            reconcileListedTerminalSessions(
              current,
              rootId,
              sessions,
              listStartedAtMs,
            ),
          ),
        );
        setTerminalPaneOrderByGroup((current) =>
          placeTerminalSessions(
            current,
            terminalPanes,
            sessions,
            workbenchGroups,
            paneOrderByGroup,
          ),
        );
      })
      .catch(() => undefined);
  }, [workbenchModel?.root.id]);

  // Fetch named-agent activity for the selected workRoot through the Phase 1
  // protected route. Loading/error are bounded badge states; a failure never
  // breaks the workbench.
  useEffect(() => {
    const rootId = workbenchModel?.root.id;
    if (!rootId) {
      return;
    }
    let cancelled = false;
    setWorkRootActivityState({ rootId, activity: { phase: "loading" } });
    const timer = window.setTimeout(() => {
      void fetchWorkRootActivity(rootId)
        .then((view) => {
          if (!cancelled) {
            setWorkRootActivityState({
              rootId,
              activity: { phase: "ready", view },
            });
          }
        })
        .catch(() => {
          if (!cancelled) {
            setWorkRootActivityState({ rootId, activity: { phase: "error" } });
          }
        });
    }, 300);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [workbenchModel?.root.id]);

  // Activity Console live stream: while the pane is open, subscribe to the
  // daemon-owned source-neutral event stream for the selected workRoot. The
  // older recent-list polling remains a bounded fallback only when the stream
  // cannot be established or the daemon explicitly switches to pollFallback.
  useEffect(() => {
    const rootId = workbenchModel?.root.id;
    if (!rootId || !activityPaneOpenForSelected) {
      currentActivityStreamRequest.current = {
        workRootId: rootId ?? "",
        requestId: activityStreamRequestSeq.current + 1,
      };
      activityStreamRequestSeq.current = currentActivityStreamRequest.current.requestId;
      setActivityPollFallbackRootId((current) => (current === rootId ? null : current));
      return;
    }

    const requestId = activityStreamRequestSeq.current + 1;
    activityStreamRequestSeq.current = requestId;
    const expected = { workRootId: rootId, requestId };
    currentActivityStreamRequest.current = expected;
    setActivityPollFallbackRootId((current) => (current === rootId ? null : current));

    let cancelled = false;
    let streamOpened = false;
    let fallbackTimer: number | null = null;
    const after =
      workRootActivityStateRef.current.rootId === rootId &&
      workRootActivityStateRef.current.activity.phase === "ready"
        ? workRootActivityStateRef.current.activity.view.feedCursor
        : null;
    const source = new EventSource(workRootActivityEventsEndpoint(rootId, { after }));

    const requestSnapshot = () => {
      const snapshotRequestId = activitySnapshotRequestSeq.current + 1;
      activitySnapshotRequestSeq.current = snapshotRequestId;
      void fetchWorkRootActivity(rootId)
        .then((view) => {
          if (
            cancelled ||
            snapshotRequestId !== activitySnapshotRequestSeq.current ||
            !shouldApplyActivityStreamRequest(expected, currentActivityStreamRequest.current) ||
            view.workRootId !== rootId
          ) {
            return;
          }
          setWorkRootActivityState({ rootId, activity: { phase: "ready", view } });
        })
        .catch(() => {
          if (
            !cancelled &&
            snapshotRequestId === activitySnapshotRequestSeq.current &&
            shouldApplyActivityStreamRequest(expected, currentActivityStreamRequest.current)
          ) {
            setActivityPollFallbackRootId(rootId);
          }
        });
    };

    const applyStreamEvent = (event: ActivityConsoleEvent) => {
      if (
        cancelled ||
        !shouldApplyActivityStreamRequest(expected, currentActivityStreamRequest.current)
      ) {
        return;
      }
      if (event.type === "snapshotInvalidated") {
        requestSnapshot();
      }
      if (event.type === "transcriptUpdated") {
        setActivityTranscriptRefresh((current) => ({
          rootId,
          activityId: event.activityId,
          cursor: event.transcriptCursor,
          sequence: (current?.sequence ?? 0) + 1,
        }));
      } else if (event.type === "modeChanged" && event.updateMode === "pollFallback") {
        setActivityPollFallbackRootId(rootId);
      } else if (
        event.type === "modeChanged" &&
        (event.updateMode === "watch" || event.updateMode === "snapshot")
      ) {
        if (fallbackTimer !== null) {
          window.clearTimeout(fallbackTimer);
          fallbackTimer = null;
        }
        setActivityPollFallbackRootId((fallbackRootId) =>
          fallbackRootId === rootId ? null : fallbackRootId,
        );
      }
      setWorkRootActivityState((current) => {
        if (current.rootId !== rootId || current.activity.phase !== "ready") {
          return current;
        }
        const result = applyActivityConsoleEvent(current.activity.view, event);
        return { rootId, activity: { phase: "ready", view: result.view } };
      });
    };

    source.onopen = () => {
      streamOpened = true;
      if (shouldApplyActivityStreamRequest(expected, currentActivityStreamRequest.current)) {
        if (fallbackTimer !== null) {
          window.clearTimeout(fallbackTimer);
          fallbackTimer = null;
        }
        setActivityPollFallbackRootId((current) => (current === rootId ? null : current));
      }
    };
    const handleActivityMessage = (message: MessageEvent) => {
      let payload: unknown;
      try {
        payload = JSON.parse(message.data);
      } catch {
        setActivityPollFallbackRootId(rootId);
        return;
      }
      const event = parseActivityConsoleEvent(payload);
      if (!event) {
        setActivityPollFallbackRootId(rootId);
        return;
      }
      applyStreamEvent(event);
    };
    source.addEventListener("activity", handleActivityMessage);
    source.onmessage = handleActivityMessage;
    source.onerror = () => {
      if (
        cancelled ||
        !shouldApplyActivityStreamRequest(expected, currentActivityStreamRequest.current)
      ) {
        return;
      }
      if (!streamOpened) {
        setActivityPollFallbackRootId(rootId);
        requestSnapshot();
        return;
      }
      if (fallbackTimer !== null) {
        window.clearTimeout(fallbackTimer);
      }
      fallbackTimer = window.setTimeout(() => {
        if (
          !cancelled &&
          shouldApplyActivityStreamRequest(expected, currentActivityStreamRequest.current)
        ) {
          setActivityPollFallbackRootId(rootId);
        }
      }, 1_000);
    };

    return () => {
      cancelled = true;
      if (fallbackTimer !== null) {
        window.clearTimeout(fallbackTimer);
      }
      source.removeEventListener("activity", handleActivityMessage);
      source.close();
      setActivityPollFallbackRootId((current) => (current === rootId ? null : current));
    };
  }, [workbenchModel?.root.id, activityPaneOpenForSelected]);

  useEffect(() => {
    const rootId = workbenchModel?.root.id;
    if (!rootId || !activityPaneOpenForSelected || activityPollFallbackRootId !== rootId) {
      return;
    }

    let cancelled = false;
    let inFlight = false;
    const refreshRecentActivity = () => {
      if (cancelled || inFlight || document.hidden) {
        return;
      }
      inFlight = true;
      const snapshotRequestId = activitySnapshotRequestSeq.current + 1;
      activitySnapshotRequestSeq.current = snapshotRequestId;
      void fetchWorkRootActivity(rootId, {
        recentLimit: workRootActivityRecentRefreshLimit,
      })
        .then((view) => {
          if (cancelled || snapshotRequestId !== activitySnapshotRequestSeq.current) {
            return;
          }
          setWorkRootActivityState((current) => {
            if (current.rootId !== rootId || view.workRootId !== rootId) {
              return current;
            }
            if (current.activity.phase !== "ready") {
              return { rootId, activity: { phase: "ready", view } };
            }
            return {
              rootId,
              activity: {
                phase: "ready",
                view: mergeWorkRootActivityViews(current.activity.view, view),
              },
            };
          });
        })
        .catch(() => undefined)
        .finally(() => {
          inFlight = false;
        });
    };

    refreshRecentActivity();
    const timer = window.setInterval(
      refreshRecentActivity,
      workRootActivityRefreshIntervalMs,
    );
    const onVisibilityChange = () => {
      if (!document.hidden) {
        refreshRecentActivity();
      }
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [workbenchModel?.root.id, activityPaneOpenForSelected, activityPollFallbackRootId]);

  // The output poll reads live terminal sessions from a ref so the polling
  // interval stays stable across renders. Depending the interval on
  // `terminalPanes` would tear down and recreate it on every output delta.
  const livePollPanesRef = useRef<
    Array<{ terminalId: string; logicalKey: string; nextSequence: number }>
  >([]);
  livePollPanesRef.current = workbenchModel
    ? Object.values(terminalPanes)
        .filter(
          (pane) =>
            pane.session.workRootId === workbenchModel.root.id &&
            shouldPollTerminalOutput(pane),
        )
        .map((pane) => ({
          terminalId: pane.session.terminalId,
          logicalKey: pane.logicalKey,
          nextSequence: pane.nextSequence,
        }))
    : [];

  useEffect(() => {
    if (!workbenchModel) {
      return;
    }
    // Track in-flight requests per PTY so a slow response never stacks
    // overlapping polls for the same terminal.
    const inFlight = new Set<string>();
    let cancelled = false;

    const poll = () => {
      for (const pane of livePollPanesRef.current) {
        if (inFlight.has(pane.terminalId)) {
          continue;
        }
        inFlight.add(pane.terminalId);
        void fetchTerminalOutput(pane.terminalId, pane.nextSequence)
          .then((output) => {
            if (cancelled) {
              return;
            }
            setTerminalPanes((current) => {
              const existing = current[pane.logicalKey];
              if (!existing) {
                return current;
              }
              // Skip the state replacement when a poll changed nothing so
              // React does not re-render the whole workbench tree every cycle
              // while terminals are quiet. A stale error still counts as a
              // change so a successful poll clears it.
              if (!canApplyTerminalOutputPoll(existing, pane.nextSequence)) {
                return current;
              }
              if (!terminalOutputPollChangedState(existing, output)) {
                return current;
              }
              return {
                ...current,
                [pane.logicalKey]: appendTerminalOutput(existing, output),
              };
            });
          })
          .catch((error) => {
            if (cancelled) {
              return;
            }
            const message =
              error instanceof Error ? error.message : "terminal output failed";
            setTerminalPanes((current) => {
              const existing = current[pane.logicalKey];
              if (!existing || existing.error === message) {
                return current;
              }
              return {
                ...current,
                [pane.logicalKey]: { ...existing, error: message },
              };
            });
          })
          .finally(() => {
            inFlight.delete(pane.terminalId);
          });
      }
    };

    const timer = window.setInterval(poll, terminalOutputPollIntervalMs);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [workbenchModel?.root.id]);

  useEffect(() => {
    if (
      !activeReadOnlyFilePaneRequest ||
      focusedReadOnlyRequest.current === activeReadOnlyFilePaneRequest.sequence
    ) {
      return;
    }

    const targetGroup = editorGroups.find((group) =>
      group.panes.some(
        (pane) => pane.id === activeReadOnlyFilePaneRequest.paneId,
      ),
    );
    if (!targetGroup) {
      return;
    }

    focusedReadOnlyRequest.current = activeReadOnlyFilePaneRequest.sequence;
    setActivePaneByGroupForSelected((current) =>
      selectWorkbenchPane(
        current,
        targetGroup.id,
        activeReadOnlyFilePaneRequest.paneId,
      ),
    );
  }, [activeReadOnlyFilePaneRequest, editorGroups]);

  useEffect(() => {
    // Focus a freshly created terminal exactly once per request. Without the
    // sequence guard this effect re-asserts the active pane on every
    // `editorGroups` rebuild (the 500ms output poll churns identity), which
    // fights a user clicking a different terminal tab.
    if (
      !activeTerminalPaneRequest ||
      focusedTerminalRequest.current === activeTerminalPaneRequest.sequence
    ) {
      return;
    }
    const targetGroup = editorGroups.find((group) =>
      group.panes.some((pane) => pane.id === activeTerminalPaneRequest.paneId),
    );
    if (!targetGroup) {
      return;
    }
    focusedTerminalRequest.current = activeTerminalPaneRequest.sequence;
    setFocusedTerminalPaneId(activeTerminalPaneRequest.paneId);
    setActivePaneByGroupForSelected((current) =>
      selectWorkbenchPane(
        current,
        targetGroup.id,
        activeTerminalPaneRequest.paneId,
      ),
    );
  }, [activeTerminalPaneRequest, editorGroups]);

  function persistTerminalPanesForWorkRoot(
    workRootId: string,
    nextPanes: Record<string, TerminalPaneState>,
  ): Record<string, TerminalPaneState> {
    const nextIntents = terminalRestoreIntentsFromPanes(
      Object.values(nextPanes).filter(
        (pane) => pane.session.workRootId === workRootId,
      ),
    );
    saveTerminalRestoreIntents(
      replaceTerminalRestoreIntentsForWorkRoot(
        loadTerminalRestoreIntents(),
        workRootId,
        nextIntents,
      ),
    );
    return nextPanes;
  }

  function createTerminalPane(options: TerminalCreateOptions = {}) {
    if (!workbenchModel) {
      return;
    }
    const rootId = workbenchModel.root.id;
    void createTerminal(rootId, options)
      .then((session) => {
        const pane = terminalPaneFromSession(session);
        setTerminalPanes((current) =>
          persistTerminalPanesForWorkRoot(rootId, {
            ...current,
            [pane.logicalKey]: pane,
          }),
        );
        setTerminalPaneOrderByGroup((current) =>
          placeTerminalSessions(
            current,
            terminalPanes,
            [session],
            workbenchGroups,
            paneOrderByGroup,
          ),
        );
        setFocusedTerminalPaneId(pane.paneId);
        setActiveTerminalPaneRequest({
          paneId: pane.paneId,
          sequence: terminalOpenSequence.current++,
        });
      })
      .catch(() => undefined);
  }

  function updateTerminalSocketStatus(
    pane: TerminalPaneState,
    socketStatus: TerminalPaneState["socketStatus"],
    error: string | null = null,
  ) {
    setTerminalPanes((current) =>
      current[pane.logicalKey]
        ? {
            ...current,
            [pane.logicalKey]: markTerminalSocketStatus(
              current[pane.logicalKey],
              socketStatus,
              error,
            ),
          }
        : current,
    );
  }

  function applyTerminalSocketMessage(
    pane: TerminalPaneState,
    message: TerminalWebSocketServerMessage,
  ) {
    if (message.type === "output") {
      return;
    }
    setTerminalPanes((current) =>
      current[pane.logicalKey]
        ? {
            ...current,
            [pane.logicalKey]: appendTerminalWebSocketMessage(
              current[pane.logicalKey],
              message,
            ),
          }
        : current,
    );
  }

  function acceptTerminalSocketResize(
    pane: TerminalPaneState,
    columns: number,
    rows: number,
  ) {
    setTerminalPanes((current) =>
      current[pane.logicalKey]
        ? {
            ...current,
            [pane.logicalKey]: {
              ...current[pane.logicalKey],
              session: { ...current[pane.logicalKey].session, columns, rows },
            },
          }
        : current,
    );
  }

  function sendTerminalData(pane: TerminalPaneState, data: string) {
    // Raw emulator input flows straight to the daemon terminal session; the
    // emulator already delivers Enter as `\r`, so no line buffering is needed.
    void sendTerminalInput(pane.session.terminalId, data).catch((error) => {
      setTerminalPanes((current) =>
        current[pane.logicalKey]
          ? {
              ...current,
              [pane.logicalKey]: {
                ...current[pane.logicalKey],
                error:
                  error instanceof Error
                    ? error.message
                    : "terminal input failed",
              },
            }
          : current,
      );
    });
  }

  function forwardTerminalResize(
    pane: TerminalPaneState,
    columns: number,
    rows: number,
  ): Promise<void> {
    // Bounded resize forwarding: the emulator debounces fit() output before
    // calling this, so logical PTY columns/rows are not rewritten on every
    // visual drag frame. The rejection is propagated (not swallowed) so the
    // caller does not record a failed resize as the last forwarded size.
    return resizeTerminal(pane.session.terminalId, columns, rows).then(
      (session) => {
        setTerminalPanes((current) =>
          current[pane.logicalKey]
            ? {
                ...current,
                [pane.logicalKey]: { ...current[pane.logicalKey], session },
              }
            : current,
        );
      },
    );
  }

  function closeTerminalPane(pane: TerminalPaneState) {
    void closeTerminal(pane.session.terminalId)
      .then(() =>
        setTerminalPanes((current) =>
          persistTerminalPanesForWorkRoot(
            pane.session.workRootId,
            removeClosedTerminalPane(current, pane.logicalKey),
          ),
        ),
      )
      .catch((error) => {
        setTerminalPanes((current) =>
          markTerminalPaneCloseError(
            current,
            pane.logicalKey,
            error instanceof Error ? error.message : "terminal close failed",
          ),
        );
      });
  }

  function closeReadOnlyFilePane(paneId: string) {
    onReadOnlyFilePanesChange((current) =>
      Object.fromEntries(
        Object.entries(current).filter(([, pane]) => pane.id !== paneId),
      ),
    );
    onReadOnlyFilePaneOrderByGroupChange((current) =>
      removePaneFromOrder(current, paneId),
    );
  }

  function closeAgentPane(paneId: string) {
    if (!selectedWorkRootId) {
      return;
    }
    setClosedAgentPaneByRoot((current) => ({
      ...current,
      [selectedWorkRootId]: [
        ...new Set([...(current[selectedWorkRootId] ?? []), paneId]),
      ],
    }));
  }

  function performWorkbenchPaneClose(
    request: DockviewTabCloseRequest & { readonly workRootId?: string },
  ) {
    if (request.workRootId && request.workRootId !== selectedWorkRootId) {
      return;
    }
    const pane = editorGroups
      .flatMap((group) => group.panes)
      .find((candidate) => candidate.id === request.paneId);
    if (!pane) {
      return;
    }

    const closeDecision = decideSurfaceClose(pane.kind);
    if (pane.kind === "persistentTerminal") {
      const terminalPane = Object.values(terminalPanes).find(
        (candidate) => candidate.paneId === pane.id,
      );
      if (terminalPane) {
        closeTerminalPane(terminalPane);
      }
    } else if (pane.kind === "editor") {
      closeReadOnlyFilePane(pane.id);
    } else if (pane.kind === "agent") {
      closeAgentPane(pane.id);
    } else if (pane.kind === "workRootActivity") {
      closeActivityPane(pane.id);
    }

    if (closeDecision.terminateReservation) {
      setFocusedTerminalPaneId((current) =>
        current === pane.id ? null : current,
      );
    }
    setActivePaneByGroupForSelected((current) =>
      reconcileActiveWorkbenchPanes(
        editorGroups.map((group) => ({
          ...group,
          panes: group.panes.filter((candidate) => candidate.id !== pane.id),
        })),
        current,
      ),
    );
  }

  // Phase 1 command-spine audit: workbench close/select/move stay on the
  // Dockview lifecycle callbacks for now. They carry confirmation anchors,
  // drag placement details, and active-pane reconciliation state that would make
  // this slice a larger lifecycle refactor; Activity Console is not blocked by
  // these local callbacks.
  function requestWorkbenchPaneClose(request: DockviewTabCloseRequest) {
    const decision = decideWorkbenchTabClosePresentation(request.surfaceKind, {
      clientX: request.clientX,
      clientY: request.clientY,
    });
    if (decision.type === "requestConfirmation") {
      if (!selectedWorkRootId) {
        return;
      }
      setPendingCloseRequest({
        ...request,
        anchor: decision.anchor,
        workRootId: selectedWorkRootId,
      });
      return;
    }
    performWorkbenchPaneClose(request);
  }

  const movePane = (
    paneId: string,
    targetGroupId: string,
    beforePaneId?: string,
    dynamicTargetGroup?: { targetGroupId: string; targetGroupLabel?: string },
  ) => {
    const result = commitWorkbenchPaneMoveIntoDynamicGroup(
      editorGroups,
      activePaneByGroup,
      {
        paneId,
        targetGroupId,
        beforePaneId,
        dynamicTargetGroup,
      },
    );
    if (workbenchModel) {
      onWorkbenchGroupsByRootChange((currentByRoot) => ({
        ...currentByRoot,
        [workbenchModel.root.id]: result.groups.map((group, index) => ({
          id: group.id,
          label:
            group.label ??
            workbenchGroups.find((candidate) => candidate.id === group.id)
              ?.label ??
            `group ${index + 1}`,
        })),
      }));
      onPaneOrderByRootChange((currentByRoot) => ({
        ...currentByRoot,
        [workbenchModel.root.id]: result.paneOrderByGroup,
      }));
    }
    setActivePaneByGroupForSelected(result.activePaneByGroup);
  };

  const selectPane = (groupId: string, paneId: string) => {
    const pane = editorGroups
      .flatMap((group) => group.panes)
      .find((candidate) => candidate.id === paneId);
    setFocusedTerminalPaneId(
      pane?.kind === "persistentTerminal" ? paneId : null,
    );
    setActivePaneByGroupForSelected((current) =>
      selectWorkbenchPane(current, groupId, paneId),
    );
  };

  function openWorkRootActivityPane() {
    if (!workbenchModel) {
      return;
    }
    // CONTRACT: The top-bar Activity badge is the selected-workRoot entrypoint
    // for one reversible WorkRoot Activity pane. Routing through
    // decideSurfaceOpenWithDynamicGroups keeps duplicate opens focusing the
    // existing pane and new opens using policy-owned support-split placement
    // instead of a raw Dockview handle.
    const rootId = workbenchModel.root.id;
    const paneId = workRootActivityPaneId(rootId);
    const decision = decideSurfaceOpenWithDynamicGroups(
      workRootActivityPlacementState(workbenchGroups, editorGroups, rootId),
      {
        surfaceKind: "workRootActivity",
        logicalKey: workRootActivityPaneLogicalKey(rootId),
        attachmentId:
          paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
      },
    );
    if (decision.type === "openNew") {
      if (decision.createdGroupId) {
        onWorkbenchGroupsByRootChange((current) => ({
          ...current,
          [rootId]: reconcileDashboardGroupsForPlacement(
            current[rootId] ?? workbenchGroups,
            decision,
          ),
        }));
      }
      setActivityPaneOpenByRoot((current) => ({ ...current, [rootId]: true }));
      onPaneOrderByRootChange((current) => ({
        ...current,
        [rootId]: addPaneToGroupOrder(
          current[rootId] ?? {},
          paneId,
          String(decision.groupId),
        ),
      }));
    }
    setActivePaneByGroupForSelected((current) =>
      selectWorkbenchPane(current, decision.groupId, paneId),
    );
  }

  function closeActivityPane(paneId: string) {
    // CONTRACT: closing the WorkRoot Activity pane only detaches the browser
    // view. It is reversible and daemon-owned, so no daemon named-agent state
    // changes here.
    if (!selectedWorkRootId) {
      return;
    }
    setActivityPaneOpenByRoot((current) => ({
      ...current,
      [selectedWorkRootId]: false,
    }));
    onPaneOrderByRootChange((current) => ({
      ...current,
      [selectedWorkRootId]: removePaneFromOrder(
        current[selectedWorkRootId] ?? {},
        paneId,
      ),
    }));
  }

  if (loading && !resources) {
    return <StatusPane title="Loading" detail="workbench resources" />;
  }

  if (error && !resources) {
    return <StatusPane title="Workbench unavailable" detail={error} />;
  }

  if (!resources || !workbenchModel) {
    return (
      <StatusPane
        title="No workRoot"
        detail="select a workRoot or main instance"
      />
    );
  }

  const { workspace, root } = workbenchModel;
  const activityBadge = workRootActivityBadge(
    workRootActivityState.rootId === root.id
      ? workRootActivityState.activity
      : { phase: "loading" },
  );

  return (
    <div className="workbench-shell">
      <WorkbenchToolbar
        activity={activityBadge}
        commandLog={commandLog}
        root={root}
        selectedEntity={selectedEntity}
        server={resources.server}
        workspace={workspace}
        onCommand={onCommand}
        onOpenActivity={openWorkRootActivityPane}
        onCreateTerminal={createTerminalPane}
      />
      {error ? (
        <InlineNotice tone="error" title="Refresh failed" detail={error} />
      ) : null}
      {loading ? (
        <InlineNotice tone="info" title="Refreshing" detail="resources" />
      ) : null}
      <DockviewWorkbenchLayout
        activePaneByGroup={activePaneByGroup}
        groups={editorGroups}
        onMovePane={movePane}
        onRequestClosePane={requestWorkbenchPaneClose}
        onSelectPane={selectPane}
      />
      {pendingCloseRequest ? (
        <WorkbenchClosePopover
          request={pendingCloseRequest}
          onCancel={() => {
            setPendingCloseRequest(null);
          }}
          onConfirm={() => {
            const request = pendingCloseRequest;
            setPendingCloseRequest(null);
            performWorkbenchPaneClose(request);
          }}
        />
      ) : null}
    </div>
  );
}

function WorkbenchClosePopover({
  request,
  onCancel,
  onConfirm,
}: {
  request: DockviewTabCloseRequest & {
    readonly anchor: { readonly clientX: number; readonly clientY: number };
    readonly workRootId: string;
  };
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div
      className="workbench-close-popover"
      data-workbench-close-popover="cursor-near"
      data-workbench-pane-id={request.paneId}
      role="dialog"
      style={{
        left: request.anchor.clientX,
        top: request.anchor.clientY,
      }}
    >
      <div className="workbench-close-popover-title">Close session?</div>
      <div className="workbench-close-popover-actions">
        <button
          className="action-button"
          data-command-id="workbench.tab.close.cancel"
          type="button"
          onClick={onCancel}
        >
          No
        </button>
        <button
          className="action-button action-button-primary"
          data-command-id="workbench.tab.close.confirm"
          type="button"
          onClick={onConfirm}
        >
          Yes
        </button>
      </div>
    </div>
  );
}

function WorkbenchToolbar({
  server,
  workspace,
  root,
  selectedEntity,
  commandLog,
  activity,
  onCommand,
  onOpenActivity,
  onCreateTerminal,
}: {
  server: ServerView;
  workspace: WorkspaceView;
  root: WorkRootView;
  selectedEntity: ResourceEntity | null;
  commandLog: CommandEntry[];
  activity: WorkRootActivityBadgeView;
  onCommand: DashboardCommandDispatcher;
  onOpenActivity: () => void;
  onCreateTerminal: () => void;
}) {
  const toggles = [
    "viewer",
    "task",
    "diagnostics",
    "events",
    "layout",
  ] as const;

  return (
    <div className="workbench-toolbar">
      <div className="workbench-breadcrumb" aria-label="Workbench breadcrumb">
        <span>{server.label}</span>
        <span>{workspace.label}</span>
        <strong>{root.label}</strong>
      </div>
      <div className="workbench-toolbar-meta">
        <StateBadge state={root.state} />
        <WorkbenchActivityBadge
          activity={activity}
          onOpenActivity={() => {
            onCommand(
              buildWorkbenchOpenActivityCommand(root.id),
              { "workbench.openActivity": onOpenActivity },
            );
          }}
        />
        <span className="meta-chip">{kindLabel(root.kind)}</span>
        <span className="meta-chip">availability: {root.availability}</span>
        <span className="meta-chip">activation: {root.activation}</span>
        {commandLog[0] ? (
          <span className="meta-chip">last: {commandLog[0].commandId}</span>
        ) : null}
      </div>
      <div
        className="workbench-toolbar-actions"
        aria-label="Workbench toggles and actions"
      >
        {toolbarActions(root, selectedEntity).map(({ action, entityId }) => (
          <button
            className="action-button"
            data-command-id={
              activationForAction(action.id)
                ? "workRoot.activation.set"
                : action.id === "refresh"
                  ? "dashboard.refresh"
                  : `resource.action.${action.id}`
            }
            disabled={!action.enabled}
            key={`${entityId}:${action.id}`}
            type="button"
            onClick={() => {
              const activation = activationForAction(action.id);
              if (activation) {
                onCommand(buildWorkRootActivationCommand(entityId, activation));
                return;
              }
              onCommand(
                action.id === "refresh"
                  ? buildDashboardRefreshCommand()
                  : {
                      commandId: `resource.action.${action.id}`,
                      payload: { type: "action", label: action.label, entityId },
                    },
              );
            }}
          >
            {action.label}
          </button>
        ))}
        <button
          className="action-button workbench-toggle"
          disabled={root.activation !== "online" || root.availability !== "available"}
          data-command-id="terminal.create"
          type="button"
          onClick={() => {
            onCommand(
              buildTerminalCreateCommand(root.id),
              { "terminal.create": onCreateTerminal },
            );
          }}
        >
          New terminal
        </button>
        {toggles.map((toggle) => (
          <button
            className="action-button workbench-toggle"
            data-command-id={`workbench.toggle.${toggle}`}
            key={toggle}
            type="button"
            onClick={() =>
              onCommand({
                commandId: `workbench.toggle.${toggle}`,
                payload: { type: "action", label: toggle, entityId: root.id },
              })
            }
          >
            {toggle}
          </button>
        ))}
      </div>
    </div>
  );
}

function WorkbenchActivityBadge({
  activity,
  onOpenActivity,
}: {
  activity: WorkRootActivityBadgeView;
  onOpenActivity: () => void;
}) {
  // CONTRACT: Phase 2 renders a compact named-agent summary chip inside the
  // existing toolbar metadata row. It is a summary/entrypoint only: no detail
  // pane, agent controls, or row diagnostics live here.
  // CONTRACT: Phase 3 turns this entrypoint into the only top-bar opener for a
  // selected-workRoot Activity pane. The click handler must route through
  // dashboard workbench placement policy, focus duplicate panes, and keep the
  // pane reversible/read-only.
  return (
    <button
      className={`meta-chip workbench-activity-badge workbench-activity-badge-${activity.tone}`}
      data-command-id="workbench.openActivity"
      data-activity-tone={activity.tone}
      type="button"
      title={activity.title}
      aria-label={`Open WorkRoot Activity: ${activity.title}`}
      onClick={onOpenActivity}
    >
      <span className="workbench-activity-badge-dot" aria-hidden="true" />
      <span className="workbench-activity-badge-label">{activity.label}</span>
      {activity.summary ? (
        <span className="workbench-activity-badge-summary">
          {activity.summary}
        </span>
      ) : null}
    </button>
  );
}

function workRootActivityPaneLogicalKey(workRootId: string) {
  return surfaceLogicalKey("workRootActivity", workRootId);
}

function workRootActivityPaneId(workRootId: string) {
  return `workRootActivity-pane:${workRootId}`;
}

// Build the placement state the WorkRoot Activity badge feeds into
// decideSurfaceOpenWithDynamicGroups. It mirrors the live editor groups so a
// duplicate open focuses the pane in whatever group it currently occupies,
// while a first open resolves through the policy-owned support-split target.
function workRootActivityPlacementState(
  groups: ReadonlyArray<{ id: string; label: string }>,
  editorGroups: WorkbenchEditorGroupModel[],
  workRootId: string,
): WorkbenchPlacementState {
  const dashboardGroups = groups.length > 0 ? groups : initialWorkbenchGroups;
  const paneId = workRootActivityPaneId(workRootId);
  const owningGroup = editorGroups.find((group) =>
    group.panes.some((pane) => pane.id === paneId),
  );
  return {
    groups: dashboardGroups.map((group) => ({
      groupId: workbenchGroupId(group.id),
    })),
    focusedGroupId: workbenchGroupId(dashboardGroups[0]?.id ?? "group-1"),
    attachments: owningGroup
      ? [
          {
            attachmentId:
              paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
            groupId: workbenchGroupId(owningGroup.id),
            surfaceKind: "workRootActivity",
            logicalKey: workRootActivityPaneLogicalKey(workRootId),
          },
        ]
      : [],
  };
}

function workRootActivityWorkbenchPane(
  root: WorkRootView,
  activity: WorkRootActivityBadgeInput,
  transcriptRefresh: ActivityTranscriptRefreshSignal | null,
  onCommand: DashboardCommandDispatcher,
): WorkbenchPane {
  const ready = activity.phase === "ready" ? activity.view : null;
  const state: ViewState = {
    status:
      activity.phase === "loading"
        ? "loading"
        : activity.phase === "error"
          ? "unavailable"
          : (ready?.status ?? "ok"),
    loading: activity.phase === "loading",
    stale: false,
    error: activity.phase === "error" ? "activity unavailable" : null,
  };
  const meta =
    ready !== null
      ? [
          `${ready.summary.total} agents`,
          `${ready.summary.active} active`,
          "read-only",
        ]
      : [activity.phase, "read-only"];
  return {
    id: workRootActivityPaneId(root.id),
    kind: "workRootActivity",
    category: "opened",
    title: "WorkRoot Activity",
    detail: `${root.label} activity console`,
    state,
    meta,
    contentRevision: workRootActivityPaneRevision(activity, transcriptRefresh),
    body: (
      <WorkRootActivityPane
        activity={activity}
        onCommand={onCommand}
        transcriptRefresh={transcriptRefresh}
      />
    ),
  };
}

type ActivityTranscriptRefreshSignal = {
  readonly rootId: string;
  readonly activityId: string;
  readonly cursor: string | null;
  readonly sequence: number;
};

function WorkRootActivityPane({
  activity,
  onCommand,
  transcriptRefresh,
}: {
  activity: WorkRootActivityBadgeInput;
  onCommand: DashboardCommandDispatcher;
  transcriptRefresh: ActivityTranscriptRefreshSignal | null;
}) {
  // CONTRACT: A reversible read-only Activity Console projection. It consumes
  // source-neutral feed items/transcripts, exposes command-routed controls, and
  // offers no agent/exec control actions or daemon-side acknowledgement.
  return (
    <section className="workroot-activity-pane" aria-label="WorkRoot Activity">
      {activity.phase === "loading" ? (
        <div className="workroot-activity-state">
          Loading workRoot activity
        </div>
      ) : activity.phase === "error" ? (
        <div className="workroot-activity-state workroot-activity-state-error">
          WorkRoot activity is unavailable
        </div>
      ) : (
        <ActivityConsole
          view={activity.view}
          onCommand={onCommand}
          transcriptRefresh={transcriptRefresh}
        />
      )}
    </section>
  );
}

function workRootActivityPaneRevision(
  activity: WorkRootActivityBadgeInput,
  transcriptRefresh: ActivityTranscriptRefreshSignal | null,
) {
  if (activity.phase !== "ready") {
    return `activity:${activity.phase}`;
  }
  const view = activity.view;
  return [
    "activity",
    view.status,
    view.updateMode,
    view.feedCursor ?? "",
    view.selectedItemId ?? "",
    view.items.length,
    transcriptRefresh?.activityId ?? "",
    transcriptRefresh?.cursor ?? "",
    transcriptRefresh?.sequence ?? 0,
  ].join(":");
}

function readOnlyFilePaneRevision(pane: ReadOnlyFilePane) {
  return [
    "readonly",
    pane.status,
    pane.path,
    pane.sizeBytes ?? "",
    pane.languageHint ?? "",
    pane.extension ?? "",
    pane.error ?? "",
    hashText(pane.content),
  ].join(":");
}

function hashText(value: string) {
  let hash = 5381;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 33) ^ value.charCodeAt(index);
  }
  return `${value.length}:${(hash >>> 0).toString(36)}`;
}

function toolbarActions(
  root: WorkRootView,
  selectedEntity: ResourceEntity | null,
) {
  const actions = root.actions.map((action) => ({ action, entityId: root.id }));

  if (selectedEntity && selectedEntity.id !== root.id) {
    actions.push(
      ...selectedEntity.actions.map((action) => ({
        action,
        entityId: selectedEntity.id,
      })),
    );
  }

  return actions;
}

function activationForAction(actionId: string): "online" | "offline" | null {
  if (actionId === "workRoot.activation.online") {
    return "online";
  }
  if (actionId === "workRoot.activation.offline") {
    return "offline";
  }
  return null;
}

type WorkbenchPane = {
  readonly id: string;
  readonly kind: SurfaceKind;
  readonly category: WorkbenchPaneCategory;
  readonly title: string;
  readonly detail: string;
  readonly state: ViewState;
  readonly meta: readonly string[];
  readonly contentRevision?: string;
  readonly body?: ReactNode;
};

type WorkbenchEditorGroupModel = {
  readonly id: string;
  readonly label: string;
  readonly panes: readonly WorkbenchPane[];
};

function buildWorkbenchEditorGroups(
  root: WorkRootView,
  groups: ReadonlyArray<{ id: string; label: string }>,
  mainInstance: InstanceView | null,
  selectedInstance: InstanceView | null,
  supportEntity: ResourceEntity | null,
  readOnlyFilePanes: ReadOnlyFilePane[],
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder,
  activityPaneOrderByGroup: WorkbenchPaneOrder,
  terminalPanes: TerminalPaneState[],
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
  terminalActions: TerminalPaneActions,
  closedAgentPaneIds: readonly string[] = [],
  activityPaneOpen = false,
  activityState: WorkRootActivityBadgeInput = { phase: "loading" },
  activityTranscriptRefresh: ActivityTranscriptRefreshSignal | null,
  onCommand: DashboardCommandDispatcher,
): WorkbenchEditorGroupModel[] {
  void selectedInstance;
  void supportEntity;
  const dashboardGroups = groups.length > 0 ? groups : initialWorkbenchGroups;
  const readOnlyPanesByGroup = readOnlyWorkbenchPanesByGroup(
    root,
    readOnlyFilePanes,
    readOnlyFilePaneOrderByGroup,
    dashboardGroups,
  );
  const terminalPanesByGroup = terminalWorkbenchPanesByGroup(
    root,
    terminalPanes,
    terminalPaneOrderByGroup,
    terminalActions,
    dashboardGroups,
  );
  const closedAgentPaneIdSet = new Set(closedAgentPaneIds);
  const agentPane: WorkbenchPane[] =
    mainInstance && !closedAgentPaneIdSet.has("main-agent")
      ? [
          {
            id: "main-agent",
            kind: "agent",
            category: "pinned",
            title: mainInstance.label,
            detail: instanceSummary(mainInstance),
            state: mainInstance.state,
            meta: [
              mainInstance.kind,
              mainInstance.interactionMode,
              closeContractLabel("agent"),
            ],
          },
        ]
      : [];

  const activityPane = activityPaneOpen
    ? workRootActivityWorkbenchPane(
        root,
        activityState,
        activityTranscriptRefresh,
        onCommand,
      )
    : null;
  const activityGroupId = activityPane
    ? activityPaneGroupIdFromOrder(
        activityPane.id,
        activityPaneOrderByGroup,
        dashboardGroups,
      )
    : null;

  return dashboardGroups.map((group, index) => ({
    id: group.id,
    label: group.label,
    panes: [
      ...(index === 0 ? agentPane : []),
      ...(terminalPanesByGroup[group.id] ?? []),
      ...(activityPane && activityGroupId === group.id ? [activityPane] : []),
      ...(readOnlyPanesByGroup[group.id] ?? []),
    ],
  }));
}

function placeTerminalSessions(
  current: WorkbenchPaneOrder,
  existingPanes: Record<string, TerminalPaneState>,
  sessions: TerminalSessionView[],
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
): WorkbenchPaneOrder {
  let next = { ...current };
  let placementState = terminalPlacementState(
    existingPanes,
    groups,
    workbenchPaneOrderByGroup,
    current,
  );
  for (const session of sessions) {
    const decision = decideSurfaceOpenWithDynamicGroups(placementState, {
      surfaceKind: "persistentTerminal",
      logicalKey: surfaceLogicalKey(
        "persistentTerminal",
        session.workRootId,
        session.terminalId,
      ),
    });
    if (decision.type === "openNew") {
      const pane = terminalPaneFromSession(session);
      next = {
        ...next,
        [decision.groupId]: [...(next[decision.groupId] ?? []), pane.paneId],
      };
      placementState = {
        ...placementState,
        attachments: [
          ...placementState.attachments,
          {
            attachmentId:
              pane.paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
            groupId: decision.groupId,
            surfaceKind: "persistentTerminal",
            logicalKey: decision.logicalKey,
          },
        ],
      };
    }
  }
  return next;
}

function terminalPlacementState(
  panesByLogicalKey: Record<string, TerminalPaneState>,
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
): WorkbenchPlacementState {
  const firstGroupId = groups[0]?.id ?? "group-1";
  return {
    groups: groups.map((group) => ({ groupId: workbenchGroupId(group.id) })),
    focusedGroupId: workbenchGroupId(firstGroupId),
    attachments: Object.values(panesByLogicalKey).map((pane) => ({
      attachmentId:
        pane.paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
      groupId: workbenchGroupId(
        groupIdForPaneOrder(
          pane.paneId,
          workbenchPaneOrderByGroup,
          terminalPaneOrderByGroup,
          firstGroupId,
        ),
      ),
      surfaceKind: "persistentTerminal",
      logicalKey: surfaceLogicalKey(
        "persistentTerminal",
        pane.session.workRootId,
        pane.session.terminalId,
      ),
    })),
  };
}

function terminalWorkbenchPanesByGroup(
  root: WorkRootView,
  terminalPanes: TerminalPaneState[],
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
  terminalActions: TerminalPaneActions,
  groups: ReadonlyArray<{ id: string; label: string }>,
): Record<string, WorkbenchPane[]> {
  const panes = terminalPanes
    .filter((pane) => pane.session.workRootId === root.id)
    .map((pane) => terminalWorkbenchPane(pane, terminalActions));
  const paneById = new Map(panes.map((pane) => [pane.id, pane]));
  const consumed = new Set<string>();
  const byGroup: Record<string, WorkbenchPane[]> = Object.fromEntries(
    groups.map((group) => [group.id, []]),
  );
  for (const groupId of groups.map((group) => group.id)) {
    for (const paneId of terminalPaneOrderByGroup[groupId] ?? []) {
      const pane = paneById.get(paneId);
      if (pane && !consumed.has(paneId)) {
        byGroup[groupId].push(pane);
        consumed.add(paneId);
      }
    }
  }
  for (const pane of panes) {
    if (!consumed.has(pane.id))
      (byGroup[groups[0]?.id ?? "group-1"] ??= []).push(pane);
  }
  return byGroup;
}

type TerminalPaneActions = {
  onSendData: (pane: TerminalPaneState, data: string) => void;
  onClose: (pane: TerminalPaneState) => void;
  onResize: (
    pane: TerminalPaneState,
    columns: number,
    rows: number,
  ) => Promise<void>;
  onSocketStatus: (
    pane: TerminalPaneState,
    socketStatus: TerminalPaneState["socketStatus"],
    error?: string | null,
  ) => void;
  onSocketMessage: (
    pane: TerminalPaneState,
    message: TerminalWebSocketServerMessage,
  ) => void;
  onSocketResize: (
    pane: TerminalPaneState,
    columns: number,
    rows: number,
  ) => void;
  onFocusInput: (pane: TerminalPaneState) => void;
  isActivePane: (pane: TerminalPaneState) => boolean;
};

function terminalWorkbenchPane(
  pane: TerminalPaneState,
  actions: TerminalPaneActions,
): WorkbenchPane {
  const state: ViewState = {
    status: pane.session.status,
    loading: pane.session.status === "starting",
    stale: false,
    error: pane.error,
  };
  return {
    id: pane.paneId,
    kind: "persistentTerminal",
    category: "pinned",
    title: pane.session.title,
    detail: pane.session.terminalId,
    state,
    meta: [
      pane.session.status,
      pane.socketStatus,
      `${pane.session.columns}x${pane.session.rows}`,
    ],
    contentRevision: `terminal:${pane.paneId}`,
    body: <TerminalPaneBody key={pane.paneId} pane={pane} actions={actions} />,
  };
}

function TerminalPaneBody({
  pane,
  actions,
}: {
  pane: TerminalPaneState;
  actions: TerminalPaneActions;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const writtenLengthRef = useRef(0);
  const lastForwardedSizeRef = useRef<{ columns: number; rows: number } | null>(
    null,
  );
  const socketRef = useRef<WebSocket | null>(null);
  const keepTerminalFocusRef = useRef(false);
  const [displaySession, setDisplaySession] = useState(() => pane.session);
  // Latest pane/actions for emulator callbacks registered once at mount.
  const liveRef = useRef({ pane, actions });
  liveRef.current = { pane, actions };

  const terminalId = pane.session.terminalId;
  const refocusActiveTerminal = () => {
    window.setTimeout(() => {
      if (keepTerminalFocusRef.current && containerRef.current?.offsetParent) {
        terminalRef.current?.focus();
        containerRef.current
          ?.querySelector<HTMLTextAreaElement>(".xterm-helper-textarea")
          ?.focus();
      }
    }, 0);
  };

  useEffect(() => {
    setDisplaySession(pane.session);
  }, [pane.session]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }

    const terminal = new Terminal({
      cursorBlink: true,
      // Prefer Powerline/Nerd Font capable families so prompt glyphs render
      // correctly, falling back to plain monospace when none are installed.
      fontFamily:
        '"MesloLGS NF", "JetBrainsMono Nerd Font", "CaskaydiaCove Nerd Font", ' +
        '"FiraCode Nerd Font", "Hack Nerd Font", ui-monospace, SFMono-Regular, ' +
        'Menlo, Consolas, "Liberation Mono", monospace',
      fontSize: 12,
      theme: { background: "#0b0d10" },
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(container);
    terminalRef.current = terminal;
    writtenLengthRef.current = 0;

    // Replay PTY output buffered before this surface mounted so reselecting a
    // terminal tab restores its emulator contents.
    const initialOutput = liveRef.current.pane.output;
    if (initialOutput.length > 0) {
      terminal.write(initialOutput);
      writtenLengthRef.current = initialOutput.length;
    }

    // Keyboard input originates from the focused emulator surface and reaches
    // the daemon terminal session.
    const sendInputBytes = (data: string) => {
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: "input", data }));
        refocusActiveTerminal();
        return;
      }
      liveRef.current.actions.onSendData(liveRef.current.pane, data);
      refocusActiveTerminal();
    };

    const inputDisposable = terminal.onData(sendInputBytes);
    let composingInput = false;
    const markComposing = () => {
      composingInput = true;
    };
    const clearComposing = () => {
      composingInput = false;
    };
    const markFocusedTerminal = () => {
      keepTerminalFocusRef.current = true;
      liveRef.current.actions.onFocusInput(liveRef.current.pane);
      terminal.focus();
    };
    const clearFocusedTerminal = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && !container.contains(target)) {
        keepTerminalFocusRef.current = false;
      }
    };
    const clearFocusedTerminalOnOutsideFocus = (event: FocusEvent) => {
      const target = event.target as Node | null;
      if (target && !container.contains(target)) {
        keepTerminalFocusRef.current = false;
      }
    };
    container.addEventListener("compositionstart", markComposing);
    container.addEventListener("compositionend", clearComposing);
    container.addEventListener("focusin", markFocusedTerminal);
    container.addEventListener("pointerdown", markFocusedTerminal);
    window.addEventListener("pointerdown", clearFocusedTerminal, true);
    window.addEventListener("focusin", clearFocusedTerminalOnOutsideFocus, true);

    const keydownFallback = (event: KeyboardEvent) => {
      if (!container.offsetParent) {
        return;
      }
      if (!liveRef.current.actions.isActivePane(liveRef.current.pane)) {
        return;
      }
      if (event.isComposing || event.key === "Process" || composingInput) {
        return;
      }
      const isMetaLineStart = event.metaKey && event.key.toLowerCase() === "a";
      if (container.contains(document.activeElement) && !isMetaLineStart) {
        return;
      }
      const target = event.target as HTMLElement | null;
      const tagName = target?.tagName.toLowerCase();
      if (
        target?.isContentEditable ||
        tagName === "input" ||
        tagName === "textarea" ||
        tagName === "select"
      ) {
        return;
      }
      let data: string | null = null;
      if (event.ctrlKey || event.metaKey) {
        const key = event.key.toLowerCase();
        if (key === "c") data = "\x03";
        if (key === "l") data = "\x0c";
        if (key === "a") data = "\x01";
        // Native shell line-editing controls: ctrl-u clears the current line
        // and ctrl-w deletes the previous word. Dockview does not reliably keep
        // the xterm helper textarea focused, so this fallback forwards the same
        // raw control bytes xterm's onData path would send when it is focused.
        if (key === "u") data = "\x15";
        if (key === "w") data = "\x17";
      } else if (event.key.length === 1) {
        data = event.key;
      } else if (event.key === "Enter") {
        data = "\r";
      } else if (event.key === "Backspace") {
        data = "\x7f";
      } else if (event.key === "ArrowLeft") {
        data = "\x1b[D";
      } else if (event.key === "ArrowRight") {
        data = "\x1b[C";
      } else if (event.key === "ArrowUp") {
        data = "\x1b[A";
      } else if (event.key === "ArrowDown") {
        data = "\x1b[B";
      }
      if (data !== null) {
        event.preventDefault();
        liveRef.current.actions.onFocusInput(liveRef.current.pane);
        sendInputBytes(data);
      }
    };
    window.addEventListener("keydown", keydownFallback);

    const fitNow = () => {
      try {
        fitAddon.fit();
      } catch {
        /* container not measurable yet */
        return;
      }
      // Cap the emulator grid to the PTY size contract so the emulator and the
      // daemon-owned logical PTY size never disagree on very wide/tall panes.
      const capped = clampTerminalSize(terminal.cols, terminal.rows);
      if (capped.columns !== terminal.cols || capped.rows !== terminal.rows) {
        terminal.resize(capped.columns, capped.rows);
      }
      while (terminal.rows > 1 && !terminalScreenFitsVisibleBox(container)) {
        terminal.resize(terminal.cols, terminal.rows - 1);
      }
    };

    const forwardSize = () => {
      // The emulator grid is already capped to the PTY bounds by fitNow, so
      // this size is always inside the daemon resize contract. When Dockview is
      // stacked below the fold in a narrow viewport, its internal cached width
      // may lag the viewport; still bound the PTY columns to the viewport so
      // the daemon-visible logical size follows responsive relayout.
      const viewportColumns = Math.max(
        1,
        Math.floor((window.innerWidth - 32) / 8),
      );
      const next = clampTerminalSize(
        Math.min(terminal.cols, viewportColumns),
        terminal.rows,
      );
      if (next.columns !== terminal.cols || next.rows !== terminal.rows) {
        terminal.resize(next.columns, next.rows);
      }
      const prev = lastForwardedSizeRef.current;
      if (prev && prev.columns === next.columns && prev.rows === next.rows) {
        return;
      }
      // Record the forwarded size only after the daemon accepts it; a rejected
      // resize must stay retryable rather than being suppressed as a no-op.
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(
          JSON.stringify({
            type: "resize",
            columns: next.columns,
            rows: next.rows,
          }),
        );
        setDisplaySession((current) => ({
          ...current,
          columns: next.columns,
          rows: next.rows,
        }));
        liveRef.current.actions.onSocketResize(
          liveRef.current.pane,
          next.columns,
          next.rows,
        );
        lastForwardedSizeRef.current = next;
        return;
      }
      void liveRef.current.actions
        .onResize(liveRef.current.pane, next.columns, next.rows)
        .then(() => {
          lastForwardedSizeRef.current = next;
        })
        .catch(() => {
          /* leave lastForwardedSizeRef unchanged so the next fit retries */
        });
    };

    fitNow();

    // ResizeObserver keeps the emulator fitted to the pane; resize forwarding
    // to the daemon is debounced so visual split drag does not continuously
    // rewrite logical PTY dimensions.
    let resizeTimer: number | null = null;
    const scheduleResizeForward = () => {
      fitNow();
      if (resizeTimer !== null) {
        window.clearTimeout(resizeTimer);
      }
      resizeTimer = window.setTimeout(() => {
        resizeTimer = null;
        forwardSize();
      }, 250);
    };
    const observer = new ResizeObserver(scheduleResizeForward);
    observer.observe(container);
    window.addEventListener("resize", scheduleResizeForward);
    const focusWatchdog = window.setInterval(() => {
      if (!keepTerminalFocusRef.current) {
        return;
      }
      if (!container.offsetParent) {
        return;
      }
      if (!liveRef.current.actions.isActivePane(liveRef.current.pane)) {
        return;
      }
      if (container.contains(document.activeElement)) {
        return;
      }
      refocusActiveTerminal();
    }, 100);

    return () => {
      observer.disconnect();
      window.removeEventListener("resize", scheduleResizeForward);
      window.clearInterval(focusWatchdog);
      if (resizeTimer !== null) {
        window.clearTimeout(resizeTimer);
      }
      window.removeEventListener("keydown", keydownFallback);
      container.removeEventListener("compositionstart", markComposing);
      container.removeEventListener("compositionend", clearComposing);
      container.removeEventListener("focusin", markFocusedTerminal);
      container.removeEventListener("pointerdown", markFocusedTerminal);
      window.removeEventListener("pointerdown", clearFocusedTerminal, true);
      window.removeEventListener(
        "focusin",
        clearFocusedTerminalOnOutsideFocus,
        true,
      );
      inputDisposable.dispose();
      terminal.dispose();
      terminalRef.current = null;
    };
  }, []);

  useEffect(() => {
    let disposed = false;
    const socket = new WebSocket(
      terminalWebSocketUrl(
        terminalId,
        terminalWebSocketCursor(liveRef.current.pane),
      ),
    );
    socketRef.current = socket;
    liveRef.current.actions.onSocketStatus(
      liveRef.current.pane,
      "connecting",
      null,
    );

    socket.addEventListener("open", () => {
      if (!disposed)
        liveRef.current.actions.onSocketStatus(
          liveRef.current.pane,
          "connected",
          null,
        );
    });
    socket.addEventListener("message", (event) => {
      if (disposed || typeof event.data !== "string") return;
      try {
        const message = JSON.parse(
          event.data,
        ) as TerminalWebSocketServerMessage;
        if (message.type === "output") {
          terminalRef.current?.write(message.chunk.data);
          writtenLengthRef.current += message.chunk.data.length;
          if (liveRef.current.actions.isActivePane(liveRef.current.pane)) {
            refocusActiveTerminal();
          }
        } else {
          setDisplaySession((current) => ({
            ...current,
            status: message.status,
          }));
        }
        liveRef.current.actions.onSocketMessage(liveRef.current.pane, message);
      } catch {
        // Ignore malformed daemon frames and allow the socket close/fallback path to recover.
      }
    });
    socket.addEventListener("error", () => {
      if (!disposed) {
        liveRef.current.actions.onSocketStatus(
          liveRef.current.pane,
          "fallback",
          "terminal WebSocket failed",
        );
      }
    });
    socket.addEventListener("close", () => {
      if (!disposed)
        liveRef.current.actions.onSocketStatus(
          liveRef.current.pane,
          "fallback",
          null,
        );
    });

    return () => {
      disposed = true;
      if (socketRef.current === socket) socketRef.current = null;
      socket.close();
    };
  }, [terminalId]);

  // Stream PTY output deltas into the emulator so ANSI color and control
  // sequences render as terminal behavior rather than raw text.
  useEffect(() => {
    const terminal = terminalRef.current;
    if (!terminal) {
      return;
    }
    if (pane.output.length > writtenLengthRef.current) {
      terminal.write(pane.output.slice(writtenLengthRef.current));
      writtenLengthRef.current = pane.output.length;
    } else if (pane.output.length < writtenLengthRef.current) {
      terminal.clear();
      terminal.write(pane.output);
      writtenLengthRef.current = pane.output.length;
    }
  }, [pane.output]);

  return (
    <div className="terminal-pane" data-terminal-id={terminalId}>
      <div
        className="terminal-surface"
        data-command-id="terminal.input"
        ref={containerRef}
      />
      {pane.error ? <div className="terminal-error">{pane.error}</div> : null}
      <div className="terminal-controls">
        <span className="terminal-status-line">
          {displaySession.status} · {displaySession.columns}x
          {displaySession.rows}
        </span>
        <button
          className="action-button"
          data-command-id="terminal.close"
          type="button"
          onClick={() => actions.onClose(pane)}
        >
          Terminate
        </button>
      </div>
    </div>
  );
}

function terminalScreenFitsVisibleBox(container: HTMLElement) {
  const screen = container.querySelector<HTMLElement>(".xterm-screen");
  if (!screen) {
    return true;
  }
  const containerBox = container.getBoundingClientRect();
  const screenBox = screen.getBoundingClientRect();
  return screenBox.bottom <= containerBox.bottom + 0.5;
}

function readOnlyFilePlacementState(
  panesByLogicalKey: Record<string, ReadOnlyFilePane>,
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder,
): WorkbenchPlacementState {
  const fallbackGroupId = groups[1]?.id ?? groups[0]?.id ?? "group-2";
  return {
    groups: groups.map((group) => ({ groupId: workbenchGroupId(group.id) })),
    attachments: Object.values(panesByLogicalKey).map((pane) => ({
      attachmentId:
        pane.id as WorkbenchPlacementState["attachments"][number]["attachmentId"],
      groupId: workbenchGroupId(
        groupIdForPaneOrder(
          pane.id,
          workbenchPaneOrderByGroup,
          readOnlyFilePaneOrderByGroup,
          fallbackGroupId,
        ),
      ),
      surfaceKind: "editor",
      logicalKey: surfaceLogicalKey(...pane.logicalKey.split("/")),
    })),
  };
}

function sameReadOnlyOpenRequest(
  current: ReadOnlyFilePane | undefined,
  requested: ReadOnlyFilePane,
): current is ReadOnlyFilePane {
  return (
    current !== undefined &&
    current.workRootId === requested.workRootId &&
    current.path === requested.path &&
    current.mode === requested.mode
  );
}

function addPaneToGroupOrder(
  orderByGroup: WorkbenchPaneOrder,
  paneId: string,
  groupId: string,
): WorkbenchPaneOrder {
  const withoutPane = removePaneFromOrder(orderByGroup, paneId);
  return {
    ...withoutPane,
    [groupId]: [...(withoutPane[groupId] ?? []), paneId],
  };
}

function removePaneFromOrder(
  orderByGroup: WorkbenchPaneOrder,
  paneId: string | undefined,
): WorkbenchPaneOrder {
  if (!paneId) {
    return orderByGroup;
  }
  return Object.fromEntries(
    Object.entries(orderByGroup).map(([groupId, paneIds]) => [
      groupId,
      paneIds.filter((candidate) => candidate !== paneId),
    ]),
  );
}

function activityPaneGroupIdFromOrder(
  paneId: string,
  orderByGroup: WorkbenchPaneOrder,
  groups: ReadonlyArray<{ id: string }>,
): string {
  return groupIdForPaneOrder(
    paneId,
    orderByGroup,
    {},
    groups[1]?.id ?? groups[0]?.id ?? "group-1",
  );
}

function groupIdForPaneOrder(
  paneId: string,
  primaryOrderByGroup: WorkbenchPaneOrder,
  fallbackOrderByGroup: WorkbenchPaneOrder,
  fallbackGroupId: string,
): string {
  return (
    Object.entries(primaryOrderByGroup).find(([, paneIds]) =>
      paneIds.includes(paneId),
    )?.[0] ??
    Object.entries(fallbackOrderByGroup).find(([, paneIds]) =>
      paneIds.includes(paneId),
    )?.[0] ??
    fallbackGroupId
  );
}

function readOnlyWorkbenchPanesByGroup(
  root: WorkRootView,
  readOnlyFilePanes: ReadOnlyFilePane[],
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder,
  groups: ReadonlyArray<{ id: string; label: string }>,
): Record<string, WorkbenchPane[]> {
  const panes = readOnlyFilePanes
    .filter((pane) => pane.workRootId === root.id)
    .map((pane) => readOnlyWorkbenchPane(root, pane));
  const paneById = new Map(panes.map((pane) => [pane.id, pane]));
  const consumed = new Set<string>();
  const byGroup: Record<string, WorkbenchPane[]> = Object.fromEntries(
    groups.map((group) => [group.id, []]),
  );

  for (const groupId of groups.map((group) => group.id)) {
    for (const paneId of readOnlyFilePaneOrderByGroup[groupId] ?? []) {
      const pane = paneById.get(paneId);
      if (pane && !consumed.has(paneId)) {
        byGroup[groupId].push(pane);
        consumed.add(paneId);
      }
    }
  }

  for (const pane of panes) {
    if (!consumed.has(pane.id)) {
      (byGroup[groups[1]?.id ?? groups[0]?.id ?? "group-2"] ??= []).push(pane);
    }
  }

  return byGroup;
}

function readOnlyWorkbenchPane(
  root: WorkRootView,
  pane: ReadOnlyFilePane,
): WorkbenchPane {
  const state: ViewState = {
    status: pane.status,
    loading: pane.status === "loading",
    stale: false,
    error: pane.error,
  };
  const meta = [
    pane.mode,
    "read-only",
    pane.languageHint ?? pane.extension ?? "text",
    pane.sizeBytes === null ? "pending" : `${pane.sizeBytes} bytes`,
  ];

  return {
    id: pane.id,
    kind: "editor",
    // Pinned files get the left-border accent matching other stable/pinned
    // surfaces (agent, terminal). Preview files stay in the opened chip style.
    category: pane.mode === "pinned" ? "pinned" : "opened",
    title: pane.title,
    detail: pane.path,
    state,
    meta,
    contentRevision: readOnlyFilePaneRevision(pane),
    body: <ReadOnlyTextPane pane={pane} root={root} />,
  };
}

function ReadOnlyTextPane({
  pane,
  root,
}: {
  pane: ReadOnlyFilePane;
  root: WorkRootView;
}) {
  return (
    <div className="readonly-text-pane">
      <div className="readonly-text-pane-header">
        <div className="readonly-text-pane-title-block">
          <div className="readonly-text-pane-title">{pane.title}</div>
          <div className="readonly-text-pane-path" title={pane.path}>
            {root.label} / {pane.path}
          </div>
        </div>
        <div className="readonly-text-pane-badges">
          <span className="meta-chip">{pane.mode}</span>
          <span className="meta-chip">read-only</span>
          <span className="meta-chip">
            {pane.languageHint ?? pane.extension ?? "text"}
          </span>
        </div>
      </div>
      {pane.status === "loading" ? (
        <div className="readonly-text-pane-state">Loading file content</div>
      ) : pane.status === "error" ? (
        <div className="readonly-text-pane-state readonly-text-pane-error">
          {pane.error ?? "file read failed"}
        </div>
      ) : (
        <pre className="readonly-text-content">
          <code>{pane.content}</code>
        </pre>
      )}
    </div>
  );
}

function SubInstancePane({
  mainInstance,
}: {
  mainInstance: InstanceView | null;
}) {
  if (!mainInstance || mainInstance.subInstances.length === 0) {
    return (
      <p className="workbench-pane-empty">
        No sub instances attached to this main surface.
      </p>
    );
  }

  return (
    <div className="subinstance-list">
      {mainInstance.subInstances.map((instance) => (
        <div className="subinstance-pill" key={instance.id}>
          <span>{instance.label}</span>
          <StateBadge state={instance.state} />
        </div>
      ))}
    </div>
  );
}

function ResourceSummary({ entity }: { entity: ResourceEntity }) {
  return (
    <dl className="resource-summary">
      <DetailItem label="id" value={entity.id} />
      <DetailItem label="status" value={entity.state.status} />
      {entity.type === "workRoot" ? (
        <DetailItem label="workRoot" value={entity.path.workRootId} />
      ) : null}
      {entity.type === "instance" ? (
        <DetailItem label="instance" value={entity.path.instanceId ?? ""} />
      ) : null}
    </dl>
  );
}

function WorkspaceRows({
  workspace,
  selectedId,
  onCommand,
}: {
  workspace: WorkspaceView;
  selectedId: string | null;
  onCommand: DashboardCommandDispatcher;
}) {
  const compactMain = compactMainInstance(workspace);

  if (compactMain) {
    return (
      <div className="resource-group">
        <ResourceRow
          id={compactMain.root.id}
          title={`${workspace.label} / ${compactMain.root.label}`}
          eyebrow="compact workRoot"
          state={compactMain.root.state}
          depth={0}
          selected={selectedId === compactMain.root.id}
          meta={[
            kindLabel(compactMain.root.kind),
            `availability: ${compactMain.root.availability}`,
            `activation: ${compactMain.root.activation}`,
            compactMain.instance.kind,
          ]}
          onCommand={onCommand}
        />
      </div>
    );
  }

  return (
    <div className="resource-group">
      <ResourceRow
        id={workspace.id}
        title={workspace.label}
        eyebrow="workspace"
        state={workspace.state}
        depth={0}
        selected={selectedId === workspace.id}
        meta={[`${workspace.workRoots.length} roots`]}
        onCommand={onCommand}
      />
      {workspace.workRoots.map((root) => (
        <div key={root.id}>
          <ResourceRow
            id={root.id}
            title={root.label}
            eyebrow="workRoot"
            state={root.state}
            depth={1}
            selected={selectedId === root.id}
            meta={[
              kindLabel(root.kind),
              `availability: ${root.availability}`,
              `activation: ${root.activation}`,
            ]}
            onCommand={onCommand}
          />
          {root.mainInstances.length > 0 ? (
            <div className="nav-secondary-context">
              {root.mainInstances.length} pinned main surface
              {root.mainInstances.length === 1 ? "" : "s"}
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function ResourceRow({
  id,
  title,
  eyebrow,
  state,
  depth,
  selected,
  meta,
  onCommand,
}: {
  id: string;
  title: string;
  eyebrow: string;
  state: ViewState;
  depth: number;
  selected: boolean;
  meta: string[];
  onCommand: DashboardCommandDispatcher;
}) {
  return (
    <button
      className={`resource-row${selected ? " resource-row-selected" : ""}`}
      data-command-id="resource.select"
      style={{ "--depth": depth } as CSSProperties}
      type="button"
      onClick={() =>
        onCommand({ commandId: "resource.select", payload: { type: "select", entityId: id } })
      }
    >
      <span className="resource-row-main">
        <span className="row-eyebrow">{eyebrow}</span>
        <span className="row-title">{title}</span>
      </span>
      <span className="resource-row-meta">
        {meta.map((value) => (
          <span className="meta-chip" key={value}>
            {value}
          </span>
        ))}
        <StateBadge state={state} />
      </span>
    </button>
  );
}

function ResourceDetail({
  entity,
  loading,
  error,
}: {
  entity: ResourceEntity | null;
  loading: boolean;
  error: string | null;
}) {
  if (loading && !entity) {
    return <StatusPane title="Loading" detail="detail" />;
  }

  if (error && !entity) {
    return <StatusPane title="Unavailable" detail={error} />;
  }

  if (!entity) {
    return <StatusPane title="Empty" detail="no selection" />;
  }

  return (
    <div className="detail-body">
      <div className="detail-heading">
        <div>
          <div className="detail-type">{entity.type}</div>
          <h1>{entity.label}</h1>
        </div>
        <StateBadge state={entity.state} />
      </div>

      <dl className="detail-grid">
        <DetailItem label="id" value={entity.id} />
        <DetailItem label="status" value={entity.state.status} />
        <DetailItem label="loading" value={String(entity.state.loading)} />
        <DetailItem label="stale" value={String(entity.state.stale)} />
        {entity.type === "workspace" ? (
          <>
            <DetailItem
              label="compactable"
              value={String(entity.compactable)}
            />
            <DetailItem
              label="workRoots"
              value={String(entity.workRootCount)}
            />
          </>
        ) : null}
        {entity.type === "workRoot" ? (
          <>
            <DetailItem label="kind" value={kindLabel(entity.kind)} />
            <DetailItem label="availability" value={entity.availability} />
            <DetailItem label="activation" value={entity.activation} />
            <DetailItem label="workRootStatus" value={entity.status} />
            <DetailItem
              label="instances"
              value={String(entity.instanceCount)}
            />
            <DetailItem label="workRootId" value={entity.path.workRootId} />
          </>
        ) : null}
        {entity.type === "instance" ? (
          <>
            <DetailItem label="role" value={entity.role} />
            <DetailItem label="kind" value={entity.kind} />
            <DetailItem label="mode" value={entity.interactionMode} />
            <DetailItem
              label="subInstances"
              value={String(entity.subInstanceCount)}
            />
            <DetailItem
              label="instanceId"
              value={entity.path.instanceId ?? ""}
            />
          </>
        ) : null}
      </dl>

      {entity.state.error ? (
        <div className="detail-alert" role="status">
          {entity.state.error}
        </div>
      ) : null}
    </div>
  );
}

function ViewerReserve({
  entity,
  commandLog,
}: {
  entity: ResourceEntity | null;
  commandLog: CommandEntry[];
}) {
  return (
    <div className="viewer-body">
      <div className="viewer-target">
        <div className="detail-type">selection</div>
        <div className="viewer-title">{entity?.label ?? "none"}</div>
        <div className="viewer-subtitle">{entity?.type ?? "empty"}</div>
      </div>

      <div className="command-log">
        <div className="section-label">commands</div>
        {commandLog.length === 0 ? (
          <div className="log-empty">none</div>
        ) : (
          commandLog.map((entry) => (
            <div className="log-row" key={entry.id}>
              <span>{entry.commandId}</span>
              <strong>{entry.label}</strong>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function StatusPane({
  title,
  detail,
  action,
}: {
  title: string;
  detail: string;
  action?: ReactNode;
}) {
  return (
    <div className="status-pane">
      <div className="status-title">{title}</div>
      <div className="status-detail">{detail}</div>
      {action ? <div className="status-action">{action}</div> : null}
    </div>
  );
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="detail-item">
      <dt>{label}</dt>
      <dd>{value || "none"}</dd>
    </div>
  );
}

function StateLine({ state }: { state: ViewState }) {
  return (
    <div className="state-line">
      <StateDot state={state} />
      <span>{state.status}</span>
      {state.stale ? <span>stale</span> : null}
      {state.loading ? <span>loading</span> : null}
    </div>
  );
}

function StateBadge({ state }: { state: ViewState }) {
  return (
    <span
      className={`state-badge ${state.loading ? "state-loading" : ""} ${
        state.stale ? "state-stale" : ""
      } ${state.error ? "state-error" : ""}`}
    >
      <StateDot state={state} />
      {state.status}
    </span>
  );
}

function StateDot({ state }: { state: ViewState }) {
  return (
    <span
      className={`state-dot ${state.loading ? "state-loading" : ""} ${
        state.stale ? "state-stale" : ""
      } ${state.error ? "state-error" : ""}`}
      aria-hidden="true"
    />
  );
}

function normalizeServerRoute(serverId: string) {
  if (typeof window === "undefined") {
    return;
  }

  const normalizedPath = normalizeServerRouteLocation(
    window.location,
    serverId,
  );
  if (normalizedPath) {
    window.history.replaceState(null, "", normalizedPath);
  }
}

function resolveWorkbenchSelection(
  resources: DashboardResourcesView | null,
  selectedId: string | null,
): WorkbenchSelection | null {
  if (!resources) {
    return null;
  }

  let fallback: WorkbenchSelection | null = null;

  for (const workspace of resources.workspaces) {
    for (const root of workspace.workRoots) {
      const mainInstance = root.mainInstances[0] ?? null;
      const rootSelection = {
        workspace,
        root,
        mainInstance,
        selectedInstance: mainInstance,
      };
      fallback ??= rootSelection;

      if (selectedId === workspace.id || selectedId === root.id) {
        return rootSelection;
      }

      for (const main of root.mainInstances) {
        const selectedInstance = findInstanceById(main, selectedId);
        if (selectedInstance) {
          return { workspace, root, mainInstance: main, selectedInstance };
        }
      }
    }
  }

  return fallback;
}

function findInstanceById(
  instance: InstanceView,
  selectedId: string | null,
): InstanceView | null {
  if (!selectedId) {
    return null;
  }

  if (instance.id === selectedId) {
    return instance;
  }

  for (const subInstance of instance.subInstances) {
    const nested = findInstanceById(subInstance, selectedId);
    if (nested) {
      return nested;
    }
  }

  return null;
}

function resourceEntityForWorkRoot(root: WorkRootView): ResourceEntity {
  return {
    id: root.id,
    type: "workRoot",
    label: root.label,
    state: root.state,
    actions: root.actions,
    compactable: root.compactable,
    path: root.resourcePath,
    kind: root.kind,
    activation: root.activation,
    availability: root.availability,
    status: root.status,
    instanceCount: root.mainInstances.length,
  };
}

function instanceSummary(instance: InstanceView) {
  return `${instance.role} ${instance.kind} · ${instance.interactionMode}`;
}

function closeContractLabel(kind: SurfaceKind) {
  return `close: ${decideSurfaceClose(kind).behavior}`;
}

function compactMainInstance(workspace: WorkspaceView) {
  if (!workspace.compactable || workspace.workRoots.length !== 1) {
    return null;
  }

  const root = workspace.workRoots[0];
  if (!root.compactable || root.mainInstances.length !== 1) {
    return null;
  }

  const instance = root.mainInstances[0];
  if (instance.subInstances.length > 0) {
    return null;
  }

  return { id: instance.id, root, instance };
}

function kindLabel(kind: WorkRootView["kind"]) {
  switch (kind) {
    case "gitPrimaryRoot":
      return "git root";
    case "gitLinkedWorktree":
      return "worktree";
    case "plainDirectory":
      return "directory";
  }
}
