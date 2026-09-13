/**
 * Unit tests for `audit.ts` — the `260908` subagent audit window's Phase 1
 * surface: the session-file parser, the picker's row builder, the
 * view-only `ConversationChannel`, the registration gate, and the viewer
 * overlay's Esc/Enter contract.
 *
 * `parseSessionFile`/`buildAuditPickerItems`/`createAuditChannel`/
 * `shouldRegisterAudit` are pure (or IO-light, best-effort) and driven
 * directly. `openViewer`/`openPicker`/`registerAuditCommands` are the live
 * glue — driven through a fake `pi` (`registerCommand`/`registerShortcut`
 * capturing `Map`s, mirroring `test/native-tool-registration.test.ts`'s
 * harness) and a fake `ctx.ui.custom` (mirroring `test/ask.test.ts`'s
 * `fakeTui`/`fakeChannel` fakes, extended with a `ui.custom` stub) — no
 * live TTY, no real Pi host beyond the package's own `@earendil-works/pi-tui`
 * dependency that `loadHostPiTui()` resolves at runtime.
 *
 * Run with: node --test test/audit.test.ts  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import {
  buildAuditPickerItems,
  createAuditChannel,
  openPicker,
  openViewer,
  readSessionHistory,
  parseSessionFile,
  registerAuditCommands,
  shouldRegisterAudit,
} from "../src/audit.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "../src/spawner.ts";
import type { ConversationViewComponent } from "../src/conversation-view.ts";
import { visibleWidth } from "../src/text-width.ts";

function record(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId: "11111111-2222-3333-4444-555555555555",
    sessionPath: join(tmpdir(), "ws-pi-audit-test-nonexistent-session.jsonl"),
    wsToolNames: [],
    toolGroup: "full-worker",
    streaming: false,
    running: false,
    reportLog: [],
    ...overrides,
  };
}

function registryOf(...records: RpcAgentRecord[]): RpcAgentRegistry {
  const map: RpcAgentRegistry = new Map();
  for (const r of records) map.set(r.agentId, r);
  return map;
}

function fakePi() {
  const commands = new Map<string, { description?: string; handler: (args: string, ctx: unknown) => Promise<void> }>();
  const shortcuts = new Map<string, { description?: string; handler: (ctx: unknown) => Promise<void> }>();
  const pi = {
    registerCommand: (name: string, options: unknown) => commands.set(name, options as never),
    registerShortcut: (key: string, options: unknown) => shortcuts.set(key, options as never),
  } as unknown as ExtensionAPI;
  return { pi, commands, shortcuts };
}

/**
 * A fake `ctx.ui.custom`: synchronously invokes `factory` (capturing its
 * resolved component through `componentReady`, regardless of whether/when
 * `done` is ever called) and ties the outer `Promise<T>` this call returns
 * to `done` — the exact same coupling the real `ExtensionUIContext.custom`
 * makes. `close()` resolves it from the outside, standing in for an owner
 * Esc that never happens in a given test.
 */
interface PickerComponent {
  render(width: number): string[];
  handleInput?(data: string): void;
}

function fakePickerCtx(theme?: { fg?(color: string, text: string): string }) {
  let resolveComponent!: (c: PickerComponent) => void;
  const componentReady = new Promise<PickerComponent>((res) => {
    resolveComponent = res;
  });
  let renderCount = 0;
  const ctx = {
    mode: "tui",
    ui: {
      notify: () => {},
      custom: (factory: (...args: unknown[]) => unknown) =>
        new Promise((resolve) => {
          const built = factory({ requestRender: () => { renderCount += 1; } }, theme, undefined, resolve);
          Promise.resolve(built).then((component) => resolveComponent(component as PickerComponent));
        }),
    },
  };
  return { ctx, componentReady, get renderCount() { return renderCount; } };
}

function fakeViewerCtx(theme?: { bg?(color: string, text: string): string; fg?(color: string, text: string): string }) {
  let resolveComponent!: (c: ConversationViewComponent) => void;
  const componentReady = new Promise<ConversationViewComponent>((res) => {
    resolveComponent = res;
  });
  let doneFn: ((v: unknown) => void) | undefined;
  const ctx = {
    mode: "tui",
    ui: {
      notify: () => {},
      custom: (factory: (...args: unknown[]) => unknown) =>
        new Promise((resolve) => {
          doneFn = resolve;
          const built = factory({ requestRender: () => {} }, theme, undefined, (v: unknown) => resolve(v));
          Promise.resolve(built).then((component) => resolveComponent(component as ConversationViewComponent));
        }),
    },
  };
  return { ctx, componentReady, close: () => doneFn?.(undefined) };
}

