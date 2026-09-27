import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import type { ExtensionAPI, ExtensionCommandContext } from "@earendil-works/pi-coding-agent";
import { parseModelPromptCommand, registerModelPromptCommand } from "../src/model-prompt-command.ts";
import { createModelPromptStore } from "../src/model-prompt-store.ts";
import type { ModelPromptConfig, ModelPromptStore } from "../src/model-prompt-types.ts";

describe("model prompt command grammar", () => {
  test("preserves slash-containing case-sensitive IDs and either flag order", () => {
    for (const verb of ["set", "show", "clear"]) {
      for (const flags of ["--lead-only --provider Acme", "--provider Acme --lead-only"]) {
        assert.deepEqual(parseModelPromptCommand(`${verb} ${flags} Org/Model/V2`), {
          verb, selector: { id: "Org/Model/V2", provider: "Acme" }, channel: "leadOnly",
        });
      }
      assert.deepEqual(parseModelPromptCommand(`${verb} Org/Model/V2`), {
        verb, selector: { id: "Org/Model/V2" }, channel: "general",
      });
    }
    assert.deepEqual(parseModelPromptCommand("list"), { verb: "list" });
  });

  test("rejects missing, duplicate, unknown, and extra arguments without interpreting ID slashes as providers", () => {
    for (const input of ["", "set", "set --provider", "set --provider --lead-only id", "set --provider p", "set --lead-only", "set --bogus x", "set --provider p --provider q id", "set --lead-only --lead-only id", "set id extra", "set id --lead-only", "list id", "LIST", "remove id"]) {
      assert.throws(() => parseModelPromptCommand(input), /Usage:/, input);
    }
  });
});

type Notice = { text: string; type: string };
function fakeCommand(store: ModelPromptStore, onWrite: (config: ModelPromptConfig) => void = () => {}) {
  let handler: (args: string, ctx: ExtensionCommandContext) => Promise<void> = async () => { throw new Error("not registered"); };
  const pi = { registerCommand: (name: string, spec: { handler: typeof handler }) => {
    assert.equal(name, "ws-model-prompt"); handler = spec.handler;
  } } as unknown as ExtensionAPI;
  registerModelPromptCommand(pi, store, onWrite);
  const notices: Notice[] = [];
  const editorSeeds: string[] = [];
  let editorValue: string | undefined = "new";
  let editorError: Error | undefined;
  const models: Array<{ id: string; provider: string }> = [{ id: "Org/Model/V2", provider: "Acme" }];
  const ctx = {
    mode: "tui", hasUI: true,
    modelRegistry: { getAll: () => models },
    ui: {
      editor: async (_title: string, seed: string) => { editorSeeds.push(seed); if (editorError) throw editorError; return editorValue; },
      notify: (text: string, type: string) => notices.push({ text, type }),
    },
  } as unknown as ExtensionCommandContext;
  return {
    run: (args: string) => handler(args, ctx), ctx, notices, editorSeeds, models,
    setEditor: (value: string | undefined) => { editorValue = value; },
    setEditorError: (value: Error | undefined) => { editorError = value; },
  };
}

function inTempStore(fn: (store: ModelPromptStore) => Promise<void>): () => Promise<void> {
  return async () => {
    const root = mkdtempSync(join(tmpdir(), "ws-model-command-"));
    try { await fn(createModelPromptStore(join(root, "prompts.json"))); }
    finally { rmSync(root, { recursive: true, force: true }); }
  };
}

describe("/ws-model-prompt", () => {
  test("editor seeds the exact selector/channel freshly; cancel and whitespace preserve storage and skip callback", inTempStore(async store => {
    const updates: ModelPromptConfig[] = [];
    const command = fakeCommand(store, config => updates.push(config));
    await store.update({ id: "Org/Model/V2" }, "general", "fallback");
    await store.update({ id: "Org/Model/V2", provider: "Acme" }, "leadOnly", "lead\ntext");
    command.setEditor(undefined);
    await command.run("set --lead-only --provider Acme Org/Model/V2");
    command.setEditor("  \n \t ");
    await command.run("set --provider Acme --lead-only Org/Model/V2");
    assert.deepEqual(command.editorSeeds, ["lead\ntext", "lead\ntext"]);
    assert.deepEqual(updates, []);
    assert.equal((await store.load()).rules.length, 2);
    assert.ok(command.notices.some(n => /empty/i.test(n.text)));
    command.setEditor("  preserved\n");
    await command.run("set --provider Acme --lead-only Org/Model/V2");
    assert.equal((await store.load()).rules[1]?.leadOnly, "  preserved\n");
    assert.equal(updates.length, 1);
  }));

  test("show is exact (no fallback), list shows selectors/channels, and clear only removes the selected channel", inTempStore(async store => {
    const updates: ModelPromptConfig[] = [];
    const command = fakeCommand(store, config => updates.push(config));
    await store.update({ id: "Org/Model/V2" }, "general", "fallback");
    await store.update({ id: "Org/Model/V2", provider: "Acme" }, "leadOnly", "specific");
    await command.run("show --provider Acme Org/Model/V2");
    assert.ok(command.notices.some(n => /not set/i.test(n.text)), "qualified general must not fall back");
    await command.run("show --provider Acme --lead-only Org/Model/V2");
    assert.ok(command.notices.some(n => n.text.includes("specific")));
    await command.run("list");
    assert.ok(command.notices.some(n => n.text.includes("Acme") && n.text.includes("lead-only") && n.text.includes("general")));
    await command.run("clear --lead-only --provider Acme Org/Model/V2");
    assert.deepEqual((await store.load()).rules, [{ id: "Org/Model/V2", general: "fallback" }]);
    assert.equal(updates.length, 1);
  }));

  test("writes unknown selectors with a non-blocking warning and invokes callback with saved snapshot", inTempStore(async store => {
    const updates: ModelPromptConfig[] = [];
    const command = fakeCommand(store, config => updates.push(config));
    await command.run("set --provider absent not/in/catalog");
    assert.equal(updates.length, 1);
    assert.deepEqual(updates[0], await store.load());
    assert.equal(updates[0].rules[0]?.id, "not/in/catalog");
    assert.ok(command.notices.some(n => n.type === "warning" && /catalog/i.test(n.text)));
    command.setEditor("known");
    await command.run("set --provider Acme Org/Model/V2");
    assert.equal(command.notices.filter(n => n.type === "warning").length, 1);
  }));

  test("invalid grammar, missing UI, editor errors, and store errors do not call back", inTempStore(async store => {
    const updates: ModelPromptConfig[] = [];
    const command = fakeCommand(store, config => updates.push(config));
    await command.run("set --provider");
    (command.ctx as { hasUI: boolean }).hasUI = false;
    await command.run("set Org/Model/V2");
    (command.ctx as { hasUI: boolean }).hasUI = true;
    command.setEditorError(new Error("editor failed"));
    await command.run("set Org/Model/V2");
    assert.deepEqual(updates, []);
    assert.deepEqual((await store.load()).rules, []);
    assert.ok(command.notices.some(n => /editor failed/.test(n.text)));
    const broken = fakeCommand({
      load: async () => { throw new Error("corrupt configuration"); },
      update: async () => { throw new Error("busy configuration"); },
    }, config => updates.push(config));
    await broken.run("show Org/Model/V2");
    await broken.run("set Org/Model/V2");
    await broken.run("clear Org/Model/V2");
    assert.deepEqual(updates, []);
    assert.ok(broken.notices.some(n => /corrupt configuration/.test(n.text)));
    assert.ok(broken.notices.some(n => /busy configuration/.test(n.text)));
  }));
});
