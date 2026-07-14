import {
  LOCAL_DASHBOARD_SERVER_ROUTE,
  serverScopedIdentity,
} from "../resourceModel.js";
import { browserStorage } from "../workRootFiles.js";
import {
  reconcileActiveWorkbenchPanes,
  type WorkbenchActivePaneState,
  type WorkbenchPaneOrder,
} from "./editorGroupModel.js";

// CONTRACT: Persists the app-owned dockview layout model (group membership,
// tab order, active pane per group, and best-effort split sizes) per
// serverRoute+workRootId, mirroring the shape and defensiveness of
// `workRootFiles.ts`'s read-only-file-pane restore snapshot. This module does
// not attempt Dockview's native `toJSON`/`fromJSON` layout serialization
// (see the Phase 5 plan's Codebase Findings for why that model is
// incompatible with this app's live-params panel reconciliation loop).

export type WorkbenchLayoutGroupSize = {
  width?: number;
  height?: number;
};

export type WorkbenchLayoutRestoreEntry = {
  serverRoute: string;
  workRootId: string;
  groups: ReadonlyArray<{ id: string; label: string }>;
  paneOrderByGroup: WorkbenchPaneOrder;
  activePaneByGroup: WorkbenchActivePaneState;
  groupSizeById?: Record<string, WorkbenchLayoutGroupSize>;
};

export type WorkbenchLayoutRestoreSnapshot = Record<
  string,
  WorkbenchLayoutRestoreEntry
>;

const workbenchLayoutRestoreStorageKey = "ws-dashboard.workbenchLayout.v1";

export function workbenchLayoutRestoreRootKey(
  entry: Pick<WorkbenchLayoutRestoreEntry, "serverRoute" | "workRootId">,
): string {
  return serverScopedIdentity(entry.serverRoute, entry.workRootId);
}

export function loadWorkbenchLayoutRestoreSnapshot(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): WorkbenchLayoutRestoreSnapshot {
  if (!storage) {
    return {};
  }
  try {
    const raw = storage.getItem(workbenchLayoutRestoreStorageKey);
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw) as {
      version?: unknown;
      entries?: unknown;
    };
    if (parsed.version !== 1 || !Array.isArray(parsed.entries)) {
      return {};
    }
    const snapshot: Record<string, WorkbenchLayoutRestoreEntry> = {};
    for (const value of parsed.entries) {
      const entry = parseWorkbenchLayoutRestoreEntry(value);
      if (entry) {
        snapshot[workbenchLayoutRestoreRootKey(entry)] = entry;
      }
    }
    return snapshot;
  } catch {
    return {};
  }
}

export function saveWorkbenchLayoutRestoreSnapshot(
  entries: readonly WorkbenchLayoutRestoreEntry[],
  storage: Pick<Storage, "setItem" | "removeItem"> | null = browserStorage(),
) {
  if (!storage) {
    return;
  }
  try {
    if (entries.length === 0) {
      storage.removeItem(workbenchLayoutRestoreStorageKey);
      return;
    }
    storage.setItem(
      workbenchLayoutRestoreStorageKey,
      JSON.stringify({
        version: 1,
        entries: entries.map((entry) => ({
          serverRoute: entry.serverRoute,
          workRootId: entry.workRootId,
          groups: entry.groups.map((group) => ({
            id: group.id,
            label: group.label,
          })),
          paneOrderByGroup: entry.paneOrderByGroup,
          activePaneByGroup: entry.activePaneByGroup,
          ...(entry.groupSizeById
            ? { groupSizeById: entry.groupSizeById }
            : {}),
        })),
      }),
    );
  } catch {
    // Browser persistence is best-effort; live layout state remains canonical.
  }
}

// Drops pane ids that are no longer live from an order-by-group map, and
// drops any group left empty by that filter. Mirrors
// `workRootFiles.ts`'s `pruneReadOnlyFilePaneOrder` filter shape exactly;
// duplicated rather than imported since that helper is file-private and
// specific to read-only file panes.
export function pruneWorkbenchLayoutOrder(
  orderByGroup: WorkbenchPaneOrder,
  livePaneIds: ReadonlySet<string>,
): Record<string, string[]> {
  return Object.fromEntries(
    Object.entries(orderByGroup)
      .map(([groupId, paneOrder]) => [
        groupId,
        paneOrder.filter((paneId) => livePaneIds.has(paneId)),
      ])
      .filter(([, paneOrder]) => (paneOrder as string[]).length > 0),
  );
}

