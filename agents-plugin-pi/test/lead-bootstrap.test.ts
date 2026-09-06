/**
 * Unit tests for lead-bootstrap.ts's pure exports: buildWsBlock (marker line
 * + order, now a third skills-block section per 260906 Phase 1),
 * computeBeforeAgentStartResult (the pure decision extracted from
 * registerLeadBootstrap's before_agent_start handler, mirroring
 * decideOnSettle's pure-reducer precedent in goal-loop.ts — no fake
 * ExtensionAPI/pi.on capture needed), computeSessionBootstrap (260906 Phase
 * 1's testability extraction — the one function that drives index.ts's
 * actual role-gate -> computeLeadActiveTools -> addForkToolIfLead ->
 * addAskToolsIfLead -> addSkillToolIfLeadOrFork sequencing, so this file can
 * assert the post-reshape tool surface directly rather than only the pure
 * helpers in isolation), computeSkillsBlockCached (the dogfood fix's
 * live-read-plus-cache helper), and registerLeadBootstrap itself (fake
 * pi.on/pi.getCommands capture) for the end-to-end dogfood-bug scenario.
 *
 * Dogfood bug covered here: `pi.getCommands()` returned only `imagegen` (or
 * whatever other extension's skills had already registered) at
 * `session_start` time, because Pi merges this adapter's own
 * `resources_discover` skills into the live command list AFTER
 * `session_start` returns. `computeSessionBootstrap` no longer touches
 * skills at all; the `<available_skills>` block is now built inside
 * `registerLeadBootstrap`'s `before_agent_start` handler, against a live
 * `pi.getCommands()` read.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, before, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI, SlashCommandInfo, SourceInfo } from "@earendil-works/pi-coding-agent";
import {
  buildWsBlock,
  computeBeforeAgentStartResult,
  computeSessionBootstrap,
  computeSkillsBlockCached,
  registerLeadBootstrap,
  SESSION_START_SNAPSHOT_MARKER,
  type SkillsBlockCache,
  type WsBlockBase,
} from "../src/lead-bootstrap.ts";
import { WS_SKILL_TOOL_NAME, type LoadedSkill } from "../src/lead-skills.ts";
import { FORK_TOOL_NAME } from "../src/fork.ts";
import { ASK_TOOL_NAME, RESOLVE_TOOL_NAME } from "../src/ask.ts";
import { APPROVE_TOOL_NAME, EXECUTE_TOOL_NAME, ONE_LINER_EXEC_TOOL_NAME, UGLY_READ_TOOL_NAME } from "../src/execute-gateway.ts";
import { GATED_EXEC_TOOL_NAME } from "../src/spawner.ts";

function sourceInfo(path: string): SourceInfo {
  return { path, source: "test", scope: "project", origin: "top-level" };
}

function skillCommand(name: string, description: string, path: string): SlashCommandInfo {
  return { name: `skill:${name}`, description, source: "skill", sourceInfo: sourceInfo(path) };
}

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
 * actually calls (role gate -> computeLeadActiveTools -> addForkToolIfLead ->
 * addAskToolsIfLead -> addSkillToolIfLeadOrFork), so asserting against ITS
 * output — rather than a test-local re-implementation of that order — is
 * what satisfies the ticket's "drives before_agent_start through the real
 * index.ts ordering, not only the pure helper" requirement. Skills are
 * deliberately absent from these inputs/assertions: this function no longer
 * touches the `<available_skills>` piece at all (see this file's header).
 */
