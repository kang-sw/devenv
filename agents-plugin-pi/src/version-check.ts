/**
 * Pin-and-fail version check between the bundled runtime.json and the
 * spawned ws-mcp server's reported version.
 *
 * agents-plugin-pi/runtime.json is a hand-synced copy of
 * agents-plugin/runtime.json (see the header comment in
 * agents-plugin-pi/bin/ws-mcp-launcher.py, itself a hand-synced copy of
 * agents-plugin/bin/ws-mcp-launcher.py — this mirrors the existing
 * agents-plugin-wsflow precedent, which carries its own copies rather than a
 * cross-root relative reference). No sync tooling exists yet to keep these
 * in lockstep automatically.
 */

import { readFileSync } from "node:fs";

export interface RuntimeContract {
  plugin: string;
  plugin_version: string;
  [key: string]: unknown;
}

export function readRuntimeContract(path: string): RuntimeContract {
  const raw = readFileSync(path, "utf8");
  return JSON.parse(raw) as RuntimeContract;
}

/**
 * Compares the bundled runtime.json's plugin_version against the ws-mcp
 * server's initialize response serverInfo.version (a free, no-extra-call
 * check point — no separate runtime.info call needed). Throws synchronously
 * on mismatch so the extension load fails loudly: zero tools get
 * registered, and there is no silent degraded fallback.
 */
export function assertVersionPin(runtime: RuntimeContract, serverVersion: string): void {
  if (runtime.plugin_version !== serverVersion) {
    throw new Error(
      `ws-pi-bridge: ws-mcp version mismatch — bundled agents-plugin-pi/runtime.json ` +
        `expects plugin_version "${runtime.plugin_version}" but the spawned ws-mcp ` +
        `server reported serverInfo.version "${serverVersion}". Refusing to register ` +
        `any ws/* tools. Re-copy agents-plugin/runtime.json and ` +
        `agents-plugin/bin/ws-mcp-launcher.py into agents-plugin-pi/ to resync.`,
    );
  }
}
