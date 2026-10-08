/**
 * Unit tests for display-summary-render.ts and its integration points: the
 * summary-aware tool, message and entry renderers of the lead TUI display
 * summary. TUI modules are duck-typed stand-ins (pi-tui is reached through the
 * host at runtime) except where a Pi built-in or the compaction-history entry
 * brings its own real components; the store is the real one.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { Text, visibleWidth } from "@earendil-works/pi-tui";
import { SessionManager, createEditToolDefinition, createGrepToolDefinition, createLsToolDefinition, type EntryRenderer, type ExtensionAPI, type ExtensionContext } from "@earendil-works/pi-coding-agent";
import { createDisplaySummaryStore, type DisplaySummary } from "../src/display-summary.ts";
import {
  ADAPTER_SUMMARIZED_MESSAGE_TYPES,
  buildSummaryBlock,
  buildSummaryCard,
  isSummaryComponent,
  registerAdapterMessageRenderers,
  registerSummarizedBuiltinTools,
  wrapToolRenderersWithSummary,
  type AdapterMessageTuiModules,
} from "../src/display-summary-render.ts";
import { createToolPreviewTuiRef, registerWsTool, UseNativeResultFallback, type ToolResultTuiModules } from "../src/tool-result-render.ts";
import { registerPushMessageRenderers, type PushTuiModules } from "../src/push-render.ts";
import { buildPushContent } from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { SUMMARY_ID_KEY, withItemSummaryIds, withSummaryId } from "../src/summary-id.ts";
import { COMPACTION_HISTORY_TYPE, registerCompactionHistory } from "../src/compaction-history.ts";
import { NO_KEPT_ENTRY_ID } from "../src/lead-compaction.ts";
import { stripTerminalSequences } from "../src/pi-tui.ts";

// ---------------------------------------------------------------------------
// Stand-ins
// ---------------------------------------------------------------------------

interface FakeComponent {
  render(width: number): string[];
  invalidate(): void;
}

/** Records constructor padding so field indentation is observable. */
class FakeText implements FakeComponent {
  text: string;
  paddingX: number;
  constructor(text = "", paddingX = 0, _paddingY = 0) {
    this.text = text;
    this.paddingX = paddingX;
  }
  setText(text: string): void {
    this.text = text;
  }
  render(_width: number): string[] {
    return this.text.split("\n").map((line) => `${" ".repeat(this.paddingX)}${line}`);
  }
  invalidate(): void {}
}

class FakeBox implements FakeComponent {
  children: FakeComponent[] = [];
  background: ((text: string) => string) | undefined;
  constructor(_paddingX = 0, _paddingY = 0, background?: (text: string) => string) {
    this.background = background;
  }
  addChild(child: FakeComponent): void {
    this.children.push(child);
  }
  setBgFn(background?: (text: string) => string): void {
    this.background = background;
  }
  render(width: number): string[] {
    return this.children.flatMap((child) => child.render(width)).map((line) => this.background?.(line) ?? line);
  }
  invalidate(): void {
    for (const child of this.children) child.invalidate();
  }
}

class FakeContainer implements FakeComponent {
  children: FakeComponent[] = [];
  addChild(child: FakeComponent): void { this.children.push(child); }
  render(width: number): string[] { return this.children.flatMap((child) => child.render(width)); }
  invalidate(): void { for (const child of this.children) child.invalidate(); }
}

class FakeSpacer implements FakeComponent {
  render(): string[] { return [""]; }
  invalidate(): void {}
}

const markdowns: Array<{ text: string; color?: (text: string) => string }> = [];
class FakeMarkdown implements FakeComponent {
  text: string;
  style: { color?: (text: string) => string } | undefined;
  constructor(text: string, _px: number, _py: number, _theme: unknown, style?: { color?: (text: string) => string }) {
    this.text = text;
    this.style = style;
    markdowns.push({ text, color: style?.color });
  }
  render(): string[] { return this.text.split("\n").map((line) => this.style?.color?.(line) ?? line); }
  invalidate(): void {}
}

