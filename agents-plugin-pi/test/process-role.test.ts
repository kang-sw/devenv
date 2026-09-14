/**
 * Unit tests for process-role.ts's pure exports: `readSpawnRole`'s
 * valid/invalid/absent validation and `isLeadOrFork`'s predicate over all
 * four process-role states (absent/host-lead, worker, explore, fork).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readSpawnRole, isLeadOrFork, WS_PI_SPAWN_ROLE_ENV, WS_PI_PARENT_SESSION_KEY_ENV } from "../src/process-role.ts";

describe("readSpawnRole", () => {
  test("returns \"worker\" for a valid worker marker", () => {
    assert.equal(readSpawnRole({ [WS_PI_SPAWN_ROLE_ENV]: "worker" }), "worker");
  });

  test("returns \"explore\" for a valid explore marker", () => {
    assert.equal(readSpawnRole({ [WS_PI_SPAWN_ROLE_ENV]: "explore" }), "explore");
  });

  test("returns \"fork\" for a valid fork marker (reserved, not yet spawned by any code path)", () => {
    assert.equal(readSpawnRole({ [WS_PI_SPAWN_ROLE_ENV]: "fork" }), "fork");
  });

  test("returns undefined when the marker is absent (host lead)", () => {
    assert.equal(readSpawnRole({}), undefined);
  });

  test("returns undefined for an unrecognized/invalid role value", () => {
    assert.equal(readSpawnRole({ [WS_PI_SPAWN_ROLE_ENV]: "bogus" }), undefined);
  });

  test("returns undefined for an empty-string role value", () => {
    assert.equal(readSpawnRole({ [WS_PI_SPAWN_ROLE_ENV]: "" }), undefined);
  });

  test("WS_PI_PARENT_SESSION_KEY_ENV is exported as a distinct constant (reserved for a future fork spawn path)", () => {
    assert.equal(WS_PI_PARENT_SESSION_KEY_ENV, "WS_PI_PARENT_SESSION_KEY");
    assert.notEqual(WS_PI_PARENT_SESSION_KEY_ENV, WS_PI_SPAWN_ROLE_ENV);
  });
});

describe("isLeadOrFork", () => {
  test("true for undefined (host lead, no role marker)", () => {
    assert.equal(isLeadOrFork(undefined), true);
  });

  test("true for \"fork\"", () => {
    assert.equal(isLeadOrFork("fork"), true);
  });

  test("false for \"worker\"", () => {
    assert.equal(isLeadOrFork("worker"), false);
  });

  test("false for \"explore\"", () => {
    assert.equal(isLeadOrFork("explore"), false);
  });
});
