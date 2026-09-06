/**
 * Unit tests for lead-bootstrap.ts's pure exports: buildWsBlock (marker line
 * + order, now a third skills-block section per 260906 Phase 1),
 * computeBeforeAgentStartResult (the pure decision extracted from
 * registerLeadBootstrap's before_agent_start handler, mirroring
 * decideOnSettle's pure-reducer precedent in goal-loop.ts — no fake
 * ExtensionAPI/pi.on capture needed), and computeSessionBootstrap (260906
 * Phase 1's testability extraction — the one function that drives
 * index.ts's actual role-gate -> skills-block -> buildWsBlock ->
 * computeLeadActiveTools -> addForkToolIfLead -> addAskToolsIfLead ->
 * addSkillToolIfLeadOrFork sequencing, so this file can assert the
 * post-reshape tool surface directly rather than only the pure helpers in
 * isolation).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { buildWsBlock, computeBeforeAgentStartResult, computeSessionBootstrap, SESSION_START_SNAPSHOT_MARKER } from "../src/lead-bootstrap.ts";
import { WS_SKILL_TOOL_NAME, type LoadedSkill, type SkillEntry } from "../src/lead-skills.ts";
import { FORK_TOOL_NAME } from "../src/fork.ts";
import { ASK_TOOL_NAME, RESOLVE_TOOL_NAME } from "../src/ask.ts";
import { APPROVE_TOOL_NAME, EXECUTE_TOOL_NAME, ONE_LINER_EXEC_TOOL_NAME, UGLY_READ_TOOL_NAME } from "../src/execute-gateway.ts";
import { GATED_EXEC_TOOL_NAME } from "../src/spawner.ts";

describe("buildWsBlock", () => {
  test("prefixes the manual snapshot with the fixed marker line", () => {
    const result = buildWsBlock("## Session Key\nlead-1", "guide text", "");
    assert.ok(result.startsWith(SESSION_START_SNAPSHOT_MARKER), "must start with the fixed marker line");
  });

  test("orders manual snapshot, guide text, then the skills block", () => {
    const result = buildWsBlock("MANUAL-SNAPSHOT-MARKER", "GUIDE-TEXT-MARKER", "SKILLS-BLOCK-MARKER");
    const manualIndex = result.indexOf("MANUAL-SNAPSHOT-MARKER");
    const guideIndex = result.indexOf("GUIDE-TEXT-MARKER");
    const skillsIndex = result.indexOf("SKILLS-BLOCK-MARKER");
    assert.ok(manualIndex >= 0 && guideIndex >= 0 && skillsIndex >= 0, "all three inputs must appear in the output");
    assert.ok(manualIndex < guideIndex && guideIndex < skillsIndex, "order must be manual snapshot, then guide text, then skills block");
  });

  test("tolerates an empty skills block", () => {
    const result = buildWsBlock("manual", "guide", "");
    assert.ok(result.includes("manual") && result.includes("guide"));
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

/**
 * computeSessionBootstrap drives the SAME sequencing index.ts's session_start
 * actually calls (role gate -> skills snapshot -> buildWsBlock ->
 * computeLeadActiveTools -> addForkToolIfLead -> addAskToolsIfLead ->
 * addSkillToolIfLeadOrFork), so asserting against ITS output — rather than a
 * test-local re-implementation of that order — is what satisfies the
 * ticket's "drives before_agent_start through the real index.ts ordering,
 * not only the pure helper" requirement.
 */
describe("computeSessionBootstrap", () => {
  const VISIBLE_SKILL_ENTRIES: SkillEntry[] = [{ name: "lead-proceed", description: "d", path: "/skills/lead-proceed/SKILL.md" }];
  const loadFile = (): LoadedSkill => ({ ok: true, body: "Proceed body.", disableModelInvocation: false });
  // A raw, unreshaped Pi lead session before any setActiveTools call —
  // native bash/read present, nothing ws-owned active yet.
  const RAW_LEAD_TOOLS = ["bash", "read", GATED_EXEC_TOOL_NAME];

  function run(role: "worker" | "explore" | "fork" | undefined) {
    return computeSessionBootstrap({
      role,
      manualSnapshot: "## Session Key\nlead-1",
      guideText: "GUIDE-TEXT",
      skillEntries: VISIBLE_SKILL_ENTRIES,
      loadSkillFile: loadFile,
      currentActiveTools: RAW_LEAD_TOOLS,
    });
  }

  test("host lead (role undefined): produces a ws block and the fully reshaped tool surface", () => {
    const result = run(undefined);
    assert.notEqual(result.wsBlock, undefined);
    assert.ok(result.wsBlock!.startsWith(SESSION_START_SNAPSHOT_MARKER));
    assert.match(result.wsBlock!, /GUIDE-TEXT/);
    assert.match(result.wsBlock!, /<available_skills>/);
    assert.ok(!result.activeTools.includes("bash"), "native bash must be removed");
    assert.ok(!result.activeTools.includes("read"), "native read must be removed");
    assert.ok(!result.activeTools.includes(GATED_EXEC_TOOL_NAME), "the gated-exec tool must stay excluded from the lead");
    for (const name of [EXECUTE_TOOL_NAME, APPROVE_TOOL_NAME, UGLY_READ_TOOL_NAME, ONE_LINER_EXEC_TOOL_NAME, FORK_TOOL_NAME, ASK_TOOL_NAME, RESOLVE_TOOL_NAME, WS_SKILL_TOOL_NAME]) {
      assert.ok(result.activeTools.includes(name), `expected ${name} on the host lead's reshaped surface`);
    }
  });

  test("fork role: produces a ws block and ws-skill, but never ws-fork/ws-ask/ws-resolve", () => {
    const result = run("fork");
    assert.notEqual(result.wsBlock, undefined);
    assert.match(result.wsBlock!, /<available_skills>/);
    assert.ok(!result.activeTools.includes("bash"));
    assert.ok(!result.activeTools.includes("read"));
    assert.ok(result.activeTools.includes(WS_SKILL_TOOL_NAME), "ws-skill must be present for a fork");
    assert.ok(result.activeTools.includes(EXECUTE_TOOL_NAME), "the shared lead/fork added-set must still apply to a fork");
    for (const name of [FORK_TOOL_NAME, ASK_TOOL_NAME, RESOLVE_TOOL_NAME]) {
      assert.ok(!result.activeTools.includes(name), `${name} must never be present on a fork's own surface`);
    }
  });

  test("worker role: no ws block, tool surface passed through unchanged", () => {
    const result = run("worker");
    assert.equal(result.wsBlock, undefined);
    assert.deepEqual(result.activeTools, RAW_LEAD_TOOLS);
  });

  test("explore role: no ws block, tool surface passed through unchanged", () => {
    const result = run("explore");
    assert.equal(result.wsBlock, undefined);
    assert.deepEqual(result.activeTools, RAW_LEAD_TOOLS);
  });

  test("no manual snapshot (degraded bootstrap): no ws block even for the lead, but the tool surface is still reshaped", () => {
    const result = computeSessionBootstrap({
      role: undefined,
      manualSnapshot: undefined,
      guideText: "GUIDE-TEXT",
      skillEntries: VISIBLE_SKILL_ENTRIES,
      loadSkillFile: loadFile,
      currentActiveTools: RAW_LEAD_TOOLS,
    });
    assert.equal(result.wsBlock, undefined);
    assert.ok(!result.activeTools.includes("bash"));
    assert.ok(result.activeTools.includes(WS_SKILL_TOOL_NAME));
  });
});
