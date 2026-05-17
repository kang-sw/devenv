import { apiErrorDetail } from "./apiError.js";

export type WorkRootActivitySummary = {
  total: number;
  active: number;
  blocked: number;
  failed: number;
  unavailable: number;
};

export type NamedAgentCallActivityView = {
  status: "queued" | "running" | "completed" | "failed" | "cancelled" | string;
  active: boolean;
  terminal: boolean;
  executionId: string | null;
  startedAt: string | null;
  updatedAt: string | null;
  finishedAt: string | null;
  cleanupNeeded: boolean;
  error: string | null;
};

export type NamedAgentActivityView = {
  agentId: string;
  name: string | null;
  backend: string | null;
  harness: string | null;
  tier: string | null;
  model: string | null;
  effort: string | null;
  status: "idle" | "running" | "blocked" | "failed" | "unavailable" | string;
  lastCallAt: string | null;
  sessionPresent: boolean;
  currentCall: NamedAgentCallActivityView | null;
  detailHints: string[];
  diagnostics: string[];
};

export type WorkRootActivityView = {
  // CONTRACT: This mirrors the daemon projection API. It intentionally omits
  // host paths, cache paths, process ids, stream paths, and session ids.
  workRootId: string;
  status: "ok" | "unavailable" | "degraded" | string;
  summary: WorkRootActivitySummary;
  agents: NamedAgentActivityView[];
};

export function workRootActivityEndpoint(workRootId: string) {
  return `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/activity`;
}

export async function fetchWorkRootActivity(
  workRootId: string,
): Promise<WorkRootActivityView> {
  const response = await fetch(workRootActivityEndpoint(workRootId), {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as WorkRootActivityView;
}

// CONTRACT: The top-bar activity badge is a compact summary/entrypoint only.
// It mirrors the Phase 1 daemon projection counts and stays bounded: no row
// diagnostics, host paths, or long error text reach the toolbar.

export type WorkRootActivityBadgeTone =
  | "idle"
  | "active"
  | "attention"
  | "loading"
  | "error";

export type WorkRootActivityBadgeView = {
  // `tone` drives the badge dot color; `label` is the always-shown primary
  // text and `summary` is secondary text that may be truncated/hidden under
  // constrained toolbar width. `title` is the bounded full tooltip text.
  tone: WorkRootActivityBadgeTone;
  label: string;
  summary: string;
  title: string;
};

export type WorkRootActivityBadgeInput =
  | { phase: "loading" }
  | { phase: "error" }
  | { phase: "ready"; view: WorkRootActivityView };

const ACTIVITY_BADGE_TITLE_LIMIT = 120;

function boundedActivityTitle(text: string): string {
  return text.length <= ACTIVITY_BADGE_TITLE_LIMIT
    ? text
    : `${text.slice(0, ACTIVITY_BADGE_TITLE_LIMIT - 1)}…`;
}

/**
 * Project a selected workRoot's activity fetch state into the compact top-bar
 * badge model. Loading, error, and unavailable states all collapse to bounded
 * short badge states rather than surfacing diagnostics in the toolbar.
 */
export function workRootActivityBadge(
  input: WorkRootActivityBadgeInput,
): WorkRootActivityBadgeView {
  if (input.phase === "loading") {
    return {
      tone: "loading",
      label: "agents",
      summary: "loading",
      title: "Agent activity loading",
    };
  }
  if (input.phase === "error") {
    return {
      tone: "error",
      label: "agents",
      summary: "unavailable",
      title: "Agent activity unavailable",
    };
  }

  const { status, summary } = input.view;
  if (status === "unavailable") {
    return {
      tone: "error",
      label: "agents",
      summary: "unavailable",
      title: "Agent activity unavailable",
    };
  }

  if (summary.total === 0) {
    return {
      tone: "idle",
      label: "no agents",
      summary: "",
      title: "No named agents",
    };
  }

  const parts: string[] = [];
  if (summary.active > 0) {
    parts.push(`${summary.active} active`);
  }
  if (summary.blocked > 0) {
    parts.push(`${summary.blocked} blocked`);
  }
  if (summary.failed > 0) {
    parts.push(`${summary.failed} failed`);
  }
  if (summary.unavailable > 0) {
    parts.push(`${summary.unavailable} unavailable`);
  }

  const degraded = status === "degraded";
  const needsAttention =
    degraded ||
    summary.blocked > 0 ||
    summary.failed > 0 ||
    summary.unavailable > 0;
  const tone: WorkRootActivityBadgeTone = needsAttention
    ? "attention"
    : summary.active > 0
      ? "active"
      : "idle";

  const label = `${summary.total} ${summary.total === 1 ? "agent" : "agents"}`;
  const secondary = parts.length > 0 ? parts.join(" · ") : "idle";
  const title = boundedActivityTitle(
    `${label} — ${parts.length > 0 ? parts.join(", ") : "idle"}${
      degraded ? " (degraded)" : ""
    }`,
  );

  return { tone, label, summary: secondary, title };
}