describe("computeSessionBootstrap", () => {
  // A raw, unreshaped Pi lead session before any setActiveTools call —
  // native bash/read present, nothing ws-owned active yet.
  const RAW_LEAD_TOOLS = ["bash", "read", GATED_EXEC_TOOL_NAME];

  function run(role: "worker" | "explore" | "fork" | undefined) {
    return computeSessionBootstrap({
      role,
      manualSnapshot: "## Session Key\nlead-1",
      guideText: "GUIDE-TEXT",
      currentActiveTools: RAW_LEAD_TOOLS,
    });
  }

  test("host lead (role undefined): produces a ws block base and the fully reshaped tool surface", () => {
    const result = run(undefined);
    assert.deepEqual(result.wsBlockBase, { manualSnapshot: "## Session Key\nlead-1", guideText: "GUIDE-TEXT" });
    assert.ok(!result.activeTools.includes("bash"), "native bash must be removed");
    assert.ok(!result.activeTools.includes("read"), "native read must be removed");
    assert.ok(!result.activeTools.includes(GATED_EXEC_TOOL_NAME), "the gated-exec tool must stay excluded from the lead");
    for (const name of [EXECUTE_TOOL_NAME, APPROVE_TOOL_NAME, UGLY_READ_TOOL_NAME, ONE_LINER_EXEC_TOOL_NAME, FORK_TOOL_NAME, ASK_TOOL_NAME, RESOLVE_TOOL_NAME, WS_SKILL_TOOL_NAME]) {
      assert.ok(result.activeTools.includes(name), `expected ${name} on the host lead's reshaped surface`);
    }
  });

  test("fork role: produces a ws block base and ws-skill, but never ws-fork/ws-ask/ws-resolve", () => {
    const result = run("fork");
    assert.notEqual(result.wsBlockBase, undefined);
    assert.ok(!result.activeTools.includes("bash"));
    assert.ok(!result.activeTools.includes("read"));
    assert.ok(result.activeTools.includes(WS_SKILL_TOOL_NAME), "ws-skill must be present for a fork");
    assert.ok(result.activeTools.includes(EXECUTE_TOOL_NAME), "the shared lead/fork added-set must still apply to a fork");
    for (const name of [FORK_TOOL_NAME, ASK_TOOL_NAME, RESOLVE_TOOL_NAME]) {
      assert.ok(!result.activeTools.includes(name), `${name} must never be present on a fork's own surface`);
    }
  });

  test("worker role: no ws block base, tool surface passed through unchanged", () => {
    const result = run("worker");
    assert.equal(result.wsBlockBase, undefined);
    assert.deepEqual(result.activeTools, RAW_LEAD_TOOLS);
  });

  test("explore role: no ws block base, tool surface passed through unchanged", () => {
    const result = run("explore");
    assert.equal(result.wsBlockBase, undefined);
    assert.deepEqual(result.activeTools, RAW_LEAD_TOOLS);
  });

  test("no manual snapshot (degraded bootstrap): no ws block base even for the lead, but the tool surface is still reshaped", () => {
    const result = computeSessionBootstrap({
      role: undefined,
      manualSnapshot: undefined,
      guideText: "GUIDE-TEXT",
      currentActiveTools: RAW_LEAD_TOOLS,
    });
    assert.equal(result.wsBlockBase, undefined);
    assert.ok(!result.activeTools.includes("bash"));
    assert.ok(result.activeTools.includes(WS_SKILL_TOOL_NAME));
  });
});

/**
 * computeSkillsBlockCached: the dogfood fix's live-read-plus-cache helper.
 * Reads `pi.getCommands()` (here, a plain array standing in for it) fresh on
 * every call, and rebuilds the rendered block only when the sorted, joined
 * set of live entry paths differs from the key the cache was last built
 * from — see this function's own doc comment in lead-bootstrap.ts for why a
 * path-set key replaced an earlier "freeze on first non-empty read" rule
 * (review cycle 1, Minor #1/#2: that rule could latch a pre-merge list, and
 * separately cached an empty string forever when every entry failed to load
 * or was disabled).
 */
