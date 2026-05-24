import { apiErrorDetail } from "./apiError.js";

export type WorkRootFileEntryKind = "directory" | "file" | "other";

export type WorkRootFileEntryView = {
  name: string;
  path: string;
  kind: WorkRootFileEntryKind;
  status: "ok" | "unavailable" | "unsupported" | string;
  readable: boolean;
  previewEligible: boolean;
};

export type WorkRootFileListView = {
  workRootId: string;
  path: string;
  status: "ok" | string;
  entries: WorkRootFileEntryView[];
};

export type WorkRootTextFileView = {
  workRootId: string;
  path: string;
  name: string;
  status: "ok" | string;
  readOnly: true;
  editable: boolean;
  contentHash: string;
  content: string;
  sizeBytes: number;
  languageHint: string | null;
  extension: string | null;
};

export type ReadOnlyFilePaneMode = "preview" | "pinned";
export type ReadOnlyFileOpenGesture = "singleClick" | "doubleClick";

export type ReadOnlyFilePane = {
  id: string;
  logicalKey: string;
  mode: ReadOnlyFilePaneMode;
  workRootId: string;
  path: string;
  title: string;
  status: "loading" | "loaded" | "error";
  content: string;
  error: string | null;
  readOnly: true;
  editable: boolean;
  contentHash: string | null;
  sizeBytes: number | null;
  languageHint: string | null;
  extension: string | null;
};

export type ReadOnlyFilePaneOrder = Record<string, readonly string[]>;

export type ReadOnlyFilePaneRestoreSnapshot = {
  panes: Record<string, ReadOnlyFilePane>;
  orderByGroup: Record<string, string[]>;
};

export type DocumentSaveState =
  | "idle"
  | "dirty"
  | "saving"
  | "saved"
  | "stale"
  | "conflict"
  | "error";

export type DocumentDraftContentChangeDecision =
  | { action: "preserveDraft"; saveState: "stale"; message: string }
  | { action: "syncDraft" };

export function documentDraftContentChangeDecision(
  saveState: DocumentSaveState,
): DocumentDraftContentChangeDecision {
  if (saveState === "dirty" || saveState === "stale") {
    return {
      action: "preserveDraft",
      saveState: "stale",
      message: "File changed while this draft has unsaved edits",
    };
  }
  return { action: "syncDraft" };
}

export function documentSaveStateForError(message: string): "conflict" | "error" {
  return message.toLowerCase().includes("content hash") ? "conflict" : "error";
}

export function readOnlyFilePaneSourceKey(workRootId: string, path: string) {
  return `${workRootId}\0${path}`;
}

export function applyReadOnlyFilePaneSourceContent(
  panes: Record<string, ReadOnlyFilePane>,
  file: WorkRootTextFileView,
): Record<string, ReadOnlyFilePane> {
  return Object.fromEntries(
    Object.entries(panes).map(([key, pane]) => [
      key,
      pane.workRootId === file.workRootId && pane.path === file.path
        ? applyReadOnlyFilePaneContent(pane, file)
        : pane,
    ]),
  );
}

export function applyReadOnlyFilePaneSourceError(
  panes: Record<string, ReadOnlyFilePane>,
  workRootId: string,
  path: string,
  message: string,
): Record<string, ReadOnlyFilePane> {
  return Object.fromEntries(
    Object.entries(panes).map(([key, pane]) => [
      key,
      pane.workRootId === workRootId && pane.path === path
        ? applyReadOnlyFilePaneError(pane, message)
        : pane,
    ]),
  );
}

type ReadOnlyFilePaneDescriptor = {
  workRootId: string;
  path: string;
  mode: ReadOnlyFilePaneMode;
  title: string;
};

export type DirectoryLoadState = {
  status: "idle" | "loading" | "loaded" | "error";
  entries: WorkRootFileEntryView[];
  error: string | null;
};

export type WorkRootFileTreeRow =
  | {
      type: "entry";
      depth: number;
      entry: WorkRootFileEntryView;
      expanded: boolean;
      selected: boolean;
    }
  | {
      type: "state";
      depth: number;
      path: string;
      status: "loading" | "error" | "empty";
      label: string;
    };

export const idleDirectoryLoadState = (): DirectoryLoadState => ({
  status: "idle",
  entries: [],
  error: null,
});

export function workRootFilesEndpoint(workRootId: string, path = "") {
  const encodedWorkRootId = encodeURIComponent(workRootId);
  const endpoint = `/api/dashboard/work-roots/${encodedWorkRootId}/files`;
  if (!path) {
    return endpoint;
  }

  const query = new URLSearchParams({ path });
  return `${endpoint}?${query.toString()}`;
}

