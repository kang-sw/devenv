import { appendFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { getAgentDir, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { isChildProcess } from "./goal-loop.ts";

/**
 * TEMPORARY diagnostic for the post-compaction lead OOM investigation: in the
 * lead process only, appends one `process.memoryUsage()` JSON line per minute
 * (and one at start) to `<agentDir>/ws-lead-memory.jsonl` so the next crash
 * leaves a growth curve. Remove once the root cause is found.
 */
export const LEAD_MEMORY_LOG_FILE = "ws-lead-memory.jsonl";
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
    // `node --test` sets NODE_TEST_CONTEXT in test processes; suites that load the
    // whole extension must not append to the real agent dir's log.
    if (file === undefined && process.env.NODE_TEST_CONTEXT) return;
    const target = file ?? join(getAgentDir(), LEAD_MEMORY_LOG_FILE);
    writeMemorySample(target);
    timer = setInterval(() => writeMemorySample(target), INTERVAL_MS);
    timer.unref();
  });
  pi.on("session_shutdown", async () => { stop(); });
}