// Terminal pane ids are always `terminal:`-prefixed (see
// `terminals.ts#terminalPaneId`); read-only file pane ids use `readonly:` /
// `readonly-preview:` prefixes (`workRootFiles.ts#readOnlyFilePaneId`). Used
// by `revalidateWorkbenchLayoutForRoot` to scope the terminals-ready grace
// window to terminal pane references only.
export function isTerminalPaneId(paneId: string): boolean {
  return paneId.startsWith("terminal:");
}

// CONTRACT: pure per-root revalidation transformation extracted out of
// `WorkbenchShell`'s prune/reconcile effect in App.tsx (Phase 5 review
// Test-partition finding), so the highest-restore-correctness-risk glue logic
// is unit-testable without a React harness, mirroring `findOpenWorkRoot`
// (Phase 1), `resolveClosedWorkRootRefs` (Phase 2), and
// `plan_output_backfill` (Phase 4).
//
// `terminalsReady` gates only the terminal-pane portion of the prune: restored
// file-pane references are seeded synchronously at mount and can be pruned
// immediately, but restored terminal-pane references must survive until this
// root's `listTerminals` call has resolved at least once (terminal listing is
// async, per Phase 4), otherwise the not-yet-loaded race would look identical
// to a genuinely-gone terminal and get permanently stripped. While
// `terminalsReady` is false, any `terminal:`-prefixed pane id already present
// in `orderForRoot` is treated as live (not pruned) even if absent from
// `livePaneIds`; once `terminalsReady` is true, terminal pane ids are pruned
// exactly like any other pane id.
export function revalidateWorkbenchLayoutForRoot(
  groups: ReadonlyArray<{ id: string; label: string }>,
  orderForRoot: WorkbenchPaneOrder,
  activePaneByGroup: WorkbenchActivePaneState,
  livePaneIds: ReadonlySet<string>,
  terminalsReady: boolean,
): {
  prunedOrder: Record<string, string[]>;
  reconciledActivePane: Record<string, string>;
} {
  const effectiveLivePaneIds = terminalsReady
    ? livePaneIds
    : new Set<string>([
        ...livePaneIds,
        ...Object.values(orderForRoot)
          .flat()
          .filter((paneId) => isTerminalPaneId(paneId)),
      ]);
  const prunedOrder = pruneWorkbenchLayoutOrder(
    orderForRoot,
    effectiveLivePaneIds,
  );
  const groupsWithPanes = groups.map((group) => ({
    id: group.id,
    panes: (prunedOrder[group.id] ?? []).map((paneId) => ({ id: paneId })),
  }));
  const reconciledActivePane = reconcileActiveWorkbenchPanes(
    groupsWithPanes,
    activePaneByGroup,
    activePaneByGroup,
  );
  return { prunedOrder, reconciledActivePane };
}

// CONTRACT: pure pane-order-merge transformation extracted out of
// `WorkbenchShell`'s revalidation effect in App.tsx (Phase 2 review
// Test-partition finding,
// 260707-bug-dashboard-e2e-multi-root-locator-leakage), mirroring
// `revalidateWorkbenchLayoutForRoot` above and `mergeWorkbenchLayoutRestoreEntries`
// (Phase 7) below.
//
// `orderForRoot` (the caller's `paneOrderByRoot[rootKey]`) only tracks
// agent/activity pane order for a root; a dockview group can also host
// readonly-file panes and terminal panes, whose order lives separately in the
// flat, cross-root `readOnlyFilePaneOrderByGroup` / `terminalPaneOrderByGroup`
// maps. Without merging those in, a group whose live panes are only
// readonly-file/terminal panes looks pane-less to
// `revalidateWorkbenchLayoutForRoot`, and `reconcileActiveWorkbenchPanes`
// drops that group's active-pane entry entirely (the actual mechanism behind
// the ticket's Phase 2 bug). Each source is filtered to its own live-id set
// before merging so a stale/closed readonly or terminal pane id already
// sitting in one of the flat order maps is never resurrected into a live
// group's pane list.
export function mergeReadOnlyAndTerminalPaneOrder(
  groups: ReadonlyArray<{ id: string }>,
  orderForRoot: WorkbenchPaneOrder,
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder,
  liveReadOnlyPaneIds: ReadonlySet<string>,
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
  liveTerminalPaneIds: ReadonlySet<string>,
): WorkbenchPaneOrder {
  return Object.fromEntries(
    groups.map((group) => [
      group.id,
      [
        ...(orderForRoot[group.id] ?? []),
        ...(readOnlyFilePaneOrderByGroup[group.id] ?? []).filter((paneId) =>
          liveReadOnlyPaneIds.has(paneId),
        ),
        ...(terminalPaneOrderByGroup[group.id] ?? []).filter((paneId) =>
          liveTerminalPaneIds.has(paneId),
        ),
      ],
    ]),
  );
}