describe("parseSessionFile", () => {
  function fixturePath(lines: string[]): string {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-audit-session-"));
    const path = join(dir, "session.jsonl");
    writeFileSync(path, lines.join("\n"));
    return path;
  }

  test("maps user (as lead-message), assistant text/toolCall, and toolResult — skipping thinking blocks, non-chat entries, non-chat roles, and a malformed line", () => {
    const lines = [
      `{"type":"session","version":3,"id":"s1","timestamp":"2026-09-09T00:00:00.000Z","cwd":"/tmp"}`,
      `{"type":"message","id":"a","parentId":null,"timestamp":"2026-09-09T00:00:01.000Z","message":{"role":"user","content":"do the thing","timestamp":0}}`,
      `{"type":"message","id":"b","parentId":"a","timestamp":"2026-09-09T00:00:02.000Z","message":{"role":"user","content":[{"type":"text","text":"part1"},{"type":"image","data":"xx","mimeType":"image/png"},{"type":"text","text":"part2"}],"timestamp":0}}`,
      `{"type":"model_change","id":"m1","parentId":"b","timestamp":"2026-09-09T00:00:03.000Z","provider":"openai","modelId":"gpt-4o"}`,
      `{"type":"message","id":"c","parentId":"m1","timestamp":"2026-09-09T00:00:04.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm, let's see"},{"type":"text","text":"Let me check."},{"type":"toolCall","id":"call1","name":"bash","arguments":{"cmd":"ls"}}],"provider":"anthropic","model":"claude","usage":{},"stopReason":"toolUse","timestamp":0}}`,
      `{"type":"message","id":"d","parentId":"c","timestamp":"2026-09-09T00:00:05.000Z","message":{"role":"toolResult","toolCallId":"call1","toolName":"bash","content":[{"type":"text","text":"total 0"}],"isError":false,"timestamp":0}}`,
      `{"type":"compaction","id":"comp1","parentId":"d","timestamp":"2026-09-09T00:00:06.000Z","summary":"...","tokensBefore":100}`,
      `{"type":"message","id":"e","parentId":"comp1","timestamp":"2026-09-09T00:00:07.000Z","message":{"role":"bashExecution","command":"ls","output":"","exitCode":0,"cancelled":false,"truncated":false,"timestamp":0}}`,
      `not-json-at-all-should-be-skipped`,
      ``,
    ];
    const items = parseSessionFile(fixturePath(lines));
    assert.deepEqual(items, [
      { kind: "lead-message", text: "do the thing" },
      { kind: "lead-message", text: "part1part2" },
      { kind: "assistant", text: "Let me check." },
      { kind: "tool-call", id: "call1", name: "bash", args: { cmd: "ls" } },
      { kind: "tool-result", id: "call1", name: "bash", content: "total 0", isError: false },
    ]);
  });

  test("attributes persisted user entries from the ordered owner-send log and leaves other prompts lead-authored", () => {
    const path = fixturePath([
      `{"type":"message","message":{"role":"user","content":"lead prompt"}}`,
      `{"type":"message","message":{"role":"user","content":"owner one"}}`,
      `{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"reply"}]}}`,
      `{"type":"message","message":{"role":"user","content":"owner two"}}`,
    ]);
    assert.deepEqual(readSessionHistory(path, [
      { text: "owner one", at: 1 },
      { text: "owner two", at: 2 },
    ]).items, [
      { kind: "lead-message", text: "lead prompt" },
      { kind: "user", text: "owner one" },
      { kind: "assistant", text: "reply" },
      { kind: "user", text: "owner two" },
    ]);
  });

  test("a missing/unreadable session file is an explicit unavailable state while the compatibility parser stays empty", () => {
    const missing = join(tmpdir(), "ws-pi-audit-definitely-missing-", `${Date.now()}.jsonl`);
    assert.deepEqual(readSessionHistory(missing), { status: "unavailable", items: [] });
    assert.deepEqual(parseSessionFile(missing), []);
  });

  test("an empty file yields []", () => {
    assert.deepEqual(parseSessionFile(fixturePath([])), []);
  });
});

