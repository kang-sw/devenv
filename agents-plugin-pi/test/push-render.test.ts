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
 * `pi-tui` is not resolvable from this package under `node --test` (it is
 * nested inside `pi-coding-agent`'s own `node_modules`), which is exactly why
 * the import is dynamic and guarded — so `buildPushComponent` and
 * `registerPushMessageRenderers` are driven here with an injected duck-typed
 * stand-in, and `loadPushTuiModules` is asserted to degrade to `undefined`
 * rather than throw (same convention as overlay-chat.test.ts's
 * `loadMarkdownRenderer` case).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  buildPushComponent,
  buildPushRenderLines,
  loadPushTuiModules,
  registerPushMessageRenderers,
  type PushTuiModules,
} from "../src/push-render.ts";
import { buildPushContent, PUSH_FAMILIES } from "../src/spawner.ts";

describe("buildPushRenderLines", () => {
  test("splits a real pushed message into head, payload and status", () => {
    const status = "1 of 2 delegated agents still running: w2";
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
    const content = ["[ws-agent-settled] agent a1", "reason: idle", "0 of 1 delegated agent still running"].join("\n");
    const parts = buildPushRenderLines({ content });
    assert.equal(parts?.status, "0 of 1 delegated agent still running");
    assert.deepEqual(parts?.body, ["reason: idle"]);
  });

  test("a payload line that merely looks like prose is never mistaken for the status line", () => {
    const content = ["[ws-agent-report] agent a1", "report: three of five checks still running"].join("\n");
    const parts = buildPushRenderLines({ content });
    assert.equal(parts?.status, undefined);
    assert.deepEqual(parts?.body, ["report: three of five checks still running"]);
  });

  test("details.status wins over the shape guess, so a report QUOTING a status line is not eaten", () => {
    const status = "0 of 1 delegated agent still running";
    const content = ["[ws-agent-report] agent a1", "report: the sub-lead saw `2 of 3 delegated agents still running`", status].join("\n");
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

/** Duck-typed `pi-tui` stand-in: records what the component was built out of. */
function fakeTui(): { modules: PushTuiModules; boxes: Array<{ padding: number[]; children: string[] }> } {
  const boxes: Array<{ padding: number[]; children: string[] }> = [];
  class FakeText {
    text: string;
    constructor(text = "") {
      this.text = text;
    }
  }
  class FakeBox {
    private readonly own: { padding: number[]; children: string[] };
    constructor(paddingX = 0, paddingY = 0) {
      this.own = { padding: [paddingX, paddingY], children: [] };
      boxes.push(this.own);
    }
    addChild(child: unknown): void {
      this.own.children.push((child as FakeText).text);
    }
    render(): string[] {
      return this.own.children;
    }
  }
  return { modules: { Box: FakeBox, Text: FakeText } as unknown as PushTuiModules, boxes };
}

describe("buildPushComponent", () => {
  const status = "1 of 2 delegated agents still running: w2";
  const message = {
    content: buildPushContent("ws-agent-report", "w1", { kind: "final", report: "Outcome: done" }, status),
    details: { status },
  };

  test("draws the family head ONCE — Pi's default printed it above an identical content line", () => {
    const tui = fakeTui();
    const theme = { fg: (color: string, text: string) => `<${color}>${text}` };
    buildPushComponent(tui.modules, message, theme);

    assert.equal(tui.boxes.length, 1);
    assert.deepEqual(tui.boxes[0].children, [
      "<customMessageLabel>[ws-agent-report] agent w1",
      "<customMessageText>kind: final",
      "<customMessageText>report: Outcome: done",
      `<dim>${status}`,
    ]);
    assert.deepEqual(tui.boxes[0].padding, [1, 0], "compact: one column of padding, no blank rows");
  });

  test("no theme (and a throwing theme) degrade to unpainted text rather than to no component", () => {
    const plain = fakeTui();
    assert.ok(buildPushComponent(plain.modules, message, undefined));
    assert.equal(plain.boxes[0].children[0], "[ws-agent-report] agent w1");

    const broken = fakeTui();
    assert.ok(
      buildPushComponent(broken.modules, message, {
        fg: () => {
          throw new Error("theme is gone");
        },
      }),
    );
    assert.equal(broken.boxes[0].children[0], "[ws-agent-report] agent w1");
  });

  test("a status-less message draws no status row", () => {
    const tui = fakeTui();
    buildPushComponent(tui.modules, { content: buildPushContent("ws-agent-orphaned", undefined, { count: 2 }, undefined) }, undefined);
    assert.deepEqual(tui.boxes[0].children, ["[ws-agent-orphaned]", "count: 2"]);
  });

  test("an unrecognizable message returns undefined so Pi's default rendering stands", () => {
    const tui = fakeTui();
    assert.equal(buildPushComponent(tui.modules, { content: "" }, undefined), undefined);
    assert.deepEqual(tui.boxes, [], "no half-built box is left behind");
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
    );
    assert.ok(rendered);
    assert.deepEqual(tui.boxes[0].children, ["[ws-agent-settled] agent a1", "reason: idle"]);
  });

  test("with pi-tui unavailable nothing is registered and the caller is told the default stands", async () => {
    const pi = { registerMessageRenderer: () => assert.fail("nothing may be registered without a component library") };
    assert.equal(await registerPushMessageRenderers(pi as never), false);
  });

  test("loadPushTuiModules resolves to undefined under node --test (pi-tui is not resolvable here) instead of throwing", async () => {
    assert.equal(await loadPushTuiModules(), undefined);
  });
});
