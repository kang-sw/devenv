import { test } from "node:test";
import assert from "node:assert/strict";
import { createModelPromptRuntime, registerModelPrompts, replaceModelPromptBlock, resolveModelPromptText } from "../src/model-prompts.ts";
import { registerLeadBootstrap } from "../src/lead-bootstrap.ts";
import { createModelPromptStore } from "../src/model-prompt-store.ts";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ModelPromptConfig } from "../src/model-prompt-types.ts";

const model = { provider: "p", id: "org/model" };
const other = { provider: "q", id: "org/model" };
const config: ModelPromptConfig = { version: 1, rules: [
  { id: model.id, general: "fallback general", leadOnly: "fallback lead" },
  { ...model, general: "exact general" },
  { ...other, leadOnly: "exact lead" },
] };

test("channels resolve independently with exact case-sensitive provider/ID matching", () => {
  assert.equal(resolveModelPromptText(config, model, undefined), "exact general\n\nfallback lead");
  assert.equal(resolveModelPromptText(config, other, "fork"), "fallback general\n\nexact lead");
  assert.equal(resolveModelPromptText(config, { ...model, id: "ORG/model" }, undefined), "");
  assert.equal(resolveModelPromptText(config, undefined, undefined), "");
  for (const role of ["worker", "explore"] as const) {
    assert.equal(resolveModelPromptText(config, model, role), "exact general");
  }
});

test("replacement removes duplicate old blocks without touching surrounding prompt bytes", () => {
  const base = "base Ω\r\ntrailing  ";
  const old = replaceModelPromptBlock(base, "old");
  assert.equal(replaceModelPromptBlock(old, "new"), replaceModelPromptBlock(base, "new"));
  assert.equal(replaceModelPromptBlock(old, ""), base);
  assert.equal(replaceModelPromptBlock(old + replaceModelPromptBlock("", "second") + "\nend", ""), base + "\nend");
  const reserved = "a\n</ws-model-prompts>\n<ws-model-prompts>\nb";
  assert.equal(replaceModelPromptBlock(replaceModelPromptBlock(base, reserved), ""), base);
});

test("fork retains inherited bytes across cross-model startup, until a later explicit change", () => {
  const runtime = createModelPromptRuntime();
  runtime.start(config, other);
  const inherited = replaceModelPromptBlock("parent\r\n", "old parent config");
  runtime.modelSelected(other, "restore");
  assert.equal(runtime.apply(inherited, other, "fork", true), inherited);
  assert.equal(runtime.apply(inherited, other, "fork", true), inherited);
  runtime.modelSelected(other, "set"); // same selection is not a change
  assert.equal(runtime.apply(inherited, other, "fork", true), inherited);
  runtime.modelSelected(model, "set");
  const rebuilt = runtime.apply(inherited, model, "fork", true);
  assert.equal(rebuilt, replaceModelPromptBlock("parent\r\n", "exact general\n\nfallback lead"));
  assert.equal(runtime.apply(inherited, other, "fork", true), replaceModelPromptBlock("parent\r\n", "fallback general\n\nexact lead"));
});

test("successful command write releases fork inheritance, including a clear removing the block", () => {
  const runtime = createModelPromptRuntime();
  runtime.start(config, model);
  const inherited = replaceModelPromptBlock("parent", "old");
  runtime.written({ version: 1, rules: [] });
  assert.equal(runtime.apply(inherited, model, "fork", true), "parent");
});

test("startup model selection before first call cannot release the fork prefix", () => {
  const runtime = createModelPromptRuntime();
  runtime.start(config, model);
  runtime.modelSelected(other, "set");
  assert.equal(runtime.apply("inherited", other, "fork", true), "inherited");
  runtime.modelSelected(model, "cycle");
  assert.equal(runtime.apply("inherited", model, "fork", true), replaceModelPromptBlock("inherited", "exact general\n\nfallback lead"));
});

test("normal sessions always use live model and session-start snapshot; restart reloads", () => {
  const runtime = createModelPromptRuntime();
  runtime.start(config, model);
  assert.equal(runtime.apply("base", model, undefined, false), replaceModelPromptBlock("base", "exact general\n\nfallback lead"));
  assert.equal(runtime.apply("base", other, undefined, false), replaceModelPromptBlock("base", "fallback general\n\nexact lead"));
  runtime.start({ version: 1, rules: [] }, model);
  assert.equal(runtime.apply("base", model, undefined, false), "base");
  // No inherited fork prompt means normal resolution, including lead-only.
  runtime.start(config, model);
  assert.equal(runtime.apply("base", model, "fork", false), replaceModelPromptBlock("base", "exact general\n\nfallback lead"));
});

