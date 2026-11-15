/** Hosted leads cannot use intake: the runner owns their snapshot. Send only
 * extension-observed wakes, task counts and eligible lead summaries on its reserved RPC widget instead.
 * Older runners ignore the additive summary/capability fields and keep their
 * existing wake/count handling; no new intake kind is introduced.
 */
import { onBackgroundTasksChanged } from "./background-tasks.ts";
import { defaultSchedule, FLUSH_INTERVAL_MS, PENDING_EVENT_CAP } from "./dashboard-feed.ts";
import { dashboardWakeMessages, type DashboardWake } from "./dashboard-wake.ts";
import { REASONING_SUMMARY_CAPABILITY, type ReasoningSummary } from "./reasoning-summary.ts";
import { hostedSummaryLine } from "./reasoning-summary-transport.ts";

export const HOSTED_EVENTS_WIDGET_KEY = "ws-dashboard-events";

export interface HostedDashboardEventsOptions {
  setWidget: (key: string, lines: string[]) => void;
  taskCounts: () => { timed: number; monitors: number };
  reasoningSummary?: () => ReasoningSummary | undefined;
  // Explicit process-local opt-in; omitted by legacy bundles, false on unsupported hosts.
  reasoningSummaryCapable?: boolean;
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
  let capabilityDirty = false;
  let lastFlush: number | undefined;
  let cancel: (() => void) | undefined;
  let unsubscribe: (() => void) | undefined;
  let summaryDirty = false;
  let lastSummary: string | undefined;

  function flush(): void {
    if (stopped) return;
    lastFlush = now();
    try {
      const counts = options.taskCounts();
      const status = JSON.stringify({
        kind: "status", background: counts.timed, monitors: counts.monitors,
        ...(options.reasoningSummaryCapable === undefined ? {} : {
          capabilities: options.reasoningSummaryCapable ? [REASONING_SUMMARY_CAPABILITY] : [],
        }),
      });
      const lines = pending.map((wake) => JSON.stringify(wake));
      if (status !== lastCounts || capabilityDirty) lines.push(status);
      if (lines.length) options.setWidget(HOSTED_EVENTS_WIDGET_KEY, lines);
      pending = [];
      lastCounts = status;
      capabilityDirty = false;
    } catch {
      // A stale context retains existing wake/count behavior until another mark.
    }
    if (!summaryDirty || !options.reasoningSummaryCapable) return;
    try {
      const summary = options.reasoningSummary?.();
      const line = summary && hostedSummaryLine(summary);
      if (line && line !== lastSummary) options.setWidget(HOSTED_EVENTS_WIDGET_KEY, [line]);
      lastSummary = line;
      summaryDirty = false;
    } catch {
      // No source persistence queue or acknowledgement: only current-turn coalescing.
    }
  }

  function dirty(): void {
    if (stopped || cancel) return;
    cancel = schedule(() => {
      cancel = undefined;
      flush();
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
    capabilityDirty = false;
    lastFlush = undefined;
    summaryDirty = false;
    lastSummary = undefined;
  }

  return {
    start() {
      stop();
      stopped = false;
      capabilityDirty = options.reasoningSummaryCapable !== undefined;
      unsubscribe = onBackgroundTasksChanged(dirty);
      dirty();
    },
    stop,
    leadEvent(event: unknown) {
      if (stopped) return;
      const e = event as { type?: unknown; message?: unknown } | undefined;
      if (e?.type === "agent_start") {
        capabilityDirty = options.reasoningSummaryCapable !== undefined;
        dirty();
        return;
      }
      if (e?.type === "agent_settled") {
        capabilityDirty = options.reasoningSummaryCapable !== undefined;
        cancel?.();
        cancel = undefined;
        summaryDirty = true;
        // Pi 1.5.2 awaits this handler before publishing its native settled record.
        flush();
        summaryDirty = false; // never retain a closed-turn source retry queue
        return;
      }
      if (e?.type === "before_provider_request" || e?.type === "provider_stream_event" || e?.type === "message_end") {
        summaryDirty = true;
        dirty();
        return;
      }
      if (e?.type !== "message_start" || !e.message || typeof e.message !== "object" || seen.has(e.message)) return;
      seen.add(e.message);
      pending.push(...dashboardWakeMessages(e.message));
      if (pending.length > PENDING_EVENT_CAP) pending.splice(0, pending.length - PENDING_EVENT_CAP);
      dirty();
    },
  };
}
