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

  setText(text: string): void {
    this.text = text;
    this.setTextCalls += 1;
    this.width = undefined;
  }

  render(width: number): string[] {
    if (this.width !== width) {
      this.width = width;
      this.layoutCalls += 1;
    }
    return [this.text];
  }

  invalidate(): void {
    this.width = undefined;
  }
}

function fakeTui(): { tui: ToolResultTuiModules; strips: () => number } {
  let stripCalls = 0;
  return {
    tui: {
      Text: FakeText,
      stripTerminalSequences(text: string): string {
        stripCalls += 1;
        return text.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "");
      },
    },
    strips: () => stripCalls,
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
  test("keeps a completed call Text long-lived without reserializing or relaying out unchanged redraws", () => {
    const { tui } = fakeTui();
    let serializations = 0;
    const renderers = createToolPreviewRenderers(tui, (value) => {
      serializations += 1;
      return `value: ${(value as { value: string }).value}`;
    });
    const args = { value: "large payload" };
    const state = {};
    const first = renderers.renderCall(args, undefined, context({ state })) as FakeText;
    const second = renderers.renderCall(args, undefined, context({ state, lastComponent: first }));

    assert.equal(second, first);
    assert.equal(serializations, 1, "unchanged arguments must not be serialized to form a redraw cache key");
    first.render(24);
    first.render(24);
    assert.equal(first.layoutCalls, 1, "native Text owns unchanged-width layout caching");
    assert.equal(first.setTextCalls, 1);
  });

  test("reprepares incomplete in-place-mutated arguments, then caches after completion", () => {
    const { tui } = fakeTui();
    let serializations = 0;
    const renderers = createToolPreviewRenderers(tui, (value) => {
      serializations += 1;
      return `step: ${(value as { step: number }).step}`;
    });
    const args = { step: 1 };
    const state = {};
    const first = renderers.renderCall(args, undefined, context({ state, argsComplete: false })) as FakeText;
    args.step = 2;
    const second = renderers.renderCall(args, undefined, context({ state, lastComponent: first, argsComplete: false })) as FakeText;
    const third = renderers.renderCall(args, undefined, context({ state, lastComponent: second, argsComplete: true })) as FakeText;
    renderers.renderCall(args, undefined, context({ state, lastComponent: third, argsComplete: true }));

    assert.equal(second.text, "step: 2");
    assert.equal(serializations, 3, "partial mutation must not reuse a stale object-identity cache");
  });

  test("renders JSON containers as YAML, expands only result output, and invalidates result cache by expansion", () => {
    const { tui, strips } = fakeTui();
    let serializations = 0;
    const renderers = createToolPreviewRenderers(tui, (value) => {
      serializations += 1;
      return `${Array.from({ length: 12 }, (_, index) => `line-${index + 1}: ${(value as { ok: boolean }).ok}`).join("\n")}\x1b[2J`;
    });
    const content = [{ type: "text", text: '{"ok":true}' }];
    const state = {};
    const first = renderers.renderResult({ content }, { expanded: false, isPartial: false }, undefined, context({ state })) as FakeText;
    const again = renderers.renderResult({ content }, { expanded: false, isPartial: false }, undefined, context({ state, lastComponent: first })) as FakeText;
    const collapsedText = again.text;
    const expanded = renderers.renderResult({ content }, { expanded: true, isPartial: false }, undefined, context({ state, lastComponent: again })) as FakeText;

    assert.equal(again, first);
    assert.equal(collapsedText.split("\n").length, 10);
    assert.equal(expanded.text.split("\n").length, 12);
    assert.equal(serializations, 2, "only expansion transition reparses/serializes the result");
    assert.ok(strips() >= 2, "custom YAML text is stripped through Pi's terminal-control seam");
    expanded.render(20);
    expanded.render(40);
    assert.equal(expanded.layoutCalls, 2, "width changes are delegated to native Text layout");
  });

  test("keeps errors, scalars, prose, later text blocks, images, and partial results on Pi's native fallback", () => {
    const { tui } = fakeTui();
    const renderers = createToolPreviewRenderers(tui);
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
        () => renderers.renderResult({ content: item.content }, { expanded: false, isPartial: item.partial }, undefined, context({ isError: item.error })),
        UseNativeResultFallback,
      );
    }
  });
});