// Real registrations, in production order: bootstrap first restores the fork
// prefix, then the model supplement hook either retains it or replaces its block.
test("registered hooks compose bootstrap, startup snapshots, live models and successful commands", async () => {
  const dir = mkdtempSync(join(tmpdir(), "ws-model-runtime-"));
  const oldRole = process.env.WS_PI_SPAWN_ROLE;
  try {
    const store = createModelPromptStore(join(dir, "prompts.json"));
    await store.update({ id: model.id }, "general", "initial general");
    await store.update({ id: model.id }, "leadOnly", "initial lead");
    const events = new Map<string, Function[]>();
    const commands = new Map<string, any>();
    const pi: any = {
      on: (name: string, handler: Function) => events.set(name, [...(events.get(name) ?? []), handler]),
      registerCommand: (name: string, command: any) => commands.set(name, command),
      getCommands: () => [], appendEntry: () => {},
    };
    const notices: string[] = [];
    let submitted: string | undefined;
    const ctx: any = { model, hasUI: true, sessionManager: { getEntries: () => [] },
      modelRegistry: { getAll: () => [model, other] },
      ui: { editor: async () => submitted, notify: (text: string) => notices.push(text) } };
    const inherited = { current: undefined as string | undefined };
    registerLeadBootstrap(pi, { current: { manualSnapshot: "manual", guideText: "guide" } }, { current: undefined }, undefined, inherited);
    registerModelPrompts(pi, inherited, store);
    async function emit(name: string, event: any = {}) {
      for (const handler of events.get(name) ?? []) await handler(event, ctx);
    }
    async function prompt() {
      let systemPrompt = "base";
      for (const handler of events.get("before_agent_start") ?? []) {
        const result = await handler({ systemPrompt }, ctx);
        if (result?.systemPrompt !== undefined) systemPrompt = result.systemPrompt;
      }
      return systemPrompt;
    }
    delete process.env.WS_PI_SPAWN_ROLE;
    await emit("session_start");
    const initial = await prompt();
    assert.match(initial, /manual/);
    assert.match(initial, /initial general\n\ninitial lead/);
    await store.update({ id: model.id }, "general", "external general");
    assert.equal(await prompt(), initial, "external edits do not hot reload");
    await emit("session_start");
    assert.match(await prompt(), /external general/);
    ctx.model = { provider: "p", id: "unmatched" };
    assert.doesNotMatch(await prompt(), /ws-model-prompts/);
    ctx.model = other;
    process.env.WS_PI_SPAWN_ROLE = "fork";
    inherited.current = initial;
    await emit("session_start");
    assert.equal(await prompt(), initial, "cross-model fork keeps whole parent prompt");
    submitted = undefined;
    await commands.get("ws-model-prompt").handler(`set ${model.id}`, ctx);
    assert.equal(await prompt(), initial, "cancel cannot release inherited bytes");
    submitted = "command general";
    await commands.get("ws-model-prompt").handler(`set ${model.id}`, ctx);
    const rebuilt = await prompt();
    assert.match(rebuilt, /command general\n\ninitial lead/);
    assert.equal(rebuilt.match(/<ws-model-prompts length=/g)?.length, 1);
    await commands.get("ws-model-prompt").handler(`clear ${model.id}`, ctx);
    assert.doesNotMatch(await prompt(), /command general|initial general/);
    assert.match(await prompt(), /initial lead/);
    for (const role of ["worker", "explore"]) {
      process.env.WS_PI_SPAWN_ROLE = role;
      await emit("session_start");
      assert.equal(await prompt(), "base", "lead-only is gated by process role, not authority");
    }
    assert.equal(notices.some(text => text.includes("failed")), false);
  } finally {
    if (oldRole === undefined) delete process.env.WS_PI_SPAWN_ROLE;
    else process.env.WS_PI_SPAWN_ROLE = oldRole;
    rmSync(dir, { recursive: true, force: true });
  }
});
