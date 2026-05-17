import { spawn, type ChildProcess } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
// frontend/e2e -> ws-dashboard
const repoRoot = path.resolve(here, "..", "..");

export type DashboardBindMode = "local" | "tunnel" | "public";

export type SpawnDaemonHarnessConfig = {
  mode?: "spawn";
  host?: string;
  port?: number;
  bindMode?: DashboardBindMode;
  daemonBin?: string;
  staticDir?: string;
  readinessTimeoutMs?: number;
};

export type ExternalDaemonHarnessConfig = {
  mode: "external";
  baseUrl?: string;
  pairingUrl?: string;
  readinessTimeoutMs?: number;
};

export type DaemonHarnessConfig = SpawnDaemonHarnessConfig | ExternalDaemonHarnessConfig;

export type DaemonHandle = {
  mode: "spawned" | "external";
  child?: ChildProcess;
  baseUrl: string;
  pairingUrl: string;
  command?: string[];
  readinessSignal: "pairing-url" | "http";
  stop: () => Promise<void>;
};


function scrubDiagnosticText(value: string): string {
  return value
    .replace(/https?:\/\/\S+/g, "<redacted-url>")
    .replace(/owner pairing URL:\s*\S+/gi, "owner pairing URL: <redacted-url>")
    .replace(/([A-Za-z]:)?[\\/][^\s;]+/g, "<redacted-path>");
}

function diagnosticCommand(command: string[]): string {
  return command
    .map((part, index, parts) => {
      if (index === 0) return path.basename(part);
      if (parts[index - 1] === "--static-dir") return "<static-dir>";
      if (parts[index - 1] === "--host") return "<host>";
      return scrubDiagnosticText(part);
    })
    .join(" ");
}

function optionalPort(name: string, value: string | undefined): number | undefined {
  if (value === undefined || value === "") {
    return undefined;
  }
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 0 || parsed > 65_535) {
    throw new Error(`${name} must be an integer port value between 0 and 65535`);
  }
  return parsed;
}

function optionalTimeout(name: string, value: string | undefined): number | undefined {
  if (value === undefined || value === "") {
    return undefined;
  }
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed <= 0 || parsed > 600_000) {
    throw new Error(`${name} must be an integer timeout value between 1 and 600000`);
  }
  return parsed;
}

function optionalBindMode(value: string | undefined): DashboardBindMode | undefined {
  if (value === undefined || value === "") {
    return undefined;
  }
  if (value === "local" || value === "tunnel" || value === "public") {
    return value;
  }
  throw new Error("WS_DASHBOARD_DAEMON_BIND_MODE must be local, tunnel, or public");
}

export function parseDaemonHarnessConfig(env: NodeJS.ProcessEnv = process.env): DaemonHarnessConfig {
  // CONTRACT: Environment parsing must expose fixed host/port/bind-mode,
  // daemon binary, static dir, readiness timeout, and external base/pairing URL.
  // HINT: Keep existing no-env behavior equivalent to locally spawned port 0.
  const readinessTimeoutMs = optionalTimeout(
    "WS_DASHBOARD_DAEMON_READINESS_TIMEOUT_MS",
    env.WS_DASHBOARD_DAEMON_READINESS_TIMEOUT_MS,
  );
  const mode = env.WS_DASHBOARD_DAEMON_MODE;
  const baseUrl = env.WS_DASHBOARD_DAEMON_BASE_URL;
  const pairingUrl = env.WS_DASHBOARD_DAEMON_PAIRING_URL;

  if (mode === "external" || baseUrl || pairingUrl) {
    if (mode && mode !== "external") {
      throw new Error("WS_DASHBOARD_DAEMON_MODE must be spawn or external");
    }
    return { mode: "external", baseUrl, pairingUrl, readinessTimeoutMs };
  }

  if (mode && mode !== "spawn") {
    throw new Error("WS_DASHBOARD_DAEMON_MODE must be spawn or external");
  }

  return {
    mode: "spawn",
    host: env.WS_DASHBOARD_DAEMON_HOST,
    port: optionalPort("WS_DASHBOARD_DAEMON_PORT", env.WS_DASHBOARD_DAEMON_PORT),
    bindMode: optionalBindMode(env.WS_DASHBOARD_DAEMON_BIND_MODE),
    daemonBin: env.WS_DASHBOARD_DAEMON_BIN,
    staticDir: env.WS_DASHBOARD_STATIC_DIR,
    readinessTimeoutMs,
  };
}

export function dashboardBinaryName(platform: NodeJS.Platform = process.platform): string {
  // CONTRACT: Native Windows resolves `ws-dashboard.exe`; other platforms keep
  // `ws-dashboard`.
  return platform === "win32" ? "ws-dashboard.exe" : "ws-dashboard";
}

export function resolveDaemonBinary(root: string, platform: NodeJS.Platform = process.platform): string {
  // CONTRACT: The harness must resolve the debug daemon binary in a
  // cross-platform way unless an explicit daemonBin override is provided.
  // HINT: Adjacent path is target/debug plus dashboardBinaryName(platform).
  return path.join(root, "target", "debug", dashboardBinaryName(platform));
}

export async function stopDaemonProcess(
  child: ChildProcess,
  options: { platform?: NodeJS.Platform; timeoutMs?: number } = {},
): Promise<void> {
  // CONTRACT: Shutdown must not assume POSIX-only signal behavior. It should
  // prefer graceful stop when available and report forced termination clearly.
  if (child.exitCode !== null || child.signalCode !== null) {
    return;
  }

  const platform = options.platform ?? process.platform;
  const timeoutMs = options.timeoutMs ?? 5_000;

  await new Promise<void>((resolve) => {
    const killTimer = setTimeout(() => {
      if (child.exitCode === null && child.signalCode === null) {
        child.kill("SIGKILL");
      }
      resolve();
    }, timeoutMs);
    child.once("exit", () => {
      clearTimeout(killTimer);
      resolve();
    });
    child.kill(platform === "win32" ? undefined : "SIGINT");
  });
}

