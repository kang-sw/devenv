/**
 * Unit tests for `tool-row-render.ts` — the five dispatch tools' per-tool
 * call-summary builders, the shared resolved-model-line formatter, and the
 * `createDispatchToolPreview` factory that wires both into
 * `tool-result-render.ts`'s `ToolPreviewOverrides` seam.
 *
 * Reuses the `fakeTui()`/`FakeText`/`FakeBox` harness pattern from
 * `test/tool-result-render.test.ts` rather than re-deriving one.
 *
 * Run with: node --test test/tool-row-render.test.ts (from agents-plugin-pi/).
 */
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  buildAgentSendSummary,
  buildAgentSpawnSummary,
  buildExecuteSummary,
  buildExploreSummary,
  buildForkSummary,
  createDispatchToolPreview,
  formatResolvedLine,
  truncateHead,
  type ResolvedModelInfo,
} from "../src/tool-row-render.ts";
import { createToolPreviewTuiRef, UseNativeResultFallback, type ToolResultTuiModules } from "../src/tool-result-render.ts";

class FakeText {
  text = "";
  private width: number | undefined;
  private lines: string[] | undefined;
  setText(text: string): void {
    this.text = text;
    this.width = undefined;
    this.lines = undefined;
  }
  render(width: number): string[] {
    if (this.width !== width || !this.lines) {
      this.width = width;
      this.lines = this.text.split("\n");
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
  addChild(component: { render(width: number): string[]; invalidate(): void }): void {
    this.children.push(component);
  }
  setBgFn(): void {}
  render(width: number): string[] {
    return this.children.flatMap((child) => child.render(width));
  }
  invalidate(): void {
    for (const child of this.children) child.invalidate();
  }
}

function fakeTui(): ToolResultTuiModules {
  return {
    Text: FakeText,
    Box: FakeBox,
    stripTerminalSequences: (text: string) => text,
    truncateToWidth: (text: string) => text,
  };
}

const unstyledTheme = {
  fg: (_color: string, text: string) => text,
  bold: (text: string) => text,
};

function context(overrides: Partial<{ state: object; lastComponent: unknown; argsComplete: boolean; isPartial: boolean; isError: boolean }> = {}) {
  return {
    state: overrides.state ?? {},
    lastComponent: overrides.lastComponent,
    argsComplete: overrides.argsComplete ?? true,
    isPartial: overrides.isPartial ?? false,
    isError: overrides.isError ?? false,
  };
}

describe("truncateHead", () => {
  test("passes short text through unchanged", () => {
    assert.equal(truncateHead("short", 60), "short");
  });

  test("head-truncates over-cap text with the ellipsis marker", () => {
    const long = "x".repeat(70);
    const result = truncateHead(long, 60);
    assert.equal(result.length, 61);
    assert.ok(result.endsWith("…"));
  });

  test("never throws on undefined/malformed input — defaults to empty string", () => {
    assert.equal(truncateHead(undefined, 60), "");
    assert.equal(truncateHead(123 as unknown as string, 60), "");
  });
});

describe("formatResolvedLine", () => {
  test("a tier hit renders the tier name, model, and effort", () => {
    const resolved: ResolvedModelInfo = { tier: "small", model: "openai-codex/gpt-5.6-high", effort: "high", inherited: false };
    assert.equal(formatResolvedLine(resolved), "→ small · openai-codex/gpt-5.6-high · effort high");
  });

  test("a complex:true inherit renders the literal 'inherit' tier", () => {
    const resolved: ResolvedModelInfo = { tier: "inherit", model: "anthropic/opus", effort: "medium", inherited: true };
    assert.equal(formatResolvedLine(resolved), "→ inherit · anthropic/opus · effort medium");
  });

  test("an omitted-tier 'miss' (no resolved model) renders a '?' placeholder", () => {
    const resolved: ResolvedModelInfo = { tier: "inherit", model: undefined, effort: undefined, inherited: true };
    assert.equal(formatResolvedLine(resolved), "→ inherit · ? · effort pi-default");
  });

  test("a no-thinking-level dispatch renders 'pi-default' for an empty effort", () => {
    const resolved: ResolvedModelInfo = { tier: "small", model: "provider/id", effort: "", inherited: false };
    assert.equal(formatResolvedLine(resolved), "→ small · provider/id · effort pi-default");
  });

  test("an undefined resolved value renders undefined (nothing published yet)", () => {
    assert.equal(formatResolvedLine(undefined), undefined);
  });
});

describe("per-tool call summary builders", () => {
  test("buildExploreSummary: representative and empty/partial args", () => {
    assert.equal(buildExploreSummary({ query: "why does this fail" }), "query: why does this fail");
    assert.equal(buildExploreSummary({}), "query: ");
    assert.equal(buildExploreSummary(undefined), "query: ");
  });

  test("buildExecuteSummary: command when given, then prompt head, then a complex tag only when true", () => {
    assert.equal(buildExecuteSummary({ command: "ls -la", prompt: "list files", complex: true }), "command: ls -la\nprompt: list files\ncomplex: true");
    assert.equal(buildExecuteSummary({ prompt: "no command given" }), "prompt: no command given");
    assert.equal(buildExecuteSummary({ prompt: "not complex", complex: false }), "prompt: not complex");
  });

  test("buildAgentSpawnSummary: alias/title when given, system_prompt_path basename, prompt head, model_name/model_effort", () => {
    assert.equal(
      buildAgentSpawnSummary({
        alias: "researcher-1",
        title: "Researcher",
        system_prompt_path: "/tmp/playbooks/implementer.md",
        prompt: "implement the thing",
        model_name: "small",
        model_effort: "high",
      }),
      "alias: researcher-1\ntitle: Researcher\nsystem_prompt: implementer.md\nprompt: implement the thing\nmodel_name: small\nmodel_effort: high",
    );
    assert.equal(
      buildAgentSpawnSummary({ system_prompt_path: "/tmp/p.md", prompt: "hi" }),
      "system_prompt: p.md\nprompt: hi",
    );
  });

  test("buildAgentSendSummary: target alias/id, message head, interrupt tag only when true", () => {
    assert.equal(buildAgentSendSummary({ agent_id: "agent-1", message: "steer this way", interrupt: true }), "target: agent-1\nmessage: steer this way\ninterrupt: true");
    assert.equal(buildAgentSendSummary({ agent_id: "agent-2", message: "queued" }), "target: agent-2\nmessage: queued");
  });

  test("buildForkSummary: prompt head, model_name, expects_commit tag only when true", () => {
    assert.equal(buildForkSummary({ prompt: "work on this", model_name: "large", expects_commit: true }), "prompt: work on this\nmodel_name: large\nexpects_commit: true");
    assert.equal(buildForkSummary({ prompt: "no model given" }), "prompt: no model given");
  });

  test("every builder never throws on undefined arguments", () => {
    for (const builder of [buildExploreSummary, buildExecuteSummary, buildAgentSpawnSummary, buildAgentSendSummary, buildForkSummary]) {
      assert.doesNotThrow(() => builder(undefined));
      assert.doesNotThrow(() => builder(null));
      assert.doesNotThrow(() => builder("malformed" as unknown));
      assert.doesNotThrow(() => builder([] as unknown));
    }
  });
});

describe("createDispatchToolPreview", () => {
  test("cold tuiRef still throws UseNativeResultFallback for both hooks", () => {
    const ref = createToolPreviewTuiRef();
    const preview = createDispatchToolPreview(ref, "explore", buildExploreSummary);
    assert.throws(() => preview.renderCall({ query: "x" }, unstyledTheme, context()), UseNativeResultFallback);
    assert.throws(
      () => preview.renderResult({ content: [{ type: "text", text: "{}" }] }, { expanded: false, isPartial: false }, unstyledTheme, context()),
      UseNativeResultFallback,
    );
  });

  test("renderCall renders the tool's own summary once the ref is warm", () => {
    const ref = createToolPreviewTuiRef();
    ref.current = fakeTui();
    const preview = createDispatchToolPreview(ref, "explore", buildExploreSummary);
    const call = preview.renderCall({ query: "why does this fail" }, unstyledTheme, context());
    assert.match(call.render(80).join("\n"), /query: why does this fail/);
  });

  test("resolved line renders across partial, success, and error results alike", () => {
    const ref = createToolPreviewTuiRef();
    ref.current = fakeTui();
    const preview = createDispatchToolPreview(ref, "ws-agent-spawn", buildAgentSpawnSummary);
    const state = {};
    const resolved: ResolvedModelInfo = { tier: "small", model: "provider/id", effort: "high", inherited: false };

    // Partial: no content yet, but details.resolved is already published.
    const partial = preview.renderResult({ content: [], details: { resolved } }, { expanded: false, isPartial: true }, unstyledTheme, context({ state }));
    assert.match(partial.render(80).join("\n"), /→ small · provider\/id · effort high/);

    // Success: body renders beneath the (unchanged) resolved line.
    const success = preview.renderResult(
      { content: [{ type: "text", text: '{"agent_id":"a1"}' }], details: { resolved } },
      { expanded: false, isPartial: false },
      unstyledTheme,
      context({ state, lastComponent: partial }),
    );
    const successText = success.render(80).join("\n");
    assert.match(successText, /→ small · provider\/id · effort high/);
    assert.match(successText, /agent_id: a1/);

    // Error: no details.resolved on this particular call, but the previously
    // cached line (the cross-call cache path) must still show.
    const error = preview.renderResult(
      { content: [{ type: "text", text: "boom" }] },
      { expanded: false, isPartial: false },
      unstyledTheme,
      context({ state, lastComponent: success, isError: true }),
    );
    assert.match(error.render(80).join("\n"), /→ small · provider\/id · effort high/);
  });

  test("a long completed explore answer body is still capped at the shared row budget, beneath the resolved line", () => {
    // Review relay #1 (Important): the ticket's "Tests:" item asks for a
    // long synchronous explore ANSWER (the result BODY, not the query) to be
    // trimmed to the row budget below the resolved line. The pre-existing
    // "resolved line renders across partial/success/error" test above only
    // ever feeds a short body ({"agent_id":"a1"}), so it never exercises
    // tool-result-render.ts's PREVIEW_ROWS(10)/truncatedMarker cap through
    // this new resolvedLine/hasBody path — only that the two can coexist at
    // all. This drives a genuinely long (15-line) RAW body through it.
    const ref = createToolPreviewTuiRef();
    ref.current = fakeTui();
    const preview = createDispatchToolPreview(ref, "explore", buildExploreSummary);
    const resolved: ResolvedModelInfo = { tier: "small", model: "provider/id", effort: "high", inherited: false };
    const longAnswer = Array.from({ length: 15 }, (_, i) => `line-${i}`).join("\n");

    const result = preview.renderResult(
      { content: [{ type: "text", text: longAnswer }], details: { resolved } },
      { expanded: false, isPartial: false },
      unstyledTheme,
      context(),
    );
    const lines = result.render(80).join("\n").split("\n");

    // Row 0 is the mandatory resolved line; row 1 is the blank separator
    // shared with the plain (non-resolved) body layout below it.
    assert.match(lines[0]!, /→ small · provider\/id · effort high/);
    assert.equal(lines[1], "");

    // The body itself: the cap does not count the resolved line or the
    // separator against its own 10-row budget — exactly 10 content rows
    // plus one truncation marker row are expected beneath them.
    const bodyLines = lines.slice(2);
    assert.equal(bodyLines.length, 11, "10 capped content rows plus one truncation marker row");
    for (let i = 0; i < 10; i++) {
      assert.match(bodyLines[i]!, new RegExp(`line-${i}$`), `row ${i} of the body must be the ${i}-th input line`);
    }
    assert.equal(bodyLines[10], "...", "the 11th body row is the truncation marker, not an 11th content line");
    assert.ok(!lines.join("\n").includes("line-10"), "line-10 (the 11th input line) must be cut by the cap — proving it was actually applied, not just present under a big enough budget");
  });

  test("with no resolved line ever published and no body available, still falls back to native", () => {
    const ref = createToolPreviewTuiRef();
    ref.current = fakeTui();
    const preview = createDispatchToolPreview(ref, "ws-fork", buildForkSummary);
    assert.throws(
      () => preview.renderResult({ content: [] }, { expanded: false, isPartial: true }, unstyledTheme, context()),
      UseNativeResultFallback,
    );
  });
});
