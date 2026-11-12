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
 * for the subagent tree (`dashboardAgentEntries`, `createAgentEventDiff`),
 * and the daemon takes only those. No reset, turn, or lead event is ever
 * sent here; those stay the daemon's own.
 *
 * Like the feed, a dirty mark is sampled at most once per
 * `FLUSH_INTERVAL_MS`, and the tree is sampled again every `AGENT_POLL_MS`
 * while any subagent was last sent as running. A widget update is
 * fire-and-forget; one the host drops is not detected, so while any
 * subagent exists the whole tree is sent again every `FULL_RESEND_MS`,
 * without batches (the next change carries them), and rows a lost update
 * left behind recover. A throwing `setWidget` (a stale context) forgets
 * what was sent, so the next change sends every subagent.
 */

import { AGENT_POLL_MS, FLUSH_INTERVAL_MS, createAgentEventDiff, dashboardAgentEvent, dashboardPermissionEvent, defaultSchedule, type DashboardAgentEntry, type DashboardEvent } from "./dashboard-feed.ts";

/** The widget key the daemon reads subagent rows from; every other widget is the host's own. */
export const HOSTED_AGENTS_WIDGET_KEY = "ws-dashboard-agents";

/** Interval of the whole-tree resend while any subagent exists. */
export const FULL_RESEND_MS = 30_000;

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

export function createHostedAgentRows(options: HostedAgentRowsOptions): HostedAgentRows {
  const now = options.now ?? Date.now;
  const schedule = options.schedule ?? defaultSchedule;
  const sent = createAgentEventDiff();
  let stopped = true;
  let dirty = false;
  let lastFlushAt: number | undefined;
  let cancelFlush: (() => void) | undefined;
  let cancelPoll: (() => void) | undefined;
  let cancelResend: (() => void) | undefined;

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
    emit(sent.events(tree));
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
    cancelFlush = cancelPoll = cancelResend = undefined;
  }

  return {
    start() {
      cancelAll();
      sent.clear();
      lastFlushAt = undefined;
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
