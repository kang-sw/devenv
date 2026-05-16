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
  content: string;
  sizeBytes: number;
  languageHint: string | null;
  extension: string | null;
};

export type ReadOnlyFilePane = {
  id: string;
  logicalKey: string;
  workRootId: string;
  path: string;
  title: string;
  status: "loading" | "loaded" | "error";
  content: string;
  error: string | null;
  readOnly: true;
  sizeBytes: number | null;
  languageHint: string | null;
  extension: string | null;
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

export function readOnlyFilePaneLogicalKey(workRootId: string, path: string) {
  return ["editor", workRootId, path].join("/");
}

export function readOnlyFilePaneId(workRootId: string, path: string) {
  return `readonly:${encodeURIComponent(workRootId)}:${encodeURIComponent(path)}`;
}

export function createLoadingReadOnlyFilePane(workRootId: string, path: string): ReadOnlyFilePane {
  return {
    id: readOnlyFilePaneId(workRootId, path),
    logicalKey: readOnlyFilePaneLogicalKey(workRootId, path),
    workRootId,
    path,
    title: fileNameFromPath(path),
    status: "loading",
    content: "",
    error: null,
    readOnly: true,
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