// CONTRACT: pure helper, moved here from App.tsx (Phase 2 review
// Test-partition finding) so it is unit-testable alongside the other
// pane-order transformations in this file. Drops every pane id in `paneIds`
// from every group's order — used both by the revalidation effect's
// persisted `paneOrderByRoot` write (which must stay agnostic to
// readonly-file/terminal pane order, owned separately by the maps above) and
// by call sites elsewhere in App.tsx that need to drop a closed/removed pane
// id from every group at once.
export function removePanesFromOrder(
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

// CONTRACT: pure merge/clobber-fix transformation extracted out of
// `WorkbenchShell`'s layout save effect in App.tsx (Phase 7 review
// Test-partition finding), mirroring `revalidateWorkbenchLayoutForRoot`
// (Phase 5) and `resolveTerminalMountWrite` (Phase 6) — this is the highest-
// restore-correctness-risk glue logic in the phase (the clobber-bug fix
// itself), so it must be unit-testable without a React harness.
//
// Computes the save effect's persisted entry set as the union of (a) live
// entries built from the currently-open roots' in-memory state and (b)
// untouched entries carried over from `priorSnapshot` for every rootKey NOT
// currently open, then rebuilds a fresh snapshot object (keyed via
// `workbenchLayoutRestoreRootKey`) from that same merged list so the caller
// can write it back into its live ref. Passing the effect's own just-written
// `priorSnapshot` back in on the next run (rather than a frozen mount-time
// value) is what fixes the clobber bug: a just-closed root's untouched-entry
// fallback then reflects its last live state instead of a stale snapshot.
export function mergeWorkbenchLayoutRestoreEntries(
  openRootKeys: readonly string[],
  openRootRefs: Record<string, { rootId: string; serverRoute: string }>,
  groupsByRoot: Record<string, ReadonlyArray<{ id: string; label: string }>>,
  paneOrderByRoot: Record<string, WorkbenchPaneOrder>,
  activePaneByRoot: Record<string, WorkbenchActivePaneState>,
  groupSizeByRoot: Record<string, Record<string, WorkbenchLayoutGroupSize>>,
  priorSnapshot: WorkbenchLayoutRestoreSnapshot,
): {
  mergedEntries: WorkbenchLayoutRestoreEntry[];
  mergedSnapshot: WorkbenchLayoutRestoreSnapshot;
} {
  const openRootKeysSet = new Set(openRootKeys);
  const liveEntries: WorkbenchLayoutRestoreEntry[] = openRootKeys.flatMap(
    (rootKey) => {
      const ref = openRootRefs[rootKey];
      const groups = groupsByRoot[rootKey];
      if (!ref || !groups) {
        return [];
      }
      const groupSizeById = groupSizeByRoot[rootKey];
      return [
        {
          serverRoute: ref.serverRoute,
          workRootId: ref.rootId,
          groups,
          paneOrderByGroup: paneOrderByRoot[rootKey] ?? {},
          activePaneByGroup: activePaneByRoot[rootKey] ?? {},
          ...(groupSizeById ? { groupSizeById } : {}),
        },
      ];
    },
  );
  const untouchedEntries = Object.entries(priorSnapshot)
    .filter(([rootKey]) => !openRootKeysSet.has(rootKey))
    .map(([, restoredEntry]) => restoredEntry);
  const mergedEntries = [...liveEntries, ...untouchedEntries];
  const mergedSnapshot: WorkbenchLayoutRestoreSnapshot = {};
  for (const entry of mergedEntries) {
    mergedSnapshot[workbenchLayoutRestoreRootKey(entry)] = entry;
  }
  return { mergedEntries, mergedSnapshot };
}

// CONTRACT: pure render-time layout resolver (260714 active-root derivation
// refactor Phase 1, D7). Extracted from `WorkbenchShell`'s per-root layout
// reads so a freshly-mounted root (mounted by the D1 render-time union on the
// same render its selection resolves) shows its restored layout on that same
// render, closing the one-render "flash to empty layout" the async seeding
// effects (App.tsx groups/paneOrder + activePane seeds) would otherwise leave
// open. Follows the "extracted from a `WorkbenchShell` effect for
// unit-testability" precedent of the transforms above.
//
// Precedence per field is live-state -> restore-snapshot -> caller default:
// the live per-root map wins whenever it has an entry for `rootKey`, else the
// `restoreSnapshot[rootKey]` entry's corresponding field, else `null`/`{}` for
// the caller to apply its own ultimate default (e.g. `?? initialWorkbenchGroups`
// at the groups read site, which owns the module-local default and is not
// duplicated here).
export function resolveRootLayout(
  rootKey: string,
  workbenchGroupsByRoot: Record<
    string,
    ReadonlyArray<{ id: string; label: string }>
  >,
  paneOrderByRoot: Record<string, WorkbenchPaneOrder>,
  activePaneByRoot: Record<string, WorkbenchActivePaneState>,
  restoreSnapshot: WorkbenchLayoutRestoreSnapshot,
): {
  groups: ReadonlyArray<{ id: string; label: string }> | null;
  paneOrderByGroup: WorkbenchPaneOrder;
  activePaneByGroup: WorkbenchActivePaneState;
} {
  const restored = restoreSnapshot[rootKey];
  return {
    groups: workbenchGroupsByRoot[rootKey] ?? restored?.groups ?? null,
    paneOrderByGroup:
      paneOrderByRoot[rootKey] ?? restored?.paneOrderByGroup ?? {},
    activePaneByGroup:
      activePaneByRoot[rootKey] ?? restored?.activePaneByGroup ?? {},
  };
}

function parseWorkbenchLayoutRestoreEntry(
  value: unknown,
): WorkbenchLayoutRestoreEntry | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const record = value as Record<string, unknown>;
  const workRootId =
    typeof record.workRootId === "string" ? record.workRootId.trim() : "";
  if (!workRootId) {
    return null;
  }
  const serverRoute =
    typeof record.serverRoute === "string" && record.serverRoute.trim()
      ? record.serverRoute.trim()
      : LOCAL_DASHBOARD_SERVER_ROUTE;
  const groups = parseWorkbenchLayoutGroups(record.groups);
  const groupIds = new Set(groups.map((group) => group.id));
  const paneOrderByGroup = parseWorkbenchPaneOrderByGroup(
    record.paneOrderByGroup,
    groupIds,
  );
  const activePaneByGroup = parseWorkbenchActivePaneByGroup(
    record.activePaneByGroup,
    groupIds,
  );
  const groupSizeById = parseWorkbenchGroupSizeById(
    record.groupSizeById,
    groupIds,
  );
  return {
    serverRoute,
    workRootId,
    groups,
    paneOrderByGroup,
    activePaneByGroup,
    ...(groupSizeById ? { groupSizeById } : {}),
  };
}

