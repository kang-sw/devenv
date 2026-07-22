import { Activity, RefreshCw } from "lucide-react";
import {
  gitChangeStatusSegments,
  gitSyncStatusSegments,
  gitStatusSegments,
  type GitStatusSegment,
  type WorkRootGitStatus,
} from "./gitToolbar.js";
import type { WorkRootActivityBadgeView } from "./workRootActivity.js";

export function GitStatusPill({
  status,
  pendingAction,
  onFetch,
  onPush,
  onPull,
}: {
  status: WorkRootGitStatus;
  pendingAction: "fetch" | "push" | "pull" | null;
  onFetch: () => void;
  onPush: () => void;
  onPull: () => void;
}) {
  const changeSegments = gitChangeStatusSegments(status);
  const syncSegments = gitSyncStatusSegments(status);
  const renderSegment = (segment: GitStatusSegment) => {
    const className = `git-status-segment git-status-segment-${segment.tone}`;
    if (segment.commandId === "git.push") {
      return (
        <button
          key={segment.key}
          className={className}
          data-command-id="git.push"
          type="button"
          disabled={segment.disabled || pendingAction === "push"}
          aria-label={
            pendingAction === "push" ? "Pushing Git changes" : undefined
          }
          onClick={onPush}
        >
          {pendingAction === "push" ? (
            <RefreshCw
              className="git-spinner"
              aria-hidden="true"
              size={12}
              strokeWidth={1.9}
            />
          ) : (
            segment.label
          )}
        </button>
      );
    }
    if (segment.commandId === "git.pullFfOnly") {
      return (
        <button
          key={segment.key}
          className={className}
          data-command-id="git.pullFfOnly"
          type="button"
          disabled={segment.disabled}
          onClick={onPull}
        >
          {segment.label}
        </button>
      );
    }
    return (
      <span key={segment.key} className={className}>
        {segment.label}
      </span>
    );
  };

  return (
    <span
      className="meta-chip ws-chip git-status-pill"
      title={status.branch?.upstream ?? "Git status"}
      aria-label={`Git status ${gitStatusSegments(status)}`}
    >
      <button
        className="git-status-refresh"
        data-command-id="git.fetch"
        type="button"
        aria-label={
          pendingAction === "fetch" ? "Fetching Git status" : "Fetch Git status"
        }
        disabled={pendingAction === "fetch"}
        onClick={onFetch}
      >
        <RefreshCw
          className={pendingAction === "fetch" ? "git-spinner" : undefined}
          aria-hidden="true"
          size={12}
          strokeWidth={1.9}
        />
      </button>
      {changeSegments.length ? (
        changeSegments.map(renderSegment)
      ) : (
        <span className="git-status-segment git-status-segment-clean">
          clean
        </span>
      )}
      {syncSegments.length ? (
        <span className="git-status-separator" aria-hidden="true">
          |
        </span>
      ) : null}
      {syncSegments.map(renderSegment)}
    </span>
  );
}

export function WorkbenchActivityBadge({
  activity,
  onOpenActivity,
}: {
  activity: WorkRootActivityBadgeView;
  onOpenActivity: () => void;
}) {
  // CONTRACT: Phase 2 renders a compact named-agent summary chip inside the
  // existing toolbar metadata row. It is a summary/entrypoint only: no detail
  // pane, agent controls, or row diagnostics live here.
  // CONTRACT: Phase 3 turns this entrypoint into the only top-bar opener for a
  // selected-workRoot Activity pane. The click handler must route through
  // dashboard workbench placement policy, focus duplicate panes, and keep the
  // pane reversible/read-only.
  return (
    <button
      className={`meta-chip ws-chip workbench-activity-badge workbench-activity-badge-${activity.tone}`}
      data-command-id="workbench.openActivity"
      data-activity-tone={activity.tone}
      type="button"
      title={activity.title}
      aria-label={`Open WorkRoot Activity: ${activity.title}`}
      onClick={onOpenActivity}
    >
      <span className="workbench-activity-badge-icon" aria-hidden="true">
        <Activity size={13} strokeWidth={1.8} />
      </span>
      <span className="workbench-activity-badge-dot" aria-hidden="true" />
      <span className="workbench-activity-badge-label">{activity.label}</span>
      {activity.summary ? (
        <span className="workbench-activity-badge-summary">
          {activity.summary}
        </span>
      ) : null}
    </button>
  );
}
