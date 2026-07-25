import type { AgentAttentionState } from "../agentAttention.js";
import type { WorkbenchPaneCategory } from "./editorGroupModel.js";
import type { SurfaceKind } from "./surfaceRegistry.js";

export type DockviewWorkbenchPanelParamsForSync = {
  readonly groupId: string;
  readonly groupLabel: string;
  readonly paneId: string;
  readonly category: WorkbenchPaneCategory;
  readonly surfaceKind: SurfaceKind;
  readonly title: string;
  readonly detail: string;
  readonly meta: readonly string[];
  readonly body?: unknown;
  readonly contentRevision?: string;
  readonly attentionState?: AgentAttentionState;
};

export type DockviewPanelForActiveSync = {
  readonly id: string;
  readonly group: {
    readonly activePanel?: { readonly id: string } | null;
  };
  readonly api: {
    readonly isActive?: boolean;
  };
};

export function dockviewPanelIsSelectedWithinGroup(
  panel: DockviewPanelForActiveSync,
): boolean {
  return panel.group.activePanel?.id === panel.id || panel.api.isActive === true;
}

export function shouldUpdateDockviewWorkbenchPanelParams(
  current: DockviewWorkbenchPanelParamsForSync | undefined,
  next: DockviewWorkbenchPanelParamsForSync,
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
    // CONTRACT (260725 Phase 6, REQUIRED fix - load-bearing): the attention
    // comparison must run BEFORE the socket-status early return, not after.
    // A terminal tab showing an attention indicator is by definition an
    // agent terminal that is CONNECTED (`meta[1]` is `"connected"`), which
    // is exactly the case the return below suppresses - so without this
    // branch a `working` -> `ready` -> cleared transition changes
    // `attentionState` correctly in the pane model and then never reaches
    // Dockview's `updateParameters`, leaving the tab painted with a stale
    // (or absent) badge until some unrelated remount happens to repaint it.
    // The blur concern the early return exists for does not apply here:
    // `attentionState` changes at most once per turn boundary, not per
    // output chunk.
    if (current.attentionState !== next.attentionState) {
      return true;
    }
    const socketStatus = next.meta[1];
    return socketStatus !== "connecting" && socketStatus !== "connected";
  }
  return (
    current.contentRevision !== next.contentRevision ||
    current.meta.join("\0") !== next.meta.join("\0")
  );
}