describe("buildAuditPickerItems", () => {
  test("uses alias-or-short-ID identity and preserves all four status tiers and ordering", () => {
    const NOW = Date.parse("2026-09-09T10:00:00.000Z");
    const owner = record({ agentId: "owner-agent-id", alias: "scout", threadBound: true, runStartedAt: NOW - 180_000, telemetry: { version: 1, origin: { sessionId: "owner", sessionPath: "/tmp/owner", emptyPrefix: true }, model: "gpt-5.6-terra", contextTokens: 0 } });
    const approval = record({ agentId: "appr-agent-id", title: "ignored title", pendingApproval: { cmdId: "c1", command: "rm -rf /" }, runStartedAt: NOW - 120_000, observedModel: "stored-model", observedContextTokens: 132_400 });
    const runningOld = record({ agentId: "run-old-id", alias: "old-runner", client: {} as never, runStartedAt: NOW - 60_000, telemetry: { version: 1, origin: { sessionId: "old", sessionPath: "/tmp/old", emptyPrefix: true }, model: "large-model", contextTokens: 1_354_100 } });
    const runningNew = record({ agentId: "run-new-id", alias: "new-runner", client: {} as never, runStartedAt: NOW - 5_000 });
    const dormantRecent = record({ agentId: "dorm-recent-id", alias: "recent-dormant", lastLeadPromptAt: NOW - 1_000 });
    const dormantOld = record({ agentId: "dorm-old-id", alias: "old-dormant", lastLeadPromptAt: NOW - 100_000 });

    // Insertion order deliberately scrambled — the function must sort, not preserve Map order.
    const registry = registryOf(dormantOld, runningOld, approval, dormantRecent, owner, runningNew);
    assert.deepEqual(buildAuditPickerItems(registry, NOW), [
      { value: "owner-agent-id", label: "scout · awaiting owner · gpt-5.6-terra · ctx 0.0k · running for 3m" },
      { value: "appr-agent-id", label: "ignored title · awaiting approval · stored-model · ctx 132.4k · running for 2m" },
      { value: "run-old-id", label: "old-runner · running · large-model · ctx 1354.1k · running for 1m" },
      { value: "run-new-id", label: "new-runner · running · — · ctx ? · running for 5s" },
      { value: "dorm-recent-id", label: "recent-dormant · dormant · — · ctx ? · last active 1s ago" },
      { value: "dorm-old-id", label: "old-dormant · dormant · — · ctx ? · last active 1m ago" },
    ]);
  });

  test("clamps future run and activity timestamps to zero-duration labels", () => {
    const NOW = Date.parse("2026-09-09T10:00:00.000Z");
    const running = record({ agentId: "future-running-id", client: {} as never, runStartedAt: NOW + 60_000 });
    const dormant = record({ agentId: "future-dormant-id", lastLeadPromptAt: NOW + 60_000 });
    assert.deepEqual(buildAuditPickerItems(registryOf(running, dormant), NOW), [
      { value: "future-running-id", label: "future-r · running · — · ctx ? · running for 0s" },
      { value: "future-dormant-id", label: "future-d · dormant · — · ctx ? · last active 0s ago" },
    ]);
  });

  test("dormant activity labels make missing, non-positive, and invalid timestamps unknown while preserving valid old and future values", () => {
    const NOW = Date.parse("2026-09-09T10:00:00.000Z");
    const labelFor = (overrides: Partial<RpcAgentRecord>) => buildAuditPickerItems(registryOf(record(overrides)), NOW)[0]!.label;

    assert.match(labelFor({ agentId: "missing-id" }), /last active —$/);
    assert.match(labelFor({ agentId: "zero-id", lastLeadPromptAt: 0 }), /last active —$/);
    assert.match(labelFor({ agentId: "nan-id", lastLeadPromptAt: Number.NaN }), /last active —$/);
    assert.match(labelFor({ agentId: "invalid-id", lastReportAtOverride: "not-a-timestamp" }), /last active —$/);
    assert.match(labelFor({ agentId: "old-id", lastLeadPromptAt: NOW - 90_000 }), /last active 1m ago$/);
    assert.match(labelFor({ agentId: "future-id", lastLeadPromptAt: NOW + 60_000 }), /last active 0s ago$/);
  });

  test("a plain idle record with no client/threadBound/pendingApproval is dormant (tier 4), never one of the first three tiers", () => {
    const NOW = Date.parse("2026-09-09T10:00:00.000Z");
    const idle = record({ agentId: "idle-1", lastLeadPromptAt: NOW });
    assert.deepEqual(buildAuditPickerItems(registryOf(idle), NOW), [
      { value: "idle-1", label: "idle-1 · dormant · — · ctx ? · last active 0s ago" },
    ]);
  });

  test("an owner-held settled record is an idle-awaiting-owner live tier, not dormant", () => {
    const NOW = Date.parse("2026-09-09T10:00:00.000Z");
    const held = record({ agentId: "held-id", alias: "held", lastWriter: "owner", ownerSends: [{ text: "look again", at: NOW - 2_000 }] });
    assert.deepEqual(buildAuditPickerItems(registryOf(held), NOW), [
      { value: "held-id", label: "held · idle awaiting owner · — · ctx ? · last active 2s ago" },
    ]);
  });

  test("an empty registry yields no items", () => {
    assert.deepEqual(buildAuditPickerItems(new Map(), Date.now()), []);
  });
});

