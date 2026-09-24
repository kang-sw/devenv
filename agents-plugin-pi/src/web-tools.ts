import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { dirname } from "node:path";
import { readOwnership } from "./agent-storage.ts";
import { assertPolicyTool, readDelegationPolicy } from "./delegation-policy.ts";
import { readSpawnRole } from "./process-role.ts";
import { boundedWebFetch, WEB_FETCH_CAPS } from "./web-fetch.ts";
import { createWebSearch, webSearchParameters } from "./web-search.ts";
import { WEB_HOME_ENV, WEB_READINESS_KIND, proveWebReadiness, type WebReadiness } from "./web-readiness.ts";
import { registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import type { ChildChannel } from "./agent-channel.ts";

export const webFetchParameters = {
  type: "object", additionalProperties: false, required: ["url"],
  properties: {
    url: { type: "string", minLength: 1 },
    options: { type: "object", additionalProperties: false, properties: Object.fromEntries(Object.entries(WEB_FETCH_CAPS).map(([key, maximum]) => [key, { type: "integer", minimum: key === "maxRedirects" || key === "inlineBytes" ? 0 : 1, maximum }])) },
  },
};

/**
 * Only Explore receives the active web surface; other roles carry authority for
 * delegation. `channel` is the child's control channel to its parent; the
 * readiness proof is published on it after `session_start` (stage 2), and a
 * child without one (not spawned by the adapter) proves locally but publishes nowhere.
 */
export function registerWebTools(pi: ExtensionAPI, extensionPath: string, tui: ToolPreviewTuiRef, env: NodeJS.ProcessEnv = process.env, channel?: ChildChannel): void {
  if (readSpawnRole(env) !== "explore") return;
  const policy = readDelegationPolicy(env);
  const home = env[WEB_HOME_ENV] ?? "";
  const search = createWebSearch({ packageRoot: dirname(dirname(extensionPath)), env });
  registerWsTool(pi, {
    name: "web_search", label: "Web search",
    description: "Search the web with one query through the bundled, isolated direct-only provider boundary. Returns bounded normalized external search metadata. On failure inspect the redacted diagnostic's local README/configuration paths for setup; no transport proxies are supported.",
    parameters: webSearchParameters as never,
    async execute(_id, args, signal) {
      if (!policy) throw new Error("web-search-tool-unavailable: network authority missing");
      assertPolicyTool(policy, "web_search");
      return search.execute(args, signal);
    },
  }, tui);
  registerWsTool(pi, {
    name: "ws_web_fetch", label: "Bounded web fetch",
    description: "Retrieve a public HTTP(S) URL with bounded GET transport. Returns sanitized, explicitly untrusted content inline up to 8 KiB, otherwise an owned cache path. No credentials, custom headers, private destinations, or writes to the repository. Optional limits may only lower server caps.",
    parameters: webFetchParameters as never,
    async execute(_id, args: any, signal) {
      if (!policy) throw new Error("ws_web_fetch: network authority missing");
      assertPolicyTool(policy, "ws_web_fetch");
      if (!args || typeof args !== "object" || Array.isArray(args) || Object.keys(args).some(key => key !== "url" && key !== "options")) throw new Error("ws_web_fetch: invalid arguments");
      const ownership = readOwnership(home);
      if (!ownership || ownership.role !== "explore") throw new Error("ws_web_fetch: owned Explore cache unavailable");
      const result = await boundedWebFetch({ url: args.url, cacheHome: ownership.home, signal, options: args.options });
      const { content, ...metadata } = result;
      return { content: [{ type: "text", text: content ? `${JSON.stringify(metadata)}\n${content}` : JSON.stringify(metadata) }], details: metadata };
    },
  }, tui);
  pi.on("session_start", () => {
    // A proof failure is published too, so the parent fails the launch at
    // once instead of waiting out its readiness bound.
    let readiness: WebReadiness | { error: string };
    try { readiness = proveWebReadiness(pi, extensionPath); }
    catch (error) { readiness = { error: error instanceof Error ? error.message : String(error) }; }
    channel?.publishReadiness(WEB_READINESS_KIND, readiness);
    if ("error" in readiness) throw new Error(readiness.error);
  });
}
