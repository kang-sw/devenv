import { isMarkdownDocumentSource } from "../documentViewer.js";
import { ReadOnlyDocumentPane } from "../readOnlyDocumentPane.js";
import type { DashboardCommandDispatcher } from "../commands.js";
import type { ViewState, WorkRootView } from "../resourceModel.js";
import type { ReadOnlyFilePane } from "../workRootFiles.js";
import type { WorkbenchPane } from "./editorGroups.js";
import type { WorkbenchPaneOrder } from "./editorGroupModel.js";
import { readOnlyFilePaneRevision } from "./readOnlyFilePlacement.js";

export function readOnlyWorkbenchPanesByGroup(
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

export function readOnlyWorkbenchPane(
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
