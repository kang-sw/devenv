import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  approximateCodePointWidth,
  completedTextPreview,
  createToolPreviewRenderers,
  createToolPreviewTuiRef,
  physicalPreview,
  registerWsTool,
  sanitizePreviewText,
  UseNativeResultFallback,
  yamlContainerDisplay,
  yamlInputPreview,
  type ToolResultTuiModules,
} from "../src/tool-result-render.ts";

class FakeText {
  text = "";
  setTextCalls = 0;
  layoutCalls = 0;
  private width: number | undefined;
  private lines: string[] | undefined;

  setText(text: string): void {
    this.text = text;
    this.setTextCalls += 1;
    this.width = undefined;
    this.lines = undefined;
  }

  render(width: number): string[] {
    if (this.width !== width || !this.lines) {
      this.width = width;
      this.lines = this.text.split("\n");
      this.layoutCalls += 1;
    }
    return this.lines;
  }

  invalidate(): void {
    this.width = undefined;
    this.lines = undefined;
  }
}

class FakeBox {
  children: Array<{ render(width: number): string[]; invalidate(): void }> = [];
  background: ((text: string) => string) | undefined;

  constructor(_paddingX = 0, _paddingY = 0, background?: (text: string) => string) {
    this.background = background;
  }

  addChild(component: { render(width: number): string[]; invalidate(): void }): void {
    this.children.push(component);
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

function fakeTui(): {
  tui: ToolResultTuiModules;
  strips: () => number;
  texts: FakeText[];
  boxes: FakeBox[];
  truncations: () => number;
  layouts: () => number;
  joins: () => number;
  styles: () => number;
} {
  let stripCalls = 0;
  let truncationCalls = 0;
  let physicalLayouts = 0;
  let joins = 0;
  let styles = 0;
  const texts: FakeText[] = [];
  const boxes: FakeBox[] = [];
  class CapturedText extends FakeText {
    constructor() {
      super();
      texts.push(this);
    }
  }
  class CapturedBox extends FakeBox {
    constructor(paddingX = 0, paddingY = 0, background?: (text: string) => string) {
      super(paddingX, paddingY, background);
      boxes.push(this);
    }
  }
  return {
    tui: {
      Text: CapturedText,
      Box: CapturedBox,
      stripTerminalSequences(text: string): string {
        stripCalls += 1;
        return text.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "");
      },
      truncateToWidth(text: string, _width: number): string {
        truncationCalls += 1;
        return text;
      },
      onPreviewLayout(): void {
        physicalLayouts += 1;
      },
      onPreviewJoin(): void {
        joins += 1;
      },
      onPreviewStyle(): void {
        styles += 1;
      },
    },
    strips: () => stripCalls,
    texts,
    boxes,
    truncations: () => truncationCalls,
    layouts: () => physicalLayouts,
    joins: () => joins,
    styles: () => styles,
  };
}

function fakeTheme(id = "one") {
  const fgCalls: Array<{ color: string; text: string }> = [];
  const bgCalls: Array<{ color: string; text: string }> = [];
  const boldCalls: string[] = [];
  return {
    fg(color: string, text: string): string {
      fgCalls.push({ color, text });
      return `<${id}:fg:${color}>${text}</${id}:fg>`;
    },
    bg(color: string, text: string): string {
      bgCalls.push({ color, text });
      return `<${id}:bg:${color}>${text}</${id}:bg>`;
    },
    bold(text: string): string {
      boldCalls.push(text);
      return `<${id}:bold>${text}</${id}:bold>`;
    },
    fgCalls,
    bgCalls,
    boldCalls,
  };
}

function plain(text: string): string {
  return text.replace(/<[^>]+>/g, "");
}

function context(overrides: Partial<{ state: object; lastComponent: unknown; argsComplete: boolean; isPartial: boolean; isError: boolean }> = {}) {
  return {
    state: overrides.state ?? {},
    lastComponent: overrides.lastComponent,
    argsComplete: overrides.argsComplete ?? true,
    isPartial: overrides.isPartial ?? false,
    isError: overrides.isError ?? false,
  };
}

const unstyledTheme = {
  fg: (_color: string, text: string) => text,
  bg: (_color: string, text: string) => text,
  bold: (text: string) => text,
};

describe("bounded YAML preview preparation", () => {
  test("uses accepted conservative code-point widths", () => {
    assert.equal(approximateCodePointWidth("a"), 1);
    assert.equal(approximateCodePointWidth("界"), 2);
    assert.equal(approximateCodePointWidth("👩"), 2);
    assert.equal(approximateCodePointWidth("\u0301"), 2, "combining marks deliberately wrap early");
  });

  test("normalizes terminal controls and tabs before row-width accounting", () => {
    assert.equal(sanitizePreviewText("a\t\u0007b\r\nc"), "a    ?b\nc");
    assert.deepEqual(
      physicalPreview(sanitizePreviewText("a\tbc"), 8, { expanded: false, trimOuterWhitespace: false }),
      ["    a   ", "    bc"],
    );
  });

  test("trims only outer input whitespace and indents starts one column beyond continuations", () => {
    assert.deepEqual(
      physicalPreview("  first  \nsecond\n  ", 9, { expanded: false, trimOuterWhitespace: true }),
      ["    first", "     ", "    secon", "   d"],
    );
  });

  test("is safe at zero and narrow widths while conservatively splitting emoji sequences", () => {
    assert.deepEqual(physicalPreview("", 0, { expanded: false, trimOuterWhitespace: false }), [""]);
    assert.deepEqual(physicalPreview("wide", 3, { expanded: false, trimOuterWhitespace: false }), ["  w", "  i", "  d", "  e"]);
    assert.deepEqual(
      physicalPreview("👩‍💻", 8, { expanded: false, trimOuterWhitespace: false }),
      ["    👩‍", "   💻"],
    );
  });

  test("preserves expanded narrow output and fits the truncation marker to one row", () => {
    assert.deepEqual(
      physicalPreview("界\nTAIL", 5, { expanded: true, trimOuterWhitespace: false }),
      ["   界", "    T", "   AI", "   L"],
    );
    const collapsed = physicalPreview("abcdefghijkl", 1, { expanded: false, trimOuterWhitespace: false });
    assert.equal(collapsed.length, 11);
    assert.equal(collapsed.at(-1), ".", "narrow marker stays one terminal-safe row");
  });

  test("caps collapsed physical content at ten rows with a separate marker", () => {
    const ten = Array.from({ length: 10 }, (_, index) => `line-${index}`).join("\n");
    const eleven = `${ten}\nline-10`;
    assert.equal(physicalPreview(ten, 80, { expanded: false, trimOuterWhitespace: false }).length, 10);
    assert.deepEqual(
      physicalPreview(eleven, 80, { expanded: false, trimOuterWhitespace: false }).slice(-2),
      ["    line-9", "..."],
    );
  });

  test("indents input markers while retaining narrow one-row fitting", () => {
    const source = Array.from({ length: 11 }, (_, index) => `line-${index}`).join("\n");
    assert.equal(
      physicalPreview(source, 80, { expanded: false, trimOuterWhitespace: true, markerIndent: 4 }).at(-1),
      "    ...",
    );
    assert.equal(
      physicalPreview(source, 5, { expanded: false, trimOuterWhitespace: true, markerIndent: 4 }).at(-1),
      "    .",
    );
    assert.equal(
      physicalPreview(source, 3, { expanded: false, trimOuterWhitespace: true, markerIndent: 4 }).at(-1),
      "  .",
    );
  });

  test("keeps serializing containers and leaves scalar JSON and prose native", () => {
    assert.match(yamlContainerDisplay('{"task":"render","count":2}') ?? "", /task: render/);
    assert.equal(yamlContainerDisplay('"plain string"'), undefined);
    assert.equal(yamlContainerDisplay("not json"), undefined);
    assert.match(yamlInputPreview({ nested: { count: 2 } }), /nested:/);
    assert.equal(yamlInputPreview(undefined), "");
    assert.equal(yamlInputPreview(["not", "tool", "arguments"]), "");
  });
});

describe("native YAML preview renderers", () => {
  test("keeps bold tool identity, white input, gray output, native backgrounds, and exact blank rows", () => {
    const { tui, texts, boxes } = fakeTui();
    const theme = fakeTheme();
    const renderers = createToolPreviewRenderers(tui, "ws__git_status", (value) =>
      "value" in value ? "\nfirst\nsecond\n" : "ok: true",
    );
    const state = {};
    const call = renderers.renderCall({ value: "ignored" }, theme, context({ state }));
    const result = renderers.renderResult(
      { content: [{ type: "text", text: '{"ok":true}' }] },
      { expanded: false, isPartial: false },
      theme,
      context({ state }),
    );

    const rows = [...call.render(30), ...result.render(30)].map(plain);
    assert.deepEqual(rows, ["ws__git_status", "", "    first", "    second", "", "    ok: true"]);
    assert.deepEqual(theme.boldCalls, ["ws__git_status"]);
    assert.ok(theme.fgCalls.some((call) => call.color === "text" && call.text.includes("first")), "input uses the theme default foreground for dark/light readability");
    assert.ok(theme.fgCalls.some((call) => call.color === "toolOutput" && call.text.includes("ok: true")), "output stays gray through toolOutput");
    assert.equal(boxes[0]!.background, undefined, "input inherits the native parent lifecycle background");
    assert.equal(boxes[1]!.background, undefined, "output inherits the native parent lifecycle background");
    assert.equal(theme.bgCalls.length, 0, "renderer installs no nested background callbacks");
    assert.equal(texts.length, 3);
  });

  test("styles the indented input marker gray and reuses its bounded layout", () => {
    const { tui, layouts } = fakeTui();
    const theme = fakeTheme();
    const renderers = createToolPreviewRenderers(tui, "ws__test", () => "x".repeat(200_000));
    const input = renderers.renderCall({ value: "ignored" }, theme, context());
    const first = input.render(80).map(plain);
    const initialLayouts = layouts();
    input.render(80);

    assert.equal(first.at(-2), "    ...");
    assert.ok(theme.fgCalls.some((call) => call.color === "toolOutput" && call.text === "    ..."));
    assert.equal(layouts(), initialLayouts, "unchanged input redraw reuses the bounded physical layout");
  });

  test("wraps long logical rows before the ten-row budget and expands full output", () => {
    const { tui } = fakeTui();
    const renderers = createToolPreviewRenderers(tui, "ws__test", () => "abcdefghijklmno");
    const content = [{ type: "text", text: '{"ok":true}' }];
    const state = {};
    const call = renderers.renderCall({ value: "ignored" }, unstyledTheme, context({ state }));
    assert.deepEqual(call.render(10), ["ws__test", "", "    abcdef", "   ghijklm", "   no", ""]);

    const serializer = (_value: object) => Array.from({ length: 11 }, (_, index) => `line-${index}`).join("\n");
    const capped = createToolPreviewRenderers(tui, "ws__test", serializer);
    const collapsed = capped.renderResult({ content }, { expanded: false, isPartial: false }, unstyledTheme, context({ state: {} }));
    assert.deepEqual(collapsed.render(80).slice(-2), ["    line-9", "..."]);
    const expanded = capped.renderResult({ content }, { expanded: true, isPartial: false }, unstyledTheme, context({ state: {}, lastComponent: collapsed }));
    assert.equal(expanded.render(80).filter((line) => line.includes("line-")).length, 11);
  });

  test("preserves input/result payload identity while caching preparation and reusing native layout", () => {
    const { tui, texts, strips, truncations } = fakeTui();
    const theme = fakeTheme();
    let serializations = 0;
    const serialize = (value: object) => {
      serializations += 1;
      return `value: ${(value as { value?: string }).value ?? "true"}`;
    };
    const renderers = createToolPreviewRenderers(tui, "ws__test", serialize);
    const args = { value: "large payload" };
    const content = [{ type: "text", text: '{"value":"unchanged"}' }];
    const state = {};
    const first = renderers.renderCall(args, theme, context({ state }));
    first.render(24);
    const second = renderers.renderCall(args, theme, context({ state, lastComponent: first }));
    second.render(24);
    assert.equal(second, first);
    assert.equal(serializations, 1);
    assert.equal(texts[1]!.layoutCalls, 1, "unchanged width stays in the native Text cache");

    const output = renderers.renderResult({ content }, { expanded: false, isPartial: false }, theme, context({ state }));
    output.render(24);
    const expanded = renderers.renderResult({ content }, { expanded: true, isPartial: false }, theme, context({ state, lastComponent: output }));
    expanded.render(24);
    assert.equal(serializations, 2, "one input and one output serialization; expansion reuses YAML");
    assert.deepEqual(args, { value: "large payload" });
    assert.deepEqual(content, [{ type: "text", text: '{"value":"unchanged"}' }]);
    assert.equal(strips(), 3, "title, input, and output sanitize once each");
    assert.ok(truncations() > 0, "native terminal-safe final fitting remains active");
  });

  test("caches physical layout separately from native layout and theme styling", () => {
    const { tui, layouts } = fakeTui();
    const renderers = createToolPreviewRenderers(tui, "ws__test", (value) =>
      "changed" in value ? "changed" : "x".repeat(200_000),
    );
    const state = {};
    const content = [{ type: "text", text: "{}" }];
    const output = renderers.renderResult({ content }, { expanded: true, isPartial: false }, unstyledTheme, context({ state }));
    output.render(80);
    output.render(80);
    assert.equal(layouts(), 1, "unchanged expanded redraw reuses its physical layout");

    output.render(79);
    assert.equal(layouts(), 2, "width changes rebuild layout");
    const collapsed = renderers.renderResult({ content }, { expanded: false, isPartial: false }, unstyledTheme, context({ state, lastComponent: output }));
    collapsed.render(79);
    assert.equal(layouts(), 3, "expansion changes rebuild layout");
    const changed = renderers.renderResult(
      { content: [{ type: "text", text: '{"changed":true}' }] },
      { expanded: false, isPartial: false },
      unstyledTheme,
      context({ state, lastComponent: collapsed }),
    );
    changed.render(79);
    assert.equal(layouts(), 4, "source changes rebuild layout");
  });

  test("caches large expanded RAW joins and styling until layout or theme invalidates", () => {
    const { tui, layouts, joins, styles } = fakeTui();
    const renderers = createToolPreviewRenderers(tui, "ws__test");
    const state = {};
    const raw = "x".repeat(2_250_000);
    const firstTheme = fakeTheme("first");
    const content = [{ type: "text", text: raw }];
    const output = renderers.renderResult({ content }, { expanded: true, isPartial: false }, firstTheme, context({ state }));
    output.render(80);
    for (let i = 0; i < 100; i += 1) output.render(80);
    assert.deepEqual({ layouts: layouts(), joins: joins(), styles: styles() }, { layouts: 1, joins: 1, styles: 1 }, "unchanged RAW redraws must not rebuild source-sized strings or colors");

    const secondTheme = fakeTheme("second");
    const themed = renderers.renderResult({ content }, { expanded: true, isPartial: false }, secondTheme, context({ state, lastComponent: output }));
    themed.render(80);
    assert.deepEqual({ layouts: layouts(), joins: joins(), styles: styles() }, { layouts: 1, joins: 1, styles: 2 }, "theme changes restyle cached plain output without rewrapping or joining");
    themed.render(79);
    assert.deepEqual({ layouts: layouts(), joins: joins(), styles: styles() }, { layouts: 2, joins: 2, styles: 3 }, "width changes invalidate layout, joined text, and display");
  });

  test("reprepares in-place streamed arguments and rebuilds colors after a theme change", () => {
    const { tui, texts } = fakeTui();
    const firstTheme = fakeTheme("first");
    const secondTheme = fakeTheme("second");
    let serializations = 0;
    const renderers = createToolPreviewRenderers(tui, "ws__test", (value) => {
      serializations += 1;
      return `step: ${(value as { step: number }).step}`;
    });
    const args = { step: 1 };
    const state = {};
    const first = renderers.renderCall(args, firstTheme, context({ state, argsComplete: false }));
    first.render(30);
    args.step = 2;
    const second = renderers.renderCall(args, secondTheme, context({ state, lastComponent: first, argsComplete: false }));
    second.render(30);
    const completed = renderers.renderCall(args, secondTheme, context({ state, lastComponent: second, argsComplete: true }));
    completed.render(30);
    renderers.renderCall(args, secondTheme, context({ state, lastComponent: completed, argsComplete: true })).render(30);

    assert.match(texts[1]!.text, /step: 2/);
    assert.match(texts[1]!.text, /<second:fg:text>/);
    assert.equal(serializations, 3, "incomplete mutable arguments reprepare; completed identity then caches");
  });

  test("renders every completed single text result as YAML or RAW and preserves the original payload", () => {
    const { tui } = fakeTui();
    const renderers = createToolPreviewRenderers(tui, "ws__test");
    const rawCases = ["plain prose", '"scalar"', "{ malformed", "", "line\twith\u0007control", "literal...dots\n界"];
    for (const raw of rawCases) {
      const content = [{ type: "text", text: raw }];
      const component = renderers.renderResult({ content }, { expanded: true, isPartial: false }, unstyledTheme, context());
      const displayed = component.render(80).join("\n");
      assert.equal(completedTextPreview(raw).kind, "raw");
      assert.equal(
        displayed,
        physicalPreview(sanitizePreviewText(raw), 80, { expanded: true, trimOuterWhitespace: false }).join("\n"),
      );
      assert.deepEqual(content, [{ type: "text", text: raw }], "rendering must not alter model-visible payload text");
    }

    const yaml = renderers.renderResult(
      { content: [{ type: "text", text: '{"ok":true}' }] },
      { expanded: false, isPartial: false },
      unstyledTheme,
      context(),
    );
    assert.match(yaml.render(80).join("\n"), /ok: true/);

    const fallbackCases = [
      { content: [{ type: "text", text: '{"ok":true}' }], partial: true },
      { content: [{ type: "text", text: '{"ok":true}' }], partial: false, error: true },
      { content: [{ type: "text", text: "{}" }, { type: "text", text: "later text" }], partial: false },
      { content: [{ type: "text", text: "{}" }, { type: "image", data: "abc", mimeType: "image/png" }], partial: false },
    ];
    for (const item of fallbackCases) {
      assert.throws(
        () => renderers.renderResult({ content: item.content }, { expanded: false, isPartial: item.partial }, unstyledTheme, context({ isError: item.error })),
        UseNativeResultFallback,
      );
    }
  });

  test("uses the shared registration seam while preserving specialized renderers and cold fallback", () => {
    const { tui } = fakeTui();
    const ref = createToolPreviewTuiRef();
    const registered: Array<Record<string, unknown>> = [];
    const pi = { registerTool: (definition: Record<string, unknown>) => registered.push(definition) };
    const definition = {
      name: "ws-agent-send",
      label: "ws-agent-send",
      description: "send",
      parameters: {},
      async execute() { return { content: [{ type: "text", text: "ok" }] }; },
    };
    registerWsTool(pi as never, definition as never, ref);
    assert.equal(typeof registered[0]?.renderCall, "function");
    assert.equal(typeof registered[0]?.renderResult, "function");
    assert.throws(() => (registered[0]?.renderCall as Function)({}, unstyledTheme, context()), UseNativeResultFallback);

    ref.current = tui;
    const call = (registered[0]?.renderCall as Function)({ message: "hello" }, unstyledTheme, context());
    assert.match(call.render(80).join("\n"), /message: hello/);
    const raw = (registered[0]?.renderResult as Function)(
      { content: [{ type: "text", text: "RAW text" }] }, { expanded: false, isPartial: false }, unstyledTheme, context(),
    );
    assert.match(raw.render(80).join("\n"), /RAW text/);

    const custom = () => ({ render: () => [], invalidate() {} });
    const specialized = { ...definition, name: "ws-special", renderCall: custom };
    registerWsTool(pi as never, specialized as never, ref);
    assert.equal(registered[1]?.renderCall, custom, "specialized renderer is never overwritten");
    assert.equal(registered[1]?.renderResult, undefined);
  });

  test("preserves expanded narrow ASCII/CJK output and fits its narrow marker in real Pi composition", async () => {
    const codingAgentUrl = import.meta.resolve("@earendil-works/pi-coding-agent");
    const requireFromPi = createRequire(codingAgentUrl);
    const tui = await import(pathToFileURL(requireFromPi.resolve("@earendil-works/pi-tui")).href) as unknown as ToolResultTuiModules;
    const theme = await import(new URL("./modes/interactive/theme/theme.js", codingAgentUrl).href) as { initTheme(): void };
    theme.initTheme();
    const toolExecution = await import(new URL("./modes/interactive/components/tool-execution.js", codingAgentUrl).href) as {
      ToolExecutionComponent: new (
        toolName: string,
        toolCallId: string,
        args: unknown,
        options: unknown,
        toolDefinition: unknown,
        ui: { requestRender(): void },
        cwd: string,
      ) => { render(width: number): string[]; setArgsComplete(): void; setExpanded(expanded: boolean): void; updateResult(result: unknown, isPartial: boolean): void };
    };
    const createComponent = (serialized: string) => {
      const renderers = createToolPreviewRenderers(tui, "ws__test", (value) => "input" in value ? "" : serialized);
      const component = new toolExecution.ToolExecutionComponent(
        "ws__test", "call-1", { input: true }, { showImages: false },
        { renderCall: renderers.renderCall, renderResult: renderers.renderResult }, { requestRender() {} }, process.cwd(),
      );
      component.setArgsComplete();
      component.updateResult({ content: [{ type: "text", text: "{}" }], isError: false }, false);
      return component;
    };

    for (const outerWidth of [5, 6, 7]) {
      const component = createComponent("界\nTAIL");
      component.setExpanded(true);
      const compact = component.render(outerWidth).map((line) => tui.stripTerminalSequences(line)).join("").replace(/\s/g, "");
      assert.match(compact, /界/, `CJK survives at outer width ${outerWidth}`);
      assert.match(compact, /T.*A.*I.*L/, `ASCII survives at outer width ${outerWidth}`);
    }

    const marker = createComponent(Array.from({ length: 11 }, (_, index) => `line-${index}`).join("\n"));
    const markerLines = marker.render(3).map((line) => tui.stripTerminalSequences(line).trim());
    assert.equal(markerLines.filter((line) => line === ".").length, 1, "narrow marker renders as one row, not three wrapped dots");
  });

  test("renders the installed Pi input marker indented, gray, and narrow-safe", async () => {
    const codingAgentUrl = import.meta.resolve("@earendil-works/pi-coding-agent");
    const requireFromPi = createRequire(codingAgentUrl);
    const tui = await import(pathToFileURL(requireFromPi.resolve("@earendil-works/pi-tui")).href) as unknown as ToolResultTuiModules;
    const themeModule = await import(new URL("./modes/interactive/theme/theme.js", codingAgentUrl).href) as {
      initTheme(): void;
      theme: { fg(color: "toolOutput", text: string): string };
    };
    themeModule.initTheme();
    const toolExecution = await import(new URL("./modes/interactive/components/tool-execution.js", codingAgentUrl).href) as {
      ToolExecutionComponent: new (
        toolName: string,
        toolCallId: string,
        args: unknown,
        options: unknown,
        toolDefinition: unknown,
        ui: { requestRender(): void },
        cwd: string,
      ) => { render(width: number): string[]; setArgsComplete(): void };
    };
    const renderers = createToolPreviewRenderers(tui, "ws__test", () => Array.from({ length: 11 }, (_, index) => `line-${index}`).join("\n"));
    const component = new toolExecution.ToolExecutionComponent(
      "ws__test", "call-1", { input: true }, { showImages: false },
      { renderCall: renderers.renderCall, renderResult: renderers.renderResult }, { requestRender() {} }, process.cwd(),
    );
    component.setArgsComplete();

    const wide = component.render(40);
    const marker = wide.find((line) => tui.stripTerminalSequences(line).trim() === "...");
    assert.ok(marker);
    assert.match(tui.stripTerminalSequences(marker), /^ {5}\.\.\. *$/, "parent padding plus four-column input marker indent");
    assert.ok(marker.includes(themeModule.theme.fg("toolOutput", "    ...")), "marker uses toolOutput independently of input text");

    const narrowMarkers = component.render(3).filter((line) => tui.stripTerminalSequences(line).trim() === ".");
    assert.equal(narrowMarkers.length, 1, "narrow input marker remains a single fitted row");
  });

  test("uses the input-owned separator for YAML, raw, error, and pending installed Pi rows", async () => {
    const codingAgentUrl = import.meta.resolve("@earendil-works/pi-coding-agent");
    const requireFromPi = createRequire(codingAgentUrl);
    const tui = await import(pathToFileURL(requireFromPi.resolve("@earendil-works/pi-tui")).href) as unknown as ToolResultTuiModules;
    const theme = await import(new URL("./modes/interactive/theme/theme.js", codingAgentUrl).href) as { initTheme(): void };
    theme.initTheme();
    const toolExecution = await import(new URL("./modes/interactive/components/tool-execution.js", codingAgentUrl).href) as {
      ToolExecutionComponent: new (
        toolName: string,
        toolCallId: string,
        args: unknown,
        options: unknown,
        toolDefinition: unknown,
        ui: { requestRender(): void },
        cwd: string,
      ) => { render(width: number): string[]; setArgsComplete(): void; updateResult(result: unknown, isPartial: boolean): void };
    };
    const createComponent = () => {
      const renderers = createToolPreviewRenderers(tui, "ws__test", (value) => "input" in value ? "input: one" : "yaml: success");
      const component = new toolExecution.ToolExecutionComponent(
        "ws__test", "call-1", { input: true }, { showImages: false },
        { renderCall: renderers.renderCall, renderResult: renderers.renderResult }, { requestRender() {} }, process.cwd(),
      );
      component.setArgsComplete();
      return component;
    };
    const rows = (component: { render(width: number): string[] }) =>
      component.render(40).map((line) => tui.stripTerminalSequences(line));
    const assertInputBoundary = (lines: string[], output: string) => {
      const title = lines.findIndex((line) => line.includes("ws__test"));
      const input = lines.findIndex((line) => line.includes("input: one"));
      const result = lines.findIndex((line) => line.includes(output));
      assert.ok(title >= 0 && input > title && result > input);
      assert.equal(lines[title + 1]?.trim(), "", "one title/input separator");
      assert.equal(result, input + 2, "input's trailing separator is the only input/result separator");
      assert.equal(lines[input + 1]?.trim(), "");
    };

    const yaml = createComponent();
    yaml.updateResult({ content: [{ type: "text", text: '{"yaml":true}' }], isError: false }, false);
    assertInputBoundary(rows(yaml), "yaml: success");

    const raw = createComponent();
    raw.updateResult({ content: [{ type: "text", text: "raw fallback" }], isError: false }, false);
    assertInputBoundary(rows(raw), "raw fallback");

    const error = createComponent();
    error.updateResult({ content: [{ type: "text", text: "error fallback" }], isError: true }, false);
    assertInputBoundary(rows(error), "error fallback");

    const pending = rows(createComponent());
    const pendingTitle = pending.findIndex((line) => line.includes("ws__test"));
    const pendingInput = pending.findIndex((line) => line.includes("input: one"));
    assert.equal(pending[pendingTitle + 1]?.trim(), "", "pending keeps the title/input separator");
    assert.equal(pending[pendingInput + 1]?.trim(), "", "pending keeps the input-owned trailing separator");
  });

  test("uses real installed Pi parent-shell composition and retains its padding", async () => {
    const codingAgentUrl = import.meta.resolve("@earendil-works/pi-coding-agent");
    const requireFromPi = createRequire(codingAgentUrl);
    const tui = await import(pathToFileURL(requireFromPi.resolve("@earendil-works/pi-tui")).href) as unknown as ToolResultTuiModules;
    const theme = await import(new URL("./modes/interactive/theme/theme.js", codingAgentUrl).href) as { initTheme(): void };
    theme.initTheme();
    const toolExecution = await import(new URL("./modes/interactive/components/tool-execution.js", codingAgentUrl).href) as {
      ToolExecutionComponent: new (
        toolName: string,
        toolCallId: string,
        args: unknown,
        options: unknown,
        toolDefinition: unknown,
        ui: { requestRender(): void },
        cwd: string,
      ) => { render(width: number): string[]; getRenderShell(): string; setArgsComplete(): void; updateResult(result: unknown, isPartial: boolean): void };
    };
    const renderers = createToolPreviewRenderers(tui, "ws__git_status", (value) =>
      "ok" in value ? "ok: true" : "value: abcdefghijk",
    );
    const component = new toolExecution.ToolExecutionComponent(
      "ws__git_status",
      "call-1",
      { value: "ignored" },
      { showImages: false },
      { renderCall: renderers.renderCall, renderResult: renderers.renderResult },
      { requestRender() {} },
      process.cwd(),
    );
    component.setArgsComplete();
    component.updateResult({ content: [{ type: "text", text: '{"ok":true}' }], isError: false }, false);

    assert.equal(component.getRenderShell(), "default");
    const lines = component.render(24).map((line) => tui.stripTerminalSequences(line));
    const title = lines.findIndex((line) => line.includes("ws__git_status"));
    assert.ok(title >= 0);
    assert.match(lines[title + 2] ?? "", /^ {5}value:/, "parent shell's one-column padding plus four-column input indent");
    assert.ok(lines.some((line) => line.includes("ok: true")));
  });
});