export function workRootFileReadEndpoint(workRootId: string, path: string) {
  const encodedWorkRootId = encodeURIComponent(workRootId);
  const query = new URLSearchParams({ path });
  return `/api/dashboard/work-roots/${encodedWorkRootId}/files/read?${query.toString()}`;
}

export async function fetchWorkRootTextFile(
  workRootId: string,
  path: string,
): Promise<WorkRootTextFileView> {
  const response = await fetch(workRootFileReadEndpoint(workRootId, path), {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as WorkRootTextFileView;
}

export type WorkRootFileWriteRequest = {
  path: string;
  baseContentHash: string;
  content: string;
};

export type WorkRootFileWriteResponse = {
  contentHash: string;
  sizeBytes: number;
  savedAtMs: number;
};

export type WorkRootDocumentEvent = {
  type: "document.contentChanged";
  workRootId: string;
  path: string;
  contentHash: string;
  source: "dashboard" | "filesystem" | string;
  savedAtMs: number;
};

export function workRootDocumentEventsEndpoint(workRootId: string) {
  return `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/documents/events`;
}

export function parseWorkRootDocumentEvent(value: unknown): WorkRootDocumentEvent | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const record = value as Record<string, unknown>;
  const source = record.source as Record<string, unknown> | undefined;
  if (
    record.type !== "document.contentChanged" ||
    !source ||
    typeof source !== "object" ||
    typeof source.workRootId !== "string" ||
    typeof source.path !== "string" ||
    typeof record.contentHash !== "string" ||
    typeof record.changedAtMs !== "number"
  ) {
    return null;
  }
  return {
    type: record.type,
    workRootId: source.workRootId,
    path: source.path,
    contentHash: record.contentHash,
    source: "dashboard",
    savedAtMs: record.changedAtMs,
  };
}

export function workRootFileWriteEndpoint(workRootId: string) {
  return `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/files/write`;
}

export async function writeWorkRootTextFile(
  workRootId: string,
  request: WorkRootFileWriteRequest,
): Promise<WorkRootFileWriteResponse> {
  const response = await fetch(workRootFileWriteEndpoint(workRootId), {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as WorkRootFileWriteResponse;
}

export function applyReadOnlyFilePaneSavedContent(
  pane: ReadOnlyFilePane,
  content: string,
  contentHash: string,
  sizeBytes: number,
): ReadOnlyFilePane {
  return {
    ...pane,
    status: "loaded",
    content,
    contentHash,
    sizeBytes,
    error: null,
  };
}

export function readOnlyFilePaneModeForOpenGesture(
  gesture: ReadOnlyFileOpenGesture,
): ReadOnlyFilePaneMode {
  return gesture === "singleClick" ? "preview" : "pinned";
}

export function readOnlyFilePaneLogicalKey(
  workRootId: string,
  path: string,
  mode: ReadOnlyFilePaneMode = "pinned",
) {
  if (mode === "preview") {
    return ["editor-preview", workRootId].join("/");
  }

  return ["editor", workRootId, path].join("/");
}

export function readOnlyFilePaneId(
  workRootId: string,
  path: string,
  mode: ReadOnlyFilePaneMode = "pinned",
) {
  if (mode === "preview") {
    return `readonly-preview:${encodeURIComponent(workRootId)}`;
  }

  return `readonly:${encodeURIComponent(workRootId)}:${encodeURIComponent(path)}`;
}

export function createLoadingReadOnlyFilePane(
  workRootId: string,
  path: string,
  mode: ReadOnlyFilePaneMode = "pinned",
): ReadOnlyFilePane {
  // CONTRACT: Preview panes are one replaceable logical surface per workRoot;
  // pinned panes remain file-path-addressed stable tabs. App-level single-click
  // and double-click handlers must choose the mode, then placement policy
  // focuses existing pinned files or replaces the preview pane.
  return {
    id: readOnlyFilePaneId(workRootId, path, mode),
    logicalKey: readOnlyFilePaneLogicalKey(workRootId, path, mode),
    mode,
    workRootId,
    path,
    title: fileNameFromPath(path),
    status: "loading",
    content: "",
    error: null,
    readOnly: true,
    editable: false,
    contentHash: null,
    sizeBytes: null,
    languageHint: null,
    extension: null,
  };
}

export function applyReadOnlyFilePaneContent(
  pane: ReadOnlyFilePane,
  file: WorkRootTextFileView,
): ReadOnlyFilePane {
  return {
    ...pane,
    title: file.name || fileNameFromPath(file.path),
    status: "loaded",
    content: file.content,
    error: null,
    editable: file.editable,
    contentHash: file.contentHash,
    sizeBytes: file.sizeBytes,
    languageHint: file.languageHint,
    extension: file.extension,
  };
}

export function applyReadOnlyFilePaneError(pane: ReadOnlyFilePane, error: string): ReadOnlyFilePane {
  return {
    ...pane,
    status: "error",
    content: "",
    error,
  };
}

export function readOnlyFilePaneRestoreSnapshot(
  panes: readonly ReadOnlyFilePane[],
  orderByGroup: ReadOnlyFilePaneOrder = {},
): ReadOnlyFilePaneRestoreSnapshot {
  return {
    panes: Object.fromEntries(
      panes.map((pane) => [
        pane.logicalKey,
        createRestoredReadOnlyFilePane({
          workRootId: pane.workRootId,
          path: pane.path,
          mode: pane.mode,
          title: pane.title,
        }),
      ]),
    ),
    orderByGroup: pruneReadOnlyFilePaneOrder(
      orderByGroup,
      new Set(panes.map((pane) => pane.id)),
    ),
  };
}

export function loadReadOnlyFilePaneRestoreSnapshot(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): ReadOnlyFilePaneRestoreSnapshot {
  if (!storage) {
    return { panes: {}, orderByGroup: {} };
  }
  try {
    const raw = storage.getItem(readOnlyFilePaneRestoreStorageKey);
    if (!raw) {
      return { panes: {}, orderByGroup: {} };
    }
    const parsed = JSON.parse(raw) as {
      version?: unknown;
      panes?: unknown;
      orderByGroup?: unknown;
    };
    if (parsed.version !== 1 || !Array.isArray(parsed.panes)) {
      return { panes: {}, orderByGroup: {} };
    }
    const panes = Object.fromEntries(
      parsed.panes.flatMap((value): Array<[string, ReadOnlyFilePane]> => {
        const descriptor = parseReadOnlyFilePaneDescriptor(value);
        if (!descriptor) {
          return [];
        }
        const pane = createRestoredReadOnlyFilePane(descriptor);
        return [[pane.logicalKey, pane]];
      }),
    );
    return {
      panes,
      orderByGroup: parseReadOnlyFilePaneOrder(
        parsed.orderByGroup,
        new Set(Object.values(panes).map((pane) => pane.id)),
      ),
    };
  } catch {
    return { panes: {}, orderByGroup: {} };
  }
}

export function saveReadOnlyFilePaneRestoreSnapshot(
  panes: readonly ReadOnlyFilePane[],
  orderByGroup: ReadOnlyFilePaneOrder = {},
  storage: Pick<Storage, "setItem" | "removeItem"> | null = browserStorage(),
) {
  if (!storage) {
    return;
  }
  try {
    if (panes.length === 0) {
      storage.removeItem(readOnlyFilePaneRestoreStorageKey);
      return;
    }
    const paneIds = new Set(panes.map((pane) => pane.id));
    storage.setItem(
      readOnlyFilePaneRestoreStorageKey,
      JSON.stringify({
        version: 1,
        panes: panes.map((pane): ReadOnlyFilePaneDescriptor => ({
          workRootId: pane.workRootId,
          path: pane.path,
          mode: pane.mode,
          title: pane.title,
        })),
        orderByGroup: pruneReadOnlyFilePaneOrder(orderByGroup, paneIds),
      }),
    );
  } catch {
    // Browser persistence is best-effort; live pane state remains canonical.
  }
}

export async function fetchWorkRootFiles(
  workRootId: string,
  path = "",
): Promise<WorkRootFileListView> {
  const response = await fetch(workRootFilesEndpoint(workRootId, path), {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as WorkRootFileListView;
}

export function workRootExplorerInitialLoadPath(
  snapshot: { directories: Record<string, DirectoryLoadState> } | null | undefined,
) {
  const rootState = snapshot?.directories[""];
  return !rootState || rootState.status === "idle" ? "" : null;
}

export function workRootExplorerShouldLoadOnExpand(
  snapshot: { directories: Record<string, DirectoryLoadState> } | null | undefined,
  path: string,
  wasExpanded: boolean,
) {
  if (wasExpanded) {
    return false;
  }

  const directoryState = snapshot?.directories[path];
  return !directoryState || directoryState.status === "idle";
}

export function workRootExplorerRefreshPaths(expandedPaths: ReadonlySet<string>) {
  const paths = Array.from(expandedPaths);
  return paths.length > 0 ? paths : [""];
}

export function toggleExpandedPath(expandedPaths: ReadonlySet<string>, path: string) {
  const next = new Set(expandedPaths);
  if (next.has(path)) {
    next.delete(path);
  } else {
    next.add(path);
  }
  return next;
}

export function flattenWorkRootFileTree({
  rootPath = "",
  expandedPaths,
  directories,
  selectedPath,
}: {
  rootPath?: string;
  expandedPaths: ReadonlySet<string>;
  directories: Record<string, DirectoryLoadState>;
  selectedPath: string | null;
}): WorkRootFileTreeRow[] {
  const rows: WorkRootFileTreeRow[] = [];
  appendDirectoryRows(rows, rootPath, 0, expandedPaths, directories, selectedPath);
  return rows;
}

function createRestoredReadOnlyFilePane(
  descriptor: ReadOnlyFilePaneDescriptor,
): ReadOnlyFilePane {
  return {
    ...createLoadingReadOnlyFilePane(
      descriptor.workRootId,
      descriptor.path,
      descriptor.mode,
    ),
    title: descriptor.title.trim() || fileNameFromPath(descriptor.path),
  };
}

function parseReadOnlyFilePaneDescriptor(
  value: unknown,
): ReadOnlyFilePaneDescriptor | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const record = value as Record<string, unknown>;
  if (
    typeof record.workRootId !== "string" ||
    typeof record.path !== "string" ||
    typeof record.title !== "string" ||
    (record.mode !== "preview" && record.mode !== "pinned")
  ) {
    return null;
  }
  const workRootId = record.workRootId.trim();
  const path = record.path.trim();
  const pathSegments = path.split("/");
  if (
    !workRootId ||
    !path ||
    path.startsWith("/") ||
    pathSegments.some((segment) => segment === "." || segment === "..")
  ) {
    return null;
  }
  return {
    workRootId,
    path,
    mode: record.mode,
    title: record.title,
  };
}

function parseReadOnlyFilePaneOrder(
  value: unknown,
  paneIds: ReadonlySet<string>,
): Record<string, string[]> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const order: Record<string, string[]> = {};
  for (const [groupId, paneOrder] of Object.entries(value)) {
    if (!Array.isArray(paneOrder)) {
      continue;
    }
    const ids = paneOrder.filter(
      (paneId): paneId is string =>
        typeof paneId === "string" && paneIds.has(paneId),
    );
    if (ids.length > 0) {
      order[groupId] = [...new Set(ids)];
    }
  }
  return order;
}

function pruneReadOnlyFilePaneOrder(
  orderByGroup: ReadOnlyFilePaneOrder,
  paneIds: ReadonlySet<string>,
): Record<string, string[]> {
  return Object.fromEntries(
    Object.entries(orderByGroup)
      .map(([groupId, paneOrder]) => [
        groupId,
        paneOrder.filter((paneId) => paneIds.has(paneId)),
      ])
      .filter(([, paneOrder]) => paneOrder.length > 0),
  );
}

function fileNameFromPath(path: string) {
  return path.split("/").filter(Boolean).at(-1) ?? path;
}

function appendDirectoryRows(
  rows: WorkRootFileTreeRow[],
  path: string,
  depth: number,
  expandedPaths: ReadonlySet<string>,
  directories: Record<string, DirectoryLoadState>,
  selectedPath: string | null,
) {
  const directory = directories[path] ?? idleDirectoryLoadState();

  if (directory.status === "loading") {
    rows.push({ type: "state", depth, path, status: "loading", label: "Loading" });
    return;
  }

  if (directory.status === "error") {
    rows.push({
      type: "state",
      depth,
      path,
      status: "error",
      label: directory.error ?? "Listing failed",
    });
    return;
  }

  if (directory.status === "loaded" && directory.entries.length === 0) {
    rows.push({ type: "state", depth, path, status: "empty", label: "Empty directory" });
    return;
  }

  for (const entry of directory.entries) {
    const expanded = entry.kind === "directory" && expandedPaths.has(entry.path);
    rows.push({
      type: "entry",
      depth,
      entry,
      expanded,
      selected: selectedPath === entry.path,
    });

    if (expanded) {
      appendDirectoryRows(rows, entry.path, depth + 1, expandedPaths, directories, selectedPath);
    }
  }
}

const readOnlyFilePaneRestoreStorageKey = "ws-dashboard.readOnlyFilePanes.v1";

function browserStorage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.localStorage;
  } catch {
    return null;
  }
}
