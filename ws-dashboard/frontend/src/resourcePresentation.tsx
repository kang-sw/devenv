import { normalizeServerRouteLocation } from "./routeBasis.js";
// Import from the narrow policy/surfaceRegistry modules rather than the
// workbench barrel (./workbench/index.js): the barrel re-exports
// dockviewLayout.tsx, which pulls in the dockview package's CSS import -
// fine for the Vite-bundled app, but it breaks this module's inclusion in
// the plain-Node route-tests runner (tsconfig.route-tests.json), which has
// no CSS loader. Same runtime behavior, narrower module graph.
import { decideSurfaceClose } from "./workbench/policy.js";
import {
  EMPTY_NAV_ATTENTION_COUNTS,
  type NavAttentionCounts,
} from "./agentAttention.js";
import type { SurfaceKind } from "./workbench/surfaceRegistry.js";
import type {
  InstanceView,
  ResourceEntity,
  ViewState,
  WorkRootView,
} from "./resourceModel.js";

export function DetailItem({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <div className="detail-item">
      <dt>{label}</dt>
      <dd>{value || "none"}</dd>
    </div>
  );
}

export function StateLine({ state }: { state: ViewState }) {
  return (
    <div className="state-line">
      <StateDot state={state} />
      <span>{state.status}</span>
      {state.stale ? <span>stale</span> : null}
      {state.loading ? <span>loading</span> : null}
    </div>
  );
}

export function StateBadge({ state }: { state: ViewState }) {
  return (
    <span
      className={`state-badge ws-badge ${state.loading ? "state-loading" : ""} ${
        state.stale ? "state-stale" : ""
      } ${state.error ? "state-error" : ""}`}
    >
      <StateDot state={state} />
      {state.status}
    </span>
  );
}

export function StateDot({ state }: { state: ViewState }) {
  return (
    <span
      className={`state-dot ws-state-dot ${state.loading ? "state-loading" : ""} ${
        state.stale ? "state-stale" : ""
      } ${state.error ? "state-error" : ""}`}
      aria-hidden="true"
    />
  );
}

export function normalizeServerRoute(serverRoute: string) {
  if (typeof window === "undefined") {
    return;
  }

  const normalizedPath = normalizeServerRouteLocation(
    window.location,
    serverRoute,
  );
  if (normalizedPath) {
    window.history.replaceState(null, "", normalizedPath);
  }
}

export function resourceEntityForWorkRoot(root: WorkRootView): ResourceEntity {
  return {
    id: root.id,
    type: "workRoot",
    label: root.label,
    state: root.state,
    actions: root.actions,
    compactable: root.compactable,
    path: root.resourcePath,
    kind: root.kind,
    activation: root.activation,
    availability: root.availability,
    status: root.status,
    instanceCount: root.mainInstances.length,
  };
}

export function instanceSummary(instance: InstanceView) {
  return `${instance.role} ${instance.kind} · ${instance.interactionMode}`;
}

export function closeContractLabel(kind: SurfaceKind) {
  return `close: ${decideSurfaceClose(kind).behavior}`;
}

export function resourcePresentationLabel(
  presentation: "compactWorkRoot" | "workspace" | "workRoot",
) {
  switch (presentation) {
    case "compactWorkRoot":
      return "compact workRoot";
    case "workspace":
      return "workspace";
    case "workRoot":
      return "workRoot";
  }
}

export function resourceRowTone(
  state: ViewState,
  availability?: WorkRootView["availability"],
  activation?: WorkRootView["activation"],
) {
  if (
    state.error ||
    availability === "inaccessible" ||
    availability === "missing"
  ) {
    return "error";
  }
  if (state.stale || availability === "moved" || activation === "offline") {
    return "muted";
  }
  return "ready";
}

export function kindLabel(kind: WorkRootView["kind"]) {
  switch (kind) {
    case "gitPrimaryRoot":
      return "git root";
    case "gitLinkedWorktree":
      return "worktree";
    case "plainDirectory":
      return "directory";
  }
}

// 260725 nav-row-two-line-open-state Phase 1: shared per-root grouping used
// by both the document-count map (App(), synchronous useMemo over
// readOnlyFilePanes) and the terminal-count map (WorkbenchShell, signature-
// gated per its own churn-avoidance comment) - see the ticket's Decision 1.
// Pure and side-effect free so it is unit-testable without React.
export function countByRootKey<T>(
  items: readonly T[],
  keyOf: (item: T) => string,
): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const item of items) {
    const key = keyOf(item);
    counts[key] = (counts[key] ?? 0) + 1;
  }
  return counts;
}

// The nav row's reserved second line (260725 Phase 1) always renders for
// workRoot/compactWorkRoot rows regardless of open state - only its text
// varies, per the ticket's no-height-jump constraint - so this needs a
// single formatter covering both the "nothing open yet" and populated cases.
//
// 260725 Phase 7 adds the agent split as a THIRD, optional argument rather
// than a required one, deliberately: the agent segment is appended only when
// `agents > 0`, so both zero-agent strings ("no open surfaces" /
// "N terminals, M documents") stay BYTE-IDENTICAL to Phase 1's. That is what
// keeps `dashboard-acceptance.spec.ts`'s existing two-argument call sites
// (:2862/:2914/:2927, which compare row text against this function's own
// output) correct rather than merely compiling. The terminal count passed in
// EXCLUDES agent terminals (see `terminalCountByRoot`'s profileId filter in
// App.tsx) - an agent pane is counted in the agent segment only, never in
// both halves.
export function formatOpenSurfaceCounts(
  terminalCount: number,
  documentCount: number,
  agentCounts: NavAttentionCounts = EMPTY_NAV_ATTENTION_COUNTS,
): string {
  const surfaces =
    terminalCount === 0 && documentCount === 0
      ? "no open surfaces"
      : `${terminalCount} terminal${terminalCount === 1 ? "" : "s"}, ${documentCount} document${documentCount === 1 ? "" : "s"}`;
  if (agentCounts.agents === 0) {
    return surfaces;
  }
  // Both halves always render once there is at least one agent (working
  // before ready), so the Phase 3 turn-start spike's `working` spinner is
  // visible rather than being collapsed away whenever it happens to be zero.
  return `${surfaces} · ${agentCounts.agents} agent${agentCounts.agents === 1 ? "" : "s"}: ${agentCounts.working} working, ${agentCounts.ready} ready`;
}
