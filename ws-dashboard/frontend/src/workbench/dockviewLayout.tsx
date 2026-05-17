import { useCallback, useEffect, useMemo, useRef } from "react";
import type { ReactNode } from "react";
import {
  DockviewReact,
  type DockviewApi,
  type DockviewReadyEvent,
  type IDockviewPanel,
  type IDockviewPanelHeaderProps,
  type IDockviewPanelProps,
} from "dockview";
import "dockview/dist/styles/dockview.css";
import { dockviewBridgeOptions, type WorkbenchDockviewPanelParams } from "./dockviewBridge.js";
import type { WorkbenchPaneCategory } from "./editorGroupModel.js";
import { defaultSurfaceRegistry, type SurfaceKind } from "./surfaceRegistry.js";

export type DockviewWorkbenchPane = {
  readonly id: string;
  readonly kind: SurfaceKind;
  readonly category: WorkbenchPaneCategory;
  readonly title: string;
  readonly detail: string;
  readonly meta: readonly string[];
  readonly body?: ReactNode;
};

export type DockviewWorkbenchGroup = {
  readonly id: string;
  readonly label: string;
  readonly panes: readonly DockviewWorkbenchPane[];
};

export type DockviewWorkbenchLayoutProps = {
  readonly groups: readonly DockviewWorkbenchGroup[];
  readonly activePaneByGroup: Readonly<Record<string, string>>;
  readonly onSelectPane: (groupId: string, paneId: string) => void;
  readonly onMovePane: (paneId: string, targetGroupId: string, beforePaneId?: string) => void;
};

type DockviewWorkbenchPanelParams = WorkbenchDockviewPanelParams & {
  readonly [key: string]: unknown;
  readonly groupId: string;
  readonly groupLabel: string;
  readonly paneId: string;
  readonly category: WorkbenchPaneCategory;
  readonly title: string;
  readonly detail: string;
  readonly meta: readonly string[];
  readonly body?: ReactNode;
};

const workbenchDockviewComponent = "workbenchPane";
const workbenchDockviewTabComponent = "workbenchTab";

export function DockviewWorkbenchLayout({
  groups,
  activePaneByGroup,
  onMovePane,
  onSelectPane,
}: DockviewWorkbenchLayoutProps) {
  const apiRef = useRef<DockviewApi | null>(null);
  const syncingRef = useRef(false);
  const callbacksRef = useRef({ onMovePane, onSelectPane });
  callbacksRef.current = { onMovePane, onSelectPane };

  const components = useMemo(
    () => ({
      [workbenchDockviewComponent]: DockviewWorkbenchPanel,
    }),
    [],
  );
  const tabComponents = useMemo(
    () => ({
      [workbenchDockviewTabComponent]: DockviewWorkbenchTab,
    }),
    [],
  );

  const syncPanels = useCallback(() => {
    if (!apiRef.current) {
      return;
    }
    syncingRef.current = true;
    try {
      syncDockviewWorkbench(apiRef.current, groups, activePaneByGroup);
    } finally {
      queueMicrotask(() => {
        syncingRef.current = false;
      });
    }
  }, [activePaneByGroup, groups]);

  const handleReady = useCallback(
    (event: DockviewReadyEvent) => {
      apiRef.current = event.api;
      // CONTRACT: Dashboard policy remains outside Dockview. Dockview events are
      // reduced to dashboard pane/group ids before invoking product callbacks;
      // raw Dockview panel/group handles must not escape this adapter.
      event.api.onDidActivePanelChange((panel) => {
        if (syncingRef.current) {
          return;
        }
        const params = panel?.params as DockviewWorkbenchPanelParams | undefined;
        if (params) {
          callbacksRef.current.onSelectPane(params.groupId, params.paneId);
        }
      });
      event.api.onDidMovePanel((move) => {
        if (syncingRef.current) {
          return;
        }
        const params = move.panel.params as DockviewWorkbenchPanelParams | undefined;
        if (params) {
          callbacksRef.current.onMovePane(params.paneId, params.groupId);
        }
      });
      syncPanels();
    },
    [syncPanels],
  );

  useEffect(syncPanels, [syncPanels]);

  return (
    <div
      className="dockview-workbench-layout dockview-theme-dark"
      data-workbench-layout-owner="dockview"
      role="presentation"
    >
      {/* CONTRACT: This component is the only visible workbench split/tab/pane
          layout owner. DockviewReact is mounted under this stable owner marker
          so browser acceptance can distinguish it from the retired custom
          `.workbench-splits > .workbench-group` shell. */}
      <DockviewReact
        components={components}
        defaultTabComponent={DockviewWorkbenchTab}
        disableFloatingGroups={dockviewBridgeOptions.disableFloatingGroups}
        noPanelsOverlay="emptyGroup"
        onReady={handleReady}
        tabComponents={tabComponents}
      />
    </div>
  );
}

