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
  child: ChildProcess;
  baseUrl: string;
  pairingUrl: string;
  stop: () => Promise<void>;
};

export function parseDaemonHarnessConfig(env: NodeJS.ProcessEnv = process.env): DaemonHarnessConfig {
  // CONTRACT: Environment parsing must expose fixed host/port/bind-mode,
  // daemon binary, static dir, readiness timeout, and external base/pairing URL.
  // HINT: Keep existing no-env behavior equivalent to locally spawned port 0.
  // HOLE: Choose stable env var names and validation diagnostics.
  void env;
  throw new Error("HOLE: parse daemon harness config");
}

export function dashboardBinaryName(platform: NodeJS.Platform = process.platform): string {
  // CONTRACT: Native Windows resolves `ws-dashboard.exe`; other platforms keep
  // `ws-dashboard`.
  // HOLE: Normalize platform-specific executable naming in one place.
  void platform;
  throw new Error("HOLE: dashboard binary name");
}

export function resolveDaemonBinary(root: string, platform: NodeJS.Platform = process.platform): string {
  // CONTRACT: The harness must resolve the debug daemon binary in a
  // cross-platform way unless an explicit daemonBin override is provided.
  // HINT: Adjacent path is target/debug plus dashboardBinaryName(platform).
  void root;
  void platform;
  throw new Error("HOLE: resolve daemon binary");
}

export async function stopDaemonProcess(
  child: ChildProcess,
  options: { platform?: NodeJS.Platform; timeoutMs?: number } = {},
): Promise<void> {
  // CONTRACT: Shutdown must not assume POSIX-only signal behavior. It should
  // prefer graceful stop when available and report forced termination clearly.
  // HOLE: Fill Windows-safe stop behavior and timeout diagnostics.
  void child;
  void options;
  throw new Error("HOLE: stop daemon process");
}

/**
 * Boot the dashboard daemon serving the production `frontend/dist` build and
 * scrape the one-time owner pairing URL from startup output. The browser gate
 * must exercise the daemon-served frontend, not a Vite dev server.
 */
export async function startDaemon(config: DaemonHarnessConfig = {}): Promise<DaemonHandle> {
  // CONTRACT: startDaemon must support both spawned-daemon mode and external
  // fixed-endpoint mode. External mode attaches to a supplied base/pairing URL
  // and uses a no-op stop handle.
  // HINT: The existing implementation below is the spawned default path.
  // HOLE: Wire config into CLI args, endpoint readiness, and external attach.
  void config;
  const daemonBin = path.join(repoRoot, "target", "debug", "ws-dashboard");
  const staticDir = path.join(repoRoot, "frontend", "dist");

  const child = spawn(daemonBin, ["serve", "--static-dir", staticDir], {
    cwd: repoRoot,
    stdio: ["ignore", "pipe", "pipe"],
  });

  let startupBuffer = "";
  let scrape: ((chunk: Buffer) => void) | null = null;
  const pairingUrl = await new Promise<string>((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error("daemon did not report a pairing URL within 60s"));
    }, 60_000);

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
    child.once("exit", (code) => {
      clearTimeout(timer);
      reject(new Error(`daemon exited before pairing (code ${code})`));
    });
    child.once("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
  });

  // Stop scraping startup output, then just drain the pipes so a full buffer
  // never blocks the daemon and `startupBuffer` does not grow for the run.
  if (scrape) {
    child.stderr?.off("data", scrape);
    child.stdout?.off("data", scrape);
  }
  child.stderr?.on("data", () => {});
  child.stdout?.on("data", () => {});

  const baseUrl = new URL(pairingUrl).origin;

  const stop = () =>
    new Promise<void>((resolve) => {
      if (child.exitCode !== null || child.signalCode !== null) {
        resolve();
        return;
      }
      const killTimer = setTimeout(() => {
        child.kill("SIGKILL");
        resolve();
      }, 5_000);
      child.once("exit", () => {
        clearTimeout(killTimer);
        resolve();
      });
      child.kill("SIGINT");
    });

  return { mode: "spawned", child, baseUrl, pairingUrl, stop };
}
