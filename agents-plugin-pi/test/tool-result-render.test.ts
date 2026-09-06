import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  createToolPreviewRenderers,
  logicalPreview,
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
      this.lines = [this.text];
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

function context(overrides: Partial<{ state: object; lastComponent: unknown; argsComplete: boolean; isPartial: boolean; isError: boolean }> = {}) {
  return {
    state: overrides.state ?? {},
    lastComponent: overrides.lastComponent,
    argsComplete: overrides.argsComplete ?? true,
    isPartial: overrides.isPartial ?? false,
    isError: overrides.isError ?? false,
  };
}

describe("logical YAML preview preparation", () => {
  test("selects ten logical CRLF-normalized lines before native layout", () => {
    const text = Array.from({ length: 12 }, (_, index) => `line-${index + 1}`).join("\r\n");
    assert.deepEqual(logicalPreview(text).split("\n"), Array.from({ length: 10 }, (_, index) => `line-${index + 1}`));
  });

  test("serializes containers only, leaving scalar JSON and prose unclaimed", () => {
    assert.match(yamlContainerDisplay('{"task":"render","count":2}') ?? "", /task: render/);
    assert.equal(yamlContainerDisplay('"plain string"'), undefined);
    assert.equal(yamlContainerDisplay('not json'), undefined);
  });

  test("input uses YAML while absent or malformed streamed arguments remain safe and empty", () => {
    assert.match(yamlInputPreview({ nested: { count: 2 } }), /nested:/);
    assert.equal(yamlInputPreview(undefined), "");
    assert.equal(yamlInputPreview(["not", "tool", "arguments"]), "");
  });
});

