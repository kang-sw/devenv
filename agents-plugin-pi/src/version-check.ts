/**
 * Pin-and-fail version check between the bundled runtime.json and the
 * spawned ws-mcp server's reported version.
 *
 * agents-plugin-pi/runtime.json is a hand-synced, byte-identical copy of
 * agents-plugin/runtime.json (see the HAND-SYNC NOTE in index.ts for the
 * full 3-way copy surface — this mirrors the existing agents-plugin-wsflow
 * precedent, which carries its own copies rather than a cross-root relative
 * reference). agents-plugin-tool/scripts/bump-ws-version.sh resyncs all
 * three copies at every ws version bump (260914); this check still guards
 * the case where a mirror was hand-edited or a bump was skipped.
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
 * check point — no separate runtime.info call needed). The server is the
 * machine's daemon, which the launcher shares with every same-minor plugin
 * on the machine and which the newest of them runs, so the rule is the
 * daemon's own (wsdashboard.CompareVersions): the server must be in the
 * bundled version's minor and equal or newer, a pre-release suffix counting
 * as newer than none. Throws synchronously otherwise so the extension load
 * fails loudly: zero tools get registered, and there is no silent degraded
 * fallback.
 */
export function assertVersionPin(runtime: RuntimeContract, serverVersion: string): void {
  if (serverVersionAcceptable(runtime.plugin_version, serverVersion)) return;
  throw new Error(
    `ws-pi-bridge: ws-mcp version mismatch — bundled agents-plugin-pi/runtime.json ` +
      `expects plugin_version "${runtime.plugin_version}" (the same minor, equal or newer) but the spawned ws-mcp ` +
      `server reported serverInfo.version "${serverVersion}". Refusing to register ` +
      `any ws/* tools. Re-copy agents-plugin/runtime.json and ` +
      `agents-plugin/bin/ws-mcp-launcher.py into agents-plugin-pi/ to resync.`,
  );
}

/** Same major.minor, and server not older than bundled (pre-release after none). */
export function serverVersionAcceptable(bundled: string, server: string): boolean {
  const a = parseVersion(bundled);
  const b = parseVersion(server);
  if (!a || !b) return false;
  if (a.core[0] !== b.core[0] || a.core[1] !== b.core[1]) return false;
  return compareVersions(a, b) <= 0;
}

interface ParsedVersion {
  core: [number, number, number];
  pre: string;
}

function parseVersion(value: string): ParsedVersion | null {
  let v = value.trim();
  if (v.startsWith("v")) v = v.slice(1);
  let pre = "";
  const dash = v.indexOf("-");
  if (dash >= 0) {
    pre = v.slice(dash + 1);
    v = v.slice(0, dash);
  }
  const parts = v.split(".");
  if (parts.length !== 3) return null;
  const core: number[] = [];
  for (const part of parts) {
    if (!/^\d+$/.test(part)) return null;
    core.push(Number(part));
  }
  return { core: core as [number, number, number], pre };
}

function compareVersions(a: ParsedVersion, b: ParsedVersion): number {
  for (let i = 0; i < 3; i++) {
    if (a.core[i] !== b.core[i]) return a.core[i] < b.core[i] ? -1 : 1;
  }
  if (a.pre === b.pre) return 0;
  if (a.pre === "") return -1;
  if (b.pre === "") return 1;
  return a.pre < b.pre ? -1 : 1;
}
