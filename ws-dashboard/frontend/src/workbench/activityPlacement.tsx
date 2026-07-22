import type { ReactNode } from "react";
import type { DashboardCommandDispatcher } from "../commands.js";
import {
  fetchWorkRootActivityTranscript,
  type WorkRootActivityBadgeInput,
} from "../workRootActivity.js";
import { ActivityConsole } from "../ActivityConsole.js";
import { serverScopedIdentity } from "../resourceModel.js";
import type { ViewState, WorkRootView } from "../resourceModel.js";
import {
  surfaceLogicalKey,
  workbenchGroupId,
  type WorkbenchPlacementState,
} from "./policy.js";
import {
  initialWorkbenchGroups,
  type WorkbenchEditorGroupModel,
  type WorkbenchPane,
} from "./editorGroups.js";

export function workRootActivityPaneLogicalKey(workRootId: string) {
  return surfaceLogicalKey("workRootActivity", workRootId);
}

export function workRootActivityPaneId(workRootId: string) {
  return `workRootActivity-pane:${workRootId}`;
}

export type ActivityTranscriptRefreshSignal = {
  readonly rootId: string;
  readonly serverRoute?: string | null;
  readonly activityId: string;
  readonly cursor: string | null;
  readonly sequence: number;
};

export function WorkRootActivityPane({
  activity,
  onCommand,
  transcriptRefresh,
  serverRoute,
}: {
  activity: WorkRootActivityBadgeInput;
  onCommand: DashboardCommandDispatcher;
  transcriptRefresh: ActivityTranscriptRefreshSignal | null;
  serverRoute: string;
}): ReactNode {
  // CONTRACT: A reversible read-only Activity Console projection. It consumes
  // source-neutral feed items/transcripts, exposes command-routed controls, and
  // offers no agent/exec control actions or daemon-side acknowledgement.
  return (
    <section className="workroot-activity-pane" aria-label="WorkRoot Activity">
      {activity.phase === "loading" ? (
        <div className="workroot-activity-state">Loading workRoot activity</div>
      ) : activity.phase === "error" ? (
        <div className="workroot-activity-state workroot-activity-state-error">
          WorkRoot activity is unavailable
        </div>
      ) : (
        <ActivityConsole
          view={activity.view}
          onCommand={onCommand}
          loadTranscript={(workRootId, activityId, options) =>
            fetchWorkRootActivityTranscript(workRootId, activityId, {
              ...options,
              serverRoute,
            })
          }
          transcriptRefresh={transcriptRefresh}
        />
      )}
    </section>
  );
}

export function workRootActivityPaneRevision(
  activity: WorkRootActivityBadgeInput,
  transcriptRefresh: ActivityTranscriptRefreshSignal | null,
) {
  if (activity.phase !== "ready") {
    return `activity:${activity.phase}`;
  }
  const view = activity.view;
  return [
    "activity",
    view.status,
    view.updateMode,
    view.feedCursor ?? "",
    view.selectedItemId ?? "",
    view.items.length,
    transcriptRefresh?.activityId ?? "",
    transcriptRefresh?.cursor ?? "",
    transcriptRefresh?.sequence ?? 0,
  ].join(":");
}

// Build the placement state the WorkRoot Activity badge feeds into
// decideSurfaceOpenWithDynamicGroups. It mirrors the live editor groups so a
// duplicate open focuses the pane in whatever group it currently occupies,
// while a first open resolves through the policy-owned support-split target.
export function workRootActivityPlacementState(
  groups: ReadonlyArray<{ id: string; label: string }>,
  editorGroups: WorkbenchEditorGroupModel[],
  workRootId: string,
): WorkbenchPlacementState {
  const dashboardGroups = groups.length > 0 ? groups : initialWorkbenchGroups;
  const paneId = workRootActivityPaneId(workRootId);
  const owningGroup = editorGroups.find((group) =>
    group.panes.some((pane) => pane.id === paneId),
  );
  return {
    groups: dashboardGroups.map((group) => ({
      groupId: workbenchGroupId(group.id),
    })),
    focusedGroupId: workbenchGroupId(dashboardGroups[0]?.id ?? "group-1"),
    attachments: owningGroup
      ? [
          {
            attachmentId:
              paneId as WorkbenchPlacementState["attachments"][number]["attachmentId"],
            groupId: workbenchGroupId(owningGroup.id),
            surfaceKind: "workRootActivity",
            logicalKey: workRootActivityPaneLogicalKey(workRootId),
          },
        ]
      : [],
  };
}

export function workRootActivityWorkbenchPane(
  root: WorkRootView,
  activity: WorkRootActivityBadgeInput,
  transcriptRefresh: ActivityTranscriptRefreshSignal | null,
  onCommand: DashboardCommandDispatcher,
): WorkbenchPane {
  const ready = activity.phase === "ready" ? activity.view : null;
  const state: ViewState = {
    status:
      activity.phase === "loading"
        ? "loading"
        : activity.phase === "error"
          ? "unavailable"
          : (ready?.status ?? "ok"),
    loading: activity.phase === "loading",
    stale: false,
    error: activity.phase === "error" ? "activity unavailable" : null,
  };
  const meta =
    ready !== null
      ? [
          `${ready.summary.total} agents`,
          `${ready.summary.active} active`,
          "read-only",
        ]
      : [activity.phase, "read-only"];
  return {
    id: workRootActivityPaneId(
      serverScopedIdentity(root.resourcePath.serverId, root.id),
    ),
    kind: "workRootActivity",
    category: "opened",
    title: "WorkRoot Activity",
    detail: `${root.label} activity console`,
    state,
    meta,
    contentRevision: workRootActivityPaneRevision(activity, transcriptRefresh),
    body: (
      <WorkRootActivityPane
        activity={activity}
        onCommand={onCommand}
        transcriptRefresh={transcriptRefresh}
        serverRoute={root.resourcePath.serverId}
      />
    ),
  };
}
