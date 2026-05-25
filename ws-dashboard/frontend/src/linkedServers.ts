import { apiErrorDetail } from "./apiError.js";
import type { ServerConnectionView } from "./resourceModel.js";

export const endpointLinkedServerEndpoint = "/api/dashboard/servers/link";

export type EndpointLinkedServerRequest = {
  serverId: string;
  label: string;
  endpoint: string;
  passphrase?: string;
};

export async function linkEndpointServer(
  request: EndpointLinkedServerRequest,
): Promise<ServerConnectionView> {
  const response = await fetch(endpointLinkedServerEndpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(request),
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as ServerConnectionView;
}

export async function linkServerPassphrase(
  serverId: string,
  passphrase: string,
): Promise<ServerConnectionView> {
  const response = await fetch(
    `/api/dashboard/servers/${encodeURIComponent(serverId)}/link-auth`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ passphrase }),
    },
  );

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as ServerConnectionView;
}

export async function reconnectServerTunnel(
  serverId: string,
): Promise<ServerConnectionView> {
  const response = await fetch(
    `/api/dashboard/servers/${encodeURIComponent(serverId)}/tunnel/reconnect`,
    {
      method: "POST",
      headers: { Accept: "application/json" },
    },
  );

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as ServerConnectionView;
}

export function defaultLinkedServerId(label: string, endpoint: string): string {
  const source = label.trim() || endpoint.trim() || "remote";
  const slug = source
    .toLowerCase()
    .replace(/^[a-z]+:\/\//, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 48);
  return `server-${slug || "remote"}`;
}