const modules = {
  Text: FakeText,
  Box: FakeBox,
  Container: FakeContainer,
  Spacer: FakeSpacer,
  Markdown: FakeMarkdown,
  stripTerminalSequences: (text: string) => text,
  truncateToWidth: (text: string) => text,
};
const toolTui = modules as unknown as ToolResultTuiModules;
const pushTui = modules as unknown as PushTuiModules;
const adapterTui = modules as unknown as AdapterMessageTuiModules;

/** Tags every paint so color and weight are visible in rendered text. */
const theme = {
  fg: (color: string, text: string) => `<${color}>${text}</${color}>`,
  bg: (color: string, text: string) => `{${color}}${text}`,
  bold: (text: string) => `<b>${text}</b>`,
};

const summary: DisplaySummary = { optionalContext: "after the auth check", toolIntention: "searched for the token", toolResult: "found two call sites" };
const lean: DisplaySummary = { toolIntention: "read the plan", toolResult: "three phases" };

function toolContext(toolCallId: string, overrides: Record<string, unknown> = {}) {
  const invalidations: string[] = [];
  return {
    invalidations,
    context: {
      toolCallId,
      invalidate: () => { invalidations.push(toolCallId); },
      lastComponent: undefined as unknown,
      state: {},
      argsComplete: true,
      isPartial: false,
      isError: false,
      expanded: false,
      showImages: false,
      cwd: process.cwd(),
      executionStarted: true,
      ...overrides,
    },
  };
}

function text(component: unknown, width = 80): string {
  return (component as FakeComponent).render(width).join("\n");
}

function capturePi() {
  const tools: Array<Record<string, any>> = [];
  const renderers = new Map<string, (message: unknown, options: unknown, theme: unknown) => unknown>();
  return {
    tools,
    renderers,
    pi: {
      registerTool: (definition: Record<string, any>) => { tools.push(definition); },
      registerMessageRenderer: (customType: string, renderer: (message: unknown, options: unknown, theme: unknown) => unknown) => { renderers.set(customType, renderer); },
    },
  };
}

// ---------------------------------------------------------------------------
// Shared summary layout
// ---------------------------------------------------------------------------

describe("shared summary layout", () => {
  test("blocks and cards use exactly one blank row after the title and before the result", () => {
    const expected = ["title", "", "    <text>read the plan</text>", "", "<muted>three phases</muted>"];
    assert.deepEqual(buildSummaryBlock(modules, "title", lean, theme).render(80), expected);
    assert.deepEqual(buildSummaryCard(modules, "title", lean, theme).render(80), expected.map((line) => `{customMessageBg}${line}`));
  });

  test("real Text wraps every intention line at four spaces and leaves muted result lines unindented", () => {
    const long: DisplaySummary = {
      toolIntention: "Inspect several long inputs 한글 😀 for the summary\nThen verify wrapping",
      toolResult: "Found several long outputs 한글 😀 in the summary\nAll checks complete",
    };
    for (const width of [12, 20, 40]) {
      const tokens: string[] = [];
      const ansiTheme = { fg: (token: string, value: string) => {
        tokens.push(token);
        return `${token === "text" ? "\x1b[97m" : "\x1b[90m"}${value}\x1b[0m`;
      } };
      const lines = buildSummaryBlock({ Text }, "title", long, ansiTheme).render(width);
      const plain = lines.map((line) => stripTerminalSequences(line).trimEnd());
      assert.deepEqual(tokens, ["text", "muted"], "foreground and result use separate semantic tokens");
      assert.equal(plain[1], "", "one blank line after the title");
      // Text may emit whitespace-only wrapped lines at narrow widths;
      // distinguish those styled lines from the shared unstyled spacer.
      const separator = lines.indexOf("", 2);
      assert.ok(separator > 2);
      const intention = lines.slice(2, separator);
      const result = lines.slice(separator + 1);
      assert.ok(intention.length > 1 && result.length > 1, "both fields wrapped");
      for (const line of intention) {
        assert.match(stripTerminalSequences(line), /^ {4}/);
        assert.ok(line.includes("\x1b[97m"), "wrapped intention retains normal foreground");
      }
      for (const line of result) {
        if (stripTerminalSequences(line).trim()) assert.match(stripTerminalSequences(line), /^\S/);
        assert.ok(line.includes("\x1b[90m"), "wrapped result retains muted foreground");
      }
      for (const line of lines) assert.ok(visibleWidth(line) <= width, `fits ${width} columns`);
      assert.equal(lines.filter((line) => line === "").length, 2, "no extra blank margins");
    }
    // Text reduces its margins when four spaces cannot fit, rather than
    // violating the terminal width contract on very narrow viewports.
    for (let width = 1; width <= 9; width++) {
      const lines = buildSummaryBlock({ Text }, "title", lean, undefined).render(width);
      for (const line of lines) assert.ok(visibleWidth(line) <= width, `fits narrow ${width} columns`);
    }
  });
});

