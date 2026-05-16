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

export async function fetchWorkRootFiles(
  workRootId: string,
  path = "",
): Promise<WorkRootFileListView> {
  const response = await fetch(workRootFilesEndpoint(workRootId, path), {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(await workRootFilesErrorMessage(response));
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

async function workRootFilesErrorMessage(response: Response) {
  try {
    const value = (await response.json()) as { error?: unknown };
    if (typeof value.error === "string" && value.error.trim()) {
      return value.error;
    }
  } catch {
    // Fall through to bounded HTTP status text.
  }

  return `HTTP ${response.status}`;
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
