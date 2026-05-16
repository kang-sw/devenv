import { spawn, type ChildProcess } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
// frontend/e2e -> ws-dashboard
const repoRoot = path.resolve(here, "..", "..");

export type DaemonHandle = {
  child: ChildProcess;
  baseUrl: string;
  pairingUrl: string;
  stop: () => Promise<void>;
};

/**
 * Boot the dashboard daemon serving the production `frontend/dist` build and
 * scrape the one-time owner pairing URL from startup output. The browser gate
 * must exercise the daemon-served frontend, not a Vite dev server.
 */
export async function startDaemon(): Promise<DaemonHandle> {
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

  return { child, baseUrl, pairingUrl, stop };
}
