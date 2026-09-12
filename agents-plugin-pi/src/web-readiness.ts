/** Per-launch proof that the exact ws extension registered both Explore web facades. */
import { readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { NETWORK_TOOLS } from "./delegation-policy.ts";

export const WEB_HOME_ENV = "WS_PI_WEB_HOME";
export const WEB_NONCE_ENV = "WS_PI_WEB_READY_NONCE";
const READY_FILE = "web-tools-ready.json";

export function clearWebReadiness(home: string): void {
  rmSync(join(home, READY_FILE), { force: true });
}

export function writeWebReadiness(pi: ExtensionAPI, home: string, nonce: string, extensionPath: string): void {
  if (!home || !nonce) throw new Error("web-search-tool-unavailable: missing Explore launch identity");
  const registered = pi.getAllTools();
  const active = pi.getActiveTools();
  for (const name of NETWORK_TOOLS) {
    const matches = registered.filter(tool => tool.name === name);
    if (matches.length !== 1 || !active.includes(name) || matches[0].sourceInfo?.path !== extensionPath) {
      throw new Error("web-search-tool-unavailable: Explore web facade registration mismatch");
    }
  }
  writeFileSync(join(home, READY_FILE), JSON.stringify({ nonce, tools: NETWORK_TOOLS }), { mode: 0o600 });
}

export function verifyWebReadiness(home: string, nonce: string): void {
  try {
    if (!nonce) throw new Error("missing nonce");
    const ready = JSON.parse(readFileSync(join(home, READY_FILE), "utf8"));
    if (ready.nonce !== nonce || JSON.stringify(ready.tools) !== JSON.stringify(NETWORK_TOOLS)) throw new Error("mismatch");
  } catch {
    throw new Error("web-search-tool-unavailable: Explore web facade readiness was not proved");
  }
}
