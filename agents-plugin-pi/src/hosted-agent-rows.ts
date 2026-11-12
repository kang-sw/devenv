/**
 * The hosted lead's subagent rows for its ws dashboard.
 *
 * In a hosted session (hosted-session.ts) the daemon reads the lead's RPC
 * output and keeps the session's live snapshot itself, so the lead sends no
 * dashboard feed: a second intake writer would open a competing slot and
 * hide the daemon's lead snapshot. The daemon cannot see the lead's native
 * subagents, though, and RPC mode carries an extension's output to stdout
 * only as UI requests. This channel therefore reports the subagents through
 * `ctx.ui.setWidget(HOSTED_AGENTS_WIDGET_KEY, lines)`: each line is one JSON
 * event, the same `agent` and `permission` events the dashboard feed sends
 * for the subagent tree (`dashboardAgentEntries`, `dashboardAgentEvent`),
 * and the daemon takes only those. No reset, turn, or lead event is ever
 * sent here; those stay the daemon's own.
 *
 * Like the feed, a dirty mark is sampled at most once per
 * `FLUSH_INTERVAL_MS`, and the tree is sampled again every `AGENT_POLL_MS`
 * while any subagent was last sent as running. Unlike the feed, a change
 * that is only in a subagent's progress (its batches, `contextTokens`, and
 * `outputTps`) is held until `PROGRESS_COALESCE_MS` has passed since the
 * channel last sent one, and streaming character growth
 * (`generating.chars`) is no change at all: these lines are the hosted
 * lead's stdout, which the daemon's holder ring keeps, and per-token lines
 * would wrap it. Any other field (status, label, model, effort, parent,
 * session key, caption), and a permission, still goes out at the next
 * flush, carrying the current progress. A widget update is
 * fire-and-forget; one the host drops is not detected, so while any
 * subagent exists the whole tree is sent again every `FULL_RESEND_MS`,
 * without batches (the next change carries them), and rows a lost update
 * left behind recover. A throwing `setWidget` (a stale context) forgets
 * what was sent, so the next change sends every subagent.
 */

import type { BatchView } from "./agent-batches.ts";
import { AGENT_POLL_MS, FLUSH_INTERVAL_MS, dashboardAgentEvent, dashboardPermissionEvent, defaultSchedule, type DashboardAgentEntry, type DashboardEvent } from "./dashboard-feed.ts";

/** The widget key the daemon reads subagent rows from; every other widget is the host's own. */
export const HOSTED_AGENTS_WIDGET_KEY = "ws-dashboard-agents";

/** Interval of the whole-tree resend while any subagent exists. */
export const FULL_RESEND_MS = 30_000;

/** Least interval between two sends that carry a progress-only change. */
export const PROGRESS_COALESCE_MS = 2_000;

export interface HostedAgentRowsOptions {
  /** `ctx.ui.setWidget` with string lines; may throw. */
  setWidget: (key: string, lines: string[]) => void;
  /** The subagent tree, read when a flush drains after a dirty mark. */
  agents: () => DashboardAgentEntry[];
  now?: () => number;
  /** Arms a timer and returns its cancel. Defaults to an unref'd `setTimeout`. */
  schedule?: (fn: () => void, ms: number) => () => void;
}

export interface HostedAgentRows {
  /** Clears all state and sends the current tree. */
  start(): void;
  /** Cancels timers; a stopped channel sends nothing more. */
  stop(): void;
  /** Dirty mark for the subagent tree. */
  agentsChanged(): void;
}

/** A row's progress as change detection sees it: batches without streaming character counts, and the per-token numbers. */
function progressKey(entry: DashboardAgentEntry): string {
  const batches = entry.batches.map((view: BatchView) => (view.generating?.chars !== undefined ? { ...view, generating: { ...view.generating, chars: undefined } } : view));
  return JSON.stringify([batches, entry.contextTokens ?? null, entry.outputTps ?? null]);
}

/** A row's `agent` event without its progress: what goes out at once when it changes. */
function headKey(entry: DashboardAgentEntry): string {
  const { contextTokens: _tokens, outputTps: _tps, ...head } = dashboardAgentEvent(entry, { batches: false });
  return JSON.stringify(head);
}

interface SentRow {
  head: string;
  progress: string;
  status: DashboardAgentEntry["status"];
  permission: string;
}

/**
 * The channel's own change detection: the feed's `createAgentEventDiff`
 * (same events, same done for a child gone while running), except that a
 * progress-only change is sent only when `progressDue`, and otherwise stays
 * unsent and reported as held.
 */
