/**
 * Unit tests for discuss.ts: buildDiscussKickoff pins the `/ws-discuss` kickoff
 * wording that steers the Phase 4 live gate — the leading `/skill:lead-discuss`
 * expansion (skills-load), the passthrough of the user topic as skill args, the
 * default topic on blank input, and the appended `explore` spawn instruction
 * (the gate-(c) proof that is NOT inherent to the discuss skill).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { buildDiscussKickoff } from "../src/discuss.ts";

describe("buildDiscussKickoff", () => {
  test("starts with the /skill:lead-discuss expansion so the skill loads", () => {
    const kickoff = buildDiscussKickoff("migrate the bridge");
    assert.ok(kickoff.startsWith("/skill:lead-discuss "), "must lead with the skill command for expandPromptTemplates to expand it");
  });

  test("passes the trimmed user topic as the skill args", () => {
    const kickoff = buildDiscussKickoff("  scope the MVP  ");
    assert.ok(kickoff.startsWith("/skill:lead-discuss scope the MVP\n"), "topic rides the first line as the skill's User: args");
  });

  test("falls back to a default topic when args are blank", () => {
    const blank = buildDiscussKickoff("   ");
    const empty = buildDiscussKickoff("");
    assert.equal(blank, empty, "whitespace-only args behave like empty args");
    assert.ok(blank.startsWith("/skill:lead-discuss the ws-pi-native MVP"), "blank input uses the default PoC topic");
  });

  test("always appends the explore spawn instruction (gate (c) proof)", () => {
    const withArgs = buildDiscussKickoff("anything");
    const blank = buildDiscussKickoff("");
    for (const kickoff of [withArgs, blank]) {
      assert.match(kickoff, /`explore`/, "must name the explore leaf so the spawn round-trip is deterministic");
      assert.match(kickoff, /bridge and the delegation spawner compose/, "must state the compose-proof intent");
    }
  });

  test("separates the skill line from the spawn instruction with a blank line", () => {
    const kickoff = buildDiscussKickoff("topic");
    assert.match(kickoff, /^\/skill:lead-discuss topic\n\n/, "blank line keeps the spawn instruction out of the skill command line");
  });
});
