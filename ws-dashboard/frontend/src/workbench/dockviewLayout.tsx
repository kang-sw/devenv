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
import {
  dockviewPanelIsSelectedWithinGroup,
  shouldUpdateDockviewWorkbenchPanelParams,
} from "./dockviewLayoutModel.js";
import type { AgentAttentionState } from "../agentAttention.js";
import type { WorkbenchPaneCategory } from "./editorGroupModel.js";
import { defaultSurfaceRegistry, type SurfaceKind } from "./surfaceRegistry.js";

export type DockviewWorkbenchPane = {
  readonly id: string;
  readonly kind: SurfaceKind;
  readonly category: WorkbenchPaneCategory;
  readonly title: string;
  readonly detail: string;
  readonly meta: readonly string[];
  readonly contentRevision?: string;
  readonly body?: ReactNode;
  readonly attentionState?: AgentAttentionState;
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

export type DockviewWorkbenchGroupSize = {
  readonly width?: number;
  readonly height?: number;
};

export type DockviewWorkbenchLayoutProps = {
  readonly groups: readonly DockviewWorkbenchGroup[];
  readonly activePaneByGroup: Readonly<Record<string, string>>;
  readonly onSelectPane: (groupId: string, paneId: string) => void;
  // CONTRACT (260725 Phase 6 review cycle 1, Critical): fired on EVERY tab
  // click, including a click on the tab that is already Dockview's active
  // panel. `onSelectPane` cannot serve this purpose: it is driven by
  // `onDidActivePanelChange`, and dockview-core only emits that when the
  // active panel actually CHANGES (`DockviewComponent.doSetGroupActive`
  // compares against the current value; `DockviewGroupPanelModel.openPanel`
  // early-returns when the panel is already active). Anything that must
  // happen when the user deliberately returns to an ALREADY-ACTIVE tab -
  // acknowledging that tab's attention indicator being the motivating case -
  // needs this second, change-independent trigger. Deliberately carries no
  // group id and performs no selection: it is a pure "the user touched this
  // tab" signal, so it can never fight `onSelectPane`'s placement policy.
  readonly onAcknowledgePane?: (paneId: string) => void;
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
  // Best-effort split-size restore (ticket's "split proportions" clause).
  // Applied once per workbench group id, the first time that group exists
  // after a sync; later live resizes are never overridden by this prop
  // changing identity (see `appliedInitialSizeRef` in the component).
  readonly initialGroupSizeById?: Readonly<
    Record<string, DockviewWorkbenchGroupSize>
  >;
  // Fired (debounced via `queueMicrotask`, matching this file's existing
  // sync-tick idiom) whenever Dockview reports a layout change, with the
  // current width/height of every live workbench group. Best-effort: a
  // group id absent from Dockview's current live set is simply absent from
  // the callback's map, not an error.
  readonly onLayoutSnapshot?: (
    sizeByWorkbenchGroupId: Record<string, DockviewWorkbenchGroupSize>,
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
  readonly contentRevision?: string;
  readonly body?: ReactNode;
  readonly attentionState?: AgentAttentionState;
  readonly onAcknowledgePane?: (paneId: string) => void;
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
  onAcknowledgePane,
  onRequestClosePane,
  onSelectPane,
  initialGroupSizeById,
  onLayoutSnapshot,
}: DockviewWorkbenchLayoutProps) {
  const apiRef = useRef<DockviewApi | null>(null);
  const syncingRef = useRef(false);
  const dockGroupToWorkbenchGroupRef = useRef<ReadonlyMap<string, string>>(
    new Map(),
  );
  const callbacksRef = useRef({
    onMovePane,
    onAcknowledgePane,
    onRequestClosePane,
    onSelectPane,
    onLayoutSnapshot,
  });
  callbacksRef.current = {
    onMovePane,
    onAcknowledgePane,
    onRequestClosePane,
    onSelectPane,
    onLayoutSnapshot,
  };
  // CONTRACT: a STABLE identity handed to every panel's params, forwarding
  // to whatever the latest render passed. Panel params are only refreshed
  // when `shouldUpdateDockviewWorkbenchPanelParams` says so - and for a
  // connected terminal that is deliberately almost never - so embedding the
  // raw prop would freeze a stale closure over `App()`'s attention state
  // into the tab and acknowledge the wrong revision. Reading through
  // `callbacksRef` (refreshed on every render above) is this file's existing
  // answer to that, previously used only for Dockview's own event handlers.
  const acknowledgePane = useCallback((paneId: string) => {
    callbacksRef.current.onAcknowledgePane?.(paneId);
  }, []);
  // Same contract as `acknowledgePane` above, for the close affordance
  // (260726 Phase 1, D1). Handing the RAW `onRequestClosePane` prop to
  // `syncDockviewWorkbench` froze whichever closure happened to be current
  // the last time `shouldUpdateDockviewWorkbenchPanelParams` allowed a param
  // push into the tab - and for a connected terminal that predicate
  // deliberately almost never allows one, so a restored tab could keep
  // calling into a stale `App()` render's state for the rest of the session.
  const closePane = useCallback((request: DockviewTabCloseRequest) => {
    callbacksRef.current.onRequestClosePane?.(request);
  }, []);
  const initialGroupSizeByIdRef = useRef(initialGroupSizeById);
  initialGroupSizeByIdRef.current = initialGroupSizeById;
  // Applying a restored split size is a one-shot best-effort action per
  // workbench group id, not a continuous constraint - once applied (or once
  // determined there is nothing to apply for a group), later live resizes by
  // the user must never be overridden by a stale restore value.
  const appliedInitialSizeRef = useRef<Set<string>>(new Set());

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
        closePane,
        acknowledgePane,
      );
      const initialSizeById = initialGroupSizeByIdRef.current;
      if (initialSizeById) {
        for (const dockGroup of apiRef.current.groups) {
          const workbenchGroupId = dockGroupToWorkbenchGroupRef.current.get(
            dockGroup.id,
          );
          if (
            !workbenchGroupId ||
            appliedInitialSizeRef.current.has(workbenchGroupId)
          ) {
            continue;
          }
          appliedInitialSizeRef.current.add(workbenchGroupId);
          const size = initialSizeById[workbenchGroupId];
          if (size && (size.width !== undefined || size.height !== undefined)) {
            dockGroup.api.setSize(size);
          }
        }
      }
    } finally {
      queueMicrotask(() => {
        syncingRef.current = false;
      });
    }
  }, [acknowledgePane, activePaneByGroup, closePane, groups]);

  const handleReady = useCallback(
    (event: DockviewReadyEvent) => {
      apiRef.current = event.api;
      // CONTRACT: Dashboard policy remains outside Dockview. Dockview events are
      // reduced to dashboard pane/group ids before invoking product callbacks;
      // raw Dockview panel/group handles must not escape this adapter.
      event.api.onDidLayoutChange(() => {
        if (syncingRef.current || !callbacksRef.current.onLayoutSnapshot) {
          return;
        }
        // Debounced via microtask (matching this file's `syncPanels`
        // finally-block idiom) so a burst of layout events collapses to one
        // snapshot read of the settled group sizes.
        queueMicrotask(() => {
          if (!apiRef.current || !callbacksRef.current.onLayoutSnapshot) {
            return;
          }
          const sizeByWorkbenchGroupId: Record<
            string,
            DockviewWorkbenchGroupSize
          > = {};
          for (const dockGroup of apiRef.current.groups) {
            const workbenchGroupId = dockGroupToWorkbenchGroupRef.current.get(
              dockGroup.id,
            );
            if (!workbenchGroupId) {
              continue;
            }
            sizeByWorkbenchGroupId[workbenchGroupId] = {
              width: dockGroup.api.width,
              height: dockGroup.api.height,
            };
          }
          callbacksRef.current.onLayoutSnapshot(sizeByWorkbenchGroupId);
        });
      });
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
      // CONTRACT (260725 Phase 6): ALWAYS present, `"none"` when there is
      // nothing to show, rather than omitted - a browser assertion for the
      // CLEARED state must be able to distinguish "indicator gone" from
      // "tab not rendered yet"/"attribute never written", which an absent
      // attribute cannot. Exposed as a data attribute (rather than left to
      // CSS/visual inspection) so it is Playwright-assertable.
      //
      // NAMING (review cycle 1, Fit Minor): this name is the plan's, and it
      // does NOT follow the `data-workbench-*` prefix every sibling
      // attribute on this element uses - `data-workbench-group-id`,
      // `data-workbench-pane-id`, `data-workbench-close-confirmation`, and
      // the badge span's own `data-workbench-tab-attention` below all do.
      // Kept as specified rather than silently renamed (the acceptance spec
      // selects on it); recorded here so the inconsistency is a known,
      // deliberate exception instead of a claim of conformance.
      data-attention-state={params.attentionState ?? "none"}
      role="tab"
      title={api.title ?? params.title}
      // CONTRACT (review cycle 1, Critical): acknowledgement must fire on
      // EVERY tab click, not only on a click that changes Dockview's active
      // panel. The primary flow for this feature is an agent working in the
      // tab the user left focused: the badge appears on the ALREADY-ACTIVE
      // tab, so `onDidActivePanelChange` (and therefore `onSelectPane`)
      // never fires when the user comes back and clicks it, and before this
      // handler existed the badge was unclearable - permanently so with a
      // single open pane. `onAcknowledgePane` performs no selection, so
      // firing it alongside `onSelectPane` on a genuine tab change is
      // idempotent, not a double-select. The close button stops propagation,
      // so closing a tab never routes through here.
      onClick={() => {
        params.onAcknowledgePane?.(params.paneId);
      }}
    >
      {/* Dockview owns one deterministic tab strip per group in this slice.
          Dashboard row policy is retained as pane category metadata instead
          of rendering the retired two-row pinned/opened custom header. */}
      {/* CONTRACT: Hover-only close UI belongs in this Dockview tab component.
          Close clicks must call the dashboard callback with pane identity and
          pointer coordinates so App policy can decide immediate close versus
          cursor-near session confirmation without exposing Dockview handles. */}
      <span
        aria-hidden="true"
        className="workbench-tab-icon"
        data-workbench-tab-icon={params.surfaceKind}
      />
      {params.attentionState ? (
        <span
          aria-hidden="true"
          className="workbench-tab-attention"
          data-workbench-tab-attention={params.attentionState}
        />
      ) : null}
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
  onAcknowledgePane?: (paneId: string) => void,
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
      const params = toDockviewWorkbenchPanelParams(
        group,
        pane,
        onRequestClosePane,
        onAcknowledgePane,
      );
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

      if (
        pane.id === activePaneId &&
        !dockviewPanelIsSelectedWithinGroup(existingPanel)
      ) {
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

function toDockviewWorkbenchPanelParams(
  group: DockviewWorkbenchGroup,
  pane: DockviewWorkbenchPane,
  onRequestClosePane?: (request: DockviewTabCloseRequest) => void,
  onAcknowledgePane?: (paneId: string) => void,
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
    contentRevision: pane.contentRevision,
    body: pane.body,
    attentionState: pane.attentionState,
    onAcknowledgePane,
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