describe("createAuditChannel", () => {
  test("a dormant record (no client) never attaches to anything — opening it resumes nothing", () => {
    const registry = registryOf(record({ agentId: "a1" }));
    const channel = createAuditChannel(registry, "a1");
    let received: unknown = "untouched";
    const unsubscribe = channel.onEvent((evt) => {
      received = evt;
    });
    assert.equal(channel.liveness(), "settled");
    unsubscribe();
    assert.equal(received, "untouched", "no client existed to ever fire an event through");
  });

  test("a live record's events reach the channel's listeners — the tail appends as they arrive", () => {
    let capturedListener: ((evt: unknown) => void) | undefined;
    const client = {
      onEvent: (listener: (evt: unknown) => void) => {
        capturedListener = listener;
        return () => {
          capturedListener = undefined;
        };
      },
    };
    const registry = registryOf(record({ agentId: "a1", client: client as never, streaming: true, running: true }));
    const channel = createAuditChannel(registry, "a1");
    assert.equal(channel.liveness(), "running");

    const events: unknown[] = [];
    channel.onEvent((evt) => events.push(evt));
    assert.ok(capturedListener, "onEvent attaches to the live client");
    capturedListener?.({ type: "tool_execution_start", toolCallId: "c1", toolName: "bash", args: {} });
    capturedListener?.({ type: "tool_execution_end", toolCallId: "c1", toolName: "bash", result: { content: [{ type: "text", text: "ok" }] }, isError: false });
    assert.deepEqual(events, [
      { type: "tool_execution_start", toolCallId: "c1", toolName: "bash", args: {} },
      { type: "tool_execution_end", toolCallId: "c1", toolName: "bash", result: { content: [{ type: "text", text: "ok" }] }, isError: false },
    ]);
  });

  test("a dormant owner send resyncs the event subscription to the resumed client", async () => {
    const r = record({ agentId: "a1" });
    const registry = registryOf(r);
    let attached: ((evt: unknown) => void) | undefined;
    const channel = createAuditChannel(registry, "a1", async () => {
      r.client = { onEvent: (listener: (evt: unknown) => void) => { attached = listener; return () => {}; } } as never;
    });
    const events: unknown[] = [];
    channel.onEvent((evt) => events.push(evt));
    assert.equal(attached, undefined);
    await channel.send?.("resume");
    assert.ok(attached);
    attached?.({ type: "message_update" });
    assert.deepEqual(events, [{ type: "message_update" }]);
  });

  test("liveness reads record.running and last-writer ownership fresh each call", () => {
    const r = record({ agentId: "a1", running: false });
    const registry = registryOf(r);
    const channel = createAuditChannel(registry, "a1");
    assert.equal(channel.liveness(), "settled");
    r.lastWriter = "owner";
    assert.equal(channel.liveness(), "idle-awaiting-owner");
    r.running = true;
    assert.equal(channel.liveness(), "running");
  });
});

describe("shouldRegisterAudit (lead-only, TUI-only gate — stricter than isLeadOrFork)", () => {
  const roles = [undefined, "worker", "fork", "explore"] as const;
  const modes = ["tui", "headless", undefined] as const;
  for (const role of roles) {
    for (const mode of modes) {
      const expected = role === undefined && mode === "tui";
      test(`role=${String(role)} mode=${String(mode)} -> ${expected}`, () => {
        assert.equal(shouldRegisterAudit(role, mode), expected);
      });
    }
  }
});

