// Pure endpoint-builder and parse helpers for the daemon's server-wide,
// work-root-independent per-terminal attention (turn-state) event stream.
// See ai-docs/spec/ws-web-dashboard/index.md#260726-dashboard-terminal-attention-event-stream.
//
// CONTRACT: mirrors `workRootActivity.ts`'s split of pure parsing from
// `App.tsx`'s live-wiring effect - this module stays free of React/DOM/
// `EventSource` so it is unit-testable without a browser. Unlike every other
// endpoint builder in this codebase, `attentionEventsEndpoint` addresses no
// per-work-root path segment: attention is keyed by terminal id across the
// whole daemon, not scoped to a workRoot.

import {
  localCompatibleDashboardApiRoute,
  serverScopedIdentity,
} from "./resourceModel.js";

export type AgentAttentionState = "working" | "ready" | "idle";

export type AgentAttentionEntry = {
  terminalId: string;
  workRootId: string;
  state: AgentAttentionState;
  updatedAtMs: number;
};

export function attentionEventsEndpoint(serverRoute?: string | null): string {
  return localCompatibleDashboardApiRoute(serverRoute, [
    "terminals",
    "attention",
    "events",
  ]);
}

// CONTRACT (260725 Phase 5 review cycle 1, finding C): the subscription
// effect's `EventSource` had no `onerror` handler. Per the WHATWG
// `EventSource` algorithm, a non-2xx response status (a 401 from an expired
// owner session, or the forwarder's bounded 502 when a linked server is
// momentarily unreachable) FAILS the connection PERMANENTLY - `readyState`
// goes to `CLOSED` and the UA never auto-reconnects on its own, unlike a
// clean stream end (this stream's `Lagged` case), which IS a reestablish
// condition the browser retries by itself. Combined with the effect's
// `sources.has(serverRoute)` re-creation guard, a permanently-failed source
// was never replaced, silently defeating the lag -> reconnect -> snapshot
// resync that is this design's only resync path.
//
// This module stays DOM-free (see the header comment), so the numeric
// `EventSource.CLOSED` readyState (`2`) is named here rather than imported,
// and the predicate takes a plain `readyState: number` rather than an
// `EventSource` - the one piece of the fix worth unit-testing without a
// browser. `App.tsx`'s `onerror` handler calls this, and on `true` closes
// the dead source and removes it from the subscription map so the next
// `serversView` poll's effect re-run (`resourceAvailabilityPollIntervalMs`,
// 5s) recreates it - a natural backoff bound against hammering a
// persistently-401/502 server, since recreation only ever happens on the
// NEXT poll tick, never synchronously inside this handler. A transient
// `CONNECTING` readyState means `EventSource` is already retrying with its
// own backoff and must be left alone, or this would fight the browser's own
// reconnect logic.
export const ATTENTION_SOURCE_CLOSED_READY_STATE = 2;

export function shouldReplaceAttentionSourceOnError(readyState: number): boolean {
  return readyState === ATTENTION_SOURCE_CLOSED_READY_STATE;
}

// Ack-watermark map for the tab-label indicator (260725 Phase 6), keyed by
// the SAME `serverScopedIdentity(serverRoute, terminalId)` string
// `attentionByKey` uses, valued by the last acknowledged
// `AgentAttentionEntry.updatedAtMs`.
//
// CONTRACT: this mirrors `workRootActivity.ts`'s
// `ActivityAcknowledgements`/`initializeActivityDirtyItems` PATTERN (an id ->
// last-seen revision token map, "dirty" == "no ack, or ack token !== current
// token") rather than adding an `attention` field to the durable session
// model (`TerminalPaneState.session` / `TerminalSessionView` /
// `TerminalRegistryEntry`), which the ticket forbids. `updatedAtMs` is the
// revision token, playing `activityItemRevisionToken`'s role: acknowledging
// a `ready` at T1 must NOT suppress a later `ready` at T2.
export type AgentAttentionAcknowledgements = Record<string, number>;

