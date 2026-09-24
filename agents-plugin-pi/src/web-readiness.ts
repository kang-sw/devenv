/**
 * Per-launch proof that the exact ws extension registered both Explore web
 * facades. The child proves it in `session_start` and publishes the payload
 * as the stage-2 readiness message on its control channel; the parent
 * validates it at the same launch and relaunch points that used to read the
 * `web-tools-ready.json` file. The channel's per-launch credential and
 * generation replace the nonce that file carried.
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { NETWORK_TOOLS } from "./delegation-policy.ts";

export const WEB_HOME_ENV = "WS_PI_WEB_HOME";
/** Readiness kind the Explore child publishes on the control channel. */
export const WEB_READINESS_KIND = "web";

export interface WebReadiness { tools: readonly string[] }

export function proveWebReadiness(pi: ExtensionAPI, extensionPath: string): WebReadiness {
  const registered = pi.getAllTools();
  const active = pi.getActiveTools();
  for (const name of NETWORK_TOOLS) {
    const matches = registered.filter(tool => tool.name === name);
    if (matches.length !== 1 || !active.includes(name) || matches[0].sourceInfo?.path !== extensionPath) {
      throw new Error("web-search-tool-unavailable: Explore web facade registration mismatch");
    }
  }
  return { tools: NETWORK_TOOLS };
}

export function verifyWebReadiness(payload: unknown): void {
  const ready = payload as (Partial<WebReadiness> & { error?: unknown }) | null | undefined;
  // A payload carrying `error` is the child's own proof failure, published so
  // the parent fails at once instead of at its readiness bound.
  if (!ready || typeof ready !== "object" || ready.error !== undefined || JSON.stringify(ready.tools) !== JSON.stringify(NETWORK_TOOLS)) {
    throw new Error("web-search-tool-unavailable: Explore web facade readiness was not proved");
  }
}
