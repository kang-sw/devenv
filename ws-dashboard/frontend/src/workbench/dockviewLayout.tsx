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
import {
  dockviewBridgeOptions,
  type WorkbenchDockviewPanelParams,
} from "./dockviewBridge.js";
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
  readonly onRequestClosePane?: (request: DockviewTabCloseRequest) => void;
};

export type DockviewWorkbenchGroup = {
  readonly id: string;
  readonly label: string;
  readonly panes: readonly DockviewWorkbenchPane[];
};

export type DockviewTabCloseRequest = {
  readonly groupId: string;
  readonly paneId: string;
  readonly surfaceKind: SurfaceKind;
  readonly clientX: number;
  readonly clientY: number;
};

export type DockviewWorkbenchLayoutProps = {
  readonly groups: readonly DockviewWorkbenchGroup[];
  readonly activePaneByGroup: Readonly<Record<string, string>>;
  readonly onSelectPane: (groupId: string, paneId: string) => void;
  readonly onRequestClosePane?: (request: DockviewTabCloseRequest) => void;
  readonly onMovePane: (
    paneId: string,
    targetGroupId: string,
    beforePaneId?: string,
    dynamicTargetGroup?: {
      readonly targetGroupId: string;
      readonly targetGroupLabel?: string;
    },
  ) => void;
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
  readonly onRequestClosePane?: (request: DockviewTabCloseRequest) => void;
};

const workbenchDockviewComponent = "workbenchPane";
const workbenchDockviewTabComponent = "workbenchTab";

export type DockviewTabCategoryPresentation =
  | "dockview-category-chip"
  | "pinned-left-badge-fallback";

export function dockviewTabCategoryPresentation(
  category: WorkbenchPaneCategory,
): DockviewTabCategoryPresentation {
  return category === "pinned"
    ? "pinned-left-badge-fallback"
    : "dockview-category-chip";
}

export function DockviewWorkbenchLayout({
  groups,
  activePaneByGroup,
  onMovePane,
  onRequestClosePane,
  onSelectPane,
}: DockviewWorkbenchLayoutProps) {
  const apiRef = useRef<DockviewApi | null>(null);
  const syncingRef = useRef(false);
  const dockGroupToWorkbenchGroupRef = useRef<ReadonlyMap<string, string>>(
    new Map(),
  );
  const callbacksRef = useRef({ onMovePane, onRequestClosePane, onSelectPane });
  callbacksRef.current = { onMovePane, onRequestClosePane, onSelectPane };

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
      dockGroupToWorkbenchGroupRef.current = syncDockviewWorkbench(
        apiRef.current,
        groups,
        activePaneByGroup,
        callbacksRef.current.onRequestClosePane,
      );
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
        if (!panel) {
          return;
        }
        const params = panel.params as DockviewWorkbenchPanelParams | undefined;
        if (params) {
          callbacksRef.current.onSelectPane(
            dockGroupToWorkbenchGroupRef.current.get(panel.group.id) ??
              params.groupId,
            params.paneId,
          );
        }
      });
      event.api.onDidMovePanel((move) => {
        if (syncingRef.current) {
          return;
        }
        const params = move.panel.params as
          | DockviewWorkbenchPanelParams
          | undefined;
        const mappedTargetGroupId = dockGroupToWorkbenchGroupRef.current.get(
          move.panel.group.id,
        );
        if (!params) {
          return;
        }
        const targetGroupId =
          mappedTargetGroupId ??
          nextDynamicWorkbenchGroupId(dockGroupToWorkbenchGroupRef.current);
        const dynamicTargetGroup = mappedTargetGroupId
          ? undefined
          : {
              targetGroupId,
              targetGroupLabel: `group ${Number(targetGroupId.replace(/^group-/, "")) || dockGroupToWorkbenchGroupRef.current.size + 1}`,
            };
        const siblingAfterMoved = move.panel.group.panels
          .slice(
            move.panel.group.panels.findIndex(
              (panel) => panel.id === move.panel.id,
            ) + 1,
          )
          .map(
            (panel) => panel.params as DockviewWorkbenchPanelParams | undefined,
          )
          .find((panelParams) => panelParams?.paneId);
        callbacksRef.current.onMovePane(
          params.paneId,
          targetGroupId,
          siblingAfterMoved?.paneId,
          dynamicTargetGroup,
        );
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

function DockviewWorkbenchPanel({
  params,
}: IDockviewPanelProps<DockviewWorkbenchPanelParams>) {
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
        {params.body ? (
          <div className="workbench-pane-content">{params.body}</div>
        ) : null}
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
      data-workbench-pane-category={params.category}
      data-workbench-tab-category-presentation={dockviewTabCategoryPresentation(
        params.category,
      )}
      data-workbench-tab-close-affordance="hover-only"
      data-workbench-close-confirmation={registry.closeConfirmationPolicy}
      data-workbench-group-id={params.groupId}
      data-workbench-pane-id={params.paneId}
      role="tab"
      title={api.title ?? params.title}
    >
      {/* Dockview owns one deterministic tab strip per group in this slice.
          Dashboard row policy is retained as pane category metadata instead
          of rendering the retired two-row pinned/opened custom header. */}
      {/* CONTRACT: Hover-only close UI belongs in this Dockview tab component.
          Close clicks must call the dashboard callback with pane identity and
          pointer coordinates so App policy can decide immediate close versus
          cursor-near session confirmation without exposing Dockview handles. */}
      <span className="workbench-tab-kind">{registry.label}</span>
      <span className="workbench-tab-title">{api.title ?? params.title}</span>
      <button
        aria-label={`Close ${api.title ?? params.title}`}
        className="workbench-tab-close"
        data-command-id="workbench.tab.close"
        tabIndex={-1}
        type="button"
        onClick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          params.onRequestClosePane?.({
            groupId: params.groupId,
            paneId: params.paneId,
            surfaceKind: params.surfaceKind,
            clientX: event.clientX,
            clientY: event.clientY,
          });
        }}
      >
        ×
      </button>
    </div>
  );
}

