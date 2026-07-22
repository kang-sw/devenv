import { normalizeServerRouteLocation } from "./routeBasis.js";
import { decideSurfaceClose, type SurfaceKind } from "./workbench/index.js";
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
