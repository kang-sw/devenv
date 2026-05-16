import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { normalizeServerRouteLocation } from "./routeBasis";
import {
  decideSurfaceClose,
  decideSurfaceOpen,
  defaultSurfaceRegistry,
  applyWorkbenchPaneOrder,
  commitWorkbenchPaneMove,
  partitionWorkbenchPanesByCategory,
  resolveWorkbenchPaneDrop,
  selectWorkbenchPane,
  workbenchPaneDragMimeType,
  surfaceLogicalKey,
  workbenchGroupId,
  type SurfaceKind,
  type WorkbenchPaneCategory,
  type WorkbenchPaneOrder,
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
  reconcileListedTerminalSessions,
  removeClosedTerminalPane,
  resizeTerminal,
  sendTerminalInput,
  shouldPollTerminalOutput,
  terminalOutputPollChangedState,
  terminalPaneFromSession,
  terminalPaneLogicalKey,
  terminalWebSocketCursor,
  terminalWebSocketUrl,
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

type CommandPayload =
  | { type: "select"; entityId: string }
  | { type: "action"; label: string; entityId: string }
  | { type: "refresh" };

type CommandEntry = {
  id: number;
  commandId: string;
  label: string;
};

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

const resourceEndpoint = "/api/dashboard/resources";

// Terminal output is short-polled over HTTP (the daemon output route returns
// immediately). A snappy interval keeps keystroke echo latency low; idle polls
// are guarded below so they do not re-render the workbench.
const terminalOutputPollIntervalMs = 120;

export function App() {
  const [resources, setResources] = useState<DashboardResourcesView | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [commandLog, setCommandLog] = useState<CommandEntry[]>([]);
  const [readOnlyFilePanes, setReadOnlyFilePanes] = useState<Record<string, ReadOnlyFilePane>>({});
  const [activeReadOnlyFilePaneRequest, setActiveReadOnlyFilePaneRequest] = useState<{
    paneId: string;
    sequence: number;
  } | null>(null);
  const [readOnlyFilePaneOrderByGroup, setReadOnlyFilePaneOrderByGroup] = useState<WorkbenchPaneOrder>({});
  const commandSequence = useRef(0);
  const fileOpenSequence = useRef(0);

  const loadResources = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const response = await fetch(resourceEndpoint, {
        headers: { Accept: "application/json" },
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const nextResources = (await response.json()) as DashboardResourcesView;
      setResources(nextResources);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "request failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadResources();
  }, [loadResources]);

  const handleWorkRootOpened = useCallback(
    (openedView: DashboardResourcesView) => {
      // Identify the just-opened workRoot: the workRoot present in the
      // aggregated open response but absent from the prior resource view.
      const priorWorkRootIds = new Set(
        flattenEntities(resources)
          .filter((entity) => entity.type === "workRoot")
          .map((entity) => entity.id),
      );
      const openedWorkRootId = flattenEntities(openedView).find(
        (entity) => entity.type === "workRoot" && !priorWorkRootIds.has(entity.id),
      )?.id;

      // Reconcile immediately with the aggregated open response and select the
      // opened workRoot, then re-fetch the canonical endpoint so it stays the
      // source of truth for refresh and re-entry.
      setResources(openedView);
      if (openedWorkRootId) {
        setSelectedId(openedWorkRootId);
      }
      void loadResources();
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
    (workRoot: WorkRootView, entry: WorkRootFileEntryView) => {
      const pane = createLoadingReadOnlyFilePane(workRoot.id, entry.path);
      const placement = decideSurfaceOpen(readOnlyFilePlacementState(readOnlyFilePanes), {
        surfaceKind: "editor",
        logicalKey: surfaceLogicalKey("editor", workRoot.id, entry.path),
      });
      const focusPane = () =>
        setActiveReadOnlyFilePaneRequest({
          paneId: pane.id,
          sequence: fileOpenSequence.current++,
        });

      if (readOnlyFilePanes[pane.logicalKey]) {
        focusPane();
        return;
      }

      setReadOnlyFilePanes((current) => ({
        ...current,
        [pane.logicalKey]: pane,
      }));
      if (placement.type === "openNew") {
        setReadOnlyFilePaneOrderByGroup((current) => ({
          ...current,
          [placement.groupId]: [...(current[placement.groupId] ?? []), pane.id],
        }));
      }
      focusPane();

      void fetchWorkRootTextFile(workRoot.id, entry.path)
        .then((file) => {
          setReadOnlyFilePanes((current) => ({
            ...current,
            [pane.logicalKey]: applyReadOnlyFilePaneContent(current[pane.logicalKey] ?? pane, file),
          }));
        })
        .catch((error) => {
          setReadOnlyFilePanes((current) => ({
            ...current,
            [pane.logicalKey]: applyReadOnlyFilePaneError(
              current[pane.logicalKey] ?? pane,
              error instanceof Error ? error.message : "file read failed",
            ),
          }));
        });
    },
    [readOnlyFilePanes],
  );

  const executeCommand = useCallback(
    (commandId: string, payload: CommandPayload) => {
      if (payload.type === "select") {
        setSelectedId(payload.entityId);
      }

      if (payload.type === "refresh") {
        void loadResources();
      }

      setCommandLog((entries) =>
        [
          {
            id: commandSequence.current++,
            commandId,
            label:
              payload.type === "action"
                ? payload.label
                : payload.type === "refresh"
                  ? "Refresh"
                  : "Select",
          },
          ...entries,
        ].slice(0, 6),
      );
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
          <OpenWorkRootControl onOpened={handleWorkRootOpened} onCommand={executeCommand} />
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

        <section className="shell-panel shell-panel-workbench" aria-label="WorkRoot workbench">
          <WorkbenchShell
            commandLog={commandLog}
            error={error}
            loading={loading}
            resources={resources}
            selectedEntity={selectedEntity}
            selection={workbenchSelection}
            onCommand={executeCommand}
            readOnlyFilePanes={Object.values(readOnlyFilePanes)}
            readOnlyFilePaneOrderByGroup={readOnlyFilePaneOrderByGroup}
            activeReadOnlyFilePaneRequest={activeReadOnlyFilePaneRequest}
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
  onCommand?: (commandId: string, payload: CommandPayload) => void;
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
              data-command-id={`resource.action.${action.id}`}
              disabled={!action.enabled}
              key={action.id}
              title={action.label}
              type="button"
              onClick={() =>
                onCommand(`resource.action.${action.id}`, {
                  type: action.id === "refresh" ? "refresh" : "action",
                  label: action.label,
                  entityId,
                })
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
  onOpened: (view: DashboardResourcesView) => void;
  onCommand: (commandId: string, payload: CommandPayload) => void;
}) {
  const [path, setPath] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    const requestedPath = path.trim();
    if (requestedPath.length === 0 || pending) {
      return;
    }

    onCommand("workRoot.open", {
      type: "action",
      label: "Open workRoot",
      entityId: "",
    });
    setPending(true);
    setError(null);

    try {
      const openedView = await requestOpenWorkRoot(requestedPath);
      setPath("");
      onOpened(openedView);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "open failed");
    } finally {
      setPending(false);
    }
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
      {error ? <InlineNotice tone="error" title="Open failed" detail={error} /> : null}
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
  onCommand: (commandId: string, payload: CommandPayload) => void;
  onOpenFile: (workRoot: WorkRootView, entry: WorkRootFileEntryView) => void;
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
            onClick={() => onCommand("dashboard.refresh", { type: "refresh" })}
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
        {error ? <InlineNotice tone="error" title="Refresh failed" detail={error} /> : null}
        {loading ? <InlineNotice tone="info" title="Refreshing" detail="resources" /> : null}
        {resources.workspaces.map((workspace) => (
          <WorkspaceRows
            key={workspace.id}
            workspace={workspace}
            selectedId={selectedId}
            onCommand={onCommand}
          />
        ))}
      </div>
      <WorkRootFileExplorer workRoot={selectedWorkRoot} onCommand={onCommand} onOpenFile={onOpenFile} />
    </div>
  );
}

function WorkRootFileExplorer({
  workRoot,
  onCommand,
  onOpenFile,
}: {
  workRoot: WorkRootView | null;
  onCommand: (commandId: string, payload: CommandPayload) => void;
  onOpenFile: (workRoot: WorkRootView, entry: WorkRootFileEntryView) => void;
}) {
  const [snapshots, setSnapshots] = useState<Record<string, WorkRootExplorerSnapshot>>({});

  const snapshot = workRoot ? snapshots[workRoot.id] ?? initialExplorerSnapshot() : null;

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
    updateSnapshot(workRoot.id, (current) => ({ ...current, selectedPath: entry.path }));
    onCommand("fileExplorer.selectEntry", {
      type: "action",
      label: entry.name,
      entityId: workRoot.id,
    });
  };

  const toggleDirectory = (entry: WorkRootFileEntryView) => {
    const isExpanded = snapshot?.expandedPaths.has(entry.path) ?? false;
    updateSnapshot(workRoot.id, (current) => ({
      ...current,
      expandedPaths: toggleExpandedPath(current.expandedPaths, entry.path),
      selectedPath: entry.path,
    }));
    onCommand("fileExplorer.toggleDirectory", {
      type: "action",
      label: entry.name,
      entityId: workRoot.id,
    });

    if (workRootExplorerShouldLoadOnExpand(snapshot, entry.path, isExpanded)) {
      void loadDirectory(workRoot.id, entry.path);
    }
  };

  const openFile = (entry: WorkRootFileEntryView) => {
    updateSnapshot(workRoot.id, (current) => ({ ...current, selectedPath: entry.path }));
    onCommand("fileExplorer.openFile", {
      type: "action",
      label: entry.name,
      entityId: workRoot.id,
    });
    onOpenFile(workRoot, entry);
  };

  const refreshExplorer = () => {
    onCommand("fileExplorer.refresh", {
      type: "action",
      label: "Refresh files",
      entityId: workRoot.id,
    });
    const paths = workRootExplorerRefreshPaths(snapshot?.expandedPaths ?? new Set([""]));
    for (const path of paths) {
      void loadDirectory(workRoot.id, path);
    }
  };

  return (
    <section className="file-explorer" aria-label={`Files for ${workRoot.label}`}>
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
      <div className="file-explorer-body" role="tree" aria-label="WorkRoot file tree">
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
  onOpenFile: (entry: WorkRootFileEntryView) => void;
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
      onOpenFile(entry);
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
    >
      <span className="file-explorer-twisty" aria-hidden="true">
        {isDirectory ? (expanded ? "▾" : "▸") : ""}
      </span>
      <span className="file-explorer-name">
        {entry.name}
        {isDirectory ? "/" : ""}
      </span>
      {!isOk ? <span className="file-explorer-row-status">{entry.status}</span> : null}
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
}: {
  resources: DashboardResourcesView | null;
  selection: WorkbenchSelection | null;
  selectedEntity: ResourceEntity | null;
  commandLog: CommandEntry[];
  loading: boolean;
  error: string | null;
  onCommand: (commandId: string, payload: CommandPayload) => void;
  readOnlyFilePanes: ReadOnlyFilePane[];
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder;
  activeReadOnlyFilePaneRequest: { paneId: string; sequence: number } | null;
}) {
  const [activePaneByGroup, setActivePaneByGroup] = useState<Record<string, string>>({});
  const [paneOrderByGroup, setPaneOrderByGroup] = useState<WorkbenchPaneOrder>({});
  const [draggedPaneId, setDraggedPaneId] = useState<string | null>(null);
  const [terminalPanes, setTerminalPanes] = useState<Record<string, TerminalPaneState>>({});
  const [activeTerminalPaneRequest, setActiveTerminalPaneRequest] = useState<{ paneId: string; sequence: number } | null>(null);
  const [terminalPaneOrderByGroup, setTerminalPaneOrderByGroup] = useState<WorkbenchPaneOrder>({});
  const focusedReadOnlyRequest = useRef<number | null>(null);
  const focusedTerminalRequest = useRef<number | null>(null);
  const terminalOpenSequence = useRef(0);

  const workbenchModel = resources && selection
    ? (() => {
        const { workspace, root, mainInstance, selectedInstance } = selection;
        const supportEntity = selectedEntity ?? resourceEntityForWorkRoot(root);
        const editorGroups = applyWorkbenchPaneOrder(
          buildWorkbenchEditorGroups(
            root,
            mainInstance,
            selectedInstance,
            supportEntity,
            readOnlyFilePanes,
            readOnlyFilePaneOrderByGroup,
            Object.values(terminalPanes),
            terminalPaneOrderByGroup,
            {
              onSendData: sendTerminalData,
              onClose: closeTerminalPane,
              onResize: forwardTerminalResize,
              onSocketStatus: updateTerminalSocketStatus,
              onSocketMessage: applyTerminalSocketMessage,
              onSocketResize: acceptTerminalSocketResize,
            },
          ),
          paneOrderByGroup,
        );
        return { workspace, root, mainInstance, selectedInstance, editorGroups };
      })()
    : null;
  const editorGroups = workbenchModel?.editorGroups ?? [];


  useEffect(() => {
    if (!workbenchModel) {
      return;
    }
    const listStartedAtMs = Date.now();
    void listTerminals(workbenchModel.root.id)
      .then((sessions) => {
        setTerminalPanes((current) =>
          reconcileListedTerminalSessions(current, workbenchModel.root.id, sessions, listStartedAtMs),
        );
        setTerminalPaneOrderByGroup((current) => placeTerminalSessions(current, terminalPanes, sessions));
      })
      .catch(() => undefined);
  }, [workbenchModel?.root.id]);

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
              return { ...current, [pane.logicalKey]: { ...existing, error: message } };
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
      group.panes.some((pane) => pane.id === activeReadOnlyFilePaneRequest.paneId),
    );
    if (!targetGroup) {
      return;
    }

    focusedReadOnlyRequest.current = activeReadOnlyFilePaneRequest.sequence;
    setActivePaneByGroup((current) =>
      selectWorkbenchPane(current, targetGroup.id, activeReadOnlyFilePaneRequest.paneId),
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
    setActivePaneByGroup((current) =>
      selectWorkbenchPane(current, targetGroup.id, activeTerminalPaneRequest.paneId),
    );
  }, [activeTerminalPaneRequest, editorGroups]);

  function createTerminalPane() {
    if (!workbenchModel) {
      return;
    }
    void createTerminal(workbenchModel.root.id)
      .then((session) => {
        const pane = terminalPaneFromSession(session);
        setTerminalPanes((current) => ({ ...current, [pane.logicalKey]: pane }));
        setTerminalPaneOrderByGroup((current) => placeTerminalSessions(current, terminalPanes, [session]));
        setActiveTerminalPaneRequest({ paneId: pane.paneId, sequence: terminalOpenSequence.current++ });
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
        ? { ...current, [pane.logicalKey]: markTerminalSocketStatus(current[pane.logicalKey], socketStatus, error) }
        : current,
    );
  }

  function applyTerminalSocketMessage(pane: TerminalPaneState, message: TerminalWebSocketServerMessage) {
    setTerminalPanes((current) =>
      current[pane.logicalKey]
        ? { ...current, [pane.logicalKey]: appendTerminalWebSocketMessage(current[pane.logicalKey], message) }
        : current,
    );
  }

  function acceptTerminalSocketResize(pane: TerminalPaneState, columns: number, rows: number) {
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
                error: error instanceof Error ? error.message : "terminal input failed",
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
    return resizeTerminal(pane.session.terminalId, columns, rows).then((session) => {
      setTerminalPanes((current) =>
        current[pane.logicalKey]
          ? { ...current, [pane.logicalKey]: { ...current[pane.logicalKey], session } }
          : current,
      );
    });
  }

  function closeTerminalPane(pane: TerminalPaneState) {
    void closeTerminal(pane.session.terminalId)
      .then(() => setTerminalPanes((current) => removeClosedTerminalPane(current, pane.logicalKey)))
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

  const movePane = (paneId: string, targetGroupId: string, beforePaneId?: string) => {
    const result = commitWorkbenchPaneMove(editorGroups, activePaneByGroup, { paneId, targetGroupId, beforePaneId });
    setPaneOrderByGroup(result.paneOrderByGroup);
    setActivePaneByGroup(result.activePaneByGroup);
  };

  if (loading && !resources) {
    return <StatusPane title="Loading" detail="workbench resources" />;
  }

  if (error && !resources) {
    return <StatusPane title="Workbench unavailable" detail={error} />;
  }

  if (!resources || !workbenchModel) {
    return <StatusPane title="No workRoot" detail="select a workRoot or main instance" />;
  }

  const { workspace, root } = workbenchModel;

  return (
    <div className="workbench-shell">
      <WorkbenchToolbar
        commandLog={commandLog}
        root={root}
        selectedEntity={selectedEntity}
        server={resources.server}
        workspace={workspace}
        onCommand={onCommand}
        onCreateTerminal={createTerminalPane}
      />
      {error ? <InlineNotice tone="error" title="Refresh failed" detail={error} /> : null}
      {loading ? <InlineNotice tone="info" title="Refreshing" detail="resources" /> : null}
      <div className="workbench-splits" aria-label="Default two-split workbench preset">
        {editorGroups.map((group) => (
          <WorkbenchEditorGroup
            activePaneId={activePaneByGroup[group.id]}
            draggedPaneId={draggedPaneId}
            group={group}
            key={group.id}
            onDragEnd={() => setDraggedPaneId(null)}
            onDragStart={(paneId) => setDraggedPaneId(paneId)}
            onMovePane={movePane}
            onSelectPane={(paneId) =>
              setActivePaneByGroup((current) => selectWorkbenchPane(current, group.id, paneId))
            }
          />
        ))}
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
  onCommand,
  onCreateTerminal,
}: {
  server: ServerView;
  workspace: WorkspaceView;
  root: WorkRootView;
  selectedEntity: ResourceEntity | null;
  commandLog: CommandEntry[];
  onCommand: (commandId: string, payload: CommandPayload) => void;
  onCreateTerminal: () => void;
}) {
  const toggles = ["viewer", "task", "diagnostics", "events", "layout"] as const;

  return (
    <div className="workbench-toolbar">
      <div className="workbench-breadcrumb" aria-label="Workbench breadcrumb">
        <span>{server.label}</span>
        <span>{workspace.label}</span>
        <strong>{root.label}</strong>
      </div>
      <div className="workbench-toolbar-meta">
        <StateBadge state={root.state} />
        <span className="meta-chip">{kindLabel(root.kind)}</span>
        <span className="meta-chip">{root.status}</span>
        {commandLog[0] ? <span className="meta-chip">last: {commandLog[0].commandId}</span> : null}
      </div>
      <div className="workbench-toolbar-actions" aria-label="Workbench toggles and actions">
        {toolbarActions(root, selectedEntity).map(({ action, entityId }) => (
          <button
            className="action-button"
            data-command-id={`resource.action.${action.id}`}
            disabled={!action.enabled}
            key={`${entityId}:${action.id}`}
            type="button"
            onClick={() =>
              onCommand(`resource.action.${action.id}`, {
                type: action.id === "refresh" ? "refresh" : "action",
                label: action.label,
                entityId,
              })
            }
          >
            {action.label}
          </button>
        ))}
        <button
          className="action-button workbench-toggle"
          data-command-id="terminal.create"
          type="button"
          onClick={() => {
            onCommand("terminal.create", { type: "action", label: "Create terminal", entityId: root.id });
            onCreateTerminal();
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
              onCommand(`workbench.toggle.${toggle}`, {
                type: "action",
                label: toggle,
                entityId: root.id,
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

function toolbarActions(root: WorkRootView, selectedEntity: ResourceEntity | null) {
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

type WorkbenchPane = {
  readonly id: string;
  readonly kind: SurfaceKind;
  readonly category: WorkbenchPaneCategory;
  readonly title: string;
  readonly detail: string;
  readonly state: ViewState;
  readonly meta: readonly string[];
  readonly body?: ReactNode;
};

type WorkbenchEditorGroupModel = {
  readonly id: string;
  readonly label: string;
  readonly panes: readonly WorkbenchPane[];
};

function buildWorkbenchEditorGroups(
  root: WorkRootView,
  mainInstance: InstanceView | null,
  selectedInstance: InstanceView | null,
  supportEntity: ResourceEntity | null,
  readOnlyFilePanes: ReadOnlyFilePane[],
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder,
  terminalPanes: TerminalPaneState[],
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
  terminalActions: TerminalPaneActions,
): WorkbenchEditorGroupModel[] {
  const readOnlyPanesByGroup = readOnlyWorkbenchPanesByGroup(root, readOnlyFilePanes, readOnlyFilePaneOrderByGroup);
  const terminalPanesByGroup = terminalWorkbenchPanesByGroup(
    root,
    terminalPanes,
    terminalPaneOrderByGroup,
    terminalActions,
  );
  return [
    {
      id: "primary",
      label: "workRoot",
      panes: [
        {
          id: "main-agent",
          kind: "agent",
          category: "pinned",
          title: mainInstance?.label ?? "Main agent",
          detail: mainInstance ? instanceSummary(mainInstance) : "Waiting for a main instance.",
          state: mainInstance?.state ?? root.state,
          meta: mainInstance
            ? [mainInstance.kind, mainInstance.interactionMode, closeContractLabel("agent")]
            : [kindLabel(root.kind), closeContractLabel("agent")],
        },
        ...(terminalPanesByGroup.primary ?? []),
        {
          id: "selected-viewer",
          kind: "viewer",
          category: "opened",
          title: selectedInstance?.label ?? root.label,
          detail: "Selected resource projection.",
          state: selectedInstance?.state ?? root.state,
          meta: [selectedInstance?.role ?? "workRoot", selectedInstance?.kind ?? root.status],
          body: <SubInstancePane mainInstance={mainInstance} />,
        },
        ...(readOnlyPanesByGroup.primary ?? []),
      ],
    },
    {
      id: "support",
      label: "inspect",
      panes: [
        {
          id: "editor-detail",
          kind: "editor",
          category: "opened",
          title: "Editor / detail",
          detail: supportEntity ? `${supportEntity.type}: ${supportEntity.label}` : "No selection.",
          state: supportEntity?.state ?? root.state,
          meta: ["fixture data"],
          body: supportEntity ? <ResourceSummary entity={supportEntity} /> : undefined,
        },
        {
          id: "task-view",
          kind: "taskView",
          category: "opened",
          title: "Tasks",
          detail: "WorkRoot-scoped task surface reserved.",
          state: root.state,
          meta: [`${root.mainInstances.length} main`],
        },
        {
          id: "diagnostics-events",
          kind: "diagnostics",
          category: "opened",
          title: "Diagnostics",
          detail: root.state.error ?? "Resource and command events.",
          state: root.state,
          meta: [root.state.stale ? "stale" : "current"],
        },
        {
          id: "inspector",
          kind: "inspector",
          category: "opened",
          title: "Inspector",
          detail: "Dashboard-owned metadata surface.",
          state: supportEntity?.state ?? root.state,
          meta: [supportEntity?.type ?? "workRoot"],
        },
        ...(readOnlyPanesByGroup.support ?? []),
      ],
    },
  ];
}




function placeTerminalSessions(
  current: WorkbenchPaneOrder,
  existingPanes: Record<string, TerminalPaneState>,
  sessions: TerminalSessionView[],
): WorkbenchPaneOrder {
  let next = { ...current };
  let placementState = terminalPlacementState(existingPanes);
  for (const session of sessions) {
    const decision = decideSurfaceOpen(placementState, {
      surfaceKind: "persistentTerminal",
      logicalKey: surfaceLogicalKey("persistentTerminal", session.workRootId, session.terminalId),
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
            attachmentId: pane.paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
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

function terminalPlacementState(panesByLogicalKey: Record<string, TerminalPaneState>): WorkbenchPlacementState {
  return {
    groups: [{ groupId: workbenchGroupId("primary") }, { groupId: workbenchGroupId("support") }],
    focusedGroupId: workbenchGroupId("primary"),
    attachments: Object.values(panesByLogicalKey).map((pane) => ({
      attachmentId: pane.paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
      groupId: workbenchGroupId("primary"),
      surfaceKind: "persistentTerminal",
      logicalKey: surfaceLogicalKey("persistentTerminal", pane.session.workRootId, pane.session.terminalId),
    })),
  };
}

function terminalWorkbenchPanesByGroup(
  root: WorkRootView,
  terminalPanes: TerminalPaneState[],
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
  terminalActions: TerminalPaneActions,
): Record<string, WorkbenchPane[]> {
  const panes = terminalPanes
    .filter((pane) => pane.session.workRootId === root.id)
    .map((pane) => terminalWorkbenchPane(pane, terminalActions));
  const paneById = new Map(panes.map((pane) => [pane.id, pane]));
  const consumed = new Set<string>();
  const byGroup: Record<string, WorkbenchPane[]> = { primary: [], support: [] };
  for (const groupId of ["primary", "support"]) {
    for (const paneId of terminalPaneOrderByGroup[groupId] ?? []) {
      const pane = paneById.get(paneId);
      if (pane && !consumed.has(paneId)) {
        byGroup[groupId].push(pane);
        consumed.add(paneId);
      }
    }
  }
  for (const pane of panes) {
    if (!consumed.has(pane.id)) byGroup.primary.push(pane);
  }
  return byGroup;
}

type TerminalPaneActions = {
  onSendData: (pane: TerminalPaneState, data: string) => void;
  onClose: (pane: TerminalPaneState) => void;
  onResize: (pane: TerminalPaneState, columns: number, rows: number) => Promise<void>;
  onSocketStatus: (pane: TerminalPaneState, socketStatus: TerminalPaneState["socketStatus"], error?: string | null) => void;
  onSocketMessage: (pane: TerminalPaneState, message: TerminalWebSocketServerMessage) => void;
  onSocketResize: (pane: TerminalPaneState, columns: number, rows: number) => void;
};

function terminalWorkbenchPane(pane: TerminalPaneState, actions: TerminalPaneActions): WorkbenchPane {
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
    meta: [pane.session.status, pane.socketStatus, `${pane.session.columns}x${pane.session.rows}`],
    body: <TerminalPaneBody key={pane.paneId} pane={pane} actions={actions} />,
  };
}

function TerminalPaneBody({ pane, actions }: { pane: TerminalPaneState; actions: TerminalPaneActions }) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const writtenLengthRef = useRef(0);
  const lastForwardedSizeRef = useRef<{ columns: number; rows: number } | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  // Latest pane/actions for emulator callbacks registered once at mount.
  const liveRef = useRef({ pane, actions });
  liveRef.current = { pane, actions };

  const terminalId = pane.session.terminalId;

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
    const inputDisposable = terminal.onData((data) => {
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: "input", data }));
        return;
      }
      liveRef.current.actions.onSendData(liveRef.current.pane, data);
    });

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
    };

    const forwardSize = () => {
      // The emulator grid is already capped to the PTY bounds by fitNow, so
      // this size is always inside the daemon resize contract.
      const next = clampTerminalSize(terminal.cols, terminal.rows);
      const prev = lastForwardedSizeRef.current;
      if (prev && prev.columns === next.columns && prev.rows === next.rows) {
        return;
      }
      // Record the forwarded size only after the daemon accepts it; a rejected
      // resize must stay retryable rather than being suppressed as a no-op.
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: "resize", columns: next.columns, rows: next.rows }));
        liveRef.current.actions.onSocketResize(liveRef.current.pane, next.columns, next.rows);
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
    const observer = new ResizeObserver(() => {
      fitNow();
      if (resizeTimer !== null) {
        window.clearTimeout(resizeTimer);
      }
      resizeTimer = window.setTimeout(() => {
        resizeTimer = null;
        forwardSize();
      }, 250);
    });
    observer.observe(container);

    return () => {
      observer.disconnect();
      if (resizeTimer !== null) {
        window.clearTimeout(resizeTimer);
      }
      inputDisposable.dispose();
      terminal.dispose();
      terminalRef.current = null;
    };
  }, []);

  useEffect(() => {
    let disposed = false;
    const socket = new WebSocket(terminalWebSocketUrl(terminalId, terminalWebSocketCursor(liveRef.current.pane)));
    socketRef.current = socket;
    liveRef.current.actions.onSocketStatus(liveRef.current.pane, "connecting", null);

    socket.addEventListener("open", () => {
      if (!disposed) liveRef.current.actions.onSocketStatus(liveRef.current.pane, "connected", null);
    });
    socket.addEventListener("message", (event) => {
      if (disposed || typeof event.data !== "string") return;
      try {
        liveRef.current.actions.onSocketMessage(
          liveRef.current.pane,
          JSON.parse(event.data) as TerminalWebSocketServerMessage,
        );
      } catch {
        // Ignore malformed daemon frames and allow the socket close/fallback path to recover.
      }
    });
    socket.addEventListener("error", () => {
      if (!disposed) {
        liveRef.current.actions.onSocketStatus(liveRef.current.pane, "fallback", "terminal WebSocket failed");
      }
    });
    socket.addEventListener("close", () => {
      if (!disposed) liveRef.current.actions.onSocketStatus(liveRef.current.pane, "fallback", null);
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
          {pane.session.status} · {pane.session.columns}x{pane.session.rows}
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

function readOnlyFilePlacementState(
  panesByLogicalKey: Record<string, ReadOnlyFilePane>,
): WorkbenchPlacementState {
  return {
    groups: [{ groupId: workbenchGroupId("primary") }, { groupId: workbenchGroupId("support") }],
    attachments: Object.values(panesByLogicalKey).map((pane) => ({
      attachmentId: pane.id as WorkbenchPlacementState["attachments"][number]["attachmentId"],
      groupId: workbenchGroupId("support"),
      surfaceKind: "editor",
      logicalKey: surfaceLogicalKey("editor", pane.workRootId, pane.path),
    })),
  };
}

function readOnlyWorkbenchPanesByGroup(
  root: WorkRootView,
  readOnlyFilePanes: ReadOnlyFilePane[],
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder,
): Record<string, WorkbenchPane[]> {
  const panes = readOnlyFilePanes
    .filter((pane) => pane.workRootId === root.id)
    .map((pane) => readOnlyWorkbenchPane(root, pane));
  const paneById = new Map(panes.map((pane) => [pane.id, pane]));
  const consumed = new Set<string>();
  const byGroup: Record<string, WorkbenchPane[]> = { primary: [], support: [] };

  for (const groupId of ["primary", "support"]) {
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
      byGroup.support.push(pane);
    }
  }

  return byGroup;
}

function readOnlyWorkbenchPane(root: WorkRootView, pane: ReadOnlyFilePane): WorkbenchPane {
  const state: ViewState = {
    status: pane.status,
    loading: pane.status === "loading",
    stale: false,
    error: pane.error,
  };
  const meta = [
    "read-only",
    pane.languageHint ?? pane.extension ?? "text",
    pane.sizeBytes === null ? "pending" : `${pane.sizeBytes} bytes`,
  ];

  return {
    id: pane.id,
    kind: "editor",
    category: "opened",
    title: pane.title,
    detail: pane.path,
    state,
    meta,
    body: <ReadOnlyTextPane pane={pane} root={root} />,
  };
}

function ReadOnlyTextPane({ pane, root }: { pane: ReadOnlyFilePane; root: WorkRootView }) {
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
          <span className="meta-chip">read-only</span>
          <span className="meta-chip">{pane.languageHint ?? pane.extension ?? "text"}</span>
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

function WorkbenchEditorGroup({
  group,
  activePaneId,
  draggedPaneId,
  onDragEnd,
  onDragStart,
  onMovePane,
  onSelectPane,
}: {
  group: WorkbenchEditorGroupModel;
  activePaneId: string | undefined;
  draggedPaneId: string | null;
  onDragEnd: () => void;
  onDragStart: (paneId: string) => void;
  onMovePane: (paneId: string, targetGroupId: string, beforePaneId?: string) => void;
  onSelectPane: (paneId: string) => void;
}) {
  const activePane = group.panes.find((pane) => pane.id === activePaneId) ?? group.panes[0];
  const panesByCategory = partitionWorkbenchPanesByCategory(group.panes);

  if (!activePane) {
    return (
      <section className="workbench-group" aria-label={`${group.label} editor group`}>
        <div className="workbench-tab-header workbench-tab-header-empty" aria-label={group.label}>
          <WorkbenchTabLane
            activePaneId={undefined}
            category="pinned"
            draggedPaneId={draggedPaneId}
            groupId={group.id}
            panes={panesByCategory.pinned}
            onDragEnd={onDragEnd}
            onDragStart={onDragStart}
            onMovePane={onMovePane}
            onSelectPane={onSelectPane}
          />
          <WorkbenchTabLane
            activePaneId={undefined}
            category="opened"
            draggedPaneId={draggedPaneId}
            groupId={group.id}
            panes={panesByCategory.opened}
            onDragEnd={onDragEnd}
            onDragStart={onDragStart}
            onMovePane={onMovePane}
            onSelectPane={onSelectPane}
          />
        </div>
        <article
          className="workbench-pane workbench-pane-empty-state"
          role="status"
          onDragOver={(event) => {
            if (draggedPaneId) {
              event.preventDefault();
            }
          }}
          onDrop={(event) => {
            event.preventDefault();
            const move = resolveWorkbenchPaneDrop({
              dataTransferPaneId: event.dataTransfer.getData(workbenchPaneDragMimeType),
              fallbackPaneId: draggedPaneId,
              targetGroupId: group.id,
            });
            if (move) {
              onMovePane(move.paneId, move.targetGroupId, move.beforePaneId);
            }
            onDragEnd();
          }}
        >
          <div className="workbench-pane-body">
            <p>Drop a tab here to add a pane to this split.</p>
          </div>
        </article>
      </section>
    );
  }

  const activeRegistry = defaultSurfaceRegistry()[activePane.kind];

  return (
    <section className="workbench-group" aria-label={`${group.label} editor group`}>
      <div className="workbench-tab-header" aria-label={group.label}>
        <WorkbenchTabLane
          activePaneId={activePane.id}
          category="pinned"
          draggedPaneId={draggedPaneId}
          groupId={group.id}
          panes={panesByCategory.pinned}
          onDragEnd={onDragEnd}
          onDragStart={onDragStart}
          onMovePane={onMovePane}
          onSelectPane={onSelectPane}
        />
        <WorkbenchTabLane
          activePaneId={activePane.id}
          category="opened"
          draggedPaneId={draggedPaneId}
          groupId={group.id}
          panes={panesByCategory.opened}
          onDragEnd={onDragEnd}
          onDragStart={onDragStart}
          onMovePane={onMovePane}
          onSelectPane={onSelectPane}
        />
      </div>
      <article
        aria-label={`${activeRegistry.label}: ${activePane.title}`}
        className="workbench-pane"
        data-surface-kind={activePane.kind}
        id={`pane-${group.id}-${activePane.id}`}
        role="tabpanel"
      >
        <div className="workbench-pane-body">
          <p>{activePane.detail}</p>
          {activePane.body ? <div className="workbench-pane-content">{activePane.body}</div> : null}
        </div>
      </article>
    </section>
  );
}

function WorkbenchTabLane({
  activePaneId,
  category,
  draggedPaneId,
  groupId,
  panes,
  onDragEnd,
  onDragStart,
  onMovePane,
  onSelectPane,
}: {
  activePaneId: string | undefined;
  category: WorkbenchPaneCategory;
  draggedPaneId: string | null;
  groupId: string;
  panes: readonly WorkbenchPane[];
  onDragEnd: () => void;
  onDragStart: (paneId: string) => void;
  onMovePane: (paneId: string, targetGroupId: string, beforePaneId?: string) => void;
  onSelectPane: (paneId: string) => void;
}) {
  const label = category === "pinned" ? "pinned" : "opened";

  return (
    <div className={`workbench-tab-lane workbench-tab-lane-${category}`}>
      <span className="workbench-tab-lane-label">{label}</span>
      <div
        className={`workbench-tab-strip ${panes.length === 0 ? "workbench-tab-strip-empty" : ""}`}
        role="tablist"
        aria-label={`${label} panes`}
        onDragOver={(event) => {
          if (draggedPaneId) {
            event.preventDefault();
          }
        }}
        onDrop={(event) => {
          event.preventDefault();
          const move = resolveWorkbenchPaneDrop({
            dataTransferPaneId: event.dataTransfer.getData(workbenchPaneDragMimeType),
            fallbackPaneId: draggedPaneId,
            targetGroupId: groupId,
          });
          if (move) {
            onMovePane(move.paneId, move.targetGroupId, move.beforePaneId);
          }
          onDragEnd();
        }}
      >
        {panes.length === 0 ? <span className="workbench-tab-drop-hint">drop</span> : null}
        {panes.map((pane) => {
          const selected = pane.id === activePaneId;
          const registry = defaultSurfaceRegistry()[pane.kind];

          return (
            <button
              aria-controls={`pane-${groupId}-${pane.id}`}
              aria-selected={selected}
              className={`workbench-tab ${selected ? "workbench-tab-active" : ""} ${
                draggedPaneId === pane.id ? "workbench-tab-dragging" : ""
              }`}
              draggable
              key={pane.id}
              role="tab"
              title="Drag to reorder or move to another split"
              type="button"
              onClick={() => onSelectPane(pane.id)}
              onDragEnd={onDragEnd}
              onDragOver={(event) => {
                if (draggedPaneId && draggedPaneId !== pane.id) {
                  event.preventDefault();
                }
              }}
              onDragStart={(event) => {
                event.dataTransfer.effectAllowed = "move";
                event.dataTransfer.setData(workbenchPaneDragMimeType, pane.id);
                onDragStart(pane.id);
              }}
              onDrop={(event) => {
                event.preventDefault();
                event.stopPropagation();
                const move = resolveWorkbenchPaneDrop({
                  dataTransferPaneId: event.dataTransfer.getData(workbenchPaneDragMimeType),
                  fallbackPaneId: draggedPaneId,
                  targetGroupId: groupId,
                  beforePaneId: pane.id,
                });
                if (move) {
                  onMovePane(move.paneId, move.targetGroupId, move.beforePaneId);
                }
                onDragEnd();
              }}
            >
              <span className="workbench-tab-kind">{registry.label}</span>
              <span className="workbench-tab-title">{pane.title}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function SubInstancePane({ mainInstance }: { mainInstance: InstanceView | null }) {
  if (!mainInstance || mainInstance.subInstances.length === 0) {
    return <p className="workbench-pane-empty">No sub instances attached to this main surface.</p>;
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
      {entity.type === "workRoot" ? <DetailItem label="workRoot" value={entity.path.workRootId} /> : null}
      {entity.type === "instance" ? <DetailItem label="instance" value={entity.path.instanceId ?? ""} /> : null}
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
  onCommand: (commandId: string, payload: CommandPayload) => void;
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
          meta={[kindLabel(compactMain.root.kind), compactMain.root.status, compactMain.instance.kind]}
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
            meta={[kindLabel(root.kind), root.status]}
            onCommand={onCommand}
          />
          {root.mainInstances.length > 0 ? (
            <div className="nav-secondary-context">
              {root.mainInstances.length} pinned main surface{root.mainInstances.length === 1 ? "" : "s"}
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
  onCommand: (commandId: string, payload: CommandPayload) => void;
}) {
  return (
    <button
      className={`resource-row${selected ? " resource-row-selected" : ""}`}
      data-command-id="resource.select"
      style={{ "--depth": depth } as CSSProperties}
      type="button"
      onClick={() => onCommand("resource.select", { type: "select", entityId: id })}
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
            <DetailItem label="compactable" value={String(entity.compactable)} />
            <DetailItem label="workRoots" value={String(entity.workRootCount)} />
          </>
        ) : null}
        {entity.type === "workRoot" ? (
          <>
            <DetailItem label="kind" value={kindLabel(entity.kind)} />
            <DetailItem label="workRootStatus" value={entity.status} />
            <DetailItem label="instances" value={String(entity.instanceCount)} />
            <DetailItem label="workRootId" value={entity.path.workRootId} />
          </>
        ) : null}
        {entity.type === "instance" ? (
          <>
            <DetailItem label="role" value={entity.role} />
            <DetailItem label="kind" value={entity.kind} />
            <DetailItem label="mode" value={entity.interactionMode} />
            <DetailItem label="subInstances" value={String(entity.subInstanceCount)} />
            <DetailItem label="instanceId" value={entity.path.instanceId ?? ""} />
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

  const normalizedPath = normalizeServerRouteLocation(window.location, serverId);
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
      const rootSelection = { workspace, root, mainInstance, selectedInstance: mainInstance };
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

function findInstanceById(instance: InstanceView, selectedId: string | null): InstanceView | null {
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