function createHostedRowDiff() {
  let sentRows = new Map<string, SentRow>();
  return {
    events(entries: DashboardAgentEntry[], progressDue: boolean): { events: DashboardEvent[]; progressSent: boolean; progressHeld: boolean } {
      const out: DashboardEvent[] = [];
      let progressSent = false;
      let progressHeld = false;
      const seen = new Set<string>();
      for (const entry of entries) {
        seen.add(entry.agentId);
        const head = headKey(entry);
        const progress = progressKey(entry);
        const permission = entry.permission ? JSON.stringify(entry.permission) : "";
        const sent = sentRows.get(entry.agentId);
        const progressChanged = sent?.progress !== progress;
        const send = sent?.head !== head || (progressChanged && progressDue);
        if (send) {
          out.push(dashboardAgentEvent(entry));
          if (progressChanged) progressSent = true;
        } else if (progressChanged) {
          progressHeld = true;
        }
        if ((sent?.permission ?? "") !== permission) out.push(dashboardPermissionEvent(entry));
        sentRows.set(entry.agentId, { head, progress: send ? progress : sent!.progress, status: entry.status, permission });
      }
      for (const [agentId, sent] of sentRows) {
        if (seen.has(agentId)) continue;
        // Gone from the registry: one that was still running ends as done.
        if (sent.status === "running") out.push({ kind: "agent", agentId, status: "done" });
        sentRows.delete(agentId);
      }
      return { events: out, progressSent, progressHeld };
    },
    anyRunning(): boolean {
      return [...sentRows.values()].some((sent) => sent.status === "running");
    },
    clear(): void {
      sentRows = new Map();
    },
  };
}

export function createHostedAgentRows(options: HostedAgentRowsOptions): HostedAgentRows {
  const now = options.now ?? Date.now;
  const schedule = options.schedule ?? defaultSchedule;
  const sent = createHostedRowDiff();
  let stopped = true;
  let dirty = false;
  let lastFlushAt: number | undefined;
  let lastProgressSendAt: number | undefined;
  let cancelFlush: (() => void) | undefined;
  let cancelPoll: (() => void) | undefined;
  let cancelResend: (() => void) | undefined;
  let cancelProgress: (() => void) | undefined;

  function entries(): DashboardAgentEntry[] {
    try {
      return options.agents();
    } catch {
      return [];
    }
  }

  function emit(events: DashboardEvent[]): void {
    if (events.length === 0) return;
    try {
      options.setWidget(HOSTED_AGENTS_WIDGET_KEY, events.map((event) => JSON.stringify(event)));
    } catch {
      // Nothing reached the daemon: send every subagent at the next change.
      sent.clear();
    }
  }

  function scheduleFlush(): void {
    if (stopped || cancelFlush || !dirty) return;
    const delay = lastFlushAt === undefined ? 0 : Math.max(0, lastFlushAt + FLUSH_INTERVAL_MS - now());
    cancelFlush = schedule(() => {
      cancelFlush = undefined;
      flush();
    }, delay);
  }

  function armPoll(): void {
    if (stopped || cancelPoll || !sent.anyRunning()) return;
    cancelPoll = schedule(() => {
      cancelPoll = undefined;
      agentsChanged();
    }, AGENT_POLL_MS);
  }

  /** A held progress change goes out at the first flush after its window ends. */
  function armProgress(): void {
    if (stopped || cancelProgress || lastProgressSendAt === undefined) return;
    cancelProgress = schedule(() => {
      cancelProgress = undefined;
      agentsChanged();
    }, Math.max(0, lastProgressSendAt + PROGRESS_COALESCE_MS - now()));
  }

  function armResend(): void {
    if (stopped || cancelResend) return;
    cancelResend = schedule(() => {
      cancelResend = undefined;
      resend();
    }, FULL_RESEND_MS);
  }

  function flush(): void {
    if (stopped) return;
    dirty = false;
    lastFlushAt = now();
    const tree = entries();
    const progressDue = lastProgressSendAt === undefined || lastFlushAt >= lastProgressSendAt + PROGRESS_COALESCE_MS;
    const diff = sent.events(tree, progressDue);
    if (diff.progressSent) lastProgressSendAt = lastFlushAt;
    emit(diff.events);
    if (diff.progressHeld) armProgress();
    armPoll();
    if (tree.length > 0) armResend();
  }

  function resend(): void {
    if (stopped) return;
    const tree = entries();
    if (tree.length === 0) return;
    emit(tree.flatMap((entry) => [dashboardAgentEvent(entry, { batches: false }), dashboardPermissionEvent(entry)]));
    armResend();
  }

  function agentsChanged(): void {
    if (stopped) return;
    dirty = true;
    scheduleFlush();
  }

  function cancelAll(): void {
    cancelFlush?.();
    cancelPoll?.();
    cancelResend?.();
    cancelProgress?.();
    cancelFlush = cancelPoll = cancelResend = cancelProgress = undefined;
  }

  return {
    start() {
      cancelAll();
      sent.clear();
      lastFlushAt = lastProgressSendAt = undefined;
      stopped = false;
      agentsChanged();
    },
    stop() {
      cancelAll();
      sent.clear();
      dirty = false;
      stopped = true;
    },
    agentsChanged() {
      try {
        agentsChanged();
      } catch {
        // advisory
      }
    },
  };
}
