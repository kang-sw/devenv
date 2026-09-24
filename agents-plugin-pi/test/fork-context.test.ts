import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { applyForkAffinity, captureForkContext, captureRegisteredTools, classifyForkRegistrations, compareForkRegistrations, parseForkContext, readForkLaunchContext, restoreForkContext, writePrivateJson } from "../src/fork-context.ts";

function renameFailure(code: string): NodeJS.ErrnoException {
  return Object.assign(new Error(code), { code });
}

describe("ForkContext", () => {
  const context = captureForkContext({
    kind: "task",
    effectiveSystemPrompt: "lead\r\nprompt  ",
    wsBlock: "block",
    parentSessionKey: "parent",
    parentAffinityId: "affinity",
    activeTools: ["read", "ws-fork"],
    registeredTools: [
      { name: "read", description: "Read", parameters: { type: "object" } },
      { name: "ws-fork", description: "Fork", parameters: { type: "object", properties: { prompt: { type: "string" } } } },
    ],
    modelDescriptor: { provider: "openai-codex", api: "openai-codex-responses", model: "x" },
  });

  test("round-trips exact prompt bytes and rejects malformed present metadata", () => {
    assert.deepEqual(parseForkContext(JSON.stringify(context)), context);
    assert.equal(parseForkContext(undefined), undefined, "only absent metadata is legacy");
    assert.throws(() => parseForkContext('{'), /malformed/);
    assert.throws(() => parseForkContext({ version: 1 }), /malformed/);
    assert.throws(() => parseForkContext(null), /malformed/);
    assert.throws(() => parseForkContext(""), /malformed/);
  });

  test("captures actual registrations in active order and classifies every strict mismatch", () => {
    const actual = captureRegisteredTools(context.activeTools, context.registeredTools);
    assert.equal(compareForkRegistrations(context.registeredTools, actual), undefined);
    assert.deepEqual(classifyForkRegistrations(context.registeredTools, [actual[1]]), {
      missing: [actual[0]], extra: [], reordered: false, changed: [],
    });
    assert.deepEqual(classifyForkRegistrations(context.registeredTools, [...actual, { name: "extra", description: "Extra", parameters: {} }]), {
      missing: [], extra: [{ name: "extra", description: "Extra", parameters: {} }], reordered: false, changed: [],
    });
    assert.match(compareForkRegistrations(context.registeredTools, [...actual].reverse()) ?? "", /reordered callable tools/);
    assert.match(compareForkRegistrations(context.registeredTools, [{ ...actual[0], description: "changed" }, actual[1]]) ?? "", /changed callable tools: read/);
  });

  test("rejects an envelope that omits present context rather than taking the legacy path", () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-fork-context-"));
    try {
      const path = join(directory, "launch.json");
      for (const envelope of [{}, { context: undefined }, { legacy: false }]) {
        writePrivateJson(path, envelope);
        assert.throws(() => readForkLaunchContext({ WS_PI_FORK_CONTEXT: path }), /malformed launch envelope/, JSON.stringify(envelope));
      }
      writePrivateJson(path, { context });
      assert.deepEqual(readForkLaunchContext({ WS_PI_FORK_CONTEXT: path }), { context });
      writePrivateJson(path, { legacy: true });
      assert.deepEqual(readForkLaunchContext({ WS_PI_FORK_CONTEXT: path }), { context: undefined });
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  test("retries Windows EPERM and EBUSY renames with exponential synchronous backoff", () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-fork-context-"));
    try {
      for (const code of ["EPERM", "EBUSY"]) {
        const path = join(directory, `${code}.json`);
        const temporary = `${path}.${code}.tmp`;
        const delays: number[] = [];
        let attempts = 0;
        writeFileSync(path, JSON.stringify({ previous: code }));
        writePrivateJson(path, { replacement: code }, {
          platform: "win32",
          temporaryName: () => code,
          sleep: milliseconds => delays.push(milliseconds),
          rename: (source, destination) => {
            attempts += 1;
            assert.equal(source, temporary);
            assert.equal(destination, path);
            if (attempts === 1) {
              assert.deepEqual(JSON.parse(readFileSync(path, "utf8")), { previous: code });
              throw renameFailure(code);
            }
            renameSync(source, destination);
          },
        });
        assert.equal(attempts, 2);
        assert.deepEqual(delays, [10]);
        assert.deepEqual(JSON.parse(readFileSync(path, "utf8")), { replacement: code });
        assert.equal(existsSync(temporary), false);
      }
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  test("does not retry ineligible rename failures", () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-fork-context-"));
    try {
      for (const { platform, code } of [{ platform: "win32" as const, code: "EACCES" }, { platform: "linux" as const, code: "EPERM" }]) {
        const path = join(directory, `${platform}-${code}.json`);
        const temporary = `${path}.${platform}-${code}.tmp`;
        const failure = renameFailure(code);
        const delays: number[] = [];
        let attempts = 0;
        writeFileSync(path, JSON.stringify({ previous: code }));
        assert.throws(() => writePrivateJson(path, { replacement: code }, {
          platform,
          temporaryName: () => `${platform}-${code}`,
          sleep: milliseconds => delays.push(milliseconds),
          rename: () => { attempts += 1; throw failure; },
        }), error => error === failure);
        assert.equal(attempts, 1);
        assert.deepEqual(delays, []);
        assert.deepEqual(JSON.parse(readFileSync(path, "utf8")), { previous: code });
        assert.equal(existsSync(temporary), false);
      }
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  test("preserves destination and removes the temporary file after exhausted Windows retries", () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-fork-context-"));
    try {
      const path = join(directory, "p".repeat(240), "q".repeat(240), "r".repeat(100));
      const nonce = "n".repeat(600);
      const temporary = `${path}.fixed.tmp`;
      const failures = Array.from({ length: 5 }, () => renameFailure("EPERM"));
      const delays: number[] = [];
      let attempts = 0;
      let diagnostic: Error | undefined;
      mkdirSync(dirname(path), { recursive: true });
      writeFileSync(path, JSON.stringify({ previous: true }));
      assert.throws(() => writePrivateJson(path, {
        replacement: true,
        nonce,
        revision: 17,
        prompt: "never expose this prompt",
        transcript: "never expose this transcript",
      }, {
        platform: "win32",
        temporaryName: () => "fixed",
        sleep: milliseconds => delays.push(milliseconds),
        rename: source => {
          assert.equal(source, temporary);
          assert.deepEqual(JSON.parse(readFileSync(path, "utf8")), { previous: true });
          throw failures[attempts++];
        },
      }), error => {
        diagnostic = error as Error;
        return true;
      });
      assert.equal(attempts, 5);
      assert.ok(diagnostic);
      const fields = diagnostic.message.match(/path=(.+?) writer_pid=\d+ channel_nonce=(.+?) snapshot_revision=/);
      assert.ok(fields, "the transport diagnostic keeps both bounded fields");
      const [, encodedPath, encodedNonce] = fields;
      assert.equal(JSON.parse(encodedPath!), `${path.slice(0, 512)}…`);
      assert.equal(JSON.parse(encodedNonce!), `${nonce.slice(0, 512)}…`);
      assert.equal(JSON.parse(encodedPath!).length, 513, "the path retains 512 characters plus its truncation marker");
      assert.equal(JSON.parse(encodedNonce!).length, 513, "the nonce retains 512 characters plus its truncation marker");
      assert.match(diagnostic.message, new RegExp(`writer_pid=${process.pid}`));
      assert.match(diagnostic.message, /snapshot_revision=17/);
      assert.match(diagnostic.message, /attempts=5/);
      assert.doesNotMatch(diagnostic.message, /never expose|prompt|transcript/);
      assert.equal((diagnostic as NodeJS.ErrnoException).code, "EPERM");
      assert.deepEqual(delays, [10, 20, 40, 80]);
      assert.deepEqual(JSON.parse(readFileSync(path, "utf8")), { previous: true });
      assert.equal(existsSync(temporary), false);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  test("writes readable JSON normally", () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-fork-context-"));
    try {
      const path = join(directory, "normal.json");
      writePrivateJson(path, { nested: ["normal", 1] });
      assert.deepEqual(JSON.parse(readFileSync(path, "utf8")), { nested: ["normal", 1] });
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  test("does not trust a parent-copied durable entry", () => {
    const entry = { type: "custom", customType: "ws-pi-fork-context", data: { sessionId: "parent", context } };
    assert.equal(restoreForkContext([entry], "child"), undefined);
    assert.deepEqual(restoreForkContext([entry], "parent"), context);
  });

  test("rewrites only compatible Codex body affinity", () => {
    const payload = { prompt_cache_key: "child", instructions: "same", tools: [], input: [], model: "x" };
    assert.deepEqual(applyForkAffinity(payload, context, context.modelDescriptor, "child-affinity"), { ...payload, prompt_cache_key: "affinity" });
    assert.equal(applyForkAffinity(payload, context, { ...context.modelDescriptor, api: "openai-completions" }, "child-affinity"), undefined);
    assert.equal(applyForkAffinity({ instructions: "same" }, context, context.modelDescriptor, "child-affinity"), undefined);
  });
});