// The single derivation the tab indicator renders from. `null` means "show
// nothing"; a state means "show that state's badge".
//
// Three independent suppressions, in order:
//  1. Session liveness (plan step 2, the chosen stale-indicator fix): only a
//     `"running"` session may show an indicator. The daemon keeps listing a
//     dead-but-in-grace-window terminal for up to 30s
//     (`terminal.rs::DAEMON_GRACE_WINDOW_MS`) and its `AttentionHub` entry
//     survives past that until an unrelated `insert()` runs its retain step,
//     so without this gate a terminal whose helper died mid-turn would keep
//     a stale `ready` badge indefinitely. `status` flips off `"running"` on
//     the very next `listTerminals` reconciliation after
//     `apply_helper_status`/`mark_ipc_closed` fire, so this render-time gate
//     closes the user-visible half of that gap with no daemon change.
//  2. `"idle"` is the explicit "nothing to show" state in the pinned
//     three-state vocabulary, never a badge.
//  3. The ack watermark: an entry whose `updatedAtMs` the user has already
//     acknowledged (by selecting that tab) is cleared until the NEXT state
//     change bumps `updatedAtMs`.
export function pendingAttentionStateFor(
  entry: AgentAttentionEntry | undefined,
  acknowledgedUpdatedAtMs: number | undefined,
  sessionStatus: string,
): AgentAttentionState | null {
  if (sessionStatus !== "running" || !entry || entry.state === "idle") {
    return null;
  }
  return acknowledgedUpdatedAtMs === entry.updatedAtMs ? null : entry.state;
}

// Records `entry.updatedAtMs` as acknowledged for `key`, returning the SAME
// object identity when nothing changes so a repeated selection of an
// already-acknowledged tab never re-renders (mirrors
// `acknowledgeActivityItem`'s role, with the no-op guard this map's
// setState-in-a-click-handler call site needs).
export function acknowledgeAttentionEntry(
  acknowledgements: AgentAttentionAcknowledgements,
  key: string,
  entry: AgentAttentionEntry | undefined,
): AgentAttentionAcknowledgements {
  if (!entry || acknowledgements[key] === entry.updatedAtMs) {
    return acknowledgements;
  }
  return { ...acknowledgements, [key]: entry.updatedAtMs };
}

// --- 260725 Phase 7: nav-row agent counter -----------------------------
//
// Per-work-root roll-up of the agent terminals open under that root, split
// into the pinned three-state vocabulary's two visible halves.
// `agents` counts every AGENT terminal mounted for the root regardless of
// turn state; `working`/`ready` count only those currently showing a pending
// state. `agents - working - ready` is therefore the idle/acknowledged
// remainder, not an error.
export type NavAttentionCounts = {
  agents: number;
  working: number;
  ready: number;
};

export const EMPTY_NAV_ATTENTION_COUNTS: NavAttentionCounts = {
  agents: 0,
  working: 0,
  ready: 0,
};

// CONTRACT (import direction): this is a MINIMAL STRUCTURAL restatement of
// the fields `TerminalPaneState.session` carries, declared here rather than
// imported from `workbench/terminalWorkbenchPane.ts`. That module is
// reachable from the `workbench/index.ts` barrel, and a back-import from a
// barrel-reachable module drags all of `App.tsx` into the NodeNext
// route-tests program (mental model `ws-web-dashboard` `## Common
// Mistakes`). The KEY STRINGS built below are byte-identical to
// `terminalAttentionKey`'s join, which is the only coupling that matters.
export type NavAttentionPane = {
  readonly serverRoute?: string;
  readonly terminalId: string;
  readonly workRootId: string;
  readonly status: string;
  // The Phase 2 pane-recorded spawn profile. `!= null` IS the agent
  // predicate this counter is pinned to - NOT "the daemon minted a hook
  // config" and NOT "this terminal has posted a hook event", either of which
  // reads zero for a freshly spawned agent that has not finished a turn.
  readonly profileId: string | null;
};

export type NavAttentionInput = {
  readonly attentionByKey: Readonly<Record<string, AgentAttentionEntry>>;
  readonly acknowledgements: AgentAttentionAcknowledgements;
};

