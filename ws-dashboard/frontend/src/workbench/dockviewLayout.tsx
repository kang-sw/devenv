import type { ReactNode } from "react";
import type { SurfaceKind, WorkbenchPaneCategory } from "./index.js";

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

export function DockviewWorkbenchLayout(_props: DockviewWorkbenchLayoutProps) {
  // CONTRACT: This component is the only visible workbench split/tab/pane layout
  // owner. The populated implementation must mount DockviewReact under this
  // root and must expose the stable owner marker used by browser acceptance.
  // CONTRACT: Dashboard policy remains outside Dockview. Callers pass dashboard
  // groups, pane identity, active selection, and movement callbacks; raw
  // Dockview panel/group handles must not escape this adapter.
  // HINT: Preserve the existing pane body ReactNodes from App.tsx and route
  // Dockview panel params through dockviewBridge.ts metadata.
  // HOLE: Populate DockviewReact components, component registry, group/panel
  // synchronization, and selected layout library DOM assertions.
  return (
    <div data-workbench-layout-owner="dockview" role="presentation">
      <div data-dockview-contract="pending" />
    </div>
  );
}
