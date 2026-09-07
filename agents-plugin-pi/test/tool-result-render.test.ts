import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  approximateCodePointWidth,
  createToolPreviewRenderers,
  physicalPreview,
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
} {
  let stripCalls = 0;
  let truncationCalls = 0;
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
    },
    strips: () => stripCalls,
    texts,
    boxes,
    truncations: () => truncationCalls,
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
    assert.deepEqual(physicalPreview("wide", 3, { expanded: false, trimOuterWhitespace: false }), ["", "..."]);
    assert.deepEqual(
      physicalPreview("👩‍💻", 8, { expanded: false, trimOuterWhitespace: false }),
      ["    👩‍", "   💻"],
    );
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
  test("keeps bold tool identity, white input, gray output, separated backgrounds, and exact blank rows", () => {
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
    assert.equal(boxes[0]!.background?.("sample"), "<one:bg:toolPendingBg>sample</one:bg>");
    assert.equal(boxes[1]!.background?.("sample"), "<one:bg:toolSuccessBg>sample</one:bg>");
    assert.equal(texts.length, 3);
  });

  test("wraps long logical rows before the ten-row budget and expands full output", () => {
    const { tui } = fakeTui();
    const renderers = createToolPreviewRenderers(tui, "ws__test", () => "abcdefghijklmno");
    const content = [{ type: "text", text: '{"ok":true}' }];
    const state = {};
    const call = renderers.renderCall({ value: "ignored" }, unstyledTheme, context({ state }));
    assert.deepEqual(call.render(10), ["ws__test", "", "    abcdef", "   ghijklm", "   no"]);

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

  test("keeps errors, partials, scalars, prose, later text blocks, and images on Pi native fallback", () => {
    const { tui } = fakeTui();
    const renderers = createToolPreviewRenderers(tui, "ws__test");
    const cases = [
      { content: [{ type: "text", text: '{"ok":true}' }], partial: true },
      { content: [{ type: "text", text: '"scalar"' }], partial: false },
      { content: [{ type: "text", text: '{"ok":true}' }], partial: false, error: true },
      { content: [{ type: "text", text: "plain error text" }], partial: false },
      { content: [{ type: "text", text: "{}" }, { type: "text", text: "later text" }], partial: false },
      { content: [{ type: "text", text: "{}" }, { type: "image", data: "abc", mimeType: "image/png" }], partial: false },
    ];
    for (const item of cases) {
      assert.throws(
        () => renderers.renderResult({ content: item.content }, { expanded: false, isPartial: item.partial }, unstyledTheme, context({ isError: item.error })),
        UseNativeResultFallback,
      );
    }
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