function parseWorkbenchLayoutGroups(
  value: unknown,
): Array<{ id: string; label: string }> {
  if (!Array.isArray(value)) {
    return [];
  }
  const groups: Array<{ id: string; label: string }> = [];
  for (const entry of value) {
    if (!entry || typeof entry !== "object") {
      continue;
    }
    const record = entry as Record<string, unknown>;
    if (typeof record.id !== "string" || !record.id) {
      continue;
    }
    groups.push({
      id: record.id,
      label: typeof record.label === "string" ? record.label : record.id,
    });
  }
  return groups;
}

function parseWorkbenchPaneOrderByGroup(
  value: unknown,
  groupIds: ReadonlySet<string>,
): Record<string, string[]> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const order: Record<string, string[]> = {};
  for (const [groupId, paneOrder] of Object.entries(
    value as Record<string, unknown>,
  )) {
    if (!groupIds.has(groupId) || !Array.isArray(paneOrder)) {
      continue;
    }
    const ids = paneOrder.filter(
      (paneId): paneId is string => typeof paneId === "string",
    );
    if (ids.length > 0) {
      order[groupId] = [...new Set(ids)];
    }
  }
  return order;
}

function parseWorkbenchActivePaneByGroup(
  value: unknown,
  groupIds: ReadonlySet<string>,
): Record<string, string> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const active: Record<string, string> = {};
  for (const [groupId, paneId] of Object.entries(
    value as Record<string, unknown>,
  )) {
    if (groupIds.has(groupId) && typeof paneId === "string" && paneId) {
      active[groupId] = paneId;
    }
  }
  return active;
}

function parseWorkbenchGroupSizeById(
  value: unknown,
  groupIds: ReadonlySet<string>,
): Record<string, WorkbenchLayoutGroupSize> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  const sizeById: Record<string, WorkbenchLayoutGroupSize> = {};
  for (const [groupId, size] of Object.entries(
    value as Record<string, unknown>,
  )) {
    if (!groupIds.has(groupId) || !size || typeof size !== "object") {
      continue;
    }
    const record = size as Record<string, unknown>;
    const width = typeof record.width === "number" ? record.width : undefined;
    const height =
      typeof record.height === "number" ? record.height : undefined;
    if (width !== undefined || height !== undefined) {
      sizeById[groupId] = {
        ...(width !== undefined ? { width } : {}),
        ...(height !== undefined ? { height } : {}),
      };
    }
  }
  return Object.keys(sizeById).length > 0 ? sizeById : undefined;
}
