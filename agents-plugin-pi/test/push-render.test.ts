/**
 * Unit tests for push-render.ts: the compact TUI rendering of the six pushed
 * child-report families.
 *
 * The seam under test is `buildPushRenderLines` — splitting a pushed message's
 * plain-text content back into head / payload / status — because that is where
 * the duplicate-header fix actually lives: the content keeps its `[family]
 * agent <id>` head (the lead's model reads only `content`), and the renderer is
 * what stops Pi's default component from printing the family label a second
 * time above it.
 *
 * `pi-tui` is reached through `./pi-tui.ts`'s `loadHostPiTui()` (see that
 * file's Addendum doc comment), so `buildPushComponent` and
 * `registerPushMessageRenderers` are driven here with an injected duck-typed
 * stand-in rather than exercising the real host resolution — this suite
 * never needs an "unavailable" case any more (`loadPushTuiModules` always
 * resolves now).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { buildPushComponent, buildPushRenderLines, registerPushMessageRenderers, type PushTuiModules } from "../src/push-render.ts";
import { buildPushContent, PUSH_FAMILIES } from "../src/spawner.ts";
import { approximateCodePointWidth } from "../src/tool-result-render.ts";

describe("buildPushRenderLines", () => {
  test("splits a real pushed message into head, payload and status", () => {
    const status = "1 delegated agent still running";
    const content = buildPushContent("ws-agent-report", "w1", { kind: "final", report: "Outcome: done" }, status);

    assert.deepEqual(buildPushRenderLines({ content, details: { status } }), {
      head: "[ws-agent-report] agent w1",
      body: ["kind: final", "report: Outcome: done"],
      status,
    });
  });

  test("a message with no status line keeps its last payload line as payload", () => {
    const content = buildPushContent("ws-agent-orphaned", undefined, { count: 2 }, undefined);
    assert.deepEqual(buildPushRenderLines({ content, details: { count: 2 } }), {
      head: "[ws-agent-orphaned]",
      body: ["count: 2"],
      status: undefined,
    });
  });

  test("without a details.status the status line is still recognized by shape", () => {
    const content = ["[ws-agent-settled] agent a1", "reason: idle", "0 delegated agents still running"].join("\n");
    const parts = buildPushRenderLines({ content });
    assert.equal(parts?.status, "0 delegated agents still running");
    assert.deepEqual(parts?.body, ["reason: idle"]);
  });

  test("a payload line that merely looks like prose is never mistaken for the status line", () => {
    const content = ["[ws-agent-report] agent a1", "report: three of five checks still running"].join("\n");
    const parts = buildPushRenderLines({ content });
    assert.equal(parts?.status, undefined);
    assert.deepEqual(parts?.body, ["report: three of five checks still running"]);
  });

  test("details.status wins over the shape guess, so a report QUOTING a status line is not eaten", () => {
    const status = "0 delegated agents still running";
    const content = ["[ws-agent-report] agent a1", "report: the sub-lead saw `2 delegated agents still running`", status].join("\n");
    const parts = buildPushRenderLines({ content, details: { status } });
    assert.equal(parts?.status, status);
    assert.equal(parts?.body.length, 1);
  });

  test("array content (text parts) is read the same as string content", () => {
    const parts = buildPushRenderLines({
      content: [
        { type: "text", text: "[ws-agent-advisory] agent a1\nadvisory: stalled" },
        { type: "image", data: "ignored" },
      ],
    });
    assert.deepEqual(parts, { head: "[ws-agent-advisory] agent a1", body: ["advisory: stalled"], status: undefined });
  });

  test("an empty or unrecognizable message returns undefined, which is Pi's fall-back-to-default signal", () => {
    assert.equal(buildPushRenderLines({ content: "" }), undefined);
    assert.equal(buildPushRenderLines({ content: "   " }), undefined);
    assert.equal(buildPushRenderLines({}), undefined);
    assert.equal(buildPushRenderLines({ content: 42 }), undefined);
  });

  test("a head-only message renders as a head with no body", () => {
    assert.deepEqual(buildPushRenderLines({ content: "[ws-agent-orphaned]" }), {
      head: "[ws-agent-orphaned]",
      body: [],
      status: undefined,
    });
  });
});

type FakeComponent = { render(width: number): string[]; invalidate(): void };

/**
 * Duck-typed `pi-tui` stand-in. `Text`/`Box` are shaped to satisfy
 * `NativeText`/`NativeBox` (`setText`/`render`/`invalidate`,
 * `addChild`/`setBgFn`/`render`/`invalidate`) — the same contract
 * `test/tool-result-render.test.ts`'s `FakeBox` captures — because
 * `buildPushComponent`'s body now goes through `createBoundedText`/
 * `updateText` from `tool-result-render.ts`, which requires that shape.
 */
