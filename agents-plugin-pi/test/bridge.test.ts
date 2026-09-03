/**
 * Unit tests for bridge.ts's pure-logic seams: sanitizeToolName,
 * withOptionalSessionKey, resolveSessionKey.
 *
 * `withOptionalSessionKey` regressed once already (the initial live gate
 * run discovered Pi validates tool-call args against the registered
 * `parameters` schema before execute() runs, so an unstripped `required`
 * array silently broke every keyed tool) — these tests are
 * regression-prevention, not ceremony (see rsrc/impl-playbook.md's "Pure
 * logic -> tests first" row).
 *
 * Run with: node --test test/  (from agents-plugin-pi/, Node v22+ native
 * TypeScript type-stripping, zero added dependencies).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { sanitizeToolName, withOptionalSessionKey, resolveSessionKey, maybeAppendModelCatalogAdvisory, MODEL_CATALOG_ADVISORY } from "../src/bridge.ts";
import type { ModelCatalogConfig } from "../src/model-catalog.ts";

// Live snapshot of ws-mcp's tools/list response (60 tools), captured via a
// direct spawnWsMcpClient() probe against this repo's ws-mcp launcher. Not
// re-fetched on every test run (that would make this a subprocess-spawning
// integration test, not a unit test) — if ws-mcp's tool set changes, the
// live gate re-verification step (run separately, see the implementation
// report) is what catches drift; this fixture only locks in the naming
// invariants against the set known at review-fix time.
const LIVE_TOOL_NAMES = [
  "agenda.clear", "agenda.list", "agenda.set", "api.list", "config.list",
  "config.tune", "convention.read", "enter.implement", "enter.proceed",
  "ferrule", "git.commit", "git.diff", "git.log", "git.merge_base",
  "git.status", "infra.read", "mental_models.find", "mental_models.list",
  "mental_models.status", "note.erase", "note.mute", "note.search",
  "note.unmute", "note.write", "path.generate", "playbook.print",
  "playbook.render", "project_tree", "references.trace",
  "runtime.debug_events", "runtime.info", "session.children",
  "session.note", "spec_index.verify", "spec_stem.generate", "specs.find",
  "specs.list", "specs.status", "tickets.checklist", "tickets.close",
  "tickets.create_empty", "tickets.find", "tickets.list", "tickets.move",
  "tickets.sage_gate", "tickets.sage_stamp", "tickets.status",
  "tickets.template", "tickets.verify", "todo.append", "todo.check",
  "todo.clear", "todo.erase", "todo.insert_after", "todo.insert_before",
  "todo.list", "todo.read", "todo.reorder", "workflow_manual",
  "workflow_state",
];

describe("sanitizeToolName", () => {
  test("replaces . with _ and prefixes ws__", () => {
    assert.equal(sanitizeToolName("playbook.print"), "ws__playbook_print");
    assert.equal(sanitizeToolName("tickets.list"), "ws__tickets_list");
    assert.equal(sanitizeToolName("workflow_manual"), "ws__workflow_manual");
    assert.equal(sanitizeToolName("ferrule"), "ws__ferrule");
  });

  test("live tool set: exactly 60 names", () => {
    assert.equal(LIVE_TOOL_NAMES.length, 60);
  });

  test("live tool set: every sanitized name matches provider-legal charset ^[a-zA-Z0-9_-]+$", () => {
    for (const raw of LIVE_TOOL_NAMES) {
      const sanitized = sanitizeToolName(raw);
      assert.match(
        sanitized,
        /^[a-zA-Z0-9_-]+$/,
        `sanitizeToolName(${JSON.stringify(raw)}) = ${JSON.stringify(sanitized)} contains an illegal char`,
      );
    }
  });

  test("live tool set: sanitized names are collision-free", () => {
    const sanitized = LIVE_TOOL_NAMES.map(sanitizeToolName);
    const unique = new Set(sanitized);
    assert.equal(unique.size, sanitized.length, "sanitizeToolName produced a name collision over the live tool set");
  });
});

describe("withOptionalSessionKey", () => {
  test("strips session_key from required[] only, keeps it in properties", () => {
    const input = {
      type: "object",
      properties: {
        session_key: { type: "string" },
        root: { type: "string" },
      },
      required: ["session_key", "root"],
    };
    const result = withOptionalSessionKey(input);
    assert.deepEqual(result.required, ["root"]);
    assert.ok("session_key" in (result.properties as Record<string, unknown>), "session_key must remain in properties");
    assert.deepEqual(
      (result.properties as Record<string, unknown>).session_key,
      { type: "string" },
      "session_key's property definition must be unchanged",
    );
  });

  test("does not mutate the source schema object", () => {
    const input = {
      type: "object",
      properties: { session_key: { type: "string" } },
      required: ["session_key"],
    };
    const inputRequiredRef = input.required;
    const inputSnapshot = JSON.parse(JSON.stringify(input));
    const result = withOptionalSessionKey(input);
    assert.deepEqual(input, inputSnapshot, "input schema object must not be mutated");
    assert.equal(input.required, inputRequiredRef, "input.required array identity must be preserved (not spliced in place)");
    assert.notEqual(result, input, "must return a fresh object, not the same reference");
  });

  test("returns the schema unchanged (same reference) when required is absent", () => {
    const input = { type: "object", properties: {} };
    const result = withOptionalSessionKey(input);
    assert.equal(result, input);
  });

  test("returns the schema unchanged when required does not include session_key", () => {
    const input = { type: "object", properties: { root: { type: "string" } }, required: ["root"] };
    const result = withOptionalSessionKey(input);
    assert.equal(result, input);
  });
});

describe("resolveSessionKey", () => {
  test("fills the default key when session_key is absent", () => {
    const result = resolveSessionKey({ root: "." }, { current: "default-key-123" });
    assert.equal(result.session_key, "default-key-123");
    assert.equal(result.root, ".");
  });

  test("fills the default key when session_key is null", () => {
    const result = resolveSessionKey({ session_key: null }, { current: "default-key-123" });
    assert.equal(result.session_key, "default-key-123");
  });

  test("fills the default key when session_key is an empty string", () => {
    const result = resolveSessionKey({ session_key: "" }, { current: "default-key-123" });
    assert.equal(result.session_key, "default-key-123");
  });

  test("forwards an explicit session_key verbatim, not overwritten by the default", () => {
    const result = resolveSessionKey({ session_key: "explicit-key-456" }, { current: "default-key-123" });
    assert.equal(result.session_key, "explicit-key-456");
  });

  test("leaves session_key omitted when no default is set yet", () => {
    const result = resolveSessionKey({ root: "." }, { current: undefined });
    assert.equal("session_key" in result, false);
  });

  test("handles undefined params", () => {
    const result = resolveSessionKey(undefined, { current: "default-key-123" });
    assert.equal(result.session_key, "default-key-123");
  });

  test("does not mutate the input params object", () => {
    const input = { session_key: "", root: "." };
    const inputSnapshot = { ...input };
    resolveSessionKey(input, { current: "default-key-123" });
    assert.deepEqual(input, inputSnapshot, "input params object must not be mutated");
  });
});

describe("maybeAppendModelCatalogAdvisory", () => {
  const unsetConfig: ModelCatalogConfig | undefined = undefined;
  const setConfig: ModelCatalogConfig = { tiers: { small: "openrouter/cheap-model" } };

  test("appends the advisory for workflow_manual when the catalog is unset", () => {
    const content = [{ type: "text", text: "manual body" }];
    const result = maybeAppendModelCatalogAdvisory("workflow_manual", content, unsetConfig);
    assert.equal(result.length, 2);
    assert.deepEqual(result[0], { type: "text", text: "manual body" });
    assert.equal(result[1].type, "text");
    assert.equal(result[1].text, MODEL_CATALOG_ADVISORY);
  });

  test("appends (not prepends) — the advisory is the last item", () => {
    const content = [{ type: "text", text: "first" }, { type: "text", text: "second" }];
    const result = maybeAppendModelCatalogAdvisory("workflow_manual", content, unsetConfig);
    assert.equal(result[result.length - 1].text, MODEL_CATALOG_ADVISORY);
  });

  test("returns a copy — does not mutate the input content array", () => {
    const content = [{ type: "text", text: "manual body" }];
    const contentRef = content;
    const result = maybeAppendModelCatalogAdvisory("workflow_manual", content, unsetConfig);
    assert.equal(content, contentRef, "input array identity must be preserved (not spliced in place)");
    assert.equal(content.length, 1, "input array must not be mutated");
    assert.notEqual(result, content, "must return a fresh array, not the same reference");
  });

  test("does not append when the small tier is configured", () => {
    const content = [{ type: "text", text: "manual body" }];
    const result = maybeAppendModelCatalogAdvisory("workflow_manual", content, setConfig);
    assert.equal(result, content, "content must be returned unchanged (same reference) when the catalog is set");
    assert.equal(result.length, 1);
  });

  test("does not append for a different tool name, even when the catalog is unset", () => {
    const content = [{ type: "text", text: "some other tool's body" }];
    const result = maybeAppendModelCatalogAdvisory("playbook.render", content, unsetConfig);
    assert.equal(result, content);
    assert.equal(result.length, 1);
  });

  test("does not append for the sanitized ws__workflow_manual name — only the raw dotted-less name matches", () => {
    const content = [{ type: "text", text: "manual body" }];
    const result = maybeAppendModelCatalogAdvisory("ws__workflow_manual", content, unsetConfig);
    assert.equal(result, content);
  });
});
