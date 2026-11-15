import { test } from "node:test";
import assert from "node:assert/strict";
import { boot, leadKeyEntry } from "./fixtures/lead-key-bridge.ts";

const warning = "ws-pi-bridge: non-main Pi process has no known parent; set WS_PI_PARENT_SESSION_KEY to the launching lead's session key. Falling back to a top-level lead key.";
const warnings = (h: Awaited<ReturnType<typeof boot>>) => h.notices.filter(n => n.level === "warning").map(n => n.message);

for (const role of ["worker", "explore", "fork"]) {
  test(`${role} without a policy bootstraps as a parent-bound lead and default-fills only omitted keys`, async () => {
    const h = await boot({}, { role, parentKey: " lead-now ", sessionId: "child", entries: [leadKeyEntry("child", "inherited-lead")] });
    try {
      assert.deepEqual(h.calls().filter(c => c.name === "ferrule").map(c => c.args), [{ root: h.cwd, format: "json", parent_session_key: "lead-now" }]);
      assert.deepEqual(warnings(h), []);
      await h.tools.get("ws__workflow_manual").execute("omitted", {});
      await h.tools.get("ws__workflow_manual").execute("explicit", { session_key: "rendered-worker-key" });
      const manualKeys = h.calls().filter(c => c.name === "workflow_manual" || c.name === "workflow_state").map(c => c.args.session_key);
      assert.ok(manualKeys.includes("minted-1"));
      assert.equal(manualKeys.at(-1), "rendered-worker-key", "explicit no-policy keys remain unchanged");
      assert.deepEqual(h.appended, [], "children do not persist a top-level lead key");
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test(`${role} with no parent keeps legacy ferrule arguments and warns once`, async () => {
    const h = await boot({}, { role });
    try {
      assert.deepEqual(h.calls().filter(c => c.name === "ferrule").map(c => c.args), [{ root: h.cwd, format: "json" }]);
      await h.tools.get("ws__workflow_manual").execute("omitted", {});
      assert.deepEqual(warnings(h), [warning]);
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
    } finally { h.handle.shutdown(); h.restore(); }
  });
}

test("unknown parent retries exactly once with legacy arguments and the same warning", async () => {
  const h = await boot({ parentErrors: { pruned: 'session bootstrap: parent_session_key "pruned" is not a known session key' } }, { role: "worker", parentKey: "pruned" });
  try {
    assert.deepEqual(h.calls().filter(c => c.name === "ferrule").map(c => c.args), [
      { root: h.cwd, format: "json", parent_session_key: "pruned" },
      { root: h.cwd, format: "json" },
    ]);
    assert.deepEqual(warnings(h), [warning]);
    assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
  } finally { h.handle.shutdown(); h.restore(); }
});

test("other parent bootstrap refusals never retry a potentially successful mint", async () => {
  const h = await boot({ parentErrors: { parent: "session store unavailable" } }, { role: "worker", parentKey: "parent" });
  try {
    assert.equal(h.mints(), 1);
    assert.equal(h.handle.defaultSessionKeyRef.current, undefined);
    assert.deepEqual(warnings(h), ["ws-pi-bridge: ferrule bootstrap failed: session store unavailable"]);
  } finally { h.handle.shutdown(); h.restore(); }
});

test("main bootstrap ignores inherited parent env and keeps harness reporting", async () => {
  const h = await boot({}, { parentKey: "ancestor", sessionId: "main", sessionFile: "/sessions/main.jsonl" });
  try {
    assert.deepEqual(h.calls().filter(c => c.name === "ferrule").map(c => c.args), [{ root: h.cwd, format: "json", harness_session: "/sessions/main.jsonl", harness_cwd: h.cwd }]);
    assert.deepEqual(warnings(h), []);
  } finally { h.handle.shutdown(); h.restore(); }
});

test("policy pre-minted keys are reused and foreign explicit keys stay refused", async () => {
  const h = await boot({}, { role: "worker", parentKey: "ignored", policy: { version: 1, depth: 1, maxDepth: 2, authority: "lead", tools: ["ws__workflow_manual"], write: { mode: "none" }, sessionKey: "child-own" } });
  try {
    assert.equal(h.mints(), 0);
    assert.deepEqual(warnings(h), []);
    assert.equal(h.handle.defaultSessionKeyRef.current, "child-own");
    await assert.rejects(() => h.tools.get("ws__workflow_manual").execute("foreign", { session_key: "foreign" }), /session key/i);
  } finally { h.handle.shutdown(); h.restore(); }
});