describe("computeSkillsBlockCached", () => {
  const loadFile = (): LoadedSkill => ({ ok: true, body: "body", disableModelInvocation: false });
  const FAILING_LOAD_FILE = (): LoadedSkill => ({ ok: false, error: "boom" });

  test("returns '' for an empty live list, and a later non-empty list still invalidates that cached entry", () => {
    const cache: { current: SkillsBlockCache | undefined } = { current: undefined };
    assert.equal(computeSkillsBlockCached([], loadFile, cache), "");
    assert.equal(cache.current?.key, "", "an empty list is cached under the empty key, same as any other path set");

    const block = computeSkillsBlockCached([skillCommand("lead-proceed", "d", "/skills/lead-proceed/SKILL.md")], loadFile, cache);
    assert.match(block, /<name>lead-proceed<\/name>/, "a non-empty path set must not be masked by the earlier empty-key cache entry");
  });

  test("builds the block and stores {key, block} on the first non-empty read", () => {
    const cache: { current: SkillsBlockCache | undefined } = { current: undefined };
    const commands = [skillCommand("lead-proceed", "d", "/skills/lead-proceed/SKILL.md")];
    const block = computeSkillsBlockCached(commands, loadFile, cache);
    assert.match(block, /<name>lead-proceed<\/name>/);
    assert.equal(cache.current?.block, block);
    assert.equal(cache.current?.key, "/skills/lead-proceed/SKILL.md");
  });

  // (a) same path set -> cached block reused without rebuilding.
  test("an unchanged path set returns the cached block without rebuilding", () => {
    const cache: { current: SkillsBlockCache | undefined } = { current: undefined };
    const commands = [skillCommand("lead-proceed", "d", "/skills/lead-proceed/SKILL.md")];
    let loadCalls = 0;
    const countingLoadFile = (): LoadedSkill => {
      loadCalls += 1;
      return { ok: true, body: "body", disableModelInvocation: false };
    };
    const first = computeSkillsBlockCached(commands, countingLoadFile, cache);
    const second = computeSkillsBlockCached([...commands], countingLoadFile, cache);
    assert.equal(second, first);
    assert.equal(loadCalls, 1, "a second call with the same path set must not re-read any SKILL.md");
  });

  // (b) a path added after the first build -> block rebuilt, new skill listed.
  test("a path added to the live list forces a rebuild and lists the new skill", () => {
    const cache: { current: SkillsBlockCache | undefined } = { current: undefined };
    const first = computeSkillsBlockCached([skillCommand("lead-proceed", "d", "/skills/lead-proceed/SKILL.md")], loadFile, cache);
    const second = computeSkillsBlockCached(
      [skillCommand("lead-proceed", "d", "/skills/lead-proceed/SKILL.md"), skillCommand("lead-discuss", "d2", "/skills/lead-discuss/SKILL.md")],
      loadFile,
      cache,
    );
    assert.notEqual(second, first);
    assert.match(second, /<name>lead-proceed<\/name>/);
    assert.match(second, /<name>lead-discuss<\/name>/, "the newly merged skill must be listed after the rebuild");
  });

  // (c) first read whose entries all fail to load / are all disabled,
  // followed by a merge that adds a loadable skill -> the new skill is
  // listed (the exact "empty string cached forever" case Minor #2 named).
  test("a first read with only unloadable/disabled entries, followed by a merge adding a loadable skill, lists the new skill", () => {
    const cache: { current: SkillsBlockCache | undefined } = { current: undefined };
    const disabledLoadFile = (path: string): LoadedSkill =>
      path === "/skills/hidden/SKILL.md" ? { ok: true, body: "b", disableModelInvocation: true } : { ok: false, error: "boom" };

    const first = computeSkillsBlockCached(
      [skillCommand("broken", "d", "/skills/broken/SKILL.md"), skillCommand("hidden", "d2", "/skills/hidden/SKILL.md")],
      disabledLoadFile,
      cache,
    );
    assert.equal(first, "", "every entry fails to load or is disabled, so the rendered block is empty");

    const second = computeSkillsBlockCached(
      [
        skillCommand("broken", "d", "/skills/broken/SKILL.md"),
        skillCommand("hidden", "d2", "/skills/hidden/SKILL.md"),
        skillCommand("lead-drain-ready-queue", "d3", "/skills/lead-drain-ready-queue/SKILL.md"),
      ],
      (path) => (path === "/skills/lead-drain-ready-queue/SKILL.md" ? { ok: true, body: "b", disableModelInvocation: false } : disabledLoadFile(path)),
      cache,
    );
    assert.match(second, /<name>lead-drain-ready-queue<\/name>/, "the later-merged loadable skill must be listed, not hidden behind a frozen empty cache");
  });

  test("FAILING_LOAD_FILE alone never throws and yields an empty block", () => {
    const cache: { current: SkillsBlockCache | undefined } = { current: undefined };
    assert.doesNotThrow(() => {
      const block = computeSkillsBlockCached([skillCommand("broken", "d", "/skills/broken/SKILL.md")], FAILING_LOAD_FILE, cache);
      assert.equal(block, "");
    });
  });
});

/**
 * registerLeadBootstrap end-to-end, against a fake `pi` that captures the
 * `before_agent_start` handler and exposes a mutable `pi.getCommands()` —
 * reproducing the EXACT dogfood scenario: `pi.getCommands()` returns only a
 * user skill (e.g. `imagegen`) while `session_start`/registration runs, and
 * the ws skills are merged into the live list only afterward (Pi's real
 * `extendResourcesFromExtensions` ordering) — well before the first
 * `before_agent_start` firing, exactly like a real Pi turn. The fix must
 * list the ws skills in the block at that firing.
 */
