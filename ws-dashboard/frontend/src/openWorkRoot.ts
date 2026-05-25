import { apiErrorDetail } from "./apiError.js";
import {
  localCompatibleDashboardApiRoute,
  type DashboardResourcesView,
} from "./resourceModel.js";

// Open-workRoot API helper, mirroring the per-API fetch-wrapper pattern of
// workRootFiles.ts and terminals.ts.
export const openWorkRootEndpoint = "/api/dashboard/work-roots/open";

export function serverOpenWorkRootEndpoint(
  serverId: string | null | undefined,
) {
  return localCompatibleDashboardApiRoute(serverId, ["work-roots", "open"]);
}

// POST a host path to the daemon open-workRoot route. On success the daemon
// returns the aggregated live resource view of every opened workRoot.
export type OpenWorkRootResult = {
  view: DashboardResourcesView;
  openedWorkRootId: string | null;
};

export async function requestOpenWorkRoot(
  path: string,
  serverId?: string | null,
): Promise<OpenWorkRootResult> {
  const response = await fetch(serverOpenWorkRootEndpoint(serverId), {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ path }),
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return {
    view: (await response.json()) as DashboardResourcesView,
    openedWorkRootId: response.headers.get(
      "x-ws-dashboard-opened-work-root-id",
    ),
  };
}
