import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  CSSProperties,
  Dispatch,
  FormEvent,
  Key,
  ReactNode,
  RefObject,
  SetStateAction,
} from "react";
import {
  Activity,
  Bot,
  BriefcaseBusiness,
  CirclePower,
  Eye,
  File,
  Folder,
  FolderGit2,
  FolderOpen,
  GitBranch,
  History,
  KeyRound,
  Languages,
  MessageSquarePlus,
  Plus,
  PlugZap,
  LayoutPanelTop,
  ListTodo,
  MoreHorizontal,
  PanelsTopLeft,
  Pencil,
  RefreshCw,
  RotateCcw,
  Save,
  Server,
  SquareTerminal,
  Stethoscope,
  Trash2,
  X,
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
import { SerializeAddon } from "@xterm/addon-serialize";
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
import { DocumentRawEditor } from "./documentRawEditor";
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
  buildAgentChatCreateCommand,
  buildTerminalCreateCommand,
  buildWorkbenchOpenActivityCommand,
  buildWorkspaceMenuOpenCommand,
  buildWorkspaceRemoveCommand,
  buildWorkRootActivationCommand,
  buildWorkRootCloseCommand,
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
  findOpenWorkRoot,
  resolveClosedWorkRootRefs,
  loadWorkbenchLayoutRestoreSnapshot,
  saveWorkbenchLayoutRestoreSnapshot,
  mergeWorkbenchLayoutRestoreEntries,
  revalidateWorkbenchLayoutForRoot,
  mergeReadOnlyAndTerminalPaneOrder,
  removePanesFromOrder,
  loadTerminalVisualRestoreSnapshot,
  upsertTerminalVisualRestoreEntry,
  upsertTerminalVisualRestoreEntryInSnapshot,
  resolveTerminalMountWrite,
  terminalVisualRestoreScrollbackLines,
  terminalVisualRestoreDebounceMs,
  type SurfaceKind,
  type WorkbenchPaneCategory,
  type WorkbenchPaneOrder,
  type DockviewTabCloseRequest,
  type WorkbenchPlacementState,
  type WorkbenchLayoutRestoreSnapshot,
  type TerminalVisualRestoreEntry,
  type TerminalVisualRestoreSnapshot,
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
  markTerminalOutputCursor,
  markTerminalPaneCloseError,
  markTerminalPaneVisibilityGated,
  markTerminalSocketStatus,
  loadTerminalRestoreIntents,
  reconcileListedTerminalSessions,
  removeClosedTerminalPane,
  removeTerminalPanesForWorkRoot,
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
  agentChatHarnesses,
  attachAgentChatSession,
  createEmptyAgentChatPane,
  markAgentChatPaneError,
  markAgentChatPaneStarting,
  removeAgentChatPane,
  removeAgentChatPanesForWorkRoot,
  type AgentChatHarness,
  type AgentChatPaneState,
  type AgentChatSessionView,
} from "./agentChatSessions";
import {
  agentChatHarnessLabel,
  stubActivityHistoryList,
  stubBeginStreamingTurn,
  stubResumeAgentChatSession,
  stubStartNewAgentChatSession,
} from "./activitySessionStub";
import { AgentChatTranscriptBubbles } from "./agentChatBubbles";
import {
  compactWorkspaceWorkRoot,
  compactWorkspaceWorkRootTitle,
  dashboardServerRoute,
  flattenEntities,
  isLocalDashboardServerRoute,
  isValidServerRouteSegment,
  reconcileSelectedId,
  serverScopedIdentity,
  workRootActivationEndpoint,
  workspaceEndpoint,
  type ActionHint,
  type DashboardResourcesView,
  type DashboardServersView,
  type InstanceView,
  type ResourceEntity,
  type ResourcePath,
  type ServerConnectionView,
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
import {
  defaultLinkedServerId,
  linkEndpointServer,
  linkServerPassphrase,
  reconnectServerTunnel,
} from "./linkedServers";
import { ActivityConsole } from "./ActivityConsole";
import {
  createResourceRefreshCoordinator,
  requestDashboardResources,
  requestDashboardServers,
  resourceAvailabilityPollIntervalMs,
  type ResourceRefreshCoordinator,
} from "./resourceRefresh";
import {
  applyActivityConsoleEvent,
  fetchWorkRootActivity,
  fetchWorkRootActivityTranscript,
  mergeWorkRootActivityViews,
  parseActivityConsoleEvent,
  shouldApplyActivityStreamRequest,
  workRootActivityBadge,
  workRootActivityEventsEndpoint,
  type ActivityConsoleEvent,
  type ActivityConsoleStreamRequest,
  type ActivityItem,
  type TranscriptBlock,
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

type ServerModalState =
  | { mode: "add" }
  | { mode: "auth"; server: ServerConnectionView };

async function requestWorkRootActivation(
  workRootId: string,
  activation: "online" | "offline",
  serverRoute: string | null | undefined,
): Promise<DashboardResourcesView> {
  const response = await fetch(
    workRootActivationEndpoint(workRootId, serverRoute),
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
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
  serverRoute: string | null | undefined,
): Promise<DashboardResourcesView> {
  const response = await fetch(workspaceEndpoint(workspaceId, serverRoute), {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
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
  const [serversView, setServersView] = useState<DashboardServersView | null>(
    null,
  );
  const [selectedServerId, setSelectedServerId] = useState("server-local");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [gitWorktreeTarget, setGitWorktreeTarget] = useState<{
    serverRoute: string;
    workspaceId: string;
  } | null>(null);
  const [serverModal, setServerModal] = useState<ServerModalState | null>(null);
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
  // Session-lifetime snapshot of persisted per-work-root dockview layouts
  // (group membership, pane order, active pane, best-effort group size),
  // seeded once at mount and kept live for the rest of the browser session
  // by the layout save effect below (which writes every merged save back
  // into this ref). Consumed by the open-work-root effect (seeding a root's
  // layout the first time it is opened this session, including a same-
  // session reopen after an explicit close) and by
  // `DockviewWorkbenchLayout`'s `initialGroupSizeById`, both of which need
  // the ref's current value rather than a value frozen at mount.
  const workbenchLayoutRestoreRef = useRef<WorkbenchLayoutRestoreSnapshot>(
    loadWorkbenchLayoutRestoreSnapshot(),
  );
  // Session-lifetime snapshot of persisted per-terminal visual-buffer
  // restore entries (serialized scrollback/cursor/styles + scroll
  // viewport), seeded once at mount alongside `workbenchLayoutRestoreRef`
  // and kept live for the rest of the browser session by the terminal
  // visual-capture handler (which writes each captured entry back into this
  // ref alongside its existing localStorage upsert). Consumed by the
  // `listTerminals` reattach path (to seed a matching pane's `nextSequence`)
  // and by `TerminalPaneBody`'s mount effect (to replace the plain-text
  // `pane.output` replay with the restored serialized buffer).
  const terminalVisualRestoreRef = useRef<TerminalVisualRestoreSnapshot>(
    loadTerminalVisualRestoreSnapshot(),
  );
  // Work roots the user has visited this session stay mounted (own dockview
  // instance each) instead of being destroyed on selection switch. Ordered,
  // de-duplicated set of `serverScopedIdentity(serverId, rootId)` keys, plus
  // the raw ids needed to re-resolve each root from `resources` without
  // depending on the tree-walk `selectedId`/`selection` state. Lifted here
  // (rather than local to `WorkbenchShell`) so the left panel's close
  // affordance can read membership and trigger removal.
  const [openWorkRootKeys, setOpenWorkRootKeys] = useState<string[]>([]);
  const [openWorkRootRefs, setOpenWorkRootRefs] = useState<
    Record<string, { rootId: string; serverRoute: string }>
  >({});
  const commandSequence = useRef(0);
  const fileOpenSequence = useRef(0);
  const restoredReadOnlyPaneKeys = useRef(
    new Set(Object.keys(initialReadOnlyFilePaneRestore.panes)),
  );
  const resourceRefreshCoordinatorRef =
    useRef<ResourceRefreshCoordinator | null>(null);
  const selectedServerIdRef = useRef(selectedServerId);

  if (!resourceRefreshCoordinatorRef.current) {
    resourceRefreshCoordinatorRef.current = createResourceRefreshCoordinator({
      fetchResources: () =>
        requestDashboardResources(selectedServerIdRef.current),
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

  const loadServers = useCallback(async () => {
    try {
      setServersView(await requestDashboardServers());
    } catch {
      // The selected server resource request already owns the visible error
      // surface; keep the last server list rather than blanking the nav.
    }
  }, []);

  const serverConnections = useMemo(
    () =>
      serversView?.servers ??
      (resources ? [serverViewToConnection(resources.server)] : []),
    [resources, serversView],
  );
  const activeResources =
    resources?.server.id === selectedServerId ? resources : null;

  useEffect(() => {
    selectedServerIdRef.current = selectedServerId;
  }, [selectedServerId]);

  useEffect(() => {
    resourceRefreshCoordinatorRef.current?.resume();
    void loadServers();
    void loadResources("initial");
  }, [loadResources, loadServers]);

  useEffect(() => {
    resourceRefreshCoordinatorRef.current?.resume();
    const interval = window.setInterval(() => {
      void loadServers();
      void loadResources("poll");
    }, resourceAvailabilityPollIntervalMs);

    return () => {
      window.clearInterval(interval);
      resourceRefreshCoordinatorRef.current?.dispose();
    };
  }, [loadResources, loadServers]);

  const handleWorkRootOpened = useCallback(
    (openedView: DashboardResourcesView, requestedWorkRootId?: string) => {
      // Identify the just-opened workRoot: the workRoot present in the
      // aggregated open response but absent from the prior resource view.
      const priorWorkRootIds = new Set(
        flattenEntities(resources)
          .filter((entity) => entity.type === "workRoot")
          .map((entity) => entity.id),
      );
      const openedWorkRootId =
        requestedWorkRootId ??
        flattenEntities(openedView).find(
          (entity) =>
            entity.type === "workRoot" && !priorWorkRootIds.has(entity.id),
        )?.id;

      // Reconcile immediately with the aggregated open response and select the
      // opened workRoot, then re-fetch the canonical endpoint so it stays the
      // source of truth for refresh and re-entry.
      selectedServerIdRef.current = openedView.server.id;
      setSelectedServerId(openedView.server.id);
      resourceRefreshCoordinatorRef.current?.applyExternalResources(openedView);
      if (openedWorkRootId) {
        setSelectedId(openedWorkRootId);
      }
      void loadServers();
      void loadResources("open");
    },
    [loadResources, loadServers, resources],
  );

  useEffect(() => {
    if (!resources) {
      return;
    }

    if (resources.server.id !== selectedServerIdRef.current) {
      selectedServerIdRef.current = resources.server.id;
      setSelectedServerId(resources.server.id);
    }
    normalizeServerRoute(resources.server.id);
  }, [resources]);

  const handleServerSelected = useCallback(
    (server: ServerConnectionView) => {
      selectedServerIdRef.current = server.id;
      setSelectedServerId(server.id);
      setSelectedId(server.id);
      if (server.status === "connected") {
        void loadResources("explicit");
      }
    },
    [loadResources],
  );

  const applyServerConnection = useCallback(
    (server: ServerConnectionView) => {
      setServersView((current) => {
        const servers = current?.servers ?? serverConnections;
        const nextServers = servers.some(
          (candidate) => candidate.id === server.id,
        )
          ? servers.map((candidate) =>
              candidate.id === server.id ? server : candidate,
            )
          : [...servers, server];
        return { servers: nextServers };
      });
      selectedServerIdRef.current = server.id;
      setSelectedServerId(server.id);
      setSelectedId(server.id);
      void loadServers();
      if (server.status === "connected") {
        void loadResources("explicit");
      }
    },
    [loadResources, loadServers, serverConnections],
  );

  const reconnectServer = useCallback(
    (server: ServerConnectionView) => {
      void reconnectServerTunnel(server.id)
        .then(applyServerConnection)
        .catch((nextError) => {
          setError(
            nextError instanceof Error
              ? nextError.message
              : "server reconnect failed",
          );
        });
    },
    [applyServerConnection],
  );

  const entities = useMemo(
    () => flattenEntities(activeResources),
    [activeResources],
  );

  useEffect(() => {
    // Reconcile after every resource change so a selection that left the
    // entity set (the mock workspace once the tree turns live) cannot remain
    // active.
    const serverIds = new Set(serverConnections.map((server) => server.id));
    if (selectedId && serverIds.has(selectedId)) {
      return;
    }
    if (entities.length === 0) {
      const nextSelectedId = selectedId ?? selectedServerId;
      if (nextSelectedId !== selectedId) {
        setSelectedId(nextSelectedId);
      }
      return;
    }
    const nextSelectedId = reconcileSelectedId(entities, selectedId);
    if (nextSelectedId !== selectedId) {
      setSelectedId(nextSelectedId);
    }
  }, [entities, selectedId, selectedServerId, serverConnections]);

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
        workspace.workRoots.map((root) =>
          serverScopedIdentity(root.resourcePath.serverId, root.id),
        ),
      ),
    );
    for (const logicalKey of Array.from(restoredReadOnlyPaneKeys.current)) {
      const pane = readOnlyFilePanes[logicalKey];
      if (!pane || pane.status !== "loading") {
        restoredReadOnlyPaneKeys.current.delete(logicalKey);
        continue;
      }
      if (
        !knownWorkRootIds.has(
          serverScopedIdentity(pane.serverRoute, pane.workRootId),
        )
      ) {
        continue;
      }
      restoredReadOnlyPaneKeys.current.delete(logicalKey);
      void fetchWorkRootTextFile(pane.workRootId, pane.path, pane.serverRoute)
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
    () => resolveWorkbenchSelection(activeResources, selectedId),
    [activeResources, selectedId],
  );
  useEffect(() => {
    if (!workbenchSelection) {
      return;
    }
    const rootKey = serverScopedIdentity(
      workbenchSelection.root.resourcePath.serverId,
      workbenchSelection.root.id,
    );
    const rootId = workbenchSelection.root.id;
    const serverRoute = workbenchSelection.root.resourcePath.serverId;
    setOpenWorkRootKeys((current) =>
      current.includes(rootKey) ? current : [...current, rootKey],
    );
    setOpenWorkRootRefs((current) =>
      current[rootKey]
        ? current
        : { ...current, [rootKey]: { rootId, serverRoute } },
    );
    // Seed this root's layout from a persisted restore snapshot the first
    // time it is opened this session. Guarded on `workbenchGroupsByRoot`
    // absence so this never clobbers an in-session live layout (e.g. a root
    // reopened after an explicit close within the same session already has
    // live state by the time this effect re-runs for it).
    const restoredEntry = workbenchLayoutRestoreRef.current[rootKey];
    if (restoredEntry) {
      setWorkbenchGroupsByRoot((current) =>
        current[rootKey]
          ? current
          : { ...current, [rootKey]: restoredEntry.groups },
      );
      setPaneOrderByRoot((current) =>
        current[rootKey]
          ? current
          : { ...current, [rootKey]: restoredEntry.paneOrderByGroup },
      );
    }
  }, [workbenchSelection]);
  const openWorkRootKeysSet = useMemo(
    () => new Set(openWorkRootKeys),
    [openWorkRootKeys],
  );

  const openReadOnlyFile = useCallback(
    (
      workRoot: WorkRootView,
      entry: WorkRootFileEntryView,
      gesture: ReadOnlyFileOpenGesture = "singleClick",
    ) => {
      const mode = readOnlyFilePaneModeForOpenGesture(gesture);
      const serverRoute = workRoot.resourcePath.serverId;
      const workRootStateKey = serverScopedIdentity(serverRoute, workRoot.id);
      const pane = createLoadingReadOnlyFilePane(
        workRoot.id,
        entry.path,
        mode,
        serverRoute,
      );
      const pinnedLogicalKey = readOnlyFilePaneLogicalKey(
        workRoot.id,
        entry.path,
        "pinned",
        serverRoute,
      );
      const previewLogicalKey = readOnlyFilePaneLogicalKey(
        workRoot.id,
        entry.path,
        "preview",
        serverRoute,
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
          removePaneFromOrder(
            current,
            readOnlyFilePanes[previewLogicalKey]?.id,
          ),
        );
        focusPane(existingPinnedPane.id);
        return;
      }

      const groupsForPlacement =
        workbenchGroupsByRoot[workRootStateKey] ?? initialWorkbenchGroups;
      const placement = decideSurfaceOpenWithDynamicGroups(
        readOnlyFilePlacementState(
          readOnlyFilePanes,
          groupsForPlacement,
          paneOrderByRoot[workRootStateKey] ?? {},
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
          [workRootStateKey]: reconcileDashboardGroupsForPlacement(
            current[workRootStateKey] ?? groupsForPlacement,
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
        let next =
          mode === "pinned"
            ? removePaneFromOrder(
                current,
                readOnlyFilePanes[previewLogicalKey]?.id,
              )
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

      void fetchWorkRootTextFile(workRoot.id, entry.path, serverRoute)
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
          void loadServers();
          void loadResources("explicit");
        };
      } else if (command.payload.type === "workRoot.activation.set") {
        const { workRootId, activation, serverRoute } = command.payload;
        executableHandlers[command.commandId] = () => {
          void requestWorkRootActivation(workRootId, activation, serverRoute)
            .then((nextResources) => {
              resourceRefreshCoordinatorRef.current?.applyExternalResources(
                nextResources,
              );
            })
            .catch((nextError) => {
              setError(
                nextError instanceof Error
                  ? nextError.message
                  : "activation failed",
              );
            });
        };
      } else if (command.payload.type === "gitWorktreeAdd.open") {
        const { workspaceId, serverRoute } = command.payload;
        executableHandlers[command.commandId] = () =>
          setGitWorktreeTarget({
            serverRoute: serverRoute ?? "server-local",
            workspaceId,
          });
      } else if (command.payload.type === "gitWorktreeAdd.close") {
        executableHandlers[command.commandId] = () =>
          setGitWorktreeTarget(null);
      } else if (command.payload.type === "workRoot.close") {
        const { workRootId, serverRoute } = command.payload;
        executableHandlers[command.commandId] = () => {
          const closeServerRoute = serverRoute ?? "server-local";
          const rootKey = serverScopedIdentity(closeServerRoute, workRootId);
          // This only stops rendering the root's DockviewWorkbenchLayout
          // instance (which unmounts its dockview panels and fires their
          // existing xterm dispose/socket-close cleanup) - it deliberately
          // does not call closeTerminal()/closeTerminalPane, since the
          // daemon terminal session must stay alive for a future reopen to
          // reattach by id.
          setOpenWorkRootKeys((current) =>
            current.filter((key) => key !== rootKey),
          );
          setOpenWorkRootRefs((current) => {
            if (!(rootKey in current)) {
              return current;
            }
            const next = { ...current };
            delete next[rootKey];
            return next;
          });
          setWorkbenchGroupsByRoot((current) => {
            if (!(rootKey in current)) {
              return current;
            }
            const next = { ...current };
            delete next[rootKey];
            return next;
          });
          setPaneOrderByRoot((current) => {
            if (!(rootKey in current)) {
              return current;
            }
            const next = { ...current };
            delete next[rootKey];
            return next;
          });
        };
      } else if (command.payload.type === "workspace.remove") {
        const { workspaceId, serverRoute } = command.payload;
        executableHandlers[command.commandId] = () => {
          const workspace = activeResources?.workspaces.find(
            (candidate) => candidate.id === workspaceId,
          );
          if (
            !window.confirm(
              "Remove this workspace from the dashboard? Files and Git worktrees on disk will not be deleted.",
            )
          ) {
            return;
          }
          const removalServerRoute = serverRoute ?? "server-local";
          const removedRootKeys = new Set(
            workspace?.workRoots
              .filter(
                (root) => root.resourcePath.serverId === removalServerRoute,
              )
              .map((root) =>
                serverScopedIdentity(removalServerRoute, root.id),
              ) ?? [],
          );
          void requestWorkspaceRemoval(workspaceId, serverRoute)
            .then((nextResources) => {
              resourceRefreshCoordinatorRef.current?.applyExternalResources(
                nextResources,
              );
              if (removedRootKeys.size > 0) {
                setReadOnlyFilePanes((current) =>
                  Object.fromEntries(
                    Object.entries(current).filter(
                      ([, pane]) =>
                        !removedRootKeys.has(
                          serverScopedIdentity(pane.serverRoute, pane.workRootId),
                        ),
                    ),
                  ),
                );
                setReadOnlyFilePaneOrderByGroup((current) =>
                  removePanesFromOrder(
                    current,
                    Object.values(readOnlyFilePanes)
                      .filter((pane) =>
                        removedRootKeys.has(
                          serverScopedIdentity(
                            pane.serverRoute,
                            pane.workRootId,
                          ),
                        ),
                      )
                      .map((pane) => pane.id),
                  ),
                );
                setPaneOrderByRoot((current) =>
                  Object.fromEntries(
                    Object.entries(current).filter(
                      ([rootKey]) => !removedRootKeys.has(rootKey),
                    ),
                  ),
                );
                setWorkbenchGroupsByRoot((current) =>
                  Object.fromEntries(
                    Object.entries(current).filter(
                      ([rootKey]) => !removedRootKeys.has(rootKey),
                    ),
                  ),
                );
              }
            })
            .catch((nextError) => {
              setError(
                nextError instanceof Error
                  ? nextError.message
                  : "workspace removal failed",
              );
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
    [activeResources, loadResources, loadServers, readOnlyFilePanes],
  );

  const applyDocumentSaved = useCallback(
    (source: {
      serverRoute?: string;
      workRootId: string;
      path: string;
      content: string;
      contentHash: string;
      sizeBytes: number;
    }) => {
      setReadOnlyFilePanes((current) =>
        Object.fromEntries(
          Object.entries(current).map(([key, pane]) => [
            key,
            pane.serverRoute === (source.serverRoute ?? "server-local") &&
            pane.workRootId === source.workRootId &&
            pane.path === source.path
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
        <aside
          className="shell-panel shell-panel-nav ws-panel"
          aria-label="Resources"
        >
          <ResourceNavigation
            resources={activeResources}
            servers={serverConnections}
            selectedServerId={selectedServerId}
            loading={loading}
            error={error}
            selectedId={selectedId}
            selectedWorkRoot={workbenchSelection?.root ?? null}
            openWorkRootKeys={openWorkRootKeysSet}
            onOpenWorkRoot={handleWorkRootOpened}
            onOpenAddServer={() => setServerModal({ mode: "add" })}
            onOpenServerAuth={(server) =>
              setServerModal({ mode: "auth", server })
            }
            onReconnectServer={reconnectServer}
            onSelectServer={handleServerSelected}
            onCommand={executeCommand}
            onOpenFile={openReadOnlyFile}
          />
          <GitWorktreeAddModal
            target={gitWorktreeTarget}
            onCommand={executeCommand}
            onClose={() => setGitWorktreeTarget(null)}
            onCreated={(response) => {
              resourceRefreshCoordinatorRef.current?.applyExternalResources(
                response.resources,
              );
              if (response.createdWorkRootId) {
                setSelectedId(response.createdWorkRootId);
              }
              setGitWorktreeTarget(null);
            }}
          />
          <LinkedServerModal
            state={serverModal}
            onClose={() => setServerModal(null)}
            onLinked={(server) => {
              applyServerConnection(server);
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
            resources={activeResources}
            selectedEntity={selectedEntity}
            selection={workbenchSelection}
            workbenchGroupsByRoot={workbenchGroupsByRoot}
            paneOrderByRoot={paneOrderByRoot}
            openWorkRootKeys={openWorkRootKeys}
            openWorkRootRefs={openWorkRootRefs}
            workbenchLayoutRestoreRef={workbenchLayoutRestoreRef}
            terminalVisualRestoreRef={terminalVisualRestoreRef}
            onCommand={executeCommand}
            onWorkbenchGroupsByRootChange={setWorkbenchGroupsByRoot}
            onPaneOrderByRootChange={setPaneOrderByRoot}
            onOpenWorkRootKeysChange={setOpenWorkRootKeys}
            onOpenWorkRootRefsChange={setOpenWorkRootRefs}
            readOnlyFilePanes={Object.values(readOnlyFilePanes)}
            readOnlyFilePaneOrderByGroup={readOnlyFilePaneOrderByGroup}
            activeReadOnlyFilePaneRequest={activeReadOnlyFilePaneRequest}
            onReadOnlyFilePanesChange={setReadOnlyFilePanes}
            onReadOnlyFilePaneOrderByGroupChange={
              setReadOnlyFilePaneOrderByGroup
            }
            onDocumentSaved={applyDocumentSaved}
          />
        </section>
      </div>
    </main>
  );
}

function useDismissableMenu(
  open: boolean,
  containerRef: RefObject<HTMLElement | null>,
  onDismiss: () => void,
) {
  useEffect(() => {
    if (!open) {
      return;
    }
    const dismissIfOutside = (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Node) || containerRef.current?.contains(target)) {
        return;
      }
      onDismiss();
    };
    const dismissOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onDismiss();
      }
    };
    document.addEventListener("click", dismissIfOutside);
    document.addEventListener("keydown", dismissOnEscape, true);
    return () => {
      document.removeEventListener("click", dismissIfOutside);
      document.removeEventListener("keydown", dismissOnEscape, true);
    };
  }, [containerRef, onDismiss, open]);
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
  const Icon =
    kind === "plainDirectory"
      ? Folder
      : kind === "gitLinkedWorktree"
        ? GitBranch
        : FolderGit2;
  return <Icon aria-hidden="true" size={14} strokeWidth={1.8} />;
}

function serverViewToConnection(server: ServerView): ServerConnectionView {
  return {
    id: server.id,
    label: server.label,
    kind: "local",
    status: "connected",
    state: server.state,
    actions: server.actions,
  };
}

function serverConnectionStatusLabel(server: ServerConnectionView): string {
  switch (server.status) {
    case "connected":
      return "connected";
    case "authRequired":
      return "auth required";
    case "tunnelRequired":
      return "tunnel required";
    case "staleEndpoint":
      return "stale endpoint";
    case "starting":
      return "starting";
    case "unreachable":
      return "unreachable";
  }
}

function ToggleIcon({ toggle }: { toggle: WorkbenchToggle }) {
  const Icon = workbenchToggleIcon(toggle);
  return <Icon aria-hidden="true" size={14} strokeWidth={1.8} />;
}

function OpenWorkRootControl({
  server,
  onOpened,
  onCommand,
  variant = "section",
  disabled = false,
}: {
  server?: Pick<ServerConnectionView, "id" | "label"> | null;
  onOpened: (
    view: DashboardResourcesView,
    requestedWorkRootId?: string,
  ) => void;
  onCommand: DashboardCommandDispatcher;
  variant?: "section" | "icon";
  disabled?: boolean;
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
  const pickerOpenRef = useRef(open);
  const pickerRequestSequence = useRef(0);
  const pickerServerRoute = dashboardServerRoute(server?.id);
  const pickerServerLabel = server?.label ?? "Local ws dashboard";
  const pickerIsLocal = isLocalDashboardServerRoute(pickerServerRoute);
  const pickerContextLabel = pickerIsLocal ? "this host" : pickerServerLabel;

  const loadPicker = useCallback(
    async (path: string | null, historyMode: "push" | "replace" = "push") => {
      const requestSequence = pickerRequestSequence.current + 1;
      pickerRequestSequence.current = requestSequence;
      setLoading(true);
      setError(null);
      try {
        const view = await fetchRootPicker(path, pickerServerRoute);
        if (
          pickerRequestSequence.current !== requestSequence ||
          !pickerOpenRef.current
        ) {
          return;
        }
        setPickerView(view);
        setSelectedPath(view.currentPath);
        setAddressPath(view.currentPath);
        setExactPath(view.currentPath);
        if (historyMode === "push") {
          setHistory((current) =>
            rootPickerHistoryPush(current, view.currentPath),
          );
        }
      } catch (nextError) {
        if (
          pickerRequestSequence.current !== requestSequence ||
          !pickerOpenRef.current
        ) {
          return;
        }
        setError(
          nextError instanceof Error ? nextError.message : "picker load failed",
        );
      } finally {
        if (pickerRequestSequence.current === requestSequence) {
          setLoading(false);
        }
      }
    },
    [pickerServerRoute],
  );

  useEffect(() => {
    pickerRequestSequence.current += 1;
    setPickerView(null);
    setSelectedPath(null);
    setAddressPath("");
    setExactPath("");
    setCreateName("");
    setHistory(rootPickerHistoryInitial());
    setLoading(false);
    setPendingOpen(false);
    setCreating(false);
    setPinningPath(null);
    setError(null);
  }, [pickerServerRoute]);

  useEffect(() => {
    if (!open || pickerView || loading) {
      return;
    }
    void loadPicker(null);
  }, [loadPicker, loading, open, pickerView]);

  useEffect(() => {
    pickerOpenRef.current = open;
    if (wasOpenRef.current && !open) {
      openerRef.current?.focus();
    }
    wasOpenRef.current = open;
  }, [open]);

  const closePicker = () => {
    onCommand(buildRootPickerCloseCommand(pickerServerRoute), {
      "rootPicker.close": () => {
        pickerRequestSequence.current += 1;
        setLoading(false);
        setOpen(false);
      },
    });
  };

  const openPicker = () => {
    onCommand(buildRootPickerOpenCommand(pickerServerRoute), {
      "rootPicker.open": () => {
        pickerOpenRef.current = true;
        setError(null);
        setOpen(true);
      },
    });
  };

  const navigateTo = (
    path: string,
    historyMode: "push" | "replace" = "push",
  ) => {
    onCommand(buildRootPickerNavigateCommand(path, pickerServerRoute), {
      "rootPicker.navigate": () => {
        void loadPicker(path, historyMode);
      },
    });
  };

  const selectDirectory = (path: string) => {
    onCommand(buildRootPickerSelectDirectoryCommand(path, pickerServerRoute), {
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

    onCommand(buildWorkRootOpenCommand(requestedPath, pickerServerRoute), {
      "workRoot.open": () => {
        setPendingOpen(true);
        setError(null);
        void requestOpenWorkRoot(requestedPath, pickerServerRoute)
          .then((result) => {
            pickerRequestSequence.current += 1;
            pickerOpenRef.current = false;
            setLoading(false);
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
            setError(
              nextError instanceof Error ? nextError.message : "open failed",
            );
          })
          .finally(() => {
            setPendingOpen(false);
          });
      },
    });
  };

  const createDirectory = () => {
    const parentPath = pickerView?.currentPath;
    const name = createName.trim();
    if (!parentPath || name.length === 0 || creating) {
      return;
    }
    onCommand(
      buildRootPickerCreateDirectoryCommand(
        parentPath,
        name,
        pickerServerRoute,
      ),
      {
        "rootPicker.createDirectory": () => {
          setCreating(true);
          setError(null);
          void createRootPickerDirectory(parentPath, name, pickerServerRoute)
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
              setError(
                nextError instanceof Error
                  ? nextError.message
                  : "create failed",
              );
            })
            .finally(() => {
              setCreating(false);
            });
        },
      },
    );
  };

  const updatePickerPlaces = (places: RootPickerView["places"]) => {
    setPickerView((current) => (current ? { ...current, places } : current));
  };

  const pinDirectory = (path: string) => {
    if (pinningPath) {
      return;
    }
    onCommand(buildRootPickerPinDirectoryCommand(path, pickerServerRoute), {
      "rootPicker.pinDirectory": () => {
        setPinningPath(path);
        setError(null);
        void pinRootPickerDirectory(path, pickerServerRoute)
          .then((view) => updatePickerPlaces(view.places))
          .catch((nextError) => {
            setError(
              nextError instanceof Error ? nextError.message : "pin failed",
            );
          })
          .finally(() => setPinningPath(null));
      },
    });
  };

  const unpinDirectory = (path: string) => {
    if (pinningPath) {
      return;
    }
    onCommand(buildRootPickerUnpinDirectoryCommand(path, pickerServerRoute), {
      "rootPicker.unpinDirectory": () => {
        setPinningPath(path);
        setError(null);
        void unpinRootPickerDirectory(path, pickerServerRoute)
          .then((view) => updatePickerPlaces(view.places))
          .catch((nextError) => {
            setError(
              nextError instanceof Error ? nextError.message : "unpin failed",
            );
          })
          .finally(() => setPinningPath(null));
      },
    });
  };

  const selectedLabel = selectedPath
    ? rootPickerEntryLabel(selectedPath)
    : "None";
  const selectedEntry = pickerView?.entries.find(
    (entry) => entry.path === selectedPath,
  );
  const visibleEntries = rootPickerVisibleEntries(pickerView?.entries ?? []);
  const visiblePlaces = rootPickerVisiblePlaces(pickerView);
  const pinnedPaths = rootPickerPinnedPathSet(pickerView);
  const selectedPathIsPinned = selectedPath
    ? pinnedPaths.has(selectedPath)
    : false;

  const openerButton = (
    <button
      ref={openerRef}
      aria-label="Open workRoot"
      className="icon-button icon-button-primary"
      data-command-id="rootPicker.open"
      disabled={disabled}
      title={
        disabled
          ? `Open workRoot is unavailable for ${pickerServerLabel}`
          : `Open workRoot on ${pickerContextLabel}`
      }
      type="button"
      onClick={openPicker}
    >
      <FolderOpen aria-hidden="true" size={15} strokeWidth={1.8} />
    </button>
  );

  return (
    <div
      className={
        variant === "icon"
          ? "open-work-root open-work-root-icon"
          : "open-work-root"
      }
      aria-label="Open workRoot"
    >
      {variant === "section" ? (
        <div className="open-work-root-row">
          <div>
            <div className="section-label">Open workRoot</div>
            <div className="open-work-root-summary">
              Choose a directory from {pickerContextLabel}
            </div>
          </div>
          {openerButton}
        </div>
      ) : (
        openerButton
      )}
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
            <div className="root-picker-titlebar">
              <Heading className="root-picker-title" slot="title">
                Open workRoot on {pickerContextLabel}
              </Heading>
              <div className="root-picker-window-actions">
                <ChromeIconButton
                  className="root-picker-close-button"
                  commandId="rootPicker.close"
                  icon={X}
                  label="Close"
                  onClick={closePicker}
                />
              </div>
            </div>
            <div
              className="root-picker-current root-picker-context"
              title={pickerView?.currentPath ?? ""}
            >
              {pickerView?.currentPath ??
                `Loading directories from ${pickerContextLabel}`}
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
                        place.source === "pin"
                          ? "root-picker-place-row-pinned"
                          : ""
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
                        <span className="root-picker-place-label">
                          {place.label}
                        </span>
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

              <section
                className="root-picker-list-region"
                aria-label="Current folder"
              >
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
                    selectedKeys={
                      selectedPath ? new Set([selectedPath]) : new Set()
                    }
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
                        <span
                          className="root-picker-row-icon"
                          aria-hidden="true"
                        >
                          /
                        </span>
                        <span
                          className="root-picker-row-name"
                          title={entry.path}
                        >
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

            {error ? (
              <InlineNotice tone="error" title="Root picker" detail={error} />
            ) : null}

            <div className="root-picker-create">
              <label
                className="section-label"
                htmlFor="root-picker-create-name"
              >
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
                  disabled={
                    !pickerView || createName.trim().length === 0 || creating
                  }
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
              <label
                className="root-picker-selection"
                htmlFor="root-picker-exact-path"
              >
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
  target,
  onCommand,
  onClose,
  onCreated,
}: {
  target: { serverRoute: string; workspaceId: string } | null;
  onCommand: DashboardCommandDispatcher;
  onClose: () => void;
  onCreated: (response: {
    resources: DashboardResourcesView;
    createdWorkRootId?: string;
  }) => void;
}) {
  const workspaceId = target?.workspaceId ?? null;
  const serverRoute = target?.serverRoute ?? null;
  const [options, setOptions] = useState<GitWorktreeAddOptions | null>(null);
  const [worktreeName, setWorktreeName] = useState("");
  const [branchMode, setBranchMode] = useState<"auto" | "manual">("auto");
  const [manualBranch, setManualBranch] = useState("");
  const [pathMode, setPathMode] = useState<"auto" | "custom">("auto");
  const [customPath, setCustomPath] = useState("");
  const [preview, setPreview] = useState<GitWorktreeAddPreview | null>(null);
  const [previewRequestKey, setPreviewRequestKey] = useState<string | null>(
    null,
  );
  const previewSequenceRef = useRef(0);
  const currentRequestKeyRef = useRef<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId || !serverRoute) {
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
    void fetchGitWorktreeAddOptions(workspaceId, serverRoute)
      .then(setOptions)
      .catch((nextError) =>
        setError(
          nextError instanceof Error
            ? nextError.message
            : "worktree options failed",
        ),
      )
      .finally(() => setLoading(false));
  }, [serverRoute, workspaceId]);

  const request = useMemo<GitWorktreeAddPreviewRequest | null>(() => {
    if (!workspaceId) {
      return null;
    }
    return {
      worktreeName,
      branch:
        branchMode === "auto"
          ? { mode: "auto" }
          : { mode: "manual", name: manualBranch },
      path:
        pathMode === "auto"
          ? { mode: "auto" }
          : { mode: "custom", targetPath: customPath },
    };
  }, [
    branchMode,
    customPath,
    manualBranch,
    pathMode,
    worktreeName,
    workspaceId,
  ]);

  const requestKey = request ? JSON.stringify(request) : null;

  useEffect(() => {
    currentRequestKeyRef.current = requestKey;
  }, [requestKey]);

  useEffect(() => {
    if (
      !workspaceId ||
      !request ||
      !requestKey ||
      worktreeName.trim().length === 0
    ) {
      setPreview(null);
      setPreviewRequestKey(null);
      return;
    }
    const sequence = previewSequenceRef.current + 1;
    previewSequenceRef.current = sequence;
    setPreview(null);
    setPreviewRequestKey(null);
    const timer = window.setTimeout(() => {
      void previewGitWorktreeAdd(workspaceId, request, serverRoute)
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
          setError(
            nextError instanceof Error
              ? nextError.message
              : "worktree preview failed",
          );
        });
    }, 150);
    return () => window.clearTimeout(timer);
  }, [request, requestKey, serverRoute, worktreeName, workspaceId]);

  if (!workspaceId || !serverRoute) {
    return null;
  }

  const close = () => {
    onCommand(buildGitWorktreeAddCloseCommand(workspaceId, serverRoute), {
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
    onCommand(buildGitWorktreeAddSubmitCommand(workspaceId, serverRoute), {
      "gitWorktreeAdd.submit": () => {
        const submittedRequestKey = requestKey;
        setSubmitting(true);
        setError(null);
        void submitGitWorktreeAdd(
          workspaceId,
          { ...request, activate: true },
          serverRoute,
        )
          .then(onCreated)
          .catch((nextError) => {
            if (
              nextError instanceof GitWorktreeAddSubmitError &&
              nextError.preview
            ) {
              if (currentRequestKeyRef.current !== submittedRequestKey) {
                return;
              }
              setPreview(nextError.preview);
              setPreviewRequestKey(submittedRequestKey);
              setError("Submit blocked by current server validation");
              return;
            }
            setError(
              nextError instanceof Error
                ? nextError.message
                : "worktree add failed",
            );
          })
          .finally(() => setSubmitting(false));
      },
    });
  };

  const severity = preview?.status ?? "blocked";
  const manualBranchOptions = (options?.branches ?? []).filter(
    (branch) => !branch.checkedOut,
  );
  const autoBranchDisplay = preview?.branchName ?? worktreeName.trim();
  const autoPathDisplay =
    preview?.targetPathLabel ??
    (worktreeName.trim()
      ? `${options?.defaults.worktreeBaseDirLabel ?? ".git/ws-worktree"}/${worktreeName.trim()}`
      : "");
  return (
    <ModalOverlay
      className="root-picker-backdrop"
      isDismissable
      isOpen
      onOpenChange={(isOpen) => {
        if (!isOpen) close();
      }}
    >
      <Modal className="root-picker-modal git-worktree-modal">
        <Dialog aria-label="Add Git worktree" className="root-picker-dialog">
          <div className="root-picker-titlebar">
            <Heading className="root-picker-title" slot="title">
              Add worktree
            </Heading>
            <div className="root-picker-window-actions">
              <ChromeIconButton
                className="root-picker-close-button"
                commandId="gitWorktreeAdd.close"
                icon={X}
                label="Close"
                onClick={close}
              />
            </div>
          </div>
          <div className="root-picker-current root-picker-context">
            {options?.git.rootLabel ?? "Loading Git workspace"}
          </div>
          <form className="git-worktree-form" onSubmit={submit}>
            <label className="git-worktree-field">
              <span className="section-label">Worktree name</span>
              <input
                className="root-picker-input"
                autoComplete="off"
                value={worktreeName}
                onChange={(event) => setWorktreeName(event.target.value)}
                placeholder="feature-name"
              />
            </label>
            <fieldset className="git-worktree-fieldset">
              <legend className="section-label">Branch</legend>
              <div className="git-worktree-radio-grid">
                <label>
                  <input
                    type="radio"
                    checked={branchMode === "auto"}
                    onChange={() => setBranchMode("auto")}
                  />{" "}
                  Auto from name
                </label>
                <label>
                  <input
                    type="radio"
                    checked={branchMode === "manual"}
                    onChange={() => setBranchMode("manual")}
                  />{" "}
                  Existing/manual
                </label>
              </div>
              {branchMode === "auto" ? (
                <input
                  className="root-picker-input git-worktree-derived-input"
                  readOnly
                  value={autoBranchDisplay}
                  placeholder="derived from worktree name"
                />
              ) : (
                <label
                  className="git-worktree-select-wrap"
                  aria-label="Existing or manual branch"
                >
                  <select
                    className="root-picker-input git-worktree-select"
                    value={manualBranch}
                    onChange={(event) => setManualBranch(event.target.value)}
                  >
                    <option value="">Select or type below…</option>
                    {manualBranchOptions.map((branch) => (
                      <option key={branch.name} value={branch.name}>
                        {branch.name}
                        {branch.current ? " (current)" : ""}
                      </option>
                    ))}
                  </select>
                  <input
                    className="root-picker-input"
                    value={manualBranch}
                    onChange={(event) => setManualBranch(event.target.value)}
                    placeholder="or type branch-name"
                  />
                </label>
              )}
            </fieldset>
            <fieldset className="git-worktree-fieldset">
              <legend className="section-label">Path</legend>
              <div className="git-worktree-radio-grid">
                <label>
                  <input
                    type="radio"
                    checked={pathMode === "auto"}
                    onChange={() => setPathMode("auto")}
                  />{" "}
                  Auto path
                </label>
                <label>
                  <input
                    type="radio"
                    checked={pathMode === "custom"}
                    onChange={() => setPathMode("custom")}
                  />{" "}
                  Custom path
                </label>
              </div>
              <input
                className="root-picker-input"
                readOnly={pathMode === "auto"}
                value={pathMode === "auto" ? autoPathDisplay : customPath}
                onChange={(event) => setCustomPath(event.target.value)}
                placeholder={
                  pathMode === "auto"
                    ? "derived from worktree name"
                    : "/path/to/worktree"
                }
              />
            </fieldset>
            {options && !options.git.available ? (
              <InlineNotice
                tone="error"
                title="Git unavailable"
                detail={options.git.reason ?? "workspace is not Git-capable"}
              />
            ) : null}
            {preview ? (
              <div
                className={`git-worktree-preview git-worktree-preview-${severity}`}
                role="status"
              >
                <strong>{preview.message}</strong>
                <span>
                  {preview.branchName
                    ? `Branch: ${preview.branchName}`
                    : "Branch pending"}
                </span>
                <span>
                  {preview.targetPathLabel
                    ? `Target: ${preview.targetPathLabel}`
                    : "Target pending"}
                </span>
                {preview.blockers.map((blocker) => (
                  <span key={`${blocker.code}:${blocker.field ?? ""}`}>
                    {blocker.message}
                  </span>
                ))}
              </div>
            ) : loading ? (
              <InlineNotice
                tone="info"
                title="Loading"
                detail="Git worktree options"
              />
            ) : null}
            {error ? (
              <InlineNotice tone="error" title="Add worktree" detail={error} />
            ) : null}
            <div className="root-picker-footer-actions">
              <button
                className="action-button action-button-primary"
                data-command-id="gitWorktreeAdd.submit"
                disabled={submitDisabled}
                type="submit"
              >
                {submitting ? "Creating" : "Create worktree"}
              </button>
              <button
                className="action-button"
                data-command-id="gitWorktreeAdd.close"
                type="button"
                onClick={close}
              >
                Cancel
              </button>
            </div>
          </form>
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}

function LinkedServerModal({
  state,
  onClose,
  onLinked,
}: {
  state: ServerModalState | null;
  onClose: () => void;
  onLinked: (server: ServerConnectionView) => void;
}) {
  const [label, setLabel] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!state) {
      return;
    }
    setError(null);
    setSubmitting(false);
    setPassphrase("");
    if (state.mode === "add") {
      setLabel("");
      setEndpoint("");
    } else {
      setLabel(state.server.label);
      setEndpoint("");
    }
  }, [state]);

  if (!state) {
    return null;
  }

  const addMode = state.mode === "add";
  const title = addMode ? "Add server" : `Authenticate ${state.server.label}`;
  const submitDisabled =
    submitting ||
    (addMode && (label.trim().length === 0 || endpoint.trim().length === 0)) ||
    (!addMode && passphrase.trim().length === 0);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (submitDisabled) {
      return;
    }
    setError(null);
    const passphraseValue = passphrase.trim();
    let request: Promise<ServerConnectionView>;
    if (addMode) {
      const serverRoute = defaultLinkedServerId(label, endpoint);
      if (!isValidServerRouteSegment(serverRoute)) {
        setError(
          "Server route must contain only letters, digits, hyphen, or underscore (dot is reserved).",
        );
        return;
      }
      request = linkEndpointServer({
        serverId: serverRoute,
        label: label.trim(),
        endpoint: endpoint.trim(),
        ...(passphraseValue ? { passphrase: passphraseValue } : {}),
      });
    } else {
      request = linkServerPassphrase(state.server.id, passphraseValue);
    }
    setSubmitting(true);
    void request
      .then((server) => {
        onLinked(server);
        if (server.status === "connected") {
          onClose();
          return;
        }
        if (addMode && passphraseValue.length === 0) {
          onClose();
          return;
        }
        setError(
          "Passphrase was not accepted; the server is saved and still requires authentication.",
        );
      })
      .catch((nextError) => {
        setError(
          nextError instanceof Error ? nextError.message : "server link failed",
        );
      })
      .finally(() => setSubmitting(false));
  };

  return (
    <ModalOverlay
      className="root-picker-backdrop"
      isDismissable
      isOpen
      onOpenChange={(isOpen) => {
        if (!isOpen) {
          onClose();
        }
      }}
    >
      <Modal className="root-picker-modal linked-server-modal">
        <Dialog aria-label={title} className="root-picker-dialog">
          <div className="root-picker-titlebar">
            <Heading className="root-picker-title" slot="title">
              {title}
            </Heading>
            <div className="root-picker-window-actions">
              <ChromeIconButton
                className="root-picker-close-button"
                commandId="resource.action.server.modal.close"
                icon={X}
                label="Close"
                onClick={onClose}
              />
            </div>
          </div>
          <form className="linked-server-form" onSubmit={submit}>
            {addMode ? (
              <>
                <label className="linked-server-field">
                  <span className="section-label">Name</span>
                  <input
                    className="root-picker-input"
                    autoComplete="off"
                    value={label}
                    onChange={(event) => setLabel(event.target.value)}
                    placeholder="Remote dev"
                  />
                </label>
                <label className="linked-server-field">
                  <span className="section-label">Endpoint</span>
                  <input
                    className="root-picker-input"
                    autoComplete="off"
                    spellCheck={false}
                    value={endpoint}
                    onChange={(event) => setEndpoint(event.target.value)}
                    placeholder="http://127.0.0.1:49170"
                  />
                </label>
                <div className="linked-server-hint">
                  Use an endpoint already reachable from this host, including a
                  loopback tunnel you created outside the dashboard.
                </div>
              </>
            ) : (
              <div className="linked-server-hint">
                Enter the daemon-lifetime passphrase printed by the remote
                dashboard daemon.
              </div>
            )}
            <label className="linked-server-field">
              <span className="section-label">Passphrase</span>
              <input
                className="root-picker-input"
                autoComplete="off"
                spellCheck={false}
                type="password"
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
                placeholder={addMode ? "Optional" : "Required"}
              />
            </label>
            {error ? (
              <InlineNotice tone="error" title="Server link" detail={error} />
            ) : null}
            <div className="root-picker-footer-actions">
              <button
                className="action-button action-button-primary"
                data-command-id="resource.action.server.link.submit"
                disabled={submitDisabled}
                type="submit"
              >
                {submitting
                  ? "Connecting"
                  : addMode
                    ? "Connect"
                    : "Authenticate"}
              </button>
              <button
                className="action-button"
                data-command-id="resource.action.server.modal.close"
                type="button"
                onClick={onClose}
              >
                Cancel
              </button>
            </div>
          </form>
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}

function ResourceNavigation({
  resources,
  servers,
  selectedServerId,
  loading,
  error,
  selectedId,
  selectedWorkRoot,
  openWorkRootKeys,
  onOpenWorkRoot,
  onOpenAddServer,
  onOpenServerAuth,
  onReconnectServer,
  onSelectServer,
  onCommand,
  onOpenFile,
}: {
  resources: DashboardResourcesView | null;
  servers: ServerConnectionView[];
  selectedServerId: string;
  loading: boolean;
  error: string | null;
  selectedId: string | null;
  selectedWorkRoot: WorkRootView | null;
  openWorkRootKeys: ReadonlySet<string>;
  onOpenWorkRoot: (
    view: DashboardResourcesView,
    requestedWorkRootId?: string,
  ) => void;
  onOpenAddServer: () => void;
  onOpenServerAuth: (server: ServerConnectionView) => void;
  onReconnectServer: (server: ServerConnectionView) => void;
  onSelectServer: (server: ServerConnectionView) => void;
  onCommand: DashboardCommandDispatcher;
  onOpenFile: (
    workRoot: WorkRootView,
    entry: WorkRootFileEntryView,
    gesture: ReadOnlyFileOpenGesture,
  ) => void;
}) {
  const selectedServer = servers.find(
    (server) => server.id === selectedServerId,
  );
  const serverStatusDetail = selectedServer
    ? serverConnectionStatusLabel(selectedServer)
    : "server not listed";

  return (
    <div className="nav-stack">
      <div className="server-nav-toolbar">
        <div className="server-nav-title">Servers</div>
        <ChromeIconButton
          commandId="resource.action.server.add"
          icon={Plus}
          label="Add server"
          onClick={() =>
            onCommand(
              {
                commandId: "resource.action.server.add",
                payload: {
                  type: "action",
                  label: "Add server",
                  entityId: "servers",
                },
              },
              { "resource.action.server.add": onOpenAddServer },
            )
          }
        />
      </div>
      <div className="resource-list resource-list-region">
        {error ? (
          <InlineNotice tone="error" title="Refresh failed" detail={error} />
        ) : null}
        {loading ? (
          <InlineNotice tone="info" title="Refreshing" detail="resources" />
        ) : null}
        {servers.length === 0 ? (
          <InlineNotice
            tone="info"
            title="Servers"
            detail="no linked servers"
          />
        ) : null}
        {servers.map((server) => (
          <ServerRows
            key={server.id}
            server={server}
            selected={server.id === selectedServerId}
            selectedId={selectedId}
            resources={server.id === selectedServerId ? resources : null}
            openWorkRootKeys={openWorkRootKeys}
            onCommand={onCommand}
            onOpenWorkRoot={onOpenWorkRoot}
            onOpenServerAuth={onOpenServerAuth}
            onReconnectServer={onReconnectServer}
            onSelectServer={onSelectServer}
          />
        ))}
        {!loading && !resources && selectedServer ? (
          <div className="server-empty-state">
            {selectedServer.status === "connected"
              ? "No workspace tree loaded"
              : serverStatusDetail}
          </div>
        ) : null}
        {!loading && resources && resources.workspaces.length === 0 ? (
          <div className="server-empty-state">No workspaces</div>
        ) : null}
      </div>
      <WorkRootFileExplorer
        workRoot={selectedWorkRoot}
        onCommand={onCommand}
        onOpenFile={onOpenFile}
      />
    </div>
  );
}

function ServerRows({
  server,
  selected,
  selectedId,
  resources,
  openWorkRootKeys,
  onCommand,
  onOpenWorkRoot,
  onOpenServerAuth,
  onReconnectServer,
  onSelectServer,
}: {
  server: ServerConnectionView;
  selected: boolean;
  selectedId: string | null;
  resources: DashboardResourcesView | null;
  openWorkRootKeys: ReadonlySet<string>;
  onCommand: DashboardCommandDispatcher;
  onOpenWorkRoot: (
    view: DashboardResourcesView,
    requestedWorkRootId?: string,
  ) => void;
  onOpenServerAuth: (server: ServerConnectionView) => void;
  onReconnectServer: (server: ServerConnectionView) => void;
  onSelectServer: (server: ServerConnectionView) => void;
}) {
  const actions = server.actions.length > 0 ? server.actions : [];
  return (
    <div className="server-group">
      <div
        className={`server-row ws-row${selected ? " server-row-selected ws-row-selected" : ""}`}
        data-server-kind={server.kind}
        data-server-status={server.status}
        title={[
          server.label,
          server.kind,
          server.status,
          server.state.status,
        ].join(" · ")}
      >
        <button
          aria-label={`Select server ${server.label}`}
          className="server-row-select"
          data-command-id="server.select"
          type="button"
          onClick={() => onSelectServer(server)}
        >
          <span className="server-row-main">
            <Server aria-hidden="true" size={15} strokeWidth={1.8} />
            <span className="server-row-title">{server.label}</span>
          </span>
          <span
            className={`server-status-chip server-status-chip-${server.status}`}
          >
            {serverConnectionStatusLabel(server)}
          </span>
        </button>
        <span className="server-row-actions">
          {actions.map((action) => (
            <ServerActionButton
              key={action.id}
              action={action}
              server={server}
              onCommand={onCommand}
              onOpenServerAuth={onOpenServerAuth}
              onReconnectServer={onReconnectServer}
            />
          ))}
          {server.actions.some(
            (action) => action.id === "openRoot" && action.enabled,
          ) ? (
            <OpenWorkRootControl
              server={server}
              variant="icon"
              onOpened={onOpenWorkRoot}
              onCommand={onCommand}
            />
          ) : null}
        </span>
      </div>
      {selected && resources ? (
        <div className="server-workspaces">
          {resources.workspaces.map((workspace) => (
            <WorkspaceRows
              key={workspace.id}
              workspace={workspace}
              serverId={server.id}
              selectedId={selectedId}
              openWorkRootKeys={openWorkRootKeys}
              onCommand={onCommand}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ServerActionButton({
  action,
  server,
  onCommand,
  onOpenServerAuth,
  onReconnectServer,
}: {
  action: ActionHint;
  server: ServerConnectionView;
  onCommand: DashboardCommandDispatcher;
  onOpenServerAuth: (server: ServerConnectionView) => void;
  onReconnectServer: (server: ServerConnectionView) => void;
}) {
  if (action.id === "openRoot") {
    return null;
  }

  const commandId: DashboardCommand["commandId"] = `resource.action.server.${action.id}`;
  if (action.id === "refresh") {
    return (
      <ChromeIconButton
        className="server-row-action"
        commandId={commandId}
        disabled={!action.enabled}
        icon={RefreshCw}
        label={action.label}
        onClick={() =>
          onCommand(
            {
              commandId,
              payload: {
                type: "action",
                label: action.label,
                entityId: server.id,
              },
            },
            { [commandId]: () => onCommand(buildDashboardRefreshCommand()) },
          )
        }
      />
    );
  }
  if (action.id === "enterPassphrase") {
    return (
      <ChromeIconButton
        className="server-row-action"
        commandId={commandId}
        disabled={!action.enabled}
        icon={KeyRound}
        label={action.label}
        onClick={() =>
          onCommand(
            {
              commandId,
              payload: {
                type: "action",
                label: action.label,
                entityId: server.id,
              },
            },
            { [commandId]: () => onOpenServerAuth(server) },
          )
        }
      />
    );
  }
  if (action.id === "reconnectTunnel") {
    return (
      <ChromeIconButton
        className="server-row-action"
        commandId={commandId}
        disabled={!action.enabled}
        icon={PlugZap}
        label={action.label}
        onClick={() =>
          onCommand(
            {
              commandId,
              payload: {
                type: "action",
                label: action.label,
                entityId: server.id,
              },
            },
            { [commandId]: () => onReconnectServer(server) },
          )
        }
      />
    );
  }

  return null;
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

  const workRootStateKey = workRoot
    ? serverScopedIdentity(workRoot.resourcePath.serverId, workRoot.id)
    : null;
  const snapshot = workRootStateKey
    ? (snapshots[workRootStateKey ?? ""] ?? initialExplorerSnapshot())
    : null;

  const updateSnapshot = useCallback(
    (
      workRootKey: string,
      updater: (snapshot: WorkRootExplorerSnapshot) => WorkRootExplorerSnapshot,
    ) => {
      setSnapshots((current) => ({
        ...current,
        [workRootKey]: updater(
          current[workRootKey] ?? initialExplorerSnapshot(),
        ),
      }));
    },
    [],
  );

  const loadDirectory = useCallback(
    async (
      workRootId: string,
      path: string,
      serverRoute: string,
      workRootKey: string,
    ) => {
      updateSnapshot(workRootKey, (current) => ({
        ...current,
        directories: {
          ...current.directories,
          [path]: { ...idleDirectoryLoadState(), status: "loading" },
        },
      }));

      try {
        const listing = await fetchWorkRootFiles(workRootId, path, serverRoute);
        updateSnapshot(workRootKey, (current) => ({
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
        updateSnapshot(workRootKey, (current) => ({
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

    const initialPath = workRootExplorerInitialLoadPath(
      snapshots[workRootStateKey ?? ""],
    );
    if (initialPath !== null) {
      void loadDirectory(
        workRoot.id,
        initialPath,
        workRoot.resourcePath.serverId,
        currentWorkRootStateKey,
      );
    }
  }, [loadDirectory, snapshots, workRoot, workRootStateKey]);

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

  const currentWorkRootStateKey = workRootStateKey ?? "";

  const rows = flattenWorkRootFileTree({
    expandedPaths: snapshot?.expandedPaths ?? new Set([""]),
    directories: snapshot?.directories ?? {},
    selectedPath: snapshot?.selectedPath ?? null,
  });

  const selectEntry = (entry: WorkRootFileEntryView) => {
    onCommand(
      buildFileExplorerSelectEntryCommand(
        workRoot.id,
        entry.path,
        workRoot.resourcePath.serverId,
      ),
      {
        "fileExplorer.selectEntry": () => {
          updateSnapshot(currentWorkRootStateKey, (current) => ({
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
      buildFileExplorerToggleDirectoryCommand(
        workRoot.id,
        entry.path,
        workRoot.resourcePath.serverId,
      ),
      {
        "fileExplorer.toggleDirectory": () => {
          updateSnapshot(currentWorkRootStateKey, (current) => ({
            ...current,
            expandedPaths: toggleExpandedPath(
              current.expandedPaths,
              entry.path,
            ),
            selectedPath: entry.path,
          }));

          if (
            workRootExplorerShouldLoadOnExpand(snapshot, entry.path, isExpanded)
          ) {
            void loadDirectory(
              workRoot.id,
              entry.path,
              workRoot.resourcePath.serverId,
              currentWorkRootStateKey,
            );
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
      buildFileExplorerOpenFileCommand(
        workRoot.id,
        entry.path,
        gesture,
        workRoot.resourcePath.serverId,
      ),
      {
        "fileExplorer.openFile": () => {
          updateSnapshot(currentWorkRootStateKey, (current) => ({
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
      buildFileExplorerRefreshCommand(
        workRoot.id,
        workRoot.resourcePath.serverId,
      ),
      {
        "fileExplorer.refresh": () => {
          const paths = workRootExplorerRefreshPaths(
            snapshot?.expandedPaths ?? new Set([""]),
          );
          for (const path of paths) {
            void loadDirectory(
              workRoot.id,
              path,
              workRoot.resourcePath.serverId,
              currentWorkRootStateKey,
            );
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
  onOpenFile: (
    entry: WorkRootFileEntryView,
    gesture: ReadOnlyFileOpenGesture,
  ) => void;
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
  openWorkRootKeys,
  openWorkRootRefs,
  workbenchLayoutRestoreRef,
  terminalVisualRestoreRef,
  onWorkbenchGroupsByRootChange,
  onPaneOrderByRootChange,
  onOpenWorkRootKeysChange,
  onOpenWorkRootRefsChange,
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
  openWorkRootKeys: string[];
  openWorkRootRefs: Record<string, { rootId: string; serverRoute: string }>;
  workbenchLayoutRestoreRef: RefObject<WorkbenchLayoutRestoreSnapshot>;
  terminalVisualRestoreRef: RefObject<TerminalVisualRestoreSnapshot>;
  onWorkbenchGroupsByRootChange: Dispatch<
    SetStateAction<Record<string, ReadonlyArray<{ id: string; label: string }>>>
  >;
  onPaneOrderByRootChange: Dispatch<
    SetStateAction<Record<string, WorkbenchPaneOrder>>
  >;
  onOpenWorkRootKeysChange: Dispatch<SetStateAction<string[]>>;
  onOpenWorkRootRefsChange: Dispatch<
    SetStateAction<Record<string, { rootId: string; serverRoute: string }>>
  >;
  onReadOnlyFilePanesChange: Dispatch<
    SetStateAction<Record<string, ReadOnlyFilePane>>
  >;
  onReadOnlyFilePaneOrderByGroupChange: Dispatch<
    SetStateAction<WorkbenchPaneOrder>
  >;
  onDocumentSaved: (source: {
    serverRoute?: string;
    workRootId: string;
    path: string;
    content: string;
    contentHash: string;
    sizeBytes: number;
  }) => void;
}) {
  const [activePaneByRoot, setActivePaneByRoot] = useState<
    Record<string, Record<string, string>>
  >({});
  // Best-effort per-root dockview group split sizes, captured from
  // `DockviewWorkbenchLayout`'s `onLayoutSnapshot` and persisted alongside
  // groups/pane order/active pane. Not seeded eagerly like the other three -
  // `DockviewWorkbenchLayout` applies the restored size itself, once, from
  // `workbenchLayoutRestoreRef` directly (see the render below), so this
  // state only needs to track the live/updated values for the save effect.
  const [groupSizeByRoot, setGroupSizeByRoot] = useState<
    Record<string, Record<string, { width?: number; height?: number }>>
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
  // Agent-chat tabs from `260711-feat-ws-dashboard-agent-activity-chat-ui`
  // Phase 1 - a new multi-instance `"agentChat"` SurfaceKind, kept in its own
  // state parallel to `terminalPanes`/`terminalPaneOrderByGroup` (not reused
  // from the singleton `"agent"` pane or the read-only `"workRootActivity"`
  // tab). A pane starts "empty" (`session: null`) the instant it is created
  // and only gains a `session` once a tile or history entry is picked and the
  // stub (or, later, real) `activity.session.create`/`start` call resolves.
  const [agentChatPanes, setAgentChatPanes] = useState<
    Record<string, AgentChatPaneState>
  >({});
  const [agentChatPaneOrderByGroup, setAgentChatPaneOrderByGroup] =
    useState<WorkbenchPaneOrder>({});
  const [focusedAgentChatPaneId, setFocusedAgentChatPaneId] = useState<
    string | null
  >(null);
  const [activeAgentChatPaneRequest, setActiveAgentChatPaneRequest] =
    useState<{ paneId: string; sequence: number } | null>(null);
  const agentChatOpenSequence = useRef(0);
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
  const focusedAgentChatRequest = useRef<number | null>(null);
  const terminalOpenSequence = useRef(0);
  const restoredTerminalIntentRoots = useRef<Set<string>>(new Set());
  // Tracks which roots' `listTerminals` call has resolved (success or
  // failure) at least once this session. Terminal listing is async
  // (Phase 4), while a restored `paneOrderByRoot[rootKey]` can be seeded
  // synchronously the moment a root is opened - so the prune/reconcile
  // revalidation effect must not treat "not yet loaded" the same as
  // "genuinely gone" for terminal pane references, or it permanently strips
  // a restored terminal layout before terminals ever get a chance to load.
  // Read-only file panes need no equivalent gate: they are seeded
  // synchronously at mount (App.tsx:379-381) and are always immediately
  // revalidatable.
  const [terminalsReadyRootKeys, setTerminalsReadyRootKeys] = useState<
    ReadonlySet<string>
  >(new Set());
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
    serverRoute: string | null;
    activity: WorkRootActivityBadgeInput;
  }>({ rootId: null, serverRoute: null, activity: { phase: "loading" } });
  const workRootActivityStateRef = useRef(workRootActivityState);
  workRootActivityStateRef.current = workRootActivityState;
  const activityStreamRequestSeq = useRef(0);
  const currentActivityStreamRequest = useRef<ActivityConsoleStreamRequest>({
    serverRoute: "server-local",
    workRootId: "",
    requestId: 0,
  });
  const activitySnapshotRequestSeq = useRef(0);
  const [activityPollFallbackRootKey, setActivityPollFallbackRootKey] =
    useState<string | null>(null);
  const [activityTranscriptRefresh, setActivityTranscriptRefresh] = useState<{
    rootId: string;
    serverRoute?: string | null;
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
  const selectedWorkRootServerId =
    selection?.root.resourcePath.serverId ?? null;
  const selectedWorkRootStateKey = selection
    ? serverScopedIdentity(
        selection.root.resourcePath.serverId,
        selection.root.id,
      )
    : null;
  const workbenchGroups = selectedWorkRootId
    ? (workbenchGroupsByRoot[selectedWorkRootStateKey ?? selectedWorkRootId] ??
      initialWorkbenchGroups)
    : initialWorkbenchGroups;
  const paneOrderByGroup = selectedWorkRootId
    ? (paneOrderByRoot[selectedWorkRootStateKey ?? selectedWorkRootId] ?? {})
    : {};
  const activePaneByGroup = selectedWorkRootId
    ? (activePaneByRoot[selectedWorkRootStateKey ?? selectedWorkRootId] ?? {})
    : {};
  // `openWorkRootKeys`/`openWorkRootRefs` are lifted to `App()` (so the left
  // panel can read membership and trigger a close), but the state that a
  // close must clear - `terminalPanes`, `activityPaneOpenByRoot`,
  // `activePaneByRoot`, `closedAgentPaneByRoot` - stays local to
  // `WorkbenchShell`. Detect a rootKey dropping out of `openWorkRootKeys`
  // (vs. the last-seen snapshot) and clear that root's local state; the
  // rootKey's disappearance from `openWorkRootKeys` is what unmounts the
  // root's `DockviewWorkbenchLayout` instance and fires the dispose/socket-
  // close cleanup already wired into `TerminalPaneBody`.
  // `openWorkRootRefs` is cleared for a rootKey in the same command handler
  // (and therefore the same React commit/render) that removes it from
  // `openWorkRootKeys` - so by the time this effect observes the removal,
  // the incoming `openWorkRootRefs` prop no longer has the entry either.
  // Snapshot both together, and only update the snapshot from inside the
  // effect (after use), so the *previous* render's ref is still available
  // to resolve `{rootId, serverRoute}` for the just-closed key.
  const lastOpenWorkRootKeysRef = useRef<string[]>(openWorkRootKeys);
  const lastOpenWorkRootRefsRef = useRef(openWorkRootRefs);
  useEffect(() => {
    const previousKeys = lastOpenWorkRootKeysRef.current;
    const previousRefs = lastOpenWorkRootRefsRef.current;
    lastOpenWorkRootKeysRef.current = openWorkRootKeys;
    lastOpenWorkRootRefsRef.current = openWorkRootRefs;
    // Seed `activePaneByRoot` for a rootKey the first time it appears in
    // `openWorkRootKeys` this session, mirroring the App()-level
    // `workbenchGroupsByRoot`/`paneOrderByRoot` seed. `activePaneByRoot` is
    // local to this component (unlike those two), so it must be seeded here
    // rather than alongside them. Only seeds when absent, so an in-session
    // live layout (e.g. a freshly created group with no restore entry yet)
    // is never clobbered.
    const addedKeys = openWorkRootKeys.filter(
      (rootKey) => !previousKeys.includes(rootKey),
    );
    for (const rootKey of addedKeys) {
      const restoredEntry = workbenchLayoutRestoreRef.current[rootKey];
      if (!restoredEntry) {
        continue;
      }
      setActivePaneByRoot((current) =>
        current[rootKey]
          ? current
          : { ...current, [rootKey]: restoredEntry.activePaneByGroup },
      );
    }
    const closedRefs = resolveClosedWorkRootRefs(
      previousKeys,
      previousRefs,
      openWorkRootKeys,
    );
    if (closedRefs.length === 0) {
      return;
    }
    for (const { rootKey, rootId, serverRoute } of closedRefs) {
      setTerminalPanes((current) =>
        removeTerminalPanesForWorkRoot(current, rootId, serverRoute),
      );
      setAgentChatPanes((current) =>
        removeAgentChatPanesForWorkRoot(current, rootId, serverRoute),
      );
      setActivityPaneOpenByRoot((current) => {
        if (!(rootKey in current)) {
          return current;
        }
        const next = { ...current };
        delete next[rootKey];
        return next;
      });
      setActivePaneByRoot((current) => {
        if (!(rootKey in current)) {
          return current;
        }
        const next = { ...current };
        delete next[rootKey];
        return next;
      });
      setGroupSizeByRoot((current) => {
        if (!(rootKey in current)) {
          return current;
        }
        const next = { ...current };
        delete next[rootKey];
        return next;
      });
      // Clear the closed root's terminals-ready flag alongside its other
      // per-root state: it is append-only otherwise, so a same-session
      // reopen of this root would find the stale flag still set and
      // immediately prune the freshly re-seeded restored terminal refs
      // before the re-triggered `listTerminals` call resolves, instead of
      // re-entering the not-yet-loaded grace window.
      setTerminalsReadyRootKeys((current) => {
        if (!current.has(rootKey)) {
          return current;
        }
        const next = new Set(current);
        next.delete(rootKey);
        return next;
      });
      setClosedAgentPaneByRoot((current) => {
        if (!(rootId in current)) {
          return current;
        }
        const next = { ...current };
        delete next[rootId];
        return next;
      });
    }
  }, [openWorkRootKeys, openWorkRootRefs]);

  // Persist each open root's dockview layout (groups, pane order, active
  // pane) whenever any of the three change, mirroring the App()-level
  // read-only-file-pane save effect's style: a plain effect, no bespoke
  // debounce, since that existing precedent doesn't use one either.
  //
  // CONTRACT: this save must never drop a persisted root just because it
  // wasn't (re)visited this session. `openWorkRootKeys` only ever grows with
  // roots actually selected this session (App.tsx's selection-seed effect),
  // so on first render it is `[]` and would otherwise `removeItem` the whole
  // snapshot before the user opens anything. Saved entries are therefore the
  // union of (a) live entries for `openWorkRootKeys` (this session's current
  // state) and (b) untouched entries from `workbenchLayoutRestoreRef`'s
  // current value for every rootKey NOT in `openWorkRootKeys` this session -
  // mirroring how `readOnlyFilePanes` avoids the same bug by being seeded at
  // mount from *all* persisted panes (App.tsx:379-381) before any save
  // happens. The merged result is written back into
  // `workbenchLayoutRestoreRef` every run (not just read from it), so a
  // just-closed root's untouched-entry fallback on the *next* run reflects
  // its last live state instead of the frozen mount-time snapshot - fixing
  // the clobber bug where a close previously caused this effect to write the
  // mount-time entry back over the closed root's live layout.
  useEffect(() => {
    const { mergedEntries, mergedSnapshot } = mergeWorkbenchLayoutRestoreEntries(
      openWorkRootKeys,
      openWorkRootRefs,
      workbenchGroupsByRoot,
      paneOrderByRoot,
      activePaneByRoot,
      groupSizeByRoot,
      workbenchLayoutRestoreRef.current,
    );
    saveWorkbenchLayoutRestoreSnapshot(mergedEntries);
    workbenchLayoutRestoreRef.current = mergedSnapshot;
  }, [
    openWorkRootKeys,
    openWorkRootRefs,
    workbenchGroupsByRoot,
    paneOrderByRoot,
    activePaneByRoot,
    groupSizeByRoot,
  ]);

  // Revalidate restored/live pane references against currently-known live
  // pane ids whenever the live terminal or read-only-file pane sets change.
  // Restore never treats persisted layout as authoritative over live
  // daemon/resource state: unavailable references are dropped from the pane
  // order, and `reconcileActiveWorkbenchPanes` (already falls back to
  // `group.panes[0]?.id`) repairs any active-pane entry left pointing at a
  // now-pruned pane id.
  //
  // The actual prune+reconcile transformation is delegated to
  // `revalidateWorkbenchLayoutForRoot` (a pure function extracted for unit
  // testing), gated per-root on `terminalsReadyRootKeys` so a restored
  // terminal-pane order is never destructively pruned before that root's
  // `listTerminals` call has resolved at least once.
  useEffect(() => {
    for (const rootKey of openWorkRootKeys) {
      const ref = openWorkRootRefs[rootKey];
      const orderForRoot = paneOrderByRoot[rootKey];
      if (!ref || !orderForRoot) {
        continue;
      }
      const liveReadOnlyPaneIds = new Set(
        readOnlyFilePanes
          .filter(
            (pane) =>
              pane.workRootId === ref.rootId &&
              pane.serverRoute === ref.serverRoute,
          )
          .map((pane) => pane.id),
      );
      const liveTerminalPaneIds = new Set(
        Object.values(terminalPanes)
          .filter(
            (pane) =>
              pane.session.workRootId === ref.rootId &&
              (pane.session.serverRoute ?? "server-local") === ref.serverRoute,
          )
          .map((pane) => pane.paneId),
      );
      // CONTRACT: agentChat panes must be included in this revalidation's
      // live-pane-id set for the same reason terminal/readonly panes are -
      // omitting them here (as originally written, before agentChat panes
      // existed) makes this effect treat every agentChat pane id as "not
      // live", so `revalidateWorkbenchLayoutForRoot` immediately reconciles
      // a just-set agentChat active-pane entry back to whatever readonly/
      // terminal pane was previously active in its shared group. This is the
      // same class of bug this effect's own comment/CONTRACT above already
      // documents for 260707-bug-dashboard-e2e-multi-root-locator-leakage
      // Phase 2 (readonly/terminal pane order being invisible to this
      // revalidation) - confirmed here by live instrumentation for
      // 260711-feat-ws-dashboard-agent-activity-chat-ui Phase 1: a freshly
      // created agent chat tab's `setActivePaneByGroupForSelected` write was
      // correctly applied, then silently reverted by this effect re-running
      // (it depends on `activePaneByRoot`) moments later.
      const liveAgentChatPaneIds = new Set(
        Object.values(agentChatPanes)
          .filter(
            (pane) =>
              pane.workRootId === ref.rootId &&
              (pane.serverRoute ?? "server-local") === ref.serverRoute,
          )
          .map((pane) => pane.paneId),
      );
      const livePaneIds = new Set<string>([
        ...liveTerminalPaneIds,
        ...liveReadOnlyPaneIds,
        ...liveAgentChatPaneIds,
      ]);
      const groupsForRoot =
        workbenchGroupsByRoot[rootKey] ?? initialWorkbenchGroups;
      // `paneOrderByRoot` only tracks agent/activity pane order for this
      // root; a dockview group can also host readonly-file panes and
      // terminal panes, whose order lives separately in (flat, cross-root)
      // `readOnlyFilePaneOrderByGroup` / `terminalPaneOrderByGroup`. Without
      // merging those in here, a group whose live panes are only
      // readonly-file/terminal panes looks pane-less to this revalidation,
      // and `reconcileActiveWorkbenchPanes` below drops its active-pane
      // entry entirely on every readOnlyFilePanes/terminalPanes change (e.g.
      // every file open/preview, or every terminal output poll) — this is
      // the actual mechanism behind
      // 260707-bug-dashboard-e2e-multi-root-locator-leakage Phase 2, confirmed
      // by live instrumentation: a just-computed pane activation (preview-tab
      // open, or a terminal-tab click) was computed correctly, then wiped by
      // this effect moments later because its own surface kind's pane order
      // was invisible to this revalidation's group.panes reconstruction. The
      // merge itself is delegated to `mergeReadOnlyAndTerminalPaneOrder` (a
      // pure function extracted for unit testing, mirroring
      // `revalidateWorkbenchLayoutForRoot` below).
      const mergedOrderBeforeAgentChat: WorkbenchPaneOrder =
        mergeReadOnlyAndTerminalPaneOrder(
          groupsForRoot,
          orderForRoot,
          readOnlyFilePaneOrderByGroup,
          liveReadOnlyPaneIds,
          terminalPaneOrderByGroup,
          liveTerminalPaneIds,
        );
      // CONTRACT: agentChat pane order lives in its own flat
      // `agentChatPaneOrderByGroup` map, just like readonly/terminal panes -
      // `mergeReadOnlyAndTerminalPaneOrder` predates the agentChat surface
      // kind and only merges those two, so without this extra merge step a
      // group whose live panes include an agentChat pane looks like it's
      // missing that pane to `revalidateWorkbenchLayoutForRoot` below, which
      // then reconciles its active-pane entry away from it (same class of
      // bug as the readonly/terminal merge above; see the `liveAgentChatPaneIds`
      // CONTRACT note near its declaration).
      const mergedOrderForRoot: WorkbenchPaneOrder = Object.fromEntries(
        groupsForRoot.map((group) => [
          group.id,
          [
            ...(mergedOrderBeforeAgentChat[group.id] ?? []),
            ...(agentChatPaneOrderByGroup[group.id] ?? []).filter((paneId) =>
              liveAgentChatPaneIds.has(paneId),
            ),
          ],
        ]),
      );
      // CONTRACT: `prunedOrder` here only depends on group/order/live-id
      // inputs already closed over from this render, so it is safe to read
      // from this render's closure — every code path that makes a pane
      // active also appends it to its order map in the same synchronous
      // batch (e.g. `openReadOnlyFile` appends to
      // `readOnlyFilePaneOrderByGroup` before requesting focus; terminal
      // creation adds to `terminalPaneOrderByGroup` before focusing), so the
      // render whose closure sees a fresh active-pane id also sees that id
      // already present in `mergedOrderForRoot`. `reconciledActivePane`,
      // however, is derived from `activePaneByGroup` (this root's
      // active-pane map) directly, with no such same-batch-order guarantee —
      // a user action (e.g. a terminal-tab click) can write a fresher
      // `activePaneByRoot` update in the same window this effect's render-time
      // closure predates. Computing `reconciledActivePane` from that
      // render-time closure here and unconditionally writing it back below
      // would silently clobber the fresher update with a stale one —
      // confirmed by live instrumentation (see
      // 260707-bug-dashboard-e2e-multi-root-locator-leakage Phase 2: a
      // single `[diag-revalidate-change]` write reverted a just-applied
      // terminal-tab selection because `preferred` — read fresh inside the
      // `setActivePaneByRoot` updater below — already reflected the click,
      // while the outer `reconciledActivePane` did not). Fixed by deferring
      // the reconcile computation itself into the updater, so the
      // active-pane read (`preferred`) is always truly current at the moment
      // it's compared/written, instead of racing a stale render-time closure
      // of the same state.
      const { prunedOrder } = revalidateWorkbenchLayoutForRoot(
        groupsForRoot,
        mergedOrderForRoot,
        activePaneByRoot[rootKey] ?? {},
        livePaneIds,
        terminalsReadyRootKeys.has(rootKey),
      );
      // Strip the readonly-file/terminal/agentChat pane ids back out before
      // persisting into `paneOrderByRoot`, which must stay agnostic to those
      // (their order is separately owned by `readOnlyFilePaneOrderByGroup` /
      // `terminalPaneOrderByGroup` / `agentChatPaneOrderByGroup`).
      const prunedOrderWithoutMerged = removePanesFromOrder(prunedOrder, [
        ...liveReadOnlyPaneIds,
        ...liveTerminalPaneIds,
        ...liveAgentChatPaneIds,
      ]);
      if (
        JSON.stringify(prunedOrderWithoutMerged) !== JSON.stringify(orderForRoot)
      ) {
        onPaneOrderByRootChange((current) => ({
          ...current,
          [rootKey]: prunedOrderWithoutMerged,
        }));
      }
      setActivePaneByRoot((current) => {
        const preferred = current[rootKey] ?? {};
        const { reconciledActivePane } = revalidateWorkbenchLayoutForRoot(
          groupsForRoot,
          mergedOrderForRoot,
          preferred,
          livePaneIds,
          terminalsReadyRootKeys.has(rootKey),
        );
        if (JSON.stringify(preferred) === JSON.stringify(reconciledActivePane)) {
          return current;
        }
        return { ...current, [rootKey]: reconciledActivePane };
      });
    }
  }, [
    terminalPanes,
    readOnlyFilePanes,
    readOnlyFilePaneOrderByGroup,
    terminalPaneOrderByGroup,
    agentChatPanes,
    agentChatPaneOrderByGroup,
    openWorkRootKeys,
    openWorkRootRefs,
    paneOrderByRoot,
    activePaneByRoot,
    workbenchGroupsByRoot,
    terminalsReadyRootKeys,
    onPaneOrderByRootChange,
  ]);
  const activityPaneOpenForSelected = selectedWorkRootId
    ? (activityPaneOpenByRoot[selectedWorkRootStateKey ?? selectedWorkRootId] ??
      false)
    : false;
  const readOnlyFilePanesRef = useRef(readOnlyFilePanes);
  readOnlyFilePanesRef.current = readOnlyFilePanes;
  const documentRefreshSequence = useRef<Record<string, number>>({});

  const refreshOpenDocument = useCallback(
    (
      workRootId: string,
      path: string,
      expectedContentHash?: string,
      serverRoute: string | null | undefined = selectedWorkRootServerId,
    ) => {
      const sourceKey = readOnlyFilePaneSourceKey(workRootId, path, serverRoute);
      const requestSequence =
        (documentRefreshSequence.current[sourceKey] ?? 0) + 1;
      documentRefreshSequence.current[sourceKey] = requestSequence;
      if (
        !readOnlyFilePanesRef.current.some(
          (pane) =>
            pane.serverRoute === (serverRoute ?? "server-local") &&
            pane.workRootId === workRootId &&
            pane.path === path &&
            (!expectedContentHash || pane.contentHash !== expectedContentHash),
        )
      ) {
        return;
      }
      void fetchWorkRootTextFile(workRootId, path, serverRoute)
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
          const message =
            error instanceof Error ? error.message : "file read failed";
          onReadOnlyFilePanesChange((current) =>
            applyReadOnlyFilePaneSourceError(
              current,
              workRootId,
              path,
              message,
              serverRoute,
            ),
          );
        });
    },
    [onReadOnlyFilePanesChange, selectedWorkRootServerId],
  );

  const refreshVisibleDocuments = useCallback(() => {
    const rootId = selectedWorkRootId;
    if (!rootId) {
      return;
    }
    const paths = [
      ...new Set(
        readOnlyFilePanesRef.current
          .filter(
            (pane) =>
              pane.serverRoute === (selectedWorkRootServerId ?? "server-local") &&
              pane.workRootId === rootId &&
              pane.status === "loaded",
          )
          .map((pane) => pane.path),
      ),
    ];
    for (const path of paths) {
      refreshOpenDocument(rootId, path, undefined, selectedWorkRootServerId);
    }
  }, [refreshOpenDocument, selectedWorkRootId, selectedWorkRootServerId]);

  const setActivePaneByGroupForSelected = (
    next:
      | Record<string, string>
      | ((current: Record<string, string>) => Record<string, string>),
  ) => {
    if (!selectedWorkRootId) {
      return;
    }
    setActivePaneByRoot((currentByRoot) => {
      const rootKey = selectedWorkRootStateKey ?? selectedWorkRootId;
      const current = currentByRoot[rootKey] ?? {};
      return {
        ...currentByRoot,
        [rootKey]: typeof next === "function" ? next(current) : next,
      };
    });
  };

  // Compute the dockview editor-group model for a single open work root.
  // Called once per open root per render (a plain function call, not a
  // hook — `buildWorkbenchEditorGroups` is pure) so every open root's
  // dockview instance gets its own pane set. Only the selected root gets
  // live selection/activity data; inactive roots reuse the same defaults
  // `buildWorkbenchEditorGroups` already falls back to.
  const buildEditorGroupsForRoot = (
    root: WorkRootView,
    mainInstance: InstanceView | null,
    rootKey: string,
  ): WorkbenchEditorGroupModel[] => {
    const isSelectedRoot = rootKey === selectedWorkRootStateKey;
    const groupsForRoot =
      workbenchGroupsByRoot[rootKey] ?? initialWorkbenchGroups;
    const paneOrderForRoot = paneOrderByRoot[rootKey] ?? {};
    const selectedInstanceForRoot = isSelectedRoot
      ? (selection?.selectedInstance ?? mainInstance)
      : mainInstance;
    const supportEntityForRoot = isSelectedRoot
      ? (selectedEntity ?? resourceEntityForWorkRoot(root))
      : resourceEntityForWorkRoot(root);
    return applyWorkbenchPaneOrder(
      buildWorkbenchEditorGroups(
        root,
        groupsForRoot,
        mainInstance,
        selectedInstanceForRoot,
        supportEntityForRoot,
        readOnlyFilePanes,
        readOnlyFilePaneOrderByGroup,
        paneOrderForRoot,
        Object.values(terminalPanes),
        terminalPaneOrderByGroup,
        {
          onSendData: sendTerminalData,
          onClose: closeTerminalPane,
          onResize: forwardTerminalResize,
          onSocketStatus: updateTerminalSocketStatus,
          onVisibilityGated: updateTerminalPaneVisibilityGated,
          onSocketMessage: applyTerminalSocketMessage,
          onSocketResize: acceptTerminalSocketResize,
          onFocusInput: (pane) => setFocusedTerminalPaneId(pane.paneId),
          isActivePane: (pane) =>
            focusedTerminalPaneIdRef.current === pane.paneId,
          onVisualRestoreEntryFor: (pane) =>
            terminalVisualRestoreRef.current[pane.logicalKey],
          onVisualCapture: (pane, capture) => {
            const entry = {
              logicalKey: pane.logicalKey,
              serialized: capture.serialized,
              viewportY: capture.viewportY,
              nextSequence: capture.nextSequence,
              capturedAtMs: Date.now(),
            };
            upsertTerminalVisualRestoreEntry(entry);
            terminalVisualRestoreRef.current =
              upsertTerminalVisualRestoreEntryInSnapshot(
                terminalVisualRestoreRef.current,
                entry,
              );
          },
        },
        Object.values(agentChatPanes),
        agentChatPaneOrderByGroup,
        {
          onClose: closeAgentChatPane,
          onStartHarness: startAgentChatHarness,
          onResumeHistoryItem: resumeAgentChatHistoryItem,
          onLoadHistory: (workRootId, serverRoute) =>
            stubActivityHistoryList({ workRootId, serverRoute }),
          isActivePane: (pane) => focusedAgentChatPaneId === pane.paneId,
        },
        closedAgentPaneByRoot[root.id] ?? [],
        isSelectedRoot ? activityPaneOpenByRoot[rootKey] ?? false : false,
        isSelectedRoot &&
          workRootActivityState.rootId === root.id &&
          workRootActivityState.serverRoute === root.resourcePath.serverId
          ? workRootActivityState.activity
          : { phase: "loading" },
        isSelectedRoot &&
          activityTranscriptRefresh?.rootId === root.id &&
          activityTranscriptRefresh.serverRoute === root.resourcePath.serverId
          ? activityTranscriptRefresh
          : null,
        onCommand,
        onDocumentSaved,
      ),
      paneOrderForRoot,
    );
  };

  const workbenchModel =
    resources && selection
      ? (() => {
          const { workspace, root, mainInstance, selectedInstance } =
            selection;
          const rootKey = serverScopedIdentity(
            root.resourcePath.serverId,
            root.id,
          );
          const editorGroups = buildEditorGroupsForRoot(
            root,
            mainInstance,
            rootKey,
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
  // Every open work root's own resolved root/mainInstance + editor groups,
  // used to mount one persistent `DockviewWorkbenchLayout` per root below.
  // Roots that no longer resolve against `resources` (e.g. transient
  // resource-fetch gaps) are silently skipped for that render.
  const openWorkRootInstances = openWorkRootKeys
    .map((rootKey) => {
      const ref = openWorkRootRefs[rootKey];
      if (!ref) {
        return null;
      }
      const resolved = findOpenWorkRoot(resources, ref);
      if (!resolved) {
        return null;
      }
      return {
        rootKey,
        root: resolved.root,
        mainInstance: resolved.mainInstance,
        editorGroups: buildEditorGroupsForRoot(
          resolved.root,
          resolved.mainInstance,
          rootKey,
        ),
      };
    })
    .filter(
      (entry): entry is NonNullable<typeof entry> => entry !== null,
    );

  useEffect(() => {
    if (!workbenchModel) {
      return;
    }
    const rootId = workbenchModel.root.id;
    const serverRoute = workbenchModel.root.resourcePath.serverId;
    const rootKey = serverScopedIdentity(serverRoute, rootId);
    const listStartedAtMs = Date.now();
    void listTerminals(rootId, serverRoute)
      .then((sessions) => {
        const restoreIntents = terminalRestoreIntentsForWorkRoot(
          loadTerminalRestoreIntents(),
          rootId,
          serverRoute,
        );
        if (
          sessions.length === 0 &&
          restoreIntents.length > 0 &&
          !restoredTerminalIntentRoots.current.has(rootKey)
        ) {
          restoredTerminalIntentRoots.current.add(rootKey);
          for (const intent of restoreIntents) {
            onCommand(buildTerminalCreateCommand(rootId, serverRoute), {
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
              serverRoute,
              terminalVisualRestoreRef.current,
            ),
            serverRoute,
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
      .catch(() => undefined)
      .finally(() => {
        setTerminalsReadyRootKeys((current) =>
          current.has(rootKey) ? current : new Set(current).add(rootKey),
        );
      });
  }, [workbenchModel?.root.id, workbenchModel?.root.resourcePath.serverId]);

  // Fetch named-agent activity for the selected workRoot through the Phase 1
  // protected route. Loading/error are bounded badge states; a failure never
  // breaks the workbench.
  useEffect(() => {
    const rootId = workbenchModel?.root.id;
    const serverRoute = workbenchModel?.root.resourcePath.serverId;
    if (!rootId || !serverRoute) {
      return;
    }
    let cancelled = false;
    setWorkRootActivityState({
      rootId,
      serverRoute,
      activity: { phase: "loading" },
    });
    const timer = window.setTimeout(() => {
      void fetchWorkRootActivity(rootId, { serverRoute })
        .then((view) => {
          if (!cancelled) {
            setWorkRootActivityState({
              rootId,
              serverRoute,
              activity: { phase: "ready", view },
            });
          }
        })
        .catch(() => {
          if (!cancelled) {
            setWorkRootActivityState({
              rootId,
              serverRoute,
              activity: { phase: "error" },
            });
          }
        });
    }, 300);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [workbenchModel?.root.id, workbenchModel?.root.resourcePath.serverId]);

  // Activity Console live stream: while the pane is open, subscribe to the
  // daemon-owned source-neutral event stream for the selected workRoot. The
  // older recent-list polling remains a bounded fallback only when the stream
  // cannot be established or the daemon explicitly switches to pollFallback.
  useEffect(() => {
    const rootId = workbenchModel?.root.id;
    const serverRoute = workbenchModel?.root.resourcePath.serverId;
    const rootKey =
      rootId && serverRoute ? serverScopedIdentity(serverRoute, rootId) : null;
    if (!rootId || !serverRoute || !rootKey || !activityPaneOpenForSelected) {
      currentActivityStreamRequest.current = {
        serverRoute: serverRoute ?? "server-local",
        workRootId: rootId ?? "",
        requestId: activityStreamRequestSeq.current + 1,
      };
      activityStreamRequestSeq.current =
        currentActivityStreamRequest.current.requestId;
      setActivityPollFallbackRootKey((current) =>
        current === rootKey ? null : current,
      );
      return;
    }

    const requestId = activityStreamRequestSeq.current + 1;
    activityStreamRequestSeq.current = requestId;
    const expected = { serverRoute, workRootId: rootId, requestId };
    currentActivityStreamRequest.current = expected;
    setActivityPollFallbackRootKey((current) =>
      current === rootKey ? null : current,
    );

    let cancelled = false;
    let streamOpened = false;
    let fallbackTimer: number | null = null;
    const after =
      workRootActivityStateRef.current.rootId === rootId &&
      workRootActivityStateRef.current.serverRoute === serverRoute &&
      workRootActivityStateRef.current.activity.phase === "ready"
        ? workRootActivityStateRef.current.activity.view.feedCursor
        : null;
    const source = new EventSource(
      workRootActivityEventsEndpoint(rootId, { after, serverRoute }),
    );

    const requestSnapshot = () => {
      const snapshotRequestId = activitySnapshotRequestSeq.current + 1;
      activitySnapshotRequestSeq.current = snapshotRequestId;
      void fetchWorkRootActivity(rootId, { serverRoute })
        .then((view) => {
          if (
            cancelled ||
            snapshotRequestId !== activitySnapshotRequestSeq.current ||
            !shouldApplyActivityStreamRequest(
              expected,
              currentActivityStreamRequest.current,
            ) ||
            view.workRootId !== rootId
          ) {
            return;
          }
          setWorkRootActivityState({
            rootId,
            serverRoute,
            activity: { phase: "ready", view },
          });
        })
        .catch(() => {
          if (
            !cancelled &&
            snapshotRequestId === activitySnapshotRequestSeq.current &&
            shouldApplyActivityStreamRequest(
              expected,
              currentActivityStreamRequest.current,
            )
          ) {
            setActivityPollFallbackRootKey(rootKey);
          }
        });
    };

    const applyStreamEvent = (event: ActivityConsoleEvent) => {
      if (
        cancelled ||
        !shouldApplyActivityStreamRequest(
          expected,
          currentActivityStreamRequest.current,
        )
      ) {
        return;
      }
      if (event.type === "snapshotInvalidated") {
        requestSnapshot();
      }
      if (event.type === "transcriptUpdated") {
        setActivityTranscriptRefresh((current) => ({
          rootId,
          serverRoute,
          activityId: event.activityId,
          cursor: event.transcriptCursor,
          sequence: (current?.sequence ?? 0) + 1,
        }));
      } else if (
        event.type === "modeChanged" &&
        event.updateMode === "pollFallback"
      ) {
        setActivityPollFallbackRootKey(rootKey);
      } else if (
        event.type === "modeChanged" &&
        (event.updateMode === "watch" || event.updateMode === "snapshot")
      ) {
        if (fallbackTimer !== null) {
          window.clearTimeout(fallbackTimer);
          fallbackTimer = null;
        }
        setActivityPollFallbackRootKey((fallbackRootId) =>
          fallbackRootId === rootKey ? null : fallbackRootId,
        );
      }
      setWorkRootActivityState((current) => {
        if (
          current.rootId !== rootId ||
          current.serverRoute !== serverRoute ||
          current.activity.phase !== "ready"
        ) {
          return current;
        }
        const result = applyActivityConsoleEvent(current.activity.view, event);
        return {
          rootId,
          serverRoute,
          activity: { phase: "ready", view: result.view },
        };
      });
    };

    source.onopen = () => {
      streamOpened = true;
      if (
        shouldApplyActivityStreamRequest(
          expected,
          currentActivityStreamRequest.current,
        )
      ) {
        if (fallbackTimer !== null) {
          window.clearTimeout(fallbackTimer);
          fallbackTimer = null;
        }
        setActivityPollFallbackRootKey((current) =>
          current === rootKey ? null : current,
        );
      }
    };
    const handleActivityMessage = (message: MessageEvent) => {
      let payload: unknown;
      try {
        payload = JSON.parse(message.data);
      } catch {
        setActivityPollFallbackRootKey(rootKey);
        return;
      }
      const event = parseActivityConsoleEvent(payload);
      if (!event) {
        setActivityPollFallbackRootKey(rootKey);
        return;
      }
      applyStreamEvent(event);
    };
    source.addEventListener("activity", handleActivityMessage);
    source.onmessage = handleActivityMessage;
    source.onerror = () => {
      if (
        cancelled ||
        !shouldApplyActivityStreamRequest(
          expected,
          currentActivityStreamRequest.current,
        )
      ) {
        return;
      }
      if (!streamOpened) {
        setActivityPollFallbackRootKey(rootKey);
        requestSnapshot();
        return;
      }
      if (fallbackTimer !== null) {
        window.clearTimeout(fallbackTimer);
      }
      fallbackTimer = window.setTimeout(() => {
        if (
          !cancelled &&
          shouldApplyActivityStreamRequest(
            expected,
            currentActivityStreamRequest.current,
          )
        ) {
          setActivityPollFallbackRootKey(rootKey);
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
      setActivityPollFallbackRootKey((current) =>
        current === rootKey ? null : current,
      );
    };
  }, [
    workbenchModel?.root.id,
    workbenchModel?.root.resourcePath.serverId,
    activityPaneOpenForSelected,
  ]);

  useEffect(() => {
    const rootId = workbenchModel?.root.id;
    const serverRoute = workbenchModel?.root.resourcePath.serverId;
    if (
      !rootId ||
      !serverRoute ||
      !activityPaneOpenForSelected ||
      activityPollFallbackRootKey !== serverScopedIdentity(serverRoute, rootId)
    ) {
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
        serverRoute,
      })
        .then((view) => {
          if (
            cancelled ||
            snapshotRequestId !== activitySnapshotRequestSeq.current
          ) {
            return;
          }
          setWorkRootActivityState((current) => {
            if (
              current.rootId !== rootId ||
              current.serverRoute !== serverRoute ||
              view.workRootId !== rootId
            ) {
              return current;
            }
            if (current.activity.phase !== "ready") {
              return { rootId, serverRoute, activity: { phase: "ready", view } };
            }
            return {
              rootId,
              serverRoute,
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
  }, [
    workbenchModel?.root.id,
    activityPaneOpenForSelected,
    activityPollFallbackRootKey,
    workbenchModel?.root.resourcePath.serverId,
  ]);

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
    const source = new EventSource(
      workRootDocumentEventsEndpoint(rootId, selectedWorkRootServerId),
    );
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
      refreshOpenDocument(
        event.workRootId,
        event.path,
        event.contentHash,
        selectedWorkRootServerId,
      );
    };
    source.addEventListener("document", handleDocumentMessage);
    source.onmessage = handleDocumentMessage;
    return () => {
      cancelled = true;
      source.removeEventListener("document", handleDocumentMessage);
      source.close();
    };
  }, [refreshOpenDocument, selectedWorkRootId, selectedWorkRootServerId]);

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
    Array<{
      terminalId: string;
      logicalKey: string;
      nextSequence: number;
      serverRoute?: string | null;
    }>
  >([]);
  livePollPanesRef.current = workbenchModel
    ? Object.values(terminalPanes)
        .filter(
          (pane) =>
            pane.session.workRootId === workbenchModel.root.id &&
            (pane.session.serverRoute ?? "server-local") ===
              workbenchModel.root.resourcePath.serverId &&
            shouldPollTerminalOutput(pane),
        )
        .map((pane) => ({
          terminalId: pane.session.terminalId,
          serverRoute: pane.session.serverRoute,
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
        void fetchTerminalOutput(
          pane.terminalId,
          pane.nextSequence,
          pane.serverRoute,
        )
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
  }, [workbenchModel?.root.id, workbenchModel?.root.resourcePath.serverId]);

  // A single-clicked preview pane must be the active Dockview panel on the
  // very first `syncDockviewWorkbench` pass that creates it, not a later
  // pass: `editorGroups` reflects the just-added pane one render before an
  // *effect-driven* `activePaneByRoot` update could catch up, so
  // `DockviewWorkbenchLayout`'s own passive `syncPanels` effect would create
  // the panel `inactive: true` on that first pass; a later `setActive()`
  // call then races Dockview's own active-panel bookkeeping and can be
  // silently reverted (confirmed by live instrumentation — see
  // 260707-bug-dashboard-e2e-multi-root-locator-leakage Phase 2). Computing
  // the pending activation as a plain render-time value (rather than via an
  // effect) means the *first* render that includes the new pane already
  // reflects the correct active id, so `syncPanels`'s first pass creates the
  // panel active directly — no deferred `setActive()` call, no race.
  //
  // INVARIANT: this memo's "already handled" guard (comparing
  // `focusedReadOnlyRequest.current` against the request's `sequence`) is
  // only re-evaluated when a dependency changes. It relies on `editorGroups`
  // being a fresh array reference on every render — true today because
  // `editorGroups` (below, via `buildEditorGroupsForRoot`) is a plain inline
  // expression, not itself memoized. If `editorGroups` is ever wrapped in its
  // own `useMemo`, this memo could return a stale cached `{groupId, paneId}`
  // on renders where nothing else changed, and `effectiveActivePaneByGroup`
  // would re-force the preview pane active — clobbering a user's subsequent
  // same-group tab switch (e.g. clicking back to the pinned sibling pane).
  // Any future memoization of `editorGroups` must add an explicit dependency
  // here that changes exactly when a *new* activation request is handled
  // (not merely when `editorGroups`' content is unchanged).
  const pendingReadOnlyActivation = useMemo(() => {
    if (
      !activeReadOnlyFilePaneRequest ||
      focusedReadOnlyRequest.current === activeReadOnlyFilePaneRequest.sequence
    ) {
      return null;
    }
    const targetGroup = editorGroups.find((group) =>
      group.panes.some(
        (pane) => pane.id === activeReadOnlyFilePaneRequest.paneId,
      ),
    );
    if (!targetGroup) {
      return null;
    }
    return { groupId: targetGroup.id, paneId: activeReadOnlyFilePaneRequest.paneId };
  }, [activeReadOnlyFilePaneRequest, editorGroups]);

  // Persists the render-time-only decision above into real
  // `activePaneByRoot` state (so it survives beyond this one render, e.g.
  // once the user's next action re-derives `activePaneByGroup` from state
  // directly), and marks the request handled so this stops recomputing.
  useEffect(() => {
    if (!activeReadOnlyFilePaneRequest || !pendingReadOnlyActivation) {
      return;
    }
    focusedReadOnlyRequest.current = activeReadOnlyFilePaneRequest.sequence;
    setActivePaneByGroupForSelected((current) =>
      selectWorkbenchPane(
        current,
        pendingReadOnlyActivation.groupId,
        pendingReadOnlyActivation.paneId,
      ),
    );
  }, [activeReadOnlyFilePaneRequest, pendingReadOnlyActivation]);

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

  // A freshly created agent chat pane shares its target group ("opened" row
  // policy, group 2) with read-only file panes/WorkRoot Activity - the same
  // group persistentTerminal's dedicated group-1 pinned row never contends
  // for. An effect-only round-trip (render pane -> effect flips active state
  // -> next render syncs Dockview) loses the race against whichever pane was
  // already active in that shared group, because the same-render
  // `effectiveActivePaneByGroup` used below still reflects the old active
  // pane on the render that first adds the new panel to Dockview. Mirror
  // `pendingReadOnlyActivation`'s same-render `useMemo` instead, so a brand
  // new agent chat tab is the group's active pane on the very render Dockview
  // first sees it - not one render late.
  const pendingAgentChatActivation = useMemo(() => {
    if (
      !activeAgentChatPaneRequest ||
      focusedAgentChatRequest.current === activeAgentChatPaneRequest.sequence
    ) {
      return null;
    }
    const targetGroup = editorGroups.find((group) =>
      group.panes.some(
        (pane) => pane.id === activeAgentChatPaneRequest.paneId,
      ),
    );
    if (!targetGroup) {
      return null;
    }
    return {
      groupId: targetGroup.id,
      paneId: activeAgentChatPaneRequest.paneId,
    };
  }, [activeAgentChatPaneRequest, editorGroups]);

  useEffect(() => {
    if (!activeAgentChatPaneRequest || !pendingAgentChatActivation) {
      return;
    }
    focusedAgentChatRequest.current = activeAgentChatPaneRequest.sequence;
    setFocusedAgentChatPaneId(pendingAgentChatActivation.paneId);
    setActivePaneByGroupForSelected((current) =>
      selectWorkbenchPane(
        current,
        pendingAgentChatActivation.groupId,
        pendingAgentChatActivation.paneId,
      ),
    );
  }, [activeAgentChatPaneRequest, pendingAgentChatActivation]);

  function persistTerminalPanesForWorkRoot(
    workRootId: string,
    nextPanes: Record<string, TerminalPaneState>,
    serverRoute: string | null | undefined = "server-local",
  ): Record<string, TerminalPaneState> {
    const nextIntents = terminalRestoreIntentsFromPanes(
      Object.values(nextPanes).filter(
        (pane) =>
          pane.session.workRootId === workRootId &&
          (pane.session.serverRoute ?? "server-local") ===
            (serverRoute ?? "server-local"),
      ),
    );
    saveTerminalRestoreIntents(
      replaceTerminalRestoreIntentsForWorkRoot(
        loadTerminalRestoreIntents(),
        workRootId,
        nextIntents,
        serverRoute,
      ),
    );
    return nextPanes;
  }

  function createTerminalPane(options: TerminalCreateOptions = {}) {
    if (!workbenchModel) {
      return;
    }
    const rootId = workbenchModel.root.id;
    const serverRoute = workbenchModel.root.resourcePath.serverId;
    void createTerminal(rootId, options, serverRoute)
      .then((session) => {
        const pane = terminalPaneFromSession(session);
        setTerminalPanes((current) =>
          persistTerminalPanesForWorkRoot(
            rootId,
            {
              ...current,
              [pane.logicalKey]: pane,
            },
            serverRoute,
          ),
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

  // CONTRACT: mirrors `createTerminalPane`'s multi-instance registration
  // pattern, but the pane itself is created synchronously and empty - the
  // "open new agent tab" button never blocks on a harness/session picker.
  // A tile click or history-entry pick (see `startAgentChatHarness` /
  // `resumeAgentChatHistoryItem` below) later calls the stub (or, once
  // `260620` lands, real) `activity.session.create`/`start` and attaches the
  // resulting session to this same pane.
  function createAgentChatPane() {
    if (!workbenchModel) {
      return;
    }
    const rootId = workbenchModel.root.id;
    const serverRoute = workbenchModel.root.resourcePath.serverId;
    const pane = createEmptyAgentChatPane(rootId, serverRoute);
    setAgentChatPanes((current) => ({
      ...current,
      [pane.logicalKey]: pane,
    }));
    setAgentChatPaneOrderByGroup((current) =>
      placeAgentChatPane(
        current,
        agentChatPanes,
        pane,
        workbenchGroups,
        paneOrderByGroup,
      ),
    );
    setFocusedAgentChatPaneId(pane.paneId);
    setActiveAgentChatPaneRequest({
      paneId: pane.paneId,
      sequence: agentChatOpenSequence.current++,
    });
  }

  function startAgentChatHarness(
    pane: AgentChatPaneState,
    harness: AgentChatHarness,
  ) {
    setAgentChatPanes((current) =>
      current[pane.logicalKey]
        ? {
            ...current,
            [pane.logicalKey]: markAgentChatPaneStarting(
              current[pane.logicalKey],
            ),
          }
        : current,
    );
    // Tile-launch semantics (fixture-review follow-up): the click actually
    // invokes the create/start call path against whatever provider is wired
    // in - the stub today, a real per-harness adapter once `260620` lands -
    // it is never a UI-only state transition.
    void stubStartNewAgentChatSession(pane.workRootId, harness, pane.serverRoute)
      .then((session) => {
        applyAgentChatSession(pane.logicalKey, session);
      })
      .catch((error) => {
        setAgentChatPanes((current) =>
          current[pane.logicalKey]
            ? {
                ...current,
                [pane.logicalKey]: markAgentChatPaneError(
                  current[pane.logicalKey],
                  error instanceof Error
                    ? error.message
                    : "agent chat session failed to start",
                ),
              }
            : current,
        );
      });
  }

  function resumeAgentChatHistoryItem(
    pane: AgentChatPaneState,
    item: ActivityItem,
  ) {
    setAgentChatPanes((current) =>
      current[pane.logicalKey]
        ? {
            ...current,
            [pane.logicalKey]: markAgentChatPaneStarting(
              current[pane.logicalKey],
            ),
          }
        : current,
    );
    void stubResumeAgentChatSession(item, pane.workRootId, pane.serverRoute)
      .then((session) => {
        applyAgentChatSession(pane.logicalKey, session);
      })
      .catch((error) => {
        setAgentChatPanes((current) =>
          current[pane.logicalKey]
            ? {
                ...current,
                [pane.logicalKey]: markAgentChatPaneError(
                  current[pane.logicalKey],
                  error instanceof Error
                    ? error.message
                    : "agent chat session failed to resume",
                ),
              }
            : current,
        );
      });
  }

  function applyAgentChatSession(
    logicalKey: string,
    session: AgentChatSessionView,
  ) {
    setAgentChatPanes((current) =>
      current[logicalKey]
        ? {
            ...current,
            [logicalKey]: attachAgentChatSession(current[logicalKey], session),
          }
        : current,
    );
  }

  function closeAgentChatPane(pane: AgentChatPaneState) {
    // Phase 1 sessions are stub/local-only (see `activitySessionStub.ts`) -
    // there is no daemon-side resource to detach yet, unlike
    // `closeTerminalPane`'s `closeTerminal(...)` call. A future real-adapter
    // phase may need an equivalent close/detach call here.
    setAgentChatPanes((current) => removeAgentChatPane(current, pane.logicalKey));
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

  function updateTerminalPaneVisibilityGated(
    pane: TerminalPaneState,
    visibilityGated: boolean,
  ) {
    setTerminalPanes((current) =>
      current[pane.logicalKey]
        ? {
            ...current,
            [pane.logicalKey]: markTerminalPaneVisibilityGated(
              current[pane.logicalKey],
              visibilityGated,
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
      setTerminalPanes((current) =>
        current[pane.logicalKey]
          ? {
              ...current,
              [pane.logicalKey]: markTerminalOutputCursor(
                current[pane.logicalKey],
                message.chunk.sequence,
              ),
            }
          : current,
      );
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
    void sendTerminalInput(
      pane.session.terminalId,
      data,
      pane.session.serverRoute,
    ).catch((error) => {
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
    return resizeTerminal(
      pane.session.terminalId,
      columns,
      rows,
      pane.session.serverRoute,
    ).then((session) => {
      setTerminalPanes((current) =>
        current[pane.logicalKey]
          ? {
              ...current,
              [pane.logicalKey]: { ...current[pane.logicalKey], session },
            }
          : current,
      );
    });
  }

  function closeTerminalPane(pane: TerminalPaneState) {
    void closeTerminal(pane.session.terminalId, pane.session.serverRoute)
      .then(() =>
        setTerminalPanes((current) =>
          persistTerminalPanesForWorkRoot(
            pane.session.workRootId,
            removeClosedTerminalPane(current, pane.logicalKey),
            pane.session.serverRoute,
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

  function closeAgentPane(
    paneId: string,
    workRootId: string | null | undefined,
  ) {
    if (!workRootId) {
      return;
    }
    setClosedAgentPaneByRoot((current) => ({
      ...current,
      [workRootId]: [...new Set([...(current[workRootId] ?? []), paneId])],
    }));
  }

  function performWorkbenchPaneClose(
    request: DockviewTabCloseRequest & { readonly workRootId?: string },
  ) {
    if (request.workRootId && request.workRootId !== selectedWorkRootId) {
      if (request.surfaceKind === "agent") {
        closeAgentPane(request.paneId, request.workRootId);
      }
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
    } else if (pane.kind === "agentChat") {
      const agentChatPane = Object.values(agentChatPanes).find(
        (candidate) => candidate.paneId === pane.id,
      );
      if (agentChatPane) {
        closeAgentChatPane(agentChatPane);
      }
    }

    if (closeDecision.terminateReservation) {
      setFocusedTerminalPaneId((current) =>
        current === pane.id ? null : current,
      );
      setFocusedAgentChatPaneId((current) =>
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
      const requestWorkRootId = workbenchModel?.root.id ?? selectedWorkRootId;
      if (!requestWorkRootId) {
        return;
      }
      setPendingCloseRequest({
        ...request,
        anchor: decision.anchor,
        workRootId: requestWorkRootId,
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
      // `terminalPaneOrderByGroup` is a separate registry from
      // `paneOrderByRoot` (see the CONTRACT note near its declaration), so a
      // drag/drop move must be mirrored into it for terminal panes or
      // `terminalWorkbenchPanesByGroup` snaps them back to their original
      // group on the next render.
      setTerminalPaneOrderByGroup((current) => {
        const next = { ...current };
        for (const [groupId, paneIds] of Object.entries(
          result.paneOrderByGroup,
        )) {
          next[groupId] = paneIds.filter((id) => id in terminalPanes);
        }
        return next;
      });
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
    const serverRoute = workbenchModel.root.resourcePath.serverId;
    const rootKey = serverScopedIdentity(serverRoute, rootId);
    const paneId = workRootActivityPaneId(rootKey);
    const decision = decideSurfaceOpenWithDynamicGroups(
      workRootActivityPlacementState(workbenchGroups, editorGroups, rootKey),
      {
        surfaceKind: "workRootActivity",
        logicalKey: workRootActivityPaneLogicalKey(rootKey),
        attachmentId:
          paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
      },
    );
    if (decision.type === "openNew") {
      if (decision.createdGroupId) {
        onWorkbenchGroupsByRootChange((current) => ({
          ...current,
          [rootKey]: reconcileDashboardGroupsForPlacement(
            current[rootKey] ?? workbenchGroups,
            decision,
          ),
        }));
      }
      setActivityPaneOpenByRoot((current) => ({ ...current, [rootKey]: true }));
      onPaneOrderByRootChange((current) => ({
        ...current,
        [rootKey]: addPaneToGroupOrder(
          current[rootKey] ?? {},
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
    if (!selectedWorkRootStateKey) {
      return;
    }
    setActivityPaneOpenByRoot((current) => ({
      ...current,
      [selectedWorkRootStateKey]: false,
    }));
    onPaneOrderByRootChange((current) => ({
      ...current,
      [selectedWorkRootStateKey]: removePaneFromOrder(
        current[selectedWorkRootStateKey] ?? {},
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
    workRootActivityState.rootId === root.id &&
      workRootActivityState.serverRoute === root.resourcePath.serverId
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
        onCreateAgentChat={createAgentChatPane}
      />
      {error ? (
        <InlineNotice tone="error" title="Refresh failed" detail={error} />
      ) : null}
      {loading ? (
        <InlineNotice tone="info" title="Refreshing" detail="resources" />
      ) : null}
      {openWorkRootInstances.map(({ rootKey, editorGroups: rootGroups }) => {
        const isActiveRoot = rootKey === selectedWorkRootStateKey;
        let effectiveActivePaneByGroup = activePaneByRoot[rootKey] ?? {};
        if (isActiveRoot && pendingReadOnlyActivation) {
          effectiveActivePaneByGroup = selectWorkbenchPane(
            effectiveActivePaneByGroup,
            pendingReadOnlyActivation.groupId,
            pendingReadOnlyActivation.paneId,
          );
        }
        if (isActiveRoot && pendingAgentChatActivation) {
          effectiveActivePaneByGroup = selectWorkbenchPane(
            effectiveActivePaneByGroup,
            pendingAgentChatActivation.groupId,
            pendingAgentChatActivation.paneId,
          );
        }
        return (
          <div
            key={rootKey}
            className="workbench-root-instance"
            data-workbench-root-active={isActiveRoot ? "true" : "false"}
            style={isActiveRoot ? undefined : { display: "none" }}
          >
            <DockviewWorkbenchLayout
              activePaneByGroup={effectiveActivePaneByGroup}
              groups={rootGroups}
              initialGroupSizeById={
                workbenchLayoutRestoreRef.current[rootKey]?.groupSizeById
              }
              onMovePane={movePane}
              onRequestClosePane={requestWorkbenchPaneClose}
              onSelectPane={selectPane}
              onLayoutSnapshot={(sizeByWorkbenchGroupId) => {
                setGroupSizeByRoot((current) => ({
                  ...current,
                  [rootKey]: sizeByWorkbenchGroupId,
                }));
              }}
            />
          </div>
        );
      })}
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
  onCreateAgentChat,
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
  onCreateAgentChat: () => void;
}) {
  const [overflowOpen, setOverflowOpen] = useState(false);
  const overflowRef = useRef<HTMLDivElement | null>(null);
  useDismissableMenu(overflowOpen, overflowRef, () => setOverflowOpen(false));
  const toggles: WorkbenchToggle[] = [
    "viewer",
    "task",
    "diagnostics",
    "events",
    "layout",
  ];
  const actions = toolbarActions(root, selectedEntity);
  const activationAction = actions.find((entry) =>
    activationForAction(entry.action.id),
  );
  const activation = activationAction
    ? activationForAction(activationAction.action.id)
    : null;
  const openRootAction = actions.find(
    ({ action }) => action.id === "openRoot" || action.id === "reconnect",
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
        disabled={
          !activationAction || !activationAction.action.enabled || !activation
        }
        icon={CirclePower}
        label={activationAction?.action.label ?? "Set workRoot activation"}
        onClick={() => {
          if (!activationAction || !activation) {
            return;
          }
          onCommand(
            buildWorkRootActivationCommand(
              activationAction.entityId,
              activation,
              root.resourcePath.serverId,
            ),
          );
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
                buildWorkbenchOpenActivityCommand(
                  root.id,
                  root.resourcePath.serverId,
                ),
                {
                  "workbench.openActivity": onOpenActivity,
                },
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
            onClick={() =>
              runResourceAction(openRootAction.action, openRootAction.entityId)
            }
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
          disabled={
            root.activation !== "online" || root.availability !== "available"
          }
          icon={SquareTerminal}
          label="New terminal"
          onClick={() => {
            onCommand(
              buildTerminalCreateCommand(root.id, root.resourcePath.serverId),
              {
                "terminal.create": onCreateTerminal,
              },
            );
          }}
        />
        <ChromeIconButton
          commandId="agentChat.create"
          disabled={
            root.activation !== "online" || root.availability !== "available"
          }
          icon={MessageSquarePlus}
          label="Open new agent tab"
          onClick={() => {
            onCommand(
              buildAgentChatCreateCommand(root.id, root.resourcePath.serverId),
              {
                "agentChat.create": onCreateAgentChat,
              },
            );
          }}
        />
        <div className="workbench-overflow" ref={overflowRef}>
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
                  <PanelsTopLeft
                    aria-hidden="true"
                    size={14}
                    strokeWidth={1.8}
                  />
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
                      payload: {
                        type: "action",
                        label: toggle,
                        entityId: root.id,
                      },
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
  const [statusState, setStatusState] = useState<{
    serverRoute: string;
    workRootId: string;
    status: WorkRootGitStatus;
  } | null>(null);
  const [branchesState, setBranchesState] = useState<{
    serverRoute: string;
    workRootId: string;
    branches: GitBranchList;
  } | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const branchMenuRef = useRef<HTMLDivElement | null>(null);
  useDismissableMenu(menuOpen, branchMenuRef, () => setMenuOpen(false));
  const [pendingGitAction, setPendingGitAction] = useState<
    "fetch" | "push" | "pull" | null
  >(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [newBranchName, setNewBranchName] = useState("");
  const [baseBranchName, setBaseBranchName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const requestSeq = useRef(0);
  const serverRoute = root.resourcePath.serverId;
  const currentRootIdentity = useRef({
    serverRoute,
    workRootId: root.id,
  });
  currentRootIdentity.current = {
    serverRoute,
    workRootId: root.id,
  };

  const status =
    statusState?.workRootId === root.id && statusState.serverRoute === serverRoute
      ? statusState.status
      : null;
  const branches =
    branchesState?.workRootId === root.id &&
    branchesState.serverRoute === serverRoute
      ? branchesState.branches
      : null;

  const refreshGit = useCallback(
    (reason: string) => {
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
        fetchWorkRootGitStatus(requestedRootId, serverRoute),
        fetchWorkRootGitBranches(requestedRootId, serverRoute),
      ])
        .then(([nextStatus, nextBranches]) => {
          if (
            requestSeq.current !== seq ||
            currentRootIdentity.current.workRootId !== requestedRootId ||
            currentRootIdentity.current.serverRoute !== serverRoute
          )
            return;
          setStatusState(
            nextStatus.available
              ? { serverRoute, workRootId: requestedRootId, status: nextStatus }
              : null,
          );
          setBranchesState({
            serverRoute,
            workRootId: requestedRootId,
            branches: nextBranches,
          });
          setError(null);
        })
        .catch((nextError) => {
          if (
            requestSeq.current !== seq ||
            currentRootIdentity.current.workRootId !== requestedRootId ||
            currentRootIdentity.current.serverRoute !== serverRoute
          )
            return;
          setStatusState(null);
          setBranchesState(null);
          setMenuOpen(false);
          setModalOpen(false);
          setError(
            nextError instanceof Error ? nextError.message : `${reason} failed`,
          );
        });
    },
    [gitCapable, root.id, serverRoute],
  );

  useEffect(() => {
    refreshGit("git status");
  }, [refreshGit]);

  useEffect(() => {
    if (!gitCapable) return;
    return startGitRefreshScheduler(refreshGit, {
      isDocumentHidden: () => document.hidden,
      addDocumentListener: (event, listener) =>
        document.addEventListener(event, listener),
      removeDocumentListener: (event, listener) =>
        document.removeEventListener(event, listener),
      addWindowListener: (event, listener) =>
        window.addEventListener(event, listener),
      removeWindowListener: (event, listener) =>
        window.removeEventListener(event, listener),
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

  const branchLabel =
    status.branch?.name ??
    (status.branch?.detachedOid ? `HEAD ${status.branch.detachedOid}` : "Git");
  const branchOptions = branches?.branches ?? [];
  const defaultBaseBranch =
    branches?.current ??
    branchOptions.find((branch) => branch.current)?.name ??
    branchOptions[0]?.name ??
    "";
  const selectedBaseBranch = baseBranchName || defaultBaseBranch;
  const closeBranchModal = () => {
    setModalOpen(false);
    setNewBranchName("");
    setBaseBranchName("");
  };
  const mutate = (
    command: ReturnType<typeof buildGitRefreshCommand>,
    run: () => Promise<WorkRootGitStatus>,
    pendingAction: "fetch" | "push" | "pull" | null = null,
  ) => {
    const targetRootId = root.id;
    onCommand(command, {
      [command.commandId]: () => {
        if (pendingAction) setPendingGitAction(pendingAction);
        void run()
          .then((nextStatus) => {
            if (
              currentRootIdentity.current.workRootId !== targetRootId ||
              currentRootIdentity.current.serverRoute !== serverRoute
            )
              return;
            setStatusState(
              nextStatus.available
                ? { serverRoute, workRootId: targetRootId, status: nextStatus }
                : null,
            );
            refreshGit("git mutation refresh");
          })
          .catch((nextError) => {
            if (
              currentRootIdentity.current.workRootId !== targetRootId ||
              currentRootIdentity.current.serverRoute !== serverRoute
            )
              return;
            setError(
              nextError instanceof Error
                ? nextError.message
                : "git action failed",
            );
            refreshGit("git mutation failure refresh");
          })
          .finally(() => {
            if (
              currentRootIdentity.current.workRootId === targetRootId &&
              currentRootIdentity.current.serverRoute === serverRoute &&
              pendingAction
            )
              setPendingGitAction(null);
          });
      },
    });
  };

  const runBranchCreateCloseCommand = () =>
    onCommand(buildGitBranchCreateCloseCommand(root.id, serverRoute), {
      "git.branchCreate.close": closeBranchModal,
    });

  return (
    <div className="git-toolbar" aria-label="Git toolbar">
      <div className="git-branch-menu-wrap" ref={branchMenuRef}>
        <button
          className="meta-chip ws-chip git-branch-chip"
          data-command-id="git.branchMenu.open"
          type="button"
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          onClick={() =>
            onCommand(buildGitBranchMenuOpenCommand(root.id, serverRoute), {
              "git.branchMenu.open": () => setMenuOpen((open) => !open),
            })
          }
        >
          <GitBranch aria-hidden="true" size={13} strokeWidth={1.8} />
          <span>{branchLabel}</span>
        </button>
        {menuOpen ? (
          <div className="workbench-overflow-menu git-branch-menu" role="menu">
            <button
              className="workbench-overflow-item"
              data-command-id="git.branchCreate.open"
              role="menuitem"
              type="button"
              onClick={() => {
                setMenuOpen(false);
                onCommand(buildGitBranchCreateOpenCommand(root.id, serverRoute), {
                  "git.branchCreate.open": () => setModalOpen(true),
                });
              }}
            >
              <Plus aria-hidden="true" size={14} strokeWidth={1.8} />
              <span>+ New branch...</span>
            </button>
            {branchOptions.map((branch) => (
              <button
                key={branch.name}
                className="workbench-overflow-item"
                data-command-id="git.branch.switch"
                disabled={
                  branch.current || (branch.checkedOut && !branch.current)
                }
                role="menuitem"
                type="button"
                title={branch.disabledReason ?? branch.name}
                onClick={() => {
                  setMenuOpen(false);
                  mutate(
                    buildGitBranchSwitchCommand(root.id, branch.name, serverRoute),
                    () =>
                      switchWorkRootGitBranch(root.id, branch.name, serverRoute),
                  );
                }}
              >
                <GitBranch aria-hidden="true" size={14} strokeWidth={1.8} />
                <span>
                  {branch.name}
                  {branch.current ? " ✓" : ""}
                  {branch.checkedOut && !branch.current ? " (checked out)" : ""}
                </span>
              </button>
            ))}
          </div>
        ) : null}
      </div>
      <GitStatusPill
        status={status}
        pendingAction={pendingGitAction}
        onFetch={() =>
          mutate(
            buildGitFetchCommand(root.id, serverRoute),
            () => fetchWorkRootGit(root.id, serverRoute),
            "fetch",
          )
        }
        onPush={() =>
          mutate(
            buildGitPushCommand(root.id, serverRoute),
            () => pushWorkRootGit(root.id, serverRoute),
            "push",
          )
        }
        onPull={() =>
          mutate(
            buildGitPullFfOnlyCommand(root.id, serverRoute),
            () => pullWorkRootGitFfOnly(root.id, serverRoute),
            "pull",
          )
        }
      />
      {error ? (
        <span className="meta-chip ws-chip git-error-chip">{error}</span>
      ) : null}
      <ModalOverlay
        className="root-picker-backdrop"
        isDismissable
        isOpen={modalOpen}
        onOpenChange={(open) => {
          if (!open) runBranchCreateCloseCommand();
        }}
      >
        <Modal className="root-picker-modal git-branch-modal">
          <Dialog aria-label="New Git branch" className="root-picker-dialog">
            <div className="root-picker-titlebar">
              <Heading className="root-picker-title" slot="title">
                New branch
              </Heading>
              <div className="root-picker-window-actions">
                <ChromeIconButton
                  className="root-picker-close-button"
                  commandId="git.branchCreate.close"
                  icon={X}
                  label="Close"
                  onClick={runBranchCreateCloseCommand}
                />
              </div>
            </div>
            <form
              className="git-branch-create-form"
              onSubmit={(event) => {
                event.preventDefault();
                const branchName = newBranchName.trim();
                const baseBranch = selectedBaseBranch.trim();
                if (!branchName) return;
                const targetRootId = root.id;
                onCommand(
                  buildGitBranchCreateSubmitCommand(
                    root.id,
                    branchName,
                    baseBranch || undefined,
                    serverRoute,
                  ),
                  {
                    "git.branchCreate.submit": () => {
                      void createWorkRootGitBranch(
                        root.id,
                        branchName,
                        baseBranch || undefined,
                        serverRoute,
                      )
                        .then((nextStatus) => {
                          if (
                            currentRootIdentity.current.workRootId !==
                              targetRootId ||
                            currentRootIdentity.current.serverRoute !==
                              serverRoute
                          )
                            return;
                          setStatusState({
                            serverRoute,
                            workRootId: targetRootId,
                            status: nextStatus,
                          });
                          closeBranchModal();
                          refreshGit("git branch create refresh");
                        })
                        .catch((nextError) => {
                          if (
                            currentRootIdentity.current.workRootId !==
                              targetRootId ||
                            currentRootIdentity.current.serverRoute !==
                              serverRoute
                          )
                            return;
                          setError(
                            nextError instanceof Error
                              ? nextError.message
                              : "branch create failed",
                          );
                          refreshGit("git branch create failure refresh");
                        });
                    },
                  },
                );
              }}
            >
              <label className="git-worktree-field">
                <span className="section-label">Branch name</span>
                <input
                  className="root-picker-input"
                  value={newBranchName}
                  onChange={(event) => setNewBranchName(event.target.value)}
                  placeholder="feature-name"
                />
              </label>
              <label className="git-worktree-field">
                <span className="section-label">Base branch</span>
                <select
                  className="root-picker-input"
                  value={selectedBaseBranch}
                  onChange={(event) => setBaseBranchName(event.target.value)}
                >
                  {branchOptions.map((branch) => (
                    <option key={branch.name} value={branch.name}>
                      {branch.name}
                      {branch.current ? " (current)" : ""}
                    </option>
                  ))}
                </select>
              </label>
              <div className="root-picker-footer-actions">
                <button
                  className="action-button action-button-primary"
                  data-command-id="git.branchCreate.submit"
                  type="submit"
                  disabled={!newBranchName.trim() || !selectedBaseBranch}
                >
                  Create and switch
                </button>
                <button
                  className="action-button"
                  data-command-id="git.branchCreate.close"
                  type="button"
                  onClick={runBranchCreateCloseCommand}
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

function GitStatusPill({
  status,
  pendingAction,
  onFetch,
  onPush,
  onPull,
}: {
  status: WorkRootGitStatus;
  pendingAction: "fetch" | "push" | "pull" | null;
  onFetch: () => void;
  onPush: () => void;
  onPull: () => void;
}) {
  const changeSegments = gitChangeStatusSegments(status);
  const syncSegments = gitSyncStatusSegments(status);
  const renderSegment = (segment: GitStatusSegment) => {
    const className = `git-status-segment git-status-segment-${segment.tone}`;
    if (segment.commandId === "git.push") {
      return (
        <button
          key={segment.key}
          className={className}
          data-command-id="git.push"
          type="button"
          disabled={segment.disabled || pendingAction === "push"}
          aria-label={
            pendingAction === "push" ? "Pushing Git changes" : undefined
          }
          onClick={onPush}
        >
          {pendingAction === "push" ? (
            <RefreshCw
              className="git-spinner"
              aria-hidden="true"
              size={12}
              strokeWidth={1.9}
            />
          ) : (
            segment.label
          )}
        </button>
      );
    }
    if (segment.commandId === "git.pullFfOnly") {
      return (
        <button
          key={segment.key}
          className={className}
          data-command-id="git.pullFfOnly"
          type="button"
          disabled={segment.disabled}
          onClick={onPull}
        >
          {segment.label}
        </button>
      );
    }
    return (
      <span key={segment.key} className={className}>
        {segment.label}
      </span>
    );
  };

  return (
    <span
      className="meta-chip ws-chip git-status-pill"
      title={status.branch?.upstream ?? "Git status"}
      aria-label={`Git status ${gitStatusSegments(status)}`}
    >
      <button
        className="git-status-refresh"
        data-command-id="git.fetch"
        type="button"
        aria-label={
          pendingAction === "fetch" ? "Fetching Git status" : "Fetch Git status"
        }
        disabled={pendingAction === "fetch"}
        onClick={onFetch}
      >
        <RefreshCw
          className={pendingAction === "fetch" ? "git-spinner" : undefined}
          aria-hidden="true"
          size={12}
          strokeWidth={1.9}
        />
      </button>
      {changeSegments.length ? (
        changeSegments.map(renderSegment)
      ) : (
        <span className="git-status-segment git-status-segment-clean">
          clean
        </span>
      )}
      {syncSegments.length ? (
        <span className="git-status-separator" aria-hidden="true">
          |
        </span>
      ) : null}
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
    id: workRootActivityPaneId(
      serverScopedIdentity(root.resourcePath.serverId, root.id),
    ),
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
        serverRoute={root.resourcePath.serverId}
      />
    ),
  };
}

type ActivityTranscriptRefreshSignal = {
  readonly rootId: string;
  readonly serverRoute?: string | null;
  readonly activityId: string;
  readonly cursor: string | null;
  readonly sequence: number;
};

function WorkRootActivityPane({
  activity,
  onCommand,
  transcriptRefresh,
  serverRoute,
}: {
  activity: WorkRootActivityBadgeInput;
  onCommand: DashboardCommandDispatcher;
  transcriptRefresh: ActivityTranscriptRefreshSignal | null;
  serverRoute: string;
}) {
  // CONTRACT: A reversible read-only Activity Console projection. It consumes
  // source-neutral feed items/transcripts, exposes command-routed controls, and
  // offers no agent/exec control actions or daemon-side acknowledgement.
  return (
    <section className="workroot-activity-pane" aria-label="WorkRoot Activity">
      {activity.phase === "loading" ? (
        <div className="workroot-activity-state">Loading workRoot activity</div>
      ) : activity.phase === "error" ? (
        <div className="workroot-activity-state workroot-activity-state-error">
          WorkRoot activity is unavailable
        </div>
      ) : (
        <ActivityConsole
          view={activity.view}
          onCommand={onCommand}
          loadTranscript={(workRootId, activityId, options) =>
            fetchWorkRootActivityTranscript(workRootId, activityId, {
              ...options,
              serverRoute,
            })
          }
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
  agentChatPanes: AgentChatPaneState[],
  agentChatPaneOrderByGroup: WorkbenchPaneOrder,
  agentChatActions: AgentChatPaneActions,
  closedAgentPaneIds: readonly string[] = [],
  activityPaneOpen = false,
  activityState: WorkRootActivityBadgeInput = { phase: "loading" },
  activityTranscriptRefresh: ActivityTranscriptRefreshSignal | null,
  onCommand: DashboardCommandDispatcher,
  onDocumentSaved: (source: {
    serverRoute?: string;
    workRootId: string;
    path: string;
    content: string;
    contentHash: string;
    sizeBytes: number;
  }) => void,
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
  const agentChatPanesByGroup = agentChatWorkbenchPanesByGroup(
    root,
    agentChatPanes,
    agentChatPaneOrderByGroup,
    agentChatActions,
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
      // CONTRACT: agentChat panes must be spread in *after* the read-only
      // file panes, not before. This array's index drives each pane's
      // position within its Dockview group; a brand-new agentChat pane
      // always sits at the *end* of this array (nothing else is inserted
      // after it), so appending it here means every pre-existing pane in
      // the group keeps its prior index. If agentChat panes were spliced in
      // earlier (as they were originally), adding one shifts every
      // already-open pane's index by one, forcing
      // `syncDockviewWorkbench` to call `existingPanel.api.moveTo(...,
      // { skipSetActive: true })` on those already-active panes -
      // confirmed by instrumentation to reassert them as Dockview's active
      // tab despite `skipSetActive`, which silently clobbers the new
      // agentChat panel's `inactive: false` placement (see
      // 260711-feat-ws-dashboard-agent-activity-chat-ui Phase 1). Keeping
      // existing panes' indices stable avoids the moveTo call entirely.
      ...(agentChatPanesByGroup[group.id] ?? []),
    ],
  }));
}

function placeAgentChatPane(
  current: WorkbenchPaneOrder,
  existingPanes: Record<string, AgentChatPaneState>,
  pane: AgentChatPaneState,
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
): WorkbenchPaneOrder {
  const placementState = agentChatPlacementState(
    existingPanes,
    groups,
    workbenchPaneOrderByGroup,
    current,
  );
  const decision = decideSurfaceOpenWithDynamicGroups(placementState, {
    surfaceKind: "agentChat",
    logicalKey: surfaceLogicalKey("agentChat", pane.workRootId, pane.tabId),
  });
  if (decision.type !== "openNew") {
    return current;
  }
  return {
    ...current,
    [decision.groupId]: [...(current[decision.groupId] ?? []), pane.paneId],
  };
}

function agentChatPlacementState(
  panesByLogicalKey: Record<string, AgentChatPaneState>,
  groups: ReadonlyArray<{ id: string; label: string }>,
  workbenchPaneOrderByGroup: WorkbenchPaneOrder,
  agentChatPaneOrderByGroup: WorkbenchPaneOrder,
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
          agentChatPaneOrderByGroup,
          firstGroupId,
        ),
      ),
      surfaceKind: "agentChat",
      logicalKey: surfaceLogicalKey("agentChat", pane.workRootId, pane.tabId),
    })),
  };
}

function agentChatWorkbenchPanesByGroup(
  root: WorkRootView,
  agentChatPanes: AgentChatPaneState[],
  agentChatPaneOrderByGroup: WorkbenchPaneOrder,
  actions: AgentChatPaneActions,
  groups: ReadonlyArray<{ id: string; label: string }>,
): Record<string, WorkbenchPane[]> {
  const panes = agentChatPanes
    .filter(
      (pane) =>
        pane.workRootId === root.id &&
        (pane.serverRoute ?? "server-local") === root.resourcePath.serverId,
    )
    .map((pane) => agentChatWorkbenchPane(pane, actions));
  const paneById = new Map(panes.map((pane) => [pane.id, pane]));
  const consumed = new Set<string>();
  const byGroup: Record<string, WorkbenchPane[]> = Object.fromEntries(
    groups.map((group) => [group.id, []]),
  );
  for (const groupId of groups.map((group) => group.id)) {
    for (const paneId of agentChatPaneOrderByGroup[groupId] ?? []) {
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

type AgentChatPaneActions = {
  onClose: (pane: AgentChatPaneState) => void;
  onStartHarness: (pane: AgentChatPaneState, harness: AgentChatHarness) => void;
  onResumeHistoryItem: (pane: AgentChatPaneState, item: ActivityItem) => void;
  onLoadHistory: (
    workRootId: string,
    serverRoute: string | null | undefined,
  ) => Promise<{ items: ActivityItem[] }>;
  isActivePane: (pane: AgentChatPaneState) => boolean;
};

function agentChatWorkbenchPane(
  pane: AgentChatPaneState,
  actions: AgentChatPaneActions,
): WorkbenchPane {
  const session = pane.session;
  const state: ViewState = {
    status: pane.starting ? "starting" : session ? "running" : "idle",
    loading: pane.starting,
    stale: false,
    error: pane.error,
  };
  return {
    id: pane.paneId,
    kind: "agentChat",
    category: "opened",
    title: session
      ? `${agentChatHarnessLabel[session.harness]}${session.title ? ` — ${session.title}` : ""}`
      : "New agent chat",
    detail: session ? session.activityId : "no conversation started yet",
    state,
    meta: session
      ? [session.harness, closeContractLabel("agentChat")]
      : ["empty", closeContractLabel("agentChat")],
    contentRevision: session
      ? `agentChat:${pane.paneId}:${session.activityId}`
      : `agentChat:${pane.paneId}:empty`,
    body: <AgentChatPaneBody key={pane.paneId} pane={pane} actions={actions} />,
  };
}

function mergeStreamingTranscriptBlocks(
  blocks: readonly TranscriptBlock[],
  streaming: Record<string, TranscriptBlock>,
): TranscriptBlock[] {
  const merged = blocks.map((block) => streaming[block.cursor] ?? block);
  const appended = Object.values(streaming).filter(
    (block) => !blocks.some((existing) => existing.cursor === block.cursor),
  );
  return [...merged, ...appended];
}

function AgentChatPaneBody({
  pane,
  actions,
}: {
  pane: AgentChatPaneState;
  actions: AgentChatPaneActions;
}) {
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyItems, setHistoryItems] = useState<ActivityItem[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const historyRef = useRef<HTMLDivElement | null>(null);
  useDismissableMenu(historyOpen, historyRef, () => setHistoryOpen(false));

  // Stub-side per-line streaming demo (`260711` Phase 2): once a session is
  // active, grow one synthetic agent-turn block over several ticks so the
  // bubble transcript renders incrementally, mirroring what a live streaming
  // harness backend would eventually push. Purely additive to the session's
  // own `transcript.blocks` - never mutates them.
  const [streamingBlocks, setStreamingBlocks] = useState<Record<string, TranscriptBlock>>({});
  const activeActivityId = pane.session?.activityId ?? null;
  useEffect(() => {
    if (!activeActivityId) {
      return;
    }
    setStreamingBlocks({});
    const handle = stubBeginStreamingTurn((block) => {
      setStreamingBlocks((current) => ({ ...current, [block.cursor]: block }));
    });
    return () => handle.stop();
  }, [activeActivityId]);

  if (pane.session) {
    const { session } = pane;
    const transcriptBlocks = mergeStreamingTranscriptBlocks(
      session.transcript.blocks,
      streamingBlocks,
    );
    return (
      <div className="agent-chat-pane" data-agent-chat-pane-state="active">
        <div className="agent-chat-pane-header">
          <span className="agent-chat-pane-harness">
            {agentChatHarnessLabel[session.harness]}
          </span>
          <span className="agent-chat-pane-title">{session.title}</span>
        </div>
        <div className="agent-chat-pane-transcript" data-testid="agent-chat-transcript">
          <AgentChatTranscriptBubbles
            blocks={transcriptBlocks}
            sourceKind={session.transcript.source.kind}
          />
        </div>
      </div>
    );
  }

  const openHistory = () => {
    setHistoryOpen(true);
    setHistoryLoading(true);
    void actions
      .onLoadHistory(pane.workRootId, pane.serverRoute)
      .then((response) => setHistoryItems(response.items))
      .catch(() => setHistoryItems([]))
      .finally(() => setHistoryLoading(false));
  };

  return (
    <div className="agent-chat-pane" data-agent-chat-pane-state="empty">
      <div className="agent-chat-pane-topbar" ref={historyRef}>
        <button
          className="agent-chat-resume-control"
          data-command-id="agentChat.history.open"
          type="button"
          onClick={openHistory}
        >
          <History aria-hidden="true" size={15} strokeWidth={1.8} />
          resume a past conversation
        </button>
        {historyOpen ? (
          <div
            className="agent-chat-history-popover"
            data-testid="agent-chat-history-popover"
            role="dialog"
          >
            {historyLoading ? (
              <div className="agent-chat-history-empty">Loading…</div>
            ) : historyItems.length === 0 ? (
              <div className="agent-chat-history-empty">
                No past conversations for this work root.
              </div>
            ) : (
              <ul className="agent-chat-history-list">
                {historyItems.map((item) => (
                  <li key={item.id}>
                    <button
                      className="agent-chat-history-item"
                      data-history-item-id={item.id}
                      type="button"
                      onClick={() => {
                        setHistoryOpen(false);
                        actions.onResumeHistoryItem(pane, item);
                      }}
                    >
                      <span className="agent-chat-history-item-harness">
                        {item.source.harness ?? item.kind}
                      </span>
                      <span className="agent-chat-history-item-title">
                        {item.label}
                      </span>
                      <span className="agent-chat-history-item-meta">
                        {item.updatedAt ?? ""}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        ) : null}
      </div>
      <div className="agent-chat-tiles" data-testid="agent-chat-tiles">
        {agentChatHarnesses.map((harness) => (
          <button
            className="agent-chat-tile"
            data-agent-chat-tile={harness}
            disabled={pane.starting}
            key={harness}
            type="button"
            onClick={() => actions.onStartHarness(pane, harness)}
          >
            <Bot aria-hidden="true" size={22} strokeWidth={1.6} />
            <span>{agentChatHarnessLabel[harness]}</span>
          </button>
        ))}
      </div>
      {pane.error ? (
        <div className="agent-chat-pane-error" role="alert">
          {pane.error}
        </div>
      ) : null}
    </div>
  );
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
    .filter(
      (pane) =>
        pane.session.workRootId === root.id &&
        (pane.session.serverRoute ?? "server-local") ===
          root.resourcePath.serverId,
    )
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
  onVisibilityGated: (pane: TerminalPaneState, visibilityGated: boolean) => void;
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
  // Looked up once at TerminalPaneBody mount, by `pane.logicalKey`, against
  // the session-lifetime `terminalVisualRestoreRef` snapshot. Returns
  // `undefined` for a brand-new session (restore-intent fallback or a
  // logicalKey never captured before) so the mount effect falls back to the
  // existing plain-text `pane.output` replay.
  onVisualRestoreEntryFor: (
    pane: TerminalPaneState,
  ) => TerminalVisualRestoreEntry | undefined;
  // Fired from the debounced capture effect once per ~900ms of quiet PTY
  // output. Persists (or replaces) this pane's entry in the browser-local
  // visual-restore snapshot, keyed by `pane.logicalKey`.
  onVisualCapture: (
    pane: TerminalPaneState,
    capture: { serialized: string; viewportY: number; nextSequence: number },
  ) => void;
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
  // Latest fitNow/forwardSize closures from the mount effect below, exposed so
  // the paneVisible-gated socket effect can trigger a corrective refit (on a
  // false -> true visibility transition, or a terminalId change while
  // already visible) without duplicating the fit logic. Nulled on
  // mount-effect cleanup so a stray call can never reach a disposed terminal.
  const fitNowRef = useRef<(() => void) | null>(null);
  const forwardSizeRef = useRef<(() => void) | null>(null);
  // Serialize addon instance for this pane's terminal, loaded once at mount
  // so the debounced visual-buffer capture effect below can call
  // `.serialize()` without re-creating it on every output frame. Nulled on
  // mount-effect cleanup so a stray fire cannot reach a disposed terminal.
  const serializeAddonRef = useRef<SerializeAddon | null>(null);
  const visualCaptureTimerRef = useRef<number | null>(null);
  const keepTerminalFocusRef = useRef(false);
  const [displaySession, setDisplaySession] = useState(() => pane.session);
  // Optimistic default matches current always-connect behavior for the
  // common case of a newly mounted, actually-visible pane; a pane mounted
  // while already hidden briefly opens then closes on the first watchdog
  // tick, an accepted minor inefficiency, not a correctness issue.
  const [paneVisible, setPaneVisible] = useState(true);
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
    const serializeAddon = new SerializeAddon();
    terminal.loadAddon(serializeAddon);
    serializeAddonRef.current = serializeAddon;
    terminal.open(container);
    terminalRef.current = terminal;
    writtenLengthRef.current = 0;

    // A reattached pane with a matching persisted visual-restore snapshot
    // (id-reattach to a still-alive daemon terminal) writes that serialized
    // buffer - scrollback, cursor position, styles - plus its scroll
    // viewport offset, instead of the plain-text `pane.output` replay below.
    // `writtenLengthRef` stays at 0 in both branches: the delta-write effect
    // tracks `pane.output` length independent of whichever initial write
    // happened here, so a restored snapshot's own escape-sequence text is
    // never diffed against `pane.output` (which starts at "" for a freshly
    // reattached pane either way). New sessions spawned via the
    // restore-intent fallback have no matching entry and fall through to the
    // existing replay path unchanged.
    //
    // The three-way branch selection itself (restore vs. replay vs. no-op)
    // is pure and lives in `resolveTerminalMountWrite` (`workbench/terminalVisualRestore.ts`)
    // so it is unit testable independent of xterm/DOM; only the actual
    // `terminal.write`/`scrollToLine`/`writtenLengthRef` side effects stay here.
    const restoreEntry = liveRef.current.actions.onVisualRestoreEntryFor(
      liveRef.current.pane,
    );
    const mountWrite = resolveTerminalMountWrite(
      liveRef.current.pane,
      restoreEntry,
    );
    if (mountWrite.kind === "restore") {
      // `terminal.write()` is asynchronous (parsed on a later tick via the
      // internal write buffer), so `scrollToLine` must run in the write's
      // completion callback - calling it immediately after `write()` would
      // clamp the scroll target against the then-still-empty buffer.
      terminal.write(mountWrite.serialized, () => {
        terminal.scrollToLine(mountWrite.viewportY);
      });
    } else if (mountWrite.kind === "replay") {
      // Replay PTY output buffered before this surface mounted so reselecting
      // a terminal tab restores its emulator contents.
      terminal.write(mountWrite.text);
      writtenLengthRef.current = mountWrite.text.length;
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
    window.addEventListener(
      "focusin",
      clearFocusedTerminalOnOutsideFocus,
      true,
    );

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
      // Guard against a degenerate short-container collapse: a *visible* pane
      // whose usable height momentarily measures too small to host a real
      // grid (e.g. during dockview relayout on a tab/session switch, or an
      // actually-short window/split) makes both `fitAddon.fit()` and the
      // shrink loop below drive `terminal.rows` down to the vendor floor
      // (`1`), which also clears the rendered screen. `proposeDimensions()`
      // reports what `fit()` would apply without applying it; when it is
      // unmeasurable or proposes the degenerate floor, skip the fit/shrink
      // entirely and preserve the last-good emulator size instead. This is
      // the fit-relevant *measured* signal, unlike `offsetParent`, which
      // stays non-null (pane visible) throughout this collapse.
      const proposed = fitAddon.proposeDimensions();
      if (!proposed || proposed.rows <= 1) {
        return;
      }
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
    fitNowRef.current = fitNow;

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
    forwardSizeRef.current = forwardSize;

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
      const nowVisible = Boolean(container.offsetParent);
      setPaneVisible((current) => (current === nowVisible ? current : nowVisible));
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
      // Belt-and-suspenders alongside the debounced capture effect's own
      // cleanup: guarantees no pending serialize callback can ever fire
      // against a disposed terminal, even if effect cleanup ordering changed.
      if (visualCaptureTimerRef.current !== null) {
        window.clearTimeout(visualCaptureTimerRef.current);
        visualCaptureTimerRef.current = null;
      }
      terminal.dispose();
      terminalRef.current = null;
      serializeAddonRef.current = null;
      fitNowRef.current = null;
      forwardSizeRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!paneVisible) {
      liveRef.current.actions.onSocketStatus(
        liveRef.current.pane,
        "disconnected",
        null,
      );
      // Mark this closure as "gated because hidden," not a real disconnect,
      // so the HTTP output-poll fallback does not pick up an idle hidden pane
      // (see `shouldPollTerminalOutput`) - only genuine socket errors/exits
      // should fall back to polling.
      liveRef.current.actions.onVisibilityGated(liveRef.current.pane, true);
      return;
    }
    liveRef.current.actions.onVisibilityGated(liveRef.current.pane, false);
    // Deterministic corrective refit: this runs on every effect setup where
    // `paneVisible` is true (deps `[terminalId, paneVisible]`) - both a
    // false -> true visibility transition (pane shown again after a
    // tab/session/workRoot switch) and a `terminalId` change while the pane
    // stays visible. Either way, the pane may have been measured
    // short-but-visible for a frame while it was still transitioning (see
    // fitNow's degenerate-container guard above), or measured while briefly
    // hidden/detached (no ResizeObserver correction). Explicitly re-fit now
    // rather than relying solely on the next incidental ResizeObserver
    // callback, and forward the size only if it actually changed, reusing
    // the existing fitNow/forwardSize closures. Harmless on the extra
    // terminalId-change trigger: the refit is idempotent and forwardSize
    // dedupes via `lastForwardedSizeRef`.
    const beforeFit = terminalRef.current
      ? { columns: terminalRef.current.cols, rows: terminalRef.current.rows }
      : null;
    fitNowRef.current?.();
    if (
      beforeFit &&
      terminalRef.current &&
      (terminalRef.current.cols !== beforeFit.columns ||
        terminalRef.current.rows !== beforeFit.rows)
    ) {
      forwardSizeRef.current?.();
    }
    let disposed = false;
    const socket = new WebSocket(
      terminalWebSocketUrl(
        terminalId,
        terminalWebSocketCursor(liveRef.current.pane),
        window.location,
        liveRef.current.pane.session.serverRoute,
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
  }, [terminalId, paneVisible]);

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

  // Debounced capture of this pane's serialized visual buffer (scrollback,
  // cursor, styles) plus scroll viewport offset, persisted browser-locally
  // so a page reload can restore this pane's appearance for an id-reattached
  // terminal (see the mount effect's restore branch above), rather than only
  // the plain-text `pane.output` history. Triggered on the same `pane.output`
  // changes as the delta-write effect above - any new PTY output is a reason
  // to refresh the snapshot - but coalesced behind an idle timer per the
  // ticket's "writes debounced" constraint, since output can arrive many
  // times per second. The timer is owned by this effect's own cleanup, which
  // React runs on every dependency change and on unmount, so a disposed
  // terminal can never have a pending serialize callback fire against it.
  useEffect(() => {
    visualCaptureTimerRef.current = window.setTimeout(() => {
      visualCaptureTimerRef.current = null;
      const serializeAddon = serializeAddonRef.current;
      const terminal = terminalRef.current;
      if (!serializeAddon || !terminal) {
        return;
      }
      const serialized = serializeAddon.serialize({
        scrollback: terminalVisualRestoreScrollbackLines,
      });
      liveRef.current.actions.onVisualCapture(liveRef.current.pane, {
        serialized,
        viewportY: terminal.buffer.active.viewportY,
        nextSequence: liveRef.current.pane.nextSequence,
      });
    }, terminalVisualRestoreDebounceMs);
    return () => {
      if (visualCaptureTimerRef.current !== null) {
        window.clearTimeout(visualCaptureTimerRef.current);
        visualCaptureTimerRef.current = null;
      }
    };
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
  onCommand: DashboardCommandDispatcher,
  onDocumentSaved: (source: {
    serverRoute?: string;
    workRootId: string;
    path: string;
    content: string;
    contentHash: string;
    sizeBytes: number;
  }) => void,
): Record<string, WorkbenchPane[]> {
  const panes = readOnlyFilePanes
    .filter(
      (pane) =>
        pane.workRootId === root.id &&
        pane.serverRoute === root.resourcePath.serverId,
    )
    .map((pane) =>
      readOnlyWorkbenchPane(root, pane, onCommand, onDocumentSaved),
    );
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
  onDocumentSaved: (source: {
    serverRoute?: string;
    workRootId: string;
    path: string;
    content: string;
    contentHash: string;
    sizeBytes: number;
  }) => void,
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
  onDocumentSaved: (source: {
    serverRoute?: string;
    workRootId: string;
    path: string;
    content: string;
    contentHash: string;
    sizeBytes: number;
  }) => void;
}) {
  const [translationEnabled, setTranslationEnabled] = useState(false);
  const [translationStatus, setTranslationStatus] = useState<
    "idle" | "loading" | "ready" | "unavailable" | "error"
  >("idle");
  const [translationMessage, setTranslationMessage] = useState<string | null>(
    null,
  );
  const [translationOverlay, setTranslationOverlay] = useState<
    DocumentTranslationOverlay | undefined
  >();
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
    const command = buildDocumentModeSetCommand(
      pane.workRootId,
      pane.path,
      mode,
      pane.serverRoute,
    );
    onCommand(command, { [command.commandId]: () => setDocumentMode(mode) });
  };

  const revertDraft = () => {
    const command = buildDocumentRevertCommand(
      pane.workRootId,
      pane.path,
      pane.serverRoute,
    );
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
    const command = buildDocumentSaveCommand(
      pane.workRootId,
      pane.path,
      pane.serverRoute,
    );
    onCommand(command, {
      [command.commandId]: () => {
        if (!baseContentHash) {
          setSaveState("error");
          setSaveMessage("Missing base content hash");
          return;
        }
        setSaveState("saving");
        setSaveMessage("Saving");
        void writeWorkRootTextFile(
          pane.workRootId,
          {
            path: pane.path,
            baseContentHash,
            content: draft,
          },
          pane.serverRoute,
        )
          .then((response) => {
            setBaseContentHash(response.contentHash);
            setSaveState("saved");
            setSaveMessage("Saved");
            setTranslationOverlay(undefined);
            onDocumentSaved({
              serverRoute: pane.serverRoute,
              workRootId: pane.workRootId,
              path: pane.path,
              content: draft,
              contentHash: response.contentHash,
              sizeBytes: response.sizeBytes,
            });
          })
          .catch((error) => {
            const message =
              error instanceof Error ? error.message : "Save failed";
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
        const provider = providers.providers.find(
          (candidate) => candidate.configured,
        );
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
            : `Translation ${response.status}`,
        );
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }
        setTranslationOverlay(undefined);
        setTranslationStatus("unavailable");
        setTranslationMessage(
          error instanceof Error ? error.message : "Translation unavailable",
        );
      });
    return () => {
      cancelled = true;
    };
  }, [
    pane.content,
    pane.path,
    pane.status,
    pane.title,
    pane.workRootId,
    pane.serverRoute,
    renderMarkdown,
    translationEnabled,
  ]);

  const documentFormatLabel = renderMarkdown
    ? "markdown"
    : (pane.languageHint ?? pane.extension ?? "text");
  const translationButtonLabel = translationEnabled
    ? "Disable Korean translation"
    : "Enable Korean translation";
  const translationStatusVisible =
    translationStatus === "loading" ||
    translationStatus === "ready" ||
    translationStatus === "unavailable" ||
    translationStatus === "error";
  const documentPathLabel = pane.path;
  const documentPathTitle = pane.path.startsWith("/")
    ? pane.path
    : `${root.label} / ${pane.path}`;
  const saveStatusLabel =
    saveState === "idle"
      ? draft === pane.content
        ? "clean"
        : "dirty"
      : saveState;
  const showSaveStatusChip =
    documentMode === "edit" && pane.status === "loaded";

  return (
    <div className="readonly-text-pane document-pane ws-pane">
      <div className="readonly-text-pane-header readonly-text-pane-ribbon ws-toolbar">
        <div className="document-ribbon-file">
          <div className="readonly-text-pane-path" title={documentPathTitle}>
            {documentPathLabel}
          </div>
          <div className="readonly-text-pane-badges">
            <span className="meta-chip ws-chip">{pane.mode}</span>
            <span className="meta-chip ws-chip">{documentFormatLabel}</span>
            {showSaveStatusChip ? (
              <span
                className={`meta-chip ws-chip document-save-chip document-save-chip-${saveStatusLabel}`}
                data-document-save-state={saveState}
                title={saveMessage ?? saveStatusLabel}
              >
                {saveStatusLabel}
              </span>
            ) : null}
          </div>
        </div>
        <div className="document-ribbon-controls">
          {pane.status === "loaded" ? (
            <div
              className="document-viewer-segmented"
              role="group"
              aria-label="Document mode"
            >
              <button
                type="button"
                className={`document-viewer-segment${documentMode === "view" ? " is-active" : ""}`}
                aria-label="View document"
                title="View"
                data-command-id="document.mode.set"
                data-document-mode="view"
                onClick={() => setModeCommand("view")}
              >
                <Eye aria-hidden="true" size={13} strokeWidth={1.8} />
              </button>
              <button
                type="button"
                className={`document-viewer-segment${documentMode === "edit" ? " is-active" : ""}`}
                aria-label="Edit document"
                title="Edit"
                data-command-id="document.mode.set"
                data-document-mode="edit"
                onClick={() => setModeCommand("edit")}
              >
                <Pencil aria-hidden="true" size={13} strokeWidth={1.8} />
              </button>
            </div>
          ) : null}
          {pane.status === "loaded" &&
          renderMarkdown &&
          documentMode === "view" ? (
            <button
              type="button"
              className={`document-translation-toggle${translationEnabled ? " is-active" : ""}`}
              aria-label={translationButtonLabel}
              aria-pressed={translationEnabled}
              title={`${translationButtonLabel}; target: Korean`}
              data-command-id="document.translation.toggle"
              onClick={() => {
                const command = buildDocumentTranslationToggleCommand(
                  pane.workRootId,
                  pane.path,
                  pane.serverRoute,
                );
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
              <Languages aria-hidden="true" size={13} strokeWidth={1.8} />
            </button>
          ) : null}
          {translationStatusVisible ? (
            <span
              className="document-translation-status"
              data-translation-status={translationStatus}
            >
              {translationMessage ?? translationStatus}
            </span>
          ) : null}
          {documentMode === "edit" && pane.status === "loaded" ? (
            <div className="document-edit-actions">
              <button
                type="button"
                className="icon-button document-edit-icon-button"
                data-command-id="document.save"
                disabled={saveState === "saving" || draft === pane.content}
                title="Save"
                aria-label="Save document"
                onClick={saveDraft}
              >
                <Save aria-hidden="true" size={13} strokeWidth={1.8} />
              </button>
              <button
                type="button"
                className="icon-button document-edit-icon-button"
                data-command-id="document.revert"
                disabled={saveState === "saving" || draft === pane.content}
                title="Revert"
                aria-label="Revert document draft"
                onClick={revertDraft}
              >
                <RotateCcw aria-hidden="true" size={13} strokeWidth={1.8} />
              </button>
            </div>
          ) : null}
        </div>
      </div>
      {pane.status === "loading" ? (
        <div className="readonly-text-pane-state ws-state-surface">
          Loading file content
        </div>
      ) : pane.status === "error" ? (
        <div className="readonly-text-pane-state readonly-text-pane-error ws-state-surface">
          {pane.error ?? "file read failed"}
        </div>
      ) : (
        <>
          {documentMode === "edit" ? (
            <DocumentRawEditor
              value={draft}
              source={pane}
              ariaLabel={`Raw editor for ${pane.path}`}
              onChange={(nextDraft) => {
                setDraft(nextDraft);
                setSaveState("dirty");
                setSaveMessage("Unsaved changes");
              }}
            />
          ) : renderMarkdown ? (
            <DocumentViewer
              markdown={pane.content}
              path={pane.path}
              overlay={translationOverlay}
            />
          ) : (
            <DocumentRawEditor
              value={pane.content}
              source={pane}
              ariaLabel={`Read-only source viewer for ${pane.path}`}
              editable={false}
            />
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
  serverId,
  selectedId,
  openWorkRootKeys,
  onCommand,
}: {
  workspace: WorkspaceView;
  serverId: string;
  selectedId: string | null;
  openWorkRootKeys: ReadonlySet<string>;
  onCommand: DashboardCommandDispatcher;
}) {
  const compactRoot = compactWorkspaceWorkRoot(workspace);
  const childWorkRoots = workspace.workRoots.filter(
    isWorkspaceNavChildWorkRoot,
  );
  const selectedChildWorkRootIds = new Set(
    childWorkRoots.map((root) => root.id),
  );
  const selectedWorkspace =
    selectedId === workspace.id ||
    workspace.workRoots.some(
      (root) =>
        root.id === selectedId && !selectedChildWorkRootIds.has(root.id),
    );

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
          actionServerId={serverId}
          kind={compactRoot.kind}
          availability={compactRoot.availability}
          activation={compactRoot.activation}
          canAddWorktree={
            compactRoot.kind === "gitPrimaryRoot" ||
            compactRoot.kind === "gitLinkedWorktree"
          }
          isOpenWorkRoot={openWorkRootKeys.has(
            serverScopedIdentity(serverId, compactRoot.id),
          )}
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
        selected={selectedWorkspace}
        actions={workspace.actions}
        actionEntityId={workspace.id}
        actionServerId={serverId}
        canAddWorktree={workspace.workRoots.some(
          (root) =>
            root.kind === "gitPrimaryRoot" || root.kind === "gitLinkedWorktree",
        )}
        debugMeta={["workspace", `${workspace.workRoots.length} roots`]}
        onCommand={onCommand}
      />
      {childWorkRoots.map((root) => (
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
            actionServerId={serverId}
            kind={root.kind}
            availability={root.availability}
            activation={root.activation}
            isOpenWorkRoot={openWorkRootKeys.has(
              serverScopedIdentity(serverId, root.id),
            )}
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

function isWorkspaceNavChildWorkRoot(root: WorkRootView): boolean {
  return root.kind === "gitLinkedWorktree";
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
  actionServerId = "server-local",
  kind,
  availability,
  activation,
  canAddWorktree = false,
  isOpenWorkRoot = false,
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
  actionServerId?: string;
  kind?: WorkRootView["kind"];
  availability?: WorkRootView["availability"];
  activation?: WorkRootView["activation"];
  canAddWorktree?: boolean;
  isOpenWorkRoot?: boolean;
  debugMeta: string[];
  onCommand: DashboardCommandDispatcher;
}) {
  const hasWorkspaceRemove = actions.some(
    (action) => action.enabled && action.id === "workspace.remove",
  );
  const canCloseWorkRoot =
    (presentation === "workRoot" || presentation === "compactWorkRoot") &&
    isOpenWorkRoot &&
    !selected;
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLSpanElement | null>(null);
  useDismissableMenu(menuOpen, menuRef, () => setMenuOpen(false));
  const tone = resourceRowTone(state, availability, activation);
  const metadataTitle = [title, ...debugMeta, `status: ${state.status}`].join(
    " · ",
  );
  return (
    <div
      className={`resource-row ws-row resource-row-${tone}${selected ? " resource-row-selected ws-row-selected" : ""}`}
      data-command-id="resource.select"
      data-resource-id={id}
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
          onCommand({
            commandId: "resource.select",
            payload: { type: "select", entityId: id },
          })
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
      {hasWorkspaceRemove || canCloseWorkRoot ? (
        <span className="resource-row-actions">
          {canCloseWorkRoot ? (
            <ChromeIconButton
              className="resource-row-action"
              commandId="workRoot.close"
              icon={X}
              label={`Close ${title}`}
              onClick={() =>
                onCommand(buildWorkRootCloseCommand(id, actionServerId))
              }
            />
          ) : null}
          {hasWorkspaceRemove ? (
            <span className="workspace-row-menu-wrap" ref={menuRef}>
              <ChromeIconButton
                className="resource-row-action"
                commandId="workspace.menu.open"
                icon={MoreHorizontal}
                label={`More actions for ${title}`}
                onClick={() =>
                  onCommand(buildWorkspaceMenuOpenCommand(actionEntityId), {
                    "workspace.menu.open": () =>
                      setMenuOpen((current) => !current),
                  })
                }
              />
              {menuOpen ? (
                <div
                  className="workbench-overflow-menu workspace-row-menu"
                  role="menu"
                >
                  {canAddWorktree ? (
                    <button
                      className="workbench-overflow-item"
                      data-command-id="gitWorktreeAdd.open"
                      role="menuitem"
                      type="button"
                      onClick={() => {
                        setMenuOpen(false);
                        onCommand(
                          buildGitWorktreeAddOpenCommand(
                            actionEntityId,
                            actionServerId,
                          ),
                        );
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
                      onCommand(
                        buildWorkspaceRemoveCommand(
                          actionEntityId,
                          actionServerId,
                        ),
                      );
                    }}
                  >
                    <Trash2 aria-hidden="true" size={14} strokeWidth={1.8} />
                    <span>Remove workspace...</span>
                  </button>
                </div>
              ) : null}
            </span>
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

function normalizeServerRoute(serverRoute: string) {
  if (typeof window === "undefined") {
    return;
  }

  const normalizedPath = normalizeServerRouteLocation(
    window.location,
    serverRoute,
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
    const workspaceRoot =
      workspace.workRoots.find((root) => !isWorkspaceNavChildWorkRoot(root)) ??
      workspace.workRoots[0] ??
      null;
    if (selectedId === workspace.id && workspaceRoot) {
      const mainInstance = workspaceRoot.mainInstances[0] ?? null;
      return {
        workspace,
        root: workspaceRoot,
        mainInstance,
        selectedInstance: mainInstance,
      };
    }

    for (const root of workspace.workRoots) {
      const mainInstance = root.mainInstances[0] ?? null;
      const rootSelection = {
        workspace,
        root,
        mainInstance,
        selectedInstance: mainInstance,
      };
      fallback ??= rootSelection;

      if (selectedId === root.id) {
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
  if (
    state.error ||
    availability === "inaccessible" ||
    availability === "missing"
  ) {
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
