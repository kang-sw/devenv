import {
  defaultLinkedServerId,
  endpointLinkedServerEndpoint,
  linkEndpointServer,
  linkServerPassphrase,
  reconnectServerTunnel,
} from "./linkedServers.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

assertEqual(
  defaultLinkedServerId("Remote Dev", ""),
  "server-remote-dev",
  "server id derives from label",
);
assertEqual(
  defaultLinkedServerId("", "http://127.0.0.1:49170/"),
  "server-127-0-0-1-49170",
  "server id falls back to endpoint",
);

const originalFetch = globalThis.fetch;
let capturedUrl = "";
let capturedMethod = "";
let capturedBody = "";
globalThis.fetch = (async (input, init) => {
  capturedUrl = String(input);
  capturedMethod = String(init?.method ?? "GET");
  capturedBody = String(init?.body ?? "");
  return new Response(
    JSON.stringify({
      id: "server-remote",
      label: "Remote",
      kind: "manual",
      status: "connected",
      state: { status: "connected", loading: false, stale: false, error: null },
      actions: [],
    }),
    { status: 200, headers: { "content-type": "application/json" } },
  );
}) as typeof fetch;

await linkEndpointServer({
  serverId: "server-remote",
  label: "Remote",
  endpoint: "http://127.0.0.1:49170",
  passphrase: "secret",
});
assertEqual(capturedUrl, endpointLinkedServerEndpoint, "endpoint link uses stable route");
assertEqual(capturedMethod, "POST", "endpoint link posts");
assertEqual(
  JSON.parse(capturedBody).endpoint,
  "http://127.0.0.1:49170",
  "endpoint link sends endpoint",
);

await linkServerPassphrase("server remote/1", "secret");
assertEqual(
  capturedUrl,
  "/api/dashboard/servers/server%20remote%2F1/link-auth",
  "passphrase link scopes server id",
);
assertEqual(JSON.parse(capturedBody).passphrase, "secret", "passphrase body is sent");

await reconnectServerTunnel("server remote/1");
assertEqual(
  capturedUrl,
  "/api/dashboard/servers/server%20remote%2F1/tunnel/reconnect",
  "reconnect scopes server id",
);
assertEqual(capturedMethod, "POST", "reconnect posts");

globalThis.fetch = originalFetch;