describe("registerAuditCommands", () => {
  test("registers neither the command nor the shortcut when the gate is false", () => {
    const pi = {
      registerCommand: () => {
        throw new Error("must not register in a non-lead/non-tui session");
      },
      registerShortcut: () => {
        throw new Error("must not register in a non-lead/non-tui session");
      },
    } as unknown as ExtensionAPI;
    assert.doesNotThrow(() => registerAuditCommands(pi, new Map(), "worker", "tui"));
    assert.doesNotThrow(() => registerAuditCommands(pi, new Map(), "fork", "tui"));
    assert.doesNotThrow(() => registerAuditCommands(pi, new Map(), undefined, "headless"));
  });

  test("registers both /audit and its shortcut when the gate is true", () => {
    const { pi, commands, shortcuts } = fakePi();
    registerAuditCommands(pi, new Map(), undefined, "tui");
    assert.ok(commands.has("audit"));
    assert.ok(shortcuts.has("ctrl+shift+u"));
  });

  test("the shortcut opens the same picker as /audit with no id", async () => {
    const registry = registryOf(record({ agentId: "a1", alias: "scout", client: {} as never, runStartedAt: Date.now() - 1_000 }));
    const { pi, commands, shortcuts } = fakePi();
    registerAuditCommands(pi, registry, undefined, "tui");

    async function capturePicker(invoke: (ctx: unknown) => Promise<void>): Promise<string[]> {
      const opened = fakePickerCtx();
      const command = invoke(opened.ctx);
      const component = await opened.componentReady;
      const lines = component.render(40);
      component.handleInput?.("\x1b");
      await command;
      return lines;
    }

    const fromCommand = await capturePicker((ctx) => commands.get("audit")!.handler("", ctx));
    const fromShortcut = await capturePicker((ctx) => shortcuts.get("ctrl+shift+u")!.handler(ctx));
    assert.deepEqual(fromCommand, fromShortcut);
  });

  test("the picker frames its SelectList and delegates selection and cancellation", async () => {
    const registry = registryOf(
      record({ agentId: "a1", alias: "scout", client: {} as never, runStartedAt: Date.now() - 2_000 }),
      record({ agentId: "a2", alias: "reviewer", client: {} as never, runStartedAt: Date.now() - 1_000 }),
    );

    const selected = fakePickerCtx();
    const selectedResult = openPicker(selected.ctx as never, registry);
    const selectedComponent = await selected.componentReady;
    for (const width of [8, 40]) {
      const lines = selectedComponent.render(width);
      assert.ok(lines[0]?.startsWith("┌") && lines[0]?.endsWith("┐"), `top border at width ${width}`);
      assert.ok(lines.at(-1)?.startsWith("└") && lines.at(-1)?.endsWith("┘"), `bottom border at width ${width}`);
      if (width === 40) assert.ok(lines.some((line) => line.includes("ws audit: select subagent")), "audit picker header");
      for (const line of lines) assert.ok(visibleWidth(line) <= width, `picker line exceeds ${width}: ${JSON.stringify(line)}`);
    }
    selectedComponent.handleInput?.("\x1b[B");
    selectedComponent.handleInput?.("\r");
    assert.equal(await selectedResult, "a2", "down then Enter selects through the framed wrapper");
    assert.ok(selected.renderCount >= 2, "delegated input requests repaint");

    const cancelled = fakePickerCtx();
    const cancelledResult = openPicker(cancelled.ctx as never, registry);
    const cancelledComponent = await cancelled.componentReady;
    cancelledComponent.handleInput?.("\x1b");
    assert.equal(await cancelledResult, undefined, "Esc cancels through the framed wrapper");
  });

  test("the framed picker drops whole context/model fields before narrowing protected identity, status, and activity", async () => {
    const registry = registryOf(record({
      agentId: "orphan-copy-hotfix-id",
      alias: "orphan-copy-hotfix",
      client: {} as never,
      runStartedAt: Date.now() - 180_000,
      telemetry: { version: 1, origin: { sessionId: "picker", sessionPath: "/tmp/picker", emptyPrefix: true }, model: "provider/gpt-5.6-terra", contextTokens: 132_400 },
    }));
    const opened = fakePickerCtx();
    const result = openPicker(opened.ctx as never, registry);
    const component = await opened.componentReady;
    const rendered = new Map([8, 40, 80, 120].map((width) => [width, component.render(width)]));

    for (const [width, lines] of rendered) {
      for (const line of lines) assert.ok(visibleWidth(line) <= width, `picker line exceeds ${width}: ${JSON.stringify(line)}`);
    }
    const at40 = rendered.get(40)!.join("\n");
    assert.match(at40, /running · running for 3m/, "protected status/activity survive a practical narrow picker");
    assert.doesNotMatch(at40, /provider\/gpt-5\.6-terra|132\.4k/, "narrow rows omit optional fields whole");
    const at80 = rendered.get(80)!.join("\n");
    assert.match(at80, /provider\/gpt-5\.6-terra/, "model remains while it fits");
    assert.doesNotMatch(at80, /132\.4k/, "tokens are omitted before model");
    assert.match(rendered.get(120)!.join("\n"), /orphan-copy-hotfix · running · provider\/gpt-5\.6-terra · ctx 132\.4k · running for 3m/, "wide rows retain every semantic field");

    component.handleInput?.("\x1b");
    assert.equal(await result, undefined);
  });
});