function DockviewWorkbenchPanel({ params }: IDockviewPanelProps<DockviewWorkbenchPanelParams>) {
  const registry = defaultSurfaceRegistry()[params.surfaceKind];

  return (
    <article
      aria-label={`${registry.label}: ${params.title}`}
      className="workbench-pane dockview-workbench-pane"
      data-surface-kind={params.surfaceKind}
      data-workbench-group-id={params.groupId}
      data-workbench-pane-id={params.paneId}
      role="tabpanel"
    >
      <div className="workbench-pane-body">
        <p>{params.detail}</p>
        {params.body ? <div className="workbench-pane-content">{params.body}</div> : null}
      </div>
    </article>
  );
}

function DockviewWorkbenchTab({
  api,
  containerApi,
  params,
}: IDockviewPanelHeaderProps<DockviewWorkbenchPanelParams>) {
  const registry = defaultSurfaceRegistry()[params.surfaceKind];
  const selected = containerApi.activePanel?.id === params.paneId;

  return (
    <div
      aria-selected={selected}
      className={`dockview-workbench-tab ${selected ? "dockview-workbench-tab-active" : ""}`}
      data-workbench-group-id={params.groupId}
      data-workbench-pane-id={params.paneId}
      role="tab"
      title={api.title ?? params.title}
    >
      <span className="workbench-tab-kind">{registry.label}</span>
      <span className="workbench-tab-title">{api.title ?? params.title}</span>
    </div>
  );
}

function syncDockviewWorkbench(
  api: DockviewApi,
  groups: readonly DockviewWorkbenchGroup[],
  activePaneByGroup: Readonly<Record<string, string>>,
) {
  // Stub-only synchronization: rebuild the Dockview surface from dashboard
  // groups/panes so the visible owner is Dockview while the full incremental
  // reconciliation policy remains a future implementation concern.
  api.clear();

  const dockGroupByWorkbenchGroup = new Map<string, string>();
  let firstDockGroupId: string | null = null;

  for (const group of groups) {
    const activePaneId = activePaneByGroup[group.id] ?? group.panes[0]?.id;
    for (const [index, pane] of group.panes.entries()) {
      const existingDockGroupId = dockGroupByWorkbenchGroup.get(group.id);
      const panel: IDockviewPanel = api.addPanel<DockviewWorkbenchPanelParams>({
        id: pane.id,
        component: workbenchDockviewComponent,
        tabComponent: workbenchDockviewTabComponent,
        title: pane.title,
        params: toDockviewWorkbenchPanelParams(group, pane),
        inactive: pane.id !== activePaneId,
        ...(existingDockGroupId
          ? { position: { referenceGroup: existingDockGroupId, direction: "within", index } }
          : firstDockGroupId
            ? { position: { referenceGroup: firstDockGroupId, direction: "right" } }
            : {}),
      });

      dockGroupByWorkbenchGroup.set(group.id, panel.group.id);
      firstDockGroupId ??= panel.group.id;
    }
  }
}

function toDockviewWorkbenchPanelParams(
  group: DockviewWorkbenchGroup,
  pane: DockviewWorkbenchPane,
): DockviewWorkbenchPanelParams {
  return {
    attachmentId: pane.id,
    surfaceKind: pane.kind,
    groupId: group.id,
    groupLabel: group.label,
    paneId: pane.id,
    category: pane.category,
    title: pane.title,
    detail: pane.detail,
    meta: pane.meta,
    body: pane.body,
  };
}