// ---------------------------------------------------------------------------
// Tool rows
// ---------------------------------------------------------------------------

describe("registerWsTool tool rows", () => {
  function registered() {
    const store = createDisplaySummaryStore();
    const ref = createToolPreviewTuiRef();
    ref.current = toolTui;
    ref.summaries = store;
    const { pi, tools } = capturePi();
    registerWsTool(pi as never, { name: "ws__tickets_query", label: "q", description: "d", parameters: {}, execute: async () => ({ content: [] }) } as never, ref);
    return { store, ref, tool: tools[0]! };
  }
  const result = { content: [{ type: "text", text: "{\"hits\":2}" }] };

  test("collapsed rows show the raw preview before a summary and the summary after it", () => {
    const { store, tool } = registered();
    assert.ok(store.toolNames.has("ws__tickets_query"), "the tool's rows are queued for summaries");
    const { context, invalidations } = toolContext("call-1");

    const rawCall = tool.renderCall({ query: "auth" }, theme, context);
    const rawResult = tool.renderResult(result, { expanded: false, isPartial: false }, theme, context);
    assert.match(text(rawCall), /query: auth/);
    assert.match(text(rawResult), /hits: 2/);

    store.set("call-1", summary);
    store.notify(["call-1"]);
    assert.deepEqual(invalidations, ["call-1"], "the row's invalidate handle was recorded by its renderer");

    const call = tool.renderCall({ query: "auth" }, theme, { ...context, lastComponent: rawCall });
    const fields = tool.renderResult(result, { expanded: false, isPartial: false }, theme, { ...context, lastComponent: rawResult });
    assert.equal(text(call), "<toolTitle><b>ws__tickets_query</b></toolTitle>", "the call slot is the bold tool-name header only");
    assert.doesNotMatch(text(call), /query: auth/, "the raw argument preview is hidden once summarized");
    assert.deepEqual((fields as FakeComponent).render(80), [
      "",
      "  <dim>after the auth check</dim>",
      "    <text>searched for the token</text>",
      "",
      "<muted>found two call sites</muted>",
    ], "blank rows separate the header and fields; context stays dim, intention is indented normal text, result is unindented muted");
  });

  test("expanded rows are always raw", () => {
    const { store, tool } = registered();
    store.set("call-2", summary);
    const { context } = toolContext("call-2", { expanded: true });
    assert.match(text(tool.renderCall({ query: "auth" }, theme, context)), /query: auth/);
    assert.match(text(tool.renderResult(result, { expanded: true, isPartial: false }, theme, context)), /hits: 2/);
  });

  test("a summary omits an absent optionalContext", () => {
    const { store, tool } = registered();
    store.set("call-3", lean);
    const { context } = toolContext("call-3");
    assert.deepEqual((tool.renderResult(result, { expanded: false, isPartial: false }, theme, context) as FakeComponent).render(80), ["", "    <text>read the plan</text>", "", "<muted>three phases</muted>"]);
  });

  test("a tool with its own renderers is wrapped too, and an absent slot keeps Pi's native fallback", () => {
    const store = createDisplaySummaryStore();
    const ref = createToolPreviewTuiRef();
    ref.current = toolTui;
    ref.summaries = store;
    const { pi, tools } = capturePi();
    const own = () => new FakeText("OWN CALL");
    registerWsTool(pi as never, { name: "explore", label: "e", description: "d", parameters: {}, execute: async () => ({ content: [] }), renderCall: own } as never, ref);
    assert.ok(store.toolNames.has("explore"));
    const { context } = toolContext("call-4");
    assert.equal(text(tools[0]!.renderCall({}, theme, context)), "OWN CALL");
    assert.throws(() => tools[0]!.renderResult(result, { expanded: false, isPartial: false }, theme, context), UseNativeResultFallback);
    store.set("call-4", lean);
    assert.equal(text(tools[0]!.renderCall({}, theme, context)), "<toolTitle><b>explore</b></toolTitle>");
    assert.match(text(tools[0]!.renderResult(result, { expanded: false, isPartial: false }, theme, context)), /three phases/);
  });

  test("with no store attached the row renders as today", () => {
    const ref = createToolPreviewTuiRef();
    ref.current = toolTui;
    const { pi, tools } = capturePi();
    registerWsTool(pi as never, { name: "ws__x", label: "x", description: "d", parameters: {}, execute: async () => ({ content: [] }) } as never, ref);
    const { context } = toolContext("call-5");
    assert.match(text(tools[0]!.renderCall({ a: 1 }, theme, context)), /a: 1/, "no store: today's rendering");
  });
});

