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
import { mkdirSync, mkdtempSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { AgentSession, createEventBus, discoverAndLoadExtensions, SessionManager, type ExtensionAPI, type SlashCommandInfo, type SourceInfo } from "@earendil-works/pi-coding-agent";
import { findActivePlaybook, humanTextOf } from "../src/lead-compaction.ts";
import {
  WS_SKILL_TOOL_NAME,
  addSkillToolIfLeadOrFork,
  buildSkillsBlock,
  computeTypedSkillInput,
  computeWsSkillResult,
  expandWsSkillPreload,
  formatSkillBlock,
  isInsideDir,
  loadSkillFile,
  PRELOAD_EXCLUDED,
  registerTypedSkillExpansion,
  registerWsSkillTool,
  resolveSkillEntries,
  type LoadedSkill,
  type SkillEntry,
  type WsSkillPreload,
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
      { name: "ws-model-catalog-list", description: "Catalog", source: "extension", sourceInfo: sourceInfo("/ext/ws-model-catalog-list") },
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
    assert.equal(computeWsSkillResult("lead-proceed", entries, loader), "Proceed body.");
  });

  test("a disable-model-invocation:true skill is still loadable by exact name", () => {
    assert.equal(computeWsSkillResult("hidden-skill", entries, loader), "Hidden body.");
  });

  test("unknown name lists every available name, sorted", () => {
    const result = computeWsSkillResult("does-not-exist", entries, loader);
    assert.equal(result, 'Unknown skill "does-not-exist". Available skills: hidden-skill, lead-proceed');
  });

  test("unknown name with no entries at all reports '(none)'", () => {
    assert.equal(computeWsSkillResult("anything", [], loader), 'Unknown skill "anything". Available skills: (none)');
  });

  test("a load failure for a known entry is reported without throwing", () => {
    const failing: SkillEntry[] = [{ name: "broken", description: "d", path: "/skills/broken/SKILL.md" }];
    const result = computeWsSkillResult("broken", failing, fakeLoader({ "/skills/broken/SKILL.md": { ok: false, error: 'could not read "/skills/broken/SKILL.md": ENOENT' } }));
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
      result = computeWsSkillResult("broken-yaml", broken, (path) => loadSkillFile(path, () => "---\nfoo: [\nbar\n---\nBody."));
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
 * anything, so a fake `registerTool` capture plus a fake `getCommands` are
 * enough.
 */
describe("registerWsSkillTool (fake pi)", () => {
  interface FakeTool {
    name: string;
    execute(id: string, params: unknown, signal?: unknown, update?: unknown, ctx?: unknown): Promise<{ content: Array<{ type: string; text: string }> }>;
  }

  function fakePi(tools: Map<string, FakeTool>, getCommands: () => SlashCommandInfo[]): ExtensionAPI {
    return { registerTool: (t: FakeTool) => tools.set(t.name, t), getCommands } as unknown as ExtensionAPI;
  }

  test("registers WS_SKILL_TOOL_NAME and its execute() delegates to computeWsSkillResult", async () => {
    const tools = new Map<string, FakeTool>();
    const pi = fakePi(tools, () => [skillCommand("lead-proceed", "d", "/skills/lead-proceed/SKILL.md")]);
    registerWsSkillTool(pi);

    const tool = tools.get(WS_SKILL_TOOL_NAME);
    assert.ok(tool, `${WS_SKILL_TOOL_NAME} must be registered`);
    const result = await tool!.execute("call-1", { name: "does-not-exist" });
    assert.match(result.content[0].text, /Unknown skill "does-not-exist"/);
  });

  test("schema has no args parameter, and a stray args value still returns the plain body", async () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-skill-no-args-"));
    const path = join(directory, "SKILL.md");
    writeFileSync(path, "---\nname: plain\n---\nPlain body.");
    const tools = new Map<string, FakeTool>();
    const pi = fakePi(tools, () => [skillCommand("plain", "fixture", path)]);
    registerWsSkillTool(pi);
    const tool = tools.get(WS_SKILL_TOOL_NAME)!;
    const schema = (tool as unknown as { parameters: { properties: Record<string, unknown>; required: string[] } }).parameters;
    assert.deepEqual(Object.keys(schema.properties), ["name"]);
    assert.deepEqual(schema.required, ["name"]);
    const result = await tool.execute("call-1", { name: "plain", args: "User approves autonomously" }, undefined, undefined, { sessionManager: { buildContextEntries: () => [] } });
    assert.equal(result.content[0].text, "Plain body.");
  });

  // Dogfood bug this fixes: Pi runs session_start BEFORE merging an
  // extension's own resources_discover skills into pi.getCommands() (see
  // this file's header). A ws-skill call must therefore resolve
  // pi.getCommands() LIVE at call time, not a snapshot captured at
  // registration or session_start time — otherwise a skill that only
  // appears in the list after registration answers "Unknown skill" forever,
  // exactly like the reported `lead-drain-ready-queue` failure.
  test("resolves pi.getCommands() live at call time — a skill merged in after registration is loadable and listed", async () => {
    const tools = new Map<string, FakeTool>();
    let commands: SlashCommandInfo[] = [skillCommand("imagegen", "d", "/skills/imagegen/SKILL.md")];
    const pi = fakePi(tools, () => commands);
    registerWsSkillTool(pi);

    // Merged AFTER registration — mirrors Pi's real ordering: session_start
    // (which registers ws-skill) runs and returns BEFORE
    // extendResourcesFromExtensions merges the ws skill path into
    // pi.getCommands().
    commands = [...commands, skillCommand("lead-drain-ready-queue", "d", "/skills/lead-drain-ready-queue/SKILL.md")];

    const tool = tools.get(WS_SKILL_TOOL_NAME)!;
    const loadResult = await tool.execute("call-1", { name: "lead-drain-ready-queue" });
    assert.doesNotMatch(loadResult.content[0].text, /Unknown skill/, "the live-merged skill must resolve, not report Unknown");

    const unknownResult = await tool.execute("call-2", { name: "does-not-exist" });
    assert.match(unknownResult.content[0].text, /lead-drain-ready-queue/, "the unknown-name listing must include the live-merged skill too");
  });

  test("production registration never dedupes unknown-skill errors", async () => {
    const tools = new Map<string, FakeTool>();
    const pi = fakePi(tools, () => []);
    registerWsSkillTool(pi);
    const context = { sessionManager: { buildContextEntries: () => [{ type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "one", name: "ws-skill", arguments: { name: "missing" } }] } }, { type: "message", message: { role: "toolResult", toolCallId: "one", content: [{ type: "text", text: 'Unknown skill "missing". Available skills: (none)' }], isError: false } }] } };
    const result = await tools.get(WS_SKILL_TOOL_NAME)!.execute("two", { name: "missing" }, undefined, undefined, context);
    assert.match(result.content[0].text, /^Unknown skill "missing"/);
    assert.doesNotMatch(result.content[0].text, /unchanged result/);
  });

  test("production registration returns full, pointer, then full for a successful ws-skill read", async () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-dedupe-skill-"));
    const path = join(directory, "SKILL.md");
    writeFileSync(path, "---\nname: repeat\n---\n# Repeat\n## Detail\n");
    const tools = new Map<string, FakeTool>();
    const pi = fakePi(tools, () => [skillCommand("repeat", "fixture", path)]);
    registerWsSkillTool(pi);
    const tool = tools.get(WS_SKILL_TOOL_NAME)!;
    const first = await tool.execute("one", { name: "repeat" }, undefined, undefined, { sessionManager: { buildContextEntries: () => [] } });
    assert.match(first.content[0].text, /^# Repeat/);
    const secondContext = { sessionManager: { buildContextEntries: () => [
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "one", name: "ws-skill", arguments: { name: "repeat" } }] } },
      { type: "message", message: { role: "toolResult", toolCallId: "one", content: first.content, isError: false } },
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "two", name: "ws-skill", arguments: { name: "repeat" } }] } },
    ] } };
    const second = await tool.execute("two", { name: "repeat" }, undefined, undefined, secondContext);
    const [prose] = second.content[0].text.split("\n");
    assert.equal(prose, "The result is unchanged. Continue using the full result already present 1 tool call ago. Do not read it again (headings: # Repeat; ## Detail).");
    assert.doesNotMatch(prose!, /one/, "the toolCallId is carried only by the provenance envelope");
    const thirdContext = { sessionManager: { buildContextEntries: () => [...secondContext.sessionManager.buildContextEntries(), { type: "message", message: { role: "toolResult", toolCallId: "two", content: second.content, isError: false } }, { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "three", name: "ws-skill", arguments: { name: "repeat" } }] } }] } };
    const third = await tool.execute("three", { name: "repeat" }, undefined, undefined, thirdContext);
    assert.equal(third.content[0].text, first.content[0].text);
  });

  test("expands the preload block of an own skill with the process's key, and the expanded text dedupes", async () => {
    const own = mkdtempSync(join(tmpdir(), "ws-pi-preload-skills-"));
    const path = join(own, "SKILL.md");
    writeFileSync(path, '---\nname: lead-discuss\n---\n# Discuss\n\n<ws-preload playbook="lead-discuss">\nCall `ws/playbook.read(name: "lead-discuss")`.\n</ws-preload>\n');
    const tools = new Map<string, FakeTool>();
    const pi = fakePi(tools, () => [skillCommand("lead-discuss", "d", path)]);
    const reads: [string, string][] = [];
    registerWsSkillTool(pi, undefined, () => ({ ownSkillsDir: own, sessionKey: "fork-key", readPlaybook: async (name, key) => (reads.push([name, key]), `# ${name}\n## Steps\nwork\n`) }));
    const tool = tools.get(WS_SKILL_TOOL_NAME)!;
    const first = await tool.execute("one", { name: "lead-discuss" }, undefined, undefined, { sessionManager: { buildContextEntries: () => [] } });
    assert.equal(first.content[0].text, "# Discuss\n\n# lead-discuss\n## Steps\nwork");
    assert.deepEqual(reads, [["lead-discuss", "fork-key"]]);
    const context = { sessionManager: { buildContextEntries: () => [
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "one", name: "ws-skill", arguments: { name: "lead-discuss" } }] } },
      { type: "message", message: { role: "toolResult", toolCallId: "one", content: first.content, isError: false } },
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "two", name: "ws-skill", arguments: { name: "lead-discuss" } }] } },
    ] } };
    const second = await tool.execute("two", { name: "lead-discuss" }, undefined, undefined, context);
    // The inlined playbook dedupes as its unit: the shim text stays, the body becomes a pointer.
    assert.match(second.content[0].text, /^# Discuss\n\nThe full `lead-discuss` procedure is already present 1 tool call ago; follow it and do not read it again \(headings: # lead-discuss; ## Steps\)\.\n<!-- ws-pi-read-dedupe-v1 \{"family":"playbook",[^\n]*"originalToolCallId":"one"\} -->$/);
    const after = { sessionManager: { buildContextEntries: () => [
      ...context.sessionManager.buildContextEntries(),
      { type: "message", message: { role: "toolResult", toolCallId: "two", content: second.content, isError: false } },
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "three", name: "ws-skill", arguments: { name: "lead-discuss" } }] } },
    ] } };
    assert.equal((await tool.execute("three", { name: "lead-discuss" }, undefined, undefined, after)).content[0].text, first.content[0].text, "the copy after a pointer is full");
    const viaRead = { sessionManager: { buildContextEntries: () => [
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "r", name: "ws__playbook_read", arguments: { name: "lead-discuss" } }] } },
      { type: "message", message: { role: "toolResult", toolCallId: "r", content: [{ type: "text", text: "# lead-discuss\n## Steps\nwork\n" }], isError: false } },
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "four", name: "ws-skill", arguments: { name: "lead-discuss" } }] } },
    ] } };
    assert.match((await tool.execute("four", { name: "lead-discuss" }, undefined, undefined, viaRead)).content[0].text, /^# Discuss\n\nThe full `lead-discuss` procedure is already present 1 tool call ago;[^\n]*\n<!-- [^\n]*"originalToolCallId":"r"\} -->$/, "an earlier ws__playbook_read copy is the same unit");
  });

  test("an expansion failure returns the plain body, which still dedupes; an unknown skill still bypasses dedupe", async () => {
    const own = mkdtempSync(join(tmpdir(), "ws-pi-preload-skills-"));
    const path = join(own, "SKILL.md");
    const plain = '# Run\n\n<ws-preload playbook="lead-run">\nCall `ws/playbook.read(name: "lead-run")`.\n</ws-preload>';
    writeFileSync(path, `---\nname: lead-run\n---\n${plain}\n`);
    const tools = new Map<string, FakeTool>();
    const pi = fakePi(tools, () => [skillCommand("lead-run", "d", path)]);
    registerWsSkillTool(pi, undefined, () => ({ ownSkillsDir: own, sessionKey: "lead-key", readPlaybook: async () => Promise.reject(new Error("bridge down")) }));
    const tool = tools.get(WS_SKILL_TOOL_NAME)!;
    const first = await tool.execute("one", { name: "lead-run" }, undefined, undefined, { sessionManager: { buildContextEntries: () => [] } });
    assert.equal(first.content[0].text, plain);
    const context = { sessionManager: { buildContextEntries: () => [
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "one", name: "ws-skill", arguments: { name: "lead-run" } }] } },
      { type: "message", message: { role: "toolResult", toolCallId: "one", content: first.content, isError: false } },
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "two", name: "ws-skill", arguments: { name: "lead-run" } }] } },
    ] } };
    assert.match((await tool.execute("two", { name: "lead-run" }, undefined, undefined, context)).content[0].text, /^The result is unchanged\./);
    assert.match((await tool.execute("three", { name: "missing" }, undefined, undefined, context)).content[0].text, /^Unknown skill "missing"/);
  });
});

