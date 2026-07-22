import type { ReactNode } from "react";
import type { DashboardCommandDispatcher } from "../commands.js";
import {
  fetchWorkRootActivityTranscript,
  type WorkRootActivityBadgeInput,
} from "../workRootActivity.js";
import { ActivityConsole } from "../ActivityConsole.js";
import { surfaceLogicalKey } from "./policy.js";

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
