// Pure browser-local ordering logic for SIBLING work-root nav entries
// (workspace rows under a server; worktree rows under a workspace).
//
// CONTRACT: this module is frontend/browser-local nav display-order state
// only. It never touches the daemon-side `OpenedWorkRoots` /
// `opened-workroots.json` registry (see
// `ai-docs/tickets/.done/260523-feat-ws-dashboard-persist-open-workroots.md`),
// which is an unrelated Rust-side "which roots are opened" store. Ids here
// are the same `workspace.id` / `WorkRootView.id` the resource model already
// exposes, deterministically re-derived from canonicalized paths on every
// daemon load (not stored server-side), so a persisted order keyed by them
// stays stable across reloads as long as the underlying path/workspace
// doesn't change, and simply goes stale/ignored (falls back to natural
// order) if it does.
//
// Shape and persistence mirror the `paneOrderByRoot` /
// `workbench/layoutRestore.ts` precedent: an id-order map applied at render
// time over server-supplied arrays (never mutating the source data), with a
// versioned localStorage blob and a defensive parser that silently drops
// anything malformed/mismatched-version rather than throwing.

import { browserStorage } from "./workRootFiles.js";

export type WorkNavSiblingOrder = {
  readonly workspaceOrderByServer: Readonly<Record<string, readonly string[]>>;
  readonly worktreeOrderByWorkspace: Readonly<
    Record<string, readonly string[]>
  >;
};

export const emptyWorkNavSiblingOrder: WorkNavSiblingOrder = {
  workspaceOrderByServer: {},
  worktreeOrderByWorkspace: {},
};

export const workNavSiblingDragMimeType = "application/x-ws-worknav-sibling";

export type WorkNavSiblingDragPayload = {
  readonly sourceId: string;
  readonly scopeKey: string;
};

// The single accept/reject decision for a sibling-reorder drag-over/drop:
// enforces the ticket's central non-goal ("NOT re-parenting") by requiring
// the in-flight drag to belong to the same sibling scope as the candidate
// drop target, and to not be a drop onto itself. Pure so both the
// `dragover` (gates `preventDefault`) and `drop` (gates whether
// `onSiblingReorder` fires) call sites in App.tsx can share one definition
// instead of hand-duplicating the De Morgan dual of this boolean.
export function isAcceptableSiblingDrop(
  dragged: WorkNavSiblingDragPayload | null,
  targetScopeKey: string,
  targetId: string,
): boolean {
  return (
    !!dragged &&
    dragged.scopeKey === targetScopeKey &&
    dragged.sourceId !== targetId
  );
}

// Applies a persisted id-order over a server-supplied sibling list without
// mutating or reordering the source array in place: ids named in `order`
// come first (in `order`'s sequence, skipping any id no longer present in
// `items` — mirrors `applyWorkbenchPaneOrder`'s dangling-id handling), then
// any remaining `items` not named in `order` are appended in their original
// (server-supplied) order. An `undefined`/empty `order` returns `items`
// unchanged — this is the "don't break existing ordering when no custom
// order is set" guarantee, and is also the back-compat path for a store
// written before this feature existed.
export function applySiblingOrder<T extends { id: string }>(
  items: readonly T[],
  order: readonly string[] | undefined,
): T[] {
  if (!order || order.length === 0) {
    return items.slice();
  }
  const itemById = new Map(items.map((item) => [item.id, item]));
  const consumed = new Set<string>();
  const ordered: T[] = [];
  for (const id of order) {
    const item = itemById.get(id);
    if (item && !consumed.has(id)) {
      consumed.add(id);
      ordered.push(item);
    }
  }
  for (const item of items) {
    if (!consumed.has(item.id)) {
      ordered.push(item);
    }
  }
  return ordered;
}

// Pure move-within-flat-list helper. Caller passes the *effective* (already
// `applySiblingOrder`'d) id list so the returned array is always the full
// current sibling set in the new order, including any ids that weren't yet
// in the persisted map. Removes `sourceId`, then splices it back in
// immediately before `beforeId` (or appends at the end if `beforeId` is
// undefined/not found/equal to `sourceId`).
export function reorderSiblingIds(
  effectiveOrder: readonly string[],
  sourceId: string,
  beforeId: string | undefined,
): string[] {
  if (sourceId === beforeId) {
    return effectiveOrder.slice();
  }
  const withoutSource = effectiveOrder.filter((id) => id !== sourceId);
  if (!effectiveOrder.includes(sourceId)) {
    return withoutSource;
  }
  const insertAt = beforeId
    ? withoutSource.findIndex((id) => id === beforeId)
    : -1;
  const result = withoutSource.slice();
  result.splice(insertAt >= 0 ? insertAt : result.length, 0, sourceId);
  return result;
}

const workNavOrderStorageKey = "ws-dashboard.workNavOrder.v1";

export function loadWorkNavOrderSnapshot(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): WorkNavSiblingOrder {
  if (!storage) {
    return emptyWorkNavSiblingOrder;
  }
  try {
    const raw = storage.getItem(workNavOrderStorageKey);
    if (!raw) {
      return emptyWorkNavSiblingOrder;
    }
    const parsed = JSON.parse(raw) as {
      version?: unknown;
      workspaceOrderByServer?: unknown;
      worktreeOrderByWorkspace?: unknown;
    };
    if (parsed.version !== 1) {
      return emptyWorkNavSiblingOrder;
    }
    return {
      workspaceOrderByServer: parseOrderByScope(parsed.workspaceOrderByServer),
      worktreeOrderByWorkspace: parseOrderByScope(
        parsed.worktreeOrderByWorkspace,
      ),
    };
  } catch {
    return emptyWorkNavSiblingOrder;
  }
}

export function saveWorkNavOrderSnapshot(
  order: WorkNavSiblingOrder,
  storage: Pick<Storage, "setItem" | "removeItem"> | null = browserStorage(),
) {
  if (!storage) {
    return;
  }
  try {
    const hasWorkspaceOrder =
      Object.keys(order.workspaceOrderByServer).length > 0;
    const hasWorktreeOrder =
      Object.keys(order.worktreeOrderByWorkspace).length > 0;
    if (!hasWorkspaceOrder && !hasWorktreeOrder) {
      storage.removeItem(workNavOrderStorageKey);
      return;
    }
    storage.setItem(
      workNavOrderStorageKey,
      JSON.stringify({
        version: 1,
        workspaceOrderByServer: order.workspaceOrderByServer,
        worktreeOrderByWorkspace: order.worktreeOrderByWorkspace,
      }),
    );
  } catch {
    // Browser persistence is best-effort; in-memory order remains canonical
    // for the current session.
  }
}

function parseOrderByScope(value: unknown): Record<string, readonly string[]> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const order: Record<string, readonly string[]> = {};
  for (const [scopeKey, ids] of Object.entries(
    value as Record<string, unknown>,
  )) {
    if (!Array.isArray(ids)) {
      continue;
    }
    const stringIds = ids.filter(
      (id): id is string => typeof id === "string" && id.length > 0,
    );
    if (stringIds.length > 0) {
      order[scopeKey] = [...new Set(stringIds)];
    }
  }
  return order;
}
