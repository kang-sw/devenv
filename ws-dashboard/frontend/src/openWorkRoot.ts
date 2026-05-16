import { apiErrorDetail } from "./apiError.js";
import type { DashboardResourcesView } from "./resourceModel.js";

// Open-workRoot API helper, mirroring the per-API fetch-wrapper pattern of
// workRootFiles.ts and terminals.ts.
export const openWorkRootEndpoint = "/api/dashboard/work-roots/open";

// POST a host path to the daemon open-workRoot route. On success the daemon
// returns the aggregated live resource view of every opened workRoot.
export async function requestOpenWorkRoot(path: string): Promise<DashboardResourcesView> {
  const response = await fetch(openWorkRootEndpoint, {
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

  return (await response.json()) as DashboardResourcesView;
}