function syncDockviewWorkbench(
  api: DockviewApi,
  groups: readonly DockviewWorkbenchGroup[],
  activePaneByGroup: Readonly<Record<string, string>>,
  onRequestClosePane?: (request: DockviewTabCloseRequest) => void,
): ReadonlyMap<string, string> {
  const desiredPaneIds = new Set(
    groups.flatMap((group) => group.panes.map((pane) => pane.id)),
  );
  for (const panel of [...api.panels]) {
    if (!desiredPaneIds.has(panel.id)) {
      api.removePanel(panel);
    }
  }

  const dockGroupByWorkbenchGroup = new Map<string, IDockviewPanel["group"]>();
  for (const group of groups) {
    const existingPanel = group.panes
      .map((pane) => api.getPanel(pane.id))
      .find((panel): panel is IDockviewPanel => Boolean(panel));
    if (existingPanel) {
      dockGroupByWorkbenchGroup.set(group.id, existingPanel.group);
    }
  }

  let firstDockGroup = api.groups[0] ?? null;

  for (const [groupIndex, group] of groups.entries()) {
    let targetDockGroup = dockGroupByWorkbenchGroup.get(group.id) ?? null;
    const activePaneId = activePaneByGroup[group.id] ?? group.panes[0]?.id;

    for (const [index, pane] of group.panes.entries()) {
      const params = toDockviewWorkbenchPanelParams(group, pane, onRequestClosePane);
      const existingPanel = api.getPanel(pane.id);

      if (!existingPanel) {
        const panel = api.addPanel<DockviewWorkbenchPanelParams>({
          id: pane.id,
          component: workbenchDockviewComponent,
          tabComponent: workbenchDockviewTabComponent,
          title: pane.title,
          params,
          inactive: pane.id !== activePaneId,
          ...(targetDockGroup
            ? {
                position: {
                  referenceGroup: targetDockGroup,
                  direction: "within" as const,
                  index,
                },
              }
            : firstDockGroup
              ? {
                  position: {
                    referenceGroup: firstDockGroup,
                    direction:
                      groupIndex === 0 && dockGroupByWorkbenchGroup.size > 0
                        ? ("left" as const)
                        : groupIndex === 0
                          ? ("within" as const)
                          : ("right" as const),
                    index,
                  },
                }
              : {}),
        });
        targetDockGroup = panel.group;
        dockGroupByWorkbenchGroup.set(group.id, panel.group);
        firstDockGroup ??= panel.group;
        continue;
      }

      existingPanel.setTitle(pane.title);
      const currentParams = existingPanel.params as
        | DockviewWorkbenchPanelParams
        | undefined;
      if (shouldUpdateDockviewWorkbenchPanelParams(currentParams, params)) {
        existingPanel.api.updateParameters(params);
      }

      if (targetDockGroup && existingPanel.group.id !== targetDockGroup.id) {
        existingPanel.api.moveTo({
          group: targetDockGroup,
          index,
          skipSetActive: true,
        });
      } else if (targetDockGroup) {
        const currentIndex = targetDockGroup.panels.findIndex(
          (panel) => panel.id === pane.id,
        );
        if (currentIndex !== index) {
          existingPanel.api.moveTo({
            group: targetDockGroup,
            index,
            skipSetActive: true,
          });
        }
      } else {
        targetDockGroup = existingPanel.group;
        dockGroupByWorkbenchGroup.set(group.id, existingPanel.group);
        firstDockGroup ??= existingPanel.group;
      }

      if (pane.id === activePaneId && !existingPanel.api.isActive) {
        existingPanel.api.setActive();
      }
    }
  }

  const workbenchGroupByDockGroup = new Map<string, string>();
  for (const [workbenchGroupId, dockGroup] of dockGroupByWorkbenchGroup) {
    workbenchGroupByDockGroup.set(dockGroup.id, workbenchGroupId);
  }
  return workbenchGroupByDockGroup;
}

function shouldUpdateDockviewWorkbenchPanelParams(
  current: DockviewWorkbenchPanelParams | undefined,
  next: DockviewWorkbenchPanelParams,
) {
  if (!current) {
    return true;
  }
  if (
    current.groupId !== next.groupId ||
    current.groupLabel !== next.groupLabel ||
    current.paneId !== next.paneId ||
    current.category !== next.category ||
    current.surfaceKind !== next.surfaceKind ||
    current.title !== next.title ||
    current.detail !== next.detail
  ) {
    return true;
  }
  // Connected terminals stream directly into their mounted xterm instance.
  // Avoid Dockview parameter churn for output/socket metadata so ordinary
  // command output does not blur the emulator between keystrokes.
  if (next.surfaceKind === "persistentTerminal") {
    const socketStatus = next.meta[1];
    return socketStatus !== "connecting" && socketStatus !== "connected";
  }
  return (
    current.body !== next.body ||
    current.meta.join("\0") !== next.meta.join("\0")
  );
}

function toDockviewWorkbenchPanelParams(
  group: DockviewWorkbenchGroup,
  pane: DockviewWorkbenchPane,
  onRequestClosePane?: (request: DockviewTabCloseRequest) => void,
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
    onRequestClosePane,
  };
}

function nextDynamicWorkbenchGroupId(
  current: ReadonlyMap<string, string>,
): string {
  const used = new Set(current.values());
  for (let index = used.size + 1; ; index += 1) {
    const candidate = `group-${index}`;
    if (!used.has(candidate)) {
      return candidate;
    }
  }
}
