// Pure dashboard resource view-model: the JSON contract returned by
// GET /api/dashboard/resources plus the flattened navigation entity model.
//
// This module is intentionally free of React and DOM dependencies so the
// mock-fixture -> live opened-workRoot transition is testable as pure logic.

export type ViewState = {
  status: string;
  loading: boolean;
  stale: boolean;
  error: string | null;
};

export type ActionHint = {
  id: string;
  label: string;
  enabled: boolean;
};

export type ResourcePath = {
  serverId: string;
  workspaceId: string;
  workRootId: string;
  instanceId: string | null;
};

export type ServerView = {
  id: string;
  label: string;
  state: ViewState;
  actions: ActionHint[];
};

export type WorkspaceView = {
  id: string;
  label: string;
  state: ViewState;
  compactable: boolean;
  workRoots: WorkRootView[];
  actions: ActionHint[];
};

export type WorkRootView = {
  id: string;
  resourcePath: ResourcePath;
  label: string;
  kind: "plainDirectory" | "gitPrimaryRoot" | "gitLinkedWorktree";
  activation: "online" | "offline";
  availability: "available" | "missing" | "moved" | "inaccessible" | "unknown";
  status: "online" | "offline" | "moved" | "inaccessible";
  state: ViewState;
  compactable: boolean;
  mainInstances: InstanceView[];
  actions: ActionHint[];
};

export type InstanceView = {
  id: string;
  resourcePath: ResourcePath;
  role: "main" | "sub";
  kind:
    | "harness"
    | "agent"
    | "terminal"
    | "editor"
    | "viewer"
    | "exec"
    | "translation"
    | "task";
  interactionMode: "direct" | "delegated" | "passive";
  label: string;
  state: ViewState;
  subInstances: InstanceView[];
  actions: ActionHint[];
};

export type DashboardResourcesView = {
  server: ServerView;
  workspaces: WorkspaceView[];
};

export type ResourceEntity =
  | {
      id: string;
      type: "server";
      label: string;
      state: ViewState;
      actions: ActionHint[];
    }
  | {
      id: string;
      type: "workspace";
      label: string;
      state: ViewState;
      actions: ActionHint[];
      compactable: boolean;
      workRootCount: number;
    }
  | {
      id: string;
      type: "workRoot";
      label: string;
      state: ViewState;
      actions: ActionHint[];
      compactable: boolean;
      path: ResourcePath;
      kind: WorkRootView["kind"];
      activation: WorkRootView["activation"];
      availability: WorkRootView["availability"];
      status: WorkRootView["status"];
      instanceCount: number;
    }
  | {
      id: string;
      type: "instance";
      label: string;
      state: ViewState;
      actions: ActionHint[];
      path: ResourcePath;
      role: InstanceView["role"];
      kind: InstanceView["kind"];
      interactionMode: InstanceView["interactionMode"];
      subInstanceCount: number;
    };

// A dashboard with exactly one workspace containing exactly one workRoot is
// rendered as one compact workRoot row in the browser left nav. The selected
// location remains the concrete workRoot id; main/sub instances are workbench
// surfaces and do not participate in this presentation decision.
export function compactWorkspaceWorkRoot(
  workspace: WorkspaceView,
  workspaceCount: number,
): WorkRootView | null {
  if (workspaceCount !== 1 || workspace.workRoots.length !== 1) {
    return null;
  }

  return workspace.workRoots[0];
}

// Flatten the resource hierarchy into the left-nav entity rows.
//
// Main and sub instances are workbench surfaces/projections, not default
// left-nav rows, so they are intentionally omitted here.
export function flattenEntities(
  resources: DashboardResourcesView | null,
): ResourceEntity[] {
  if (!resources) {
    return [];
  }

  const entities: ResourceEntity[] = [
    {
      id: resources.server.id,
      type: "server",
      label: resources.server.label,
      state: resources.server.state,
      actions: resources.server.actions,
    },
  ];

  for (const workspace of resources.workspaces) {
    entities.push({
      id: workspace.id,
      type: "workspace",
      label: workspace.label,
      state: workspace.state,
      actions: workspace.actions,
      compactable: workspace.compactable,
      workRootCount: workspace.workRoots.length,
    });

    for (const root of workspace.workRoots) {
      entities.push({
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
      });
    }
  }

  return entities;
}

// The entity that should be selected by default: the first workRoot row, or
// the first entity (the server) when no workRoot exists yet. Returns undefined
// only for an empty entity list.
export function preferredSelection(entities: ResourceEntity[]): string | undefined {
  return (
    entities.find((entity) => entity.type === "workRoot")?.id ??
    entities[0]?.id
  );
}

// Reconcile the active selection after the resource tree changes.
//
// Keeps a still-present selection, otherwise falls back to `preferredSelection`
// so a selection that left the entity set (for example the mock workspace
// after the tree turns live) cannot remain active.
export function reconcileSelectedId(
  entities: ResourceEntity[],
  selectedId: string | null,
): string | null {
  if (entities.length === 0) {
    return selectedId;
  }

  if (selectedId && entities.some((entity) => entity.id === selectedId)) {
    return selectedId;
  }

  return preferredSelection(entities) ?? null;
}
