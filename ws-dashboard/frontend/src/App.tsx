import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, Dispatch, FormEvent, Key, ReactNode, SetStateAction } from "react";
import {
  Activity,
  BriefcaseBusiness,
  CirclePower,
  Eye,
  File,
  Folder,
  FolderGit2,
  FolderOpen,
  GitBranch,
  Plus,
  LayoutPanelTop,
  ListTodo,
  MoreHorizontal,
  PanelsTopLeft,
  RefreshCw,
  SquareTerminal,
  Stethoscope,
  Trash2,
  type LucideIcon,
} from "lucide-react";
import {
  Dialog,
  GridList,
  GridListItem,
  Heading,
  Modal,
  ModalOverlay,
} from "react-aria-components";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { normalizeServerRouteLocation } from "./routeBasis";
import {
  DocumentViewer,
  buildDocumentTranslationRequestPayload,
  fetchTranslationProviders,
  isMarkdownDocumentSource,
  overlayFromTranslationResponse,
  requestDocumentTranslation,
  type DocumentTranslationOverlay,
} from "./documentViewer";
import {
  buildDashboardRefreshCommand,
  buildDocumentModeSetCommand,
  buildDocumentRevertCommand,
  buildDocumentSaveCommand,
  buildDocumentTranslationToggleCommand,
  buildFileExplorerOpenFileCommand,
  buildFileExplorerRefreshCommand,
  buildFileExplorerSelectEntryCommand,
  buildFileExplorerToggleDirectoryCommand,
  buildGitWorktreeAddCloseCommand,
  buildGitWorktreeAddOpenCommand,
  buildGitBranchCreateCloseCommand,
  buildGitBranchCreateOpenCommand,
  buildGitBranchCreateSubmitCommand,
  buildGitBranchMenuOpenCommand,
  buildGitBranchSwitchCommand,
  buildGitFetchCommand,
  buildGitPullFfOnlyCommand,
  buildGitPushCommand,
  buildGitRefreshCommand,
  buildGitWorktreeAddSubmitCommand,
  buildRootPickerCloseCommand,
  buildRootPickerCreateDirectoryCommand,
  buildRootPickerNavigateCommand,
  buildRootPickerOpenCommand,
  buildRootPickerPinDirectoryCommand,
  buildRootPickerSelectDirectoryCommand,
  buildRootPickerUnpinDirectoryCommand,
  buildTerminalCreateCommand,
  buildWorkbenchOpenActivityCommand,
  buildWorkspaceMenuOpenCommand,
  buildWorkspaceRemoveCommand,
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
  applyReadOnlyFilePaneSavedContent,
  applyReadOnlyFilePaneSourceContent,
  applyReadOnlyFilePaneSourceError,
  createLoadingReadOnlyFilePane,
  fetchWorkRootFiles,
  fetchWorkRootTextFile,
  flattenWorkRootFileTree,
  idleDirectoryLoadState,
  loadReadOnlyFilePaneRestoreSnapshot,
  toggleExpandedPath,
  saveReadOnlyFilePaneRestoreSnapshot,
  workRootExplorerInitialLoadPath,
  workRootExplorerRefreshPaths,
  workRootExplorerShouldLoadOnExpand,
  documentDraftContentChangeDecision,
  documentSaveStateForError,
  parseWorkRootDocumentEvent,
  workRootDocumentEventsEndpoint,
  readOnlyFilePaneSourceKey,
  writeWorkRootTextFile,
  type DirectoryLoadState,
  type DocumentSaveState,
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
  compactWorkspaceWorkRoot,
  compactWorkspaceWorkRootTitle,
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
import {
  createRootPickerDirectory,
  fetchRootPicker,
  pinRootPickerDirectory,
  rootPickerHistoryBack,
  rootPickerHistoryForward,
  rootPickerHistoryInitial,
  rootPickerHistoryPush,
  rootPickerEntryLabel,
  rootPickerInsertEntry,
  rootPickerModifiedTimeLabel,
  rootPickerPinnedPathSet,
  rootPickerVisibleEntries,
  rootPickerVisiblePlaces,
  unpinRootPickerDirectory,
  type RootPickerNavigationHistory,
  type RootPickerView,
} from "./rootPicker";
import {
  fetchGitWorktreeAddOptions,
  previewGitWorktreeAdd,
  GitWorktreeAddSubmitError,
  submitGitWorktreeAdd,
  type GitWorktreeAddOptions,
  type GitWorktreeAddPreview,
  type GitWorktreeAddPreviewRequest,
} from "./gitWorktreeAdd";
import {
  createWorkRootGitBranch,
  fetchWorkRootGit,
  fetchWorkRootGitBranches,
  fetchWorkRootGitStatus,
  gitChangeStatusSegments,
  gitSyncStatusSegments,
  gitStatusSegments,
  pullWorkRootGitFfOnly,
  pushWorkRootGit,
  startGitRefreshScheduler,
  switchWorkRootGitBranch,
  type GitBranchList,
  type GitStatusSegment,
  type WorkRootGitStatus,
} from "./gitToolbar";
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

async function requestWorkspaceRemoval(
  workspaceId: string,
): Promise<DashboardResourcesView> {
  const response = await fetch(
    `/api/dashboard/workspaces/${encodeURIComponent(workspaceId)}`,
    { method: "DELETE", headers: { Accept: "application/json" } },
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
  const [gitWorktreeWorkspaceId, setGitWorktreeWorkspaceId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [commandLog, setCommandLog] = useState<CommandEntry[]>([]);
  const [initialReadOnlyFilePaneRestore] = useState(() =>
    loadReadOnlyFilePaneRestoreSnapshot(),
  );
  const [readOnlyFilePanes, setReadOnlyFilePanes] = useState<
    Record<string, ReadOnlyFilePane>
  >(initialReadOnlyFilePaneRestore.panes);
  const [activeReadOnlyFilePaneRequest, setActiveReadOnlyFilePaneRequest] =
    useState<{
      paneId: string;
      sequence: number;
    } | null>(null);
  const [readOnlyFilePaneOrderByGroup, setReadOnlyFilePaneOrderByGroup] =
    useState<WorkbenchPaneOrder>(initialReadOnlyFilePaneRestore.orderByGroup);
  const [workbenchGroupsByRoot, setWorkbenchGroupsByRoot] = useState<
    Record<string, ReadonlyArray<{ id: string; label: string }>>
  >({});
  const [paneOrderByRoot, setPaneOrderByRoot] = useState<
    Record<string, WorkbenchPaneOrder>
  >({});
  const commandSequence = useRef(0);
  const fileOpenSequence = useRef(0);
  const restoredReadOnlyPaneKeys = useRef(
    new Set(Object.keys(initialReadOnlyFilePaneRestore.panes)),
  );
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

  useEffect(() => {
    saveReadOnlyFilePaneRestoreSnapshot(
      Object.values(readOnlyFilePanes),
      readOnlyFilePaneOrderByGroup,
    );
  }, [readOnlyFilePanes, readOnlyFilePaneOrderByGroup]);

  useEffect(() => {
    if (!resources || restoredReadOnlyPaneKeys.current.size === 0) {
      return;
    }
    const knownWorkRootIds = new Set(
      resources.workspaces.flatMap((workspace) =>
        workspace.workRoots.map((root) => root.id),
      ),
    );
    for (const logicalKey of Array.from(restoredReadOnlyPaneKeys.current)) {
      const pane = readOnlyFilePanes[logicalKey];
      if (!pane || pane.status !== "loading") {
        restoredReadOnlyPaneKeys.current.delete(logicalKey);
        continue;
      }
      if (!knownWorkRootIds.has(pane.workRootId)) {
        continue;
      }
      restoredReadOnlyPaneKeys.current.delete(logicalKey);
      void fetchWorkRootTextFile(pane.workRootId, pane.path)
        .then((file) => {
          setReadOnlyFilePanes((current) => {
            const currentPane = current[logicalKey];
            if (!sameReadOnlyOpenRequest(currentPane, pane)) {
              return current;
            }
            return {
              ...current,
              [logicalKey]: applyReadOnlyFilePaneContent(currentPane, file),
            };
          });
        })
        .catch((error) => {
          setReadOnlyFilePanes((current) => {
            const currentPane = current[logicalKey];
            if (!sameReadOnlyOpenRequest(currentPane, pane)) {
              return current;
            }
            return {
              ...current,
              [logicalKey]: applyReadOnlyFilePaneError(
                currentPane,
                error instanceof Error ? error.message : "file read failed",
              ),
            };
          });
        });
    }
  }, [readOnlyFilePanes, resources]);

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
      } else if (command.payload.type === "gitWorktreeAdd.open") {
        const { workspaceId } = command.payload;
        executableHandlers[command.commandId] = () => setGitWorktreeWorkspaceId(workspaceId);
      } else if (command.payload.type === "gitWorktreeAdd.close") {
        executableHandlers[command.commandId] = () => setGitWorktreeWorkspaceId(null);
      } else if (command.payload.type === "workspace.remove") {
        const { workspaceId } = command.payload;
        executableHandlers[command.commandId] = () => {
          const workspace = resources?.workspaces.find(
            (candidate) => candidate.id === workspaceId,
          );
          if (
            !window.confirm(
              "Remove this workspace from the dashboard? Files and Git worktrees on disk will not be deleted.",
            )
          ) {
            return;
          }
          const removedRootIds = new Set(
            workspace?.workRoots.map((root) => root.id) ?? [],
          );
          void requestWorkspaceRemoval(workspaceId)
            .then((nextResources) => {
              resourceRefreshCoordinatorRef.current?.applyExternalResources(nextResources);
              if (removedRootIds.size > 0) {
                setReadOnlyFilePanes((current) =>
                  Object.fromEntries(
                    Object.entries(current).filter(
                      ([, pane]) => !removedRootIds.has(pane.workRootId),
                    ),
                  ),
                );
                setReadOnlyFilePaneOrderByGroup((current) =>
                  removePanesFromOrder(
                    current,
                    Object.values(readOnlyFilePanes)
                      .filter((pane) => removedRootIds.has(pane.workRootId))
                      .map((pane) => pane.id),
                  ),
                );
                setPaneOrderByRoot((current) =>
                  Object.fromEntries(
                    Object.entries(current).filter(
                      ([rootId]) => !removedRootIds.has(rootId),
                    ),
                  ),
                );
                setWorkbenchGroupsByRoot((current) =>
                  Object.fromEntries(
                    Object.entries(current).filter(
                      ([rootId]) => !removedRootIds.has(rootId),
                    ),
                  ),
                );
              }
            })
            .catch((nextError) => {
              setError(nextError instanceof Error ? nextError.message : "workspace removal failed");
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
    [loadResources, readOnlyFilePanes, resources],
  );

  const applyDocumentSaved = useCallback(
    (source: { workRootId: string; path: string; content: string; contentHash: string; sizeBytes: number }) => {
      setReadOnlyFilePanes((current) =>
        Object.fromEntries(
          Object.entries(current).map(([key, pane]) => [
            key,
            pane.workRootId === source.workRootId && pane.path === source.path
              ? applyReadOnlyFilePaneSavedContent(
                  pane,
                  source.content,
                  source.contentHash,
                  source.sizeBytes,
                )
              : pane,
          ]),
        ),
      );
    },
    [],
  );

  return (
    <main className="app-shell" aria-label="ws dashboard">
      <div className="shell-grid shell-grid-workbench">
        <aside className="shell-panel shell-panel-nav ws-panel" aria-label="Resources">
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
          <GitWorktreeAddModal
            workspaceId={gitWorktreeWorkspaceId}
            onCommand={executeCommand}
            onClose={() => setGitWorktreeWorkspaceId(null)}
            onCreated={(response) => {
              resourceRefreshCoordinatorRef.current?.applyExternalResources(response.resources);
              if (response.createdWorkRootId) {
                setSelectedId(response.createdWorkRootId);
              }
              setGitWorktreeWorkspaceId(null);
            }}
          />
        </aside>

        <section
          className="shell-panel shell-panel-workbench ws-panel"
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
            onDocumentSaved={applyDocumentSaved}
          />
        </section>
      </div>
    </main>
  );
}

function ChromeIconButton({
  icon: Icon,
  label,
  className = "",
  commandId,
  disabled = false,
  tone = "default",
  onClick,
}: {
  icon: LucideIcon;
  label: string;
  className?: string;
  commandId: string;
  disabled?: boolean;
  tone?: "default" | "primary" | "danger";
  onClick: () => void;
}) {
  return (
    <button
      aria-label={label}
      className={`icon-button icon-button-${tone} ${className}`.trim()}
      data-command-id={commandId}
      disabled={disabled}
      title={label}
      type="button"
      onClick={onClick}
    >
      <Icon aria-hidden="true" size={15} strokeWidth={1.8} />
    </button>
  );
}

function ResourceGlyph({
  presentation,
}: {
  presentation: "compactWorkRoot" | "workspace" | "workRoot";
}) {
  if (presentation === "compactWorkRoot") {
    return (
      <span
        className="resource-row-icon resource-row-icon-compact"
        aria-hidden="true"
      >
        <FolderOpen size={15} strokeWidth={1.8} />
      </span>
    );
  }

  const Icon = presentation === "workspace" ? BriefcaseBusiness : FolderGit2;
  return (
    <span className="resource-row-icon" aria-hidden="true">
      <Icon size={15} strokeWidth={1.8} />
    </span>
  );
}

function WorkRootKindIcon({ kind }: { kind: WorkRootView["kind"] }) {
  const Icon = kind === "plainDirectory" ? Folder : kind === "gitLinkedWorktree" ? GitBranch : FolderGit2;
  return <Icon aria-hidden="true" size={14} strokeWidth={1.8} />;
}

function ToggleIcon({ toggle }: { toggle: WorkbenchToggle }) {
  const Icon = workbenchToggleIcon(toggle);
  return <Icon aria-hidden="true" size={14} strokeWidth={1.8} />;
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
    <div className="panel-header ws-toolbar">
      <div className="panel-title-block">
        <div className="panel-title">{title}</div>
        {state ? <StateLine state={state} /> : null}
      </div>
      {actions.length > 0 && onCommand ? (
        <div className="action-strip">
          {actions.map((action) => (
            <ChromeIconButton
              commandId={
                action.id === "refresh"
                  ? "dashboard.refresh"
                  : `resource.action.${action.id}`
              }
              disabled={!action.enabled}
              icon={action.id === "refresh" ? RefreshCw : PanelsTopLeft}
              key={action.id}
              label={action.label}
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
            />
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
  const [open, setOpen] = useState(false);
  const [pickerView, setPickerView] = useState<RootPickerView | null>(null);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [addressPath, setAddressPath] = useState("");
  const [exactPath, setExactPath] = useState("");
  const [createName, setCreateName] = useState("");
  const [history, setHistory] = useState<RootPickerNavigationHistory>(() =>
    rootPickerHistoryInitial(),
  );
  const [loading, setLoading] = useState(false);
  const [pendingOpen, setPendingOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [pinningPath, setPinningPath] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const openerRef = useRef<HTMLButtonElement | null>(null);
  const wasOpenRef = useRef(false);

  const loadPicker = useCallback(
    async (path: string | null, historyMode: "push" | "replace" = "push") => {
      setLoading(true);
      setError(null);
      try {
        const view = await fetchRootPicker(path);
        setPickerView(view);
        setSelectedPath(view.currentPath);
        setAddressPath(view.currentPath);
        setExactPath(view.currentPath);
        if (historyMode === "push") {
          setHistory((current) => rootPickerHistoryPush(current, view.currentPath));
        }
      } catch (nextError) {
        setError(nextError instanceof Error ? nextError.message : "picker load failed");
      } finally {
        setLoading(false);
      }
    },
    [],
  );

  useEffect(() => {
    if (!open || pickerView || loading) {
      return;
    }
    void loadPicker(null);
  }, [loadPicker, loading, open, pickerView]);

  useEffect(() => {
    if (wasOpenRef.current && !open) {
      openerRef.current?.focus();
    }
    wasOpenRef.current = open;
  }, [open]);

  const closePicker = () => {
    onCommand(buildRootPickerCloseCommand(), {
      "rootPicker.close": () => {
        setOpen(false);
      },
    });
  };

  const openPicker = () => {
    onCommand(buildRootPickerOpenCommand(), {
      "rootPicker.open": () => {
        setError(null);
        setOpen(true);
      },
    });
  };

  const navigateTo = (path: string, historyMode: "push" | "replace" = "push") => {
    onCommand(buildRootPickerNavigateCommand(path), {
      "rootPicker.navigate": () => {
        void loadPicker(path, historyMode);
      },
    });
  };

  const selectDirectory = (path: string) => {
    onCommand(buildRootPickerSelectDirectoryCommand(path), {
      "rootPicker.selectDirectory": () => {
        setSelectedPath(path);
        setExactPath(path);
      },
    });
  };

  const navigateBack = () => {
    const transition = rootPickerHistoryBack(history);
    if (!transition.targetPath) {
      return;
    }
    setHistory(transition.history);
    navigateTo(transition.targetPath, "replace");
  };

  const navigateForward = () => {
    const transition = rootPickerHistoryForward(history);
    if (!transition.targetPath) {
      return;
    }
    setHistory(transition.history);
    navigateTo(transition.targetPath, "replace");
  };

  const handleGridSelection = (keys: "all" | Set<Key>) => {
    if (keys === "all") {
      return;
    }
    const nextPath = Array.from(keys).at(0);
    if (typeof nextPath === "string") {
      selectDirectory(nextPath);
    }
  };

  const submitPath = (submittedPath: string) => {
    const requestedPath = submittedPath.trim();
    if (requestedPath.length === 0 || pendingOpen) {
      return;
    }

    onCommand(
      buildWorkRootOpenCommand(requestedPath),
      {
        "workRoot.open": () => {
          setPendingOpen(true);
          setError(null);
          void requestOpenWorkRoot(requestedPath)
            .then((result) => {
              setOpen(false);
              setPickerView(null);
              setSelectedPath(null);
              setAddressPath("");
              setExactPath("");
              setCreateName("");
              setHistory(rootPickerHistoryInitial());
              onOpened(result.view, result.openedWorkRootId ?? undefined);
            })
            .catch((nextError) => {
              setError(nextError instanceof Error ? nextError.message : "open failed");
            })
            .finally(() => {
              setPendingOpen(false);
            });
        },
      },
    );
  };

  const createDirectory = () => {
    const parentPath = pickerView?.currentPath;
    const name = createName.trim();
    if (!parentPath || name.length === 0 || creating) {
      return;
    }
    onCommand(buildRootPickerCreateDirectoryCommand(parentPath, name), {
      "rootPicker.createDirectory": () => {
        setCreating(true);
        setError(null);
        void createRootPickerDirectory(parentPath, name)
          .then((entry) => {
            setPickerView((current) =>
              current
                ? {
                    ...current,
                    entries: rootPickerInsertEntry(current.entries, entry),
                  }
                : current,
            );
            setSelectedPath(entry.path);
            setExactPath(entry.path);
            setCreateName("");
          })
          .catch((nextError) => {
            setError(nextError instanceof Error ? nextError.message : "create failed");
          })
          .finally(() => {
            setCreating(false);
          });
      },
    });
  };

  const updatePickerPlaces = (places: RootPickerView["places"]) => {
    setPickerView((current) => (current ? { ...current, places } : current));
  };

  const pinDirectory = (path: string) => {
    if (pinningPath) {
      return;
    }
    onCommand(buildRootPickerPinDirectoryCommand(path), {
      "rootPicker.pinDirectory": () => {
        setPinningPath(path);
        setError(null);
        void pinRootPickerDirectory(path)
          .then((view) => updatePickerPlaces(view.places))
          .catch((nextError) => {
            setError(nextError instanceof Error ? nextError.message : "pin failed");
          })
          .finally(() => setPinningPath(null));
      },
    });
  };

  const unpinDirectory = (path: string) => {
    if (pinningPath) {
      return;
    }
    onCommand(buildRootPickerUnpinDirectoryCommand(path), {
      "rootPicker.unpinDirectory": () => {
        setPinningPath(path);
        setError(null);
        void unpinRootPickerDirectory(path)
          .then((view) => updatePickerPlaces(view.places))
          .catch((nextError) => {
            setError(nextError instanceof Error ? nextError.message : "unpin failed");
          })
          .finally(() => setPinningPath(null));
      },
    });
  };

  const selectedLabel = selectedPath ? rootPickerEntryLabel(selectedPath) : "None";
  const selectedEntry = pickerView?.entries.find((entry) => entry.path === selectedPath);
  const visibleEntries = rootPickerVisibleEntries(pickerView?.entries ?? []);
  const visiblePlaces = rootPickerVisiblePlaces(pickerView);
  const pinnedPaths = rootPickerPinnedPathSet(pickerView);
  const selectedPathIsPinned = selectedPath ? pinnedPaths.has(selectedPath) : false;

  return (
    <div className="open-work-root" aria-label="Open workRoot">
      <div className="open-work-root-row">
        <div>
          <div className="section-label">Open workRoot</div>
          <div className="open-work-root-summary">Choose a directory from this host</div>
        </div>
        <button
          ref={openerRef}
          aria-label="Open workRoot"
          className="icon-button icon-button-primary"
          data-command-id="rootPicker.open"
          title="Open workRoot"
          type="button"
          onClick={openPicker}
        >
          <FolderOpen aria-hidden="true" size={15} strokeWidth={1.8} />
        </button>
      </div>
      <ModalOverlay
        className="root-picker-backdrop"
        isDismissable
        isOpen={open}
        onOpenChange={(isOpen) => {
          if (!isOpen && open) {
            closePicker();
          }
        }}
      >
        <Modal className="root-picker-modal">
          <Dialog aria-label="Open workRoot" className="root-picker-dialog">
            <div className="root-picker-header">
              <div className="root-picker-title-block">
                <Heading className="root-picker-title" slot="title">
                  Open workRoot
                </Heading>
                <div className="root-picker-current" title={pickerView?.currentPath ?? ""}>
                  {pickerView?.currentPath ?? "Loading host directories"}
                </div>
              </div>
              <button
                className="action-button"
                data-command-id="rootPicker.close"
                type="button"
                onClick={closePicker}
              >
                Close
              </button>
            </div>

            <form
              className="root-picker-toolbar"
              onSubmit={(event) => {
                event.preventDefault();
                if (addressPath.trim().length > 0) {
                  navigateTo(addressPath.trim());
                }
              }}
            >
              <div className="root-picker-toolbar-buttons">
                <button
                  className="action-button"
                  data-command-id="rootPicker.navigate"
                  disabled={history.backStack.length === 0 || loading}
                  type="button"
                  onClick={navigateBack}
                >
                  Back
                </button>
                <button
                  className="action-button"
                  data-command-id="rootPicker.navigate"
                  disabled={history.forwardStack.length === 0 || loading}
                  type="button"
                  onClick={navigateForward}
                >
                  Forward
                </button>
                <button
                  className="action-button"
                  data-command-id="rootPicker.navigate"
                  disabled={!pickerView?.parentPath || loading}
                  type="button"
                  onClick={() => {
                    if (pickerView?.parentPath) {
                      navigateTo(pickerView.parentPath);
                    }
                  }}
                >
                  Up
                </button>
                <button
                  className="action-button"
                  data-command-id="rootPicker.navigate"
                  disabled={!pickerView || loading}
                  type="button"
                  onClick={() => {
                    if (pickerView) {
                      navigateTo(pickerView.currentPath, "replace");
                    }
                  }}
                >
                  Refresh
                </button>
              </div>
              <input
                aria-label="Address"
                className="root-picker-input root-picker-address"
                type="text"
                autoComplete="off"
                spellCheck={false}
                value={addressPath}
                onChange={(event) => setAddressPath(event.target.value)}
              />
            </form>

            <div className="root-picker-content">
              <aside className="root-picker-places" aria-label="Known places">
                <div className="root-picker-places-header">
                  <div className="root-picker-column-label">Places</div>
                  <button
                    className="action-button action-button-compact"
                    data-command-id={
                      selectedPathIsPinned
                        ? "rootPicker.unpinDirectory"
                        : "rootPicker.pinDirectory"
                    }
                    disabled={!selectedPath || Boolean(pinningPath)}
                    type="button"
                    onClick={() => {
                      if (!selectedPath) {
                        return;
                      }
                      if (selectedPathIsPinned) {
                        unpinDirectory(selectedPath);
                      } else {
                        pinDirectory(selectedPath);
                      }
                    }}
                  >
                    {selectedPathIsPinned ? "Unpin" : "Pin"}
                  </button>
                </div>
                {visiblePlaces.length === 0 ? (
                  <div className="root-picker-state root-picker-state-compact">
                    No known places
                  </div>
                ) : (
                  visiblePlaces.map((place) => (
                    <div
                      className={`root-picker-place-row ${
                        place.source === "pin" ? "root-picker-place-row-pinned" : ""
                      }`}
                      key={place.id}
                    >
                      <button
                        className="root-picker-place"
                        data-command-id="rootPicker.navigate"
                        data-root-picker-place-kind={place.kind}
                        disabled={!place.available}
                        title={place.path}
                        type="button"
                        onClick={() => navigateTo(place.path)}
                      >
                        <span className="root-picker-place-label">{place.label}</span>
                        <span className="root-picker-place-path">
                          {place.available ? place.path : "Unavailable"}
                        </span>
                      </button>
                      {place.source === "pin" ? (
                        <button
                          aria-label={`Unpin ${place.label}`}
                          className="root-picker-place-unpin"
                          data-command-id="rootPicker.unpinDirectory"
                          disabled={Boolean(pinningPath)}
                          type="button"
                          onClick={() => unpinDirectory(place.path)}
                        >
                          x
                        </button>
                      ) : null}
                    </div>
                  ))
                )}
              </aside>

              <section className="root-picker-list-region" aria-label="Current folder">
                <div className="root-picker-list-heading" aria-hidden="true">
                  <span>Name</span>
                  <span>Kind</span>
                  <span>Modified</span>
                </div>
                {loading && !pickerView ? (
                  <div className="root-picker-state">Loading directories</div>
                ) : visibleEntries.length === 0 ? (
                  <div className="root-picker-state">No child directories</div>
                ) : (
                  <GridList
                    aria-label="Directories"
                    className="root-picker-grid-list"
                    disabledKeys={visibleEntries
                      .filter((entry) => !entry.selectable)
                      .map((entry) => entry.path)}
                    keyboardNavigationBehavior="arrow"
                    layout="stack"
                    onAction={(key) => navigateTo(String(key))}
                    onSelectionChange={handleGridSelection}
                    selectedKeys={selectedPath ? new Set([selectedPath]) : new Set()}
                    selectionBehavior="replace"
                    selectionMode="single"
                  >
                    {visibleEntries.map((entry) => (
                      <GridListItem
                        className="root-picker-row"
                        data-command-id="rootPicker.selectDirectory"
                        id={entry.path}
                        key={entry.path}
                        textValue={entry.name}
                        onDoubleClick={() => navigateTo(entry.path)}
                      >
                        <span className="root-picker-row-icon" aria-hidden="true">
                          /
                        </span>
                        <span className="root-picker-row-name" title={entry.path}>
                          {entry.name}
                        </span>
                        <span className="root-picker-row-kind">
                          {entry.kindLabel ?? "Directory"}
                        </span>
                        <span className="root-picker-row-modified">
                          {rootPickerModifiedTimeLabel(entry.modifiedTime)}
                        </span>
                      </GridListItem>
                    ))}
                  </GridList>
                )}
              </section>
            </div>

            {error ? <InlineNotice tone="error" title="Root picker" detail={error} /> : null}

            <div className="root-picker-create">
              <label className="section-label" htmlFor="root-picker-create-name">
                Create empty folder
              </label>
              <div className="root-picker-create-row">
                <input
                  id="root-picker-create-name"
                  className="root-picker-input"
                  type="text"
                  autoComplete="off"
                  spellCheck={false}
                  placeholder="folder-name"
                  value={createName}
                  onChange={(event) => setCreateName(event.target.value)}
                />
                <button
                  className="action-button"
                  data-command-id="rootPicker.createDirectory"
                  disabled={!pickerView || createName.trim().length === 0 || creating}
                  type="button"
                  onClick={createDirectory}
                >
                  {creating ? "Creating" : "Create"}
                </button>
              </div>
            </div>

            <form
              className="root-picker-footer"
              onSubmit={(event) => {
                event.preventDefault();
                submitPath(exactPath);
              }}
            >
              <label className="root-picker-selection" htmlFor="root-picker-exact-path">
                Selected: {selectedLabel}
              </label>
              <input
                id="root-picker-exact-path"
                className="root-picker-input"
                type="text"
                autoComplete="off"
                spellCheck={false}
                value={exactPath}
                onChange={(event) => setExactPath(event.target.value)}
              />
              <div className="root-picker-footer-actions">
                <button
                  className="action-button"
                  data-command-id="rootPicker.navigate"
                  disabled={!selectedEntry || loading}
                  type="button"
                  onClick={() => {
                    if (selectedEntry) {
                      navigateTo(selectedEntry.path);
                    }
                  }}
                >
                  Open folder
                </button>
                <button
                  className="action-button action-button-primary"
                  data-command-id="workRoot.open"
                  disabled={exactPath.trim().length === 0 || pendingOpen}
                  type="submit"
                >
                  {pendingOpen ? "Opening" : "Open"}
                </button>
                <button
                  className="action-button"
                  data-command-id="rootPicker.close"
                  type="button"
                  onClick={closePicker}
                >
                  Cancel
                </button>
              </div>
            </form>
          </Dialog>
        </Modal>
      </ModalOverlay>
    </div>
  );
}


function GitWorktreeAddModal({
  workspaceId,
  onCommand,
  onClose,
  onCreated,
}: {
  workspaceId: string | null;
  onCommand: DashboardCommandDispatcher;
  onClose: () => void;
  onCreated: (response: { resources: DashboardResourcesView; createdWorkRootId?: string }) => void;
}) {
  const [options, setOptions] = useState<GitWorktreeAddOptions | null>(null);
  const [worktreeName, setWorktreeName] = useState("");
  const [branchMode, setBranchMode] = useState<"auto" | "manual">("auto");
  const [manualBranch, setManualBranch] = useState("");
  const [pathMode, setPathMode] = useState<"auto" | "custom">("auto");
  const [customPath, setCustomPath] = useState("");
  const [preview, setPreview] = useState<GitWorktreeAddPreview | null>(null);
  const [previewRequestKey, setPreviewRequestKey] = useState<string | null>(null);
  const previewSequenceRef = useRef(0);
  const currentRequestKeyRef = useRef<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId) {
      setOptions(null);
      setPreview(null);
      setPreviewRequestKey(null);
      setError(null);
      return;
    }
    setWorktreeName("");
    setBranchMode("auto");
    setManualBranch("");
    setPathMode("auto");
    setCustomPath("");
    setPreview(null);
    setPreviewRequestKey(null);
    setError(null);
    setLoading(true);
    void fetchGitWorktreeAddOptions(workspaceId)
      .then(setOptions)
      .catch((nextError) => setError(nextError instanceof Error ? nextError.message : "worktree options failed"))
      .finally(() => setLoading(false));
  }, [workspaceId]);

  const request = useMemo<GitWorktreeAddPreviewRequest | null>(() => {
    if (!workspaceId) {
      return null;
    }
    return {
      worktreeName,
      branch: branchMode === "auto" ? { mode: "auto" } : { mode: "manual", name: manualBranch },
      path: pathMode === "auto" ? { mode: "auto" } : { mode: "custom", targetPath: customPath },
    };
  }, [branchMode, customPath, manualBranch, pathMode, worktreeName, workspaceId]);

  const requestKey = request ? JSON.stringify(request) : null;

  useEffect(() => {
    currentRequestKeyRef.current = requestKey;
  }, [requestKey]);

  useEffect(() => {
    if (!workspaceId || !request || !requestKey || worktreeName.trim().length === 0) {
      setPreview(null);
      setPreviewRequestKey(null);
      return;
    }
    const sequence = previewSequenceRef.current + 1;
    previewSequenceRef.current = sequence;
    setPreview(null);
    setPreviewRequestKey(null);
    const timer = window.setTimeout(() => {
      void previewGitWorktreeAdd(workspaceId, request)
        .then((nextPreview) => {
          if (previewSequenceRef.current !== sequence) {
            return;
          }
          setPreview(nextPreview);
          setPreviewRequestKey(requestKey);
        })
        .catch((nextError) => {
          if (previewSequenceRef.current !== sequence) {
            return;
          }
          setError(nextError instanceof Error ? nextError.message : "worktree preview failed");
        });
    }, 150);
    return () => window.clearTimeout(timer);
  }, [request, requestKey, worktreeName, workspaceId]);

  if (!workspaceId) {
    return null;
  }

  const close = () => {
    onCommand(buildGitWorktreeAddCloseCommand(workspaceId), {
      "gitWorktreeAdd.close": onClose,
    });
  };
  const submitDisabled =
    submitting ||
    !request ||
    worktreeName.trim().length === 0 ||
    (branchMode === "manual" && manualBranch.trim().length === 0) ||
    (pathMode === "custom" && customPath.trim().length === 0) ||
    !preview ||
    previewRequestKey !== requestKey ||
    preview.status === "blocked" ||
    options?.git.available === false;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!request || submitDisabled) {
      return;
    }
    onCommand(buildGitWorktreeAddSubmitCommand(workspaceId), {
      "gitWorktreeAdd.submit": () => {
        const submittedRequestKey = requestKey;
        setSubmitting(true);
        setError(null);
        void submitGitWorktreeAdd(workspaceId, { ...request, activate: true })
          .then(onCreated)
          .catch((nextError) => {
            if (nextError instanceof GitWorktreeAddSubmitError && nextError.preview) {
              if (currentRequestKeyRef.current !== submittedRequestKey) {
                return;
              }
              setPreview(nextError.preview);
              setPreviewRequestKey(submittedRequestKey);
              setError("Submit blocked by current server validation");
              return;
            }
            setError(nextError instanceof Error ? nextError.message : "worktree add failed");
          })
          .finally(() => setSubmitting(false));
      },
    });
  };

  const severity = preview?.status ?? "blocked";
  return (
    <ModalOverlay className="root-picker-backdrop" isDismissable isOpen onOpenChange={(isOpen) => { if (!isOpen) close(); }}>
      <Modal className="root-picker-modal git-worktree-modal">
        <Dialog aria-label="Add Git worktree" className="root-picker-dialog">
          <div className="root-picker-header">
            <div className="root-picker-title-block">
              <Heading className="root-picker-title" slot="title">Add worktree</Heading>
              <div className="root-picker-current">{options?.git.rootLabel ?? "Loading Git workspace"}</div>
            </div>
            <button className="action-button" data-command-id="gitWorktreeAdd.close" type="button" onClick={close}>Close</button>
          </div>
          <form className="git-worktree-form" onSubmit={submit}>
            <label className="git-worktree-field">
              <span className="section-label">Worktree name</span>
              <input className="root-picker-input" autoComplete="off" value={worktreeName} onChange={(event) => setWorktreeName(event.target.value)} placeholder="feature-name" />
            </label>
            <fieldset className="git-worktree-fieldset">
              <legend className="section-label">Branch</legend>
              <label><input type="radio" checked={branchMode === "auto"} onChange={() => setBranchMode("auto")} /> Auto from name</label>
              <label><input type="radio" checked={branchMode === "manual"} onChange={() => setBranchMode("manual")} /> Existing/manual branch</label>
              <input className="root-picker-input" list="git-worktree-branches" disabled={branchMode !== "manual"} value={manualBranch} onChange={(event) => setManualBranch(event.target.value)} placeholder="branch-name" />
              <datalist id="git-worktree-branches">
                {(options?.branches ?? []).filter((branch) => !branch.checkedOut).map((branch) => <option key={branch.name} value={branch.name} />)}
              </datalist>
              <div className="git-worktree-branch-list">
                {(options?.branches ?? []).map((branch) => (
                  <span className={`meta-chip ws-chip ${branch.checkedOut ? "meta-chip-disabled" : ""}`} key={branch.name}>{branch.name}{branch.current ? " (current)" : ""}{branch.checkedOut ? " — checked out" : ""}</span>
                ))}
              </div>
            </fieldset>
            <fieldset className="git-worktree-fieldset">
              <legend className="section-label">Path</legend>
              <label><input type="radio" checked={pathMode === "auto"} onChange={() => setPathMode("auto")} /> Auto under {options?.defaults.worktreeBaseDirLabel ?? ".git/ws-worktree"}</label>
              <label><input type="radio" checked={pathMode === "custom"} onChange={() => setPathMode("custom")} /> Custom target path</label>
              <input className="root-picker-input" disabled={pathMode !== "custom"} value={customPath} onChange={(event) => setCustomPath(event.target.value)} placeholder="/path/to/worktree" />
            </fieldset>
            {options && !options.git.available ? <InlineNotice tone="error" title="Git unavailable" detail={options.git.reason ?? "workspace is not Git-capable"} /> : null}
            {preview ? (
              <div className={`git-worktree-preview git-worktree-preview-${severity}`} role="status">
                <strong>{preview.message}</strong>
                <span>{preview.branchName ? `Branch: ${preview.branchName}` : "Branch pending"}</span>
                <span>{preview.targetPathLabel ? `Target: ${preview.targetPathLabel}` : "Target pending"}</span>
                {preview.blockers.map((blocker) => <span key={`${blocker.code}:${blocker.field ?? ""}`}>{blocker.message}</span>)}
              </div>
            ) : loading ? <InlineNotice tone="info" title="Loading" detail="Git worktree options" /> : null}
            {error ? <InlineNotice tone="error" title="Add worktree" detail={error} /> : null}
            <div className="root-picker-footer-actions">
              <button className="action-button action-button-primary" data-command-id="gitWorktreeAdd.submit" disabled={submitDisabled} type="submit">{submitting ? "Creating" : "Create worktree"}</button>
              <button className="action-button" data-command-id="gitWorktreeAdd.close" type="button" onClick={close}>Cancel</button>
            </div>
          </form>
        </Dialog>
      </Modal>
    </ModalOverlay>
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
        <ChromeIconButton
          className="file-explorer-refresh"
          commandId="fileExplorer.refresh"
          icon={RefreshCw}
          label="Refresh files"
          onClick={refreshExplorer}
        />
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
      <span className="file-explorer-icon" aria-hidden="true">
        {isDirectory ? (
          <Folder size={14} strokeWidth={1.8} />
        ) : (
          <File size={14} strokeWidth={1.8} />
        )}
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
  onDocumentSaved,
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
  onDocumentSaved: (source: { workRootId: string; path: string; content: string; contentHash: string; sizeBytes: number }) => void;
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
  const readOnlyFilePanesRef = useRef(readOnlyFilePanes);
  readOnlyFilePanesRef.current = readOnlyFilePanes;
  const documentRefreshSequence = useRef<Record<string, number>>({});

  const refreshOpenDocument = useCallback(
    (workRootId: string, path: string, expectedContentHash?: string) => {
      const sourceKey = readOnlyFilePaneSourceKey(workRootId, path);
      const requestSequence = (documentRefreshSequence.current[sourceKey] ?? 0) + 1;
      documentRefreshSequence.current[sourceKey] = requestSequence;
      if (
        !readOnlyFilePanesRef.current.some(
          (pane) =>
            pane.workRootId === workRootId &&
            pane.path === path &&
            (!expectedContentHash || pane.contentHash !== expectedContentHash),
        )
      ) {
        return;
      }
      void fetchWorkRootTextFile(workRootId, path)
        .then((file) => {
          if (documentRefreshSequence.current[sourceKey] !== requestSequence) {
            return;
          }
          onReadOnlyFilePanesChange((current) =>
            applyReadOnlyFilePaneSourceContent(current, file),
          );
        })
        .catch((error) => {
          if (documentRefreshSequence.current[sourceKey] !== requestSequence) {
            return;
          }
          const message = error instanceof Error ? error.message : "file read failed";
          onReadOnlyFilePanesChange((current) =>
            applyReadOnlyFilePaneSourceError(current, workRootId, path, message),
          );
        });
    },
    [onReadOnlyFilePanesChange],
  );

  const refreshVisibleDocuments = useCallback(() => {
    const rootId = selectedWorkRootId;
    if (!rootId) {
      return;
    }
    const paths = [
      ...new Set(
        readOnlyFilePanesRef.current
          .filter((pane) => pane.workRootId === rootId && pane.status === "loaded")
          .map((pane) => pane.path),
      ),
    ];
    for (const path of paths) {
      refreshOpenDocument(rootId, path);
    }
  }, [refreshOpenDocument, selectedWorkRootId]);

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
              onDocumentSaved,
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

  // Document content events are source-neutral invalidations for open file panes.
  // A save from one pane fans out by re-reading the daemon view for matching
  // clean panes, while pane-local edit state marks dirty drafts stale when the
  // content prop changes underneath them. Browser focus/visibility re-reads are
  // the bounded fallback when the SSE stream is unavailable.
  useEffect(() => {
    const rootId = selectedWorkRootId;
    if (!rootId) {
      return;
    }

    let cancelled = false;
    const source = new EventSource(workRootDocumentEventsEndpoint(rootId));
    const handleDocumentMessage = (message: MessageEvent) => {
      if (cancelled) {
        return;
      }
      let payload: unknown;
      try {
        payload = JSON.parse(message.data);
      } catch {
        return;
      }
      const event = parseWorkRootDocumentEvent(payload);
      if (!event || event.workRootId !== rootId) {
        return;
      }
      refreshOpenDocument(event.workRootId, event.path, event.contentHash);
    };
    source.addEventListener("document", handleDocumentMessage);
    source.onmessage = handleDocumentMessage;
    return () => {
      cancelled = true;
      source.removeEventListener("document", handleDocumentMessage);
      source.close();
    };
  }, [refreshOpenDocument, selectedWorkRootId]);

  useEffect(() => {
    const onFocus = () => refreshVisibleDocuments();
    const onVisibilityChange = () => {
      if (!document.hidden) {
        refreshVisibleDocuments();
      }
    };
    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [refreshVisibleDocuments]);

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

  function closeAgentPane(paneId: string, workRootId: string | null | undefined) {
    if (!workRootId) {
      return;
    }
    setClosedAgentPaneByRoot((current) => ({
      ...current,
      [workRootId]: [
        ...new Set([...(current[workRootId] ?? []), paneId]),
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
      closeAgentPane(pane.id, request.workRootId ?? selectedWorkRootId);
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

type WorkbenchToggle = "viewer" | "task" | "diagnostics" | "events" | "layout";

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
  const [overflowOpen, setOverflowOpen] = useState(false);
  const toggles: WorkbenchToggle[] = [
    "viewer",
    "task",
    "diagnostics",
    "events",
    "layout",
  ];
  const actions = toolbarActions(root, selectedEntity);
  const activationAction = actions.find((entry) => activationForAction(entry.action.id));
  const activation = activationAction
    ? activationForAction(activationAction.action.id)
    : null;
  const openRootAction = actions.find(({ action }) =>
    action.id === "openRoot" || action.id === "reconnect",
  );
  const secondaryActions = actions.filter(
    ({ action }) =>
      !activationForAction(action.id) &&
      action.id !== "openRoot" &&
      action.id !== "reconnect",
  );
  const rootMetadataTitle = [
    kindLabel(root.kind),
    `availability: ${root.availability}`,
    `activation: ${root.activation}`,
    commandLog[0] ? `last: ${commandLog[0].commandId}` : null,
  ]
    .filter(Boolean)
    .join(" · ");

  const runResourceAction = (action: ActionHint, entityId: string) => {
    onCommand({
      commandId: `resource.action.${action.id}`,
      payload: { type: "action", label: action.label, entityId },
    });
  };

  return (
    <div
      className="workbench-toolbar ws-toolbar"
      data-last-command-id={commandLog[0]?.commandId ?? ""}
    >
      <ChromeIconButton
        className={`workbench-power-button workbench-power-button-${root.activation}`}
        commandId="workRoot.activation.set"
        disabled={!activationAction || !activationAction.action.enabled || !activation}
        icon={CirclePower}
        label={activationAction?.action.label ?? "Set workRoot activation"}
        onClick={() => {
          if (!activationAction || !activation) {
            return;
          }
          onCommand(buildWorkRootActivationCommand(activationAction.entityId, activation));
        }}
      />
      <div className="workbench-toolbar-center" title={rootMetadataTitle}>
        <div className="workbench-breadcrumb" aria-label="Workbench breadcrumb">
          <span>{server.label}</span>
          <span>{workspace.label}</span>
          <strong>{root.label}</strong>
        </div>
        <div className="workbench-toolbar-meta" title={rootMetadataTitle}>
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
          <WorkRootGitToolbar root={root} onCommand={onCommand} />
          {root.availability !== "available" ? (
            <span className="meta-chip ws-chip">{root.availability}</span>
          ) : null}
          {root.activation !== "online" ? (
            <span className="meta-chip ws-chip">{root.activation}</span>
          ) : null}
        </div>
      </div>
      <div
        className="workbench-toolbar-actions"
        aria-label="Workbench primary actions"
      >
        {openRootAction ? (
          <ChromeIconButton
            commandId={`resource.action.${openRootAction.action.id}`}
            disabled={!openRootAction.action.enabled}
            icon={FolderOpen}
            label={openRootAction.action.label}
            onClick={() => runResourceAction(openRootAction.action, openRootAction.entityId)}
          />
        ) : null}
        <ChromeIconButton
          commandId="dashboard.refresh"
          icon={RefreshCw}
          label="Refresh dashboard"
          onClick={() => onCommand(buildDashboardRefreshCommand())}
        />
        <ChromeIconButton
          commandId="terminal.create"
          disabled={root.activation !== "online" || root.availability !== "available"}
          icon={SquareTerminal}
          label="New terminal"
          onClick={() => {
            onCommand(
              buildTerminalCreateCommand(root.id),
              { "terminal.create": onCreateTerminal },
            );
          }}
        />
        <div className="workbench-overflow">
          <button
            aria-expanded={overflowOpen}
            aria-haspopup="menu"
            aria-label="More workbench actions"
            className="icon-button"
            title="More workbench actions"
            type="button"
            onClick={() => setOverflowOpen((current) => !current)}
          >
            <MoreHorizontal aria-hidden="true" size={15} strokeWidth={1.8} />
          </button>
          {overflowOpen ? (
            <div className="workbench-overflow-menu" role="menu">
              {secondaryActions.map(({ action, entityId }) => (
                <button
                  className="workbench-overflow-item"
                  data-command-id={`resource.action.${action.id}`}
                  disabled={!action.enabled}
                  key={`${entityId}:${action.id}`}
                  role="menuitem"
                  title={action.label}
                  type="button"
                  onClick={() => {
                    setOverflowOpen(false);
                    runResourceAction(action, entityId);
                  }}
                >
                  <PanelsTopLeft aria-hidden="true" size={14} strokeWidth={1.8} />
                  <span>{action.label}</span>
                </button>
              ))}
              {toggles.map((toggle) => (
                <button
                  className="workbench-overflow-item"
                  data-command-id={`workbench.toggle.${toggle}`}
                  key={toggle}
                  role="menuitem"
                  title={`Toggle ${toggle}`}
                  type="button"
                  onClick={() => {
                    setOverflowOpen(false);
                    onCommand({
                      commandId: `workbench.toggle.${toggle}`,
                      payload: { type: "action", label: toggle, entityId: root.id },
                    });
                  }}
                >
                  <ToggleIcon toggle={toggle} />
                  <span>{toggle}</span>
                </button>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}


function WorkRootGitToolbar({
  root,
  onCommand,
}: {
  root: WorkRootView;
  onCommand: DashboardCommandDispatcher;
}) {
  const gitCapable =
    (root.kind === "gitPrimaryRoot" || root.kind === "gitLinkedWorktree") &&
    root.activation === "online" &&
    root.availability === "available";
  const [statusState, setStatusState] = useState<{ workRootId: string; status: WorkRootGitStatus } | null>(null);
  const [branchesState, setBranchesState] = useState<{ workRootId: string; branches: GitBranchList } | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [newBranchName, setNewBranchName] = useState("");
  const [baseBranchName, setBaseBranchName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const requestSeq = useRef(0);
  const currentRootId = useRef(root.id);
  currentRootId.current = root.id;

  const status = statusState?.workRootId === root.id ? statusState.status : null;
  const branches = branchesState?.workRootId === root.id ? branchesState.branches : null;

  const refreshGit = useCallback((reason: string) => {
    if (!gitCapable) {
      requestSeq.current += 1;
      setStatusState(null);
      setBranchesState(null);
      return;
    }
    const seq = requestSeq.current + 1;
    requestSeq.current = seq;
    const requestedRootId = root.id;
    void Promise.all([
      fetchWorkRootGitStatus(requestedRootId),
      fetchWorkRootGitBranches(requestedRootId),
    ])
      .then(([nextStatus, nextBranches]) => {
        if (requestSeq.current !== seq || currentRootId.current !== requestedRootId) return;
        setStatusState(nextStatus.available ? { workRootId: requestedRootId, status: nextStatus } : null);
        setBranchesState({ workRootId: requestedRootId, branches: nextBranches });
        setError(null);
      })
      .catch((nextError) => {
        if (requestSeq.current !== seq || currentRootId.current !== requestedRootId) return;
        setStatusState(null);
        setBranchesState(null);
        setMenuOpen(false);
        setModalOpen(false);
        setError(nextError instanceof Error ? nextError.message : `${reason} failed`);
      });
  }, [gitCapable, root.id]);

  useEffect(() => {
    refreshGit("git status");
  }, [refreshGit]);

  useEffect(() => {
    if (!gitCapable) return;
    return startGitRefreshScheduler(refreshGit, {
      isDocumentHidden: () => document.hidden,
      addDocumentListener: (event, listener) => document.addEventListener(event, listener),
      removeDocumentListener: (event, listener) => document.removeEventListener(event, listener),
      addWindowListener: (event, listener) => window.addEventListener(event, listener),
      removeWindowListener: (event, listener) => window.removeEventListener(event, listener),
      setInterval: (listener, ms) => window.setInterval(listener, ms),
      clearInterval: (handle) => window.clearInterval(handle),
    });
  }, [gitCapable, refreshGit]);

  if (!gitCapable) return null;
  if (!status) {
    return error ? (
      <div className="git-toolbar" aria-label="Git toolbar">
        <span className="meta-chip ws-chip git-error-chip">{error}</span>
      </div>
    ) : null;
  }

  const branchLabel = status.branch?.name ?? (status.branch?.detachedOid ? `HEAD ${status.branch.detachedOid}` : "Git");
  const branchOptions = branches?.branches ?? [];
  const defaultBaseBranch = branches?.current ?? branchOptions.find((branch) => branch.current)?.name ?? branchOptions[0]?.name ?? "";
  const selectedBaseBranch = baseBranchName || defaultBaseBranch;
  const closeBranchModal = () => {
    setModalOpen(false);
    setNewBranchName("");
    setBaseBranchName("");
  };
  const mutate = (command: ReturnType<typeof buildGitRefreshCommand>, run: () => Promise<WorkRootGitStatus>) => {
    const targetRootId = root.id;
    onCommand(command, {
      [command.commandId]: () => {
        void run()
          .then((nextStatus) => {
            if (currentRootId.current !== targetRootId) return;
            setStatusState(nextStatus.available ? { workRootId: targetRootId, status: nextStatus } : null);
            refreshGit("git mutation refresh");
          })
          .catch((nextError) => {
            if (currentRootId.current !== targetRootId) return;
            setError(nextError instanceof Error ? nextError.message : "git action failed");
            refreshGit("git mutation failure refresh");
          });
      },
    });
  };

  const runBranchCreateCloseCommand = () =>
    onCommand(buildGitBranchCreateCloseCommand(root.id), {
      "git.branchCreate.close": closeBranchModal,
    });

  return (
    <div className="git-toolbar" aria-label="Git toolbar">
      <div className="git-branch-menu-wrap">
        <button
          className="meta-chip ws-chip git-branch-chip"
          data-command-id="git.branchMenu.open"
          type="button"
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          onClick={() => onCommand(buildGitBranchMenuOpenCommand(root.id), { "git.branchMenu.open": () => setMenuOpen((open) => !open) })}
        >
          <GitBranch aria-hidden="true" size={13} strokeWidth={1.8} />
          <span>{branchLabel}</span>
        </button>
        {menuOpen ? (
          <div className="workbench-overflow-menu git-branch-menu" role="menu">
            <button className="workbench-overflow-item" data-command-id="git.branchCreate.open" role="menuitem" type="button" onClick={() => { setMenuOpen(false); onCommand(buildGitBranchCreateOpenCommand(root.id), { "git.branchCreate.open": () => setModalOpen(true) }); }}>
              <Plus aria-hidden="true" size={14} strokeWidth={1.8} />
              <span>+ New branch...</span>
            </button>
            {branchOptions.map((branch) => (
              <button key={branch.name} className="workbench-overflow-item" data-command-id="git.branch.switch" disabled={branch.current || (branch.checkedOut && !branch.current)} role="menuitem" type="button" title={branch.disabledReason ?? branch.name} onClick={() => { setMenuOpen(false); mutate(buildGitBranchSwitchCommand(root.id, branch.name), () => switchWorkRootGitBranch(root.id, branch.name)); }}>
                <GitBranch aria-hidden="true" size={14} strokeWidth={1.8} />
                <span>{branch.name}{branch.current ? " ✓" : ""}{branch.checkedOut && !branch.current ? " (checked out)" : ""}</span>
              </button>
            ))}
          </div>
        ) : null}
      </div>
      <GitStatusPill
        status={status}
        onFetch={() => mutate(buildGitFetchCommand(root.id), () => fetchWorkRootGit(root.id))}
        onPush={() => mutate(buildGitPushCommand(root.id), () => pushWorkRootGit(root.id))}
        onPull={() => mutate(buildGitPullFfOnlyCommand(root.id), () => pullWorkRootGitFfOnly(root.id))}
      />
      {error ? <span className="meta-chip ws-chip git-error-chip">{error}</span> : null}
      <ModalOverlay className="root-picker-backdrop" isDismissable isOpen={modalOpen} onOpenChange={(open) => { if (!open) runBranchCreateCloseCommand(); }}>
        <Modal className="root-picker-modal git-branch-modal">
          <Dialog aria-label="New Git branch" className="root-picker-dialog">
            <div className="root-picker-header">
              <Heading className="root-picker-title" slot="title">New branch</Heading>
              <button className="action-button" data-command-id="git.branchCreate.close" type="button" onClick={runBranchCreateCloseCommand}>Close</button>
            </div>
            <form className="git-branch-create-form" onSubmit={(event) => { event.preventDefault(); const branchName = newBranchName.trim(); const baseBranch = selectedBaseBranch.trim(); if (!branchName) return; const targetRootId = root.id; onCommand(buildGitBranchCreateSubmitCommand(root.id, branchName, baseBranch || undefined), { "git.branchCreate.submit": () => { void createWorkRootGitBranch(root.id, branchName, baseBranch || undefined).then((nextStatus) => { if (currentRootId.current !== targetRootId) return; setStatusState({ workRootId: targetRootId, status: nextStatus }); closeBranchModal(); refreshGit("git branch create refresh"); }).catch((nextError) => { if (currentRootId.current !== targetRootId) return; setError(nextError instanceof Error ? nextError.message : "branch create failed"); refreshGit("git branch create failure refresh"); }); } }); }}>
              <label className="git-worktree-field"><span className="section-label">Branch name</span><input className="root-picker-input" value={newBranchName} onChange={(event) => setNewBranchName(event.target.value)} placeholder="feature-name" /></label>
              <label className="git-worktree-field"><span className="section-label">Base branch</span><select className="root-picker-input" value={selectedBaseBranch} onChange={(event) => setBaseBranchName(event.target.value)}>{branchOptions.map((branch) => <option key={branch.name} value={branch.name}>{branch.name}{branch.current ? " (current)" : ""}</option>)}</select></label>
              <div className="root-picker-footer-actions"><button className="action-button action-button-primary" data-command-id="git.branchCreate.submit" type="submit" disabled={!newBranchName.trim() || !selectedBaseBranch}>Create and switch</button><button className="action-button" data-command-id="git.branchCreate.close" type="button" onClick={runBranchCreateCloseCommand}>Cancel</button></div>
            </form>
          </Dialog>
        </Modal>
      </ModalOverlay>
    </div>
  );
}

function GitStatusPill({
  status,
  onFetch,
  onPush,
  onPull,
}: {
  status: WorkRootGitStatus;
  onFetch: () => void;
  onPush: () => void;
  onPull: () => void;
}) {
  const changeSegments = gitChangeStatusSegments(status);
  const syncSegments = gitSyncStatusSegments(status);
  const renderSegment = (segment: GitStatusSegment) => {
    const className = `git-status-segment git-status-segment-${segment.tone}`;
    if (segment.commandId === "git.push") {
      return <button key={segment.key} className={className} data-command-id="git.push" type="button" disabled={segment.disabled} onClick={onPush}>{segment.label}</button>;
    }
    if (segment.commandId === "git.pullFfOnly") {
      return <button key={segment.key} className={className} data-command-id="git.pullFfOnly" type="button" disabled={segment.disabled} onClick={onPull}>{segment.label}</button>;
    }
    return <span key={segment.key} className={className}>{segment.label}</span>;
  };

  return (
    <span className="meta-chip ws-chip git-status-pill" title={status.branch?.upstream ?? "Git status"} aria-label={`Git status ${gitStatusSegments(status)}`}>
      <button className="git-status-refresh" data-command-id="git.fetch" type="button" aria-label="Fetch Git status" onClick={onFetch}>
        <RefreshCw aria-hidden="true" size={12} strokeWidth={1.9} />
      </button>
      {changeSegments.length ? changeSegments.map(renderSegment) : <span className="git-status-segment git-status-segment-clean">clean</span>}
      {syncSegments.length ? <span className="git-status-separator" aria-hidden="true">|</span> : null}
      {syncSegments.map(renderSegment)}
    </span>
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
      className={`meta-chip ws-chip workbench-activity-badge workbench-activity-badge-${activity.tone}`}
      data-command-id="workbench.openActivity"
      data-activity-tone={activity.tone}
      type="button"
      title={activity.title}
      aria-label={`Open WorkRoot Activity: ${activity.title}`}
      onClick={onOpenActivity}
    >
      <span className="workbench-activity-badge-icon" aria-hidden="true">
        <Activity size={13} strokeWidth={1.8} />
      </span>
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

function workbenchToggleIcon(toggle: WorkbenchToggle): LucideIcon {
  switch (toggle) {
    case "viewer":
      return Eye;
    case "task":
      return ListTodo;
    case "diagnostics":
      return Stethoscope;
    case "events":
      return Activity;
    case "layout":
      return LayoutPanelTop;
  }
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
  onDocumentSaved: (source: { workRootId: string; path: string; content: string; contentHash: string; sizeBytes: number }) => void,
): WorkbenchEditorGroupModel[] {
  void selectedInstance;
  void supportEntity;
  const dashboardGroups = groups.length > 0 ? groups : initialWorkbenchGroups;
  const readOnlyPanesByGroup = readOnlyWorkbenchPanesByGroup(
    root,
    readOnlyFilePanes,
    readOnlyFilePaneOrderByGroup,
    dashboardGroups,
    onCommand,
    onDocumentSaved,
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

function removePanesFromOrder(
  orderByGroup: WorkbenchPaneOrder,
  paneIds: readonly string[],
): WorkbenchPaneOrder {
  const paneIdSet = new Set(paneIds);
  return Object.fromEntries(
    Object.entries(orderByGroup).map(([groupId, orderedPaneIds]) => [
      groupId,
      orderedPaneIds.filter((paneId) => !paneIdSet.has(paneId)),
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
  onCommand: DashboardCommandDispatcher,
  onDocumentSaved: (source: { workRootId: string; path: string; content: string; contentHash: string; sizeBytes: number }) => void,
): Record<string, WorkbenchPane[]> {
  const panes = readOnlyFilePanes
    .filter((pane) => pane.workRootId === root.id)
    .map((pane) => readOnlyWorkbenchPane(root, pane, onCommand, onDocumentSaved));
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
  onCommand: DashboardCommandDispatcher,
  onDocumentSaved: (source: { workRootId: string; path: string; content: string; contentHash: string; sizeBytes: number }) => void,
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
    body: (
      <ReadOnlyDocumentPane
        pane={pane}
        root={root}
        renderMarkdown={isMarkdownDocumentSource(pane)}
        onCommand={onCommand}
        onDocumentSaved={onDocumentSaved}
      />
    ),
  };
}

function ReadOnlyDocumentPane({
  pane,
  root,
  renderMarkdown,
  onCommand,
  onDocumentSaved,
}: {
  pane: ReadOnlyFilePane;
  root: WorkRootView;
  renderMarkdown: boolean;
  onCommand: DashboardCommandDispatcher;
  onDocumentSaved: (source: { workRootId: string; path: string; content: string; contentHash: string; sizeBytes: number }) => void;
}) {
  const [translationEnabled, setTranslationEnabled] = useState(false);
  const [translationStatus, setTranslationStatus] = useState<
    "idle" | "loading" | "ready" | "unavailable" | "error"
  >("idle");
  const [translationMessage, setTranslationMessage] = useState<string | null>(null);
  const [translationOverlay, setTranslationOverlay] = useState<DocumentTranslationOverlay | undefined>();
  const [documentMode, setDocumentMode] = useState<"view" | "edit">("view");
  const [draft, setDraft] = useState(pane.content);
  const [baseContentHash, setBaseContentHash] = useState(pane.contentHash);
  const [saveState, setSaveState] = useState<DocumentSaveState>("idle");
  const [saveMessage, setSaveMessage] = useState<string | null>(null);

  useEffect(() => {
    const decision = documentDraftContentChangeDecision(saveState);
    if (decision.action === "preserveDraft") {
      setSaveState(decision.saveState);
      setSaveMessage(decision.message);
      return;
    }
    setDraft(pane.content);
    setBaseContentHash(pane.contentHash);
    setTranslationOverlay(undefined);
  }, [pane.content, pane.contentHash]);

  const setModeCommand = (mode: "view" | "edit") => {
    const command = buildDocumentModeSetCommand(pane.workRootId, pane.path, mode);
    onCommand(command, { [command.commandId]: () => setDocumentMode(mode) });
  };

  const revertDraft = () => {
    const command = buildDocumentRevertCommand(pane.workRootId, pane.path);
    onCommand(command, {
      [command.commandId]: () => {
        setDraft(pane.content);
        setBaseContentHash(pane.contentHash);
        setSaveState("idle");
        setSaveMessage(null);
      },
    });
  };

  const saveDraft = () => {
    const command = buildDocumentSaveCommand(pane.workRootId, pane.path);
    onCommand(command, {
      [command.commandId]: () => {
        if (!baseContentHash) {
          setSaveState("error");
          setSaveMessage("Missing base content hash");
          return;
        }
        setSaveState("saving");
        setSaveMessage("Saving");
        void writeWorkRootTextFile(pane.workRootId, {
          path: pane.path,
          baseContentHash,
          content: draft,
        })
          .then((response) => {
            setBaseContentHash(response.contentHash);
            setSaveState("saved");
            setSaveMessage("Saved");
            setTranslationOverlay(undefined);
            onDocumentSaved({
              workRootId: pane.workRootId,
              path: pane.path,
              content: draft,
              contentHash: response.contentHash,
              sizeBytes: response.sizeBytes,
            });
          })
          .catch((error) => {
            const message = error instanceof Error ? error.message : "Save failed";
            setSaveState(documentSaveStateForError(message));
            setSaveMessage(message);
          });
      },
    });
  };

  useEffect(() => {
    if (!renderMarkdown || !translationEnabled || pane.status !== "loaded") {
      return;
    }
    let cancelled = false;
    setTranslationStatus("loading");
    setTranslationMessage("Requesting document translation");
    const payload = buildDocumentTranslationRequestPayload({
      markdown: pane.content,
      workRootId: pane.workRootId,
      path: pane.path,
      title: pane.title,
      targetLocale: "ko",
    });
    void fetchTranslationProviders()
      .then((providers) => {
        const provider = providers.providers.find((candidate) => candidate.configured);
        if (!provider) {
          throw new Error("No translation provider configured");
        }
        return requestDocumentTranslation({
          ...payload,
          provider: {
            id: provider.id,
            model: provider.defaultModel ?? provider.models[0]?.id,
          },
        });
      })
      .then((response) => {
        if (cancelled) {
          return;
        }
        setTranslationOverlay(overlayFromTranslationResponse(response));
        setTranslationStatus(response.status === "failed" ? "error" : "ready");
        setTranslationMessage(
          response.status === "completed"
            ? `Translated to ${response.targetLocale}`
            : `Translation ${response.status}`
        );
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }
        setTranslationOverlay(undefined);
        setTranslationStatus("unavailable");
        setTranslationMessage(error instanceof Error ? error.message : "Translation unavailable");
      });
    return () => {
      cancelled = true;
    };
  }, [pane.content, pane.path, pane.status, pane.title, pane.workRootId, renderMarkdown, translationEnabled]);

  return (
    <div className="readonly-text-pane document-pane ws-pane">
      <div className="readonly-text-pane-header ws-toolbar">
        <div className="readonly-text-pane-title-block">
          <div className="readonly-text-pane-title">{pane.title}</div>
          <div className="readonly-text-pane-path" title={pane.path}>
            {root.label} / {pane.path}
          </div>
        </div>
        <div className="readonly-text-pane-badges">
          <span className="meta-chip ws-chip">{pane.mode}</span>
          <span className="meta-chip ws-chip">read-only</span>
          <span className="meta-chip ws-chip">
            {renderMarkdown ? "markdown" : (pane.languageHint ?? pane.extension ?? "text")}
          </span>
        </div>
      </div>
      {pane.status === "loading" ? (
        <div className="readonly-text-pane-state ws-state-surface">Loading file content</div>
      ) : pane.status === "error" ? (
        <div className="readonly-text-pane-state readonly-text-pane-error ws-state-surface">
          {pane.error ?? "file read failed"}
        </div>
      ) : (
        <>
          {renderMarkdown ? (
            <div className="document-translation-toolbar ws-toolbar">
            <button
              type="button"
              className={`document-translation-toggle${translationEnabled ? " is-active" : ""}`}
              aria-pressed={translationEnabled}
              data-command-id="document.translation.toggle"
              onClick={() => {
                const command = buildDocumentTranslationToggleCommand(pane.workRootId, pane.path);
                onCommand(command, {
                  [command.commandId]: () => {
                    setTranslationEnabled((current) => !current);
                    setTranslationOverlay(undefined);
                    setTranslationStatus("idle");
                    setTranslationMessage(null);
                  },
                });
              }}
            >
              Translate: {translationEnabled ? "on" : "off"}
            </button>
            <span className="document-translation-status" data-translation-status={translationStatus}>
              {translationMessage ?? "Target: Korean"}
            </span>
            </div>
          ) : null}
          <div className="document-edit-toolbar ws-toolbar">
            <div className="document-viewer-segmented" role="group" aria-label="Document mode">
              <button
                type="button"
                className={`document-viewer-segment${documentMode === "view" ? " is-active" : ""}`}
                data-command-id="document.mode.set"
                onClick={() => setModeCommand("view")}
              >
                view
              </button>
              <button
                type="button"
                className={`document-viewer-segment${documentMode === "edit" ? " is-active" : ""}`}
                data-command-id="document.mode.set"
                onClick={() => setModeCommand("edit")}
              >
                edit
              </button>
            </div>
            {documentMode === "edit" ? (
              <div className="document-edit-actions">
                <button type="button" data-command-id="document.save" disabled={saveState === "saving" || draft === pane.content} onClick={saveDraft}>
                  Save
                </button>
                <button type="button" data-command-id="document.revert" disabled={saveState === "saving" || draft === pane.content} onClick={revertDraft}>
                  Revert
                </button>
                <span data-document-save-state={saveState}>{saveMessage ?? (draft === pane.content ? "Clean" : "Unsaved changes")}</span>
              </div>
            ) : null}
          </div>
          {documentMode === "edit" ? (
            <textarea
              className="document-raw-editor ws-code-block"
              value={draft}
              onChange={(event) => {
                setDraft(event.currentTarget.value);
                setSaveState("dirty");
                setSaveMessage("Unsaved changes");
              }}
            />
          ) : renderMarkdown ? (
            <DocumentViewer markdown={pane.content} path={pane.path} overlay={translationOverlay} />
          ) : (
            <pre className="readonly-text-content ws-doc-surface ws-code-block">
              <code>{pane.content}</code>
            </pre>
          )}
        </>
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
  const compactRoot = compactWorkspaceWorkRoot(workspace);

  if (compactRoot) {
    return (
      <div className="resource-group">
        <ResourceRow
          id={compactRoot.id}
          title={compactWorkspaceWorkRootTitle(workspace, compactRoot)}
          presentation="compactWorkRoot"
          state={compactRoot.state}
          depth={0}
          selected={selectedId === compactRoot.id}
          actions={workspace.actions}
          actionEntityId={workspace.id}
          kind={compactRoot.kind}
          availability={compactRoot.availability}
          activation={compactRoot.activation}
          canAddWorktree={compactRoot.kind === "gitPrimaryRoot" || compactRoot.kind === "gitLinkedWorktree"}
          debugMeta={[
            "compact workRoot",
            kindLabel(compactRoot.kind),
            `availability: ${compactRoot.availability}`,
            `activation: ${compactRoot.activation}`,
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
        presentation="workspace"
        state={workspace.state}
        depth={0}
        selected={selectedId === workspace.id}
        actions={workspace.actions}
        actionEntityId={workspace.id}
        canAddWorktree={workspace.workRoots.some((root) => root.kind === "gitPrimaryRoot" || root.kind === "gitLinkedWorktree")}
        debugMeta={["workspace", `${workspace.workRoots.length} roots`]}
        onCommand={onCommand}
      />
      {workspace.workRoots.map((root) => (
        <div key={root.id}>
          <ResourceRow
            id={root.id}
            title={root.label}
            presentation="workRoot"
            state={root.state}
            depth={1}
            selected={selectedId === root.id}
            actions={[]}
            actionEntityId={root.id}
            kind={root.kind}
            availability={root.availability}
            activation={root.activation}
            debugMeta={[
              "workRoot",
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
  presentation,
  state,
  depth,
  selected,
  actions = [],
  actionEntityId = id,
  kind,
  availability,
  activation,
  canAddWorktree = false,
  debugMeta,
  onCommand,
}: {
  id: string;
  title: string;
  presentation: "compactWorkRoot" | "workspace" | "workRoot";
  state: ViewState;
  depth: number;
  selected: boolean;
  actions?: ActionHint[];
  actionEntityId?: string;
  kind?: WorkRootView["kind"];
  availability?: WorkRootView["availability"];
  activation?: WorkRootView["activation"];
  canAddWorktree?: boolean;
  debugMeta: string[];
  onCommand: DashboardCommandDispatcher;
}) {
  const hasWorkspaceRemove = actions.some(
    (action) => action.enabled && action.id === "workspace.remove",
  );
  const [menuOpen, setMenuOpen] = useState(false);
  const tone = resourceRowTone(state, availability, activation);
  const metadataTitle = [title, ...debugMeta, `status: ${state.status}`].join(" · ");
  return (
    <div
      className={`resource-row ws-row resource-row-${tone}${selected ? " resource-row-selected ws-row-selected" : ""}`}
      data-command-id="resource.select"
      data-resource-presentation={presentation}
      data-resource-kind={kind ?? presentation}
      data-resource-activation={activation ?? ""}
      data-resource-availability={availability ?? ""}
      style={{ "--depth": depth } as CSSProperties}
      title={metadataTitle}
    >
      <button
        aria-label={`Select ${resourcePresentationLabel(presentation)} ${title}`}
        className="resource-row-select"
        data-command-id="resource.select"
        title={metadataTitle}
        type="button"
        onClick={() =>
          onCommand({ commandId: "resource.select", payload: { type: "select", entityId: id } })
        }
      >
        <span className="resource-row-main">
          <ResourceGlyph presentation={presentation} />
          <span className="row-title">{title}</span>
          {kind ? (
            <span className="resource-kind-glyph" title={kindLabel(kind)}>
              <WorkRootKindIcon kind={kind} />
            </span>
          ) : null}
        </span>
      </button>
      {hasWorkspaceRemove ? (
        <span className="resource-row-actions workspace-row-menu-wrap">
          <ChromeIconButton
            className="resource-row-action"
            commandId="workspace.menu.open"
            icon={MoreHorizontal}
            label={`More actions for ${title}`}
            onClick={() =>
              onCommand(buildWorkspaceMenuOpenCommand(actionEntityId), {
                "workspace.menu.open": () => setMenuOpen((current) => !current),
              })
            }
          />
          {menuOpen ? (
            <div className="workbench-overflow-menu workspace-row-menu" role="menu">
              {canAddWorktree ? (
              <button
                className="workbench-overflow-item"
                data-command-id="gitWorktreeAdd.open"
                role="menuitem"
                type="button"
                onClick={() => {
                  setMenuOpen(false);
                  onCommand(buildGitWorktreeAddOpenCommand(actionEntityId));
                }}
              >
                <Plus aria-hidden="true" size={14} strokeWidth={1.8} />
                <span>Add worktree...</span>
              </button>
              ) : null}
              <button
                className="workbench-overflow-item workbench-overflow-item-danger"
                data-command-id="workspace.remove"
                role="menuitem"
                type="button"
                onClick={() => {
                  setMenuOpen(false);
                  onCommand(buildWorkspaceRemoveCommand(actionEntityId));
                }}
              >
                <Trash2 aria-hidden="true" size={14} strokeWidth={1.8} />
                <span>Remove workspace...</span>
              </button>
            </div>
          ) : null}
        </span>
      ) : null}
    </div>
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
    <div className="status-pane ws-state-surface">
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
      className={`state-badge ws-badge ${state.loading ? "state-loading" : ""} ${
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
      className={`state-dot ws-state-dot ${state.loading ? "state-loading" : ""} ${
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

function resourcePresentationLabel(
  presentation: "compactWorkRoot" | "workspace" | "workRoot",
) {
  switch (presentation) {
    case "compactWorkRoot":
      return "compact workRoot";
    case "workspace":
      return "workspace";
    case "workRoot":
      return "workRoot";
  }
}

function resourceRowTone(
  state: ViewState,
  availability?: WorkRootView["availability"],
  activation?: WorkRootView["activation"],
) {
  if (state.error || availability === "inaccessible" || availability === "missing") {
    return "error";
  }
  if (state.stale || availability === "moved" || activation === "offline") {
    return "muted";
  }
  return "ready";
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
