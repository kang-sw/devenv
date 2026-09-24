import { test } from "node:test";
import assert from "node:assert/strict";
import { proveWebReadiness, verifyWebReadiness } from "../src/web-readiness.ts";

const names = ["web_search", "ws_web_fetch"];
const entry = "/package/src/index.ts";
function api(tools = names.map(name => ({ name, sourceInfo: { path: entry } })), active = names): any {
  return { getAllTools: () => tools, getActiveTools: () => active };
}

// Launch freshness (a stale launch's proof never satisfying the current one)
// is the control channel's per-launch credential + generation; this pins the
// proof the child builds and the payload check the parent applies.
test("launch proof requires exact facade provenance and active tools; the parent accepts only the exact payload", () => {
  const proof = proveWebReadiness(api(), entry);
  assert.deepEqual(proof, { tools: ["web_search", "ws_web_fetch"] });
  assert.doesNotThrow(() => verifyWebReadiness(proof));
  assert.throws(() => proveWebReadiness(api(undefined, ["web_search"]), entry), /registration mismatch/);
  assert.throws(() => proveWebReadiness(api([{ name: "web_search", sourceInfo: { path: "/other/index.ts" } }]), entry), /registration mismatch/);
  assert.throws(() => proveWebReadiness(api([...names.map(name => ({ name, sourceInfo: { path: entry } })), { name: "web_search", sourceInfo: { path: entry } }]), entry), /registration mismatch/);
  for (const payload of [undefined, null, "invalid", {}, { tools: ["web_search"] }, { tools: [...names].reverse() }, { tools: names, error: "web-search-tool-unavailable: Explore web facade registration mismatch" }]) {
    assert.throws(() => verifyWebReadiness(payload), /web-search-tool-unavailable/, JSON.stringify(payload));
  }
});