describe("wrapToolRenderersWithSummary", () => {
  test("switching back to raw never hands a summary component to the inner renderer as lastComponent", () => {
    const store = createDisplaySummaryStore();
    const seen: unknown[] = [];
    const inner = (_args: unknown, _theme: unknown, context: unknown) => {
      seen.push((context as { lastComponent?: unknown }).lastComponent);
      return new FakeText("RAW");
    };
    const wrapped = wrapToolRenderersWithSummary("grep", inner, undefined, store, toolTui);
    store.set("c", lean);
    const { context } = toolContext("c");
    const summaryCall = wrapped.renderCall({}, theme, context);
    assert.ok(isSummaryComponent(summaryCall));
    const raw = wrapped.renderCall({}, theme, { ...context, expanded: true, lastComponent: summaryCall });
    assert.equal(seen[0], undefined, "the summary component was replaced by undefined");
    const keep = new FakeText("prior raw");
    wrapped.renderCall({}, theme, { ...context, expanded: true, lastComponent: keep });
    assert.equal(seen[1], keep, "a raw lastComponent is passed through untouched");
    assert.equal(text(raw), "RAW");
  });

  test("with no store the inner renderer gets the very same context object", () => {
    let received: unknown;
    const wrapped = wrapToolRenderersWithSummary("x", (_a, _t, context) => { received = context; return new FakeText(""); }, undefined, undefined, toolTui);
    const { context } = toolContext("c");
    wrapped.renderCall({}, theme, context);
    assert.equal(received, context);
  });
});