// CONTRACT (260725 Phase 7, the phase's central decision): the count is
// derived by ITERATING PANES and classifying each through
// `pendingAttentionStateFor`, never by iterating `attentionByKey`. Three
// consequences, all load-bearing:
//   1. An `AgentAttentionEntry` carries no profile field, so a map-derived
//      count could not tell an agent terminal from a shell terminal.
//   2. A freshly spawned agent that has not finished a turn has no entry at
//      all, yet must still increment `agents` (the pinned carrier rule).
//   3. A dead agent's entry can outlive its session daemon-side (Phase 5
//      Result finding 1 - `AttentionHub` has no IPC-death hook). Iterating
//      panes makes such an entry structurally UNREACHABLE rather than merely
//      filtered, and the same holds for entries in work roots the browser
//      has closed.
// Routing every pane through `pendingAttentionStateFor` also means this
// counter reuses the tab indicator's ONE acknowledgement watermark: a row's
// badge clears exactly when its last pending child terminal is acknowledged,
// with no second watermark and no timer of its own.
export function aggregateNavAttentionCounts(
  panes: readonly NavAttentionPane[],
  attention: NavAttentionInput,
): Record<string, NavAttentionCounts> {
  const byRoot: Record<string, NavAttentionCounts> = {};
  for (const pane of panes) {
    if (pane.profileId == null) {
      continue;
    }
    const rootKey = serverScopedIdentity(pane.serverRoute, pane.workRootId);
    const counts = (byRoot[rootKey] ??= { agents: 0, working: 0, ready: 0 });
    counts.agents += 1;
    const terminalKey = serverScopedIdentity(pane.serverRoute, pane.terminalId);
    const pending = pendingAttentionStateFor(
      attention.attentionByKey[terminalKey],
      attention.acknowledgements[terminalKey],
      pane.status,
    );
    if (pending === "working") {
      counts.working += 1;
    } else if (pending === "ready") {
      counts.ready += 1;
    }
  }
  return byRoot;
}

// The pinned per-row priority: `ready` outranks `working` outranks none.
export function navAttentionTone(
  counts: NavAttentionCounts | undefined,
): "ready" | "working" | null {
  if (!counts) {
    return null;
  }
  if (counts.ready > 0) {
    return "ready";
  }
  if (counts.working > 0) {
    return "working";
  }
  return null;
}

// The pinned SERVER-row roll-up: the highest-priority state among that
// server's work roots, same `ready > working > none` order. Callers pass the
// root keys they know belong to the server rather than prefix-matching the
// map, so a server whose route string is a prefix of another's can never
// absorb its neighbour's rows.
export function aggregateNavAttentionTone(
  countsByRoot: Readonly<Record<string, NavAttentionCounts>>,
  rootKeys: readonly string[],
): "ready" | "working" | null {
  let tone: "ready" | "working" | null = null;
  for (const rootKey of rootKeys) {
    const candidate = navAttentionTone(countsByRoot[rootKey]);
    if (candidate === "ready") {
      return "ready";
    }
    if (candidate === "working") {
      tone = "working";
    }
  }
  return tone;
}

// Change-detector string for the count map, mirroring
// `terminalCountByRootSignature`'s role in `WorkbenchShell`: `terminalPanes`
// churns on every batched output-cursor flush, so the derived map must only
// be rebuilt (and pushed up to `App()`) when a root's actual agent/working/
// ready triple moves. The DERIVED counts go in, not the raw maps - an
// `attentionByKey` write that changes no visible count must not re-render
// the nav tree.
export function navAttentionCountsSignature(
  countsByRoot: Readonly<Record<string, NavAttentionCounts>>,
): string {
  return Object.entries(countsByRoot)
    .map(
      ([rootKey, counts]) =>
        `${rootKey}:${counts.agents}:${counts.working}:${counts.ready}`,
    )
    .sort()
    .join(",");
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isAgentAttentionState(value: unknown): value is AgentAttentionState {
  return value === "working" || value === "ready" || value === "idle";
}

// Parses a single `event: attention` frame body.
export function parseAgentAttentionEntry(
  value: unknown,
): AgentAttentionEntry | null {
  if (
    !isObject(value) ||
    typeof value.terminalId !== "string" ||
    typeof value.workRootId !== "string" ||
    !isAgentAttentionState(value.state) ||
    typeof value.updatedAtMs !== "number"
  ) {
    return null;
  }
  return {
    terminalId: value.terminalId,
    workRootId: value.workRootId,
    state: value.state,
    updatedAtMs: value.updatedAtMs,
  };
}

// Parses an `event: attentionSnapshot` frame body (`{ items: [...] }`),
// emitted once on every fresh connection so a reconnect never loses a
// pending state that changed while no stream was open. Returns `null` (not
// a partial list) if any item fails to parse, so a malformed snapshot never
// silently drops entries the caller would otherwise trust as complete.
export function parseAgentAttentionSnapshot(
  value: unknown,
): AgentAttentionEntry[] | null {
  if (!isObject(value) || !Array.isArray(value.items)) {
    return null;
  }
  const entries: AgentAttentionEntry[] = [];
  for (const raw of value.items) {
    const entry = parseAgentAttentionEntry(raw);
    if (!entry) {
      return null;
    }
    entries.push(entry);
  }
  return entries;
}
