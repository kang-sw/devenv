import { apiErrorDetail } from "./apiError.js";

export type RootPickerEntryType = "directory";

export type RootPickerEntry = {
  name: string;
  path: string;
  entryType: RootPickerEntryType;
  selectable: boolean;
  kindLabel?: string;
  modifiedTime?: string | null;
  size?: number | null;
};

export type RootPickerPlaceKind = "home" | "root" | "mount" | "drive" | "pin";
export type RootPickerPlaceSource = "builtIn" | "pin";

export type RootPickerPlace = {
  id: string;
  label: string;
  path: string;
  kind: RootPickerPlaceKind;
  source: RootPickerPlaceSource;
  available: boolean;
};

export type RootPickerPlacesView = {
  places: RootPickerPlace[];
};

export type RootPickerView = {
  currentPath: string;
  parentPath: string | null;
  entries: RootPickerEntry[];
  places?: RootPickerPlace[];
};

export type RootPickerEntryFilter = "foldersOnly" | "all";

export type RootPickerNavigationHistory = {
  currentPath: string | null;
  backStack: string[];
  forwardStack: string[];
};

export const rootPickerEndpoint = "/api/dashboard/root-picker";
export const rootPickerCreateDirectoryEndpoint =
  "/api/dashboard/root-picker/directories";
export const rootPickerPinsEndpoint = "/api/dashboard/root-picker/pins";

export function rootPickerListEndpoint(path: string | null = null) {
  if (path === null || path.length === 0) {
    return rootPickerEndpoint;
  }

  const query = new URLSearchParams({ path });
  return `${rootPickerEndpoint}?${query.toString()}`;
}

export async function fetchRootPicker(path: string | null = null): Promise<RootPickerView> {
  const response = await fetch(rootPickerListEndpoint(path), {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as RootPickerView;
}

export async function createRootPickerDirectory(
  parentPath: string,
  name: string,
): Promise<RootPickerEntry> {
  const response = await fetch(rootPickerCreateDirectoryEndpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ parentPath, name }),
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as RootPickerEntry;
}

export async function pinRootPickerDirectory(path: string): Promise<RootPickerPlacesView> {
  return requestRootPickerPin("POST", path);
}

export async function unpinRootPickerDirectory(path: string): Promise<RootPickerPlacesView> {
  return requestRootPickerPin("DELETE", path);
}

async function requestRootPickerPin(
  method: "POST" | "DELETE",
  path: string,
): Promise<RootPickerPlacesView> {
  const response = await fetch(rootPickerPinsEndpoint, {
    method,
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ path }),
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as RootPickerPlacesView;
}

export function rootPickerEntryLabel(path: string) {
  const parts = path.split(/[\\/]+/).filter(Boolean);
  return parts.at(-1) ?? path;
}

export function rootPickerVisibleEntries(
  entries: readonly RootPickerEntry[],
  filter: RootPickerEntryFilter = "foldersOnly",
): RootPickerEntry[] {
  if (filter === "all") {
    return [...entries];
  }
  return entries.filter((entry) => entry.entryType === "directory");
}

export function rootPickerVisiblePlaces(view: RootPickerView | null): RootPickerPlace[] {
  return (view?.places ?? []).filter(
    (place) => place.available || place.source === "pin",
  );
}

export function rootPickerPinnedPathSet(view: RootPickerView | null): Set<string> {
  return new Set(
    (view?.places ?? [])
      .filter((place) => place.source === "pin")
      .map((place) => place.path),
  );
}

export function rootPickerModifiedTimeLabel(value: string | null | undefined): string {
  if (!value) {
    return "";
  }
  const epochSeconds = Number(value);
  if (!Number.isFinite(epochSeconds)) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "short",
    timeStyle: "short",
  }).format(new Date(epochSeconds * 1000));
}

export function rootPickerInsertEntry(
  entries: readonly RootPickerEntry[],
  entry: RootPickerEntry,
): RootPickerEntry[] {
  const next = entries.filter((candidate) => candidate.path !== entry.path);
  next.push(entry);
  return next.sort((left, right) => left.name.localeCompare(right.name));
}

export function rootPickerHistoryInitial(
  currentPath: string | null = null,
): RootPickerNavigationHistory {
  return { currentPath, backStack: [], forwardStack: [] };
}

export function rootPickerHistoryPush(
  history: RootPickerNavigationHistory,
  nextPath: string,
): RootPickerNavigationHistory {
  if (history.currentPath === nextPath) {
    return history;
  }
  return {
    currentPath: nextPath,
    backStack: history.currentPath
      ? [...history.backStack, history.currentPath]
      : history.backStack,
    forwardStack: [],
  };
}

export function rootPickerHistoryBack(history: RootPickerNavigationHistory): {
  history: RootPickerNavigationHistory;
  targetPath: string | null;
} {
  const targetPath = history.backStack.at(-1) ?? null;
  if (!targetPath) {
    return { history, targetPath: null };
  }
  return {
    targetPath,
    history: {
      currentPath: targetPath,
      backStack: history.backStack.slice(0, -1),
      forwardStack: history.currentPath
        ? [history.currentPath, ...history.forwardStack]
        : history.forwardStack,
    },
  };
}

export function rootPickerHistoryForward(history: RootPickerNavigationHistory): {
  history: RootPickerNavigationHistory;
  targetPath: string | null;
} {
  const targetPath = history.forwardStack[0] ?? null;
  if (!targetPath) {
    return { history, targetPath: null };
  }
  return {
    targetPath,
    history: {
      currentPath: targetPath,
      backStack: history.currentPath
        ? [...history.backStack, history.currentPath]
        : history.backStack,
      forwardStack: history.forwardStack.slice(1),
    },
  };
}
