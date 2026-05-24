import { apiErrorDetail } from "./apiError.js";

export type RootPickerEntryType = "directory";

export type RootPickerEntry = {
  name: string;
  path: string;
  entryType: RootPickerEntryType;
  selectable: boolean;
};

export type RootPickerView = {
  currentPath: string;
  parentPath: string | null;
  entries: RootPickerEntry[];
};

export const rootPickerEndpoint = "/api/dashboard/root-picker";
export const rootPickerCreateDirectoryEndpoint =
  "/api/dashboard/root-picker/directories";

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

export function rootPickerEntryLabel(path: string) {
  const parts = path.split(/[\\/]+/).filter(Boolean);
  return parts.at(-1) ?? path;
}

export function rootPickerInsertEntry(
  entries: readonly RootPickerEntry[],
  entry: RootPickerEntry,
): RootPickerEntry[] {
  const next = entries.filter((candidate) => candidate.path !== entry.path);
  next.push(entry);
  return next.sort((left, right) => left.name.localeCompare(right.name));
}

