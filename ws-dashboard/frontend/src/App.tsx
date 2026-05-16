import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { normalizeServerRouteLocation } from "./routeBasis";
import { defaultSurfaceRegistry, type SurfaceKind } from "./workbench";

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

type WorkbenchSelection = {
  workspace: WorkspaceView;
  root: WorkRootView;
  mainInstance: InstanceView | null;
  selectedInstance: InstanceView | null;
};

const resourceEndpoint = "/api/dashboard/resources";

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
  const workbenchSelection = useMemo(
    () => resolveWorkbenchSelection(resources, selectedId),
    [resources, selectedId],
  );

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
      <div className="shell-grid shell-grid-workbench">
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

        <section className="shell-panel shell-panel-workbench" aria-label="WorkRoot workbench">
          <WorkbenchShell
            commandLog={commandLog}
            error={error}
            loading={loading}
            resources={resources}
            selectedEntity={selectedEntity}
            selection={workbenchSelection}
            onCommand={executeCommand}
          />
        </section>
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

function WorkbenchShell({
  resources,
  selection,
  selectedEntity,
  commandLog,
  loading,
  error,
  onCommand,
}: {
  resources: DashboardResourcesView | null;
  selection: WorkbenchSelection | null;
  selectedEntity: ResourceEntity | null;
  commandLog: CommandEntry[];
  loading: boolean;
  error: string | null;
  onCommand: (commandId: string, payload: CommandPayload) => void;
}) {
  if (loading && !resources) {
    return <StatusPane title="Loading" detail="workbench resources" />;
  }

  if (error && !resources) {
    return <StatusPane title="Workbench unavailable" detail={error} />;
  }

  if (!resources || !selection) {
    return <StatusPane title="No workRoot" detail="select a workRoot or main instance" />;
  }

  const { workspace, root, mainInstance, selectedInstance } = selection;
  const supportEntity = selectedEntity ?? resourceEntityForWorkRoot(root);

  return (
    <div className="workbench-shell">
      <WorkbenchToolbar
        commandLog={commandLog}
        root={root}
        selectedEntity={selectedEntity}
        server={resources.server}
        workspace={workspace}
        onCommand={onCommand}
      />
      {error ? <InlineNotice tone="error" title="Refresh failed" detail={error} /> : null}
      {loading ? <InlineNotice tone="info" title="Refreshing" detail="resources" /> : null}
      <div className="workbench-splits" aria-label="Default two-split workbench preset">
        <WorkbenchSplitGroup title="Primary" description="durable workRoot surfaces">
          <SurfaceRow title="Pinned row">
            <SurfaceTile
              kind="agent"
              title={mainInstance?.label ?? "No main agent"}
              detail={mainInstance ? instanceSummary(mainInstance) : "waiting for main instance"}
              state={mainInstance?.state ?? root.state}
              meta={mainInstance ? [mainInstance.kind, mainInstance.interactionMode] : [kindLabel(root.kind)]}
            />
            <SurfaceTile
              kind="persistentTerminal"
              title="Persistent terminal"
              detail={`${root.label} command surface reserved`}
              state={root.state}
              meta={[root.status, kindLabel(root.kind)]}
            />
          </SurfaceRow>
          <SurfaceRow title="Opened row">
            <SurfaceTile
              kind="viewer"
              title={selectedInstance?.label ?? root.label}
              detail="selected resource projection"
              state={selectedInstance?.state ?? root.state}
              meta={[selectedInstance?.role ?? "workRoot", selectedInstance?.kind ?? root.status]}
            />
            <SubInstanceStrip mainInstance={mainInstance} />
          </SurfaceRow>
        </WorkbenchSplitGroup>

        <WorkbenchSplitGroup title="Support" description="opened inspection surfaces">
          <SurfaceRow title="Pinned row">
            <SurfaceTile
              kind="persistentTerminal"
              title="Support pinned reserve"
              detail="durable support slot reserved for later focused groups"
              state={root.state}
              meta={["reserved"]}
            />
          </SurfaceRow>
          <SurfaceRow title="Opened row">
            <SurfaceTile
              kind="editor"
              title="Editor / detail"
              detail={supportEntity ? `${supportEntity.type}: ${supportEntity.label}` : "no selection"}
              state={supportEntity?.state ?? root.state}
              meta={["fixture data"]}
            >
              {supportEntity ? <ResourceSummary entity={supportEntity} /> : null}
            </SurfaceTile>
            <SurfaceTile
              kind="taskView"
              title="Task view"
              detail="workRoot-scoped task surface reserved"
              state={root.state}
              meta={[`${root.mainInstances.length} main`]}
            />
            <SurfaceTile
              kind="diagnostics"
              title="Diagnostics / events"
              detail={root.state.error ?? "resource and command events"}
              state={root.state}
              meta={[root.state.stale ? "stale" : "current"]}
            />
            <SurfaceTile
              kind="inspector"
              title="Inspector"
              detail="dashboard-owned metadata surface"
              state={supportEntity?.state ?? root.state}
              meta={[supportEntity?.type ?? "workRoot"]}
            />
          </SurfaceRow>
        </WorkbenchSplitGroup>
      </div>
    </div>
  );
}

