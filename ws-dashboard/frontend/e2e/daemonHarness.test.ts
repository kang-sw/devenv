import { createServer } from "node:http";
import {
  dashboardBinaryName,
  parseDaemonHarnessConfig,
  resolveDaemonBinary,
  startDaemon,
} from "./daemonHarness.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) throw new Error(`${label}: expected ${expected}, got ${actual}`);
}

function assertIncludes(actual: string, expected: string, label: string) {
  if (!actual.includes(expected)) throw new Error(`${label}: expected ${actual} to include ${expected}`);
}

async function assertRejects(
  fn: () => Promise<unknown>,
  expected: string,
  label: string,
  unexpected: string[] = [],
) {
  try {
    await fn();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    assertIncludes(message, expected, label);
    for (const value of unexpected) {
      if (message.includes(value)) {
        throw new Error(`${label}: expected ${message} not to include ${value}`);
      }
    }
    return;
  }
  throw new Error(`${label}: expected rejection`);
}

function assertThrows(fn: () => unknown, expected: string, label: string) {
  try {
    fn();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    assertIncludes(message, expected, label);
    return;
  }
  throw new Error(`${label}: expected throw`);
}

const spawnConfig = parseDaemonHarnessConfig({
  WS_DASHBOARD_DAEMON_MODE: "spawn",
  WS_DASHBOARD_DAEMON_HOST: "127.0.0.1",
  WS_DASHBOARD_DAEMON_PORT: "47173",
  WS_DASHBOARD_DAEMON_BIND_MODE: "tunnel",
  WS_DASHBOARD_DAEMON_BIN: "custom-daemon",
  WS_DASHBOARD_STATIC_DIR: "dist-fixture",
  WS_DASHBOARD_DAEMON_READINESS_TIMEOUT_MS: "1234",
});
assertEqual(spawnConfig.mode, "spawn", "spawn mode parsed");
if (spawnConfig.mode !== "spawn") throw new Error("expected spawn config");
assertEqual(spawnConfig.host, "127.0.0.1", "spawn host parsed");
assertEqual(spawnConfig.port, 47173, "spawn fixed port parsed");
assertEqual(spawnConfig.bindMode, "tunnel", "spawn bind mode parsed");
assertEqual(spawnConfig.daemonBin, "custom-daemon", "spawn daemon bin parsed");
assertEqual(spawnConfig.staticDir, "dist-fixture", "spawn static dir parsed");
assertEqual(spawnConfig.readinessTimeoutMs, 1234, "spawn readiness timeout parsed");

const externalConfig = parseDaemonHarnessConfig({
  WS_DASHBOARD_DAEMON_BASE_URL: "http://127.0.0.1:47173",
  WS_DASHBOARD_DAEMON_PAIRING_URL: "http://127.0.0.1:47173/pair?token=redacted",
});
assertEqual(externalConfig.mode, "external", "external mode inferred from URL env");

assertThrows(
  () => parseDaemonHarnessConfig({ WS_DASHBOARD_DAEMON_MODE: "bogus" }),
  "WS_DASHBOARD_DAEMON_MODE",
  "invalid mode names variable",
);
assertThrows(
  () => parseDaemonHarnessConfig({ WS_DASHBOARD_DAEMON_PORT: "70000" }),
  "WS_DASHBOARD_DAEMON_PORT",
  "invalid port names variable",
);
assertThrows(
  () => parseDaemonHarnessConfig({ WS_DASHBOARD_DAEMON_READINESS_TIMEOUT_MS: "0" }),
  "WS_DASHBOARD_DAEMON_READINESS_TIMEOUT_MS",
  "invalid timeout names variable",
);
assertThrows(
  () => parseDaemonHarnessConfig({ WS_DASHBOARD_DAEMON_BIND_MODE: "wide" }),
  "WS_DASHBOARD_DAEMON_BIND_MODE",
  "invalid bind mode names variable",
);

assertEqual(dashboardBinaryName("win32"), "ws-dashboard.exe", "Windows binary name");
assertEqual(dashboardBinaryName("linux"), "ws-dashboard", "non-Windows binary name");
assertIncludes(resolveDaemonBinary("/repo", "win32"), "ws-dashboard.exe", "Windows daemon path uses exe");

const server = createServer((request, response) => {
  if (request.url === "/healthz") {
    response.writeHead(401).end("owner required");
    return;
  }
  response.writeHead(404).end("not found");
});
await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
const address = server.address();
if (!address || typeof address === "string") throw new Error("expected TCP server address");
const baseUrl = `http://127.0.0.1:${address.port}`;
try {
  const fromPairing = await startDaemon({
    mode: "external",
    pairingUrl: `${baseUrl}/pair?token=secret-token`,
    readinessTimeoutMs: 1000,
  });
  assertEqual(fromPairing.mode, "external", "external pairing mode returned");
  assertEqual(fromPairing.baseUrl, baseUrl, "external base URL derived from pairing URL");
  assertEqual(fromPairing.pairingUrl, `${baseUrl}/pair?token=secret-token`, "external pairing URL preserved");
  assertEqual(fromPairing.readinessSignal, "http", "external readiness signal is HTTP");
  await fromPairing.stop();

  const fromBase = await startDaemon({
    mode: "external",
    baseUrl,
    readinessTimeoutMs: 1000,
  });
  assertEqual(fromBase.baseUrl, baseUrl, "external base-only endpoint accepted");
  assertEqual(fromBase.pairingUrl, baseUrl, "external base-only pairing target preserved for prepared endpoints");
  await fromBase.stop();
} finally {
  await new Promise<void>((resolve) => server.close(() => resolve()));
}

await assertRejects(
  () => startDaemon({ mode: "external", baseUrl: "http://127.0.0.1:9/pair?token=secret-token", readinessTimeoutMs: 1 }),
  "daemon endpoint <redacted-url> was not reachable",
  "external readiness failure is explicit and scrubbed",
  ["secret-token", "127.0.0.1"],
);
