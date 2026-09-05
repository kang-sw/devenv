/**
 * Unit tests for version-check.ts's assertVersionPin: a matching version
 * passes silently, a mismatch throws synchronously (before any tools get
 * registered — see bridge.ts's ordering, confirmed correct in review).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { assertVersionPin, type RuntimeContract } from "../src/version-check.ts";

describe("assertVersionPin", () => {
  test("does not throw when the bundled runtime.json version matches the live server version", () => {
    const runtime: RuntimeContract = { plugin: "ws", plugin_version: "0.43.4" };
    assert.doesNotThrow(() => assertVersionPin(runtime, "0.43.4"));
  });

  test("throws synchronously when the versions mismatch", () => {
    const runtime: RuntimeContract = { plugin: "ws", plugin_version: "0.43.4" };
    assert.throws(
      () => assertVersionPin(runtime, "0.44.0"),
      (err: unknown) => {
        assert.ok(err instanceof Error);
        assert.match(err.message, /0\.43\.4/);
        assert.match(err.message, /0\.44\.0/);
        return true;
      },
    );
  });
});

/**
 * Review relay #1 (Important, correctness): version-check.ts's own header
 * comment declares agents-plugin-pi/runtime.json a "hand-synced,
 * byte-identical copy of agents-plugin/runtime.json" with "no sync tooling
 * ... to keep these in lockstep automatically" — the two files desynced
 * once already (pi's copy was stuck at 0.43.4 / missing config.resolve_agent
 * while agents-plugin/runtime.json had moved to 0.44.4), and nothing in
 * `npm test` caught it, because the other tests in this file feed a runtime
 * object's own `plugin_version` back into `assertVersionPin` rather than
 * reading the bundled file from disk. This test closes that specific gap:
 * it reads both files directly and fails loudly on the next desync.
 */
describe("agents-plugin-pi/runtime.json hand-sync (review relay #1, Important #2)", () => {
  const testDir = dirname(fileURLToPath(import.meta.url));
  const piRuntimePath = join(testDir, "..", "runtime.json");
  const sourceRuntimePath = join(testDir, "..", "..", "agents-plugin", "runtime.json");

  test("is byte-identical to agents-plugin/runtime.json", () => {
    const piRuntime = readFileSync(piRuntimePath, "utf8");
    const sourceRuntime = readFileSync(sourceRuntimePath, "utf8");
    assert.equal(
      piRuntime,
      sourceRuntime,
      "agents-plugin-pi/runtime.json must be re-copied verbatim from agents-plugin/runtime.json whenever the source changes (no shared sync tooling exists yet)",
    );
  });
});