describe("expandWsSkillPreload", () => {
  const own = mkdtempSync(join(tmpdir(), "ws-pi-preload-own-"));
  const ownPath = join(own, "lead-discuss", "SKILL.md");
  const body = '# Discuss\n\n<ws-preload playbook="lead-discuss">\nfallback prose\n</ws-preload>\n\nafter';

  function preload(over: Partial<WsSkillPreload> = {}): WsSkillPreload & { reads: [string, string][] } {
    const reads: [string, string][] = [];
    return { ownSkillsDir: own, sessionKey: "lead-key", readPlaybook: async (name, key) => (reads.push([name, key]), `# ${name}\n\nBody.\n`), ...over, reads };
  }

  test("replaces the block with the trimmed playbook body, keeping the text around it", async () => {
    const p = preload();
    assert.equal(await expandWsSkillPreload("lead-discuss", ownPath, body, p), "# Discuss\n\n# lead-discuss\n\nBody.\n\nafter");
    assert.deepEqual(p.reads, [["lead-discuss", "lead-key"]]);
  });

  test("passes through without reading: another tree, an excluded skill, a mismatched block, a tool block, no key, no bridge", async () => {
    const cases: [string, string, string, WsSkillPreload & { reads: [string, string][] } | undefined][] = [
      ["lead-discuss", "/other/skills/lead-discuss/SKILL.md", body, preload()],
      ["lead-use-mailbox", join(own, "lead-use-mailbox", "SKILL.md"), body.replace("lead-discuss", "lead-use-mailbox"), preload()],
      ["lead-run", join(own, "lead-run", "SKILL.md"), body, preload()],
      ["lead-discuss", ownPath, '<ws-preload tool="workflow_state">\nx\n</ws-preload>', preload()],
      ["lead-discuss", ownPath, body, preload({ sessionKey: undefined })],
      ["lead-discuss", ownPath, body, preload({ sessionKey: "  " })],
      ["lead-discuss", ownPath, body, undefined],
    ];
    for (const [name, path, text, p] of cases) {
      assert.equal(await expandWsSkillPreload(name, path, text, p), text, `${name} at ${path}`);
      assert.deepEqual(p?.reads ?? [], []);
    }
    assert.ok(PRELOAD_EXCLUDED.has("lead-use-mailbox"));
  });

  test("a failed, empty, or key-carrying read passes through, never inlining the key or state", async () => {
    assert.equal(await expandWsSkillPreload("lead-discuss", ownPath, body, preload({ readPlaybook: async () => Promise.reject(new Error("down")) })), body);
    assert.equal(await expandWsSkillPreload("lead-discuss", ownPath, body, preload({ readPlaybook: async () => " \n" })), body);
    assert.equal(await expandWsSkillPreload("lead-discuss", ownPath, body, preload({ readPlaybook: async () => "wait --session-key lead-key" })), body);
    const expanded = await expandWsSkillPreload("lead-discuss", ownPath, body, preload());
    assert.doesNotMatch(expanded, /lead-key|Session State|Todos/);
  });

  test("isInsideDir accepts only paths strictly inside the directory", () => {
    assert.equal(isInsideDir(ownPath, own), true);
    assert.equal(isInsideDir(own, own), false);
    assert.equal(isInsideDir(join(own, "..", "elsewhere", "SKILL.md"), own), false);
    // An existing file reached through a link to the tree counts as inside.
    mkdirSync(join(own, "linked-skill"), { recursive: true });
    writeFileSync(join(own, "linked-skill", "SKILL.md"), "# Linked\n");
    const link = join(mkdtempSync(join(tmpdir(), "ws-pi-preload-link-")), "skills");
    symlinkSync(own, link);
    assert.equal(isInsideDir(join(link, "linked-skill", "SKILL.md"), own), true);
  });
});

