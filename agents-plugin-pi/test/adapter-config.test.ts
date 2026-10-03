/**
 * Unit tests for adapter-config.ts: the shipped `pi.*` manifest stays in
 * step with the adapter's built-in defaults, and the ws-mcp reader turns
 * `config.get` answers into a config object without ever failing. Plus the
 * wake-recovery timer's asynchronous delay read (spawner.ts).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, afterEach } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  ADAPTER_CONFIG_MANIFEST_FILE,
  ADAPTER_CONFIG_NAMESPACE,
  createWsConfigReader,
  staticConfigReader,
  thenOrNow,
  type GoalLoopConfigKey,
} from "../src/adapter-config.ts";
import {
  COMPACTION_BUDGET_CONFIG_KEYS,
  COMPACTION_TRIGGER_CONFIG_KEYS,
  SETTLE_CONFIG_KEYS,
  resolveAgentWaitAnimation,
  resolveChildRetentionTtlDays,
  resolveCompactionAdvisoryPercent,
  resolveCompactionHardPercent,
  resolveContextWindowOverride,
  resolveRunawayThreshold,
  resolveSettleDelayMs,
  resolveDialogBudgetBytes,
  type GoalLoopConfig,
} from "../src/goal-loop.ts";
import { clearWakeStart, leadWakeStartPendingRef, reserveWakeStart } from "../src/spawner.ts";

interface ManifestKey { key: string; type: string; default: unknown }
const manifest = JSON.parse(readFileSync(join(process.cwd(), ADAPTER_CONFIG_MANIFEST_FILE), "utf8")) as { namespace: string; keys: ManifestKey[] };

/** Every knob the adapter reads, with the resolver that turns it into behavior. */
const resolvers: Record<GoalLoopConfigKey, (config: GoalLoopConfig | undefined) => unknown> = {
  agent_wait_animation: resolveAgentWaitAnimation,
  runaway_threshold: resolveRunawayThreshold,
  settle_delay_ms: resolveSettleDelayMs,
  compaction_advisory_percent: resolveCompactionAdvisoryPercent,
  compaction_hard_percent: resolveCompactionHardPercent,
  context_window_override: resolveContextWindowOverride,
  compaction_dialog_budget_bytes: resolveDialogBudgetBytes,
  child_retention_ttl_days: resolveChildRetentionTtlDays,
};

describe("shipped adapter manifest", () => {
  test("declares exactly the knobs the adapter reads, under its namespace", () => {
    assert.equal(manifest.namespace, ADAPTER_CONFIG_NAMESPACE);
    const declared = manifest.keys.map((k) => k.key).sort();
    assert.deepEqual(declared, Object.keys(resolvers).map((k) => `${ADAPTER_CONFIG_NAMESPACE}${k}`).sort());
    const read = new Set<GoalLoopConfigKey>([...SETTLE_CONFIG_KEYS, ...COMPACTION_TRIGGER_CONFIG_KEYS, ...COMPACTION_BUDGET_CONFIG_KEYS, "agent_wait_animation", "child_retention_ttl_days"]);
    assert.deepEqual([...read].sort(), Object.keys(resolvers).sort());
  });

  test("each manifest default resolves exactly like an untuned knob", () => {
    // ws-mcp answers an untuned key with the manifest default, while an
    // unreachable ws-mcp leaves the knob absent; both must behave the same.
    for (const { key, default: value } of manifest.keys) {
      const knob = key.slice(ADAPTER_CONFIG_NAMESPACE.length) as GoalLoopConfigKey;
      const resolve = resolvers[knob];
      assert.deepEqual(resolve({ [knob]: value } as GoalLoopConfig), resolve(undefined), key);
    }
  });
});

describe("createWsConfigReader", () => {
  function fakeClient(answer: (args: Record<string, unknown>) => unknown) {
    const calls: Array<{ name: string; args: Record<string, unknown> }> = [];
    return {
      calls,
      client: {
        async callTool(name: string, args: Record<string, unknown>) {
          calls.push({ name, args });
          const result = answer(args);
          if (result instanceof Error) throw result;
          return result as { content: Array<{ type: "text"; text: string }>; isError?: boolean };
        },
      },
    };
  }
  const json = (value: unknown) => ({ content: [{ type: "text" as const, text: JSON.stringify(value) }] });

  test("reads each requested knob as a typed value through config.get", async () => {
    const { calls, client } = fakeClient((args) => json({ key: args.key, value: args.key === "pi.settle_delay_ms" ? 1500 : false, scope: "project" }));
    const read = createWsConfigReader(() => ({ client, sessionKey: "lead-key" }));
    assert.deepEqual(await read(["settle_delay_ms", "agent_wait_animation"]), { settle_delay_ms: 1500, agent_wait_animation: false });
    assert.deepEqual(calls.map((c) => c.name), ["config.get", "config.get"]);
    assert.deepEqual(calls[0]!.args, { key: "pi.settle_delay_ms", format: "json", session_key: "lead-key" });
  });

  test("an unset, failed, malformed, or slow read leaves the knob absent", async () => {
    const answers: Record<string, unknown> = {
      "pi.runaway_threshold": json({ key: "pi.runaway_threshold", value: null, scope: "unset" }),
      "pi.settle_delay_ms": { content: [{ type: "text", text: "config.get: boom" }], isError: true },
      "pi.compaction_hard_percent": new Error("ws-mcp exited"),
      "pi.compaction_advisory_percent": { content: [{ type: "text", text: "not json" }] },
    };
    const { client } = fakeClient((args) => answers[String(args.key)]);
    const slow = { callTool: (name: string, args: Record<string, unknown>) => args.key === "pi.context_window_override" ? new Promise<never>(() => {}) : client.callTool(name, args) };
    const read = createWsConfigReader(() => ({ client: slow }), 10);
    assert.deepEqual(await read(["runaway_threshold", "settle_delay_ms", "compaction_hard_percent", "compaction_advisory_percent", "context_window_override"]), {});
  });

  test("no bridge reads nothing", async () => {
    assert.deepEqual(await createWsConfigReader(() => undefined)(["settle_delay_ms"]), {});
  });

  test("a static reader answers synchronously with only the requested knobs", () => {
    const read = staticConfigReader({ settle_delay_ms: 7, runaway_threshold: 2 });
    assert.deepEqual(read(["settle_delay_ms"]), { settle_delay_ms: 7 });
    assert.equal(thenOrNow(read(["settle_delay_ms"]), (c) => c.settle_delay_ms), 7);
  });
});

describe("wake-recovery delay read", () => {
  afterEach(() => clearWakeStart());

  function fakeTimers() {
    const scheduled: number[] = [];
    let cleared = 0;
    return {
      scheduled,
      cleared: () => cleared,
      scheduleTimer: (_cb: () => void, ms: number) => { scheduled.push(ms); return {} as NodeJS.Timeout; },
      clearTimer: () => { cleared += 1; },
    };
  }

  test("an asynchronous delay reserves at once and schedules when the read lands", async () => {
    const timers = fakeTimers();
    assert.equal(reserveWakeStart({ delayMs: async () => 1500, ...timers }, () => {}), true);
    assert.equal(leadWakeStartPendingRef.current, true);
    assert.deepEqual(timers.scheduled, []);
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(timers.scheduled, [1500]);
  });

  test("a cancel during the read schedules nothing", async () => {
    const timers = fakeTimers();
    reserveWakeStart({ delayMs: async () => 1500, ...timers }, () => {});
    clearWakeStart();
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(timers.scheduled, []);
  });
});