describe("registerSummarizedBuiltinTools", () => {
  const cwd = process.cwd();

  test("registers only the active summarized built-ins, unchanged in name, description and parameters", () => {
    const store = createDisplaySummaryStore();
    const { pi, tools } = capturePi();
    const names = registerSummarizedBuiltinTools(pi as never, cwd, store, ["read", "bash", "edit", "grep", "ls"], toolTui);
    assert.deepEqual(names, ["edit", "grep", "ls"]);
    assert.deepEqual(tools.map((tool) => tool.name), ["edit", "grep", "ls"]);
    assert.deepEqual([...store.toolNames].sort(), ["edit", "grep", "ls"]);
    const natives = [createEditToolDefinition(cwd), createGrepToolDefinition(cwd), createLsToolDefinition(cwd)];
    for (const [index, native] of natives.entries()) {
      assert.equal(tools[index]!.name, native.name);
      assert.equal(tools[index]!.description, native.description);
      assert.deepEqual(tools[index]!.parameters, native.parameters);
      assert.equal(tools[index]!.renderShell, (native as { renderShell?: unknown }).renderShell);
      assert.equal(typeof tools[index]!.execute, "function");
    }
  });

  test("delegates to the native renderer when expanded or unsummarized, and survives the switch back", async () => {
    const themeModule = await import(new URL("./modes/interactive/theme/theme.js", import.meta.resolve("@earendil-works/pi-coding-agent")).href);
    themeModule.initTheme();
    const plainTheme = { fg: (_c: string, t: string) => t, bg: (_c: string, t: string) => t, bold: (t: string) => t };
    const store = createDisplaySummaryStore();
    const { pi, tools } = capturePi();
    registerSummarizedBuiltinTools(pi as never, cwd, store, ["grep"], toolTui);
    const native = createGrepToolDefinition(cwd);
    const args = { pattern: "needle", path: "src" };
    const grepResult = { content: [{ type: "text", text: "src/a.ts:1: needle" }] };
    const render = (component: unknown) => (component as FakeComponent).render(80).map((line) => stripTerminalSequences(line).trimEnd()).join("\n");

    const nativeCall = render(native.renderCall!(args as never, plainTheme as never, toolContext("g").context as never));
    const nativeResult = render(native.renderResult!(grepResult as never, { expanded: false, isPartial: false }, plainTheme as never, toolContext("g").context as never));
    assert.equal(render(tools[0]!.renderCall(args, plainTheme, toolContext("g").context)), nativeCall, "no summary: native call row");
    assert.equal(render(tools[0]!.renderResult(grepResult, { expanded: false, isPartial: false }, plainTheme, toolContext("g").context)), nativeResult, "no summary: native result row");

    store.set("g", lean);
    const { context } = toolContext("g");
    const summaryCall = tools[0]!.renderCall(args, plainTheme, context);
    const summaryResult = tools[0]!.renderResult(grepResult, { expanded: false, isPartial: false }, plainTheme, context);
    assert.equal(text(summaryCall), "grep");
    assert.match(text(summaryResult), /read the plan/);

    // Native grep calls setText on lastComponent: a summary component handed back would throw.
    const expandedCall = tools[0]!.renderCall(args, plainTheme, { ...context, expanded: true, lastComponent: summaryCall });
    const expandedResult = tools[0]!.renderResult(grepResult, { expanded: true, isPartial: false }, plainTheme, { ...context, expanded: true, lastComponent: summaryResult });
    assert.equal(render(expandedCall), nativeCall, "expanded: native call row");
    assert.match(render(expandedResult), /needle/, "expanded: native result row");
  });

  test("a self-framed built-in (edit) indents its summary one column", () => {
    const store = createDisplaySummaryStore();
    const { pi, tools } = capturePi();
    registerSummarizedBuiltinTools(pi as never, cwd, store, ["edit"], toolTui);
    store.set("e", lean);
    const { context } = toolContext("e");
    assert.equal(text(tools[0]!.renderCall({}, theme, context)), " <toolTitle><b>edit</b></toolTitle>");
    assert.deepEqual((tools[0]!.renderResult({ content: [] }, { expanded: false, isPartial: false }, theme, context) as FakeComponent).render(80), ["", "     <text>read the plan</text>", "", " <muted>three phases</muted>"]);
  });
});

// ---------------------------------------------------------------------------
// Message rows
// ---------------------------------------------------------------------------

