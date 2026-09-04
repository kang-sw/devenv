/**
 * Unit tests for lead-bootstrap.ts's pure exports: buildWsBlock (marker line
 * + order) and computeBeforeAgentStartResult (the pure decision extracted
 * from registerLeadBootstrap's before_agent_start handler, mirroring
 * decideOnSettle's pure-reducer precedent in goal-loop.ts — no fake
 * ExtensionAPI/pi.on capture needed).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { buildWsBlock, computeBeforeAgentStartResult, SESSION_START_SNAPSHOT_MARKER } from "../src/lead-bootstrap.ts";

describe("buildWsBlock", () => {
  test("prefixes the manual snapshot with the fixed marker line", () => {
    const result = buildWsBlock("## Session Key\nlead-1", "guide text");
    assert.ok(result.startsWith(SESSION_START_SNAPSHOT_MARKER), "must start with the fixed marker line");
  });

  test("orders manual snapshot before the guide text", () => {
    const result = buildWsBlock("MANUAL-SNAPSHOT-MARKER", "GUIDE-TEXT-MARKER");
    const manualIndex = result.indexOf("MANUAL-SNAPSHOT-MARKER");
    const guideIndex = result.indexOf("GUIDE-TEXT-MARKER");
    assert.ok(manualIndex >= 0 && guideIndex >= 0, "both inputs must appear in the output");
    assert.ok(manualIndex < guideIndex, "manual snapshot must come before the guide text");
  });
});

describe("computeBeforeAgentStartResult", () => {
  const SYSTEM_PROMPT = "base system prompt";
  const WS_BLOCK = "WS-BLOCK-CONTENT";

  test("returns undefined for a worker role, even with a ws block set", () => {
    assert.equal(computeBeforeAgentStartResult(SYSTEM_PROMPT, WS_BLOCK, "worker"), undefined);
  });

  test("returns undefined for an explore role, even with a ws block set", () => {
    assert.equal(computeBeforeAgentStartResult(SYSTEM_PROMPT, WS_BLOCK, "explore"), undefined);
  });

  test("returns the chained systemPrompt for the host lead (role undefined)", () => {
    const result = computeBeforeAgentStartResult(SYSTEM_PROMPT, WS_BLOCK, undefined);
    assert.equal(result?.systemPrompt, `${SYSTEM_PROMPT}\n\n${WS_BLOCK}`);
  });

  test("returns the chained systemPrompt for a fork role", () => {
    const result = computeBeforeAgentStartResult(SYSTEM_PROMPT, WS_BLOCK, "fork");
    assert.equal(result?.systemPrompt, `${SYSTEM_PROMPT}\n\n${WS_BLOCK}`);
  });

  test("returns undefined for the host lead when the ws block is not yet filled (bootstrap in flight / degraded)", () => {
    assert.equal(computeBeforeAgentStartResult(SYSTEM_PROMPT, undefined, undefined), undefined);
  });

  test("always appends (never replaces) the given systemPrompt", () => {
    const result = computeBeforeAgentStartResult("turn-specific prompt", WS_BLOCK, undefined);
    assert.ok(result?.systemPrompt.startsWith("turn-specific prompt"));
    assert.ok(result?.systemPrompt.endsWith(WS_BLOCK));
  });
});
