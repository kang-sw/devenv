import { appendFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { getAgentDir, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { isChildProcess } from "./goal-loop.ts";

/**
 * Opt-in diagnostic, kept after the post-compaction lead OOM investigation:
 * with `WS_PI_LEAD_MEMORY_LOG=1`, the lead process (never a child) appends one
 * `process.memoryUsage()` JSON line per minute (and one at start) to
 * `<agentDir>/ws-lead-memory.jsonl`, unrotated. Off by default so shipped
 * installs do not grow a file in every user's agent dir.
 */
export const LEAD_MEMORY_LOG_FILE = "ws-lead-memory.jsonl";
export const LEAD_MEMORY_LOG_ENV = "WS_PI_LEAD_MEMORY_LOG";
const INTERVAL_MS = 60_000;

export function writeMemorySample(file: string, now = Date.now()): void {
  try {
    const { rss, heapUsed, heapTotal, external, arrayBuffers } = process.memoryUsage();
    mkdirSync(dirname(file), { recursive: true });
    appendFileSync(file, `${JSON.stringify({ t: now, pid: process.pid, rss, heapUsed, heapTotal, external, arrayBuffers })}\n`);
  } catch { /* diagnostic only */ }
}

export function registerLeadMemoryLog(pi: ExtensionAPI, file?: string): void {
  let timer: ReturnType<typeof setInterval> | undefined;
  const stop = () => { if (timer) clearInterval(timer); timer = undefined; };
  pi.on("session_start", async () => {
    if (isChildProcess(process.env) || timer) return;
    if (file === undefined && process.env[LEAD_MEMORY_LOG_ENV] !== "1") return;
    const target = file ?? join(getAgentDir(), LEAD_MEMORY_LOG_FILE);
    writeMemorySample(target);
    timer = setInterval(() => writeMemorySample(target), INTERVAL_MS);
    timer.unref();
  });
  pi.on("session_shutdown", async () => { stop(); });
}