describe("push message rows", () => {
  test("a collapsed push row reads the store at render time: raw, then the summary without a rebuild", async () => {
    const store = createDisplaySummaryStore();
    const { pi, renderers } = capturePi();
    await registerPushMessageRenderers(pi as never, pushTui, store);
    const details = withSummaryId({ agent_id: "a1" });
    const message = { content: buildPushContent("ws-agent-report", "scout (a1)", { report: "Outcome: done" }, undefined), details };
    const component = renderers.get("ws-agent-report")!(message, { expanded: false }, theme) as FakeComponent;
    assert.match(text(component), /report: Outcome: done/);

    store.set(details[SUMMARY_ID_KEY] as string, summary);
    const lines = component.render(80);
    assert.equal(lines[0], "{customMessageBg}<customMessageLabel><b>scout · report</b></customMessageLabel>", "bold human head on the push card background");
    assert.deepEqual(lines.slice(1), [
      "{customMessageBg}",
      "{customMessageBg}  <dim>after the auth check</dim>",
      "{customMessageBg}    <text>searched for the token</text>",
      "{customMessageBg}",
      "{customMessageBg}<muted>found two call sites</muted>",
    ]);
    assert.doesNotMatch(lines.join("\n"), /Outcome: done/);

    const expanded = renderers.get("ws-agent-report")!(message, { expanded: true }, theme) as FakeComponent;
    assert.match(text(expanded), /report: Outcome: done/, "expanded is raw");
  });

  test("without a store the push card is today's card", async () => {
    const { pi, renderers } = capturePi();
    await registerPushMessageRenderers(pi as never, pushTui);
    const message = { content: buildPushContent("ws-agent-settled", "a1", { reason: "idle" }, undefined), details: withSummaryId({}) };
    const component = renderers.get("ws-agent-settled")!(message, { expanded: false }, undefined);
    assert.ok(component instanceof FakeBox, "the raw box itself, not a switch");
  });

  test("a ws-push-batch shows each item card's own summary", async () => {
    const store = createDisplaySummaryStore();
    const { pi, renderers } = capturePi();
    await registerPushMessageRenderers(pi as never, pushTui, store);
    const items = withItemSummaryIds([
      { customType: "ws-agent-report", content: buildPushContent("ws-agent-report", "w1", { report: "first raw" }, undefined), display: true, details: { agent_id: "w1" }, state: "informational" },
      { customType: "ws-mailbox", content: "mail raw body", display: true, details: { from: "scout" }, state: "informational" },
      { customType: "ws-agent-settled", content: buildPushContent("ws-agent-settled", "w2", { reason: "idle" }, undefined), display: true, details: { agent_id: "w2" }, state: "informational" },
    ]);
    const component = renderers.get(PUSH_BATCH_CUSTOM_TYPE)!({ details: { version: 1, items } }, { expanded: false }, theme) as FakeComponent;
    const before = text(component);
    assert.match(before, /first raw/);
    assert.match(before, /mail raw body/);

    store.set(items[0]!.details[SUMMARY_ID_KEY] as string, { toolIntention: "w1 reported", toolResult: "first summary" });
    store.set(items[1]!.details[SUMMARY_ID_KEY] as string, { toolIntention: "scout mailed", toolResult: "mail summary" });
    const after = text(component);
    assert.match(after, /first summary/);
    assert.match(after, /<b>Mailbox<\/b>/, "a non-push item gets a readable kind header");
    assert.match(after, /mail summary/);
    assert.doesNotMatch(after, /first raw|mail raw body/);
    assert.match(after, /reason: idle/, "an unsummarized item keeps its raw card");
  });
});

describe("adapter message renderers", () => {
  test("registers the three formerly default-rendered types; raw mode replicates Pi's default custom message", async () => {
    const store = createDisplaySummaryStore();
    const { pi, renderers } = capturePi();
    await registerAdapterMessageRenderers(pi as never, store, adapterTui);
    assert.deepEqual([...renderers.keys()], ["ws-lead-compact", "ws-lead-context-milestone", "ws-thread-summary"]);
    assert.deepEqual([...ADAPTER_SUMMARIZED_MESSAGE_TYPES], [...renderers.keys()]);

    for (const customType of ADAPTER_SUMMARIZED_MESSAGE_TYPES) {
      markdowns.length = 0;
      const details = withSummaryId({});
      const message = { customType, content: [{ type: "text", text: "line one" }, { type: "image" }, { type: "text", text: "line two" }], details };
      const component = renderers.get(customType)!(message, { expanded: false }, theme) as FakeComponent;
      assert.deepEqual(component.render(80), [
        `{customMessageBg}<customMessageLabel>\x1b[1m[${customType}]\x1b[22m</customMessageLabel>`,
        "{customMessageBg}",
        "{customMessageBg}<customMessageText>line one</customMessageText>",
        "{customMessageBg}<customMessageText>line two</customMessageText>",
      ]);
      assert.equal(markdowns[0]!.text, "line one\nline two");

      store.set(details[SUMMARY_ID_KEY] as string, lean);
      const lines = component.render(80);
      assert.match(lines[0]!, /^\{customMessageBg\}<customMessageLabel><b>[A-Z][a-z]+ [a-z]+<\/b><\/customMessageLabel>$/, "a short readable kind header");
      assert.deepEqual(lines.slice(1), ["{customMessageBg}", "{customMessageBg}    <text>read the plan</text>", "{customMessageBg}", "{customMessageBg}<muted>three phases</muted>"]);

      const expanded = renderers.get(customType)!({ ...message, content: "raw text" }, { expanded: true }, theme) as FakeComponent;
      assert.match(text(expanded), /raw text/, "expanded is raw");
    }
  });
});