describe("typed /skill: expansion", () => {
  const own = mkdtempSync(join(tmpdir(), "ws-pi-typed-own-"));
  const shim = '# Discuss\n\n<ws-preload playbook="lead-discuss">\nfallback prose\n</ws-preload>';
  const path = join(own, "lead-discuss", "SKILL.md");
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, `---\nname: lead-discuss\ndescription: d\n---\n${shim}\n`);
  const playbook = "# lead-discuss\n\n## Steps\nwork";
  const deps = (over: Partial<WsSkillPreload> = {}, commands = [skillCommand("lead-discuss", "d", path)]) => ({
    commands: () => commands,
    preload: () => ({ ownSkillsDir: own, sessionKey: "lead-key", readPlaybook: async () => `${playbook}\n`, ...over }),
  });
  const typed = (text: string, source: "interactive" | "rpc" | "extension" = "interactive") => ({ text, source, images: undefined });

  test("an own skill becomes Pi's own <skill> block around the expanded body, args kept", async () => {
    const result = await computeTypedSkillInput(typed("/skill:lead-discuss  pick the  tier "), [], deps());
    assert.deepEqual(result, { action: "transform", text: formatSkillBlock("lead-discuss", path, `# Discuss\n\n${playbook}`, "pick the  tier"), images: undefined });
    assert.equal((await computeTypedSkillInput(typed("/skill:lead-discuss", "rpc"), [], deps())).action, "transform");
  });

  test("passes through: another source, not a skill command, unknown/foreign/excluded skill, no bridge, failed read", async () => {
    const foreign = join(mkdtempSync(join(tmpdir(), "ws-pi-typed-foreign-")), "SKILL.md");
    writeFileSync(foreign, `---\nname: lead-discuss\n---\n${shim}\n`);
    const mailbox = join(own, "lead-use-mailbox", "SKILL.md");
    mkdirSync(dirname(mailbox), { recursive: true });
    writeFileSync(mailbox, `---\nname: lead-use-mailbox\n---\n${shim.replace("lead-discuss", "lead-use-mailbox")}\n`);
    const cases: [ReturnType<typeof typed>, ReturnType<typeof deps> | { commands: () => SlashCommandInfo[]; preload: () => undefined }][] = [
      [typed("/skill:lead-discuss", "extension"), deps()],
      [typed("please /skill:lead-discuss"), deps()],
      [typed("/skill:missing"), deps()],
      [typed("/skill:lead-discuss"), deps({}, [skillCommand("lead-discuss", "d", foreign)])],
      [typed("/skill:lead-use-mailbox"), deps({}, [skillCommand("lead-use-mailbox", "d", mailbox)])],
      [typed("/skill:lead-discuss"), { commands: () => [skillCommand("lead-discuss", "d", path)], preload: () => undefined }],
      [typed("/skill:lead-discuss"), deps({ sessionKey: undefined })],
      [typed("/skill:lead-discuss"), deps({ readPlaybook: async () => Promise.reject(new Error("down")) })],
      [typed("/skill:lead-discuss"), { ...deps(), commands: () => { throw new Error("boom"); } }],
    ];
    for (const [event, d] of cases) assert.deepEqual(await computeTypedSkillInput(event, [], d), { action: "continue" }, event.text);
  });

  test("a visible copy of the playbook turns the inlined body into a pointer keyed by the copy", async () => {
    const entries = [
      { type: "message", message: { role: "assistant", content: [{ type: "toolCall", id: "r", name: "ws__playbook_read", arguments: { name: "lead-discuss" } }] } },
      { type: "message", message: { role: "toolResult", toolCallId: "r", content: [{ type: "text", text: playbook }], isError: false } },
    ];
    const result = await computeTypedSkillInput(typed("/skill:lead-discuss"), entries, deps());
    assert.equal(result.action, "transform");
    assert.match((result as { text: string }).text, /\n\n# Discuss\n\nThe full `lead-discuss` procedure is already present earlier in this conversation; [^\n]*\n<!-- [^\n]*"originalToolCallId":"r"\} -->\n<\/skill>$/);
  });

  test("the block format matches the installed Pi's _expandSkillCommand", () => {
    const fakeThis = { resourceLoader: { getSkills: () => ({ skills: [{ name: "lead-discuss", filePath: path, baseDir: dirname(path) }] }) }, _extensionRunner: { emitError() {} } };
    const expand = (text: string) => (AgentSession.prototype as unknown as { _expandSkillCommand(this: unknown, t: string): string })._expandSkillCommand.call(fakeThis, text);
    assert.equal(formatSkillBlock("lead-discuss", path, shim, "some args"), expand("/skill:lead-discuss some args"));
    assert.equal(formatSkillBlock("lead-discuss", path, shim, ""), expand("/skill:lead-discuss"));
    // Pi's expansion passes the transformed text through unchanged.
    const block = formatSkillBlock("lead-discuss", path, `# Discuss\n\n${playbook}`, "x");
    assert.equal(expand(block), block);
  });

  test("the transformed message still reads as the typed command for compaction", async () => {
    const result = await computeTypedSkillInput(typed("/skill:lead-discuss pick a tier"), [], deps()) as { text: string };
    assert.equal(humanTextOf(result.text), "/skill:lead-discuss pick a tier");
    const manager = SessionManager.inMemory(mkdtempSync(join(tmpdir(), "ws-pi-typed-compact-")));
    manager.appendMessage({ role: "user", content: result.text, timestamp: 1 });
    assert.deepEqual(findActivePlaybook(manager.getBranch()), { name: "lead-discuss", via: "/skill" });
  });

  test("registers a named input handler reading the live context", async () => {
    const handlers: ((event: unknown, ctx: unknown) => Promise<unknown>)[] = [];
    registerTypedSkillExpansion({ on: (event: string, handler: never) => { assert.equal(event, "input"); handlers.push(handler); }, getCommands: () => [skillCommand("lead-discuss", "d", path)] } as never, deps().preload);
    assert.equal(handlers.length, 1);
    assert.equal((handlers[0] as { name: string }).name, "typedSkillExpansion");
    assert.equal(((await handlers[0]!(typed("/skill:lead-discuss"), { sessionManager: { buildContextEntries: () => [] } })) as { action: string }).action, "transform");
    assert.equal(((await handlers[0]!(typed("/skill:lead-discuss"), undefined)) as { action: string }).action, "transform");
  });

  test("the extension registers it after the fork first-input framing handler, and framed fork text passes through", async () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-typed-loader-"));
    const extensionPath = fileURLToPath(new URL("../src/index.ts", import.meta.url));
    const loaded = await discoverAndLoadExtensions([extensionPath], root, join(root, "agent"), createEventBus());
    assert.deepEqual(loaded.errors, []);
    const input = loaded.extensions.find((candidate) => candidate.resolvedPath === extensionPath)!.handlers.get("input") ?? [];
    const fork = input.findIndex((handler) => String(handler).includes("frameForkInput"));
    const typedIndex = input.findIndex((handler) => handler.name === "typedSkillExpansion");
    assert.ok(fork !== -1 && typedIndex !== -1 && fork < typedIndex, `fork ${fork}, typed ${typedIndex}`);
    const framed = `Current fork-owned ws session_key: k. Use this key.\n\n/skill:lead-discuss`;
    assert.deepEqual(await computeTypedSkillInput(typed(framed), [], deps()), { action: "continue" });
  });
});