function fakeTui(): { modules: PushTuiModules; boxes: FakeBox[]; strips: number; truncations: number; layouts: number; joins: number; styles: number } {
  const boxes: FakeBox[] = [];
  let strips = 0;
  let truncations = 0;
  let layouts = 0;
  let joins = 0;
  let styles = 0;

  class FakeText implements FakeComponent {
    private text: string;
    constructor(text = "") {
      this.text = text;
    }
    setText(text: string): void {
      this.text = text;
    }
    render(_width: number): string[] {
      return this.text.split("\n");
    }
    invalidate(): void {}
  }

  class FakeBoxImpl implements FakeComponent {
    padding: number[];
    background: ((text: string) => string) | undefined;
    children: FakeComponent[] = [];
    constructor(paddingX = 0, paddingY = 0, background?: (text: string) => string) {
      this.padding = [paddingX, paddingY];
      this.background = background;
      boxes.push(this);
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
  type FakeBox = FakeBoxImpl;

  return {
    modules: {
      Box: FakeBoxImpl,
      Text: FakeText,
      stripTerminalSequences: (text: string) => {
        strips += 1;
        return text.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "");
      },
      truncateToWidth: (text: string) => {
        truncations += 1;
        return text;
      },
      onPreviewLayout: () => { layouts += 1; },
      onPreviewJoin: () => { joins += 1; },
      onPreviewStyle: () => { styles += 1; },
    } as unknown as PushTuiModules,
    boxes,
    get strips() { return strips; },
    get truncations() { return truncations; },
    get layouts() { return layouts; },
    get joins() { return joins; },
    get styles() { return styles; },
  };
}

/** Mirrors `test/tool-result-render.test.ts`'s tracked fake theme. */
function fakeTheme() {
  const fgCalls: Array<{ color: string; text: string }> = [];
  const bgCalls: Array<{ color: string; text: string }> = [];
  let variant = "initial";
  return {
    fg(color: string, text: string): string {
      fgCalls.push({ color, text });
      return `<fg:${color}:${variant}>${text}</fg>`;
    },
    bg(color: string, text: string): string {
      bgCalls.push({ color, text });
      return `<bg:${color}>${text}</bg>`;
    },
    fgCalls,
    bgCalls,
    setVariant(next: string): void {
      variant = next;
    },
  };
}

function plainLine(text: string): string {
  return text.replace(/<[^>]+>/g, "");
}

function displayWidth(text: string): number {
  return [...plainLine(text)].reduce((width, codePoint) => width + approximateCodePointWidth(codePoint), 0);
}

describe("buildPushComponent", () => {
  const status = "1 delegated agent still running";
  const message = {
    content: buildPushContent("ws-agent-report", "w1", { kind: "final", report: "Outcome: done" }, status),
    details: { status },
  };

  test("draws the family head ONCE — Pi's default printed it above an identical content line", () => {
    const tui = fakeTui();
    const theme = fakeTheme();
    const component = buildPushComponent(tui.modules, message, theme, false, "ws-agent-report") as FakeComponent;

    const rendered = component.render(80);
    assert.deepEqual(rendered.map(plainLine), [
      "[ws-agent-report] agent w1",
      "kind: final",
      "report: Outcome: done",
      status,
    ]);
    assert.equal(tui.boxes.length, 1);
    assert.deepEqual(tui.boxes[0].padding, [1, 0], "compact: one column of padding, no blank rows");
    assert.ok(theme.bgCalls.every((call) => call.color === "customMessageBg"), "every background paint uses the shared token");
    assert.ok(theme.bgCalls.length > 0, "the box paints a background");
    assert.ok(
      theme.fgCalls.some((call) => call.color === "customMessageLabel" && call.text.includes("[ws-agent-report] agent w1")),
      "report head uses the theme's existing label role",
    );
    assert.ok(
      theme.fgCalls.some((call) => call.color === "muted" && call.text.includes("kind: final")),
      "body is muted",
    );
    assert.ok(
      theme.fgCalls.some((call) => call.color === "dim" && call.text === status),
      "status stays dim",
    );
    assert.ok(
      !theme.fgCalls.some((call) => call.color === "customMessageText"),
      "does not paint with Pi's default body color",
    );
  });

  test("no theme (and a throwing theme) degrade to unpainted text rather than to no component", () => {
    const plain = fakeTui();
    const plainComponent = buildPushComponent(plain.modules, message, undefined, false, "ws-agent-report") as FakeComponent;
    assert.ok(plainComponent);
    assert.equal(plainComponent.render(80)[0], "[ws-agent-report] agent w1");

    const broken = fakeTui();
    const brokenComponent = buildPushComponent(broken.modules, message, {
      fg: () => {
        throw new Error("theme is gone");
      },
      bg: () => {
        throw new Error("theme is gone");
      },
    }, false, "ws-agent-report") as FakeComponent;
    assert.ok(brokenComponent);
    assert.equal(brokenComponent.render(80)[0], "[ws-agent-report] agent w1");
  });

  test("a status-less message draws no status row", () => {
    const tui = fakeTui();
    const component = buildPushComponent(
      tui.modules,
      { content: buildPushContent("ws-agent-orphaned", undefined, { count: 2 }, undefined) },
      undefined,
    ) as FakeComponent;
    assert.deepEqual(component.render(80), ["[ws-agent-orphaned]", "count: 2"]);
  });

  test("an unrecognizable message returns undefined so Pi's default rendering stands", () => {
    const tui = fakeTui();
    assert.equal(buildPushComponent(tui.modules, { content: "" }, undefined), undefined);
    assert.deepEqual(tui.boxes, [], "no half-built box is left behind");
  });

  test("260906 Phase 1: a body of ten or fewer logical lines renders in full with no marker", () => {
    const tui = fakeTui();
    const body = Array.from({ length: 10 }, (_, index) => `line-${index}: value`);
    const component = buildPushComponent(
      tui.modules,
      { content: buildPushContent("ws-agent-report", "w1", Object.fromEntries(body.map((line) => line.split(": "))), undefined) },
      undefined,
    ) as FakeComponent;
    const lines = component.render(80);
    assert.equal(lines.length, 11, "head + ten body lines, no marker");
    assert.ok(!lines.some((line) => line.trim() === "..."));
  });

  test("260906 Phase 1: a body over ten logical lines is collapsed to ten plus a marker, and expansion recovers all of it", () => {
    const tui = fakeTui();
    const entries = Object.fromEntries(Array.from({ length: 12 }, (_, index) => [`k${index}`, `v${index}`]));
    const built = { content: buildPushContent("ws-agent-report", "w1", entries, undefined) };

    const collapsed = buildPushComponent(tui.modules, built, undefined, false) as FakeComponent;
    const collapsedLines = collapsed.render(80);
    // head + 10 capped body logical lines + marker.
    assert.equal(collapsedLines.length, 12);
    assert.equal(collapsedLines.at(-1)?.trim(), "...");
    assert.doesNotMatch(collapsedLines.join("\n"), /k11: v11/, "the 12th logical line is cut when collapsed");

    const expanded = buildPushComponent(tui.modules, built, undefined, true) as FakeComponent;
    const expandedLines = expanded.render(80);
    assert.match(expandedLines.join("\n"), /k11: v11/, "expansion recovers every hidden logical line");
    assert.equal(expandedLines.length, 13, "head + all 12 body logical lines, no marker");
  });

  test("reuses the bounded body preparation and layout at one width while a mutated theme restyles it", () => {
    const tui = fakeTui();
    const theme = fakeTheme();
    const entries = Object.fromEntries(Array.from({ length: 12 }, (_, index) => [`k${index}`, `v${index}`]));
    const component = buildPushComponent(
      tui.modules,
      { content: buildPushContent("ws-agent-report", "w1", entries, undefined) },
      theme,
      false,
      "ws-agent-report",
    ) as FakeComponent;

    component.render(80);
    const prepared = { strips: tui.strips, layouts: tui.layouts, joins: tui.joins };
    theme.fgCalls.length = 0;
    theme.setVariant("mutated");
    component.invalidate();
    const rerendered = component.render(80);

    assert.deepEqual({ strips: tui.strips, layouts: tui.layouts, joins: tui.joins }, prepared, "theme restyling does not reprepare or relayout the hidden body");
    assert.ok(tui.styles >= 2, "the same theme object is restyled after mutation/invalidation");
    assert.ok(theme.fgCalls.some((call) => call.color === "muted" && call.text.includes("k0: v0")), "body is restyled after same-object theme mutation");
    assert.match(rerendered.join("\n"), /<fg:muted:mutated>/, "the mutated theme output reaches the unchanged body");
  });

  test("260906 Phase 1: a head-only message with no body draws no empty body row", () => {
    const tui = fakeTui();
    const component = buildPushComponent(tui.modules, { content: "[ws-agent-orphaned]" }, undefined) as FakeComponent;
    assert.deepEqual(component.render(80), ["[ws-agent-orphaned]"]);
  });
});

describe("registerPushMessageRenderers", () => {
  test("registers exactly one renderer per push family, and each one renders", async () => {
    const registered = new Map<string, (message: unknown, options: unknown, theme: unknown) => unknown>();
    const pi = {
      registerMessageRenderer: (customType: string, renderer: (message: unknown, options: unknown, theme: unknown) => unknown) => {
        registered.set(customType, renderer);
      },
    };
    const tui = fakeTui();

    assert.equal(await registerPushMessageRenderers(pi as never, tui.modules), true);
    assert.deepEqual([...registered.keys()], [...PUSH_FAMILIES]);

    const rendered = registered.get("ws-agent-settled")!(
      { content: buildPushContent("ws-agent-settled", "a1", { reason: "idle" }, undefined) },
      { expanded: false, outputPad: 1 },
      undefined,
    ) as FakeComponent;
    assert.ok(rendered);
    assert.deepEqual(rendered.render(80), ["[ws-agent-settled] agent a1", "reason: idle"]);
  });

  test("260906 Phase 1: the shared customMessageBg background reaches every push family", async () => {
    const registered = new Map<string, (message: unknown, options: unknown, theme: unknown) => unknown>();
    const pi = {
      registerMessageRenderer: (customType: string, renderer: (message: unknown, options: unknown, theme: unknown) => unknown) => {
        registered.set(customType, renderer);
      },
    };
    const tui = fakeTui();
    await registerPushMessageRenderers(pi as never, tui.modules);

    for (const family of PUSH_FAMILIES) {
      const theme = fakeTheme();
      const rendered = registered.get(family)!(
        { content: buildPushContent(family, "a1", { note: "hi" }, undefined) },
        { expanded: false, outputPad: 1 },
        theme,
      ) as FakeComponent;
      rendered.render(80);
      assert.ok(theme.bgCalls.some((call) => call.color === "customMessageBg"), `${family} paints the shared background`);
    }
  });

  test("uses customMessageLabel only for registered report heads and preserves all comparison bands", async () => {
    const registered = new Map<string, (message: unknown, options: unknown, theme: unknown) => unknown>();
    const pi = {
      registerMessageRenderer: (customType: string, renderer: (message: unknown, options: unknown, theme: unknown) => unknown) => {
        registered.set(customType, renderer);
      },
    };
    const tui = fakeTui();
    await registerPushMessageRenderers(pi as never, tui.modules);

    for (const family of PUSH_FAMILIES) {
      const theme = fakeTheme();
      const status = family === "ws-agent-report" ? "1 delegated agent still running" : undefined;
      const component = registered.get(family)!(
        { content: buildPushContent(family, "a1", { note: "body" }, status), details: status ? { status } : undefined },
        { expanded: false },
        theme,
      ) as FakeComponent;
      component.render(80);
      const headColor = family === "ws-agent-report" ? "customMessageLabel" : "muted";
      assert.ok(theme.fgCalls.some((call) => call.color === headColor && call.text.includes(`[${family}]`)), `${family} head uses ${headColor}`);
      assert.ok(theme.fgCalls.some((call) => call.color === "muted" && call.text.includes("note: body")), `${family} body stays muted`);
      if (status) assert.ok(theme.fgCalls.some((call) => call.color === "dim" && call.text === status), "report status stays dim");
    }
  });

  test("registered reports fit long ANSI and multibyte payload rows at 40, 80, and 120 columns", async () => {
    const registered = new Map<string, (message: unknown, options: unknown, theme: unknown) => unknown>();
    const pi = { registerMessageRenderer: (family: string, renderer: (message: unknown, options: unknown, theme: unknown) => unknown) => registered.set(family, renderer) };
    const tui = fakeTui();
    await registerPushMessageRenderers(pi as never, tui.modules);
    const entries = Object.fromEntries(Array.from({ length: 12 }, (_, index) => [
      `k${index}`,
      index === 0 ? `\x1b[35m${"界".repeat(48)}\x1b[0m` : `v${index}`,
    ]));
    const message = { content: buildPushContent("ws-agent-report", "a1", entries, undefined) };

    for (const width of [40, 80, 120]) {
      const collapsed = registered.get("ws-agent-report")!(message, { expanded: false }, fakeTheme()) as FakeComponent;
      const collapsedLines = collapsed.render(width);
      assert.equal(collapsedLines.at(-1)?.replace(/<[^>]+>/g, "").trim(), "...");
      assert.doesNotMatch(collapsedLines.join("\n"), /k11: v11/, `collapsed report retains the logical-line cap at ${width}`);
      assert.ok(collapsedLines.every((line) => displayWidth(line) <= width), `collapsed report rows fit ${width} columns`);
      assert.doesNotMatch(collapsedLines.join("\n"), /\x1b\[/, `collapsed report sanitizes ANSI at ${width}`);
      const expanded = registered.get("ws-agent-report")!(message, { expanded: true }, fakeTheme()) as FakeComponent;
      const expandedLines = expanded.render(width);
      assert.match(expandedLines.join("\n"), /k11: v11/, `expanded report recovers all logical lines at ${width}`);
      assert.ok(expandedLines.every((line) => displayWidth(line) <= width), `expanded report rows fit ${width} columns`);
      assert.doesNotMatch(expandedLines.join("\n"), /\x1b\[/, `expanded report sanitizes ANSI at ${width}`);
    }
  });

  test("260906 Phase 1: registerPushMessageRenderers forwards options.expanded through to the body cap", async () => {
    const registered = new Map<string, (message: unknown, options: unknown, theme: unknown) => unknown>();
    const pi = {
      registerMessageRenderer: (customType: string, renderer: (message: unknown, options: unknown, theme: unknown) => unknown) => {
        registered.set(customType, renderer);
      },
    };
    const tui = fakeTui();
    await registerPushMessageRenderers(pi as never, tui.modules);
    const entries = Object.fromEntries(Array.from({ length: 12 }, (_, index) => [`k${index}`, `v${index}`]));
    const message = { content: buildPushContent("ws-agent-report", "a1", entries, undefined) };

    const collapsed = registered.get("ws-agent-report")!(message, { expanded: false, outputPad: 1 }, undefined) as FakeComponent;
    assert.doesNotMatch(collapsed.render(80).join("\n"), /k11: v11/);

    const expanded = registered.get("ws-agent-report")!(message, { expanded: true, outputPad: 1 }, undefined) as FakeComponent;
    assert.match(expanded.render(80).join("\n"), /k11: v11/);
  });
});