// ---------------------------------------------------------------------------
// Compaction-history entry
// ---------------------------------------------------------------------------

describe("compaction-history entry", () => {
  test("the appended entry reports its id and text, and its row switches to the summary by entry id", async () => {
    const themeModule = await import(new URL("./modes/interactive/theme/theme.js", import.meta.resolve("@earendil-works/pi-coding-agent")).href);
    themeModule.initTheme();
    const store = createDisplaySummaryStore();
    const sm = SessionManager.inMemory();
    const handlers = new Map<string, (event: any, ctx: ExtensionContext) => unknown>();
    let renderer: EntryRenderer<any> | undefined;
    const appended: Array<{ id: string; text: string }> = [];
    const pi = {
      on: (event: string, handler: any) => handlers.set(event, handler),
      registerEntryRenderer: (_type: string, render: EntryRenderer<any>) => { renderer = render; },
      appendEntry: (type: string, data: unknown) => { sm.appendCustomEntry(type, data); },
    } as unknown as ExtensionAPI;
    registerCompactionHistory(pi, { summaries: store, onAppended: (id, text) => appended.push({ id, text }) });
    const ctx = { sessionManager: sm } as unknown as ExtensionContext;
    sm.appendMessage({ role: "user", content: "fix the auth bug", timestamp: 1 });
    sm.appendMessage({ role: "assistant", content: [{ type: "text", text: "Fixed it." }], timestamp: 2 } as never);
    const compactionId = sm.appendCompaction("summary", NO_KEPT_ENTRY_ID, 99, { kind: "ws-pi-lead-compaction" }, true);
    handlers.get("session_compact")!({ compactionEntry: sm.getEntry(compactionId), reason: "manual", fromExtension: true }, ctx);

    const entry = sm.getBranch().find((e) => e.type === "custom" && e.customType === COMPACTION_HISTORY_TYPE)!;
    assert.deepEqual(appended, [{ id: entry.id, text: "User: fix the auth bug\nAssistant: Fixed it." }]);

    const plainTheme = { fg: (_c: string, t: string) => t, bg: (_c: string, t: string) => t, bold: (t: string) => `<b>${t}</b>` };
    const component = renderer!(entry as never, { expanded: false }, plainTheme as never)!;
    const raw = component.render(80).map((line) => stripTerminalSequences(line).trimEnd()).join("\n");
    assert.match(raw, /Previous conversation · display-only/);
    assert.match(raw, /fix the auth bug/);

    store.set(entry.id, summary);
    assert.deepEqual(component.render(80).map((line) => line.trimEnd()), [
      "<b>Previous conversation</b>",
      "",
      "  after the auth check",
      "    searched for the token",
      "",
      "found two call sites",
    ], "summary block on the same component, read at render time");

    const expanded = renderer!(entry as never, { expanded: true }, plainTheme as never)!;
    assert.match(expanded.render(80).map((line) => stripTerminalSequences(line)).join("\n"), /fix the auth bug/, "expanded is raw");
  });
});
