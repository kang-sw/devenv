import type { DashboardResourcesView } from "./resourceModel.js";

export const resourceEndpoint = "/api/dashboard/resources";
export const resourceAvailabilityPollIntervalMs = 5_000;

export type ResourceRefreshReason = "initial" | "explicit" | "poll" | "open";

export type ResourceFetch = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

export async function requestDashboardResources(
  fetchResource: ResourceFetch = fetch,
): Promise<DashboardResourcesView> {
  const response = await fetchResource(resourceEndpoint, {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return (await response.json()) as DashboardResourcesView;
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
