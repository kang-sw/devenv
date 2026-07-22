import type { ReactNode } from "react";
import type {
  InstanceView,
  ResourceEntity,
  ViewState,
  WorkRootView,
} from "../resourceModel.js";
import { closeContractLabel, instanceSummary } from "../resourcePresentation.js";
import type { DashboardCommandDispatcher } from "../commands.js";
import type { ReadOnlyFilePane } from "../workRootFiles.js";
import type { TerminalPaneState } from "../terminals.js";
import type { TerminalPaneActions } from "../terminalPaneBody.js";
import type { AgentChatPaneState } from "../agentChatSessions.js";
import type { AgentChatPaneActions } from "../agentChatPaneBody.js";
import type { WorkRootActivityBadgeInput } from "../workRootActivity.js";
import type {
  WorkbenchPaneCategory,
  WorkbenchPaneOrder,
} from "./editorGroupModel.js";
import type { SurfaceKind } from "./surfaceRegistry.js";
import { activityPaneGroupIdFromOrder } from "./paneOrder.js";
import {
  workRootActivityWorkbenchPane,
  type ActivityTranscriptRefreshSignal,
} from "./activityPlacement.js";
import { readOnlyWorkbenchPanesByGroup } from "./readOnlyWorkbenchPane.js";
import { terminalWorkbenchPanesByGroup } from "./terminalWorkbenchPane.js";
import { agentChatWorkbenchPanesByGroup } from "./agentChatWorkbenchPane.js";

export type WorkbenchPane = {
  readonly id: string;
  readonly kind: SurfaceKind;
  readonly category: WorkbenchPaneCategory;
  readonly title: string;
  readonly detail: string;
  readonly state: ViewState;
  readonly meta: readonly string[];
  readonly contentRevision?: string;
  readonly body?: ReactNode;
};

export type WorkbenchEditorGroupModel = {
  readonly id: string;
  readonly label: string;
  readonly panes: readonly WorkbenchPane[];
};

export const initialWorkbenchGroups = [
  { id: "group-1", label: "group 1" },
  { id: "group-2", label: "group 2" },
] as const;

export function buildWorkbenchEditorGroups(
  root: WorkRootView,
  groups: ReadonlyArray<{ id: string; label: string }>,
  mainInstance: InstanceView | null,
  selectedInstance: InstanceView | null,
  supportEntity: ResourceEntity | null,
  readOnlyFilePanes: ReadOnlyFilePane[],
  readOnlyFilePaneOrderByGroup: WorkbenchPaneOrder,
  activityPaneOrderByGroup: WorkbenchPaneOrder,
  terminalPanes: TerminalPaneState[],
  terminalPaneOrderByGroup: WorkbenchPaneOrder,
  terminalActions: TerminalPaneActions,
  agentChatPanes: AgentChatPaneState[],
  agentChatPaneOrderByGroup: WorkbenchPaneOrder,
  agentChatActions: AgentChatPaneActions,
  closedAgentPaneIds: readonly string[] = [],
  activityPaneOpen = false,
  activityState: WorkRootActivityBadgeInput = { phase: "loading" },
  activityTranscriptRefresh: ActivityTranscriptRefreshSignal | null,
  onCommand: DashboardCommandDispatcher,
  onDocumentSaved: (source: {
    serverRoute?: string;
    workRootId: string;
    path: string;
    content: string;
    contentHash: string;
    sizeBytes: number;
  }) => void,
): WorkbenchEditorGroupModel[] {
  void selectedInstance;
  void supportEntity;
  const dashboardGroups = groups.length > 0 ? groups : initialWorkbenchGroups;
  const readOnlyPanesByGroup = readOnlyWorkbenchPanesByGroup(
    root,
    readOnlyFilePanes,
    readOnlyFilePaneOrderByGroup,
    dashboardGroups,
    onCommand,
    onDocumentSaved,
  );
  const terminalPanesByGroup = terminalWorkbenchPanesByGroup(
    root,
    terminalPanes,
    terminalPaneOrderByGroup,
    terminalActions,
    dashboardGroups,
  );
  const agentChatPanesByGroup = agentChatWorkbenchPanesByGroup(
    root,
    agentChatPanes,
    agentChatPaneOrderByGroup,
    agentChatActions,
    dashboardGroups,
  );
  const closedAgentPaneIdSet = new Set(closedAgentPaneIds);
  const agentPane: WorkbenchPane[] =
    mainInstance && !closedAgentPaneIdSet.has("main-agent")
      ? [
          {
            id: "main-agent",
            kind: "agent",
            category: "pinned",
            title: mainInstance.label,
            detail: instanceSummary(mainInstance),
            state: mainInstance.state,
            meta: [
              mainInstance.kind,
              mainInstance.interactionMode,
              closeContractLabel("agent"),
            ],
          },
        ]
      : [];

  const activityPane = activityPaneOpen
    ? workRootActivityWorkbenchPane(
        root,
        activityState,
        activityTranscriptRefresh,
        onCommand,
      )
    : null;
  const activityGroupId = activityPane
    ? activityPaneGroupIdFromOrder(
        activityPane.id,
        activityPaneOrderByGroup,
        dashboardGroups,
      )
    : null;

  return dashboardGroups.map((group, index) => ({
    id: group.id,
    label: group.label,
    panes: [
      ...(index === 0 ? agentPane : []),
      ...(terminalPanesByGroup[group.id] ?? []),
      ...(activityPane && activityGroupId === group.id ? [activityPane] : []),
      ...(readOnlyPanesByGroup[group.id] ?? []),
      // CONTRACT: agentChat panes must be spread in *after* the read-only
      // file panes, not before. This array's index drives each pane's
      // position within its Dockview group; a brand-new agentChat pane
      // always sits at the *end* of this array (nothing else is inserted
      // after it), so appending it here means every pre-existing pane in
      // the group keeps its prior index. If agentChat panes were spliced in
      // earlier (as they were originally), adding one shifts every
      // already-open pane's index by one, forcing
      // `syncDockviewWorkbench` to call `existingPanel.api.moveTo(...,
      // { skipSetActive: true })` on those already-active panes -
      // confirmed by instrumentation to reassert them as Dockview's active
      // tab despite `skipSetActive`, which silently clobbers the new
      // agentChat panel's `inactive: false` placement (see
      // 260711-feat-ws-dashboard-agent-activity-chat-ui Phase 1). Keeping
      // existing panes' indices stable avoids the moveTo call entirely.
      ...(agentChatPanesByGroup[group.id] ?? []),
    ],
  }));
}
