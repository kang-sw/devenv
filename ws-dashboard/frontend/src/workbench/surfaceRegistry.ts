export type SurfaceKind =
  | "agent"
  | "persistentTerminal"
  | "editor"
  | "viewer"
  | "diff"
  | "diagnostics"
  | "eventsLog"
  | "taskView"
  | "inspector";

export type WorkbenchRowPolicy = "pinned" | "opened";

export type WorkbenchLifecycleOwner =
  | "browserAttachment"
  | "daemonProcess"
  | "daemonProjection"
  | "documentProvider";

export type WorkbenchClosePolicy =
  | "closeAttachment"
  | "detachDaemonResource"
  | "releaseProjection"
  | "deferToProvider";

export type WorkbenchCloseConfirmationPolicy =
  | "none"
  | "confirmSessionClose";

export type SurfaceRegistryEntry = {
  readonly kind: SurfaceKind;
  readonly label: string;
  readonly rowPolicy: WorkbenchRowPolicy;
  readonly lifecycleOwner: WorkbenchLifecycleOwner;
  readonly closePolicy: WorkbenchClosePolicy;
  readonly closeConfirmationPolicy: WorkbenchCloseConfirmationPolicy;
};

export type SurfaceRegistry = Readonly<Record<SurfaceKind, SurfaceRegistryEntry>>;

const defaultSurfaceRegistryEntries = {
  agent: {
    kind: "agent",
    label: "Agent",
    rowPolicy: "pinned",
    lifecycleOwner: "daemonProcess",
    closePolicy: "detachDaemonResource",
    closeConfirmationPolicy: "confirmSessionClose",
  },
  persistentTerminal: {
    kind: "persistentTerminal",
    label: "Terminal",
    rowPolicy: "pinned",
    lifecycleOwner: "daemonProcess",
    closePolicy: "detachDaemonResource",
    closeConfirmationPolicy: "confirmSessionClose",
  },
  editor: {
    kind: "editor",
    label: "Editor",
    rowPolicy: "opened",
    lifecycleOwner: "documentProvider",
    closePolicy: "deferToProvider",
    closeConfirmationPolicy: "none",
  },
  viewer: {
    kind: "viewer",
    label: "Viewer",
    rowPolicy: "opened",
    lifecycleOwner: "daemonProjection",
    closePolicy: "releaseProjection",
    closeConfirmationPolicy: "none",
  },
  diff: {
    kind: "diff",
    label: "Diff",
    rowPolicy: "opened",
    lifecycleOwner: "documentProvider",
    closePolicy: "deferToProvider",
    closeConfirmationPolicy: "none",
  },
  diagnostics: {
    kind: "diagnostics",
    label: "Diagnostics",
    rowPolicy: "opened",
    lifecycleOwner: "daemonProjection",
    closePolicy: "releaseProjection",
    closeConfirmationPolicy: "none",
  },
  eventsLog: {
    kind: "eventsLog",
    label: "Events",
    rowPolicy: "opened",
    lifecycleOwner: "daemonProjection",
    closePolicy: "releaseProjection",
    closeConfirmationPolicy: "none",
  },
  taskView: {
    kind: "taskView",
    label: "Task",
    rowPolicy: "opened",
    lifecycleOwner: "daemonProjection",
    closePolicy: "releaseProjection",
    closeConfirmationPolicy: "none",
  },
  inspector: {
    kind: "inspector",
    label: "Inspector",
    rowPolicy: "opened",
    lifecycleOwner: "browserAttachment",
    closePolicy: "closeAttachment",
    closeConfirmationPolicy: "none",
  },
} as const satisfies SurfaceRegistry;

export const defaultSurfaceKinds = Object.freeze(
  Object.keys(defaultSurfaceRegistryEntries) as SurfaceKind[],
);

export function defaultSurfaceRegistry(): SurfaceRegistry {
  return defaultSurfaceRegistryEntries;
}

export function surfaceRegistryEntry(kind: SurfaceKind): SurfaceRegistryEntry {
  return defaultSurfaceRegistryEntries[kind];
}
