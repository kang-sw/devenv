/**
 * Unit tests for lead-skills.ts's pure exports (260906 Phase 1, lead/fork
 * skill exposure fix): `resolveSkillEntries`, `loadSkillFile`,
 * `buildSkillsBlock`, `computeWsSkillResult`, `addSkillToolIfLeadOrFork`, and
 * `registerWsSkillTool`'s tool body (fake `pi`, same convention as
 * `test/ask.test.ts`'s `registerAsk (fake pi)` describe block — no live
 * filesystem/subprocess needed since `loadSkillFile` takes an injected
 * `readFile`).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import type { ExtensionAPI, SlashCommandInfo, SourceInfo } from "@earendil-works/pi-coding-agent";
import {
  WS_SKILL_TOOL_NAME,
  addSkillToolIfLeadOrFork,
  buildSkillsBlock,
  computeWsSkillResult,
  loadSkillFile,
  registerWsSkillTool,
  resolveSkillEntries,
  type LoadedSkill,
  type SkillEntry,
} from "../src/lead-skills.ts";

function sourceInfo(path: string): SourceInfo {
  return { path, source: "test", scope: "project", origin: "top-level" };
}

function skillCommand(name: string, description: string | undefined, path: string): SlashCommandInfo {
  return { name: `skill:${name}`, description, source: "skill", sourceInfo: sourceInfo(path) };
}

describe("resolveSkillEntries", () => {
  test("filters to source:'skill' only, stripping the 'skill:' name prefix", () => {
    const commands: SlashCommandInfo[] = [
      skillCommand("lead-proceed", "Route a task", "/skills/lead-proceed/SKILL.md"),
      { name: "ws-discuss", description: "PoC", source: "extension", sourceInfo: sourceInfo("/ext/ws-discuss") },
      { name: "custom-prompt", description: undefined, source: "prompt", sourceInfo: sourceInfo("/prompts/custom") },
    ];
    const entries = resolveSkillEntries(commands);
    assert.deepEqual(entries, [{ name: "lead-proceed", description: "Route a task", path: "/skills/lead-proceed/SKILL.md" }]);
  });

  test("defaults a missing description to an empty string", () => {
    const entries = resolveSkillEntries([skillCommand("no-desc", undefined, "/skills/no-desc/SKILL.md")]);
    assert.equal(entries[0].description, "");
  });

  test("covers every source:'skill' entry, not just ws ones", () => {
    const commands: SlashCommandInfo[] = [
      skillCommand("lead-proceed", "ws skill", "/ws/lead-proceed/SKILL.md"),
      skillCommand("third-party-skill", "Not a ws skill", "/other/third-party-skill/SKILL.md"),
    ];
    const entries = resolveSkillEntries(commands);
    assert.deepEqual(
      entries.map((e) => e.name),
      ["lead-proceed", "third-party-skill"],
    );
  });
});

describe("loadSkillFile", () => {
  test("strips frontmatter and reports disableModelInvocation:false when absent", () => {
    const result = loadSkillFile("/skills/x/SKILL.md", () => "---\nname: x\ndescription: does x\n---\nBody text.");
    assert.deepEqual(result, { ok: true, body: "Body text.", disableModelInvocation: false });
  });

  test("reports disableModelInvocation:true when the frontmatter sets it", () => {
    const result = loadSkillFile("/skills/hidden/SKILL.md", () => "---\ndisable-model-invocation: true\n---\nHidden body.");
    assert.equal(result.ok, true);
    assert.equal((result as { disableModelInvocation: boolean }).disableModelInvocation, true);
  });

  test("never throws on a read failure — returns {ok:false} naming the path", () => {
    const result = loadSkillFile("/skills/missing/SKILL.md", () => {
      throw new Error("ENOENT");
    });
    assert.equal(result.ok, false);
    assert.match((result as { error: string }).error, /\/skills\/missing\/SKILL\.md/);
    assert.match((result as { error: string }).error, /ENOENT/);
  });

  // Review cycle 1, Important #1: `parseFrontmatter` (the `yaml` package's
  // `parse()` under the hood) throws `YAMLParseError` on malformed YAML —
  // verified against the real, unmocked `parseFrontmatter` re-exported from
  // `@earendil-works/pi-coding-agent` (not a fake), so this proves the
  // actual dependency's throw is caught, not an assumption about it.
  const MALFORMED_FRONTMATTER = "---\nfoo: [\nbar\n---\nBody text.";

  test("never throws on malformed YAML frontmatter — reports a parse error naming the path", () => {
    const result = loadSkillFile("/skills/broken-yaml/SKILL.md", () => MALFORMED_FRONTMATTER);
    assert.equal(result.ok, false);
    assert.match((result as { error: string }).error, /\/skills\/broken-yaml\/SKILL\.md/);
    assert.match((result as { error: string }).error, /parse frontmatter/);
  });
});

function fakeLoader(bodies: Record<string, LoadedSkill>): (path: string) => LoadedSkill {
  return (path: string) => bodies[path] ?? { ok: false, error: `no fake entry for "${path}"` };
}

describe("buildSkillsBlock", () => {
  test("returns '' when there are no entries", () => {
    assert.equal(buildSkillsBlock([], fakeLoader({})), "");
  });

  test("lists every readable, non-disabled entry with name/description/location", () => {
    const entries: SkillEntry[] = [{ name: "lead-proceed", description: "Route a task", path: "/skills/lead-proceed/SKILL.md" }];
    const block = buildSkillsBlock(
      entries,
      fakeLoader({ "/skills/lead-proceed/SKILL.md": { ok: true, body: "body", disableModelInvocation: false } }),
    );
    assert.match(block, /<available_skills>/);
    assert.match(block, /<name>lead-proceed<\/name>/);
    assert.match(block, /<description>Route a task<\/description>/);
    assert.match(block, /<location>\/skills\/lead-proceed\/SKILL\.md<\/location>/);
    assert.match(block, /ws-skill <name>/);
    assert.doesNotMatch(block, /resolve relative paths/);
  });

  test("XML-escapes name/description/location", () => {
    const entries: SkillEntry[] = [{ name: "a&b", description: '<tag> "quoted"', path: "/skills/a&b/SKILL.md" }];
    const block = buildSkillsBlock(entries, fakeLoader({ "/skills/a&b/SKILL.md": { ok: true, body: "body", disableModelInvocation: false } }));
    assert.match(block, /<name>a&amp;b<\/name>/);
    assert.match(block, /&lt;tag&gt; &quot;quoted&quot;/);
  });

  test("silently skips an unreadable entry (no error text in the block)", () => {
    const entries: SkillEntry[] = [
      { name: "broken", description: "d", path: "/skills/broken/SKILL.md" },
      { name: "ok", description: "d2", path: "/skills/ok/SKILL.md" },
    ];
    const block = buildSkillsBlock(
      entries,
      fakeLoader({
        "/skills/broken/SKILL.md": { ok: false, error: "boom" },
        "/skills/ok/SKILL.md": { ok: true, body: "b", disableModelInvocation: false },
      }),
    );
    assert.doesNotMatch(block, /broken/);
    assert.match(block, /<name>ok<\/name>/);
  });

  test("excludes a disable-model-invocation:true entry from the block", () => {
    const entries: SkillEntry[] = [{ name: "hidden", description: "d", path: "/skills/hidden/SKILL.md" }];
    const block = buildSkillsBlock(entries, fakeLoader({ "/skills/hidden/SKILL.md": { ok: true, body: "b", disableModelInvocation: true } }));
    assert.equal(block, "");
  });

  // Review cycle 1, Important #1: end-to-end against the REAL `loadSkillFile`
  // (not `fakeLoader`) so this exercises the actual `parseFrontmatter` throw
  // path this ticket's fix wraps, not a fake that never had the throw to
  // begin with. Malformed frontmatter must silently drop the entry from the
  // block — never propagate out of `buildSkillsBlock`.
  test("a malformed-frontmatter entry (real loadSkillFile) is silently skipped, never throws", () => {
    const entries: SkillEntry[] = [
      { name: "broken-yaml", description: "d", path: "/skills/broken-yaml/SKILL.md" },
      { name: "ok", description: "d2", path: "/skills/ok/SKILL.md" },
    ];
    const readFile = (path: string) => (path === "/skills/broken-yaml/SKILL.md" ? "---\nfoo: [\nbar\n---\nBody." : "---\nname: ok\n---\nOk body.");
    let block = "";
    assert.doesNotThrow(() => {
      block = buildSkillsBlock(entries, (path) => loadSkillFile(path, readFile));
    });
    assert.doesNotMatch(block, /broken-yaml/);
    assert.match(block, /<name>ok<\/name>/);
  });
});

describe("computeWsSkillResult", () => {
  const entries: SkillEntry[] = [
    { name: "lead-proceed", description: "d", path: "/skills/lead-proceed/SKILL.md" },
    { name: "hidden-skill", description: "d2", path: "/skills/hidden-skill/SKILL.md" },
  ];
  const loader = fakeLoader({
    "/skills/lead-proceed/SKILL.md": { ok: true, body: "Proceed body.", disableModelInvocation: false },
    "/skills/hidden-skill/SKILL.md": { ok: true, body: "Hidden body.", disableModelInvocation: true },
  });

  test("returns the stripped body for a known skill", () => {
    assert.equal(computeWsSkillResult("lead-proceed", undefined, entries, loader), "Proceed body.");
  });

  test("appends args as a trailing 'User: <args>' line", () => {
    assert.equal(computeWsSkillResult("lead-proceed", "drain the queue", entries, loader), "Proceed body.\n\nUser: drain the queue");
  });

  test("a disable-model-invocation:true skill is still loadable by exact name", () => {
    assert.equal(computeWsSkillResult("hidden-skill", undefined, entries, loader), "Hidden body.");
  });

  test("unknown name lists every available name, sorted", () => {
    const result = computeWsSkillResult("does-not-exist", undefined, entries, loader);
    assert.equal(result, 'Unknown skill "does-not-exist". Available skills: hidden-skill, lead-proceed');
  });

  test("unknown name with no entries at all reports '(none)'", () => {
    assert.equal(computeWsSkillResult("anything", undefined, [], loader), 'Unknown skill "anything". Available skills: (none)');
  });

  test("a load failure for a known entry is reported without throwing", () => {
    const failing: SkillEntry[] = [{ name: "broken", description: "d", path: "/skills/broken/SKILL.md" }];
    const result = computeWsSkillResult("broken", undefined, failing, fakeLoader({ "/skills/broken/SKILL.md": { ok: false, error: 'could not read "/skills/broken/SKILL.md": ENOENT' } }));
    assert.equal(result, 'Error loading skill "broken": could not read "/skills/broken/SKILL.md": ENOENT');
  });

  // Review cycle 1, Important #1: end-to-end against the REAL `loadSkillFile`
  // — a mid-session `ws-skill` call against a live-edited SKILL.md with a
  // YAML typo must return the documented error string, never throw out of
  // the tool's `execute()`.
  test("a malformed-frontmatter entry (real loadSkillFile) reports an error string, never throws", () => {
    const broken: SkillEntry[] = [{ name: "broken-yaml", description: "d", path: "/skills/broken-yaml/SKILL.md" }];
    let result = "";
    assert.doesNotThrow(() => {
      result = computeWsSkillResult("broken-yaml", undefined, broken, (path) => loadSkillFile(path, () => "---\nfoo: [\nbar\n---\nBody."));
    });
    assert.match(result, /^Error loading skill "broken-yaml": could not parse frontmatter in "\/skills\/broken-yaml\/SKILL\.md"/);
  });
});

describe("addSkillToolIfLeadOrFork", () => {
  test("adds ws-skill for the host lead (role undefined)", () => {
    assert.deepEqual(addSkillToolIfLeadOrFork(["bash"], undefined), ["bash", WS_SKILL_TOOL_NAME]);
  });

  test("adds ws-skill for a fork role", () => {
    assert.deepEqual(addSkillToolIfLeadOrFork(["bash"], "fork"), ["bash", WS_SKILL_TOOL_NAME]);
  });

  test("does not add ws-skill for a worker role", () => {
    assert.deepEqual(addSkillToolIfLeadOrFork(["bash"], "worker"), ["bash"]);
  });

  test("does not add ws-skill for an explore role", () => {
    assert.deepEqual(addSkillToolIfLeadOrFork(["bash"], "explore"), ["bash"]);
  });

  test("does not duplicate an already-present ws-skill", () => {
    assert.deepEqual(addSkillToolIfLeadOrFork([WS_SKILL_TOOL_NAME], undefined), [WS_SKILL_TOOL_NAME]);
  });
});

/**
 * `registerWsSkillTool`'s tool body against a fake `pi` (same convention as
 * `test/ask.test.ts`'s "registerAsk (fake pi)" block) — it never spawns
 * anything, so a fake `registerTool` capture is enough.
 */
