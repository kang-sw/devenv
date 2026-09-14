import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { clearWebReadiness, verifyWebReadiness, writeWebReadiness } from "../src/web-readiness.ts";

const names = ["web_search", "ws_web_fetch"];
const entry = "/package/src/index.ts";
function api(tools = names.map(name => ({ name, sourceInfo: { path: entry } })), active = names): any {
  return { getAllTools: () => tools, getActiveTools: () => active };
}

test("launch proof requires exact facade provenance, active tools, and matching nonce", () => {
  const home = mkdtempSync(join(tmpdir(), "ws-web-readiness-"));
  try {
    assert.throws(() => verifyWebReadiness(home, "launch"), /web-search-tool-unavailable/);
    writeWebReadiness(api(), home, "launch", entry);
    assert.doesNotThrow(() => verifyWebReadiness(home, "launch"));
    assert.throws(() => verifyWebReadiness(home, "stale"), /web-search-tool-unavailable/);
    assert.throws(() => writeWebReadiness(api(undefined, ["web_search"]), home, "next", entry), /registration mismatch/);
    assert.throws(() => writeWebReadiness(api([{ name: "web_search", sourceInfo: { path: "/other/index.ts" } }]), home, "next", entry), /registration mismatch/);
    assert.throws(() => writeWebReadiness(api([...names.map(name => ({ name, sourceInfo: { path: entry } })), { name: "web_search", sourceInfo: { path: entry } }]), home, "next", entry), /registration mismatch/);
    clearWebReadiness(home);
    assert.throws(() => verifyWebReadiness(home, "launch"), /web-search-tool-unavailable/);
    writeFileSync(join(home, "web-tools-ready.json"), "invalid");
    assert.throws(() => verifyWebReadiness(home, "launch"), /web-search-tool-unavailable/);
  } finally { rmSync(home, { recursive: true, force: true }); }
});
