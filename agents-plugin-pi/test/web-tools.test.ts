import { test } from "node:test";
import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";
import { mkdtempSync, mkdirSync, writeFileSync, existsSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { registerWebTools, webFetchParameters } from "../src/web-tools.ts";
import { createToolPreviewTuiRef } from "../src/tool-result-render.ts";
import { allocateAgentHome, createAgentStorageContext } from "../src/agent-storage.ts";
import { WEB_HOME_ENV } from "../src/web-readiness.ts";
import { DELEGATION_ENV } from "../src/delegation-policy.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";

const entry = fileURLToPath(new URL("../src/index.ts", import.meta.url));
function tools(role?: string, policy?: unknown, home = "") {
  const registered = new Map<string, any>();
  const pi = { registerTool: (tool: any) => registered.set(tool.name, tool), on() {} };
  registerWebTools(pi as never, entry, createToolPreviewTuiRef(), { [WS_PI_SPAWN_ROLE_ENV]: role, [DELEGATION_ENV]: policy ? JSON.stringify(policy) : "", [WEB_HOME_ENV]: home });
  return registered;
}
const authority = { version: 1, depth: 1, maxDepth: 2, tools: ["web_search", "ws_web_fetch"], network: { search: true, fetch: true }, authority: "leaf" };

test("only Explore registers ws-owned web tools; all schemas are closed", () => {
  for (const role of [undefined, "worker", "fork", "execute-worker"]) assert.equal(tools(role).size, 0);
  const registered = tools("explore", authority);
  assert.deepEqual([...registered.keys()], ["web_search", "ws_web_fetch"]);
  assert.deepEqual(Object.keys(registered.get("web_search").parameters.properties), ["query"]);
  assert.equal(registered.get("web_search").parameters.additionalProperties, false);
  assert.equal(webFetchParameters.additionalProperties, false);
  assert.equal(webFetchParameters.properties.options.additionalProperties, false);
});

test("runtime invocation enforces explicit authority, unknown args, owned cache, and SSRF boundary", async () => {
  await assert.rejects(() => tools("explore").get("web_search").execute("id", { query: "test" }), /authority missing/);
  await assert.rejects(() => tools("explore").get("ws_web_fetch").execute("id", { url: "https://example.com" }), /authority missing/);
  const denied = tools("explore", { ...authority, network: { search: false, fetch: false } });
  await assert.rejects(() => denied.get("web_search").execute("id", { query: "test" }), /ceiling/);
  const registered = tools("explore", authority);
  await assert.rejects(() => registered.get("ws_web_fetch").execute("id", { url: "https://example.com", headers: {} }), /invalid arguments/);
  await assert.rejects(() => registered.get("ws_web_fetch").execute("id", { url: "https://example.com" }), /owned Explore cache unavailable/);
  const root = mkdtempSync(join(tmpdir(), "ws-web-tools-"));
  try {
    const owner = allocateAgentHome(createAgentStorageContext("owner", root), "researcher", "explore", "simple");
    const fetch = tools("explore", authority, owner.home).get("ws_web_fetch");
    await assert.rejects(() => fetch.execute("id", { url: "http://127.0.0.1/" }), /public|private|destination|address/i);
    // Fetch artifacts share the ownership unit, not a global or repository cache.
    const artifact = join(owner.home, "web-fetch", "example", "content.md");
    mkdirSync(join(owner.home, "web-fetch", "example"), { recursive: true });
    writeFileSync(artifact, "external data");
    rmSync(owner.home, { recursive: true });
    assert.equal(existsSync(artifact), false);
  } finally { rmSync(root, { recursive: true, force: true }); }
});