describe("openViewer (shared view/steering overlay and one-overlay-at-a-time singleton)", () => {
  test("wires host semantic muted/dim foregrounds into tool rows and the working marker", async () => {
    const path = join(mkdtempSync(join(tmpdir(), "ws-pi-audit-theme-")), "session.jsonl");
    writeFileSync(path, [
      `{"type":"message","message":{"role":"assistant","content":[{"type":"toolCall","id":"c1","name":"bash","arguments":{"cmd":"pwd"}}]}}`,
      `{"type":"message","message":{"role":"toolResult","toolCallId":"c1","toolName":"bash","content":[{"type":"text","text":"/tmp"}]}}`,
    ].join("\n"));
    const calls: Array<{ color: string; text: string }> = [];
    const theme = {
      fg: (color: string, text: string) => {
        calls.push({ color, text });
        return `\x1b[2m${text}\x1b[0m`;
      },
    };
    const registry = registryOf(record({ agentId: "a1", sessionPath: path, streaming: true, running: true }));
    const opened = fakeViewerCtx(theme);
    const promise = openViewer(opened.ctx as never, registry, "a1");
    const component = await opened.componentReady;

    component.render(120);
    assert.ok(calls.some((call) => call.color === "muted" && call.text.includes("bash")), "tool use is painted with the host's muted semantic");
    assert.ok(calls.some((call) => call.color === "dim" && call.text === "working…"), "the activity marker is painted with the host's dim semantic");

    opened.close();
    await promise;
  });

  test("renders missing pruned history as explicitly unavailable rather than an empty or parse-failed transcript", async () => {
    const registry = registryOf(record({ agentId: "gone", sessionPath: join(tmpdir(), `ws-pi-pruned-${Date.now()}.jsonl`) }));
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registry, "gone");
    const component = await opened.componentReady;
    assert.match(component.render(120).join("\n"), /history unavailable.*gone/i);
    opened.close();
    await promise;
  });

  test("uses the /answer border frame and remains width-safe at narrow and normal widths", async () => {
    const registry = registryOf(record({ agentId: "a1" }));
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registry, "a1");
    const component = await opened.componentReady;

    for (const width of [5, 40]) {
      const lines = component.render(width);
      assert.ok(lines[0]?.startsWith("┌") && lines[0]?.endsWith("┐"), `top border at width ${width}`);
      assert.ok(lines.at(-1)?.startsWith("└") && lines.at(-1)?.endsWith("┘"), `bottom border at width ${width}`);
      assert.ok(lines[1]?.startsWith("│ ") && lines[1]?.endsWith(" │"), `top breathing row at width ${width}`);
      assert.ok(lines.at(-2)?.startsWith("│ ") && lines.at(-2)?.endsWith(" │"), `bottom breathing row at width ${width}`);
      for (const line of lines) assert.ok(visibleWidth(line) <= width, `viewer line exceeds ${width}: ${JSON.stringify(line)}`);
    }

    opened.close();
    await promise;
  });

  test("Esc closes directly in view mode", async () => {
    const registry = registryOf(record({ agentId: "a1" }));
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registry, "a1");
    const component = await opened.componentReady;
    assert.equal(component.getMode(), "view");

    component.handleInput("\x1b"); // a single Esc — no second interaction is needed to actually close.
    assert.equal(await promise, undefined);
  });

  test("Enter in view mode raises the viewer to interactive via setMode — the record is left untouched", async () => {
    const registry = registryOf(record({ agentId: "a1" }));
    const before = JSON.stringify([...registry.values()]);
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registry, "a1");
    const component = await opened.componentReady;

    component.handleInput("\r");
    assert.equal(component.getMode(), "interactive");
    assert.equal(JSON.stringify([...registry.values()]), before, "raising to interactive never mutates the registry record");

    opened.close();
    await promise;
  });

  test("opening a dormant child resumes nothing — the viewer builds with no live client attached", async () => {
    const registry = registryOf(record({ agentId: "a1" })); // no client: dormant
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registry, "a1");
    const component = await opened.componentReady;
    assert.equal(component.getMode(), "view");
    opened.close();
    await promise;
  });

  test("interactive Esc opens a width-safe action modal; Ctrl+C is swallowed and Esc cancels it", async () => {
    const registry = registryOf(record({ agentId: "a1" }));
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registry, "a1");
    const component = await opened.componentReady;
    component.handleInput("\r");
    component.handleInput("\x1b");
    for (const width of [40, 80, 120]) {
      const lines = component.render(width);
      assert.match(lines.join("\n"), /\[hold\].*\[finish\].*\[interrupt\]/);
      for (const line of lines) assert.ok(visibleWidth(line) <= width);
    }
    component.handleInput("\x1b[C");
    component.handleInput("\x1b[C");
    component.handleInput("\r");
    assert.match(component.render(80).join("\n"), /Leave owner steering/, "disabled interrupt is a no-op");
    component.handleInput("\x03");
    assert.match(component.render(80).join("\n"), /Leave owner steering/);
    component.handleInput("\x1b");
    assert.equal(component.getMode(), "interactive");
    opened.close();
    await promise;
  });

  test("hold is the default modal action and closes without changing ownership", async () => {
    const held = record({ agentId: "a1", lastWriter: "owner" });
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registryOf(held), "a1");
    const component = await opened.componentReady;
    component.handleInput("\r");
    component.handleInput("\x1b");
    component.handleInput("\r");
    await promise;
    assert.equal(held.lastWriter, "owner");
  });

  test("interrupt aborts only a running turn, preserves owner ownership, and leaves the view open", async () => {
    let aborted = 0;
    const live = record({
      agentId: "a1",
      running: true,
      streaming: true,
      lastWriter: "owner",
      client: { onEvent: () => () => {}, abort: async () => { aborted++; } } as never,
    });
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registryOf(live), "a1");
    const component = await opened.componentReady;
    component.handleInput("\r");
    component.handleInput("\x1b");
    component.handleInput("\x1b[C");
    component.handleInput("\x1b[C");
    component.handleInput("\r");
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(aborted, 1);
    assert.equal(live.lastWriter, "owner");
    assert.equal(component.getMode(), "interactive");
    opened.close();
    await promise;
  });

  test("finish sends one lead-attributed handoff only for owner-held work and closes", async () => {
    const followed: string[] = [];
    const live = record({
      agentId: "a1",
      running: true,
      streaming: true,
      lastWriter: "owner",
      client: {
        onEvent: () => () => {},
        followUp: async (text: string) => { followed.push(text); },
        steer: async () => {},
      } as never,
    });
    const registry = registryOf(live);
    const opened = fakeViewerCtx();
    const promise = openViewer(opened.ctx as never, registry, "a1", { pi: {} as ExtensionAPI, cwd: process.cwd(), extensionPath: "test-extension.ts" });
    const component = await opened.componentReady;
    component.handleInput("\r");
    component.handleInput("\x1b");
    component.handleInput("\x1b[C");
    component.handleInput("\r");
    await promise;
    assert.equal(followed.length, 1);
    assert.match(followed[0]!, /owner has finished steering/i);
    assert.equal(live.lastWriter, "lead");
  });

  test("a second /audit closes the first overlay — the first agent's own record/session is untouched", async () => {
    const registry = registryOf(record({ agentId: "a1" }), record({ agentId: "a2" }));

    const first = fakeViewerCtx();
    const firstPromise = openViewer(first.ctx as never, registry, "a1");
    await first.componentReady; // the first overlay is now the active singleton

    const second = fakeViewerCtx();
    const secondPromise = openViewer(second.ctx as never, registry, "a2");
    await second.componentReady;

    // Opening the second overlay closed the first one on its own — no Esc needed on it.
    assert.equal(await firstPromise, undefined);

    second.close();
    assert.equal(await secondPromise, undefined);
  });
});
