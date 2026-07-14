import type { DashboardResourcesView, DashboardServersView } from "./resourceModel.js";

export const resourceEndpoint = "/api/dashboard/resources";
export const serversEndpoint = "/api/dashboard/servers";
export const resourceAvailabilityPollIntervalMs = 5_000;

export type ResourceRefreshReason = "initial" | "explicit" | "poll" | "open";

export type ResourceFetch = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

export async function requestDashboardResources(
  serverId = "server-local",
  fetchResource: ResourceFetch = fetch,
): Promise<DashboardResourcesView> {
  const endpoint =
    serverId === "server-local"
      ? resourceEndpoint
      : `/api/dashboard/servers/${encodeURIComponent(serverId)}/resources`;
  const response = await fetchResource(endpoint, { headers: { Accept: "application/json" } });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return (await response.json()) as DashboardResourcesView;
}

export async function requestDashboardServers(
  fetchResource: ResourceFetch = fetch,
): Promise<DashboardServersView> {
  const response = await fetchResource(serversEndpoint, {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return (await response.json()) as DashboardServersView;
}

type ResourceRefreshCoordinatorOptions = {
  fetchResources?: () => Promise<DashboardResourcesView>;
  applyResources: (resources: DashboardResourcesView) => void;
  setLoading?: (loading: boolean) => void;
  setError?: (error: string | null) => void;
};

export type ResourceRefreshResult =
  | { status: "applied"; reason: ResourceRefreshReason }
  | { status: "failed"; reason: ResourceRefreshReason; error: string }
  | { status: "skipped"; reason: ResourceRefreshReason; cause: "disposed" | "inFlight" }
  | { status: "stale"; reason: ResourceRefreshReason };

export type ResourceRefreshCoordinator = {
  refresh: (reason: ResourceRefreshReason) => Promise<ResourceRefreshResult>;
  applyExternalResources: (resources: DashboardResourcesView) => void;
  invalidate: (serverId: string) => void;
  resume: () => void;
  dispose: () => void;
  isInFlight: () => boolean;
};

export function createResourceRefreshCoordinator({
  fetchResources = requestDashboardResources,
  applyResources,
  setLoading = () => {},
  setError = () => {},
}: ResourceRefreshCoordinatorOptions): ResourceRefreshCoordinator {
  let disposed = false;
  let inFlight = false;
  let issuedSequence = 0;
  let appliedSequence = 0;
  let pendingForegroundRefresh: ResourceRefreshReason | null = null;
  // Server id whose in-flight fetch (issued at or before `invalidatedAtSequence`)
  // must be dropped rather than applied. Set by `invalidate` when a server is
  // turned Off while its resource fetch is already in flight; without this, the
  // stale response would merge back the just-deallocated server entry.
  let invalidatedServerId: string | null = null;
  let invalidatedAtSequence = 0;

  const startRefresh = async (
    reason: ResourceRefreshReason,
  ): Promise<ResourceRefreshResult> => {
    if (disposed) {
      return { status: "skipped", reason, cause: "disposed" };
    }

    if (inFlight) {
      if (reason !== "poll") {
        pendingForegroundRefresh = reason;
      }
      return { status: "skipped", reason, cause: "inFlight" };
    }

    inFlight = true;
    const sequence = ++issuedSequence;
    const showLoading = reason !== "poll";
    if (showLoading) {
      setLoading(true);
      setError(null);
    }

    try {
      const resources = await fetchResources();
      if (disposed || sequence < appliedSequence) {
        return { status: "stale", reason };
      }
      // Drop a response for a server that was invalidated (Off'd) after this
      // fetch was issued. Scoped to the invalidated server id and to fetches at
      // or before the invalidation point, so a later re-add of the same server
      // (a newer sequence) still applies, and responses for any other server
      // are untouched.
      if (
        invalidatedServerId !== null &&
        sequence <= invalidatedAtSequence &&
        resources.server.id === invalidatedServerId
      ) {
        return { status: "stale", reason };
      }
      appliedSequence = sequence;
      applyResources(resources);
      setError(null);
      return { status: "applied", reason };
    } catch (error) {
      if (!disposed && sequence >= appliedSequence) {
        setError(error instanceof Error ? error.message : "request failed");
      }
      return {
        status: "failed",
        reason,
        error: error instanceof Error ? error.message : "request failed",
      };
    } finally {
      if (showLoading && !disposed) {
        setLoading(false);
      }
      inFlight = false;
      const pending = pendingForegroundRefresh;
      pendingForegroundRefresh = null;
      if (pending && !disposed) {
        void startRefresh(pending);
      }
    }
  };

  return {
    refresh: startRefresh,
    applyExternalResources: (resources: DashboardResourcesView) => {
      if (disposed) {
        return;
      }
      issuedSequence += 1;
      appliedSequence = issuedSequence;
      applyResources(resources);
      setError(null);
    },
    invalidate: (serverId: string) => {
      invalidatedServerId = serverId;
      invalidatedAtSequence = issuedSequence;
    },
    resume: () => {
      disposed = false;
    },
    dispose: () => {
      disposed = true;
      pendingForegroundRefresh = null;
    },
    isInFlight: () => inFlight,
  };
}
