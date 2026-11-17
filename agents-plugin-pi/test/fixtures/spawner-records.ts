import {
  leadIdleRef,
  pushToLead,
  registerPushFlush,
  type RpcAgentRecord,
} from "../../src/spawner.ts";
// 260906 (registerAgentTools's role-keyed explore registration): a VALUE
// import, not `import type` — the new describe block below monkey-patches
// RpcClient.prototype.{start,onEvent,prompt} for the lead/fork execute() path
// so the real spawnAgent code runs to completion with no real subprocess
// (see that describe block's own doc comment for why this is safe and the
// worker/execute-worker leaf path deliberately never invokes execute() at all).
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { PUSH_BATCH_CUSTOM_TYPE } from "../../src/push-protocol.ts";

export function freshRpcRecord(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId: "rpc-agent-1",
    sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
    systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
    wsToolNames: [],
    toolGroup: "full-worker",
    streaming: false,
    running: false,
    reportLog: [],
    ...overrides,
  };
}

/**
 * A record in the LIVE resting state. 260905 (alias/park/cap ticket):
 * presence no longer depends on `client` — any non-threadBound registry
 * member (dormant/parked included) keeps `computeRunningStatusLine`'s line
 * present — but most fan-in fixtures below still want a live client to
 * exercise `running`/`streaming` transitions. Never a real `RpcClient`.
 */
export function liveRpcRecord(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return freshRpcRecord({ client: {} as RpcClient, ...overrides });
}

/**
 * Duck-typed `ExtensionAPI` stand-in exposing only the push methods. `sent`
 * is the item-level delivery view used by the lifecycle assertions below:
 * a `ws-push-batch` is flattened back to its structured `details.items` while
 * dedicated push-wake tests assert the real one-envelope transport.
 */
export function fakePi(overrides: { sendMessage?: (message: unknown, options?: unknown) => void } = {}): {
  api: Parameters<typeof pushToLead>[0];
  wakes: unknown[];
  sent: Array<{ message: { customType?: string; content?: string; display?: boolean; details?: unknown }; options?: { deliverAs?: string; triggerTurn?: boolean } }>;
} {
  const sent: Array<{ message: { customType?: string; content?: string; display?: boolean; details?: unknown }; options?: { deliverAs?: string; triggerTurn?: boolean } }> = [];
  const wakes: unknown[] = [];
  leadIdleRef.current ??= () => true;
  const handlers = new Map<string, () => void>();
  const api = {
    on: (event: string, handler: () => void) => handlers.set(event, handler),
    sendUserMessage: (content: unknown) => {
      wakes.push(content);
      const accessor = leadIdleRef.current;
      leadIdleRef.current = () => false;
      handlers.get("agent_start")?.();
      leadIdleRef.current = accessor;
    },
    sendMessage:
      overrides.sendMessage ??
      ((message: unknown, options?: unknown) => {
        const candidate = message as { customType?: string; details?: { items?: unknown[] } };
        if (candidate.customType === PUSH_BATCH_CUSTOM_TYPE && Array.isArray(candidate.details?.items)) {
          for (const item of candidate.details.items) sent.push({ message: item as never, options: options as never });
        } else sent.push({ message: message as never, options: options as never });
      }),
  };
  registerPushFlush(api as never, { delayMs: () => 10 });
  return { api: api as unknown as Parameters<typeof pushToLead>[0], sent, wakes };
}

/**
 * Duck-typed `RpcClient` stand-in exposing only the methods these tests
 * drive (`steer`/`followUp`/`prompt`/`getLastAssistantText`), cast as
 * `RpcClient` — mirroring the existing `client: {} as RpcClient` pattern
 * already used by the `listAgents` tests above. Never a real `RpcClient`
 * construction, never a subprocess.
 */
export function fakeRpcClient(overrides: {
  steer?: (message: string) => Promise<void>;
  followUp?: (message: string) => Promise<void>;
  prompt?: (message: string) => Promise<void>;
  getLastAssistantText?: () => Promise<string | null>;
} = {}): { client: RpcClient; calls: Array<[string, string]> } {
  const calls: Array<[string, string]> = [];
  const client = {
    steer: overrides.steer ?? (async (message: string) => void calls.push(["steer", message])),
    followUp: overrides.followUp ?? (async (message: string) => void calls.push(["followUp", message])),
    prompt: overrides.prompt ?? (async (message: string) => void calls.push(["prompt", message])),
    getLastAssistantText: overrides.getLastAssistantText ?? (async () => null),
  };
  return { client: client as unknown as RpcClient, calls };
}
