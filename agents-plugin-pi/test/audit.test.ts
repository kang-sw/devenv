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
  openViewer,
  parseSessionFile,
  registerAuditCommands,
  shouldRegisterAudit,
} from "../src/audit.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "../src/spawner.ts";
import type { ConversationViewComponent } from "../src/conversation-view.ts";

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
function fakeViewerCtx() {
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
          const built = factory({ requestRender: () => {} }, undefined, undefined, (v: unknown) => resolve(v));
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

  test("a missing/unreadable session file yields [] — never throws", () => {
    assert.deepEqual(parseSessionFile(join(tmpdir(), "ws-pi-audit-definitely-missing-", `${Date.now()}.jsonl`)), []);
  });

  test("an empty file yields []", () => {
    assert.deepEqual(parseSessionFile(fixturePath([])), []);
  });
});

describe("buildAuditPickerItems", () => {
  test("naming (alias > title > shortened uuid) and the four-tier ordering: awaiting-owner, awaiting-approval, running by elapsed desc, then dormant by last-activity desc", () => {
    const NOW = Date.parse("2026-09-09T10:00:00.000Z");
    const owner = record({ agentId: "owner-agent-id", alias: "scout", threadBound: true });
    const approval = record({ agentId: "appr-agent-id", title: "reviewer", pendingApproval: { cmdId: "c1", command: "rm -rf /" } });
    const runningOld = record({ agentId: "run-old-id", alias: "old-runner", client: {} as never, runStartedAt: NOW - 60_000 });
    const runningNew = record({ agentId: "run-new-id", alias: "new-runner", client: {} as never, runStartedAt: NOW - 5_000 });
    const dormantRecent = record({ agentId: "dorm-recent-id", alias: "recent-dormant", lastLeadPromptAt: NOW - 1_000 });
    const dormantOld = record({ agentId: "dorm-old-id", alias: "old-dormant", lastLeadPromptAt: NOW - 100_000 });

    // Insertion order deliberately scrambled — the function must sort, not preserve Map order.
    const registry = registryOf(dormantOld, runningOld, approval, dormantRecent, owner, runningNew);
    const items = buildAuditPickerItems(registry, NOW);

    // Running tier sorts by elapsed DESCENDING (longest-running first) —
    // `agent-widget.ts`'s own `STATE_RANK`/elapsed convention, reused
    // verbatim: `run-old-id` has been running 60s, `run-new-id` only 5s.
    assert.deepEqual(items, [
      { value: "owner-agent-id", label: "scout · awaiting owner" },
      { value: "appr-agent-id", label: "reviewer · awaiting approval" },
      { value: "run-old-id", label: "old-runner · running" },
      { value: "run-new-id", label: "new-runner · running" },
      { value: "dorm-recent-id", label: "recent-dormant · dormant" },
      { value: "dorm-old-id", label: "old-dormant · dormant" },
    ]);
  });

  test("a plain idle record with no client/threadBound/pendingApproval is dormant (tier 4), never one of the first three tiers", () => {
    const NOW = Date.parse("2026-09-09T10:00:00.000Z");
    const idle = record({ agentId: "idle-1" });
    const items = buildAuditPickerItems(registryOf(idle), NOW);
    assert.deepEqual(items, [{ value: "idle-1", label: "idle-1 · dormant" }]);
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
    const registry = registryOf(record({ agentId: "a1", client: client as never, streaming: true }));
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

  test("liveness delegates to resolveChildLiveness off record.streaming, read fresh each call", () => {
    const r = record({ agentId: "a1", streaming: false });
    const registry = registryOf(r);
    const channel = createAuditChannel(registry, "a1");
    assert.equal(channel.liveness(), "settled");
    r.streaming = true;
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

    // Reads the constructed `SelectList`'s own `items` field directly rather
    // than `render()`ing it — `render()` reaches into the real
    // `getSelectListTheme()`'s host theme, which is never initialized under
    // `node --test` (no live TUI host); comparing the underlying item list
    // proves "the same picker" without that unrelated dependency.
    async function capture(invoke: (ctx: unknown) => Promise<void>): Promise<unknown> {
      let items: unknown;
      const ctx = {
        mode: "tui",
        ui: {
          notify: () => {},
          custom: (factory: (...args: unknown[]) => unknown) =>
            new Promise((resolve) => {
              const built = factory({ requestRender: () => {} }, undefined, undefined, resolve);
              Promise.resolve(built).then((component) => {
                items = (component as { items?: unknown }).items;
                (component as { onCancel?: () => void }).onCancel?.();
              });
            }),
        },
      };
      await invoke(ctx);
      return items;
    }

    const fromCommand = await capture((ctx) => commands.get("audit")!.handler("", ctx));
    const fromShortcut = await capture((ctx) => shortcuts.get("ctrl+shift+u")!.handler(ctx));
    assert.deepEqual(fromCommand, fromShortcut);
    assert.deepEqual(fromCommand, [{ value: "a1", label: "scout · running" }]);
  });
});

describe("openViewer (the read-only overlay: Esc/Enter contract and the one-overlay-at-a-time singleton)", () => {
  test("Esc closes the viewer directly, with no confirmation modal in the way", async () => {
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