function WorkbenchToolbar({
  server,
  workspace,
  root,
  selectedEntity,
  commandLog,
  onCommand,
}: {
  server: ServerView;
  workspace: WorkspaceView;
  root: WorkRootView;
  selectedEntity: ResourceEntity | null;
  commandLog: CommandEntry[];
  onCommand: (commandId: string, payload: CommandPayload) => void;
}) {
  const toggles = ["viewer", "task", "diagnostics", "events", "layout"] as const;

  return (
    <div className="workbench-toolbar">
      <div className="workbench-breadcrumb" aria-label="Workbench breadcrumb">
        <span>{server.label}</span>
        <span>{workspace.label}</span>
        <strong>{root.label}</strong>
      </div>
      <div className="workbench-toolbar-meta">
        <StateBadge state={root.state} />
        <span className="meta-chip">{kindLabel(root.kind)}</span>
        <span className="meta-chip">{root.status}</span>
        {commandLog[0] ? <span className="meta-chip">last: {commandLog[0].commandId}</span> : null}
      </div>
      <div className="workbench-toolbar-actions" aria-label="Workbench toggles and actions">
        {toolbarActions(root, selectedEntity).map(({ action, entityId }) => (
          <button
            className="action-button"
            data-command-id={`resource.action.${action.id}`}
            disabled={!action.enabled}
            key={`${entityId}:${action.id}`}
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
        {toggles.map((toggle) => (
          <button
            className="action-button workbench-toggle"
            data-command-id={`workbench.toggle.${toggle}`}
            key={toggle}
            type="button"
            onClick={() =>
              onCommand(`workbench.toggle.${toggle}`, {
                type: "action",
                label: toggle,
                entityId: root.id,
              })
            }
          >
            {toggle}
          </button>
        ))}
      </div>
    </div>
  );
}

function toolbarActions(root: WorkRootView, selectedEntity: ResourceEntity | null) {
  const actions = root.actions.map((action) => ({ action, entityId: root.id }));

  if (selectedEntity && selectedEntity.id !== root.id) {
    actions.push(
      ...selectedEntity.actions.map((action) => ({
        action,
        entityId: selectedEntity.id,
      })),
    );
  }

  return actions;
}

function WorkbenchSplitGroup({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <section className="workbench-group" aria-label={`${title} split group`}>
      <div className="workbench-group-header">
        <div>
          <div className="section-label">split group</div>
          <h2>{title}</h2>
        </div>
        <span>{description}</span>
      </div>
      <div className="workbench-group-rows">{children}</div>
    </section>
  );
}

function SurfaceRow({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="surface-row">
      <div className="surface-row-label">{title}</div>
      <div className="surface-row-items">{children}</div>
    </div>
  );
}

function SurfaceTile({
  kind,
  title,
  detail,
  state,
  meta,
  children,
}: {
  kind: SurfaceKind;
  title: string;
  detail: string;
  state: ViewState;
  meta: string[];
  children?: ReactNode;
}) {
  const registry = defaultSurfaceRegistry()[kind];

  return (
    <article className="surface-tile" data-surface-kind={kind}>
      <div className="surface-tile-header">
        <div>
          <div className="surface-kind">{registry.label}</div>
          <h3>{title}</h3>
        </div>
        <StateBadge state={state} />
      </div>
      <p>{detail}</p>
      <div className="surface-meta">
        <span className="meta-chip">{registry.rowPolicy}</span>
        {meta.map((value) => (
          <span className="meta-chip" key={value}>
            {value}
          </span>
        ))}
      </div>
      {children ? <div className="surface-body">{children}</div> : null}
    </article>
  );
}

function SubInstanceStrip({ mainInstance }: { mainInstance: InstanceView | null }) {
  if (!mainInstance || mainInstance.subInstances.length === 0) {
    return (
      <div className="surface-tile surface-tile-muted">
        <div className="surface-kind">Sub projections</div>
        <p>No sub instances attached to this main surface.</p>
      </div>
    );
  }

  return (
    <div className="surface-tile surface-tile-muted">
      <div className="surface-kind">Sub projections</div>
      <div className="subinstance-list">
        {mainInstance.subInstances.map((instance) => (
          <div className="subinstance-pill" key={instance.id}>
            <span>{instance.label}</span>
            <StateBadge state={instance.state} />
          </div>
        ))}
      </div>
    </div>
  );
}

function ResourceSummary({ entity }: { entity: ResourceEntity }) {
  return (
    <dl className="resource-summary">
      <DetailItem label="id" value={entity.id} />
      <DetailItem label="status" value={entity.state.status} />
      {entity.type === "workRoot" ? <DetailItem label="workRoot" value={entity.path.workRootId} /> : null}
      {entity.type === "instance" ? <DetailItem label="instance" value={entity.path.instanceId ?? ""} /> : null}
    </dl>
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

  const normalizedPath = normalizeServerRouteLocation(window.location, serverId);
  if (normalizedPath) {
    window.history.replaceState(null, "", normalizedPath);
  }
}

function resolveWorkbenchSelection(
  resources: DashboardResourcesView | null,
  selectedId: string | null,
): WorkbenchSelection | null {
  if (!resources) {
    return null;
  }

  let fallback: WorkbenchSelection | null = null;

  for (const workspace of resources.workspaces) {
    for (const root of workspace.workRoots) {
      const mainInstance = root.mainInstances[0] ?? null;
      const rootSelection = { workspace, root, mainInstance, selectedInstance: mainInstance };
      fallback ??= rootSelection;

      if (selectedId === workspace.id || selectedId === root.id) {
        return rootSelection;
      }

      for (const main of root.mainInstances) {
        const selectedInstance = findInstanceById(main, selectedId);
        if (selectedInstance) {
          return { workspace, root, mainInstance: main, selectedInstance };
        }
      }
    }
  }

  return fallback;
}

function findInstanceById(instance: InstanceView, selectedId: string | null): InstanceView | null {
  if (!selectedId) {
    return null;
  }

  if (instance.id === selectedId) {
    return instance;
  }

  for (const subInstance of instance.subInstances) {
    const nested = findInstanceById(subInstance, selectedId);
    if (nested) {
      return nested;
    }
  }

  return null;
}

function resourceEntityForWorkRoot(root: WorkRootView): ResourceEntity {
  return {
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
  };
}

function instanceSummary(instance: InstanceView) {
  return `${instance.role} ${instance.kind} · ${instance.interactionMode}`;
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
