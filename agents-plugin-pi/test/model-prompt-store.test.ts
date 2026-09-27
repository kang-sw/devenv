import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import { createModelPromptStore } from "../src/model-prompt-store.ts";

function tempStore(fn: (path: string) => Promise<void>): () => Promise<void> {
  return async () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-model-store-"));
    try { await fn(join(dir, "prompts.json")); }
    finally { rmSync(dir, { recursive: true, force: true }); }
  };
}

describe("model prompt store", () => {
  test("missing file is versioned empty config; restart preserves selectors, channels and text", tempStore(async path => {
    const first = createModelPromptStore(path);
    assert.deepEqual(await first.load(), { version: 1, rules: [] });
    await first.update({ id: "Org/Model/V2" }, "general", "  first\nline  ");
    await first.update({ id: "Org/Model/V2", provider: "Acme" }, "leadOnly", "specific");
    await first.update({ id: "Else" }, "general", "unrelated");
    const restarted = createModelPromptStore(path);
    assert.deepEqual(await restarted.load(), {
      version: 1,
      rules: [
        { id: "Org/Model/V2", general: "  first\nline  " },
        { id: "Org/Model/V2", provider: "Acme", leadOnly: "specific" },
        { id: "Else", general: "unrelated" },
      ],
    });
    await restarted.update({ id: "Org/Model/V2" }, "leadOnly", "fallback lead");
    await restarted.update({ id: "Org/Model/V2" }, "general", undefined);
    assert.deepEqual((await first.load()).rules[0], { id: "Org/Model/V2", leadOnly: "fallback lead" });
    assert.match(readFileSync(path, "utf8"), /"version": 1/);
  }));

  test("malformed, unsupported and ambiguous schema rejects rather than discards on update", tempStore(async path => {
    const malformed: unknown[] = [
      "not-json", null, [], {}, { version: 2, rules: [] }, { version: 1, rules: "not-array" },
      { version: 1, rules: [{ id: "a", general: 2 }] },
      { version: 1, rules: [{ id: "a" }] },
      { version: 1, rules: [{ id: "a", provider: "", general: "x" }] },
      { version: 1, rules: [{ id: "a", general: "x" }, { id: "a", leadOnly: "y" }] },
      { version: 1, rules: [{ id: "a", provider: "p", general: "x" }, { id: "a", provider: "p", leadOnly: "y" }] },
      { version: 1, rules: [], surprise: true },
      { version: 1, rules: [{ id: "a", general: "x", surprise: true }] },
    ];
    const store = createModelPromptStore(path);
    for (const input of malformed) {
      const text = typeof input === "string" ? input : JSON.stringify(input);
      writeFileSync(path, text);
      await assert.rejects(store.load(), /model prompt|unsupported/i, text);
      await assert.rejects(store.update({ id: "new" }, "general", "not lost"), /model prompt|unsupported/i, text);
      assert.equal(readFileSync(path, "utf8"), text, "bad data remains untouched");
    }
    await assert.rejects(store.update({ id: "  " }, "general", "text"), /model prompt|selector/i);
    await assert.rejects(store.update({ id: "valid" }, "general", " \n "), /model prompt|empty/i);
  }));

  test("busy lock times out without stealing and without mutating the old file", tempStore(async path => {
    const store = createModelPromptStore(path);
    await store.update({ id: "existing" }, "general", "old");
    const before = readFileSync(path, "utf8");
    const lock = `${path}.lock`;
    const { mkdirSync } = await import("node:fs");
    mkdirSync(lock);
    const start = Date.now();
    await assert.rejects(store.update({ id: "new" }, "general", "new"), /busy.*lock|lock.*busy/i);
    assert.ok(Date.now() - start < 5000, "bounded lock wait");
    assert.equal(existsSync(lock), true, "lock is never stolen even if its holder is unknown");
    assert.equal(readFileSync(path, "utf8"), before);
  }));

  test("independent parallel process writes merge instead of losing selectors/channels", tempStore(async path => {
    // Two real Node processes contend for one lock, not two stores sharing an event loop.
    const jobs = Array.from({ length: 8 }, (_, n) => {
      const selector = n < 6 ? { id: `other-${n}` } : { id: "shared", provider: "Acme" };
      const channel = n === 7 ? "leadOnly" : "general";
      const script = `import { createModelPromptStore } from ${JSON.stringify(new URL("../src/model-prompt-store.ts", import.meta.url).href)};\n` +
        `await createModelPromptStore(process.argv[1]).update(${JSON.stringify(selector)}, ${JSON.stringify(channel)}, ${JSON.stringify(`value-${n}`)});`;
      return new Promise<void>((resolve, reject) => {
        const child = spawn(process.execPath, ["--input-type=module", "-e", script, path], { stdio: ["ignore", "ignore", "pipe"] });
        let error = "";
        child.stderr.on("data", chunk => { error += chunk; });
        child.on("error", reject);
        child.on("exit", code => code === 0 ? resolve() : reject(new Error(`child ${n} exited ${code}: ${error}`)));
      });
    });
    await Promise.all(jobs);
    const rules = (await createModelPromptStore(path).load()).rules;
    for (const n of [0, 1, 2, 3, 4, 5]) assert.equal(rules.find(r => r.id === `other-${n}`)?.general, `value-${n}`);
    assert.deepEqual(rules.find(r => r.id === "shared" && r.provider === "Acme"), { id: "shared", provider: "Acme", general: "value-6", leadOnly: "value-7" });
    assert.equal(rules.length, 7);
    assert.equal(existsSync(`${path}.lock`), false);
  }));
});
