/**
 * Unit tests for version-check.ts's assertVersionPin: a matching version
 * passes silently, a mismatch throws synchronously (before any tools get
 * registered — see bridge.ts's ordering, confirmed correct in review).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
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