/**
 * Boot the dashboard daemon serving the production `frontend/dist` build and
 * scrape the one-time owner pairing URL from startup output. The browser gate
 * must exercise the daemon-served frontend, not a Vite dev server.
 */
async function waitForHttpReadiness(baseUrl: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastError: unknown;
  while (Date.now() <= deadline) {
    try {
      const response = await fetch(new URL("/healthz", baseUrl));
      if (response.status === 200 || response.status === 401) {
        return;
      }
      lastError = new Error(`unexpected /healthz status ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  const detail = lastError instanceof Error ? `: ${scrubDiagnosticText(lastError.message)}` : "";
  throw new Error(`daemon endpoint ${scrubDiagnosticText(baseUrl)} was not reachable within ${timeoutMs}ms${detail}`);
}

export async function startDaemon(config: DaemonHarnessConfig = parseDaemonHarnessConfig()): Promise<DaemonHandle> {
  // CONTRACT: startDaemon must support both spawned-daemon mode and external
  // fixed-endpoint mode. External mode attaches to a supplied base/pairing URL
  // and uses a no-op stop handle.
  // HINT: The existing implementation below is the spawned default path.
  const readinessTimeoutMs = config.readinessTimeoutMs ?? 60_000;

  if (config.mode === "external") {
    const pairingUrl = config.pairingUrl ?? config.baseUrl;
    if (!pairingUrl) {
      throw new Error("external daemon mode requires WS_DASHBOARD_DAEMON_PAIRING_URL or WS_DASHBOARD_DAEMON_BASE_URL");
    }
    let baseUrl: string;
    try {
      baseUrl = config.baseUrl && config.pairingUrl ? config.baseUrl : new URL(pairingUrl).origin;
      new URL(baseUrl);
      new URL(pairingUrl);
    } catch (error) {
      const detail = error instanceof Error ? `: ${error.message}` : "";
      throw new Error(`external daemon mode received an invalid WS_DASHBOARD_DAEMON_BASE_URL or WS_DASHBOARD_DAEMON_PAIRING_URL${detail}`);
    }
    await waitForHttpReadiness(baseUrl, readinessTimeoutMs);
    return { mode: "external", baseUrl, pairingUrl, readinessSignal: "http", stop: async () => {} };
  }

  const daemonBin = config.daemonBin ?? resolveDaemonBinary(repoRoot);
  const staticDir = config.staticDir ?? path.join(repoRoot, "frontend", "dist");
  const args = ["serve", "--static-dir", staticDir];
  if (config.host) {
    args.push("--host", config.host);
  }
  if (config.bindMode) {
    args.push("--bind-mode", config.bindMode);
  }
  if (config.port !== undefined) {
    args.push("--port", String(config.port));
  }

  const command = [daemonBin, ...args];
  const child = spawn(daemonBin, args, {
    cwd: repoRoot,
    env: {
      ...process.env,
      WS_DASHBOARD_E2E_AGENT_FIXTURE:
        process.env.WS_DASHBOARD_E2E_AGENT_FIXTURE ?? "1",
    },
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });

  let startupBuffer = "";
  let scrape: ((chunk: Buffer) => void) | null = null;
  let pairingUrl: string;
  try {
    pairingUrl = await new Promise<string>((resolve, reject) => {
      const timer = setTimeout(() => {
        reject(
          new Error(
            `daemon startup timed out waiting for owner pairing URL within ${readinessTimeoutMs}ms; command=${diagnosticCommand(command)}; startup=${scrubDiagnosticText(startupBuffer.slice(-4000))}`,
          ),
        );
      }, readinessTimeoutMs);

      scrape = (chunk: Buffer) => {
        startupBuffer += chunk.toString();
        // Require the trailing newline so a pairing token split across a
        // stdout/stderr chunk boundary is never resolved truncated.
        const match = startupBuffer.match(/owner pairing URL:\s*(\S+)\r?\n/);
        if (match) {
          clearTimeout(timer);
          resolve(match[1]);
        }
      };

      child.stderr?.on("data", scrape);
      child.stdout?.on("data", scrape);
      child.once("exit", (code, signal) => {
        clearTimeout(timer);
        reject(
          new Error(
            `daemon exited before pairing (code ${code}, signal ${signal}); command=${diagnosticCommand(command)}; startup=${scrubDiagnosticText(startupBuffer.slice(-4000))}`,
          ),
        );
      });
      child.once("error", (error) => {
        clearTimeout(timer);
        reject(error);
      });
    });
  } catch (error) {
    await stopDaemonProcess(child).catch(() => {});
    throw error;
  }

  // Stop scraping startup output, then just drain the pipes so a full buffer
  // never blocks the daemon and `startupBuffer` does not grow for the run.
  if (scrape) {
    child.stderr?.off("data", scrape);
    child.stdout?.off("data", scrape);
  }
  child.stderr?.on("data", () => {});
  child.stdout?.on("data", () => {});

  const baseUrl = new URL(pairingUrl).origin;
  try {
    await waitForHttpReadiness(baseUrl, readinessTimeoutMs);
  } catch (error) {
    await stopDaemonProcess(child).catch(() => {});
    throw error;
  }

  const stop = () => stopDaemonProcess(child);

  return { mode: "spawned", child, baseUrl, pairingUrl, command, readinessSignal: "pairing-url", stop };
}
