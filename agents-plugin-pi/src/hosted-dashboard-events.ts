/** Hosted leads cannot use intake: the runner owns their snapshot. Send only
 * extension-observed wakes and task counts on its reserved RPC widget instead.
 * Older runners ignore this widget, so no new intake kind/field reaches them.
 */
import { onBackgroundTasksChanged } from "./background-tasks.ts";
import { defaultSchedule, FLUSH_INTERVAL_MS, PENDING_EVENT_CAP } from "./dashboard-feed.ts";
import { dashboardWakeMessages, type DashboardWake } from "./dashboard-wake.ts";

export const HOSTED_EVENTS_WIDGET_KEY = "ws-dashboard-events";

export interface HostedDashboardEventsOptions {
  setWidget: (key: string, lines: string[]) => void;
  taskCounts: () => { timed: number; monitors: number };
  now?: () => number;
  schedule?: (fn: () => void, ms: number) => () => void;
}

export function createHostedDashboardEvents(options: HostedDashboardEventsOptions) {
  const now = options.now ?? Date.now;
  const schedule = options.schedule ?? defaultSchedule;
  let stopped = true;
  let pending: DashboardWake[] = [];
  let seen = new WeakSet<object>();
  let lastCounts: string | undefined;
  let lastFlush: number | undefined;
  let cancel: (() => void) | undefined;
  let unsubscribe: (() => void) | undefined;

  function dirty(): void {
    if (stopped || cancel) return;
    cancel = schedule(() => {
      cancel = undefined;
      if (stopped) return;
      lastFlush = now();
      try {
        const counts = options.taskCounts();
        const status = JSON.stringify({ kind: "status", background: counts.timed, monitors: counts.monitors });
        const lines = pending.map((wake) => JSON.stringify(wake));
        if (status !== lastCounts) lines.push(status);
        if (lines.length) options.setWidget(HOSTED_EVENTS_WIDGET_KEY, lines);
        pending = [];
        lastCounts = status;
      } catch {
        // A stale context loses nothing: the next mark retries held wakes
        // and counts. Do not spin a timer while the session is being replaced.
      }
    }, lastFlush === undefined ? 0 : Math.max(0, lastFlush + FLUSH_INTERVAL_MS - now()));
  }

  function stop(): void {
    stopped = true;
    cancel?.();
    cancel = undefined;
    unsubscribe?.();
    unsubscribe = undefined;
    pending = [];
    seen = new WeakSet();
    lastCounts = undefined;
    lastFlush = undefined;
  }

  return {
    start() {
      stop();
      stopped = false;
      unsubscribe = onBackgroundTasksChanged(dirty);
      dirty();
    },
    stop,
    leadEvent(event: unknown) {
      if (stopped) return;
      const e = event as { type?: unknown; message?: unknown } | undefined;
      if (e?.type !== "message_start" || !e.message || typeof e.message !== "object" || seen.has(e.message)) return;
      seen.add(e.message);
      pending.push(...dashboardWakeMessages(e.message));
      if (pending.length > PENDING_EVENT_CAP) pending.splice(0, pending.length - PENDING_EVENT_CAP);
      dirty();
    },
  };
}
