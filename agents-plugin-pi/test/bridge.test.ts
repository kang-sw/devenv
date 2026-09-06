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
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createRequire } from "node:module";
import { initTheme } from "@earendil-works/pi-coding-agent";
import { pathToFileURL } from "node:url";
import { loadToolResultWidth, createToolResultComponent, type RenderResultRowsOptions } from "../src/tool-result-render.ts";
import {
  sanitizeToolName,
  filterOutMercenaryTools,
  withOptionalSessionKey,
  resolveSessionKey,
  normalizeSessionKey,
  maybeAppendModelCatalogAdvisory,
  MODEL_CATALOG_ADVISORY,
  computePiAliasTableReport,
  computeRawDispatchPiAliasTableReport,
  cutStaticBody,
  prependWorkflowStateLine,
  shouldMapWorkflowManual,
  dispatchMappedWorkflowManual,
  renderResultRows as renderRows,
  renderResultText,
  yamlDisplayText,
  startBridge,
} from "../src/bridge.ts";
import type { McpToolCallResult } from "../src/mcp-stdio-client.ts";

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

describe("filterOutMercenaryTools", () => {
  test("drops every mercenary.* raw name, keeps the rest in original order", () => {
    const fixture = [
      { name: "playbook.print" },
      { name: "mercenary.register" },
      { name: "ferrule" },
      { name: "mercenary.call" },
      { name: "mercenary.debug.tail" },
      { name: "tickets.list" },
    ];
    const filtered = filterOutMercenaryTools(fixture);
    assert.deepEqual(
      filtered.map((tool) => tool.name),
      ["playbook.print", "ferrule", "tickets.list"],
    );
  });

  test("no mercenary.* present: list passes through unchanged", () => {
    const fixture = [{ name: "playbook.print" }, { name: "ferrule" }, { name: "tickets.list" }];
    const filtered = filterOutMercenaryTools(fixture);
    assert.deepEqual(filtered.map((tool) => tool.name), ["playbook.print", "ferrule", "tickets.list"]);
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

describe("normalizeSessionKey", () => {
  const SENTINEL = "obsidian-latch";

  test("rewrites the sentinel to the bridge's own key", () => {
    const result = normalizeSessionKey({ session_key: SENTINEL }, { ownKey: "own-key-1", sentinel: SENTINEL });
    assert.equal(result?.session_key, "own-key-1");
  });

  test("rewrites an explicit parentLeadKey (fork-only) to the bridge's own key", () => {
    const result = normalizeSessionKey(
      { session_key: "parent-lead-key" },
      { ownKey: "own-key-1", sentinel: SENTINEL, parentLeadKey: "parent-lead-key" },
    );
    assert.equal(result?.session_key, "own-key-1");
  });

  test("leaves an unrelated explicit (child) key completely unchanged — regression: must still reach ws-mcp unchanged", () => {
    const result = normalizeSessionKey(
      { session_key: "some-child-session-key" },
      { ownKey: "own-key-1", sentinel: SENTINEL, parentLeadKey: "parent-lead-key" },
    );
    assert.equal(result?.session_key, "some-child-session-key");
  });

  test("leaves an omitted session_key untouched (resolveSessionKey's job, not normalizeSessionKey's)", () => {
    const result = normalizeSessionKey({ root: "." }, { ownKey: "own-key-1", sentinel: SENTINEL });
    assert.equal("session_key" in (result ?? {}), false);
    assert.equal(result?.root, ".");
  });

  test("degraded bootstrap (ownKey unset): sentinel passes through unchanged, both rewrites disabled", () => {
    const result = normalizeSessionKey(
      { session_key: SENTINEL },
      { ownKey: undefined, sentinel: SENTINEL, parentLeadKey: "parent-lead-key" },
    );
    assert.equal(result?.session_key, SENTINEL);
  });

  test("degraded bootstrap (ownKey unset): parentLeadKey match also passes through unchanged", () => {
    const result = normalizeSessionKey(
      { session_key: "parent-lead-key" },
      { ownKey: undefined, sentinel: SENTINEL, parentLeadKey: "parent-lead-key" },
    );
    assert.equal(result?.session_key, "parent-lead-key");
  });

  test("handles undefined params (no explicit key at all)", () => {
    const result = normalizeSessionKey(undefined, { ownKey: "own-key-1", sentinel: SENTINEL });
    assert.equal(result, undefined);
  });

  test("does not mutate the input params object", () => {
    const input = { session_key: SENTINEL };
    const inputSnapshot = { ...input };
    normalizeSessionKey(input, { ownKey: "own-key-1", sentinel: SENTINEL });
    assert.deepEqual(input, inputSnapshot, "input params object must not be mutated");
  });
});

describe("cutStaticBody", () => {
  test("removes the first occurrence of the static body substring", () => {
    const result = cutStaticBody("HEADER\nSTATIC BODY\nFOOTER", "STATIC BODY\n");
    assert.equal(result.found, true);
    assert.equal(result.text, "HEADER\nFOOTER");
  });

  test("found:false when the static body does not appear (renderer drift)", () => {
    const result = cutStaticBody("HEADER\nsomething else\nFOOTER", "STATIC BODY\n");
    assert.equal(result.found, false);
    assert.equal(result.text, "HEADER\nsomething else\nFOOTER");
  });

  test("only removes the first occurrence when the substring repeats", () => {
    const result = cutStaticBody("Xabc Xabc", "Xabc");
    assert.equal(result.found, true);
    assert.equal(result.text, " Xabc");
  });
});

describe("prependWorkflowStateLine", () => {
  test("prepends the fixed line ahead of the given text", () => {
    const result = prependWorkflowStateLine("## Session Key\n...");
    assert.match(result, /^Workflow manual is in your system prompt; this is your current session state\./);
    assert.ok(result.endsWith("## Session Key\n..."));
  });
});

describe("shouldMapWorkflowManual", () => {
  test("worker role: false even with a snapshot present (must forward workflow_manual verbatim)", () => {
    assert.equal(shouldMapWorkflowManual("workflow_manual", true, "worker"), false);
  });

  test("explore role: false even with a snapshot present (must forward workflow_manual verbatim)", () => {
    assert.equal(shouldMapWorkflowManual("workflow_manual", true, "explore"), false);
  });

  test("host lead (role undefined) with a snapshot present: true", () => {
    assert.equal(shouldMapWorkflowManual("workflow_manual", true, undefined), true);
  });

  test("fork role with a snapshot present: true", () => {
    assert.equal(shouldMapWorkflowManual("workflow_manual", true, "fork"), true);
  });

  test("host lead with no snapshot present: false (degraded bootstrap)", () => {
    assert.equal(shouldMapWorkflowManual("workflow_manual", false, undefined), false);
  });

  test("fork role with no snapshot present: false (degraded bootstrap)", () => {
    assert.equal(shouldMapWorkflowManual("workflow_manual", false, "fork"), false);
  });

  test("a different rawName never maps, even for a lead role with a snapshot present", () => {
    assert.equal(shouldMapWorkflowManual("playbook.render", true, undefined), false);
  });

  test("the sanitized ws__workflow_manual name never matches — only the raw dotted-less name does", () => {
    assert.equal(shouldMapWorkflowManual("ws__workflow_manual", true, undefined), false);
  });
});

describe("dispatchMappedWorkflowManual", () => {
  function textResult(text: string): McpToolCallResult {
    return { content: [{ type: "text", text }] };
  }

  /** A `config.resolve_agent` stub reply carrying no genuine `pi` hit — every tier's own tests below don't care which tier was asked. */
  function noHitResolveAgentResult(): McpToolCallResult {
    return textResult(JSON.stringify({ resolved_from: "default", model: "gpt-5.6-terra" }));
  }

  /** A `config.resolve_agent` stub reply carrying a genuine `pi` hit. */
  function hitResolveAgentResult(): McpToolCallResult {
    return textResult(JSON.stringify({ resolved_from: "pi", model: "openrouter/cheap-model" }));
  }

  test("cut found: returns the cut text with the fixed line prepended, no workflow_state dispatch", async () => {
    const calls: Array<{ name: string; args: Record<string, unknown> }> = [];
    const result = await dispatchMappedWorkflowManual(
      { session_key: "lead-1", root: "/repo" },
      {
        callTool: async (name, args) => {
          calls.push({ name, args });
          if (name === "config.resolve_agent") return noHitResolveAgentResult();
          return textResult("HEADER\nSTATIC-BODY\n## Session Key\nlead-1");
        },
        staticBodySnapshot: "STATIC-BODY\n",
        catalog: [{ provider: "openrouter", id: "cheap-model", hasAuth: true }],
        notifyMappingDegraded: () => assert.fail("notifyMappingDegraded must not be called on a cut hit"),
      },
    );
    const wsCalls = calls.filter((c) => c.name !== "config.resolve_agent");
    assert.deepEqual(wsCalls, [{ name: "workflow_manual", args: { session_key: "lead-1", root: "/repo" } }]);
    const text = result.content.find((item) => item.type === "text")?.text;
    assert.match(text ?? "", /^Workflow manual is in your system prompt/);
    assert.ok(text?.includes("## Session Key\nlead-1"));
    assert.ok(!text?.includes("STATIC-BODY"));
  });

  test("cut miss: falls back to workflow_state, dropping root, and notifies once", async () => {
    const calls: Array<{ name: string; args: Record<string, unknown> }> = [];
    let notified = 0;
    const result = await dispatchMappedWorkflowManual(
      { session_key: "lead-1", root: "/repo" },
      {
        callTool: async (name, args) => {
          calls.push({ name, args });
          if (name === "workflow_manual") return textResult("HEADER\nsomething drifted\n## Session Key\nlead-1");
          if (name === "config.resolve_agent") return noHitResolveAgentResult();
          return textResult("## Session State\ntodos: none");
        },
        staticBodySnapshot: "STATIC-BODY\n",
        catalog: [{ provider: "openrouter", id: "cheap-model", hasAuth: true }],
        notifyMappingDegraded: () => {
          notified += 1;
        },
      },
    );
    const wsCalls = calls.filter((c) => c.name !== "config.resolve_agent");
    assert.deepEqual(wsCalls, [
      { name: "workflow_manual", args: { session_key: "lead-1", root: "/repo" } },
      { name: "workflow_state", args: { session_key: "lead-1" } },
    ]);
    assert.equal(notified, 1);
    const text = result.content.find((item) => item.type === "text")?.text;
    assert.match(text ?? "", /^Workflow manual is in your system prompt/);
    assert.ok(text?.includes("## Session State"));
  });

  test("advisory still appends on the mapped response when no tier has a genuine pi entry", async () => {
    const result = await dispatchMappedWorkflowManual(
      { session_key: "lead-1" },
      {
        callTool: async (name) => (name === "config.resolve_agent" ? noHitResolveAgentResult() : textResult("HEADER\nSTATIC-BODY\nBODY")),
        staticBodySnapshot: "STATIC-BODY\n",
        catalog: [{ provider: "openrouter", id: "cheap-model", hasAuth: true }],
        notifyMappingDegraded: () => {},
      },
    );
    assert.equal(result.content.length, 2, "advisory must be appended as an additional content item");
    assert.equal(result.content[1].text, MODEL_CATALOG_ADVISORY);
  });

  test("advisory is suppressed when at least one tier has a genuine pi entry", async () => {
    const result = await dispatchMappedWorkflowManual(
      { session_key: "lead-1" },
      {
        callTool: async (name, args) => {
          if (name === "config.resolve_agent" && args.tier === "large") return hitResolveAgentResult();
          if (name === "config.resolve_agent") return noHitResolveAgentResult();
          return textResult("HEADER\nSTATIC-BODY\nBODY");
        },
        staticBodySnapshot: "STATIC-BODY\n",
        catalog: [{ provider: "openrouter", id: "cheap-model", hasAuth: true }],
        notifyMappingDegraded: () => {},
      },
    );
    assert.equal(result.content.length, 1, "no advisory item when a genuine pi tier exists");
  });

  test("throws when the workflow_manual dispatch itself errors", async () => {
    await assert.rejects(
      () =>
        dispatchMappedWorkflowManual(
          { session_key: "lead-1" },
          {
            callTool: async () => ({ content: [{ type: "text", text: "boom" }], isError: true }),
            staticBodySnapshot: "STATIC-BODY\n",
        catalog: [{ provider: "openrouter", id: "cheap-model", hasAuth: true }],
            notifyMappingDegraded: () => {},
          },
        ),
      /boom/,
    );
  });
});

describe("computePiAliasTableReport", () => {
  function textResult(text: string): McpToolCallResult {
    return { content: [{ type: "text", text }] };
  }

  test("a genuine pi hit on any tier -> false", async () => {
    const result = await computePiAliasTableReport(async (name, args) => {
      assert.equal(name, "config.resolve_agent");
      return args.tier === "medium"
        ? textResult(JSON.stringify({ resolved_from: "pi", model: "openrouter/big" }))
        : textResult(JSON.stringify({ resolved_from: "default" }));
    }, [{ provider: "openrouter", id: "big", hasAuth: true }]);
    assert.deepEqual(result, { unset: false, rejected: [] });
  });

  test("no tier resolves to pi -> true", async () => {
    const result = await computePiAliasTableReport(async (name) => {
      assert.equal(name, "config.resolve_agent");
      return textResult(JSON.stringify({ resolved_from: "default", model: "gpt-5.6-terra" }));
    });
    assert.deepEqual(result, { unset: true, rejected: [] });
  });

  test("rejected tiers replace the empty table sentence, even after an accepted tier", async () => {
    const tiers: unknown[] = [];
    const catalog = [{ provider: "p", id: "good", hasAuth: true }, { provider: "locked", id: "good", hasAuth: false }];
    const result = await computePiAliasTableReport(async (_name, args) => {
      tiers.push(args.tier);
      const model = { small: "p/good", medium: "good", large: "locked/good", xlarge: "typo/good" }[args.tier as string];
      return textResult(JSON.stringify({ resolved_from: "pi", model }));
    }, catalog);
    assert.deepEqual(tiers, ["small", "medium", "large", "xlarge"]);
    assert.equal(result.unset, false);
    assert.deepEqual(result.rejected.map(r => r.alias), ["medium", "large", "xlarge"]);
    const content = maybeAppendModelCatalogAdvisory("workflow_manual", [], result, "lead/model", false);
    const text = content[0].text!;
    assert.equal(text.match(/warning: tier/g)?.length, 3);
    assert.match(text, /Did you mean p\/good, locked\/good\?/);
    assert.match(text, /provider locked has no configured auth/);
    assert.doesNotMatch(text, /has no entries|ws-model-catalog-list/);
  });

  for (const cut of [true, false]) {
    test(`mapped ${cut ? "cut" : "fallback"} appends one rejection advisory`, async () => {
      const original = textResult(cut ? "HEADER STATIC FOOTER" : "drifted");
      const result = await dispatchMappedWorkflowManual({}, {
        staticBodySnapshot: "STATIC", catalog: [], inheritModel: "lead/model",
        notifyMappingDegraded: () => {},
        callTool: async (name, args) => name === "workflow_manual" ? original : name === "workflow_state" ? textResult("state") :
          textResult(JSON.stringify(args.tier === "xlarge" ? { resolved_from: "pi", model: "bad" } : { resolved_from: "tiers" })),
      });
      assert.equal(result.content.length, 2);
      assert.equal(result.content[1].text!.match(/warning: tier/g)?.length, 1);
      assert.match(result.content[1].text!, /Pi's model catalog is empty/);
      assert.doesNotMatch(result.content[1].text!, /has no entries|ws-model-catalog-list/);
      assert.equal(original.content.length, 1);
      assert.equal(original.content[0].text, cut ? "HEADER STATIC FOOTER" : "drifted");
    });
  }

  test("an isError result on a tier is treated as a miss, not a crash", async () => {
    const result = await computePiAliasTableReport(async (name) => {
      assert.equal(name, "config.resolve_agent");
      return { content: [{ type: "text", text: "boom" }], isError: true };
    });
    assert.deepEqual(result, { unset: true, rejected: [] });
  });

  test("unparsable text on a tier is treated as a miss, not a crash", async () => {
    const result = await computePiAliasTableReport(async (name) => {
      assert.equal(name, "config.resolve_agent");
      return textResult("not json");
    });
    assert.deepEqual(result, { unset: true, rejected: [] });
  });

  test("a thrown call is treated as a miss, not a crash (never-hard-fail)", async () => {
    const result = await computePiAliasTableReport(async (name) => {
      assert.equal(name, "config.resolve_agent");
      throw new Error("stdio pipe broke");
    });
    assert.deepEqual(result, { unset: true, rejected: [] });
  });

  test("queries all four fixed tiers with format:json", async () => {
    const seenTiers: unknown[] = [];
    await computePiAliasTableReport(async (name, args) => {
      assert.equal(name, "config.resolve_agent");
      seenTiers.push(args.tier);
      assert.equal(args.format, "json");
      return textResult(JSON.stringify({ resolved_from: "default" }));
    });
    assert.deepEqual(seenTiers, ["small", "medium", "large", "xlarge"]);
  });
});

describe("computeRawDispatchPiAliasTableReport (review relay #1, Important/test: the raw-dispatch advisory gate)", () => {
  function textResult(text: string): McpToolCallResult {
    return { content: [{ type: "text", text }] };
  }

  test("a non-workflow_manual rawName never calls callTool at all (no config.resolve_agent round-trip)", async () => {
    let called = false;
    const result = await computeRawDispatchPiAliasTableReport("playbook.render", async () => {
      called = true;
      return textResult("{}");
    });
    assert.deepEqual(result, { unset: false, rejected: [] }, "the gate itself must resolve to false without ever invoking callTool");
    assert.equal(called, false, "callTool must never be invoked for a rawName other than workflow_manual");
  });

  test("workflow_manual delegates to computePiAliasTableReport (config.resolve_agent IS called)", async () => {
    let sawResolveAgentCall = false;
    const result = await computeRawDispatchPiAliasTableReport("workflow_manual", async (name) => {
      if (name === "config.resolve_agent") sawResolveAgentCall = true;
      return textResult(JSON.stringify({ resolved_from: "default" }));
    });
    assert.equal(sawResolveAgentCall, true, "workflow_manual must trigger the config.resolve_agent round-trips");
    assert.deepEqual(result, { unset: true, rejected: [] }, "no tier resolved to a genuine pi hit, so the advisory-trigger value is true");
  });
});

// Import the actual installed Pi oracle independently of the production loader.
const piTuiPath = createRequire(import.meta.resolve("@earendil-works/pi-coding-agent")).resolve("@earendil-works/pi-tui");
const { visibleWidth: visibleDisplayWidth } = await import(pathToFileURL(piTuiPath).href);

describe("Pi tool-result YAML rendering", () => {
  const plainHint = "E to expand";
  const renderResultRows = (content: Parameters<typeof renderRows>[0], options: Omit<RenderResultRowsOptions, "measure">) =>
    renderRows(content, { ...options, measure: visibleDisplayWidth });

  test("renders JSON objects and arrays as YAML only in the display text", () => {
    assert.equal(renderResultText([{ type: "text", text: '{"name":"orca","ready":true}' }], { isError: false }), "name: orca\nready: true\n");
    assert.equal(renderResultText([{ type: "text", text: '["one",2]' }], { isError: false }), "- one\n- 2\n");
  });

  test("keeps prose, bare JSON values, malformed JSON-looking text, and errors raw", () => {
    for (const text of ["plain prose", '"bare string"', "42", "null", "{not json}"]) {
      assert.equal(renderResultText([{ type: "text", text }], { isError: false }), text);
    }
    assert.equal(renderResultText([{ type: "text", text: '{"error":"raw"}' }], { isError: true }), '{"error":"raw"}');
  });

  test("converts only the first text block and retains later text plus image blocks", () => {
    const content = [
      { type: "image", data: "base64", mimeType: "image/png" },
      { type: "text", text: '{"first":true}' },
      { type: "text", text: '{"later":"must stay JSON"}' },
    ];
    const wirePayload = JSON.stringify(content);
    assert.equal(renderResultText(content, { isError: false }), "first: true\n\n{\"later\":\"must stay JSON\"}");
    renderResultRows(content, { isError: false, expanded: false, width: 80, expandHint: plainHint });
    assert.equal(JSON.stringify(content), wirePayload, "display rendering must not alter the model-visible execute content");
    assert.equal(content[0].type, "image", "renderer must not remove Pi-managed image content");
    assert.equal(content[2].text, '{"later":"must stay JSON"}', "later text is retained raw");
  });

  test("wraps long raw and YAML lines by display width on narrow terminals", () => {
    const rawRows = renderResultRows([{ type: "text", text: "abcdefghijkl" }], { isError: false, expanded: false, width: 4, expandHint: plainHint });
    const yamlRows = renderResultRows([{ type: "text", text: '{"long":"abcdefghijkl"}' }], { isError: false, expanded: false, width: 6, expandHint: plainHint });
    assert.deepEqual(rawRows, ["abcd", "efgh", "ijkl"]);
    assert.ok(yamlRows.length > 1);
    for (const row of [...rawRows, ...yamlRows]) assert.ok(visibleDisplayWidth(row) <= 6);
  });

  test("keeps CJK, combining marks, and emoji graphemes within width boundaries", () => {
    const rows = renderResultRows([{ type: "text", text: "界e\u0301🙂界" }], { isError: false, expanded: true, width: 4, expandHint: plainHint });
    assert.deepEqual(rows, ["界e\u0301", "🙂界"]);
    for (const row of rows) assert.ok(visibleDisplayWidth(row) <= 4, `${JSON.stringify(row)} is too wide`);
  });

  test("collapsed previews show exactly ten body rows and a truthful visual-row marker", () => {
    const body = Array.from({ length: 12 }, (_, i) => `line-${i + 1}`).join("\n");
    const collapsed = renderResultRows([{ type: "text", text: body }], { isError: false, expanded: false, width: 80, expandHint: plainHint });
    const expanded = renderResultRows([{ type: "text", text: body }], { isError: false, expanded: true, width: 80, expandHint: plainHint });
    assert.deepEqual(collapsed.slice(0, 10), Array.from({ length: 10 }, (_, i) => `line-${i + 1}`));
    assert.equal(collapsed[10], "… 2 more rows (E to expand)");
    assert.deepEqual(expanded, Array.from({ length: 12 }, (_, i) => `line-${i + 1}`));
  });

  test("collapsed marker counts wrapped raw and YAML body rows, and every marker row fits", () => {
    const raw = renderResultRows([{ type: "text", text: "abcdefghijkl" }], { isError: false, expanded: false, width: 1, expandHint: plainHint });
    const yaml = renderResultRows([{ type: "text", text: `{"a":"${"x".repeat(60)}"}` }], { isError: false, expanded: false, width: 4, expandHint: plainHint });
    assert.equal(raw[10], "…");
    assert.ok(yaml.slice(10).join("").includes("more rows"), "YAML preview must carry a hidden visual-row marker");
    for (const row of [...raw, ...yaml]) assert.ok(visibleDisplayWidth(row) <= 4, `${JSON.stringify(row)} is too wide`);
  });

  test("falls back when YAML serialization throws and keeps mixed error content raw", () => {
    const raw = '{"value":"must stay raw"}';
    assert.equal(yamlDisplayText(raw, () => { throw new Error("serializer failed"); }), raw);
    const errorContent = [
      { type: "text", text: raw },
      { type: "image", data: "base64", mimeType: "image/png" },
      { type: "text", text: '{"later":"also raw"}' },
    ];
    assert.equal(
      renderResultText(errorContent, { isError: true, showImages: false }),
      `${raw}\n{\"later\":\"also raw\"}\n[Image: [image/png]]`,
    );
  });

  test("normalizes tabs, keycaps, decorated emoji, and terminal control sequences before width-safe rendering", () => {
    const tabs = renderResultRows([{ type: "text", text: "a\tb" }], { isError: false, expanded: true, width: 2, expandHint: plainHint });
    const keycaps = renderResultRows([{ type: "text", text: "1️⃣1️⃣" }], { isError: false, expanded: true, width: 2, expandHint: plainHint });
    const decoratedEmoji = ["🙂\u0301🙂\u0301", "🙂\uFE0E🙂\uFE0E"].map((text) =>
      renderResultRows([{ type: "text", text }], { isError: false, expanded: true, width: 2, expandHint: plainHint }),
    );
    const controls = renderResultRows([{ type: "text", text: "before\x1b[2J\x1b[Hafter\x1b]52;c;clipboard\x07" }], { isError: false, expanded: true, width: 80, expandHint: plainHint });
    assert.deepEqual(tabs, ["a ", "  ", "b"]);
    assert.deepEqual(keycaps, ["1️⃣", "1️⃣"]);
    assert.deepEqual(decoratedEmoji, [["🙂\u0301", "🙂\u0301"], ["🙂\uFE0E", "🙂\uFE0E"]]);
    assert.deepEqual(controls, ["beforeafter"]);
    for (const row of [...tabs, ...keycaps, ...decoratedEmoji.flat(), ...controls]) assert.ok(visibleDisplayWidth(row) <= 2 || row === "beforeafter");
  });

  test("loads Pi's width dynamically, supports nested installs, and degrades without a guessed metric", async () => {
    assert.equal(await loadToolResultWidth(), visibleDisplayWidth);
    assert.equal(await loadToolResultWidth(async () => ({ visibleWidth: visibleDisplayWidth }), () => {
      throw new Error("nested lookup must not run after a direct hit");
    }), visibleDisplayWidth);
    const calls: string[] = [];
    assert.equal(await loadToolResultWidth(async (specifier) => {
      calls.push(specifier);
      if (specifier === "nested") return { visibleWidth: visibleDisplayWidth };
      throw new Error("not hoisted");
    }, () => "nested"), visibleDisplayWidth);
    assert.deepEqual(calls, ["@earendil-works/pi-tui", "nested"]);
    assert.equal(await loadToolResultWidth(async () => ({}), () => "nested"), undefined);
    assert.equal(await loadToolResultWidth(async () => { throw new Error("unavailable"); }, () => { throw new Error("unresolved"); }), undefined);
  });

  test("uses actual Pi width for indivisible modifier/mark chains, including clusters wider than two cells", () => {
    const text = "\u{1F3FB}\u0301\u{1F3FB}\u0301";
    assert.equal(visibleDisplayWidth(text), 4);
    const content = [{ type: "text", text }];
    const original = JSON.stringify(content);
    for (const width of [1, 2, 3]) {
      assert.deepEqual(renderResultRows(content, { isError: true, expanded: true, width }), ["?"]);
    }
    assert.deepEqual(renderResultRows(content, { isError: true, expanded: true, width: 4 }), [text]);
    assert.deepEqual(renderResultRows(content, { isError: true, expanded: true, width: 0 }), []);
    assert.equal(JSON.stringify(content), original);
  });

  test("comparative grapheme sweep bounds every row against actual Pi, not a local Unicode approximation", () => {
    const segmenter = new Intl.Segmenter(undefined, { granularity: "grapheme" });
    const bases = ["a", "界", "ᄀ", "ᅡ", "각", "ｶ", "ﾞ", "क्‍ष", "กำ", "ກຳ", "\u093e", "\u0bbe", "\u0301", "1️⃣", "🇰🇷", "👩🏽‍💻"];
    for (let code = 0x1f000; code <= 0x1faff; code++) bases.push(String.fromCodePoint(code));
    let cases = 0;
    for (const base of bases) {
      for (const suffix of ["", "\u0301", "\uFE0E", "\uFE0F"]) {
        const text = (base + suffix).repeat(2);
        const segments = [...segmenter.segment(text)].map(({ segment }) => segment);
        for (const width of [1, 2, 3, 4]) {
          const rows = renderResultRows([{ type: "text", text }], { isError: true, expanded: true, width });
          for (const row of rows) assert.ok(visibleDisplayWidth(row) <= width, `${JSON.stringify(text)} at ${width}: ${JSON.stringify(row)}`);
          assert.equal(rows.join(""), segments.map((segment) => visibleDisplayWidth(segment) > width ? "?" : segment).join(""));
          cases++;
        }
      }
    }
    assert.equal(cases, 45312);
  });

  test("actual Pi validates styled raw/YAML collapse and expansion across scripts and narrow widths", () => {
    initTheme("dark", false);
    const samples = ["\u{1F3FB}\u0301\u{1F3FB}\u0301", "🙂\uFE0E", "🙂\uFE0F", "a\u0301", "界", "1️⃣", "👩🏽‍💻", "क्‍ष", "กำ", "ｶﾞ", "\u093e"];
    const theme = { fg: (_color: string, text: string) => `\x1b[32m${text}\x1b[0m` };
    for (const sample of samples) {
      const body = Array.from({ length: 12 }, () => sample.repeat(3)).join("\n");
      for (const text of [body, JSON.stringify({ body })]) {
        const content = [{ type: "text", text }];
        for (const width of [1, 2, 3, 4, 10, 40]) {
          const options = { isError: false, width, expandHint: plainHint };
          const expanded = renderResultRows(content, { ...options, expanded: true });
          const collapsed = renderResultRows(content, { ...options, expanded: false });
          assert.ok(expanded.length > 10);
          assert.deepEqual(collapsed.slice(0, 10), expanded.slice(0, 10));
          assert.equal(collapsed.slice(10).join(""), `… ${expanded.length - 10} more rows (${plainHint})`);
          for (const expand of [true, false]) {
            const component = createToolResultComponent(content, { expanded: expand }, theme, { isError: false, showImages: true }, visibleDisplayWidth);
            for (const row of component.render(width)) assert.ok(visibleDisplayWidth(row) <= width);
          }
        }
      }
    }
  });

  test("ASCII fallback sanitizes and escapes every body/marker character without guessing Unicode width", () => {
    const raw = "A界🙂\u0301\u007f\u0085\u200d\ud800";
    const escaped = "A\\u754c\\ud83d\\ude42\\u0301\\u007f\\u0085\\u200d\\ud800";
    const content = [{ type: "text", text: Array(12).fill(raw).join("\n") }];
    const bytes = JSON.stringify(content);
    const expandHint = "⌘\x1b[31mE\x1b[0m\n to expand\x1b]52;c;clipboard\x07";
    for (const width of [0, 1, 2, 3, 40]) {
      for (const expanded of [false, true]) {
        const rows = renderRows(content, { isError: false, expanded, width, expandHint });
        if (width === 0) {
          assert.deepEqual(rows, []);
          continue;
        }
        assert.equal(rows.join(""), expanded ? escaped.repeat(12) :
          escaped.repeat(10) + "\\u2026 2 more lines (\\u2318E\\u000a to expand)");
        for (const row of rows) {
          assert.match(row, /^[\x20-\x7e]*$/);
          assert.ok(visibleDisplayWidth(row) <= width);
        }
      }
    }
    assert.equal(JSON.stringify(content), bytes);
  });

  test("ASCII fallback preserves parser/serializer/error and mixed-content policies", () => {
    const options = { isError: false, expanded: true, width: 1, expandHint: plainHint };
    for (const [text, expected] of [
      ['"bare界"', '"bare\\u754c"'],
      ["{bad🙂}", "{bad\\ud83d\\ude42}"],
      ["a\t界", "a   \\u754c"],
    ]) assert.equal(renderRows([{ type: "text", text }], options).join(""), expected);
    const text = '{"name":"界"}';
    assert.equal(renderRows([{ type: "text", text }], {
      ...options, serialize: () => { throw new Error("serialization failed"); },
    }).join(""), '{"name":"\\u754c"}');
    const mixed = [
      { type: "image", data: "unchanged", mimeType: "image/png\x1b[2J界\u007f" },
      { type: "text", text: "prose界" },
      { type: "text", text: '{"later":"🙂"}' },
    ];
    const bytes = JSON.stringify(mixed);
    assert.equal(renderRows(mixed, { ...options, showImages: false }).join(""),
      'prose\\u754c{"later":"\\ud83d\\ude42"}[Image: [image/png\\u754c\\u007f]]');
    assert.equal(renderRows(mixed, { ...options, showImages: true }).join(""),
      'prose\\u754c{"later":"\\ud83d\\ude42"}');
    assert.equal(JSON.stringify(mixed), bytes);
  });

  test("preserves an image fallback when Pi image display is disabled", () => {
    assert.deepEqual(
      renderResultRows([{ type: "image", data: "base64", mimeType: "image/png" }], { isError: false, showImages: false, expanded: true, width: 80, expandHint: plainHint }),
      ["[Image: [image/png]]"],
    );
  });

  test("narrow raw and YAML previews keep ten visual body rows, a truthful marker, and every expanded row", () => {
    const rawContent = [{ type: "text", text: Array.from({ length: 12 }, () => "界界").join("\n") }];
    const yamlContent = [{ type: "text", text: JSON.stringify({ rows: Array.from({ length: 12 }, () => "界界") }) }];
    for (const content of [rawContent, yamlContent]) {
      const collapsed = renderResultRows(content, { isError: false, expanded: false, width: 4, expandHint: plainHint });
      const expanded = renderResultRows(content, { isError: false, expanded: true, width: 4, expandHint: plainHint });
      const hidden = expanded.length - 10;
      assert.deepEqual(collapsed.slice(0, 10), expanded.slice(0, 10));
      assert.equal(collapsed.slice(10).join(""), `… ${hidden} more rows (${plainHint})`);
      assert.ok(expanded.length > 10);
      for (const row of [...collapsed, ...expanded]) assert.ok(visibleDisplayWidth(row) <= 4, `${JSON.stringify(row)} is too wide`);
    }
  });

  for (const missingWidth of [false, true]) test(`real bridge keeps YAML renderer and execute bytes with width capability ${missingWidth ? "unavailable" : "available"}`, async () => {
    const directory = await mkdtemp(join(tmpdir(), "ws-pi-bridge-test-"));
    const launcherPath = join(directory, "fake-mcp.py");
    const runtimeJsonPath = join(directory, "runtime.json");
    const responseContent = [
      { type: "text", text: '{ "first": true }' },
      { type: "image", data: "base64", mimeType: "image/png" },
      { type: "text", text: '{"later":"raw🙂"}' },
    ];
    await writeFile(runtimeJsonPath, JSON.stringify({ plugin_version: "test-version" }));
    await writeFile(launcherPath, [
      "import json, sys",
      "for line in sys.stdin:",
      "  request = json.loads(line)",
      "  if request['method'] == 'initialize': result = {'protocolVersion':'2025-03-26','capabilities':{},'serverInfo':{'name':'fake','version':'test-version'}}",
      "  elif request['method'] == 'tools/list': result = {'tools':[{'name':'sample.tool','description':'sample','inputSchema':{'type':'object'}}]}",
      "  else:",
      "    name = request['params']['name']",
      "    content = [{'type':'text','text':'{\\\"session_key\\\":\\\"test-key\\\"}'}] if name == 'ferrule' else [{'type':'text','text':'{ \\\"first\\\": true }'},{'type':'image','data':'base64','mimeType':'image/png'},{'type':'text','text':'{\\\"later\\\":\\\"raw🙂\\\"}'}]",
      "    result = {'content': content}",
      "  print(json.dumps({'jsonrpc':'2.0','id':request['id'],'result':result}), flush=True)",
    ].join("\n"));
    const tools = new Map<string, any>();
    const previousRole = process.env.WS_PI_SPAWN_ROLE;
    process.env.WS_PI_SPAWN_ROLE = "worker";
    try {
      const handle = await startBridge({ registerTool: (tool: any) => tools.set(tool.name, tool) } as any, {
        launcherPath,
        pluginDir: directory,
        runtimeJsonPath,
        cwd: directory,
        resultWidthLoader: missingWidth ? () => loadToolResultWidth(async () => {
          throw new Error("Pi width import unavailable");
        }, () => "missing-nested-pi") : undefined,
      });
      try {
        const tool = tools.get("ws__sample_tool");
        assert.ok(tool?.renderResult, "every bridge-loop registration must carry the shared renderer");
        const result = await tool.execute("call-1", {}, undefined, undefined, undefined);
        assert.deepEqual(result.content, responseContent, "execute must forward the model-visible MCP content unchanged");
        assert.deepEqual(result.details, { content: responseContent });
        const resultBytes = JSON.stringify(result);
        const component = tool.renderResult(result, { expanded: false, isPartial: false }, { fg: (_color: string, text: string) => text }, { isError: false, showImages: true });
        assert.deepEqual(component.render(80), ["first: true", "", missingWidth ? '{"later":"raw\\ud83d\\ude42"}' : '{"later":"raw🙂"}']);
        assert.equal(JSON.stringify(result), resultBytes, "rendering must not mutate content or details");
        const trickyText = "\u{1F3FB}\u0301\u{1F3FB}\u0301";
        const tricky = tool.renderResult({ content: [{ type: "text", text: trickyText }] }, { expanded: true }, { fg: (_color: string, text: string) => text }, { isError: true, showImages: true });
        if (!missingWidth) {
          assert.deepEqual(tricky.render(2), ["?"]);
          assert.deepEqual(tricky.render(4), [trickyText]);
        } else {
          assert.equal(tricky.render(2).join(""), "\\ud83c\\udffb\\u0301\\ud83c\\udffb\\u0301");
          initTheme("dark", false);
          const longValue = "界🙂".repeat(4);
          const escapedValue = "\\u754c\\ud83d\\ude42".repeat(4);
          const content = [
            { type: "text", text: JSON.stringify(Object.fromEntries(Array.from({ length: 12 }, (_, i) => [`field${i}`, longValue]))) },
            { type: "image", data: "base64", mimeType: "image/png" },
            { type: "text", text: '\x1b[2J{"later":"raw JSON"}\x1b]52;c;clipboard\x07' },
          ];
          const displayResult = { content, details: { content, extra: "原🙂" } };
          const bytes = JSON.stringify(displayResult);
          const context = { isError: false, showImages: false };
          // Fallback is plain ASCII even if the theme would add unmeasured text.
          const theme = { fg: (_color: string, text: string) => `\x1b[32m界${text}\x1b[0m` };
          const expected = [
            ...Array.from({ length: 12 }, (_, i) => `field${i}: ${escapedValue}`),
            "", '{"later":"raw JSON"}', "[Image: [image/png]]",
          ];
          for (const width of [-1, 0, 1, 2, 7, 4096]) {
            const expanded = tool.renderResult(displayResult, { expanded: true }, theme, context).render(width);
            const collapsed = tool.renderResult(displayResult, { expanded: false }, theme, context).render(width);
            if (width <= 0) {
              assert.deepEqual(expanded, []);
              assert.deepEqual(collapsed, []);
              continue;
            }
            assert.equal(expanded.join(""), expected.join(""));
            const selectedBody = expected.slice(0, 10).join("");
            assert.ok(collapsed.join("").startsWith(selectedBody));
            assert.match(collapsed.join("").slice(selectedBody.length), /^\\u2026 5 more lines \(.+to expand\)$/);
            assert.ok(!collapsed.join("").includes("field10:"), "select ten logical lines before physical wrapping");
            if (width === 1) assert.ok(collapsed.length > 10, "logical budget does not cap physical rows");
            if (width === 4096) assert.deepEqual(expanded, expected);
            for (const row of [...expanded, ...collapsed]) {
              assert.match(row, /^[\x20-\x7e]*$/, "all final fallback rows are printable ASCII");
              assert.ok(visibleDisplayWidth(row) <= width, "actual Pi validates each fallback row");
            }
          }
          assert.equal(JSON.stringify(displayResult), bytes);
          const errorRows = tool.renderResult(displayResult, { expanded: true }, theme, { ...context, isError: true }).render(4096);
          assert.equal(errorRows[0], content[0].text.replaceAll(longValue, escapedValue), "errors stay raw JSON, with display-only Unicode escapes");
          assert.equal(errorRows[1], '{"later":"raw JSON"}');
        }
      } finally {
        handle.shutdown();
      }
    } finally {
      if (previousRole === undefined) delete process.env.WS_PI_SPAWN_ROLE;
      else process.env.WS_PI_SPAWN_ROLE = previousRole;
      await rm(directory, { recursive: true, force: true });
    }
  });

  test("has no static pi-tui import", async () => {
    for (const path of [new URL("../src/bridge.ts", import.meta.url), new URL("../src/tool-result-render.ts", import.meta.url)]) {
      const source = await readFile(path, "utf8");
      assert.doesNotMatch(source, /^\s*import(?:[\s\S]*?from\s*)?["']@earendil-works\/pi-tui["']/m);
    }
  });
});

describe("maybeAppendModelCatalogAdvisory", () => {
  test("appends the advisory for workflow_manual when piAliasTableUnset is true", () => {
    const content = [{ type: "text", text: "manual body" }];
    const result = maybeAppendModelCatalogAdvisory("workflow_manual", content, { unset: true, rejected: [] });
    assert.equal(result.length, 2);
    assert.deepEqual(result[0], { type: "text", text: "manual body" });
    assert.equal(result[1].type, "text");
    assert.equal(result[1].text, MODEL_CATALOG_ADVISORY);
  });

  test("appends (not prepends) — the advisory is the last item", () => {
    const content = [{ type: "text", text: "first" }, { type: "text", text: "second" }];
    const result = maybeAppendModelCatalogAdvisory("workflow_manual", content, { unset: true, rejected: [] });
    assert.equal(result[result.length - 1].text, MODEL_CATALOG_ADVISORY);
  });

  test("returns a copy — does not mutate the input content array", () => {
    const content = [{ type: "text", text: "manual body" }];
    const contentRef = content;
    const result = maybeAppendModelCatalogAdvisory("workflow_manual", content, { unset: true, rejected: [] });
    assert.equal(content, contentRef, "input array identity must be preserved (not spliced in place)");
    assert.equal(content.length, 1, "input array must not be mutated");
    assert.notEqual(result, content, "must return a fresh array, not the same reference");
  });

  test("does not append when piAliasTableUnset is false", () => {
    const content = [{ type: "text", text: "manual body" }];
    const result = maybeAppendModelCatalogAdvisory("workflow_manual", content, { unset: false, rejected: [] });
    assert.equal(result, content, "content must be returned unchanged (same reference) when a genuine pi tier exists");
    assert.equal(result.length, 1);
  });

  test("does not append for a different tool name, even when piAliasTableUnset is true", () => {
    const content = [{ type: "text", text: "some other tool's body" }];
    const result = maybeAppendModelCatalogAdvisory("playbook.render", content, { unset: true, rejected: [] });
    assert.equal(result, content);
    assert.equal(result.length, 1);
  });

  test("does not append for the sanitized ws__workflow_manual name — only the raw dotted-less name matches", () => {
    const content = [{ type: "text", text: "manual body" }];
    const result = maybeAppendModelCatalogAdvisory("ws__workflow_manual", content, { unset: true, rejected: [] });
    assert.equal(result, content);
  });
});