describe("native YAML preview renderers", () => {
  test("styles the registered title and puts ten argument lines in a pending input box", () => {
    const { tui, texts, boxes } = fakeTui();
    const theme = fakeTheme();
    const renderers = createToolPreviewRenderers(tui, "ws__git_status");
    const missing = renderers.renderCall(undefined, theme, context({ argsComplete: false }));
    assert.match(texts[0]?.text ?? "", /ws__git_status/);
    assert.deepEqual(theme.boldCalls, ["ws__git_status"]);
    assert.deepEqual(theme.fgCalls[0], { color: "toolTitle", text: "<one:bold>ws__git_status</one:bold>" });

    const args = Object.fromEntries(Array.from({ length: 12 }, (_, index) => [`item${index + 1}`, index + 1]));
    renderers.renderCall(args, theme, context({ state: {}, lastComponent: missing }));
    const lines = texts[1]!.text.split("\n");
    assert.equal(lines.length, 10, "ten logical argument lines");
    assert.equal(boxes[0]!.background?.("sample"), "<one:bg:toolPendingBg>sample</one:bg>");
  });

  test("installed ToolExecutionComponent retains the registered bridge title for missing arguments", async () => {
    const codingAgentUrl = import.meta.resolve("@earendil-works/pi-coding-agent");
    const requireFromPi = createRequire(codingAgentUrl);
    const tui = await import(pathToFileURL(requireFromPi.resolve("@earendil-works/pi-tui")).href) as unknown as ToolResultTuiModules;
    const theme = await import(new URL("./modes/interactive/theme/theme.js", codingAgentUrl).href) as {
      initTheme(): void;
    };
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
      ) => { render(width: number): string[] };
    };
    const renderers = createToolPreviewRenderers(tui, "ws__git_status");
    const component = new toolExecution.ToolExecutionComponent(
      "ws__git_status",
      "call-1",
      undefined,
      { showImages: false },
      { renderCall: renderers.renderCall },
      { requestRender() {} },
      process.cwd(),
    );

    assert.equal((component as unknown as { getRenderShell(): string }).getRenderShell(), "default", "bridge leaves Pi's parent shell enabled");
    assert.match(tui.stripTerminalSequences(component.render(80).join("\n")), /ws__git_status/);
  });

  test("adds only a top row and left input padding without asking native text for an impossible width", () => {
    const { tui, texts, boxes } = fakeTui();
    const theme = {
      fg: (_color: string, text: string) => text,
      bg: (_color: string, text: string) => text,
      bold: (text: string) => text,
    };
    const renderers = createToolPreviewRenderers(tui, "ws__test", () => "first\nsecond");
    renderers.renderCall({ value: "ignored" }, theme, context());

    assert.deepEqual(boxes[0]!.render(0), []);
    assert.equal(texts[1]!.layoutCalls, 0, "zero-width input must not reach native Text");
    assert.deepEqual(boxes[0]!.render(1), ["", " "], "width one keeps only the blank row and left column");
    assert.equal(texts[1]!.layoutCalls, 0, "one-column input must not ask native Text for width zero");
    assert.deepEqual(boxes[0]!.render(20), ["", " first\nsecond"], "normal rows have no bottom or right padding");
    assert.equal(texts[1]!.layoutCalls, 1);
  });

  test("reapplies theme styling without reserializing cached YAML", () => {
    const { tui, texts, boxes } = fakeTui();
    const firstTheme = fakeTheme("first");
    const secondTheme = fakeTheme("second");
    let serializations = 0;
    const renderers = createToolPreviewRenderers(tui, "ws__test", (value) => {
      serializations += 1;
      return `value: ${(value as { value: string }).value}`;
    });
    const args = { value: "cached" };
    const state = {};
    const first = renderers.renderCall(args, firstTheme, context({ state }));
    first.render(20);
    first.invalidate();
    const second = renderers.renderCall(args, secondTheme, context({ state, lastComponent: first }));

    assert.equal(second, first);
    assert.equal(serializations, 1, "theme changes do not reserialize YAML");
    assert.match(texts[0]!.text, /<second:fg:toolTitle>/);
    assert.match(texts[1]!.text, /<second:fg:toolOutput>/);
    assert.equal(boxes[0]!.background?.("sample"), "<second:bg:toolPendingBg>sample</second:bg>");
  });

  test("keeps a completed call Text long-lived without reserializing or relaying out unchanged redraws", () => {
    const { tui, texts, truncations } = fakeTui();
    const theme = fakeTheme();
    let serializations = 0;
    const renderers = createToolPreviewRenderers(tui, "ws__test", (value) => {
      serializations += 1;
      return `value: ${(value as { value: string }).value}`;
    });
    const args = { value: "large payload" };
    const state = {};
    const first = renderers.renderCall(args, theme, context({ state }));
    const second = renderers.renderCall(args, theme, context({ state, lastComponent: first }));

    assert.equal(second, first);
    assert.equal(serializations, 1, "unchanged arguments must not be serialized to form a redraw cache key");
    first.render(24);
    first.render(24);
    assert.equal(texts[1]!.layoutCalls, 1, "native Text owns unchanged-width YAML layout caching");
    assert.equal(texts[1]!.setTextCalls, 1);
    assert.equal(truncations(), 2, "title and YAML rows are not remapped on an unchanged redraw");
  });

  test("reprepares incomplete in-place-mutated arguments, then caches after completion", () => {
    const { tui, texts } = fakeTui();
    const theme = fakeTheme();
    let serializations = 0;
    const renderers = createToolPreviewRenderers(tui, "ws__test", (value) => {
      serializations += 1;
      return `step: ${(value as { step: number }).step}`;
    });
    const args = { step: 1 };
    const state = {};
    const first = renderers.renderCall(args, theme, context({ state, argsComplete: false }));
    args.step = 2;
    const second = renderers.renderCall(args, theme, context({ state, lastComponent: first, argsComplete: false }));
    const third = renderers.renderCall(args, theme, context({ state, lastComponent: second, argsComplete: true }));
    renderers.renderCall(args, theme, context({ state, lastComponent: third, argsComplete: true }));

    assert.match(texts[1]!.text, /step: 2/);
    assert.equal(serializations, 3, "partial mutation must not reuse a stale object-identity cache");
  });

  test("renders JSON containers as YAML, expands only result output, and invalidates result cache by expansion", () => {
    const { tui, strips, texts, boxes } = fakeTui();
    const theme = fakeTheme();
    let serializations = 0;
    const renderers = createToolPreviewRenderers(tui, "ws__test", (value) => {
      serializations += 1;
      return `${Array.from({ length: 12 }, (_, index) => `line-${index + 1}: ${(value as { ok: boolean }).ok}`).join("\n")}\x1b[2J`;
    });
    const content = [{ type: "text", text: '{"ok":true}' }];
    const state = {};
    const first = renderers.renderResult({ content }, { expanded: false, isPartial: false }, theme, context({ state }));
    const again = renderers.renderResult({ content }, { expanded: false, isPartial: false }, theme, context({ state, lastComponent: first }));
    const collapsedText = texts[0]!.text;
    const expanded = renderers.renderResult({ content }, { expanded: true, isPartial: false }, theme, context({ state, lastComponent: again }));

    assert.equal(again, first);
    assert.equal(collapsedText.split("\n").length, 10);
    assert.equal(texts[0]!.text.split("\n").length, 12);
    assert.equal(serializations, 2, "only expansion transition reparses/serializes the result");
    assert.ok(strips() >= 2, "custom YAML text is stripped through Pi's terminal-control seam");
    assert.ok(theme.fgCalls.every((call) => !call.text.includes("\x1b")), "styles run after untrusted text is sanitized");
    assert.equal(boxes[0]!.background?.("sample"), "<one:bg:toolSuccessBg>sample</one:bg>");
    expanded.render(20);
    expanded.render(40);
    assert.equal(texts[0]!.layoutCalls, 2, "width changes are delegated to native Text layout");
  });

  test("keeps errors, scalars, prose, later text blocks, images, and partial results on Pi's native fallback", () => {
    const { tui } = fakeTui();
    const theme = fakeTheme();
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
        () => renderers.renderResult({ content: item.content }, { expanded: false, isPartial: item.partial }, theme, context({ isError: item.error })),
        UseNativeResultFallback,
      );
    }
  });

  test("installed Pi TUI seam bounds indivisible CJK and emoji at width one", async () => {
    const requireFromPi = createRequire(import.meta.resolve("@earendil-works/pi-coding-agent"));
    const tui = await import(pathToFileURL(requireFromPi.resolve("@earendil-works/pi-tui")).href) as unknown as ToolResultTuiModules & {
      visibleWidth(text: string): number;
    };
    const renderers = createToolPreviewRenderers(tui, "ws__git_status");
    const theme = { fg: (_color: string, text: string) => text, bg: (_color: string, text: string) => text, bold: (text: string) => text };
    for (const value of ["界", "👩‍💻"]) {
      const component = renderers.renderCall({ x: value }, theme, context());
      for (const line of component.render(1)) {
        assert.ok(tui.visibleWidth(line) <= 1, `${JSON.stringify(value)} emitted an overwide row: ${JSON.stringify(line)}`);
      }
    }
  });
});