describe("registerLeadBootstrap (fake pi, dogfood fix)", () => {
  interface FakePi {
    on: (event: string, handler: (event: { systemPrompt: string }) => unknown) => void;
    getCommands: () => SlashCommandInfo[];
  }

  function fakePi(getCommands: () => SlashCommandInfo[]): { pi: FakePi; fire: (systemPrompt: string) => unknown } {
    let handler: ((event: { systemPrompt: string }) => unknown) | undefined;
    const pi: FakePi = {
      on: (event, h) => {
        if (event === "before_agent_start") handler = h;
      },
      getCommands,
    };
    return { pi, fire: (systemPrompt: string) => handler!({ systemPrompt }) };
  }

  // `registerLeadBootstrap`'s `before_agent_start` handler reads real
  // SKILL.md files off disk (via the real `loadSkillFile`, never a fake
  // reader — it takes no injected IO), so this scenario needs real files, not
  // just fake SlashCommandInfo entries, for the block to actually render
  // their names.
  let dir: string;
  let imagegenPath: string;
  let wsSkillPath: string;

  before(() => {
    dir = mkdtempSync(join(tmpdir(), "ws-pi-lead-bootstrap-test-"));
    imagegenPath = join(dir, "imagegen-SKILL.md");
    wsSkillPath = join(dir, "lead-drain-ready-queue-SKILL.md");
    writeFileSync(imagegenPath, "---\nname: imagegen\ndescription: Generate images\n---\nImagegen body.");
    writeFileSync(wsSkillPath, "---\nname: lead-drain-ready-queue\ndescription: Drain the ready queue\n---\nDrain body.");
  });
  after(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  test("a skill merged in after session_start (only a user skill present then) is listed at before_agent_start", () => {
    // At "session_start" time, pi.getCommands() carries only a user skill —
    // mirrors the reported bug where the model saw only `imagegen`.
    let commands: SlashCommandInfo[] = [skillCommand("imagegen", "Generate images", imagegenPath)];
    const { pi, fire } = fakePi(() => commands);

    const baseRef: { current: WsBlockBase | undefined } = { current: { manualSnapshot: "## Session Key\nlead-1", guideText: "GUIDE-TEXT" } };
    const cacheRef: { current: SkillsBlockCache | undefined } = { current: undefined };
    registerLeadBootstrap(pi as unknown as ExtensionAPI, baseRef, cacheRef);

    // Merged in AFTER session_start/registration returns, BEFORE the first
    // agent turn — the real Pi ordering (`extendResourcesFromExtensions`
    // runs after `bindExtensions` awaits `session_start`, and always before
    // `before_agent_start` can fire for a user turn).
    commands = [...commands, skillCommand("lead-drain-ready-queue", "Drain the ready queue", wsSkillPath)];

    const result = fire("base prompt") as { systemPrompt: string } | undefined;
    assert.ok(result, "the lead must get a ws block override");
    assert.match(result!.systemPrompt, /<name>lead-drain-ready-queue<\/name>/, "the ws skill merged after session_start must be listed");
    assert.match(result!.systemPrompt, /<name>imagegen<\/name>/, "the pre-existing user skill must still be listed");
  });

  test("no override, and no live pi.getCommands()/skills-block work, while wsBlockBaseRef is unset", () => {
    // Mirrors a worker/explore process (computeSessionBootstrap returns
    // wsBlockBase: undefined for those roles, so wsBlockBaseRef is never
    // filled) as well as a still-degraded lead bootstrap — either way the
    // handler must bail out before even reading pi.getCommands().
    let getCommandsCalls = 0;
    const { pi, fire } = fakePi(() => {
      getCommandsCalls += 1;
      return [skillCommand("lead-drain-ready-queue", "d", "/skills/lead-drain-ready-queue/SKILL.md")];
    });
    const baseRef: { current: WsBlockBase | undefined } = { current: undefined };
    const cacheRef: { current: SkillsBlockCache | undefined } = { current: undefined };
    registerLeadBootstrap(pi as unknown as ExtensionAPI, baseRef, cacheRef);

    assert.equal(fire("base prompt"), undefined);
    assert.equal(getCommandsCalls, 0, "must not read pi.getCommands() when there is no base to build a block for");
  });
});
