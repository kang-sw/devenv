/**
 * Pi adapter settings read through ws-mcp's `config.get`.
 *
 * The adapter declares its knobs in `config-manifest.json` (namespace `pi.`);
 * the bridge points ws-mcp at that manifest through
 * `WS_MCP_CONFIG_MANIFESTS`, so a lead tunes them with `config.tune` like any
 * other ws setting and ws-mcp validates every write against the declared
 * type. This module is the read side: one `config.get` per requested knob,
 * at each use, with no adapter-side cache, so a tuned value takes effect at
 * the next read without a restart.
 *
 * Reads never fail. A knob whose read errors, times out, or arrives with no
 * bridge is simply absent from the returned config, and every `resolve*`
 * function in `goal-loop.ts` maps an absent knob to its built-in default; the
 * manifest's `default` values mirror those constants (a test pins them
 * equal), so "ws-mcp unreachable" and "nothing tuned" resolve identically.
 */

import type { GoalLoopConfig } from "./goal-loop.ts";
import type { McpToolCallResult } from "./mcp-stdio-client.ts";

/** Package-relative manifest path; the bridge resolves it against the plugin directory. */
export const ADAPTER_CONFIG_MANIFEST_FILE = "config-manifest.json";

/** Prefix every adapter knob carries in ws config. */
export const ADAPTER_CONFIG_NAMESPACE = "pi.";

/** The ws-mcp launcher variable naming adapter manifests (a path list). */
export const WS_MCP_CONFIG_MANIFESTS_ENV = "WS_MCP_CONFIG_MANIFESTS";

/** Bound on one `config.get` round trip; a slower answer degrades to the default. */
export const CONFIG_READ_TIMEOUT_MS = 2000;

export type GoalLoopConfigKey = keyof GoalLoopConfig;

/**
 * Reads the named knobs. A synchronous result is allowed so tests (and any
 * caller holding a fixed config) keep the state machine fully synchronous;
 * the ws-mcp reader is asynchronous. Must never throw or reject.
 */
export type GoalLoopConfigReader = (keys: readonly GoalLoopConfigKey[]) => GoalLoopConfig | Promise<GoalLoopConfig>;

/** The bridge pieces a read needs; `undefined` means no live bridge (defaults apply). */
export interface ConfigGetTarget {
  client: { callTool(name: string, args: Record<string, unknown>): Promise<McpToolCallResult> };
  sessionKey?: string;
}

/** A reader that always answers `config` (or nothing: every knob at its default). */
export function staticConfigReader(config: GoalLoopConfig = {}): GoalLoopConfigReader {
  return (keys) => {
    const out: GoalLoopConfig = {};
    for (const key of keys) {
      if (config[key] !== undefined) (out as Record<string, unknown>)[key] = config[key];
    }
    return out;
  };
}

/** Runs `fn` now on a plain value, or after a promise resolves. Keeps sync readers sync end to end. */
export function thenOrNow<T, R>(value: T | Promise<T>, fn: (resolved: T) => R): R | Promise<R> {
  return value instanceof Promise ? value.then(fn) : fn(value);
}

/**
 * The production reader: one `config.get` per knob through the bridge's
 * already-connected ws-mcp client, read fresh on every call. `target` is
 * consulted per call so a bridge that starts, restarts, or adopts a new
 * session key is picked up without re-creating the reader.
 */
export function createWsConfigReader(
  target: () => ConfigGetTarget | undefined,
  timeoutMs: number = CONFIG_READ_TIMEOUT_MS,
): GoalLoopConfigReader {
  return async (keys) => {
    const current = target();
    if (!current) return {};
    const out: Record<string, unknown> = {};
    await Promise.all(keys.map(async (key) => {
      const value = await readOne(current, key, timeoutMs);
      if (value !== undefined) out[key] = value;
    }));
    return out as GoalLoopConfig;
  };
}

async function readOne(target: ConfigGetTarget, key: GoalLoopConfigKey, timeoutMs: number): Promise<unknown> {
  const args: Record<string, unknown> = { key: `${ADAPTER_CONFIG_NAMESPACE}${key}`, format: "json" };
  if (target.sessionKey) args.session_key = target.sessionKey;
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const timeout = new Promise<undefined>((resolve) => {
      timer = setTimeout(() => resolve(undefined), timeoutMs);
      timer.unref?.();
    });
    const result = await Promise.race([target.client.callTool("config.get", args), timeout]);
    if (!result || result.isError) return undefined;
    const text = result.content.find((item) => item.type === "text")?.text;
    if (!text) return undefined;
    const parsed = JSON.parse(text) as { value?: unknown };
    return parsed.value === null ? undefined : parsed.value;
  } catch {
    return undefined;
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}