describe("registerWsSkillTool (fake pi)", () => {
  interface FakeTool {
    name: string;
    execute(id: string, params: unknown): Promise<{ content: Array<{ type: string; text: string }> }>;
  }

  test("registers WS_SKILL_TOOL_NAME and its execute() delegates to computeWsSkillResult", async () => {
    const tools = new Map<string, FakeTool>();
    const pi = { registerTool: (t: FakeTool) => tools.set(t.name, t) } as unknown as ExtensionAPI;
    const entriesRef: { current: SkillEntry[] } = { current: [{ name: "lead-proceed", description: "d", path: "/skills/lead-proceed/SKILL.md" }] };
    registerWsSkillTool(pi, entriesRef);

    const tool = tools.get(WS_SKILL_TOOL_NAME);
    assert.ok(tool, `${WS_SKILL_TOOL_NAME} must be registered`);
    const result = await tool!.execute("call-1", { name: "does-not-exist" });
    assert.match(result.content[0].text, /Unknown skill "does-not-exist"/);
  });

  test("reads entriesRef.current at call time, not at registration time", async () => {
    const tools = new Map<string, FakeTool>();
    const pi = { registerTool: (t: FakeTool) => tools.set(t.name, t) } as unknown as ExtensionAPI;
    const entriesRef: { current: SkillEntry[] } = { current: [] };
    registerWsSkillTool(pi, entriesRef);

    // Populated AFTER registration — mirrors index.ts filling skillEntriesRef
    // inside session_start, after registerWsSkillTool ran at factory scope.
    entriesRef.current = [{ name: "late-skill", description: "d", path: "/skills/late-skill/SKILL.md" }];

    const tool = tools.get(WS_SKILL_TOOL_NAME)!;
    const result = await tool.execute("call-1", { name: "does-not-exist" });
    assert.match(result.content[0].text, /late-skill/);
  });
});
