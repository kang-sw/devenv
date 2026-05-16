import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";

type ViewState = {
  status: string;
  loading: boolean;
  stale: boolean;
  error: string | null;
};

type ActionHint = {
  id: string;
  label: string;
  enabled: boolean;
};

type ResourcePath = {
  serverId: string;
  workspaceId: string;
  workRootId: string;
  instanceId: string | null;
};

type ServerView = {
  id: string;
  label: string;
  state: ViewState;
  actions: ActionHint[];
};

type WorkspaceView = {
  id: string;
  label: string;
  state: ViewState;
  compactable: boolean;
  workRoots: WorkRootView[];
  actions: ActionHint[];
};

type WorkRootView = {
  id: string;
  resourcePath: ResourcePath;
  label: string;
  kind: "plainDirectory" | "gitPrimaryRoot" | "gitLinkedWorktree";
  status: "online" | "offline" | "moved" | "inaccessible";
  state: ViewState;
  compactable: boolean;
  mainInstances: InstanceView[];
  actions: ActionHint[];
};

type InstanceView = {
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

type DashboardResourcesView = {
  server: ServerView;
  workspaces: WorkspaceView[];
};

type ResourceEntity =
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

type CommandPayload =
  | { type: "select"; entityId: string }
  | { type: "action"; label: string; entityId: string }
  | { type: "refresh" };

type CommandEntry = {
  id: number;
  commandId: string;
  label: string;
};

const resourceEndpoint = "/api/dashboard/resources";
const serverRoutePrefix = "/servers";

export function App() {
  const [resources, setResources] = useState<DashboardResourcesView | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [commandLog, setCommandLog] = useState<CommandEntry[]>([]);
  const commandSequence = useRef(0);

  const loadResources = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const response = await fetch(resourceEndpoint, {
        headers: { Accept: "application/json" },
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const nextResources = (await response.json()) as DashboardResourcesView;
      setResources(nextResources);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "request failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadResources();
  }, [loadResources]);

  useEffect(() => {
    if (!resources) {
      return;
    }

    normalizeServerRoute(resources.server.id);
  }, [resources]);

  const entities = useMemo(() => flattenEntities(resources), [resources]);

  useEffect(() => {
    if (
      entities.length > 0 &&
      (!selectedId || !entities.some((entity) => entity.id === selectedId))
    ) {
      setSelectedId(preferredSelection(entities));
    }
  }, [entities, selectedId]);

  const selectedEntity =
    entities.find((entity) => entity.id === selectedId) ?? entities[0] ?? null;

  const executeCommand = useCallback(
    (commandId: string, payload: CommandPayload) => {
      if (payload.type === "select") {
        setSelectedId(payload.entityId);
      }

      if (payload.type === "refresh") {
        void loadResources();
      }

      setCommandLog((entries) =>
        [
          {
            id: commandSequence.current++,
            commandId,
            label:
              payload.type === "action"
                ? payload.label
                : payload.type === "refresh"
                  ? "Refresh"
                  : "Select",
          },
          ...entries,
        ].slice(0, 6),
      );
    },
    [loadResources],
  );

  return (
    <main className="app-shell" aria-label="ws dashboard">
      <div className="shell-grid">
        <aside className="shell-panel shell-panel-nav" aria-label="Resources">
          <PanelHeader
            title={resources?.server.label ?? "ws dashboard"}
            state={resources?.server.state}
            actions={resources?.server.actions ?? []}
            entityId={resources?.server.id ?? "server"}
            onCommand={executeCommand}
          />
          <ResourceNavigation
            resources={resources}
            loading={loading}
            error={error}
            selectedId={selectedEntity?.id ?? null}
            onCommand={executeCommand}
          />
        </aside>

        <section className="shell-panel shell-panel-main" aria-label="Detail">
          <PanelHeader
            title="Resource"
            state={selectedEntity?.state}
            actions={selectedEntity?.actions ?? []}
            entityId={selectedEntity?.id ?? "selection"}
            onCommand={executeCommand}
          />
          <ResourceDetail entity={selectedEntity} loading={loading} error={error} />
        </section>

        <aside className="shell-panel shell-panel-viewer" aria-label="Viewer">
          <PanelHeader title="Viewer" />
          <ViewerReserve entity={selectedEntity} commandLog={commandLog} />
        </aside>
      </div>
    </main>
  );
}

function PanelHeader({
  title,
  state,
  actions = [],
  entityId = "panel",
  onCommand,
}: {
  title: string;
  state?: ViewState;
  actions?: ActionHint[];
  entityId?: string;
  onCommand?: (commandId: string, payload: CommandPayload) => void;
}) {
  return (
    <div className="panel-header">
      <div className="panel-title-block">
        <div className="panel-title">{title}</div>
        {state ? <StateLine state={state} /> : null}
      </div>
      {actions.length > 0 && onCommand ? (
        <div className="action-strip">
          {actions.map((action) => (
            <button
              className="action-button"
              data-command-id={`resource.action.${action.id}`}
              disabled={!action.enabled}
              key={action.id}
              title={action.label}
              type="button"
              onClick={() =>
                onCommand(`resource.action.${action.id}`, {
                  type: action.id === "refresh" ? "refresh" : "action",
                  label: action.label,
                  entityId,
                })
              }
            >
              {action.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ResourceNavigation({
  resources,
  loading,
  error,
  selectedId,
  onCommand,
}: {
  resources: DashboardResourcesView | null;
  loading: boolean;
  error: string | null;
  selectedId: string | null;
  onCommand: (commandId: string, payload: CommandPayload) => void;
}) {
  if (loading && !resources) {
    return <StatusPane title="Loading" detail="resources" />;
  }

  if (error && !resources) {
    return (
      <StatusPane
        title="Fetch failed"
        detail={error}
        action={
          <button
            className="action-button action-button-primary"
            data-command-id="dashboard.refresh"
            type="button"
            onClick={() => onCommand("dashboard.refresh", { type: "refresh" })}
          >
            Refresh
          </button>
        }
      />
    );
  }

  if (!resources || resources.workspaces.length === 0) {
    return <StatusPane title="Empty" detail="no workspaces" />;
  }

  return (
    <div className="resource-list">
      {error ? <InlineNotice tone="error" title="Refresh failed" detail={error} /> : null}
      {loading ? <InlineNotice tone="info" title="Refreshing" detail="resources" /> : null}
      {resources.workspaces.map((workspace) => (
        <WorkspaceRows
          key={workspace.id}
          workspace={workspace}
          selectedId={selectedId}
          onCommand={onCommand}
        />
      ))}
    </div>
  );
}

function InlineNotice({
  tone,
  title,
  detail,
}: {
  tone: "error" | "info";
  title: string;
  detail: string;
}) {
  return (
    <div className={`inline-notice inline-notice-${tone}`} role="status">
      <strong>{title}</strong>
      <span>{detail}</span>
    </div>
  );
}

function WorkspaceRows({
  workspace,
  selectedId,
  onCommand,
}: {
  workspace: WorkspaceView;
  selectedId: string | null;
  onCommand: (commandId: string, payload: CommandPayload) => void;
}) {
  const compactMain = compactMainInstance(workspace);

  if (compactMain) {
    return (
      <div className="resource-group">
        <ResourceRow
          id={compactMain.id}
          title={`${workspace.label} / ${compactMain.root.label} / ${compactMain.instance.label}`}
          eyebrow="compact"
          state={compactMain.instance.state}
          depth={0}
          selected={selectedId === compactMain.id}
          meta={[kindLabel(compactMain.root.kind), compactMain.instance.kind]}
          onCommand={onCommand}
        />
      </div>
    );
  }

  return (
    <div className="resource-group">
      <ResourceRow
        id={workspace.id}
        title={workspace.label}
        eyebrow="workspace"
        state={workspace.state}
        depth={0}
        selected={selectedId === workspace.id}
        meta={[`${workspace.workRoots.length} roots`]}
        onCommand={onCommand}
      />
      {workspace.workRoots.map((root) => (
        <div key={root.id}>
          <ResourceRow
            id={root.id}
            title={root.label}
            eyebrow="workRoot"
            state={root.state}
            depth={1}
            selected={selectedId === root.id}
            meta={[kindLabel(root.kind), root.status]}
            onCommand={onCommand}
          />
          {root.mainInstances.map((instance) => (
            <InstanceRows
              depth={2}
              instance={instance}
              key={instance.id}
              selectedId={selectedId}
              onCommand={onCommand}
            />
          ))}
        </div>
      ))}
    </div>
  );
}

function InstanceRows({
  instance,
  selectedId,
  depth,
  onCommand,
}: {
  instance: InstanceView;
  selectedId: string | null;
  depth: number;
  onCommand: (commandId: string, payload: CommandPayload) => void;
}) {
  return (
    <>
      <ResourceRow
        id={instance.id}
        title={instance.label}
        eyebrow={instance.role === "main" ? "mainInstance" : "subInstance"}
        state={instance.state}
        depth={depth}
        selected={selectedId === instance.id}
        meta={[instance.kind, instance.interactionMode]}
        onCommand={onCommand}
      />
      {instance.subInstances.map((subInstance) => (
        <InstanceRows
          depth={depth + 1}
          instance={subInstance}
          key={subInstance.id}
          selectedId={selectedId}
          onCommand={onCommand}
        />
      ))}
    </>
  );
}

function ResourceRow({
  id,
  title,
  eyebrow,
  state,
  depth,
  selected,
  meta,
  onCommand,
}: {
  id: string;
  title: string;
  eyebrow: string;
  state: ViewState;
  depth: number;
  selected: boolean;
  meta: string[];
  onCommand: (commandId: string, payload: CommandPayload) => void;
}) {
  return (
    <button
      className={`resource-row${selected ? " resource-row-selected" : ""}`}
      data-command-id="resource.select"
      style={{ "--depth": depth } as CSSProperties}
      type="button"
      onClick={() => onCommand("resource.select", { type: "select", entityId: id })}
    >
      <span className="resource-row-main">
        <span className="row-eyebrow">{eyebrow}</span>
        <span className="row-title">{title}</span>
      </span>
      <span className="resource-row-meta">
        {meta.map((value) => (
          <span className="meta-chip" key={value}>
            {value}
          </span>
        ))}
        <StateBadge state={state} />
      </span>
    </button>
  );
}

function ResourceDetail({
  entity,
  loading,
  error,
}: {
  entity: ResourceEntity | null;
  loading: boolean;
  error: string | null;
}) {
  if (loading && !entity) {
    return <StatusPane title="Loading" detail="detail" />;
  }

  if (error && !entity) {
    return <StatusPane title="Unavailable" detail={error} />;
  }

  if (!entity) {
    return <StatusPane title="Empty" detail="no selection" />;
  }

  return (
    <div className="detail-body">
      <div className="detail-heading">
        <div>
          <div className="detail-type">{entity.type}</div>
          <h1>{entity.label}</h1>
        </div>
        <StateBadge state={entity.state} />
      </div>

      <dl className="detail-grid">
        <DetailItem label="id" value={entity.id} />
        <DetailItem label="status" value={entity.state.status} />
        <DetailItem label="loading" value={String(entity.state.loading)} />
        <DetailItem label="stale" value={String(entity.state.stale)} />
        {entity.type === "workspace" ? (
          <>
            <DetailItem label="compactable" value={String(entity.compactable)} />
            <DetailItem label="workRoots" value={String(entity.workRootCount)} />
          </>
        ) : null}
        {entity.type === "workRoot" ? (
          <>
            <DetailItem label="kind" value={kindLabel(entity.kind)} />
            <DetailItem label="workRootStatus" value={entity.status} />
            <DetailItem label="instances" value={String(entity.instanceCount)} />
            <DetailItem label="workRootId" value={entity.path.workRootId} />
          </>
        ) : null}
        {entity.type === "instance" ? (
          <>
            <DetailItem label="role" value={entity.role} />
            <DetailItem label="kind" value={entity.kind} />
            <DetailItem label="mode" value={entity.interactionMode} />
            <DetailItem label="subInstances" value={String(entity.subInstanceCount)} />
            <DetailItem label="instanceId" value={entity.path.instanceId ?? ""} />
          </>
        ) : null}
      </dl>

      {entity.state.error ? (
        <div className="detail-alert" role="status">
          {entity.state.error}
        </div>
      ) : null}
    </div>
  );
}

function ViewerReserve({
  entity,
  commandLog,
}: {
  entity: ResourceEntity | null;
  commandLog: CommandEntry[];
}) {
  return (
    <div className="viewer-body">
      <div className="viewer-target">
        <div className="detail-type">selection</div>
        <div className="viewer-title">{entity?.label ?? "none"}</div>
        <div className="viewer-subtitle">{entity?.type ?? "empty"}</div>
      </div>

      <div className="command-log">
        <div className="section-label">commands</div>
        {commandLog.length === 0 ? (
          <div className="log-empty">none</div>
        ) : (
          commandLog.map((entry) => (
            <div className="log-row" key={entry.id}>
              <span>{entry.commandId}</span>
              <strong>{entry.label}</strong>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function StatusPane({
  title,
  detail,
  action,
}: {
  title: string;
  detail: string;
  action?: ReactNode;
}) {
  return (
    <div className="status-pane">
      <div className="status-title">{title}</div>
      <div className="status-detail">{detail}</div>
      {action ? <div className="status-action">{action}</div> : null}
    </div>
  );
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="detail-item">
      <dt>{label}</dt>
      <dd>{value || "none"}</dd>
    </div>
  );
}

function StateLine({ state }: { state: ViewState }) {
  return (
    <div className="state-line">
      <StateDot state={state} />
      <span>{state.status}</span>
      {state.stale ? <span>stale</span> : null}
      {state.loading ? <span>loading</span> : null}
    </div>
  );
}

function StateBadge({ state }: { state: ViewState }) {
  return (
    <span
      className={`state-badge ${state.loading ? "state-loading" : ""} ${
        state.stale ? "state-stale" : ""
      } ${state.error ? "state-error" : ""}`}
    >
      <StateDot state={state} />
      {state.status}
    </span>
  );
}

function StateDot({ state }: { state: ViewState }) {
  return (
    <span
      className={`state-dot ${state.loading ? "state-loading" : ""} ${
        state.stale ? "state-stale" : ""
      } ${state.error ? "state-error" : ""}`}
      aria-hidden="true"
    />
  );
}

function flattenEntities(resources: DashboardResourcesView | null): ResourceEntity[] {
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
        status: root.status,
        instanceCount: root.mainInstances.length,
      });

      for (const instance of root.mainInstances) {
        appendInstanceEntities(entities, instance);
      }
    }
  }

  return entities;
}

function appendInstanceEntities(entities: ResourceEntity[], instance: InstanceView) {
  entities.push({
    id: instance.id,
    type: "instance",
    label: instance.label,
    state: instance.state,
    actions: instance.actions,
    path: instance.resourcePath,
    role: instance.role,
    kind: instance.kind,
    interactionMode: instance.interactionMode,
    subInstanceCount: instance.subInstances.length,
  });

  for (const subInstance of instance.subInstances) {
    appendInstanceEntities(entities, subInstance);
  }
}

function normalizeServerRoute(serverId: string) {
  if (typeof window === "undefined") {
    return;
  }

  const targetPath = serverRoutePath(serverId);
  const currentPath = window.location.pathname;

  if (currentPath === targetPath || currentPath.startsWith(`${targetPath}/`)) {
    return;
  }

  if (
    currentPath === "/" ||
    currentPath === serverRoutePrefix ||
    currentPath.startsWith(`${serverRoutePrefix}/`)
  ) {
    window.history.replaceState(
      null,
      "",
      `${targetPath}${window.location.search}${window.location.hash}`,
    );
  }
}

function serverRoutePath(serverId: string) {
  return `${serverRoutePrefix}/${encodeURIComponent(serverId)}`;
}

function preferredSelection(entities: ResourceEntity[]) {
  return (
    entities.find(
      (entity) => entity.type === "instance" && entity.role === "main",
    )?.id ?? entities[0]?.id
  );
}

function compactMainInstance(workspace: WorkspaceView) {
  if (!workspace.compactable || workspace.workRoots.length !== 1) {
    return null;
  }

  const root = workspace.workRoots[0];
  if (!root.compactable || root.mainInstances.length !== 1) {
    return null;
  }

  const instance = root.mainInstances[0];
  if (instance.subInstances.length > 0) {
    return null;
  }

  return { id: instance.id, root, instance };
}

function kindLabel(kind: WorkRootView["kind"]) {
  switch (kind) {
    case "gitPrimaryRoot":
      return "git root";
    case "gitLinkedWorktree":
      return "worktree";
    case "plainDirectory":
      return "directory";
  }
}
